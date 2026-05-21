package eventloop

import (
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"
	"weak"
)

// trackRejection tracks a rejected promise for unhandled rejection detection.
// This is called from the reject() method.
//
// This implementation records the rejection before the Rejected state is
// published, then schedules runRejectionCheck at the end of a microtask
// checkpoint. Handlers registered by later microtasks in the same checkpoint set
// the promise-local rejectionHandled bit synchronously and are observed without
// any wall-clock sleep.
//
// creationStack is read from the JS.debugStacks side table.
func (js *JS) trackRejection(p *ChainedPromise, reason any) {
	js.recordRejection(p, reason)
	js.scheduleRejectionCheck(p)
}

func (js *JS) recordRejection(p *ChainedPromise, reason any) {
	// Read creation stack from side table (keyed by weak.Pointer).
	wp := weak.Make(p)
	js.debugStacksMu.Lock()
	creationStack := js.debugStacks[wp]
	js.debugStacksMu.Unlock()

	// Store rejection info
	info := &rejectionInfo{
		reason:        reason,
		promise:       p,
		creationStack: creationStack, // Store for debug output
		timestamp:     time.Now().UnixNano(),
	}
	// A rejection created after immediate Close wins requires terminal fallback
	// diagnostics even if it is absorbed by a normal checker that was already
	// running on the loop owner.
	if js.loop != nil && (js.loop.immediateClose.Load() ||
		(js.loop.state.Load() == StateTerminated && !js.loop.terminalDraining.Load())) {
		info.terminalFallback.Store(true)
	}
	js.rejectionsMu.Lock()
	js.unhandledRejections[p] = info
	js.rejectionsMu.Unlock()
}

func (js *JS) scheduleRejectionCheck(p *ChainedPromise) {
	handlerReady := make(chan struct{})

	// Try to store the channel so then() can signal when handler is registered
	// Use atomic compare-and-swap to avoid races with concurrent rejections
	js.handlerReadyMu.Lock()
	// Check if another rejection already stored a channel
	if _, exists := js.handlerReadyChans[p]; !exists {
		// No channel yet, store ours
		js.handlerReadyChans[p] = handlerReady
	}
	js.handlerReadyMu.Unlock()

	// CRITICAL FIX: Use atomic counter to prevent duplicate microtasks.
	// checkUnhandledRejections checks all unhandled rejections, so we only need
	// ONE scheduled check running at a time, not one per rejection.
	if !js.claimRejectionCheckSchedule() {
		// Another check is already scheduled. If the loop has already terminated,
		// that scheduled checkpoint may have been discarded by terminateCleanup, so
		// run the deterministic diagnostic fallback synchronously instead of
		// leaving checkRejectionScheduled stuck true forever.
		if js.loop.terminalDraining.Load() {
			js.runUnhandledRejectionCheckAfterTerminalDrain()
		} else if js.loop.state.Load() == StateTerminated {
			js.runUnhandledRejectionFallback()
		}
		return
	}
	// Schedule the check at the end of the next microtask checkpoint. If
	// scheduling fails (for example, because the loop has terminated), run the
	// diagnostic path through a deterministic fallback instead of orphaning
	// unhandledRejections forever. During terminal drain, defer that fallback until
	// the active drain window closes so same-checkpoint terminal microtasks can
	// still attach handlers before diagnostics inspect the snapshot.
	if err := js.loop.scheduleMicrotaskCheckpoint(js.runRejectionCheck); err != nil {
		if js.loop.terminalDraining.Load() {
			js.runUnhandledRejectionCheckAfterTerminalDrain()
			return
		}
		// Rejection after termination is an expected handoff to the configured
		// terminal fallback, not a scheduler failure. The fallback owns the one
		// user-facing diagnostic for disabled callback delivery.
		js.runUnhandledRejectionFallback()
		return
	}
}

func (js *JS) runRejectionCheck() {
	js.runRejectionCheckWith(js.runRejectionCheck, js.loop.safeExecuteFn, false)
}

func (js *JS) runRejectionCheckFallback() {
	js.runRejectionCheckWith(js.runRejectionCheckFallback, js.loop.safeExecuteFallback, true)
}

