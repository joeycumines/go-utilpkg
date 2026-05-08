package eventloop

import "time"

// YieldMicrotasks suspends the active nextTick / Promise-microtask checkpoint
// until the next task or check-phase boundary. It is a host-integration control
// seam for runtimes whose handled callback exceptions unwind the current
// checkpoint. The suspension is not a user callback and does not affect
// callback metrics.
//
// The caller must hold the logical loop owner role. Ordinary goroutines receive
// [ErrCallbackOwner]. A terminating loop receives [ErrLoopTerminated].
func (x *Loop) YieldMicrotasks() error {
	if x == nil {
		panic("eventloop: nil Loop")
	}
	state := x.state.Load()
	if state == StateTerminating || state == StateTerminated {
		return ErrLoopTerminated
	}
	if !x.ownsLocalQueues() {
		return ErrCallbackOwner
	}
	if x.microtaskYield.CompareAndSwap(false, true) {
		x.submissionEpoch.Add(1)
	}
	return nil
}

// RunMicrotaskCheckpoint exhausts the current nextTick, Promise-microtask, and
// checkpoint queues without entering a synthetic user callback or recording a
// callback metric. It is a host-integration seam for boundaries that Node.js
// treats as a microtask checkpoint even when no user callback was invoked.
//
// The caller must hold the logical loop or terminal-drain owner role. Ordinary
// goroutines receive [ErrCallbackOwner]. An active [Loop.YieldMicrotasks]
// suspension is preserved, leaving the queued work for its next task or check
// phase boundary.
//
// RunMicrotaskCheckpoint panics if the Loop receiver is nil.
func (x *Loop) RunMicrotaskCheckpoint() error {
	if x == nil {
		panic("eventloop: nil Loop")
	}
	if !x.ownsLocalQueues() {
		return ErrCallbackOwner
	}
	if x.hardAbortRequested() {
		return ErrLoopTerminated
	}
	x.drainMicrotasks()
	return nil
}

// AdvanceMicrotaskCheckpoint advances one host task-selection boundary without
// entering a synthetic user callback or recording a callback metric. If
// [Loop.YieldMicrotasks] suspended the current checkpoint, Advance consumes
// exactly that one suspension and leaves queued work pending. Otherwise it
// starts draining the current nextTick, Promise-microtask, and checkpoint
// queues. A callback handled during that drain may unwind it; Advance clears
// the resulting suspension before returning so the next host boundary drains
// the remainder instead of consuming a second skip.
//
// The caller must hold the logical loop or terminal-drain owner role. Ordinary
// goroutines receive [ErrCallbackOwner].
//
// AdvanceMicrotaskCheckpoint panics if the Loop receiver is nil.
func (x *Loop) AdvanceMicrotaskCheckpoint() error {
	if x == nil {
		panic("eventloop: nil Loop")
	}
	if !x.ownsLocalQueues() {
		return ErrCallbackOwner
	}
	if x.hardAbortRequested() {
		return ErrLoopTerminated
	}
	if x.microtaskYield.CompareAndSwap(true, false) {
		return nil
	}
	x.drainMicrotasks()
	x.releaseMicrotaskYield()
	return nil
}

// ResumeMicrotaskCheckpoint clears an active [Loop.YieldMicrotasks]
// suspension and exhausts the current nextTick, Promise-microtask, and
// checkpoint queues without entering a synthetic user callback or recording a
// callback metric. It is the forced phase-exit counterpart to
// [Loop.RunMicrotaskCheckpoint]; hosts should use it only after publishing all
// task-selection and liveness state for the phase being exited.
//
// The caller must hold the logical loop or terminal-drain owner role. Ordinary
// goroutines receive [ErrCallbackOwner].
//
// ResumeMicrotaskCheckpoint panics if the Loop receiver is nil.
func (x *Loop) ResumeMicrotaskCheckpoint() error {
	if x == nil {
		panic("eventloop: nil Loop")
	}
	if !x.ownsLocalQueues() {
		return ErrCallbackOwner
	}
	if x.hardAbortRequested() {
		return ErrLoopTerminated
	}
	x.releaseMicrotaskYield()
	x.drainMicrotasks()
	return nil
}

func (x *Loop) releaseMicrotaskYield() {
	x.microtaskYield.Store(false)
}

func (x *Loop) releaseMicrotaskYieldAtEmptyCheck() {
	if !x.microtaskYield.Load() {
		return
	}
	deadline, timerPending := x.nextTimerDeadline()
	timerReady := timerPending && !deadline.After(time.Now())
	if timerReady ||
		x.commandIngressPending.Load() ||
		x.ownerInternalCount.Load() != 0 ||
		x.ownerExternalCount.Load() != 0 ||
		x.ownerCloseCount.Load() != 0 {
		return
	}
	x.releaseMicrotaskYield()
}
