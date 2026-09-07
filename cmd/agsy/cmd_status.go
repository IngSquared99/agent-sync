package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/IngSquared99/agent-sync/i18n"
	"github.com/IngSquared99/agent-sync/internal/build"
	"github.com/IngSquared99/agent-sync/internal/mount"
	"github.com/IngSquared99/agent-sync/internal/prompt"
	"github.com/IngSquared99/agent-sync/internal/state"
)

// cmdStatus prints the full report plus an action menu at the bottom.
// The body is always read-only; the menu only jumps into the apply flow.
// Non-TTY (CI / git hook): print the report only, exit 0=in sync 1=gaps found.
func cmdStatus(withMenu bool) int {
	cfg, err := loadConfig()
	if err != nil {
		return errExit(err)
	}
	out := cfg.OutDir()
	m, err := build.LoadManifest(out)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf(i18n.T("not built yet (manifest not found). Run agsy apply for the first build.\n"))
			return 1
		}
		return errExit(err)
	}
	rep, err := state.Collect(cfg, m)
	if err != nil {
		return errExit(err)
	}
	fmt.Printf(i18n.T("read manifest (built at %s, %d sources) ✔\n"), m.BuiltAt, len(m.Sources))
	if len(rep.MissingSources) > 0 {
		fmt.Println(i18n.T("\n⚠ the following source paths do not exist (shared repo not cloned yet? external drive not mounted? typo in the path?):"))
		for _, s := range rep.MissingSources {
			fmt.Println("  -", s)
		}
		fmt.Println(i18n.T("  Items from these sources are marked \"source path missing\" below — the path is gone, not the files deleted."))
		fmt.Println(i18n.T("  Fix the paths and look again; apply also blocks rebuilding from an incomplete source list."))
	}
	printForeignFrom(rep.ForeignFrom)
	if rep.ScanErr != nil {
		fmt.Printf(i18n.T("\n⚠ source scan failed (%v); new-item detection did not run this time\n"), rep.ScanErr)
	}
	if len(rep.RouteErrors) > 0 {
		fmt.Printf(i18n.T("\n⚠ %d workflow target or hook declaration problems (apply will refuse to build; agsy plan lists them):\n"), len(rep.RouteErrors))
		for _, e := range rep.RouteErrors {
			fmt.Println("  -", e)
		}
	}

	fmt.Printf(i18n.T("\n═══ list A: sources → %s/ (apply will sync these) ═══\n"), cfg.Build.Out)
	if len(rep.SourceChanges) == 0 && len(rep.News) == 0 && len(rep.Gone) == 0 {
		fmt.Println(i18n.T("\n(no gaps)"))
	}
	if len(rep.SourceChanges) > 0 || len(rep.News) > 0 {
		fmt.Println()
		for _, sc := range rep.SourceChanges {
			note := i18n.T("content changed")
			switch sc.Kind {
			case state.SrcDeleted:
				note = i18n.T("source deleted ⚠ (this item and its derived forms disappear after apply)")
			case state.SrcRootMissing:
				note = i18n.T("source path missing ⚠ (fix the path first, do not rush to apply)")
			}
			if n := len(sc.Files); n > 0 {
				note += fmt.Sprintf(i18n.T(" (%d files)"), n)
			}
			fmt.Printf(i18n.T("  %-32s source: %s    %s\n"), sc.Item.Category+"/"+sc.Item.Name, sc.Item.From, note)
		}
		for _, n := range rep.News {
			fmt.Printf(i18n.T("  %-32s source: %s    new\n"), n.Category+"/"+n.Name, n.From)
		}
	}
	if len(rep.Gone) > 0 {
		fmt.Printf(i18n.T("\nmissing outputs: %d items (output copy deleted ⚠, unreadable from the mount side; apply rebuilds)\n"), len(rep.Gone))
		for _, g := range rep.Gone {
			fmt.Printf(i18n.T("  %-32s missing: %s\n"), g.Item.Category+"/"+g.Item.Name, strings.Join(g.OutPaths, ", "))
		}
	}

	fmt.Printf(i18n.T("\n═══ list B: changes on the artifact side (apply will discard these) ═══\n"))
	modifiedMerges := 0
	for _, mp := range rep.Merges {
		if mp.State == mount.MergeModified {
			modifiedMerges++
		}
	}
	if len(rep.Artifacts) == 0 && modifiedMerges == 0 {
		fmt.Println(i18n.T("\n(no artifact-side changes)"))
	} else {
		if len(rep.Artifacts) > 0 {
			printArtifactChanges(cfg, rep, false)
		}
		if modifiedMerges > 0 {
			if len(rep.Artifacts) == 0 {
				fmt.Println()
			}
			printMergeChanges(rep, false)
		}
		fmt.Println(i18n.T("  There is no write-back: to keep a change, move or merge it into a source, then agsy apply."))
	}

	fmt.Print(i18n.T("\n═══ mounts ═══\n\n"))
	curDir := ""
	dirOK := true
	flush := func() {
		if curDir != "" && dirOK {
			fmt.Printf(i18n.T("%s/   all links correct ✔\n"), curDir)
		}
	}
	for _, l := range rep.Links {
		if l.Dir != curDir {
			flush()
			curDir, dirOK = l.Dir, true
		}
		switch l.State {
		case mount.Missing:
			fmt.Printf(i18n.T("%s/   %s → ✘ link missing (possibly deleted by hand; rerun agsy apply to repair)\n"), l.Dir, l.Name)
			dirOK = false
		case mount.IsStale:
			fmt.Printf(i18n.T("%s/   %s → ✘ %s (rerun agsy apply to repair)\n"), l.Dir, l.Name, l.Note)
			dirOK = false
		case mount.IsReal:
			fmt.Printf(i18n.T("%s/   %s → ✘ occupied by a real directory or file (not created by this tool, handle it manually)\n"), l.Dir, l.Name)
			dirOK = false
		}
	}
	flush()
	for _, o := range rep.Orphans {
		fmt.Printf(i18n.T("%s → ⚠ created by an earlier apply but no longer referenced by the mount config; delete it manually or run agsy clean\n"), o)
	}
	for _, mp := range rep.Merges {
		target := mp.Dir + "/   " + mp.Name
		switch mp.State {
		case mount.MergeClean:
			fmt.Printf(i18n.T("%s ⇐ %s   merged, %d agsy entries in sync ✔\n"), target, filepath.Base(mp.Registry), mp.Owned)
		case mount.MergeIdle:
			fmt.Printf(i18n.T("%s ⇐ %s   no hook entries to merge, file untouched ✔\n"), target, filepath.Base(mp.Registry))
		case mount.MergeStale:
			fmt.Printf(i18n.T("%s ⇐ %s   ✘ agsy entries point at a previous output path (project moved or build.out renamed; apply updates them)\n"), target, filepath.Base(mp.Registry))
		case mount.MergeModified:
			fmt.Printf(i18n.T("%s ⇐ %s   ⚠ agsy entries in the \"hooks\" key were modified (listed above; apply rebuilds them)\n"), target, filepath.Base(mp.Registry))
		case mount.MergeMissing, mount.MergeAbsent:
			fmt.Printf(i18n.T("%s ⇐ %s   ✘ agsy entries in the \"hooks\" key are missing (apply restores them)\n"), target, filepath.Base(mp.Registry))
		case mount.MergeInvalid:
			fmt.Printf(i18n.T("%s ⇐ %s   ✘ %s; apply will refuse until it is fixed\n"), target, filepath.Base(mp.Registry), mp.Note)
		}
	}

	for _, o := range rep.MergeOrphans {
		fmt.Printf(i18n.T("%s → ⚠ merged by an earlier apply but no longer named by the mount config; it still holds agsy hook entries — remove them by hand or run agsy clean\n"), o)
	}
	printForeignMerges(rep.ForeignMerges)

	fmt.Println(i18n.T("\n═══ summary ═══"))
	fmt.Printf(i18n.T("source changes %d │ artifact-side changes %d │ missing outputs %d │ mount anomalies %d\n"),
		len(rep.SourceChanges)+len(rep.News), len(rep.Artifacts)+modifiedMerges, len(rep.Gone), rep.LinkBad+rep.MergeBad)
	if len(rep.Artifacts) > 0 || modifiedMerges > 0 {
		fmt.Println(i18n.T("suggestion: move anything worth keeping into a source first, then run agsy apply"))
	} else if len(rep.SourceChanges) > 0 || len(rep.News) > 0 || len(rep.Gone) > 0 || rep.LinkBad > len(rep.Orphans) || rep.MergeBad > len(rep.MergeOrphans)+len(rep.ForeignMerges) {
		fmt.Println(i18n.T("suggestion: run agsy apply to rebuild"))
	}
	if len(rep.Orphans) > 0 {
		// apply never removes links, so "rebuild" is not the remedy here.
		fmt.Println(i18n.T("suggestion: orphaned links are not removed by apply — delete them manually or run agsy clean"))
	}
	if len(rep.MergeOrphans) > 0 {
		fmt.Println(i18n.T("suggestion: orphaned hook entries are not removed by apply — remove them by hand or run agsy clean"))
	}

	code := 0
	if rep.HasGap {
		code = 1
	}

	// Action menu (only on a TTY, and only when allowed)
	if withMenu && prompt.IsTTY() && rep.HasGap {
		opts := []string{i18n.T("run apply (artifact-side changes are confirmed first)"), i18n.T("quit")}
		i := prompt.Select(i18n.T("\nWhat next?"), opts, len(opts)-1)
		if i == 0 {
			return cmdApply()
		}
	}
	return code
}
