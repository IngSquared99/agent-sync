// Package build implements the build phase: scan → candidate list → name-conflict handling → routing → copy → manifest.
package build

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/IngSquared99/agent-sync/i18n"
	"github.com/IngSquared99/agent-sync/internal/config"
	"github.com/IngSquared99/agent-sync/internal/yaml"
)

// Item is one entry of the candidate list: in-memory intermediate data of the build
type Item struct {
	Category  string   // rules / skills / workflows
	Name      string   // original file or directory name
	OutName   string   // final name after conflict handling (equals Name when there is no conflict)
	From      string   // absolute source path
	SourceIdx int      // index of the source in the sources array (0-based; order is priority)
	SourceTag string   // source tag (used by rename)
	IsDir     bool     // skills are directories, everything else is a single file
	Tools     []string // workflows: effective tool targets (empty only while unresolved)
	SkillName string   // workflows: name of the derived skill directory
	Renamed   bool     // renamed due to a conflict
	RouteNote string   // routing note shown by plan
}

// Ignored is a file skipped during scanning: not an error, but it must be
// reported — silent discarding is not allowed
type Ignored struct {
	Category string
	Name     string
	From     string
	Reason   string
}

// ManifestName is the manifest file name, located under build.out. The
// .agsy- prefix is part of the file name only; paths are always derived
// from build.out.
const ManifestName = ".agsy-manifest.json"

// MaxMDBytes is the acceptance limit for single-file items (rules and
// workflows). Skills are exempt: a skill directory may legitimately carry
// large assets alongside its SKILL.md.
const MaxMDBytes = 5 << 20

// ManifestVersion is the manifest format version this agsy writes / understands.
// Distinct from agsy.yaml's version: that one is the config format, this one is the
// record-file format.
const ManifestVersion = 1

// OutEntry is one output produced for an item. Derived marks conversions
// (workflow→skill, stub, AGENTS.md) as opposed to verbatim copies; every
// output is read-only either way — the distinction only refines the guidance
// status prints when the file is modified on the artifact side.
type OutEntry struct {
	Path    string            `json:"path"` // relative to the out directory
	Hash    string            `json:"hash"`
	Files   map[string]string `json:"files,omitempty"` // per-file hashes for directory outputs
	Derived bool              `json:"derived,omitempty"`
}

type ManifestItem struct {
	Category string            `json:"category"`            // rules / skills / workflows / agents-md
	Name     string            `json:"name"`                // final name in the output
	Original string            `json:"original"`            // original name
	From     string            `json:"from"`                // source path (empty for agents-md)
	SrcHash  string            `json:"src_hash"`            // source hash at build time (staleness baseline)
	SrcFiles map[string]string `json:"src_files,omitempty"` // per-file hashes on the source side
	Outs     []OutEntry        `json:"outs"`
	Tools    []string          `json:"tools,omitempty"` // workflows: effective tool targets
	Renamed  bool              `json:"renamed,omitempty"`
}

type Manifest struct {
	Version int            `json:"version"`
	BuiltAt string         `json:"built_at"`
	Sources []string       `json:"sources"`
	Items   []ManifestItem `json:"items"`
	// Mounts records the link paths created by the last apply. Its only use is
	// detecting orphans: links agsy created earlier that the current mount
	// config no longer references (tools keep reading them while status would
	// otherwise claim all green). Untrusted like the rest of the manifest —
	// consumers must verify a path IS a link into the output before touching it.
	Mounts []string `json:"mounts,omitempty"`
}

// SourceState is the result of expanding a source
type SourceState struct {
	Raw      string // original spelling from the config file
	Abs      string // expanded absolute path
	Exists   bool
	Tag      string
	Warnings []string // e.g. "missing workflow/ subdirectory"
}

// Conflict is a same-name conflict (reported under the error strategy)
type Conflict struct {
	Category string
	Name     string
	Froms    []string
}

// Collision means rename / a naming coincidence made two *final output names* collide.
// Silently overwriting would destroy one of the copies, so it is always blocked.
type Collision struct {
	Category string
	OutName  string
	Froms    []string
}

// Plan is the complete computed result of one build (shared by plan and apply;
// plan only computes, never writes)
type Plan struct {
	Sources     []SourceState
	Items       []Item
	Conflicts   []Conflict  // non-empty when on_conflict=error and names clash
	Collisions  []Collision // final output names collide
	Skipped     []Item      // items dropped by the first strategy
	Ignored     []Ignored   // files skipped during scanning for not matching the acceptance rules
	RouteErrors []string    // per-file target problems (broken front matter, unknown tool); collected so plan lists them all at once
	Incomplete  bool        // some source does not exist; the preview is incomplete
}

