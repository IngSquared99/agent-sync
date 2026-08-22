//go:build !windows

package mount

import (
	"os"
	"path/filepath"
)

// linkDir on non-Windows: relative-path symlink (not tied to one machine)
func linkDir(absTarget, linkPath string) error {
	rel, err := filepath.Rel(filepath.Dir(linkPath), absTarget)
	if err != nil {
		rel = absTarget
	}
	return os.Symlink(rel, linkPath)
}

func isLink(fi os.FileInfo, _ string) bool {
	return fi.Mode()&os.ModeSymlink != 0
}

func platformHint() string { return "" }
