package eventloop

import (
	"errors"
	"fmt"
)

// IOEvents is a readiness interest or callback-result mask.
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
	// ErrFDAlreadyRegistered is returned when a file descriptor already has a live registration.
	ErrFDAlreadyRegistered = errors.New("eventloop: fd already registered")
	// ErrFDNotRegistered is returned when a file descriptor has no live registration.
	ErrFDNotRegistered = errors.New("eventloop: fd not registered")
	// ErrFDRegistrationExhausted is returned when readiness registration identity cannot advance without reuse.
	ErrFDRegistrationExhausted = errors.New("eventloop: file descriptor registration identity exhausted")
	// ErrReadinessUnsupported is returned where descriptor readiness polling is unavailable.
	ErrReadinessUnsupported = errors.New("eventloop: descriptor readiness unsupported")

	errPollerClosed             = errors.New("eventloop: poller closed")
	errPollerAlreadyInitialized = errors.New("eventloop: poller already initialized")
	errLoopUninitialized        = errors.New("eventloop: Loop must be constructed with New")
	errFDNegative               = errors.New("eventloop: file descriptor is negative")
	errFDNilCallback            = errors.New("eventloop: file descriptor callback is nil")
	errFDInvalidEvents          = errors.New("eventloop: invalid file descriptor event interests")
)

// ioCallback is the poller's internal callback type.
type ioCallback func(IOEvents)

// FDRegistrationRollbackError reports a partial platform registration or a
// failed rollback after [Loop.RegisterFD] later discovered a lifecycle or mode
// rejection. [FDRegistrationRollbackError.Registered] describes final ownership
// after every attempted cleanup. When it is true, callers must treat the FD as
// still owned by the loop until a successful [Loop.UnregisterFD] or loop
// termination cleanup. The error unwraps both underlying failures so [errors.Is]
// and [errors.As] continue to match them.
type FDRegistrationRollbackError struct {
	cause      error
	rollback   error
	registered bool
}

func (e *FDRegistrationRollbackError) Error() string {
	if e == nil {
		return "eventloop: fd registration rollback failed"
	}
	return fmt.Sprintf("eventloop: fd registration rollback failed; cause=%v rollback=%v registered=%v", e.cause, e.rollback, e.registered)
}

// Registered reports whether the loop retained registration ownership after
// every rollback attempt.
func (e *FDRegistrationRollbackError) Registered() bool {
	return e != nil && e.registered
}

func (e *FDRegistrationRollbackError) Unwrap() []error {
	if e == nil {
		return nil
	}
	errs := make([]error, 0, 2)
	if err := nonNilError(e.cause); err != nil {
		errs = append(errs, err)
	}
	if err := nonNilError(e.rollback); err != nil {
		errs = append(errs, err)
	}
	return errs
}

// timerPool for amortized timer allocations.
func (l *Loop) ensurePollerForModeChange() error {
	l.fdMu.Lock()
	defer l.fdMu.Unlock()
	l.livenessMu.Lock()
	state := l.state.Load()
	if state == StateTerminating || state == StateTerminated {
		l.livenessMu.Unlock()
		return ErrLoopTerminated
	}
	l.livenessMu.Unlock()
	return l.ensurePollerLocked()
}

func (l *Loop) ensurePollerLocked() error {
	_, err := l.initPollerLocked(true)
	return err
}

