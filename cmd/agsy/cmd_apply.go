package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/IngSquared99/agent-sync/i18n"
	"github.com/IngSquared99/agent-sync/internal/build"
	"github.com/IngSquared99/agent-sync/internal/config"
	"github.com/IngSquared99/agent-sync/internal/mount"
	"github.com/IngSquared99/agent-sync/internal/prompt"
	"github.com/IngSquared99/agent-sync/internal/state"
)

// cmdApply is the commit flow:
// pre-checks → list A (sources that will sync) → confirm list B (artifact-side
// changes that will be discarded) → wipe → build → mount.
// The order is fixed: wiping before the confirmation would make it meaningless.
func cmdApply() int {
	cfg, err := loadConfig()
	if err != nil {
		return errExit(err)
	}
	release, err := acquireLock(cfg)
	if err != nil {
		return errExit(err)
	}
	defer release()

	// Pre-check: all sources must exist (plan may run incomplete, apply must not)
	sources, err := build.ExpandSources(cfg)
	if err != nil {
		return errExit(err)
	}
	var missing []string
	for _, s := range sources {
		if !s.Exists {
			missing = append(missing, s.Raw+" ("+s.Abs+")")
		}
	}
	if len(missing) > 0 {
		fmt.Println(i18n.T("✘ the following source paths do not exist; apply refuses to rebuild from an incomplete source list:"))
		for _, m := range missing {
			fmt.Println("  -", m)
		}
		return 1
	}

	// Pre-check: mount targets occupied by real files.
	// Detectable before build; surfacing it only at the mount stage would
	// discard artifact-side changes and spend a full rebuild on a run that
	// cannot complete. This also covers a pre-existing real AGENTS.md at the
	// project root: it is never merged or overwritten.
	preLinks, err := mount.Inspect(cfg)
	if err != nil {
		return errExit(err)
	}
	var occupied []string
	for _, l := range preLinks {
		if l.State == mount.IsReal {
			occupied = append(occupied, l.LinkPath)
		}
	}
	if len(occupied) > 0 {
		fmt.Println(i18n.T("✘ the following mount targets are already occupied by a real directory or file (not created by this tool, refusing to delete); move or delete them manually first:"))
		for _, p := range occupied {
			fmt.Println("  -", p)
		}
		fmt.Println(i18n.T("  (for a real AGENTS.md: move its content into a source rules/ directory, or rename the file to keep it)"))
		return 1
	}
	// Pre-check: merge targets that cannot be written (a symlink, or not a
	// JSON object). Same reasoning: detectable now, so fail before the
	// rebuild rather than after it.
	preMerges, err := mount.InspectMerge(cfg, nil)
	if err != nil {
		return errExit(err)
	}
	var badMerge []string
	for _, mp := range preMerges {
		if mp.State == mount.MergeInvalid {
			badMerge = append(badMerge, mp.FilePath+"  ("+mp.Note+")")
		}
	}
	if len(badMerge) > 0 {
		fmt.Println(i18n.T("✘ the following merge targets cannot be updated (symbolic link or not a JSON object); fix them manually first:"))
		for _, b := range badMerge {
			fmt.Println("  -", b)
		}
		return 1
	}

	// Compute runs before the confirmation (read-only): fatal problems
	// (target errors, name conflicts, collisions) must surface before the
	// discard confirmation.
	p, err := build.Compute(cfg, sources)
	if err != nil {
		return errExit(err)
	}
	if len(p.RouteErrors) > 0 {
		fmt.Println(i18n.T("✘ workflow target or hook declaration problems; fix these files first:"))
		for _, e := range p.RouteErrors {
			fmt.Println("  -", e)
		}
		return 1
	}
	if len(p.Conflicts) > 0 {
		fmt.Println(i18n.T("✘ name conflicts (strategy error), resolve them first (agsy plan shows the full list):"))
		for _, c := range p.Conflicts {
			fmt.Printf(i18n.T("  %s/%s (%d sources)\n"), c.Category, c.Name, len(c.Froms))
		}
		return 1
	}
	if len(p.Collisions) > 0 {
		fmt.Println(i18n.T("✘ the final output paths of the following items collide; proceeding would overwrite one of them:"))
		for _, c := range p.Collisions {
			fmt.Printf("  %s\n", c.OutName)
			for _, f := range c.Froms {
				fmt.Printf("    - %s\n", f)
			}
		}
		fmt.Println(i18n.T("  Rename one of them, or switch to on_conflict: error and resolve it manually."))
		return 1
	}

	// Lists A and B (every run).
	// Three cases must be kept apart:
	//   manifest readable → compare item by item, ask only if something changed
	//   manifest unreadable but the output dir still exists → whether corrupted
	//   or deleted, there is no way to know what's inside, so always ask
	//   output dir does not exist → first build, nothing can be overwritten
	out := cfg.OutDir()
	m, mErr := build.LoadManifest(out)
	switch {
	case mErr == nil:
		rep, err := state.Collect(cfg, m)
		if err != nil {
			return errExit(err)
		}
		printSourceChanges(rep)
		printForeignFrom(rep.ForeignFrom)
		modifiedMerges := 0
		for _, mp := range rep.Merges {
			if mp.State == mount.MergeModified {
				modifiedMerges++
			}
		}
		if len(rep.Artifacts) > 0 || modifiedMerges > 0 {
			if len(rep.Artifacts) > 0 {
				printArtifactChanges(cfg, rep, true)
			}
			if modifiedMerges > 0 {
				printMergeChanges(rep, true)
			}
			if !prompt.Confirm(i18n.T("Discard these artifact-side changes and rebuild?")) {
				fmt.Println(i18n.T("Cancelled."))
				return 1
			}
		}
	case dirExists(out):
		fmt.Printf(i18n.T("⚠ %s/ exists but the manifest cannot be read (%v).\n"), cfg.Build.Out, mErr)
		fmt.Println(i18n.T("  There is no way to tell whether it holds changes made by you or the AI; rebuilding will wipe it entirely."))
		if !prompt.Confirm(i18n.T("Wipe and rebuild?")) {
			fmt.Println(i18n.T("Cancelled."))
			return 1
		}
	}

	if len(p.Ignored) > 0 {
		fmt.Printf(i18n.T("⚠ %d files do not match the inclusion rules and were left out (agsy plan shows the list)\n"), len(p.Ignored))
	}
	// Windows mounts AGENTS.md as a hard link: rebuilding the artifact under a
	// new inode would strand the old file at the mount path, and the post-build
	// inspection would then refuse to replace it as a "real file". Remove the
	// verified file links up front; the mount step recreates them against the
	// fresh artifact.
	if err := mount.ClearFileLinks(cfg); err != nil {
		return errExit(err)
	}
	// The previous apply's merge records (created flag, containers agsy
	// added, previous hooks directory) travel through the build: Execute
	// writes them first, so they survive a rebuild that fails halfway.
	var oldMerges []build.MergeRecord
	if mErr == nil && m != nil {
		oldMerges = m.Merges
	}
	newM, err := build.ExecuteWith(cfg, p, oldMerges)
	if err != nil {
		fmt.Println("✘", err)
		fmt.Println(i18n.T("(the rebuild did not finish; the output and file links may be incomplete — fix the issue and rerun agsy apply)"))
		return 1
	}
	fmt.Printf(i18n.T("✔ build done: %d items → %s/\n"), p.Placed(), cfg.Build.Out)

	// mount
	links, err := mount.Inspect(cfg)
	if err != nil {
		return errExit(err)
	}
	if err := mount.Apply(cfg, links); err != nil {
		fmt.Println("✘", err)
		fmt.Printf(i18n.T("(build finished, %s/ is intact; only the mount step is incomplete — fix the issue and rerun agsy apply)\n"), cfg.Build.Out)
		return 1
	}
	fmt.Printf(i18n.T("✔ mount done: %d links\n"), len(links))

	// merge (Claude Code's settings.json): the registry's groups replace the
	// agsy-owned groups; everything else in the file is preserved.
	merges, err := mount.InspectMerge(cfg, oldMerges)
	if err != nil {
		return errExit(err)
	}
	// From here on the manifest carries this apply's records; previous ones
	// are kept only as orphans (re-added below).
	newM.Merges = nil
	if len(merges) > 0 {
		recs, err := mount.ApplyMerge(cfg, merges, oldMerges)
		if err != nil {
			// Targets written before the failure get their fresh record;
			// the rest keep the previous one.
			newM.Merges = mergeRecords(cfg, recs, oldMerges)
			if werr := build.WriteManifest(cfg.OutDir(), newM); werr != nil {
				fmt.Println(i18n.T("⚠ failed to record merge targets in the manifest:"), werr)
			}
			fmt.Println("✘", err)
			fmt.Printf(i18n.T("(build finished, %s/ and links are intact; only the merge step is incomplete — fix the issue and rerun agsy apply)\n"), cfg.Build.Out)
			return 1
		}
		newM.Merges = recs
		for _, mp := range merges {
			if mp.State == mount.MergeIdle {
				fmt.Printf(i18n.T("✔ merge skipped: %s (no hook entries to merge, file untouched)\n"), filepath.Join(mp.Dir, mp.Name))
				continue
			}
			fmt.Printf(i18n.T("✔ merge done: %s ← %s\n"), filepath.Join(mp.Dir, mp.Name), filepath.Base(mp.Registry))
		}
	}
	// Orphaned merge targets keep their record (status keeps reporting them,
	// clean strips them) and are listed below with the orphaned links.
	mergeOrphans, err := mount.MergeOrphans(cfg, oldMerges)
	if err != nil {
		return errExit(err)
	}
	for _, o := range mergeOrphans {
		if r := mount.FindRecord(cfg, oldMerges, o); r != nil {
			newM.Merges = append(newM.Merges, *r)
		}
	}

	// Record the links this apply created (orphan detection needs them), then
	// surface links a previous apply created that this config no longer
	// references. Reported, never deleted — same rule as real directories:
	// only clean removes what agsy built, and only after its confirmation.
	created := map[string]bool{}
	for _, l := range links {
		newM.Mounts = append(newM.Mounts, l.LinkPath)
		created[l.LinkPath] = true
	}
	var orphans []string
	seenOrphan := map[string]bool{}
	addOrphan := func(lp string) {
		if created[lp] || seenOrphan[lp] || !mount.IsManagedLink(lp, cfg.OutDir()) {
			return
		}
		seenOrphan[lp] = true
		orphans = append(orphans, lp)
	}
	if mErr == nil && m != nil {
		for _, lp := range m.Mounts {
			addOrphan(lp)
		}
	}
	// The mounts record lives in the AI-writable output; when it is unreadable
	// (or was tampered away) the built-in adapter locations are swept as a
	// fallback so the common orphans keep being reported. Custom mounts have no
	// record to fall back on.
	for _, lp := range adapterLinkCandidates(cfg) {
		addOrphan(lp)
	}
	// Keep orphans on the record so status keeps reporting them until handled.
	newM.Mounts = append(newM.Mounts, orphans...)
	if err := build.WriteManifest(cfg.OutDir(), newM); err != nil {
		fmt.Println(i18n.T("⚠ failed to record mounts in the manifest:"), err)
	}
	if len(orphans) > 0 {
		fmt.Printf(i18n.T("⚠ %d links from a previous apply are no longer referenced by the current mount config:\n"), len(orphans))
		for _, o := range orphans {
			fmt.Println("  -", o)
		}
		fmt.Println(i18n.T("  Tools reading those directories still see old content. Delete them manually, or agsy clean removes them together with everything else agsy built."))
	}
	if len(mergeOrphans) > 0 {
		fmt.Printf(i18n.T("⚠ %d files merged by a previous apply are no longer named by the current mount config but still hold agsy hook entries:\n"), len(mergeOrphans))
		for _, o := range mergeOrphans {
			fmt.Println("  -", o)
		}
		fmt.Println(i18n.T("  The tool keeps running those old hooks. Remove the entries by hand, or agsy clean strips them together with everything else agsy built."))
	}
	return 0
}

