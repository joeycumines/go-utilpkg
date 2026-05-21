package eventloop

import (
	"context"
	"time"

	"github.com/joeycumines/goroutineid"
)

// Run owns event-loop execution and blocks until that owner exits.
//
// A winning immediate [Loop.Close] completes terminal resource cleanup before
// Run returns, and Run includes any cleanup failure in its result. Graceful
// terminal completion may outlive Run while already-admitted [Loop.Promisify]
// functions finish. External Shutdown or Close calls whose initial completion
// probe observes that barrier still open join its result; Shutdown remains
// bounded by its context.
//
// When context cancellation and clean auto-exit compete, cancellation already
// observable at the final terminal-admission boundary wins and is included in
// the result. Once auto-exit commits terminal admission, a later cancellation
// does not replace that clean completion.
//
// Run observes context cancellation only after owner work returns to the outer
// run loop. It does not preempt a callback or its exhaustive microtask
// checkpoint, so a recursively replenished nextTick, microtask, checkpoint, or
// Promise reaction chain can delay cancellation.
//
// To run in a separate goroutine, use: `go loop.Run(ctx)`.
func (l *Loop) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if l.isLoopThread() {
		return ErrReentrantRun
	}
	// A published terminal state needs no lifecycle arbitration. This fast
	// probe also lets Run report the stable result while the terminal winner is
	// still publishing mode under livenessMu.
	currentState := l.state.Load()
	if currentState == StateTerminated || currentState == StateTerminating {
		return ErrLoopTerminated
	}

	if l.testHooks != nil && l.testHooks.BeforeRunLifecycleLock != nil {
		l.testHooks.BeforeRunLifecycleLock()
	}
	l.livenessMu.Lock()
	if !l.state.TryTransition(StateAwake, StateRunning) {
		currentState = l.state.Load()
		l.livenessMu.Unlock()
		if currentState == StateTerminated || currentState == StateTerminating {
			return ErrLoopTerminated
		}
		return ErrLoopAlreadyRunning
	}
	l.livenessMu.Unlock()
	if l.testHooks != nil && l.testHooks.AfterRunStateRunningBeforeStart != nil {
		l.testHooks.AfterRunStateRunningBeforeStart()
	}
	l.runStarted.Store(true)

	now := time.Now()
	l.setTickAnchor(now)
	l.tickNow = now

	runErr := l.run(ctx)
	// Only the successful Run owner publishes loopDone. Immediate Close uses
	// that signal to begin resource cleanup; Run then joins the shorter terminal
	// barrier so it cannot publish a stale pre-cleanup result. Graceful completion
	// remains independent because it may wait for a Promisify worker that itself
	// depends on Run returning.
	l.closeLoopDoneOnce.Do(func() { close(l.loopDone) })
	if l.immediateCloseWon() {
		<-l.terminalDone
		return joinErrors(runErr, l.terminalError())
	}
	return runErr
}

// Shutdown gracefully shuts down the event loop.
//
// The external caller that commits the shutdown transition waits for accepted
// queued tasks, in-flight Promisify operations, loop exit, and terminal cleanup.
// If ctx is canceled first, Shutdown returns ctx.Err while cleanup continues
// independently. A Promisify worker that commits the transition returns nil once
// the independent completion path owns the request; that nil acknowledges the
// request, not completed cleanup, because cleanup waits for that worker to return.
// An external caller that observes terminal completion already published
// returns [ErrLoopTerminated]. If it observes an in-progress terminal operation,
// it joins the same completion barrier subject to its own context. Caller roles
// that cannot safely join use a mode-sensitive result instead: a loop callback,
// Promisify worker, or post-drain callback holding terminal-completion ownership
// returns nil when it observes an active graceful termination and
// [ErrLoopTerminated] when it observes an active immediate termination. A
// callback executing in the terminal drain retains drain ownership instead:
// Shutdown acknowledges graceful mode, while Close returns [ErrReentrantClose].
// If terminal cleanup fails, every external caller that observes terminalDone
// before context cancellation receives the aggregate terminal error.
// Loop-callback and Promisify-worker winners return the earlier nil request
// acknowledgement described above. Use [Loop.Requests] when a dependency child
// must acknowledge a graceful request without joining terminal cleanup.
//
// Graceful Shutdown does not preempt a running callback or its exhaustive
// microtask checkpoint. The logical callback or terminal-drain owner may keep
// admitting checkpoint continuations until the queues empty; an unbounded chain
// can therefore delay terminal completion. A caller context still bounds that
// caller's wait.
func (l *Loop) Shutdown(ctx context.Context) error {
	return l.shutdownImpl(ctx, true)
}