func (l *Loop) initPollerLocked(publish bool) (bool, error) {
	if l.pollerReady.Load() {
		return false, nil
	}
	if l.pollerCleanupPending {
		if err := l.closePollerLocked(); err != nil {
			return false, err
		}
	}

	wakeFd, wakeWriteFd, err := createWakeFD()
	if err != nil {
		return false, err
	}
	if err := l.poller.Init(); err != nil {
		return false, joinErrors(err, closeWakeFDs(wakeFd, wakeWriteFd))
	}
	if wakeFd >= 0 {
		if l.testHooks != nil && l.testHooks.BeforeWakeFDRegister != nil {
			l.testHooks.BeforeWakeFDRegister(wakeFd, wakeWriteFd)
		}
		if err := l.poller.RegisterFD(wakeFd, EventRead, func(IOEvents) {
			l.drainWakeUpPipe()
		}); err != nil {
			cleanupErr := l.closePollerLocked()
			cleanupErr = joinErrors(cleanupErr, closeWakeFDs(wakeFd, wakeWriteFd))
			return false, joinErrors(err, cleanupErr)
		}
		if !l.poller.markFDInternal(wakeFd) {
			cleanupErr := l.closePollerLocked()
			cleanupErr = joinErrors(cleanupErr, closeWakeFDs(wakeFd, wakeWriteFd))
			return false, joinErrors(ErrFDNotRegistered, cleanupErr)
		}
	}
	l.wakePipe = wakeFd
	l.wakePipeWrite = wakeWriteFd
	if publish {
		l.pollerReady.Store(true)
	}
	return true, nil
}

func (l *Loop) resetUnpublishedPollerLocked() error {
	err := l.closePollerLocked()
	err = joinErrors(err, closeWakeFDs(l.wakePipe, l.wakePipeWrite))
	l.wakePipe = -1
	l.wakePipeWrite = -1
	return err
}

// closePollerLocked retains a closed poller in place while it still owns
// retryable cleanup. The caller holds fdMu, so a successful retry can replace
// the synchronization-bearing poller value without copying it while active.
func (l *Loop) closePollerLocked() error {
	err := l.poller.Close()
	l.pollerCleanupPending = err != nil
	if err == nil {
		l.poller = newFastPoller()
	}
	return err
}

func (l *Loop) retryPollerCleanup() {
	l.fdMu.Lock()
	if l.pollerCleanupPending {
		_ = l.closePollerLocked()
	}
	l.fdMu.Unlock()
}

func closeWakeFDs(wakeFd, wakeWriteFd int) error {
	var err error
	if wakeFd >= 0 {
		err = closeFD(wakeFd)
	}
	if wakeWriteFd >= 0 && wakeWriteFd != wakeFd {
		err = joinErrors(err, closeFD(wakeWriteFd))
	}
	return err
}

