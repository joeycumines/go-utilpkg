//go:build !windows

package eventloop

import (
	"os"
	"testing"
)

// testCreateIOFD creates a file descriptor suitable for RegisterFD.
// On Darwin (kqueue) and Linux (epoll), pipe file descriptors are supported.
func testCreateIOFD(t *testing.T) (fd int, cleanup func()) {
	t.Helper()
	pipeR, pipeW, err := os.Pipe()
	if err != nil {
		t.Fatal("os.Pipe failed:", err)
	}
	return int(pipeR.Fd()), func() {
		pipeR.Close()
		pipeW.Close()
	}
}
