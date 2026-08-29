package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/IngSquared99/agent-sync/i18n"
	"github.com/IngSquared99/agent-sync/internal/config"
)

// lockStale is the age past which a lock file is treated as a leftover from
// a crashed run and replaced. apply / clean finish within seconds.
const lockStale = 15 * time.Minute

// acquireLock guards the mutating commands (apply / clean) against concurrent
// runs in one project. Advisory: the lock file sits next to agsy.yaml (the
// output directory is wiped by apply and cannot hold it). The returned
// release removes the lock.
func acquireLock(cfg *config.Config) (func(), error) {
	return acquireLockDir(cfg.BaseDir)
}

// acquireLockDir is the config-free form: init also writes into the project
// directory (agsy.yaml, .gitignore) and must not interleave with a concurrent
// apply, but it runs before a loadable config necessarily exists.
func acquireLockDir(dir string) (func(), error) {
	path := filepath.Join(dir, ".agsy.lock")
	for attempt := 0; ; attempt++ {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			fmt.Fprintf(f, "pid %d at %s\n", os.Getpid(), time.Now().Format(time.RFC3339))
			fi, _ := f.Stat()
			f.Close()
			// Belt and braces: owning the lock means the path still names the
			// file this run created.
			if di, derr := os.Lstat(path); derr != nil || fi == nil || !os.SameFile(fi, di) {
				return nil, lockBusyErr(path)
			}
			return func() { os.Remove(path) }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		// A crashed run cannot release its lock; a stale one is replaced once
		// instead of blocking every future run. Takeover is two-phase to close
		// the remove-the-wrong-lock race: blindly removing path here could hit
		// a FRESH lock created by another run between our Stat and Remove.
		// Rename claims the file atomically (only one contender succeeds),
		// staleness is re-verified on the claimed file, and a file that turns
		// out to be fresh is put back instead of deleted.
		if st, serr := os.Stat(path); serr == nil && time.Since(st.ModTime()) > lockStale && attempt == 0 {
			breaking := path + ".breaking"
			if rerr := os.Rename(path, breaking); rerr == nil {
				bst, berr := os.Stat(breaking)
				if berr == nil && time.Since(bst.ModTime()) <= lockStale {
					// The claimed lock is no longer stale (another run replaced
					// it in between): restore it and back off.
					_ = os.Rename(breaking, path)
					return nil, lockBusyErr(path)
				}
				_ = os.Remove(breaking)
			}
			// A failed rename means another contender claimed the stale lock
			// first; either way, retry the exclusive create exactly once.
			continue
		}
		return nil, lockBusyErr(path)
	}
}

func lockBusyErr(path string) error {
	return fmt.Errorf(i18n.T("another agsy run appears to be in progress (%s exists); wait for it to finish, or delete the file if it is a leftover"), path)
}