func (js *JS) runRejectionCheckWith(runCheck func(), executeCallback func(func()), terminalFallback bool) {
	js.consumeHandlerReadySignals()

	// ALWAYS run checkUnhandledRejections to catch ALL pending unhandled
	// rejections, not just the promise that won the scheduler CAS. Without this,
	// concurrent rejections where one has a handler and another does not could
	// result in the unhandled one never being reported.
	js.runUnhandledRejectionChecks(runCheck, executeCallback, terminalFallback)
}

func (js *JS) consumeHandlerReadySignals() {
	js.handlerReadyMu.Lock()
	for p, ch := range js.handlerReadyChans {
		select {
		case <-ch:
			delete(js.handlerReadyChans, p)
		default:
		}
	}
	js.handlerReadyMu.Unlock()
}

func (js *JS) hasReportableUnhandledRejections() bool {
	js.rejectionsMu.RLock()
	defer js.rejectionsMu.RUnlock()

	for p, info := range js.unhandledRejections {
		terminal := js.loop != nil && js.loop.state.Load() == StateTerminated
		if promiseState(p.state.Load()) != Pending && !info.propagated.Load() && (terminal || info.propagationPending.Load() == 0) {
			return true
		}
	}
	return false
}

func (js *JS) runUnhandledRejectionFallback() {
	js.markUnhandledRejectionFallback()
	owned, done := js.beginUnhandledRejectionCheck(true)
	if !owned {
		if js.loop.isTerminalDrainOwner() {
			<-done
		}
		return
	}
	if js.unhandledFallback == UnhandledRejectionFallbackDisabled {
		// The user requested loop-affine handlers only. Drain the diagnostic
		// bookkeeping synchronously so Shutdown/terminal-drain callers do not return
		// before already-determined disabled fallback reports and clears tracking.
		js.loop.safeExecuteFallback(js.dropUnhandledRejectionFallbackOwned)
		return
	}

	// Terminal-drain diagnostics are already owned by the drain goroutine;
	// execute them synchronously so Shutdown cannot return before an
	// already-determined diagnostic is settled. Late post-termination diagnostics
	// use an isolated goroutine for the historical fallback contract and to avoid
	// running user callbacks on the rejecting caller goroutine.
	if js.loop.isTerminalDrainOwner() {
		js.loop.safeExecuteFallback(func() {
			js.runUnhandledRejectionChecksOwned(js.runRejectionCheckFallback, js.loop.safeExecuteFallback, true)
		})
		return
	}
	go js.loop.safeExecuteFallback(func() {
		js.runUnhandledRejectionChecksOwned(js.runRejectionCheckFallback, js.loop.safeExecuteFallback, true)
	})
}

func (js *JS) markUnhandledRejectionFallback() {
	js.rejectionsMu.RLock()
	for _, info := range js.unhandledRejections {
		info.terminalFallback.Store(true)
	}
	js.rejectionsMu.RUnlock()
}

func (js *JS) dropUnhandledRejectionFallbackOwned() {
	panicked := true
	defer func() {
		if panicked {
			js.abortUnhandledRejectionCheck(js.runRejectionCheckFallback)
		}
	}()

	for {
		js.consumeHandlerReadySignals()
		js.logAndClearUnhandledRejections()
		js.pruneOrphanedHandlerReadyChans()
		js.checkRejectionScheduled.Store(false)

		switch js.finishUnhandledRejectionIteration(true) {
		case rejectionCheckStop:
			js.releaseRejectionCheck()
			panicked = false
			return
		case rejectionCheckRerun:
		case rejectionCheckFallbackHandoff:
			panic("eventloop: disabled rejection fallback requested impossible handoff")
		}
	}
}

func (js *JS) pruneOrphanedHandlerReadyChans() {
	js.rejectionsMu.RLock()
	js.handlerReadyMu.Lock()
	for p := range js.handlerReadyChans {
		if _, ok := js.unhandledRejections[p]; !ok {
			delete(js.handlerReadyChans, p)
		}
	}
	js.handlerReadyMu.Unlock()
	js.rejectionsMu.RUnlock()
}

