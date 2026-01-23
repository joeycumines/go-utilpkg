// Copyright 2025 Joseph Cumines
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
)

// RejectionHandler is a callback function invoked when an unhandled promise rejection
// is detected. The reason parameter contains the rejection reason/value.
// This follows the JavaScript unhandledrejection event pattern.
type RejectionHandler func(reason Result)

// JSOption configures a [JS] adapter instance.
// Options are applied in order during [NewJS] construction.
type JSOption func(*jsOptions)

type jsOptions struct {
	onUnhandled RejectionHandler
}

func resolveJSOptions(opts []JSOption) (*jsOptions, error) {
	o := &jsOptions{}
	for _, opt := range opts {
		opt(o)
	}
	return o, nil
}

// WithUnhandledRejection configures a handler that is invoked when a rejected
// promise has no catch handler attached after the microtask queue is drained.
// This follows the JavaScript unhandledrejection event semantics.
func WithUnhandledRejection(handler RejectionHandler) JSOption {
	return func(o *jsOptions) {
		o.onUnhandled = handler
	}
}

// jsTimerData tracks timer information for timeout and interval management.
type jsTimerData struct {
	jsTimerID   uint64  // The JS API timer ID (from SetTimeout/SetInterval)
	loopTimerID TimerID // The actual Loop timer ID
}

// intervalState tracks the state of an interval timer.
// It is stored in js.intervals map (uint64 -> *intervalState)
type intervalState struct {

	// Pointer fields last (all require 8-byte alignment)
	fn      SetTimeoutFunc
	wrapper func()
	js      *JS
	wg      sync.WaitGroup // Tracks wrapper execution for ClearInterval

	// Non-pointer, non-atomic fields first to reduce pointer alignment scope
	delayMs            int
	currentLoopTimerID TimerID

	// Sync primitives
	m sync.Mutex // Protects state fields

	// Atomic flag (requires 8-byte alignment)
	canceled atomic.Bool
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
	unhandledCallback RejectionHandler

	// Last pointer field
	loop *Loop

	// Group pointer/sync.Map fields together
	timers              sync.Map
	intervals           sync.Map
	unhandledRejections sync.Map
	promiseHandlers     sync.Map

	nextTimerID atomic.Uint64
	mu          sync.Mutex
}

// SetTimeoutFunc is a callback function for [JS.SetTimeout] and [JS.SetInterval].
// The callback is always invoked on the event loop thread.
type SetTimeoutFunc func()

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

	js := &JS{loop: loop}

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
func (js *JS) SetTimeout(fn SetTimeoutFunc, delayMs int) (uint64, error) {
	if fn == nil {
		return 0, nil
	}

	id := js.nextTimerID.Add(1)
	delay := time.Duration(delayMs) * time.Millisecond

	// Wrap user callback to clean up timerData after execution
	// This fixes a memory leak where timerData entries never get removed
	wrappedFn := func() {
		defer js.timers.Delete(id)
		fn()
	}

	// Schedule on underlying loop with wrapped callback
	loopTimerID, err := js.loop.ScheduleTimer(delay, wrappedFn)
	if err != nil {
		return 0, err
	}

	// Store data mapping JS API timer ID -> {jsTimerID, loopTimerID}
	data := &jsTimerData{
		jsTimerID:   id,
		loopTimerID: loopTimerID,
	}
	js.timers.Store(id, data)

	return id, nil
}