// RegisterFD registers a file descriptor for I/O event monitoring.
//
// Platform support: this public readiness API uses epoll on Linux and Android;
// kqueue on Darwin, iOS, DragonFly, FreeBSD, NetBSD, and OpenBSD; and poll on
// AIX/ppc64, Solaris/amd64, and illumos/amd64. Windows, Plan 9, js/wasm, and
// wasip1/wasm return [ErrReadinessUnsupported] without initializing readiness
// resources.
//
// Invariant: RegisterFD is incompatible with FastPathForced mode.
//   - Returns ErrFastPathIncompatible if mode is FastPathForced.
//   - FastPath mode changes and FD registration are linearized by livenessMu
//     before and after poller registration; if FastPathForced is selected while
//     the OS registration is in progress, RegisterFD rolls the poller entry back.
//
// Registering a user FD requires the native readiness poll path.
// The callback must be non-nil, and events must contain [EventRead],
// [EventWrite], or both. Error and hangup conditions are callback results, not
// registration interests.
// RegisterFD panics if the Loop was not constructed by [New], fd is negative,
// callback is nil, or events is not a non-empty combination of [EventRead] and
// [EventWrite]. These static contracts are checked before platform support so
// misuse fails consistently on every target.
// Descriptor zero is valid on every readiness target. Each native poll batch invokes
// at most one callback per registration; simultaneous read and write readiness
// is combined in the callback mask.
// The Unix poller duplicates each accepted descriptor for native ownership.
// Closing the caller's descriptor therefore does not redirect later poller
// control to a reused numeric descriptor; UnregisterFD or loop termination
// releases the owned duplicate.
//
// Kqueue registration can partially succeed when one filter is installed and a
// later filter plus rollback fail. A later lifecycle or mode rejection can
// also fail to roll a completed platform registration back. Both cases return an
// [FDRegistrationRollbackError]; its Registered method describes ownership after
// all cleanup attempted before return. When true, callers must not assume the FD
// is unregistered merely because RegisterFD returned an error; use [errors.As]
// and clean up through [Loop.UnregisterFD] or loop termination.
// [ErrFDRegistrationExhausted] reports the practically unreachable point where
// allocating another stable registration identity would require reuse.
// Registration identities never wrap.
//
// Successful and retained-error registrations publish their final callback
// eligibility at the return boundary. Native readiness observed earlier is
// discarded, not allowed to block the loop owner; the active readiness interest
// reports it again after publication. A concurrent UnregisterFD can suppress an
// unpublished callback before it claims entry.
//
// Thread Safety: Safe to call concurrently with SetFastPathMode.
// Uses livenessMu to linearize FD registration with fast-path mode changes
// without holding the liveness lock across the OS poller registration call.
func (l *Loop) RegisterFD(fd int, events IOEvents, callback func(events IOEvents)) error {
	l.requireReadinessLoop("RegisterFD")
	if fd < 0 {
		panic(fmt.Errorf("eventloop: RegisterFD: %w", errFDNegative))
	}
	if err := validateFDRegistration(events, callback); err != nil {
		panic(fmt.Errorf("eventloop: RegisterFD: %w", err))
	}
	if !fdPollingSupported {
		return ErrReadinessUnsupported
	}
	dispatch := newFDDispatchGate(false)
	defer func() {
		if l.testHooks != nil && l.testHooks.BeforeRegisterFDReturn != nil {
			l.testHooks.BeforeRegisterFDReturn(fd)
		}
		dispatch.publish()
	}()

	// RegisterFD adds liveness and therefore follows the same terminal/quiescing
	// gate as ScheduleTimer, RefTimer(ref=true), and Promisify.
	l.livenessMu.Lock()
	if err := l.rejectLivenessAddLocked(); err != nil {
		l.livenessMu.Unlock()
		return err
	}

	// Fast rejection before expensive syscall.
	if FastPathMode(l.fastPathMode.Load()) == FastPathForced {
		l.livenessMu.Unlock()
		return ErrFastPathIncompatible
	}
	l.livenessMu.Unlock()

	// Perform registration with the platform readiness backend. This can
	// be slow or block behind platform internals, so do not hold livenessMu while
	// it runs. fdMu still serializes poller ownership with UnregisterFD/ModifyFD
	// until the post-registration check below either commits or rolls back.
	l.fdMu.Lock()
	defer l.fdMu.Unlock()
	l.livenessMu.Lock()
	if err := l.rejectLivenessAddLocked(); err != nil {
		l.livenessMu.Unlock()
		return err
	}
	l.livenessMu.Unlock()
	createdPoller, err := l.initPollerLocked(false)
	if err != nil {
		return err
	}
	err = l.poller.stageFD(fd, events, callback, dispatch)
	registrationErr := err
	if err != nil {
		var partial *FDRegistrationRollbackError
		if !errors.As(err, &partial) || !partial.Registered() {
			var cleanupErr error
			if createdPoller {
				cleanupErr = l.resetUnpublishedPollerLocked()
			}
			return joinErrors(err, cleanupErr)
		}
	}
	if l.testHooks != nil && l.testHooks.BeforeRegisterFDRollbackCheck != nil {
		l.testHooks.BeforeRegisterFDRollbackCheck()
	}

	l.livenessMu.Lock()

	// Native registration occurs outside livenessMu. Re-check quiescing after
	// reacquiring the lock and roll back if the lifecycle changed meanwhile.
	if err := l.rejectLivenessAddLocked(); err != nil {
		l.livenessMu.Unlock()
		rollbackErr := l.rollbackFDRegistration(fd)
		if rollbackErr != nil && !fdRollbackReleased(rollbackErr) {
			// The poller still owns the FD entry if rollback failed. Count it so
			// Alive() and later cleanup stay consistent with actual poller state,
			// force a polling-capable mode, and return both the lifecycle rejection
			// and rollback failure.
			l.livenessMu.Lock()
			commitErr := l.commitFDRegistrationLocked(fd, createdPoller)
			if commitErr != nil {
				l.livenessMu.Unlock()
				var cleanupErr error
				if createdPoller {
					cleanupErr = l.resetUnpublishedPollerLocked()
				}
				return fdRegistrationRollbackResult(registrationErr, err, joinErrors(joinErrors(rollbackErr, commitErr), cleanupErr), false)
			}
			if FastPathMode(l.fastPathMode.Load()) == FastPathForced {
				l.fastPathMode.Store(int32(FastPathAuto))
				l.fastPathInvariantLogged.Store(false)
			}
			l.livenessMu.Unlock()
			l.doWakeup()
			return fdRegistrationRollbackResult(registrationErr, err, rollbackErr, true)
		}
		cleanupErr := fdRollbackCleanupError(rollbackErr)
		if createdPoller {
			cleanupErr = joinErrors(cleanupErr, l.resetUnpublishedPollerLocked())
		}
		return fdRegistrationRollbackResult(registrationErr, err, cleanupErr, false)
	}
	if l.testHooks != nil && l.testHooks.BeforeRegisterFDCommit != nil {
		l.testHooks.BeforeRegisterFDCommit()
	}
	if FastPathMode(l.fastPathMode.Load()) == FastPathForced {
		l.livenessMu.Unlock()
		rollbackErr := l.rollbackFDRegistration(fd)
		if rollbackErr != nil && !fdRollbackReleased(rollbackErr) {
			l.livenessMu.Lock()
			commitErr := l.commitFDRegistrationLocked(fd, createdPoller)
			if commitErr != nil {
				l.livenessMu.Unlock()
				var cleanupErr error
				if createdPoller {
					cleanupErr = l.resetUnpublishedPollerLocked()
				}
				return fdRegistrationRollbackResult(registrationErr, ErrFastPathIncompatible, joinErrors(joinErrors(rollbackErr, commitErr), cleanupErr), false)
			}
			l.fastPathMode.Store(int32(FastPathAuto))
			l.fastPathInvariantLogged.Store(false)
			l.livenessMu.Unlock()
			l.doWakeup()
			return fdRegistrationRollbackResult(registrationErr, ErrFastPathIncompatible, rollbackErr, true)
		}
		cleanupErr := fdRollbackCleanupError(rollbackErr)
		if createdPoller {
			cleanupErr = joinErrors(cleanupErr, l.resetUnpublishedPollerLocked())
		}
		return fdRegistrationRollbackResult(registrationErr, ErrFastPathIncompatible, cleanupErr, false)
	}

	// Commit liveness before the poller dispatch gate. A native event may be
	// collected while registration is provisional, but its callback cannot start
	// until all lifecycle and fast-path ownership checks have succeeded.
	if err := l.commitFDRegistrationLocked(fd, createdPoller); err != nil {
		l.livenessMu.Unlock()
		rollbackErr := l.rollbackFDRegistration(fd)
		if createdPoller {
			rollbackErr = joinErrors(rollbackErr, l.resetUnpublishedPollerLocked())
		}
		return fdRegistrationRollbackResult(registrationErr, err, rollbackErr, false)
	}
	l.livenessMu.Unlock()

	// Successfully registered FD in non-Forced mode. Wake the loop to transition
	// from fast-path to poll-path immediately if needed.
	l.doWakeup()

	return registrationErr
}