// shutdownImpl contains the actual Shutdown implementation.
func (l *Loop) shutdownImpl(ctx context.Context, join bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if l.isLoopThread() {
		return l.shutdownLoopThread()
	}
	if l.isTerminalDrainOwner() {
		// A callback delegated by the pre-Run graceful drain owns this narrow
		// capability instead of loop-thread identity. It must acknowledge the
		// active graceful request without joining the drain that is waiting for
		// the callback to return.
		if !l.immediateCloseWon() {
			return nil
		}
		return ErrLoopTerminated
	}
	nonjoining := !join || l.isPromisifyWorker()
	if l.terminalCompletionPublished() {
		l.retryPollerCleanup()
		return ErrLoopTerminated
	}
	if l.isTerminalCompletionOwner() {
		return l.terminalRequestResult(false)
	}

	for {
		currentState := l.state.Load()
		if currentState == StateTerminated || currentState == StateTerminating {
			if nonjoining {
				return l.terminalRequestResult(false)
			}
			l.beforeTerminalJoin()
			return l.waitShutdownCompletion(ctx)
		}
		if l.testHooks != nil && l.testHooks.BeforeShutdownLifecycleLock != nil {
			l.testHooks.BeforeShutdownLifecycleLock()
		}

		endTerminalDrain, ok := l.tryBeginTerminalDrainRequest(currentState, StateTerminating)
		if !ok {
			continue
		}
		if l.testHooks != nil && l.testHooks.AfterShutdownStateTerminating != nil {
			l.testHooks.AfterShutdownStateTerminating()
		}
		l.startTerminalDependencyRelease()

		if currentState == StateAwake {
			// Run has not acquired ownership. A dedicated drain goroutine must own
			// callbacks so cancellation of this caller never abandons cleanup and
			// never leaves the caller executing an unbounded callback or worker wait.
			l.startAwakeShutdown(endTerminalDrain)
		} else {
			// Wake up the loop - in fast path mode, the loop may be blocking on
			// fastWakeupCh without transitioning to StateSleeping.
			l.doWakeup()
		}
		if nonjoining {
			return nil
		}
		return l.waitShutdownCompletion(ctx)
	}
}

func (l *Loop) waitShutdownCompletion(ctx context.Context) error {
	// Prefer a completed graceful shutdown over a context that became ready at
	// the same boundary. Context cancellation is authoritative only while
	// terminal cleanup remains incomplete.
	select {
	case <-l.terminalDone:
		return l.terminalError()
	default:
	}

	select {
	case <-l.terminalDone:
		return l.terminalError()
	case <-ctx.Done():
		if l.testHooks != nil && l.testHooks.AfterShutdownJoinContext != nil {
			l.testHooks.AfterShutdownJoinContext()
		}
		select {
		case <-l.terminalDone:
			return l.terminalError()
		default:
			return ctx.Err()
		}
	}
}

func (l *Loop) terminalCompletionPublished() bool {
	select {
	case <-l.terminalDone:
		return true
	default:
		return false
	}
}

func (l *Loop) terminalRequestResult(immediate bool) error {
	if l.immediateCloseWon() == immediate {
		return nil
	}
	return ErrLoopTerminated
}

func (l *Loop) beforeTerminalJoin() {
	if l.testHooks != nil && l.testHooks.BeforeTerminalJoin != nil {
		l.testHooks.BeforeTerminalJoin()
	}
}

