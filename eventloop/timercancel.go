package eventloop

import (
	"slices"
)

// RefTimer marks the timer as keeping the event loop alive.
// Analogous to libuv's uv_ref(). Timers are ref'd by default.
//
// Thread-safe: safe to call from any goroutine.
// When called from the logical loop owner: immediate synchronous effect via
// applyTimerRefChange (timerMap lookup + refedTimerCount update).
// When called from external goroutines after Run starts: blocks until the owner
// processes the ref command in ingress FIFO order with timer registration and
// cancellation commands. If terminal dependency release wins after admission
// but before owner application, RefTimer returns nil and discards the now
// unobservable command; no later timer phase can observe its ref state.
// Before Run starts, the change is queued in that same order and returns
// without waiting for an owner.
// Silently ignores timers that have already fired or don't exist.
func (x *Loop) RefTimer(id TimerID) error {
	return x.submitTimerRefChange(id, true, true)
}

// UnrefTimer marks the timer as NOT keeping the event loop alive.
// Analogous to libuv's uv_unref(). If the only remaining work is
// unref'd timers, the loop is considered idle.
//
// Thread-safe: safe to call from any goroutine.
// When called from the logical loop owner: immediate synchronous effect via
// applyTimerRefChange (timerMap lookup + refedTimerCount update).
// When called from external goroutines after Run starts: blocks until the owner
// processes the unref command in ingress FIFO order with timer registration and
// cancellation commands. If terminal dependency release wins after admission
// but before owner application, UnrefTimer returns nil and discards the now
// unobservable command; no later timer phase can observe its ref state.
// Before Run starts, the change is queued in that same order and returns
// without waiting for an owner.
// Silently ignores timers that have already fired or don't exist.
func (x *Loop) UnrefTimer(id TimerID) error {
	return x.submitTimerRefChange(id, false, true)
}

// queueTimerRefChange accepts a timer ref-state change without waiting for the
// loop owner. Adapter handle arbitration must establish validity before calling
// this method because asynchronous completion cannot report a later timer miss.
func (x *Loop) queueTimerRefChange(id TimerID, ref bool) error {
	return x.submitTimerRefChange(id, ref, false)
}

func (x *Loop) submitTimerRefChange(id TimerID, ref, wait bool) error {
	initialState := x.state.Load()
	if initialState == StateTerminated || (!wait && initialState == StateTerminating) {
		return ErrLoopTerminated
	}
	kind := loopCommandTimerUnref
	if ref {
		kind = loopCommandTimerRef
	}
	if !wait {
		cmd := loopCommand{kind: kind, token: uint64(id)}
		if ref {
			var beforeCommit func()
			if x.testHooks != nil {
				beforeCommit = x.testHooks.BeforeTimerRefCommit
			}
			return x.submitLivenessCommand(cmd, beforeCommit)
		}
		return x.enqueueCommand(cmd, x.terminalQueueAllowed)
	}
	if x.ownsLocalQueues() {
		if ref {
			x.livenessMu.Lock()
			if err := x.rejectLivenessAddLocked(); err != nil {
				x.livenessMu.Unlock()
				return err
			}
		}
		x.materializeCommandIngress()
		x.applyTimerRefChange(id, ref)
		if ref {
			x.livenessMu.Unlock()
		}
		return nil
	}
	state := x.state.Load()
	if state == StateAwake {
		cmd := loopCommand{kind: kind, token: uint64(id)}
		if ref {
			var beforeCommit func()
			if x.testHooks != nil {
				beforeCommit = x.testHooks.BeforeTimerRefCommit
			}
			return x.submitLivenessCommand(cmd, beforeCommit)
		}
		return x.enqueueCommand(cmd, x.terminalQueueAllowed)
	}
	// External goroutine: synchronous command submission to ensure the ref change
	// is applied before any later due timer phase that observes the accepted
	// command. This preserves timer-add/cancel/ref/unref FIFO ordering in both
	// normal and fast-path owner ticks.
	result := make(chan error, 1)
	cmd := loopCommand{kind: kind, token: uint64(id), result: result}
	if ref {
		var beforeCommit func()
		if x.testHooks != nil {
			beforeCommit = x.testHooks.BeforeTimerRefCommit
		}
		if err := x.submitLivenessCommand(cmd, beforeCommit); err != nil {
			return err
		}
	} else if err := x.enqueueCommand(cmd, x.terminalQueueAllowed); err != nil {
		return err
	}
	if x.testHooks != nil && x.testHooks.AfterSynchronousTimerCommandPublish != nil {
		x.testHooks.AfterSynchronousTimerCommandPublish(kind)
	}
	return x.awaitCancelTimerResult(result)
}

