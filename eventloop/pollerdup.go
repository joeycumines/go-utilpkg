//go:build (aix && ppc64) || (solaris && amd64)

package eventloop

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func duplicatePollDescriptorLegacy(fd int) (int, error) {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()

	duplicate, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD, 0)
	if err != nil {
		return -1, err
	}
	if _, err := unix.FcntlInt(uintptr(duplicate), unix.F_SETFD, unix.FD_CLOEXEC); err != nil {
		return -1, joinErrors(err, unix.Close(duplicate))
	}
	return duplicate, nil
}
