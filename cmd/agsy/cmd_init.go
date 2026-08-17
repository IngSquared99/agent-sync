package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/IngSquared99/agent-sync/i18n"
	"github.com/IngSquared99/agent-sync/internal/config"
	"github.com/IngSquared99/agent-sync/internal/prompt"
)

// cmdInit interactively generates agsy.yaml.
// If it already exists, enter edit mode (§12-7): load the existing config as
// defaults so pressing Enter keeps current values, and show a diff for
// confirmation before writing. yaml comments get replaced with template
// comments during regeneration — that risk is covered by keeping agsy.yaml in
// version control, and the pre-write diff is the last manual checkpoint.
func cmdInit(argSources []string) int {
	// Non-interactive gate (§12-19): the core of init is "the strategy must be
	// explicitly chosen by the user" (§12-1). When nobody can answer and --yes
	// is absent, silently writing a config with defaults would be making the
	// decision on the user's behalf, so always cancel; scripts must state
	// --yes explicitly (= explicit consent to the recommended defaults).
	if !prompt.IsStdinTTY() && !prompt.AssumeYes {
		fmt.Println(i18n.T("✘ init requires interactive prompts (the same-name strategy must be chosen by you explicitly)."))
		fmt.Println(i18n.T("  In non-interactive environments add --yes to accept the recommended defaults (rules=rename, skills=error, workflows=rename)."))
		return 1
	}
	wd, _ := os.Getwd()
	path, exists := config.Find(wd)

	var cur *config.Config
	var oldRaw string
	if exists {
		if raw, err := os.ReadFile(path); err == nil {
			oldRaw = string(raw)
		}
		c, err := config.Load(path)
		if err != nil {
			fmt.Printf(i18n.T("⚠ Existing %s cannot be loaded: %v\n"), config.FileName, err)
			fmt.Println(i18n.T("  You can reconfigure from scratch (changes are shown before writing); abort now if you'd rather fix it yourself."))
			if !prompt.Confirm(i18n.T("Continue with reconfiguration?")) {
				fmt.Println(i18n.T("Cancelled."))
				return 1
			}
		} else {
			cur = c
		}
		fmt.Printf(i18n.T("\nDetected existing %s, entering edit mode (Enter keeps the current value)\n\n"), config.FileName)
	} else {
		fmt.Print(i18n.T("Setting up agsy (Enter accepts the default)\n\n"))
	}

	// ── Sources ──
	var sources []string
	if cur != nil && len(cur.Sources) > 0 {
		fmt.Println(i18n.T("Current sources (by priority):"))
		for i, s := range cur.Sources {
			fmt.Printf("  [%d] %s\n", i+1, s)
		}
		if prompt.Select(i18n.T("Change the source paths?"), []string{i18n.T("Keep as is"), i18n.T("Re-enter")}, 0) == 0 {
			sources = append(sources, cur.Sources...)
		}
	}
	// Sources given directly on the command line (agsy init ~/all-ai-lib ./repo-ai-lib):
	// non-interactive environments have no prompts, so project-specific data
	// like sources must be stated explicitly via arguments.
	if len(sources) == 0 && len(argSources) > 0 {
		sources = append(sources, argSources...)
	}
	if len(sources) == 0 {
		sources = askSources()
		if len(sources) == 0 {
			fmt.Println(i18n.T("✘ At least one source is required; in non-interactive mode pass them as arguments, e.g. agsy init --yes ~/all-ai-lib ./repo-ai-lib"))
			return 1
		}
	}

	// ── Tools to mount (from adapters) ──
	adapters, err := loadAdapters()
	if err != nil {
		return errExit(err)
	}
	var labels []string
	for _, a := range adapters {
		labels = append(labels, fmt.Sprintf(i18n.T("%s (%s/)"), a.Display, a.Mount.Dir))
	}
	// Edit mode: pre-check the tools that are currently mounted
	var preselected []int
	for i, a := range adapters {
		if cur == nil {
			break
		}
		for _, m := range cur.Mount {
			if m.Dir == a.Mount.Dir {
				preselected = append(preselected, i)
				break
			}
		}
	}
	picked := prompt.MultiSelect(i18n.T("Which tools should be mounted?"), labels, preselected)
	if len(picked) == 0 {
		fmt.Println(i18n.T("✘ At least one mount target is required"))
		return 1
	}
	// Mount entries the user added manually in agsy.yaml that don't belong to
	// any adapter are kept as is. Washing them away during regeneration would
	// silently disconnect tools the user wired up themselves.
	var customMounts []config.MountCfg
	if cur != nil {
		known := map[string]bool{}
		for _, a := range adapters {
			known[a.Mount.Dir] = true
		}
		for _, m := range cur.Mount {
			if !known[m.Dir] {
				customMounts = append(customMounts, m)
			}
		}
		if len(customMounts) > 0 {
			fmt.Println(i18n.T("\n(the following custom mounts are not built-in adapters and will be kept as is)"))
			for _, m := range customMounts {
				fmt.Printf("  %s/\n", m.Dir)
			}
		}
	}

	// ── The three mandatory on_conflict questions (§12-1) ──
	strategies := map[string]string{}
	opts := []string{
		i18n.T("rename   keep both copies, tagging filenames with their source"),
		i18n.T("error    stop and list conflicts for you to resolve manually (most conservative)"),
		i18n.T("first    keep only the copy from the higher-priority source, discard the rest"),
	}
	optIdx := map[string]int{"rename": 0, "error": 1, "first": 2}
	ask := func(cat, hint string, defIdx int) {
		if cur != nil {
			if s, ok := cur.Build.OnConflict[cat]; ok {
				if i, ok := optIdx[s]; ok {
					defIdx = i
					hint = fmt.Sprintf(i18n.T("(current: %s)"), s)
				}
			}
		}
		i := prompt.Select(fmt.Sprintf(i18n.T("\nHow should same-name conflicts in %s be handled?%s"), cat, hint), opts, defIdx)
		strategies[cat] = strings.Fields(opts[i])[0]
	}
	ask("rules", i18n.T("(recommended rename: \"global base + project extras\" often need to coexist)"), 0)
	ask("skills", i18n.T("(recommended error: skills trigger on description semantics, so coexistence is unpredictable; rename also rewrites front-matter)"), 1)
	ask("workflows", "", 0)

	// ── Output directory ──
	outDef := ".agsy"
	if cur != nil && cur.Build.Out != "" {
		outDef = cur.Build.Out
	}
	out := prompt.Input(i18n.T("\nBuild output directory"), outDef)

	// ── route: buckets = union across enabled adapters, plus buckets used by custom mounts ──
	bucketSet := map[string]bool{}
	for _, i := range picked {
		bucketSet[adapters[i].Bucket] = true
	}
	// Hand-edited buckets are carried over (union): dropping one would turn
	// every workflow targeting it into a route error after regeneration.
	if cur != nil {
		for _, b := range cur.Build.Route.Buckets {
			bucketSet[b] = true
		}
	}
	for _, m := range customMounts {
		for _, sub := range m.Links {
			parts := strings.Split(strings.Trim(filepath.ToSlash(sub), "/"), "/")
			if len(parts) == 2 {
				bucketSet[parts[1]] = true
			}
		}
	}
	var buckets []string
	for b := range bucketSet {
		buckets = append(buckets, b)
	}
	sort.Strings(buckets)

	defIdx := 0
	partialDefault := false
	if cur != nil {
		if len(cur.Build.Route.Default) == 0 {
			defIdx = 1
		} else if len(cur.Build.Route.Default) != len(buckets) {
			// A hand-edited partial default is not expressible by the two
			// standard answers; offer to keep it verbatim (same carry-over
			// policy as custom mounts).
			partialDefault = true
		}
	}
	defOpts := []string{
		i18n.T("All buckets (recommended: a file showing up everywhere is easier to understand than one that mysteriously disappears)"),
		i18n.T("Nowhere, and warn during plan / apply"),
	}
	if partialDefault {
		defOpts = append([]string{fmt.Sprintf(i18n.T("Keep current value (%v)"), cur.Build.Route.Default)}, defOpts...)
		defIdx = 0
	}
	di := prompt.Select(i18n.T("\nWhere should workflows without a target go?"), defOpts, defIdx)
	var routeDefault []string
	switch {
	case partialDefault && di == 0:
		for _, d := range cur.Build.Route.Default {
			if containsStr(buckets, d) {
				routeDefault = append(routeDefault, d)
			}
		}
	case (partialDefault && di == 1) || (!partialDefault && di == 0):
		routeDefault = buckets
	}

	// categories and route.field have no questionnaire of their own; carry the
	// current values over verbatim (same policy as custom mounts) instead of
	// resetting hand-edited files to template defaults.
	var keepCategories map[string]config.Category
	routeField := "target"
	if cur != nil {
		keepCategories = cur.Build.Categories
		if cur.Build.Route.Field != "" {
			routeField = cur.Build.Route.Field
		}
	}
	newRaw := renderConfig(sources, out, strategies, buckets, routeDefault, adapters, picked, customMounts, keepCategories, routeField)

	// ── Edit mode: show diff before writing ──
	if exists {
		if newRaw == oldRaw {
			fmt.Printf(i18n.T("\nNo changes; %s left as is.\n"), config.FileName)
			return 0
		}
		fmt.Println(i18n.T("\nAbout to write the following changes:"))
		printDiff(oldRaw, newRaw)
		fmt.Println(i18n.T("\n(note: yaml comments are replaced with template comments; re-add custom comments after writing)"))
		if !prompt.Confirm(i18n.T("Confirm write?")) {
			fmt.Println(i18n.T("Cancelled; original file untouched."))
			return 1
		}
	}

	if err := os.WriteFile(path, []byte(newRaw), 0o644); err != nil {
		return errExit(err)
	}
	fmt.Printf(i18n.T("\n✔ Wrote %s\n"), filepath.Base(path))
	warnUnmountedCategories(newRaw, adapters, picked, customMounts)
	// Ask on both first run and edit mode: when edit mode changes build.out,
	// the new directory likewise shouldn't be version-controlled.
	// offerGitignore already guards against duplicates — it returns silently
	// when the entry exists, so it won't nag every time.
	offerGitignore(wd, out)
	fmt.Println(i18n.T("  Next: agsy plan to preview → agsy apply to execute"))
	return 0
}

