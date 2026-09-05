package config

import (
	"os"
	"path/filepath"
	"testing"
)

// A mount entry without dir must be rejected regardless of its position in
// the list: merging it into the "- dir: ." entry would silently re-anchor
// its links at the project root.
func TestMountMissingDirRejectedAfterRootEntry(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `version: 1
sources:
  - ./lib
build:
  out: .agsy
  on_conflict:
    rules: rename
    skills: error
    workflows: rename
    hooks: error
  tools: [claude]
mount:
  - dir: .
    links:
      AGENTS.md: AGENTS.md
  - links:
      rules: rules
      skills: skills
`
	p := filepath.Join(dir, FileName)
	if err := os.WriteFile(p, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("expected validation error for the mount entry without dir, got nil")
	}
}
