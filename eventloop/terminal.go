package eventloop

import (
	"runtime"
	"sync"
	"weak"

	"github.com/joeycumines/goroutineid"
)

func (l *Loop) transitionToTerminatedForShutdown() {
	l.livenessMu.Lock()
	endTerminalDrain := l.beginTerminalDrainAllChecks()
	l.livenessMu.Unlock()
	l.transitionToTerminatedStartedForShutdown(endTerminalDrain)
}

func (l *Loop) transitionToTerminatedStarted(endTerminalDrain func()) {
	l.transitionToTerminatedStartedWithReject(endTerminalDrain, true)
}

func (l *Loop) transitionToTerminatedStartedForShutdown(endTerminalDrain func()) {
	l.transitionToTerminatedStartedWithReject(endTerminalDrain, false)
}

func (l *Loop) transitionToTerminatedStartedWithReject(endTerminalDrain func(), rejectPending bool) {
	l.waitTerminalDependencyRelease()
	if l.testHooks != nil && l.testHooks.BeforeTerminateState != nil {
		l.testHooks.BeforeTerminateState()
	}

	// Lock promisifyMu to prevent new Promisify operations while we shut down
	l.livenessMu.Lock()
	l.promisifyMu.Lock()
	l.state.Store(StateTerminated)
	l.promisifyMu.Unlock()
	l.livenessMu.Unlock()
	if l.testHooks != nil && l.testHooks.AfterTerminateStateBeforeDrain != nil {
		l.testHooks.AfterTerminateStateBeforeDrain()
	}

	// Drain loop queues with normal microtask checkpoints.
	// Tasks that are already queued will get executed.
	// Tasks submitted after this point will be rejected.
	l.drainTerminalQueuesStarted()
	endTerminalDrain()

	if rejectPending {
		// Reject all remaining pending promises.
		l.rejectAllPendingPromises(ErrLoopTerminated)
	}
}

func (l *Loop) rejectAllPendingPromises(err error) {
	l.rejectAllOnce.Do(func() {
		l.registry.RejectAll(err)
	})
}

func (l *Loop) closeTerminalDone() {
	l.closeTerminalOnce.Do(func() {
		close(l.terminalDone)
		if l.testHooks != nil && l.testHooks.AfterTerminalDoneClose != nil {
			l.testHooks.AfterTerminalDoneClose()
		}
	})
}

func (l *Loop) waitPromisifyGoroutines() {
	l.promisifyMu.Lock()
	//lint:ignore SA2001 intentional memory barrier
	l.promisifyMu.Unlock()
	l.promisifyWg.Wait()
}

func (l *Loop) beginTerminalDrain() func() {
	return l.beginTerminalDrainMode(false, false)
}

func (l *Loop) beginAutoExitTerminalDrain() func() {
	return l.beginTerminalDrainMode(false, true)
}

func (l *Loop) beginTerminalDrainAllChecks() func() {
	return l.beginTerminalDrainMode(true, false)
}

func (l *Loop) beginTerminalDrainMode(allChecks, skipChecks bool) func() {
	done := make(chan struct{})
	var once sync.Once
	l.terminalDrainMu.Lock()
	if l.terminalDraining.Load() && l.terminalDrainDone != nil {
		if allChecks {
			l.terminalDrainAllChecks.Store(true)
			l.terminalDrainSkipChecks.Store(false)
		}
		l.terminalDrainMu.Unlock()
		return func() {}
	}
	l.terminalDrainDone = done
	l.terminalDrainOwner.Store(goroutineid.Get())
	l.terminalDrainAllChecks.Store(allChecks)
	l.terminalDrainSkipChecks.Store(skipChecks)
	l.terminalDraining.Store(true)
	l.terminalDrainMu.Unlock()
	return func() { once.Do(func() { l.finishTerminalDrain(done) }) }
}

func (l *Loop) tryBeginTerminalDrainTransition(from, to LoopState) (func(), bool) {
	return l.tryBeginTerminalDrainTransitionMode(from, to, true)
}

func (l *Loop) tryBeginTerminalDrainRequest(from, to LoopState) (func(), bool) {
	return l.tryBeginTerminalDrainTransitionOwner(from, to, true, 0)
}