// askSources asks for source paths line by line
func askSources() []string {
	var sources []string
	fmt.Println(i18n.T("Source paths, ordered by priority (~ prefix = shared library, ./ prefix = in-project)"))
	fmt.Println(i18n.T("One per line; press Enter on an empty line to finish (e.g. ~/all-ai-lib, ./repo-ai-lib)"))
	for i := 1; ; i++ {
		s := prompt.Input(fmt.Sprintf(i18n.T("  source %d"), i), "")
		if s == "" {
			// Only re-ask when someone can actually answer; re-asking in
			// non-interactive mode (EOF) would become an infinite loop, so
			// hand back an empty list for the caller to wrap up (init already
			// guards against empty sources).
			if len(sources) == 0 && prompt.IsStdinTTY() {
				fmt.Println(i18n.T("  At least one source path is required, please enter one (or Ctrl+C to quit)"))
				i--
				continue
			}
			break
		}
		sources = append(sources, s)
	}
	return sources
}

// renderConfig assembles the commented agsy.yaml content
func renderConfig(sources []string, out string, strategies map[string]string,
	buckets, routeDefault []string, adapters []Adapter, picked []int, customMounts []config.MountCfg,
	categories map[string]config.Category, routeField string) string {
	var b strings.Builder
	b.WriteString(i18n.T("# agsy config file (agent-sync)\n"))
	b.WriteString(i18n.T("# Path syntax: ~ prefix = home expansion; relative paths = resolved from this file's directory; absolute paths = as is\n"))
	b.WriteString(i18n.T("version: 1\n\nsources:                      # ordered array, earlier entries win\n"))
	for _, s := range sources {
		b.WriteString("  - " + s + "\n")
	}
	b.WriteString("\nbuild:\n")
	b.WriteString("  out: " + out + i18n.T("                   # must be inside the project directory (apply wipes it entirely)\n\n"))
	b.WriteString(i18n.T("  categories:                 # source subdir → output subdir (the three to values must all differ)\n"))
	cats := map[string]config.Category{
		"rules":     {From: "rule", To: "rules"},
		"skills":    {From: "skill", To: "skills"},
		"workflows": {From: "workflow", To: "workflows"},
	}
	if categories != nil {
		cats = categories // loaded configs are already default-filled
	}
	for _, cat := range config.CategoryOrder {
		b.WriteString(fmt.Sprintf("    %-10s { from: %s, to: %s }\n", cat+":", cats[cat].From, cats[cat].To))
	}
	b.WriteString("\n")
	b.WriteString(i18n.T("  on_conflict:                # same-name handling: first / rename / error (required per category)\n"))
	b.WriteString("    rules:     " + strategies["rules"] + "\n")
	b.WriteString("    skills:    " + strategies["skills"] + "\n")
	b.WriteString("    workflows: " + strategies["workflows"] + "\n\n")
	b.WriteString(i18n.T("  route:                      # workflow routing\n"))
	if routeField == "" {
		routeField = "target"
	}
	b.WriteString(fmt.Sprintf("    field: %-19s", routeField) + i18n.T("# which front-matter field to read (agsy custom field)\n"))
	b.WriteString("    default: [" + strings.Join(routeDefault, ", ") + "]\n")
	b.WriteString("    buckets: [" + strings.Join(buckets, ", ") + "]\n\n")
	b.WriteString("mount:\n")
	for _, i := range picked {
		a := adapters[i]
		b.WriteString("  - dir: " + a.Mount.Dir + "        # " + a.Display + "\n")
		b.WriteString("    links:\n")
		var names []string
		for k := range a.Mount.Links {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			b.WriteString(fmt.Sprintf("      %-10s %s\n", k+":", a.Mount.Links[k]))
		}
	}
	for _, m := range customMounts {
		b.WriteString("  - dir: " + m.Dir + i18n.T("        # custom mount (not a built-in adapter)\n"))
		b.WriteString("    links:\n")
		var names []string
		for k := range m.Links {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			b.WriteString(fmt.Sprintf("      %-10s %s\n", k+":", m.Links[k]))
		}
	}
	return b.String()
}