// mergeRecords returns newer plus every older record whose path newer does
// not carry.
func mergeRecords(cfg *config.Config, newer, older []build.MergeRecord) []build.MergeRecord {
	out := append([]build.MergeRecord{}, newer...)
	for _, o := range older {
		if mount.FindRecord(cfg, newer, mount.RecordAbs(cfg, o.Path)) == nil {
			out = append(out, o)
		}
	}
	return out
}

// printSourceChanges prints list A: informational, no confirmation needed —
// syncing sources is exactly what apply is for.
func printSourceChanges(rep *state.Report) {
	if len(rep.SourceChanges) == 0 && len(rep.News) == 0 {
		return
	}
	fmt.Println(i18n.T("Source changes this apply will sync to every tool:"))
	for _, sc := range rep.SourceChanges {
		switch sc.Kind {
		case state.SrcDeleted:
			fmt.Printf(i18n.T("  remove  %s/%s   ⚠ source deleted; its outputs (including derived forms) disappear\n"), sc.Item.Category, sc.Item.Name)
		case state.SrcRootMissing:
			// unreachable in apply (missing roots abort earlier); kept for the shared printer
			fmt.Printf(i18n.T("  ⚠ %s/%s: source path missing\n"), sc.Item.Category, sc.Item.Name)
		default:
			note := ""
			if n := len(sc.Files); n > 0 {
				note = fmt.Sprintf(i18n.T(" (%d files)"), n)
			}
			fmt.Printf(i18n.T("  update  %s/%s%s\n"), sc.Item.Category, sc.Item.Name, note)
		}
	}
	for _, n := range rep.News {
		fmt.Printf(i18n.T("  add     %s/%s\n"), n.Category, n.Name)
	}
}

