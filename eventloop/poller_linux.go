//go:build linux

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
// Note: This must be < math.MaxInt32 (2147483647) because EpollEvent.Fd is int32.
// 100M is enough for production with ulimit -n > 1M.
const maxFDLimit = 100000000

// IOEvents represents the type of I/O events to monitor.
type IOEvents uint32

const (
	// EventRead indicates readability (data available to read).
	EventRead IOEvents = 1 << iota
	// EventWrite indicates writability (buffer space available to write).
	EventWrite
	// EventError indicates an error condition on the file descriptor.
	EventError
	// EventHangup indicates the remote end has closed the connection.
	EventHangup
)

var (
	// ErrFDOutOfRange is returned when a file descriptor exceeds the maximum supported value.
	ErrFDOutOfRange = errors.New("eventloop: fd out of range (max 100000000)")
	// ErrFDAlreadyRegistered is returned when attempting to register an FD that is already registered.
	ErrFDAlreadyRegistered = errors.New("eventloop: fd already registered")
	// ErrFDNotRegistered is returned when attempting to modify or unregister an FD that is not registered.
	ErrFDNotRegistered = errors.New("eventloop: fd not registered")
	// ErrPollerClosed is returned when performing operations on a closed poller.
	ErrPollerClosed = errors.New("eventloop: poller closed")
	// ErrPollerAlreadyInitialized is returned when Init is called on an already initialized poller.
	ErrPollerAlreadyInitialized = errors.New("eventloop: poller already initialized")
)

// ioCallback is the callback type for I/O events.
type ioCallback func(IOEvents)

// fdInfo stores per-FD callback information.
type fdInfo struct {
	callback ioCallback
	events   IOEvents
	active   bool
}

// fastPoller manages I/O event registration using epoll (Linux).
//
// PERFORMANCE: RWMutex design with dynamic FD indexing.
// It uses a dynamic slice instead of a fixed array for flexible FD support.
//
// CACHE LINE PADDING: Padding fields (marked with //nolint:unused) isolate
// frequently-accessed fields (epfd, closed) to reduce false sharing across cache lines.
// The betteralign tool ensures correct cache line alignment during struct layout optimization.
type fastPoller struct { // betteralign:ignore
	_           [sizeOfCacheLine]byte     // Cache line padding before epfd (isolates from previous fields) //nolint:unused
	epfd        int32                     // epoll file descriptor
	_           [sizeOfCacheLine - 4]byte // Padding to isolate eventBuf from isolated field //nolint:unused
	eventBuf    [256]unix.EpollEvent      // Preallocated event buffer (256 epoll events)
	fds         []fdInfo                  // Dynamic slice, grows on demand
	fdMu        sync.RWMutex              // Protects fds array access
	_           [sizeOfCacheLine]byte     // Cache line padding before closed (isolates from previous fields) //nolint:unused
	closed      atomic.Bool               // Closed flag
	_           [sizeOfCacheLine - 1]byte // Padding to isolate initialized from closed //nolint:unused
	initialized atomic.Bool               // Initialization flag
}

// Init initializes the epoll instance.
func (p *fastPoller) Init() error {
	// Prevent double-initialization (would leak epoll fd)
	if p.initialized.Load() {
		return ErrPollerAlreadyInitialized
	}
	if p.closed.Load() {
		return ErrPollerClosed
	}

	epfd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		return err
	}
	p.epfd = int32(epfd)

	p.fds = make([]fdInfo, maxFDs)
	p.initialized.Store(true)

	return nil
}

// Close closes the epoll instance.
func (p *fastPoller) Close() error {
	if p.closed.Swap(true) {
		// Already closed, return nil for idempotent behavior
		return nil
	}
	if p.epfd > 0 {
		return unix.Close(int(p.epfd))
	}
	return nil
}

// Wakeup is a stub on Linux/Darwin platforms.
// On these platforms, wake-up is handled by writing to the eventfd/pipe
// rather than calling Wakeup() on the poller. This method exists primarily
// for API compatibility with Windows, which uses PostQueuedCompletionStatus.
// It returns nil because the eventfd mechanism is used instead.
//
// NOTE: This method should never be called on Linux/Darwin under normal
// operation. The loop.go submitWakeup() function checks wakePipe < 0 and
// writes to the wake pipe for Unix platforms instead of calling Wakeup().
func (p *fastPoller) Wakeup() error {
	// Linux: Write to wake eventfd in submitWakeup()
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

	// Hold lock across EpollCtl to prevent race with concurrent UnregisterFD.
	// Without this, UnregisterFD could clear fds[fd] and call EpollCtl(DEL)
	// between our unlock and our EpollCtl(ADD), causing DEL to get ENOENT
	// (fd not yet added) and the count to leak.
	ev := &unix.EpollEvent{
		Events: eventsToEpoll(events),
		Fd:     int32(fd),
	}
	err := unix.EpollCtl(int(p.epfd), unix.EPOLL_CTL_ADD, fd, ev)
	if err != nil {
		p.fds[fd] = fdInfo{} // Rollback
		p.fdMu.Unlock()
		return err
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

	// Remove from epoll while holding lock to prevent race with RegisterFD.
	// Order: DEL from epoll first, then clear fds entry. This ensures
	// the fd is fully removed from both epoll and fds atomically.
	err := unix.EpollCtl(int(p.epfd), unix.EPOLL_CTL_DEL, fd, nil)
	if err != nil {
		p.fdMu.Unlock()
		return err
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

	p.fds[fd].events = events
	p.fdMu.Unlock()

	ev := &unix.EpollEvent{
		Events: eventsToEpoll(events),
		Fd:     int32(fd),
	}
	return unix.EpollCtl(int(p.epfd), unix.EPOLL_CTL_MOD, fd, ev)
}

// PollIO polls for I/O events.
func (p *fastPoller) PollIO(timeoutMs int) (int, error) {
	if p.closed.Load() {
		return 0, ErrPollerClosed
	}

	n, err := unix.EpollWait(int(p.epfd), p.eventBuf[:], timeoutMs)
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
		fd := int(p.eventBuf[i].Fd)
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
			events := epollToEvents(p.eventBuf[i].Events)
			info.callback(events)
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
