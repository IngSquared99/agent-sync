package state

import (
	"os"
	"path/filepath"
	"testing"
)

// OS metadata files (.DS_Store etc.) inside the output must not be reported
// as untracked: they are never produced by build and never collected from
// sources, so nagging about them would make every macOS status noisy.
func TestHiddenFilesAreNotUntracked(t *testing.T) {
	cfg, m, _, _ := setup(t)
	out := cfg.OutDir()
	if err := os.WriteFile(filepath.Join(out, ".DS_Store"), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "rules", ".DS_Store"), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := collect(t, cfg, m)
	if len(r.Untracked) != 0 {
		t.Fatalf("hidden files reported as untracked: %v", r.Untracked)
	}
}