// Accepts is the single source of truth for the acceptance rules.
// The build scan and the doctor stats share the same judgment so their numbers
// can never disagree.
// It takes a path rather than a file name: skills need a peek into the directory
// for a SKILL.md.
func Accepts(cat, path string, isDir bool) (bool, string) {
	name := filepath.Base(path)
	if strings.HasPrefix(name, ".") {
		return false, i18n.T("starts with .")
	}
	// Symbolic links are never collected: build copies file *contents*, so a
	// link inside a (possibly shared) source could smuggle arbitrary files from
	// outside it — e.g. a "rule" pointing at a private key — straight into the
	// mounted output. Lstat inspects the entry itself instead of following it.
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return false, i18n.T("symbolic links are not collected")
		}
		// Irregular files (FIFOs, sockets, devices) are never collected
		// either: opening a named pipe for reading blocks forever, so one
		// planted in a half-trusted source would hang the build. openNoFollow
		// re-checks at open time; rejecting here also reports the file in
		// plan and doctor instead of failing mid-apply.
		if !fi.IsDir() && !fi.Mode().IsRegular() {
			return false, i18n.T("not a regular file (FIFO, socket or device); it cannot be collected")
		}
	}
	if cat == "skills" {
		if !isDir {
			return false, i18n.T("skills are directories; a single file is not accepted")
		}
		// A directory without SKILL.md is not a skill (assets, scratch, drafts).
		// Accepting it would mount a skill with no description that can never
		// trigger.
		if st, err := os.Stat(filepath.Join(path, "SKILL.md")); err != nil || st.IsDir() {
			return false, i18n.T("skill directory has no SKILL.md")
		}
		// The directory itself is real, but any entry inside could still be a
		// link; reject the whole skill so nothing smuggled reaches the output.
		if rel, found := FirstSymlinkWithin(path); found {
			return false, fmt.Sprintf(i18n.T("contains a symbolic link (%s); the skill is not collected"), rel)
		}
		// Same rule for irregular entries: a FIFO inside a skill would hang
		// the copy (and any tool later reading the mount), so the whole
		// skill is rejected rather than silently dropping one file.
		if rel, found := FirstIrregularWithin(path); found {
			return false, fmt.Sprintf(i18n.T("contains an irregular file (%s, e.g. a FIFO); the skill is not collected"), rel)
		}
		return true, ""
	}
	if isDir {
		return false, fmt.Sprintf(i18n.T("%s entries are single .md files; a directory is not accepted"), cat)
	}
	if !strings.HasSuffix(name, ".md") {
		return false, i18n.T("extension is not .md")
	}
	// An instruction file this large is never useful as context and would
	// bloat every AGENTS.md concatenation; refusing it here also caps the
	// work a single runaway file can cause.
	if fi, err := os.Stat(path); err == nil && fi.Size() > MaxMDBytes {
		return false, fmt.Sprintf(i18n.T("file is %d MB, over the %d MB limit for instruction files"), fi.Size()>>20, MaxMDBytes>>20)
	}
	return true, ""
}

// ExpandSources expands all sources, checks their existence, and assigns
// mutually unique source tags
func ExpandSources(cfg *config.Config) ([]SourceState, error) {
	var out []SourceState
	for _, raw := range cfg.Sources {
		abs, err := cfg.ExpandPath(raw)
		if err != nil {
			return nil, err
		}
		st := SourceState{Raw: raw, Abs: abs}
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			st.Exists = true
		}
		out = append(out, st)
	}
	AssignTags(out)
	return out, nil
}

// AssignTags assigns unique source tags.
// Start with the last path segment; when several sources clash (~/x/.flow and
// ./.flow are both "flow") merge in parent directory names level by level, and
// only fall back to appending a number if they are still identical. If tags are
// not unique, renamed outputs would overwrite each other and the "both copies
// are kept" promise would silently break.
func AssignTags(sources []SourceState) {
	tags := make([]string, len(sources))
	for i, s := range sources {
		tags[i] = config.SourceTag(s.Abs)
	}
	for depth := 1; depth <= 4; depth++ {
		dup := map[string][]int{}
		for i, t := range tags {
			dup[t] = append(dup[t], i)
		}
		clash := false
		for _, idxs := range dup {
			if len(idxs) < 2 {
				continue
			}
			clash = true
			for _, i := range idxs {
				tags[i] = tagWithParents(sources[i].Abs, depth+1)
			}
		}
		if !clash {
			break
		}
	}
	// Still duplicated (e.g. two source paths differ only further up) → append a
	// number as the last resort. Each candidate is checked against every tag
	// finalized so far: blindly appending could itself collide with another
	// source's original tag (x, x, x2 → the second x must not become x2).
	seen := map[string]bool{}
	for i, t := range tags {
		if !seen[t] {
			seen[t] = true
			continue
		}
		for n := 2; ; n++ {
			cand := t + strconv.Itoa(n)
			if !seen[cand] {
				tags[i] = cand
				seen[cand] = true
				break
			}
		}
	}
	for i := range sources {
		sources[i].Tag = tags[i]
	}
}

