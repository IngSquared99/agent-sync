package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/IngSquared99/agent-sync/i18n"
	"github.com/IngSquared99/agent-sync/internal/build"
	"github.com/IngSquared99/agent-sync/internal/config"
	"github.com/IngSquared99/agent-sync/internal/mount"
	"github.com/IngSquared99/agent-sync/internal/prompt"
	"github.com/IngSquared99/agent-sync/internal/state"
)

// cmdClean uninstalls: removes links and build outputs, leaving only agsy.yaml.
// Only deletes what the tool created: links and outputs are removable; real paths are skipped and reported.
func cmdClean() int {
	cfg, err := loadConfig()
	if err != nil {
		return errExit(err)
	}
	release, err := acquireLock(cfg)
	if err != nil {
		return errExit(err)
	}
	defer release()
	if !prompt.Confirm(fmt.Sprintf(i18n.T("Will remove mount links, agsy's hook entries from merged files, and %s/ (agsy.yaml untouched). Continue?"), cfg.Build.Out)) {
		fmt.Println(i18n.T("Cancelled."))
		return 1
	}
	// Load the manifest before the output is removed: its mount record is the
	// only trace of links whose mount entry was later edited away (orphans).
	var recorded []string
	var mergeRecs []build.MergeRecord
	if m, err := build.LoadManifest(cfg.OutDir()); err == nil {
		recorded = m.Mounts
		mergeRecs = m.Merges
	}
	// Merge targets first: they are the user's files, so only agsy's own
	// entries are stripped; a file agsy created and left empty is deleted.
	cleaned, deletedFiles, skippedMerge, err := mount.RemoveMerge(cfg, mergeRecs)
	if err != nil {
		return errExit(err)
	}
	for _, c := range cleaned {
		fmt.Println(i18n.T("✔ Removed agsy hooks from"), c)
	}
	for _, d := range deletedFiles {
		fmt.Println(i18n.T("✔ Removed (created by agsy)"), d)
	}
	for _, sk := range skippedMerge {
		fmt.Println(i18n.T("⚠ Skipped (symbolic link or not a JSON object)"), sk)
	}
	removed, skipped, err := mount.RemoveLinks(cfg)
	if err != nil {
		return errExit(err)
	}
	for _, lp := range recorded {
		// Untrusted list: only paths that verifiably are agsy links (a link
		// resolving into the output) are removed; anything else is left alone.
		if mount.IsManagedLink(lp, cfg.OutDir()) {
			if e := os.Remove(lp); e == nil {
				removed = append(removed, lp)
				if entries, e := os.ReadDir(filepath.Dir(lp)); e == nil && len(entries) == 0 {
					_ = os.Remove(filepath.Dir(lp))
				}
			}
		}
	}
	// Best-effort sweep of the built-in adapter locations: when the manifest
	// (and its mounts record) is unreadable, orphaned links would otherwise
	// survive clean. Only verified agsy links are touched.
	for _, lp := range adapterLinkCandidates(cfg) {
		if mount.IsManagedLink(lp, cfg.OutDir()) {
			if e := os.Remove(lp); e == nil {
				removed = append(removed, lp)
				if entries, e := os.ReadDir(filepath.Dir(lp)); e == nil && len(entries) == 0 {
					_ = os.Remove(filepath.Dir(lp))
				}
			}
		}
	}
	for _, r := range removed {
		fmt.Println(i18n.T("✔ Removed link"), r)
	}
	for _, s := range skipped {
		fmt.Println(i18n.T("⚠ Skipped (real path, not created by this tool)"), s)
	}
	if err := build.RemoveOut(cfg); err != nil {
		return errExit(err)
	}
	fmt.Printf(i18n.T("✔ Removed %s/\n"), cfg.Build.Out)
	return 0
}

