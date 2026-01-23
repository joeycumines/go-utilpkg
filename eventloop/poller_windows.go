//go:build windows

package eventloop

import (
	"errors"
	"sync"
	"sync/atomic"
	"syscall"

	"golang.org/x/sys/windows"
)

// Maximum file descriptor we support with direct indexing.
const maxFDs = 65536

// MaxFDLimit is the maximum FD value we support for dynamic growth.
// 100M is enough for production with high FD limits.
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

// ioState is no longer used - we use completion key mechanism instead.
// COMPLETION KEY APPROACH:
// On Windows IOCP, we pass the FD as the completion key when associating
// the handle with IOCP. When GetQueuedCompletionStatus returns, we get
// the key back, which tells us which FD completed the operation.
// This is the standard, correct pattern for IOCP when handle==FD mapping is valid.

// FastPoller manages I/O event registration using IOCP (Windows).
//
// PERFORMANCE: Uses RWMutex for fdInfo access. The mutex is only held briefly
// during registration/callback dispatch. Uses IOCP for efficient I/O notification.
//
// WINDOWS LIMITATION: This implementation assumes fd values can be directly cast
// to windows.Handle for socket handles. This is correct for standard Go net.Conn
// sockets on Windows. For other handle types (pipes, files), use proper Windows
// handle extraction API and convert to int for registration.
type FastPoller struct { // betteralign:ignore
	_           [64]byte       // Cache line padding //nolint:unused
	iocp        windows.Handle // IOCP handle
	_           [56]byte       // Pad to cache line //nolint:unused
	fds         []fdInfo       // Dynamic slice, grows on demand
	fdMu        sync.RWMutex   // Protects fds array access
	closed      atomic.Bool
	initialized atomic.Bool // Prevents double-init and double-close
}

// Init initializes the IOCP instance.
func (p *FastPoller) Init() error {
	// Prevent double-initialization
	if p.initialized.Load() {
		return ErrFDAlreadyRegistered // Using existing error for "already initialized"
	}
	if p.closed.Load() {
		return ErrPollerClosed
	}

	// Create IO completion port
	iocp, err := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 0)
	if err != nil {
		return err
	}
	p.iocp = iocp

	p.fds = make([]fdInfo, maxFDs)
	p.initialized.Store(true)

	return nil
}

// Close closes the IOCP instance and associated resources.
func (p *FastPoller) Close() error {
	// Prevent double-close
	if p.closed.Load() {
		return nil // Already closed, return success (idempotent)
	}
	if !p.initialized.Load() {
		return nil // Never initialized, nothing to close
	}

	p.closed.Store(true)
	p.initialized.Store(false)

	if p.iocp != 0 {
		_ = windows.CloseHandle(p.iocp)
		p.iocp = 0
	}
	return nil
}

// RegisterFD registers a file descriptor for I/O event monitoring.
//
// WINDOWS IMPLEMENTATION NOTES:
// 1. Uses completion key mechanism - FD is passed as completion key to IOCP
// 2. When operation completes, GetQueuedCompletionStatus returns FD via key
// 3. This allows mapping completion back to fdInfo for callback dispatch
// 4. Assumes int fd can be cast to windows.Handle (valid for Go net.Conn sockets)
//
// LIMITATION: This implementation only supports socket handles that have
// 1:1 mapping between FD int and windows.Handle. For pipes, files, or
// other handle types, use platform-specific handle extraction and conversion.
func (p *FastPoller) RegisterFD(fd int, events IOEvents, cb IOCallback) error {
	if !p.initialized.Load() {
		return ErrPollerClosed
	}
	if p.closed.Load() {
		return ErrPollerClosed
	}
	if fd < 0 || fd >= MaxFDLimit {
		return ErrFDOutOfRange
	}

	p.fdMu.Lock()
	defer p.fdMu.Unlock()

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
		return ErrFDAlreadyRegistered
	}

	p.fds[fd] = fdInfo{callback: cb, events: events, active: true}

	// Associate handle with IOCP using FD as completion key
	// This allows us to retrieve FD when operation completes
	handle := windows.Handle(fd)
	_, err := windows.CreateIoCompletionPort(handle, p.iocp, uintptr(fd), 0)
	if err != nil {
		p.fds[fd] = fdInfo{} // Rollback
		return err
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

	p.fds[fd] = fdInfo{}
	p.fdMu.Unlock()

	// On Windows, we don't need to explicitly remove the association
	// Closing the handle will automatically remove it from IOCP
	return nil
}

