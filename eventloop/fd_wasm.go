//go:build js && wasm

package eventloop

// WASM-specific FD operations - all stub implementations

// closeFD is a stub on WASM (no real file descriptors)
func closeFD(fd int) error {
	return nil
}

// readFD is a stub on WASM (no real file descriptors)
func readFD(fd int, buf []byte) (int, error) {
	return 0, nil
}

// writeFD is a stub on WASM (no real file descriptors)
func writeFD(fd int, buf []byte) (int, error) {
	return 0, nil
}

// WASM constants for eventfd (stub values)
const (
	efdCloexec  = 0
	efdNonblock = 0
)

// createWakeFd is a stub on WASM (no eventfd)
func createWakeFd(initval uint, flags int) (int, int, error) {
	// Return -1 for both file descriptors to indicate no wake mechanism
	return -1, -1, nil
}

// closeWakeFd is a stub on WASM
func closeWakeFd(wakeFd, wakeWriteFd int) error {
	return nil
}

// isWakeFdSupported returns false on WASM
func isWakeFdSupported() bool {
	return false
}

// drainWakeUpPipe is a stub on WASM
func drainWakeUpPipe() error {
	return nil
}

// getWakeReadFd returns -1 on WASM (no wake pipe)
func getWakeReadFd() int {
	return -1
}

// submitGenericWakeup is a stub on WASM
func submitGenericWakeup(_ uintptr) error {
	return nil
}
