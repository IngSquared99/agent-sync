package config

import (
	"strings"
	"testing"

	"github.com/IngSquared99/agent-sync/i18n"
)

// Mount dirs are merged by the real directory they name, not by spelling:
// ".claude" and "./.claude" put links in the same place, so a same-name link
// with different targets across the two spellings must surface as a
// validation error instead of silently becoming last-writer-wins at mount
// time.
func TestMergeMountsBySpellingVariants(t *testing.T) {
	i18n.SetLang("en")
	yaml := `version: 1
sources:
  - ./src
build:
  out: .agsy
  on_conflict: {rules: rename, skills: error, workflows: rename, hooks: error}
  tools: [claude, codex]
mount:
  - dir: .claude
    links: {skills: skills}
  - dir: ./.claude
    links: {skills: rules}
`
	mustFail(t, yaml, "more than once with different targets")
}

// The same two spellings with agreeing links are one mount entry after
// merging — the adapter-sharing behavior, independent of how dir is written.
func TestMergeMountsSpellingVariantsAgree(t *testing.T) {
	yaml := `version: 1
sources:
  - ./src
build:
  out: .agsy
  on_conflict: {rules: rename, skills: error, workflows: rename, hooks: error}
  tools: [claude, codex]
mount:
  - dir: .claude
    links: {skills: skills}
  - dir: ./.claude
    links: {rules: rules}
`
	cfg, err := load(t, yaml)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Mount) != 1 {
		t.Fatalf("spelling variants of one dir must merge into one entry, got %d: %+v", len(cfg.Mount), cfg.Mount)
	}
	m := cfg.Mount[0]
	if m.Links["skills"] != "skills" || m.Links["rules"] != "rules" {
		t.Errorf("merged links = %v", m.Links)
	}
	// First-seen spelling is kept for display.
	if m.Dir != ".claude" {
		t.Errorf("merged dir spelling = %q, want first-seen %q", m.Dir, ".claude")
	}
	for k := range m.Links {
		if strings.Contains(k, "\x00") {
			t.Errorf("agreeing links must not be marked as duplicates: %q", k)
		}
	}
}
