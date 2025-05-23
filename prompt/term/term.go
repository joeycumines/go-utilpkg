//go:build !windows

package term

import (
	"sync"

	"github.com/pkg/term/termios"
	"golang.org/x/sys/unix"
)

var (
	saveTermios     unix.Termios
	saveTermiosErr  error
	saveTermiosFD   int
	saveTermiosOnce sync.Once
)

func getOriginalTermios(fd int) (*unix.Termios, error) {
	saveTermiosOnce.Do(func() {
		saveTermiosFD = fd
		var v *unix.Termios
		v, saveTermiosErr = termios.Tcgetattr(uintptr(fd))
		if saveTermiosErr == nil {
			// save a copy
			saveTermios = *v
		}
	})
	if saveTermiosErr != nil {
		return nil, saveTermiosErr
	}
	// return a copy
	v := saveTermios
	return &v, nil
}

// Restore terminal's mode.
func Restore() error {
	return RestoreFD(saveTermiosFD)
}

// RestoreFD restores terminal's mode for the given file descriptor.
func RestoreFD(fd int) error {
	o, err := getOriginalTermios(fd)
	if err != nil {
		return err
	}
	return termios.Tcsetattr(uintptr(fd), termios.TCSANOW, o)
}
