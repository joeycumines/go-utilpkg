//go:build windows

package alternateone

// closeFD is a no-op on Windows (wake FDs are not used).
func closeFD(fd int) error {
	return nil
}

// closeWakeFDs closes wake file descriptors. No-op on Windows.
func closeWakeFDs(readFd, writeFd int) {
	_ = closeFD(readFd)
	if writeFd != readFd {
		_ = closeFD(writeFd)
	}
}