// tagWithParents builds a tag from the last n path segments: /a/b/.flow at depth 2 → b-flow
func tagWithParents(abs string, n int) string {
	parts := strings.Split(filepath.ToSlash(filepath.Clean(abs)), "/")
	var keep []string
	for i := len(parts) - 1; i >= 0 && len(keep) < n; i-- {
		seg := strings.TrimLeft(parts[i], ".")
		if seg == "" {
			continue
		}
		keep = append([]string{seg}, keep...)
	}
	return strings.Join(keep, "-")
}

// Compute runs the scan, conflict handling and routing, producing the full plan
// (writes nothing)
func Compute(cfg *config.Config, sources []SourceState) (*Plan, error) {
	p := &Plan{Sources: sources}
	// 1. scan → candidate list
	for idx, src := range sources {
		if !src.Exists {
			p.Incomplete = true
			continue
		}
		for _, cat := range config.CategoryOrder {
			cc := cfg.Build.Categories[cat]
			dir := filepath.Join(src.Abs, cc.From)
			entries, err := os.ReadDir(dir)
			if err != nil {
				// A source missing this category subdirectory is normal (⚠ not ✘); don't block
				continue
			}
			for _, e := range entries {
				name := e.Name()
				isDir := e.IsDir()
				ok, reason := Accepts(cat, filepath.Join(dir, name), isDir)
				if !ok {
					if !strings.HasPrefix(name, ".") { // hidden files are not reported
						p.Ignored = append(p.Ignored, Ignored{
							Category: cat, Name: name,
							From: filepath.Join(dir, name), Reason: reason,
						})
					}
					continue
				}
				p.Items = append(p.Items, Item{
					Category: cat, Name: name, OutName: name,
					From: filepath.Join(dir, name), SourceIdx: idx,
					SourceTag: src.Tag, IsDir: isDir,
				})
			}
		}
	}
	// 2. name-conflict handling
	if err := resolveConflicts(cfg, p); err != nil {
		return nil, err
	}
	// 3. workflow target filtering and derived-name assignment
	if err := filterWorkflows(cfg, p); err != nil {
		return nil, err
	}
	// 4. final output-path uniqueness (collisions are still possible after
	// rename, and a workflow-derived skill can clash with a real skill)
	detectCollisions(cfg, p)
	return p, nil
}

func resolveConflicts(cfg *config.Config, p *Plan) error {
	groups := map[string][]int{} // "cat/name" → item indexes
	for i, it := range p.Items {
		key := it.Category + "/" + it.Name
		groups[key] = append(groups[key], i)
	}
	var keep []Item
	handled := map[int]bool{}
	// process in original order to keep the output stable
	for i, it := range p.Items {
		if handled[i] {
			continue
		}
		key := it.Category + "/" + it.Name
		idxs := groups[key]
		if len(idxs) == 1 {
			keep = append(keep, it)
			handled[i] = true
			continue
		}
		strategy := cfg.Build.OnConflict[it.Category]
		switch strategy {
		case "first":
			// keep the one with the smallest source index (sources order is priority)
			minIdx := idxs[0]
			for _, j := range idxs {
				if p.Items[j].SourceIdx < p.Items[minIdx].SourceIdx {
					minIdx = j
				}
			}
			for _, j := range idxs {
				handled[j] = true
				if j == minIdx {
					keep = append(keep, p.Items[j])
				} else {
					p.Skipped = append(p.Skipped, p.Items[j])
				}
			}
		case "rename":
			// both sides of the conflict get a source tag
			for _, j := range idxs {
				handled[j] = true
				x := p.Items[j]
				x.OutName = tagged(x.Name, x.SourceTag, x.IsDir)
				x.Renamed = true
				keep = append(keep, x)
			}
		case "error":
			var froms []string
			for _, j := range idxs {
				handled[j] = true
				froms = append(froms, p.Items[j].From)
				keep = append(keep, p.Items[j]) // keep for the plan display
			}
			p.Conflicts = append(p.Conflicts, Conflict{Category: it.Category, Name: it.Name, Froms: froms})
		}
	}
	p.Items = keep
	return nil
}

