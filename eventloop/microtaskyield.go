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
func (l *Loop) YieldMicrotasks() error {
	if l == nil {
		panic("eventloop: nil Loop")
	}
	state := l.state.Load()
	if state == StateTerminating || state == StateTerminated {
		return ErrLoopTerminated
	}
	if !l.ownsLocalQueues() {
		return ErrCallbackOwner
	}
	if l.microtaskYield.CompareAndSwap(false, true) {
		l.submissionEpoch.Add(1)
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
func (l *Loop) RunMicrotaskCheckpoint() error {
	if l == nil {
		panic("eventloop: nil Loop")
	}
	if !l.ownsLocalQueues() {
		return ErrCallbackOwner
	}
	if l.hardAbortRequested() {
		return ErrLoopTerminated
	}
	l.drainMicrotasks()
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
func (l *Loop) AdvanceMicrotaskCheckpoint() error {
	if l == nil {
		panic("eventloop: nil Loop")
	}
	if !l.ownsLocalQueues() {
		return ErrCallbackOwner
	}
	if l.hardAbortRequested() {
		return ErrLoopTerminated
	}
	if l.microtaskYield.CompareAndSwap(true, false) {
		return nil
	}
	l.drainMicrotasks()
	l.releaseMicrotaskYield()
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
func (l *Loop) ResumeMicrotaskCheckpoint() error {
	if l == nil {
		panic("eventloop: nil Loop")
	}
	if !l.ownsLocalQueues() {
		return ErrCallbackOwner
	}
	if l.hardAbortRequested() {
		return ErrLoopTerminated
	}
	l.releaseMicrotaskYield()
	l.drainMicrotasks()
	return nil
}

func (l *Loop) releaseMicrotaskYield() {
	l.microtaskYield.Store(false)
}

func (l *Loop) releaseMicrotaskYieldAtEmptyCheck() {
	if !l.microtaskYield.Load() {
		return
	}
	deadline, timerPending := l.nextTimerDeadline()
	timerReady := timerPending && !deadline.After(time.Now())
	if timerReady ||
		l.commandIngressPending.Load() ||
		l.ownerInternalCount.Load() != 0 ||
		l.ownerExternalCount.Load() != 0 ||
		l.ownerCloseCount.Load() != 0 {
		return
	}
	l.releaseMicrotaskYield()
}
