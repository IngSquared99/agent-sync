package main

import (
	"fmt"
	"os"

	"github.com/IngSquared99/agent-sync/i18n"
	"github.com/IngSquared99/agent-sync/internal/build"
	"github.com/IngSquared99/agent-sync/internal/mount"
	"github.com/IngSquared99/agent-sync/internal/prompt"
	"github.com/IngSquared99/agent-sync/internal/state"
)

// cmdApply is the commit flow (§12):
// pre-checks → confirm local changes (status detection logic) → clear artifacts (shares deletion logic with clean) → build → mount
// The order must not be reversed: clearing before checking would turn the check into decoration.
func cmdApply() int {
	cfg, err := loadConfig()
	if err != nil {
		return errExit(err)
	}

	// Pre-check: all sources must exist (plan may run incomplete, apply must not, §12-9)
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

	// Pre-check: mount targets occupied by real files (§12-10).
	// This error is detectable before build — reporting it only at the mount
	// stage, after the wipe and rebuild, means the user has already agreed to
	// discard local changes and waited for the build, only to find the run was
	// doomed from the start.
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
		return 1
	}

	// Compute runs before the confirmation (read-only): problems that doom
	// the run — routing errors, name conflicts — must surface before the
	// user agrees to discard local changes.
	p, err := build.Compute(cfg, sources)
	if err != nil {
		return errExit(err)
	}
	if len(p.RouteErrors) > 0 {
		fmt.Println(i18n.T("✘ workflow routing problems (front-matter target); fix these files first:"))
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
		fmt.Println(i18n.T("✘ the final artifact names of the following items collide; proceeding would overwrite one of them:"))
		for _, c := range p.Collisions {
			fmt.Printf("  %s/%s\n", c.Category, c.OutName)
			for _, f := range c.Froms {
				fmt.Printf("    - %s\n", f)
			}
		}
		fmt.Println(i18n.T("  Rename one of them, or switch to on_conflict: error and resolve it manually."))
		return 1
	}

	// Confirm local changes (forced on every run, §12-3).
	// Three cases must be kept apart:
	//   manifest readable → compare item by item, ask only if something changed
	//   manifest unreadable but the artifact dir still exists → whether corrupted or deleted,
	//   there is no way to know what's inside, so always ask
	//   artifact dir does not exist → first build, nothing can be overwritten
	out := cfg.OutDir()
	m, mErr := build.LoadManifest(out)
	switch {
	case mErr == nil:
		rep, err := state.Collect(cfg, m)
		if err != nil {
			return errExit(err)
		}
		if len(rep.Locals) > 0 || len(rep.Untracked) > 0 {
			if len(rep.Locals) > 0 {
				fmt.Printf(i18n.T("⚠ detected %d local changes not yet promoted; continuing apply will lose them:\n"), len(rep.Locals))
				for _, lc := range rep.Locals {
					fmt.Printf("  - %s/%s\n", lc.Item.Category, lc.Item.Name)
				}
				fmt.Println(i18n.T("  (run agsy promote first to keep the changes)"))
			}
			if len(rep.Untracked) > 0 {
				fmt.Printf(i18n.T("⚠ %d untracked files were added on the mount side; rebuilding deletes them:\n"), len(rep.Untracked))
				for _, u := range rep.Untracked {
					fmt.Println("  -", u)
				}
				fmt.Println(i18n.T("  (to keep one, move it into a source directory first; promote has no origin to write it back to)"))
			}
			if !prompt.Confirm(i18n.T("Discard these changes and rebuild?")) {
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

	for _, w := range p.NoBucket {
		fmt.Printf(i18n.T("⚠ workflow %s has no target marker and default is empty; it goes into no bucket\n"), w)
	}
	if len(p.Ignored) > 0 {
		fmt.Printf(i18n.T("⚠ %d files do not match the inclusion rules and were left out (agsy plan shows the list)\n"), len(p.Ignored))
	}
	newM, err := build.Execute(cfg, p)
	if err != nil {
		return errExit(err)
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
	if mErr == nil && m != nil {
		for _, lp := range m.Mounts {
			if !created[lp] && mount.IsManagedLink(lp, cfg.OutDir()) {
				orphans = append(orphans, lp)
			}
		}
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
	return 0
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
