//go:build windows

package alternatethree

import (
	"errors"
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

// ioPoller manages I/O event registration using IOCP (Windows).
type ioPoller struct {
	callbacks   map[int]*fdCallback
	mu          sync.RWMutex
	iocp        windows.Handle
	initialized bool
	closed      bool
}

// initPoller initializes the IOCP instance.
func (p *ioPoller) initPoller() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.initPollerImpl()
}

// initPollerImpl initializes without acquiring the lock.
func (p *ioPoller) initPollerImpl() error {
	if p.closed {
		return errEventLoopClosed
	}
	if p.initialized {
		return nil
	}

	iocp, err := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 0)
	if err != nil {
		return err
	}

	p.iocp = iocp
	p.callbacks = make(map[int]*fdCallback)
	p.initialized = true
	return nil
}

// closePoller closes the IOCP instance.
func (p *ioPoller) closePoller() error {
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

// Wakeup wakes up the poller using PostQueuedCompletionStatus.
func (p *ioPoller) Wakeup() error {
	p.mu.RLock()
	if p.closed || !p.initialized {
		p.mu.RUnlock()
		return errEventLoopClosed
	}
	iocp := p.iocp
	p.mu.RUnlock()
	return windows.PostQueuedCompletionStatus(iocp, 0, 0, nil)
}

// RegisterFD registers a file descriptor for I/O event monitoring.
func (l *Loop) RegisterFD(fd int, events IOEvents, callback func(events IOEvents)) error {
	p := &l.ioPoller
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.initPollerImpl(); err != nil {
		return err
	}

	if _, exists := p.callbacks[fd]; exists {
		return errors.New("alternatethree: fd already registered")
	}

	// Associate handle with IOCP
	handle := windows.Handle(fd)
	_, err := windows.CreateIoCompletionPort(handle, p.iocp, uintptr(fd), 0)
	if err != nil {
		return err
	}

	p.callbacks[fd] = &fdCallback{
		callback: callback,
		events:   events,
	}
	return nil
}

// UnregisterFD removes a file descriptor from monitoring.
func (l *Loop) UnregisterFD(fd int) error {
	p := &l.ioPoller
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.initialized {
		return errEventLoopClosed
	}

	if _, exists := p.callbacks[fd]; !exists {
		return errors.New("alternatethree: fd not registered")
	}

	delete(p.callbacks, fd)
	return nil
}

// ModifyFD updates the events being monitored for a file descriptor.
func (l *Loop) ModifyFD(fd int, events IOEvents) error {
	p := &l.ioPoller
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.initialized {
		return errEventLoopClosed
	}

	cb, exists := p.callbacks[fd]
	if !exists {
		return errors.New("alternatethree: fd not registered")
	}

	cb.events = events
	return nil
}

// pollIO polls for I/O events using IOCP.
func (l *Loop) pollIO(timeout int, maxEvents int) (int, error) {
	p := &l.ioPoller
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return 0, errEventLoopClosed
	}
	if !p.initialized {
		p.mu.RUnlock()
		return 0, nil
	}
	iocp := p.iocp
	p.mu.RUnlock()

	var timeoutMs uint32
	if timeout >= 0 {
		timeoutMs = uint32(timeout)
	}

	var bytes uint32
	var key uintptr
	var overlapped *windows.Overlapped

	err := windows.GetQueuedCompletionStatus(iocp, &bytes, &key, &overlapped, timeoutMs)
	if err != nil {
		if errno, ok := err.(syscall.Errno); ok {
			if errno == windows.WAIT_TIMEOUT {
				return 0, nil
			}
			if errno == windows.ERROR_ABANDONED_WAIT_0 || errno == windows.ERROR_INVALID_HANDLE {
				return 0, errEventLoopClosed
			}
		}
		return 0, nil
	}

	if overlapped == nil && key == 0 {
		// Wake-up notification
		return 0, nil
	}

	// Dispatch the completion
	fd := int(key)
	p.mu.RLock()
	var info *fdCallback
	if cb, ok := p.callbacks[fd]; ok {
		info = cb
	}
	p.mu.RUnlock()

	if info != nil && info.callback != nil {
		info.callback(info.events)
		return 1, nil
	}

	return 0, nil
}
