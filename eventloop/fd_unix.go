//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"errors"

	"golang.org/x/sys/unix"
)

// closeFD closes a file descriptor on Unix systems.
func closeFD(fd int) error {
	return unix.Close(fd)
}

// readFD reads from a file descriptor on Unix systems.
func readFD(fd int, buf []byte) (int, error) {
	return unix.Read(fd, buf)
}

// writeFD writes to a file descriptor on Unix systems.
func writeFD(fd int, buf []byte) (int, error) {
	return unix.Write(fd, buf)
}

func wakeIOInterrupted(err error) bool {
	return errors.Is(err, unix.EINTR)
}

func wakeWritePending(err error) bool {
	return errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK)
}

func wakeReadComplete(err error) bool {
	return errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK)
}
