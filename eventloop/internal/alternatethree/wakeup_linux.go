//go:build linux

package alternatethree

import (
	"golang.org/x/sys/unix"
)

const (
	EfdCloexec  = unix.EFD_CLOEXEC
	EfdNonblock = unix.EFD_NONBLOCK
)

// createWakeFd creates an eventfd for wake-up notifications (Linux).
// Returns the single eventfd as both read and write ends.
func createWakeFd(initval uint, flags int) (int, int, error) {
	fd, err := unix.Eventfd(initval, flags)
	return fd, fd, err
}
