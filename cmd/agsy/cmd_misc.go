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

// cmdClean uninstalls (§12-11): removes links and build outputs, leaving only agsy.yaml.
// Only deletes what the tool created: links and outputs are removable; real paths are skipped and reported.
func cmdClean() int {
	cfg, err := loadConfig()
	if err != nil {
		return errExit(err)
	}
	if !prompt.Confirm(fmt.Sprintf(i18n.T("Will remove mount links and %s/ (agsy.yaml untouched). Continue?"), cfg.Build.Out)) {
		fmt.Println(i18n.T("Cancelled."))
		return 1
	}
	removed, skipped, err := mount.RemoveLinks(cfg)
	if err != nil {
		return errExit(err)
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

// cmdDoctor runs a read-only environment health check; it performs no actions (§12-8).
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
			// The count must follow the same inclusion rules as build; otherwise
			// doctor says 3 while only 1 actually gets picked up, and the user
			// can't tell where the other two went (§12-8)
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
			if cat == "skills" {
				unit = i18n.T("directories")
			}
			fmt.Printf("  %-28s ✔ %d %s\n", cc.From+"/", n, unit)
			for _, ig := range ignored {
				fmt.Printf(i18n.T("  %-28s ⚠ skipped %s\n"), "", ig)
				warns++
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
	// Link capability: actually create one and delete it, not just print a claim
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

// cmdMenu is the interactive menu behind the dual entry point (§7).
// Guides the user into init when no config file is found (§12-18).
func cmdMenu() int {
	fmt.Printf("agsy %s\n\n", version)
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
				summary = fmt.Sprintf(i18n.T("  Status: behind %d │ new %d │ local changes %d │ untracked %d │ missing outputs %d │ mount issues %d\n"),
					len(rep.Lags), len(rep.News), len(rep.Locals), len(rep.Untracked), len(rep.Gone), rep.LinkBad)
				localN = len(rep.Locals)
			}
		} else {
			summary = i18n.T("  Status: not built yet (run plan to preview, then apply)\n")
		}
	}
	fmt.Print(summary, "\n")

	applyNote := ""
	if localN > 0 {
		applyNote = fmt.Sprintf(i18n.T("      ⚠ %d local changes, confirmation required first"), localN)
	}
	opts := []string{
		i18n.T("apply    rebuild outputs and mount") + applyNote,
		i18n.T("plan     preview changes without writing"),
		fmt.Sprintf(i18n.T("promote  write back local changes (%d items)"), localN),
		i18n.T("status   view detailed status"),
		i18n.T("doctor   environment health check"),
		i18n.T("init     configure (enters edit mode if config exists)"),
		i18n.T("clean    remove outputs and mounts"),
		i18n.T("Exit"),
	}
	i := prompt.Select(i18n.T("What would you like to do?"), opts, 3)
	switch i {
	case 0:
		return cmdApply()
	case 1:
		return cmdPlan()
	case 2:
		return cmdPromote(nil)
	case 3:
		return cmdStatus(true)
	case 4:
		return cmdDoctor()
	case 5:
		return cmdInit(nil)
	case 6:
		return cmdClean()
	}
	return 0
}
