//go:build windows

package alternateone

// EfdCloexec and EfdNonblock are no-ops on Windows.
const (
	EfdCloexec  = 0
	EfdNonblock = 0
)

// createWakeFd returns sentinel values on Windows.
// Wakeup is handled via IOCP PostQueuedCompletionStatus instead of pipes.
func createWakeFd(initval uint, flags int) (int, int, error) {
	return -1, -1, nil
}