// applyTimerRefChange applies the ref/unref change directly.
// MUST be called by the logical loop or terminal-drain owner (timerMap is not
// thread-safe).
// Silently ignores timers that have already fired, been cancelled, or don't exist.
// When called from external goroutines, FIFO command ingress ensures timer
// registrations, cancellations, and ref changes apply in acceptance order before
// due timers can run.
// When called from the logical owner, ScheduleTimer registers synchronously.
func (x *Loop) applyTimerRefChange(id TimerID, ref bool) {
	t, ok := x.timerMap[id]
	if !ok {
		// Timer already fired, was cancelled, or doesn't exist. Silently ignore.
		return
	}
	old := t.refed.Swap(ref)
	if old != ref {
		if ref {
			x.refedTimerCount.Add(1)
		} else {
			x.refedTimerCount.Add(-1)
		}
		// Increment epoch to ensure Alive() detects the liveness change
		x.submissionEpoch.Add(1)
		// Wake the loop so auto-exit re-checks Alive() after the count changes.
		// Only needed when auto-exit is enabled: the loop may be in PollIO
		// and needs to return so the auto-exit check sees the liveness transition.
		// When auto-exit is disabled, no liveness re-evaluation wake is needed.
		if x.autoExit {
			x.doWakeup()
		}
	}
}

// CancelTimer cancels a scheduled timer before it fires.
// Returns ErrTimerNotFound if the timer does not exist.
// A timer scheduled before [Loop.Run] starts can also be canceled before Run;
// the cancellation is queued behind the timer registration and applied when
// the loop starts. An invalid ID returns ErrTimerNotFound before Run and after
// the loop owner evaluates the cancellation during Run. A terminated loop
// returns ErrLoopTerminated. If a terminal transition wins after admission but
// before the owner publishes the exact result, CancelTimer returns
// ErrLoopTerminated; graceful drain still applies the accepted cancellation in
// FIFO order, while immediate Close may discard it. Use [Loop.Requests] when
// admission acknowledgement is sufficient and the caller must not wait for
// owner application.
//
// Not gated by the quiescing flag: cancellation reduces liveness (opposite of
// ScheduleTimer which IS gated). This asymmetry is intentional — during the
// quiescing window, callers can cancel timers but not schedule new ones.
func (x *Loop) CancelTimer(id TimerID) error {
	return x.cancelTimer(id, true)
}

// queueTimerCancel accepts cancellation without waiting for the loop owner. It
// does not suppress a callback whose entry has already been claimed. Adapter
// callers that promise stronger suppression must arbitrate their handle first.
func (x *Loop) queueTimerCancel(id TimerID) error {
	return x.cancelTimer(id, false)
}

func (x *Loop) queueTimerCancels(ids []TimerID) error {
	if len(ids) == 0 {
		return nil
	}
	return x.enqueueCommand(loopCommand{
		kind: loopCommandTimerCancelBatch,
		ids:  slices.Clone(ids),
	}, x.terminalQueueAllowed)
}