// cmdDoctor runs a read-only environment health check; it performs no actions.
// ✔ / ⚠ / ✘ are strictly distinguished: a source lacking a category subdirectory is
// normal (⚠); only a source missing entirely is an error (✘).
func cmdDoctor() int {
	wd, _ := os.Getwd()
	path, ok := config.FindUp(wd) // same lookup rules as loadConfig, so running from a subdirectory also finds it
	if !ok {
		fmt.Printf(i18n.T("Checking %s ............ ✘ not found (searched up to the root; agsy init can create it)\n"), config.FileName)
		return 1
	}
	cfg, err := config.Load(path)
	if err != nil {
		fmt.Printf(i18n.T("Checking %s ............ ✘ %v\n"), config.FileName, err)
		return 1
	}
	fmt.Printf(i18n.T("Checking %s ............ ✔ format OK\n"), config.FileName)

	errs, warns := 0, 0
	hooksSeen := false
	sources, _ := build.ExpandSources(cfg)
	fmt.Println(i18n.T("Checking source paths"))
	for _, s := range sources {
		if s.Exists {
			fmt.Printf(i18n.T("  %-28s ✔ exists\n"), s.Raw)
		} else {
			fmt.Printf(i18n.T("  %-28s ✘ missing (typo? not created yet?) → %s\n"), s.Raw, s.Abs)
			errs++
		}
	}
	for _, s := range sources {
		if !s.Exists {
			continue
		}
		// Use the full source path in the heading: it's the only way to tell
		// two sources apart when their last path segments are identical
		fmt.Printf(i18n.T("Checking source subdirectories (%s)\n"), s.Abs)
		for _, cat := range config.CategoryOrder {
			cc := cfg.Build.Categories[cat]
			dir := filepath.Join(s.Abs, cc.From)
			entries, err := os.ReadDir(dir)
			if err != nil {
				fmt.Printf(i18n.T("  %-28s ⚠ directory missing (this source has no %s; not an error)\n"), cc.From+"/", cat)
				warns++
				continue
			}
			// The count must follow the same inclusion rules as build so the
			// reported numbers match what a build would collect
			n := 0
			var ignored []string
			for _, e := range entries {
				ok, reason := build.Accepts(cat, filepath.Join(dir, e.Name()), e.IsDir())
				if ok {
					n++
					continue
				}
				if !strings.HasPrefix(e.Name(), ".") {
					ignored = append(ignored, fmt.Sprintf(i18n.T("%s (%s)"), e.Name(), reason))
				}
			}
			unit := i18n.T("files")
			if cat == "skills" || cat == "hooks" {
				unit = i18n.T("directories")
			}
			fmt.Printf("  %-28s ✔ %d %s\n", cc.From+"/", n, unit)
			for _, ig := range ignored {
				fmt.Printf(i18n.T("  %-28s ⚠ skipped %s\n"), "", ig)
				warns++
			}
			if cat == "hooks" {
				hooksSeen = true
				for _, e := range entries {
					if !e.IsDir() {
						continue
					}
					for _, rel := range build.HookScriptPaths(filepath.Join(dir, e.Name())) {
						if runtime.GOOS != "windows" {
							if fi, err := os.Stat(filepath.Join(dir, e.Name(), rel)); err == nil && fi.Mode().Perm()&0o111 == 0 {
								fmt.Printf(i18n.T("  %-28s ⚠ %s/%s is not executable (chmod +x)\n"), "", e.Name(), rel)
								warns++
							}
						}
					}
				}
			}
		}
	}
	fmt.Println(i18n.T("Checking mount targets"))
	links, err := mount.Inspect(cfg)
	if err != nil {
		return errExit(err)
	}
	for _, l := range links {
		p := filepath.Join(l.Dir, l.Name)
		switch l.State {
		case mount.Missing:
			fmt.Printf(i18n.T("  %-28s ✔ absent, can be created directly\n"), p)
		case mount.IsLink:
			fmt.Printf(i18n.T("  %-28s ✔ already a link (apply deletes and recreates it)\n"), p)
		case mount.IsStale:
			fmt.Printf(i18n.T("  %-28s ⚠ %s (apply will rebuild and fix it)\n"), p, l.Note)
			warns++
		case mount.IsReal:
			fmt.Printf(i18n.T("  %-28s ⚠ a real directory or file exists; apply will fail, handle it manually\n"), p)
			warns++
		}
	}
	merges, err := mount.InspectMerge(cfg, nil)
	if err != nil {
		return errExit(err)
	}
	for _, mp := range merges {
		p := filepath.Join(mp.Dir, mp.Name)
		switch mp.State {
		case mount.MergeMissing:
			fmt.Printf(i18n.T("  %-28s ✔ absent, apply creates it with the hook entries\n"), p)
		case mount.MergeAbsent, mount.MergeClean, mount.MergeModified, mount.MergeStale:
			fmt.Printf(i18n.T("  %-28s ✔ JSON object, hook entries are merged into its \"hooks\" key\n"), p)
		case mount.MergeIdle:
			fmt.Printf(i18n.T("  %-28s ✔ no hook entries to merge; apply leaves it untouched\n"), p)
		case mount.MergeInvalid:
			fmt.Printf(i18n.T("  %-28s ⚠ %s; apply will fail, handle it manually\n"), p, mp.Note)
			warns++
		}
	}
	if hooksSeen {
		for _, tool := range cfg.Build.Tools {
			if build.HasHookDialect(tool) && !cfg.HookRegistryMounted(tool) {
				fmt.Printf(i18n.T("  %-28s ⚠ build.tools lists %q, but nothing mounts %s; hooks will not reach that tool\n"), "", tool, config.HookRegistryFiles[tool])
				warns++
			}
		}
	}
	// Link capability is verified by creating and removing a real link
	if err := mount.Probe(cfg.BaseDir); err != nil {
		fmt.Printf(i18n.T("Checking link capability ................ ✘ cannot create directory link: %v\n"), err)
		errs++
	} else if runtime.GOOS == "windows" {
		fmt.Println(i18n.T("Checking link capability ................ ✔ can create (junction, no privileges needed)"))
	} else {
		fmt.Println(i18n.T("Checking link capability ................ ✔ can create (relative symlink)"))
	}

	fmt.Printf(i18n.T("\n%d errors, %d warnings.\n"), errs, warns)
	if errs > 0 {
		return 1
	}
	return 0
}

