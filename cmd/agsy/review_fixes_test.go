package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IngSquared99/agent-sync/internal/prompt"
)

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

// Bare `agsy promote` (interactive menu) must obey the same non-interactive
// gate as every other confirming command: without --yes it cancels instead of
// treating EOF as "select everything" and silently writing sources back.
func TestBarePromoteNonInteractiveCancels(t *testing.T) {
	proj := setupApplied(t)

	write(t, filepath.Join(proj, ".agsy", "rules", "python-style.md"), "# 風格\n## AI 加寫\n")
	before, err := os.ReadFile(filepath.Join(proj, "repo-ai-lib", "rule", "python-style.md"))
	if err != nil {
		t.Fatal(err)
	}

	if code := cmdPromote(nil); code == 0 {
		t.Fatal("bare promote must cancel in a non-interactive environment without --yes")
	}
	after, err := os.ReadFile(filepath.Join(proj, "repo-ai-lib", "rule", "python-style.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("a cancelled promote wrote the source back anyway")
	}
}

// After a successful write-back the source baseline must be refreshed:
// otherwise the very next edit of the same item is misflagged as "the source
// changed too" (by promote's own write) and both promote and apply block.
func TestPromoteThenEditAgainDoesNotDeadlock(t *testing.T) {
	proj := setupApplied(t)
	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()

	write(t, filepath.Join(proj, ".agsy", "rules", "python-style.md"), "# 風格\n## 第一次\n")
	if code := cmdPromote([]string{"rules/python-style.md"}); code != 0 {
		t.Fatal("first promote failed")
	}

	write(t, filepath.Join(proj, ".agsy", "rules", "python-style.md"), "# 風格\n## 第一次\n## 第二次\n")
	if code := cmdPromote([]string{"rules/python-style.md"}); code != 0 {
		t.Fatal("second promote must succeed — the only source change was promote's own write")
	}
	src, err := os.ReadFile(filepath.Join(proj, "repo-ai-lib", "rule", "python-style.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "第二次") {
		t.Fatal("second write-back did not reach the source")
	}
}

// Copies edited to IDENTICAL contents across buckets are one change, not a
// conflict: refusing them (with a message claiming they differ) forced manual
// work in the common "both tools mount the same workflow" case.
func TestPromoteAcceptsIdenticalMultiCopies(t *testing.T) {
	proj := setupApplied(t)
	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()

	// release-note.md has no target → one copy per bucket. Edit two copies
	// to the same new content.
	newBody := "沒標 target\n## 兩邊一致的新段落\n"
	copies, err := filepath.Glob(filepath.Join(proj, ".agsy", "workflows", "*", "release-note.md"))
	if err != nil || len(copies) < 2 {
		t.Fatalf("expected multiple bucket copies, got %v", copies)
	}
	write(t, copies[0], newBody)
	write(t, copies[1], newBody)

	if code := cmdPromote([]string{"workflows/release-note.md"}); code != 0 {
		t.Fatal("identical multi-copy edits must be treated as a single change")
	}
	src, err := os.ReadFile(filepath.Join(proj, "repo-ai-lib", "workflow", "release-note.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "兩邊一致的新段落") {
		t.Fatal("write-back did not reach the source")
	}
	// Every copy (including untouched buckets) must hold the same content now.
	for _, c := range copies {
		raw, err := os.ReadFile(c)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "兩邊一致的新段落") {
			t.Errorf("copy %s was not synced after write-back", c)
		}
	}
}