func (x *Loop) cancelTimer(id TimerID, wait bool) error {
	// Check if loop is in a valid state for cancellation.
	state := x.state.Load()
	if state == StateTerminated || (!wait && state == StateTerminating) {
		return ErrLoopTerminated
	}
	if !wait {
		return x.enqueueCommand(loopCommand{kind: loopCommandTimerCancel, token: uint64(id)}, x.terminalQueueAllowed)
	}

	if x.ownsLocalQueues() {
		x.materializeCommandIngress()
		return x.applyCancelTimer(id)
	}

	if state == StateAwake {
		if err, handled := x.cancelTimerBeforeRun(id); handled {
			return err
		}
	}

	result := make(chan error, 1)

	// Submit as a timer lifecycle command so an accepted cancellation cannot be
	// overtaken by a due timer merely because internal/control work runs after the
	// timer phase.
	if err := x.enqueueCommand(loopCommand{kind: loopCommandTimerCancel, token: uint64(id), result: result}, x.terminalQueueAllowed); err != nil {
		return err
	}
	if x.testHooks != nil && x.testHooks.AfterSynchronousTimerCommandPublish != nil {
		x.testHooks.AfterSynchronousTimerCommandPublish(loopCommandTimerCancel)
	}

	return x.awaitCancelTimerResult(result)
}

func (x *Loop) cancelTimerBeforeRun(id TimerID) (error, bool) {
	x.externalMu.Lock()
	if x.state.Load() != StateAwake {
		x.externalMu.Unlock()
		return nil, false
	}
	live := x.pendingTimerIDsLocked()
	if _, ok := live[id]; !ok {
		x.externalMu.Unlock()
		return ErrTimerNotFound, true
	}
	x.enqueueCommandLocked(loopCommand{kind: loopCommandTimerCancel, token: uint64(id)})
	x.externalMu.Unlock()
	x.wakeAfterIngress()
	return nil, true
}

func (x *Loop) awaitCancelTimerResult(result <-chan error) error {
	select {
	case err := <-result:
		return err
	case <-x.loopDone:
		select {
		case err := <-result:
			return err
		default:
		}
		if done, active := x.terminalDrainWaiter(); active {
			select {
			case err := <-result:
				return err
			case <-done:
				select {
				case err := <-result:
					return err
				default:
				}
			}
		}
		return ErrLoopTerminated
	}
}

// applyCancelTimer cancels a timer by ID. It must be called by the logical loop
// or terminal-drain owner.
func (x *Loop) applyCancelTimer(id TimerID) error {
	t, exists := x.timerMap[id]
	if !exists {
		// Timer not in map — already fired or cancelled
		return ErrTimerNotFound
	}
	// Mark as canceled
	t.canceled.Store(true)
	// If timer was already popped from its deadline list (e.g., by runTimers
	// during the currently executing timer's callback), runTimers owns pool return.
	// Map visibility and liveness still retire at the first successful
	// cancellation so a duplicate observes the sequential ErrTimerNotFound
	// result. runTimers later sees refed=false and cannot double-decrement.
	if t.list == nil {
		x.deleteTimer(id)
		if t.refed.Swap(false) {
			x.refedTimerCount.Add(-1)
		}
		return nil
	}
	// Timer is still pending in a deadline list — we own the cleanup.
	x.deleteTimer(id)
	if t.refed.Swap(false) {
		x.refedTimerCount.Add(-1)
	}
	x.unlinkTimerNode(t)
	// Return timer to pool
	resetTimerForPool(t)
	timerPool.Put(t)
	return nil
}

