// Package prompt provides dependency-free terminal Q&A: confirmation,
// single-choice selection, text input, and TTY detection.
package prompt

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/IngSquared99/agent-sync/i18n"
)

var reader = bufio.NewReader(os.Stdin)

// AssumeYes is set by the global --yes / -y flag: every confirmation is treated as y.
// Why it exists: apply / promote --all / clean all stop to ask, and without this switch
// they could never run in CI or scripts — "stdin happens to hit EOF" is not a behavior
// you can put in the docs.
var AssumeYes bool

// IsTTY reports whether stdout is a terminal (when it is not, status prints no menu).
// Uses a real isatty (see tty_*.go), not ModeCharDevice — /dev/null is a character device too.
func IsTTY() bool {
	return isTerminal(os.Stdout)
}

// IsStdinTTY reports whether stdin is a terminal (i.e. whether anyone can answer questions).
// Prompts look at stdin: when output is redirected to a file (agsy apply > log) a human is
// still present, and that must not be treated as non-interactive.
func IsStdinTTY() bool {
	return isTerminal(os.Stdin)
}

// Confirm asks y/N; the default is No.
func Confirm(msg string) bool {
	if AssumeYes {
		fmt.Printf(i18n.T("%s (y/N) y (--yes assumed)\n"), msg)
		return true
	}
	if !IsStdinTTY() {
		fmt.Printf(i18n.T("%s (y/N)\n"), msg)
		fmt.Println(i18n.T("  ✘ Non-interactive and --yes not given; treating as cancelled."))
		return false
	}
	fmt.Printf(i18n.T("%s (y/N) "), msg)
	line, _ := reader.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes"
}

// Input reads a line of text with a default value.
func Input(msg, def string) string {
	if def != "" {
		fmt.Printf(i18n.T("%s (default: %s): "), msg, def)
	} else {
		fmt.Printf("%s: ", msg)
	}
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

// Select presents a numbered single-choice menu and returns the chosen index.
// When nobody can answer (non-interactive / --yes) it takes the default value
// right away instead of spinning while waiting for input.
func Select(msg string, options []string, defIdx int) int {
	fmt.Println(msg)
	if AssumeYes || !IsStdinTTY() {
		fmt.Printf(i18n.T("  (non-interactive; using default: %s)\n"), options[defIdx])
		return defIdx
	}
	for i, o := range options {
		mark := "  "
		if i == defIdx {
			mark = "❯ "
		}
		fmt.Printf("  %s%d) %s\n", mark, i+1, o)
	}
	for {
		fmt.Printf(i18n.T("Enter a number (Enter = %d): "), defIdx+1)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return defIdx
		}
		n, err := strconv.Atoi(line)
		if err == nil && n >= 1 && n <= len(options) {
			return n - 1
		}
		fmt.Println(i18n.T("  Invalid input; please try again"))
	}
}

// MultiSelect presents a numbered multi-choice menu (space-separated; a = all).
// When preselected is non-empty, Enter keeps the current selection (init edit mode);
// otherwise Enter = select all.
func MultiSelect(msg string, options []string, preselected []int) []int {
	mark := map[int]bool{}
	for _, i := range preselected {
		mark[i] = true
	}
	tail := i18n.T("Enter = all")
	if len(preselected) > 0 {
		tail = i18n.T("Enter = keep current selection")
	}
	fmt.Println(msg + i18n.T(" (space-separate multiple numbers, a = all, ") + tail + ")")
	for i, o := range options {
		flag := "  "
		if mark[i] {
			flag = "[x]"
		}
		fmt.Printf("    %s %d) %s\n", flag, i+1, o)
	}
	if AssumeYes || !IsStdinTTY() {
		// Nobody can answer (or --yes already answered): never spin on input.
		if len(preselected) > 0 {
			fmt.Println(i18n.T("  (non-interactive; keeping current selection)"))
			return append([]int{}, preselected...)
		}
		if AssumeYes {
			// --yes on a selection question accepts the default (= all),
			// consistent with Select taking its default value.
			fmt.Println(i18n.T("  (--yes given; selecting all)"))
			all := make([]int, len(options))
			for i := range options {
				all[i] = i
			}
			return all
		}
		// Non-interactive without --yes and nothing preselected: cancel.
		// Falling through would read EOF as an empty line and select ALL —
		// the promote menu would then silently write sources back, breaking
		// the "needs confirmation → cancelled, never forced" promise.
		fmt.Println(i18n.T("  ✘ Non-interactive and --yes not given; treating as cancelled."))
		return nil
	}
	for {
		fmt.Print(i18n.T("Enter your choice: "))
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" && len(preselected) > 0 {
			return append([]int{}, preselected...)
		}
		if line == "" || strings.ToLower(line) == "a" {
			all := make([]int, len(options))
			for i := range options {
				all[i] = i
			}
			return all
		}
		var picked []int
		ok := true
		for _, tok := range strings.Fields(line) {
			n, err := strconv.Atoi(tok)
			if err != nil || n < 1 || n > len(options) {
				ok = false
				break
			}
			picked = append(picked, n-1)
		}
		if ok && len(picked) > 0 {
			return picked
		}
		fmt.Println(i18n.T("  Invalid input; please try again"))
	}
}

// Pause waits for Enter before finishing.
func Pause() {
	fmt.Print(i18n.T("\nPress Enter to exit..."))
	_, _ = reader.ReadString('\n')
}

// PauseIfDoubleClicked keeps the console from vanishing in the Windows
// double-click scenario. It only pauses when the console was opened
// just for this program: when run from an existing terminal or CI, an
// extra Enter press would only be an annoyance.
func PauseIfDoubleClicked() {
	if !IsTTY() || !launchedByDoubleClick() {
		return
	}
	Pause()
}
