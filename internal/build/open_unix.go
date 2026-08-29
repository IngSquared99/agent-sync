//go:build !windows

package build

import (
	"fmt"
	"os"
	"syscall"

	"github.com/IngSquared99/agent-sync/i18n"
)

// openNoFollow opens path for reading, refusing to follow a symlink in the
// final component and refusing anything that is not a regular file. The
// callers' Lstat guard and the open itself are separate syscalls; O_NOFOLLOW
// closes the swap window between them, so a file exchanged for a link at
// exactly the wrong moment fails to open instead of smuggling its target's
// content into the output.
//
// O_NONBLOCK closes the equivalent window for irregular files: opening a
// FIFO read-only without it blocks forever, so a source file swapped for a
// named pipe would hang the build before any check could run. With
// O_NONBLOCK the open returns immediately, the fstat below rejects the
// irregular file, and for the regular files that pass the flag has no
// effect on subsequent reads.
func openNoFollow(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if !st.Mode().IsRegular() {
		f.Close()
		return nil, fmt.Errorf(i18n.T("refusing to read %s: not a regular file"), path)
	}
	return f, nil
}
