package config

import (
	"os"
	"path/filepath"
	"testing"
)

// FindUp: relative paths resolve against the config file's directory, so
// running from a project subdirectory must still find the project root's
// config (same convention as git).
func TestFindUpFromSubdir(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, FileName), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	p, ok := FindUp(sub)
	if !ok || p != filepath.Join(root, FileName) {
		t.Errorf("FindUp(%s) = %q, %v, want %s", sub, p, ok, filepath.Join(root, FileName))
	}
	// Find (used by init) still only looks at the current directory: running init
	// in a subdirectory must not pick up an enclosing project's config
	if _, ok := Find(sub); ok {
		t.Error("Find must only look at the current directory, never upward")
	}
}
