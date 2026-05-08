//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

func closeAmbientFDT(t testing.TB, fd int) {
	t.Helper()
	if err := unix.Close(fd); err != nil && !errors.Is(err, unix.EBADF) {
		t.Fatalf("close ambient descriptor %d: %v", fd, err)
	}
}

func closeTestFDT(t testing.TB, fd *int) {
	t.Helper()
	if *fd < 0 {
		return
	}
	descriptor := *fd
	*fd = -1
	if err := unix.Close(descriptor); err != nil {
		t.Errorf("close descriptor %d: %v", descriptor, err)
	}
}

func registerTestFDCleanupT(t testing.TB, fds ...*int) {
	t.Helper()
	t.Cleanup(func() {
		for _, fd := range fds {
			closeTestFDT(t, fd)
		}
	})
}
