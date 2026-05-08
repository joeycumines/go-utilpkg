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
func (x *Loop) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if x.isLoopThread() {
		return ErrReentrantRun
	}
	// A published terminal state needs no lifecycle arbitration. This fast
	// probe also lets Run report the stable result while the terminal winner is
	// still publishing mode under livenessMu.
	currentState := x.state.Load()
	if currentState == StateTerminated || currentState == StateTerminating {
		return ErrLoopTerminated
	}

	if x.testHooks != nil && x.testHooks.BeforeRunLifecycleLock != nil {
		x.testHooks.BeforeRunLifecycleLock()
	}
	x.livenessMu.Lock()
	if !x.state.TryTransition(StateAwake, StateRunning) {
		currentState = x.state.Load()
		x.livenessMu.Unlock()
		if currentState == StateTerminated || currentState == StateTerminating {
			return ErrLoopTerminated
		}
		return ErrLoopAlreadyRunning
	}
	x.livenessMu.Unlock()
	if x.testHooks != nil && x.testHooks.AfterRunStateRunningBeforeStart != nil {
		x.testHooks.AfterRunStateRunningBeforeStart()
	}
	x.runStarted.Store(true)

	now := time.Now()
	x.setTickAnchor(now)
	x.tickNow = now

	runErr := x.run(ctx)
	// Only the successful Run owner publishes loopDone. Immediate Close uses
	// that signal to begin resource cleanup; Run then joins the shorter terminal
	// barrier so it cannot publish a stale pre-cleanup result. Graceful completion
	// remains independent because it may wait for a Promisify worker that itself
	// depends on Run returning.
	x.closeLoopDoneOnce.Do(func() { close(x.loopDone) })
	if x.immediateCloseWon() {
		<-x.terminalDone
		return joinErrors(runErr, x.terminalError())
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
func (x *Loop) Shutdown(ctx context.Context) error {
	return x.shutdownImpl(ctx, true)
}

// shutdownImpl contains the actual Shutdown implementation.
func (x *Loop) shutdownImpl(ctx context.Context, join bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if x.isLoopThread() {
		return x.shutdownLoopThread()
	}
	if x.isTerminalDrainOwner() {
		// A callback delegated by the pre-Run graceful drain owns this narrow
		// capability instead of loop-thread identity. It must acknowledge the
		// active graceful request without joining the drain that is waiting for
		// the callback to return.
		if !x.immediateCloseWon() {
			return nil
		}
		return ErrLoopTerminated
	}
	nonjoining := !join || x.isPromisifyWorker()
	if x.terminalCompletionPublished() {
		x.retryPollerCleanup()
		return ErrLoopTerminated
	}
	if x.isTerminalCompletionOwner() {
		return x.terminalRequestResult(false)
	}

	for {
		currentState := x.state.Load()
		if currentState == StateTerminated || currentState == StateTerminating {
			if nonjoining {
				return x.terminalRequestResult(false)
			}
			x.beforeTerminalJoin()
			return x.waitShutdownCompletion(ctx)
		}
		if x.testHooks != nil && x.testHooks.BeforeShutdownLifecycleLock != nil {
			x.testHooks.BeforeShutdownLifecycleLock()
		}

		endTerminalDrain, ok := x.tryBeginTerminalDrainRequest(currentState, StateTerminating)
		if !ok {
			continue
		}
		if x.testHooks != nil && x.testHooks.AfterShutdownStateTerminating != nil {
			x.testHooks.AfterShutdownStateTerminating()
		}
		x.startTerminalDependencyRelease()

		if currentState == StateAwake {
			// Run has not acquired ownership. A dedicated drain goroutine must own
			// callbacks so cancellation of this caller never abandons cleanup and
			// never leaves the caller executing an unbounded callback or worker wait.
			x.startAwakeShutdown(endTerminalDrain)
		} else {
			// Wake up the loop - in fast path mode, the loop may be blocking on
			// fastWakeupCh without transitioning to StateSleeping.
			x.doWakeup()
		}
		if nonjoining {
			return nil
		}
		return x.waitShutdownCompletion(ctx)
	}
}

func (x *Loop) waitShutdownCompletion(ctx context.Context) error {
	// Prefer a completed graceful shutdown over a context that became ready at
	// the same boundary. Context cancellation is authoritative only while
	// terminal cleanup remains incomplete.
	select {
	case <-x.terminalDone:
		return x.terminalError()
	default:
	}

	select {
	case <-x.terminalDone:
		return x.terminalError()
	case <-ctx.Done():
		if x.testHooks != nil && x.testHooks.AfterShutdownJoinContext != nil {
			x.testHooks.AfterShutdownJoinContext()
		}
		select {
		case <-x.terminalDone:
			return x.terminalError()
		default:
			return ctx.Err()
		}
	}
}

func (x *Loop) terminalCompletionPublished() bool {
	select {
	case <-x.terminalDone:
		return true
	default:
		return false
	}
}

func (x *Loop) terminalRequestResult(immediate bool) error {
	if x.immediateCloseWon() == immediate {
		return nil
	}
	return ErrLoopTerminated
}

func (x *Loop) beforeTerminalJoin() {
	if x.testHooks != nil && x.testHooks.BeforeTerminalJoin != nil {
		x.testHooks.BeforeTerminalJoin()
	}
}

func (x *Loop) startAwakeShutdown(endTerminalDrain func()) {
	x.terminalCompletionOnce.Do(func() {
		go x.finishAwakeShutdown(endTerminalDrain)
	})
}

func (x *Loop) finishAwakeShutdown(endTerminalDrain func()) {
	x.waitTerminalDependencyRelease()
	releaseCompletionOwner := x.claimTerminalCompletionOwner()
	defer releaseCompletionOwner()

	// Public transition requests publish no drain owner. Claim that narrow
	// admission capability on the dedicated goroutine before running callbacks.
	x.claimTerminalDrainOwner()
	x.transitionToTerminatedStartedForShutdown(endTerminalDrain)

	// Accepted callback dependencies are now drained, so workers waiting on
	// those callbacks can finish. New workers and queue submissions have already
	// been excluded by StateTerminated under promisifyMu/livenessMu.
	x.waitPromisifyGoroutines()
	x.rejectAllPendingPromises(ErrLoopTerminated)
	x.terminateCleanup()
	x.closeFDs()

	// Run never acquired this loop, so this finisher owns both completion
	// signals. terminalDone is the public completion barrier and closes last.
	x.closeLoopDoneOnce.Do(func() { close(x.loopDone) })
	releaseCompletionOwner()
	x.closeTerminalDone()
}

func (x *Loop) startTerminalCompletion() {
	x.terminalCompletionOnce.Do(func() {
		go x.finishTerminalCompletion()
	})
}

func (x *Loop) finishTerminalCompletion() {
	x.waitTerminalDependencyRelease()
	releaseCompletionOwner := x.claimTerminalCompletionOwner()
	defer releaseCompletionOwner()

	// The loop drains all accepted queue dependencies before starting this
	// finisher. Wait for the owner goroutine to exit, then wait for workers that
	// can no longer enqueue owner work. The winning public Shutdown caller
	// independently selects terminalDone against its own context.
	x.waitLoopDoneAfterTerminal()
	x.waitPromisifyGoroutines()
	x.rejectAllPendingPromises(ErrLoopTerminated)
	releaseCompletionOwner()
	x.closeTerminalDone()
}

func (x *Loop) startImmediateTerminalCompletion(waitLoop bool) {
	x.terminalCompletionOnce.Do(func() {
		go x.finishImmediateTerminalCompletion(waitLoop)
	})
}

func (x *Loop) finishImmediateTerminalCompletion(waitLoop bool) {
	x.waitTerminalDependencyRelease()
	releaseCompletionOwner := x.claimTerminalCompletionOwner()
	defer releaseCompletionOwner()

	if waitLoop {
		<-x.loopDone
	}
	x.closeFDs()
	x.terminateCleanup()
	if !waitLoop {
		x.closeLoopDoneOnce.Do(func() { close(x.loopDone) })
	}
	releaseCompletionOwner()
	x.closeTerminalDone()
}

func (x *Loop) shutdownLoopThread() error {
	for {
		currentState := x.state.Load()
		switch currentState {
		case StateTerminating, StateTerminated:
			// A logical loop owner cannot join completion that waits for that owner
			// to return. Resolve the already-committed mode instead. This also covers
			// synchronous cleanup callbacks after a graceful drain has ended.
			if x.terminalCompletionPublished() {
				return ErrLoopTerminated
			}
			return x.terminalRequestResult(false)
		}

		if _, ok := x.tryBeginTerminalDrainTransition(currentState, StateTerminating); ok {
			// The loop goroutine already owns execution. It will observe
			// StateTerminating after the current callback/checkpoint returns, drain
			// terminal continuations, clean loop-owned state, close loopDone via
			// Run's defer, and let the independent finisher publish terminalDone
			// after admitted Promisify workers complete.
			x.startTerminalDependencyRelease()
			return nil
		}
	}
}

// run is the main loop goroutine.
func (x *Loop) run(ctx context.Context) error {
	x.loopGoroutineID.Store(goroutineid.Get())
	defer x.loopGoroutineID.Store(0)

	// Start context watcher goroutine to wake loop on cancellation
	ctxDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			x.doWakeup()
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
				x.livenessMu.Lock()
				current := x.state.Load()
				if current == StateTerminating {
					x.livenessMu.Unlock()
					if x.immediateCloseWon() {
						return ctx.Err()
					}
					x.claimTerminalDrainOwner()
					x.transitionToTerminatedStartedForShutdown(x.finishActiveTerminalDrain)
					x.terminateCleanup()
					x.closeFDs()
					x.startTerminalCompletion()
					return joinErrors(ctx.Err(), x.terminalError())
				}
				if current == StateTerminated {
					x.livenessMu.Unlock()
					if x.immediateCloseWon() {
						return ctx.Err()
					}
					x.closeFDs()
					return joinErrors(ctx.Err(), x.terminalError())
				}
				if x.state.TryTransition(current, StateTerminating) {
					ownedTermination = true
					x.livenessMu.Unlock()
					if current == StateSleeping {
						x.doWakeup()
					}
					break
				}
				x.livenessMu.Unlock()
			}
			if !ownedTermination {
				x.closeFDs()
				return joinErrors(ctx.Err(), x.terminalError())
			}
			// Transition state to Terminated so new Promisify operations are rejected.
			// Drain queues on the loop goroutine, then defer promise rejection and the
			// terminal completion signal until in-flight Promisify callbacks have
			// reached their terminal state.
			x.transitionToTerminatedForShutdown()
			x.terminateCleanup() // GAP-AE-06: full cleanup resets all liveness counters
			x.closeFDs()
			x.startTerminalCompletion()
			return joinErrors(ctx.Err(), x.terminalError())
		default:
		}

		terminalState := x.state.Load()
		if terminalState == StateTerminating || terminalState == StateTerminated {
			// Immediate Close owns resource cleanup and only needs the loop owner to
			// stop. Graceful termination keeps drain execution on this goroutine so
			// callbacks preserve their affinity contract.
			if x.immediateCloseWon() {
				return nil
			}
			if terminalState == StateTerminating {
				x.claimTerminalDrainOwner()
				x.transitionToTerminatedStartedForShutdown(x.finishActiveTerminalDrain)
				x.terminateCleanup()
				x.startTerminalCompletion()
			}
			x.closeFDs()
			return x.terminalError()
		}

		if !startupQueuesDrained {
			startupQueuesDrained = true
			x.drainStartupQueues()
			if x.hardAbortRequested() {
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
		if x.autoExit {
			alive := x.Alive()
			if x.quiescing.Load() && alive {
				// runFastPath may return after setting quiescing when it observes
				// no liveness. If ephemeral work (Submit, ScheduleMicrotask,
				// ScheduleNextTick) arrives before termination commits, Alive()
				// becomes true and termination must be aborted. Clear the stale
				// fast-path gate before executing that work so liveness-adding APIs
				// called by the accepted task are not falsely rejected.
				x.quiescing.Store(false)
			}

			if !alive {
				if x.runQuiescenceHandlers() || x.Alive() {
					x.quiescing.Store(false)
					continue
				}
				x.livenessMu.Lock()
				x.beginQuiescing()
				x.livenessMu.Unlock()

				// Re-check after gate: catches work added between !Alive() and the flag.
				if x.Alive() {
					x.quiescing.Store(false)
					continue
				}
				if x.testHooks != nil && x.testHooks.BeforeAutoExitCommit != nil {
					x.testHooks.BeforeAutoExitCommit()
				}
				if x.Alive() {
					x.quiescing.Store(false)
					continue
				}
				if x.testHooks != nil && x.testHooks.AfterAutoExitFinalAliveCheck != nil {
					x.testHooks.AfterAutoExitFinalAliveCheck()
				}
				x.externalMu.Lock()
				checkJobs := x.snapshotCheckJobsLocked()
				commands := x.snapshotCommandsLocked()
				queuesActive := x.microtaskYield.Load() || !x.microtaskQueuesEmpty() || x.ownerInternalCount.Load() > 0 || x.ownerExternalCount.Load() > 0 || x.activePhaseJobCount.Load() > 0
				x.externalMu.Unlock()
				checkJobs = append(checkJobs, x.snapshotOwnerCheckJobs()...)
				if queuesActive || x.hasLiveCheckJob(checkJobs) || x.hasLiveCommand(commands) {
					x.quiescing.Store(false)
					continue
				}
				endTerminalDrain, ok := x.commitAutoExitTerminalDrain(ctx.Done())
				if !ok {
					continue
				}
				x.transitionToTerminatedStarted(endTerminalDrain)
				x.terminateCleanup()
				x.closeFDs()
				x.startTerminalCompletion()
				return x.fdResourceCloseError()
			}
		}

		// Use the tight fast-path loop for task-only workloads.
		if x.canUseFastPath() && !x.hasTimersPending() {
			if x.runFastPath(ctx) {
				// Fast path completed or needs mode switch - continue to check termination
				continue
			}
			// Fall through to regular tick for feature transition
		}

		x.tick()
	}
}

