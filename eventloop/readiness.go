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
func (x *Loop) ensurePollerForModeChange() error {
	x.fdMu.Lock()
	defer x.fdMu.Unlock()
	x.livenessMu.Lock()
	state := x.state.Load()
	if state == StateTerminating || state == StateTerminated {
		x.livenessMu.Unlock()
		return ErrLoopTerminated
	}
	x.livenessMu.Unlock()
	return x.ensurePollerLocked()
}

func (x *Loop) ensurePollerLocked() error {
	_, err := x.initPollerLocked(true)
	return err
}

func (x *Loop) initPollerLocked(publish bool) (bool, error) {
	if x.pollerReady.Load() {
		return false, nil
	}
	if x.pollerCleanupPending {
		if err := x.closePollerLocked(); err != nil {
			return false, err
		}
	}

	wakeFd, wakeWriteFd, err := createWakeFD()
	if err != nil {
		return false, err
	}
	if err := x.poller.Init(); err != nil {
		return false, joinErrors(err, closeWakeFDs(wakeFd, wakeWriteFd))
	}
	if wakeFd >= 0 {
		if x.testHooks != nil && x.testHooks.BeforeWakeFDRegister != nil {
			x.testHooks.BeforeWakeFDRegister(wakeFd, wakeWriteFd)
		}
		if err := x.poller.RegisterFD(wakeFd, EventRead, func(IOEvents) {
			x.drainWakeUpPipe()
		}); err != nil {
			cleanupErr := x.closePollerLocked()
			cleanupErr = joinErrors(cleanupErr, closeWakeFDs(wakeFd, wakeWriteFd))
			return false, joinErrors(err, cleanupErr)
		}
		if !x.poller.markFDInternal(wakeFd) {
			cleanupErr := x.closePollerLocked()
			cleanupErr = joinErrors(cleanupErr, closeWakeFDs(wakeFd, wakeWriteFd))
			return false, joinErrors(ErrFDNotRegistered, cleanupErr)
		}
	}
	x.wakePipe = wakeFd
	x.wakePipeWrite = wakeWriteFd
	if publish {
		x.pollerReady.Store(true)
	}
	return true, nil
}

func (x *Loop) resetUnpublishedPollerLocked() error {
	err := x.closePollerLocked()
	err = joinErrors(err, closeWakeFDs(x.wakePipe, x.wakePipeWrite))
	x.wakePipe = -1
	x.wakePipeWrite = -1
	return err
}

// closePollerLocked retains a closed poller in place while it still owns
// retryable cleanup. The caller holds fdMu, so a successful retry can replace
// the synchronization-bearing poller value without copying it while active.
func (x *Loop) closePollerLocked() error {
	err := x.poller.Close()
	x.pollerCleanupPending = err != nil
	if err == nil {
		x.poller = newFastPoller()
	}
	return err
}

