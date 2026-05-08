package eventloop

import (
	"sync"
	"time"
)

type timerPromiseState struct {
	settle func()
	reject RejectFunc
	once   sync.Once
}

func (s *timerPromiseState) finish() {
	s.once.Do(s.settle)
}

func (s *timerPromiseState) fail(err error) {
	s.once.Do(func() { s.reject(err) })
}

// Sleep returns a promise that resolves after the specified delay.
//
// This is a convenience helper for promise-based delays,
// similar to the delay() or sleep() patterns common in JavaScript.
//
// Parameters:
//   - ms: The delay duration.
//
// Returns:
//   - A ChainedPromise that resolves with nil after the delay.
//
// Example:
//
//	// Wait for 100ms, then do something
//	js.Sleep(100 * time.Millisecond).Then(func(r any) any {
//	    fmt.Println("100ms elapsed")
//	    return nil
//	}, nil)
//
// Thread Safety: Safe to call from any goroutine.
// The returned promise is safe for concurrent access.
// If timer admission fails, the promise rejects with the admission error.
// If terminal cleanup discards an accepted timer before it fires, the promise
// resolves with nil during that cleanup rather than remaining pending.
func (js *JS) Sleep(ms time.Duration) *ChainedPromise {
	promise, resolve, reject := js.NewChainedPromise()
	js.scheduleTimerPromise(ms, func() { resolve(nil) }, reject)

	return promise
}

// Timeout returns a promise that rejects after the specified delay.
//
// This is the rejection counterpart to [JS.Sleep]. While Sleep resolves
// after a delay, Timeout rejects with a [TimeoutError] after a delay.
//
// Use Timeout in combination with [JS.Race] to implement operation timeouts:
//
//	// Timeout an operation after 5 seconds
//	result := js.Race([]*eventloop.ChainedPromise{
//	    longRunningOperation(),
//	    js.Timeout(5 * time.Second),
//	})
//	// result will reject with TimeoutError if operation takes > 5s
//
// Parameters:
//   - delay: The duration to wait before rejecting.
//
// Returns:
//   - A ChainedPromise that rejects with [TimeoutError] after the delay.
//
// Thread Safety: Safe to call from any goroutine.
// The returned promise is safe for concurrent access.
// If timer admission fails, the promise rejects with the admission error rather
// than fabricating a [TimeoutError].
// If terminal cleanup discards an accepted timer before it fires, the promise
// rejects with the same [TimeoutError] used by normal expiry rather than
// remaining pending.
func (js *JS) Timeout(delay time.Duration) *ChainedPromise {
	promise, _, reject := js.NewChainedPromise()

	msg := "timeout after " + delay.String()
	js.scheduleTimerPromise(delay, func() { reject(&TimeoutError{Message: msg}) }, reject)

	return promise
}

// scheduleTimerPromise registers terminal settlement before publishing the
// native timer. Loop.livenessMu makes registration atomic with terminal cleanup:
// cleanup either owns the registered settlement, or the timer remains eligible
// to publish and eventually owns it. Promise settlement itself runs without the
// lifecycle lock because rejection handling may schedule follow-up work. A
// timer rejected before admission rejects its promise with the admission error;
// terminal cleanup retains normal settlement ownership after registration.
func (js *JS) scheduleTimerPromise(delay time.Duration, settle func(), reject RejectFunc) {
	state := &timerPromiseState{settle: settle, reject: reject}

	js.loop.livenessMu.Lock()
	loopState := js.loop.state.Load()
	if loopState == StateTerminating || loopState == StateTerminated {
		js.loop.livenessMu.Unlock()
		state.fail(ErrLoopTerminated)
		return
	}
	js.timerPromisesMu.Lock()
	js.timerPromises[state] = struct{}{}
	js.timerPromisesMu.Unlock()
	js.loop.livenessMu.Unlock()
	if js.loop.testHooks != nil && js.loop.testHooks.AfterJSTimerPromiseRegister != nil {
		js.loop.testHooks.AfterJSTimerPromiseRegister()
	}

	if _, err := js.loop.ScheduleTimer(delay, func() {
		if js.loop.testHooks != nil && js.loop.testHooks.BeforeJSTimerPromiseCallbackFinish != nil {
			js.loop.testHooks.BeforeJSTimerPromiseCallbackFinish()
		}
		js.finishTimerPromise(state)
	}); err != nil {
		js.failTimerPromise(state, err)
	}
}

func (js *JS) failTimerPromise(state *timerPromiseState, err error) {
	js.timerPromisesMu.Lock()
	if _, ok := js.timerPromises[state]; ok {
		delete(js.timerPromises, state)
		js.timerPromisesMu.Unlock()
		state.fail(err)
		return
	}
	js.timerPromisesMu.Unlock()
}

func (js *JS) finishTimerPromise(state *timerPromiseState) {
	js.timerPromisesMu.Lock()
	delete(js.timerPromises, state)
	js.timerPromisesMu.Unlock()
	state.finish()
}
