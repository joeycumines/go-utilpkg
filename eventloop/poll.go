package eventloop

import (
	"time"
)

const maxFinitePollTimeoutMs = 10_000

// maxPhysicalPollWaitMs is the failed-wake recovery SLO for native FD polling.
// Normal wakes still return PollIO immediately. Capping every longer wait is
// necessary before a failure occurs because a failed descriptor write cannot
// retroactively shorten an already-running poll. For a healthy idle I/O loop,
// the cap costs at most one empty native poll turn per second. Ordinary
// task-only loops remain on the indefinite fast-channel path; FastPathDisabled
// deliberately uses native polling even without user descriptors.
const maxPhysicalPollWaitMs = 1_000

func boundedPhysicalPollTimeout(timeout int) int {
	if timeout < 0 || timeout > maxPhysicalPollWaitMs {
		return maxPhysicalPollWaitMs
	}
	return timeout
}

// poll blocks on the channel wake path for ordinary task-only workloads. It
// uses native readiness polling when user descriptors are registered or when
// FastPathDisabled deliberately selects the physical wake path.
func (x *Loop) poll() {
	currentState := x.state.Load()
	if currentState != StateRunning {
		return
	}
	x.drainCommandIngress()

	// Read and reset forceNonBlockingPoll
	forced := x.forceNonBlockingPoll
	x.forceNonBlockingPoll = false

	if x.testHooks != nil && x.testHooks.PrePollSleep != nil {
		x.testHooks.PrePollSleep()
	}

	// Optimistic state transition
	if !x.state.TryTransition(StateRunning, StateSleeping) {
		return
	}

	// Quick length check (need to hold mutexes for accurate count)
	x.externalMu.Lock()
	extLen := x.commands.Len() + int(x.ownerExternalCount.Load())
	hasPhaseJobs := len(x.checkJobs) > 0 || len(x.closeJobs) > 0 || x.ownerCheckCount.Load() > 0 || x.ownerCloseCount.Load() > 0
	x.externalMu.Unlock()
	intLen := int(x.ownerInternalCount.Load())

	if extLen > 0 || intLen > 0 || hasPhaseJobs || x.microtaskYield.Load() || !x.microtaskQueuesEmpty() {
		x.state.TryTransition(StateSleeping, StateRunning)
		return
	}

	// Auto-exit check: don't block in poll if loop should exit.
	if x.autoExit && !x.Alive() {
		x.state.TryTransition(StateSleeping, StateRunning)
		return
	}

	state := x.state.Load()
	if state == StateTerminating || state == StateTerminated {
		return
	}

	// Calculate timeout
	timeout := x.calculateTimeout()
	if forced {
		timeout = 0
	}

	// Check for termination AGAIN after calculating timeout
	// but BEFORE blocking in poll. This prevents racing with Shutdown.
	state = x.state.Load()
	if state == StateTerminating || state == StateTerminated {
		x.state.TryTransition(StateSleeping, StateRunning)
		return
	}

	// Task-only Auto/Forced loops and targets without native readiness block on
	// the channel wake path. FastPathDisabled deliberately exercises the native
	// poll path on readiness-capable targets even when no user descriptor is registered.
	if x.userIOFDCount.Load() == 0 && (FastPathMode(x.fastPathMode.Load()) != FastPathDisabled || !fdPollingSupported) {
		x.pollFastMode(timeout)
		return
	}

	// Native readiness mode: user FDs are registered or FastPathDisabled was
	// selected explicitly.
	if err := x.ensurePoller(); err != nil {
		x.handlePollError(err)
		return
	}
	// A wake committed while lazy initialization still had pollerReady=false is
	// represented only by the fast channel. Consume that handoff before entering
	// the newly published native poller. Wakes published after this check observe
	// pollerReady and also submit the physical signal.
	if x.consumeFastWakeup() {
		return
	}
	if x.testHooks != nil && x.testHooks.BeforePollIO != nil {
		x.testHooks.BeforePollIO()
	}
	state = x.state.Load()
	if state == StateTerminating || state == StateTerminated {
		x.state.TryTransition(StateSleeping, StateRunning)
		return
	}
	timeout = boundedPhysicalPollTimeout(timeout)
	pollIO := x.poller.PollIO
	if x.testHooks != nil && x.testHooks.PollIO != nil {
		pollIO = x.testHooks.PollIO
	}
	_, err := pollIO(timeout)
	if x.testHooks != nil && x.testHooks.PollError != nil {
		err = x.testHooks.PollError()
	}
	if err != nil {
		x.poller.clearReadyEvents()
		x.handlePollError(err)
		return
	}
	if x.state.Load() == StateTerminated {
		x.poller.clearReadyEvents()
		return
	}

	if x.testHooks != nil && x.testHooks.PrePollAwake != nil {
		x.testHooks.PrePollAwake()
	}
	if !x.state.TryTransition(StateSleeping, StateRunning) {
		x.poller.clearReadyEvents()
		return
	}
	x.dispatchPollEvents(x.poller.readyEventsSnapshot())
	x.poller.clearReadyEvents()

}