func (x *Loop) retryPollerCleanup() {
	x.fdMu.Lock()
	if x.pollerCleanupPending {
		_ = x.closePollerLocked()
	}
	x.fdMu.Unlock()
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
func (x *Loop) RegisterFD(fd int, events IOEvents, callback func(events IOEvents)) error {
	x.requireReadinessLoop("RegisterFD")
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
		if x.testHooks != nil && x.testHooks.BeforeRegisterFDReturn != nil {
			x.testHooks.BeforeRegisterFDReturn(fd)
		}
		dispatch.publish()
	}()

	// RegisterFD adds liveness and therefore follows the same terminal/quiescing
	// gate as ScheduleTimer, RefTimer(ref=true), and Promisify.
	x.livenessMu.Lock()
	if err := x.rejectLivenessAddLocked(); err != nil {
		x.livenessMu.Unlock()
		return err
	}

	// Fast rejection before expensive syscall.
	if FastPathMode(x.fastPathMode.Load()) == FastPathForced {
		x.livenessMu.Unlock()
		return ErrFastPathIncompatible
	}
	x.livenessMu.Unlock()

	// Perform registration with the platform readiness backend. This can
	// be slow or block behind platform internals, so do not hold livenessMu while
	// it runs. fdMu still serializes poller ownership with UnregisterFD/ModifyFD
	// until the post-registration check below either commits or rolls back.
	x.fdMu.Lock()
	defer x.fdMu.Unlock()
	x.livenessMu.Lock()
	if err := x.rejectLivenessAddLocked(); err != nil {
		x.livenessMu.Unlock()
		return err
	}
	x.livenessMu.Unlock()
	createdPoller, err := x.initPollerLocked(false)
	if err != nil {
		return err
	}
	err = x.poller.stageFD(fd, events, callback, dispatch)
	registrationErr := err
	if err != nil {
		var partial *FDRegistrationRollbackError
		if !errors.As(err, &partial) || !partial.Registered() {
			var cleanupErr error
			if createdPoller {
				cleanupErr = x.resetUnpublishedPollerLocked()
			}
			return joinErrors(err, cleanupErr)
		}
	}
	if x.testHooks != nil && x.testHooks.BeforeRegisterFDRollbackCheck != nil {
		x.testHooks.BeforeRegisterFDRollbackCheck()
	}

	x.livenessMu.Lock()

	// Native registration occurs outside livenessMu. Re-check quiescing after
	// reacquiring the lock and roll back if the lifecycle changed meanwhile.
	if err := x.rejectLivenessAddLocked(); err != nil {
		x.livenessMu.Unlock()
		rollbackErr := x.rollbackFDRegistration(fd)
		if rollbackErr != nil && !fdRollbackReleased(rollbackErr) {
			// The poller still owns the FD entry if rollback failed. Count it so
			// Alive() and later cleanup stay consistent with actual poller state,
			// force a polling-capable mode, and return both the lifecycle rejection
			// and rollback failure.
			x.livenessMu.Lock()
			commitErr := x.commitFDRegistrationLocked(fd, createdPoller)
			if commitErr != nil {
				x.livenessMu.Unlock()
				var cleanupErr error
				if createdPoller {
					cleanupErr = x.resetUnpublishedPollerLocked()
				}
				return fdRegistrationRollbackResult(registrationErr, err, joinErrors(joinErrors(rollbackErr, commitErr), cleanupErr), false)
			}
			if FastPathMode(x.fastPathMode.Load()) == FastPathForced {
				x.fastPathMode.Store(int32(FastPathAuto))
				x.fastPathInvariantLogged.Store(false)
			}
			x.livenessMu.Unlock()
			x.doWakeup()
			return fdRegistrationRollbackResult(registrationErr, err, rollbackErr, true)
		}
		cleanupErr := fdRollbackCleanupError(rollbackErr)
		if createdPoller {
			cleanupErr = joinErrors(cleanupErr, x.resetUnpublishedPollerLocked())
		}
		return fdRegistrationRollbackResult(registrationErr, err, cleanupErr, false)
	}
	if x.testHooks != nil && x.testHooks.BeforeRegisterFDCommit != nil {
		x.testHooks.BeforeRegisterFDCommit()
	}
	if FastPathMode(x.fastPathMode.Load()) == FastPathForced {
		x.livenessMu.Unlock()
		rollbackErr := x.rollbackFDRegistration(fd)
		if rollbackErr != nil && !fdRollbackReleased(rollbackErr) {
			x.livenessMu.Lock()
			commitErr := x.commitFDRegistrationLocked(fd, createdPoller)
			if commitErr != nil {
				x.livenessMu.Unlock()
				var cleanupErr error
				if createdPoller {
					cleanupErr = x.resetUnpublishedPollerLocked()
				}
				return fdRegistrationRollbackResult(registrationErr, ErrFastPathIncompatible, joinErrors(joinErrors(rollbackErr, commitErr), cleanupErr), false)
			}
			x.fastPathMode.Store(int32(FastPathAuto))
			x.fastPathInvariantLogged.Store(false)
			x.livenessMu.Unlock()
			x.doWakeup()
			return fdRegistrationRollbackResult(registrationErr, ErrFastPathIncompatible, rollbackErr, true)
		}
		cleanupErr := fdRollbackCleanupError(rollbackErr)
		if createdPoller {
			cleanupErr = joinErrors(cleanupErr, x.resetUnpublishedPollerLocked())
		}
		return fdRegistrationRollbackResult(registrationErr, ErrFastPathIncompatible, cleanupErr, false)
	}

	// Commit liveness before the poller dispatch gate. A native event may be
	// collected while registration is provisional, but its callback cannot start
	// until all lifecycle and fast-path ownership checks have succeeded.
	if err := x.commitFDRegistrationLocked(fd, createdPoller); err != nil {
		x.livenessMu.Unlock()
		rollbackErr := x.rollbackFDRegistration(fd)
		if createdPoller {
			rollbackErr = joinErrors(rollbackErr, x.resetUnpublishedPollerLocked())
		}
		return fdRegistrationRollbackResult(registrationErr, err, rollbackErr, false)
	}
	x.livenessMu.Unlock()

	// Successfully registered FD in non-Forced mode. Wake the loop to transition
	// from fast-path to poll-path immediately if needed.
	x.doWakeup()

	return registrationErr
}