// commitFDRegistrationLocked publishes Loop liveness before enabling user
// callback dispatch. The caller holds livenessMu and fdMu.
func (l *Loop) commitFDRegistrationLocked(fd int, createdPoller bool) error {
	if createdPoller {
		l.pollerReady.Store(true)
	}
	l.userIOFDCount.Add(1)
	l.submissionEpoch.Add(1)
	if err := l.poller.commitFD(fd); err != nil {
		l.userIOFDCount.Add(-1)
		l.submissionEpoch.Add(1)
		if createdPoller {
			l.pollerReady.Store(false)
		}
		return err
	}
	return nil
}

func fdRollbackReleased(err error) bool {
	return errors.Is(err, ErrFDNotRegistered) || errors.Is(err, errPollerClosed) || fdUnregisterReleased(err)
}

func fdRollbackCleanupError(err error) error {
	if fdUnregisterReleased(err) {
		return err
	}
	return nil
}

func fdRegistrationRollbackResult(registrationErr, cause, rollbackErr error, registered bool) error {
	var partial *FDRegistrationRollbackError
	if errors.As(registrationErr, &partial) && partial != nil {
		registrationErr = partial.cause
		rollbackErr = joinErrors(partial.rollback, rollbackErr)
	}
	if registrationErr == nil && rollbackErr == nil {
		return cause
	}
	return &FDRegistrationRollbackError{
		cause:      joinErrors(cause, registrationErr),
		rollback:   rollbackErr,
		registered: registered,
	}
}

