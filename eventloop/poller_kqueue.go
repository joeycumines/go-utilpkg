//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package eventloop

import (
	"errors"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const fdPollingSupported = true

// fdInfo stores per-FD callback information.
type fdInfo struct {
	callback    ioCallback
	dispatch    *fdDispatchGate
	kernelTag   keventTag
	generation  uint64
	pollFD      int
	events      IOEvents
	active      bool
	internal    bool
	provisional bool
	ownsPollFD  bool
}

// fastPoller manages I/O event registration using kqueue.
//
// PERFORMANCE: Uses RWMutex for fdInfo access. The mutex is only held briefly
// during registration/callback dispatch. The polling syscall itself is lock-free.
// It uses a dynamic slice instead of a fixed array for flexible FD support.
//
// CACHE LINE PADDING: Padding fields (marked with //nolint:unused) isolate
// frequently-accessed fields (kq, closed) to reduce false sharing across cache lines.
// The betteralign tool ensures correct cache line alignment during struct layout optimization.
type fastPoller struct { // betteralign:ignore
	_                   [sizeOfCacheLine]byte     // Cache line padding before kq (isolates from previous fields) //nolint:unused
	kq                  int32                     // kqueue file descriptor
	_                   [sizeOfCacheLine - 4]byte // Padding to isolate eventBuf from isolated field //nolint:unused
	eventBuf            [256]unix.Kevent_t        // Preallocated event buffer (256 kevents)
	fds                 []fdInfo                  // Dynamic slice, grows on demand
	sparseFDs           map[int]fdInfo            // Sparse entries for high-numbered FDs
	tokenFDs            map[uint64]int            // Kernel-carried registration token to numeric FD
	kernelTags          map[keventTag]uint64      // Stable kernel-carried tag to live registration token
	kernelTagStore      keventTagStore            // OS-specific tag storage retained until native waits join retirement
	readyEvents         []pollEvent               // Ready callbacks collected by PollIO for loop-owned dispatch
	keventChange        keventChangeCall          // Optional deterministic native-change seam used by tests
	descriptorClose     func(int) error           // Optional deterministic descriptor-close seam used by tests
	pollerCreate        func() (int, error)       // Optional deterministic native-creation seam used by tests
	beforeNativePoll    func()                    // Optional deterministic poll-ownership seam used by tests
	afterNativePoll     func()                    // Optional deterministic native-result ownership seam used by tests
	beforeResourceClose func()                    // Optional deterministic close-ownership seam used by tests
	beforeDispatchWait  func()                    // Optional deterministic dispatch-start join seam used by tests
	beforeTagRecycle    func()                    // Optional deterministic retired-Udata barrier seam used by tests
	lifecycleMu         sync.Mutex                // Serializes native descriptor initialization and release
	resourceMu          sync.RWMutex              // Joins native waits with descriptor close/reuse and Udata unpinning
	pollMu              sync.Mutex                // Serializes PollIO access to eventBuf/readyEvents
	fdMu                sync.RWMutex              // Protects fds array access
	readyMu             sync.Mutex                // Protects ready callback ownership against Close
	fdGeneration        atomic.Uint64             // Monotonic registration generation for stale ready-event rejection
	_                   [sizeOfCacheLine]byte     // Cache line padding before closed (isolates from previous fields) //nolint:unused
	closed              atomic.Bool               // Closed flag
	_                   [sizeOfCacheLine - 1]byte // Padding to isolate initialized from closed //nolint:unused
	initialized         atomic.Bool               // Initialization flag
}

func newFastPoller() fastPoller {
	return fastPoller{kq: -1}
}

func createKqueueDescriptor(create func() (int, error)) (int, error) {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()

	fd, err := create()
	if err != nil {
		return -1, err
	}
	if _, err := unix.FcntlInt(uintptr(fd), unix.F_SETFD, unix.FD_CLOEXEC); err != nil {
		return -1, joinErrors(err, unix.Close(fd))
	}
	return fd, nil
}

// Init initializes the kqueue instance.
func (p *fastPoller) Init() error {
	p.lifecycleMu.Lock()
	defer p.lifecycleMu.Unlock()

	// Prevent double-initialization (would leak kqueue fd)
	if p.initialized.Load() {
		return errPollerAlreadyInitialized
	}
	if p.closed.Load() {
		return errPollerClosed
	}

	p.kq = -1
	create := unix.Kqueue
	if p.pollerCreate != nil {
		create = p.pollerCreate
	}
	kq, err := createKqueueDescriptor(create)
	if err != nil {
		return err
	}
	p.kq = int32(kq)

	p.fdMu.Lock()
	p.initFDTable()
	p.fdMu.Unlock()
	p.initialized.Store(true)

	return nil
}

// Close closes the kqueue instance.
func (p *fastPoller) Close() error {
	p.lifecycleMu.Lock()
	if p.closed.Swap(true) {
		err := p.kernelTagStore.close()
		p.lifecycleMu.Unlock()
		return err
	}
	initialized := p.initialized.Swap(false)
	kq := p.kq
	p.kq = -1
	if p.beforeResourceClose != nil {
		p.beforeResourceClose()
	}
	p.resourceMu.Lock()
	var err error
	if initialized && kq >= 0 {
		err = p.closeDescriptor(int(kq))
	}
	p.fdMu.Lock()
	ownedPollFDs := p.ownedPollFDsLocked()
	gates := p.clearFDTableLocked()
	p.kernelTags = nil
	p.fdMu.Unlock()
	p.readyMu.Lock()
	p.readyEvents = nil
	p.readyMu.Unlock()
	clear(p.eventBuf[:])
	err = joinErrors(err, p.kernelTagStore.close())
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
	kernelTag, err := p.newKeventTagLocked()
	if err != nil {
		p.fdMu.Unlock()
		return joinErrors(err, p.closeDescriptor(pollFD))
	}
	info := fdInfo{callback: cb, dispatch: dispatch, kernelTag: kernelTag, generation: generation, events: events, active: true, provisional: provisional, pollFD: pollFD, ownsPollFD: true}
	p.setFDInfoLocked(fd, info)
	if p.kernelTags == nil {
		p.kernelTags = make(map[keventTag]uint64)
	}
	p.kernelTags[kernelTag] = generation

	// Hold the lock across changes so registration and rollback remain one local
	// ownership transaction even though kqueue applies one filter at a time.
	actual, changeErr := applyKeventInterestsCall(int(p.kq), pollFD, kernelTag, 0, events, false, p.callKeventChange)
	if changeErr != nil {
		rollbackActual, rollbackErr := applyKeventInterestsCall(int(p.kq), pollFD, kernelTag, actual, 0, true, p.callKeventChange)
		if rollbackActual == 0 {
			delete(p.kernelTags, kernelTag)
			p.clearFDInfoLocked(fd)
		} else {
			info.events = rollbackActual
			p.setFDInfoLocked(fd, info)
		}
		retired := rollbackActual == 0
		p.fdMu.Unlock()
		if retired {
			rollbackErr = joinErrors(rollbackErr, p.closeDescriptor(pollFD))
			p.recycleKeventTag(kernelTag)
		}
		if rollbackActual != 0 || rollbackErr != nil {
			return &FDRegistrationRollbackError{cause: changeErr, rollback: rollbackErr, registered: rollbackActual != 0}
		}
		return changeErr
	}
	info.events = actual
	p.setFDInfoLocked(fd, info)
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

	remaining, err := applyKeventInterestsCall(int(p.kq), info.pollFD, info.kernelTag, info.events, 0, true, p.callKeventChange)
	if remaining == 0 {
		delete(p.kernelTags, info.kernelTag)
		p.clearFDInfoLocked(fd)
	} else {
		info.events = remaining
		p.setFDInfoLocked(fd, info)
	}
	p.fdMu.Unlock()
	if remaining != 0 {
		return &FDUnregisterError{cause: err}
	}
	if info.ownsPollFD {
		if closeErr := p.closeDescriptor(info.pollFD); closeErr != nil {
			p.recycleKeventTag(info.kernelTag)
			p.waitPendingDispatchStarts(info.dispatch)
			return &FDUnregisterError{cause: closeErr, released: true}
		}
	}
	p.recycleKeventTag(info.kernelTag)
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

	actual, err := modifyKeventInterestsCall(int(p.kq), info.pollFD, info.kernelTag, info.events, events, p.callKeventChange)
	info.events = actual
	p.setFDInfoLocked(fd, info)
	p.fdMu.Unlock()
	return err
}

// PollIO polls for I/O events.
func (p *fastPoller) PollIO(timeoutMs int) (int, error) {
	p.lifecycleMu.Lock()
	if p.closed.Load() || !p.initialized.Load() || p.kq < 0 {
		p.lifecycleMu.Unlock()
		return 0, errPollerClosed
	}
	kq := int(p.kq)
	p.lifecycleMu.Unlock()
	p.pollMu.Lock()
	defer p.pollMu.Unlock()
	p.clearReadyEvents()

	readyCount := 0
	wait := func(timeout time.Duration) (int, error) {
		p.resourceMu.RLock()
		defer p.resourceMu.RUnlock()
		if p.beforeNativePoll != nil {
			p.beforeNativePoll()
		}
		if p.closed.Load() {
			return 0, errPollerClosed
		}
		var ts *unix.Timespec
		if value, finite := keventWaitTimespec(timeout); finite {
			ts = &value
		}
		n, err := unix.Kevent(kq, nil, p.eventBuf[:], ts)
		n, err = normalizeKeventWaitResult(n, err)
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

func keventWaitTimespec(timeout time.Duration) (unix.Timespec, bool) {
	if timeout < 0 {
		return unix.Timespec{}, false
	}
	return unix.NsecToTimespec(timeout.Nanoseconds()), true
}

func normalizeKeventWaitResult(count int, err error) (int, error) {
	if errors.Is(err, unix.ETIMEDOUT) {
		return 0, nil
	}
	return count, err
}

// dispatchEvents collects callbacks for loop-owned dispatch.
// RACE SAFETY: Uses RLock to safely read fdInfo while allowing concurrent
// modifications to other fds. Callback is copied under lock and returned to the
// loop, which restores StateRunning before executing user callbacks.
func (p *fastPoller) dispatchEvents(n int) int {
	p.clearReadyEvents()
	for i := range n {
		p.fdMu.RLock()
		generation := p.kernelTags[keventEventTag(&p.eventBuf[i])]
		p.fdMu.RUnlock()
		p.appendReadyToken(generation, keventToEvents(&p.eventBuf[i]))
	}
	return len(p.readyEventsSnapshot())
}

func eventsToKeventsToken(fd int, events IOEvents, flags int, kernelTag keventTag) []unix.Kevent_t {
	var kevents []unix.Kevent_t

	if events&EventRead != 0 {
		kevents = append(kevents, newKevent(fd, unix.EVFILT_READ, flags, 0, 0, kernelTag))
	}

	if events&EventWrite != 0 {
		kevents = append(kevents, newKevent(fd, unix.EVFILT_WRITE, flags, 0, 0, kernelTag))
	}

	return kevents
}

func newKevent(fd, filter, flags int, fflags uint32, data int64, kernelTag keventTag) unix.Kevent_t {
	var event unix.Kevent_t
	unix.SetKevent(&event, fd, filter, flags)
	event.Fflags = fflags
	event.Data = data
	setKeventTag(&event, kernelTag)
	return event
}

func keventIdent(event *unix.Kevent_t) uint64 {
	return uint64(event.Ident)
}

func keventFilter(event *unix.Kevent_t) int {
	return int(event.Filter)
}

func keventFlags(event *unix.Kevent_t) uint32 {
	return uint32(event.Flags)
}

func keventFflags(event *unix.Kevent_t) uint32 {
	return event.Fflags
}

func keventData(event *unix.Kevent_t) int64 {
	return event.Data
}

type keventChangeCall func(kq int, changes []unix.Kevent_t) error

func (p *fastPoller) newKeventTagLocked() (keventTag, error) {
	return p.kernelTagStore.allocate()
}

// recycleKeventTag waits for native result conversion that may still carry the
// retired pointer, while lifecycleMu prevents a new native wait from starting.
// Only then can a later registration safely reuse the non-Go Udata address.
func (p *fastPoller) recycleKeventTag(kernelTag keventTag) {
	if !keventTagValid(kernelTag) {
		return
	}
	if p.beforeTagRecycle != nil {
		p.beforeTagRecycle()
	}
	p.resourceMu.Lock()
	p.fdMu.Lock()
	p.kernelTagStore.recycle(kernelTag)
	p.fdMu.Unlock()
	p.resourceMu.Unlock()
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

func (p *fastPoller) callKeventChange(kq int, changes []unix.Kevent_t) error {
	call := p.keventChange
	if call == nil {
		call = func(kq int, changes []unix.Kevent_t) error {
			_, err := unix.Kevent(kq, changes, nil, nil)
			return err
		}
	}
	return applyKeventChanges(kq, changes, call)
}

func applyKeventChanges(kq int, changes []unix.Kevent_t, call keventChangeCall) error {
	for index := range changes {
		change := changes[index : index+1]
		interrupted := false
		for {
			err := call(kq, change)
			if errors.Is(err, unix.EINTR) {
				interrupted = true
				continue
			}
			if interrupted && keventDelete(&change[0]) && errors.Is(err, unix.ENOENT) {
				err = nil
			}
			if err != nil {
				return err
			}
			break
		}
	}
	return nil
}

func keventDelete(event *unix.Kevent_t) bool {
	return keventFlags(event)&uint32(unix.EV_DELETE) != 0
}

func applyKeventInterestsCall(
	kq, fd int,
	kernelTag keventTag,
	current, target IOEvents,
	missingRemoved bool,
	change keventChangeCall,
) (IOEvents, error) {
	actual := current
	var resultErr error
	for _, interest := range []IOEvents{EventRead, EventWrite} {
		if target&interest != 0 && actual&interest == 0 {
			changes := eventsToKeventsToken(fd, interest, unix.EV_ADD|unix.EV_ENABLE, kernelTag)
			if err := change(kq, changes); err != nil {
				resultErr = joinErrors(resultErr, err)
				if !missingRemoved {
					return actual, resultErr
				}
				continue
			}
			actual |= interest
		}
	}
	for _, interest := range []IOEvents{EventRead, EventWrite} {
		if target&interest == 0 && actual&interest != 0 {
			changes := eventsToKeventsToken(fd, interest, unix.EV_DELETE, kernelTag)
			if err := change(kq, changes); err != nil {
				missing := errors.Is(err, unix.ENOENT)
				if missing {
					actual &^= interest
				}
				if !missingRemoved || !missing {
					resultErr = joinErrors(resultErr, err)
					if !missingRemoved {
						return actual, resultErr
					}
					continue
				}
				continue
			}
			actual &^= interest
		}
	}
	return actual, resultErr
}

func modifyKeventInterestsCall(
	kq, fd int,
	kernelTag keventTag,
	current, target IOEvents,
	change keventChangeCall,
) (IOEvents, error) {
	actual := current
	rollbackTarget := current
	var added IOEvents
	var changeErr error
	for _, interest := range []IOEvents{EventRead, EventWrite} {
		if target&interest == 0 || actual&interest != 0 {
			continue
		}
		if err := change(kq, eventsToKeventsToken(fd, interest, unix.EV_ADD|unix.EV_ENABLE, kernelTag)); err != nil {
			changeErr = err
			break
		}
		actual |= interest
		added |= interest
	}
	if changeErr == nil {
		for _, interest := range []IOEvents{EventRead, EventWrite} {
			if target&interest != 0 || actual&interest == 0 {
				continue
			}
			if err := change(kq, eventsToKeventsToken(fd, interest, unix.EV_DELETE, kernelTag)); err != nil {
				if errors.Is(err, unix.ENOENT) {
					actual &^= interest
					rollbackTarget = actual &^ added
				}
				changeErr = err
				break
			}
			actual &^= interest
		}
	}
	if changeErr == nil {
		return actual, nil
	}
	actual, rollbackErr := applyKeventInterestsCall(kq, fd, kernelTag, actual, rollbackTarget, true, change)
	return actual, joinErrors(changeErr, rollbackErr)
}

// keventToEvents converts kqueue event to IOEvents.
func keventToEvents(kev *unix.Kevent_t) IOEvents {
	var events IOEvents
	switch keventFilter(kev) {
	case unix.EVFILT_READ:
		events |= EventRead
	case unix.EVFILT_WRITE:
		events |= EventWrite
	}
	flags := keventFlags(kev)
	if flags&uint32(unix.EV_ERROR) != 0 {
		events |= EventError
	}
	if flags&uint32(unix.EV_EOF) != 0 {
		events |= EventHangup
		if keventFflags(kev) != 0 {
			events |= EventError
		}
	}
	return events
}
