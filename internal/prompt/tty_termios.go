//go:build linux || darwin

package prompt

import (
	"os"
	"syscall"
	"unsafe"
)

// isTerminal uses the termios ioctl to decide whether fd is a terminal
// (equivalent to libc's isatty).
// Checking os.ModeCharDevice alone is not enough: /dev/null is a character
// device too, so the CI-common `agsy init </dev/null` would be misread as
// interactive and bypass the non-interactive gate — only a real terminal
// answers the termios query.
func isTerminal(f *os.File) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, f.Fd(),
		uintptr(ioctlReadTermios), uintptr(unsafe.Pointer(&t)), 0, 0, 0)
	return errno == 0
}
