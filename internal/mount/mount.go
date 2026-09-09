// Package mount implements the mount phase: creating a link for every entry
// of every mount block.
// Directory targets: non-Windows uses relative-path symlinks; Windows uses
// junctions (no privileges needed).
// File targets (the derived AGENTS.md): non-Windows uses relative-path
// symlinks; Windows uses hard links (no privileges needed either).
// Existing-path rules: links are always deleted and recreated; real
// directories/files raise an error and are never deleted.
package mount

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/IngSquared99/agent-sync/i18n"
	"github.com/IngSquared99/agent-sync/internal/config"
)

// LinkState is the current state of a single link.
type LinkState int

const (
	Missing LinkState = iota // does not exist → will be created
	IsLink                   // already a link pointing to the right target → delete and recreate (idempotent)
	IsStale                  // a link, but points elsewhere or the target is gone (broken) → abnormal, recreating fixes it
	IsReal                   // real directory or file → raise an error, never delete
)

type LinkPlan struct {
	Dir      string // mount directory as written in the config, e.g. .claude
	Name     string // link name, e.g. skills
	LinkPath string // full path of the link
	Target   string // link target (relative form, for display)
	AbsTgt   string // absolute path of the target
	FileTgt  bool   // the target is a single file (AGENTS.md), not a directory
	State    LinkState
	Current  string // where the link currently points, if it is one (for display)
	Note     string // explains what is wrong when IsStale
}

// Inspect reports the current state of every mount target (shared by
// plan / status / doctor, read-only).
// A link pointing elsewhere, or whose target no longer exists, is effectively
// unmounted; these states are reported separately.
func Inspect(cfg *config.Config) ([]LinkPlan, error) {
	out := cfg.OutDir()
	var plans []LinkPlan
	for _, m := range cfg.Mount {
		mdir, err := cfg.ExpandPath(m.Dir)
		if err != nil {
			return nil, err
		}
		// links sorted by name for stable output
		names := make([]string, 0, len(m.Links))
		for k := range m.Links {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, name := range names {
			sub := m.Links[name]
			absTgt := filepath.Join(out, filepath.FromSlash(sub))
			linkPath := filepath.Join(mdir, name)
			rel, err := filepath.Rel(mdir, absTgt)
			if err != nil {
				rel = absTgt
			}
			lp := LinkPlan{
				Dir: m.Dir, Name: name, LinkPath: linkPath,
				Target: filepath.ToSlash(rel), AbsTgt: absTgt,
				FileTgt: IsFileTarget(sub),
			}
			inspectOne(&lp)
			plans = append(plans, lp)
		}
	}
	return plans, nil
}

func inspectOne(lp *LinkPlan) {
	fi, err := os.Lstat(lp.LinkPath)
	switch {
	case err != nil:
		lp.State = Missing
	case isLink(fi, lp.LinkPath):
		lp.State = IsLink
		// Readlink can fail for junctions on some platforms; an unreadable
		// target is not classified as stale — only the broken-link check
		// below still applies.
		if cur, err := os.Readlink(lp.LinkPath); err == nil {
			lp.Current = filepath.ToSlash(cur)
			resolved := cur
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(filepath.Dir(lp.LinkPath), resolved)
			}
			if filepath.Clean(resolved) != filepath.Clean(lp.AbsTgt) {
				lp.State = IsStale
				lp.Note = i18n.T("points to ") + lp.Current + i18n.T(", not the currently configured target")
			}
		}
		// Broken link: the link itself exists, but following it leads nowhere.
		if lp.State == IsLink {
			if _, err := os.Stat(lp.LinkPath); err != nil {
				lp.State = IsStale
				lp.Note = i18n.T("target does not exist (broken link); tools will fail outright when reading it")
			}
		}
	case lp.FileTgt && fi.Mode().IsRegular():
		// File targets on Windows are hard links: indistinguishable from a
		// plain file by mode, so identity with the target decides.
		if sameAsTarget(lp.LinkPath, lp.AbsTgt) {
			lp.State = IsLink
			break
		}
		lp.State = IsReal
	default:
		lp.State = IsReal
	}
}

// sameAsTarget reports whether path and target are the same file (hard link
// identity). Only meaningful for file targets.
func sameAsTarget(path, target string) bool {
	a, err := os.Stat(path)
	if err != nil {
		return false
	}
	b, err := os.Stat(target)
	if err != nil {
		return false
	}
	return os.SameFile(a, b)
}

// IsFileTarget reports whether a link target names one of the derived files
// (AGENTS.md, hook registries) rather than a category directory.
func IsFileTarget(sub string) bool {
	clean := strings.Trim(filepath.ToSlash(sub), "/")
	return clean == config.AgentsMD || config.IsRegistryFile(clean)
}

