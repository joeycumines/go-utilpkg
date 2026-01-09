//go:build linux

package alternateone

import (
	"sync"

	"golang.org/x/sys/unix"
)

// IOEvents represents the type of I/O events to monitor.
type IOEvents uint32

const (
	// EventRead indicates the file descriptor is ready for reading.
	EventRead IOEvents = unix.EPOLLIN
	// EventWrite indicates the file descriptor is ready for writing.
	EventWrite IOEvents = unix.EPOLLOUT
	// EventError indicates an error condition on the file descriptor.
	EventError IOEvents = unix.EPOLLERR
	// EventHangup indicates the peer closed its end of the connection.
	EventHangup IOEvents = unix.EPOLLHUP
)

// fdCallback stores the callback and events for a registered file descriptor.
type fdCallback struct {
	callback func(events IOEvents)
	events   IOEvents
}

// SafePoller manages I/O event registration using epoll (Linux).
//
// SAFETY: Uses sync.Mutex (write lock) for ALL operations including poll.
// This prevents any concurrent modifications during poll processing.
// We accept potential starvation for correctness.
type SafePoller struct {
	mu          sync.Mutex
	epfd        int
	callbacks   map[int]*fdCallback
	eventBuf    []unix.EpollEvent
	initialized bool
	closed      bool
}

// initPoller initializes the epoll instance.
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

	epfd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		return WrapError("initPoller", err)
	}

	p.epfd = epfd
	p.callbacks = make(map[int]*fdCallback)
	p.eventBuf = make([]unix.EpollEvent, 128)
	p.initialized = true
	return nil
}

// closePoller closes the epoll instance and releases resources.
func (p *SafePoller) closePoller() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.initialized {
		return nil
	}

	p.closed = true
	err := unix.Close(p.epfd)
	p.callbacks = nil
	p.initialized = false
	return err
}

// RegisterFD registers a file descriptor for I/O event monitoring.
//
// SAFETY: Holds write lock for entire operation.
// Callbacks execute under lock to prevent re-entrancy issues.
func (p *SafePoller) RegisterFD(fd int, events IOEvents, callback func(events IOEvents)) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if err := p.initPollerLocked(); err != nil {
		return err
	}

	if _, exists := p.callbacks[fd]; exists {
		return ErrFDAlreadyRegistered
	}

	event := unix.EpollEvent{
		Events: uint32(events),
		Fd:     int32(fd),
	}
	if err := unix.EpollCtl(p.epfd, unix.EPOLL_CTL_ADD, fd, &event); err != nil {
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

	if err := unix.EpollCtl(p.epfd, unix.EPOLL_CTL_DEL, fd, nil); err != nil {
		return WrapError("UnregisterFD", err)
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

	event := unix.EpollEvent{
		Events: uint32(events),
		Fd:     int32(fd),
	}
	if err := unix.EpollCtl(p.epfd, unix.EPOLL_CTL_MOD, fd, &event); err != nil {
		return WrapError("ModifyFD", err)
	}

	cb.events = events
	return nil
}

// PollIO polls for I/O events.
//
// SAFETY: Uses write Lock() instead of RLock().
// This prevents any concurrent modifications during poll processing.
// Callbacks are executed UNDER LOCK.
//
// Trade-off: RegisterFD blocks during poll, but there's zero risk of zombie poller access.
func (p *SafePoller) PollIO(timeoutMs int) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return 0, ErrPollerClosed
	}

	if !p.initialized || len(p.callbacks) == 0 {
		return 0, nil
	}

	// Blocking syscall under lock (accepts starvation for correctness)
	n, err := unix.EpollWait(p.epfd, p.eventBuf, timeoutMs)
	if err != nil {
		if err == unix.EINTR {
			return 0, nil
		}
		return 0, WrapError("PollIO", err)
	}

	// Process events under lock
	p.processEventsLocked(n)

	return n, nil
}

// processEventsLocked executes callbacks under lock.
// SAFETY: Callbacks execute under lock to prevent re-entrancy issues.
// Documented: User must not call RegisterFD from callback.
func (p *SafePoller) processEventsLocked(n int) {
	for i := 0; i < n; i++ {
		fd := int(p.eventBuf[i].Fd)
		if info, ok := p.callbacks[fd]; ok && info.callback != nil {
			triggered := IOEvents(p.eventBuf[i].Events)
			// SAFETY: Callback executes under lock
			// User must not call RegisterFD from callback (documented)
			p.safeCallback(info.callback, triggered)
		}
	}
}

// safeCallback executes a callback with panic recovery.
func (p *SafePoller) safeCallback(callback func(IOEvents), events IOEvents) {
	defer func() {
		if r := recover(); r != nil {
			// Log panic but continue processing other events
			// In a real implementation, this would use a proper logger
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
