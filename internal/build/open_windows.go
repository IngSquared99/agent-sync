//go:build windows

package build

import (
	"fmt"
	"os"

	"github.com/IngSquared99/agent-sync/i18n"
)

// openNoFollow on Windows: a plain open plus a regular-file check. There is
// no direct O_NOFOLLOW equivalent here, and creating symlinks on Windows
// requires Developer Mode or elevation in the first place, so the callers'
// Lstat guard remains the effective symlink check on this platform. The
// fstat below keeps the irregular-file rule identical across platforms:
// only regular files are ever read into a build.
func openNoFollow(path string) (*os.File, error) {
	f, err := os.Open(path)
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
