package build

import (
	"os"
	"path/filepath"
	"testing"
)

// Routing problems are collected per file so plan can list them all at once;
// unaffected workflows still get their buckets, and Execute refuses to build.
func TestRouteProblemsAreCollectedNotFatal(t *testing.T) {
	cfg, lib, _ := setupTwoSources(t, "rename", "error")
	writeFile(t, filepath.Join(lib, "workflows", "bad1.md"), "---\ntarget: cursor\n---\nx\n")
	writeFile(t, filepath.Join(lib, "workflows", "bad2.md"), "---\ntarget: [nope]\n---\nx\n")

	p := compute(t, cfg)
	if len(p.RouteErrors) != 2 {
		t.Fatalf("RouteErrors = %v, want both bad files listed", p.RouteErrors)
	}
	for _, it := range p.Items {
		if it.Category == "workflows" && it.Name == "deploy.md" && len(it.Buckets) == 0 {
			t.Error("a broken file must not affect routing of other workflows")
		}
	}
	if _, err := Execute(cfg, p); err == nil {
		t.Error("Execute must refuse while route errors exist")
	}
}

// The closing front-matter delimiter must be a standalone line: an unclosed
// block whose body contains a horizontal rule is not front matter and must
// not have its body parsed as YAML.
func TestFrontMatterRequiresStandaloneClosingLine(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "w.md")

	if err := os.WriteFile(p, []byte("---\ntitle: [broken\n-----\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fm, err := frontMatter(p)
	if err != nil {
		t.Fatalf("body must not be parsed as YAML: %v", err)
	}
	if fm != nil {
		t.Errorf("no standalone closing line means no front matter, got %v", fm)
	}

	// A properly closed block (including CRLF) still parses.
	if err := os.WriteFile(p, []byte("---\r\ntarget: claude\r\n---\r\nbody\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fm, err = frontMatter(p)
	if err != nil {
		t.Fatal(err)
	}
	if fm == nil || fm["target"] != "claude" {
		t.Errorf("front matter with CRLF closing line = %v", fm)
	}
}
