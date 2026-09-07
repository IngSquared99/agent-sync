package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/IngSquared99/agent-sync/i18n"
	"github.com/IngSquared99/agent-sync/internal/build"
	"github.com/IngSquared99/agent-sync/internal/config"
	"github.com/IngSquared99/agent-sync/internal/mount"
)

// cmdPlan rehearses everything apply would do, without writing anything.
// Missing sources: skipping is fine, staying silent is not. Same for
// files that cannot be included.
func cmdPlan() int {
	cfg, err := loadConfig()
	if err != nil {
		return errExit(err)
	}
	fmt.Printf(i18n.T("read %s ✔\n"), config.FileName)

	sources, err := build.ExpandSources(cfg)
	if err != nil {
		return errExit(err)
	}
	fmt.Println(i18n.T("sources (by priority):"))
	for i, s := range sources {
		mark := "✔"
		note := ""
		if !s.Exists {
			mark = "✘"
			note = i18n.T(" path does not exist; the preview below EXCLUDES this source")
		}
		fmt.Printf(i18n.T("  [%d] %s   %s   tag: @%s%s\n"), i+1, s.Raw, mark, s.Tag, note)
	}
	p, err := build.Compute(cfg, sources)
	if err != nil {
		return errExit(err)
	}
	if p.Incomplete {
		fmt.Println(i18n.T("\n⚠ this preview is incomplete. Results will differ once the sources are fixed."))
	}

	fmt.Println(i18n.T("\n═══ build preview ═══"))
	for _, cat := range config.CategoryOrder {
		cc := cfg.Build.Categories[cat]
		fmt.Printf(i18n.T("\n%s → %s/%s/ (strategy: %s)\n"), cat, cfg.Build.Out, cc.To, cfg.Build.OnConflict[cat])
		n := 0
		for _, it := range p.Items {
			if it.Category != cat {
				continue
			}
			n++
			line := fmt.Sprintf("  %-28s ← [%d] %s", it.OutName, it.SourceIdx+1, it.SourceTag)
			if it.Renamed {
				line += i18n.T("   ⚠ duplicate name, source tag appended")
			}
			if cat == "workflows" {
				var forms []string
				if it.SkillName != "" {
					forms = append(forms, fmt.Sprintf(i18n.T("skill %s/%s"), cfg.Build.Categories["skills"].To, it.SkillName))
				}
				if len(it.Tools) > 0 && slices.Contains(it.Tools, build.StubTool) {
					forms = append(forms, fmt.Sprintf(i18n.T("stub %s/%s"), cc.To, it.OutName))
				}
				line += "   → " + strings.Join(forms, ", ")
				if it.RouteNote != "" {
					line += " (" + it.RouteNote + ")"
				}
			}
			fmt.Println(line)
			if cat == "hooks" && it.Hook != nil {
				if d := strings.TrimSpace(it.Hook.Description); d != "" {
					fmt.Println("      " + d)
				}
				var reach []string
				for _, tool := range cfg.Build.Tools {
					if !build.HasHookDialect(tool) {
						continue
					}
					mark := "—"
					if it.HookOut[tool] {
						mark = "✓"
					}
					reach = append(reach, tool+" "+mark)
				}
				fmt.Println("      " + strings.Join(reach, "  "))
				for _, n := range strings.Split(it.RouteNote, "; ") {
					if n != "" {
						fmt.Println("      " + n)
					}
				}
			}
		}
		for _, it := range p.Skipped {
			if it.Category == cat {
				fmt.Printf(i18n.T("  %-28s ← [%d] %s   (first strategy, will be dropped)\n"), it.Name, it.SourceIdx+1, it.SourceTag)
			}
		}
		if n == 0 {
			fmt.Println(i18n.T("  (no items)"))
		}
	}
	rulesN := 0
	for _, it := range p.Items {
		if it.Category == "rules" {
			rulesN++
		}
	}
	fmt.Printf(i18n.T("\nderived: %s (all %d rules concatenated, read by tools without a rules directory)\n"), config.AgentsMD, rulesN)
	var regs []string
	hooksN := 0
	for _, it := range p.Items {
		if it.Category == "hooks" {
			hooksN++
		}
	}
	for _, tool := range cfg.Build.Tools {
		if build.HasHookDialect(tool) {
			regs = append(regs, config.HookRegistryFiles[tool])
		}
	}
	if len(regs) > 0 {
		fmt.Printf(i18n.T("derived: %s (hook registries, one per tool, %d hooks translated)\n"), strings.Join(regs, ", "), hooksN)
	}
	if hooksN > 0 {
		for _, tool := range cfg.Build.Tools {
			if build.HasHookDialect(tool) && !cfg.HookRegistryMounted(tool) {
				fmt.Printf(i18n.T("⚠ build.tools lists %q, but nothing mounts %s; hooks will not reach that tool\n"), tool, config.HookRegistryFiles[tool])
			}
		}
	}
	if len(p.Ignored) > 0 {
		fmt.Printf(i18n.T("\n⚠ the following %d files do not match the inclusion rules and will not enter the artifacts:\n"), len(p.Ignored))
		for _, ig := range p.Ignored {
			fmt.Printf("  %-28s %s (%s)\n", ig.Category+"/"+ig.Name, ig.Reason, ig.From)
		}
	}
	if len(p.Conflicts) > 0 {
		fmt.Println(i18n.T("\n✘ name conflicts (strategy error), apply will stop:"))
		for _, c := range p.Conflicts {
			fmt.Printf("  %s/%s\n", c.Category, c.Name)
			for _, f := range c.Froms {
				fmt.Printf("    - %s\n", f)
			}
		}
	}
	if len(p.Collisions) > 0 {
		fmt.Println(i18n.T("\n✘ final output paths collide; proceeding would overwrite one copy, apply will stop:"))
		for _, c := range p.Collisions {
			fmt.Printf("  %s\n", c.OutName)
			for _, f := range c.Froms {
				fmt.Printf("    - %s\n", f)
			}
		}
	}
	if len(p.RouteErrors) > 0 {
		fmt.Println(i18n.T("\n✘ workflow target or hook declaration problems, apply will stop:"))
		for _, e := range p.RouteErrors {
			fmt.Println("  -", e)
		}
	}

	fmt.Println(i18n.T("\n═══ mount preview ═══"))
	links, err := mount.Inspect(cfg)
	if err != nil {
		return errExit(err)
	}
	// Merge states are judged against the manifest like status does; without
	// one the registry file stands in.
	var mergeRecs []build.MergeRecord
	if m, err := build.LoadManifest(cfg.OutDir()); err == nil {
		mergeRecs = m.Merges
	}
	merges, err := mount.InspectMerge(cfg, mergeRecs)
	if err != nil {
		return errExit(err)
	}
	mergeBad := 0
	// Merge entries print under their directory, after that directory's links.
	printMerges := func(dir string) {
		for _, mp := range merges {
			if mp.Dir != dir {
				continue
			}
			var note string
			switch mp.State {
			case mount.MergeMissing:
				note = i18n.T("(merge into \"hooks\"; file will be created)")
			case mount.MergeAbsent:
				note = i18n.T("(merge into \"hooks\"; other keys untouched)")
			case mount.MergeClean:
				note = fmt.Sprintf(i18n.T("(merge into \"hooks\"; %d agsy entries present, will be refreshed)"), mp.Owned)
			case mount.MergeIdle:
				note = i18n.T("(no hook entries to merge; file untouched)")
			case mount.MergeStale:
				note = i18n.T("⚠ agsy entries point at a previous output path; apply updates them")
			case mount.MergeModified:
				note = i18n.T("⚠ agsy entries were modified — apply will ask before rebuilding them")
			case mount.MergeInvalid:
				note = "✘ " + mp.Note + i18n.T("; apply will fail, handle it manually")
				mergeBad++
			}
			fmt.Printf("  %-10s ⇐ %-28s %s\n", mp.Name, filepath.Base(mp.Registry), note)
		}
	}
	printed := map[string]bool{}
	curDir := ""
	realCnt, staleCnt := 0, 0
	for _, l := range links {
		if l.Dir != curDir {
			if curDir != "" {
				printMerges(curDir)
			}
			curDir = l.Dir
			printed[curDir] = true
			fmt.Printf("\n%s/\n", l.Dir)
		}
		var note string
		switch l.State {
		case mount.Missing:
			note = i18n.T("(missing, will be created)")
		case mount.IsLink:
			note = i18n.T("(existing link, will be deleted and recreated)")
		case mount.IsStale:
			note = "⚠ " + l.Note + i18n.T(", will be rebuilt to fix")
			staleCnt++
		case mount.IsReal:
			note = i18n.T("✘ a real directory or file already exists; apply will fail, handle it manually")
			realCnt++
		}
		fmt.Printf("  %-10s → %-28s %s\n", l.Name, l.Target, note)
	}
	if curDir != "" {
		printMerges(curDir)
	}
	for _, mp := range merges {
		if !printed[mp.Dir] {
			printed[mp.Dir] = true
			fmt.Printf("\n%s/\n", mp.Dir)
			printMerges(mp.Dir)
		}
	}

	fmt.Println(i18n.T("\n═══ summary ═══"))
	renames := 0
	for _, it := range p.Items {
		if it.Renamed {
			renames++
		}
	}
	// "Dropped" and "excluded" are two different things; lumping them into one
	// "skipped" would make the numbers impossible to reconcile: the former are
	// same-name items actively discarded by the first strategy, the latter are
	// files that do not match the inclusion rules.
	fmt.Printf(i18n.T("%d items │ %d renamed │ %d conflicts │ %d name collisions │ %d dropped (first) │ %d excluded │ %d links │ %d mount anomalies │ %d mount conflicts\n"),
		p.Placed(), renames, len(p.Conflicts), len(p.Collisions), len(p.Skipped), len(p.Ignored), len(links)+len(merges), staleCnt, realCnt+mergeBad)
	fmt.Println(i18n.T("\nNo files were written. Run agsy apply once everything looks right."))

	if len(p.Conflicts) > 0 || len(p.Collisions) > 0 || len(p.RouteErrors) > 0 {
		return 1
	}
	return 0
}
