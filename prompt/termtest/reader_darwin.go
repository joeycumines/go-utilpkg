//go:build darwin

package termtest

import (
	"errors"
	"syscall"

	"golang.org/x/sys/unix"
)

type ptyOpsUnixKevent_t = unix.Kevent_t
type ptyOpsEpollEvent_t = any

func (x *ptyOps) init() {
	x.kqueue = unix.Kqueue
	x.kevent = unix.Kevent
}

// initPoller initializes a kqueue poller for the pty reader.
func (r *ptyReader) initPoller() error {
	kq, err := r.ops.kqueue()
	if err != nil {
		return err
	}
	r.pollFD = kq

	// Create wake pipe to reliably wake the kqueue waiter across threads
	var fds [2]int
	if err := r.ops.pipe(fds[:]); err != nil {
		_ = r.ops.closeFD(r.pollFD)
		r.pollFD = -1
		return err
	}
	r.wakeR = fds[0]
	r.wakeW = fds[1]

	// Register the Read event for the PTY file descriptor
	events := []unix.Kevent_t{
		{
			Ident:  uint64(r.fd),
			Filter: unix.EVFILT_READ,
			Flags:  unix.EV_ADD | unix.EV_ENABLE,
		},
	}

	// Apply the registration for the PTY FD
	_, err = r.ops.kevent(r.pollFD, events, nil, nil)
	if err != nil {
		_ = r.ops.closeFD(r.pollFD)
		r.pollFD = -1
		_ = r.ops.closeFD(r.wakeR)
		_ = r.ops.closeFD(r.wakeW)
		r.wakeR = -1
		r.wakeW = -1
		return err
	}

	// Register the wake pipe read end
	wakeEvents := []unix.Kevent_t{{
		Ident:  uint64(r.wakeR),
		Filter: unix.EVFILT_READ,
		Flags:  unix.EV_ADD | unix.EV_ENABLE,
	}}
	_, err = r.ops.kevent(r.pollFD, wakeEvents, nil, nil)
	if err != nil {
		_ = r.ops.closeFD(r.pollFD)
		r.pollFD = -1
		_ = r.ops.closeFD(r.wakeR)
		_ = r.ops.closeFD(r.wakeW)
		r.wakeR = -1
		r.wakeW = -1
		return err
	}
	return err
}

// closePoller closes the kqueue fd.
func (r *ptyReader) closePoller() error {
	var errs []error
	if r.pollFD >= 0 {
		if err := r.ops.closeFD(r.pollFD); err != nil {
			errs = append(errs, err)
		}
		r.pollFD = -1
	}
	if r.wakeR >= 0 {
		if err := r.ops.closeFD(r.wakeR); err != nil {
			errs = append(errs, err)
		}
		r.wakeR = -1
	}
	if r.wakeW >= 0 {
		if err := r.ops.closeFD(r.wakeW); err != nil {
			errs = append(errs, err)
		}
		r.wakeW = -1
	}
	if len(errs) > 0 {
		return errs[0] // Return first error
	}
	return nil
}

// waitForRead blocks until the FD is readable or a wake event occurs.
// It uses an infinite timeout to eliminate busy-waiting.
func (r *ptyReader) waitForRead() error {
	var events [2]unix.Kevent_t
	// nil timespec means block indefinitely
	n, err := r.ops.kevent(r.pollFD, nil, events[:], nil)
	if err != nil {
		// Interrupted system call is not a fatal error
		if err == syscall.EINTR {
			return nil
		}
		return err
	}

	if n > 0 {
		for i := 0; i < n; i++ {
			if int(events[i].Ident) == r.wakeR {
				var buf [128]byte
				_, _ = r.ops.read(r.wakeR, buf[:])
			}
		}
	}
	return nil
}

// shouldInterpretAsEOF determines if a specific read error should be treated as EOF on Darwin.
// On macOS, reading from a master PTY often returns EIO when the slave side is closed.
func (r *ptyReader) shouldInterpretAsEOF(err error) bool {
	return errors.Is(err, syscall.EIO)
}
