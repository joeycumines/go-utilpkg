//go:build solaris && amd64

package eventloop

import (
	"errors"

	"golang.org/x/sys/unix"
)

func duplicatePollDescriptor(fd int) (int, error) {
	duplicate, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 0)
	if err == nil {
		return duplicate, nil
	}
	if !errors.Is(err, unix.EINVAL) && !errors.Is(err, unix.ENOSYS) {
		return -1, err
	}
	return duplicatePollDescriptorLegacy(fd)
}