// printArtifactChanges prints list B: the artifact-side changes the rebuild
// will discard, each with guidance on how to keep it. There is no write-back;
// keeping a change always means moving it into a source by hand.
func printArtifactChanges(cfg *config.Config, rep *state.Report, forApply bool) {
	if forApply {
		fmt.Printf(i18n.T("⚠ %d artifact-side changes detected; continuing apply will discard them:\n"), len(rep.Artifacts))
	} else {
		fmt.Printf(i18n.T("\nartifact-side changes: %d items (the next apply will discard them)\n"), len(rep.Artifacts))
	}
	for _, a := range rep.Artifacts {
		switch {
		case a.Untracked:
			fmt.Printf(i18n.T("  added    %s   to keep it: move it into a source directory\n"), a.Path)
		case a.Item != nil && a.Item.Category == "agents-md":
			fmt.Printf(i18n.T("  modified %s   derived from the rules; to keep the change: edit the matching source rule (see the <!-- agsy: --> markers)\n"), a.Path)
		case a.Item != nil && a.Item.Category == "hooks-registry":
			fmt.Printf(i18n.T("  modified %s   derived from the hooks; to keep the change: edit the matching hook.yaml in a source\n"), a.Path)
		case a.Derived:
			fmt.Printf(i18n.T("  modified %s   derived form; to keep the change: edit the source %s\n"), a.Path, fromLabel(cfg, a.Item.From))
		default:
			files := ""
			if n := len(a.Files); n > 0 {
				files = fmt.Sprintf(i18n.T(" (%d files)"), n)
			}
			fmt.Printf(i18n.T("  modified %s%s   to keep it: merge into the source %s\n"), a.Path, files, fromLabel(cfg, a.Item.From))
		}
	}
}

