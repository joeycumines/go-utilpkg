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

// MaxFDLimit is the maximum FD value we support for dynamic growth.
// 100M is enough for production with ulimit -n > 1M.
const MaxFDLimit = 100000000

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

// IOCallback is the callback type for I/O events.
type IOCallback func(IOEvents)

// fdInfo stores per-FD callback information.
type fdInfo struct {
	callback IOCallback
	events   IOEvents
	active   bool
}

// FastPoller manages I/O event registration using kqueue (Darwin).
//
// PERFORMANCE: Uses RWMutex for fdInfo access. The mutex is only held briefly
// during registration/callback dispatch. The polling syscall itself is lock-free.
// It uses a dynamic slice instead of a fixed array for flexible FD support.
type FastPoller struct { // betteralign:ignore
	_        [64]byte           // Cache line padding //nolint:unused
	kq       int32              // kqueue file descriptor
	_        [60]byte           // Pad to cache line //nolint:unused
	eventBuf [256]unix.Kevent_t // Preallocated event buffer
	fds      []fdInfo           // Dynamic slice, grows on demand
	fdMu     sync.RWMutex       // Protects fds array access
	closed   atomic.Bool
}

// Init initializes the kqueue instance.
func (p *FastPoller) Init() error {
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
func (p *FastPoller) Close() error {
	p.closed.Store(true)
	if p.kq > 0 {
		return unix.Close(int(p.kq))
	}
	return nil
}

// RegisterFD registers a file descriptor for I/O event monitoring.
func (p *FastPoller) RegisterFD(fd int, events IOEvents, cb IOCallback) error {
	if p.closed.Load() {
		return ErrPollerClosed
	}
	if fd < 0 || fd >= MaxFDLimit {
		return ErrFDOutOfRange
	}

	p.fdMu.Lock()
	if fd >= len(p.fds) {
		newSize := fd*2 + 1
		if newSize > MaxFDLimit {
			newSize = MaxFDLimit + 1
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
	p.fdMu.Unlock()

	kevents := eventsToKevents(fd, events, unix.EV_ADD|unix.EV_ENABLE)
	if len(kevents) > 0 {
		_, err := unix.Kevent(int(p.kq), kevents, nil, nil)
		if err != nil {
			p.fdMu.Lock()
			p.fds[fd] = fdInfo{} // Rollback
			p.fdMu.Unlock()
			return err
		}
	}
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
func (p *FastPoller) UnregisterFD(fd int) error {
	if fd < 0 {
		return ErrFDOutOfRange
	}

	p.fdMu.Lock()
	if fd >= len(p.fds) || !p.fds[fd].active {
		p.fdMu.Unlock()
		return ErrFDNotRegistered
	}

	events := p.fds[fd].events
	p.fds[fd] = fdInfo{}
	p.fdMu.Unlock()

	kevents := eventsToKevents(fd, events, unix.EV_DELETE)
	if len(kevents) > 0 {
		unix.Kevent(int(p.kq), kevents, nil, nil) // Ignore errors on delete
	}
	return nil
}

// ModifyFD updates the events being monitored for a file descriptor.
func (p *FastPoller) ModifyFD(fd int, events IOEvents) error {
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
func (p *FastPoller) PollIO(timeoutMs int) (int, error) {
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
func (p *FastPoller) dispatchEvents(n int) {
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
