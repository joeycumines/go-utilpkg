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
func (x *Loop) SetFastPathMode(mode FastPathMode) error {
	if err := validateFastPathMode(mode); err != nil {
		panic(fmt.Errorf("eventloop: SetFastPathMode: %w", err))
	}
	if mode == FastPathDisabled && fdPollingSupported {
		if err := x.ensurePollerForModeChange(); err != nil {
			return err
		}
	}

	if x.testHooks != nil && x.testHooks.BeforeSetFastPathModeLock != nil {
		x.testHooks.BeforeSetFastPathModeLock()
	}
	x.livenessMu.Lock()
	if x.state.Load() == StateTerminating || x.state.Load() == StateTerminated {
		x.livenessMu.Unlock()
		return ErrLoopTerminated
	}

	if mode == FastPathForced && x.userIOFDCount.Load() > 0 {
		x.livenessMu.Unlock()
		return ErrFastPathIncompatible
	}

	x.fastPathMode.Store(int32(mode))
	if mode != FastPathForced {
		x.fastPathInvariantLogged.Store(false)
	}
	x.livenessMu.Unlock()

	// Wake the loop so it immediately re-evaluates the mode.
	x.doWakeup()

	return nil
}

// canUseFastPath returns true if fast path can be used right now.
// This consolidates all conditions into a single check.
func (x *Loop) canUseFastPath() bool {
	mode := FastPathMode(x.fastPathMode.Load())
	switch mode {
	case FastPathForced:
		if x.userIOFDCount.Load() > 0 {
			if !x.fastPathInvariantLogged.Swap(true) {
				x.logCritical("eventloop: FastPathForced with registered I/O FDs; falling back to poll path", ErrFastPathIncompatible)
			}
			return false
		}
		x.fastPathInvariantLogged.Store(false)
		return true
	case FastPathDisabled:
		return false
	default: // FastPathAuto
		return x.userIOFDCount.Load() == 0
	}
}

// runAux performs one non-blocking owner tick for fast-path task-only work.
//
// It achieves low latency by:
//   - using the same phase-snapshot helpers as the normal tick,
//   - skipping only the blocking poll and timer phases while no timers exist,
//   - preserving per-callback microtask checkpoints, and
//   - continuing later fast turns for work admitted beyond the current snapshot.
func (x *Loop) runAux() {
	if x.hardAbortRequested() {
		return
	}
	x.tickCount++
	x.drainCommandIngress()
	x.recordQueueMetrics()
	if x.hasTimersPending() {
		return
	}
	x.tickActive = true
	defer func() { x.tickActive = false }()
	x.refreshTickTime()

	// Fast-path turns must begin with the same nextTick / promise-microtask
	// checkpoint as the normal tick path. Without this, work submitted before Run
	// or while the fast path is blocked can overtake already-pending microtasks.
	x.drainMicrotasks()
	if x.hardAbortRequested() {
		return
	}
	if x.autoExitReady() {
		return
	}
	x.processCheckQueue()
	if x.hardAbortRequested() {
		return
	}

	x.drainMicrotasks()
	if x.hardAbortRequested() {
		return
	}
	if x.autoExitReady() {
		return
	}

	x.processCloseQueue()
	if x.hardAbortRequested() {
		return
	}

	x.drainMicrotasks()
	if x.hardAbortRequested() {
		return
	}
	if x.autoExitReady() {
		return
	}

	x.processInternalQueue()
	if x.hardAbortRequested() {
		return
	}

	x.drainMicrotasks()
	if x.hardAbortRequested() {
		return
	}

	// Drain public Submit work after internal/control work, matching normal tick
	// priority while retaining processExternal's bounded snapshot and pressure
	// signal for producer pressure.
	x.processExternal()
	if x.hardAbortRequested() {
		return
	}

	x.drainMicrotasks()
	if x.hardAbortRequested() {
		return
	}

	// Drain microtasks (safety net — catches any microtasks from the last task's drain)
	x.drainMicrotasks()
}

