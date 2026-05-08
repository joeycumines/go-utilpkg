package eventloop

import (
	"runtime"
	"sync"
	"weak"

	"github.com/joeycumines/goroutineid"
)

func (x *Loop) transitionToTerminatedForShutdown() {
	x.livenessMu.Lock()
	endTerminalDrain := x.beginTerminalDrainAllChecks()
	x.livenessMu.Unlock()
	x.transitionToTerminatedStartedForShutdown(endTerminalDrain)
}

func (x *Loop) transitionToTerminatedStarted(endTerminalDrain func()) {
	x.transitionToTerminatedStartedWithReject(endTerminalDrain, true)
}

func (x *Loop) transitionToTerminatedStartedForShutdown(endTerminalDrain func()) {
	x.transitionToTerminatedStartedWithReject(endTerminalDrain, false)
}

func (x *Loop) transitionToTerminatedStartedWithReject(endTerminalDrain func(), rejectPending bool) {
	x.waitTerminalDependencyRelease()
	if x.testHooks != nil && x.testHooks.BeforeTerminateState != nil {
		x.testHooks.BeforeTerminateState()
	}

	// Lock promisifyMu to prevent new Promisify operations while we shut down
	x.livenessMu.Lock()
	x.promisifyMu.Lock()
	x.state.Store(StateTerminated)
	x.promisifyMu.Unlock()
	x.livenessMu.Unlock()
	if x.testHooks != nil && x.testHooks.AfterTerminateStateBeforeDrain != nil {
		x.testHooks.AfterTerminateStateBeforeDrain()
	}

	// Drain loop queues with normal microtask checkpoints.
	// Tasks that are already queued will get executed.
	// Tasks submitted after this point will be rejected.
	x.drainTerminalQueuesStarted()
	endTerminalDrain()

	if rejectPending {
		// Reject all remaining pending promises.
		x.rejectAllPendingPromises(ErrLoopTerminated)
	}
}

func (x *Loop) rejectAllPendingPromises(err error) {
	x.rejectAllOnce.Do(func() {
		x.registry.RejectAll(err)
	})
}

func (x *Loop) closeTerminalDone() {
	x.closeTerminalOnce.Do(func() {
		close(x.terminalDone)
		if x.testHooks != nil && x.testHooks.AfterTerminalDoneClose != nil {
			x.testHooks.AfterTerminalDoneClose()
		}
	})
}

func (x *Loop) waitPromisifyGoroutines() {
	x.promisifyMu.Lock()
	//lint:ignore SA2001 intentional memory barrier
	x.promisifyMu.Unlock()
	x.promisifyWg.Wait()
}

func (x *Loop) beginTerminalDrain() func() {
	return x.beginTerminalDrainMode(false, false)
}

func (x *Loop) beginAutoExitTerminalDrain() func() {
	return x.beginTerminalDrainMode(false, true)
}

func (x *Loop) beginTerminalDrainAllChecks() func() {
	return x.beginTerminalDrainMode(true, false)
}

func (x *Loop) beginTerminalDrainMode(allChecks, skipChecks bool) func() {
	done := make(chan struct{})
	var once sync.Once
	x.terminalDrainMu.Lock()
	if x.terminalDraining.Load() && x.terminalDrainDone != nil {
		if allChecks {
			x.terminalDrainAllChecks.Store(true)
			x.terminalDrainSkipChecks.Store(false)
		}
		x.terminalDrainMu.Unlock()
		return func() {}
	}
	x.terminalDrainDone = done
	x.terminalDrainOwner.Store(goroutineid.Get())
	x.terminalDrainAllChecks.Store(allChecks)
	x.terminalDrainSkipChecks.Store(skipChecks)
	x.terminalDraining.Store(true)
	x.terminalDrainMu.Unlock()
	return func() { once.Do(func() { x.finishTerminalDrain(done) }) }
}

