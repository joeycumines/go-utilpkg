// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import (
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"
	"weak"
)

// maxSafeInteger is `2^53 - 1`, the maximum safe integer in JavaScript
const maxSafeInteger = 9007199254740991

// ErrImmediateIDExhausted is returned when all immediate IDs have been exhausted.
// This occurs when nextImmediateID would exceed JavaScript's MAX_SAFE_INTEGER (2^53 - 1).
var ErrImmediateIDExhausted = errors.New("eventloop: immediate ID exceeded MAX_SAFE_INTEGER")

// ErrIntervalIDExhausted is returned when all interval IDs have been exhausted.
// This occurs when nextTimerID would exceed JavaScript's MAX_SAFE_INTEGER (2^53 - 1).
var ErrIntervalIDExhausted = errors.New("eventloop: interval ID exceeded MAX_SAFE_INTEGER")

// RejectionHandler is a callback function invoked when an unhandled promise rejection
// is detected. The reason parameter contains the rejection reason/value.
// This follows the JavaScript unhandledrejection event pattern.
type RejectionHandler func(reason Result)

// JSOption configures a [JS] adapter instance.
// Options are applied in order during [NewJS] construction.
type JSOption interface {
	applyJSOption(*jsOptions) error
}

type jsOptions struct {
	onUnhandled RejectionHandler
}

// jsOptionImpl implements JSOption.
type jsOptionImpl struct {
	applyJSOptionFunc func(*jsOptions) error
}

func (j *jsOptionImpl) applyJSOption(opts *jsOptions) error {
	return j.applyJSOptionFunc(opts)
}

func resolveJSOptions(opts []JSOption) (*jsOptions, error) {
	o := &jsOptions{}
	for _, opt := range opts {
		if opt == nil {
			continue // Skip nil options gracefully
		}
		if err := opt.applyJSOption(o); err != nil {
			return nil, err
		}
	}
	return o, nil
}

// WithUnhandledRejection configures a handler that is invoked when a rejected
// promise has no catch handler attached after the microtask queue is drained.
// This follows the JavaScript unhandledrejection event semantics.
func WithUnhandledRejection(handler RejectionHandler) JSOption {
	return &jsOptionImpl{func(o *jsOptions) error {
		o.onUnhandled = handler
		return nil
	}}
}

// intervalState tracks the state of an interval timer.
// It is stored in js.intervals map (uint64 -> *intervalState)
type intervalState struct {

	// Pointer fields last (all require 8-byte alignment)
	fn      setTimeoutFunc
	wrapper func()
	js      *JS

	// Non-pointer, non-atomic fields first to reduce pointer alignment scope
	delayMs int

	// Sync primitives
	m sync.Mutex // Protects state fields

	// Atomic fields (must be 8-byte aligned, placed after non-atomic fields)
	currentLoopTimerID atomic.Uint64
	canceled           atomic.Bool
	running            atomic.Bool // Tracks when wrapper is actively executing to fix ClearInterval race
}

// JS provides JavaScript-compatible timer and microtask operations on top of [Loop].
//
// JS is a runtime-agnostic adapter that implements the semantics of JavaScript's
// setTimeout, setInterval, clearTimeout, clearInterval, and queueMicrotask APIs.
// It can be bridged to JavaScript runtimes like Goja for full interoperability.
//
// Timer Semantics:
//   - [JS.SetTimeout] schedules a one-time callback after a delay
//   - [JS.SetInterval] schedules a repeating callback with a fixed delay
//   - [JS.ClearTimeout] and [JS.ClearInterval] cancel scheduled timers
//
// Microtask Semantics:
//   - [JS.QueueMicrotask] schedules a high-priority callback that runs before any timer
//   - Microtasks are processed in FIFO order within each tick
//
// Promise Support:
//   - [JS.NewChainedPromise] creates Promise/A+ compatible promises
//   - Promises integrate with the microtask queue for proper async semantics
//   - Promise combinators: [JS.All], [JS.Race], [JS.AllSettled], [JS.Any]
//
// Thread Safety:
//   - JS is safe for concurrent use from multiple goroutines
//   - Callbacks are always executed on the event loop thread
type JS struct {
	// Pointer/map fields first for better cache alignment
	unhandledCallback   RejectionHandler
	loop                *Loop
	intervals           map[uint64]*intervalState
	unhandledRejections map[*ChainedPromise]*rejectionInfo
	promiseHandlers     map[*ChainedPromise]bool
	setImmediateMap     map[uint64]*setImmediateState
	handlerReadyChans   map[*ChainedPromise]chan struct{}
	debugStacks         map[weak.Pointer[ChainedPromise]][]uintptr
	toChannels          map[*ChainedPromise][]chan Result

	// WARNING: Do not use sync.Map here! (It isn't a good fit for this use case)

	// Sync primitives
	intervalsMu       sync.RWMutex
	rejectionsMu      sync.RWMutex
	promiseHandlersMu sync.RWMutex
	setImmediateMu    sync.RWMutex
	mu                sync.Mutex
	handlerReadyMu    sync.Mutex
	debugStacksMu     sync.Mutex
	toChannelsMu      sync.Mutex

	// Atomic counters and flags
	nextImmediateID         atomic.Uint64
	nextTimerID             atomic.Uint64
	checkRejectionScheduled atomic.Bool // Prevents duplicate checkUnhandledRejections microtasks
}