// cmdMenu is the interactive menu behind the dual entry point.
// Guides the user into init when no config file is found.
func cmdMenu() int {
	v, _, _ := resolveVersion()
	fmt.Printf("agsy %s\n\n", v)
	wd, _ := os.Getwd()
	if _, ok := config.FindUp(wd); !ok {
		fmt.Printf(i18n.T("⚠ %s not found; looks like the first use in this project\n"), config.FileName)
		i := prompt.Select(i18n.T("Create the configuration now?"), []string{i18n.T("Yes, start setup (init)"), i18n.T("No, exit")}, 0)
		if i == 0 {
			return cmdInit(nil)
		}
		return 0
	}

	// Top status summary (runs the status detection once here, read-only)
	summary := ""
	localN := 0
	if cfg, err := loadConfig(); err == nil {
		if m, err := build.LoadManifest(cfg.OutDir()); err == nil {
			if rep, err := state.Collect(cfg, m); err == nil {
				// Same arithmetic as status: edited merge entries count as
				// artifact-side changes, merge gaps as mount issues.
				modifiedMerges := 0
				for _, mp := range rep.Merges {
					if mp.State == mount.MergeModified {
						modifiedMerges++
					}
				}
				summary = fmt.Sprintf(i18n.T("  Status: source changes %d │ artifact-side changes %d │ missing outputs %d │ mount issues %d\n"),
					len(rep.SourceChanges)+len(rep.News), len(rep.Artifacts)+modifiedMerges, len(rep.Gone), rep.LinkBad+rep.MergeBad)
				localN = len(rep.Artifacts) + modifiedMerges
			}
		} else if os.IsNotExist(err) {
			summary = i18n.T("  Status: not built yet (run plan to preview, then apply)\n")
		} else {
			// A manifest that exists but cannot be read is not "not built":
			// apply will ask before wiping; say so instead of implying a
			// clean slate.
			summary = fmt.Sprintf(i18n.T("  Status: %s/ exists but its manifest cannot be read; apply will ask before wiping and rebuilding\n"), cfg.Build.Out)
		}
	}
	fmt.Print(summary, "\n")

	applyNote := ""
	if localN > 0 {
		applyNote = fmt.Sprintf(i18n.T("      ⚠ %d artifact-side changes, confirmation required first"), localN)
	}
	opts := []string{
		i18n.T("apply    rebuild outputs and mount") + applyNote,
		i18n.T("plan     preview changes without writing"),
		i18n.T("status   view detailed status"),
		i18n.T("doctor   environment health check"),
		i18n.T("init     configure (enters edit mode if config exists)"),
		i18n.T("clean    remove outputs and mounts"),
		i18n.T("Exit"),
	}
	i := prompt.Select(i18n.T("What would you like to do?"), opts, 2)
	switch i {
	case 0:
		return cmdApply()
	case 1:
		return cmdPlan()
	case 2:
		return cmdStatus(true)
	case 3:
		return cmdDoctor()
	case 4:
		return cmdInit(nil)
	case 5:
		return cmdClean()
	}
	return 0
}
