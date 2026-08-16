package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/IngSquared99/agent-sync/internal/build"
	"github.com/IngSquared99/agent-sync/i18n"
	"github.com/IngSquared99/agent-sync/internal/mount"
	"github.com/IngSquared99/agent-sync/internal/prompt"
	"github.com/IngSquared99/agent-sync/internal/state"
)

// cmdStatus prints the full report plus an action menu at the bottom (§12-12).
// The body is always read-only; the menu only jumps into the existing
// promote / apply flows.
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

	fmt.Printf(i18n.T("\n═══ sources → %s/ (the direction apply covers) ═══\n"), cfg.Build.Out)
	if len(rep.Lags) == 0 && len(rep.News) == 0 && len(rep.Gone) == 0 {
		fmt.Println(i18n.T("\n(no gaps)"))
	}
	if len(rep.Lags) > 0 {
		fmt.Printf(i18n.T("\nbehind: %d items (source updated, artifact not rebuilt yet)\n"), len(rep.Lags))
		for _, l := range rep.Lags {
			note := i18n.T("content changed")
			switch l.Kind {
			case state.SrcDeleted:
				note = i18n.T("source deleted ⚠ (this item will disappear after apply)")
			case state.SrcRootMissing:
				note = i18n.T("source path missing ⚠ (fix the path first, do not rush to apply)")
			}
			if n := len(l.Files); n > 0 {
				note += fmt.Sprintf(i18n.T(" (%d files)"), n)
			}
			fmt.Printf(i18n.T("  %-32s source: %s    %s\n"), l.Item.Category+"/"+l.Item.Name, l.Item.From, note)
		}
	}
	if len(rep.News) > 0 {
		fmt.Printf(i18n.T("\nnew: %d items (new files in the sources, not yet in the artifacts)\n"), len(rep.News))
		for _, n := range rep.News {
			fmt.Printf(i18n.T("  %-32s source: %s\n"), n.Category+"/"+n.Name, n.From)
		}
	}
	if len(rep.Gone) > 0 {
		fmt.Printf(i18n.T("\nmissing artifacts: %d items (artifact copy deleted ⚠, unreadable from the mount side; apply can rebuild)\n"), len(rep.Gone))
		for _, g := range rep.Gone {
			fmt.Printf(i18n.T("  %-32s missing: %s\n"), g.Item.Category+"/"+g.Item.Name, strings.Join(g.OutPaths, ", "))
		}
	}

	fmt.Printf(i18n.T("\n═══ %s/ → sources (the direction promote covers) ═══\n"), cfg.Build.Out)
	if len(rep.Locals) == 0 {
		fmt.Println(i18n.T("\n(no local changes)"))
	} else {
		fmt.Printf(i18n.T("\nlocal changes: %d items (artifact differs from when it was built, can be written back)\n"), len(rep.Locals))
		for _, lc := range rep.Locals {
			extra := ""
			if lc.SrcAlso {
				extra = i18n.T("   ⚠ the source changed as well, writing back needs manual comparison")
			}
			if lc.Multiple {
				extra += i18n.T("   ⚠ copies in multiple buckets were all modified")
			}
			files := ""
			if n := len(lc.Files); n > 0 {
				files = fmt.Sprintf(i18n.T("   %d files changed"), n)
			}
			fmt.Printf(i18n.T("  %-32s original source: %s%s%s\n"), lc.Item.Category+"/"+lc.Item.Name, lc.Item.From, files, extra)
		}
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
			fmt.Printf(i18n.T("%s/   %s → ✘ occupied by a real directory (not created by this tool, handle it manually)\n"), l.Dir, l.Name)
			dirOK = false
		}
	}
	flush()

	fmt.Println(i18n.T("\n═══ summary ═══"))
	fmt.Printf(i18n.T("behind %d │ new %d │ local changes %d │ missing artifacts %d │ mount anomalies %d\n"),
		len(rep.Lags), len(rep.News), len(rep.Locals), len(rep.Gone), rep.LinkBad)
	if len(rep.Locals) > 0 {
		fmt.Println(i18n.T("suggestion: run agsy promote first to keep the changes, then agsy apply to rebuild"))
	} else if len(rep.Lags) > 0 || len(rep.News) > 0 || len(rep.Gone) > 0 || rep.LinkBad > 0 {
		fmt.Println(i18n.T("suggestion: run agsy apply to rebuild"))
	}

	code := 0
	if rep.HasGap {
		code = 1
	}

	// Action menu (only on a TTY, and only when allowed)
	if withMenu && prompt.IsTTY() && rep.HasGap {
		opts := []string{}
		if len(rep.Locals) > 0 {
			opts = append(opts, fmt.Sprintf(i18n.T("run promote (write back %d local changes)"), len(rep.Locals)))
		}
		opts = append(opts, i18n.T("run apply (local changes are confirmed first)"), i18n.T("quit"))
		i := prompt.Select(i18n.T("\nWhat next?"), opts, len(opts)-1)
		switch {
		case opts[i] == i18n.T("quit"):
			return code
		case opts[i] == i18n.T("run apply (local changes are confirmed first)"):
			return cmdApply()
		default:
			return cmdPromote(nil)
		}
	}
	return code
}
