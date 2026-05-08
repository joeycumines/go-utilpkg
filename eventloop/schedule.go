package eventloop

// Done returns a read-only signal that is closed after terminal cleanup has
// completed and no callback accepted by this Loop can still execute.
//
// Done is stable for the lifetime of the Loop. It is intended for integrations
// that must release work whose owner callback was admitted before an immediate
// terminal transition but was discarded by that transition.
func (x *Loop) Done() <-chan struct{} {
	if x == nil {
		panic("eventloop: nil Loop")
	}
	return x.terminalDone
}

// Submit admits a task to the external task phase.
//
// State Policy during shutdown:
//   - StateTerminated: returns ErrLoopTerminated
//   - StateTerminating: returns ErrLoopTerminated. Terminal drain allows only
//     microtask/nextTick/checkpoint continuations, not new Submit work.
//   - StateSleeping/StateRunning: normal operation
//
// Quiescing Protocol: Submit is intentionally NOT gated by the quiescing flag.
// Submitted tasks are ephemeral work detected by Alive() via the submissionEpoch
// mechanism. If a task is submitted during the quiescing window, the epoch change
// causes the Alive() re-check to abort termination, and the task executes normally
// in the next tick. Adding a quiescing check here would be harmful: it would reject
// work that correctly prevents the (now-invalid) termination.
//
// Thread Safety: Uses a mutex-protected atomic state-check and typed-command
// publication. The loop owner transfers accepted commands to its local queue.
// Submit panics if task is nil.
func (x *Loop) Submit(task func()) error {
	if task == nil {
		panic("eventloop: nil Submit callback")
	}
	return x.enqueueTerminalCommand(loopCommand{kind: loopCommandExternal, fn: task})
}

// ScheduleImmediate schedules fn into the loop's check phase. Check callbacks
// run after the poll phase in a phase snapshot; callbacks scheduled from a
// running check callback roll over to a later iteration. This is the primitive
// used by JavaScript setImmediate bindings.
// ScheduleImmediate panics if fn is nil.
func (x *Loop) ScheduleImmediate(fn func()) error {
	return x.ScheduleImmediateRef(fn, nil)
}

// ScheduleImmediateRef schedules fn into the loop's check phase with a dynamic
// liveness predicate. When refed is nil or returns true, the pending check
// callback contributes to [Loop.Alive]. When refed returns false, the callback
// may still run if other ref'd work keeps the loop alive, but it will not stop
// auto-exit on its own.
//
// Only the loop or terminal-drain owner evaluates refed; external liveness
// observers conservatively retain the pending callback without invoking it.
// The predicate may be evaluated multiple times during one auto-exit decision
// and runs outside loop locks, but on the owner goroutine. It must not recurse
// into [Loop.Alive] or [Loop.HasMacrotaskWork], or block waiting for owner work.
// Scheduling APIs may be called: an accepted mutation advances the loop epoch
// and invalidates a stale auto-exit decision. If the final owner evaluation is
// false and no loop mutation invalidates it, that result is the auto-exit
// linearization point; changing only captured predicate state afterward does
// not revive the callback.
// ScheduleImmediateRef panics if fn is nil. A nil refed predicate means the
// callback is ref'd.
func (x *Loop) ScheduleImmediateRef(fn func(), refed func() bool) error {
	if fn == nil {
		panic("eventloop: nil ScheduleImmediateRef callback")
	}
	if x.ownsLocalQueues() {
		state := LoopState(x.state.Load())
		if !x.terminalQueueAllowed(state) {
			return ErrLoopTerminated
		}
		x.pushOwnerCheck(checkJob{fn: fn, refed: refed, seq: x.phaseSeq.Add(1)})
		x.submissionEpoch.Add(1)
		return nil
	}
	return x.enqueueCommand(loopCommand{kind: loopCommandImmediate, fn: fn, refed: refed}, x.terminalQueueAllowed)
}

