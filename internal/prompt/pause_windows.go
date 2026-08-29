//go:build windows

package prompt

import (
	"syscall"
	"unsafe"
)

var (
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
)

// launchedByDoubleClick reports whether this program was started by double-click.
// Rationale: on double-click the console was created just for this program, so
// the only process attached to it is ourselves; when run from cmd / PowerShell,
// the shell shares the same console and the count is >= 2.
func launchedByDoubleClick() bool {
	var pids [8]uint32
	n, _, _ := procGetConsoleProcessList.Call(
		uintptr(unsafe.Pointer(&pids[0])), uintptr(len(pids)))
	return n == 1
}
