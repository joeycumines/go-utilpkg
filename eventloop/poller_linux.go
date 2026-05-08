//go:build linux

package eventloop

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
)

const fdPollingSupported = true

// fdInfo stores per-FD callback information.
type fdInfo struct {
	callback     ioCallback
	dispatch     *fdDispatchGate
	generation   uint64
	pollFD       int
	events       IOEvents
	active       bool
	internal     bool
	provisional  bool
	kernelActive bool
	ownsPollFD   bool
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
	_                   [sizeOfCacheLine]byte                          // Cache line padding before epfd (isolates from previous fields) //nolint:unused
	epfd                int32                                          // epoll file descriptor
	_                   [sizeOfCacheLine - 4]byte                      // Padding to isolate eventBuf from isolated field //nolint:unused
	eventBuf            [256]unix.EpollEvent                           // Preallocated event buffer (256 epoll events)
	fds                 []fdInfo                                       // Dynamic slice, grows on demand
	sparseFDs           map[int]fdInfo                                 // Sparse entries for high-numbered FDs
	tokenFDs            map[uint64]int                                 // Kernel-carried registration token to numeric FD
	readyEvents         []pollEvent                                    // Ready callbacks collected by PollIO for loop-owned dispatch
	rebuildNeeded       bool                                           // Retired absent watch requires epoll-set rebuild before the next wait
	descriptorClose     func(int) error                                // Optional deterministic descriptor-close seam used by tests
	pollerCreate        func() (int, error)                            // Optional deterministic native-creation seam used by tests
	beforeNativePoll    func()                                         // Optional deterministic poll-ownership seam used by tests
	afterNativePoll     func()                                         // Optional deterministic native-result ownership seam used by tests
	beforeResourceClose func()                                         // Optional deterministic close-ownership seam used by tests
	beforeDispatchWait  func()                                         // Optional deterministic dispatch-start join seam used by tests
	epollCtl            func(int, int, int, *unix.EpollEvent) error    // Optional deterministic native-control seam used by tests
	epollWait           func(int, []unix.EpollEvent, int) (int, error) // Optional deterministic native-wait seam used by tests
	lifecycleMu         sync.Mutex                                     // Serializes native descriptor initialization and release
	resourceMu          sync.RWMutex                                   // Joins native waits with descriptor close/reuse
	pollMu              sync.Mutex                                     // Serializes PollIO access to eventBuf/readyEvents
	fdMu                sync.RWMutex                                   // Protects fds array access
	readyMu             sync.Mutex                                     // Protects ready callback ownership against Close
	fdGeneration        atomic.Uint64                                  // Monotonic registration generation for stale ready-event rejection
	_                   [sizeOfCacheLine]byte                          // Cache line padding before closed (isolates from previous fields) //nolint:unused
	closed              atomic.Bool                                    // Closed flag
	_                   [sizeOfCacheLine - 1]byte                      // Padding to isolate initialized from closed //nolint:unused
	initialized         atomic.Bool                                    // Initialization flag
}

func newFastPoller() fastPoller {
	return fastPoller{epfd: -1}
}

// Init initializes the epoll instance.
func (p *fastPoller) Init() error {
	p.lifecycleMu.Lock()
	defer p.lifecycleMu.Unlock()

	// Prevent double-initialization (would leak epoll fd)
	if p.initialized.Load() {
		return errPollerAlreadyInitialized
	}
	if p.closed.Load() {
		return errPollerClosed
	}

	p.epfd = -1
	var epfd int
	var err error
	if p.pollerCreate != nil {
		epfd, err = p.pollerCreate()
	} else {
		epfd, err = unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	}
	if err != nil {
		return err
	}
	p.epfd = int32(epfd)

	p.fdMu.Lock()
	p.initFDTable()
	p.fdMu.Unlock()
	p.initialized.Store(true)

	return nil
}

// Close closes the epoll instance.
func (p *fastPoller) Close() error {
	p.lifecycleMu.Lock()
	if p.closed.Swap(true) {
		p.lifecycleMu.Unlock()
		return nil
	}
	initialized := p.initialized.Swap(false)
	epfd := p.epfd
	p.epfd = -1
	if p.beforeResourceClose != nil {
		p.beforeResourceClose()
	}
	p.resourceMu.Lock()
	var err error
	if initialized && epfd >= 0 {
		err = p.closeDescriptor(int(epfd))
	}
	p.fdMu.Lock()
	ownedPollFDs := p.ownedPollFDsLocked()
	gates := p.clearFDTableLocked()
	p.rebuildNeeded = false
	p.fdMu.Unlock()
	p.readyMu.Lock()
	p.readyEvents = nil
	p.readyMu.Unlock()
	p.resourceMu.Unlock()
	for _, fd := range ownedPollFDs {
		err = joinErrors(err, p.closeDescriptor(fd))
	}

	for _, gate := range gates {
		gate.waitPendingStarts()
	}
	p.lifecycleMu.Unlock()
	return err
}

