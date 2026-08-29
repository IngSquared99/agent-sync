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
// If it already exists, enter edit mode: load the existing config as
// defaults so pressing Enter keeps current values, and show a diff for
// confirmation before writing. yaml comments get replaced with template
// comments during regeneration — that risk is covered by keeping agsy.yaml in
// version control, and the pre-write diff is the last manual checkpoint.
func cmdInit(argSources []string) int {
	// Non-interactive gate: conflict strategies require an explicit
	// choice. Without a TTY and without --yes the run is cancelled;
	// --yes is the explicit consent to the recommended defaults.
	if !prompt.IsStdinTTY() && !prompt.AssumeYes {
		fmt.Println(i18n.T("✘ init requires interactive prompts (the same-name strategy must be chosen by you explicitly)."))
		fmt.Println(i18n.T("  In non-interactive environments add --yes to accept the recommended defaults (rules=rename, skills=error, workflows=rename; a fresh init also selects ALL built-in tools)."))
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
	// Edit mode: pre-check the tools currently listed in build.tools
	var preselected []int
	for i, a := range adapters {
		if cur == nil {
			break
		}
		if cur.HasTool(a.Name) {
			preselected = append(preselected, i)
		}
	}
	picked := prompt.MultiSelect(i18n.T("Which tools should be served?"), labels, preselected)
	if len(picked) == 0 {
		fmt.Println(i18n.T("✘ At least one tool is required"))
		return 1
	}
	// Mount entries not matching any built-in adapter are kept as is;
	// regeneration must not silently drop manually configured mounts.
	// The root AGENTS.md entry is regenerated, never treated as custom.
	var customMounts []config.MountCfg
	rootExtras := map[string]string{}
	if cur != nil {
		known := map[string]bool{}
		for _, a := range adapters {
			known[a.Mount.Dir] = true
		}
		for _, m := range cur.Mount {
			if m.Dir == "." {
				// The root AGENTS.md entry is regenerated; every other link the
				// user added under the project root is kept, never silently
				// dropped by the regeneration.
				for k, v := range m.Links {
					if k == config.AgentsMD {
						continue
					}
					rootExtras[k] = v
				}
				continue
			}
			if !known[m.Dir] {
				customMounts = append(customMounts, m)
			}
		}
		if len(customMounts) > 0 || len(rootExtras) > 0 {
			fmt.Println(i18n.T("\n(the following custom mounts are not built-in adapters and will be kept as is)"))
			if len(rootExtras) > 0 {
				fmt.Println(i18n.T("  . (custom links besides AGENTS.md are kept)"))
			}
			for _, m := range customMounts {
				fmt.Printf("  %s/\n", m.Dir)
			}
		}
	}

	// ── The three mandatory on_conflict questions ──
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

	// ── tools: the picked adapters, plus hand-added names carried over ──
	toolSet := map[string]bool{}
	var tools []string
	for _, i := range picked {
		if !toolSet[adapters[i].Name] {
			toolSet[adapters[i].Name] = true
			tools = append(tools, adapters[i].Name)
		}
	}
	if cur != nil {
		adapterNames := map[string]bool{}
		for _, a := range adapters {
			adapterNames[a.Name] = true
		}
		for _, t := range cur.Build.Tools {
			if !adapterNames[t] && !toolSet[t] {
				toolSet[t] = true
				tools = append(tools, t)
			}
		}
	}

	// categories has no questionnaire of its own; carry the current values
	// over verbatim (same policy as custom mounts) instead of resetting
	// hand-edited files to template defaults.
	var keepCategories map[string]config.Category
	if cur != nil {
		keepCategories = cur.Build.Categories
	}
	newRaw := renderConfig(sources, out, strategies, tools, adapters, picked, customMounts, rootExtras, keepCategories)

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

	// Hold the project lock for the write phase only — not during the
	// prompts, where an abandoned interactive session would block every
	// apply for the stale-lock window. The prompts collect answers; the
	// writes below are what must not interleave with a concurrent apply.
	release, lerr := acquireLockDir(wd)
	if lerr != nil {
		return errExit(lerr)
	}
	defer release()
	if err := os.WriteFile(path, []byte(newRaw), 0o644); err != nil {
		return errExit(err)
	}
	fmt.Printf(i18n.T("\n✔ Wrote %s\n"), filepath.Base(path))
	// A real AGENTS.md at the project root blocks the root mount; say so now
	// instead of letting the first apply fail.
	if needAgentsMD(adapters, picked) {
		if fi, err := os.Lstat(filepath.Join(wd, config.AgentsMD)); err == nil && fi.Mode().IsRegular() {
			fmt.Printf(i18n.T("  ⚠ %s already exists at the project root. agsy mounts its own generated %s there and never merges or overwrites a real file.\n"), config.AgentsMD, config.AgentsMD)
			fmt.Println(i18n.T("     Move its content into a source rules/ directory (or rename the file to keep it), then run agsy apply."))
		}
	}
	offerGitignore(wd, out, adapters, picked, customMounts, rootExtras)
	fmt.Println(i18n.T("  Next: agsy plan to preview → agsy apply to execute"))
	return 0
}

func needAgentsMD(adapters []Adapter, picked []int) bool {
	for _, i := range picked {
		if adapters[i].NeedsAgentsMD {
			return true
		}
	}
	return false
}

// askSources asks for source paths line by line
func askSources() []string {
	var sources []string
	fmt.Println(i18n.T("Source paths, ordered by priority (~ prefix = shared library, ./ prefix = in-project)"))
	fmt.Println(i18n.T("One per line; press Enter on an empty line to finish (e.g. ~/all-ai-lib, ./repo-ai-lib)"))
	for i := 1; ; i++ {
		s := prompt.Input(fmt.Sprintf(i18n.T("  source %d"), i), "")
		if s == "" {
			// Re-ask only on a TTY; in non-interactive mode (EOF) re-asking
			// would loop forever, so return the empty list to the caller
			// (init rejects empty sources).
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
	tools []string, adapters []Adapter, picked []int, customMounts []config.MountCfg,
	rootExtras map[string]string, categories map[string]config.Category) string {
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
		"rules":     {From: "rules", To: "rules"},
		"skills":    {From: "skills", To: "skills"},
		"workflows": {From: "workflows", To: "workflows"},
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
	b.WriteString("  tools: [" + strings.Join(tools, ", ") + "]" + i18n.T("   # valid values for a workflow's target: front matter\n"))
	b.WriteString("\nmount:\n")
	if needAgentsMD(adapters, picked) || len(rootExtras) > 0 {
		b.WriteString(i18n.T("  - dir: .                    # project root: AGENTS.md for Codex / Cursor / Antigravity\n"))
		b.WriteString("    links:\n")
		if needAgentsMD(adapters, picked) {
			b.WriteString("      AGENTS.md: AGENTS.md\n")
		}
		var extraNames []string
		for k := range rootExtras {
			extraNames = append(extraNames, k)
		}
		sort.Strings(extraNames)
		for _, k := range extraNames {
			b.WriteString(fmt.Sprintf("      %-10s %s\n", k+":", rootExtras[k]))
		}
	}
	// Adapters sharing a dir are merged here so the generated file is already
	// in canonical form (config.Load would merge them anyway).
	type mountAcc struct {
		dir      string
		links    map[string]string
		displays []string
	}
	var accs []*mountAcc
	byDir := map[string]*mountAcc{}
	for _, i := range picked {
		a := adapters[i]
		acc, ok := byDir[a.Mount.Dir]
		if !ok {
			acc = &mountAcc{dir: a.Mount.Dir, links: map[string]string{}}
			byDir[a.Mount.Dir] = acc
			accs = append(accs, acc)
		}
		acc.displays = append(acc.displays, a.Display)
		for k, v := range a.Mount.Links {
			acc.links[k] = v
		}
	}
	for _, acc := range accs {
		b.WriteString("  - dir: " + acc.dir + "        # " + strings.Join(acc.displays, " + ") + "\n")
		b.WriteString("    links:\n")
		var names []string
		for k := range acc.links {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			b.WriteString(fmt.Sprintf("      %-10s %s\n", k+":", acc.links[k]))
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

// offerGitignore proposes ignoring every generated path: the output
// directory, the lock file, and all mount links (adapter, custom, root).
// Existing entries are skipped; the user picks which ones to add.
func offerGitignore(wd, out string, adapters []Adapter, picked []int, customMounts []config.MountCfg, rootExtras map[string]string) {
	candSet := map[string]bool{}
	var cands []string
	add := func(e string) {
		if e != "" && !candSet[e] {
			candSet[e] = true
			cands = append(cands, e)
		}
	}
	addLinks := func(dir string, links map[string]string) {
		prefix := strings.TrimPrefix(dir, "./")
		if prefix == "." {
			prefix = ""
		} else {
			prefix += "/"
		}
		var names []string
		for k := range links {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			add(prefix + k)
		}
	}
	add(strings.TrimSuffix(strings.TrimPrefix(out, "./"), "/") + "/")
	add(".agsy.lock")
	if needAgentsMD(adapters, picked) {
		add(config.AgentsMD)
	}
	for _, i := range picked {
		addLinks(adapters[i].Mount.Dir, adapters[i].Mount.Links)
	}
	for _, m := range customMounts {
		addLinks(m.Dir, m.Links)
	}
	addLinks(".", rootExtras)
	gi := filepath.Join(wd, ".gitignore")
	existing := map[string]bool{}
	raw, err := os.ReadFile(gi)
	if err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			existing[strings.TrimSpace(line)] = true
		}
	} else if !os.IsNotExist(err) {
		return
	}
	var missing []string
	for _, e := range cands {
		if !existing[e] && !existing[strings.TrimSuffix(e, "/")] {
			missing = append(missing, e)
		}
	}
	if len(missing) == 0 {
		return
	}
	fmt.Println(i18n.T("\nThe following generated paths are rebuildable and usually belong in .gitignore:"))
	picks := prompt.MultiSelect(i18n.T("Add which entries to .gitignore?"), missing, nil)
	if len(picks) == 0 {
		fmt.Println(i18n.T("  (you can add them to .gitignore later yourself; agsy.yaml, however, should be version-controlled)"))
		return
	}
	content := string(raw)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	for _, i := range picks {
		content += missing[i] + "\n"
	}
	if err := os.WriteFile(gi, []byte(content), 0o644); err != nil {
		fmt.Println(i18n.T("  ⚠ Failed to write .gitignore:"), err)
		return
	}
	fmt.Printf(i18n.T("  ✔ Added %d entries to .gitignore\n"), len(picks))
}
