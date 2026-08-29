package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IngSquared99/agent-sync/internal/prompt"
	"github.com/IngSquared99/agent-sync/internal/state"
)

// Removing a mount entry from agsy.yaml must not leave its links invisible:
// status reports them as orphans (mount anomalies), and clean removes them.
// Links are still never deleted by apply itself.
func TestOrphanLinksAreReportedAndCleaned(t *testing.T) {
	proj := setupApplied(t)
	if _, err := os.Lstat(filepath.Join(proj, ".claude", "rules")); err != nil {
		t.Skipf("no usable directory link on this platform: %v", err)
	}

	// Drop the .claude mount from the config and re-apply.
	raw, err := os.ReadFile(filepath.Join(proj, "agsy.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	var kept []string
	skip := false
	for _, ln := range lines {
		if strings.Contains(ln, "- dir: .claude") {
			skip = true
			continue
		}
		if skip {
			// skip the links block of the removed entry until the next mount entry
			if strings.HasPrefix(ln, "  - dir:") {
				skip = false
			} else {
				continue
			}
		}
		kept = append(kept, ln)
	}
	write(t, filepath.Join(proj, "agsy.yaml"), strings.Join(kept, "\n"))

	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()
	if code := cmdApply(); code != 0 {
		t.Fatal("apply after removing a mount entry failed")
	}

	// The old links still exist (apply never deletes) …
	if _, err := os.Lstat(filepath.Join(proj, ".claude", "rules")); err != nil {
		t.Fatal("apply must not delete links on its own")
	}
	// … but they are reported as orphans / mount anomalies.
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
	if len(rep.Orphans) == 0 || rep.LinkBad == 0 || !rep.HasGap {
		t.Fatalf("orphaned links must be reported: orphans=%v linkBad=%d", rep.Orphans, rep.LinkBad)
	}

	// clean removes them together with everything else agsy built.
	if code := cmdClean(); code != 0 {
		t.Fatal("clean failed")
	}
	if _, err := os.Lstat(filepath.Join(proj, ".claude", "rules")); err == nil {
		t.Fatal("clean should remove orphaned links recorded in the manifest")
	}
}