// RegisterFD registers a file descriptor for I/O event monitoring.
func (p *fastPoller) RegisterFD(fd int, events IOEvents, cb ioCallback) error {
	return p.registerFD(fd, events, cb, newFDDispatchGate(true), false)
}

func (p *fastPoller) stageFD(fd int, events IOEvents, cb ioCallback, dispatch *fdDispatchGate) error {
	return p.registerFD(fd, events, cb, dispatch, true)
}

func (p *fastPoller) registerFD(fd int, events IOEvents, cb ioCallback, dispatch *fdDispatchGate, provisional bool) error {
	if err := validateFDRegistration(events, cb); err != nil {
		return err
	}
	p.lifecycleMu.Lock()
	defer p.lifecycleMu.Unlock()
	if p.closed.Load() || !p.initialized.Load() {
		return errPollerClosed
	}
	if fd < 0 {
		return errFDNegative
	}

	p.fdMu.Lock()
	if _, active := p.fdInfoLocked(fd); active {
		p.fdMu.Unlock()
		return ErrFDAlreadyRegistered
	}

	generation, err := p.nextFDGenerationLocked()
	if err != nil {
		p.fdMu.Unlock()
		return err
	}
	pollFD, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD_CLOEXEC, 0)
	if err != nil {
		p.fdMu.Unlock()
		return err
	}
	p.setFDInfoLocked(fd, fdInfo{callback: cb, dispatch: dispatch, generation: generation, events: events, active: true, provisional: provisional, kernelActive: true, pollFD: pollFD, ownsPollFD: true})

	// Hold lock across EpollCtl to prevent race with concurrent UnregisterFD.
	// Without this, UnregisterFD could clear fds[fd] and call EpollCtl(DEL)
	// between our unlock and our EpollCtl(ADD), causing DEL to get ENOENT
	// (fd not yet added) and the count to leak.
	ev := newEpollEvent(generation, eventsToEpoll(events))
	err = p.callEpollCtl(int(p.epfd), unix.EPOLL_CTL_ADD, pollFD, ev)
	if err != nil {
		p.clearFDInfoLocked(fd) // Rollback
		p.fdMu.Unlock()
		return joinErrors(err, p.closeDescriptor(pollFD))
	}
	p.fdMu.Unlock()
	return nil
}

func (p *fastPoller) commitFD(fd int) error {
	p.lifecycleMu.Lock()
	defer p.lifecycleMu.Unlock()
	if p.closed.Load() || !p.initialized.Load() {
		return errPollerClosed
	}
	p.fdMu.Lock()
	defer p.fdMu.Unlock()
	info, active := p.fdInfoLocked(fd)
	if !active {
		return ErrFDNotRegistered
	}
	info.provisional = false
	p.setFDInfoLocked(fd, info)
	return nil
}

// UnregisterFD removes a file descriptor from monitoring.
//
// CALLBACK LIFETIME SAFETY:
// Ready events carry the registration generation observed during PollIO, and
// loop-owned dispatch revalidates that generation immediately before invoking
// user callbacks. UnregisterFD therefore suppresses stale, not-yet-dispatched
// ready events, including unregister/re-register ABA cases. A callback already
// executing on the loop goroutine may still complete normally.
func (p *fastPoller) UnregisterFD(fd int) error {
	if fd < 0 {
		return errFDNegative
	}

	p.lifecycleMu.Lock()
	defer p.lifecycleMu.Unlock()
	if p.closed.Load() || !p.initialized.Load() {
		return errPollerClosed
	}
	p.fdMu.Lock()
	info, active := p.fdInfoLocked(fd)
	if !active {
		p.fdMu.Unlock()
		return ErrFDNotRegistered
	}

	// Remove from epoll while holding lock to prevent race with RegisterFD.
	// A detached entry was already proven absent while rebuilding the epoll set.
	var err error
	if info.kernelActive {
		err = p.callEpollCtl(int(p.epfd), unix.EPOLL_CTL_DEL, info.pollFD, nil)
	}
	if err != nil && !epollRegistrationAbsent(err) {
		p.fdMu.Unlock()
		return &FDUnregisterError{cause: err}
	}
	if epollRegistrationAbsent(err) {
		p.rebuildNeeded = true
	}

	p.clearFDInfoLocked(fd)
	p.fdMu.Unlock()
	if info.ownsPollFD {
		if closeErr := p.closeDescriptor(info.pollFD); closeErr != nil {
			p.waitPendingDispatchStarts(info.dispatch)
			return &FDUnregisterError{cause: closeErr, released: true}
		}
	}
	p.waitPendingDispatchStarts(info.dispatch)
	return nil
}

