package eventloop

import (
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

type callbackPublication struct {
	ready chan struct{}
	once  sync.Once
}

func newCallbackPublication() callbackPublication {
	return callbackPublication{ready: make(chan struct{})}
}

func (p *callbackPublication) wait() {
	<-p.ready
}

func (p *callbackPublication) release() {
	p.once.Do(func() { close(p.ready) })
}

// intervalState tracks the state of an interval timer.
// It is stored in js.intervals map (uint64 -> *intervalState)
type intervalState struct {
	callback    setTimeoutFunc
	publication callbackPublication
	callbackMu  sync.Mutex

	currentLoopTimerID atomic.Uint64
	canceled           atomic.Bool
}

func (s *intervalState) claimCallback() setTimeoutFunc {
	s.callbackMu.Lock()
	callback := s.callback
	s.callbackMu.Unlock()
	return callback
}

func (s *intervalState) clearCallback() {
	s.callbackMu.Lock()
	s.callback = nil
	s.callbackMu.Unlock()
}

type timeoutStatus uint8

const (
	timeoutPending timeoutStatus = iota
	timeoutFired
	timeoutCleared
)

type timeoutState struct {
	fn          setTimeoutFunc
	publication callbackPublication
	loopTimerID TimerID
	status      timeoutStatus
}

// setTimeoutFunc is a callback function for [JS.SetTimeout] and [JS.SetInterval].
// During normal [Loop.Run] execution, the callback is invoked on the logical
// callback-owner goroutine.
type setTimeoutFunc func()

// SetTimeout schedules a one-time host callback after a millisecond delay.
//
// Parameters:
//   - fn: The callback to execute.
//   - delayMs: Delay in milliseconds. Values < 0 are normalized to 0 before scheduling.
//
// Returns:
//   - Timer ID that can be passed to [JS.ClearTimeout] to cancel
//   - Error if the loop is shutting down or has been closed
//
// During normal [Loop.Run] execution, the callback runs on the logical
// callback-owner goroutine. The adapter handle and native timer ID are fully
// published before callback entry. If the delay is 0, the callback is still
// scheduled (not executed synchronously) but will run after all pending
// microtasks are processed.
//
// SetTimeout panics if fn is nil or a non-negative delay cannot be represented
// as a [time.Duration].
func (js *JS) SetTimeout(fn setTimeoutFunc, delayMs int) (uint64, error) {
	return js.setTimeout(fn, delayMs, true)
}

// SetTimeoutUnrefed schedules a one-time host callback whose underlying timer
// is unreferenced from its initial publication. It remains eligible while
// other referenced work keeps the loop alive, but never creates a transient
// liveness claim visible to concurrent Loop observers.
//
// SetTimeoutUnrefed panics if fn is nil or a non-negative delay cannot be
// represented as a [time.Duration].
func (js *JS) SetTimeoutUnrefed(fn setTimeoutFunc, delayMs int) (uint64, error) {
	return js.setTimeout(fn, delayMs, false)
}

func (js *JS) setTimeout(fn setTimeoutFunc, delayMs int, refed bool) (uint64, error) {
	if fn == nil {
		panic("eventloop: nil SetTimeout callback")
	}

	delay := timerDuration(delayMs)

	id, ok := allocateID(&js.nextTimerID, maxSafeInteger)
	if !ok {
		return 0, ErrTimerIDExhausted
	}

	timeout := &timeoutState{fn: fn, publication: newCallbackPublication()}
	schedule := js.loop.ScheduleTimer
	if !refed {
		schedule = js.loop.ScheduleTimerUnrefed
	}
	loopTimerID, err := schedule(delay, func() {
		if js.loop.testHooks != nil && js.loop.testHooks.BeforeJSTimeoutPublicationWait != nil {
			js.loop.testHooks.BeforeJSTimeoutPublicationWait()
		}
		timeout.publication.wait()
		if js.loop.testHooks != nil && js.loop.testHooks.BeforeJSTimeoutCallbackClaim != nil {
			js.loop.testHooks.BeforeJSTimeoutCallbackClaim()
		}
		js.timeoutsMu.Lock()
		if timeout.status != timeoutPending {
			js.timeoutsMu.Unlock()
			return
		}
		timeout.status = timeoutFired
		callback := timeout.fn
		timeout.fn = nil
		if current, ok := js.timeouts[id]; ok && current == timeout {
			js.timeouts, _ = retainedMapDelete(js.timeouts, &js.timeoutsRetention, id)
		}
		js.timeoutsMu.Unlock()
		callback()
	})
	if err != nil {
		return 0, err
	}
	timeout.loopTimerID = loopTimerID

	if js.loop != nil && js.loop.testHooks != nil && js.loop.testHooks.BeforeJSTimeoutRegistryPublish != nil {
		js.loop.testHooks.BeforeJSTimeoutRegistryPublish(id)
	}

	js.loop.livenessMu.Lock()
	state := js.loop.state.Load()
	if state == StateTerminating || state == StateTerminated {
		js.timeoutsMu.Lock()
		timeout.status = timeoutCleared
		timeout.fn = nil
		js.timeoutsMu.Unlock()
		timeout.publication.release()
		js.loop.livenessMu.Unlock()
		return 0, ErrLoopTerminated
	}
	js.timeoutsMu.Lock()
	if timeout.status == timeoutPending {
		js.timeouts = retainedMapStore(js.timeouts, &js.timeoutsRetention, id, timeout)
	}
	js.timeoutsMu.Unlock()
	timeout.publication.release()
	js.loop.livenessMu.Unlock()

	return id, nil
}

// ClearTimeout cancels a scheduled timeout timer by its ID.
//
// Returns [ErrTimerNotFound] if the timer ID is invalid or has already fired,
// or [ErrLoopTerminated] if terminal transition prevents owner cancellation.
// A successful call synchronously claims the adapter handle, guarantees the
// callback will not begin afterward, and returns after owner cancellation is
// accepted without waiting for the loop owner. Repeated calls are safe and
// return [ErrTimerNotFound].
func (js *JS) ClearTimeout(id uint64) error {
	if err := js.clearTimeoutOnly(id); err == nil || !errors.Is(err, ErrTimerNotFound) {
		return err
	}
	return js.clearIntervalOnly(id)
}

// UnrefTimeout marks a timeout as unref'd. The timeout remains scheduled, but
// its underlying timer no longer contributes to [Loop.Alive]. When only unref'd
// work remains, an auto-exit loop may terminate before the timeout fires.
// The owner command is accepted in timer-lifecycle FIFO order without waiting
// for the loop owner to apply it. Returns [ErrLoopTerminated] if terminal
// transition prevents admission.
func (js *JS) UnrefTimeout(id uint64) error {
	js.timeoutsMu.Lock()
	state, ok := js.timeouts[id]
	if !ok || state.status != timeoutPending {
		js.timeoutsMu.Unlock()
		return ErrTimerNotFound
	}
	loopTimerID := state.loopTimerID
	js.timeoutsMu.Unlock()
	return js.loop.queueTimerRefChange(loopTimerID, false)
}

// RefTimeout reverses a previous [JS.UnrefTimeout] call, making the timeout's
// underlying timer contribute to [Loop.Alive] again.
// The owner command is accepted in timer-lifecycle FIFO order without waiting
// for the loop owner to apply it. Returns [ErrLoopTerminated] if terminal
// transition prevents admission.
func (js *JS) RefTimeout(id uint64) error {
	js.timeoutsMu.Lock()
	state, ok := js.timeouts[id]
	if !ok || state.status != timeoutPending {
		js.timeoutsMu.Unlock()
		return ErrTimerNotFound
	}
	loopTimerID := state.loopTimerID
	js.timeoutsMu.Unlock()
	return js.loop.queueTimerRefChange(loopTimerID, true)
}

func (js *JS) clearTimeoutOnly(id uint64) error {
	js.timeoutsMu.Lock()
	state, ok := js.timeouts[id]
	if !ok || state.status != timeoutPending {
		js.timeoutsMu.Unlock()
		return ErrTimerNotFound
	}
	state.status = timeoutCleared
	state.fn = nil
	js.timeouts, _ = retainedMapDelete(js.timeouts, &js.timeoutsRetention, id)
	js.timeoutsMu.Unlock()

	return js.loop.queueTimerCancel(state.loopTimerID)
}

// SetInterval schedules a function to run repeatedly with a fixed delay.
//
// Parameters:
//   - fn: The callback to execute.
//   - delayMs: Interval in milliseconds between executions. Values < 0 are normalized to 0 before scheduling.
//
// Returns:
//   - Timer ID that can be passed to [JS.ClearInterval] to cancel
//   - Error if the loop is shutting down or has been closed
//
// The callback will continue to fire at the specified interval until
// [JS.ClearInterval] is called with the returned ID. Each repeat is anchored to
// the start of the previous callback, matching Node.js repeating timers without
// accumulating callback-duration drift. An overdue repeat remains ineligible
// until a later loop turn. The adapter handle and stable native repeating-timer
// ID are fully published before the first callback entry.
//
// SetInterval panics if fn is nil or a non-negative delay cannot be represented
// as a [time.Duration].
func (js *JS) SetInterval(fn setTimeoutFunc, delayMs int) (uint64, error) {
	if fn == nil {
		panic("eventloop: nil SetInterval callback")
	}

	delay := timerDuration(delayMs)

	// IMPORTANT: Assign id BEFORE any scheduling.
	id, ok := allocateID(&js.nextTimerID, maxSafeInteger)
	if !ok {
		return 0, ErrTimerIDExhausted
	}

	// One adapter state owns the stable native repeating timer for its full
	// lifetime. The core timer is refed by default.
	state := &intervalState{publication: newCallbackPublication(), callback: fn}

	// Create wrapper function for the native repeating timer.
	wrapper := func() {
		if js.loop.testHooks != nil && js.loop.testHooks.BeforeJSIntervalPublicationWait != nil {
			js.loop.testHooks.BeforeJSIntervalPublicationWait()
		}
		state.publication.wait()
		// Adapter cancellation claims the handle before owner cancellation is
		// queued, so a callback that has not passed this boundary is suppressed.
		if state.canceled.Load() {
			return
		}
		callback := state.claimCallback()
		if callback == nil {
			return
		}
		if js.loop.testHooks != nil && js.loop.testHooks.BeforeJSIntervalCallbackEntry != nil {
			js.loop.testHooks.BeforeJSIntervalCallbackEntry(id)
		}
		callback()
	}

	loopTimerID, err := js.loop.scheduleRepeatingTimer(delay, wrapper)
	if err != nil {
		return 0, err
	}

	if js.loop != nil && js.loop.testHooks != nil && js.loop.testHooks.BeforeJSIntervalTimerIDPublish != nil {
		js.loop.testHooks.BeforeJSIntervalTimerIDPublish(id, state, loopTimerID)
	}

	js.loop.livenessMu.Lock()
	loopState := js.loop.state.Load()
	if loopState == StateTerminating || loopState == StateTerminated {
		state.canceled.Store(true)
		state.clearCallback()
		state.publication.release()
		js.loop.livenessMu.Unlock()
		return 0, ErrLoopTerminated
	}

	// Publish the native repeating loop timer and adapter handle atomically with
	// terminal cleanup. Public callers cannot cancel an unpublished JS handle;
	// terminal cleanup is the only competing pre-publication invalidation path
	// and is serialized by livenessMu.
	state.currentLoopTimerID.Store(uint64(loopTimerID))

	// Store interval state in global map with initial mapping
	js.intervalsMu.Lock()
	js.intervals = retainedMapStore(js.intervals, &js.intervalsRetention, id, state)
	js.intervalsMu.Unlock()
	state.publication.release()
	js.loop.livenessMu.Unlock()

	// NOTE: Intervals are managed exclusively through js.intervals map
	// ClearInterval loads state from js.intervals and reads state.currentLoopTimerID
	// We do NOT create a js.timers entry for intervals

	return id, nil
}

func timerDuration(delayMs int) time.Duration {
	if delayMs < 0 {
		return 0
	}
	const maxDelayMilliseconds = math.MaxInt64 / int64(time.Millisecond)
	if uint64(delayMs) > uint64(maxDelayMilliseconds) {
		panic("eventloop: timer delay overflows time.Duration")
	}
	return time.Duration(delayMs) * time.Millisecond
}

// ClearInterval cancels a scheduled interval timer by its ID.
//
// Returns [ErrTimerNotFound] if the timer ID is invalid, or
// [ErrLoopTerminated] if terminal transition prevents owner cancellation.
// This is safe to call from any goroutine, including from within
// the interval's own callback.
//
// A successful call synchronously claims the adapter handle and queues owner
// cancellation without waiting for the loop owner. A wrapper that observes
// the cancellation flag is suppressed. A wrapper that passed that check before
// the call claimed the handle may still enter or finish its current callback
// after this method returns, but owner cancellation prevents a later repeat.
func (js *JS) ClearInterval(id uint64) error {
	if err := js.clearIntervalOnly(id); err == nil || !errors.Is(err, ErrTimerNotFound) {
		return err
	}
	return js.clearTimeoutOnly(id)
}

func (js *JS) clearIntervalOnly(id uint64) error {
	js.intervalsMu.Lock()
	state, ok := js.intervals[id]
	if !ok {
		js.intervalsMu.Unlock()
		return ErrTimerNotFound
	}
	// Claim adapter cancellation before submitting the owner command. The
	// wrapper observes canceled before entering user code, and deleting under the
	// registry lock makes concurrent/repeated clears report one stable winner.
	state.canceled.Store(true)
	state.clearCallback()
	js.intervals, _ = retainedMapDelete(js.intervals, &js.intervalsRetention, id)
	js.intervalsMu.Unlock()

	// Native repeating intervals retain one timer ID for their full lifetime.
	// Clearing owns that ID exactly once; owner cancellation is accepted in FIFO
	// order without waiting for an executing callback to return.
	loopTimerID := TimerID(state.currentLoopTimerID.Swap(0))
	if loopTimerID == 0 {
		return nil
	}
	return js.loop.queueTimerCancel(loopTimerID)
}

// UnrefInterval marks the interval as unref'd. The interval continues to fire,
// but its native repeating timer no longer contributes to [Loop.Alive]. The
// owner command is accepted in timer-lifecycle FIFO order without waiting for
// the loop owner to apply it. Returns [ErrLoopTerminated] if terminal
// transition prevents admission.
func (js *JS) UnrefInterval(id uint64) error {
	js.intervalsMu.RLock()
	state, ok := js.intervals[id]
	js.intervalsMu.RUnlock()

	if !ok {
		return ErrTimerNotFound
	}
	currentID := TimerID(state.currentLoopTimerID.Load())
	if currentID == 0 {
		return ErrTimerNotFound
	}
	return js.loop.queueTimerRefChange(currentID, false)
}

// RefInterval reverses a previous [JS.UnrefInterval] call, making the native
// repeating timer contribute to [Loop.Alive] again. The owner command is
// accepted in timer-lifecycle FIFO order without waiting for the loop owner
// to apply it. Returns [ErrLoopTerminated] if terminal transition prevents
// admission.
func (js *JS) RefInterval(id uint64) error {
	js.intervalsMu.RLock()
	state, ok := js.intervals[id]
	js.intervalsMu.RUnlock()

	if !ok {
		return ErrTimerNotFound
	}
	currentID := TimerID(state.currentLoopTimerID.Load())
	if currentID == 0 {
		return ErrTimerNotFound
	}
	return js.loop.queueTimerRefChange(currentID, true)
}
