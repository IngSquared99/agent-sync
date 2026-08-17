package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/IngSquared99/agent-sync/i18n"
	"github.com/IngSquared99/agent-sync/internal/build"
	"github.com/IngSquared99/agent-sync/internal/config"
	"github.com/IngSquared99/agent-sync/internal/prompt"
	"github.com/IngSquared99/agent-sync/internal/state"
)

// cmdPromote writes changes made in the build output back to the sources (§12-4):
//
//	agsy promote                 interactive multi-select, per-item target override
//	agsy promote --all           write back everything, each to its original source; list first, then confirm
//	agsy promote <cat/name>      single item, back to its original source
//	agsy promote <cat/name> --to <source>  single item, redirected
//
// --all --to is not offered (batch + redirect would desync sources from the manifest).
func cmdPromote(args []string) int {
	cfg, err := loadConfig()
	if err != nil {
		return errExit(err)
	}
	out := cfg.OutDir()
	m, err := build.LoadManifest(out)
	if err != nil {
		return errExit(fmt.Errorf(i18n.T("manifest not found; run agsy apply first to complete the initial build")))
	}
	rep, err := state.Collect(cfg, m)
	if err != nil {
		return errExit(err)
	}
	if len(rep.Locals) == 0 {
		fmt.Println(i18n.T("✔ No local changes; nothing to write back."))
		return 0
	}

	// Parse arguments
	all := false
	toArg := ""
	var target string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--all":
			all = true
		case "--to":
			if i+1 >= len(args) {
				return errExit(fmt.Errorf(i18n.T("--to requires a source path")))
			}
			toArg = args[i+1]
			i++
		default:
			target = args[i]
		}
	}
	if all && toArg != "" {
		return errExit(fmt.Errorf(i18n.T("--all --to is not supported: batch redirection desyncs sources from the manifest; --to is single-item only")))
	}

	var code int
	switch {
	case target != "": // single item
		// A bare name (without the category prefix) is ambiguous when two
		// categories have same-named items; silently taking the first match
		// could write back the wrong thing, so collect all matches before deciding.
		var matches []state.LocalChange
		for _, lc := range rep.Locals {
			key := lc.Item.Category + "/" + lc.Item.Name
			if key == target || lc.Item.Name == target {
				matches = append(matches, lc)
			}
		}
		switch len(matches) {
		case 0:
			return errExit(fmt.Errorf(i18n.T("%q is not in the local change list (see agsy status)"), target))
		case 1:
			code = promoteOne(cfg, out, m, matches[0], toArg, true)
		default:
			fmt.Printf(i18n.T("✘ %q matches multiple items; use the full \"category/name\" form:\n"), target)
			for _, lc := range matches {
				fmt.Printf("  agsy promote %s/%s\n", lc.Item.Category, lc.Item.Name)
			}
			return 1
		}
	case all:
		code = promoteAll(cfg, out, m, rep.Locals)
	default: // interactive
		code = promoteInteractive(cfg, out, m, rep.Locals)
	}
	// For items written back successfully, the current output content is now
	// accepted as "no change worth keeping"; update the output hashes in the
	// manifest so status stops nagging about the same thing.
	// (Source hashes stay untouched → "behind" still shows, reminding you to apply next.)
	if err := build.WriteManifest(out, m); err != nil {
		fmt.Println(i18n.T("⚠ manifest update failed; status may still show local changes:"), err)
	}
	return code
}

// sourceRootLabel returns the source root an item belongs to; falls back to the parent directory when it cannot be derived
func sourceRootLabel(cfg *config.Config, from string) string {
	if root, ok := cfg.SourceRootOf(from); ok {
		return root
	}
	return filepath.Dir(from)
}

// outsideProject reports whether a path lies outside the project (a shared library); write-backs there deserve an extra warning
func outsideProject(cfg *config.Config, p string) bool {
	return !config.IsAncestor(cfg.BaseDir, p)
}