func (l *Loop) tryBeginTerminalDrainTransitionMode(from, to LoopState, allChecks bool) (func(), bool) {
	return l.tryBeginTerminalDrainTransitionOwner(from, to, allChecks, goroutineid.Get())
}

func (l *Loop) tryBeginTerminalDrainTransitionOwner(from, to LoopState, allChecks bool, owner int64) (func(), bool) {
	done := make(chan struct{})
	var once sync.Once
	l.livenessMu.Lock()
	l.terminalDrainMu.Lock()
	if l.terminalDraining.Load() && l.terminalDrainDone != nil {
		activeDone := l.terminalDrainDone
		if l.state.TryTransition(from, to) {
			if to == StateTerminating && l.testHooks != nil && l.testHooks.TerminalStateCAS != nil {
				l.testHooks.TerminalStateCAS()
			}
			if allChecks {
				l.terminalDrainAllChecks.Store(true)
				l.terminalDrainSkipChecks.Store(false)
			}
			l.terminalDrainMu.Unlock()
			l.livenessMu.Unlock()
			return func() { once.Do(func() { l.finishTerminalDrain(activeDone) }) }, true
		}
		l.terminalDrainMu.Unlock()
		l.livenessMu.Unlock()
		return nil, false
	}
	if l.state.TryTransition(from, to) {
		if to == StateTerminating && l.testHooks != nil && l.testHooks.TerminalStateCAS != nil {
			l.testHooks.TerminalStateCAS()
		}
		l.terminalDrainDone = done
		l.terminalDrainOwner.Store(owner)
		l.terminalDrainAllChecks.Store(allChecks)
		l.terminalDrainSkipChecks.Store(false)
		l.terminalDraining.Store(true)
		l.terminalDrainMu.Unlock()
		l.livenessMu.Unlock()
		return func() { once.Do(func() { l.finishTerminalDrain(done) }) }, true
	}
	l.terminalDrainMu.Unlock()
	l.livenessMu.Unlock()
	return nil, false
}

func (l *Loop) finishTerminalDrain(done chan struct{}) {
	if done == nil {
		return
	}
	for {
		diagnostics, shouldClose := l.takeTerminalDiagnosticsOrFinish(done)
		if len(diagnostics) == 0 {
			if shouldClose {
				close(done)
			}
			return
		}
		for i, diagnostic := range diagnostics {
			if diagnostic != nil {
				l.safeExecuteFallback(diagnostic)
			}
			diagnostics[i] = nil
		}
		l.drainTerminalQueuesStarted()
	}
}

func (l *Loop) scheduleTerminalDiagnostic(fn func()) bool {
	if fn == nil {
		return true
	}
	l.terminalDrainMu.Lock()
	if !l.terminalDraining.Load() || l.terminalDrainDone == nil {
		l.terminalDrainMu.Unlock()
		return false
	}
	l.terminalDiagnostics = append(l.terminalDiagnostics, fn)
	l.terminalDrainMu.Unlock()
	return true
}

func (l *Loop) takeTerminalDiagnosticsOrFinish(done chan struct{}) ([]func(), bool) {
	l.terminalDrainMu.Lock()
	defer l.terminalDrainMu.Unlock()
	if l.terminalDrainDone != done {
		return nil, false
	}
	if len(l.terminalDiagnostics) == 0 {
		if l.testHooks != nil && l.testHooks.BeforeTerminalDrainFinish != nil {
			l.testHooks.BeforeTerminalDrainFinish()
		}
		l.finishTerminalDrainLocked(done)
		return nil, true
	}
	diagnostics := l.terminalDiagnostics
	l.terminalDiagnostics = nil
	return diagnostics, false
}

func (l *Loop) finishTerminalDrainLocked(done chan struct{}) {
	if l.terminalDrainDone == done {
		l.terminalDraining.Store(false)
		l.terminalDrainAllChecks.Store(false)
		l.terminalDrainSkipChecks.Store(false)
		l.terminalDrainOwner.Store(0)
		l.terminalDrainDone = nil
	}
}

func (l *Loop) terminalDrainWaiter() (<-chan struct{}, bool) {
	l.terminalDrainMu.Lock()
	defer l.terminalDrainMu.Unlock()
	if !l.terminalDraining.Load() || l.terminalDrainDone == nil {
		return nil, false
	}
	return l.terminalDrainDone, true
}

