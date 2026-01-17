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
// Note: initval and flags parameters are ignored on Darwin (API compatibility with Linux eventfd).
func createWakeFd(initval uint, flags int) (int, int, error) {
	_ = initval // unused on Darwin (eventfd compatibility)
	_ = flags   // unused on Darwin (pipe is always non-blocking and close-on-exec)

	// Create a pipe for wake-up
	var fds [2]int
	if err := syscall.Pipe(fds[:]); err != nil {
		return 0, 0, err
	}

	// Set close-on-exec and non-blocking flags with proper error handling
	// On failure, close both pipe ends to avoid resource leak
	cleanup := func() {
		syscall.Close(fds[0])
		syscall.Close(fds[1])
	}

	syscall.CloseOnExec(fds[0])
	syscall.CloseOnExec(fds[1])

	if err := syscall.SetNonblock(fds[0], true); err != nil {
		cleanup()
		return 0, 0, err
	}
	if err := syscall.SetNonblock(fds[1], true); err != nil {
		cleanup()
		return 0, 0, err
	}

	// Return read end (0) and write end (1)
	return fds[0], fds[1], nil
}
