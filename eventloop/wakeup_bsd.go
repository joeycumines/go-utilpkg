//go:build dragonfly || freebsd || netbsd || openbsd

package eventloop

import "golang.org/x/sys/unix"

// createWakeFD creates the atomically nonblocking, close-on-exec self-pipe used
// to interrupt kqueue.
func createWakeFD() (int, int, error) {
	var fds [2]int
	if err := unix.Pipe2(fds[:], unix.O_CLOEXEC|unix.O_NONBLOCK); err != nil {
		return -1, -1, err
	}
	return fds[0], fds[1], nil
}
