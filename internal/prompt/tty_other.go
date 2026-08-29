//go:build !linux && !darwin && !windows

package prompt

import "os"

// Other platforms fall back to the character-device check (shipping targets
// are only linux / darwin / windows; this exists just to keep the code
// compiling and cannot tell apart edge cases like /dev/null).
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
