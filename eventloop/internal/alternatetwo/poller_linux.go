//go:build linux

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
// PERFORMANCE: Small struct, no pointers except callback.
type fdInfo struct {
	callback IOCallback
	events   IOEvents
	active   bool
}

// FastPoller manages I/O event registration using epoll (Linux).
//
// PERFORMANCE: Zero-lock design with direct FD indexing.
// - Direct array indexing instead of map for O(1) lookup
// - Version-based consistency for safe poll access
// - Inline callback execution
type FastPoller struct { // betteralign:ignore
	_        [64]byte             // Cache line padding //nolint:unused
	epfd     int32                // epoll file descriptor
	_        [60]byte             // Pad to cache line //nolint:unused
	version  atomic.Uint64        // Version counter for consistency
	_        [56]byte             // Pad to cache line //nolint:unused
	eventBuf [256]unix.EpollEvent // Larger buffer, preallocated
	fds      [maxFDs]fdInfo       // Direct indexing, no map
	closed   atomic.Bool          // Closed flag
}

// Init initializes the epoll instance.
func (p *FastPoller) Init() error {
	if p.closed.Load() {
		return ErrPollerClosed
	}

	epfd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		return err
	}
	p.epfd = int32(epfd)
	return nil
}

// Close closes the epoll instance.
func (p *FastPoller) Close() error {
	p.closed.Store(true)
	if p.epfd > 0 {
		return unix.Close(int(p.epfd))
	}
	return nil
}

// RegisterFD registers a file descriptor for I/O event monitoring.
// PERFORMANCE: No lock, direct array access.
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
	p.version.Add(1) // Invalidate any in-flight polls

	ev := &unix.EpollEvent{
		Events: eventsToEpoll(events),
		Fd:     int32(fd),
	}
	return unix.EpollCtl(int(p.epfd), unix.EPOLL_CTL_ADD, fd, ev)
}

// UnregisterFD removes a file descriptor from monitoring.
func (p *FastPoller) UnregisterFD(fd int) error {
	if fd < 0 || fd >= maxFDs {
		return ErrFDOutOfRange
	}
	if !p.fds[fd].active {
		return ErrFDNotRegistered
	}

	p.fds[fd] = fdInfo{} // Clear
	p.version.Add(1)

	return unix.EpollCtl(int(p.epfd), unix.EPOLL_CTL_DEL, fd, nil)
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

	ev := &unix.EpollEvent{
		Events: eventsToEpoll(events),
		Fd:     int32(fd),
	}
	return unix.EpollCtl(int(p.epfd), unix.EPOLL_CTL_MOD, fd, ev)
}

// PollIO polls for I/O events.
// PERFORMANCE: No lock during poll. Version-based consistency check.
// Returns the number of events processed.
func (p *FastPoller) PollIO(timeoutMs int) (int, error) {
	if p.closed.Load() {
		return 0, ErrPollerClosed
	}

	v := p.version.Load()

	n, err := unix.EpollWait(int(p.epfd), p.eventBuf[:], timeoutMs)
	if err != nil {
		if err == unix.EINTR {
			return 0, nil
		}
		return 0, err
	}

	// Check version after syscall
	if p.version.Load() != v {
		// Poller was modified, results may be stale - discard
		return 0, nil
	}

	// Dispatch events inline
	p.dispatchEvents(n)

	return n, nil
}

// dispatchEvents executes callbacks inline.
// PERFORMANCE: Inline execution, no collection overhead.
func (p *FastPoller) dispatchEvents(n int) {
	for i := 0; i < n; i++ {
		fd := int(p.eventBuf[i].Fd)
		if fd >= 0 && fd < maxFDs {
			info := &p.fds[fd]
			if info.active && info.callback != nil {
				events := epollToEvents(p.eventBuf[i].Events)
				info.callback(events)
			}
		}
	}
}

// eventsToEpoll converts IOEvents to epoll event flags.
func eventsToEpoll(events IOEvents) uint32 {
	var epollEvents uint32
	if events&EventRead != 0 {
		epollEvents |= unix.EPOLLIN
	}
	if events&EventWrite != 0 {
		epollEvents |= unix.EPOLLOUT
	}
	return epollEvents
}

// epollToEvents converts epoll event flags to IOEvents.
func epollToEvents(epollEvents uint32) IOEvents {
	var events IOEvents
	if epollEvents&unix.EPOLLIN != 0 {
		events |= EventRead
	}
	if epollEvents&unix.EPOLLOUT != 0 {
		events |= EventWrite
	}
	if epollEvents&unix.EPOLLERR != 0 {
		events |= EventError
	}
	if epollEvents&unix.EPOLLHUP != 0 {
		events |= EventHangup
	}
	return events
}