// setImmediateState tracks a single setImmediate callback
type setImmediateState struct {
	fn      setTimeoutFunc
	js      *JS
	id      uint64
	cleared atomic.Bool // CAS flag for "was cleared"
}

// setTimeoutFunc is a callback function for [JS.SetTimeout] and [JS.SetInterval].
// The callback is always invoked on the event loop thread.
type setTimeoutFunc func()

// NewJS creates a new [JS] adapter for the given event loop.
//
// The adapter provides JavaScript-compatible timer and promise APIs that
// execute callbacks on the provided loop's thread.
//
// Example:
//
//	loop := eventloop.New()
//	js, err := eventloop.NewJS(loop,
//	    eventloop.WithUnhandledRejection(func(reason eventloop.Result) {
//	        log.Printf("Unhandled rejection: %v", reason)
//	    }),
//	)
//	if err != nil {
//	    return err
//	}
//
//	// Schedule a timeout
//	js.SetTimeout(func() {
//	    fmt.Println("Hello after 100ms")
//	}, 100)
func NewJS(loop *Loop, opts ...JSOption) (*JS, error) {
	options, err := resolveJSOptions(opts)
	if err != nil {
		return nil, err
	}

	js := &JS{
		loop:                loop,
		intervals:           make(map[uint64]*intervalState),
		unhandledRejections: make(map[*ChainedPromise]*rejectionInfo),
		promiseHandlers:     make(map[*ChainedPromise]bool),
		setImmediateMap:     make(map[uint64]*setImmediateState),
		handlerReadyChans:   make(map[*ChainedPromise]chan struct{}),
		debugStacks:         make(map[weak.Pointer[ChainedPromise]][]uintptr),
		toChannels:          make(map[*ChainedPromise][]chan Result),
	}

	// ID Separation: SetImmediates start at high IDs to prevent collision
	// with timeout IDs that start at 1. This ensures namespace separation
	// across both timer systems even as they grow.
	js.nextImmediateID.Store(1 << 48)

	// Store onUnhandled callback
	if options.onUnhandled != nil {
		js.unhandledCallback = options.onUnhandled
	}

	return js, nil
}

// Loop returns the underlying [Loop] that this JS adapter is bound to.
// All callbacks scheduled through this JS adapter will execute on this loop's thread.
func (js *JS) Loop() *Loop {
	return js.loop
}

// notifyToChannels writes the result to all channels registered for the given promise
// in the toChannels side table, then removes the entry. This is called synchronously
// from resolve/reject while holding p.mu, ensuring ToChannel works without the
// microtask queue (i.e., even when the event loop is not running).
//
// Lock ordering: caller holds p.mu, this method acquires js.toChannelsMu.
func (js *JS) notifyToChannels(p *ChainedPromise, result Result) {
	js.toChannelsMu.Lock()
	channels, ok := js.toChannels[p]
	if ok {
		delete(js.toChannels, p)
	}
	js.toChannelsMu.Unlock()

	for _, ch := range channels {
		ch <- result
		close(ch)
	}
}

