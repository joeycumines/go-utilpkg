//go:build darwin

package eventloop

import (
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	EFD_CLOEXEC  = unix.O_CLOEXEC
	EFD_NONBLOCK = unix.O_NONBLOCK
)

// createWakeFd creates a self-pipe for wake-up notifications (Darwin).
// Returns the read end and the write end of the pipe.
func createWakeFd(initval uint, flags int) (int, int, error) {
	// Create a pipe for wake-up (flags parameter is ignored for pipe)
	var fds [2]int
	err := syscall.Pipe(fds[:])
	if err != nil {
		return 0, 0, err
	}

	// Set non-blocking and close-on-exec flags
	syscall.CloseOnExec(fds[0])
	syscall.CloseOnExec(fds[1])

	// Set non-blocking
	syscall.SetNonblock(fds[0], true)
	syscall.SetNonblock(fds[1], true)

	// Return read end (0) and write end (1)
	return fds[0], fds[1], nil
}

// getWakePipeWriteFd is a helper for platforms where write FD is separate.
// But now we return it directly from createWakeFd.
// Keeping this for compatibility if needed, but likely we can remove/ignore.
//
//lint:ignore U1000 Will be used for future per-loop write fd management
func getWakePipeWriteFd(wakePipe int) int {
	return 0 // Dummy implementation, we manage write FD in Loop struct
}