// ScheduleCloseCallback schedules fn into the close-callback phase. Close
// callbacks run after the check phase in the same loop iteration when the loop
// is already alive; callbacks queued while the loop is otherwise idle keep the
// loop alive and run before auto-exit commits.
// ScheduleCloseCallback panics if fn is nil.
func (x *Loop) ScheduleCloseCallback(fn func()) error {
	if fn == nil {
		panic("eventloop: nil ScheduleCloseCallback callback")
	}
	if x.ownsLocalQueues() {
		state := LoopState(x.state.Load())
		if !x.terminalQueueAllowed(state) {
			return ErrLoopTerminated
		}
		x.pushOwnerClose(checkJob{fn: fn, seq: x.phaseSeq.Add(1)})
		x.submissionEpoch.Add(1)
		return nil
	}
	return x.enqueueCommand(loopCommand{kind: loopCommandClose, fn: fn}, x.terminalQueueAllowed)
}

// doWakeup attempts both the channel and physical wake signals. This handles
// the race where canUseFastPath()
// disagrees with the loop's actual poll path (e.g., mode=Forced + count>0 due
// to concurrent RegisterFD/SetFastPathMode). On platforms without public FD
// polling, submitWakeup is a no-op and the channel is the sole wait primitive.
func (x *Loop) doWakeup() {
	// Always try channel wakeup (covers fast path mode)
	select {
	case x.fastWakeupCh <- struct{}{}:
	default:
		// Channel already has pending wakeup
	}

	// Always try pipe/eventfd wakeup (covers I/O poll mode)
	// This is unconditional to prevent lost wakeups when mode and count
	// are transiently inconsistent due to concurrent SetFastPathMode/RegisterFD.
	_ = x.submitWakeup()
}

// wakeAfterIngress wakes whichever wait primitive the loop is currently using
// after work has already been admitted to a queue. The decision is deliberately
// post-admission: mode and FD counts may have changed while the caller was
// waiting to commit the task. A buffered fast wake covers fast-channel waiters;
// a physical wake is added whenever the loop can be blocked in PollIO, either
// because a user FD is registered or FastPathDisabled explicitly selects native
// polling for a task-only loop. FD-count and mode transitions perform their own
// physical wakeups, so ordinary task-only submissions do not pay the pipe/eventfd
// cost.
func (x *Loop) wakeAfterIngress() {
	select {
	case x.fastWakeupCh <- struct{}{}:
	default:
	}

	if x.state.Load() == StateSleeping && x.nativePollSelected() {
		_ = x.submitPendingWakeup()
	}
}

func (x *Loop) nativePollSelected() bool {
	return x.userIOFDCount.Load() > 0 || FastPathMode(x.fastPathMode.Load()) == FastPathDisabled
}

func (x *Loop) forceWakeup() {
	select {
	case x.fastWakeupCh <- struct{}{}:
	default:
	}
	_ = x.submitWakeupPhysical()
}

// SubmitInternal admits a task to the internal priority phase.
//
// State Policy during shutdown:
//   - StateTerminated: returns ErrLoopTerminated
//   - StateTerminating: returns ErrLoopTerminated. Terminal drain allows only
//     microtask/nextTick/checkpoint continuations, not new SubmitInternal work.
//   - StateSleeping/StateRunning: normal operation
//
// Thread Safety: Owner calls append directly to the local priority queue;
// external calls use mutex-protected typed command ingress.
// SubmitInternal panics if task is nil.
func (x *Loop) SubmitInternal(task func()) error {
	if task == nil {
		panic("eventloop: nil SubmitInternal callback")
	}
	return x.submitToQueue(task)
}

// submitToQueue admits a task to the internal phase and wakes the loop.
// The logical owner appends directly; other callers use typed ingress.
func (x *Loop) submitToQueue(task func()) error {
	if x.ownsLocalQueues() {
		state := LoopState(x.state.Load())
		if !x.terminalQueueAllowed(state) {
			return ErrLoopTerminated
		}
		x.materializeCommandIngress()
		x.pushOwnerInternal(task)
		x.submissionEpoch.Add(1)
		return nil
	}
	return x.enqueueTerminalCommand(loopCommand{kind: loopCommandInternal, fn: task})
}

