// Copyright 2025 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// RejectionHandler is called when an unhandled promise rejection is detected.
type RejectionHandler func(reason Result)

// JSOption configures a JS adapter.
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

// WithUnhandledRejection sets a callback that is invoked when a rejected promise has no handler.
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
	// Non-pointer, non-atomic fields first to reduce pointer alignment scope
	delayMs            int
	currentLoopTimerID TimerID

	// Mutex first (requires 8-byte alignment anyway)
	m sync.Mutex

	// Atomic flag (requires 8-byte alignment)
	canceled atomic.Bool

	// Pointer fields last (all require 8-byte alignment)
	fn      SetTimeoutFunc
	wrapper func()
	js      *JS
}

// JS provides JavaScript-like timer and microtask operations on top of event loop.
// This is a runtime-agnostic adapter that can be bridged to JavaScript runtimes.
type JS struct {
	unhandledCallback RejectionHandler
	nextTimerID       atomic.Uint64
	mu                sync.Mutex

	// Group pointer/sync.Map fields together
	timers              sync.Map
	intervals           sync.Map
	unhandledRejections sync.Map
	promiseHandlers     sync.Map

	// Last pointer field
	loop *Loop
}

// SetTimeoutFunc is a callback function for setTimeout/setInterval.
type SetTimeoutFunc func()

// NewJS creates a new JS adapter for the given event loop.
func NewJS(loop *Loop, opts ...JSOption) (*JS, error) {
	options, err := resolveJSOptions(opts)
	if err != nil {
		return nil, err
	}

	js := &JS{
		loop:                loop,
		nextTimerID:         atomic.Uint64{},
		timers:              sync.Map{},
		intervals:           sync.Map{},
		unhandledRejections: sync.Map{},
		promiseHandlers:     sync.Map{},
	}

	// Store onUnhandled callback
	if options.onUnhandled != nil {
		js.unhandledCallback = options.onUnhandled
	}

	return js, nil
}

// Loop returns the underlying event loop.
func (js *JS) Loop() *Loop {
	return js.loop
}

// SetTimeout schedules a function to run after a delay.
// Returns a timer ID that can be used with ClearTimeout.
func (js *JS) SetTimeout(fn SetTimeoutFunc, delayMs int) (uint64, error) {
	if fn == nil {
		return 0, nil
	}

	id := js.nextTimerID.Add(1)
	delay := time.Duration(delayMs) * time.Millisecond

	// Schedule on underlying loop
	loopTimerID, err := js.loop.ScheduleTimer(delay, fn)
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

// ClearTimeout cancels a scheduled timeout timer.
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
// Returns a timer ID that can be used with ClearInterval.
func (js *JS) SetInterval(fn SetTimeoutFunc, delayMs int) (uint64, error) {
	if fn == nil {
		return 0, nil
	}

	delay := time.Duration(delayMs) * time.Millisecond

	// Create interval state that persists across invocations
	state := &intervalState{fn: fn, delayMs: delayMs, js: js}

	// Create wrapper function that accesses itself via state.wrapper
	// We define it as a closure that will be set below
	wrapper := func() {
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

	// IMPORTANT: Store the wrapper function in state for self-reference BEFORE any scheduling
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

	// Create mapping from JS API ID to the first scheduled loop timer ID
	data := &jsTimerData{
		jsTimerID:   id,
		loopTimerID: loopTimerID,
	}
	js.timers.Store(loopTimerID, data)

	return id, nil
}

// ClearInterval cancels a scheduled interval timer.
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
		if err := js.loop.CancelTimer(state.currentLoopTimerID); err != nil && !errors.Is(err, ErrTimerNotFound) {
			return err
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

	return nil
}

// MicrotaskFunc is a callback function for queueMicrotask.
type MicrotaskFunc func()

// QueueMicrotask schedules a microtask to run before the next timer.
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
