package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/IngSquared99/agent-sync/internal/prompt"
)

// Two mutating runs in one project must not interleave: apply refuses while a
// fresh lock file exists, and a stale one (crashed run) is replaced instead of
// blocking every future apply.
func TestApplyRefusesWhenLocked(t *testing.T) {
	proj := setupApplied(t)
	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()

	lock := filepath.Join(proj, ".agsy.lock")
	write(t, lock, "pid 12345\n")
	if code := cmdApply(); code == 0 {
		t.Fatal("apply must refuse while another run holds the lock")
	}

	// A stale lock (older than lockStale) is a leftover, not a live run.
	old := time.Now().Add(-lockStale - time.Minute)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatal(err)
	}
	if code := cmdApply(); code != 0 {
		t.Fatal("apply must replace a stale lock and proceed")
	}
	// The lock is released after a successful run.
	if _, err := os.Stat(lock); err == nil {
		t.Error("lock file should be removed after apply finishes")
	}
}