func (x *Loop) tryBeginTerminalDrainTransition(from, to LoopState) (func(), bool) {
	return x.tryBeginTerminalDrainTransitionMode(from, to, true)
}

func (x *Loop) tryBeginTerminalDrainRequest(from, to LoopState) (func(), bool) {
	return x.tryBeginTerminalDrainTransitionOwner(from, to, true, 0)
}

func (x *Loop) tryBeginTerminalDrainTransitionMode(from, to LoopState, allChecks bool) (func(), bool) {
	return x.tryBeginTerminalDrainTransitionOwner(from, to, allChecks, goroutineid.Get())
}

func (x *Loop) tryBeginTerminalDrainTransitionOwner(from, to LoopState, allChecks bool, owner int64) (func(), bool) {
	done := make(chan struct{})
	var once sync.Once
	x.livenessMu.Lock()
	x.terminalDrainMu.Lock()
	if x.terminalDraining.Load() && x.terminalDrainDone != nil {
		activeDone := x.terminalDrainDone
		if x.state.TryTransition(from, to) {
			if to == StateTerminating && x.testHooks != nil && x.testHooks.TerminalStateCAS != nil {
				x.testHooks.TerminalStateCAS()
			}
			if allChecks {
				x.terminalDrainAllChecks.Store(true)
				x.terminalDrainSkipChecks.Store(false)
			}
			x.terminalDrainMu.Unlock()
			x.livenessMu.Unlock()
			return func() { once.Do(func() { x.finishTerminalDrain(activeDone) }) }, true
		}
		x.terminalDrainMu.Unlock()
		x.livenessMu.Unlock()
		return nil, false
	}
	if x.state.TryTransition(from, to) {
		if to == StateTerminating && x.testHooks != nil && x.testHooks.TerminalStateCAS != nil {
			x.testHooks.TerminalStateCAS()
		}
		x.terminalDrainDone = done
		x.terminalDrainOwner.Store(owner)
		x.terminalDrainAllChecks.Store(allChecks)
		x.terminalDrainSkipChecks.Store(false)
		x.terminalDraining.Store(true)
		x.terminalDrainMu.Unlock()
		x.livenessMu.Unlock()
		return func() { once.Do(func() { x.finishTerminalDrain(done) }) }, true
	}
	x.terminalDrainMu.Unlock()
	x.livenessMu.Unlock()
	return nil, false
}

func (x *Loop) finishTerminalDrain(done chan struct{}) {
	if done == nil {
		return
	}
	for {
		diagnostics, shouldClose := x.takeTerminalDiagnosticsOrFinish(done)
		if len(diagnostics) == 0 {
			if shouldClose {
				close(done)
			}
			return
		}
		for i, diagnostic := range diagnostics {
			if diagnostic != nil {
				x.safeExecuteFallback(diagnostic)
			}
			diagnostics[i] = nil
		}
		x.drainTerminalQueuesStarted()
	}
}

func (x *Loop) scheduleTerminalDiagnostic(fn func()) bool {
	if fn == nil {
		return true
	}
	x.terminalDrainMu.Lock()
	if !x.terminalDraining.Load() || x.terminalDrainDone == nil {
		x.terminalDrainMu.Unlock()
		return false
	}
	x.terminalDiagnostics = append(x.terminalDiagnostics, fn)
	x.terminalDrainMu.Unlock()
	return true
}

func (x *Loop) takeTerminalDiagnosticsOrFinish(done chan struct{}) ([]func(), bool) {
	x.terminalDrainMu.Lock()
	defer x.terminalDrainMu.Unlock()
	if x.terminalDrainDone != done {
		return nil, false
	}
	if len(x.terminalDiagnostics) == 0 {
		if x.testHooks != nil && x.testHooks.BeforeTerminalDrainFinish != nil {
			x.testHooks.BeforeTerminalDrainFinish()
		}
		x.finishTerminalDrainLocked(done)
		return nil, true
	}
	diagnostics := x.terminalDiagnostics
	x.terminalDiagnostics = nil
	return diagnostics, false
}

