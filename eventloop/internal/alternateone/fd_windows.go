package alternateone
//go:build windows

package alternateone

// closeFD is a no-op on Windows (wake FDs are not used).
func closeFD(fd int) error {
	return nil
}

// closeWakeFDs is a no-op on Windows (wake FDs are not used).

func closeWakeFDs(readFd, writeFd int) {}