// detectCollisions finds items whose final OUTPUT PATHS collide. Grouping by
// output path instead of name catches cross-category clashes too: a workflow
// derives a skill directory, so workflows/deploy.md and skills/deploy would
// both claim skills/deploy.
// Combinations already reported by on_conflict=error are not counted again.
func detectCollisions(cfg *config.Config, p *Plan) {
	reported := map[string]bool{}
	for _, c := range p.Conflicts {
		reported[c.Category+"/"+c.Name] = true
	}
	type slot struct {
		items []Item
		key   string
	}
	groups := map[string]*slot{}
	var order []string
	add := func(path string, it Item) {
		g, ok := groups[path]
		if !ok {
			g = &slot{key: it.Category + "/" + it.Name}
			groups[path] = g
			order = append(order, path)
		}
		g.items = append(g.items, it)
	}
	for _, it := range p.Items {
		for _, path := range outputPaths(cfg, it) {
			add(path, it)
		}
	}
	for _, path := range order {
		g := groups[path]
		if len(g.items) < 2 {
			continue
		}
		allReported := true
		for _, it := range g.items {
			if !reported[it.Category+"/"+it.Name] {
				allReported = false
			}
		}
		if allReported {
			continue
		}
		var froms []string
		for _, it := range g.items {
			froms = append(froms, it.From)
		}
		p.Collisions = append(p.Collisions, Collision{
			Category: g.items[0].Category, OutName: filepath.ToSlash(path), Froms: froms,
		})
	}
}

// outputPaths lists every output path (relative to out) an item will occupy.
func outputPaths(cfg *config.Config, it Item) []string {
	to := cfg.Build.Categories
	switch it.Category {
	case "workflows":
		var out []string
		if it.SkillName != "" {
			out = append(out, filepath.ToSlash(filepath.Join(to["skills"].To, it.SkillName)))
		}
		if hasStub(it) {
			out = append(out, filepath.ToSlash(filepath.Join(to["workflows"].To, it.OutName)))
		}
		return out
	default:
		return []string{filepath.ToSlash(filepath.Join(to[it.Category].To, it.OutName))}
	}
}

// tagged builds a name carrying a source tag, one uniform separator for
// every category: python-style.md + god-lib → python-style-fromlib-god-lib.md.
// "-fromlib-" stays inside the Agent Skills name spec (lowercase a-z0-9 and
// hyphens); skill names are additionally sanitized into that character set.
func tagged(name, tag string, isDir bool) string {
	if isDir {
		return sanitizeSkillName(name + "-fromlib-" + tag)
	}
	ext := filepath.Ext(name)
	return strings.TrimSuffix(name, ext) + "-fromlib-" + tag + ext
}

// sanitizeSkillName forces a candidate skill name into the Agent Skills spec:
// lowercase a-z0-9 and hyphens, no leading/trailing/consecutive hyphens, at
// most 64 characters. Sanitized names can collide (God-Lib vs god-lib);
// detectCollisions catches that and blocks the build rather than overwrite.
func sanitizeSkillName(s string) string {
	var sb strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			sb.WriteByte('-')
			prevDash = true
		}
	}
	name := strings.Trim(sb.String(), "-")
	if len(name) > 64 {
		name = strings.Trim(name[:64], "-")
	}
	if name == "" {
		name = "skill"
	}
	return name
}

// frontMatter parses the YAML block wrapped in --- at the top of a Markdown file
func frontMatter(path string) (map[string]interface{}, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := strings.TrimPrefix(string(raw), bomPrefix)
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return nil, nil
	}
	rest := s[strings.Index(s, "\n")+1:]
	// The closing delimiter must be a standalone line: a "\n---" substring
	// search mistakes horizontal rules in the body for the close, and an
	// unclosed block would have its body parsed as YAML.
	var body []string
	closed := false
	for _, line := range strings.Split(rest, "\n") {
		if strings.TrimRight(line, "\r") == "---" {
			closed = true
			break
		}
		body = append(body, line)
	}
	if !closed {
		return nil, nil // no standalone closing line → not front matter
	}
	m := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(strings.Join(body, "\n")), &m); err != nil {
		return nil, fmt.Errorf(i18n.T("front-matter parse failed: %w"), err)
	}
	return m, nil
}

// bomPrefix is the UTF-8 byte-order mark. Some Windows editors prepend it;
// front-matter parsing, rule concatenation and name rewriting strip it so a
// BOM never silently disables target routing or leaks into derived files.
const bomPrefix = "\ufeff"

// TargetField is the front-matter key a workflow uses to limit which tools
// receive it (an agsy-specific field, stripped from the derived skill).
const TargetField = "target"

// StubTool is the tool that consumes the workflows/ stub form instead of the
// derived skill; every other tool reads workflows through the skills mount.
// The value lives in config so validation can cross-check it; this alias
// keeps existing references working.
const StubTool = config.StubTool

