//go:build linux

package alternatethree

import (
	"errors"
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

// ioPoller manages I/O event registration using epoll (Linux).
// Thread-safe: all methods can be called from any goroutine.
type ioPoller struct {
	// T10-FIX-4: Pre-allocated event buffer for zero-allocation pollIO.
	// Sized to 128 events to match typical high-throughput usage.
	// Placed first for optimal alignment (slice = 24 bytes).
	eventBuf []unix.EpollEvent

	callbacks map[int]*fdCallback // fd -> callback mapping

	mu          sync.RWMutex
	epfd        int // epoll file descriptor
	initialized bool
	closed      bool // Mark permanently closed to prevent zombie resurrection
}

// initPoller initializes the epoll instance.
// Must be called before any RegisterFD/UnregisterFD calls.
func (p *ioPoller) initPoller() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.initPollerImpl()
}

// initPollerImpl initializes the epoll instance.
// Must be called while holding the lock.
// This is the internal implementation used by RegisterFD to prevent zombie resurrection.
func (p *ioPoller) initPollerImpl() error {
	if p.closed {
		return errEventLoopClosed
	}

	if p.initialized {
		return nil
	}

	epfd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		return err
	}

	p.epfd = epfd
	p.callbacks = make(map[int]*fdCallback)
	// T10-FIX-4: Pre-allocate event buffer for zero-allocation pollIO
	p.eventBuf = make([]unix.EpollEvent, 128)
	p.initialized = true
	return nil
}

// closePoller closes the epoll instance and releases resources.
func (p *ioPoller) closePoller() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.initialized {
		return nil
	}

	p.closed = true // Mark permanently closed
	err := unix.Close(p.epfd)
	p.callbacks = nil
	p.initialized = false
	return err
}

// RegisterFD registers a file descriptor for I/O event monitoring.
//
// # Parameters
//   - fd: The file descriptor to monitor
//   - events: Bitmask of IOEvents to watch for (EventRead, EventWrite, etc.)
//   - callback: Function called when events occur. Receives the triggered events.
//
// # Thread Safety
//
// Safe to call from any goroutine. The callback will be executed on the loop thread.
//
// # Usage
//
//	loop.RegisterFD(conn.Fd(), EventRead|EventWrite, func(events IOEvents) {
//	    if events&EventRead != 0 {
//	        // Handle readable
//	    }
//	    if events&EventWrite != 0 {
//	        // Handle writable
//	    }
//	})
func (l *Loop) RegisterFD(fd int, events IOEvents, callback func(events IOEvents)) error {
	l.ioPoller.mu.Lock()
	defer l.ioPoller.mu.Unlock() // Lock held for entire function to prevent zombie resurrection

	// Use internal impl to avoid deadlock (since we hold the lock)
	if err := l.ioPoller.initPollerImpl(); err != nil {
		return err
	}

	// Check if already registered
	if _, exists := l.ioPoller.callbacks[fd]; exists {
		return errors.New("poller: fd already registered")
	}

	// Add to epoll
	event := unix.EpollEvent{
		Events: uint32(events),
		Fd:     int32(fd),
	}
	if err := unix.EpollCtl(l.ioPoller.epfd, unix.EPOLL_CTL_ADD, fd, &event); err != nil {
		return err
	}

	// Store callback
	l.ioPoller.callbacks[fd] = &fdCallback{
		callback: callback,
		events:   events,
	}

	return nil
}

// UnregisterFD removes a file descriptor from the event loop.
//
// # Safety Warning (Task 9.3)
//
// Closing a file descriptor without calling UnregisterFD first is UNDEFINED BEHAVIOR.
// On Linux, file descriptors are recycled. If you close FD X and the OS assigns
// FD X to a new file, the Event Loop might receive an event for the old FD X
// but deliver it to the callback registered for the original FD X (now aliased).
//
// ALWAYS UnregisterFD before closing it.
//
// Correct Usage:
//
//	l.UnregisterFD(fd)
//	syscall.Close(fd)
func (l *Loop) UnregisterFD(fd int) error {
	l.ioPoller.mu.Lock()
	defer l.ioPoller.mu.Unlock()

	if !l.ioPoller.initialized {
		return errors.New("poller: not initialized")
	}

	// Check if registered
	if _, exists := l.ioPoller.callbacks[fd]; !exists {
		return errors.New("poller: fd not registered")
	}

	// Remove from epoll
	if err := unix.EpollCtl(l.ioPoller.epfd, unix.EPOLL_CTL_DEL, fd, nil); err != nil {
		return err
	}

	// Remove callback
	delete(l.ioPoller.callbacks, fd)

	return nil
}