func (x *Loop) finishTerminalDrainLocked(done chan struct{}) {
	if x.terminalDrainDone == done {
		x.terminalDraining.Store(false)
		x.terminalDrainAllChecks.Store(false)
		x.terminalDrainSkipChecks.Store(false)
		x.terminalDrainOwner.Store(0)
		x.terminalDrainDone = nil
	}
}

func (x *Loop) terminalDrainWaiter() (<-chan struct{}, bool) {
	x.terminalDrainMu.Lock()
	defer x.terminalDrainMu.Unlock()
	if !x.terminalDraining.Load() || x.terminalDrainDone == nil {
		return nil, false
	}
	return x.terminalDrainDone, true
}

func (x *Loop) claimTerminalDrainOwner() {
	if x.terminalDraining.Load() {
		x.terminalDrainOwner.Store(goroutineid.Get())
	}
}

func (x *Loop) finishActiveTerminalDrain() {
	x.terminalDrainMu.Lock()
	done := x.terminalDrainDone
	x.terminalDrainMu.Unlock()
	if done != nil {
		x.finishTerminalDrain(done)
	}
}

func (x *Loop) isTerminalDrainOwner() bool {
	// Goroutine ownership is intentionally implicit but bounded by the active
	// terminal-drain window. A matching goroutine id grants no authority unless
	// terminalDraining is still true, and finishTerminalDrain clears both fields.
	if !x.terminalDraining.Load() {
		return false
	}
	owner := x.terminalDrainOwner.Load()
	return owner != 0 && owner == goroutineid.Get()
}

func (x *Loop) claimTerminalCompletionOwner() func() {
	owner := goroutineid.Get()
	x.terminalCompletionOwner.Store(owner)
	return func() {
		x.terminalCompletionOwner.CompareAndSwap(owner, 0)
	}
}

func (x *Loop) isTerminalCompletionOwner() bool {
	owner := x.terminalCompletionOwner.Load()
	return owner != 0 && owner == goroutineid.Get()
}

func (x *Loop) startTerminalDependencyRelease() {
	x.terminalDependencyOnce.Do(func() {
		go x.releaseTerminalDependencies()
	})
}

func (x *Loop) waitTerminalDependencyRelease() {
	x.startTerminalDependencyRelease()
	<-x.terminalDependencyDone
}

func (x *Loop) releaseTerminalDependencies() {
	defer close(x.terminalDependencyDone)
	releaseCompletionOwner := x.claimTerminalCompletionOwner()
	defer releaseCompletionOwner()

	var reactions []pendingPromiseReaction
	if x.immediateClose.Load() {
		reactions = x.takePendingPromiseReactions()
	}
	adapters, settlements := x.takeJSTerminalDependencies()
	x.externalMu.Lock()
	x.releaseTerminalCommandDependenciesLocked()
	x.externalMu.Unlock()

	failPendingPromiseReactions(reactions)
	for _, js := range adapters {
		js.recoverSettledAdoptions()
	}
	for _, settle := range settlements {
		settle()
	}
}

func (x *Loop) takeJSTerminalDependencies() ([]*JS, []func()) {
	x.livenessMu.Lock()
	adapters := make([]*JS, 0, len(x.jsAdapters))
	var settlements []func()
	for pointer := range x.jsAdapters {
		js := pointer.Value()
		if js == nil {
			delete(x.jsAdapters, pointer)
			continue
		}
		adapters = append(adapters, js)
		settlements = append(settlements, js.takeTimerPromiseSettlements()...)
	}
	x.livenessMu.Unlock()
	return adapters, settlements
}

