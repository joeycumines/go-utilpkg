//go:build darwin

package eventloop

import (
	"errors"
	"sync"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

// Maximum file descriptor we support with direct indexing.
const maxFDs = 65536

// maxFDLimit is the maximum FD value we support for dynamic growth.
// 100M is enough for production with ulimit -n > 1M.
const maxFDLimit = 100000000

// IOEvents represents the type of I/O events to monitor.
type IOEvents uint32

const (
	EventRead IOEvents = 1 << iota
	EventWrite
	EventError
	EventHangup
)

var (
	ErrFDOutOfRange        = errors.New("eventloop: fd out of range (max 100000000)")
	ErrFDAlreadyRegistered = errors.New("eventloop: fd already registered")
	ErrFDNotRegistered     = errors.New("eventloop: fd not registered")
	ErrPollerClosed        = errors.New("eventloop: poller closed")
)

// ioCallback is the callback type for I/O events.
type ioCallback func(IOEvents)

// fdInfo stores per-FD callback information.
type fdInfo struct {
	callback ioCallback
	events   IOEvents
	active   bool
}

// fastPoller manages I/O event registration using kqueue (Darwin).
//
// PERFORMANCE: Uses RWMutex for fdInfo access. The mutex is only held briefly
// during registration/callback dispatch. The polling syscall itself is lock-free.
// It uses a dynamic slice instead of a fixed array for flexible FD support.
//
// CACHE LINE PADDING: Padding fields (marked with //nolint:unused) isolate
// frequently-accessed fields (kq, closed) to reduce false sharing across cache lines.
// The betteralign tool ensures correct cache line alignment during struct layout optimization.
type fastPoller struct { // betteralign:ignore
	_        [sizeOfCacheLine]byte     // Cache line padding before kq (isolates from previous fields) //nolint:unused
	kq       int32                     // kqueue file descriptor
	_        [sizeOfCacheLine - 4]byte // Padding to isolate eventBuf from isolated field //nolint:unused
	eventBuf [256]unix.Kevent_t        // Preallocated event buffer (256 kevents)
	fds      []fdInfo                  // Dynamic slice, grows on demand
	fdMu     sync.RWMutex              // Protects fds array access
	_        [sizeOfCacheLine]byte     // Cache line padding before closed (isolates from previous fields) //nolint:unused
	closed   atomic.Bool               // Closed flag
}

// Init initializes the kqueue instance.
func (p *fastPoller) Init() error {
	if p.closed.Load() {
		return ErrPollerClosed
	}

	kq, err := unix.Kqueue()
	if err != nil {
		return err
	}
	unix.CloseOnExec(kq)
	p.kq = int32(kq)

	p.fds = make([]fdInfo, maxFDs)

	return nil
}

// Close closes the kqueue instance.
func (p *fastPoller) Close() error {
	if p.closed.Swap(true) {
		// Already closed, return nil for idempotent behavior
		return nil
	}
	if p.kq > 0 {
		return unix.Close(int(p.kq))
	}
	return nil
}

// Wakeup is a stub on Darwin/Linux platforms.
// On these platforms, wake-up is handled by writing to the wake pipe
// rather than calling Wakeup() on the poller. This method exists primarily
// for API compatibility with Windows, which uses PostQueuedCompletionStatus.
// It returns nil because the wake pipe mechanism is used instead.
//
// NOTE: This method should never be called on Darwin/Linux under normal
// operation. The loop.go submitWakeup() function checks wakePipe < 0 and
// writes to the wake pipe for Unix platforms instead of calling Wakeup().
func (p *fastPoller) Wakeup() error {
	// Darwin: Write to wake pipe in submitWakeup()
	// This stub exists for API compatibility with Windows
	return nil
}

// RegisterFD registers a file descriptor for I/O event monitoring.
func (p *fastPoller) RegisterFD(fd int, events IOEvents, cb ioCallback) error {
	if p.closed.Load() {
		return ErrPollerClosed
	}
	if fd < 0 || fd >= maxFDLimit {
		return ErrFDOutOfRange
	}

	p.fdMu.Lock()
	if fd >= len(p.fds) {
		newSize := fd*2 + 1
		if newSize > maxFDLimit {
			newSize = maxFDLimit + 1
		}
		newFds := make([]fdInfo, newSize)
		copy(newFds, p.fds)
		p.fds = newFds
	}

	if p.fds[fd].active {
		p.fdMu.Unlock()
		return ErrFDAlreadyRegistered
	}

	p.fds[fd] = fdInfo{callback: cb, events: events, active: true}

	// Hold lock across Kevent to prevent race with concurrent UnregisterFD.
	kevents := eventsToKevents(fd, events, unix.EV_ADD|unix.EV_ENABLE)
	if len(kevents) > 0 {
		_, err := unix.Kevent(int(p.kq), kevents, nil, nil)
		if err != nil {
			p.fds[fd] = fdInfo{} // Rollback
			p.fdMu.Unlock()
			return err
		}
	}
	p.fdMu.Unlock()
	return nil
}

// UnregisterFD removes a file descriptor from monitoring.
//
// CALLBACK LIFETIME SAFETY:
// UnregisterFD does NOT guarantee immediate cessation of in-flight callbacks.
// The dispatch logic copies callback pointers under RLock, releases the lock,
// then executes callbacks OUTSIDE of the lock. This design choice avoids:
//  1. Holding locks during callback execution (prevents deadlocks)
//  2. Performance degradation from lock convoy effects
//
// However, it creates a narrow race window:
//   - If dispatchEvents copies callback C1, then releases lock
//   - User calls UnregisterFD (clears fd[X] = {})
//   - dispatchEvents executes COPIED callback C1
//   - Result: Callback runs after UnregisterFD returns
//
// REQUIRED USER COORDINATION:
//  1. Close FD ONLY after all callbacks have completed (e.g., using sync.WaitGroup)
//  2. Callbacks must guard against accessing closed FDs
//
// This is the correct implementation for high-performance I/O multiplexing.
func (p *fastPoller) UnregisterFD(fd int) error {
	if fd < 0 {
		return ErrFDOutOfRange
	}

	p.fdMu.Lock()
	if fd >= len(p.fds) || !p.fds[fd].active {
		p.fdMu.Unlock()
		return ErrFDNotRegistered
	}

	events := p.fds[fd].events

	// Remove from kqueue while holding lock to prevent race with RegisterFD.
	kevents := eventsToKevents(fd, events, unix.EV_DELETE)
	if len(kevents) > 0 {
		unix.Kevent(int(p.kq), kevents, nil, nil) // Ignore errors on delete
	}

	p.fds[fd] = fdInfo{}
	p.fdMu.Unlock()
	return nil
}

// ModifyFD updates the events being monitored for a file descriptor.
func (p *fastPoller) ModifyFD(fd int, events IOEvents) error {
	if fd < 0 {
		return ErrFDOutOfRange
	}

	p.fdMu.Lock()
	if fd >= len(p.fds) || !p.fds[fd].active {
		p.fdMu.Unlock()
		return ErrFDNotRegistered
	}

	oldEvents := p.fds[fd].events
	p.fds[fd].events = events
	p.fdMu.Unlock()

	if oldEvents&^events != 0 {
		delKevents := eventsToKevents(fd, oldEvents&^events, unix.EV_DELETE)
		if len(delKevents) > 0 {
			unix.Kevent(int(p.kq), delKevents, nil, nil) // Ignore errors
		}
	}

	if events&^oldEvents != 0 {
		addKevents := eventsToKevents(fd, events&^oldEvents, unix.EV_ADD|unix.EV_ENABLE)
		if len(addKevents) > 0 {
			if _, err := unix.Kevent(int(p.kq), addKevents, nil, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

// PollIO polls for I/O events.
func (p *fastPoller) PollIO(timeoutMs int) (int, error) {
	if p.closed.Load() {
		return 0, ErrPollerClosed
	}

	var ts *unix.Timespec
	if timeoutMs >= 0 {
		ts = &unix.Timespec{
			Sec:  int64(timeoutMs / 1000),
			Nsec: int64((timeoutMs % 1000) * 1000000),
		}
	}

	n, err := unix.Kevent(int(p.kq), nil, p.eventBuf[:], ts)
	if err != nil {
		if err == unix.EINTR {
			return 0, nil
		}
		return 0, err
	}

	p.dispatchEvents(n)

	return n, nil
}

// dispatchEvents executes callbacks inline.
// RACE SAFETY: Uses RLock to safely read fdInfo while allowing concurrent
// modifications to other fds. Callback is copied under lock then called outside.
func (p *fastPoller) dispatchEvents(n int) {
	for i := 0; i < n; i++ {
		fd := int(p.eventBuf[i].Ident)
		if fd < 0 {
			continue
		}

		p.fdMu.RLock()
		var info fdInfo
		if fd < len(p.fds) {
			info = p.fds[fd]
		}
		p.fdMu.RUnlock()

		if info.active && info.callback != nil {
			events := keventToEvents(&p.eventBuf[i])
			info.callback(events)
		}
	}
}

// eventsToKevents converts IOEvents to kqueue kevent structures.
func eventsToKevents(fd int, events IOEvents, flags uint16) []unix.Kevent_t {
	var kevents []unix.Kevent_t

	if events&EventRead != 0 {
		kevents = append(kevents, unix.Kevent_t{
			Ident:  uint64(fd),
			Filter: unix.EVFILT_READ,
			Flags:  flags,
		})
	}

	if events&EventWrite != 0 {
		kevents = append(kevents, unix.Kevent_t{
			Ident:  uint64(fd),
			Filter: unix.EVFILT_WRITE,
			Flags:  flags,
		})
	}

	return kevents
}

// keventToEvents converts kqueue event to IOEvents.
func keventToEvents(kev *unix.Kevent_t) IOEvents {
	var events IOEvents
	switch kev.Filter {
	case unix.EVFILT_READ:
		events |= EventRead
	case unix.EVFILT_WRITE:
		events |= EventWrite
	}
	if kev.Flags&unix.EV_ERROR != 0 {
		events |= EventError
	}
	if kev.Flags&unix.EV_EOF != 0 {
		events |= EventHangup
	}
	return events
}