// printMergeChanges lists merge targets whose agsy entries were edited on
// the artifact side; the next apply rebuilds those groups.
func printMergeChanges(rep *state.Report, forApply bool) {
	for _, mp := range rep.Merges {
		if mp.State != mount.MergeModified {
			continue
		}
		if forApply {
			fmt.Printf(i18n.T("  modified %s   agsy entries in the \"hooks\" key were edited; to keep them: move them into your own group or a personal-level settings file\n"), filepath.Join(mp.Dir, mp.Name))
		} else {
			fmt.Printf(i18n.T("  modified %s   agsy entries in the \"hooks\" key were edited (apply rebuilds them); to keep them: move them into your own group or a personal-level settings file\n"), filepath.Join(mp.Dir, mp.Name))
		}
	}
}

// printForeignFrom warns about manifest source paths outside the configured
// sources. The manifest is untrusted, so these are never stat'ed or hashed;
// they also legitimately appear after editing sources — either way the next
// apply rebuilds from the current configuration.
func printForeignFrom(paths []string) {
	if len(paths) == 0 {
		return
	}
	fmt.Printf(i18n.T("\n⚠ the manifest records %d source paths outside the configured sources (sources edited? manifest tampered?). They are not compared; the next apply rebuilds from the current sources:\n"), len(paths))
	for _, f := range paths {
		fmt.Println("  -", f)
	}
}

// fromLabel renders a manifest From path for guidance text, marking paths that
// are not inside the configured sources (the manifest is untrusted).
func fromLabel(cfg *config.Config, from string) string {
	if _, ok := cfg.SourceRootOf(from); !ok {
		return from + i18n.T("   ⚠ this path is not inside the configured sources; verify it before merging")
	}
	return from
}

// adapterLinkCandidates returns the link paths the built-in adapters would
// create in this project (plus the root AGENTS.md), used as a best-effort
// orphan sweep when the manifest's own mounts record is unavailable.
func adapterLinkCandidates(cfg *config.Config) []string {
	adapters, err := loadAdapters()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, a := range adapters {
		if a.NeedsAgentsMD {
			if abs, err := cfg.ExpandPath("."); err == nil {
				add(filepath.Join(abs, config.AgentsMD))
			}
		}
		for _, m := range a.Mounts {
			abs, err := cfg.ExpandPath(m.Dir)
			if err != nil {
				continue
			}
			for name := range m.Links {
				add(filepath.Join(abs, name))
			}
		}
	}
	sort.Strings(out)
	return out
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// sourceRootLabel returns the source root an item belongs to; falls back to
// the parent directory when it cannot be derived.
func sourceRootLabel(cfg *config.Config, from string) string {
	if root, ok := cfg.SourceRootOf(from); ok {
		return root
	}
	return filepath.Dir(from)
}
