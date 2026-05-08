//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"runtime"
	"testing"

	"golang.org/x/sys/unix"
)

func TestCreateWakeFDDescriptorContract(t *testing.T) {
	readFD, writeFD, err := createWakeFD()
	if err != nil {
		t.Fatalf("createWakeFD: %v", err)
	}
	t.Cleanup(func() {
		if readFD >= 0 {
			if err := unix.Close(readFD); err != nil {
				t.Errorf("close wake read descriptor: %v", err)
			}
		}
		if writeFD >= 0 && writeFD != readFD {
			if err := unix.Close(writeFD); err != nil {
				t.Errorf("close wake write descriptor: %v", err)
			}
		}
	})

	if readFD < 0 || writeFD < 0 {
		t.Fatalf("wake descriptors = (%d, %d), want nonnegative", readFD, writeFD)
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "android" {
		if readFD != writeFD {
			t.Fatalf("eventfd wake descriptors = (%d, %d), want one descriptor", readFD, writeFD)
		}
	} else if readFD == writeFD {
		t.Fatalf("%s wake descriptors = (%d, %d), want distinct pipe ends", runtime.GOOS, readFD, writeFD)
	}

	for _, descriptor := range []struct {
		name string
		fd   int
	}{
		{name: "read", fd: readFD},
		{name: "write", fd: writeFD},
	} {
		descriptorFlags, err := unix.FcntlInt(uintptr(descriptor.fd), unix.F_GETFD, 0)
		if err != nil {
			t.Fatalf("get %s descriptor flags: %v", descriptor.name, err)
		}
		if descriptorFlags&unix.FD_CLOEXEC == 0 {
			t.Errorf("%s descriptor flags = %#x, want FD_CLOEXEC", descriptor.name, descriptorFlags)
		}

		statusFlags, err := unix.FcntlInt(uintptr(descriptor.fd), unix.F_GETFL, 0)
		if err != nil {
			t.Fatalf("get %s status flags: %v", descriptor.name, err)
		}
		if statusFlags&unix.O_NONBLOCK == 0 {
			t.Errorf("%s status flags = %#x, want O_NONBLOCK", descriptor.name, statusFlags)
		}
	}
}
