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
func (l *Loop) RefTimer(id TimerID) error {
	return l.submitTimerRefChange(id, true, true)
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
func (l *Loop) UnrefTimer(id TimerID) error {
	return l.submitTimerRefChange(id, false, true)
}

// queueTimerRefChange accepts a timer ref-state change without waiting for the
// loop owner. Adapter handle arbitration must establish validity before calling
// this method because asynchronous completion cannot report a later timer miss.
func (l *Loop) queueTimerRefChange(id TimerID, ref bool) error {
	return l.submitTimerRefChange(id, ref, false)
}

func (l *Loop) submitTimerRefChange(id TimerID, ref, wait bool) error {
	initialState := l.state.Load()
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
			if l.testHooks != nil {
				beforeCommit = l.testHooks.BeforeTimerRefCommit
			}
			return l.submitLivenessCommand(cmd, beforeCommit)
		}
		return l.enqueueCommand(cmd, l.terminalQueueAllowed)
	}
	if l.ownsLocalQueues() {
		if ref {
			l.livenessMu.Lock()
			if err := l.rejectLivenessAddLocked(); err != nil {
				l.livenessMu.Unlock()
				return err
			}
		}
		l.materializeCommandIngress()
		l.applyTimerRefChange(id, ref)
		if ref {
			l.livenessMu.Unlock()
		}
		return nil
	}
	state := l.state.Load()
	if state == StateAwake {
		cmd := loopCommand{kind: kind, token: uint64(id)}
		if ref {
			var beforeCommit func()
			if l.testHooks != nil {
				beforeCommit = l.testHooks.BeforeTimerRefCommit
			}
			return l.submitLivenessCommand(cmd, beforeCommit)
		}
		return l.enqueueCommand(cmd, l.terminalQueueAllowed)
	}
	// External goroutine: synchronous command submission to ensure the ref change
	// is applied before any later due timer phase that observes the accepted
	// command. This preserves timer-add/cancel/ref/unref FIFO ordering in both
	// normal and fast-path owner ticks.
	result := make(chan error, 1)
	cmd := loopCommand{kind: kind, token: uint64(id), result: result}
	if ref {
		var beforeCommit func()
		if l.testHooks != nil {
			beforeCommit = l.testHooks.BeforeTimerRefCommit
		}
		if err := l.submitLivenessCommand(cmd, beforeCommit); err != nil {
			return err
		}
	} else if err := l.enqueueCommand(cmd, l.terminalQueueAllowed); err != nil {
		return err
	}
	if l.testHooks != nil && l.testHooks.AfterSynchronousTimerCommandPublish != nil {
		l.testHooks.AfterSynchronousTimerCommandPublish(kind)
	}
	return l.awaitCancelTimerResult(result)
}

