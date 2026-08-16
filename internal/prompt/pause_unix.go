//go:build !windows

package prompt

// Non-Windows has no "double-clicking the executable opens a console" scenario.
func launchedByDoubleClick() bool { return false }
