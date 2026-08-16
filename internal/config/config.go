// Package config handles loading agsy.yaml, defaults, validation and path expansion.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/IngSquared99/agent-sync/i18n"
	"github.com/IngSquared99/agent-sync/internal/yaml"
)

// CategoryOrder is the fixed processing order of categories (used for both scanning and output)
var CategoryOrder = []string{"rules", "skills", "workflows"}

// ValidStrategies are the legal name-conflict strategies
var ValidStrategies = map[string]bool{"first": true, "rename": true, "error": true}

// SchemaVersion is the highest config-file version currently supported
const SchemaVersion = 1

type Category struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

type RouteCfg struct {
	Field   string   `yaml:"field"`
	Default []string `yaml:"default"`
	Buckets []string `yaml:"buckets"`
}

type BuildCfg struct {
	Out        string              `yaml:"out"`
	Categories map[string]Category `yaml:"categories"`
	OnConflict map[string]string   `yaml:"on_conflict"`
	Route      RouteCfg            `yaml:"route"`
}

type MountCfg struct {
	Dir   string            `yaml:"dir"`
	Links map[string]string `yaml:"links"`
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
// §4 guarantees relative paths are resolved against the config file's directory,
// not the user's current directory — for that guarantee to mean anything, the
// config must also be findable when running from a project subdirectory,
// otherwise the user just gets a cold "not found".
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
		c.Version = SchemaVersion // older files without a version are treated as 1
	}
	if c.Build.Out == "" {
		c.Build.Out = ".agsy"
	}
	if c.Build.Categories == nil {
		c.Build.Categories = map[string]Category{}
	}
	def := map[string]Category{
		"rules":     {From: "rule", To: "rules"},
		"skills":    {From: "skill", To: "skills"},
		"workflows": {From: "workflow", To: "workflows"},
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
	if c.Build.Route.Field == "" {
		c.Build.Route.Field = "target"
	}
}

func (c *Config) validate() error {
	var errs []string
	if c.Version > SchemaVersion {
		errs = append(errs, fmt.Sprintf(i18n.T("version: %d exceeds the maximum %d supported by this agsy, please upgrade agsy"), c.Version, SchemaVersion))
	}
	if len(c.Sources) == 0 {
		errs = append(errs, i18n.T("sources is not set; at least one source path is required"))
	}
	// on_conflict is required per category: the user must make an explicit choice;
	// a missing field never silently falls back to a default (§12-1)
	for _, cat := range CategoryOrder {
		s, ok := c.Build.OnConflict[cat]
		if !ok || s == "" {
			errs = append(errs, fmt.Sprintf(i18n.T("build.on_conflict.%s is not set, please explicitly choose first / rename / error"), cat))
			continue
		}
		if !ValidStrategies[s] {
			errs = append(errs, fmt.Sprintf(i18n.T("build.on_conflict.%s value %q is invalid, valid values: first / rename / error"), cat, s))
		}
	}
	// destructive-path checks for build.out: apply and clean both delete this whole directory
	errs = append(errs, c.validateOut()...)
	// Category output subdirectories must differ: if two categories share a level,
	// single files from rules and directories from skills mix in one folder, and
	// conflict detection groups per category (missing cross-category clashes) —
	// the result is one copy being silently overwritten.
	seenTo := map[string]string{}
	for _, cat := range CategoryOrder {
		to := c.Build.Categories[cat].To
		if prev, ok := seenTo[to]; ok {
			errs = append(errs, fmt.Sprintf(i18n.T("build.categories.%s.to and %s are both %q; different categories must output to different subdirectories"), cat, prev, to))
			continue
		}
		seenTo[to] = cat
	}

	if len(c.Mount) == 0 {
		errs = append(errs, i18n.T("mount is not set; at least one mount target is required"))
	}
	if len(c.Build.Route.Buckets) == 0 {
		errs = append(errs, i18n.T("build.route.buckets is not set"))
	}
	for _, d := range c.Build.Route.Default {
		if !contains(c.Build.Route.Buckets, d) {
			errs = append(errs, fmt.Sprintf(i18n.T("build.route.default contains nonexistent bucket %q, valid values: %v"), d, c.Build.Route.Buckets))
		}
	}
	errs = append(errs, c.validateMount()...)

	if len(errs) > 0 {
		return fmt.Errorf(i18n.T("config validation failed:\n  - %s"), strings.Join(errs, "\n  - "))
	}
	return nil
}