// runFastPath is a tight loop for task-only workloads associated with "fast path" mode.
// Returns true if the loop should continue (check termination), false if should use tick.
//
// It uses a blocking channel select and owner-local task batches without the
// regular scheduler's timer and readiness phases.
func (x *Loop) runFastPath(ctx context.Context) bool {
	x.fastPathEntries.Add(1)
	if x.testHooks != nil && x.testHooks.OnFastPathEntry != nil {
		x.testHooks.OnFastPathEntry()
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
		currentState := x.state.Load()
		if currentState == StateTerminated || currentState == StateTerminating {
			return true
		}

		// Auto-exit check: don't block in fast path if loop should exit.
		// The main run loop owns the quiescence handler and gate commit so the
		// handler runs exactly once and observes the pre-quiescing state.
		if x.autoExit && !x.Alive() {
			if x.testHooks != nil && x.testHooks.BeforeFastPathAutoExitReturn != nil {
				x.testHooks.BeforeFastPathAutoExitReturn()
			}
			return true
		}

		if x.fastPathNeedsTick() {
			return false
		}

		if x.fastPathHasReadyWork() {
			x.runAux()
			continue
		}

		select {
		case <-ctx.Done():
			return true

		case <-x.fastWakeupCh:
			// Work is observed at the top of the next turn so auto-exit can still
			// skip unref-only handles instead of executing them merely because a wake
			// was received.
		}
	}
}

