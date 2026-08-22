package state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/IngSquared99/agent-sync/internal/build"
	"github.com/IngSquared99/agent-sync/internal/config"
	"github.com/IngSquared99/agent-sync/internal/mount"
)

const stateYAML = `version: 1
sources:
  - ../lib
  - ./.flow
build:
  out: .agsy
  on_conflict: {rules: rename, skills: error, workflows: rename}
  route: {field: target, default: [agents, claude], buckets: [agents, claude]}
mount:
  - dir: .claude
    links: {rules: rules, skills: skills, commands: workflows/claude}
`

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// setup creates a project plus two sources, runs one full build, and returns
// the config and manifest.
func setup(t *testing.T) (*config.Config, *build.Manifest, string, string) {
	t.Helper()
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	lib := filepath.Join(root, "lib")
	flow := filepath.Join(proj, ".flow")
	write(t, filepath.Join(proj, config.FileName), stateYAML)
	write(t, filepath.Join(lib, "rules", "security.md"), "# 安全\n")
	write(t, filepath.Join(lib, "workflows", "release-note.md"), "沒標 target\n")
	write(t, filepath.Join(flow, "skills", "api-doc", "SKILL.md"), "---\nname: api-doc\n---\n內文\n")
	write(t, filepath.Join(flow, "skills", "api-doc", "ref.md"), "參考\n")

	cfg, err := config.Load(filepath.Join(proj, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	sources, err := build.ExpandSources(cfg)
	if err != nil {
		t.Fatal(err)
	}
	p, err := build.Compute(cfg, sources)
	if err != nil {
		t.Fatal(err)
	}
	m, err := build.Execute(cfg, p)
	if err != nil {
		t.Fatal(err)
	}
	plans, err := mount.Inspect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := mount.Apply(cfg, plans); err != nil {
		t.Fatal(err)
	}
	return cfg, m, lib, flow
}

func collect(t *testing.T, cfg *config.Config, m *build.Manifest) *Report {
	t.Helper()
	r, err := Collect(cfg, m)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCleanStateHasNoGap(t *testing.T) {
	cfg, m, _, _ := setup(t)
	r := collect(t, cfg, m)
	if r.HasGap {
		t.Errorf("right after build everything should be in sync: lags %d news %d locals %d bad links %d",
			len(r.Lags), len(r.News), len(r.Locals), r.LinkBad)
	}
}

// Exercise each of the four gap directions once.
func TestFourKindsOfGap(t *testing.T) {
	cfg, m, lib, _ := setup(t)

	// 1. Lag: source content changed
	write(t, filepath.Join(lib, "rules", "security.md"), "# 安全\n- 新增一條\n")
	// 2. New: source gained a file
	write(t, filepath.Join(lib, "rules", "testing.md"), "# 測試\n")
	// 3. Local change: output modified through the mount (simulates an AI editing it)
	skillMD := filepath.Join(cfg.BaseDir, ".claude", "skills", "api-doc", "SKILL.md")
	write(t, skillMD, "---\nname: api-doc\n---\n內文\n## AI 加寫的章節\n")
	// 4. Bad mount: link removed
	if err := os.Remove(filepath.Join(cfg.BaseDir, ".claude", "rules")); err != nil {
		t.Fatal(err)
	}

	r := collect(t, cfg, m)
	if len(r.Lags) != 1 || r.Lags[0].Kind != SrcChanged {
		t.Errorf("lags = %+v", r.Lags)
	}
	if len(r.News) != 1 || r.News[0].Name != "testing.md" {
		t.Errorf("news = %+v", r.News)
	}
	if len(r.Locals) != 1 {
		t.Fatalf("local changes = %+v", r.Locals)
	}
	// Directory items must be able to say which files changed, not just "content changed".
	if len(r.Locals[0].Files) != 1 || r.Locals[0].Files[0] != "SKILL.md" {
		t.Errorf("changed file list = %v, want [SKILL.md]", r.Locals[0].Files)
	}
	// Lineage tracking: a file edited under .claude/ must be attributed back to its home in .flow.
	if filepath.Base(r.Locals[0].Item.From) != "api-doc" {
		t.Errorf("original source = %s", r.Locals[0].Item.From)
	}
	if r.LinkBad != 1 {
		t.Errorf("bad links = %d", r.LinkBad)
	}
}

// A single deleted file vs a whole missing source root are two different things.
func TestSourceDeletedVsRootMissing(t *testing.T) {
	cfg, m, lib, _ := setup(t)

	// First delete just one file.
	if err := os.Remove(filepath.Join(lib, "rules", "security.md")); err != nil {
		t.Fatal(err)
	}
	r := collect(t, cfg, m)
	found := false
	for _, l := range r.Lags {
		if l.Item.Name == "security.md" {
			found = true
			if l.Kind != SrcDeleted {
				t.Errorf("a single deleted file should be SrcDeleted, got %v", l.Kind)
			}
		}
	}
	if !found {
		t.Fatal("deleted source file was not detected")
	}
	if len(r.MissingSources) != 0 {
		t.Errorf("source root still exists and should not be listed as missing: %v", r.MissingSources)
	}

	// Now remove the whole source directory.
	if err := os.RemoveAll(lib); err != nil {
		t.Fatal(err)
	}
	r = collect(t, cfg, m)
	if len(r.MissingSources) != 1 {
		t.Fatalf("a fully missing source must be listed separately: %v", r.MissingSources)
	}
	for _, l := range r.Lags {
		if l.Item.From != "" && filepath.Base(l.Item.From) == "security.md" && l.Kind != SrcRootMissing {
			t.Errorf("when the whole source root is missing the kind should be SrcRootMissing, got %v", l.Kind)
		}
	}
}

// With an empty route.default, a workflow without a target reaches neither the
// outputs nor the manifest — but it is not "new": otherwise status would report
// the same thing every time, and apply could never clear it.
func TestEmptyDefaultWorkflowIsNotNew(t *testing.T) {
	cfg, m, _, _ := setup(t)
	cfg.Build.Route.Default = nil
	// Rebuild so the manifest reflects "placed in no bucket".
	sources, _ := build.ExpandSources(cfg)
	p, err := build.Compute(cfg, sources)
	if err != nil {
		t.Fatal(err)
	}
	m, err = build.Execute(cfg, p)
	if err != nil {
		t.Fatal(err)
	}
	r := collect(t, cfg, m)
	for _, n := range r.News {
		if n.Name == "release-note.md" {
			t.Error("a workflow with empty default should not be repeatedly reported as new")
		}
	}
}
