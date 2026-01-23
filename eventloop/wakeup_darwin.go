//go:build darwin

package eventloop

import (
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	EFD_CLOEXEC  = unix.O_CLOEXEC
	EFD_NONBLOCK = unix.O_NONBLOCK
)

// createWakeFd creates a self-pipe for wake-up notifications (Darwin).
// Returns the read end and the write end of the pipe.
// Note: initval and flags parameters are ignored on Darwin (API compatibility with Linux eventfd).
func createWakeFd(initval uint, flags int) (int, int, error) {
	_ = initval
	_ = flags

	var fds [2]int
	if err := syscall.Pipe(fds[:]); err != nil {
		return 0, 0, err
	}

	// On failure, close both pipe ends to avoid resource leak
	cleanup := func() {
		syscall.Close(fds[0])
		syscall.Close(fds[1])
	}

	syscall.CloseOnExec(fds[0])
	syscall.CloseOnExec(fds[1])

	if err := syscall.SetNonblock(fds[0], true); err != nil {
		cleanup()
		return 0, 0, err
	}
	if err := syscall.SetNonblock(fds[1], true); err != nil {
		cleanup()
		return 0, 0, err
	}

	return fds[0], fds[1], nil
}

// getWakeReadFd returns the loop's wake pipe read fd.
// Loop is passed as parameter because this file has no access to loop instance.
func getWakeReadFd() int {
	return -1
}

// flushPipe drains the wake pipe (internal helper).
func drainWakeUpPipe() error {
	// Implementation uses loop.drainWakeUpPipe() method instead
	return nil
}

// isWakeFdSupported returns true.
func isWakeFdSupported() bool {
	return true
}

// closeWakeFd closes wake pipe fds.
func closeWakeFd(wakeFd, wakeWriteFd int) error {
	if wakeFd >= 0 {
		_ = syscall.Close(wakeFd)
	}
	if wakeWriteFd >= 0 && wakeWriteFd != wakeFd {
		_ = syscall.Close(wakeWriteFd)
	}
	return nil
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
