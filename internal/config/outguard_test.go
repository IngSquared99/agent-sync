package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// build.out nested inside a source is the mirror image of "out contains a
// source" — apply empties out, so originals living around it would be wiped.
// This was the one remaining data-loss path; both directions must be rejected.
func TestOutInsideSourceRejected(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "a-lib", "rule"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfgRaw := `version: 1
sources: [./a-lib]
build:
  out: ./a-lib/rule
  on_conflict: {rules: rename, skills: error, workflows: rename}
  route: {field: target, default: [claude], buckets: [claude]}
mount:
  - dir: .claude
    links: {rules: rules}
`
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte(cfgRaw), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("out inside a source must be rejected at config validation")
	}
	if !strings.Contains(err.Error(), "a-lib") {
		t.Errorf("error should name the offending source, got: %v", err)
	}
}
