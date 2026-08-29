package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IngSquared99/agent-sync/internal/prompt"
)

// Edit-mode regeneration must not silently drop links the user added by hand
// under the project root (dir "."); only the AGENTS.md entry itself is
// regenerated.
func TestInitKeepsCustomRootLinks(t *testing.T) {
	proj := newProject(t)
	chdir(t, proj)
	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()
	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatalf("init exit code = %d", code)
	}

	// Add a custom link under the root mount by hand.
	path := filepath.Join(proj, "agsy.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(raw),
		"      AGENTS.md: AGENTS.md\n",
		"      AGENTS.md: AGENTS.md\n      CLAUDE.md:  AGENTS.md\n", 1)
	if edited == string(raw) {
		t.Fatal("failed to inject the custom root link into agsy.yaml")
	}
	write(t, path, edited)

	// Re-run init in edit mode; the regenerated file must keep the link.
	if code := cmdInit(nil); code != 0 {
		t.Fatalf("edit-mode init exit code = %d", code)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "CLAUDE.md:") {
		t.Error("edit-mode init dropped the custom link under dir \".\"")
	}
}