// filterWorkflows resolves each workflow's effective tool targets and its
// derived skill name. target: absent = every tool in build.tools; present =
// only the listed tools. The skills output is shared by several tools, so
// exclusion is coarse: the derived skill is produced whenever at least one
// non-stub tool is targeted, and plan notes the partial reach.
func filterWorkflows(cfg *config.Config, p *Plan) error {
	for i := range p.Items {
		it := &p.Items[i]
		if it.Category != "workflows" {
			continue
		}
		fm, err := frontMatter(it.From)
		if err != nil {
			// Collect instead of failing fast: one broken file must not hide the
			// rest of the plan preview or disable status's new-item detection.
			p.RouteErrors = append(p.RouteErrors, fmt.Sprintf("%s: %v", it.From, err))
			continue
		}
		var targets []string
		if fm != nil {
			if v, ok := fm["disable-model-invocation"].(bool); ok && !v {
				// WorkflowToSkill force-sets true; say so in plan instead of
				// silently overriding the author's explicit false.
				it.RouteNote = appendNote(it.RouteNote, i18n.T("front matter disable-model-invocation: false is ignored: workflow-derived skills are always human-triggered"))
			}
			switch v := fm[TargetField].(type) {
			case string:
				targets = []string{v}
			case []interface{}:
				for _, x := range v {
					if s, ok := x.(string); ok {
						targets = append(targets, s)
					}
				}
			}
		}
		if len(targets) == 0 {
			targets = append([]string{}, cfg.Build.Tools...)
		} else {
			bad := false
			for _, t := range targets {
				if !cfg.HasTool(t) {
					p.RouteErrors = append(p.RouteErrors, fmt.Sprintf(i18n.T("%s: %s refers to unknown tool %q, valid values: %v"), it.Name, TargetField, t, cfg.Build.Tools))
					bad = true
					break
				}
			}
			if bad {
				continue // Execute refuses while RouteErrors exist
			}
			if hasNonStubTool(targets) && len(targets) < len(cfg.Build.Tools) {
				it.RouteNote = appendNote(it.RouteNote, i18n.T("partial target note: the skills output is shared, every tool that mounts skills still sees this workflow"))
			}
		}
		it.Tools = targets
		if hasNonStubTool(targets) {
			base := strings.TrimSuffix(it.OutName, filepath.Ext(it.OutName))
			it.SkillName = sanitizeSkillName(base)
			if it.SkillName == "skill" && !strings.EqualFold(base, "skill") {
				it.RouteNote = appendNote(it.RouteNote, i18n.T("file name has no a-z0-9 characters, so the derived skill is named \"skill\"; rename the file to avoid collisions"))
			}
		}
	}
	return nil
}

// appendNote joins routing notes so multiple hints on one item all survive.
func appendNote(cur, note string) string {
	if cur == "" {
		return note
	}
	return cur + "; " + note
}

func hasNonStubTool(tools []string) bool {
	for _, t := range tools {
		if t != StubTool {
			return true
		}
	}
	return false
}

func hasStub(it Item) bool {
	return slices.Contains(it.Tools, StubTool)
}

// Placed reports how many items will actually be placed into the output.
func (p *Plan) Placed() int {
	return len(p.Items)
}

