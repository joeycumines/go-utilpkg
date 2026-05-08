//go:build plan9 || windows || ((js || wasip1) && wasm)

package eventloop

func closeFD(int) error {
	return nil
}

func readFD(int, []byte) (int, error) {
	return 0, ErrReadinessUnsupported
}

func writeFD(int, []byte) (int, error) {
	return 0, ErrReadinessUnsupported
}

func wakeIOInterrupted(error) bool { return false }

func wakeWritePending(error) bool { return false }

func wakeReadComplete(error) bool { return false }

func createWakeFD() (int, int, error) {
	return -1, -1, nil
}
