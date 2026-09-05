package eventloop

import (
	"context"
	"fmt"
)

// FastPathMode controls how fast path mode selection works.
type FastPathMode int32

const (
	// FastPathAuto automatically selects mode based on conditions.
	// Default (zero value): uses fast path when userIOFDCount == 0.
	FastPathAuto FastPathMode = iota

	// FastPathForced always uses fast path.
	// Returns ErrFastPathIncompatible if I/O FDs are registered.
	FastPathForced

	// FastPathDisabled uses the regular scheduler and, on readiness-capable
	// targets, the native poll path even without user descriptors.
	FastPathDisabled
)

func validateFastPathMode(mode FastPathMode) error {
	switch mode {
	case FastPathAuto, FastPathForced, FastPathDisabled:
		return nil
	default:
		return fmt.Errorf("invalid fast path mode %d", mode)
	}
}

// SetFastPathMode sets the fast path mode for this loop.
//
// Modes:
//   - FastPathAuto (default): Automatically uses fast path when no I/O FDs registered.
//   - FastPathForced: Always uses fast path (returns error if I/O FDs present).
//   - FastPathDisabled: Uses the regular scheduler and native poll path where supported.
//
// Invariant: When mode is FastPathForced, userIOFDCount must be 0.
//
// Thread Safety: Safe to call concurrently with RegisterFD.
// Uses livenessMu to share the same mode/FD invariant section as FD
// registration and mutation.
//
// SetFastPathMode panics if mode is not a declared [FastPathMode] value.
func (l *Loop) SetFastPathMode(mode FastPathMode) error {
	if err := validateFastPathMode(mode); err != nil {
		panic(fmt.Errorf("eventloop: SetFastPathMode: %w", err))
	}
	if mode == FastPathDisabled && fdPollingSupported {
		if err := l.ensurePollerForModeChange(); err != nil {
			return err
		}
	}

	if l.testHooks != nil && l.testHooks.BeforeSetFastPathModeLock != nil {
		l.testHooks.BeforeSetFastPathModeLock()
	}
	l.livenessMu.Lock()
	if l.state.Load() == StateTerminating || l.state.Load() == StateTerminated {
		l.livenessMu.Unlock()
		return ErrLoopTerminated
	}

	if mode == FastPathForced && l.userIOFDCount.Load() > 0 {
		l.livenessMu.Unlock()
		return ErrFastPathIncompatible
	}

	l.fastPathMode.Store(int32(mode))
	if mode != FastPathForced {
		l.fastPathInvariantLogged.Store(false)
	}
	l.livenessMu.Unlock()

	// Wake the loop so it immediately re-evaluates the mode.
	l.doWakeup()

	return nil
}

// canUseFastPath returns true if fast path can be used right now.
// This consolidates all conditions into a single check.
func (l *Loop) canUseFastPath() bool {
	mode := FastPathMode(l.fastPathMode.Load())
	switch mode {
	case FastPathForced:
		if l.userIOFDCount.Load() > 0 {
			if !l.fastPathInvariantLogged.Swap(true) {
				l.logCritical("eventloop: FastPathForced with registered I/O FDs; falling back to poll path", ErrFastPathIncompatible)
			}
			return false
		}
		l.fastPathInvariantLogged.Store(false)
		return true
	case FastPathDisabled:
		return false
	default: // FastPathAuto
		return l.userIOFDCount.Load() == 0
	}
}

// runFastPath is a tight loop for task-only workloads associated with "fast path" mode.
// Returns true if the loop should continue (check termination), false if should use tick.
//
// It uses a blocking channel select and owner-local task batches without the
// regular scheduler's timer and readiness phases.
func (l *Loop) runFastPath(ctx context.Context) bool {
	l.fastPathEntries.Add(1)
	if l.testHooks != nil && l.testHooks.OnFastPathEntry != nil {
		l.testHooks.OnFastPathEntry()
	}

	for {
		select {
		case <-ctx.Done():
			return true
		default:
		}

		// Fast path must handle: StateTerminated and StateTerminating. This is
		// different from the main loop because terminal-drain ownership may already
		// be published while state is still being finalized.
		currentState := l.state.Load()
		if currentState == StateTerminated || currentState == StateTerminating {
			return true
		}

		// Auto-exit check: don't block in fast path if loop should exit.
		// The main run loop owns the quiescence handler and gate commit so the
		// handler runs exactly once and observes the pre-quiescing state.
		if l.autoExit && !l.Alive() {
			if l.testHooks != nil && l.testHooks.BeforeFastPathAutoExitReturn != nil {
				l.testHooks.BeforeFastPathAutoExitReturn()
			}
			return true
		}

		if l.fastPathNeedsTick() {
			return false
		}

		if l.fastPathHasReadyWork() {
			l.runAux()
			continue
		}

		select {
		case <-ctx.Done():
			return true

		case <-l.fastWakeupCh:
			// Work is observed at the top of the next turn so auto-exit can still
			// skip unref-only handles instead of executing them merely because a wake
			// was received.
		}
	}
}