func (x *Loop) dispatchPollEvents(events []pollEvent) {
	for _, event := range events {
		if x.testHooks != nil && x.testHooks.BeforeFDPublicationCheck != nil {
			x.testHooks.BeforeFDPublicationCheck(event.fd)
		}
		callback, _, dispatch, ok := x.poller.beginReadyEventDispatch(event)
		if !ok {
			continue
		}
		if x.testHooks != nil && x.testHooks.AfterReadyEventDispatchClaim != nil {
			x.testHooks.AfterReadyEventDispatchClaim(event.fd)
		}
		events, ok := x.poller.startReadyEventDispatch(event, dispatch)
		if !ok {
			continue
		}
		if event.internal {
			x.executePollInternal(func() { callback(events) })
			continue
		}
		x.safeExecute(func() {
			callback(events)
		})
		x.drainMicrotasks()
		if x.hardAbortRequested() {
			return
		}
	}
}

// executePollInternal isolates native wake plumbing from user callback
// admission, metrics, TPS, and microtask checkpoints while preserving panic
// containment for test hooks and future internal callbacks.
func (x *Loop) executePollInternal(callback func()) {
	x.logCallbackOutcome("internal poll callback", x.executeOwnedCallback(callback))
}

// pollFastMode is the channel-based fast path for task-only workloads.
// It blocks on fastWakeupCh instead of a native readiness backend.
func (x *Loop) pollFastMode(timeoutMs int) {
	// Drain any pending channel signal first (non-blocking)
	if x.consumeFastWakeup() {
		return
	}

	// Check for termination BEFORE blocking in pollFastMode
	// This prevents a race where shutdown happens after the channel drain
	// but before we block, causing us to sleep indefinitely.
	state := x.state.Load()
	if state == StateTerminating || state == StateTerminated {
		x.state.TryTransition(StateSleeping, StateRunning)
		return
	}

	// Auto-exit check: don't block in pollFastMode if loop should exit.
	if x.autoExit && !x.Alive() {
		x.state.TryTransition(StateSleeping, StateRunning)
		return
	}

	// Non-blocking case
	if timeoutMs == 0 {
		if x.testHooks != nil && x.testHooks.PrePollAwake != nil {
			x.testHooks.PrePollAwake()
		}
		x.state.TryTransition(StateSleeping, StateRunning)
		return
	}

	// No finite deadline: block indefinitely until external wakeup or lifecycle
	// transition. Finite deadlines, including values >= 1s, must use a real
	// timer so scheduled timers cannot sleep forever in fast mode.
	if timeoutMs < 0 {
		// Check termination before indefinite block
		if x.state.Load() == StateTerminating || x.state.Load() == StateTerminated {
			x.state.TryTransition(StateSleeping, StateRunning)
			return
		}
		if x.testHooks != nil && x.testHooks.BeforeFastPollWait != nil {
			x.testHooks.BeforeFastPollWait(timeoutMs)
		}
		// Block indefinitely on channel - no timer allocation
		<-x.fastWakeupCh
		if x.state.Load() == StateTerminated {
			return
		}
		if x.testHooks != nil && x.testHooks.PrePollAwake != nil {
			x.testHooks.PrePollAwake()
		}
		x.state.TryTransition(StateSleeping, StateRunning)
		return
	}

	// Finite timeout - use the loop-owned reusable sleep timer.
	// Check termination before timer-protected block
	state = x.state.Load()
	if state == StateTerminating || state == StateTerminated {
		x.state.TryTransition(StateSleeping, StateRunning)
		return
	}
	timerC := x.resetFastSleepTimer(time.Duration(timeoutMs) * time.Millisecond)
	if x.testHooks != nil && x.testHooks.BeforeFastPollWait != nil {
		x.testHooks.BeforeFastPollWait(timeoutMs)
	}
	select {
	case <-x.fastWakeupCh:
		x.stopFastSleepTimer()
	case <-timerC:
	}
	if x.state.Load() == StateTerminated {
		return
	}

	if x.testHooks != nil && x.testHooks.PrePollAwake != nil {
		x.testHooks.PrePollAwake()
	}

	x.state.TryTransition(StateSleeping, StateRunning)
}

func (x *Loop) consumeFastWakeup() bool {
	select {
	case <-x.fastWakeupCh:
		if x.testHooks != nil && x.testHooks.PrePollAwake != nil {
			x.testHooks.PrePollAwake()
		}
		x.state.TryTransition(StateSleeping, StateRunning)
		return true
	default:
		return false
	}
}

func (x *Loop) resetFastSleepTimer(d time.Duration) <-chan time.Time {
	if x.fastSleepTimer == nil {
		x.fastSleepTimer = time.NewTimer(d)
		return x.fastSleepTimer.C
	}
	x.stopFastSleepTimer()
	x.fastSleepTimer.Reset(d)
	return x.fastSleepTimer.C
}

func (x *Loop) stopFastSleepTimer() {
	if x.fastSleepTimer == nil {
		return
	}
	if x.fastSleepTimer.Stop() {
		return
	}
	select {
	case <-x.fastSleepTimer.C:
	default:
	}
}

// handlePollError handles errors from PollIO.
func (x *Loop) handlePollError(err error) {
	x.storeTerminalError(err)
	x.logCritical("pollIO failed", err)
	if endTerminalDrain, ok := x.tryBeginTerminalDrainTransition(StateSleeping, StateTerminating); ok {
		x.claimTerminalDrainOwner()
		x.transitionToTerminatedStartedForShutdown(endTerminalDrain)
		x.terminateCleanup()
		x.closeFDs()
		x.startTerminalCompletion()
	}
}