// warnUnmountedCategories flags categories that get built but nothing mounts.
// E.g. picking only Codex mounts just prompts — rules and skills would be
// built yet no tool would ever read them.
func warnUnmountedCategories(_ string, adapters []Adapter, picked []int, customMounts []config.MountCfg) {
	mounted := map[string]bool{}
	collect := func(links map[string]string) {
		for _, sub := range links {
			top := strings.Split(strings.Trim(filepath.ToSlash(sub), "/"), "/")[0]
			mounted[top] = true
		}
	}
	for _, i := range picked {
		collect(adapters[i].Mount.Links)
	}
	for _, m := range customMounts {
		collect(m.Links)
	}
	var orphan []string
	for _, cat := range config.CategoryOrder {
		if !mounted[cat] {
			orphan = append(orphan, cat)
		}
	}
	if len(orphan) > 0 {
		fmt.Printf(i18n.T("  ⚠ %s will be built, but the tools you selected do not mount this layer — no tool will read them for now.\n"),
			strings.Join(orphan, " / "))
		fmt.Println(i18n.T("     If needed, add a links entry under mount in agsy.yaml yourself."))
	}
}

// printDiff lists differences line by line (LCS). The config file is only a
// few dozen lines, so computing it directly is the simplest option.
func printDiff(oldRaw, newRaw string) {
	a := strings.Split(strings.TrimRight(oldRaw, "\n"), "\n")
	b := strings.Split(strings.TrimRight(newRaw, "\n"), "\n")
	// lcs[i][j] = length of the longest common subsequence of a[i:] and b[j:]
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			i, j = i+1, j+1
		case lcs[i+1][j] >= lcs[i][j+1]:
			fmt.Println("  - " + a[i])
			i++
		default:
			fmt.Println("  + " + b[j])
			j++
		}
	}
	for ; i < len(a); i++ {
		fmt.Println("  - " + a[i])
	}
	for ; j < len(b); j++ {
		fmt.Println("  + " + b[j])
	}
}

// offerGitignore: build outputs are rebuildable and shouldn't be version-controlled;
// rather than telling users to add the entry themselves in a README, ask on the spot.
func offerGitignore(wd, out string) {
	entry := strings.TrimSuffix(strings.TrimPrefix(out, "./"), "/") + "/"
	gi := filepath.Join(wd, ".gitignore")
	raw, err := os.ReadFile(gi)
	if err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			t := strings.TrimSpace(line)
			if t == entry || t == strings.TrimSuffix(entry, "/") {
				return // already present
			}
		}
	} else if !os.IsNotExist(err) {
		return
	}
	if !prompt.Confirm(fmt.Sprintf(i18n.T("\n%s is a rebuildable artifact; add it to .gitignore?"), entry)) {
		fmt.Printf(i18n.T("  (you can add %s to .gitignore later yourself; agsy.yaml, however, should be version-controlled)\n"), entry)
		return
	}
	content := string(raw)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += entry + "\n"
	if err := os.WriteFile(gi, []byte(content), 0o644); err != nil {
		fmt.Println(i18n.T("  ⚠ Failed to write .gitignore:"), err)
		return
	}
	fmt.Printf(i18n.T("  ✔ Added %s to .gitignore\n"), entry)
}

func containsStr(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