func (x *Loop) fastPathNeedsTick() bool {
	if !x.canUseFastPath() {
		return true
	}
	return x.hasTimersPending()
}

func (x *Loop) autoExitReady() bool {
	return x.autoExit && !x.Alive()
}

func (x *Loop) fastPathHasReadyWork() bool {
	if x.microtaskYield.Load() || !x.microtaskQueuesEmpty() || x.ownerInternalCount.Load() > 0 || x.ownerExternalCount.Load() > 0 || x.ownerCheckCount.Load() > 0 || x.ownerCloseCount.Load() > 0 {
		return true
	}

	x.externalMu.Lock()
	hasExternal := x.commands.Len() > 0
	hasPhase := len(x.checkJobs) > 0 || len(x.closeJobs) > 0
	x.externalMu.Unlock()
	return hasExternal || hasPhase
}

func (x *Loop) beginQuiescing() {
	x.quiescingEpoch.Store(x.submissionEpoch.Load())
	x.quiescing.Store(true)
}

func (x *Loop) quiescingRejectsLiveness() bool {
	if !x.quiescing.Load() {
		return false
	}
	state := x.state.Load()
	if state == StateTerminating || state == StateTerminated {
		return true
	}
	if x.submissionEpoch.Load() != x.quiescingEpoch.Load() {
		x.quiescing.Store(false)
		return false
	}
	return true
}