func (x *Loop) submitLivenessCommand(cmd loopCommand, beforeCommit func()) error {
	x.livenessMu.Lock()
	defer x.livenessMu.Unlock()
	if beforeCommit != nil {
		beforeCommit()
	}
	if err := x.rejectLivenessAddLocked(); err != nil {
		return err
	}
	return x.enqueueCommand(cmd, x.terminalQueueAllowed)
}

// Wake attempts to wake up the loop from a suspended state.
//
// State Policy:
//   - StateSleeping: performs wake-up (if not already pending)
//   - StateTerminated: returns nil (no-op on terminated loop)
//   - StateRunning: publishes both selected wait signals so a concurrent
//     Running-to-Sleeping transition cannot overtake the wake
//   - StateTerminating/StateAwake: returns nil
//
// A physical wake submission failure is returned to the caller. The pending
// claim is reopened so a later Wake or ingress operation can retry.
func (x *Loop) Wake() error {
	state := x.state.Load()
	if state != StateRunning && state != StateSleeping {
		return nil
	}

	select {
	case x.fastWakeupCh <- struct{}{}:
	default:
	}
	if !x.nativePollSelected() {
		return nil
	}
	err := x.submitPendingWakeup()
	if err == ErrLoopTerminated {
		return nil
	}
	return err
}

// ScheduleMicrotask schedules a microtask.
//
// Returns:
//   - ErrLoopTerminated if the loop has been shut down, except while an
//     already-accepted callback owner is extending the active graceful
//     checkpoint. Owner continuations remain admissible until that checkpoint
//     exhausts.
//
// Quiescing Protocol: ScheduleMicrotask is intentionally NOT gated by the quiescing
// flag. Microtasks are ephemeral work detected by Alive() via the submissionEpoch
// mechanism. If a microtask is scheduled during the quiescing window, the epoch change
// causes the Alive() re-check to abort termination. Adding a quiescing check here
// would be harmful: it would reject work that correctly prevents termination.
//
// ScheduleMicrotask panics if fn is nil.
func (x *Loop) ScheduleMicrotask(fn func()) error {
	if fn == nil {
		panic("eventloop: nil ScheduleMicrotask callback")
	}

	state := LoopState(x.state.Load())
	if !x.terminalMicrotaskAllowed(state) {
		return ErrLoopTerminated
	}
	if x.ownsLocalQueues() {
		x.materializeCommandIngress()
		x.pushOwnerPromiseMicrotask(fn, nil)
		x.submissionEpoch.Add(1)
		return nil
	}

	return x.enqueueCommand(loopCommand{kind: loopCommandMicrotask, fn: fn}, x.terminalMicrotaskAllowed)
}

// schedulePromiseReaction queues an internal Promise reaction together with
// the child identity that receives an explicit terminal disposition if normal
// callback admission later discards the accepted microtask.
func (x *Loop) schedulePromiseReaction(fn func(), reaction *ChainedPromise) error {
	if fn == nil || reaction == nil {
		return nil
	}

	state := LoopState(x.state.Load())
	if !x.terminalMicrotaskAllowed(state) {
		return ErrLoopTerminated
	}
	if x.ownsLocalQueues() {
		x.materializeCommandIngress()
		x.pushOwnerPromiseMicrotask(fn, reaction)
		x.submissionEpoch.Add(1)
		return nil
	}

	return x.enqueueCommand(loopCommand{
		kind:     loopCommandMicrotask,
		fn:       fn,
		reaction: reaction,
	}, x.terminalMicrotaskAllowed)
}