func (js *JS) logAndClearUnhandledRejections() {
	js.rejectionsMu.RLock()
	if len(js.unhandledRejections) == 0 {
		js.rejectionsMu.RUnlock()
		return
	}
	snapshot := make([]*rejectionInfo, 0, len(js.unhandledRejections))
	for _, info := range js.unhandledRejections {
		snapshot = append(snapshot, info)
	}
	js.rejectionsMu.RUnlock()

	for _, info := range snapshot {
		p := info.promise
		if js.loop != nil && js.loop.testHooks != nil && js.loop.testHooks.BeforeUnhandledRejectionRecordCheck != nil {
			js.loop.testHooks.BeforeUnhandledRejectionRecordCheck(p)
		}
		if promiseState(p.state.Load()) == Pending {
			continue
		}

		handled := p.rejectionHandled.Load() || info.propagated.Load()

		js.rejectionsMu.Lock()
		currentInfo, exists := js.unhandledRejections[p]
		handled = handled || p.rejectionHandled.Load() || info.propagated.Load()
		if info.fallbackReportCommitted.Load() {
			// A normal checker already committed this diagnostic, but callback
			// admission closed before it started. A late handler may suppress a
			// derived promise, but it cannot revoke the source report now owned by
			// terminal fallback.
			handled = false
		}
		if !exists || currentInfo != info {
			js.rejectionsMu.Unlock()
			continue
		}
		if !handled {
			p.rejectionHandled.markReported()
		}
		delete(js.unhandledRejections, p)
		js.rejectionsMu.Unlock()
		js.handlerReadyMu.Lock()
		delete(js.handlerReadyChans, p)
		js.handlerReadyMu.Unlock()
		if !handled && js.loop != nil {
			js.loop.logErrorValue("eventloop: unhandled rejection after loop termination (fallback callback disabled)", "reason", info.reason)
		}
	}
}

func (js *JS) runUnhandledRejectionCheckAfterTerminalDrain() {
	done, active := js.loop.terminalDrainWaiter()
	if !active {
		if js.checkRejectionScheduled.Load() || js.hasReportableUnhandledRejections() {
			js.runUnhandledRejectionFallback()
		}
		return
	}

	js.checkRejectionTerminalMu.Lock()
	if js.checkRejectionTerminalDone == done {
		js.checkRejectionTerminalMu.Unlock()
		return
	}
	js.checkRejectionTerminalDone = done
	js.checkRejectionTerminalWatchers.Add(1)
	js.checkRejectionTerminalMu.Unlock()

	registered := js.loop.scheduleTerminalDiagnostic(func() {
		js.checkRejectionTerminalMu.Lock()
		if js.checkRejectionTerminalDone == done {
			js.checkRejectionTerminalDone = nil
		}
		js.checkRejectionTerminalMu.Unlock()
		if js.checkRejectionScheduled.Load() || js.hasReportableUnhandledRejections() {
			js.runUnhandledRejectionFallback()
		}
	})
	if registered {
		return
	}

	js.checkRejectionTerminalMu.Lock()
	if js.checkRejectionTerminalDone == done {
		js.checkRejectionTerminalDone = nil
	}
	js.checkRejectionTerminalMu.Unlock()
	if js.checkRejectionScheduled.Load() || js.hasReportableUnhandledRejections() {
		js.runUnhandledRejectionFallback()
	}
}

func (js *JS) runUnhandledRejectionChecks(runCheck func(), executeCallback func(func()), terminalFallback bool) {
	owned, done := js.beginUnhandledRejectionCheck(terminalFallback)
	if !owned {
		if terminalFallback && js.loop.isTerminalDrainOwner() {
			<-done
		}
		return
	}
	js.runUnhandledRejectionChecksOwned(runCheck, executeCallback, terminalFallback)
}

