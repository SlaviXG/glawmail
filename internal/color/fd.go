package color

import "os"

// outFd returns the file descriptor for stdout as a uintptr,
// suitable for passing to term.IsTerminal and the Windows console API.
func outFd() uintptr {
	return os.Stdout.Fd()
}
