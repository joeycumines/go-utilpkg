//go:build windows

package eventloop

import (
	"errors"
)

// closeFD is a no-op on Windows since wake FDs don't exist.
// The Windows IOCP implementation uses poller.Close() to clean up.
func closeFD(fd int) error {
	// On Windows, wakePipe is -1, so this should never be called
	// with a valid FD for the wake mechanism
	if fd >= 0 {
		return errors.New("closeFD not supported on Windows for wake mechanism")
	}
	return nil
}

// readFD is a no-op on Windows since wake FDs don't exist.
// The Windows IOCP implementation uses PostQueuedCompletionStatus for wake-up.
func readFD(fd int, buf []byte) (int, error) {
	return 0, nil
}

// writeFD is a no-op on Windows since wake FDs don't exist.
// The Windows IOCP implementation uses PostQueuedCompletionStatus for wake-up.
func writeFD(fd int, buf []byte) (int, error) {
	return 0, nil
}
