// Package build implements the build phase: scan → candidate list → name-conflict handling → routing → copy → manifest.
package build

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	Buckets   []string // routing destinations for workflows
	Renamed   bool     // renamed due to a conflict
	RouteNote string   // routing note (e.g. "no target specified, default applied")
}

// Ignored is a file skipped during scanning: not an error, but it must be
// reported — silent discarding is not allowed
type Ignored struct {
	Category string
	Name     string
	From     string
	Reason   string
}

// ManifestName is the manifest file name (located under build.out).
// Note: the "no hard-coded .agsy" rule is about *paths* — paths are always
// derived from build.out. The .agsy- here is only a file-name prefix (it stays in the
// right place even if the output directory is renamed), a deliberate exception.
const ManifestName = ".agsy-manifest.json"

// ManifestVersion is the manifest format version this agsy writes / understands.
// Distinct from agsy.yaml's version: that one is the config format, this one is the
// record-file format.
const ManifestVersion = 1

type ManifestItem struct {
	Category string            `json:"category"`
	Name     string            `json:"name"`     // final name in the output
	Original string            `json:"original"` // original name
	From     string            `json:"from"`
	Hash     string            `json:"hash"`                // output hash (baseline for local-change detection)
	SrcHash  string            `json:"src_hash"`            // source hash at build time (baseline for staleness detection)
	Files    map[string]string `json:"files,omitempty"`     // per-file hashes on the output side (relpath → hash)
	SrcFiles map[string]string `json:"src_files,omitempty"` // per-file hashes on the source side (to answer "which files changed")
	Buckets  []string          `json:"buckets,omitempty"`
	OutPaths []string          `json:"out_paths"` // output paths relative to the out directory (a workflow may have several copies)
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
	NoBucket    []string    // workflows with default=[] and no target specified (warning)
	RouteErrors []string    // per-file routing problems (broken front matter, unknown bucket); collected so plan lists them all at once
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
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return false, i18n.T("symbolic links are not collected")
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
		if rel, found := firstSymlinkWithin(path); found {
			return false, fmt.Sprintf(i18n.T("contains a symbolic link (%s); the skill is not collected"), rel)
		}
		return true, ""
	}
	if isDir {
		return false, fmt.Sprintf(i18n.T("%s entries are single .md files; a directory is not accepted"), cat)
	}
	if !strings.HasSuffix(name, ".md") {
		return false, i18n.T("extension is not .md")
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
	// 3. workflow routing
	if err := routeWorkflows(cfg, p); err != nil {
		return nil, err
	}
	// 4. final-name uniqueness (collisions are still possible after rename)
	detectCollisions(p)
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

// detectCollisions finds items whose final names collide.
// Combinations already reported by on_conflict=error are not counted again.
func detectCollisions(p *Plan) {
	reported := map[string]bool{}
	for _, c := range p.Conflicts {
		reported[c.Category+"/"+c.Name] = true
	}
	groups := map[string][]Item{}
	var order []string
	for _, it := range p.Items {
		key := it.Category + "/" + it.OutName
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], it)
	}
	for _, key := range order {
		its := groups[key]
		if len(its) < 2 || reported[key] {
			continue
		}
		var froms []string
		for _, it := range its {
			froms = append(froms, it.From)
		}
		p.Collisions = append(p.Collisions, Collision{
			Category: its[0].Category, OutName: its[0].OutName, Froms: froms,
		})
	}
}

// tagged builds a name carrying a source tag, using one uniform separator for
// every category: python-style.md + god-lib → python-style-fromlib-god-lib.md,
// code-review + god-lib → code-review-fromlib-god-lib.
// "-fromlib-" (not "@") because skill names must obey the Agent Skills spec —
// lowercase a-z0-9 and single hyphens only, front-matter name == directory
// name — where "@" makes the skill silently fail to load; the same separator
// is applied to plain files too so all three categories read alike.
// Skill names are additionally sanitized into the spec's character set.
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
	s := string(raw)
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

func routeWorkflows(cfg *config.Config, p *Plan) error {
	field := cfg.Build.Route.Field
	buckets := cfg.Build.Route.Buckets
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
			switch v := fm[field].(type) {
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
			// not specified → apply route.default
			if len(cfg.Build.Route.Default) == 0 {
				p.NoBucket = append(p.NoBucket, it.OutName)
				it.RouteNote = fmt.Sprintf(i18n.T("no %s specified and default is empty → not placed, warning issued"), field)
				continue
			}
			it.Buckets = append([]string{}, cfg.Build.Route.Default...)
			it.RouteNote = fmt.Sprintf(i18n.T("no %s specified, default applied"), field)
			continue
		}
		bad := false
		for _, t := range targets {
			ok := false
			for _, b := range buckets {
				if b == t {
					ok = true
					break
				}
			}
			if !ok {
				p.RouteErrors = append(p.RouteErrors, fmt.Sprintf(i18n.T("%s: %s refers to nonexistent bucket %q, valid values: %v"), it.Name, field, t, buckets))
				bad = true
				break
			}
		}
		if bad {
			continue // file gets no bucket; Execute refuses while RouteErrors exist
		}
		it.Buckets = targets
	}
	return nil
}

// Placed reports how many items will actually be placed into the output
// (workflows with default=[] don't count)
func (p *Plan) Placed() int {
	n := 0
	for _, it := range p.Items {
		if it.Category == "workflows" && len(it.Buckets) == 0 {
			continue
		}
		n++
	}
	return n
}

