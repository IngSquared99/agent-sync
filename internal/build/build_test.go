package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IngSquared99/agent-sync/i18n"
	"github.com/IngSquared99/agent-sync/internal/config"
)

// ── Miniature project for tests ─────────────────────────────────────
// A source directory looks like: rule/*.md, skill/<name>/SKILL.md, workflow/*.md

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeSkill(t *testing.T, root, name string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "skill", name, "SKILL.md"), "---\nname: "+name+"\n---\nbody\n")
}

// newProject creates a project: proj/ is the root, libs are source names outside the project
func newProject(t *testing.T, yamlBody string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	proj := filepath.Join(dir, "proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(proj, config.FileName)
	writeFile(t, p, yamlBody)
	cfg, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

const twoSourceYAML = `version: 1
sources:
  - ../lib
  - ./.flow
build:
  out: .agsy
  on_conflict: {rules: %s, skills: %s, workflows: rename}
  route: {field: target, default: [agents, claude], buckets: [agents, claude]}
mount:
  - dir: .claude
    links: {rules: rules, skills: skills, commands: workflows/claude}
  - dir: .agents
    links: {rules: rules, skills: skills, workflows: workflows/agents}
`

func setupTwoSources(t *testing.T, rulesStrategy, skillsStrategy string) (*config.Config, string, string) {
	t.Helper()
	cfg := newProject(t, strings.Replace(strings.Replace(twoSourceYAML, "%s", rulesStrategy, 1), "%s", skillsStrategy, 1))
	lib := filepath.Join(filepath.Dir(cfg.BaseDir), "lib")
	flow := filepath.Join(cfg.BaseDir, ".flow")
	writeFile(t, filepath.Join(lib, "rule", "python-style.md"), "lib version\n")
	writeFile(t, filepath.Join(lib, "rule", "git-commit.md"), "commit\n")
	writeSkill(t, lib, "code-review")
	writeFile(t, filepath.Join(lib, "workflow", "deploy.md"), "---\ntarget: [claude]\n---\nd\n")
	writeFile(t, filepath.Join(lib, "workflow", "release-note.md"), "no target tag\n")
	writeFile(t, filepath.Join(flow, "rule", "python-style.md"), "flow version\n")
	writeSkill(t, flow, "api-doc")
	return cfg, lib, flow
}

func compute(t *testing.T, cfg *config.Config) *Plan {
	t.Helper()
	sources, err := ExpandSources(cfg)
	if err != nil {
		t.Fatal(err)
	}
	p, err := Compute(cfg, sources)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func names(p *Plan, cat string) []string {
	var out []string
	for _, it := range p.Items {
		if it.Category == cat {
			out = append(out, it.OutName)
		}
	}
	return out
}

// ── Acceptance rules ────────────────────────────────────────────────

func TestAccepts(t *testing.T) {
	i18n.SetLang("en")
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "ok.md"), "x")
	writeFile(t, filepath.Join(dir, "note.txt"), "x")
	writeSkill(t, dir, "good")
	if err := os.MkdirAll(filepath.Join(dir, "skill", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		cat, path string
		isDir     bool
		want      bool
		reason    string
	}{
		{"rules", filepath.Join(dir, "ok.md"), false, true, ""},
		{"rules", filepath.Join(dir, "note.txt"), false, false, "extension is not .md"},
		{"rules", filepath.Join(dir, ".hidden.md"), false, false, "starts with ."},
		{"rules", filepath.Join(dir, "subdir"), true, false, "a directory is not accepted"},
		{"workflows", filepath.Join(dir, "ok.md"), false, true, ""},
		{"skills", filepath.Join(dir, "skill", "good"), true, true, ""},
		{"skills", filepath.Join(dir, "skill", "empty"), true, false, "no SKILL.md"},
		{"skills", filepath.Join(dir, "ok.md"), false, false, "a single file is not accepted"},
	}
	for _, c := range cases {
		ok, reason := Accepts(c.cat, c.path, c.isDir)
		if ok != c.want {
			t.Errorf("Accepts(%s,%s)=%v(%s), want %v", c.cat, filepath.Base(c.path), ok, reason, c.want)
		}
		if !ok && !strings.Contains(reason, c.reason) {
			t.Errorf("reason %q does not mention %q", reason, c.reason)
		}
	}
}

// ── Source tags ─────────────────────────────────────────────────────

func TestAssignTagsUnique(t *testing.T) {
	// Both sources end in .flow → parent directories must be merged in to tell
	// them apart, otherwise the two renamed outputs share a name and overwrite
	// each other
	s := []SourceState{{Abs: "/home/me/team/.flow"}, {Abs: "/home/me/myproj/.flow"}}
	AssignTags(s)
	if s[0].Tag == s[1].Tag {
		t.Fatalf("tags must be unique, both are %q", s[0].Tag)
	}
	if s[0].Tag != "team-flow" || s[1].Tag != "myproj-flow" {
		t.Errorf("tags = %q / %q", s[0].Tag, s[1].Tag)
	}
	// Identical paths → fall back to appending a number, still must be unique
	s2 := []SourceState{{Abs: "/a/x"}, {Abs: "/a/x"}}
	AssignTags(s2)
	if s2[0].Tag == s2[1].Tag {
		t.Errorf("even identical paths must yield different tags, got %q", s2[0].Tag)
	}
}

func TestTagged(t *testing.T) {
	if got := tagged("python-style.md", "lib", false); got != "python-style-fromlib-lib.md" {
		t.Errorf("file = %q", got)
	}
	// One uniform "-fromlib-" separator for every category (skill names are
	// additionally sanitized: "@" is illegal in Agent Skills names).
	if got := tagged("api-doc", "flow", true); got != "api-doc-fromlib-flow" {
		t.Errorf("directory = %q", got)
	}
}

// ── The three name-conflict strategies ──────────────────────────────

func TestConflictRename(t *testing.T) {
	cfg, _, _ := setupTwoSources(t, "rename", "error")
	p := compute(t, cfg)
	got := strings.Join(names(p, "rules"), ",")
	// both sides of a conflict get tagged; unconflicted items stay untouched
	for _, want := range []string{"python-style-fromlib-lib.md", "python-style-fromlib-flow.md", "git-commit.md"} {
		if !strings.Contains(got, want) {
			t.Errorf("rules missing %s (got: %s)", want, got)
		}
	}
	if len(p.Conflicts) != 0 || len(p.Collisions) != 0 {
		t.Errorf("rename strategy should have no conflicts: %+v %+v", p.Conflicts, p.Collisions)
	}
}

func TestConflictFirst(t *testing.T) {
	cfg, _, _ := setupTwoSources(t, "first", "error")
	p := compute(t, cfg)
	n := 0
	for _, it := range p.Items {
		if it.Name == "python-style.md" {
			n++
			if it.SourceIdx != 0 {
				t.Errorf("first should keep the lowest source index, got idx=%d", it.SourceIdx)
			}
		}
	}
	if n != 1 {
		t.Errorf("first strategy should keep exactly one copy, got %d", n)
	}
	if len(p.Skipped) != 1 {
		t.Errorf("the dropped copy must be recorded for the plan display, got %d", len(p.Skipped))
	}
}

func TestConflictError(t *testing.T) {
	cfg, lib, flow := setupTwoSources(t, "error", "error")
	_ = lib
	writeSkill(t, flow, "code-review") // make skills clash too
	p := compute(t, cfg)
	if len(p.Conflicts) != 2 {
		t.Fatalf("expected one conflict each for rules and skills, got %+v", p.Conflicts)
	}
	// writing is forbidden while conflicts exist
	if _, err := Execute(cfg, p); err == nil {
		t.Error("Execute must refuse when unresolved conflicts exist")
	}
}

// Collisions are still possible after rename: the source may already contain a
// name like python-style-fromlib-flow.md
func TestDetectCollisions(t *testing.T) {
	cfg, lib, _ := setupTwoSources(t, "rename", "error")
	writeFile(t, filepath.Join(lib, "rule", "python-style-fromlib-flow.md"), "coincidental name\n")
	p := compute(t, cfg)
	if len(p.Collisions) != 1 {
		t.Fatalf("expected 1 final-name collision, got %+v", p.Collisions)
	}
	if _, err := Execute(cfg, p); err == nil {
		t.Error("Execute must refuse on final-name collisions, otherwise one copy is silently overwritten")
	}
}

// ── Workflow routing ────────────────────────────────────────────────

func TestRouteWorkflows(t *testing.T) {
	cfg, lib, _ := setupTwoSources(t, "rename", "error")
	writeFile(t, filepath.Join(lib, "workflow", "standup.md"), "---\ntarget: [agents]\n---\ns\n")
	p := compute(t, cfg)
	want := map[string][]string{
		"deploy.md":       {"claude"},
		"standup.md":      {"agents"},
		"release-note.md": {"agents", "claude"}, // not specified → default applied
	}
	for _, it := range p.Items {
		if it.Category != "workflows" {
			continue
		}
		w, ok := want[it.Name]
		if !ok {
			continue
		}
		if strings.Join(it.Buckets, ",") != strings.Join(w, ",") {
			t.Errorf("%s → %v, want %v", it.Name, it.Buckets, w)
		}
	}
}

func TestRouteUnknownBucketFails(t *testing.T) {
	// A bad bucket no longer aborts Compute (that would hide the rest of the
	// plan); it is collected into RouteErrors and Execute refuses to build.
	cfg, lib, _ := setupTwoSources(t, "rename", "error")
	writeFile(t, filepath.Join(lib, "workflow", "bad.md"), "---\ntarget: [nosuch]\n---\nx\n")
	p := compute(t, cfg)
	if len(p.RouteErrors) != 1 || !strings.Contains(p.RouteErrors[0], "nosuch") {
		t.Errorf("RouteErrors = %v, want the unknown bucket collected", p.RouteErrors)
	}
	if _, err := Execute(cfg, p); err == nil {
		t.Error("Execute must refuse while route errors exist")
	}
}

func TestRouteEmptyDefault(t *testing.T) {
	cfg, _, _ := setupTwoSources(t, "rename", "error")
	cfg.Build.Route.Default = nil
	p := compute(t, cfg)
	if len(p.NoBucket) != 1 || p.NoBucket[0] != "release-note.md" {
		t.Errorf("with an empty default, an untagged workflow must land in the NoBucket warning, got %v", p.NoBucket)
	}
	// items placed in no bucket must not count toward "will be placed in the output"
	placed := p.Placed()
	total := len(p.Items)
	if placed != total-1 {
		t.Errorf("Placed=%d, Items=%d, expected a difference of 1", placed, total)
	}
}

// ── Execute end to end ──────────────────────────────────────────────

func TestExecuteEndToEnd(t *testing.T) {
	cfg, lib, _ := setupTwoSources(t, "rename", "error")
	// the skill carries an executable script: permissions must be preserved
	script := filepath.Join(lib, "skill", "code-review", "run.sh")
	writeFile(t, script, "#!/bin/sh\necho hi\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	p := compute(t, cfg)
	m, err := Execute(cfg, p)
	if err != nil {
		t.Fatal(err)
	}
	out := cfg.OutDir()

	// 1) permissions preserved
	st, err := os.Stat(filepath.Join(out, "skills", "code-review", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o755 {
		t.Errorf("permissions after copy = %v, want 0755 (a skill's scripts must stay executable)", st.Mode().Perm())
	}

	// 2) a workflow without a target gets one copy in each of the two buckets
	for _, b := range []string{"agents", "claude"} {
		if _, err := os.Stat(filepath.Join(out, "workflows", b, "release-note.md")); err != nil {
			t.Errorf("%s bucket is missing release-note.md", b)
		}
	}

	// 3) every mount target directory must exist (otherwise links pointing there are broken)
	for _, mnt := range cfg.Mount {
		for _, sub := range mnt.Links {
			if st, err := os.Stat(filepath.Join(out, filepath.FromSlash(sub))); err != nil || !st.IsDir() {
				t.Errorf("mount target %s does not exist", sub)
			}
		}
	}

	// 4) manifest: both hash baselines + multi-copy paths + per-file source hashes
	var rn, skill *ManifestItem
	for i := range m.Items {
		switch m.Items[i].Name {
		case "release-note.md":
			rn = &m.Items[i]
		case "code-review":
			skill = &m.Items[i]
		}
	}
	if rn == nil || len(rn.OutPaths) != 2 {
		t.Fatalf("release-note should have two output paths, got %+v", rn)
	}
	if skill == nil || skill.Hash == "" || skill.SrcHash == "" || len(skill.SrcFiles) != 2 {
		t.Fatalf("skill manifest data incomplete: %+v", skill)
	}

	// 5) rerunning the build must give the same result (full reproducibility is
	// this tool's one promise)
	p2 := compute(t, cfg)
	m2, err := Execute(cfg, p2)
	if err != nil {
		t.Fatal(err)
	}
	if len(m2.Items) != len(m.Items) {
		t.Errorf("item count differs after rebuild: %d → %d", len(m.Items), len(m2.Items))
	}
}

func TestExecuteRefusesIncomplete(t *testing.T) {
	cfg, lib, _ := setupTwoSources(t, "rename", "error")
	if err := os.RemoveAll(lib); err != nil {
		t.Fatal(err)
	}
	p := compute(t, cfg)
	if !p.Incomplete {
		t.Fatal("Plan.Incomplete should be true when a source does not exist")
	}
	if _, err := Execute(cfg, p); err == nil {
		t.Error("must not rebuild with incomplete sources (that source's items would be lost)")
	}
}

// ── Deletion safety net ─────────────────────────────────────────────

func TestRemoveOutRefusesDangerousPaths(t *testing.T) {
	i18n.SetLang("en")
	cfg, lib, _ := setupTwoSources(t, "rename", "error")
	dangerous := []struct{ out, want string }{
		{cfg.BaseDir, "contains the project root"},
		{filepath.Dir(cfg.BaseDir), "contains the project root"},
		{lib, "not inside the project directory"},
		{string(filepath.Separator), "suspicious"},
	}
	for _, c := range dangerous {
		cfg.Build.Out = c.out
		err := RemoveOut(cfg)
		if err == nil {
			t.Fatalf("RemoveOut(%s) should have been blocked", c.out)
		}
		if !strings.Contains(err.Error(), c.want) {
			t.Errorf("error message %q does not mention %q", err, c.want)
		}
		// the target must be intact
		if _, statErr := os.Stat(c.out); statErr != nil {
			t.Fatalf("%s was deleted", c.out)
		}
	}
}

// ── Other helpers ───────────────────────────────────────────────────

func TestRewriteSkillName(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "SKILL.md")
	writeFile(t, p, "---\nname: api-doc\ndescription: x\n---\nbody text\n")
	if err := RewriteSkillName(p, "api-doc-fromlib-flow"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	if !strings.Contains(string(raw), "name: api-doc-fromlib-flow") {
		t.Errorf("front-matter not rewritten:\n%s", raw)
	}
	if !strings.Contains(string(raw), "description: x") || !strings.Contains(string(raw), "body text") {
		t.Error("the rest of the content must not be touched")
	}
	// a file without front-matter stays untouched
	p2 := filepath.Join(dir, "plain.md")
	writeFile(t, p2, "no front-matter\n")
	if err := RewriteSkillName(p2, "x"); err != nil {
		t.Fatal(err)
	}
	raw2, _ := os.ReadFile(p2)
	if string(raw2) != "no front-matter\n" {
		t.Errorf("must not modify: %q", raw2)
	}
}

func TestDiffFiles(t *testing.T) {
	before := map[string]string{"a": "1", "b": "2", "gone": "3"}
	after := map[string]string{"a": "1", "b": "changed", "new": "4"}
	got := strings.Join(DiffFiles(before, after), ",")
	if got != "b,gone,new" {
		t.Errorf("DiffFiles = %q, want b,gone,new (changed / removed / added all count)", got)
	}
	if DiffFiles(nil, after) != nil {
		t.Error("with a missing baseline it should return nil, not guess")
	}
}

func TestHashPathDirIsOrderStable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "A")
	writeFile(t, filepath.Join(dir, "sub", "b.md"), "B")
	h1, files, err := HashPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("per-file hashes should have 2 entries, got %d", len(files))
	}
	h2, _, _ := HashPath(dir)
	if h1 != h2 {
		t.Error("identical content must produce an identical hash")
	}
	writeFile(t, filepath.Join(dir, "sub", "b.md"), "B2")
	h3, _, _ := HashPath(dir)
	if h1 == h3 {
		t.Error("the hash must change when content changes")
	}
}

func TestManifestVersionGuard(t *testing.T) {
	i18n.SetLang("en")
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ManifestName), `{"version": 99, "items": []}`)
	if _, err := LoadManifest(dir); err == nil || !strings.Contains(err.Error(), "please upgrade") {
		t.Errorf("a future-version manifest must refuse to be force-parsed, got %v", err)
	}
}
