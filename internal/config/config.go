// Package config handles loading agsy.yaml, defaults, validation and path expansion.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/IngSquared99/agent-sync/i18n"
	"github.com/IngSquared99/agent-sync/internal/yaml"
)

// CategoryOrder is the fixed processing order of categories (used for both scanning and output)
var CategoryOrder = []string{"rules", "skills", "workflows", "hooks"}

// ValidStrategies are the legal name-conflict strategies
var ValidStrategies = map[string]bool{"first": true, "rename": true, "error": true}

// SchemaVersion is the highest config-file version currently supported
const SchemaVersion = 2

// AgentsMD is the derived single-file rules artifact: every rule concatenated
// into one AGENTS.md, read natively by Codex, Cursor and Antigravity. It lives
// at the top of the output directory and is a reserved name there: no category
// may claim it as its "to" directory, and mount links may point at it.
const AgentsMD = "AGENTS.md"

// StubTool is the tool that consumes the workflows/ stub form instead of the
// derived skill; every other tool reads workflows through the skills mount.
// It lives here (not in build) so validation can cross-check build.tools
// against the mount configuration.
const StubTool = "antigravity"

// HookTools lists the tools agsy can produce a hook registry for, in the
// order their registries are written. Adding a dialect in build/hooks.go
// also means adding the tool here so config knows the registry file name.
var HookTools = []string{"claude", "codex", "antigravity", "cursor"}

// HookRegistryFiles maps a tool name to its derived hook registry file at the
// top of the output directory. Like AGENTS.md these are reserved names: no
// category may claim one as its "to", while mount links (and merge entries)
// may point at them.
var HookRegistryFiles = map[string]string{
	"claude":      "hooks.claude.json",
	"codex":       "hooks.codex.json",
	"antigravity": "hooks.antigravity.json",
	"cursor":      "hooks.cursor.json",
}

// HookMergeTools lists the tools whose registry has the nested shape merge
// reads back ({"hooks": {Event: [group…]}}); only those registries may be a
// mount merge target. The shapes live in build/hooks.go (dialects); a test
// there keeps the two in step.
var HookMergeTools = map[string]bool{"claude": true, "codex": true}

// IsRegistryFile reports whether name is one of the hook registry files.
func IsRegistryFile(name string) bool {
	return RegistryTool(name) != ""
}

// RegistryTool returns the tool a registry file name belongs to ("" if none).
func RegistryTool(name string) string {
	for t, f := range HookRegistryFiles {
		if f == name {
			return t
		}
	}
	return ""
}

// ReservedTopNames are the top-level output names a category's "to" may
// never use: they are produced by build as derived files.
func ReservedTopNames() []string {
	out := []string{AgentsMD}
	for _, t := range HookTools {
		out = append(out, HookRegistryFiles[t])
	}
	return out
}

type Category struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

type BuildCfg struct {
	Out        string              `yaml:"out"`
	Categories map[string]Category `yaml:"categories"`
	OnConflict map[string]string   `yaml:"on_conflict"`
	// Tools is the closed set of tool names a workflow's front-matter target
	// may reference; a typo in target: fails loudly instead of silently
	// excluding the workflow everywhere.
	Tools []string `yaml:"tools"`
}

type MountCfg struct {
	Dir   string            `yaml:"dir"`
	Links map[string]string `yaml:"links"`
	// Merge maps a file name inside dir to a hook registry file in the
	// output. Instead of linking, apply merges the registry's hook entries
	// into the "hooks" key of that JSON file, leaving every other key and
	// every foreign entry untouched. Claude Code needs this: its hooks live
	// in .claude/settings.json next to user settings, with no separate file.
	Merge map[string]string `yaml:"merge"`
	// OutsideProject is the explicit opt-in required for a mount dir that
	// resolves outside the project directory. Without it such a mount is
	// rejected: an agsy.yaml inside a cloned (possibly untrusted) repository
	// could otherwise plant links in the user's home directory — e.g. point
	// the global ~/.claude/skills at repository-controlled content — the
	// moment apply runs.
	OutsideProject bool `yaml:"outside_project"`
}

