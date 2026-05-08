//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"os"
	"sync"
	"testing"
)

func registerFileCleanupT(t testing.TB, files ...*os.File) {
	t.Helper()
	t.Cleanup(func() {
		for _, file := range files {
			if file == nil {
				continue
			}
			fd := file.Fd()
			if err := file.Close(); err != nil {
				t.Errorf("close file %q descriptor %d: %v", file.Name(), fd, err)
			}
		}
	})
}

// testCreateIOFD creates a pipe descriptor suitable for readiness registration.
func testCreateIOFD(t *testing.T) (fd int, cleanup func()) {
	t.Helper()
	pipeR, pipeW, err := os.Pipe()
	if err != nil {
		t.Fatal("os.Pipe failed:", err)
	}
	var cleanupOnce sync.Once
	return int(pipeR.Fd()), func() {
		t.Helper()
		cleanupOnce.Do(func() {
			if err := pipeR.Close(); err != nil {
				t.Errorf("close I/O read descriptor: %v", err)
			}
			if err := pipeW.Close(); err != nil {
				t.Errorf("close I/O write descriptor: %v", err)
			}
		})
	}
}
