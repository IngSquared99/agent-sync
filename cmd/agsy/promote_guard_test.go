package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IngSquared99/agent-sync/internal/build"
	"github.com/IngSquared99/agent-sync/internal/prompt"
)

// The manifest lives in the build output — the one layer mounted AI tools can
// write to — so promote must never trust its "from" paths blindly. A tampered
// manifest pointing outside the configured sources could otherwise turn
// promote into an arbitrary-path overwrite (directories are removed first).
func TestPromoteRefusesDestinationOutsideSources(t *testing.T) {
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

	// Create a local change so the item shows up in the promote list.
	outCopy := filepath.Join(proj, ".agsy", "rules", "python-style.md")
	write(t, outCopy, "# 風格\n## AI 加寫\n")

	// Tamper with the manifest: redirect the item's origin to a path outside
	// every configured source (simulating a malicious or corrupted manifest).
	victim := filepath.Join(t.TempDir(), "victim.md")
	mPath := filepath.Join(proj, ".agsy", build.ManifestName)
	raw, err := os.ReadFile(mPath)
	if err != nil {
		t.Fatal(err)
	}
	var m build.Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for i := range m.Items {
		if strings.HasSuffix(m.Items[i].From, "python-style.md") {
			m.Items[i].From = victim
		}
	}
	tampered, _ := json.Marshal(m)
	if err := os.WriteFile(mPath, tampered, 0o644); err != nil {
		t.Fatal(err)
	}

	if code := cmdPromote([]string{"rules/python-style.md"}); code == 0 {
		t.Fatal("promote should refuse a destination outside the configured sources")
	}
	if _, err := os.Stat(victim); err == nil {
		t.Fatal("promote wrote to the tampered destination")
	}
}

// --to must only accept sources listed in agsy.yaml: anything else silently
// moves content out of agsy's management and would bypass the destination guard.
func TestPromoteToRefusesNonSourcePath(t *testing.T) {
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
	write(t, filepath.Join(proj, ".agsy", "rules", "python-style.md"), "# 風格\n## AI 加寫\n")

	elsewhere := t.TempDir()
	if code := cmdPromote([]string{"rules/python-style.md", "--to", elsewhere}); code == 0 {
		t.Fatal("promote --to should refuse a path that is not a configured source")
	}
	if _, err := os.Stat(filepath.Join(elsewhere, "rule", "python-style.md")); err == nil {
		t.Fatal("promote --to wrote outside the configured sources")
	}

	// The same item written back to its real source still works.
	if code := cmdPromote([]string{"rules/python-style.md"}); code != 0 {
		t.Fatal("promote to the legitimate source should still succeed")
	}
	src, err := os.ReadFile(filepath.Join(proj, "repo-ai-lib", "rule", "python-style.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "AI 加寫") {
		t.Fatal("legitimate write-back did not reach the source")
	}
}