func (l *Loop) rollbackFDRegistration(fd int) error {
	if l.testHooks != nil && l.testHooks.RegisterFDRollback != nil {
		return l.testHooks.RegisterFDRollback(fd)
	}
	return l.unregisterPollerFD(fd)
}

// unregisterPollerFD latches a successful loop wake until native retirement
// finishes. The poll backend can then avoid a duplicate private-control write;
// a failed or unavailable loop wake leaves its private fallback enabled.
func (l *Loop) unregisterPollerFD(fd int) error {
	select {
	case l.fastWakeupCh <- struct{}{}:
	default:
	}
	if l.testHooks != nil && l.testHooks.OnSubmitWakeup != nil {
		l.testHooks.OnSubmitWakeup()
	}

	loopWakeLatched := false
	if l.state.Load() != StateTerminated {
		l.wakeMu.Lock()
		if fdPollingSupported && l.pollerReady.Load() && l.wakePipeWrite >= 0 {
			loopWakeLatched = l.submitWakeupPhysicalLocked() == nil
		}
		if !loopWakeLatched {
			l.wakeMu.Unlock()
		}
	}
	if loopWakeLatched {
		defer l.wakeMu.Unlock()
	}
	return l.poller.unregisterFD(fd, loopWakeLatched)
}

// UnregisterFD removes a file descriptor from monitoring.
//
// Windows, Plan 9, js/wasm, and wasip1/wasm return
// [ErrReadinessUnsupported].
//
// After the last user FD is unregistered, Auto mode can resume channel waiting.
// FastPathDisabled continues native polling on every readiness target.
//
// Prefer unregistering an FD before closing it. On readiness targets, closing
// it first is also supported: UnregisterFD deletes and closes the poller's owned
// duplicate, then retires local ownership and liveness. UnregisterFD prevents a
// not-yet-started callback for that registration from starting and waits for a
// callback whose start was already claimed; it does not wait for that callback
// to finish. A returned [FDUnregisterError] reports whether ownership was
// released despite a native mutation or descriptor-cleanup failure.
// UnregisterFD panics if the Loop was not constructed by [New] or fd is
// negative. Those static contracts are checked before platform support.
func (l *Loop) UnregisterFD(fd int) error {
	l.requireReadinessLoop("UnregisterFD")
	if fd < 0 {
		panic(fmt.Errorf("eventloop: UnregisterFD: %w", errFDNegative))
	}
	if !fdPollingSupported {
		return ErrReadinessUnsupported
	}

	if l.testHooks != nil && l.testHooks.BeforeFDUnregisterLock != nil {
		l.testHooks.BeforeFDUnregisterLock()
	}
	remaining := int32(-1)
	underflow := false
	err, released := func() (error, bool) {
		l.fdMu.Lock()
		defer l.fdMu.Unlock()
		l.livenessMu.Lock()
		defer l.livenessMu.Unlock()
		if err := l.rejectFDMutationLocked(true); err != nil {
			return err, false
		}
		if !l.pollerReady.Load() || !l.poller.userFDRegistered(fd) {
			return ErrFDNotRegistered, false
		}
		if l.testHooks != nil && l.testHooks.BeforeFDUnregister != nil {
			l.testHooks.BeforeFDUnregister()
		}
		// A successful loop wake remains latched through native retirement. The
		// poll backend uses that same token instead of posting a duplicate control
		// wake.
		err := l.unregisterPollerFD(fd)
		if err != nil && !fdUnregisterReleased(err) {
			return err, false
		}
		remaining = l.userIOFDCount.Add(-1)
		if remaining < 0 {
			l.userIOFDCount.Store(0)
			remaining = 0
			underflow = true
		}
		l.submissionEpoch.Add(1) // Alive() consistency: FD unregistration changes liveness
		return err, true
	}()
	if !released {
		return err
	}
	if underflow {
		l.logError("eventloop: user I/O FD count went negative during UnregisterFD; clamped to zero", nil)
	}
	// When the last I/O FD is removed, the loop may still be blocked in PollIO()
	// (not listening on fastWakeupCh). Wake it so it transitions to pollFastMode
	// immediately, rather than waiting for a later submission or poll timeout.
	if remaining == 0 {
		l.doWakeup()
	}
	return err
}

