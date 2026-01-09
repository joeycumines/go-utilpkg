//go:build linux

package eventloop

import "golang.org/x/sys/unix"

// SabotagePoller closes the internal epoll/kqueue FD.
// This guarantees the NEXT call to pollIO() will return EBADF.
func sabotagePoller(l *Loop) {
	l.ioPoller.mu.Lock()
	defer l.ioPoller.mu.Unlock()

	// Linux
	if l.ioPoller.epfd > 0 {
		unix.Close(l.ioPoller.epfd)
		// CRITICAL: Do NOT set initialized = false or -1.
		// We want pollIO to attempt using the closed FD to trigger the error.
	}
}