// Execute writes according to the Plan: empty out → copy → ensure mount targets → write manifest.
// The caller (apply) must confirm local changes first; must not be called when
// conflicts (error strategy) exist.
func Execute(cfg *config.Config, p *Plan) (*Manifest, error) {
	if len(p.Conflicts) > 0 {
		return nil, fmt.Errorf(i18n.T("unresolved name conflicts exist, cannot build"))
	}
	if len(p.Collisions) > 0 {
		return nil, fmt.Errorf(i18n.T("final output names collide, cannot build"))
	}
	if len(p.RouteErrors) > 0 {
		return nil, fmt.Errorf(i18n.T("workflow routing problems exist, cannot build"))
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
	for _, it := range p.Items {
		mi := ManifestItem{
			Category: it.Category, Name: it.OutName, Original: it.Name,
			From: it.From, Buckets: it.Buckets, Renamed: it.Renamed,
		}
		toBase := cfg.Build.Categories[it.Category].To
		var dests []string
		if it.Category == "workflows" {
			for _, b := range it.Buckets {
				dests = append(dests, filepath.Join(toBase, b, it.OutName))
			}
		} else {
			dests = []string{filepath.Join(toBase, it.OutName)}
		}
		if len(dests) == 0 { // workflow with default=[]: not placed
			continue
		}
		for _, rel := range dests {
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
			mi.OutPaths = append(mi.OutPaths, filepath.ToSlash(rel))
		}
		// Two baselines: output hash (detects local changes) and source hash
		// (detects staleness). Stored separately because a renamed skill's output
		// contains the rewritten front-matter and inherently differs from the source.
		first := filepath.Join(out, filepath.FromSlash(mi.OutPaths[0]))
		h, files, err := HashPath(first)
		if err != nil {
			return nil, err
		}
		mi.Hash, mi.Files = h, files
		sh, sfiles, err := HashPath(it.From)
		if err != nil {
			return nil, err
		}
		mi.SrcHash, mi.SrcFiles = sh, sfiles
		m.Items = append(m.Items, mi)
	}
	// Ensure every mount target directory exists: with no workflows at all,
	// workflows/<bucket>/ would never be created and a link pointing there would be
	// broken (ls gives ENOENT). An empty directory is the correct "nothing here yet".
	if err := EnsureLinkTargets(cfg); err != nil {
		return nil, err
	}
	if err := WriteManifest(out, m); err != nil {
		return nil, err
	}
	return m, nil
}

// EnsureLinkTargets creates the target directory for every mount.links entry
func EnsureLinkTargets(cfg *config.Config) error {
	out := cfg.OutDir()
	for _, m := range cfg.Mount {
		for _, sub := range m.Links {
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
	if out == cfg.BaseDir || config.IsAncestor(out, cfg.BaseDir) {
		return fmt.Errorf(i18n.T("refusing to delete %q: it contains the project root"), out)
	}
	if !config.IsAncestor(cfg.BaseDir, out) {
		return fmt.Errorf(i18n.T("refusing to delete %q: it is not inside the project directory %q"), out, cfg.BaseDir)
	}
	if home, err := os.UserHomeDir(); err == nil {
		h := filepath.Clean(home)
		if out == h || config.IsAncestor(out, h) {
			return fmt.Errorf(i18n.T("refusing to delete %q: it contains the home directory"), out)
		}
	}
	for _, root := range cfg.SourceRoots() {
		if out == root || config.IsAncestor(out, root) {
			return fmt.Errorf(i18n.T("refusing to delete %q: it contains source %q"), out, root)
		}
		if config.IsAncestor(root, out) {
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
	// The manifest lives in the build output — the one layer mounted AI tools
	// can write to — so it is untrusted (same threat model as promote's
	// destination check). Every consumer joins OutPaths onto the out directory
	// (status hashing, promote's copy source, cross-bucket sync); an entry like
	// "../../x" would escape the output and turn those into arbitrary reads or
	// writes. Reject the whole manifest rather than skipping entries: a
	// tampered record must not be half-trusted.
	for _, it := range m.Items {
		for _, rel := range it.OutPaths {
			if !filepath.IsLocal(filepath.FromSlash(rel)) {
				return nil, fmt.Errorf(i18n.T("manifest contains an invalid output path %q; it may be tampered with — remove the output directory and run agsy apply to rebuild"), rel)
			}
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
	f, err := os.Open(p)
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
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	perm := st.Mode().Perm()
	in, err := os.Open(src)
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

// firstSymlinkWithin walks dir and returns the first symbolic link found, as a
// path relative to dir. WalkDir does not follow links, so the walk is safe.
func firstSymlinkWithin(dir string) (string, bool) {
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

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Second line of defense (Accepts already rejects sources containing
		// links): never copy a symbolic link's target content.
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf(i18n.T("refusing to copy symbolic link %s"), p)
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

// RewriteSkillName rewrites the name in SKILL.md front-matter to the given name.
// build uses it to apply the source tag; promote uses it to restore the original
// name so a name that only belongs to the output is never written back to the
// source.
func RewriteSkillName(skillMD, newName string) error {
	raw, err := os.ReadFile(skillMD)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no SKILL.md, nothing to do
		}
		return err
	}
	lines := strings.Split(string(raw), "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return nil
	}
	for i := 1; i < len(lines); i++ {
		t := strings.TrimRight(lines[i], "\r")
		if t == "---" {
			break
		}
		if strings.HasPrefix(strings.TrimSpace(t), "name:") {
			indent := t[:len(t)-len(strings.TrimLeft(t, " \t"))]
			lines[i] = indent + "name: " + newName
			break
		}
	}
	return os.WriteFile(skillMD, []byte(strings.Join(lines, "\n")), 0o644)
}
