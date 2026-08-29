//go:build windows

package prompt

import (
	"os"
	"syscall"
)

// isTerminal: GetConsoleMode succeeds only for a real console; NUL,
// redirection to a file, and pipes all fail — exactly matching the
// question "is anyone watching".
func isTerminal(f *os.File) bool {
	var mode uint32
	return syscall.GetConsoleMode(syscall.Handle(f.Fd()), &mode) == nil
}
