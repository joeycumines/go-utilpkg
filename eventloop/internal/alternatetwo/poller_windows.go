//go:build windows

package alternatetwo

import (
	"errors"
	"sync/atomic"
	"syscall"

	"golang.org/x/sys/windows"
)

// Maximum file descriptor we support with direct indexing.
const maxFDs = 65536

// IOEvents represents the type of I/O events to monitor.
type IOEvents uint32

const (
	EventRead IOEvents = 1 << iota
	EventWrite
	EventError
	EventHangup
)

// Standard errors.
var (
	ErrFDOutOfRange        = errors.New("alternatetwo: fd out of range (max 65535)")
	ErrFDAlreadyRegistered = errors.New("alternatetwo: fd already registered")
	ErrFDNotRegistered     = errors.New("alternatetwo: fd not registered")
	ErrPollerClosed        = errors.New("alternatetwo: poller closed")
)

// IOCallback is the callback type for I/O events.
type IOCallback func(IOEvents)

// fdInfo stores per-FD callback information.
type fdInfo struct {
	callback IOCallback
	events   IOEvents
	active   bool
}

// FastPoller manages I/O event registration using IOCP (Windows).
//
// PERFORMANCE: Uses direct FD indexing and lock-free design where possible.
type FastPoller struct { // betteralign:ignore
	_       [64]byte       // Cache line padding //nolint:unused
	iocp    windows.Handle // IOCP handle
	_       [56]byte       // Pad to cache line //nolint:unused
	version atomic.Uint64  // Version counter for consistency
	_       [56]byte       // Pad to cache line //nolint:unused
	fds     [maxFDs]fdInfo // Direct indexing, no map
	closed  atomic.Bool    // Closed flag
}

// Init initializes the IOCP instance.
func (p *FastPoller) Init() error {
	if p.closed.Load() {
		return ErrPollerClosed
	}

	iocp, err := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 0)
	if err != nil {
		return err
	}
	p.iocp = iocp
	return nil
}

// Close closes the IOCP instance.
func (p *FastPoller) Close() error {
	p.closed.Store(true)
	if p.iocp != 0 {
		err := windows.CloseHandle(p.iocp)
		p.iocp = 0
		return err
	}
	return nil
}

// RegisterFD registers a file descriptor for I/O event monitoring.
func (p *FastPoller) RegisterFD(fd int, events IOEvents, cb IOCallback) error {
	if p.closed.Load() {
		return ErrPollerClosed
	}
	if fd < 0 || fd >= maxFDs {
		return ErrFDOutOfRange
	}
	if p.fds[fd].active {
		return ErrFDAlreadyRegistered
	}

	p.fds[fd] = fdInfo{callback: cb, events: events, active: true}
	p.version.Add(1)

	// Associate handle with IOCP
	handle := windows.Handle(fd)
	_, err := windows.CreateIoCompletionPort(handle, p.iocp, uintptr(fd), 0)
	if err != nil {
		p.fds[fd] = fdInfo{} // Rollback
		return err
	}
	return nil
}

// UnregisterFD removes a file descriptor from monitoring.
func (p *FastPoller) UnregisterFD(fd int) error {
	if fd < 0 || fd >= maxFDs {
		return ErrFDOutOfRange
	}
	if !p.fds[fd].active {
		return ErrFDNotRegistered
	}

	p.fds[fd] = fdInfo{}
	p.version.Add(1)
	return nil
}

// ModifyFD updates the events being monitored for a file descriptor.
func (p *FastPoller) ModifyFD(fd int, events IOEvents) error {
	if fd < 0 || fd >= maxFDs {
		return ErrFDOutOfRange
	}
	if !p.fds[fd].active {
		return ErrFDNotRegistered
	}

	p.fds[fd].events = events
	p.version.Add(1)
	return nil
}

// PollIO polls for I/O events using IOCP.
func (p *FastPoller) PollIO(timeoutMs int) (int, error) {
	if p.closed.Load() {
		return 0, ErrPollerClosed
	}

	var timeout uint32
	if timeoutMs >= 0 {
		timeout = uint32(timeoutMs)
	}

	var bytes uint32
	var key uintptr
	var overlapped *windows.Overlapped

	err := windows.GetQueuedCompletionStatus(p.iocp, &bytes, &key, &overlapped, timeout)
	if err != nil {
		if errno, ok := err.(syscall.Errno); ok {
			if errno == windows.WAIT_TIMEOUT {
				return 0, nil
			}
			if errno == windows.ERROR_ABANDONED_WAIT_0 || errno == windows.ERROR_INVALID_HANDLE {
				return 0, ErrPollerClosed
			}
		}
		return 0, nil
	}

	if overlapped == nil && key == 0 {
		// Wake-up notification
		return 0, nil
	}

	// Dispatch inline
	fd := int(key)
	if fd >= 0 && fd < maxFDs {
		info := &p.fds[fd]
		if info.active && info.callback != nil {
			info.callback(info.events)
			return 1, nil
		}
	}

	return 0, nil
}

// Wakeup wakes up the poller using PostQueuedCompletionStatus.
func (p *FastPoller) Wakeup() error {
	if p.closed.Load() {
		return ErrPollerClosed
	}
	return windows.PostQueuedCompletionStatus(p.iocp, 0, 0, nil)
}