// ModifyFD updates the events being monitored for a file descriptor.
func (l *Loop) ModifyFD(fd int, events IOEvents) error {
	l.ioPoller.mu.Lock()
	defer l.ioPoller.mu.Unlock()

	if !l.ioPoller.initialized {
		return errors.New("poller: not initialized")
	}

	cb, exists := l.ioPoller.callbacks[fd]
	if !exists {
		return errors.New("poller: fd not registered")
	}

	event := unix.EpollEvent{
		Events: uint32(events),
		Fd:     int32(fd),
	}
	if err := unix.EpollCtl(l.ioPoller.epfd, unix.EPOLL_CTL_MOD, fd, &event); err != nil {
		return err
	}

	cb.events = events
	return nil
}

// pollIO polls for I/O events and returns the number of ready file descriptors.
// This is called from the main loop's poll phase.
// maxEvents controls the maximum number of events to return in one call (capped to 128).
//
// T10-FIX-1: Releases RLock before blocking syscall to prevent lock starvation.
// T10-FIX-2: Collects callbacks under lock, executes outside lock to prevent deadlock.
// T10-FIX-4: Uses pre-allocated eventBuf for zero allocations.
func (l *Loop) pollIO(timeout int, maxEvents int) (int, error) {
	// T10-FIX-1: Copy epfd locally under RLock, then release before syscall.
	// This prevents RegisterFD/UnregisterFD from blocking for the poll duration.
	l.ioPoller.mu.RLock()
	if !l.ioPoller.initialized || len(l.ioPoller.callbacks) == 0 {
		l.ioPoller.mu.RUnlock()
		return 0, nil
	}
	epfd := l.ioPoller.epfd       // Copy FD locally
	evtBuf := l.ioPoller.eventBuf // Capture slice header under lock to avoid shutdown races
	l.ioPoller.mu.RUnlock()       // RELEASE LOCK before blocking syscall

	// T10-FIX-4: Use pre-allocated event buffer, capped to buffer size.
	if maxEvents > len(evtBuf) {
		maxEvents = len(evtBuf)
	}
	events := evtBuf[:maxEvents]

	// Execute blocking syscall WITHOUT holding any lock
	n, err := unix.EpollWait(epfd, events, timeout)
	if err != nil {
		if err == unix.EINTR {
			return 0, nil // Interrupted, not an error
		}
		return 0, err
	}

	if n == 0 {
		return 0, nil
	}

	// T10-FIX-2: Collect-then-Execute pattern.
	// Phase 1: Collect callbacks under RLock (but don't execute yet).
	type pendingCallback struct {
		callback func(IOEvents)
		events   IOEvents
	}
	// Use stack allocation for small counts, avoiding heap escape.
	// Sized to maxEvents (128) to guarantee NO heap allocation under load.
	var pendingStack [128]pendingCallback
	pending := pendingStack[:0]

	l.ioPoller.mu.RLock()
	// CRITICAL: Re-check initialization - closePoller() might have run during syscall.
	if !l.ioPoller.initialized {
		l.ioPoller.mu.RUnlock()
		return 0, nil
	}
	for i := 0; i < n; i++ {
		fd := int(events[i].Fd)
		cb, exists := l.ioPoller.callbacks[fd]
		if exists && cb.callback != nil {
			triggered := IOEvents(events[i].Events)
			pending = append(pending, pendingCallback{
				callback: cb.callback,
				events:   triggered,
			})
		}
	}
	l.ioPoller.mu.RUnlock() // RELEASE LOCK before executing callbacks

	// Phase 2: Execute callbacks WITHOUT holding any lock.
	// This allows callbacks to safely call UnregisterFD without deadlock.
	for _, p := range pending {
		p.callback(p.events)
	}

	return n, nil
}
