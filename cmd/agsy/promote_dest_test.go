package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/IngSquared99/agent-sync/internal/build"
	"github.com/IngSquared99/agent-sync/internal/config"
)

// Table-driven exhaustion of the promote destination guard: every trust rule
// lives in resolvePromoteDest, so every way of steering the write-back is
// pinned down here in one place.
func TestResolvePromoteDest(t *testing.T) {
	proj := t.TempDir()
	libA := filepath.Join(proj, "lib-a")
	libB := filepath.Join(proj, "lib-b")
	for _, d := range []string{
		filepath.Join(libA, "rules"), filepath.Join(libA, "skills"),
		filepath.Join(libB, "rules"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.Config{
		Sources: []string{"./lib-a", "./lib-b"},
		BaseDir: proj,
	}
	cfg.Build.Categories = map[string]config.Category{
		"rules":  {From: "rules", To: "rules"},
		"skills": {From: "skills", To: "skills"},
	}
	item := func(cat, from, original string) build.ManifestItem {
		return build.ManifestItem{Category: cat, Name: original, Original: original, From: from}
	}

	cases := []struct {
		name    string
		it      build.ManifestItem
		toRaw   string
		wantErr bool
	}{
		{"legitimate origin", item("rules", filepath.Join(libA, "rules", "x.md"), "x.md"), "", false},
		{"legitimate skill dir", item("skills", filepath.Join(libA, "skills", "api-doc"), "api-doc"), "", false},
		{"legitimate --to another source", item("rules", filepath.Join(libA, "rules", "x.md"), "x.md"), "./lib-b", false},
		{"outside every source", item("rules", filepath.Join(proj, "elsewhere", "x.md"), "x.md"), "", true},
		{"category dir itself (parent-level wipe)", item("skills", filepath.Join(libA, "skills"), "api-doc"), "", true},
		{"source root itself", item("skills", libA, "api-doc"), "", true},
		{"wrong category subdir", item("skills", filepath.Join(libA, "rules", "api-doc"), "api-doc"), "", true},
		{"nested deeper than the slot", item("rules", filepath.Join(libA, "rules", "sub", "x.md"), "x.md"), "", true},
		{"basename differs from original", item("rules", filepath.Join(libA, "rules", "other.md"), "x.md"), "", true},
		{"original smuggles a path", item("rules", filepath.Join(libA, "rules", "x.md"), "../x.md"), "", true},
		{"--to not a configured source", item("rules", filepath.Join(libA, "rules", "x.md"), "x.md"), "/tmp/evil", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dest, err := resolvePromoteDest(cfg, tc.it, tc.toRaw)
			if tc.wantErr && err == nil {
				t.Fatalf("want error, got dest %q", dest)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// A destination that is itself a symlink must be refused: writing through it
// would land the content wherever the link points.
func TestResolvePromoteDestRefusesSymlinkTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks on Windows requires extra privileges")
	}
	proj := t.TempDir()
	lib := filepath.Join(proj, "lib")
	if err := os.MkdirAll(filepath.Join(lib, "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(proj, "victim.md")
	if err := os.WriteFile(victim, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(lib, "rules", "x.md")
	if err := os.Symlink(victim, link); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Sources: []string{"./lib"}, BaseDir: proj}
	cfg.Build.Categories = map[string]config.Category{"rules": {From: "rules", To: "rules"}}
	it := build.ManifestItem{Category: "rules", Name: "x.md", Original: "x.md", From: link}
	if _, err := resolvePromoteDest(cfg, it, ""); err == nil || !strings.Contains(err.Error(), "symbolic") {
		t.Fatalf("symlink destination must be refused, got err=%v", err)
	}
}
