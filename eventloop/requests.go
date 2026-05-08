package eventloop

import "context"

// LoopRequests is a transferable capability for operations that acknowledge
// admission without waiting for loop-owner application or terminal cleanup.
// Obtain a value from [Loop.Requests]; the zero value is invalid.
//
// Request methods are intended for dependency goroutines that must return to a
// loop callback or Promisify worker before the owner can make progress. They do
// not weaken ordinary external Loop callers: lifecycle callers still join when
// their role permits, and post-Run timer mutations normally wait for owner
// application. Terminal dependency release may instead finish an admitted
// ordinary ref/unref as an unobservable successful no-op or return
// [ErrLoopTerminated] when an exact cancellation result is unavailable.
// Calling a method on the zero value panics.
type LoopRequests struct {
	loop *Loop
}

// Requests returns a transferable nonjoining request capability.
// It panics if the Loop receiver is nil.
func (x *Loop) Requests() LoopRequests {
	if x == nil {
		panic("eventloop: nil Loop")
	}
	return LoopRequests{loop: x}
}

// Shutdown requests graceful termination without joining terminal cleanup.
// A nil result acknowledges the committed graceful mode, not completed cleanup.
// It returns [ErrLoopTerminated] if immediate mode won or completion was already
// published.
func (r LoopRequests) Shutdown() error {
	return r.requireLoop().shutdownImpl(context.Background(), false)
}

// Close requests immediate termination without joining terminal cleanup.
// Unlike [Loop.Close], it is safe to call from a loop callback. A nil result
// acknowledges the committed immediate mode, not completed cleanup. It returns
// [ErrLoopTerminated] if graceful mode won or completion was already published.
func (r LoopRequests) Close() error {
	return r.requireLoop().closeImpl(false)
}

// CancelTimer requests an ordered timer cancellation without waiting for the
// owner to determine whether the timer exists. A nil result acknowledges
// admission only; unknown, fired, or already-cancelled timers are accepted as
// no-ops. Immediate Close may discard an admitted request.
func (r LoopRequests) CancelTimer(id TimerID) error {
	return r.requireLoop().queueTimerCancel(id)
}

// CancelTimers requests one ordered batch cancellation without waiting for
// per-timer results. A nil result acknowledges admission only; unknown, fired,
// already-cancelled, and duplicate IDs are accepted as ordered no-ops. The
// input IDs are cloned before publication. An empty request always succeeds.
func (r LoopRequests) CancelTimers(ids ...TimerID) error {
	return r.requireLoop().queueTimerCancels(ids)
}

// RefTimer requests an ordered referenced-state change without waiting for
// owner application. A nil result acknowledges admission only; an unknown,
// fired, or cancelled timer is an accepted no-op.
func (r LoopRequests) RefTimer(id TimerID) error {
	return r.requireLoop().queueTimerRefChange(id, true)
}

// UnrefTimer requests an ordered unreferenced-state change without waiting for
// owner application. A nil result acknowledges admission only; an unknown,
// fired, or cancelled timer is an accepted no-op.
func (r LoopRequests) UnrefTimer(id TimerID) error {
	return r.requireLoop().queueTimerRefChange(id, false)
}

func (r LoopRequests) requireLoop() *Loop {
	if r.loop == nil {
		panic("eventloop: invalid LoopRequests")
	}
	return r.loop
}