func promoteAll(cfg *config.Config, out string, m *build.Manifest, locals []state.LocalChange) int {
	// Group by original source and list everything before acting (§12-4).
	// Source roots always come from prefix-matching against sources, never
	// from counting path segments upward.
	groups := map[string][]state.LocalChange{}
	for _, lc := range locals {
		groups[sourceRootLabel(cfg, lc.Item.From)] = append(groups[sourceRootLabel(cfg, lc.Item.From)], lc)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Printf(i18n.T("About to write back %d items:\n"), len(locals))
	for _, k := range keys {
		warn := ""
		if outsideProject(cfg, k) {
			warn = i18n.T(" ⚠ writes to a shared library, affecting every project that uses it")
		}
		fmt.Printf(i18n.T("  → %s (%d items)%s\n"), k, len(groups[k]), warn)
		for _, lc := range groups[k] {
			mark := ""
			if lc.SrcRootGone {
				mark = i18n.T("   ⚠ source root missing; will be skipped")
			} else if lc.SrcDeleted {
				mark = i18n.T("   ⚠ source file deleted; will be recreated")
			}
			fmt.Printf("      %s/%s%s\n", lc.Item.Category, lc.Item.Name, mark)
		}
	}
	if !prompt.Confirm(i18n.T("\nWrite back all of them?")) {
		fmt.Println(i18n.T("Cancelled."))
		return 1
	}
	// Writable items complete as usual; unwritable ones are reported individually
	// (write-backs are independent of each other, no whole-batch abort)
	okCnt := 0
	var failed []string
	for _, lc := range locals {
		if code := promoteOne(cfg, out, m, lc, "", false); code == 0 {
			okCnt++
		} else {
			failed = append(failed, lc.Item.Category+"/"+lc.Item.Name)
		}
	}
	if len(failed) > 0 {
		fmt.Printf(i18n.T("\n✘ %d items could not be written back:\n"), len(failed))
		for _, f := range failed {
			fmt.Printf(i18n.T("  %s\n  → alternatively use: agsy promote %s --to <writable source>\n"), f, f)
		}
	}
	if len(failed) > 0 {
		fmt.Printf(i18n.T("The remaining %d items were written back.\n"), okCnt)
	} else {
		fmt.Printf(i18n.T("All %d items were written back.\n"), okCnt)
	}
	if okCnt > 0 {
		fmt.Println(i18n.T("Write-back complete; sources and outputs are consistent again."))
	}
	if len(failed) > 0 {
		return 1
	}
	return 0
}

func promoteInteractive(cfg *config.Config, out string, m *build.Manifest, locals []state.LocalChange) int {
	var labels []string
	for _, lc := range locals {
		labels = append(labels, fmt.Sprintf(i18n.T("%s/%s   original source: %s"), lc.Item.Category, lc.Item.Name, lc.Item.From))
	}
	picked := prompt.MultiSelect(fmt.Sprintf(i18n.T("Detected %d local changes; select the items to write back"), len(locals)), labels, nil)
	// An empty pick only happens on the non-interactive cancel path
	// (interactive input insists on at least one selection).
	if len(picked) == 0 {
		return 1
	}
	okCnt := 0
	for _, i := range picked {
		lc := locals[i]
		dest := ""
		di := prompt.Select(fmt.Sprintf(i18n.T("\nWrite-back target for %s/%s"), lc.Item.Category, lc.Item.Name), []string{
			fmt.Sprintf(i18n.T("Original source (%s)"), lc.Item.From),
			i18n.T("Specify another source..."),
		}, 0)
		if di == 1 {
			dest = prompt.Input(i18n.T("Target source path (e.g. ./repo-ai-lib)"), "")
			if dest == "" {
				fmt.Println(i18n.T("  Skipping this item."))
				continue
			}
		}
		if code := promoteOne(cfg, out, m, lc, dest, true); code == 0 {
			okCnt++
		}
	}
	if okCnt > 0 {
		fmt.Println(i18n.T("\nReminder: sources have changed; run agsy apply to rebuild the outputs."))
	}
	return 0
}

// resolvePromoteDest computes and validates the write-back destination for an
// item. Every destination trust rule lives here, in one place.
//
// Threat model: the manifest lives in the build output — the one layer mounted
// AI tools can write to — so From / Original must never be able to point a
// write (or the RemoveAll preceding a directory write) anywhere except the
// item's own slot: <configured source>/<categories.from>/<original name>.
// "somewhere inside a source" is NOT enough — a manifest pointing at the
// category directory itself, or a parent, would let the directory write-back
// RemoveAll a whole tree of originals. agsy.yaml is user-owned and outside the
// output; its sources plus the category layout are the trust anchor.
// (Residual surface, accepted: a fully forged manifest can still target ONE
// sibling item slot in the same category — that equals what a legitimate
// promote of that item could do, and status shows the origin path first.)
func resolvePromoteDest(cfg *config.Config, it build.ManifestItem, toRaw string) (string, error) {
	from := cfg.Build.Categories[it.Category].From
	dest := it.From
	if toRaw != "" {
		abs, err := cfg.ExpandPath(toRaw)
		if err != nil {
			return "", err
		}
		// --to may only point at a configured source: content written anywhere
		// else silently leaves agsy's management, and an unrestricted redirect
		// would defeat the slot check below.
		isSource := false
		for _, root := range cfg.SourceRoots() {
			if root == abs {
				isSource = true
				break
			}
		}
		if !isSource {
			return "", fmt.Errorf(i18n.T("--to %s is not a source configured in %s; only configured sources may receive write-backs"), toRaw, config.FileName)
		}
		dest = filepath.Join(abs, from, it.Original)
	}
	dest = filepath.Clean(dest)
	inSlot := false
	for _, root := range cfg.SourceRoots() {
		if filepath.Dir(dest) == filepath.Clean(filepath.Join(root, from)) {
			inSlot = true
			break
		}
	}
	if !inSlot || filepath.Base(dest) != it.Original {
		return "", fmt.Errorf(i18n.T("destination %s is not the item's own slot <source>/%s/%s — the manifest may be tampered with or stale (agsy apply rebuilds it)"), dest, from, it.Original)
	}
	// Never write through a link sitting at the destination.
	if fi, err := os.Lstat(dest); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf(i18n.T("destination %s is a symbolic link; refusing to write through it"), dest)
	}
	return dest, nil
}

