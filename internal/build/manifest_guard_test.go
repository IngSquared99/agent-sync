package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The manifest lives in the mounted (AI-writable) output, so LoadManifest must
// reject output paths that escape the out directory — status hashing joins
// them onto out, where an escaping entry becomes an arbitrary read.
func TestLoadManifestRejectsEscapingOutPaths(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{Version: ManifestVersion, Items: []ManifestItem{{
		Category: "rules", Name: "x.md", Original: "x.md",
		Outs: []OutEntry{{Path: "rules/x.md", Hash: "sha256:x"}},
	}}}
	if err := WriteManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(dir); err != nil {
		t.Fatalf("legitimate manifest rejected: %v", err)
	}
	for _, bad := range []string{"../evil.md", "rules/../../evil.md", "/abs/evil.md", ""} {
		m.Items[0].Outs = []OutEntry{{Path: "rules/x.md"}, {Path: bad}}
		if err := WriteManifest(dir, m); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadManifest(dir); err == nil {
			t.Errorf("output path %q was accepted; escaping paths must be rejected", bad)
		}
	}
}

// Numeric-suffix disambiguation must check its candidates against existing
// tags: with sources tagged x, x and a third source whose tag is literally x2,
// blind appending would hand the second x the name x2 and collide.
func TestAssignTagsNumericFallbackUnique(t *testing.T) {
	// The two /…/x paths share their last five segments, so parent-merging
	// (depth ≤ 4) cannot separate them and the numeric fallback kicks in.
	base := "p2/p3/p4/p5/x"
	sources := []SourceState{
		{Abs: "/p1/" + base},
		{Abs: "/q1/" + base},
		{Abs: "/z/" + strings.ReplaceAll(base, "/", "-") + "2"}, // tag: p2-p3-p4-p5-x2
	}
	AssignTags(sources)
	seen := map[string]bool{}
	for _, s := range sources {
		if seen[s.Tag] {
			t.Fatalf("duplicate tag %q; tags: %v", s.Tag, []string{sources[0].Tag, sources[1].Tag, sources[2].Tag})
		}
		seen[s.Tag] = true
	}
}

// ExecuteWith writes the previous apply's merge records before building, so
// a build that fails halfway leaves a manifest that still carries them.
func TestExecuteFailureKeepsMergeRecords(t *testing.T) {
	cfg, lib, _ := setupTwoSources(t, "rename", "error")
	p := compute(t, cfg)
	// Break a source after Compute: the copy step fails inside Execute.
	rule := filepath.Join(lib, "rules", "python-style.md")
	if err := os.Remove(rule); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rule, 0o755); err != nil {
		t.Fatal(err)
	}
	prior := []MergeRecord{{Path: ".claude/settings.json", Key: "hooks", Hash: "h", Created: true, HooksDir: ".agsy/hooks", AddedKey: true}}
	if _, err := ExecuteWith(cfg, p, prior); err == nil {
		t.Fatal("Execute must fail once a source turned into a directory")
	}
	m, err := LoadManifest(cfg.OutDir())
	if err != nil {
		t.Fatalf("manifest must exist after a failed build: %v", err)
	}
	if len(m.Merges) != 1 || !m.Merges[0].Created || !m.Merges[0].AddedKey || m.Merges[0].HooksDir != ".agsy/hooks" {
		t.Errorf("merge records lost: %+v", m.Merges)
	}
	if len(m.Items) != 0 {
		t.Errorf("the minimal manifest records no items: %+v", m.Items)
	}
}