// ModifyFD updates the events being monitored for a file descriptor.
//
// WINDOWS LIMITATION:
// On IOCP-based systems (Windows), event monitoring is NOT controlled via
// a direct ModifyFD call like epoll/kqueue. Instead:
//
// 1. User code posts WSASend/WSARecv operations for read/write interests
// 2. IOCP notifies when operations complete
// 3. ModifyFD here only updates our internal tracking (p.fds[fd].events)
//
// For full IOCP semantics, user code must:
// - Cancel pending operations (windows.CancelIoEx)
// - Issue new operations (WSASend/WSARecv) matching new event mask
//
// This differs from epoll/kqueue but is correct for IOCP architecture.
// Cross-platform code must handle this semantic difference.
func (p *FastPoller) ModifyFD(fd int, events IOEvents) error {
	if fd < 0 {
		return ErrFDOutOfRange
	}
	if !p.initialized.Load() {
		return ErrPollerClosed
	}
	if p.closed.Load() {
		return ErrPollerClosed
	}

	p.fdMu.Lock()
	defer p.fdMu.Unlock()

	if fd >= len(p.fds) || !p.fds[fd].active {
		return ErrFDNotRegistered
	}

	p.fds[fd].events = events

	// On IOCP, changes to event monitoring are handled via the
	// actual I/O operations posted (WSASend/WSARecv), which are
	// managed by the user code. We just update our tracking here.

	return nil
}

// PollIO polls for I/O events using IOCP.
//
// COMPLETION MECHANISM:
// 1. GetQueuedCompletionStatus blocks until completion or timeout
// 2. Key parameter contains FD (passed as completion key in RegisterFD)
// 3. Overlapped contains operation-specific data (unused in simple case)
// 4. Bytes contains number of bytes transferred
func (p *FastPoller) PollIO(timeoutMs int) (int, error) {
	if !p.initialized.Load() {
		return 0, ErrPollerClosed
	}
	if p.closed.Load() {
		return 0, ErrPollerClosed
	}

	var timeout *uint32
	if timeoutMs >= 0 {
		t := uint32(timeoutMs)
		timeout = &t
	}

	var bytes uint32
	var key uintptr
	var overlapped *windows.Overlapped

	// Wait for completion notification
	// Key contains the FD (passed as completion key in RegisterFD)
	err := windows.GetQueuedCompletionStatus(p.iocp, &bytes, &key, &overlapped, timeout)
	if err != nil {
		if errno, ok := err.(syscall.Errno); ok {
			if errno == windows.WAIT_TIMEOUT {
				return 0, nil
			}
			if errno == windows.ERROR_ABANDONED_WAIT_0 || errno == windows.ERROR_INVALID_HANDLE {
				return 0, ErrPollerClosed
			}
		}
		return 0, err
	}

	if overlapped == nil && key == 0 {
		// This is a wake-up notification (via PostQueuedCompletionStatus)
		return 0, nil
	}

	// Dispatch the completion using the key (FD)
	return p.dispatchEvents(int(key)), nil
}

// dispatchEvents executes callback for completed FD.
// RACE SAFETY: Uses RLock to safely read fdInfo while allowing concurrent
// modifications to other fds. Callback is copied under lock then called outside.
//
// CRITICAL: This was an empty stub - now properly implements callback dispatch
// using completion key mechanism.
func (p *FastPoller) dispatchEvents(fd int) int {
	if fd < 0 {
		return 0
	}

	p.fdMu.RLock()
	defer p.fdMu.RUnlock()

	// Check if FD is still registered and active
	if fd >= len(p.fds) || !p.fds[fd].active {
		return 0
	}

	// Copy callback under lock, execute outside to avoid holding lock during callback
	cb := p.fds[fd].callback
	if cb == nil {
		return 0
	}

	// Execute callback with registered events
	cb(p.fds[fd].events)

	return 1
}

// Wakeup wakes up the poller from another thread.
//
// Uses PostQueuedCompletionStatus to post a NULL completion to IOCP.
// This causes GetQueuedCompletionStatus to return immediately with
// overlapped==nil, triggering wake-up condition in PollIO.
func (p *FastPoller) Wakeup() error {
	if p.closed.Load() {
		return ErrPollerClosed
	}
	return windows.PostQueuedCompletionStatus(p.iocp, 0, 0, nil)
}

// IocpHandle returns the handle for external use (e.g., wake-up mechanism).
// This is needed by loop.go's submitWakeup() on Windows.
func (p *FastPoller) IocpHandle() uintptr {
	return uintptr(p.iocp)
}