// ClearTimeout cancels a scheduled timeout timer by its ID.
//
// Returns [ErrTimerNotFound] if the timer ID is invalid or has already fired.
// This is safe to call multiple times for the same ID.
func (js *JS) ClearTimeout(id uint64) error {
	dataAny, ok := js.timers.Load(id)
	if !ok {
		return ErrTimerNotFound
	}
	data := dataAny.(*jsTimerData)

	if data.loopTimerID == 0 {
		// Timer was already canceled
		return ErrTimerNotFound
	}

	if err := js.loop.CancelTimer(data.loopTimerID); err != nil {
		return err
	}

	// Remove mapping
	js.timers.Delete(id)
	return nil
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
func (js *JS) SetInterval(fn SetTimeoutFunc, delayMs int) (uint64, error) {
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

	// Create wrapper function that accesses itself via state.wrapper
	// We define it as a closure that will be set below
	wrapper := func() {
		// Add to WaitGroup at START of each execution
		// This allows ClearInterval to wait for current execution to finish
		state.wg.Add(1)

		defer func() {
			if r := recover(); r != nil {
				// Log panic recovery for interval callbacks
				log.Printf("[eventloop] Interval callback panicked: %v", r)
			}
			state.wg.Done()
		}()

		// Run user's function
		state.fn()

		// Check if interval was canceled BEFORE trying to acquire lock
		// This prevents deadlock when wrapper runs on event loop thread
		// while ClearInterval holds the lock on another thread
		if state.canceled.Load() {
			return
		}

		// Cancel previous timer
		state.m.Lock()
		if state.currentLoopTimerID != 0 {
			js.loop.CancelTimer(state.currentLoopTimerID)
		}
		// Check canceled flag again after acquiring lock (for double-check)
		if state.canceled.Load() {
			state.m.Unlock()
			return
		}

		// Schedule next execution, using wrapper from state
		currentWrapper := state.wrapper
		loopTimerID, err := js.loop.ScheduleTimer(state.getDelay(), currentWrapper)
		if err != nil {
			state.m.Unlock()
			return
		}

		// Update loopTimerID in state for next cancellation
		state.currentLoopTimerID = loopTimerID
		state.m.Unlock()
	}

	// Store wrappers function in state for self-reference BEFORE any scheduling
	state.wrapper = wrapper

	// IMPORTANT: Assign id BEFORE any scheduling
	id := js.nextTimerID.Add(1)

	// Initial scheduling - call ScheduleTimer ONCE after both wrapper and id are properly assigned
	loopTimerID, err := js.loop.ScheduleTimer(delay, wrapper)
	if err != nil {
		return 0, err
	}

	// Store interval state with initial mapping
	state.m.Lock()
	state.currentLoopTimerID = loopTimerID
	state.m.Unlock()
	js.intervals.Store(id, state)

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
func (js *JS) ClearInterval(id uint64) error {
	dataAny, ok := js.intervals.Load(id)
	if !ok {
		return ErrTimerNotFound
	}
	state := dataAny.(*intervalState)

	// Mark as canceled BEFORE acquiring lock to prevent deadlock
	// This allows wrapper function to exit without blocking
	state.canceled.Store(true)

	state.m.Lock()
	defer state.m.Unlock()

	// Cancel pending scheduled timer if any
	if state.currentLoopTimerID != 0 {
		// Handle all cancellation errors gracefully - if timer is already fired or not found,
		// that's acceptable (race condition during wrapper execution)
		if err := js.loop.CancelTimer(state.currentLoopTimerID); err != nil {
			// If the error is not "timer not found", it's a real error
			if !errors.Is(err, ErrTimerNotFound) {
				return err
			}
			// ErrTimerNotFound is OK - timer already fired
		}
	} else {
		// If currentLoopTimerID is 0, it means:
		// 1. Timer hasn't been scheduled yet (race during SetInterval startup)
		// 2. Wrapper is in the process of rescheduling (temporarily 0 between cancel/schedule)
		// 3. Timer has fired and wrapper has exited
		// In all cases, we skip cancellation - the canceled flag will prevent future scheduling
	}

	// Remove from intervals map
	js.intervals.Delete(id)

	// Note: We do NOT wait for the wrapper to complete (wg.Wait()).
	// 1. Preventing Rescheduling: The state.canceled atomic flag (set above) guarantees
	//    the wrapper will not reschedule, preventing the TOCTOU race.
	// 2. Deadlock Avoidance: Waiting here would deadlock if ClearInterval is called
	//    from within the interval callback (same goroutine).
	// 3. JS Semantics: clearInterval is non-blocking.

	return nil
}

// MicrotaskFunc is a callback function for [JS.QueueMicrotask].
// The callback is always invoked on the event loop thread.
type MicrotaskFunc func()

// QueueMicrotask schedules a microtask to run before any pending timer callbacks.
//
// Microtasks are processed in FIFO order and have higher priority than timers.
// A microtask scheduled from within another microtask will be processed in the
// same tick, before any timer callbacks.
//
// This follows the JavaScript queueMicrotask semantics and is used internally
// by the Promise implementation for then/catch/finally handlers.
func (js *JS) QueueMicrotask(fn MicrotaskFunc) error {
	if fn == nil {
		return nil
	}

	return js.loop.ScheduleMicrotask(func() {
		fn()
	})
}

// getDelay returns the delay as time.Duration for scheduling.
func (s *intervalState) getDelay() time.Duration {
	return time.Duration(s.delayMs) * time.Millisecond
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
