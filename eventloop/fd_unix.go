//go:build linux || darwin

package eventloop

import (
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
