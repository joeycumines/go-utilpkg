//go:build solaris && amd64

package eventloop

import (
	"errors"

	"golang.org/x/sys/unix"
)

func createPollPipe() (int, int, error) {
	var fds [2]int
	err := unix.Pipe2(fds[:], unix.O_CLOEXEC|unix.O_NONBLOCK)
	if err == nil {
		return fds[0], fds[1], nil
	}
	if !errors.Is(err, unix.EINVAL) && !errors.Is(err, unix.ENOSYS) {
		return -1, -1, err
	}
	return createPollPipeLegacy()
}
