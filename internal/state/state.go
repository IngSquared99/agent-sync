// Package state implements the detection logic behind status: a three-way
// comparison of sources, manifest, and outputs. The data flow is strictly one
// way (sources → outputs), so the report answers two questions:
//   - list A: which sources changed → apply will update these
//   - list B: what was changed on the artifact side → apply will overwrite it
//
// This package is always read-only.
package state

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/IngSquared99/agent-sync/internal/build"
	"github.com/IngSquared99/agent-sync/internal/config"
	"github.com/IngSquared99/agent-sync/internal/mount"
)

type SrcKind int

const (
	SrcChanged     SrcKind = iota // source content has changed
	SrcDeleted                    // source was deleted (the item disappears after apply ⚠)
	SrcRootMissing                // the whole source root is missing (shared repo not cloned / external disk not mounted)
)

// SourceChange is one row of list A: a source the next apply will re-sync.
type SourceChange struct {
	Item  build.ManifestItem
	Kind  SrcKind
	Files []string // directory items: the files that actually changed on the source side
}

type NewItem struct {
	Category string
	Name     string
	From     string
}

// ArtifactChange is one row of list B: something on the artifact side differs
// from what build wrote, and the next apply will overwrite or delete it.
// There is no write-back; Item.From (when set) is where to merge the change
// manually if it should be kept.
type ArtifactChange struct {
	Item      *build.ManifestItem // nil for untracked files
	Path      string              // path relative to the out directory
	Untracked bool                // file unknown to the manifest (added on the artifact side)
	Derived   bool                // a converted form (stub, derived skill, AGENTS.md)
	Files     []string            // directory outputs: the files that actually changed
}

// GoneOut means an output copy has disappeared (deleted manually or by an
// external force). The mount side really is missing something — without
// reporting it separately, status would claim "in sync" while files are
// missing. apply rebuilds it.
type GoneOut struct {
	Item     build.ManifestItem
	OutPaths []string
}

type Report struct {
	SourceChanges  []SourceChange   // list A: sources → outputs
	News           []NewItem        // list A: new files in the sources
	Artifacts      []ArtifactChange // list B: artifact-side changes apply will discard
	Gone           []GoneOut        // missing output copies (apply rebuilds them)
	RouteErrors    []string         // workflow target problems (apply refuses to build)
	ForeignFrom    []string         // manifest From paths outside the configured sources (untrusted, never compared)
	ScanErr        error            // non-nil when the source scan failed and new-item detection did not run
	Links          []mount.LinkPlan
	Merges         []mount.MergePlan // merge targets (Claude settings.json); Modified/Absent/Missing/Invalid count as gaps
	MergeBad       int               // merge targets that are neither Clean nor Idle, plus merge orphans
	MergeOrphans   []string          // files an earlier apply merged into that the mount config no longer names, still holding agsy groups
	Orphans        []string          // links created by an earlier apply that the current mount config no longer references
	LinkBad        int               // number of links that are missing, occupied, or orphaned
	MissingSources []string          // sources whose whole path is missing (as originally written); reported first
	HasGap         bool
}