func (l *Loop) fastPathNeedsTick() bool {
	if !l.canUseFastPath() {
		return true
	}
	return l.hasTimersPending()
}

func (l *Loop) autoExitReady() bool {
	return l.autoExit && !l.Alive()
}

func (l *Loop) fastPathHasReadyWork() bool {
	if l.microtaskYield.Load() || !l.microtaskQueuesEmpty() || l.ownerInternalCount.Load() > 0 || l.ownerExternalCount.Load() > 0 || l.ownerCheckCount.Load() > 0 || l.ownerCloseCount.Load() > 0 {
		return true
	}

	l.externalMu.Lock()
	hasExternal := l.commands.Len() > 0
	hasPhase := len(l.checkJobs) > 0 || len(l.closeJobs) > 0
	l.externalMu.Unlock()
	return hasExternal || hasPhase
}

func (l *Loop) beginQuiescing() {
	l.quiescingEpoch.Store(l.submissionEpoch.Load())
	l.quiescing.Store(true)
}

func (l *Loop) quiescingRejectsLiveness() bool {
	if !l.quiescing.Load() {
		return false
	}
	state := l.state.Load()
	if state == StateTerminating || state == StateTerminated {
		return true
	}
	if l.submissionEpoch.Load() != l.quiescingEpoch.Load() {
		l.quiescing.Store(false)
		return false
	}
	return true
}

func (l *Loop) commitAutoExitTerminalDrain(contextDone <-chan struct{}) (func(), bool) {
	l.livenessMu.Lock()
	l.externalMu.Lock()

	commands := l.snapshotCommandsLocked()
	checkJobs := l.snapshotCheckJobsLocked()
	queuesActive := l.microtaskYield.Load() || !l.microtaskQueuesEmpty() || l.activePhaseJobCount.Load() > 0
	queuesActive = queuesActive || l.ownerInternalCount.Load() > 0 || l.ownerExternalCount.Load() > 0
	quiescing := l.quiescingRejectsLiveness()
	state := l.state.Load()
	terminalActive := state == StateTerminating || state == StateTerminated || l.terminalDraining.Load()
	l.externalMu.Unlock()
	l.livenessMu.Unlock()
	checkJobs = append(checkJobs, l.snapshotOwnerCheckJobs()...)

	// Dynamic check/immediate liveness predicates are user code. Evaluate them
	// outside admission and liveness locks; predicates are allowed to call back
	// into loop APIs such as ScheduleTimer or Submit without deadlocking the
	// auto-exit terminal admission path.
	if terminalActive || !quiescing || queuesActive || l.hasLiveCheckJob(checkJobs) || l.hasLiveCommand(commands) {
		// Every failed commit leaves the loop running. Lower the provisional
		// admission gate before returning to its next quiescence callback or
		// accepting an ordinary liveness-adding operation.
		l.quiescing.Store(false)
		return nil, false
	}

	l.livenessMu.Lock()
	l.externalMu.Lock()

	queuesActive = l.microtaskYield.Load() || !l.microtaskQueuesEmpty() || l.activePhaseJobCount.Load() > 0
	queuesActive = queuesActive || l.ownerInternalCount.Load() > 0 || l.ownerExternalCount.Load() > 0
	state = l.state.Load()
	terminalActive = state == StateTerminating || state == StateTerminated || l.terminalDraining.Load()
	if terminalActive || !l.quiescingRejectsLiveness() || queuesActive {
		l.quiescing.Store(false)
		l.externalMu.Unlock()
		l.livenessMu.Unlock()
		return nil, false
	}

	if l.testHooks != nil && l.testHooks.BeforeAutoExitTerminalDrainCommit != nil {
		l.testHooks.BeforeAutoExitTerminalDrainCommit()
	}
	// This is the context/auto-exit precedence cut. Cancellation already
	// observable while final admission is locked wins; once the terminal drain
	// commits, clean auto-exit wins over a later cancellation.
	select {
	case <-contextDone:
		l.quiescing.Store(false)
		l.externalMu.Unlock()
		l.livenessMu.Unlock()
		return nil, false
	default:
	}

	endTerminalDrain := l.beginAutoExitTerminalDrain()
	l.externalMu.Unlock()
	l.livenessMu.Unlock()
	return endTerminalDrain, true
}