func (x *Loop) commitAutoExitTerminalDrain(contextDone <-chan struct{}) (func(), bool) {
	x.livenessMu.Lock()
	x.externalMu.Lock()

	commands := x.snapshotCommandsLocked()
	checkJobs := x.snapshotCheckJobsLocked()
	queuesActive := x.microtaskYield.Load() || !x.microtaskQueuesEmpty() || x.activePhaseJobCount.Load() > 0
	queuesActive = queuesActive || x.ownerInternalCount.Load() > 0 || x.ownerExternalCount.Load() > 0
	quiescing := x.quiescingRejectsLiveness()
	state := x.state.Load()
	terminalActive := state == StateTerminating || state == StateTerminated || x.terminalDraining.Load()
	x.externalMu.Unlock()
	x.livenessMu.Unlock()
	checkJobs = append(checkJobs, x.snapshotOwnerCheckJobs()...)

	// Dynamic check/immediate liveness predicates are user code. Evaluate them
	// outside admission and liveness locks; predicates are allowed to call back
	// into loop APIs such as ScheduleTimer or Submit without deadlocking the
	// auto-exit terminal admission path.
	if terminalActive || !quiescing || queuesActive || x.hasLiveCheckJob(checkJobs) || x.hasLiveCommand(commands) {
		// Every failed commit leaves the loop running. Lower the provisional
		// admission gate before returning to its next quiescence callback or
		// accepting an ordinary liveness-adding operation.
		x.quiescing.Store(false)
		return nil, false
	}

	x.livenessMu.Lock()
	x.externalMu.Lock()

	queuesActive = x.microtaskYield.Load() || !x.microtaskQueuesEmpty() || x.activePhaseJobCount.Load() > 0
	queuesActive = queuesActive || x.ownerInternalCount.Load() > 0 || x.ownerExternalCount.Load() > 0
	state = x.state.Load()
	terminalActive = state == StateTerminating || state == StateTerminated || x.terminalDraining.Load()
	if terminalActive || !x.quiescingRejectsLiveness() || queuesActive {
		x.quiescing.Store(false)
		x.externalMu.Unlock()
		x.livenessMu.Unlock()
		return nil, false
	}

	if x.testHooks != nil && x.testHooks.BeforeAutoExitTerminalDrainCommit != nil {
		x.testHooks.BeforeAutoExitTerminalDrainCommit()
	}
	// This is the context/auto-exit precedence cut. Cancellation already
	// observable while final admission is locked wins; once the terminal drain
	// commits, clean auto-exit wins over a later cancellation.
	select {
	case <-contextDone:
		x.quiescing.Store(false)
		x.externalMu.Unlock()
		x.livenessMu.Unlock()
		return nil, false
	default:
	}

	endTerminalDrain := x.beginAutoExitTerminalDrain()
	x.externalMu.Unlock()
	x.livenessMu.Unlock()
	return endTerminalDrain, true
}

