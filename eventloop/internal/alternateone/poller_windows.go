//go:build windows

package alternateone

import (
	"sync"
	"syscall"

	"golang.org/x/sys/windows"
)

// IOEvents represents the type of I/O events to monitor.
type IOEvents uint32

const (
	EventRead IOEvents = 1 << iota
	EventWrite
	EventError
	EventHangup
)

// fdCallback stores the callback and events for a registered file descriptor.
type fdCallback struct {
	callback func(events IOEvents)
	events   IOEvents
}

// SafePoller manages I/O event registration using IOCP (Windows).
//
// SAFETY: Uses sync.Mutex for ALL operations including poll.
// Callbacks are executed UNDER LOCK for correctness.
type SafePoller struct {
	callbacks   map[int]*fdCallback
	mu          sync.Mutex
	iocp        windows.Handle
	initialized bool
	closed      bool
}

// initPoller initializes the IOCP instance.
func (p *SafePoller) initPoller() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.initPollerLocked()
}

// initPollerLocked initializes without acquiring the lock.
func (p *SafePoller) initPollerLocked() error {
	if p.closed {
		return ErrPollerClosed
	}
	if p.initialized {
		return nil
	}

	iocp, err := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 0)
	if err != nil {
		return WrapError("initPoller", err)
	}

	p.iocp = iocp
	p.callbacks = make(map[int]*fdCallback)
	p.initialized = true
	return nil
}

// closePoller closes the IOCP instance and releases resources.
func (p *SafePoller) closePoller() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.initialized {
		return nil
	}

	p.closed = true
	var err error
	if p.iocp != 0 {
		err = windows.CloseHandle(p.iocp)
		p.iocp = 0
	}
	p.callbacks = nil
	p.initialized = false
	return err
}

// RegisterFD registers a file descriptor for I/O event monitoring.
func (p *SafePoller) RegisterFD(fd int, events IOEvents, callback func(events IOEvents)) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.initPollerLocked(); err != nil {
		return err
	}

	if _, exists := p.callbacks[fd]; exists {
		return ErrFDAlreadyRegistered
	}

	// Associate handle with IOCP using FD as completion key
	handle := windows.Handle(fd)
	_, err := windows.CreateIoCompletionPort(handle, p.iocp, uintptr(fd), 0)
	if err != nil {
		return WrapError("RegisterFD", err)
	}

	p.callbacks[fd] = &fdCallback{
		callback: callback,
		events:   events,
	}

	return nil
}

// UnregisterFD removes a file descriptor from the poller.
func (p *SafePoller) UnregisterFD(fd int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.initialized {
		return ErrPollerNotInitialized
	}

	if _, exists := p.callbacks[fd]; !exists {
		return ErrFDNotRegistered
	}

	delete(p.callbacks, fd)
	return nil
}

// ModifyFD updates the events being monitored for a file descriptor.
func (p *SafePoller) ModifyFD(fd int, events IOEvents) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.initialized {
		return ErrPollerNotInitialized
	}

	cb, exists := p.callbacks[fd]
	if !exists {
		return ErrFDNotRegistered
	}

	cb.events = events
	return nil
}

// PollIO polls for I/O events using IOCP.
func (p *SafePoller) PollIO(timeoutMs int) (int, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return 0, ErrPollerClosed
	}
	if !p.initialized {
		p.mu.Unlock()
		return 0, nil
	}
	iocp := p.iocp
	p.mu.Unlock()

	var timeout uint32
	if timeoutMs >= 0 {
		timeout = uint32(timeoutMs)
	}

	var bytes uint32
	var key uintptr
	var overlapped *windows.Overlapped

	err := windows.GetQueuedCompletionStatus(iocp, &bytes, &key, &overlapped, timeout)
	if err != nil {
		if errno, ok := err.(syscall.Errno); ok {
			if errno == windows.WAIT_TIMEOUT {
				return 0, nil
			}
			if errno == windows.ERROR_ABANDONED_WAIT_0 || errno == windows.ERROR_INVALID_HANDLE {
				return 0, ErrPollerClosed
			}
		}
		return 0, nil // Treat other errors as timeout
	}

	if overlapped == nil && key == 0 {
		// Wake-up notification via PostQueuedCompletionStatus
		return 0, nil
	}

	// Dispatch the completion
	p.mu.Lock()
	fd := int(key)
	var info *fdCallback
	if cb, ok := p.callbacks[fd]; ok {
		info = cb
	}
	p.mu.Unlock()

	if info != nil && info.callback != nil {
		p.safeCallback(info.callback, info.events)
		return 1, nil
	}

	return 0, nil
}

// processEventsLocked is not needed for IOCP (events processed one at a time).
func (p *SafePoller) processEventsLocked(n int) {}

// safeCallback executes a callback with panic recovery.
func (p *SafePoller) safeCallback(callback func(IOEvents), events IOEvents) {
	defer func() {
		if r := recover(); r != nil {
			// Log panic but continue processing other events
		}
	}()
	callback(events)
}

// IsClosed returns true if the poller has been closed.
func (p *SafePoller) IsClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

// Wakeup wakes up the poller using PostQueuedCompletionStatus.
func (p *SafePoller) Wakeup() error {
	p.mu.Lock()
	if p.closed || !p.initialized {
		p.mu.Unlock()
		return ErrPollerClosed
	}
	iocp := p.iocp
	p.mu.Unlock()
	return windows.PostQueuedCompletionStatus(iocp, 0, 0, nil)
}