func (p *fastPoller) unregisterFD(fd int, _ bool) error {
	return p.UnregisterFD(fd)
}

func (p *fastPoller) waitPendingDispatchStarts(dispatch *fdDispatchGate) {
	if p.beforeDispatchWait != nil {
		p.beforeDispatchWait()
	}
	dispatch.waitPendingStarts()
}

// ModifyFD updates the events being monitored for a file descriptor.
func (p *fastPoller) ModifyFD(fd int, events IOEvents) error {
	if fd < 0 {
		return errFDNegative
	}
	if err := validateFDModification(events); err != nil {
		return err
	}

	p.lifecycleMu.Lock()
	defer p.lifecycleMu.Unlock()
	if p.closed.Load() || !p.initialized.Load() {
		return errPollerClosed
	}
	p.fdMu.Lock()
	info, active := p.fdInfoLocked(fd)
	if !active {
		p.fdMu.Unlock()
		return ErrFDNotRegistered
	}
	if !info.kernelActive {
		p.fdMu.Unlock()
		return unix.ENOENT
	}

	ev := newEpollEvent(info.generation, eventsToEpoll(events))
	if err := p.callEpollCtl(int(p.epfd), unix.EPOLL_CTL_MOD, info.pollFD, ev); err != nil {
		p.fdMu.Unlock()
		return err
	}
	info.events = events
	p.setFDInfoLocked(fd, info)
	p.fdMu.Unlock()
	return nil
}

// PollIO polls for I/O events.
func (p *fastPoller) PollIO(timeoutMs int) (int, error) {
	p.pollMu.Lock()
	defer p.pollMu.Unlock()
	p.lifecycleMu.Lock()
	if p.closed.Load() || !p.initialized.Load() || p.epfd < 0 {
		p.lifecycleMu.Unlock()
		return 0, errPollerClosed
	}
	p.lifecycleMu.Unlock()
	p.clearReadyEvents()

	readyCount := 0
	wait := func(timeout time.Duration) (int, error) {
		p.lifecycleMu.Lock()
		if p.closed.Load() || !p.initialized.Load() || p.epfd < 0 {
			p.lifecycleMu.Unlock()
			return 0, errPollerClosed
		}
		if p.rebuildNeeded {
			if err := p.rebuildEpollLocked(); err != nil {
				p.lifecycleMu.Unlock()
				return 0, err
			}
		}
		epfd := int(p.epfd)
		p.lifecycleMu.Unlock()
		p.resourceMu.RLock()
		defer p.resourceMu.RUnlock()
		if p.beforeNativePoll != nil {
			p.beforeNativePoll()
		}
		if p.closed.Load() {
			return 0, errPollerClosed
		}
		epollWait := unix.EpollWait
		if p.epollWait != nil {
			epollWait = p.epollWait
		}
		n, err := epollWait(epfd, p.eventBuf[:], epollWaitMillis(timeout))
		if p.afterNativePoll != nil {
			p.afterNativePoll()
		}
		if err == nil && n > 0 {
			if p.closed.Load() {
				return 0, errPollerClosed
			}
			readyCount = p.dispatchEvents(n)
		}
		return n, err
	}
	poll := func(timeout int) (int, error) {
		return waitPoll(timeout, time.Second, time.Now, wait)
	}
	var n int
	var err error
	if timeoutMs < 0 {
		for err == nil && n == 0 {
			n, err = poll(1000)
		}
	} else {
		_, err = poll(timeoutMs)
	}
	if err != nil {
		return 0, err
	}

	if p.closed.Load() {
		p.clearReadyEvents()
		return 0, errPollerClosed
	}
	return readyCount, nil
}