func (l *Loop) claimTerminalDrainOwner() {
	if l.terminalDraining.Load() {
		l.terminalDrainOwner.Store(goroutineid.Get())
	}
}

func (l *Loop) finishActiveTerminalDrain() {
	l.terminalDrainMu.Lock()
	done := l.terminalDrainDone
	l.terminalDrainMu.Unlock()
	if done != nil {
		l.finishTerminalDrain(done)
	}
}

func (l *Loop) isTerminalDrainOwner() bool {
	// Goroutine ownership is intentionally implicit but bounded by the active
	// terminal-drain window. A matching goroutine id grants no authority unless
	// terminalDraining is still true, and finishTerminalDrain clears both fields.
	if !l.terminalDraining.Load() {
		return false
	}
	owner := l.terminalDrainOwner.Load()
	return owner != 0 && owner == goroutineid.Get()
}

func (l *Loop) claimTerminalCompletionOwner() func() {
	owner := goroutineid.Get()
	l.terminalCompletionOwner.Store(owner)
	return func() {
		l.terminalCompletionOwner.CompareAndSwap(owner, 0)
	}
}

func (l *Loop) isTerminalCompletionOwner() bool {
	owner := l.terminalCompletionOwner.Load()
	return owner != 0 && owner == goroutineid.Get()
}

func (l *Loop) startTerminalDependencyRelease() {
	l.terminalDependencyOnce.Do(func() {
		go l.releaseTerminalDependencies()
	})
}

func (l *Loop) waitTerminalDependencyRelease() {
	l.startTerminalDependencyRelease()
	<-l.terminalDependencyDone
}

func (l *Loop) releaseTerminalDependencies() {
	defer close(l.terminalDependencyDone)
	releaseCompletionOwner := l.claimTerminalCompletionOwner()
	defer releaseCompletionOwner()

	var reactions []pendingPromiseReaction
	if l.immediateClose.Load() {
		reactions = l.takePendingPromiseReactions()
	}
	adapters, settlements := l.takeJSTerminalDependencies()
	l.externalMu.Lock()
	l.releaseTerminalCommandDependenciesLocked()
	l.externalMu.Unlock()

	failPendingPromiseReactions(reactions)
	for _, js := range adapters {
		js.recoverSettledAdoptions()
	}
	for _, settle := range settlements {
		settle()
	}
}

func (l *Loop) takeJSTerminalDependencies() ([]*JS, []func()) {
	l.livenessMu.Lock()
	adapters := make([]*JS, 0, len(l.jsAdapters))
	var settlements []func()
	for pointer := range l.jsAdapters {
		js := pointer.Value()
		if js == nil {
			delete(l.jsAdapters, pointer)
			continue
		}
		adapters = append(adapters, js)
		settlements = append(settlements, js.takeTimerPromiseSettlements()...)
	}
	l.livenessMu.Unlock()
	return adapters, settlements
}