// commitFDRegistrationLocked publishes Loop liveness before enabling user
// callback dispatch. The caller holds livenessMu and fdMu.
func (x *Loop) commitFDRegistrationLocked(fd int, createdPoller bool) error {
	if createdPoller {
		x.pollerReady.Store(true)
	}
	x.userIOFDCount.Add(1)
	x.submissionEpoch.Add(1)
	if err := x.poller.commitFD(fd); err != nil {
		x.userIOFDCount.Add(-1)
		x.submissionEpoch.Add(1)
		if createdPoller {
			x.pollerReady.Store(false)
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

func (x *Loop) rollbackFDRegistration(fd int) error {
	if x.testHooks != nil && x.testHooks.RegisterFDRollback != nil {
		return x.testHooks.RegisterFDRollback(fd)
	}
	return x.unregisterPollerFD(fd)
}

// unregisterPollerFD latches a successful loop wake until native retirement
// finishes. The poll backend can then avoid a duplicate private-control write;
// a failed or unavailable loop wake leaves its private fallback enabled.
func (x *Loop) unregisterPollerFD(fd int) error {
	select {
	case x.fastWakeupCh <- struct{}{}:
	default:
	}
	if x.testHooks != nil && x.testHooks.OnSubmitWakeup != nil {
		x.testHooks.OnSubmitWakeup()
	}

	loopWakeLatched := false
	if x.state.Load() != StateTerminated {
		x.wakeMu.Lock()
		if fdPollingSupported && x.pollerReady.Load() && x.wakePipeWrite >= 0 {
			loopWakeLatched = x.submitWakeupPhysicalLocked() == nil
		}
		if !loopWakeLatched {
			x.wakeMu.Unlock()
		}
	}
	if loopWakeLatched {
		defer x.wakeMu.Unlock()
	}
	return x.poller.unregisterFD(fd, loopWakeLatched)
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
func (x *Loop) UnregisterFD(fd int) error {
	x.requireReadinessLoop("UnregisterFD")
	if fd < 0 {
		panic(fmt.Errorf("eventloop: UnregisterFD: %w", errFDNegative))
	}
	if !fdPollingSupported {
		return ErrReadinessUnsupported
	}

	if x.testHooks != nil && x.testHooks.BeforeFDUnregisterLock != nil {
		x.testHooks.BeforeFDUnregisterLock()
	}
	remaining := int32(-1)
	underflow := false
	err, released := func() (error, bool) {
		x.fdMu.Lock()
		defer x.fdMu.Unlock()
		x.livenessMu.Lock()
		defer x.livenessMu.Unlock()
		if err := x.rejectFDMutationLocked(true); err != nil {
			return err, false
		}
		if !x.pollerReady.Load() || !x.poller.userFDRegistered(fd) {
			return ErrFDNotRegistered, false
		}
		if x.testHooks != nil && x.testHooks.BeforeFDUnregister != nil {
			x.testHooks.BeforeFDUnregister()
		}
		// A successful loop wake remains latched through native retirement. The
		// poll backend uses that same token instead of posting a duplicate control
		// wake.
		err := x.unregisterPollerFD(fd)
		if err != nil && !fdUnregisterReleased(err) {
			return err, false
		}
		remaining = x.userIOFDCount.Add(-1)
		if remaining < 0 {
			x.userIOFDCount.Store(0)
			remaining = 0
			underflow = true
		}
		x.submissionEpoch.Add(1) // Alive() consistency: FD unregistration changes liveness
		return err, true
	}()
	if !released {
		return err
	}
	if underflow {
		x.logError("eventloop: user I/O FD count went negative during UnregisterFD; clamped to zero", nil)
	}
	// When the last I/O FD is removed, the loop may still be blocked in PollIO()
	// (not listening on fastWakeupCh). Wake it so it transitions to pollFastMode
	// immediately, rather than waiting for a later submission or poll timeout.
	if remaining == 0 {
		x.doWakeup()
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
func (x *Loop) ModifyFD(fd int, events IOEvents) error {
	x.requireReadinessLoop("ModifyFD")
	if fd < 0 {
		panic(fmt.Errorf("eventloop: ModifyFD: %w", errFDNegative))
	}
	if err := validateFDModification(events); err != nil {
		panic(fmt.Errorf("eventloop: ModifyFD: %w", err))
	}
	if !fdPollingSupported {
		return ErrReadinessUnsupported
	}

	x.fdMu.Lock()
	defer x.fdMu.Unlock()
	x.livenessMu.Lock()
	defer x.livenessMu.Unlock()
	if err := x.rejectFDMutationLocked(false); err != nil {
		return err
	}
	if !x.pollerReady.Load() {
		return ErrFDNotRegistered
	}
	if !x.poller.userFDRegistered(fd) {
		return ErrFDNotRegistered
	}
	if x.testHooks != nil && x.testHooks.BeforeFDModify != nil {
		x.testHooks.BeforeFDModify()
	}
	return x.poller.ModifyFD(fd, events)
}

func (x *Loop) requireReadinessLoop(method string) {
	if x == nil || x.state == nil {
		panic(fmt.Errorf("eventloop: %s: %w", method, errLoopUninitialized))
	}
}
