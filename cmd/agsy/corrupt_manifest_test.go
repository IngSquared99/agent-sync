package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IngSquared99/agent-sync/internal/prompt"
	"github.com/IngSquared99/agent-sync/internal/state"
)

// The mounts record lives inside the AI-writable output. When the manifest is
// corrupted, orphan links from a removed mount entry must still be found via
// the built-in adapter sweep instead of silently surviving unreported.
func TestOrphanSweepSurvivesCorruptManifest(t *testing.T) {
	proj := setupApplied(t)
	if _, err := os.Lstat(filepath.Join(proj, ".claude", "rules")); err != nil {
		t.Skipf("no usable directory link on this platform: %v", err)
	}

	// Drop the .claude mount from the config (same trimming as the orphan test).
	raw, err := os.ReadFile(filepath.Join(proj, "agsy.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var kept []string
	skip := false
	for _, ln := range strings.Split(string(raw), "\n") {
		if strings.Contains(ln, "- dir: .claude") {
			skip = true
			continue
		}
		if skip {
			if strings.HasPrefix(ln, "  - dir:") {
				skip = false
			} else {
				continue
			}
		}
		kept = append(kept, ln)
	}
	write(t, filepath.Join(proj, "agsy.yaml"), strings.Join(kept, "\n"))

	// Corrupt the manifest: the recorded mounts are gone with it.
	write(t, filepath.Join(proj, ".agsy", ".agsy-manifest.json"), "{ not json")

	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()
	if code := cmdApply(); code != 0 {
		t.Fatal("apply over a corrupt manifest failed")
	}

	// The sweep must have re-recorded the orphaned .claude links.
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	m, err := loadManifestForTest(cfg.OutDir())
	if err != nil {
		t.Fatal(err)
	}
	rep, err := state.Collect(cfg, m)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, o := range rep.Orphans {
		if strings.Contains(o, ".claude") {
			found = true
		}
	}
	if !found {
		t.Fatalf("orphaned .claude links must survive a corrupt manifest, got %v", rep.Orphans)
	}
}
