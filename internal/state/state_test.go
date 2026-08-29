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
  tools: [claude, codex, antigravity, cursor]
mount:
  - dir: .
    links: {AGENTS.md: AGENTS.md}
  - dir: .claude
    links: {rules: rules, skills: skills}
  - dir: .agents
    links: {skills: skills, workflows: workflows}
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
	write(t, filepath.Join(lib, "rules", "security.md"), "# security\n")
	write(t, filepath.Join(lib, "workflows", "release-note.md"), "# release note\nsteps\n")
	write(t, filepath.Join(flow, "skills", "api-doc", "SKILL.md"), "---\nname: api-doc\ndescription: api docs\n---\nbody\n")
	write(t, filepath.Join(flow, "skills", "api-doc", "ref.md"), "reference\n")

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
		t.Errorf("right after build everything should be in sync: src %d news %d artifacts %+v bad links %d",
			len(r.SourceChanges), len(r.News), r.Artifacts, r.LinkBad)
	}
}

// Exercise each gap direction once.
func TestGapDirections(t *testing.T) {
	cfg, m, lib, _ := setup(t)

	// 1. list A: source content changed
	write(t, filepath.Join(lib, "rules", "security.md"), "# security\n- one more\n")
	// 2. list A: source gained a file
	write(t, filepath.Join(lib, "rules", "testing.md"), "# testing\n")
	// 3. list B: output modified through the mount (simulates an AI editing it)
	skillMD := filepath.Join(cfg.BaseDir, ".claude", "skills", "api-doc", "SKILL.md")
	write(t, skillMD, "---\nname: api-doc\ndescription: api docs\n---\nbody\n## added by AI\n")
	// 4. Bad mount: link removed
	if err := os.Remove(filepath.Join(cfg.BaseDir, ".claude", "rules")); err != nil {
		t.Fatal(err)
	}

	r := collect(t, cfg, m)
	if len(r.SourceChanges) != 1 || r.SourceChanges[0].Kind != SrcChanged {
		t.Errorf("source changes = %+v", r.SourceChanges)
	}
	if len(r.News) != 1 || r.News[0].Name != "testing.md" {
		t.Errorf("news = %+v", r.News)
	}
	if len(r.Artifacts) != 1 {
		t.Fatalf("artifact changes = %+v", r.Artifacts)
	}
	// Directory items must be able to say which files changed, not just "content changed".
	a := r.Artifacts[0]
	if len(a.Files) != 1 || a.Files[0] != "SKILL.md" {
		t.Errorf("changed file list = %v, want [SKILL.md]", a.Files)
	}
	// Lineage: a file edited under .claude/ must be attributed back to its
	// home in .flow so the guidance names the right source.
	if a.Item == nil || filepath.Base(a.Item.From) != "api-doc" {
		t.Errorf("original source = %+v", a.Item)
	}
	if a.Derived || a.Untracked {
		t.Errorf("a verbatim skill copy is neither derived nor untracked: %+v", a)
	}
	if r.LinkBad != 1 {
		t.Errorf("bad links = %d", r.LinkBad)
	}
}

// Changes to derived outputs (AGENTS.md, workflow stubs, derived skills) land
// in list B marked Derived, so the guidance points at the source instead of
// suggesting a merge of the converted file.
func TestDerivedChangesAreFlagged(t *testing.T) {
	cfg, m, _, _ := setup(t)
	out := cfg.OutDir()
	appendTo := func(p, s string) {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		write(t, p, string(raw)+s)
	}
	appendTo(filepath.Join(out, config.AgentsMD), "\nedited on the artifact side\n")
	appendTo(filepath.Join(out, "workflows", "release-note.md"), "\nedited stub\n")
	appendTo(filepath.Join(out, "skills", "release-note", "SKILL.md"), "\nedited derived skill\n")

	r := collect(t, cfg, m)
	if len(r.Artifacts) != 3 {
		t.Fatalf("artifact changes = %+v", r.Artifacts)
	}
	for _, a := range r.Artifacts {
		if !a.Derived {
			t.Errorf("%s must be flagged as derived", a.Path)
		}
	}
	// The workflow-derived entries must carry the workflow's source path.
	found := 0
	for _, a := range r.Artifacts {
		if a.Item != nil && a.Item.Category == "workflows" {
			found++
			if filepath.Base(a.Item.From) != "release-note.md" {
				t.Errorf("derived change should name the workflow source, got %s", a.Item.From)
			}
		}
	}
	if found != 2 {
		t.Errorf("expected stub and derived skill to map to the workflow item, got %d", found)
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
	for _, l := range r.SourceChanges {
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
	for _, l := range r.SourceChanges {
		if l.Item.From != "" && filepath.Base(l.Item.From) == "security.md" && l.Kind != SrcRootMissing {
			t.Errorf("when the whole source root is missing the kind should be SrcRootMissing, got %v", l.Kind)
		}
	}
}

// Output files the manifest does not track must be reported; otherwise status
// claims "in sync" right before apply deletes them.
func TestUntrackedOutputIsReported(t *testing.T) {
	cfg, m, _, _ := setup(t)
	write(t, filepath.Join(cfg.OutDir(), "rules", "ai-new.md"), "# new rule\n")
	// A new file inside a directory item (skill) is a change of that item,
	// not untracked.
	write(t, filepath.Join(cfg.OutDir(), "skills", "api-doc", "extra.md"), "extra\n")

	r := collect(t, cfg, m)
	var untracked, skillChange bool
	for _, a := range r.Artifacts {
		if a.Untracked && a.Path == "rules/ai-new.md" {
			untracked = true
		}
		if a.Item != nil && filepath.Base(a.Item.From) == "api-doc" {
			skillChange = true
		}
	}
	if !untracked {
		t.Errorf("Artifacts = %+v, want rules/ai-new.md untracked", r.Artifacts)
	}
	if !skillChange {
		t.Error("a new file inside a skill directory must be a change of that item")
	}
	if !r.HasGap {
		t.Error("untracked files must count as a gap")
	}
}

// Hidden files (.DS_Store and friends) never come from a build and are never
// collected from sources; reporting them would flag noise on every run.
func TestHiddenFilesAreNotUntracked(t *testing.T) {
	cfg, m, _, _ := setup(t)
	write(t, filepath.Join(cfg.OutDir(), "rules", ".DS_Store"), "junk")
	r := collect(t, cfg, m)
	for _, a := range r.Artifacts {
		if filepath.Base(a.Path) == ".DS_Store" {
			t.Errorf("hidden file reported: %+v", a)
		}
	}
}

// A deleted output copy is not a change to discard (there is no content) but
// the mount side really is missing files; report it separately so apply can
// rebuild it.
func TestGoneOutputIsReported(t *testing.T) {
	cfg, m, _, _ := setup(t)
	if err := os.Remove(filepath.Join(cfg.OutDir(), "rules", "security.md")); err != nil {
		t.Fatal(err)
	}
	r := collect(t, cfg, m)
	if len(r.Gone) != 1 || r.Gone[0].OutPaths[0] != "rules/security.md" {
		t.Errorf("Gone = %+v", r.Gone)
	}
	if !r.HasGap {
		t.Error("missing outputs must count as a gap")
	}
}
