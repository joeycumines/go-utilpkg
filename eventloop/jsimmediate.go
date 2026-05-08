package eventloop

import (
	"sync/atomic"
)

// setImmediateState tracks a single setImmediate callback
type setImmediateState struct {
	fn          setTimeoutFunc
	js          *JS
	publication callbackPublication
	id          uint64
	cleared     atomic.Bool // cancellation-or-execution claim
}

// SetImmediate schedules a function asynchronously in the event loop's check
// phase. A callback admitted before the current check-phase snapshot may run in
// the current iteration; one admitted from a running check callback rolls over
// to a later iteration.
//
// Unlike [JS.SetTimeout] with 0ms delay, it bypasses timer deadline scheduling,
// uses [Loop.ScheduleImmediate] directly, and retains check-phase ordering.
//
// The callback never runs inline with SetImmediate and cannot enter until its
// adapter handle has been published and scheduling has passed the final
// terminal-state check.
//
// Returns:
//   - ID that can be passed to [JS.ClearImmediate] to cancel
//   - Error if the loop is shutting down or all immediate IDs have been exhausted
//
// SetImmediate panics if fn is nil.
func (js *JS) SetImmediate(fn setTimeoutFunc) (uint64, error) {
	if fn == nil {
		panic("eventloop: nil SetImmediate callback")
	}

	id, ok := allocateID(&js.nextImmediateID, maxSafeInteger)
	if !ok {
		return 0, ErrImmediateIDExhausted
	}

	state := &setImmediateState{
		fn:          fn,
		js:          js,
		id:          id,
		publication: newCallbackPublication(),
	}

	js.loop.livenessMu.Lock()
	loopState := js.loop.state.Load()
	if loopState == StateTerminating || loopState == StateTerminated {
		js.loop.livenessMu.Unlock()
		return 0, ErrLoopTerminated
	}
	js.setImmediateMu.Lock()
	js.setImmediateMap = retainedMapStore(js.setImmediateMap, &js.immediatesRetention, id, state)
	js.setImmediateMu.Unlock()
	js.loop.livenessMu.Unlock()

	if err := js.loop.ScheduleImmediate(state.run); err != nil {
		state.cleared.Store(true)
		js.setImmediateMu.Lock()
		state.fn = nil
		js.setImmediateMap, _ = retainedMapDelete(js.setImmediateMap, &js.immediatesRetention, id)
		js.setImmediateMu.Unlock()
		state.publication.release()
		return 0, err
	}
	if js.loop.testHooks != nil && js.loop.testHooks.BeforeJSImmediateReturn != nil {
		js.loop.testHooks.BeforeJSImmediateReturn(id)
	}
	js.loop.livenessMu.Lock()
	loopState = js.loop.state.Load()
	if loopState == StateTerminating || loopState == StateTerminated {
		state.cleared.Store(true)
		js.setImmediateMu.Lock()
		state.fn = nil
		js.setImmediateMap, _ = retainedMapDelete(js.setImmediateMap, &js.immediatesRetention, id)
		js.setImmediateMu.Unlock()
		state.publication.release()
		js.loop.livenessMu.Unlock()
		return 0, ErrLoopTerminated
	}
	state.publication.release()
	js.loop.livenessMu.Unlock()

	return id, nil
}

// ClearImmediate cancels a pending setImmediate task.
//
// Returns [ErrImmediateNotFound] if the ID is invalid or callback execution has
// already claimed it. A nil result means cancellation won before callback entry
// and the callback will not run.
func (js *JS) ClearImmediate(id uint64) error {
	js.setImmediateMu.Lock()
	state, ok := js.setImmediateMap[id]
	if !ok || !state.cleared.CompareAndSwap(false, true) {
		js.setImmediateMu.Unlock()
		return ErrImmediateNotFound
	}
	state.fn = nil
	js.setImmediateMap, _ = retainedMapDelete(js.setImmediateMap, &js.immediatesRetention, id)
	js.setImmediateMu.Unlock()

	return nil
}

// run claims and executes the setImmediate callback unless cancellation won.
func (s *setImmediateState) run() {
	if s.js.loop.testHooks != nil && s.js.loop.testHooks.BeforeJSImmediatePublicationWait != nil {
		s.js.loop.testHooks.BeforeJSImmediatePublicationWait()
	}
	s.publication.wait()

	// Registry ownership and the state claim form one linearization point with
	// ClearImmediate. Deleting before callback entry makes a later clear report
	// ErrImmediateNotFound instead of claiming cancellation after execution won.
	s.js.setImmediateMu.Lock()
	current, ok := s.js.setImmediateMap[s.id]
	if !ok || current != s || !s.cleared.CompareAndSwap(false, true) {
		s.js.setImmediateMu.Unlock()
		return
	}
	callback := s.fn
	s.fn = nil
	s.js.setImmediateMap, _ = retainedMapDelete(s.js.setImmediateMap, &s.js.immediatesRetention, s.id)
	s.js.setImmediateMu.Unlock()

	if s.js.loop.testHooks != nil && s.js.loop.testHooks.BeforeJSImmediateCallbackEntry != nil {
		s.js.loop.testHooks.BeforeJSImmediateCallbackEntry(s.id)
	}
	callback()
}