// SetTimeout schedules a function to run after a delay, following JavaScript setTimeout semantics.
//
// Parameters:
//   - fn: The callback to execute. If nil, returns 0 without scheduling.
//   - delayMs: Delay in milliseconds. Values < 0 are treated as 0.
//
// Returns:
//   - Timer ID that can be passed to [JS.ClearTimeout] to cancel
//   - Error if the loop is shutting down or has been closed
//
// The callback will execute on the event loop thread. If the delay is 0,
// the callback is still scheduled (not executed synchronously) but will
// run after all pending microtasks are processed.
func (js *JS) SetTimeout(fn setTimeoutFunc, delayMs int) (uint64, error) {
	if fn == nil {
		return 0, nil
	}

	delay := time.Duration(delayMs) * time.Millisecond

	// Schedule on underlying loop
	// ScheduleTimer now validates ID <= MAX_SAFE_INTEGER BEFORE scheduling
	// If validation fails, it returns ErrTimerIDExhausted
	loopTimerID, err := js.loop.ScheduleTimer(delay, fn)
	if err != nil {
		return 0, err
	}

	return uint64(loopTimerID), nil
}

// ClearTimeout cancels a scheduled timeout timer by its ID.
//
// Returns [ErrTimerNotFound] if the timer ID is invalid or has already fired.
// This is safe to call multiple times for the same ID.
func (js *JS) ClearTimeout(id uint64) error {
	// Direct cancellation on the loop
	// We cast the uint64 back to TimerID
	// This works because we panic if the ID exceeds MAX_SAFE_INTEGER,
	// so any valid ID returned from SetTimeout is safe to cast back unless
	// the user passes junk, in which case CancelTimer handles it safely (returns ErrTimerNotFound).
	return js.loop.CancelTimer(TimerID(id))
}

// SetInterval schedules a function to run repeatedly with a fixed delay.
//
// Parameters:
//   - fn: The callback to execute. If nil, returns 0 without scheduling.
//   - delayMs: Interval in milliseconds between executions.
//
// Returns:
//   - Timer ID that can be passed to [JS.ClearInterval] to cancel
//   - Error if the loop is shutting down or has been closed
//
// The callback will continue to fire at the specified interval until
// [JS.ClearInterval] is called with the returned ID. Each execution
// is scheduled after the previous one completes.
func (js *JS) SetInterval(fn setTimeoutFunc, delayMs int) (uint64, error) {
	if fn == nil {
		return 0, nil
	}

	delay := time.Duration(delayMs) * time.Millisecond

	// Create interval state that persists across invocations
	state := &intervalState{
		fn:      fn,
		delayMs: delayMs,
		js:      js,
	}

	// Create wrapper function that captures itself via closure variable
	// This avoids race condition where wrapper reads state.wrapper without lock
	var wrapper func()
	wrapper = func() {
		// Mark wrapper as running to fix ClearInterval race condition
		state.running.Store(true)
		defer state.running.Store(false)

		defer func() {
			if r := recover(); r != nil {
				// Log panic recovery for interval callbacks
				log.Printf("[eventloop] Interval callback panicked: %v", r)
			}
		}()

		// Check if interval was canceled BEFORE running user's function
		// This prevents deadlock when wrapper runs on event loop thread
		// while ClearInterval holds the lock on another thread
		// CRITICAL: Must check before acquiring any locks or running user code
		if state.canceled.Load() {
			return
		}

		// Run user's function - might acquire locks, so check canceled first
		state.fn()

		// Check if interval was canceled AFTER running user's function
		// This catches the case where ClearInterval was called during fn execution
		if state.canceled.Load() {
			return
		}

		// Cancel previous timer using atomic operations to avoid deadlock
		// Use CompareAndSwap to atomically read-and-clear the timer ID
		oldTimerID := state.currentLoopTimerID.Load()
		if oldTimerID != 0 {
			// Try to atomically set to 0 - only succeeds if still the old value
			// This prevents races where ClearInterval might be canceling
			if state.currentLoopTimerID.CompareAndSwap(oldTimerID, 0) {
				// We successfully claimed ownership of canceling this timer
				js.loop.CancelTimer(TimerID(oldTimerID))
			}
		}

		// Check canceled flag one more time after timer operations
		// This catches the case where ClearInterval was called during timer cancellation
		if state.canceled.Load() {
			return
		}

		// Schedule next execution using captured wrapper reference
		// This avoids the race condition of reading state.wrapper without lock
		loopTimerID, err := js.loop.ScheduleTimer(state.getDelay(), wrapper)
		if err != nil {
			// Scheduling failed - try to restore timer ID if not canceled
			if !state.canceled.Load() {
				// Atomically try to restore the timer ID
				state.currentLoopTimerID.CompareAndSwap(0, uint64(loopTimerID))
			}
			return
		}

		// Atomically update the timer ID - this is safe because:
		// 1. We already checked canceled.Load() after timer cancellation
		// 2. ClearInterval uses CompareAndSwap, so it won't overwrite our new value
		// 3. If ClearInterval races with us, the CAS will fail harmlessly
		state.currentLoopTimerID.CompareAndSwap(0, uint64(loopTimerID))
	}

	// IMPORTANT: Assign id BEFORE any scheduling
	id := js.nextTimerID.Add(1)

	// Safety check for JS integer limits
	if id > maxSafeInteger {
		return 0, ErrIntervalIDExhausted
	}

	// Store wrapper in state under lock for synchronization
	state.m.Lock()
	state.wrapper = wrapper

	// Initial scheduling - call ScheduleTimer ONCE after wrapper is assigned
	loopTimerID, err := js.loop.ScheduleTimer(delay, wrapper)
	if err != nil {
		state.m.Unlock()
		return 0, err
	}

	// Atomically update loopTimerID in state for next cancellation
	state.currentLoopTimerID.Store(uint64(loopTimerID))
	state.m.Unlock()

	// Store interval state in global map with initial mapping
	js.intervalsMu.Lock()
	js.intervals[id] = state
	js.intervalsMu.Unlock()

	// NOTE: Intervals are managed exclusively through js.intervals map
	// ClearInterval loads state from js.intervals and reads state.currentLoopTimerID
	// We do NOT create a js.timers entry for intervals

	return id, nil
}

