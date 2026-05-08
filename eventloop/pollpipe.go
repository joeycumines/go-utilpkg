//go:build (aix && ppc64) || (solaris && amd64)

package eventloop

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func createPollPipeLegacy() (int, int, error) {
	syscall.ForkLock.RLock()
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		syscall.ForkLock.RUnlock()
		return -1, -1, err
	}
	if _, err := unix.FcntlInt(uintptr(fds[0]), unix.F_SETFD, unix.FD_CLOEXEC); err != nil {
		closeErr := closePollPipe(fds[0], fds[1])
		syscall.ForkLock.RUnlock()
		return -1, -1, joinErrors(err, closeErr)
	}
	if _, err := unix.FcntlInt(uintptr(fds[1]), unix.F_SETFD, unix.FD_CLOEXEC); err != nil {
		closeErr := closePollPipe(fds[0], fds[1])
		syscall.ForkLock.RUnlock()
		return -1, -1, joinErrors(err, closeErr)
	}
	syscall.ForkLock.RUnlock()

	if err := unix.SetNonblock(fds[0], true); err != nil {
		return -1, -1, joinErrors(err, closePollPipe(fds[0], fds[1]))
	}
	if err := unix.SetNonblock(fds[1], true); err != nil {
		return -1, -1, joinErrors(err, closePollPipe(fds[0], fds[1]))
	}
	return fds[0], fds[1], nil
}

func closePollPipe(readFD, writeFD int) error {
	var err error
	if readFD >= 0 {
		err = unix.Close(readFD)
	}
	if writeFD >= 0 && writeFD != readFD {
		err = joinErrors(err, unix.Close(writeFD))
	}
	return err
}
