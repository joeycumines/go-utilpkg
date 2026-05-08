//go:build darwin

package eventloop

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// createWakeFD creates the nonblocking, close-on-exec self-pipe used to wake kqueue.
func createWakeFD() (int, int, error) {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()

	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		return -1, -1, err
	}
	if _, err := unix.FcntlInt(uintptr(fds[0]), unix.F_SETFD, unix.FD_CLOEXEC); err != nil {
		return -1, -1, joinErrors(err, closeWakeFDs(fds[0], fds[1]))
	}
	if _, err := unix.FcntlInt(uintptr(fds[1]), unix.F_SETFD, unix.FD_CLOEXEC); err != nil {
		return -1, -1, joinErrors(err, closeWakeFDs(fds[0], fds[1]))
	}
	if err := unix.SetNonblock(fds[0], true); err != nil {
		return -1, -1, joinErrors(err, closeWakeFDs(fds[0], fds[1]))
	}
	if err := unix.SetNonblock(fds[1], true); err != nil {
		return -1, -1, joinErrors(err, closeWakeFDs(fds[0], fds[1]))
	}
	return fds[0], fds[1], nil
}
