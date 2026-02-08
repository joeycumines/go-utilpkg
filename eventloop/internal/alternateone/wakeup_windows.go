//go:build windows

package alternateone

// EFD_CLOEXEC and EFD_NONBLOCK are no-ops on Windows.
const (
	EFD_CLOEXEC  = 0
	EFD_NONBLOCK = 0
)

// createWakeFd returns sentinel values on Windows.
// Wakeup is handled via IOCP PostQueuedCompletionStatus instead of pipes.
func createWakeFd(initval uint, flags int) (int, int, error) {
	return -1, -1, nil
}