// Collect produces the full status report (read-only).
func Collect(cfg *config.Config, m *build.Manifest) (*Report, error) {
	r := &Report{}
	out := cfg.OutDir()

	// Expand sources first. "The whole source path is missing" (shared repo
	// not cloned, external disk not mounted) and "a single file was deleted"
	// are different conditions and are reported separately: a missing root is
	// an environment problem, not deleted files.
	sources, err := build.ExpandSources(cfg)
	if err != nil {
		return nil, err
	}
	missingRoots := map[string]bool{}
	for _, s := range sources {
		if !s.Exists {
			missingRoots[s.Abs] = true
			r.MissingSources = append(r.MissingSources, s.Raw)
		}
	}

	// List A: sources → outputs.
	known := map[string]bool{} // source paths already in the manifest
	for _, it := range m.Items {
		if it.From == "" { // derived-only items (AGENTS.md) have no source of their own
			continue
		}
		known[it.From] = true
		if _, ok := cfg.SourceRootOf(it.From); !ok {
			// The manifest is untrusted and From is an arbitrary absolute path:
			// hashing it would follow whatever a tampered record points at (or
			// walk a huge tree). It also legitimately appears after editing
			// sources. Either way it is reported, never compared — the next
			// apply rebuilds from the current sources.
			r.ForeignFrom = append(r.ForeignFrom, it.From)
			continue
		}
		if _, err := os.Stat(it.From); err != nil {
			kind := SrcDeleted
			if root, ok := cfg.SourceRootOf(it.From); ok && missingRoots[root] {
				kind = SrcRootMissing
			}
			r.SourceChanges = append(r.SourceChanges, SourceChange{Item: it, Kind: kind})
			continue
		}
		h, files, err := build.HashPath(it.From)
		if err != nil {
			return nil, err
		}
		if h != it.SrcHash {
			r.SourceChanges = append(r.SourceChanges, SourceChange{Item: it, Kind: SrcChanged, Files: build.DiffFiles(it.SrcFiles, files)})
		}
	}
	// New items: scan the current sources for paths the manifest does not know.
	plan, err := build.Compute(cfg, sources)
	if err != nil {
		// A scan failure must not block status, but must not be silent either:
		// record the cause so the report can state new-item detection did not run.
		r.ScanErr = err
	} else {
		r.RouteErrors = plan.RouteErrors
		for _, it := range plan.Items {
			if !known[it.From] {
				r.News = append(r.News, NewItem{Category: it.Category, Name: it.Name, From: it.From})
			}
		}
	}

	// List B: artifact-side changes.
	for i := range m.Items {
		it := &m.Items[i]
		var gone []string
		for _, o := range it.Outs {
			p := filepath.Join(out, filepath.FromSlash(o.Path))
			if _, err := os.Stat(p); err != nil {
				// The output was deleted: not a change to discard, but it must
				// be reported — silently skipping would let status claim "in
				// sync" while the mount side is missing files.
				gone = append(gone, o.Path)
				continue
			}
			h, files, err := build.HashPath(p)
			if err != nil {
				return nil, err
			}
			if h != o.Hash {
				r.Artifacts = append(r.Artifacts, ArtifactChange{
					Item: it, Path: o.Path, Derived: o.Derived,
					Files: build.DiffFiles(o.Files, files),
				})
			}
		}
		if len(gone) > 0 {
			r.Gone = append(r.Gone, GoneOut{Item: *it, OutPaths: gone})
		}
	}

	// Untracked files: present in the output but unknown to the manifest.
	// Without this, status reports "in sync" right before apply deletes them.
	knownOut := map[string]bool{}
	for _, it := range m.Items {
		for _, o := range it.Outs {
			knownOut[o.Path] = true
		}
	}
	_ = filepath.WalkDir(out, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		// Hidden files (.DS_Store and friends) are never flagged as untracked:
		// top-level dot entries are never accepted from sources, and a dotfile
		// INSIDE a skill directory is copied with its skill but belongs to
		// that item — it is compared through the item's per-file hashes above,
		// not as an untracked stray. Reporting dotfiles here would flag
		// .DS_Store on every run.
		if strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		rel, rerr := filepath.Rel(out, p)
		if rerr != nil {
			return nil
		}
		slash := filepath.ToSlash(rel)
		if slash == build.ManifestName || knownOut[slash] {
			return nil
		}
		for k := range knownOut {
			// files inside a directory output belong to that item and were
			// already compared as a content change, not as untracked
			if strings.HasPrefix(slash, k+"/") {
				return nil
			}
		}
		r.Artifacts = append(r.Artifacts, ArtifactChange{Path: slash, Untracked: true})
		return nil
	})
	sort.Slice(r.Artifacts, func(i, j int) bool { return r.Artifacts[i].Path < r.Artifacts[j].Path })

	// Mount check
	links, err := mount.Inspect(cfg)
	if err != nil {
		return nil, err
	}
	r.Links = links
	for _, l := range links {
		if l.State != mount.IsLink {
			r.LinkBad++
		}
	}
	// Orphaned links: recorded by an earlier apply but absent from the current
	// mount config. A tool still reading one gets stale (or broken) content
	// while everything else looks green, so they count as mount anomalies.
	// Only paths that verifiably ARE links into the output are reported — the
	// manifest is untrusted.
	current := map[string]bool{}
	for _, l := range links {
		current[l.LinkPath] = true
	}
	for _, lp := range m.Mounts {
		if current[lp] {
			continue
		}
		if mount.IsManagedLink(lp, out) {
			r.Orphans = append(r.Orphans, lp)
		}
	}
	sort.Strings(r.Orphans)
	r.LinkBad += len(r.Orphans)

	// Merge targets: ownership is re-derived from the file, the manifest only
	// supplies the hash of the last write. Modified means the artifact side
	// changed (apply overwrites after confirmation); Missing/Absent mean the
	// entries are gone (apply restores); Invalid blocks apply.
	merges, err := mount.InspectMerge(cfg, m.Merges)
	if err != nil {
		return nil, err
	}
	r.Merges = merges
	for _, mp := range merges {
		if mp.State != mount.MergeClean && mp.State != mount.MergeIdle {
			r.MergeBad++
		}
	}
	// Same rule as orphaned links: a merge target dropped from the config
	// keeps feeding agsy's old hooks to the tool until someone removes them.
	orphans, err := mount.MergeOrphans(cfg, m.Merges)
	if err != nil {
		return nil, err
	}
	r.MergeOrphans = orphans
	r.MergeBad += len(orphans)

	r.HasGap = len(r.SourceChanges) > 0 || len(r.News) > 0 || len(r.Artifacts) > 0 ||
		len(r.Gone) > 0 || len(r.RouteErrors) > 0 || len(r.ForeignFrom) > 0 || r.LinkBad > 0 || r.MergeBad > 0
	return r, nil
}