// ClearInterval cancels a scheduled interval timer by its ID.
//
// Returns [ErrTimerNotFound] if the timer ID is invalid.
// This is safe to call from any goroutine, including from within
// the interval's own callback.
//
// JavaScript Semantics and TOCTOU Race:
// ClearInterval provides a best-effort guarantee that matches JavaScript semantics:
//   - If the interval callback is not executing, it will not execute again
//   - If the callback is currently executing, the NEXT scheduled execution is cancelled
//   - There is a narrow window (between wrapper's Check 1 and lock acquisition) where
//     the interval might fire one additional time after ClearInterval returns
//
// This is intentional and matches the asynchronous nature of clearInterval in JavaScript.
func (js *JS) ClearInterval(id uint64) error {
	js.intervalsMu.RLock()
	state, ok := js.intervals[id]
	js.intervalsMu.RUnlock()

	if !ok {
		return ErrTimerNotFound
	}

	// Mark as canceled BEFORE trying to cancel timer
	// This prevents the wrapper from rescheduling after we cancel
	state.canceled.Store(true)

	// Use atomic operations to avoid deadlock with wrapper
	// Try to atomically claim ownership of the timer ID for cancellation
	oldTimerID := state.currentLoopTimerID.Load()
	if oldTimerID != 0 {
		// Try to atomically set to 0 - only succeeds if still the old value
		// This prevents races where the wrapper might be rescheduling
		if state.currentLoopTimerID.CompareAndSwap(oldTimerID, 0) {
			// We successfully claimed ownership - cancel the timer
			// Handle all cancellation errors gracefully - if timer is already fired or not found,
			// that's acceptable (race condition during wrapper execution)
			if err := js.loop.CancelTimer(TimerID(oldTimerID)); err != nil {
				// If the error is not "timer not found", it's a real error
				if !errors.Is(err, ErrTimerNotFound) {
					return err
				}
				// ErrTimerNotFound is OK - timer already fired
			}
		}
		// If CAS failed, the wrapper has claimed ownership - it will handle cleanup
	}

	// If currentLoopTimerID is 0, check running flag:
	// 1. If running=true: Interval successfully created and wrapper executing
	//    ClearInterval called before first timer scheduled (or between executions)
	//    Success - cancel flag prevents rescheduling
	// 2. If running=false: Timer hasn't been scheduled yet (race during SetInterval startup)
	//    OR wrapper exited (timer fired and done)
	//    In both cases, skip cancellation - cancel flag prevents future scheduling
	// Note: We don't need to check this separately since CAS would handle it

	// Remove from intervals map
	js.intervalsMu.Lock()
	delete(js.intervals, id)
	js.intervalsMu.Unlock()

	// Note: We do NOT wait for the wrapper to complete (wg.Wait()).
	// 1. Preventing Rescheduling: The state.canceled atomic flag (set above) guarantees
	//    the wrapper will not reschedule, preventing the TOCTOU race.
	// 2. Deadlock Avoidance: Waiting here would deadlock if ClearInterval is called
	//    from within the interval callback (same goroutine).
	// 3. JS Semantics: clearInterval is non-blocking.

	return nil
}