func (x *Loop) releaseTerminalCommandDependenciesLocked() {
	if x.commands == nil {
		return
	}
	for index := x.commands.head; index < len(x.commands.cmds); index++ {
		cmd := &x.commands.cmds[index]
		switch cmd.kind {
		case loopCommandTimerRef, loopCommandTimerUnref:
			if cmd.result != nil {
				// Admission linearized before terminal transition. No later timer phase
				// can observe this ref state, so nil remains the exact public result.
				// Discard the unapplied command before waking its waiter: leaving it
				// queued could override a later owner-local mutation after the waiter
				// resumes. Result-free LoopRequests remain queued for graceful drain.
				cmd.result <- nil
				cmd.result = nil
				cmd.kind = loopCommandNone
				cmd.token = 0
			}
		case loopCommandTimerCancelBatch:
			if cmd.results != nil {
				errors := make([]error, len(cmd.ids))
				for index := range errors {
					errors[index] = ErrLoopTerminated
				}
				cmd.results <- errors
				cmd.results = nil
			}
		case loopCommandTimerCancel:
			if cmd.result != nil {
				cmd.result <- ErrLoopTerminated
				cmd.result = nil
			}
		}
	}
}

func (x *Loop) terminalDrainActive() bool {
	x.terminalDrainMu.Lock()
	active := x.terminalDraining.Load()
	x.terminalDrainMu.Unlock()
	return active
}

func (x *Loop) terminalEphemeralAllowed(state LoopState) bool {
	if state != StateTerminating && state != StateTerminated {
		if !x.terminalDraining.Load() {
			return true
		}
		return x.isTerminalDrainOwner() || x.isLoopThread()
	}

	if x.terminalDraining.Load() {
		return x.isTerminalDrainOwner() || x.isLoopThread()
	}

	// A lifecycle transition publishes StateTerminating while holding
	// terminalDrainMu and then publishes terminalDraining before unlocking. A
	// lock-free reader can observe the state store before the drain store, so
	// synchronize here before rejecting terminal continuation work.
	if x.testHooks != nil && x.testHooks.BeforeTerminalEphemeralDrainSync != nil {
		x.testHooks.BeforeTerminalEphemeralDrainSync()
	}
	x.terminalDrainMu.Lock()
	draining := x.terminalDraining.Load()
	x.terminalDrainMu.Unlock()
	if draining {
		return x.isTerminalDrainOwner() || x.isLoopThread()
	}
	return false
}

func (x *Loop) terminalMicrotaskAllowed(state LoopState) bool {
	return x.terminalEphemeralAllowed(state)
}

func (x *Loop) terminalQueueAllowed(state LoopState) bool {
	return state != StateTerminating && state != StateTerminated && !x.terminalDraining.Load()
}

func (x *Loop) hardAbortRequested() bool {
	state := x.state.Load()
	if state == StateTerminating {
		return x.immediateCloseWon()
	}
	return state == StateTerminated && !x.terminalDraining.Load()
}

func (x *Loop) immediateCloseWon() bool {
	state := x.state.Load()
	if state != StateTerminating && state != StateTerminated {
		return false
	}
	// Close publishes StateTerminating while holding terminalDrainMu, then
	// publishes immediateClose before unlocking. Synchronize readers that see
	// the state store before the mode store.
	if x.testHooks != nil && x.testHooks.BeforeTerminalModeLock != nil {
		x.testHooks.BeforeTerminalModeLock()
	}
	x.terminalDrainMu.Lock()
	immediate := x.immediateClose.Load()
	x.terminalDrainMu.Unlock()
	return immediate
}

func (x *Loop) cleanupCommandIngressLocked() {
	for {
		cmd, ok := x.commands.Pop()
		if !ok {
			break
		}
		if cmd.kind == loopCommandTimerAdd && cmd.timer != nil {
			resetTimerForPool(cmd.timer)
			timerPool.Put(cmd.timer)
		}
	}
	x.commandIngressPending.Store(false)
}