// promoteOne writes back a single item. A non-empty toRaw redirects it
// (relocates it into the matching category subdirectory of the given source).
// When confirm is true, writing into a shared library outside the project asks
// once more (--all already confirmed at the listing stage).
func promoteOne(cfg *config.Config, out string, m *build.Manifest, lc state.LocalChange, toRaw string, confirm bool) int {
	it := lc.Item
	if lc.Multiple {
		fmt.Printf(i18n.T("✘ %s/%s has multiple bucket copies changed to different contents; compare and resolve manually\n"), it.Category, it.Name)
		return 1
	}
	if lc.SrcRootGone {
		fmt.Printf(i18n.T("✘ the source root of %s/%s is missing (repo not cloned? disk not mounted?); fix the source path before writing back\n"), it.Category, it.Name)
		return 1
	}
	if lc.SrcDeleted && toRaw == "" {
		// The source file no longer exists, so recreating it must be stated
		// explicitly (status warns the item disappears on apply; promote must
		// not recreate it silently).
		fmt.Printf(i18n.T("⚠ the source file of %s/%s was deleted; writing back will recreate %s\n"), it.Category, it.Name, it.From)
		if confirm && !prompt.Confirm(i18n.T("Recreate it?")) {
			fmt.Println(i18n.T("  Skipped."))
			return 1
		}
	}
	if lc.SrcAlso && toRaw == "" {
		fmt.Printf(i18n.T("✘ the source of %s/%s has changed too (both sides modified); overwriting would clobber the source's new content, compare manually\n"), it.Category, it.Name)
		return 1
	}
	srcCopy := filepath.Join(out, filepath.FromSlash(lc.OutPath))
	dest, destErr := resolvePromoteDest(cfg, it, toRaw)
	if destErr != nil {
		fmt.Printf(i18n.T("✘ refusing to write back %s/%s: %v\n"), it.Category, it.Name, destErr)
		return 1
	}
	if confirm && outsideProject(cfg, dest) {
		fmt.Printf(i18n.T("⚠ %s is outside the project (a shared library); writing back affects every project that uses it\n"), dest)
		if !prompt.Confirm(i18n.T("Write back anyway?")) {
			fmt.Println(i18n.T("  Skipped."))
			return 1
		}
	}
	if err := writeBack(srcCopy, dest); err != nil {
		fmt.Printf(i18n.T("✘ failed to write back %s/%s: %v\n"), it.Category, it.Name, err)
		return 1
	}
	// For a skill renamed in the output, the front-matter name is the
	// build-rewritten, output-only name. Writing it back verbatim would leak a
	// name like python-style-fromlib-lib — valid only in the output — into the source (§12-2).
	if it.Renamed && it.Category == "skills" {
		origName := it.Original
		if err := build.RewriteSkillName(filepath.Join(dest, "SKILL.md"), origName); err != nil {
			fmt.Printf(i18n.T("⚠ %s/%s written back, but restoring the name in SKILL.md failed: %v\n"), it.Category, it.Name, err)
		}
	}
	fmt.Printf(i18n.T("✔ Wrote back %s/%s → %s\n"), it.Category, it.Name, dest)
	if toRaw != "" {
		// Redirecting only moved the content: the manifest's original source is
		// unchanged and the old source's file is still in place. The next build
		// will pick up a same-named item from both sources (rename yields two
		// copies / error blocks it outright).
		fmt.Printf(i18n.T("  ⚠ the old source still holds this item: %s\n"), it.From)
		fmt.Println(i18n.T("     The next build will pick up both old and new copies (same name); once the new source is verified, remove the old one manually."))
	}
	srcPath := ""
	if toRaw == "" {
		srcPath = dest // written back to its origin; refresh the source baseline
	}
	markPromoted(out, m, it, lc.OutPath, srcPath)
	return 0
}