// hasTimersPending returns true if there are pending timers.
// NOTE: This is only called from the loop goroutine, so no mutex needed.
// tick is a single iteration of the event loop.
func (x *Loop) tick() {
	x.tickCount++
	x.drainCommandIngress()
	x.tickActive = true
	defer func() { x.tickActive = false }()

	x.recordQueueMetrics()

	// Update elapsed monotonic time offset from anchor.
	x.refreshTickTime()

	// Drain microtasks at the start of each tick to catch any that were
	// scheduled during the previous tick's final drain or between ticks.
	x.drainMicrotasks()
	if x.hardAbortRequested() {
		return
	}

	// Node v20+/libuv 1.45+ observes poll before timers in the JS-facing
	// topology; calculateTimeout returns zero for already-due timers, so this
	// poll phase will not sleep past due timer work.
	x.poll()
	if x.hardAbortRequested() {
		return
	}
	if x.autoExitReady() {
		return
	}
	x.refreshTickTime()

	x.drainMicrotasks()
	if x.hardAbortRequested() {
		return
	}
	if x.autoExitReady() {
		return
	}

	x.processCheckQueue()
	if x.hardAbortRequested() {
		return
	}

	x.drainMicrotasks()
	if x.hardAbortRequested() {
		return
	}
	if x.autoExitReady() {
		return
	}

	x.processCloseQueue()
	if x.hardAbortRequested() {
		return
	}

	x.drainMicrotasks()
	if x.hardAbortRequested() {
		return
	}
	if x.autoExitReady() {
		return
	}

	// Node v20+/libuv 1.45+ runs timers only after poll. A check callback queued
	// from a check callback reaches the next check phase before timers that were
	// scheduled from the prior check callback but have not yet reached their
	// minimum threshold.
	if x.testHooks != nil && x.testHooks.BeforeRunTimers != nil {
		x.testHooks.BeforeRunTimers()
	}
	x.runTimers()
	if x.hardAbortRequested() {
		return
	}

	// Inter-phase drain: catch microtasks from timer callbacks before
	// processing the internal queue.
	x.drainMicrotasks()
	if x.hardAbortRequested() {
		return
	}

	x.processInternalQueue()
	if x.hardAbortRequested() {
		return
	}

	// Inter-phase drain: catch microtasks from internal tasks before
	// processing the external queue.
	x.drainMicrotasks()
	if x.hardAbortRequested() {
		return
	}

	x.processExternal()
	if x.hardAbortRequested() {
		return
	}

	// Inter-phase drain: catch microtasks from external tasks.
	x.drainMicrotasks()
	if x.hardAbortRequested() {
		return
	}

	// Safety net drain: catches any microtasks that escaped per-callback draining
	// before registry scavenging or the next tick.
	x.drainMicrotasks()

	// Scavenge registry - limit per tick to avoid stalling
	const registryScavengeLimit = 20
	x.registry.Scavenge(registryScavengeLimit)
}

func (x *Loop) recordQueueMetrics() {
	if x.metrics == nil {
		return
	}
	x.externalMu.Lock()
	// Close the post-drain publication race so the sample represents every
	// command acknowledged before this owner-turn boundary.
	x.drainCommandIngressLocked()
	ingressDepth := int(x.ownerExternalCount.Load()+x.ownerCheckCount.Load()+x.ownerCloseCount.Load()) + len(x.checkJobs) + len(x.closeJobs)
	internalDepth := int(x.ownerInternalCount.Load())
	microtaskDepth := int(x.ownerMicroCount.Load() + x.ingressMicroCount.Load())
	x.externalMu.Unlock()
	x.metrics.recordQueueDepths(
		ingressDepth,
		internalDepth,
		microtaskDepth,
	)
}

// processInternalQueue drains the internal priority queue.
func (x *Loop) processInternalQueue() bool {
	if x.hardAbortRequested() {
		return false
	}
	x.drainCommandIngress()
	processed := false
	ownerSnapshot := x.ownerInternal.Len()
	for range ownerSnapshot {
		task := x.popOwnerInternal()
		if task == nil {
			break
		}
		x.safeExecute(task)
		x.drainMicrotasks()
		processed = true
		if x.hardAbortRequested() {
			return processed
		}
	}
	return processed
}

// processExternal processes the external queue phase snapshot.
func (x *Loop) processExternal() {
	if x.hardAbortRequested() {
		return
	}
	x.drainCommandIngress()
	// Drain the owner queue using a phase snapshot. Concurrent ingress is first
	// materialized by drainCommandIngress and submissions admitted during this
	// phase wait for a later tick.
	ownerSnapshot := x.ownerExternal.Len()

	processed := 0
	for range ownerSnapshot {
		task := x.popOwnerExternal()
		if task == nil {
			break
		}
		processed++
		x.safeExecute(task)
		x.drainMicrotasks()
		if x.hardAbortRequested() {
			break
		}
	}

	if x.hardAbortRequested() {
		return
	}

	if x.testHooks != nil && x.testHooks.BeforeExternalPressureCheck != nil {
		x.testHooks.BeforeExternalPressureCheck()
		if x.hardAbortRequested() {
			return
		}
	}

	x.externalMu.Lock()
	remainingTasks := int(x.ownerExternalCount.Load()) + x.externalCommandCountLocked()
	x.externalMu.Unlock()

	// Emit a pressure signal if work remains beyond this phase snapshot. This is
	// not a numeric callback budget; it indicates producers outpaced the current
	// phase snapshot and callers may want backpressure.
	if processed > 0 && remainingTasks > 0 && x.queuePressureHandler != nil {
		if !x.beginCallbackExecution() {
			return
		}
		outcome := x.executeCallback(x.queuePressureHandler, true)
		x.logCallbackOutcome("queue pressure handler", outcome)
	}
}