// applyTimerRefChange applies the ref/unref change directly.
// MUST be called by the logical loop or terminal-drain owner (timerMap is not
// thread-safe).
// Silently ignores timers that have already fired, been cancelled, or don't exist.
// When called from external goroutines, FIFO command ingress ensures timer
// registrations, cancellations, and ref changes apply in acceptance order before
// due timers can run.
// When called from the logical owner, ScheduleTimer registers synchronously.
func (l *Loop) applyTimerRefChange(id TimerID, ref bool) {
	t, ok := l.timerMap[id]
	if !ok {
		// Timer already fired, was cancelled, or doesn't exist. Silently ignore.
		return
	}
	old := t.refed.Swap(ref)
	if old != ref {
		if ref {
			l.refedTimerCount.Add(1)
		} else {
			l.refedTimerCount.Add(-1)
		}
		// Increment epoch to ensure Alive() detects the liveness change
		l.submissionEpoch.Add(1)
		// Wake the loop so auto-exit re-checks Alive() after the count changes.
		// Only needed when auto-exit is enabled: the loop may be in PollIO
		// and needs to return so the auto-exit check sees the liveness transition.
		// When auto-exit is disabled, no liveness re-evaluation wake is needed.
		if l.autoExit {
			l.doWakeup()
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
func (l *Loop) CancelTimer(id TimerID) error {
	return l.cancelTimer(id, true)
}

// queueTimerCancel accepts cancellation without waiting for the loop owner. It
// does not suppress a callback whose entry has already been claimed. Adapter
// callers that promise stronger suppression must arbitrate their handle first.
func (l *Loop) queueTimerCancel(id TimerID) error {
	return l.cancelTimer(id, false)
}

func (l *Loop) queueTimerCancels(ids []TimerID) error {
	if len(ids) == 0 {
		return nil
	}
	return l.enqueueCommand(loopCommand{
		kind: loopCommandTimerCancelBatch,
		ids:  slices.Clone(ids),
	}, l.terminalQueueAllowed)
}

func (l *Loop) cancelTimer(id TimerID, wait bool) error {
	// Check if loop is in a valid state for cancellation.
	state := l.state.Load()
	if state == StateTerminated || (!wait && state == StateTerminating) {
		return ErrLoopTerminated
	}
	if !wait {
		return l.enqueueCommand(loopCommand{kind: loopCommandTimerCancel, token: uint64(id)}, l.terminalQueueAllowed)
	}

	if l.ownsLocalQueues() {
		l.materializeCommandIngress()
		return l.applyCancelTimer(id)
	}

	if state == StateAwake {
		if err, handled := l.cancelTimerBeforeRun(id); handled {
			return err
		}
	}

	result := make(chan error, 1)

	// Submit as a timer lifecycle command so an accepted cancellation cannot be
	// overtaken by a due timer merely because internal/control work runs after the
	// timer phase.
	if err := l.enqueueCommand(loopCommand{kind: loopCommandTimerCancel, token: uint64(id), result: result}, l.terminalQueueAllowed); err != nil {
		return err
	}
	if l.testHooks != nil && l.testHooks.AfterSynchronousTimerCommandPublish != nil {
		l.testHooks.AfterSynchronousTimerCommandPublish(loopCommandTimerCancel)
	}

	return l.awaitCancelTimerResult(result)
}

func (l *Loop) cancelTimerBeforeRun(id TimerID) (error, bool) {
	l.externalMu.Lock()
	if l.state.Load() != StateAwake {
		l.externalMu.Unlock()
		return nil, false
	}
	live := l.pendingTimerIDsLocked()
	if _, ok := live[id]; !ok {
		l.externalMu.Unlock()
		return ErrTimerNotFound, true
	}
	l.enqueueCommandLocked(loopCommand{kind: loopCommandTimerCancel, token: uint64(id)})
	l.externalMu.Unlock()
	l.wakeAfterIngress()
	return nil, true
}

func (l *Loop) awaitCancelTimerResult(result <-chan error) error {
	select {
	case err := <-result:
		return err
	case <-l.loopDone:
		select {
		case err := <-result:
			return err
		default:
		}
		if done, active := l.terminalDrainWaiter(); active {
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
func (l *Loop) applyCancelTimer(id TimerID) error {
	t, exists := l.timerMap[id]
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
		l.deleteTimer(id)
		if t.refed.Swap(false) {
			l.refedTimerCount.Add(-1)
		}
		return nil
	}
	// Timer is still pending in a deadline list — we own the cleanup.
	l.deleteTimer(id)
	if t.refed.Swap(false) {
		l.refedTimerCount.Add(-1)
	}
	l.unlinkTimerNode(t)
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
func (l *Loop) CancelTimers(ids ...TimerID) []error {
	if len(ids) == 0 {
		return nil
	}

	// Check if loop is in a valid state for cancellation.
	state := l.state.Load()
	if state == StateTerminated {
		errors := make([]error, len(ids))
		for i := range errors {
			errors[i] = ErrLoopTerminated
		}
		return errors
	}

	if l.ownsLocalQueues() {
		l.materializeCommandIngress()
		return l.applyCancelTimers(ids)
	}

	ids = slices.Clone(ids)

	if state == StateAwake {
		if errors, handled := l.cancelTimersBeforeRun(ids); handled {
			return errors
		}
	}

	result := make(chan []error, 1)

	// Submit as one timer lifecycle command so the batch cannot be overtaken by a
	// due timer phase after timer registrations have transferred from ingress.
	if err := l.enqueueCommand(loopCommand{kind: loopCommandTimerCancelBatch, ids: ids, results: result}, l.terminalQueueAllowed); err != nil {
		// submitToQueue failed, return error for all IDs
		errors := make([]error, len(ids))
		for i := range errors {
			errors[i] = err
		}
		return errors
	}
	if l.testHooks != nil && l.testHooks.AfterSynchronousTimerCommandPublish != nil {
		l.testHooks.AfterSynchronousTimerCommandPublish(loopCommandTimerCancelBatch)
	}

	return l.awaitCancelTimersResult(result, len(ids))
}

func (l *Loop) cancelTimersBeforeRun(ids []TimerID) ([]error, bool) {
	l.externalMu.Lock()
	if l.state.Load() != StateAwake {
		l.externalMu.Unlock()
		return nil, false
	}
	live := l.pendingTimerIDsLocked()
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
		l.enqueueCommandLocked(loopCommand{kind: loopCommandTimerCancelBatch, ids: ids})
	}
	l.externalMu.Unlock()
	if accepted {
		l.wakeAfterIngress()
	}
	return errors, true
}

func (l *Loop) pendingTimerIDsLocked() map[TimerID]struct{} {
	live := make(map[TimerID]struct{})
	if l.commands == nil {
		return live
	}
	for index := l.commands.head; index < len(l.commands.cmds); index++ {
		cmd := l.commands.cmds[index]
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

func (l *Loop) awaitCancelTimersResult(result <-chan []error, count int) []error {
	select {
	case res := <-result:
		return res
	case <-l.loopDone:
		select {
		case res := <-result:
			return res
		default:
		}
		if done, active := l.terminalDrainWaiter(); active {
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
func (l *Loop) applyCancelTimers(ids []TimerID) []error {
	errors := make([]error, len(ids))
	for i, id := range ids {
		errors[i] = l.applyCancelTimer(id)
	}
	return errors
}