// drainStartupQueues commits work that was accepted before Run entered its
// first event-loop iteration. Timer registrations, cancellations, and ref/unref
// operations use the internal queue when scheduled before Run; draining that
// queue before the first check phase makes top-level JavaScript timer setup
// visible before setImmediate callbacks, matching Node's startup boundary. Due
// ref'ed timers may then run before the first check phase if their 1ms threshold
// elapsed while the main script was executing or before Run was called.
func (x *Loop) drainStartupQueues() {
	x.drainCommandIngress()
	x.recordQueueMetrics()
	x.processInternalQueue()
	if x.hardAbortRequested() {
		return
	}
	x.drainMicrotasks()
	if x.hardAbortRequested() {
		return
	}
	if x.autoExit && !x.Alive() {
		return
	}
	x.refreshTickTime()
	x.runTimers()
	if x.hardAbortRequested() {
		return
	}
	x.drainMicrotasks()
}

// terminateCleanup clears all remaining loop state after termination.
// It discards timers, resets liveness counters, and drains queues without
// executing callbacks. Must only be called after StateTerminated has been set
// and terminal-drain callbacks have been drained/executed.
// Pre-Run graceful Shutdown waits for promisifyWg before cleanup. Immediate
// Close deliberately rejects registered promises and cleans loop state without
// waiting for user functions. A running graceful termination drains accepted
// queue dependencies and cleans owner-only loop state first, then its independent
// finisher waits for Promisify workers. StateTerminated prevents surviving Close
// workers from adding new queue or resource work. Auto-exit and
// context-cancellation paths call this from the loop goroutine, which is the sole
// consumer of these structures.
func (x *Loop) terminateCleanup() {
	x.waitTerminalDependencyRelease()
	// A graceful drain normally executes every accepted reaction. Any residue is
	// work that cleanup is about to discard, so publish its terminal outcome
	// before resetting owner or ingress queues.
	failPendingPromiseReactions(x.takePendingPromiseReactions())
	// Cleanup settlements may use the owner callback boundary. Register this
	// retirement first so the unlock-and-settle defer runs before it, including
	// while unwinding an abnormal settlement exit.
	defer x.stopCallbackWorker()

	var (
		jsTerminalCleanup func()
		jsSettlements     []func()
	)
	x.livenessMu.Lock()
	defer func() {
		x.livenessMu.Unlock()
		x.safeExecuteFallback(jsTerminalCleanup)
		for _, settle := range jsSettlements {
			x.safeExecuteFallback(settle)
		}
	}()

	// Clear quiescing flag: the termination decision is complete, so the gate
	// is no longer needed. This maintains the invariant that quiescing is only
	// true during the brief window between !Alive() and terminal-drain commit.
	// While benign in practice (StateTerminated is checked first in all gated APIs),
	// clearing the flag prevents stale state if the code is refactored.
	x.quiescing.Store(false)
	x.releaseMicrotaskYield()

	if x.testHooks != nil && x.testHooks.BeforeJSTerminalCleanupCollect != nil {
		x.testHooks.BeforeJSTerminalCleanupCollect()
	}
	jsTerminalCleanup = x.jsTerminalCleanup
	x.jsTerminalCleanup = nil
	jsSettlements = x.cleanupJSAdaptersLocked()
	if x.testHooks != nil && x.testHooks.AfterJSTerminalSettlementCollect != nil {
		x.testHooks.AfterJSTerminalSettlementCollect()
	}
	x.cleanupTimers()
	x.stopFastSleepTimer()
	x.fastSleepTimer = nil
	x.userIOFDCount.Store(0)
	x.activePhaseJobCount.Store(0)
	x.discardOwnerQueues()
	x.externalMu.Lock()
	x.cleanupCommandIngressLocked()
	x.commands.discard()
	x.checkJobs = discardSlice(x.checkJobs)
	x.checkJobsSpare = discardSlice(x.checkJobsSpare)
	x.closeJobs = discardSlice(x.closeJobs)
	x.closeJobsSpare = discardSlice(x.closeJobsSpare)
	x.externalMu.Unlock()
	x.queuePressureHandler = nil
	x.quiescenceMu.Lock()
	x.quiescenceHandler = nil
	x.jsQuiescenceHandler = nil
	x.quiescenceMu.Unlock()

	x.terminalDrainMu.Lock()
	for i := range x.terminalDiagnostics {
		x.terminalDiagnostics[i] = nil
	}
	x.terminalDiagnostics = nil
	x.terminalDrainMu.Unlock()

}