// ModifyFD updates the events being monitored for a file descriptor.
//
// Windows, Plan 9, js/wasm, and wasip1/wasm return
// [ErrReadinessUnsupported].
// A zero events mask disables read/write interests without unregistering the FD.
// Epoll and poll can still report mandatory error or hangup conditions for a
// zero-interest entry; kqueue has no active filter to report while its mask is
// zero. A failed multi-filter kqueue update is rolled back to the prior mask
// when possible; if rollback also fails, the poller retains the exact known
// native mask so a later ModifyFD or UnregisterFD can clean it up.
// ModifyFD panics if the Loop was not constructed by [New], fd is negative, or
// events contains any bit other than [EventRead] or [EventWrite]. A zero mask
// remains valid. Static contracts are checked before platform support.
func (l *Loop) ModifyFD(fd int, events IOEvents) error {
	l.requireReadinessLoop("ModifyFD")
	if fd < 0 {
		panic(fmt.Errorf("eventloop: ModifyFD: %w", errFDNegative))
	}
	if err := validateFDModification(events); err != nil {
		panic(fmt.Errorf("eventloop: ModifyFD: %w", err))
	}
	if !fdPollingSupported {
		return ErrReadinessUnsupported
	}

	l.fdMu.Lock()
	defer l.fdMu.Unlock()
	l.livenessMu.Lock()
	defer l.livenessMu.Unlock()
	if err := l.rejectFDMutationLocked(false); err != nil {
		return err
	}
	if !l.pollerReady.Load() {
		return ErrFDNotRegistered
	}
	if !l.poller.userFDRegistered(fd) {
		return ErrFDNotRegistered
	}
	if l.testHooks != nil && l.testHooks.BeforeFDModify != nil {
		l.testHooks.BeforeFDModify()
	}
	return l.poller.ModifyFD(fd, events)
}

func (l *Loop) requireReadinessLoop(method string) {
	if l == nil || l.state == nil {
		panic(fmt.Errorf("eventloop: %s: %w", method, errLoopUninitialized))
	}
}
