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
func (l *Loop) poll() {
	currentState := l.state.Load()
	if currentState != StateRunning {
		return
	}
	l.drainCommandIngress()

	// Read and reset forceNonBlockingPoll
	forced := l.forceNonBlockingPoll
	l.forceNonBlockingPoll = false

	if l.testHooks != nil && l.testHooks.PrePollSleep != nil {
		l.testHooks.PrePollSleep()
	}

	// Optimistic state transition
	if !l.state.TryTransition(StateRunning, StateSleeping) {
		return
	}

	// Quick length check (need to hold mutexes for accurate count)
	l.externalMu.Lock()
	extLen := l.commands.Len() + int(l.ownerExternalCount.Load())
	hasPhaseJobs := len(l.checkJobs) > 0 || len(l.closeJobs) > 0 || l.ownerCheckCount.Load() > 0 || l.ownerCloseCount.Load() > 0
	l.externalMu.Unlock()
	intLen := int(l.ownerInternalCount.Load())

	if extLen > 0 || intLen > 0 || hasPhaseJobs || l.microtaskYield.Load() || !l.microtaskQueuesEmpty() {
		l.state.TryTransition(StateSleeping, StateRunning)
		return
	}

	// Auto-exit check: don't block in poll if loop should exit.
	if l.autoExit && !l.Alive() {
		l.state.TryTransition(StateSleeping, StateRunning)
		return
	}

	state := l.state.Load()
	if state == StateTerminating || state == StateTerminated {
		return
	}

	// Calculate timeout
	timeout := l.calculateTimeout()
	if forced {
		timeout = 0
	}

	// Check for termination AGAIN after calculating timeout
	// but BEFORE blocking in poll. This prevents racing with Shutdown.
	state = l.state.Load()
	if state == StateTerminating || state == StateTerminated {
		l.state.TryTransition(StateSleeping, StateRunning)
		return
	}

	// Task-only Auto/Forced loops and targets without native readiness block on
	// the channel wake path. FastPathDisabled deliberately exercises the native
	// poll path on readiness-capable targets even when no user descriptor is registered.
	if l.userIOFDCount.Load() == 0 && (FastPathMode(l.fastPathMode.Load()) != FastPathDisabled || !fdPollingSupported) {
		l.pollFastMode(timeout)
		return
	}

	// Native readiness mode: user FDs are registered or FastPathDisabled was
	// selected explicitly.
	l.pollNative(timeout)
}

// executePollInternal isolates native wake plumbing from user callback
// admission, metrics, TPS, and microtask checkpoints while preserving panic
// containment for test hooks and future internal callbacks.
func (l *Loop) executePollInternal(callback func()) {
	l.logCallbackOutcome("internal poll callback", l.executeOwnedCallback(callback))
}

// pollFastMode is the channel-based fast path for task-only workloads.
// It blocks on fastWakeupCh instead of a native readiness backend.
func (l *Loop) pollFastMode(timeoutMs int) {
	// Drain any pending channel signal first (non-blocking)
	if l.consumeFastWakeup() {
		return
	}

	// Check for termination BEFORE blocking in pollFastMode
	// This prevents a race where shutdown happens after the channel drain
	// but before we block, causing us to sleep indefinitely.
	state := l.state.Load()
	if state == StateTerminating || state == StateTerminated {
		l.state.TryTransition(StateSleeping, StateRunning)
		return
	}

	// Auto-exit check: don't block in pollFastMode if loop should exit.
	if l.autoExit && !l.Alive() {
		l.state.TryTransition(StateSleeping, StateRunning)
		return
	}

	// Non-blocking case
	if timeoutMs == 0 {
		if l.testHooks != nil && l.testHooks.PrePollAwake != nil {
			l.testHooks.PrePollAwake()
		}
		l.state.TryTransition(StateSleeping, StateRunning)
		return
	}

	// No finite deadline: block indefinitely until external wakeup or lifecycle
	// transition. Finite deadlines, including values >= 1s, must use a real
	// timer so scheduled timers cannot sleep forever in fast mode.
	if timeoutMs < 0 {
		// Check termination before indefinite block
		if l.state.Load() == StateTerminating || l.state.Load() == StateTerminated {
			l.state.TryTransition(StateSleeping, StateRunning)
			return
		}
		if l.testHooks != nil && l.testHooks.BeforeFastPollWait != nil {
			l.testHooks.BeforeFastPollWait(timeoutMs)
		}
		// Block indefinitely on channel - no timer allocation
		<-l.fastWakeupCh
		if l.state.Load() == StateTerminated {
			return
		}
		if l.testHooks != nil && l.testHooks.PrePollAwake != nil {
			l.testHooks.PrePollAwake()
		}
		l.state.TryTransition(StateSleeping, StateRunning)
		return
	}

	// Finite timeout - use the loop-owned reusable sleep timer.
	// Check termination before timer-protected block
	state = l.state.Load()
	if state == StateTerminating || state == StateTerminated {
		l.state.TryTransition(StateSleeping, StateRunning)
		return
	}
	timerC := l.resetFastSleepTimer(time.Duration(timeoutMs) * time.Millisecond)
	if l.testHooks != nil && l.testHooks.BeforeFastPollWait != nil {
		l.testHooks.BeforeFastPollWait(timeoutMs)
	}
	select {
	case <-l.fastWakeupCh:
		l.stopFastSleepTimer()
	case <-timerC:
	}
	if l.state.Load() == StateTerminated {
		return
	}

	if l.testHooks != nil && l.testHooks.PrePollAwake != nil {
		l.testHooks.PrePollAwake()
	}

	l.state.TryTransition(StateSleeping, StateRunning)
}

func (l *Loop) consumeFastWakeup() bool {
	select {
	case <-l.fastWakeupCh:
		if l.testHooks != nil && l.testHooks.PrePollAwake != nil {
			l.testHooks.PrePollAwake()
		}
		l.state.TryTransition(StateSleeping, StateRunning)
		return true
	default:
		return false
	}
}

func (l *Loop) resetFastSleepTimer(d time.Duration) <-chan time.Time {
	if l.fastSleepTimer == nil {
		l.fastSleepTimer = time.NewTimer(d)
		return l.fastSleepTimer.C
	}
	l.stopFastSleepTimer()
	l.fastSleepTimer.Reset(d)
	return l.fastSleepTimer.C
}

func (l *Loop) stopFastSleepTimer() {
	if l.fastSleepTimer == nil {
		return
	}
	if l.fastSleepTimer.Stop() {
		return
	}
	select {
	case <-l.fastSleepTimer.C:
	default:
	}
}

// handlePollError handles errors from PollIO.
func (l *Loop) handlePollError(err error) {
	l.storeTerminalError(err)
	l.logCritical("pollIO failed", err)
	if endTerminalDrain, ok := l.tryBeginTerminalDrainTransition(StateSleeping, StateTerminating); ok {
		l.claimTerminalDrainOwner()
		l.transitionToTerminatedStartedForShutdown(endTerminalDrain)
		l.terminateCleanup()
		l.closeFDs()
		l.startTerminalCompletion()
	}
}
