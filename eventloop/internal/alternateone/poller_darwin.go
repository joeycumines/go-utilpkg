//go:build darwin

package alternateone

import (
	"sync"

	"golang.org/x/sys/unix"
)

// IOEvents represents the type of I/O events to monitor.
type IOEvents uint32

const (
	// EventRead indicates the file descriptor is ready for reading.
	EventRead IOEvents = 1 << iota
	// EventWrite indicates the file descriptor is ready for writing.
	EventWrite
	// EventError indicates an error condition on the file descriptor.
	EventError
	// EventHangup indicates the peer closed its end of the connection.
	EventHangup
)

// kqueue filter constants
const (
	kqueueFilterRead  = unix.EVFILT_READ
	kqueueFilterWrite = unix.EVFILT_WRITE
)

// fdCallback stores the callback and events for a registered file descriptor.
type fdCallback struct {
	callback func(events IOEvents)
	events   IOEvents
}

// SafePoller manages I/O event registration using kqueue (Darwin).
//
// SAFETY: Uses sync.Mutex (write lock) for ALL operations including poll.
// This prevents any concurrent modifications during poll processing.
// We accept potential starvation for correctness.
type SafePoller struct {
	callbacks   map[int]*fdCallback
	eventBuf    []unix.Kevent_t
	mu          sync.Mutex
	kq          int
	initialized bool
	closed      bool
}

// initPoller initializes the kqueue instance.
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

	kq, err := unix.Kqueue()
	if err != nil {
		return WrapError("initPoller", err)
	}

	unix.CloseOnExec(kq)

	p.kq = kq
	p.callbacks = make(map[int]*fdCallback)
	p.eventBuf = make([]unix.Kevent_t, 128)
	p.initialized = true
	return nil
}

// closePoller closes the kqueue instance and releases resources.
func (p *SafePoller) closePoller() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.initialized {
		return nil
	}

	p.closed = true
	err := unix.Close(p.kq)
	p.callbacks = nil
	p.initialized = false
	return err
}

// eventsToKevents converts IOEvents to kqueue kevent structures.
func eventsToKevents(fd int, events IOEvents, flags uint16) []unix.Kevent_t {
	var kevents []unix.Kevent_t

	if events&EventRead != 0 {
		kevents = append(kevents, unix.Kevent_t{
			Ident:  uint64(fd),
			Filter: kqueueFilterRead,
			Flags:  flags,
		})
	}

	if events&EventWrite != 0 {
		kevents = append(kevents, unix.Kevent_t{
			Ident:  uint64(fd),
			Filter: kqueueFilterWrite,
			Flags:  flags,
		})
	}

	return kevents
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

	kevents := eventsToKevents(fd, events, unix.EV_ADD|unix.EV_ENABLE)
	if len(kevents) == 0 {
		return WrapError("RegisterFD", ErrFDNotRegistered)
	}

	_, err := unix.Kevent(p.kq, kevents, nil, nil)
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

	cb, exists := p.callbacks[fd]
	if !exists {
		return ErrFDNotRegistered
	}

	kevents := eventsToKevents(fd, cb.events, unix.EV_DELETE)
	_, err := unix.Kevent(p.kq, kevents, nil, nil)
	if err != nil && err != unix.ENOENT {
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

	oldEvents := cb.events

	// Delete old events
	if oldEvents&^events != 0 {
		delKevents := eventsToKevents(fd, oldEvents&^events, unix.EV_DELETE)
		if len(delKevents) > 0 {
			if _, err := unix.Kevent(p.kq, delKevents, nil, nil); err != nil {
				if err != unix.ENOENT {
					return WrapError("ModifyFD", err)
				}
			}
		}
	}

	// Add new events
	if events&^oldEvents != 0 {
		addKevents := eventsToKevents(fd, events&^oldEvents, unix.EV_ADD|unix.EV_ENABLE)
		if len(addKevents) > 0 {
			if _, err := unix.Kevent(p.kq, addKevents, nil, nil); err != nil {
				return WrapError("ModifyFD", err)
			}
		}
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

	// Convert timeout
	var ts *unix.Timespec
	if timeoutMs >= 0 {
		ts = &unix.Timespec{
			Sec:  int64(timeoutMs / 1000),
			Nsec: int64((timeoutMs % 1000) * 1000000),
		}
	}

	// Blocking syscall under lock (accepts starvation for correctness)
	n, err := unix.Kevent(p.kq, nil, p.eventBuf, ts)
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
		fd := int(p.eventBuf[i].Ident)
		if info, ok := p.callbacks[fd]; ok && info.callback != nil {
			// Convert kqueue filter to IOEvents
			var triggered IOEvents
			switch p.eventBuf[i].Filter {
			case kqueueFilterRead:
				triggered |= EventRead
			case kqueueFilterWrite:
				triggered |= EventWrite
			}
			if p.eventBuf[i].Flags&unix.EV_ERROR != 0 {
				triggered |= EventError
			}
			if p.eventBuf[i].Flags&unix.EV_EOF != 0 {
				triggered |= EventHangup
			}
			// SAFETY: Callback executes under lock
			p.safeCallback(info.callback, triggered)
		}
	}
}

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
