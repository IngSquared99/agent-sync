package mount

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IngSquared99/agent-sync/i18n"
	"github.com/IngSquared99/agent-sync/internal/config"
)

const mountYAML = `version: 1
sources:
  - ./.flow
build:
  out: .agsy
  on_conflict: {rules: rename, skills: error, workflows: rename}
  tools: [claude, antigravity]
mount:
  - dir: .claude
    links: {rules: rules, commands: workflows}
`

// setup builds a project skeleton that has already been built (output
// directory and mount targets in place).
func setup(t *testing.T) *config.Config {
	t.Helper()
	proj := t.TempDir()
	p := filepath.Join(proj, config.FileName)
	if err := os.WriteFile(p, []byte(mountYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"rules", filepath.Join("workflows", "claude")} {
		if err := os.MkdirAll(filepath.Join(cfg.OutDir(), sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return cfg
}

func stateOf(t *testing.T, cfg *config.Config, name string) LinkPlan {
	t.Helper()
	plans, err := Inspect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range plans {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("link %s not found", name)
	return LinkPlan{}
}

func TestInspectAndApply(t *testing.T) {
	cfg := setup(t)

	// 1) nothing exists at first
	if got := stateOf(t, cfg, "rules").State; got != Missing {
		t.Errorf("initial state = %v, want Missing", got)
	}

	// 2) after apply it becomes a link, and a relative one (not tied to this machine)
	plans, _ := Inspect(cfg)
	if err := Apply(cfg, plans); err != nil {
		t.Fatal(err)
	}
	lp := stateOf(t, cfg, "rules")
	if lp.State != IsLink {
		t.Fatalf("after apply = %v, want IsLink", lp.State)
	}
	if target, err := os.Readlink(lp.LinkPath); err == nil && filepath.IsAbs(target) {
		t.Errorf("non-Windows should create a relative-path symlink, got %q", target)
	}

	// 3) rerun is idempotent: links are always deleted and recreated, no error
	plans, _ = Inspect(cfg)
	if err := Apply(cfg, plans); err != nil {
		t.Fatalf("rerunning apply should be idempotent: %v", err)
	}

	// 4) an old link pointing elsewhere → IsStale, recreating fixes it
	os.Remove(lp.LinkPath)
	if err := os.Symlink(filepath.Join(cfg.BaseDir, "elsewhere"), lp.LinkPath); err != nil {
		t.Fatal(err)
	}
	if got := stateOf(t, cfg, "rules").State; got != IsStale {
		t.Errorf("mispointed link = %v, want IsStale", got)
	}
	plans, _ = Inspect(cfg)
	if err := Apply(cfg, plans); err != nil {
		t.Fatal(err)
	}
	if got := stateOf(t, cfg, "rules").State; got != IsLink {
		t.Errorf("after recreation = %v, want it repaired to IsLink", got)
	}
}

// Bottom line: the tool never deletes anything it did not create itself.
func TestApplyRefusesRealDirectory(t *testing.T) {
	i18n.SetLang("en")
	cfg := setup(t)
	real := filepath.Join(cfg.BaseDir, ".claude", "rules")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	precious := filepath.Join(real, "precious.md")
	if err := os.WriteFile(precious, []byte("six months of handwritten notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := stateOf(t, cfg, "rules").State; got != IsReal {
		t.Fatalf("real directory = %v, want IsReal", got)
	}
	plans, _ := Inspect(cfg)
	err := Apply(cfg, plans)
	if err == nil {
		t.Fatal("a real directory must raise an error, never be deleted on the user's behalf")
	}
	if !strings.Contains(err.Error(), "refusing to delete") {
		t.Errorf("error message = %v", err)
	}
	if _, statErr := os.Stat(precious); statErr != nil {
		t.Fatal("the user's file was deleted")
	}
}

func TestRemoveLinks(t *testing.T) {
	cfg := setup(t)
	plans, _ := Inspect(cfg)
	if err := Apply(cfg, plans); err != nil {
		t.Fatal(err)
	}
	// Mix in a real file: clean must skip it, not delete it.
	realFile := filepath.Join(cfg.BaseDir, ".claude", "mine.md")
	if err := os.WriteFile(realFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, skipped, err := RemoveLinks(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 {
		t.Errorf("should remove 2 links, got %v", removed)
	}
	if len(skipped) != 0 {
		t.Errorf("there should be no skipped entries: %v", skipped)
	}
	if _, err := os.Stat(realFile); err != nil {
		t.Error("a real file inside the mount directory must not be deleted")
	}
}

// doctor's "check link capability": actually create one and delete it,
// rather than just printing a claim.
func TestProbe(t *testing.T) {
	dir := t.TempDir()
	if err := Probe(dir); err != nil {
		t.Fatalf("this machine should be able to create directory links: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("Probe must clean up everything it created, leftovers %v", entries)
	}
}

// The root AGENTS.md mount is a single-file link: create, idempotent
// recreate, and stale detection must all work like directory links.
func TestFileLink(t *testing.T) {
	cfg := setup(t)
	out := cfg.OutDir()
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, config.AgentsMD), []byte("# rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg.Mount = append(cfg.Mount, config.MountCfg{Dir: ".", Links: map[string]string{config.AgentsMD: config.AgentsMD}})

	plans, err := Inspect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var fp *LinkPlan
	for i := range plans {
		if plans[i].FileTgt {
			fp = &plans[i]
		}
	}
	if fp == nil || fp.State != Missing {
		t.Fatalf("file link plan = %+v", fp)
	}
	if err := Apply(cfg, plans); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(cfg.BaseDir, config.AgentsMD)
	raw, err := os.ReadFile(linked)
	if err != nil || string(raw) != "# rules\n" {
		t.Fatalf("reading through the file link: %q %v", raw, err)
	}
	// Idempotent: a second apply recreates the link without error.
	plans, err = Inspect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range plans {
		if p.FileTgt && p.State != IsLink {
			t.Errorf("existing file link should be IsLink, got %v", p.State)
		}
	}
	if err := Apply(cfg, plans); err != nil {
		t.Fatal(err)
	}
	// A managed file link is claimed by IsManagedLink; a real file is not.
	if !IsManagedLink(linked, out) {
		t.Error("file link into the output must be recognized as managed")
	}
	if err := os.Remove(linked); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(linked, []byte("user content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plans, err = Inspect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range plans {
		if p.FileTgt && p.State != IsReal {
			t.Errorf("a real AGENTS.md must be IsReal (never deleted), got %v", p.State)
		}
	}
	if err := Apply(cfg, plans); err == nil {
		t.Error("Apply must refuse while a real AGENTS.md occupies the mount point")
	}
}
