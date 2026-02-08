//go:build windows

//lint:file-ignore U1000 Platform-specific stub functions (required for cross-platform compilation symmetry)

package eventloop

import "golang.org/x/sys/windows"

// EFD_CLOEXEC and EFD_NONBLOCK are Unix eventfd flags.
// On Windows these are unused (createWakeFd ignores flags) but must be
// defined so that loop.go's createWakeFd(0, EFD_CLOEXEC|EFD_NONBLOCK)
// compiles on all platforms.
const (
	EFD_CLOEXEC  = 0
	EFD_NONBLOCK = 0
)

// createWakeFd creates a dummy wake mechanism for Windows.
//
// WINDOWS IMPLEMENTATION NOTES:
// Windows IOCP does NOT use eventfd or pipes for wake-up.
// Instead, we use PostQueuedCompletionStatus to post NULL completions
// to the IOCP port, causing GetQueuedCompletionStatus to return immediately.
//
// This is the standard, correct pattern for IOCP wake-up on Windows.
// No actual file descriptors are needed - just the IOCP handle itself.
//
// Returns -1, -1, nil to indicate no wake FDs are needed.
// The loop.go code checks for negative wakeFd and skips wake pipe registration.
func createWakeFd(initval uint, flags int) (int, int, error) {
	// Windows IOCP uses PostQueuedCompletionStatus for wake-up
	// No pipe or eventfd needed
	return -1, -1, nil
}

// closeWakeFd closes the Windows wake mechanism.
func closeWakeFd(wakeFd, wakeWriteFd int) error {
	// No-op on Windows - no FDs to close
	return nil
}

// isWakeFdSupported returns false on Windows (no Unix wake mechanism).
func isWakeFdSupported() bool {
	// Windows uses PostQueuedCompletionStatus instead of eventfd/pipe
	return false
}

// drainWakeUpPipe drains wake-up notifications on Windows.
// No-op on Windows - PostQueuedCompletionStatus doesn't consume data.
func drainWakeUpPipe() error {
	// No-op on Windows
	return nil
}

// submitGenericWakeup wakes up the poller using PostQueuedCompletionStatus.
//
// This function is called from loop.go's submitWakeup() when
// l.wakePipe < 0 (indicating Windows/IOCP platform).
// It posts a NULL completion to the IOCP handle, which causes
// GetQueuedCompletionStatus to return immediately with overlapped==nil.
//
// This is the standard, correct wake-up mechanism for Windows IOCP.
func submitGenericWakeup(iocpHandle uintptr) error {
	return windows.PostQueuedCompletionStatus(
		windows.Handle(iocpHandle),
		0,   // bytesTransferred
		0,   // completionKey
		nil, // overlapped
	)
}
