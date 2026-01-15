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

// MaxFDLimit is the maximum FD value we support for dynamic growth.
const MaxFDLimit = 100000000 // 100M, enough for production with ulimit -n > 1M

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
	ErrFDOutOfRange        = errors.New("eventloop: fd out of range (max 100000000)")
	ErrFDAlreadyRegistered = errors.New("eventloop: fd already registered")
	ErrFDNotRegistered     = errors.New("eventloop: fd not registered")
	ErrPollerClosed        = errors.New("eventloop: poller closed")
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
// PERFORMANCE: RWMutex design with dynamic FD indexing.
// - Dynamic slice instead of fixed array for flexible FD support
// - RWMutex for thread-safe access to fds array
// - Inline callback execution
// - No version check - rely on fdMu for concurrent modification safety
type FastPoller struct { // betteralign:ignore
	_        [64]byte             // Cache line padding //nolint:unused
	epfd     int32                // epoll file descriptor
	_        [60]byte             // Pad to cache line //nolint:unused
	eventBuf [256]unix.EpollEvent // Larger buffer, preallocated
	fds      []fdInfo             // Dynamic slice, grows on demand
	fdMu     sync.RWMutex         // Protects fds array access
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

	// Initialize dynamic FD slice with initial capacity
	p.fds = make([]fdInfo, maxFDs)

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
// THREAD SAFE: Uses fdMu for array access.
func (p *FastPoller) RegisterFD(fd int, events IOEvents, cb IOCallback) error {
	if p.closed.Load() {
		return ErrPollerClosed
	}
	if fd < 0 {
		return ErrFDOutOfRange
	}
	if fd >= MaxFDLimit {
		return ErrFDOutOfRange
	}

	p.fdMu.Lock()
	// Grow slice if necessary
	if fd >= len(p.fds) {
		// Grow in chunks to minimize allocations
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

	ev := &unix.EpollEvent{
		Events: eventsToEpoll(events),
		Fd:     int32(fd),
	}
	err := unix.EpollCtl(int(p.epfd), unix.EPOLL_CTL_ADD, fd, ev)
	if err != nil {
		p.fdMu.Lock()
		p.fds[fd] = fdInfo{} // Rollback
		p.fdMu.Unlock()
		return err
	}
	return nil
}

// UnregisterFD removes a file descriptor from monitoring.//
// IMPORTANT: CALLBACK LIFETIME SAFETY
// ====================================
//
// UnregisterFD does NOT guarantee immediate cessation of in-flight callbacks.
// The dispatch logic copies callback pointers under RLock, releases the lock,
// then executes callbacks OUTSIDE the lock. This design choice avoids:
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
//  1. Close FD ONLY after all callbacks have completed
//  2. Use synchronization (e.g., sync.WaitGroup) if exact timing matters
//  3. Callbacks must guard against accessing closed FDs
//
// This is correct implementation for high-performance I/O multiplexing.
// Alternative approaches (e.g., marking callbacks for deferred deletion)
// introduce overhead and are unnecessary for typical use cases.
func (p *FastPoller) UnregisterFD(fd int) error {
	if fd < 0 {
		return ErrFDOutOfRange
	}

	p.fdMu.Lock()
	if fd >= len(p.fds) || !p.fds[fd].active {
		p.fdMu.Unlock()
		return ErrFDNotRegistered
	}

	p.fds[fd] = fdInfo{} // Clear
	p.fdMu.Unlock()

	return unix.EpollCtl(int(p.epfd), unix.EPOLL_CTL_DEL, fd, nil)
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

	p.fds[fd].events = events
	p.fdMu.Unlock()

	ev := &unix.EpollEvent{
		Events: eventsToEpoll(events),
		Fd:     int32(fd),
	}
	return unix.EpollCtl(int(p.epfd), unix.EPOLL_CTL_MOD, fd, ev)
}

// PollIO polls for I/O events.
// PERFORMANCE: No lock during poll. No version check relies on fdMu for safety.
// Returns the number of events processed.
func (p *FastPoller) PollIO(timeoutMs int) (int, error) {
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

	// Dispatch events inline
	p.dispatchEvents(n)

	return n, nil
}

// dispatchEvents executes callbacks inline.
// RACE SAFETY: Uses RLock to safely read fdInfo while allowing concurrent
// modifications to other fds. Callback is copied under lock then called outside.
func (p *FastPoller) dispatchEvents(n int) {
	for i := 0; i < n; i++ {
		fd := int(p.eventBuf[i].Fd)
		if fd < 0 {
			continue
		}

		// Copy fdInfo under read lock
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

// ============================================================================
// LEGACY COMPATIBILITY - ioPoller shim for existing tests
// ============================================================================

// ioPoller wraps FastPoller for backward compatibility with tests.
type ioPoller struct {
	mu      sync.RWMutex // For test compatibility
	p       FastPoller
	once    sync.Once   // DEFECT #1 FIX: sync.Once ensures all callers block until init completes
	closed  atomic.Bool // DEFECT #2 FIX: atomic.Bool prevents data race between init and close
	initErr error       // Captures init error for callers that didn't win the once.Do
}

// initPoller initializes the epoll poller using sync.Once to prevent concurrent initialization.
// DEFECT #1 FIX: sync.Once ensures all concurrent callers block until initialization completes,
// rather than returning nil prematurely when losing the CAS race.
func (p *ioPoller) initPoller() error {
	// DEFECT #2 FIX: Use atomic.Bool.Load() to avoid data race
	if p.closed.Load() {
		return ErrPollerClosed
	}

	// sync.Once guarantees:
	// 1. The init function runs exactly once
	// 2. All callers block until the function completes
	// 3. Memory effects are synchronized - initErr is visible after Do returns
	p.once.Do(func() {
		// Double-check closed inside Once to handle race with closePoller
		if p.closed.Load() {
			p.initErr = ErrPollerClosed
			return
		}
		p.initErr = p.p.Init()
	})

	return p.initErr
}

func (p *ioPoller) closePoller() error {
	// DEFECT #2 FIX: Use atomic.Bool.Store() to avoid data race
	p.closed.Store(true)
	return p.p.Close()
}

// pollIO is a compatibility method for ioPoller.
func (p *ioPoller) pollIO(timeout int, maxEvents int) (int, error) {
	return p.p.PollIO(timeout)
}

// DEFECT #9 FIX: Add pollIO method on Loop for platform consistency with Darwin.
// pollIO is a compatibility method for Loop.pollIO calls.
func (l *Loop) pollIO(timeout int, maxEvents int) (int, error) {
	return l.poller.PollIO(timeout)
}
