// Package color provides cross-platform ANSI colour helpers for the setup wizard.
//
// Colour is enabled when ALL of the following are true:
//   - stdout is a terminal (not a pipe or redirect)
//   - the NO_COLOR env var is not set  (https://no-color.org/)
//   - on Windows, Virtual Terminal Processing was successfully enabled
//     (requires Windows 10 1511+ / Server 2016+; falls back cleanly)
//
// On older Windows (XP/7/8) colours are silently disabled so the wizard
// still works, it just prints plain text.
package color

import (
	"fmt"
	"os"
)

// enabled is set once at init time.
var enabled bool

func init() {
	enabled = isColorSupported()
}

// ── Public colour printers ────────────────────────────────────────────────────

// Info prints a cyan  "i  <msg>" line.
func Info(msg string) { printColored("\033[36m", "i  ", msg) }

// Ok prints a green  "✔  <msg>" line.
func Ok(msg string) { printColored("\033[32m", "✔  ", msg) }

// Warn prints a yellow "⚠  <msg>" line.
func Warn(msg string) { printColored("\033[33m", "⚠  ", msg) }

// Err prints a red    "✖  <msg>" line.
func Err(msg string) { printColored("\033[31m", "✖  ", msg) }

// Heading prints a bold section header followed by a rule.
func Heading(msg string) {
	fmt.Println()
	if enabled {
		fmt.Printf("\033[1m%s\033[0m\n%s\n", msg, rule(len(msg)))
	} else {
		fmt.Printf("%s\n%s\n", msg, rule(len(msg)))
	}
}

// Bold wraps s in bold ANSI codes (if colours are enabled).
func Bold(s string) string {
	if enabled {
		return "\033[1m" + s + "\033[0m"
	}
	return s
}

// ── helpers ───────────────────────────────────────────────────────────────────

func printColored(ansi, prefix, msg string) {
	if enabled {
		fmt.Printf("%s%s%s\033[0m\n", ansi, prefix, msg)
	} else {
		fmt.Printf("%s%s\n", prefix, msg)
	}
}

func rule(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = '-'
	}
	return string(b)
}

// isColorSupported returns true when the terminal can display ANSI colours.
// The platform-specific part (Windows VTP, isTerminal) lives in color_unix.go
// and color_windows.go.
func isColorSupported() bool {
	// Respect the NO_COLOR convention (https://no-color.org/).
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	// TERM=dumb means the terminal can't handle escape codes.
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return platformColorSupported()
}