func (p *fastPoller) rebuildEpollLocked() error {
	epfd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		return err
	}
	p.fdMu.Lock()
	for fd := range p.fds {
		info := p.fds[fd]
		if !info.active {
			continue
		}
		if !info.kernelActive {
			continue
		}
		current, verifyErr := p.verifyEpollRegistration(int(p.epfd), info.pollFD, info)
		if verifyErr != nil {
			err = verifyErr
			break
		}
		if !current {
			info.kernelActive = false
			p.fds[fd] = info
			continue
		}
		if err = p.callEpollCtl(epfd, unix.EPOLL_CTL_ADD, info.pollFD, newEpollEvent(info.generation, eventsToEpoll(info.events))); err != nil {
			break
		}
	}
	if err == nil {
		for fd, info := range p.sparseFDs {
			if !info.active {
				continue
			}
			if !info.kernelActive {
				continue
			}
			current, verifyErr := p.verifyEpollRegistration(int(p.epfd), info.pollFD, info)
			if verifyErr != nil {
				err = verifyErr
				break
			}
			if !current {
				info.kernelActive = false
				p.sparseFDs[fd] = info
				continue
			}
			if err = p.callEpollCtl(epfd, unix.EPOLL_CTL_ADD, info.pollFD, newEpollEvent(info.generation, eventsToEpoll(info.events))); err != nil {
				break
			}
		}
	}
	p.fdMu.Unlock()
	if err != nil {
		return joinErrors(err, p.closeDescriptor(epfd))
	}

	p.resourceMu.Lock()
	oldEPFD := p.epfd
	p.epfd = int32(epfd)
	p.rebuildNeeded = false
	if oldEPFD >= 0 {
		err = p.closeDescriptor(int(oldEPFD))
	}
	p.resourceMu.Unlock()
	return err
}

func (p *fastPoller) verifyEpollRegistration(epfd, fd int, info fdInfo) (bool, error) {
	err := p.callEpollCtl(epfd, unix.EPOLL_CTL_MOD, fd, newEpollEvent(info.generation, eventsToEpoll(info.events)))
	if epollRegistrationAbsent(err) {
		return false, nil
	}
	return err == nil, err
}

func epollRegistrationAbsent(err error) bool {
	return errors.Is(err, unix.EBADF) || errors.Is(err, unix.ENOENT) || errors.Is(err, unix.EPERM)
}

func (p *fastPoller) ownedPollFDsLocked() []int {
	var result []int
	for i := range p.fds {
		if info := p.fds[i]; info.active && info.ownsPollFD {
			result = append(result, info.pollFD)
		}
	}
	for _, info := range p.sparseFDs {
		if info.active && info.ownsPollFD {
			result = append(result, info.pollFD)
		}
	}
	return result
}

func (p *fastPoller) closeDescriptor(fd int) error {
	if p.descriptorClose != nil {
		return p.descriptorClose(fd)
	}
	return unix.Close(fd)
}

func (p *fastPoller) callEpollCtl(epfd, operation, fd int, event *unix.EpollEvent) error {
	if p.epollCtl != nil {
		return p.epollCtl(epfd, operation, fd, event)
	}
	return unix.EpollCtl(epfd, operation, fd, event)
}

func epollWaitMillis(timeout time.Duration) int {
	if timeout <= 0 {
		return int(timeout)
	}
	return int((timeout-1)/time.Millisecond + 1)
}

// dispatchEvents collects callbacks for loop-owned dispatch.
// RACE SAFETY: Uses RLock to safely read fdInfo while allowing concurrent
// modifications to other fds. Callback is copied under lock and returned to the
// loop, which restores StateRunning before executing user callbacks.
func (p *fastPoller) dispatchEvents(n int) int {
	p.clearReadyEvents()
	for i := 0; i < n; i++ {
		p.appendReadyToken(epollEventToken(&p.eventBuf[i]), epollToEvents(p.eventBuf[i].Events))
	}
	return len(p.readyEventsSnapshot())
}

func newEpollEvent(generation uint64, events uint32) *unix.EpollEvent {
	return &unix.EpollEvent{
		Events: events,
		Fd:     int32(uint32(generation)),
		Pad:    int32(uint32(generation >> 32)),
	}
}

func epollEventToken(event *unix.EpollEvent) uint64 {
	if event == nil {
		return 0
	}
	return uint64(uint32(event.Fd)) | uint64(uint32(event.Pad))<<32
}

// eventsToEpoll converts IOEvents to epoll event flags.
func eventsToEpoll(events IOEvents) uint32 {
	epollEvents := uint32(unix.EPOLLRDHUP)
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
	if epollEvents&(unix.EPOLLRDHUP|unix.EPOLLHUP) != 0 {
		events |= EventHangup
	}
	return events
}
