//go:build darwin

package alternatetwo

import (
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	EfdCloexec  = unix.O_CLOEXEC
	EfdNonblock = unix.O_NONBLOCK
)

// createWakeFd creates a self-pipe for wake-up notifications (Darwin).
// Returns the read end and the write end of the pipe.
func createWakeFd(initval uint, flags int) (int, int, error) {
	var fds [2]int
	err := syscall.Pipe(fds[:])
	if err != nil {
		return 0, 0, err
	}

	// Set non-blocking and close-on-exec flags
	syscall.CloseOnExec(fds[0])
	syscall.CloseOnExec(fds[1])
	syscall.SetNonblock(fds[0], true)
	syscall.SetNonblock(fds[1], true)

	return fds[0], fds[1], nil
}