// validateOut rejects build.out values that would destroy data (§12-5).
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
	switch {
	case out == c.BaseDir:
		errs = append(errs, fmt.Sprintf(i18n.T("build.out resolves to the project root (%s), apply would wipe the entire project"), out))
	case IsAncestor(out, c.BaseDir):
		errs = append(errs, fmt.Sprintf(i18n.T("build.out (%s) is an ancestor of the project root, apply would wipe the project along with it"), out))
	case !IsAncestor(c.BaseDir, out):
		// The output directory must be a descendant of the project. apply empties
		// it entirely and clean removes it entirely; pointing it outside the project
		// (e.g. ~/Documents) treats the user's folder as disposable output.
		errs = append(errs, fmt.Sprintf(i18n.T("build.out (%s) is not inside the project directory (%s). apply wipes it entirely, so only a dedicated directory inside the project is allowed"), out, c.BaseDir))
	}
	if home, err := os.UserHomeDir(); err == nil {
		h := filepath.Clean(home)
		if out == h || IsAncestor(out, h) {
			errs = append(errs, fmt.Sprintf(i18n.T("build.out (%s) contains the home directory, apply would wipe it"), out))
		}
	}
	for _, s := range c.Sources {
		abs, err := c.ExpandPath(s)
		if err != nil {
			continue
		}
		if out == abs || IsAncestor(out, abs) {
			errs = append(errs, fmt.Sprintf(i18n.T("build.out (%s) contains source %s, apply would delete the source (sources must live outside the output)"), out, s))
		} else if IsAncestor(abs, out) {
			// The other direction matters just as much: an output nested inside
			// a source gets wiped by apply together with the originals around it.
			errs = append(errs, fmt.Sprintf(i18n.T("build.out (%s) lies inside source %s; apply would wipe that part of the source"), out, s))
		}
	}
	return errs
}

// validateMount cross-checks that every path mount.links points at will actually
// be produced by build (§12-14)
func (c *Config) validateMount() []string {
	var errs []string
	// top-level output directory name → category
	toCat := map[string]string{}
	for _, cat := range CategoryOrder {
		toCat[c.Build.Categories[cat].To] = cat
	}
	var validTops []string
	for _, cat := range CategoryOrder {
		validTops = append(validTops, c.Build.Categories[cat].To)
	}
	out := c.OutDir()
	seenDir := map[string]bool{}
	for _, m := range c.Mount {
		if m.Dir == "" {
			errs = append(errs, i18n.T("mount entry is missing dir"))
		} else {
			// mount.dir gets the same basic guards as build.out: duplicate dirs
			// overwrite each other's links, a dir inside the output is wiped by
			// apply, and a dir inside a source contaminates scanning.
			if seenDir[m.Dir] {
				errs = append(errs, fmt.Sprintf(i18n.T("mount dir %q appears more than once; merge its links into one entry"), m.Dir))
			}
			seenDir[m.Dir] = true
			if abs, err := c.ExpandPath(m.Dir); err == nil {
				if abs == out || IsAncestor(out, abs) {
					errs = append(errs, fmt.Sprintf(i18n.T("mount dir %q lies inside build.out; apply empties that directory and the links would be wiped with it"), m.Dir))
				}
				for _, s := range c.Sources {
					sabs, serr := c.ExpandPath(s)
					if serr != nil {
						continue
					}
					if abs == sabs || IsAncestor(sabs, abs) {
						errs = append(errs, fmt.Sprintf(i18n.T("mount dir %q lies inside source %s; the link would be scanned as source content"), m.Dir, s))
					}
				}
			}
		}
		if len(m.Links) == 0 {
			errs = append(errs, fmt.Sprintf(i18n.T("mount %s is missing links"), m.Dir))
			continue
		}
		for name, sub := range m.Links {
			clean := strings.Trim(filepath.ToSlash(sub), "/")
			if clean == "" {
				errs = append(errs, fmt.Sprintf(i18n.T("mount %s links.%s has no target"), m.Dir, name))
				continue
			}
			parts := strings.Split(clean, "/")
			cat, ok := toCat[parts[0]]
			if !ok {
				errs = append(errs, fmt.Sprintf(i18n.T("mount %s links.%s points to %q, but the output has no such top level, valid values: %v"),
					m.Dir, name, sub, validTops))
				continue
			}
			if len(parts) == 1 {
				continue
			}
			if cat != "workflows" {
				errs = append(errs, fmt.Sprintf(i18n.T("mount %s links.%s points to %q, but only workflows has a second bucket level"),
					m.Dir, name, sub))
				continue
			}
			if len(parts) > 2 {
				errs = append(errs, fmt.Sprintf(i18n.T("mount %s links.%s points to %q, which is nested too deep (at most workflows/<bucket>)"),
					m.Dir, name, sub))
				continue
			}
			if !contains(c.Build.Route.Buckets, parts[1]) {
				errs = append(errs, fmt.Sprintf(i18n.T("mount %s links.%s points to bucket %q, but it is not in build.route.buckets, valid values: %v"),
					m.Dir, name, parts[1], c.Build.Route.Buckets))
			}
		}
	}
	return errs
}

// ExpandPath resolves a path written in one of three forms (§4):
// leading ~ → the user's home directory; relative → based on the directory
// containing agsy.yaml; absolute → as-is
func (c *Config) ExpandPath(p string) (string, error) {
	if strings.HasPrefix(p, "~") {
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
// .agsy is forbidden; always obtain it here, §12-5)
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

// SourceRootOf finds which source root a path belongs to by longest-prefix match.
// It replaces the "two levels up" guess: if categories.from is a nested path,
// that guess goes wrong (§12-4).
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
// as the candidate rename tag (§12-2).
// When several sources share the candidate, build.AssignTags disambiguates further.
func SourceTag(srcPath string) string {
	base := filepath.Base(filepath.Clean(srcPath))
	return strings.TrimLeft(base, ".")
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