// registerJSAdapter links adapter-owned handle registries to terminal cleanup.
// livenessMu linearizes registration with every terminal transition and with
// cleanupJSAdaptersLocked. Weak keys avoid retaining otherwise unreachable
// adapters for the lifetime of a long-running loop.
func (x *Loop) registerJSAdapter(js *JS) {
	if x == nil || js == nil || x.state == nil {
		return
	}
	x.livenessMu.Lock()
	state := x.state.Load()
	var registration jsAdapterRegistration
	registered := false
	if state != StateTerminating && state != StateTerminated {
		registration = x.retainJSAdapterLocked(js)
		registered = true
	}
	x.livenessMu.Unlock()
	if registered {
		runtime.AddCleanup(js, cleanupJSAdapterRegistration, registration)
		runtime.KeepAlive(js)
	}
}

// bindJSAdapter gives install exclusive lifecycle ownership, then atomically
// registers js and its independent integration quiescence callback while the
// loop remains in StateAwake. It returns the state observed under livenessMu.
func (x *Loop) bindJSAdapter(js *JS, quiescence func() bool, terminalCleanup func(), install func(*JS) error) (LoopState, error) {
	if x == nil || js == nil || x.state == nil {
		return StateTerminated, ErrJSBindState
	}
	if x.testHooks != nil && x.testHooks.BeforeBindJSLifecycleLock != nil {
		x.testHooks.BeforeBindJSLifecycleLock()
	}
	x.livenessMu.Lock()
	defer x.livenessMu.Unlock()
	x.quiescenceMu.Lock()
	bound := x.jsQuiescenceBound
	x.quiescenceMu.Unlock()
	if bound {
		return x.state.Load(), ErrJSBindConflict
	}
	state := x.state.Load()
	if state != StateAwake {
		return state, ErrJSBindState
	}
	if install != nil {
		if err := install(js); err != nil {
			return state, err
		}
	}
	x.quiescenceMu.Lock()
	registration := x.retainJSAdapterLocked(js)
	x.jsQuiescenceHandler = quiescence
	x.jsQuiescenceBound = true
	x.jsTerminalCleanup = terminalCleanup
	x.quiescenceMu.Unlock()
	runtime.AddCleanup(js, cleanupJSAdapterRegistration, registration)
	runtime.KeepAlive(js)
	return state, nil
}

// retainJSAdapterLocked records js while livenessMu is held.
func (x *Loop) retainJSAdapterLocked(js *JS) jsAdapterRegistration {
	if len(x.jsAdapters) >= x.jsAdapterSweepAt {
		x.sweepJSAdaptersLocked()
	}
	pointer := weak.Make(js)
	x.jsAdapters = retainedMapStore(x.jsAdapters, &x.jsAdaptersRetention, pointer, struct{}{})
	return jsAdapterRegistration{
		loop:    weak.Make(x),
		adapter: pointer,
	}
}

type jsAdapterRegistration struct {
	loop    weak.Pointer[Loop]
	adapter weak.Pointer[JS]
}

func cleanupJSAdapterRegistration(registration jsAdapterRegistration) {
	loop := registration.loop.Value()
	if loop == nil {
		return
	}
	if loop.testHooks != nil && loop.testHooks.BeforeJSAdapterCleanup != nil {
		loop.testHooks.BeforeJSAdapterCleanup()
	}
	loop.livenessMu.Lock()
	if loop.testHooks != nil && loop.testHooks.AfterJSAdapterCleanupLock != nil {
		loop.testHooks.AfterJSAdapterCleanupLock()
	}
	loop.jsAdapters, _ = retainedMapDelete(loop.jsAdapters, &loop.jsAdaptersRetention, registration.adapter)
	state := loop.state.Load()
	if state == StateTerminating || state == StateTerminated {
		loop.jsAdapterSweepAt = 0
	} else {
		loop.jsAdapterSweepAt = nextJSAdapterSweep(len(loop.jsAdapters))
	}
	loop.livenessMu.Unlock()
}