func (x *Loop) processCheckQueue() {
	if x.hardAbortRequested() {
		return
	}
	x.drainCommandIngress()
	if x.hardAbortRequested() {
		return
	}
	x.externalMu.Lock()
	batch := x.takeCheckPhaseBatchLocked()
	count := batch.remaining()
	x.startPhaseBatch(count)
	x.externalMu.Unlock()

	if count == 0 {
		x.releaseCheckPhaseBatch(&batch)
		x.releaseMicrotaskYieldAtEmptyCheck()
		return
	}
	defer func() {
		x.releaseCheckPhaseBatch(&batch)
		x.finishPhaseBatch(count)
	}()

	for {
		job, ok := batch.next()
		if !ok {
			break
		}
		x.safeExecute(job.fn)
		x.drainMicrotasks()
		if x.hardAbortRequested() {
			break
		}
	}
}

func (x *Loop) processCloseQueue() {
	if x.hardAbortRequested() {
		return
	}
	x.drainCommandIngress()
	if x.hardAbortRequested() {
		return
	}
	x.externalMu.Lock()
	batch := x.takeClosePhaseBatchLocked()
	count := batch.remaining()
	x.startPhaseBatch(count)
	x.externalMu.Unlock()

	if count == 0 {
		x.releaseClosePhaseBatch(&batch)
		return
	}
	defer func() {
		x.releaseClosePhaseBatch(&batch)
		x.finishPhaseBatch(count)
	}()

	for {
		job, ok := batch.next()
		if !ok {
			break
		}
		x.safeExecute(job.fn)
		x.drainMicrotasks()
		if x.hardAbortRequested() {
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
func (x *Loop) drainMicrotasks() {
	if x.hardAbortRequested() || x.microtaskYield.Load() {
		return
	}
	x.drainCommandIngress()
	// Fast path: if both queues are empty, skip the loop entirely.
	// This avoids per-iteration Pop() overhead when no microtasks are pending,
	// which is the common case after most callbacks.
	if x.microtaskQueuesEmpty() {
		return
	}

	const safetyThreshold = 100000
	var count int
	warned := false

	for {
		if x.microtaskYield.Load() {
			return
		}
		x.drainCommandIngress()
		progress := false

		// Batch 1: drain ALL currently-available nextTick callbacks.
		// (Node: processTicksAndRejections nextTick while-loop.) A logical
		// callback owner can wait for another goroutine to publish another
		// nextTick while the physical owner is waiting for that callback. Transfer
		// acknowledged ingress after each completed owner batch and repeat before
		// entering the promise batch.
		for {
			executed, completed := x.drainOwnerMicrotaskBatch(x.ownerNextTick, true, false)
			progress = executed > 0 || progress
			count += executed
			if x.hardAbortRequested() || x.microtaskYield.Load() {
				return
			}
			if !warned && count >= safetyThreshold {
				x.logError("eventloop: microtask drain exceeded safety threshold, possible infinite loop in callback", nil)
				warned = true
			}
			if !completed {
				continue
			}
			x.materializeCommandIngress()
			if x.hardAbortRequested() {
				return
			}
			if x.ownerNextTick.IsEmpty() {
				break
			}
		}

		// Batch 2: drain ALL currently-available promise microtasks.
		// (Node: runMicrotasks -> V8 MicrotaskQueue::PerformCheckpoint.)
		for {
			executed, completed := x.drainOwnerPromiseMicrotaskBatch()
			progress = executed > 0 || progress
			count += executed
			if x.hardAbortRequested() || x.microtaskYield.Load() {
				return
			}
			if !warned && count >= safetyThreshold {
				x.logError("eventloop: microtask drain exceeded safety threshold, possible infinite loop in callback", nil)
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
		if x.primaryMicrotaskQueuesEmpty() {
			for {
				executed, completed := x.drainOwnerMicrotaskBatch(x.ownerCheckpt, false, true)
				progress = executed > 0 || progress
				count += executed
				if x.hardAbortRequested() || x.microtaskYield.Load() {
					return
				}
				if !warned && count >= safetyThreshold {
					x.logError("eventloop: microtask drain exceeded safety threshold, possible infinite loop in callback", nil)
					warned = true
				}
				if !x.primaryMicrotaskQueuesEmpty() {
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
func (x *Loop) drainOwnerMicrotaskBatch(queue *localFnQueue, primary, stopOnPrimary bool) (executed int, completed bool) {
	outcome := x.executeCallback(func() {
		for {
			fn := x.popOwnerMicrotask(queue, primary)
			if fn == nil {
				return
			}
			executed++
			x.safeExecuteFnDirect(fn)
			if x.hardAbortRequested() || x.microtaskYield.Load() || (stopOnPrimary && !x.primaryMicrotaskQueuesEmpty()) {
				return
			}
		}
	}, true)
	x.logCallbackOutcome("task", outcome)
	return executed, outcome.returned
}

// drainOwnerPromiseMicrotaskBatch claims a registered Promise reaction before
// normal callback admission. If immediate Close closes that admission after the
// microtask was accepted, the denied callback receives its terminal failure
// instead of disappearing with a permanently pending child.
func (x *Loop) drainOwnerPromiseMicrotaskBatch() (executed int, completed bool) {
	outcome := x.executeCallback(func() {
		for {
			job := x.popOwnerPromiseMicrotask()
			if job.fn == nil {
				return
			}
			executed++
			if job.reaction == nil {
				x.safeExecuteFnDirect(job.fn)
			} else {
				if x.testHooks != nil && x.testHooks.BeforePromiseReactionClaim != nil {
					x.testHooks.BeforePromiseReactionClaim(job.reaction)
				}
				reaction, ok := x.claimPendingPromiseReaction(job.reaction)
				if !ok {
					continue
				}
				if !x.safeExecuteFnDirect(job.fn) {
					reaction.fail()
				}
			}
			if x.hardAbortRequested() || x.microtaskYield.Load() {
				return
			}
		}
	}, true)
	x.logCallbackOutcome("task", outcome)
	return executed, outcome.returned
}

// CurrentTickTime returns the cached time for the current tick.
// The returned value uses the monotonic clock and is safe to use for timer calculations.
func (x *Loop) CurrentTickTime() time.Time {
	x.tickAnchorMu.RLock()
	anchor := x.tickAnchor
	x.tickAnchorMu.RUnlock()

	// If anchor not initialized (shouldn't happen after Run), return current wall-clock time
	if anchor.IsZero() {
		return time.Now()
	}
	// Add elapsed monotonic offset to anchor to get current monotonic time
	// This ensures timer accuracy even if wall-clock is adjusted
	elapsed := time.Duration(x.tickElapsedTime.Load())
	return anchor.Add(elapsed)
}

func (x *Loop) refreshTickTime() time.Time {
	now := time.Now()
	x.tickAnchorMu.RLock()
	anchor := x.tickAnchor
	x.tickAnchorMu.RUnlock()
	if anchor.IsZero() {
		x.tickNow = now
		return now
	}
	elapsed := now.Sub(anchor)
	x.tickElapsedTime.Store(int64(elapsed))
	x.tickNow = anchor.Add(elapsed)
	return x.tickNow
}

func (x *Loop) setTickAnchor(t time.Time) {
	x.tickAnchorMu.Lock()
	x.tickAnchor = t
	x.tickAnchorMu.Unlock()
	x.tickElapsedTime.Store(0)
}

func (x *Loop) tickAnchorTime() time.Time {
	x.tickAnchorMu.RLock()
	defer x.tickAnchorMu.RUnlock()
	return x.tickAnchor
}

// State returns the current loop state.
func (x *Loop) State() LoopState {
	return x.state.Load()
}

// calculateTimeout determines how long to block in poll. It returns -1 when
// there is no finite deadline, matching the platform pollers' indefinite wait
// convention. Finite timer deadlines always produce a non-negative timeout;
// fast mode must honor those finite values even when they are longer than one
// second.
func (x *Loop) calculateTimeout() int {
	// Pending check/close phase callbacks should run in the current iteration
	// without sleeping for an unrelated future timer. Ref state affects liveness,
	// not phase execution once the loop is already running.
	x.externalMu.Lock()
	hasPhaseJobs := len(x.checkJobs) > 0 || len(x.closeJobs) > 0 || x.ownerCheckCount.Load() > 0 || x.ownerCloseCount.Load() > 0
	x.externalMu.Unlock()
	if hasPhaseJobs {
		return 0
	}

	deadline, ok := x.nextTimerDeadline()
	if !ok {
		return -1
	}

	// Queue and microtask processing can consume a substantial portion of a
	// turn. Refresh immediately before selecting the poll timeout so elapsed
	// work cannot make an already-due timer sleep for its delay a second time.
	now := x.refreshTickTime()
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