func (x *Loop) rejectLivenessAdd() error {
	x.livenessMu.Lock()
	defer x.livenessMu.Unlock()
	return x.rejectLivenessAddLocked()
}

func (x *Loop) rejectLivenessAddLocked() error {
	state := x.state.Load()
	if state == StateTerminating || state == StateTerminated {
		return ErrLoopTerminated
	}
	if x.terminalDraining.Load() {
		return ErrLoopTerminated
	}
	if x.quiescingRejectsLiveness() {
		return ErrLoopTerminated
	}
	return nil
}

func (x *Loop) rejectFDMutationLocked(allowTerminating bool) error {
	state := x.state.Load()
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
func (x *Loop) Close() error {
	return x.closeImpl(true)
}

func (x *Loop) closeImpl(join bool) error {
	if join && (x.isLoopThread() || x.isTerminalDrainOwner()) {
		return ErrReentrantClose
	}
	nonjoining := !join || x.isPromisifyWorker()
	if x.terminalCompletionPublished() {
		x.retryPollerCleanup()
		return ErrLoopTerminated
	}
	if x.isTerminalCompletionOwner() {
		return x.terminalRequestResult(true)
	}

	for {
		currentState := x.state.Load()
		if currentState == StateTerminating || currentState == StateTerminated {
			if nonjoining {
				return x.terminalRequestResult(true)
			}
			x.beforeTerminalJoin()
			<-x.terminalDone
			return x.terminalError()
		}
		if x.testHooks != nil && x.testHooks.BeforeCloseLifecycleLock != nil {
			x.testHooks.BeforeCloseLifecycleLock()
		}

		x.livenessMu.Lock()
		currentState = x.state.Load()
		if currentState == StateTerminating || currentState == StateTerminated {
			x.livenessMu.Unlock()
			if nonjoining {
				return x.terminalRequestResult(true)
			}
			x.beforeTerminalJoin()
			<-x.terminalDone
			return x.terminalError()
		}
		x.terminalDrainMu.Lock()
		x.callbackGateMu.Lock()
		if x.state.TryTransition(currentState, StateTerminating) {
			if x.testHooks != nil && x.testHooks.TerminalStateCAS != nil {
				x.testHooks.TerminalStateCAS()
			}
			x.immediateClose.Store(true)
			x.callbackGateMode = callbackGateClosed
			x.callbackGateMu.Unlock()
			x.terminalDrainMu.Unlock()
			if x.testHooks != nil && x.testHooks.AfterCloseStateTerminating != nil {
				x.testHooks.AfterCloseStateTerminating()
			}
			waitLoop := currentState != StateAwake
			// Publish the immediate terminal state before releasing lifecycle
			// admission. A Run owner that already committed StateRunning is joined
			// even if it has not yet published runStarted.
			x.state.Store(StateTerminated)
			x.livenessMu.Unlock()
			// Release callbacks already admitted by the loop owner before waiting for
			// that owner to exit. Such a callback may be blocked on a Promisify
			// promise whose user function outlives Close; delaying rejection until
			// after loopDone would make each side wait for the other.
			if x.testHooks != nil && x.testHooks.BeforeClosePromiseRejection != nil {
				x.testHooks.BeforeClosePromiseRejection()
			}
			x.rejectAllPendingPromises(ErrLoopTerminated)
			x.startTerminalDependencyRelease()
			if waitLoop {
				x.forceWakeup()
			}
			x.startImmediateTerminalCompletion(waitLoop)
			if nonjoining {
				return nil
			}
			<-x.terminalDone
			return x.terminalError()
		}
		x.callbackGateMu.Unlock()
		x.terminalDrainMu.Unlock()
		x.livenessMu.Unlock()
	}
}
