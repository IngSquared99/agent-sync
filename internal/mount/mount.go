// Package mount implements the mount phase: creating a directory link for
// every link of every mount entry.
// Non-Windows uses relative-path symlinks; Windows uses junctions (no
// privileges needed, §12-13).
// Existing-path rules (§12-10): links are always deleted and recreated; real
// directories/files raise an error and are never deleted.
package mount

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/IngSquared99/agent-sync/internal/config"
	"github.com/IngSquared99/agent-sync/i18n"
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
	Dir      string // mount directory, e.g. .claude
	Name     string // link name, e.g. skills
	LinkPath string // full path of the link
	Target   string // link target (relative form, for display)
	AbsTgt   string // absolute path of the target
	State    LinkState
	Current  string // where the link currently points, if it is one (for display)
	Note     string // explains what is wrong when IsStale
}

// Inspect reports the current state of every mount target (shared by
// plan / status / doctor, read-only).
// Knowing "is it a link" is not enough: a link pointing elsewhere, or whose
// target no longer exists, is as good as not mounted from the user's point
// of view — those cases must be told apart (§12-12).
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
			lp := LinkPlan{Dir: m.Dir, Name: name, LinkPath: linkPath, Target: filepath.ToSlash(rel), AbsTgt: absTgt}
			fi, err := os.Lstat(linkPath)
			switch {
			case err != nil:
				lp.State = Missing
			case isLink(fi, linkPath):
				lp.State = IsLink
				// Does it point to the right place? Readlink can fail for
				// junctions on some platforms; when it cannot be read, don't
				// jump to conclusions — keep only the broken-link check below.
				if cur, err := os.Readlink(linkPath); err == nil {
					lp.Current = filepath.ToSlash(cur)
					resolved := cur
					if !filepath.IsAbs(resolved) {
						resolved = filepath.Join(mdir, resolved)
					}
					if filepath.Clean(resolved) != filepath.Clean(absTgt) {
						lp.State = IsStale
						lp.Note = i18n.T("points to ") + lp.Current + i18n.T(", not the currently configured target")
					}
				}
				// Broken link: the link itself exists, but following it leads nowhere.
				if lp.State == IsLink {
					if _, err := os.Stat(linkPath); err != nil {
						lp.State = IsStale
						lp.Note = i18n.T("target does not exist (broken link); tools will fail outright when reading it")
					}
				}
			default:
				lp.State = IsReal
			}
			plans = append(plans, lp)
		}
	}
	return plans, nil
}

// Probe actually tests whether this machine can create directory links (used
// by doctor): it creates a temporary directory and link under baseDir, and
// cleans them up immediately on success.
// "Windows junctions need no privileges" is a claim; actually creating one
// is the real check.
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
			joinLines(real))
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
		if err := linkDir(p.AbsTgt, p.LinkPath); err != nil {
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

func joinLines(xs []string) string {
	s := ""
	for i, x := range xs {
		if i > 0 {
			s += "\n  "
		}
		s += x
	}
	return s
}