func (l *Loop) startAwakeShutdown(endTerminalDrain func()) {
	l.terminalCompletionOnce.Do(func() {
		go l.finishAwakeShutdown(endTerminalDrain)
	})
}

func (l *Loop) finishAwakeShutdown(endTerminalDrain func()) {
	l.waitTerminalDependencyRelease()
	releaseCompletionOwner := l.claimTerminalCompletionOwner()
	defer releaseCompletionOwner()

	// Public transition requests publish no drain owner. Claim that narrow
	// admission capability on the dedicated goroutine before running callbacks.
	l.claimTerminalDrainOwner()
	l.transitionToTerminatedStartedForShutdown(endTerminalDrain)

	// Accepted callback dependencies are now drained, so workers waiting on
	// those callbacks can finish. New workers and queue submissions have already
	// been excluded by StateTerminated under promisifyMu/livenessMu.
	l.waitPromisifyGoroutines()
	l.rejectAllPendingPromises(ErrLoopTerminated)
	l.terminateCleanup()
	l.closeFDs()

	// Run never acquired this loop, so this finisher owns both completion
	// signals. terminalDone is the public completion barrier and closes last.
	l.closeLoopDoneOnce.Do(func() { close(l.loopDone) })
	releaseCompletionOwner()
	l.closeTerminalDone()
}

func (l *Loop) startTerminalCompletion() {
	l.terminalCompletionOnce.Do(func() {
		go l.finishTerminalCompletion()
	})
}

func (l *Loop) finishTerminalCompletion() {
	l.waitTerminalDependencyRelease()
	releaseCompletionOwner := l.claimTerminalCompletionOwner()
	defer releaseCompletionOwner()

	// The loop drains all accepted queue dependencies before starting this
	// finisher. Wait for the owner goroutine to exit, then wait for workers that
	// can no longer enqueue owner work. The winning public Shutdown caller
	// independently selects terminalDone against its own context.
	l.waitLoopDoneAfterTerminal()
	l.waitPromisifyGoroutines()
	l.rejectAllPendingPromises(ErrLoopTerminated)
	releaseCompletionOwner()
	l.closeTerminalDone()
}

func (l *Loop) startImmediateTerminalCompletion(waitLoop bool) {
	l.terminalCompletionOnce.Do(func() {
		go l.finishImmediateTerminalCompletion(waitLoop)
	})
}

func (l *Loop) finishImmediateTerminalCompletion(waitLoop bool) {
	l.waitTerminalDependencyRelease()
	releaseCompletionOwner := l.claimTerminalCompletionOwner()
	defer releaseCompletionOwner()

	if waitLoop {
		<-l.loopDone
	}
	l.closeFDs()
	l.terminateCleanup()
	if !waitLoop {
		l.closeLoopDoneOnce.Do(func() { close(l.loopDone) })
	}
	releaseCompletionOwner()
	l.closeTerminalDone()
}

func (l *Loop) shutdownLoopThread() error {
	for {
		currentState := l.state.Load()
		switch currentState {
		case StateTerminating, StateTerminated:
			// A logical loop owner cannot join completion that waits for that owner
			// to return. Resolve the already-committed mode instead. This also covers
			// synchronous cleanup callbacks after a graceful drain has ended.
			if l.terminalCompletionPublished() {
				return ErrLoopTerminated
			}
			return l.terminalRequestResult(false)
		}

		if _, ok := l.tryBeginTerminalDrainTransition(currentState, StateTerminating); ok {
			// The loop goroutine already owns execution. It will observe
			// StateTerminating after the current callback/checkpoint returns, drain
			// terminal continuations, clean loop-owned state, close loopDone via
			// Run's defer, and let the independent finisher publish terminalDone
			// after admitted Promisify workers complete.
			l.startTerminalDependencyRelease()
			return nil
		}
	}
}

