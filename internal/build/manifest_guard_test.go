package build

import (
	"strings"
	"testing"
)

// The manifest lives in the mounted (AI-writable) output, so LoadManifest must
// reject OutPaths that escape the out directory — they are joined onto out by
// status hashing, promote's copy source and the cross-bucket sync, where an
// escaping entry becomes an arbitrary read or write.
func TestLoadManifestRejectsEscapingOutPaths(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{Version: ManifestVersion, Items: []ManifestItem{{
		Category: "rules", Name: "x.md", Original: "x.md",
		OutPaths: []string{"rules/x.md"},
	}}}
	if err := WriteManifest(dir, m); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(dir); err != nil {
		t.Fatalf("legitimate manifest rejected: %v", err)
	}
	for _, bad := range []string{"../evil.md", "rules/../../evil.md", "/abs/evil.md", ""} {
		m.Items[0].OutPaths = []string{"rules/x.md", bad}
		if err := WriteManifest(dir, m); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadManifest(dir); err == nil {
			t.Errorf("OutPaths entry %q was accepted; escaping paths must be rejected", bad)
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