func (js *JS) runUnhandledRejectionChecksOwned(runCheck func(), executeCallback func(func()), terminalFallback bool) {
	panicked := true
	defer func() {
		if panicked {
			js.abortUnhandledRejectionCheck(runCheck)
		}
	}()

	for {
		if terminalFallback {
			js.markUnhandledRejectionFallback()
		}
		js.checkUnhandledRejectionsWith(executeCallback, terminalFallback)
		js.pruneOrphanedHandlerReadyChans()
		js.checkRejectionScheduled.Store(false)
		if js.loop.testHooks != nil && js.loop.testHooks.AfterUnhandledRejectionCheckClear != nil {
			js.loop.testHooks.AfterUnhandledRejectionCheckClear()
		}

		switch js.finishUnhandledRejectionIteration(terminalFallback) {
		case rejectionCheckRerun:
			continue
		case rejectionCheckFallbackHandoff:
			js.handoffUnhandledRejectionFallback()
			panicked = false
			return
		case rejectionCheckStop:
			js.rescheduleUnhandledRejectionCheck(runCheck)
			js.releaseRejectionCheck()
			panicked = false
			return
		}
	}
}

type rejectionCheckAction uint8

const (
	rejectionCheckStop rejectionCheckAction = iota
	rejectionCheckRerun
	rejectionCheckFallbackHandoff
)

func (js *JS) beginUnhandledRejectionCheck(terminalFallback bool) (bool, <-chan struct{}) {
	if terminalFallback && js.checkRejectionRunning.Load() && js.loop.testHooks != nil && js.loop.testHooks.BeforeUnhandledRejectionRerunRequest != nil {
		js.loop.testHooks.BeforeUnhandledRejectionRerunRequest()
	}
	js.checkRejectionRunMu.Lock()
	if js.checkRejectionRunning.Load() {
		if terminalFallback {
			js.checkRejectionFallbackRerun.Store(true)
		}
		js.checkRejectionRerun.Store(true)
		done := js.checkRejectionRunDone
		js.checkRejectionRunMu.Unlock()
		if terminalFallback && js.loop.testHooks != nil && js.loop.testHooks.AfterUnhandledRejectionFallbackRerun != nil {
			js.loop.testHooks.AfterUnhandledRejectionFallbackRerun()
		}
		return false, done
	}
	js.ensureRejectionCheckRetainedLocked()
	js.checkRejectionRunning.Store(true)
	done := js.checkRejectionRunDone
	js.checkRejectionRunMu.Unlock()
	return true, done
}

func (js *JS) finishUnhandledRejectionIteration(terminalFallback bool) rejectionCheckAction {
	js.checkRejectionRunMu.Lock()
	rerun := js.checkRejectionRerun.Swap(false)
	fallback := js.checkRejectionFallbackRerun.Swap(false)
	if fallback && !terminalFallback {
		js.checkRejectionRunMu.Unlock()
		return rejectionCheckFallbackHandoff
	}
	if rerun || fallback {
		js.checkRejectionRunMu.Unlock()
		return rejectionCheckRerun
	}
	// Keep the completion token open across any reschedule or terminal-fallback
	// handoff. The final quiescent owner releases strong retention and closes the
	// token only after both scheduled and running state are false.
	js.checkRejectionRunning.Store(false)
	js.checkRejectionRunMu.Unlock()
	return rejectionCheckStop
}

func (js *JS) abortUnhandledRejectionCheck(runCheck func()) {
	js.checkRejectionRunMu.Lock()
	js.checkRejectionScheduled.Store(false)
	js.checkRejectionRerun.Store(false)
	js.checkRejectionFallbackRerun.Store(false)
	js.checkRejectionRunning.Store(false)
	js.checkRejectionRunMu.Unlock()

	// An abnormal callback exit can unwind the checker after another goroutine
	// recorded a rejection but observed this generation's scheduled flag. Keep
	// the shared completion token and strong adapter owner across the transition,
	// then re-drive any residual record using the current normal or fallback
	// runner. releaseRejectionCheck closes the token only if no replacement
	// generation claimed it.
	js.rescheduleUnhandledRejectionCheck(runCheck)
	js.releaseRejectionCheck()
}