// microtaskFunc is a callback function for [JS.QueueMicrotask].
// The callback is always invoked on the event loop thread.
type microtaskFunc func()

// QueueMicrotask schedules a microtask to run before any pending timer callbacks.
//
// Microtasks are processed in FIFO order and have higher priority than timers.
// A microtask scheduled from within another microtask will be processed in the
// same tick, before any timer callbacks.
//
// This follows the JavaScript queueMicrotask semantics and is used internally
// by the Promise implementation for then/catch/finally handlers.
func (js *JS) QueueMicrotask(fn microtaskFunc) error {
	if fn == nil {
		return nil
	}

	return js.loop.ScheduleMicrotask(fn)
}

// getDelay returns the delay as time.Duration for scheduling.
func (s *intervalState) getDelay() time.Duration {
	return time.Duration(s.delayMs) * time.Millisecond
}

// SetImmediate schedules a function to run in the next iteration of the event loop.
//
// This is more efficient than [JS.SetTimeout] with 0ms delay as it bypasses
// the timer heap and uses the efficient [Loop.Submit] mechanism directly.
//
// The callback is guaranteed to run asynchronously (after the current task/tick completes).
//
// Returns:
//   - ID that can be passed to [JS.ClearImmediate] to cancel
//   - Error if the loop is shutting down or all immediate IDs have been exhausted
func (js *JS) SetImmediate(fn setTimeoutFunc) (uint64, error) {
	if fn == nil {
		return 0, nil
	}

	id := js.nextImmediateID.Add(1)
	if id > maxSafeInteger {
		return 0, ErrImmediateIDExhausted
	}

	state := &setImmediateState{
		fn: fn,
		js: js,
		id: id,
	}

	js.setImmediateMu.Lock()
	js.setImmediateMap[id] = state
	js.setImmediateMu.Unlock()

	// Submit directly - no timer heap!
	if err := js.loop.Submit(state.run); err != nil {
		js.setImmediateMu.Lock()
		delete(js.setImmediateMap, id)
		js.setImmediateMu.Unlock()
		return 0, err
	}

	return id, nil
}

// ClearImmediate cancels a pending setImmediate task.
//
// Returns [ErrTimerNotFound] if the ID is invalid or has already executed.
//
// Note: Due to the asynchronous nature, it's possible for the callback to
// execute concurrently with cancellation if run from a different goroutine.
// However, once ClearImmediate returns successfully, the callback is guaranteed
// not to run (or has arguably "already run").
func (js *JS) ClearImmediate(id uint64) error {
	js.setImmediateMu.RLock()
	state, ok := js.setImmediateMap[id]
	js.setImmediateMu.RUnlock()

	if !ok {
		return ErrTimerNotFound
	}

	// Mark as cleared; if run() hasn't executed yet, it will see this
	state.cleared.Store(true)

	js.setImmediateMu.Lock()
	delete(js.setImmediateMap, id)
	js.setImmediateMu.Unlock()

	return nil
}

// run executes the setImmediate callback if not cleared.
func (s *setImmediateState) run() {
	// CAS ensures only one of run() or ClearImmediate() wins
	// Or more accurately: if ClearImmediate happened, we don't run.
	if s.cleared.Load() {
		return
	}
	// We don't need CAS here because ClearImmediate just sets the flag.
	// We just check it. If it races, it races.
	// But to be safer against double-execution if somehow submitted twice (shouldn't happen):
	if !s.cleared.CompareAndSwap(false, true) {
		return
	}

	// DEFER cleanup to ensure map entry is removed even if fn() panics
	// This fixes Memory Leak #2 from review.md Section 2.A
	defer func() {
		s.js.setImmediateMu.Lock()
		delete(s.js.setImmediateMap, s.id)
		s.js.setImmediateMu.Unlock()
	}()

	s.fn()
}

