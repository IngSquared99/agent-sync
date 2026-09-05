package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The ancestor guards must compare resolved locations: a symlinked directory
// inside the project could otherwise make build.out (which apply wipes) read
// as project-local while pointing anywhere on disk.
func TestSymlinkedOutRejected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks on Windows requires extra privileges")
	}
	dir := t.TempDir()
	proj := filepath.Join(dir, "proj")
	outside := filepath.Join(dir, "elsewhere")
	if err := os.MkdirAll(filepath.Join(proj, "lib", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	// proj/data → elsewhere: lexically "data/out" is inside the project.
	if err := os.Symlink(outside, filepath.Join(proj, "data")); err != nil {
		t.Fatal(err)
	}
	cfgRaw := `version: 1
sources: [./lib]
build:
  out: ./data/out
  on_conflict: {rules: rename, skills: error, workflows: rename, hooks: error}
  tools: [claude]
mount:
  - dir: .claude
    links: {rules: rules}
`
	path := filepath.Join(proj, FileName)
	if err := os.WriteFile(path, []byte(cfgRaw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("build.out reached through a symlink escaping the project must be rejected")
	}
}

// ResolveSymlinks must resolve through existing ancestors even when the leaf
// does not exist yet, and must fall back to the cleaned input when nothing
// exists.
func TestResolveSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks on Windows requires extra privileges")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	got := ResolveSymlinks(filepath.Join(link, "not-yet", "out"))
	want := filepath.Join(ResolveSymlinks(real), "not-yet", "out")
	if got != want {
		t.Errorf("ResolveSymlinks = %q, want %q", got, want)
	}
	nowhere := filepath.Join(string(filepath.Separator), "no-such-root-agsy-test", "x")
	if got := ResolveSymlinks(nowhere); got != filepath.Clean(nowhere) {
		t.Errorf("nonexistent path should return its cleaned form, got %q", got)
	}
}
