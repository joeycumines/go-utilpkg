//go:build windows

package eventloop

import (
	"syscall"
	"testing"
)

// testCreateIOFD creates a file descriptor suitable for RegisterFD.
// On Windows (IOCP), only socket handles can be associated with IOCP.
// Pipe handles (from os.Pipe) are NOT supported by CreateIoCompletionPort.
// This creates a raw TCP socket not managed by Go's internal IOCP.
func testCreateIOFD(t *testing.T) (fd int, cleanup func()) {
	t.Helper()
	s, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal("syscall.Socket failed:", err)
	}
	return int(s), func() {
		syscall.Closesocket(s)
	}
}
