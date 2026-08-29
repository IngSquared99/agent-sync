package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// acquireLockDir: a fresh lock blocks, a stale one is taken over via the
// two-phase rename claim, and release removes the lock again.
func TestAcquireLockDirTakeover(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".agsy.lock")
	if err := os.WriteFile(path, []byte("pid 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// fresh lock → refused
	if _, err := acquireLockDir(dir); err == nil {
		t.Fatal("a fresh lock must block acquisition")
	}

	// stale lock → taken over; no .breaking leftover survives the takeover
	old := time.Now().Add(-lockStale - time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	release, err := acquireLockDir(dir)
	if err != nil {
		t.Fatalf("a stale lock must be replaced: %v", err)
	}
	if _, err := os.Stat(path + ".breaking"); err == nil {
		t.Error("takeover must not leave a .breaking file behind")
	}
	if st, err := os.Stat(path); err != nil || time.Since(st.ModTime()) > time.Minute {
		t.Error("the lock must exist and be fresh after takeover")
	}
	release()
	if _, err := os.Stat(path); err == nil {
		t.Error("release must remove the lock")
	}
}
