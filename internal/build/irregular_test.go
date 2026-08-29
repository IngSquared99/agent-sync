//go:build !windows

package build

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func mkfifo(t *testing.T, path string) {
	t.Helper()
	if err := syscall.Mkfifo(path, 0o644); err != nil {
		t.Skipf("cannot create FIFO on this filesystem: %v", err)
	}
}

// Irregular files (FIFOs, sockets, devices) must never be collected: opening
// a named pipe for reading blocks forever, so one planted in a half-trusted
// source would hang the build before any content check could run.
func TestAcceptsRejectsIrregularFiles(t *testing.T) {
	dir := t.TempDir()

	// A "rule" that is actually a FIFO must be rejected with a reason.
	pipe := filepath.Join(dir, "hang.md")
	mkfifo(t, pipe)
	if ok, reason := Accepts("rules", pipe, false); ok || reason == "" {
		t.Fatalf("FIFO rule accepted (ok=%v reason=%q)", ok, reason)
	}

	// A skill directory containing a FIFO anywhere inside must be rejected
	// wholesale, even when its SKILL.md is legitimate.
	sk := filepath.Join(dir, "myskill")
	if err := os.MkdirAll(filepath.Join(sk, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sk, "SKILL.md"), []byte("---\nname: myskill\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mkfifo(t, filepath.Join(sk, "assets", "inner.md"))
	if ok, reason := Accepts("skills", sk, true); ok || reason == "" {
		t.Fatalf("skill containing a FIFO accepted (ok=%v reason=%q)", ok, reason)
	}

	// The same skill without the FIFO is accepted — the rejection above is
	// really about the irregular file, not the directory layout.
	if err := os.Remove(filepath.Join(sk, "assets", "inner.md")); err != nil {
		t.Fatal(err)
	}
	if ok, reason := Accepts("skills", sk, true); !ok {
		t.Fatalf("clean skill rejected: %s", reason)
	}
}

// openNoFollow is the second line of defense shared by every read (copy,
// hash, derive): a file swapped for a FIFO after the scan must fail to open
// instead of blocking forever.
func TestOpenNoFollowRefusesFIFO(t *testing.T) {
	dir := t.TempDir()
	pipe := filepath.Join(dir, "swap.md")
	mkfifo(t, pipe)
	if f, err := openNoFollow(pipe); err == nil {
		f.Close()
		t.Fatal("openNoFollow should refuse a FIFO")
	}
}

// copyDir names the irregular entry before any open is attempted.
func TestCopyDirRefusesFIFO(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	mkfifo(t, filepath.Join(src, "hang.md"))
	if err := copyDir(src, filepath.Join(dir, "dst")); err == nil {
		t.Fatal("copyDir should refuse to copy a FIFO")
	}
}

// FirstIrregularWithin reports FIFOs but stays quiet on regular layouts and
// symlinks (those have their own reporter with a clearer message).
func TestFirstIrregularWithin(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ok.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "ok.md"), filepath.Join(dir, "link.md")); err != nil {
		t.Fatal(err)
	}
	if rel, found := FirstIrregularWithin(dir); found {
		t.Fatalf("regular+symlink layout flagged as irregular: %s", rel)
	}
	mkfifo(t, filepath.Join(dir, "pipe"))
	rel, found := FirstIrregularWithin(dir)
	if !found || rel != "pipe" {
		t.Fatalf("FirstIrregularWithin = (%q, %v), want (\"pipe\", true)", rel, found)
	}
}
