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

// Changes to derived outputs (stub, derived skill, AGENTS.md) are reported in
// the discard list like any other artifact-side change; the rebuild replaces
// them after consent.
func TestApplyDiscardsDerivedEdits(t *testing.T) {
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

	stub := filepath.Join(proj, ".agsy", "workflows", "release-note.md")
	agents := filepath.Join(proj, "AGENTS.md")
	origStub, err := os.ReadFile(stub)
	if err != nil {
		t.Fatal(err)
	}
	write(t, stub, string(origStub)+"\nedited\n")

	if code := cmdApply(); code == 0 {
		t.Error("apply must cancel while artifact-side edits are unconfirmed")
	}
	raw, err := os.ReadFile(stub)
	if err != nil || !strings.Contains(string(raw), "edited") {
		t.Fatal("a cancelled apply must not touch the edited stub")
	}

	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()
	if code := cmdApply(); code != 0 {
		t.Fatal("apply --yes failed")
	}
	raw, err = os.ReadFile(stub)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "edited") {
		t.Error("the rebuild must regenerate the stub")
	}
	// the root AGENTS.md link reads the regenerated derived file
	if _, err := os.Stat(agents); err != nil {
		t.Fatalf("root AGENTS.md link missing after apply: %v", err)
	}
}

// The sources are never written: an artifact-side edit plus a rebuild must
// leave every source file byte-identical.
func TestApplyNeverWritesSources(t *testing.T) {
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
	before, err := os.ReadFile(srcFile)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(proj, ".agsy", "rules", "python-style.md"), "# style\n## artifact-side edit\n")
	if code := cmdApply(); code != 0 {
		t.Fatal("apply failed")
	}
	after, err := os.ReadFile(srcFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("apply modified a source file; the data flow must be one-way")
	}
}

// setupApplied prepares a project, runs init --yes + apply, and returns its root.
func setupApplied(t *testing.T) string {
	t.Helper()
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
	return proj
}