// markPromoted syncs the written-back copy to the item's other bucket copies,
// then updates the output hash. Updating only OutPaths[0] is not enough: when a
// workflow fans out to multiple buckets, the modified copy may be the second one;
// status would then keep reporting the same local change forever, and next round
// it escalates to "the source has changed too", which makes promote refuse
// outright — the user would be stuck there.
func markPromoted(out string, m *build.Manifest, it build.ManifestItem, changedRel, srcPath string) {
	for i := range m.Items {
		if m.Items[i].Category != it.Category || m.Items[i].Name != it.Name {
			continue
		}
		if len(m.Items[i].OutPaths) == 0 {
			return
		}
		// Copies across buckets are supposed to hold identical content anyway
		// (the next build makes them so); while here, give the copies mounted
		// under other tools the same new content immediately
		src := filepath.Join(out, filepath.FromSlash(changedRel))
		for _, rel := range m.Items[i].OutPaths {
			if rel == changedRel {
				continue
			}
			if err := writeBack(src, filepath.Join(out, filepath.FromSlash(rel))); err != nil {
				fmt.Printf(i18n.T("⚠ failed to sync output copy %s: %v\n"), rel, err)
			}
		}
		p := filepath.Join(out, filepath.FromSlash(m.Items[i].OutPaths[0]))
		if h, files, err := build.HashPath(p); err == nil {
			m.Items[i].Hash, m.Items[i].Files = h, files
		}
		// The write-back just made the source identical to the accepted
		// content, so refresh the source baseline as well. Leaving the old
		// SrcHash would misflag the very next edit of this item as "the
		// source changed too" — a promote-refusing deadlock caused by
		// promote's own write. (srcPath is empty for --to redirects: the
		// original source was not touched there.)
		if srcPath != "" {
			if sh, sfiles, err := build.HashPath(srcPath); err == nil {
				m.Items[i].SrcHash, m.Items[i].SrcFiles = sh, sfiles
			}
		}
		return
	}
}

// writeBack: file → overwrite; directory → replace wholesale (the output copy is the desired state)
func writeBack(src, dest string) error {
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if !st.IsDir() {
		return copyFile2(src, dest)
	}
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		t := filepath.Join(dest, rel)
		if d.IsDir() {
			return os.MkdirAll(t, 0o755)
		}
		return copyFile2(p, t)
	})
}

// copyFile2 is the write-back copy that also preserves the source's permissions
// (otherwise promote would clobber the source's executable bits)
func copyFile2(src, dst string) error {
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, raw, st.Mode().Perm()); err != nil {
		return err
	}
	return os.Chmod(dst, st.Mode().Perm())
}
