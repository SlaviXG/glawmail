//go:build !windows

package color

import "golang.org/x/term"

// platformColorSupported returns true when stdout is an actual TTY.
// On Linux/macOS this is the only check needed; the kernel handles ANSI natively.
func platformColorSupported() bool {
	return term.IsTerminal(int(outFd()))
}