// Execute writes according to the Plan: empty out → copy verbatim forms →
// derive converted forms → ensure mount targets → write manifest.
// The caller (apply) must confirm artifact-side changes first; must not be
// called while conflicts (error strategy) exist.
func Execute(cfg *config.Config, p *Plan) (*Manifest, error) {
	if len(p.Conflicts) > 0 {
		return nil, fmt.Errorf(i18n.T("unresolved name conflicts exist, cannot build"))
	}
	if len(p.Collisions) > 0 {
		return nil, fmt.Errorf(i18n.T("final output names collide, cannot build"))
	}
	if len(p.RouteErrors) > 0 {
		return nil, fmt.Errorf(i18n.T("workflow target problems exist, cannot build"))
	}
	if p.Incomplete {
		return nil, fmt.Errorf(i18n.T("some source paths do not exist; apply must not rebuild from an incomplete source list (plan can still preview)"))
	}
	out := cfg.OutDir()
	// empty out (deletion logic shared with clean: out is entirely tool-built, safe to delete)
	if err := RemoveOut(cfg); err != nil {
		return nil, err
	}
	m := &Manifest{Version: ManifestVersion, BuiltAt: time.Now().Format(time.RFC3339)}
	for _, s := range p.Sources {
		m.Sources = append(m.Sources, s.Abs)
	}
	addOut := func(mi *ManifestItem, rel string, derived bool) error {
		full := filepath.Join(out, filepath.FromSlash(rel))
		h, files, err := HashPath(full)
		if err != nil {
			return err
		}
		mi.Outs = append(mi.Outs, OutEntry{Path: filepath.ToSlash(rel), Hash: h, Files: files, Derived: derived})
		return nil
	}
	for _, it := range p.Items {
		mi := ManifestItem{
			Category: it.Category, Name: it.OutName, Original: it.Name,
			From: it.From, Tools: it.Tools, Renamed: it.Renamed,
		}
		toBase := cfg.Build.Categories[it.Category].To
		switch it.Category {
		case "workflows":
			// Derived skill: the full content, invoked as /name where the tool
			// supports it, auto-loaded where it decides to.
			if it.SkillName != "" {
				rel := filepath.Join(cfg.Build.Categories["skills"].To, it.SkillName)
				if err := WorkflowToSkill(it.From, filepath.Join(out, rel), it.SkillName); err != nil {
					return nil, err
				}
				if err := addOut(&mi, rel, true); err != nil {
					return nil, err
				}
			}
			// Workflows-directory form: keeps /name available in the tool that
			// reads the workflows directory. With a derived skill it is a
			// redirect stub; a workflow targeting only the stub tool gets the
			// full content (target field stripped) instead.
			if hasStub(it) {
				rel := filepath.Join(toBase, it.OutName)
				dst := filepath.Join(out, rel)
				if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
					return nil, err
				}
				if it.SkillName != "" {
					if err := WriteWorkflowStub(dst, it.OutName, it.SkillName, cfg.Build.Categories["skills"].To); err != nil {
						return nil, err
					}
					if err := addOut(&mi, rel, true); err != nil {
						return nil, err
					}
				} else {
					derived, err := WriteWorkflowVerbatim(it.From, dst)
					if err != nil {
						return nil, err
					}
					if err := addOut(&mi, rel, derived); err != nil {
						return nil, err
					}
				}
			}
		default:
			rel := filepath.Join(toBase, it.OutName)
			dst := filepath.Join(out, rel)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return nil, err
			}
			if it.IsDir {
				if err := copyDir(it.From, dst); err != nil {
					return nil, err
				}
				// when skills use rename, also rewrite the front-matter name
				if it.Renamed && it.Category == "skills" {
					if err := RewriteSkillName(filepath.Join(dst, "SKILL.md"), it.OutName); err != nil {
						return nil, err
					}
				}
			} else {
				if err := copyFile(it.From, dst); err != nil {
					return nil, err
				}
			}
			if err := addOut(&mi, rel, false); err != nil {
				return nil, err
			}
		}
		sh, sfiles, err := HashPath(it.From)
		if err != nil {
			return nil, err
		}
		mi.SrcHash, mi.SrcFiles = sh, sfiles
		if err := verifySrcUnchanged(&mi, it); err != nil {
			return nil, err
		}
		m.Items = append(m.Items, mi)
	}
	// Derived AGENTS.md: every rule concatenated, in source-priority order.
	// Always produced, even with zero rules, so the root AGENTS.md link never
	// dangles.
	agentsRel := config.AgentsMD
	if err := ConcatRules(cfg, p, filepath.Join(out, agentsRel)); err != nil {
		return nil, err
	}
	ami := ManifestItem{Category: "agents-md", Name: config.AgentsMD}
	if err := addOut(&ami, agentsRel, true); err != nil {
		return nil, err
	}
	m.Items = append(m.Items, ami)
	// Ensure every mount target exists: with no items in a category its
	// directory would never be created and a link pointing there would be
	// broken (ls gives ENOENT). An empty directory is the correct "nothing
	// here yet".
	if err := EnsureLinkTargets(cfg); err != nil {
		return nil, err
	}
	if err := WriteManifest(out, m); err != nil {
		return nil, err
	}
	return m, nil
}

// verifySrcUnchanged closes the copy-then-hash race: Execute copies the
// output first and hashes the source afterwards for the manifest baseline, so
// a source modified in between would leave the manifest recording content the
// output does not hold — and status would then report "in sync" against a
// stale artifact. For verbatim outputs the two fingerprints must match
// exactly; the SKILL.md of a renamed skill is rewritten on purpose and is the
// only permitted difference.
func verifySrcUnchanged(mi *ManifestItem, it Item) error {
	if it.Category == "workflows" {
		return nil // derived forms only; there is no verbatim copy to compare
	}
	for _, o := range mi.Outs {
		if o.Derived {
			continue
		}
		if !it.IsDir {
			if o.Hash != mi.SrcHash {
				return fmt.Errorf(i18n.T("source %s changed while apply was copying it; rerun agsy apply"), it.From)
			}
			continue
		}
		diff := DiffFiles(mi.SrcFiles, o.Files)
		if it.Renamed && it.Category == "skills" {
			kept := diff[:0]
			for _, f := range diff {
				if f != "SKILL.md" {
					kept = append(kept, f)
				}
			}
			diff = kept
		}
		if len(diff) > 0 {
			return fmt.Errorf(i18n.T("source %s changed while apply was copying it; rerun agsy apply"), it.From)
		}
	}
	return nil
}