func (js *JS) handoffUnhandledRejectionFallback() {
	go js.loop.safeExecuteFallback(func() {
		if js.unhandledFallback == UnhandledRejectionFallbackDisabled {
			js.dropUnhandledRejectionFallbackOwned()
			return
		}
		js.runUnhandledRejectionChecksOwned(js.runRejectionCheckFallback, js.loop.safeExecuteFallback, true)
	})
}

func (js *JS) rescheduleUnhandledRejectionCheck(runCheck func()) {
	if !js.hasReportableUnhandledRejections() || !js.claimRejectionCheckSchedule() {
		return
	}
	if err := js.loop.scheduleMicrotaskCheckpoint(runCheck); err != nil {
		if js.loop.terminalDraining.Load() {
			js.runUnhandledRejectionCheckAfterTerminalDrain()
			return
		}
		if !errors.Is(err, ErrLoopTerminated) {
			js.loop.logError("eventloop: failed to reschedule unhandled rejection check microtask", err)
		}
		js.runUnhandledRejectionFallback()
		return
	}
}

func (js *JS) claimRejectionCheckSchedule() bool {
	if js.checkRejectionScheduled.Load() {
		return false
	}
	if js.loop.testHooks != nil && js.loop.testHooks.BeforeRejectionCheckScheduleClaim != nil {
		js.loop.testHooks.BeforeRejectionCheckScheduleClaim()
	}
	js.checkRejectionRunMu.Lock()
	if !js.checkRejectionScheduled.CompareAndSwap(false, true) {
		js.checkRejectionRunMu.Unlock()
		return false
	}
	js.ensureRejectionCheckRetainedLocked()
	js.checkRejectionRunMu.Unlock()
	return true
}

// ensureRejectionCheckRetainedLocked keeps an adapter reachable while a
// scheduled or running rejection check can still be discarded by terminal
// queue cleanup. The caller holds checkRejectionRunMu, establishing the global
// runMu -> rejectionCheckMu lock order and making the completion token cover
// every reschedule and fallback generation.
func (js *JS) ensureRejectionCheckRetainedLocked() {
	if js.checkRejectionRunDone == nil {
		js.checkRejectionRunDone = make(chan struct{})
	}
	js.loop.retainRejectionCheckAdapter(js)
}

func (l *Loop) retainRejectionCheckAdapter(js *JS) {
	l.rejectionCheckMu.Lock()
	if l.rejectionCheckAdapter == js {
		l.rejectionCheckMu.Unlock()
		return
	}
	if _, retained := l.rejectionCheckAdapters[js]; retained {
		l.rejectionCheckMu.Unlock()
		return
	}
	if l.rejectionCheckAdapter == nil {
		l.rejectionCheckAdapter = js
		l.rejectionCheckMu.Unlock()
		return
	}
	if l.rejectionCheckAdapters == nil {
		l.rejectionCheckAdapters = make(map[*JS]struct{})
	}
	l.rejectionCheckAdapters[js] = struct{}{}
	l.rejectionCheckMu.Unlock()
}

func (js *JS) releaseRejectionCheck() {
	js.checkRejectionRunMu.Lock()
	js.releaseRejectionCheckLocked()
	js.checkRejectionRunMu.Unlock()
}

func (js *JS) releaseRejectionCheckLocked() {
	if js.checkRejectionScheduled.Load() || js.checkRejectionRunning.Load() {
		return
	}
	js.loop.releaseRejectionCheckAdapter(js)
	if js.checkRejectionRunDone != nil {
		close(js.checkRejectionRunDone)
		js.checkRejectionRunDone = nil
	}
}

func (l *Loop) releaseRejectionCheckAdapter(js *JS) {
	l.rejectionCheckMu.Lock()
	delete(l.rejectionCheckAdapters, js)
	if l.rejectionCheckAdapter == js {
		l.rejectionCheckAdapter = nil
		for adapter := range l.rejectionCheckAdapters {
			l.rejectionCheckAdapter = adapter
			delete(l.rejectionCheckAdapters, adapter)
			break
		}
	}
	if len(l.rejectionCheckAdapters) == 0 {
		l.rejectionCheckAdapters = nil
	}
	l.rejectionCheckMu.Unlock()
}

