package build

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// Sources containing symbolic links must never be collected: build copies file
// contents, so a link could smuggle files from outside the source (e.g. a
// private key) into the mounted output.
func TestAcceptsRejectsSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks on Windows requires extra privileges")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "outside.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A rule that is actually a symlink must be rejected with a reason.
	link := filepath.Join(dir, "sneaky.md")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if ok, reason := Accepts("rules", link, false); ok || reason == "" {
		t.Fatalf("symlinked rule accepted (ok=%v reason=%q)", ok, reason)
	}

	// A skill directory that contains a symlink anywhere inside must be
	// rejected wholesale, even when its SKILL.md is legitimate.
	sk := filepath.Join(dir, "myskill")
	if err := os.MkdirAll(filepath.Join(sk, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sk, "SKILL.md"), []byte("---\nname: myskill\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(sk, "assets", "inner.md")); err != nil {
		t.Fatal(err)
	}
	if ok, reason := Accepts("skills", sk, true); ok || reason == "" {
		t.Fatalf("skill containing a symlink accepted (ok=%v reason=%q)", ok, reason)
	}

	// The same skill without the symlink is accepted — the rejection above is
	// really about the link, not about the directory layout.
	if err := os.Remove(filepath.Join(sk, "assets", "inner.md")); err != nil {
		t.Fatal(err)
	}
	if ok, reason := Accepts("skills", sk, true); !ok {
		t.Fatalf("clean skill rejected: %s", reason)
	}
}

// copyDir is the second line of defense: even if a symlink slipped past the
// scan, it must never be copied as target content.
func TestCopyDirRefusesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks on Windows requires extra privileges")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "outside.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(src, "leak.md")); err != nil {
		t.Fatal(err)
	}
	if err := copyDir(src, filepath.Join(dir, "dst")); err == nil {
		t.Fatal("copyDir should refuse to copy a symlink")
	}
}
