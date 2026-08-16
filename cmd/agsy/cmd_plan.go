package main

import (
	"fmt"

	"github.com/IngSquared99/agent-sync/internal/build"
	"github.com/IngSquared99/agent-sync/internal/config"
	"github.com/IngSquared99/agent-sync/i18n"
	"github.com/IngSquared99/agent-sync/internal/mount"
)

// cmdPlan rehearses everything apply would do, without writing anything.
// Missing sources: skipping is fine, staying silent is not (§12-9). Same for
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
				line += "   → " + joinComma(it.Buckets)
				if it.RouteNote != "" {
					line += " (" + it.RouteNote + ")"
				}
			}
			fmt.Println(line)
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
		fmt.Println(i18n.T("\n✘ final artifact names collide; proceeding would overwrite one copy, apply will stop:"))
		for _, c := range p.Collisions {
			fmt.Printf("  %s/%s\n", c.Category, c.OutName)
			for _, f := range c.Froms {
				fmt.Printf("    - %s\n", f)
			}
		}
	}
	for _, w := range p.NoBucket {
		fmt.Printf(i18n.T("\n⚠ workflow %s has no target marker and default is empty; it will go into no bucket\n"), w)
	}

	fmt.Println(i18n.T("\n═══ mount preview ═══"))
	links, err := mount.Inspect(cfg)
	if err != nil {
		return errExit(err)
	}
	curDir := ""
	realCnt, staleCnt := 0, 0
	for _, l := range links {
		if l.Dir != curDir {
			curDir = l.Dir
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
		p.Placed(), renames, len(p.Conflicts), len(p.Collisions), len(p.Skipped), len(p.Ignored), len(links), staleCnt, realCnt)
	fmt.Println(i18n.T("\nNo files were written. Run agsy apply once everything looks right."))

	if len(p.Conflicts) > 0 || len(p.Collisions) > 0 {
		return 1
	}
	return 0
}

func joinComma(xs []string) string {
	s := ""
	for i, x := range xs {
		if i > 0 {
			s += ", "
		}
		s += x
	}
	return s
}
