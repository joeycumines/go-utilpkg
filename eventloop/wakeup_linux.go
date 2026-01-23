//go:build linux

//lint:file-ignore U1000 Platform-specific stub functions (required for Windows/Darwin compatibility)

package eventloop

import (
	"golang.org/x/sys/unix"
)

const (
	EFD_CLOEXEC  = unix.EFD_CLOEXEC
	EFD_NONBLOCK = unix.EFD_NONBLOCK
)

// createWakeFd creates an eventfd for wake-up notifications (Linux).
// Returns the single eventfd as both read and write ends.
func createWakeFd(initval uint, flags int) (int, int, error) {
	fd, err := unix.Eventfd(initval, flags)
	return fd, fd, err
}

// closeWakeFd closes the wake eventfd on Linux.
func closeWakeFd(wakeFd, wakeWriteFd int) error {
	if wakeFd >= 0 {
		_ = unix.Close(wakeFd)
	}
	return nil
}

// isWakeFdSupported returns true on Linux (eventfd mechanism).
func isWakeFdSupported() bool {
	return true
}

// drainWakeUpPipe drains the wake eventfd on Linux.
func drainWakeUpPipe() error {
	if wakeFd := getWakeReadFd(); wakeFd >= 0 {
		// Read all available wake-ups
		var buf [8]byte
		for {
			_, err := unix.Read(wakeFd, buf[:])
			if err != nil {
				break
			}
		}
	}
	return nil
}

// getWakeReadFd returns the loop's wake pipe read fd.
// Loop is passed as parameter because this file has no access to loop instance.
func getWakeReadFd() int {
	return -1
}

// submitGenericWakeup is a stub for Darwin/Linux.
// This function name exists on Windows for PostQueuedCompletionStatus.
// On Darwin/Linux, we write to the wake pipe instead.
//
// Note: This is never called because wakePipe >= 0
// on Darwin/Linux, so this is a safety stub only.
func submitGenericWakeup(_ uintptr) error {
	// Darwin/Linux: Write to wake pipe in submitWakeup()
	// This stub exists for function name compatibility with Windows
	return nil
}
