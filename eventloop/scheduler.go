package eventloop

import (
	"fmt"
	"time"
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

// runAux performs one non-blocking owner tick for fast-path task-only work.
//
// It achieves low latency by:
//   - using the same phase-snapshot helpers as the normal tick,
//   - skipping only the blocking poll and timer phases while no timers exist,
//   - preserving per-callback microtask checkpoints, and
//   - continuing later fast turns for work admitted beyond the current snapshot.
func (l *Loop) runAux() {
	if l.hardAbortRequested() {
		return
	}
	l.tickCount++
	l.drainCommandIngress()
	l.recordQueueMetrics()
	if l.hasTimersPending() {
		return
	}
	l.tickActive = true
	defer func() { l.tickActive = false }()
	l.refreshTickTime()

	// Fast-path turns must begin with the same nextTick / promise-microtask
	// checkpoint as the normal tick path. Without this, work submitted before Run
	// or while the fast path is blocked can overtake already-pending microtasks.
	l.drainMicrotasks()
	if l.hardAbortRequested() {
		return
	}
	if l.autoExitReady() {
		return
	}
	l.processCheckQueue()
	if l.hardAbortRequested() {
		return
	}

	l.drainMicrotasks()
	if l.hardAbortRequested() {
		return
	}
	if l.autoExitReady() {
		return
	}

	l.processCloseQueue()
	if l.hardAbortRequested() {
		return
	}

	l.drainMicrotasks()
	if l.hardAbortRequested() {
		return
	}
	if l.autoExitReady() {
		return
	}

	l.processInternalQueue()
	if l.hardAbortRequested() {
		return
	}

	l.drainMicrotasks()
	if l.hardAbortRequested() {
		return
	}

	// Drain public Submit work after internal/control work, matching normal tick
	// priority while retaining processExternal's bounded snapshot and pressure
	// signal for producer pressure.
	l.processExternal()
	if l.hardAbortRequested() {
		return
	}

	l.drainMicrotasks()
	if l.hardAbortRequested() {
		return
	}

	// Drain microtasks (safety net — catches any microtasks from the last task's drain)
	l.drainMicrotasks()
}

// hasTimersPending returns true if there are pending timers.
// NOTE: This is only called from the loop goroutine, so no mutex needed.
// tick is a single iteration of the event loop.
func (l *Loop) tick() {
	l.tickCount++
	l.drainCommandIngress()
	l.tickActive = true
	defer func() { l.tickActive = false }()

	l.recordQueueMetrics()

	// Update elapsed monotonic time offset from anchor.
	l.refreshTickTime()

	// Drain microtasks at the start of each tick to catch any that were
	// scheduled during the previous tick's final drain or between ticks.
	l.drainMicrotasks()
	if l.hardAbortRequested() {
		return
	}

	// Node v20+/libuv 1.45+ observes poll before timers in the JS-facing
	// topology; calculateTimeout returns zero for already-due timers, so this
	// poll phase will not sleep past due timer work.
	l.poll()
	if l.hardAbortRequested() {
		return
	}
	if l.autoExitReady() {
		return
	}
	l.refreshTickTime()

	l.drainMicrotasks()
	if l.hardAbortRequested() {
		return
	}
	if l.autoExitReady() {
		return
	}

	l.processCheckQueue()
	if l.hardAbortRequested() {
		return
	}

	l.drainMicrotasks()
	if l.hardAbortRequested() {
		return
	}
	if l.autoExitReady() {
		return
	}

	l.processCloseQueue()
	if l.hardAbortRequested() {
		return
	}

	l.drainMicrotasks()
	if l.hardAbortRequested() {
		return
	}
	if l.autoExitReady() {
		return
	}

	// Node v20+/libuv 1.45+ runs timers only after poll. A check callback queued
	// from a check callback reaches the next check phase before timers that were
	// scheduled from the prior check callback but have not yet reached their
	// minimum threshold.
	if l.testHooks != nil && l.testHooks.BeforeRunTimers != nil {
		l.testHooks.BeforeRunTimers()
	}
	l.runTimers()
	if l.hardAbortRequested() {
		return
	}

	// Inter-phase drain: catch microtasks from timer callbacks before
	// processing the internal queue.
	l.drainMicrotasks()
	if l.hardAbortRequested() {
		return
	}

	l.processInternalQueue()
	if l.hardAbortRequested() {
		return
	}

	// Inter-phase drain: catch microtasks from internal tasks before
	// processing the external queue.
	l.drainMicrotasks()
	if l.hardAbortRequested() {
		return
	}

	l.processExternal()
	if l.hardAbortRequested() {
		return
	}

	// Inter-phase drain: catch microtasks from external tasks.
	l.drainMicrotasks()
	if l.hardAbortRequested() {
		return
	}

	// Safety net drain: catches any microtasks that escaped per-callback draining
	// before registry scavenging or the next tick.
	l.drainMicrotasks()

	// Scavenge registry - limit per tick to avoid stalling
	const registryScavengeLimit = 20
	l.registry.Scavenge(registryScavengeLimit)
}

func (l *Loop) recordQueueMetrics() {
	if l.metrics == nil {
		return
	}
	l.externalMu.Lock()
	// Close the post-drain publication race so the sample represents every
	// command acknowledged before this owner-turn boundary.
	l.drainCommandIngressLocked()
	ingressDepth := int(l.ownerExternalCount.Load()+l.ownerCheckCount.Load()+l.ownerCloseCount.Load()) + len(l.checkJobs) + len(l.closeJobs)
	internalDepth := int(l.ownerInternalCount.Load())
	microtaskDepth := int(l.ownerMicroCount.Load() + l.ingressMicroCount.Load())
	l.externalMu.Unlock()
	l.metrics.recordQueueDepths(
		ingressDepth,
		internalDepth,
		microtaskDepth,
	)
}

// processInternalQueue drains the internal priority queue.
func (l *Loop) processInternalQueue() bool {
	if l.hardAbortRequested() {
		return false
	}
	l.drainCommandIngress()
	processed := false
	ownerSnapshot := l.ownerInternal.Len()
	for range ownerSnapshot {
		task := l.popOwnerInternal()
		if task == nil {
			break
		}
		l.safeExecute(task)
		l.drainMicrotasks()
		processed = true
		if l.hardAbortRequested() {
			return processed
		}
	}
	return processed
}

// processExternal processes the external queue phase snapshot.
func (l *Loop) processExternal() {
	if l.hardAbortRequested() {
		return
	}
	l.drainCommandIngress()
	// Drain the owner queue using a phase snapshot. Concurrent ingress is first
	// materialized by drainCommandIngress and submissions admitted during this
	// phase wait for a later tick.
	ownerSnapshot := l.ownerExternal.Len()

	processed := 0
	for range ownerSnapshot {
		task := l.popOwnerExternal()
		if task == nil {
			break
		}
		processed++
		l.safeExecute(task)
		l.drainMicrotasks()
		if l.hardAbortRequested() {
			break
		}
	}

	if l.hardAbortRequested() {
		return
	}

	if l.testHooks != nil && l.testHooks.BeforeExternalPressureCheck != nil {
		l.testHooks.BeforeExternalPressureCheck()
		if l.hardAbortRequested() {
			return
		}
	}

	l.externalMu.Lock()
	remainingTasks := int(l.ownerExternalCount.Load()) + l.externalCommandCountLocked()
	l.externalMu.Unlock()

	// Emit a pressure signal if work remains beyond this phase snapshot. This is
	// not a numeric callback budget; it indicates producers outpaced the current
	// phase snapshot and callers may want backpressure.
	if processed > 0 && remainingTasks > 0 && l.queuePressureHandler != nil {
		if !l.beginCallbackExecution() {
			return
		}
		outcome := l.executeCallback(l.queuePressureHandler, true)
		l.logCallbackOutcome("queue pressure handler", outcome)
	}
}

func (l *Loop) processCheckQueue() {
	if l.hardAbortRequested() {
		return
	}
	l.drainCommandIngress()
	if l.hardAbortRequested() {
		return
	}
	l.externalMu.Lock()
	batch := l.takeCheckPhaseBatchLocked()
	count := batch.remaining()
	l.startPhaseBatch(count)
	l.externalMu.Unlock()

	if count == 0 {
		l.releaseCheckPhaseBatch(&batch)
		l.releaseMicrotaskYieldAtEmptyCheck()
		return
	}
	defer func() {
		l.releaseCheckPhaseBatch(&batch)
		l.finishPhaseBatch(count)
	}()

	for {
		job, ok := batch.next()
		if !ok {
			break
		}
		l.safeExecute(job.fn)
		l.drainMicrotasks()
		if l.hardAbortRequested() {
			break
		}
	}
}

func (l *Loop) processCloseQueue() {
	if l.hardAbortRequested() {
		return
	}
	l.drainCommandIngress()
	if l.hardAbortRequested() {
		return
	}
	l.externalMu.Lock()
	batch := l.takeClosePhaseBatchLocked()
	count := batch.remaining()
	l.startPhaseBatch(count)
	l.externalMu.Unlock()

	if count == 0 {
		l.releaseClosePhaseBatch(&batch)
		return
	}
	defer func() {
		l.releaseClosePhaseBatch(&batch)
		l.finishPhaseBatch(count)
	}()

	for {
		job, ok := batch.next()
		if !ok {
			break
		}
		l.safeExecute(job.fn)
		l.drainMicrotasks()
		if l.hardAbortRequested() {
			break
		}
	}
}

// drainMicrotasks drains the nextTick and microtask queues exhaustively,
// following Node.js v11+ semantics.
//
// Node.js drains in alternating BATCHES (verified against Node v26.5.0:
// lib/internal/process/task_queues.js processTicksAndRejections drains ALL
// nextTicks, then calls runMicrotasks — which maps to V8
// MicrotaskQueue::PerformCheckpoint in src/node_task_queue.cc and drains the
// ENTIRE promise queue — repeating until both are empty). First ALL pending
// nextTick callbacks are drained, then ALL pending promise microtasks, then the
// cycle repeats. A nextTick scheduled DURING a promise microtask is therefore
// processed in the next nextTick batch — it does NOT preempt the remaining
// promise microtasks of the current microtask drain. Within each batch the
// respective queue is drained FIFO and exhaustively (a nextTick scheduling
// another nextTick, or a microtask scheduling another microtask, runs in the
// same batch).
//
// After 100000 executed callbacks, a safety counter attempts one error-level
// instance diagnostic to expose runaway self-rescheduling. It does not stop
// draining, matching JavaScript's ability to starve the event loop with
// recursive microtasks. The call is still made to a nil or disabled logger,
// which returns `logiface.ErrDisabled` without emitting an event. Only immediate
// Close makes hardAbortRequested stop this checkpoint; context cancellation and
// graceful Shutdown leave it exhaustive.
func (l *Loop) drainMicrotasks() {
	if l.hardAbortRequested() || l.microtaskYield.Load() {
		return
	}
	l.drainCommandIngress()
	// Fast path: if both queues are empty, skip the loop entirely.
	// This avoids per-iteration Pop() overhead when no microtasks are pending,
	// which is the common case after most callbacks.
	if l.microtaskQueuesEmpty() {
		return
	}

	const safetyThreshold = 100000
	var count int
	warned := false

	for {
		if l.microtaskYield.Load() {
			return
		}
		l.drainCommandIngress()
		progress := false

		// Batch 1: drain ALL currently-available nextTick callbacks.
		// (Node: processTicksAndRejections nextTick while-loop.) A logical
		// callback owner can wait for another goroutine to publish another
		// nextTick while the physical owner is waiting for that callback. Transfer
		// acknowledged ingress after each completed owner batch and repeat before
		// entering the promise batch.
		for {
			executed, completed := l.drainOwnerMicrotaskBatch(l.ownerNextTick, true, false)
			progress = executed > 0 || progress
			count += executed
			if l.hardAbortRequested() || l.microtaskYield.Load() {
				return
			}
			if !warned && count >= safetyThreshold {
				l.logError("eventloop: microtask drain exceeded safety threshold, possible infinite loop in callback", nil)
				warned = true
			}
			if !completed {
				continue
			}
			l.materializeCommandIngress()
			if l.hardAbortRequested() {
				return
			}
			if l.ownerNextTick.IsEmpty() {
				break
			}
		}

		// Batch 2: drain ALL currently-available promise microtasks.
		// (Node: runMicrotasks -> V8 MicrotaskQueue::PerformCheckpoint.)
		for {
			executed, completed := l.drainOwnerPromiseMicrotaskBatch()
			progress = executed > 0 || progress
			count += executed
			if l.hardAbortRequested() || l.microtaskYield.Load() {
				return
			}
			if !warned && count >= safetyThreshold {
				l.logError("eventloop: microtask drain exceeded safety threshold, possible infinite loop in callback", nil)
				warned = true
			}
			if completed {
				break
			}
		}

		// Batch 3: drain checkpoint-end diagnostics only after all public
		// nextTick and promise microtask callbacks have exhausted. These callbacks
		// are not normal promise microtasks; re-enqueueing them into Batch 2 while
		// nextTickQueue is non-empty would starve the nextTick batch that must run
		// before the checkpoint is complete.
		if l.primaryMicrotaskQueuesEmpty() {
			for {
				executed, completed := l.drainOwnerMicrotaskBatch(l.ownerCheckpt, false, true)
				progress = executed > 0 || progress
				count += executed
				if l.hardAbortRequested() || l.microtaskYield.Load() {
					return
				}
				if !warned && count >= safetyThreshold {
					l.logError("eventloop: microtask drain exceeded safety threshold, possible infinite loop in callback", nil)
					warned = true
				}
				if !l.primaryMicrotaskQueuesEmpty() {
					break
				}
				if completed {
					break
				}
			}
		}

		if !progress {
			return
		}
	}
}

// drainOwnerMicrotaskBatch amortizes the isolated-owner handoff across an
// exhaustive queue batch. A callback that calls runtime.Goexit retires the
// worker; completed is false so the caller can resume the still-owned queue on
// a replacement worker without losing the callbacks that were not yet popped.
func (l *Loop) drainOwnerMicrotaskBatch(queue *localFnQueue, primary, stopOnPrimary bool) (executed int, completed bool) {
	outcome := l.executeCallback(func() {
		for {
			fn := l.popOwnerMicrotask(queue, primary)
			if fn == nil {
				return
			}
			executed++
			l.safeExecuteFnDirect(fn)
			if l.hardAbortRequested() || l.microtaskYield.Load() || (stopOnPrimary && !l.primaryMicrotaskQueuesEmpty()) {
				return
			}
		}
	}, true)
	l.logCallbackOutcome("task", outcome)
	return executed, outcome.returned
}

// drainOwnerPromiseMicrotaskBatch claims a registered Promise reaction before
// normal callback admission. If immediate Close closes that admission after the
// microtask was accepted, the denied callback receives its terminal failure
// instead of disappearing with a permanently pending child.
func (l *Loop) drainOwnerPromiseMicrotaskBatch() (executed int, completed bool) {
	outcome := l.executeCallback(func() {
		for {
			job := l.popOwnerPromiseMicrotask()
			if job.fn == nil {
				return
			}
			executed++
			if job.reaction == nil {
				l.safeExecuteFnDirect(job.fn)
			} else {
				if l.testHooks != nil && l.testHooks.BeforePromiseReactionClaim != nil {
					l.testHooks.BeforePromiseReactionClaim(job.reaction)
				}
				reaction, ok := l.claimPendingPromiseReaction(job.reaction)
				if !ok {
					continue
				}
				if !l.safeExecuteFnDirect(job.fn) {
					reaction.fail()
				}
			}
			if l.hardAbortRequested() || l.microtaskYield.Load() {
				return
			}
		}
	}, true)
	l.logCallbackOutcome("task", outcome)
	return executed, outcome.returned
}

// CurrentTickTime returns the cached time for the current tick.
// The returned value uses the monotonic clock and is safe to use for timer calculations.
func (l *Loop) CurrentTickTime() time.Time {
	l.tickAnchorMu.RLock()
	anchor := l.tickAnchor
	l.tickAnchorMu.RUnlock()

	// If anchor not initialized (shouldn't happen after Run), return current wall-clock time
	if anchor.IsZero() {
		return time.Now()
	}
	// Add elapsed monotonic offset to anchor to get current monotonic time
	// This ensures timer accuracy even if wall-clock is adjusted
	elapsed := time.Duration(l.tickElapsedTime.Load())
	return anchor.Add(elapsed)
}

func (l *Loop) refreshTickTime() time.Time {
	now := time.Now()
	l.tickAnchorMu.RLock()
	anchor := l.tickAnchor
	l.tickAnchorMu.RUnlock()
	if anchor.IsZero() {
		l.tickNow = now
		return now
	}
	elapsed := now.Sub(anchor)
	l.tickElapsedTime.Store(int64(elapsed))
	l.tickNow = anchor.Add(elapsed)
	return l.tickNow
}

func (l *Loop) setTickAnchor(t time.Time) {
	l.tickAnchorMu.Lock()
	l.tickAnchor = t
	l.tickAnchorMu.Unlock()
	l.tickElapsedTime.Store(0)
}

func (l *Loop) tickAnchorTime() time.Time {
	l.tickAnchorMu.RLock()
	defer l.tickAnchorMu.RUnlock()
	return l.tickAnchor
}

// State returns the current loop state.
func (l *Loop) State() LoopState {
	return l.state.Load()
}

// calculateTimeout determines how long to block in poll. It returns -1 when
// there is no finite deadline, matching the platform pollers' indefinite wait
// convention. Finite timer deadlines always produce a non-negative timeout;
// fast mode must honor those finite values even when they are longer than one
// second.
func (l *Loop) calculateTimeout() int {
	// Pending check/close phase callbacks should run in the current iteration
	// without sleeping for an unrelated future timer. Ref state affects liveness,
	// not phase execution once the loop is already running.
	l.externalMu.Lock()
	hasPhaseJobs := len(l.checkJobs) > 0 || len(l.closeJobs) > 0 || l.ownerCheckCount.Load() > 0 || l.ownerCloseCount.Load() > 0
	l.externalMu.Unlock()
	if hasPhaseJobs {
		return 0
	}

	deadline, ok := l.nextTimerDeadline()
	if !ok {
		return -1
	}

	// Queue and microtask processing can consume a substantial portion of a
	// turn. Refresh immediately before selecting the poll timeout so elapsed
	// work cannot make an already-due timer sleep for its delay a second time.
	now := l.refreshTickTime()
	return pollTimeoutMillis(deadline.Sub(now))
}

func pollTimeoutMillis(delay time.Duration) int {
	if delay <= 0 {
		return 0
	}
	// Ceiling rounding: if 0 < delta < 1ms, round up to 1ms
	if delay < time.Millisecond {
		return 1
	}

	timeoutMs := delay.Milliseconds()
	if timeoutMs > int64(maxFinitePollTimeoutMs) {
		return maxFinitePollTimeoutMs
	}
	return int(timeoutMs)
}
