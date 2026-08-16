package state

import (
	"os"
	"path/filepath"
	"testing"
)

// A deleted output copy must be reported (GoneOut).
// Before the fix, a failed Stat was silently skipped: the mount side was
// actually missing a file, yet status reported "in sync" (exit 0), so CI
// waved through a broken toolchain — that is why this test exists.
func TestGoneOutIsReported(t *testing.T) {
	cfg, m, _, _ := setup(t)
	// Simulate the user (or an external force) deleting an output file via the mount.
	if err := os.Remove(filepath.Join(cfg.OutDir(), "rules", "security.md")); err != nil {
		t.Fatal(err)
	}
	r := collect(t, cfg, m)
	if len(r.Gone) != 1 {
		t.Fatalf("gone outputs = %+v, want 1 entry", r.Gone)
	}
	if r.Gone[0].Item.Name != "security.md" {
		t.Errorf("gone item = %s, want security.md", r.Gone[0].Item.Name)
	}
	if len(r.Gone[0].OutPaths) != 1 || r.Gone[0].OutPaths[0] != "rules/security.md" {
		t.Errorf("gone copy paths = %v, want [rules/security.md]", r.Gone[0].OutPaths)
	}
	if !r.HasGap {
		t.Error("missing outputs must count toward HasGap, otherwise the status exit code lies")
	}
	// A missing copy is not a local change (there is no content to write back); never conflate the two.
	if len(r.Locals) != 0 {
		t.Errorf("missing copy was misreported as a local change: %+v", r.Locals)
	}
}