// checkUnhandledRejections checks for rejections without handlers and reports them.
func (js *JS) checkUnhandledRejections() {
	js.checkUnhandledRejectionsWith(js.loop.safeExecuteFn, false)
}

func (js *JS) checkUnhandledRejectionsWith(executeCallback func(func()), allowTerminalFallback bool) {
	// Get the unhandled rejection callback if any
	js.mu.Lock()
	callback := js.unhandledCallback
	js.mu.Unlock()

	// Collect snapshot of rejections to iterate safely
	js.rejectionsMu.RLock()
	// Early exit
	if len(js.unhandledRejections) == 0 {
		js.rejectionsMu.RUnlock()
		return
	}

	snapshot := make([]*rejectionInfo, 0, len(js.unhandledRejections))
	for _, info := range js.unhandledRejections {
		snapshot = append(snapshot, info)
	}
	js.rejectionsMu.RUnlock()

	// Process snapshot
	for _, info := range snapshot {
		p := info.promise
		if js.loop != nil && js.loop.testHooks != nil && js.loop.testHooks.BeforeUnhandledRejectionRecordCheck != nil {
			js.loop.testHooks.BeforeUnhandledRejectionRecordCheck(p)
		}
		if promiseState(p.state.Load()) == Pending {
			continue
		}

		handled := p.rejectionHandled.Load() || info.propagated.Load()

		terminalFallback := info.terminalFallback.Load()
		if terminalFallback && !allowTerminalFallback {
			continue
		}
		if !allowTerminalFallback && info.propagationPending.Load() > 0 {
			continue
		}

		// Linearize late handler/propagation publication against the reporting
		// decision. Both publication paths take rejectionsMu before changing the
		// record, so the checker either observes them or owns the report first.
		js.rejectionsMu.Lock()
		currentInfo, exists := js.unhandledRejections[p]
		handled = handled || p.rejectionHandled.Load() || info.propagated.Load()
		if info.fallbackReportCommitted.Load() {
			handled = false
		}
		if !exists || currentInfo != info {
			js.rejectionsMu.Unlock()
			continue
		}
		if !allowTerminalFallback && info.propagationPending.Load() > 0 {
			js.rejectionsMu.Unlock()
			continue
		}
		if !handled {
			p.rejectionHandled.markReported()
		}
		delete(js.unhandledRejections, p)
		js.rejectionsMu.Unlock()

		if handled {
			// Remove from auxiliary tracking but do not report it.
			js.handlerReadyMu.Lock()
			delete(js.handlerReadyChans, p)
			js.handlerReadyMu.Unlock()
			continue
		}

		cleanupAuxiliary := func() {
			js.handlerReadyMu.Lock()
			delete(js.handlerReadyChans, p)
			js.handlerReadyMu.Unlock()
		}
		callbackReturned := false
		defer func() {
			if !callbackReturned {
				cleanupAuxiliary()
			}
		}()

		// No handler found - report unhandled rejection
		if callback != nil && !(terminalFallback && js.unhandledFallback == UnhandledRejectionFallbackDisabled) {
			// If debug mode captured a creation stack, wrap the reason
			// with debug info so the callback can access where the promise was created.
			reason := info.reason
			if len(info.creationStack) > 0 {
				stackTrace := formatCreationStack(info.creationStack)
				reason = &UnhandledRejectionDebugInfo{
					Reason:             info.reason,
					CreationStackTrace: stackTrace,
				}
			}
			if js.loop.testHooks != nil && js.loop.testHooks.BeforeUnhandledRejectionCallback != nil {
				js.loop.testHooks.BeforeUnhandledRejectionCallback()
			}
			callbackStarted := false
			invokeCallback := func() {
				callbackStarted = true
				callback(reason)
			}
			if terminalFallback {
				js.loop.safeExecuteFallback(invokeCallback)
			} else {
				executeCallback(invokeCallback)
				if !callbackStarted {
					// Immediate Close closed normal callback admission after this
					// checker selected the record. Preserve it for an isolated or
					// disabled terminal-fallback handoff instead of silently deleting
					// an unexecuted diagnostic.
					info.terminalFallback.Store(true)
					info.fallbackReportCommitted.Store(true)
					js.rejectionsMu.Lock()
					if _, exists := js.unhandledRejections[p]; !exists {
						js.unhandledRejections[p] = info
					}
					js.rejectionsMu.Unlock()
					js.checkRejectionFallbackRerun.Store(true)
					callbackReturned = true
					continue
				}
			}
		} else if terminalFallback && js.unhandledFallback == UnhandledRejectionFallbackDisabled {
			js.loop.logErrorValue("eventloop: unhandled rejection after loop termination (fallback callback disabled)", "reason", info.reason)
		}

		callbackReturned = true
		cleanupAuxiliary()

		if !js.loop.primaryMicrotaskQueuesEmpty() {
			return
		}
	}
}

