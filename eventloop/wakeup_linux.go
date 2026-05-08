//go:build linux

package eventloop

import "golang.org/x/sys/unix"

// createWakeFD creates the nonblocking, close-on-exec eventfd used to wake epoll.
func createWakeFD() (int, int, error) {
	fd, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	return fd, fd, err
}