type Config struct {
	Version int        `yaml:"version"`
	Sources []string   `yaml:"sources"`
	Build   BuildCfg   `yaml:"build"`
	Mount   []MountCfg `yaml:"mount"`

	// BaseDir is the directory containing agsy.yaml (the base for resolving
	// relative paths); not serialized
	BaseDir string `yaml:"-"`
	// Path is the full path of the config file
	Path string `yaml:"-"`
}

const FileName = "agsy.yaml"

// Find looks for agsy.yaml in dir (current directory only).
// For init: create/modify the config exactly where the command runs, unaffected
// by any enclosing project.
func Find(dir string) (string, bool) {
	p := filepath.Join(dir, FileName)
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p, true
	}
	return p, false
}

// FindUp walks up from dir looking for agsy.yaml (same convention as git).
// Relative paths resolve against the config file's directory, so the
// config must also be findable from project subdirectories.
func FindUp(dir string) (string, bool) {
	for {
		if p, ok := Find(dir); ok {
			return p, true
		}
		parent := filepath.Dir(dir)
		if parent == dir { // reached the filesystem root
			return filepath.Join(dir, FileName), false
		}
		dir = parent
	}
}

// Load reads and validates the config file
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("failed to read %s: %w"), path, err)
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf(i18n.T("%s is malformed: %w"), filepath.Base(path), err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	cfg.Path = abs
	cfg.BaseDir = filepath.Dir(abs)
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Version == 0 {
		c.Version = SchemaVersion // files without a version are treated as the current schema
	}
	if c.Build.Out == "" {
		c.Build.Out = ".agsy"
	}
	if c.Build.Categories == nil {
		c.Build.Categories = map[string]Category{}
	}
	def := map[string]Category{
		"rules":     {From: "rules", To: "rules"},
		"skills":    {From: "skills", To: "skills"},
		"workflows": {From: "workflows", To: "workflows"},
		"hooks":     {From: "hooks", To: "hooks"},
	}
	for k, v := range def {
		cur, ok := c.Build.Categories[k]
		if !ok {
			c.Build.Categories[k] = v
			continue
		}
		// when only from or only to is set, fill in the default for the other half
		if cur.From == "" {
			cur.From = v.From
		}
		if cur.To == "" {
			cur.To = v.To
		}
		c.Build.Categories[k] = cur
	}
	// Merge mount entries that share a dir: adapters for different tools may
	// mount into the same directory (.agents serves Codex, Antigravity and
	// Cursor at once). Order is preserved; a same-name link with a different
	// target is reported by validate, never silently overwritten.
	c.Mount = c.mergeMounts(c.Mount)
}

// dupMark tags a link name that appeared twice with different targets, so the
// clash survives merging and validate can report it.
const dupMark = "\x00dup"

// mergeMounts folds entries whose dir names the same real directory into
// one, keeping first-seen order (and the first-seen spelling, for display).
// Entries are compared by their expanded, symlink-resolved location, not the
// raw string: ".claude" and "./.claude" land links in the same directory at
// mount time, so treating them as two entries would let a same-name link
// with different targets slip past validate and silently become
// last-writer-wins. A dir that fails to expand falls back to its raw
// spelling as the key; validate reports the expansion problem separately.
func (c *Config) mergeMounts(in []MountCfg) []MountCfg {
	var out []MountCfg
	idx := map[string]int{}
	for _, m := range in {
		// An entry without dir never merges: ExpandPath("") resolves to the
		// project root, so the entry would be folded into a "- dir: ." entry
		// and its links silently re-anchored there. It is kept as its own
		// entry so validate sees the missing dir and rejects it.
		if m.Dir == "" {
			cp := MountCfg{Links: map[string]string{}, Merge: copyMap(m.Merge), OutsideProject: m.OutsideProject}
			for k, v := range m.Links {
				cp.Links[k] = v
			}
			out = append(out, cp)
			continue
		}
		key := m.Dir
		if abs, err := c.ExpandPath(m.Dir); err == nil {
			key = ResolveSymlinks(abs)
		}
		i, seen := idx[key]
		if !seen {
			idx[key] = len(out)
			cp := MountCfg{Dir: m.Dir, Links: map[string]string{}, Merge: copyMap(m.Merge), OutsideProject: m.OutsideProject}
			for k, v := range m.Links {
				cp.Links[k] = v
			}
			out = append(out, cp)
			continue
		}
		// the opt-in survives merging no matter which entry carried it
		out[i].OutsideProject = out[i].OutsideProject || m.OutsideProject
		for k, v := range m.Links {
			if prev, ok := out[i].Links[k]; ok && prev != v {
				out[i].Links[k+dupMark] = v
				continue
			}
			out[i].Links[k] = v
		}
		for k, v := range m.Merge {
			if out[i].Merge == nil {
				out[i].Merge = map[string]string{}
			}
			if prev, ok := out[i].Merge[k]; ok && prev != v {
				out[i].Merge[k+dupMark] = v
				continue
			}
			out[i].Merge[k] = v
		}
	}
	return out
}

func copyMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	cp := make(map[string]string, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

func (c *Config) validate() error {
	var errs []string
	if c.Version > SchemaVersion {
		errs = append(errs, fmt.Sprintf(i18n.T("version: %d exceeds the maximum %d supported by this agsy, please upgrade agsy"), c.Version, SchemaVersion))
	}
	if len(c.Sources) == 0 {
		errs = append(errs, i18n.T("sources is not set; at least one source path is required"))
	}
	// Sources must be mutually distinct and non-nested. A path listed twice
	// (or spelled two ways that resolve to the same place) would be scanned
	// twice — under rename both identical copies are kept with numbered tags;
	// a source inside another gets its files collected once per root.
	// Resolved locations are compared so symlinked spellings cannot slip past.
	srcResolved := make([]string, len(c.Sources))
	for i, s := range c.Sources {
		if abs, err := c.ExpandPath(s); err == nil {
			srcResolved[i] = ResolveSymlinks(abs)
		} else {
			errs = append(errs, err.Error())
		}
	}
	for i := 0; i < len(srcResolved); i++ {
		for j := i + 1; j < len(srcResolved); j++ {
			a, b := srcResolved[i], srcResolved[j]
			if a == "" || b == "" {
				continue
			}
			switch {
			case a == b:
				errs = append(errs, fmt.Sprintf(i18n.T("sources %q and %q resolve to the same directory; list each source once"), c.Sources[i], c.Sources[j]))
			case IsAncestor(a, b) || IsAncestor(b, a):
				errs = append(errs, fmt.Sprintf(i18n.T("sources %q and %q are nested (one lies inside the other); their files would be collected twice"), c.Sources[i], c.Sources[j]))
			}
		}
	}
	// on_conflict is required per category: the user must make an explicit choice;
	// a missing field never silently falls back to a default
	for _, cat := range CategoryOrder {
		s, ok := c.Build.OnConflict[cat]
		if !ok || s == "" {
			if cat == "hooks" {
				// The message names the upgrade step; no default is assumed.
				errs = append(errs, i18n.T("build.on_conflict.hooks is not set (new in v0.2.0); choose first / rename / error, or rerun agsy init"))
				continue
			}
			errs = append(errs, fmt.Sprintf(i18n.T("build.on_conflict.%s is not set, please explicitly choose first / rename / error"), cat))
			continue
		}
		if !ValidStrategies[s] {
			errs = append(errs, fmt.Sprintf(i18n.T("build.on_conflict.%s value %q is invalid, valid values: first / rename / error"), cat, s))
		}
	}
	if len(c.Build.Tools) == 0 {
		errs = append(errs, i18n.T("build.tools is not set; list the tools this project serves (e.g. [claude, codex, antigravity, cursor])"))
	}
	// destructive-path checks for build.out: apply and clean both delete this whole directory
	errs = append(errs, c.validateOut()...)
	// Category subdirectories must be distinct on both sides: a shared "to"
	// mixes categories in one output folder past conflict detection; a shared
	// "from" collects every file in that directory once per category.
	seenTo := map[string]string{}
	seenFrom := map[string]string{}
	for _, cat := range CategoryOrder {
		cc := c.Build.Categories[cat]
		if prev, ok := seenFrom[cc.From]; ok {
			errs = append(errs, fmt.Sprintf(i18n.T("build.categories.%s.from and %s are both %q; different categories must scan different subdirectories"), cat, prev, cc.From))
		} else {
			seenFrom[cc.From] = cat
		}
		if cc.To == AgentsMD {
			errs = append(errs, fmt.Sprintf(i18n.T("build.categories.%s.to must not be %q; that name is reserved for the derived rules file"), cat, AgentsMD))
			continue
		}
		if IsRegistryFile(cc.To) {
			errs = append(errs, fmt.Sprintf(i18n.T("build.categories.%s.to must not be %q; that name is reserved for a derived hook registry file"), cat, cc.To))
			continue
		}
		if prev, ok := seenTo[cc.To]; ok {
			errs = append(errs, fmt.Sprintf(i18n.T("build.categories.%s.to and %s are both %q; different categories must output to different subdirectories"), cat, prev, cc.To))
			continue
		}
		seenTo[cc.To] = cat
	}

	if len(c.Mount) == 0 {
		errs = append(errs, i18n.T("mount is not set; at least one mount target is required"))
	}
	errs = append(errs, c.validateMount()...)
	errs = append(errs, c.validateWorkflowsMount()...)
	errs = append(errs, c.validateHooksMount()...)

	if len(errs) > 0 {
		return fmt.Errorf(i18n.T("config validation failed:\n  - %s"), strings.Join(errs, "\n  - "))
	}
	return nil
}

// validateOut rejects build.out values that would destroy data.
// The whole out directory is emptied by apply and removed by clean, so it may
// only be a dedicated directory inside the project.
func (c *Config) validateOut() []string {
	var errs []string
	out := c.OutDir()
	if out == "" {
		return []string{i18n.T("build.out must not be empty")}
	}
	if out == filepath.Dir(out) {
		return []string{fmt.Sprintf(i18n.T("build.out resolves to the filesystem root (%s), apply would wipe it"), out)}
	}
	// Ancestor checks compare resolved locations: a symlinked directory could
	// otherwise make build.out (which apply wipes) read as project-local while
	// pointing anywhere on disk.
	rout := ResolveSymlinks(out)
	rbase := ResolveSymlinks(c.BaseDir)
	switch {
	case rout == rbase:
		errs = append(errs, fmt.Sprintf(i18n.T("build.out resolves to the project root (%s), apply would wipe the entire project"), out))
	case IsAncestor(rout, rbase):
		errs = append(errs, fmt.Sprintf(i18n.T("build.out (%s) is an ancestor of the project root, apply would wipe the project along with it"), out))
	case !IsAncestor(rbase, rout):
		// The output directory must be a descendant of the project. apply empties
		// it entirely and clean removes it entirely; a location outside the
		// project would treat an unrelated directory as disposable output.
		errs = append(errs, fmt.Sprintf(i18n.T("build.out (%s) is not inside the project directory (%s). apply wipes it entirely, so only a dedicated directory inside the project is allowed"), out, c.BaseDir))
	}
	if home, err := os.UserHomeDir(); err == nil {
		h := ResolveSymlinks(filepath.Clean(home))
		if rout == h || IsAncestor(rout, h) {
			errs = append(errs, fmt.Sprintf(i18n.T("build.out (%s) contains the home directory, apply would wipe it"), out))
		}
	}
	for _, s := range c.Sources {
		abs, err := c.ExpandPath(s)
		if err != nil {
			continue
		}
		rabs := ResolveSymlinks(abs)
		if rout == rabs || IsAncestor(rout, rabs) {
			errs = append(errs, fmt.Sprintf(i18n.T("build.out (%s) contains source %s, apply would delete the source (sources must live outside the output)"), out, s))
		} else if IsAncestor(rabs, rout) {
			// The other direction matters just as much: an output nested inside
			// a source gets wiped by apply together with the originals around it.
			errs = append(errs, fmt.Sprintf(i18n.T("build.out (%s) lies inside source %s; apply would wipe that part of the source"), out, s))
		}
	}
	return errs
}

// validateWorkflowsMount keeps build.tools and the mount configuration
// consistent around the workflows directory. Only the stub tool reads it, so
// the two settings can silently drift apart after hand-editing: a mounted
// workflows link without the stub tool in build.tools serves a permanently
// empty directory (no stub is ever generated and /name never works), and the
// stub tool listed without a workflows mount builds stubs no tool ever reads.
func (c *Config) validateWorkflowsMount() []string {
	wfTo := c.Build.Categories["workflows"].To
	mounted := false
	for _, m := range c.Mount {
		for _, sub := range m.Links {
			if strings.Trim(filepath.ToSlash(sub), "/") == wfTo {
				mounted = true
			}
		}
	}
	switch {
	case mounted && !c.HasTool(StubTool):
		return []string{fmt.Sprintf(i18n.T("a mount links to the workflows output %q, but build.tools does not list %q (the only tool that reads it); the mounted directory would stay empty — add %q to build.tools or remove that link"), wfTo, StubTool, StubTool)}
	case !mounted && c.HasTool(StubTool):
		return []string{fmt.Sprintf(i18n.T("build.tools lists %q, but no mount links to the workflows output %q; the generated workflow stubs would never be read — add a workflows link (e.g. under .agents) or remove %q from build.tools"), StubTool, wfTo, StubTool)}
	}
	return nil
}

// validateMount cross-checks that every path mount.links points at will actually
// be produced by build.
func (c *Config) validateMount() []string {
	var errs []string
	// Top-level output names a link may target: each category's to value plus
	// the derived AGENTS.md file. Targets are top-level only — the output has
	// no deeper mountable structure.
	validTarget := map[string]bool{AgentsMD: true}
	var validTops []string
	// validTops lists each target once: categories sharing a "to" is a
	// misconfiguration with its own error, and must not duplicate the hint.
	seenTop := map[string]bool{AgentsMD: true}
	for _, cat := range CategoryOrder {
		to := c.Build.Categories[cat].To
		validTarget[to] = true
		if !seenTop[to] {
			seenTop[to] = true
			validTops = append(validTops, to)
		}
	}
	validTops = append(validTops, AgentsMD)
	for _, t := range HookTools {
		validTarget[HookRegistryFiles[t]] = true
		validTops = append(validTops, HookRegistryFiles[t])
	}
	out := c.OutDir()
	rout := ResolveSymlinks(out)
	for _, m := range c.Mount {
		if m.Dir == "" {
			errs = append(errs, i18n.T("mount entry is missing dir"))
		} else if abs, err := c.ExpandPath(m.Dir); err == nil {
			rabs := ResolveSymlinks(abs)
			if rabs == rout || IsAncestor(rout, rabs) {
				errs = append(errs, fmt.Sprintf(i18n.T("mount dir %q lies inside build.out; apply empties that directory and the links would be wiped with it"), m.Dir))
			}
			// A mount dir outside the project requires an explicit opt-in.
			// Without this gate, running apply on a cloned repository whose
			// agsy.yaml mounts into the home directory (dir: ~/.claude) would
			// point the user's GLOBAL tool configuration at repository-
			// controlled content — a hijack, not a sync.
			rbase := ResolveSymlinks(c.BaseDir)
			outside := rabs != rbase && !IsAncestor(rbase, rabs)
			if outside && !m.OutsideProject {
				errs = append(errs, fmt.Sprintf(i18n.T("mount dir %q resolves outside the project directory (%s). Only do this for a config you wrote yourself — a mount outside the project can redirect global tool configuration (e.g. ~/.claude) at this project's content. To confirm the intent, add outside_project: true to that mount entry"), m.Dir, abs))
			}
			// Merge targets stay inside the project, opt-in or not: agsy owns
			// the project-level file only; a user-level file such as
			// ~/.claude/settings.json is shared between projects and belongs
			// to the user.
			if outside && len(m.Merge) > 0 {
				errs = append(errs, fmt.Sprintf(i18n.T("mount dir %q resolves outside the project directory (%s), but it carries merge entries; merge targets must be inside the project (put personal hooks in the tool's own user-level file)"), m.Dir, abs))
			}
			for _, s := range c.Sources {
				sabs, serr := c.ExpandPath(s)
				if serr != nil {
					continue
				}
				rsabs := ResolveSymlinks(sabs)
				if rabs == rsabs || IsAncestor(rsabs, rabs) {
					errs = append(errs, fmt.Sprintf(i18n.T("mount dir %q lies inside source %s; the link would be scanned as source content"), m.Dir, s))
				}
			}
		}
		if len(m.Links) == 0 && len(m.Merge) == 0 {
			errs = append(errs, fmt.Sprintf(i18n.T("mount %s has neither links nor merge entries"), m.Dir))
			continue
		}
		for name, sub := range m.Merge {
			if i := strings.Index(name, dupMark); i >= 0 {
				errs = append(errs, fmt.Sprintf(i18n.T("mount %s defines merge.%s more than once with different targets; keep one"), m.Dir, name[:i]))
				continue
			}
			if strings.ContainsAny(name, "/\\") || name == ".." || name == "." {
				errs = append(errs, fmt.Sprintf(i18n.T("mount %s merge name %q is invalid; it must be a plain name without path separators"), m.Dir, name))
				continue
			}
			if _, both := m.Links[name]; both {
				errs = append(errs, fmt.Sprintf(i18n.T("mount %s defines %s both as a link and as a merge target; keep one"), m.Dir, name))
				continue
			}
			clean := strings.Trim(filepath.ToSlash(sub), "/")
			tool := RegistryTool(clean)
			if tool == "" || !HookMergeTools[tool] {
				errs = append(errs, fmt.Sprintf(i18n.T("mount %s merge.%s points to %q, but only a hook registry of the claude / codex shape can be merged, valid values: %v"), m.Dir, name, sub, mergeableRegistryNames()))
				continue
			}
			if !c.HasTool(tool) {
				errs = append(errs, fmt.Sprintf(i18n.T("mount %s merge.%s targets %q, but build.tools does not list %q"), m.Dir, name, sub, tool))
			}
		}
		for name, sub := range m.Links {
			if i := strings.Index(name, dupMark); i >= 0 {
				errs = append(errs, fmt.Sprintf(i18n.T("mount %s defines links.%s more than once with different targets; keep one"), m.Dir, name[:i]))
				continue
			}
			// A link name containing a path separator or dots would create the
			// link outside the mount dir; it must be a single path element.
			if strings.ContainsAny(name, "/\\") || name == ".." || name == "." {
				errs = append(errs, fmt.Sprintf(i18n.T("mount %s link name %q is invalid; it must be a plain name without path separators"), m.Dir, name))
				continue
			}
			clean := strings.Trim(filepath.ToSlash(sub), "/")
			if clean == "" {
				errs = append(errs, fmt.Sprintf(i18n.T("mount %s links.%s has no target"), m.Dir, name))
				continue
			}
			if strings.Contains(clean, "/") {
				errs = append(errs, fmt.Sprintf(i18n.T("mount %s links.%s points to %q, which is nested too deep (targets are top-level only)"), m.Dir, name, sub))
				continue
			}
			if !validTarget[clean] {
				errs = append(errs, fmt.Sprintf(i18n.T("mount %s links.%s points to %q, but the output has no such top level, valid values: %v"),
					m.Dir, name, sub, validTops))
			}
		}
	}
	return errs
}

// mergeableRegistryNames lists the registry file names merge accepts, in
// HookTools order.
func mergeableRegistryNames() []string {
	var out []string
	for _, t := range HookTools {
		if HookMergeTools[t] {
			out = append(out, HookRegistryFiles[t])
		}
	}
	return out
}

// validateHooksMount rejects a link to a hook registry whose tool is not in
// build.tools: build never writes content into that registry, so the mounted
// file would stay empty. The reverse (a tool listed but its registry not
// mounted) is not an error; plan and doctor warn about it when the sources
// contain hooks.
func (c *Config) validateHooksMount() []string {
	var errs []string
	linked := map[string]string{} // registry → "dir/name" of the link consuming it
	for _, m := range c.Mount {
		for name, sub := range m.Links {
			clean := strings.Trim(filepath.ToSlash(sub), "/")
			tool := RegistryTool(clean)
			if tool == "" {
				continue
			}
			if !c.HasTool(tool) {
				errs = append(errs, fmt.Sprintf(i18n.T("mount %s links.%s points to %q, but build.tools does not list %q; the file would stay empty"), m.Dir, name, sub, tool))
			}
			linked[clean] = m.Dir + "/" + name
		}
	}
	// A registry is consumed one way, in one place: linked as a whole file,
	// or merged into one user file. Two consumers mean two places run the
	// same hooks.
	merged := map[string]string{} // registry → "dir/name" of the merge consuming it
	for _, m := range c.Mount {
		for _, name := range sortedKeys(m.Merge) {
			sub := m.Merge[name]
			clean := strings.Trim(filepath.ToSlash(sub), "/")
			if via, both := linked[clean]; both {
				errs = append(errs, fmt.Sprintf(i18n.T("mount %s merge.%s and link %s both consume %q; a registry is either linked or merged, keep one"), m.Dir, name, via, sub))
			}
			if via, twice := merged[clean]; twice {
				errs = append(errs, fmt.Sprintf(i18n.T("mount %s merge.%s and merge %s both consume %q; a registry is merged into one file only, keep one"), m.Dir, name, via, sub))
				continue
			}
			merged[clean] = m.Dir + "/" + name
		}
	}
	return errs
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// HookRegistryMounted reports whether any link or merge entry consumes the
// registry of the given tool.
func (c *Config) HookRegistryMounted(tool string) bool {
	want := HookRegistryFiles[tool]
	for _, m := range c.Mount {
		for _, sub := range m.Links {
			if strings.Trim(filepath.ToSlash(sub), "/") == want {
				return true
			}
		}
		for _, sub := range m.Merge {
			if strings.Trim(filepath.ToSlash(sub), "/") == want {
				return true
			}
		}
	}
	return false
}

// ExpandPath resolves a path written in one of three forms:
// leading ~ → the user's home directory; relative → based on the directory
// containing agsy.yaml; absolute → as-is
func (c *Config) ExpandPath(p string) (string, error) {
	if strings.HasPrefix(p, "~") {
		// Only "~" and "~/..." are supported. "~user" means another user's
		// home directory in a shell; silently expanding it to $HOME/user
		// would point at the wrong place, so it is rejected instead.
		if p != "~" && !strings.HasPrefix(p, "~/") && !strings.HasPrefix(p, `~\`) {
			return "", fmt.Errorf(i18n.T("path %q: the ~user form is not supported; use ~/ for your own home directory or write an absolute path"), p)
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf(i18n.T("cannot resolve home directory to expand %q: %w"), p, err)
		}
		return filepath.Clean(filepath.Join(home, strings.TrimPrefix(p, "~"))), nil
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p), nil
	}
	return filepath.Clean(filepath.Join(c.BaseDir, p)), nil
}

// OutDir returns the absolute path of the build output directory (hard-coding
// .agsy is forbidden; always obtain it here)
func (c *Config) OutDir() string {
	p, _ := c.ExpandPath(c.Build.Out)
	return p
}

// SourceRoots returns the expanded absolute source paths (same order as sources)
func (c *Config) SourceRoots() []string {
	var out []string
	for _, s := range c.Sources {
		abs, err := c.ExpandPath(s)
		if err != nil {
			continue
		}
		out = append(out, abs)
	}
	return out
}

// SourceRootOf finds which source root a path belongs to by longest-prefix
// match; prefix matching stays correct when categories.from is a nested
// path.
func (c *Config) SourceRootOf(path string) (string, bool) {
	best, ok := "", false
	for _, root := range c.SourceRoots() {
		if path == root || IsAncestor(root, path) {
			if len(root) > len(best) {
				best, ok = root, true
			}
		}
	}
	return best, ok
}

// ResolveSymlinks resolves symlinks in the longest existing prefix of p and
// rejoins the remainder, so ancestor checks compare real locations. A path
// that does not exist yet (build.out before the first apply) still resolves
// through its existing ancestors. On any error the cleaned input is returned,
// and the lexical checks behave exactly as before.
func ResolveSymlinks(p string) string {
	cur := filepath.Clean(p)
	rest := ""
	for {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			if rest == "" {
				return filepath.Clean(r)
			}
			return filepath.Clean(filepath.Join(r, rest))
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return filepath.Clean(p)
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

// IsAncestor reports whether a is an ancestor directory of b (false when a == b)
func IsAncestor(a, b string) bool {
	rel, err := filepath.Rel(a, b)
	if err != nil {
		return false
	}
	if rel == "." || rel == "" {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// SourceTag takes the last segment of a source path with any leading dots removed,
// as the candidate rename tag.
// When several sources share the candidate, build.AssignTags disambiguates further.
func SourceTag(srcPath string) string {
	base := filepath.Base(filepath.Clean(srcPath))
	return strings.TrimLeft(base, ".")
}

// HasTool reports whether name is listed in build.tools.
func (c *Config) HasTool(name string) bool {
	return slices.Contains(c.Build.Tools, name)
}
