package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IngSquared99/agent-sync/internal/prompt"
)

// The apply confirmation must cover untracked files added on the mount side:
// non-interactive without --yes cancels and leaves them in place; explicit
// consent rebuilds and clears them.
func TestApplyGuardsUntrackedFiles(t *testing.T) {
	proj := newProject(t)
	chdir(t, proj)
	prompt.AssumeYes = true
	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatal("init failed")
	}
	if code := cmdApply(); code != 0 {
		t.Fatal("apply failed")
	}
	prompt.AssumeYes = false

	newFile := filepath.Join(proj, ".agsy", "rules", "ai-new.md")
	write(t, newFile, "# 新規則\n")

	if code := cmdApply(); code == 0 {
		t.Error("apply must cancel when untracked files exist and nobody can confirm")
	}
	if _, err := os.Stat(newFile); err != nil {
		t.Fatal("a cancelled apply must not delete untracked files")
	}

	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()
	if code := cmdApply(); code != 0 {
		t.Fatal("apply --yes failed")
	}
	if _, err := os.Stat(newFile); err == nil {
		t.Error("after explicit consent the rebuild must clear untracked files")
	}
}

// When copies in multiple buckets diverge, promote must refuse: writing back
// either copy would clobber the other's edit.
func TestPromoteRefusesDivergedCopies(t *testing.T) {
	proj := newProject(t)
	chdir(t, proj)
	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()

	// Non-interactive init --yes selects every tool; release-note has no
	// target, so each bucket holds a copy.
	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatal("init failed")
	}
	if code := cmdApply(); code != 0 {
		t.Fatal("apply failed")
	}
	a := filepath.Join(proj, ".agsy", "workflows", "agents", "release-note.md")
	c := filepath.Join(proj, ".agsy", "workflows", "claude", "release-note.md")
	for _, p := range []string{a, c} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("expected copies missing, skipping: %v", err)
		}
	}
	write(t, a, "沒標 target\n改法 A\n")
	write(t, c, "沒標 target\n改法 B\n")

	if code := cmdPromote([]string{"workflows/release-note.md"}); code == 0 {
		t.Error("promote must refuse when copies diverge")
	}
	src, err := os.ReadFile(filepath.Join(proj, "repo-ai-lib", "workflows", "release-note.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "改法") {
		t.Error("a refused promote must not touch the source")
	}
}

// A missing source root must block promote: MkdirAll would otherwise recreate
// the whole tree at the wrong place.
func TestPromoteRefusesWhenSourceRootMissing(t *testing.T) {
	proj := newProject(t)
	chdir(t, proj)
	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()

	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatal("init failed")
	}
	if code := cmdApply(); code != 0 {
		t.Fatal("apply failed")
	}
	write(t, filepath.Join(proj, ".agsy", "rules", "python-style.md"), "# 風格\n## 產物端改的\n")
	if err := os.Rename(filepath.Join(proj, "repo-ai-lib"), filepath.Join(proj, "repo-ai-lib.bak")); err != nil {
		t.Fatal(err)
	}

	if code := cmdPromote([]string{"rules/python-style.md"}); code == 0 {
		t.Error("promote must refuse when the source root is missing")
	}
	if _, err := os.Stat(filepath.Join(proj, "repo-ai-lib")); err == nil {
		t.Error("promote must not recreate a missing source root")
	}
}

// A deleted source file with the root still present may be recreated; --yes
// counts as consent.
func TestPromoteRecreatesDeletedSourceFile(t *testing.T) {
	proj := newProject(t)
	chdir(t, proj)
	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()

	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatal("init failed")
	}
	if code := cmdApply(); code != 0 {
		t.Fatal("apply failed")
	}
	srcFile := filepath.Join(proj, "repo-ai-lib", "rules", "python-style.md")
	if err := os.Remove(srcFile); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(proj, ".agsy", "rules", "python-style.md"), "# 風格\n## 想留下的改動\n")

	if code := cmdPromote([]string{"rules/python-style.md"}); code != 0 {
		t.Fatal("promote should recreate the file when the root exists and --yes consents")
	}
	raw, err := os.ReadFile(srcFile)
	if err != nil {
		t.Fatal("write-back should recreate the source file")
	}
	if !strings.Contains(string(raw), "想留下的改動") {
		t.Error("recreated source file has wrong content")
	}
}