// EnsureLinkTargets creates the target for every mount.links entry: a
// directory for category targets, nothing extra for the AGENTS.md file (the
// build always writes it).
func EnsureLinkTargets(cfg *config.Config) error {
	out := cfg.OutDir()
	for _, m := range cfg.Mount {
		for _, sub := range m.Links {
			if sub == config.AgentsMD {
				continue
			}
			p := filepath.Join(out, filepath.FromSlash(sub))
			if err := os.MkdirAll(p, 0o755); err != nil {
				return fmt.Errorf(i18n.T("failed to create output directory %s: %w"), p, err)
			}
		}
	}
	return nil
}

// RemoveOut deletes the build output directory (shared by apply's cleanup and clean).
// Dangerous paths were already rejected at config load; this is the second line of
// defense: even a weird cfg from the caller cannot cause a wrong deletion.
func RemoveOut(cfg *config.Config) error {
	out := cfg.OutDir()
	if out == "" || out == filepath.Dir(out) {
		return fmt.Errorf(i18n.T("refusing to delete suspicious output path %q"), out)
	}
	// Ancestor checks compare resolved locations: a symlinked directory could
	// otherwise make a path that reads as project-local point anywhere.
	rout := config.ResolveSymlinks(out)
	rbase := config.ResolveSymlinks(cfg.BaseDir)
	if rout == "" || rout == filepath.Dir(rout) {
		return fmt.Errorf(i18n.T("refusing to delete suspicious output path %q"), out)
	}
	if rout == rbase || config.IsAncestor(rout, rbase) {
		return fmt.Errorf(i18n.T("refusing to delete %q: it contains the project root"), out)
	}
	if !config.IsAncestor(rbase, rout) {
		return fmt.Errorf(i18n.T("refusing to delete %q: it is not inside the project directory %q"), out, cfg.BaseDir)
	}
	if home, err := os.UserHomeDir(); err == nil {
		h := config.ResolveSymlinks(filepath.Clean(home))
		if rout == h || config.IsAncestor(rout, h) {
			return fmt.Errorf(i18n.T("refusing to delete %q: it contains the home directory"), out)
		}
	}
	for _, root := range cfg.SourceRoots() {
		rroot := config.ResolveSymlinks(root)
		if rout == rroot || config.IsAncestor(rout, rroot) {
			return fmt.Errorf(i18n.T("refusing to delete %q: it contains source %q"), out, root)
		}
		if config.IsAncestor(rroot, rout) {
			return fmt.Errorf(i18n.T("refusing to delete %q: it lies inside source %q"), out, root)
		}
	}
	return os.RemoveAll(out)
}

func WriteManifest(out string, m *Manifest) error {
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, ManifestName), raw, 0o644)
}

func LoadManifest(out string) (*Manifest, error) {
	raw, err := os.ReadFile(filepath.Join(out, ManifestName))
	if err != nil {
		return nil, err
	}
	m := &Manifest{}
	if err := json.Unmarshal(raw, m); err != nil {
		return nil, fmt.Errorf(i18n.T("manifest is corrupted: %w"), err)
	}
	// A manifest written by a newer agsy may have fields this version doesn't
	// know; force-parsing would interpret new data with old rules, so refuse
	// and require an upgrade.
	if m.Version > ManifestVersion {
		return nil, fmt.Errorf(i18n.T("manifest version %d exceeds the maximum %d supported by this agsy, please upgrade agsy (or remove the output directory and apply again)"), m.Version, ManifestVersion)
	}
	// The manifest lives in the AI-writable output, so it is untrusted.
	// Consumers join output paths onto the out directory; an entry like
	// "../../x" would escape it. Reject the whole manifest — a tampered
	// record must not be half-trusted.
	for _, it := range m.Items {
		for _, o := range it.Outs {
			if !filepath.IsLocal(filepath.FromSlash(o.Path)) {
				return nil, fmt.Errorf(i18n.T("manifest contains an invalid output path %q; it may be tampered with — remove the output directory and run agsy apply to rebuild"), o.Path)
			}
		}
		// From is consumed as an absolute source path (status stats and hashes
		// it); a relative one can only come from tampering.
		if it.From != "" && !filepath.IsAbs(it.From) {
			return nil, fmt.Errorf(i18n.T("manifest contains an invalid source path %q; it may be tampered with — remove the output directory and run agsy apply to rebuild"), it.From)
		}
	}
	return m, nil
}