func (l *Loop) releaseTerminalCommandDependenciesLocked() {
	if l.commands == nil {
		return
	}
	for index := l.commands.head; index < len(l.commands.cmds); index++ {
		cmd := &l.commands.cmds[index]
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

func (l *Loop) terminalDrainActive() bool {
	l.terminalDrainMu.Lock()
	active := l.terminalDraining.Load()
	l.terminalDrainMu.Unlock()
	return active
}

func (l *Loop) terminalEphemeralAllowed(state LoopState) bool {
	if state != StateTerminating && state != StateTerminated {
		if !l.terminalDraining.Load() {
			return true
		}
		return l.isTerminalDrainOwner() || l.isLoopThread()
	}

	if l.terminalDraining.Load() {
		return l.isTerminalDrainOwner() || l.isLoopThread()
	}

	// A lifecycle transition publishes StateTerminating while holding
	// terminalDrainMu and then publishes terminalDraining before unlocking. A
	// lock-free reader can observe the state store before the drain store, so
	// synchronize here before rejecting terminal continuation work.
	if l.testHooks != nil && l.testHooks.BeforeTerminalEphemeralDrainSync != nil {
		l.testHooks.BeforeTerminalEphemeralDrainSync()
	}
	l.terminalDrainMu.Lock()
	draining := l.terminalDraining.Load()
	l.terminalDrainMu.Unlock()
	if draining {
		return l.isTerminalDrainOwner() || l.isLoopThread()
	}
	return false
}

func (l *Loop) terminalMicrotaskAllowed(state LoopState) bool {
	return l.terminalEphemeralAllowed(state)
}

func (l *Loop) terminalQueueAllowed(state LoopState) bool {
	return state != StateTerminating && state != StateTerminated && !l.terminalDraining.Load()
}

func (l *Loop) hardAbortRequested() bool {
	state := l.state.Load()
	if state == StateTerminating {
		return l.immediateCloseWon()
	}
	return state == StateTerminated && !l.terminalDraining.Load()
}

func (l *Loop) immediateCloseWon() bool {
	state := l.state.Load()
	if state != StateTerminating && state != StateTerminated {
		return false
	}
	// Close publishes StateTerminating while holding terminalDrainMu, then
	// publishes immediateClose before unlocking. Synchronize readers that see
	// the state store before the mode store.
	if l.testHooks != nil && l.testHooks.BeforeTerminalModeLock != nil {
		l.testHooks.BeforeTerminalModeLock()
	}
	l.terminalDrainMu.Lock()
	immediate := l.immediateClose.Load()
	l.terminalDrainMu.Unlock()
	return immediate
}

func (l *Loop) cleanupCommandIngressLocked() {
	for {
		cmd, ok := l.commands.Pop()
		if !ok {
			break
		}
		if cmd.kind == loopCommandTimerAdd && cmd.timer != nil {
			resetTimerForPool(cmd.timer)
			timerPool.Put(cmd.timer)
		}
	}
	l.commandIngressPending.Store(false)
}

// drainStartupQueues commits work that was accepted before Run entered its
// first event-loop iteration. Timer registrations, cancellations, and ref/unref
// operations use the internal queue when scheduled before Run; draining that
// queue before the first check phase makes top-level JavaScript timer setup
// visible before setImmediate callbacks, matching Node's startup boundary. Due
// ref'ed timers may then run before the first check phase if their 1ms threshold
// elapsed while the main script was executing or before Run was called.
func (l *Loop) drainStartupQueues() {
	l.drainCommandIngress()
	l.recordQueueMetrics()
	l.processInternalQueue()
	if l.hardAbortRequested() {
		return
	}
	l.drainMicrotasks()
	if l.hardAbortRequested() {
		return
	}
	if l.autoExit && !l.Alive() {
		return
	}
	l.refreshTickTime()
	l.runTimers()
	if l.hardAbortRequested() {
		return
	}
	l.drainMicrotasks()
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
func (l *Loop) terminateCleanup() {
	l.waitTerminalDependencyRelease()
	// A graceful drain normally executes every accepted reaction. Any residue is
	// work that cleanup is about to discard, so publish its terminal outcome
	// before resetting owner or ingress queues.
	failPendingPromiseReactions(l.takePendingPromiseReactions())
	// Cleanup settlements may use the owner callback boundary. Register this
	// retirement first so the unlock-and-settle defer runs before it, including
	// while unwinding an abnormal settlement exit.
	defer l.stopCallbackWorker()

	var (
		jsTerminalCleanup func()
		jsSettlements     []func()
	)
	l.livenessMu.Lock()
	defer func() {
		l.livenessMu.Unlock()
		l.safeExecuteFallback(jsTerminalCleanup)
		for _, settle := range jsSettlements {
			l.safeExecuteFallback(settle)
		}
	}()

	// Clear quiescing flag: the termination decision is complete, so the gate
	// is no longer needed. This maintains the invariant that quiescing is only
	// true during the brief window between !Alive() and terminal-drain commit.
	// While benign in practice (StateTerminated is checked first in all gated APIs),
	// clearing the flag prevents stale state if the code is refactored.
	l.quiescing.Store(false)
	l.releaseMicrotaskYield()

	if l.testHooks != nil && l.testHooks.BeforeJSTerminalCleanupCollect != nil {
		l.testHooks.BeforeJSTerminalCleanupCollect()
	}
	jsTerminalCleanup = l.jsTerminalCleanup
	l.jsTerminalCleanup = nil
	jsSettlements = l.cleanupJSAdaptersLocked()
	if l.testHooks != nil && l.testHooks.AfterJSTerminalSettlementCollect != nil {
		l.testHooks.AfterJSTerminalSettlementCollect()
	}
	l.cleanupTimers()
	l.stopFastSleepTimer()
	l.fastSleepTimer = nil
	l.userIOFDCount.Store(0)
	l.activePhaseJobCount.Store(0)
	l.discardOwnerQueues()
	l.externalMu.Lock()
	l.cleanupCommandIngressLocked()
	l.commands.discard()
	l.checkJobs = discardSlice(l.checkJobs)
	l.checkJobsSpare = discardSlice(l.checkJobsSpare)
	l.closeJobs = discardSlice(l.closeJobs)
	l.closeJobsSpare = discardSlice(l.closeJobsSpare)
	l.externalMu.Unlock()
	l.queuePressureHandler = nil
	l.quiescenceMu.Lock()
	l.quiescenceHandler = nil
	l.jsQuiescenceHandler = nil
	l.quiescenceMu.Unlock()

	l.terminalDrainMu.Lock()
	for i := range l.terminalDiagnostics {
		l.terminalDiagnostics[i] = nil
	}
	l.terminalDiagnostics = nil
	l.terminalDrainMu.Unlock()

}

// registerJSAdapter links adapter-owned handle registries to terminal cleanup.
// livenessMu linearizes registration with every terminal transition and with
// cleanupJSAdaptersLocked. Weak keys avoid retaining otherwise unreachable
// adapters for the lifetime of a long-running loop.
func (l *Loop) registerJSAdapter(js *JS) {
	if l == nil || js == nil || l.state == nil {
		return
	}
	l.livenessMu.Lock()
	state := l.state.Load()
	var registration jsAdapterRegistration
	registered := false
	if state != StateTerminating && state != StateTerminated {
		registration = l.retainJSAdapterLocked(js)
		registered = true
	}
	l.livenessMu.Unlock()
	if registered {
		runtime.AddCleanup(js, cleanupJSAdapterRegistration, registration)
		runtime.KeepAlive(js)
	}
}

// bindJSAdapter gives install exclusive lifecycle ownership, then atomically
// registers js and its independent integration quiescence callback while the
// loop remains in StateAwake. It returns the state observed under livenessMu.
func (l *Loop) bindJSAdapter(js *JS, quiescence func() bool, terminalCleanup func(), install func(*JS) error) (LoopState, error) {
	if l == nil || js == nil || l.state == nil {
		return StateTerminated, ErrJSBindState
	}
	if l.testHooks != nil && l.testHooks.BeforeBindJSLifecycleLock != nil {
		l.testHooks.BeforeBindJSLifecycleLock()
	}
	l.livenessMu.Lock()
	defer l.livenessMu.Unlock()
	l.quiescenceMu.Lock()
	bound := l.jsQuiescenceBound
	l.quiescenceMu.Unlock()
	if bound {
		return l.state.Load(), ErrJSBindConflict
	}
	state := l.state.Load()
	if state != StateAwake {
		return state, ErrJSBindState
	}
	if install != nil {
		if err := install(js); err != nil {
			return state, err
		}
	}
	l.quiescenceMu.Lock()
	registration := l.retainJSAdapterLocked(js)
	l.jsQuiescenceHandler = quiescence
	l.jsQuiescenceBound = true
	l.jsTerminalCleanup = terminalCleanup
	l.quiescenceMu.Unlock()
	runtime.AddCleanup(js, cleanupJSAdapterRegistration, registration)
	runtime.KeepAlive(js)
	return state, nil
}

// retainJSAdapterLocked records js while livenessMu is held.
func (l *Loop) retainJSAdapterLocked(js *JS) jsAdapterRegistration {
	if len(l.jsAdapters) >= l.jsAdapterSweepAt {
		l.sweepJSAdaptersLocked()
	}
	pointer := weak.Make(js)
	l.jsAdapters = retainedMapStore(l.jsAdapters, &l.jsAdaptersRetention, pointer, struct{}{})
	return jsAdapterRegistration{
		loop:    weak.Make(l),
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

func (l *Loop) sweepJSAdaptersLocked() {
	for pointer := range l.jsAdapters {
		if pointer.Value() == nil {
			delete(l.jsAdapters, pointer)
		}
	}
	l.jsAdapters, _ = rebuildRetainedMap(l.jsAdapters, &l.jsAdaptersRetention)
	l.jsAdapterSweepAt = nextJSAdapterSweep(len(l.jsAdapters))
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
func (l *Loop) cleanupJSAdaptersLocked() []func() {
	var settlements []func()
	for pointer := range l.jsAdapters {
		if js := pointer.Value(); js != nil {
			settlements = append(settlements, js.terminateCleanup()...)
		}
	}
	l.jsAdapters = discardRetainedMap(l.jsAdapters, &l.jsAdaptersRetention)
	l.jsAdapterSweepAt = 0
	return settlements
}

// closeFDs closes file descriptors.
// Uses sync.Once to ensure FDs are only closed once,
// even if called from multiple paths (shutdown + poll error).
func (l *Loop) closeFDs() {
	// A synchronous descriptor-cleanup diagnostic retains its caller's logical
	// loop role. If that diagnostic causes terminal fallback work, retire the
	// owner callback worker only after the complete logger call returns.
	defer l.stopCallbackWorker()

	closedNow := false
	l.closeOnce.Do(func() {
		closedNow = true
		if l.testHooks != nil && l.testHooks.BeforeCloseFDLock != nil {
			l.testHooks.BeforeCloseFDLock()
		}
		l.fdMu.Lock()
		l.livenessMu.Lock()
		if l.testHooks != nil && l.testHooks.BeforeWakeResourceClose != nil {
			l.testHooks.BeforeWakeResourceClose()
		}
		l.wakeMu.Lock()

		wakePipe := l.wakePipe
		wakePipeWrite := l.wakePipeWrite
		l.wakePipe = -1
		l.wakePipeWrite = -1
		l.pollerReady.Store(false)
		closeErr := l.closePollerLocked()
		l.userIOFDCount.Store(0)
		closeErr = joinErrors(closeErr, closeWakeFDs(wakePipe, wakePipeWrite))
		l.wakeUpSignalPending.Store(wakeSignalIdle)
		l.wakeMu.Unlock()
		l.livenessMu.Unlock()
		l.fdMu.Unlock()
		if closeErr != nil {
			l.fdCloseErr.Store(&terminalErrorBox{err: closeErr})
			l.logError("eventloop: descriptor cleanup failed", closeErr)
		}
	})
	if !closedNow {
		l.retryPollerCleanup()
	}
}

// isLoopThread reports whether the caller currently owns logical loop access.
// During callback execution, that ownership may be delegated from the physical
// Run goroutine to the callback worker.
func (l *Loop) isLoopThread() bool {
	loopID := l.loopGoroutineID.Load()
	if loopID == 0 {
		return false
	}
	return goroutineid.Get() == loopID
}

// IsCallbackOwner reports whether the calling goroutine currently holds the
// loop's logical callback-owner role — that is, whether it may execute work
// against loop-owned state directly instead of scheduling it via [Loop.Submit]
// (or [Loop.SubmitInternal]).
//
// The callback-owner role belongs to whichever goroutine the loop's
// authoritative ownership marker currently names. While the loop is active that
// is the goroutine running [Loop.Run]; for the duration of host callback work
// the marker is transferred — together with any delegated diagnostic or
// isolation roles — to the goroutine executing that work, most visibly the
// loop's isolated callback worker. Physical goroutine identity is therefore NOT
// a stable contract: this method inspects the marker itself. A host adapter
// that multiplexes re-entrant Goja work (a gRPC handler, invoked on the
// callback worker, that calls back into a public runtime method) MUST use
// IsCallbackOwner — rather than its own goroutine-id snapshot — to decide
// whether to run synchronously: the snapshot would be stale inside a callback
// and cause a Submit that deadlocks waiting for the loop the callback is
// already blocking.
//
// IsCallbackOwner returns false when the loop has never run (no owner marker is
// set), so callers can use it to distinguish "execute directly during setup,
// before Run" from "running inside the loop". It is safe to call from any
// goroutine, including one that has never interacted with the loop. A nil
// receiver returns false.
func (l *Loop) IsCallbackOwner() bool {
	if l == nil {
		return false
	}
	return l.isLoopThread()
}

func (l *Loop) waitLoopDoneAfterTerminal() {
	if l.runStarted.Load() {
		<-l.loopDone
	}
}
