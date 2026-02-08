//go:build linux || darwin

package alternateone

import "golang.org/x/sys/unix"

// closeFD closes a file descriptor.
func closeFD(fd int) error {
	return unix.Close(fd)
}

// closeWakeFDs closes the wakeup file descriptors.
func closeWakeFDs(readFd, writeFd int) {
	_ = closeFD(readFd)
	if writeFd != readFd {
		_ = closeFD(writeFd)
	}
}