// rejectionInfo holds information about a rejected promise.
type rejectionInfo struct {
	promise            *ChainedPromise // 8B pointer
	reason             any             // 16B interface (two pointers)
	creationStack      []uintptr       // 24B slice (data pointer + len + cap)
	timestamp          int64           // 8B non-pointer
	propagationPending atomic.Int32
	propagated         atomic.Bool
	terminalFallback   atomic.Bool
	// fallbackReportCommitted means a normal checker selected the diagnostic,
	// but callback admission closed before delivery. Terminal fallback must
	// complete that report even if a late handler marks the source handled.
	fallbackReportCommitted atomic.Bool
}

// UnhandledRejectionDebugInfo is passed to [RejectionHandler] when debug mode is enabled
// and the promise has a creation stack trace.
//
// This type wraps the rejection reason and includes debug information
// about where the promise was created. This helps answer "where did this promise
// come from?" when debugging unhandled rejections.
//
// Users can type-assert the reason in their [RejectionHandler] callback to access
// the debug information:
//
//	js, err := eventloop.NewJS(loop, eventloop.WithUnhandledRejection(func(r any) {
//	    if debug, ok := r.(*eventloop.UnhandledRejectionDebugInfo); ok {
//	        log.Printf("Unhandled rejection: %v\\nCreated at:\\n%s",
//	            debug.Reason, debug.CreationStackTrace)
//	    } else {
//	        log.Printf("Unhandled rejection: %v", r)
//	    }
//	}))
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// If debug mode is not enabled or the promise has no creation stack,
// the callback receives the raw rejection reason without wrapping.
type UnhandledRejectionDebugInfo struct {
	// Reason is the original rejection value from the failed promise.
	Reason any

	// CreationStackTrace is a formatted stack trace showing where the promise
	// was created. Each frame is on its own line in the format:
	//   package.function (file:line)
	CreationStackTrace string
}

// Error implements the error interface so UnhandledRejectionDebugInfo can be
// used as an error value when the underlying Reason is also an error.
func (u *UnhandledRejectionDebugInfo) Error() string {
	if u == nil {
		return "<nil>"
	}
	if err := nonNilError(u.Reason); err != nil {
		return err.Error()
	}
	return fmt.Sprintf("%v", u.Reason)
}

// Unwrap returns the underlying error if Reason is an error type.
// This enables [errors.Is] and [errors.As] to work through the wrapper.
func (u *UnhandledRejectionDebugInfo) Unwrap() error {
	if u == nil {
		return nil
	}
	return nonNilError(u.Reason)
}

// formatCreationStack formats a slice of program counters as a stack trace string.
// Used by checkUnhandledRejections to format creation stack for debug output.
func formatCreationStack(pcs []uintptr) string {
	if len(pcs) == 0 {
		return ""
	}

	frames := runtime.CallersFrames(pcs)
	var result string
	for {
		frame, more := frames.Next()
		if frame.Function != "" {
			if result != "" {
				result += "\n"
			}
			result += fmt.Sprintf("%s (%s:%d)", frame.Function, frame.File, frame.Line)
		}
		if !more {
			break
		}
	}
	return result
}

// ============================================================================