func (x *Loop) sweepJSAdaptersLocked() {
	for pointer := range x.jsAdapters {
		if pointer.Value() == nil {
			delete(x.jsAdapters, pointer)
		}
	}
	x.jsAdapters, _ = rebuildRetainedMap(x.jsAdapters, &x.jsAdaptersRetention)
	x.jsAdapterSweepAt = nextJSAdapterSweep(len(x.jsAdapters))
}

func nextJSAdapterSweep(length int) int {
	maxInt := int(^uint(0) >> 1)
	if length > maxInt/2 {
		return maxInt
	}
	return max(retainedRegistryHighWater, length*2)
}

// cleanupJSAdaptersLocked invalidates adapter handles whose loop-owned work is
// being discarded. The caller holds livenessMu, preventing a successful handle
// or timer-promise publication from crossing terminal cleanup in either
// direction. Returned promise settlements run only after that lock is released.
func (x *Loop) cleanupJSAdaptersLocked() []func() {
	var settlements []func()
	for pointer := range x.jsAdapters {
		if js := pointer.Value(); js != nil {
			settlements = append(settlements, js.terminateCleanup()...)
		}
	}
	x.jsAdapters = discardRetainedMap(x.jsAdapters, &x.jsAdaptersRetention)
	x.jsAdapterSweepAt = 0
	return settlements
}

// closeFDs closes file descriptors.
// Uses sync.Once to ensure FDs are only closed once,
// even if called from multiple paths (shutdown + poll error).
func (x *Loop) closeFDs() {
	// A synchronous descriptor-cleanup diagnostic retains its caller's logical
	// loop role. If that diagnostic causes terminal fallback work, retire the
	// owner callback worker only after the complete logger call returns.
	defer x.stopCallbackWorker()

	closedNow := false
	x.closeOnce.Do(func() {
		closedNow = true
		if x.testHooks != nil && x.testHooks.BeforeCloseFDLock != nil {
			x.testHooks.BeforeCloseFDLock()
		}
		x.fdMu.Lock()
		x.livenessMu.Lock()
		if x.testHooks != nil && x.testHooks.BeforeWakeResourceClose != nil {
			x.testHooks.BeforeWakeResourceClose()
		}
		x.wakeMu.Lock()

		wakePipe := x.wakePipe
		wakePipeWrite := x.wakePipeWrite
		x.wakePipe = -1
		x.wakePipeWrite = -1
		x.pollerReady.Store(false)
		closeErr := x.closePollerLocked()
		x.userIOFDCount.Store(0)
		closeErr = joinErrors(closeErr, closeWakeFDs(wakePipe, wakePipeWrite))
		x.wakeUpSignalPending.Store(wakeSignalIdle)
		x.wakeMu.Unlock()
		x.livenessMu.Unlock()
		x.fdMu.Unlock()
		if closeErr != nil {
			x.fdCloseErr.Store(&terminalErrorBox{err: closeErr})
			x.logError("eventloop: descriptor cleanup failed", closeErr)
		}
	})
	if !closedNow {
		x.retryPollerCleanup()
	}
}

// isLoopThread reports whether the caller currently owns logical loop access.
// During callback execution, that ownership may be delegated from the physical
// Run goroutine to the callback worker.
func (x *Loop) isLoopThread() bool {
	loopID := x.loopGoroutineID.Load()
	if loopID == 0 {
		return false
	}
	return goroutineid.Get() == loopID
}

func (x *Loop) waitLoopDoneAfterTerminal() {
	if x.runStarted.Load() {
		<-x.loopDone
	}
}
