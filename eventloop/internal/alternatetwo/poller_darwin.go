//go:build darwin

package alternatetwo

import (
	"errors"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

// Maximum file descriptor we support with direct indexing.
const maxFDs = 65536

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

// FastPoller manages I/O event registration using kqueue (Darwin).
//
// PERFORMANCE: Zero-lock design with direct FD indexing.
type FastPoller struct { // betteralign:ignore
	_        [64]byte           // Cache line padding //nolint:unused
	kq       int32              // kqueue file descriptor
	_        [60]byte           // Pad to cache line //nolint:unused
	version  atomic.Uint64      // Version counter for consistency
	_        [56]byte           // Pad to cache line //nolint:unused
	eventBuf [256]unix.Kevent_t // Larger buffer, preallocated
	fds      [maxFDs]fdInfo     // Direct indexing, no map
	closed   atomic.Bool        // Closed flag
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
	if fd < 0 || fd >= maxFDs {
		return ErrFDOutOfRange
	}
	if p.fds[fd].active {
		return ErrFDAlreadyRegistered
	}

	p.fds[fd] = fdInfo{callback: cb, events: events, active: true}
	p.version.Add(1)

	kevents := eventsToKevents(fd, events, unix.EV_ADD|unix.EV_ENABLE)
	if len(kevents) > 0 {
		_, err := unix.Kevent(int(p.kq), kevents, nil, nil)
		if err != nil {
			p.fds[fd] = fdInfo{} // Rollback
			return err
		}
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

	events := p.fds[fd].events
	p.fds[fd] = fdInfo{}
	p.version.Add(1)

	kevents := eventsToKevents(fd, events, unix.EV_DELETE)
	if len(kevents) > 0 {
		unix.Kevent(int(p.kq), kevents, nil, nil) // Ignore errors on delete
	}
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

	oldEvents := p.fds[fd].events
	p.fds[fd].events = events
	p.version.Add(1)

	// Delete old events
	if oldEvents&^events != 0 {
		delKevents := eventsToKevents(fd, oldEvents&^events, unix.EV_DELETE)
		if len(delKevents) > 0 {
			unix.Kevent(int(p.kq), delKevents, nil, nil) // Ignore errors
		}
	}

	// Add new events
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

	v := p.version.Load()

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

	// Check version after syscall
	if p.version.Load() != v {
		return 0, nil
	}

	// Dispatch events inline
	p.dispatchEvents(n)

	return n, nil
}

// dispatchEvents executes callbacks inline.
func (p *FastPoller) dispatchEvents(n int) {
	for i := 0; i < n; i++ {
		fd := int(p.eventBuf[i].Ident)
		if fd >= 0 && fd < maxFDs {
			info := &p.fds[fd]
			if info.active && info.callback != nil {
				events := keventToEvents(&p.eventBuf[i])
				info.callback(events)
			}
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
