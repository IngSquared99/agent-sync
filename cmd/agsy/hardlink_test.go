package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/IngSquared99/agent-sync/internal/prompt"
)

// With a hard-linked root AGENTS.md (the Windows file mount, reproducible
// on any platform), apply must remove the verified file link before the
// rebuild (mount.ClearFileLinks) so the mount step can recreate it against
// the fresh artifact instead of refusing a stranded "real file".
func TestApplyAgainWithHardLinkedAgentsMD(t *testing.T) {
	proj := setupApplied(t)
	root := filepath.Join(proj, "AGENTS.md")
	artifact := filepath.Join(proj, ".agsy", "AGENTS.md")

	// Simulate the Windows file mount: replace the symlink with a hard link.
	if err := os.Remove(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(artifact, root); err != nil {
		t.Skipf("hard links unsupported here: %v", err)
	}

	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()
	if code := cmdApply(); code != 0 {
		t.Fatalf("apply over a hard-linked AGENTS.md exit code = %d", code)
	}

	// The mount must be re-established against the fresh artifact.
	a, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.Stat(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(a, b) {
		t.Error("root AGENTS.md no longer matches the rebuilt artifact")
	}
}
