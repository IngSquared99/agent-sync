// Package state implements the detection logic behind status: a three-way
// comparison of source, manifest, and outputs. Gaps go in two directions:
// the source changed (needs apply) or the output changed (needs promote).
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

type LagKind int

const (
	SrcChanged     LagKind = iota // source content has changed
	SrcDeleted                    // source was deleted (the item disappears after apply ⚠)
	SrcRootMissing                // the whole source root is missing (shared repo not cloned / external disk not mounted)
)

type Lag struct {
	Item  build.ManifestItem
	Kind  LagKind
	Files []string // directory items: the files that actually changed on the source side (for "content changed (N files)")
}

type NewItem struct {
	Category string
	Name     string
	From     string
}

type LocalChange struct {
	Item        build.ManifestItem
	OutPath     string   // the output copy where the change was detected (relative to out)
	Multiple    bool     // workflow has multiple copies and more than one was modified
	SrcAlso     bool     // the source changed too (promote needs manual resolution)
	SrcDeleted  bool     // source file was deleted (promote recreates it; apply drops the item)
	SrcRootGone bool     // whole source root is missing (promote refuses)
	Files       []string // directory items: the files that actually changed on the output side
}

// GoneOut means an output copy has disappeared (deleted manually or by an
// external force). It is not a local change (there is no content to write
// back) and not a mount problem (the link itself is fine), but the mount
// side really is missing something — without reporting it separately,
// status would claim "in sync" while files are missing.
type GoneOut struct {
	Item     build.ManifestItem
	OutPaths []string // the missing output copies (relative to out)
}

type Report struct {
	Lags           []Lag
	News           []NewItem
	Locals         []LocalChange
	Gone           []GoneOut // missing output copies (apply can rebuild them)
	Untracked      []string  // output files the manifest does not track (added on the mount side; apply deletes them)
	RouteErrors    []string  // workflow routing problems (apply refuses to build)
	ScanErr        error     // non-nil when the source scan failed and new-item detection did not run
	Links          []mount.LinkPlan
	LinkBad        int      // number of links that are missing or occupied by a real file
	MissingSources []string // sources whose whole path is missing (as originally written); reported first
	HasGap         bool
}

// Collect produces the full status report (read-only).
func Collect(cfg *config.Config, m *build.Manifest) (*Report, error) {
	r := &Report{}
	out := cfg.OutDir()

	// Expand sources first. "The whole source path is missing" (shared repo
	// not cloned, external disk not mounted) and "a single file was deleted"
	// are different things; reporting both as "source deleted" would make
	// users think their files are gone.
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

	// Direction one: source → output (needs apply)
	known := map[string]bool{} // source paths already in the manifest
	for _, it := range m.Items {
		known[it.From] = true
		if _, err := os.Stat(it.From); err != nil {
			kind := SrcDeleted
			if root, ok := cfg.SourceRootOf(it.From); ok && missingRoots[root] {
				kind = SrcRootMissing
			}
			r.Lags = append(r.Lags, Lag{Item: it, Kind: kind})
			continue
		}
		h, files, err := build.HashPath(it.From)
		if err != nil {
			return nil, err
		}
		// The lag check uses SrcHash (the source fingerprint at build time),
		// kept separate from the output hash, so a renamed skill (whose
		// front matter was rewritten) is not misreported as forever lagging.
		if h != it.SrcHash {
			r.Lags = append(r.Lags, Lag{Item: it, Kind: SrcChanged, Files: build.DiffFiles(it.SrcFiles, files)})
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
			// A workflow with empty route.default and no target never reaches
			// the outputs or the manifest. Without this exemption it would be
			// reported as "new" every time, and apply could never clear it.
			if it.Category == "workflows" && len(it.Buckets) == 0 {
				continue
			}
			if !known[it.From] {
				r.News = append(r.News, NewItem{Category: it.Category, Name: it.Name, From: it.From})
			}
		}
	}

	// Direction two: output → source (needs promote)
	for _, it := range m.Items {
		var changed []string
		var changedHashes []string
		var changedFiles []string
		var gone []string
		for _, rel := range it.OutPaths {
			p := filepath.Join(out, filepath.FromSlash(rel))
			if _, err := os.Stat(p); err != nil {
				// The output copy was deleted: not a local change (no content
				// to write back), but it must be reported — silently skipping
				// would let status claim "in sync" while the mount side is
				// missing files.
				gone = append(gone, rel)
				continue
			}
			h, files, err := build.HashPath(p)
			if err != nil {
				return nil, err
			}
			if h != it.Hash {
				changed = append(changed, rel)
				changedHashes = append(changedHashes, h)
				if len(changedFiles) == 0 {
					changedFiles = build.DiffFiles(it.Files, files)
				}
			}
		}
		if len(changed) > 0 {
			// Note: promote updates the manifest's output hash directly for
			// items it writes back, so a just-promoted item is not re-listed
			// here as a local change. There is no need for an extra
			// output-vs-source content comparison to exempt it (a renamed
			// skill would never compare equal anyway).
			// Multiple means the copies diverged from EACH OTHER, not merely that
			// more than one copy changed: identical edits across buckets are one
			// change (write-back syncs every copy anyway). Only genuinely
			// different contents need a human to pick a winner.
			distinct := map[string]bool{}
			for _, ch := range changedHashes {
				distinct[ch] = true
			}
			lc := LocalChange{Item: it, OutPath: changed[0], Multiple: len(distinct) > 1, Files: changedFiles}
			// Concurrent source-side changes carry three distinct marks: content
			// changed (manual merge), file deleted (promote recreates it explicitly),
			// root missing (promote refuses).
			for _, lag := range r.Lags {
				if lag.Item.From != it.From {
					continue
				}
				switch lag.Kind {
				case SrcChanged:
					lc.SrcAlso = true
				case SrcDeleted:
					lc.SrcDeleted = true
				case SrcRootMissing:
					lc.SrcRootGone = true
				}
			}
			r.Locals = append(r.Locals, lc)
		}
		if len(gone) > 0 {
			r.Gone = append(r.Gone, GoneOut{Item: it, OutPaths: gone})
		}
	}

	// Untracked files: present in the output but unknown to the manifest.
	// Without this, status reports "in sync" right before apply deletes them.
	knownOut := map[string]bool{}
	for _, it := range m.Items {
		for _, rel := range it.OutPaths {
			knownOut[rel] = true
		}
	}
	_ = filepath.WalkDir(out, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		// Hidden files (.DS_Store and friends) are never build outputs and are
		// never collected from sources (Accepts skips the same dot prefix);
		// reporting them as untracked would nag macOS users on every status.
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
			// directory items (skills): files inside belong to the item and
			// count as a local change, not as untracked
			if strings.HasPrefix(slash, k+"/") {
				return nil
			}
		}
		r.Untracked = append(r.Untracked, slash)
		return nil
	})
	sort.Strings(r.Untracked)

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

	r.HasGap = len(r.Lags) > 0 || len(r.News) > 0 || len(r.Locals) > 0 || len(r.Gone) > 0 ||
		len(r.Untracked) > 0 || len(r.RouteErrors) > 0 || r.LinkBad > 0
	return r, nil
}
