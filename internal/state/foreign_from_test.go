package state

import (
	"path/filepath"
	"testing"
)

// A manifest From outside the configured sources (edited sources, or a
// tampered record) must never be stat'ed or hashed — a hostile path could
// point anywhere or walk a huge tree. It is reported instead, and counts as a
// gap so CI notices.
func TestForeignFromIsReportedNotCompared(t *testing.T) {
	cfg, m, _, _ := setup(t)

	// Point one item's From outside every configured source root.
	foreign := filepath.Join(t.TempDir(), "not-a-source", "security.md")
	patched := false
	for i := range m.Items {
		if m.Items[i].From != "" {
			m.Items[i].From = foreign
			patched = true
			break
		}
	}
	if !patched {
		t.Fatal("no item with a From to patch")
	}

	r := collect(t, cfg, m)
	found := false
	for _, f := range r.ForeignFrom {
		if f == foreign {
			found = true
		}
	}
	if !found {
		t.Fatalf("foreign From must be reported, got %v", r.ForeignFrom)
	}
	for _, sc := range r.SourceChanges {
		if sc.Item.From == foreign {
			t.Error("foreign From must not appear in source changes (it was compared)")
		}
	}
	if !r.HasGap {
		t.Error("a foreign From must count as a gap")
	}
}
