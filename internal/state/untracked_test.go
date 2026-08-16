package state

import (
	"os"
	"path/filepath"
	"testing"
)

// Output files the manifest does not track must be reported; otherwise status
// claims "in sync" right before apply deletes them.
func TestUntrackedOutputIsReported(t *testing.T) {
	cfg, m, _, _ := setup(t)
	write(t, filepath.Join(cfg.OutDir(), "rules", "ai-new.md"), "# 新規則\n")
	// A new file inside a directory item (skill) is a local change of that
	// item, not untracked.
	write(t, filepath.Join(cfg.OutDir(), "skills", "api-doc", "extra.md"), "多的\n")

	r := collect(t, cfg, m)
	if len(r.Untracked) != 1 || r.Untracked[0] != "rules/ai-new.md" {
		t.Errorf("Untracked = %v, want [rules/ai-new.md]", r.Untracked)
	}
	if !r.HasGap {
		t.Error("untracked files must count as a gap")
	}
	found := false
	for _, lc := range r.Locals {
		if filepath.Base(lc.Item.From) == "api-doc" {
			found = true
		}
	}
	if !found {
		t.Error("a new file inside a skill directory must be a local change of that item")
	}
}

// A deleted source file and a missing source root are marked separately on
// the local change, so promote can recreate the former and refuse the latter.
func TestLocalChangeMarksSourceGone(t *testing.T) {
	cfg, m, lib, _ := setup(t)
	write(t, filepath.Join(cfg.OutDir(), "rules", "security.md"), "# 安全\n## 產物端改的\n")

	if err := os.Remove(filepath.Join(lib, "rule", "security.md")); err != nil {
		t.Fatal(err)
	}
	r := collect(t, cfg, m)
	lc := findLocal(t, r, "security.md")
	if !lc.SrcDeleted || lc.SrcRootGone {
		t.Errorf("deleted file: SrcDeleted=%v SrcRootGone=%v, want true/false", lc.SrcDeleted, lc.SrcRootGone)
	}

	if err := os.RemoveAll(lib); err != nil {
		t.Fatal(err)
	}
	r = collect(t, cfg, m)
	lc = findLocal(t, r, "security.md")
	if !lc.SrcRootGone {
		t.Error("a missing source root must set SrcRootGone")
	}
}

func findLocal(t *testing.T, r *Report, name string) LocalChange {
	t.Helper()
	for _, lc := range r.Locals {
		if lc.Item.Name == name {
			return lc
		}
	}
	t.Fatalf("no local change named %s: %+v", name, r.Locals)
	return LocalChange{}
}