// Resolve returns an already-resolved promise with the given value.
//
// This follows the JavaScript Promise.resolve() semantics:
//   - Returns a promise resolved with the given value
//   - If the value is a promise, returns that promise
//   - Otherwise, returns a new promise resolved with the value
func (js *JS) Resolve(val any) *ChainedPromise {
	promise, resolve, _ := js.NewChainedPromise()
	resolve(val)
	return promise
}

// Reject returns an already-rejected promise with the given reason.
//
// This follows the JavaScript Promise.reject() semantics:
//   - Returns a promise rejected with the given reason
//   - The reason is typically an Error object
func (js *JS) Reject(reason any) *ChainedPromise {
	promise, _, reject := js.NewChainedPromise()
	reject(reason)
	return promise
}

// Try wraps a synchronous function call in a promise, following the ES2025
// Promise.try() proposal semantics.
//
// This method catches any panic that occurs during the function execution
// and converts it into a rejected promise. If the function executes successfully,
// the promise resolves with the returned value.
//
// Unlike Promise.resolve(fn()), Promise.try() catches synchronous exceptions
// (panics in Go) and converts them to rejections. This provides a consistent
// way to wrap any function in a promise, whether it might panic or not.
//
// Parameters:
//   - fn: A function that may panic or return a value
//
// Returns:
//   - A ChainedPromise that:
//   - Resolves with fn's return value if fn executes successfully
//   - Rejects with the panic value if fn panics
//
// Example:
//
//	// Safely wrap a function that might panic
//	promise := js.Try(func() any {
//	    return riskyOperation()
//	})
//
//	// This is equivalent to:
//	// new Promise(resolve => resolve(fn()))
//	// but catches synchronous panics too
//
// Thread Safety:
// The callback fn is executed synchronously on the calling goroutine.
// The returned promise is safe for concurrent access.
func (js *JS) Try(fn func() any) *ChainedPromise {
	promise, resolve, reject := js.NewChainedPromise()

	// Execute fn synchronously with panic recovery
	func() {
		defer func() {
			if r := recover(); r != nil {
				reject(PanicError{Value: r})
			}
		}()

		result := fn()
		resolve(result)
	}()

	return promise
}

// NextTick schedules a function to run before any microtasks in the current tick.
//
// EXPAND-020: This emulates Node.js process.nextTick() semantics. NextTick callbacks
// have higher priority than regular microtasks (promises, queueMicrotask), meaning
// they run before any promise handlers in the same tick.
//
// Unlike setTimeout(fn, 0) which schedules for the next tick, NextTick callbacks
// execute immediately after the current synchronous code, before any pending
// promise handlers.
//
// Parameters:
//   - fn: The callback to execute. If nil, returns nil without scheduling.
//
// Returns:
//   - Error if the loop is shut down.
//
// Example:
//
//	js.NextTick(func() {
//	    fmt.Println("This runs before promises")
//	})
//
//	promise.Then(func(r Result) Result {
//	    fmt.Println("This runs after nextTick")
//	    return nil
//	}, nil)
//
// Thread Safety: Safe to call from any goroutine.
func (js *JS) NextTick(fn func()) error {
	return js.loop.ScheduleNextTick(fn)
}

// Sleep returns a promise that resolves after the specified delay.
//
// EXPAND-021: This is a convenience helper for promise-based delays,
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
//	js.Sleep(100 * time.Millisecond).Then(func(r Result) Result {
//	    fmt.Println("100ms elapsed")
//	    return nil
//	}, nil)
//
// Thread Safety: Safe to call from any goroutine.
// The returned promise is safe for concurrent access.
func (js *JS) Sleep(ms time.Duration) *ChainedPromise {
	promise, resolve, _ := js.NewChainedPromise()

	// Schedule timer to resolve the promise
	_, err := js.loop.ScheduleTimer(ms, func() {
		resolve(nil)
	})
	if err != nil {
		// If scheduling fails, resolve immediately
		// This handles edge cases like loop termination
		resolve(nil)
	}

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
func (js *JS) Timeout(delay time.Duration) *ChainedPromise {
	promise, _, reject := js.NewChainedPromise()

	msg := "timeout after " + delay.String()

	// Schedule timer to reject the promise
	_, err := js.loop.ScheduleTimer(delay, func() {
		reject(&TimeoutError{Message: msg})
	})
	if err != nil {
		// If scheduling fails, reject immediately
		// This handles edge cases like loop termination
		reject(&TimeoutError{Message: msg})
	}

	return promise
}
