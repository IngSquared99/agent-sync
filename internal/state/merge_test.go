package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IngSquared99/agent-sync/internal/build"
	"github.com/IngSquared99/agent-sync/internal/config"
	"github.com/IngSquared99/agent-sync/internal/mount"

	"github.com/IngSquared99/agent-sync/i18n"
)

// Message assertions below compare the English source strings.
func init() { i18n.SetLang("en") }

// setupMerge: a project with one hook and a merge mount, fully applied
// (build + links + merge), returning the manifest with the merge record.
func setupMerge(t *testing.T) (*config.Config, *build.Manifest, string) {
	t.Helper()
	root := t.TempDir()
	proj := filepath.Join(root, "proj")
	lib := filepath.Join(root, "lib")
	yaml := strings.Replace(stateYAML, "    links: {rules: rules, skills: skills}\n",
		"    links: {rules: rules, skills: skills}\n    merge: {settings.json: hooks.claude.json}\n", 1)
	write(t, filepath.Join(proj, config.FileName), yaml)
	write(t, filepath.Join(lib, "hooks", "block-rm", "hook.yaml"), "events:\n  PreToolUse:\n    - matcher: Bash\n      hooks: [{command: ./b.sh}]\n")
	write(t, filepath.Join(lib, "hooks", "block-rm", "b.sh"), "")
	write(t, filepath.Join(proj, ".flow", "rules", "x.md"), "x\n")
	cfg, err := config.Load(filepath.Join(proj, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	sources, _ := build.ExpandSources(cfg)
	p, err := build.Compute(cfg, sources)
	if err != nil {
		t.Fatal(err)
	}
	m, err := build.Execute(cfg, p)
	if err != nil {
		t.Fatal(err)
	}
	links, _ := mount.Inspect(cfg)
	if err := mount.Apply(cfg, links); err != nil {
		t.Fatal(err)
	}
	mp, _ := mount.InspectMerge(cfg, nil)
	recs, err := mount.ApplyMerge(cfg, mp, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.Merges = recs
	return cfg, m, filepath.Join(proj, ".claude", "settings.json")
}

func TestMergeCleanNoGap(t *testing.T) {
	cfg, m, _ := setupMerge(t)
	r := collect(t, cfg, m)
	if r.HasGap || r.MergeBad != 0 || len(r.Merges) != 1 || r.Merges[0].State != mount.MergeClean {
		t.Errorf("report = gap:%v mergeBad:%d merges:%+v", r.HasGap, r.MergeBad, r.Merges)
	}
}

func TestMergeModifiedIsGap(t *testing.T) {
	cfg, m, settings := setupMerge(t)
	raw, _ := os.ReadFile(settings)
	os.WriteFile(settings, []byte(strings.Replace(string(raw), `"matcher": "Bash"`, `"matcher": "Edit"`, 1)), 0o644)
	r := collect(t, cfg, m)
	if !r.HasGap || r.Merges[0].State != mount.MergeModified {
		t.Errorf("modified not detected: %+v", r.Merges)
	}
}

func TestMergeGoneIsGap(t *testing.T) {
	cfg, m, settings := setupMerge(t)
	os.Remove(settings)
	r := collect(t, cfg, m)
	if !r.HasGap || r.Merges[0].State != mount.MergeMissing {
		t.Errorf("missing not detected: %+v", r.Merges)
	}
	os.WriteFile(settings, []byte(`{"model":"x"}`), 0o644)
	r = collect(t, cfg, m)
	if !r.HasGap || r.Merges[0].State != mount.MergeAbsent {
		t.Errorf("absent not detected: %+v", r.Merges)
	}
}

func TestMergeInvalidIsGap(t *testing.T) {
	cfg, m, settings := setupMerge(t)
	os.WriteFile(settings, []byte(`[]`), 0o644)
	r := collect(t, cfg, m)
	if !r.HasGap || r.Merges[0].State != mount.MergeInvalid {
		t.Errorf("invalid not detected: %+v", r.Merges)
	}
}

func TestHookArtifactEditsAreDetected(t *testing.T) {
	cfg, m, _ := setupMerge(t)
	out := cfg.OutDir()
	// script edited on the artifact side
	write(t, filepath.Join(out, "hooks", "block-rm", "b.sh"), "changed\n")
	r := collect(t, cfg, m)
	found := false
	for _, a := range r.Artifacts {
		if a.Item != nil && a.Item.Category == "hooks" && !a.Derived {
			found = true
		}
	}
	if !found || !r.HasGap {
		t.Errorf("edited hook script must be an artifact-side change: %+v", r.Artifacts)
	}
	// registry edited on the artifact side → derived change
	write(t, filepath.Join(out, "hooks.codex.json"), `{"hooks":{}}`)
	r = collect(t, cfg, m)
	found = false
	for _, a := range r.Artifacts {
		if a.Item != nil && a.Item.Category == "hooks-registry" && a.Derived {
			found = true
		}
	}
	if !found {
		t.Errorf("edited registry must be a derived artifact-side change: %+v", r.Artifacts)
	}
	// registry deleted → gone
	os.Remove(filepath.Join(out, "hooks.cursor.json"))
	r = collect(t, cfg, m)
	gone := false
	for _, g := range r.Gone {
		if g.Item.Name == "hooks.cursor.json" {
			gone = true
		}
	}
	if !gone {
		t.Errorf("deleted registry must be reported as gone: %+v", r.Gone)
	}
}

func TestHookSourceChangeAndNewHookDetected(t *testing.T) {
	cfg, m, _ := setupMerge(t)
	lib := filepath.Join(filepath.Dir(cfg.BaseDir), "lib")
	write(t, filepath.Join(lib, "hooks", "block-rm", "b.sh"), "v2\n")
	write(t, filepath.Join(lib, "hooks", "another", "hook.yaml"), "events:\n  Stop:\n    - hooks: [{command: ./a}]\n")
	write(t, filepath.Join(lib, "hooks", "another", "a"), "")
	r := collect(t, cfg, m)
	if len(r.SourceChanges) != 1 || r.SourceChanges[0].Item.Name != "block-rm" {
		t.Errorf("source change = %+v", r.SourceChanges)
	}
	if len(r.News) != 1 || r.News[0].Name != "another" || r.News[0].Category != "hooks" {
		t.Errorf("new hook = %+v", r.News)
	}
}