// CancelTimers cancels multiple scheduled timers in a single batch operation.
//
// The batch submits and applies one ordered lifecycle command.
//
// Returns a slice of errors corresponding to each timer ID:
//   - nil: Timer was successfully cancelled
//   - ErrTimerNotFound: Timer ID was not found in the timerMap
//
// Results are sequential, so a duplicate ID succeeds only at its first live
// occurrence. Before Run starts, queued registrations and earlier lifecycle
// commands are replayed under ingress ownership to return the same exact
// results without waiting for a loop owner. Returns ErrLoopTerminated for all
// IDs if terminal state prevents admission or wins before the owner publishes
// the exact batch result. Graceful drain still applies an already-admitted batch
// in FIFO order, while immediate Close may discard it. Use [Loop.Requests] for
// admission-only batch cancellation.
//
// Not gated by the quiescing flag: cancellation reduces liveness (opposite of
// ScheduleTimer which IS gated). See CancelTimer for rationale.
//
// Thread Safety: Safe to call from any goroutine.
func (x *Loop) CancelTimers(ids ...TimerID) []error {
	if len(ids) == 0 {
		return nil
	}

	// Check if loop is in a valid state for cancellation.
	state := x.state.Load()
	if state == StateTerminated {
		errors := make([]error, len(ids))
		for i := range errors {
			errors[i] = ErrLoopTerminated
		}
		return errors
	}

	if x.ownsLocalQueues() {
		x.materializeCommandIngress()
		return x.applyCancelTimers(ids)
	}

	ids = slices.Clone(ids)

	if state == StateAwake {
		if errors, handled := x.cancelTimersBeforeRun(ids); handled {
			return errors
		}
	}

	result := make(chan []error, 1)

	// Submit as one timer lifecycle command so the batch cannot be overtaken by a
	// due timer phase after timer registrations have transferred from ingress.
	if err := x.enqueueCommand(loopCommand{kind: loopCommandTimerCancelBatch, ids: ids, results: result}, x.terminalQueueAllowed); err != nil {
		// submitToQueue failed, return error for all IDs
		errors := make([]error, len(ids))
		for i := range errors {
			errors[i] = err
		}
		return errors
	}
	if x.testHooks != nil && x.testHooks.AfterSynchronousTimerCommandPublish != nil {
		x.testHooks.AfterSynchronousTimerCommandPublish(loopCommandTimerCancelBatch)
	}

	return x.awaitCancelTimersResult(result, len(ids))
}

func (x *Loop) cancelTimersBeforeRun(ids []TimerID) ([]error, bool) {
	x.externalMu.Lock()
	if x.state.Load() != StateAwake {
		x.externalMu.Unlock()
		return nil, false
	}
	live := x.pendingTimerIDsLocked()
	errors := make([]error, len(ids))
	accepted := false
	for index, id := range ids {
		if _, ok := live[id]; !ok {
			errors[index] = ErrTimerNotFound
			continue
		}
		delete(live, id)
		accepted = true
	}
	if accepted {
		x.enqueueCommandLocked(loopCommand{kind: loopCommandTimerCancelBatch, ids: ids})
	}
	x.externalMu.Unlock()
	if accepted {
		x.wakeAfterIngress()
	}
	return errors, true
}

func (x *Loop) pendingTimerIDsLocked() map[TimerID]struct{} {
	live := make(map[TimerID]struct{})
	if x.commands == nil {
		return live
	}
	for index := x.commands.head; index < len(x.commands.cmds); index++ {
		cmd := x.commands.cmds[index]
		switch cmd.kind {
		case loopCommandTimerAdd:
			if cmd.timer != nil {
				live[cmd.timer.id] = struct{}{}
			}
		case loopCommandTimerCancel:
			delete(live, TimerID(cmd.token))
		case loopCommandTimerCancelBatch:
			for _, id := range cmd.ids {
				delete(live, id)
			}
		}
	}
	return live
}

func (x *Loop) awaitCancelTimersResult(result <-chan []error, count int) []error {
	select {
	case res := <-result:
		return res
	case <-x.loopDone:
		select {
		case res := <-result:
			return res
		default:
		}
		if done, active := x.terminalDrainWaiter(); active {
			select {
			case res := <-result:
				return res
			case <-done:
				select {
				case res := <-result:
					return res
				default:
				}
			}
		}
		errors := make([]error, count)
		for i := range errors {
			errors[i] = ErrLoopTerminated
		}
		return errors
	}
}

// applyCancelTimers cancels multiple timers. It must run under logical owner
// access to timerMap.
func (x *Loop) applyCancelTimers(ids []TimerID) []error {
	errors := make([]error, len(ids))
	for i, id := range ids {
		errors[i] = x.applyCancelTimer(id)
	}
	return errors
}
