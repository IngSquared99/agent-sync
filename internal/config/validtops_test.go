package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The valid-values hint in a link-target error must list each target once,
// even when two categories share the same "to" (reported by its own error).
func TestMountValidTargetsDeduped(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `version: 1
sources:
  - ./lib
build:
  out: .agsy
  categories:
    rules:     { from: rules, to: shared }
    skills:    { from: skills, to: shared }
    workflows: { from: workflows, to: workflows }
  on_conflict:
    rules: rename
    skills: error
    workflows: rename
  tools: [claude, antigravity]
mount:
  - dir: .claude
    links:
      nope: nothere
  - dir: .agents
    links:
      workflows: workflows
`
	p := filepath.Join(dir, FileName)
	if err := os.WriteFile(p, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("expected validation error, got nil")
	} else {
		for _, line := range strings.Split(err.Error(), "\n") {
			if strings.Contains(line, "nothere") && strings.Contains(line, "shared shared") {
				t.Fatalf("valid-targets hint not deduplicated: %s", line)
			}
		}
	}
}