// scheduleMicrotaskCheckpoint schedules a loop-internal callback that runs
// after the current microtask checkpoint exhausts both nextTick and promise
// microtask queues. It follows ScheduleMicrotask's state, liveness, and wakeup
// rules, but keeps checkpoint-end diagnostics out of the public FIFO promise
// microtask queue so they cannot starve nextTick callbacks scheduled by promise
// microtasks.
func (x *Loop) scheduleMicrotaskCheckpoint(fn func()) error {
	if fn == nil {
		return nil
	}

	state := LoopState(x.state.Load())
	if !x.terminalMicrotaskAllowed(state) {
		return ErrLoopTerminated
	}
	if x.ownsLocalQueues() {
		x.materializeCommandIngress()
		x.pushOwnerMicrotask(x.ownerCheckpt, fn, false)
		x.submissionEpoch.Add(1)
		return nil
	}

	return x.enqueueCommand(loopCommand{kind: loopCommandCheckpoint, fn: fn}, x.terminalMicrotaskAllowed)
}

// ScheduleMicrotaskCheckpoint schedules a loop-internal-style callback that runs
// after the current microtask checkpoint exhausts both nextTick and promise
// microtask queues. It is intended for host integrations that need Node-style
// checkpoint-end diagnostics, such as unhandled rejection reporting. The
// callback runs on the logical callback-owner goroutine and follows the same
// terminal-drain rules as [Loop.ScheduleMicrotask].
// ScheduleMicrotaskCheckpoint panics if fn is nil.
func (x *Loop) ScheduleMicrotaskCheckpoint(fn func()) error {
	if fn == nil {
		panic("eventloop: nil ScheduleMicrotaskCheckpoint callback")
	}
	return x.scheduleMicrotaskCheckpoint(fn)
}

// scheduleMicrotask adds a task to the microtask queue (internal use).
//
// Used by platform-specific tests (regression_test.go, linux/darwin only).
//
//lint:ignore U1000 Used by platform-specific test files with build constraints.
func (x *Loop) scheduleMicrotask(task func()) {
	if task != nil {
		x.materializeCommandIngress()
		x.pushOwnerPromiseMicrotask(task, nil)
	}
}

// ScheduleNextTick schedules a function in the nextTick queue.
//
// This emulates Node.js process.nextTick() semantics. At a microtask checkpoint,
// all pending NextTick callbacks run before the next Promise / queueMicrotask
// batch. A NextTick admitted during an active Promise batch waits until that
// batch exhausts rather than preempting its remaining callbacks.
//
// Unlike setTimeout(fn, 0) which schedules for the next tick, NextTick callbacks
// queued by synchronous code execute at its checkpoint before pending Promise
// handlers.
//
// Returns:
//   - ErrLoopTerminated if the loop has been shut down, except while an
//     already-accepted callback owner is extending the active graceful
//     checkpoint. Owner continuations remain admissible until that checkpoint
//     exhausts.
//
// Quiescing Protocol: ScheduleNextTick is intentionally NOT gated by the quiescing
// flag. nextTick callbacks are ephemeral work detected by Alive() via the
// submissionEpoch mechanism. If a callback is scheduled during the quiescing window,
// the epoch change causes the Alive() re-check to abort termination. Adding a
// quiescing check here would be harmful: it would reject work that correctly
// prevents termination.
//
// Thread Safety: Safe to call from any goroutine.
// ScheduleNextTick panics if fn is nil.
func (x *Loop) ScheduleNextTick(fn func()) error {
	if fn == nil {
		panic("eventloop: nil ScheduleNextTick callback")
	}

	state := LoopState(x.state.Load())
	if !x.terminalMicrotaskAllowed(state) {
		return ErrLoopTerminated
	}
	if x.ownsLocalQueues() {
		x.materializeCommandIngress()
		x.pushOwnerMicrotask(x.ownerNextTick, fn, true)
		x.submissionEpoch.Add(1)
		return nil
	}

	return x.enqueueCommand(loopCommand{kind: loopCommandNextTick, fn: fn}, x.terminalMicrotaskAllowed)
}