// run is the main loop goroutine.
func (l *Loop) run(ctx context.Context) error {
	l.loopGoroutineID.Store(goroutineid.Get())
	defer l.loopGoroutineID.Store(0)

	// Start context watcher goroutine to wake loop on cancellation
	ctxDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			l.doWakeup()
		case <-ctxDone:
		}
	}()
	defer close(ctxDone)

	startupQueuesDrained := false

	for {
		select {
		case <-ctx.Done():
			// Context cancelled, initiate shutdown sequence and return
			// DO NOT wait for promisifyWg in loop goroutine itself - that causes deadlock
			// if a Promisify goroutine is blocking on something
			ownedTermination := false
			for {
				l.livenessMu.Lock()
				current := l.state.Load()
				if current == StateTerminating {
					l.livenessMu.Unlock()
					if l.immediateCloseWon() {
						return ctx.Err()
					}
					l.claimTerminalDrainOwner()
					l.transitionToTerminatedStartedForShutdown(l.finishActiveTerminalDrain)
					l.terminateCleanup()
					l.closeFDs()
					l.startTerminalCompletion()
					return joinErrors(ctx.Err(), l.terminalError())
				}
				if current == StateTerminated {
					l.livenessMu.Unlock()
					if l.immediateCloseWon() {
						return ctx.Err()
					}
					l.closeFDs()
					return joinErrors(ctx.Err(), l.terminalError())
				}
				if l.state.TryTransition(current, StateTerminating) {
					ownedTermination = true
					l.livenessMu.Unlock()
					if current == StateSleeping {
						l.doWakeup()
					}
					break
				}
				l.livenessMu.Unlock()
			}
			if !ownedTermination {
				l.closeFDs()
				return joinErrors(ctx.Err(), l.terminalError())
			}
			// Transition state to Terminated so new Promisify operations are rejected.
			// Drain queues on the loop goroutine, then defer promise rejection and the
			// terminal completion signal until in-flight Promisify callbacks have
			// reached their terminal state.
			l.transitionToTerminatedForShutdown()
			l.terminateCleanup() // GAP-AE-06: full cleanup resets all liveness counters
			l.closeFDs()
			l.startTerminalCompletion()
			return joinErrors(ctx.Err(), l.terminalError())
		default:
		}

		terminalState := l.state.Load()
		if terminalState == StateTerminating || terminalState == StateTerminated {
			// Immediate Close owns resource cleanup and only needs the loop owner to
			// stop. Graceful termination keeps drain execution on this goroutine so
			// callbacks preserve their affinity contract.
			if l.immediateCloseWon() {
				return nil
			}
			if terminalState == StateTerminating {
				l.claimTerminalDrainOwner()
				l.transitionToTerminatedStartedForShutdown(l.finishActiveTerminalDrain)
				l.terminateCleanup()
				l.startTerminalCompletion()
			}
			l.closeFDs()
			return l.terminalError()
		}

		if !startupQueuesDrained {
			startupQueuesDrained = true
			l.drainStartupQueues()
			if l.hardAbortRequested() {
				continue
			}
		}

		// Auto-exit: if enabled and no ref'd work remains, terminate cleanly.
		// This is analogous to libuv's UV_RUN_DEFAULT mode where the loop exits
		// when there are no more active and referenced handles.
		//
		// Quiescing protocol: set the quiescing flag BEFORE committing termination.
		// This gates all liveness-adding APIs (ScheduleTimer, RegisterFD, RefTimer,
		// Promisify) so no new work can be accepted after this point. Then re-check
		// Alive() to catch any work that was added between the initial !Alive()
		// decision and the flag being set (the epoch-based consistency in Alive()
		// detects concurrent epoch changes). If work was added, abort termination.
		if l.autoExit {
			alive := l.Alive()
			if l.quiescing.Load() && alive {
				// runFastPath may return after setting quiescing when it observes
				// no liveness. If ephemeral work (Submit, ScheduleMicrotask,
				// ScheduleNextTick) arrives before termination commits, Alive()
				// becomes true and termination must be aborted. Clear the stale
				// fast-path gate before executing that work so liveness-adding APIs
				// called by the accepted task are not falsely rejected.
				l.quiescing.Store(false)
			}

			if !alive {
				if l.runQuiescenceHandlers() || l.Alive() {
					l.quiescing.Store(false)
					continue
				}
				l.livenessMu.Lock()
				l.beginQuiescing()
				l.livenessMu.Unlock()

				// Re-check after gate: catches work added between !Alive() and the flag.
				if l.Alive() {
					l.quiescing.Store(false)
					continue
				}
				if l.testHooks != nil && l.testHooks.BeforeAutoExitCommit != nil {
					l.testHooks.BeforeAutoExitCommit()
				}
				if l.Alive() {
					l.quiescing.Store(false)
					continue
				}
				if l.testHooks != nil && l.testHooks.AfterAutoExitFinalAliveCheck != nil {
					l.testHooks.AfterAutoExitFinalAliveCheck()
				}
				l.externalMu.Lock()
				checkJobs := l.snapshotCheckJobsLocked()
				commands := l.snapshotCommandsLocked()
				queuesActive := l.microtaskYield.Load() || !l.microtaskQueuesEmpty() || l.ownerInternalCount.Load() > 0 || l.ownerExternalCount.Load() > 0 || l.activePhaseJobCount.Load() > 0
				l.externalMu.Unlock()
				checkJobs = append(checkJobs, l.snapshotOwnerCheckJobs()...)
				if queuesActive || l.hasLiveCheckJob(checkJobs) || l.hasLiveCommand(commands) {
					l.quiescing.Store(false)
					continue
				}
				endTerminalDrain, ok := l.commitAutoExitTerminalDrain(ctx.Done())
				if !ok {
					continue
				}
				l.transitionToTerminatedStarted(endTerminalDrain)
				l.terminateCleanup()
				l.closeFDs()
				l.startTerminalCompletion()
				return l.fdResourceCloseError()
			}
		}

		// Use the tight fast-path loop for task-only workloads.
		if l.canUseFastPath() && !l.hasTimersPending() {
			if l.runFastPath(ctx) {
				// Fast path completed or needs mode switch - continue to check termination
				continue
			}
			// Fall through to regular tick for feature transition
		}

		l.tick()
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

func (l *Loop) rejectLivenessAdd() error {
	l.livenessMu.Lock()
	defer l.livenessMu.Unlock()
	return l.rejectLivenessAddLocked()
}

func (l *Loop) rejectLivenessAddLocked() error {
	state := l.state.Load()
	if state == StateTerminating || state == StateTerminated {
		return ErrLoopTerminated
	}
	if l.terminalDraining.Load() {
		return ErrLoopTerminated
	}
	if l.quiescingRejectsLiveness() {
		return ErrLoopTerminated
	}
	return nil
}

func (l *Loop) rejectFDMutationLocked(allowTerminating bool) error {
	state := l.state.Load()
	if state == StateTerminated || (!allowTerminating && state == StateTerminating) {
		return ErrLoopTerminated
	}
	return nil
}

// Close immediately terminates the event loop when it wins the terminal transition.
//
// NOTE: A winning external Close waits for the loop goroutine to exit before
// returning. This prevents data races where the caller frees resources that the
// loop goroutine might still be accessing. A winning Promisify worker returns an
// earlier request acknowledgement because joining can depend on that worker.
//
// Close does not wait for user functions that already claimed entry through
// [Loop.Promisify]. It rejects their registered pending promises before waiting
// for the loop owner, discards queued callbacks, and then releases loop resources.
// A committed worker that has not claimed entry when Close wins skips its user
// function. The claim is the lifecycle boundary: a worker that claimed entry may
// execute its first user-function instruction even after Close returns. Such a function
// keeps running under its caller-provided context, but subsequent attempts to add
// loop work are rejected with [ErrLoopTerminated], and JS handle publication is
// serialized with terminal cleanup. Callers that need worker cancellation must
// cancel the context supplied to Promisify.
//
// Concurrent Close()/Shutdown(): Only one call chooses graceful or immediate
// terminal mode and owns cleanup. An external non-winning caller joins that
// operation when its initial completion probe observes the barrier still open.
// A Promisify worker never joins because graceful completion directly contains
// that worker, and immediate completion can depend on it transitively through a
// loop callback. A worker returns nil when immediate mode already satisfies its
// Close request and ErrLoopTerminated when graceful mode won. Calls whose probe
// observes published completion return ErrLoopTerminated. A post-drain
// dependency-release or cleanup diagnostic callback holding terminal-completion
// ownership uses the same nonjoining, mode-sensitive result so it cannot wait on
// the completion it interrupted. A callback executing in the terminal drain
// retains drain ownership, so Close returns ErrReentrantClose. Every joined or
// winning external caller receives the aggregate terminal error. Use
// [Loop.Requests] when a dependency child or loop callback must acknowledge an
// immediate request without joining terminal cleanup.
func (l *Loop) Close() error {
	return l.closeImpl(true)
}

func (l *Loop) closeImpl(join bool) error {
	if join && (l.isLoopThread() || l.isTerminalDrainOwner()) {
		return ErrReentrantClose
	}
	nonjoining := !join || l.isPromisifyWorker()
	if l.terminalCompletionPublished() {
		l.retryPollerCleanup()
		return ErrLoopTerminated
	}
	if l.isTerminalCompletionOwner() {
		return l.terminalRequestResult(true)
	}

	for {
		currentState := l.state.Load()
		if currentState == StateTerminating || currentState == StateTerminated {
			if nonjoining {
				return l.terminalRequestResult(true)
			}
			l.beforeTerminalJoin()
			<-l.terminalDone
			return l.terminalError()
		}
		if l.testHooks != nil && l.testHooks.BeforeCloseLifecycleLock != nil {
			l.testHooks.BeforeCloseLifecycleLock()
		}

		l.livenessMu.Lock()
		currentState = l.state.Load()
		if currentState == StateTerminating || currentState == StateTerminated {
			l.livenessMu.Unlock()
			if nonjoining {
				return l.terminalRequestResult(true)
			}
			l.beforeTerminalJoin()
			<-l.terminalDone
			return l.terminalError()
		}
		l.terminalDrainMu.Lock()
		l.callbackGateMu.Lock()
		if l.state.TryTransition(currentState, StateTerminating) {
			if l.testHooks != nil && l.testHooks.TerminalStateCAS != nil {
				l.testHooks.TerminalStateCAS()
			}
			l.immediateClose.Store(true)
			l.callbackGateMode = callbackGateClosed
			l.callbackGateMu.Unlock()
			l.terminalDrainMu.Unlock()
			if l.testHooks != nil && l.testHooks.AfterCloseStateTerminating != nil {
				l.testHooks.AfterCloseStateTerminating()
			}
			waitLoop := currentState != StateAwake
			// Publish the immediate terminal state before releasing lifecycle
			// admission. A Run owner that already committed StateRunning is joined
			// even if it has not yet published runStarted.
			l.state.Store(StateTerminated)
			l.livenessMu.Unlock()
			// Release callbacks already admitted by the loop owner before waiting for
			// that owner to exit. Such a callback may be blocked on a Promisify
			// promise whose user function outlives Close; delaying rejection until
			// after loopDone would make each side wait for the other.
			if l.testHooks != nil && l.testHooks.BeforeClosePromiseRejection != nil {
				l.testHooks.BeforeClosePromiseRejection()
			}
			l.rejectAllPendingPromises(ErrLoopTerminated)
			l.startTerminalDependencyRelease()
			if waitLoop {
				l.forceWakeup()
			}
			l.startImmediateTerminalCompletion(waitLoop)
			if nonjoining {
				return nil
			}
			<-l.terminalDone
			return l.terminalError()
		}
		l.callbackGateMu.Unlock()
		l.terminalDrainMu.Unlock()
		l.livenessMu.Unlock()
	}
}
