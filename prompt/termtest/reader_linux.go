//go:build linux

package termtest

import (
	"errors"
	"io"
	"syscall"

	"golang.org/x/sys/unix"
)

type ptyOpsUnixKevent_t = any
type ptyOpsEpollEvent_t = unix.EpollEvent

func (x *ptyOps) init() {
	x.epollCreate1 = unix.EpollCreate1
	x.epollCtl = unix.EpollCtl
	x.epollWait = unix.EpollWait
}

// initPoller initializes an epoll poller for the pty reader.
func (r *ptyReader) initPoller() error {
	epfd, err := r.ops.epollCreate1(0)
	if err != nil {
		return err
	}
	r.pollFD = epfd

	// Create a wake pipe for reliable cross-thread wakeups
	var fds [2]int
	if err := r.ops.pipe(fds[:]); err != nil {
		_ = r.ops.closeFD(r.pollFD)
		r.pollFD = -1
		return err
	}
	r.wakeR = fds[0]
	r.wakeW = fds[1]

	// Register the file descriptor for read events
	event := unix.EpollEvent{
		Events: unix.EPOLLIN,
		Fd:     int32(r.fd),
	}

	if err := r.ops.epollCtl(r.pollFD, unix.EPOLL_CTL_ADD, r.fd, &event); err != nil {
		_ = r.ops.closeFD(r.pollFD)
		r.pollFD = -1
		_ = r.ops.closeFD(r.wakeR)
		_ = r.ops.closeFD(r.wakeW)
		r.wakeR = -1
		r.wakeW = -1
		return err
	}

	// Register wake pipe read end
	wakeEvent := unix.EpollEvent{Events: unix.EPOLLIN, Fd: int32(r.wakeR)}
	if err := r.ops.epollCtl(r.pollFD, unix.EPOLL_CTL_ADD, r.wakeR, &wakeEvent); err != nil {
		_ = r.ops.closeFD(r.pollFD)
		r.pollFD = -1
		_ = r.ops.closeFD(r.wakeR)
		_ = r.ops.closeFD(r.wakeW)
		r.wakeR = -1
		r.wakeW = -1
		return err
	}
	return nil
}

// closePoller closes the epoll fd.
func (r *ptyReader) closePoller() error {
	var firstErr error
	if r.pollFD >= 0 {
		if err := r.ops.closeFD(r.pollFD); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
		r.pollFD = -1
	}
	if r.wakeR >= 0 {
		if err := r.ops.closeFD(r.wakeR); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
		r.wakeR = -1
	}
	if r.wakeW >= 0 {
		if err := r.ops.closeFD(r.wakeW); err != nil {
			if firstErr == nil {
				firstErr = err
			}
		}
		r.wakeW = -1
	}
	return firstErr
}

// waitForRead blocks until the FD is readable or a wake event occurs.
// It uses an infinite timeout (-1) to eliminate busy-waiting.
func (r *ptyReader) waitForRead() error {
	var events [2]unix.EpollEvent
	// -1 timeout means block indefinitely until an event occurs
	n, err := r.ops.epollWait(r.pollFD, events[:], -1)
	if err != nil {
		if err == syscall.EINTR {
			return nil
		}
		return err
	}
	// Drain wake pipe if triggered
	if n > 0 {
		for i := 0; i < n; i++ {
			if int(events[i].Fd) == r.fd {
				if events[i].Events&(unix.EPOLLHUP|unix.EPOLLERR) != 0 {
					return io.EOF
				}
			}
			if int(events[i].Fd) == r.wakeR {
				var buf [128]byte
				_, _ = r.ops.read(r.wakeR, buf[:])
			}
		}
	}
	return nil
}

// shouldInterpretAsEOF determines if a specific read error should be treated as EOF on Linux.
func (r *ptyReader) shouldInterpretAsEOF(err error) bool {
	// On Linux, EIO is returned when the PTY master is closed.
	return errors.Is(err, syscall.EIO)
}
