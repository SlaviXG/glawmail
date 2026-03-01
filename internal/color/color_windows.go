//go:build windows

package color

import (
	"golang.org/x/sys/windows"
	"golang.org/x/term"
)

// platformColorSupported enables Virtual Terminal Processing on Windows 10+
// and returns true if it succeeded. On older Windows (7/8) the syscall fails
// and we return false so the wizard falls back to plain text.
//
// VTP is the native ANSI support built into conhost.exe since Windows 10 1511.
// It is also automatically supported in Windows Terminal, VS Code terminal, and
// every modern terminal emulator on Windows.
func platformColorSupported() bool {
	if !term.IsTerminal(int(outFd())) {
		return false
	}

	stdout := windows.Handle(outFd())

	var originalMode uint32
	if err := windows.GetConsoleMode(stdout, &originalMode); err != nil {
		return false // not a console handle
	}

	// ENABLE_VIRTUAL_TERMINAL_PROCESSING = 0x0004
	const vtProcessing = windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
	newMode := originalMode | vtProcessing

	if err := windows.SetConsoleMode(stdout, newMode); err != nil {
		// Windows 7/8: SetConsoleMode rejects the VTP flag.
		// Fall back to plain output rather than crashing.
		return false
	}
	return true
}