// HashPath computes the content fingerprint of a single file or a directory.
// Directory: sha256 per file; item hash = sha256 of the sorted "relpath:hash"
// lines concatenated.
func HashPath(p string) (string, map[string]string, error) {
	st, err := os.Stat(p)
	if err != nil {
		return "", nil, err
	}
	if !st.IsDir() {
		h, err := hashFile(p)
		return h, nil, err
	}
	files := map[string]string{}
	err = filepath.WalkDir(p, func(fp string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(p, fp)
		h, err := hashFile(fp)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = h
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	agg := sha256.New()
	for _, k := range keys {
		fmt.Fprintf(agg, "%s:%s\n", k, files[k])
	}
	return "sha256:" + fmt.Sprintf("%x", agg.Sum(nil)), files, nil
}

// DiffFiles compares two per-file hash maps and returns the relative paths that
// differ (added / removed / content changed all count).
// Directory items (skills) rely on it to answer "which files changed" instead of
// just "content changed".
func DiffFiles(before, after map[string]string) []string {
	if before == nil || after == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for k, v := range before {
		seen[k] = true
		if after[k] != v {
			out = append(out, k)
		}
	}
	for k := range after {
		if !seen[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func hashFile(p string) (string, error) {
	// Never follow a link while fingerprinting: sources are only half
	// trusted, and outputs never legitimately contain links.
	f, err := openNoFollow(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + fmt.Sprintf("%x", h.Sum(nil)), nil
}

// copyFile copies a single file and preserves the source permissions.
// Skill directories often carry executable scripts/; if permissions are lost the
// mounted copy is broken.
func copyFile(src, dst string) error {
	st, err := os.Lstat(src)
	if err != nil {
		return err
	}
	// Accepts rejects links at scan time, but the file may have been swapped
	// for a link since; never copy a link's target content into the output.
	if st.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf(i18n.T("refusing to copy symbolic link %s"), src)
	}
	perm := st.Mode().Perm()
	// O_NOFOLLOW (see openNoFollow) closes the window between the Lstat
	// check above and the open: a swap-for-a-link in between fails to open.
	in, err := openNoFollow(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	// OpenFile does not apply perm when dst already exists; set it explicitly
	return os.Chmod(dst, perm)
}

// FirstSymlinkWithin returns the first symbolic link found under path
// (path itself included), relative to path. A path that is itself a link
// yields "."; single files work too. WalkDir does not follow links, so the
// walk is safe.
func FirstSymlinkWithin(dir string) (string, bool) {
	if fi, err := os.Lstat(dir); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return ".", true
	}
	found := ""
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			rel, _ := filepath.Rel(dir, p)
			found = filepath.ToSlash(rel)
			return filepath.SkipAll
		}
		return nil
	})
	return found, found != ""
}

// FirstIrregularWithin returns the first entry under path (path itself
// included) that is neither a regular file, a directory, nor a symlink —
// FIFOs, sockets, devices — relative to path. Symlinks are reported
// separately by FirstSymlinkWithin with their own message. Reading a named
// pipe blocks forever, so a skill carrying one is rejected as a whole
// rather than hanging the copy or the tools reading the mount later.
func FirstIrregularWithin(dir string) (string, bool) {
	if fi, err := os.Lstat(dir); err == nil && !fi.IsDir() && fi.Mode()&os.ModeSymlink == 0 && !fi.Mode().IsRegular() {
		return ".", true
	}
	found := ""
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		t := d.Type()
		if t.IsRegular() || d.IsDir() || t&os.ModeSymlink != 0 {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		found = filepath.ToSlash(rel)
		return filepath.SkipAll
	})
	return found, found != ""
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Same guard as copyFile: never copy a link's target content.
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf(i18n.T("refusing to copy symbolic link %s"), p)
		}
		// Nor an irregular entry: openNoFollow would refuse the read anyway,
		// but failing here names the problem before any open is attempted.
		if !d.Type().IsRegular() && !d.IsDir() {
			return fmt.Errorf(i18n.T("refusing to read %s: not a regular file"), p)
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			perm := os.FileMode(0o755)
			if info, err := d.Info(); err == nil {
				perm = info.Mode().Perm()
			}
			if err := os.MkdirAll(target, perm); err != nil {
				return err
			}
			return os.Chmod(target, perm)
		}
		return copyFile(p, target)
	})
}

// RewriteSkillName rewrites the name in SKILL.md front matter to the given
// name, keeping renamed skills valid per the Agent Skills spec (front-matter
// name must equal the directory name).
func RewriteSkillName(skillMD, newName string) error {
	raw, err := os.ReadFile(skillMD)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no SKILL.md, nothing to do
		}
		return err
	}
	lines := strings.Split(strings.TrimPrefix(string(raw), bomPrefix), "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return nil
	}
	for i := 1; i < len(lines); i++ {
		t := strings.TrimRight(lines[i], "\r")
		if t == "---" {
			break
		}
		// Only the top-level key counts: an indented "name:" is a nested
		// field (e.g. under metadata:) and must not be rewritten.
		if strings.HasPrefix(t, "name:") {
			lines[i] = "name: " + newName
			break
		}
	}
	return os.WriteFile(skillMD, []byte(strings.Join(lines, "\n")), 0o644)
}