// IsManagedLink reports whether path is a link created by this tool: it must
// be a link and it must resolve into the build output directory. Junctions
// whose target cannot be read are conservatively not claimed (never delete
// on an unverified target). This is the safety check every consumer of
// manifest.Mounts must pass a path through — the manifest itself is untrusted.
func IsManagedLink(linkPath, outDir string) bool {
	fi, err := os.Lstat(linkPath)
	if err != nil {
		return false
	}
	if !isLink(fi, linkPath) {
		// A hard-linked file mount (AGENTS.md or a hook registry on Windows)
		// is claimed only when it is the same file as the derived artifact
		// inside the output.
		if fi.Mode().IsRegular() {
			for _, name := range config.ReservedTopNames() {
				if sameAsTarget(linkPath, filepath.Join(outDir, name)) {
					return true
				}
			}
		}
		return false
	}
	cur, err := os.Readlink(linkPath)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(cur) {
		cur = filepath.Join(filepath.Dir(linkPath), cur)
	}
	cur = filepath.Clean(cur)
	out := filepath.Clean(outDir)
	return cur == out || config.IsAncestor(out, cur)
}

// Probe tests whether this machine can create directory links (used by
// doctor): it creates a temporary directory and link under baseDir, and
// cleans them up immediately on success. Support is verified by actually
// creating a link rather than assuming platform behavior.
func Probe(baseDir string) error {
	tmp, err := os.MkdirTemp(baseDir, ".agsy-probe-")
	if err != nil {
		return fmt.Errorf(i18n.T("cannot create temp directory under %s: %w"), baseDir, err)
	}
	defer os.RemoveAll(tmp)
	target := filepath.Join(tmp, "target")
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	if err := linkDir(target, filepath.Join(tmp, "link")); err != nil {
		return fmt.Errorf("%w%s", err, platformHint())
	}
	return nil
}

// Apply creates links according to the rules. On real directories it returns
// an error (never deletes); the caller decides how to proceed.
func Apply(cfg *config.Config, plans []LinkPlan) error {
	var real []string
	for _, p := range plans {
		if p.State == IsReal {
			real = append(real, p.LinkPath)
		}
	}
	if len(real) > 0 {
		return fmt.Errorf(i18n.T("the following mount targets already exist as real directories or files (not created by this tool; refusing to delete):\n  %s\nplease move or delete them manually, then retry"),
			strings.Join(real, "\n  "))
	}
	for _, p := range plans {
		if err := os.MkdirAll(filepath.Dir(p.LinkPath), 0o755); err != nil {
			return err
		}
		if p.State == IsLink || p.State == IsStale {
			if err := os.Remove(p.LinkPath); err != nil {
				return fmt.Errorf(i18n.T("failed to remove old link %s: %w"), p.LinkPath, err)
			}
		}
		var err error
		if p.FileTgt {
			err = linkFile(p.AbsTgt, p.LinkPath)
		} else {
			err = linkDir(p.AbsTgt, p.LinkPath)
		}
		if err != nil {
			return fmt.Errorf(i18n.T("failed to create link %s: %w%s"), p.LinkPath, err, platformHint())
		}
	}
	return nil
}

// RemoveLinks removes every mount that is a link (used by clean); real paths
// are skipped and reported.
func RemoveLinks(cfg *config.Config) (removed, skipped []string, err error) {
	plans, err := Inspect(cfg)
	if err != nil {
		return nil, nil, err
	}
	for _, p := range plans {
		switch p.State {
		case IsLink, IsStale:
			if e := os.Remove(p.LinkPath); e != nil {
				return removed, skipped, e
			}
			removed = append(removed, p.LinkPath)
			// If the mount directory becomes empty as a result, remove it too
			// (the empty directory was also created by this tool).
			if entries, e := os.ReadDir(filepath.Dir(p.LinkPath)); e == nil && len(entries) == 0 {
				_ = os.Remove(filepath.Dir(p.LinkPath))
			}
		case IsReal:
			skipped = append(skipped, p.LinkPath)
		}
	}
	return removed, skipped, nil
}

// ClearFileLinks removes every FILE-target mount that is currently a
// verified (or stale) link. apply calls it after the confirmations, before
// the rebuild wipes the output: on Windows the AGENTS.md mount is a hard
// link, and rebuilding the artifact under a new inode would strand the old
// file at the mount path as an untouchable "real file". Removing the link
// first lets the mount step recreate it; on other platforms Apply deletes
// and recreates the symlink anyway, so behavior is identical.
func ClearFileLinks(cfg *config.Config) error {
	plans, err := Inspect(cfg)
	if err != nil {
		return err
	}
	for _, p := range plans {
		if !p.FileTgt || (p.State != IsLink && p.State != IsStale) {
			continue
		}
		if err := os.Remove(p.LinkPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf(i18n.T("failed to remove old link %s: %w"), p.LinkPath, err)
		}
	}
	return nil
}
