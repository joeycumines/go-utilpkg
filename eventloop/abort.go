// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

//go:build linux || darwin

package eventloop

import (
	"sync"
	"time"
)

// AbortSignal represents a signal object that allows communication with an
// asynchronous operation (like a fetch request) and abort it if needed via an
// AbortController object.
//
// This implementation follows the W3C DOM AbortController/AbortSignal specification:
// https://dom.spec.whatwg.org/#interface-abortsignal
//
// Thread Safety:
// AbortSignal is safe for concurrent access from multiple goroutines.
// All state mutations are protected by an internal mutex.
//
// Usage:
//
//	controller := eventloop.NewAbortController()
//	signal := controller.Signal()
//
//	// Check if aborted
//	if signal.Aborted() {
//	    // Handle aborted state
//	}
//
//	// Add abort handler
//	signal.OnAbort(func(reason any) {
//	    fmt.Println("Aborted with reason:", reason)
//	})
//
//	// Abort the operation
//	controller.Abort("User cancelled")
type AbortSignal struct { //nolint:govet // betteralign:ignore
	handlers []func(reason any)
	reason   any
	mu       sync.RWMutex
	aborted  bool
}

// newAbortSignal creates a new AbortSignal.
// This is an internal function; signals are created via AbortController.
func newAbortSignal() *AbortSignal {
	return &AbortSignal{
		handlers: make([]func(reason any), 0),
	}
}

// Aborted returns true if the signal has been aborted.
//
// This follows the AbortSignal.aborted property from the DOM specification.
//
// Thread Safety: Safe to call concurrently.
func (s *AbortSignal) Aborted() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.aborted
}

// Reason returns the abort reason, or nil if not aborted or no reason was provided.
//
// This follows the AbortSignal.reason property from the DOM specification.
// When abort() is called without arguments, the reason defaults to an AbortError
// DOMException in browsers. In this Go implementation, it defaults to nil if
// no reason is provided.
//
// Thread Safety: Safe to call concurrently.
func (s *AbortSignal) Reason() any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reason
}

// OnAbort registers a callback function to be invoked when the signal is aborted.
//
// If the signal is already aborted at the time of registration, the callback
// is invoked immediately with the current abort reason.
//
// Multiple callbacks can be registered and will be called in registration order.
//
// This follows the AbortSignal.onabort event handler from the DOM specification.
//
// Thread Safety: Safe to call concurrently.
func (s *AbortSignal) OnAbort(handler func(reason any)) {
	if handler == nil {
		return
	}

	s.mu.Lock()
	// If already aborted, invoke handler immediately after unlocking
	if s.aborted {
		reason := s.reason
		s.mu.Unlock()
		handler(reason)
		return
	}

	s.handlers = append(s.handlers, handler)
	s.mu.Unlock()
}

// AddEventListener is an alias for OnAbort that mimics the DOM addEventListener API.
//
// The eventType parameter is provided for API compatibility but is ignored;
// only "abort" events are supported.
//
// Thread Safety: Safe to call concurrently.
func (s *AbortSignal) AddEventListener(eventType string, handler func(reason any)) {
	s.OnAbort(handler)
}

// RemoveEventListener is provided for API compatibility but does not remove handlers.
// Go function values cannot be reliably compared. Use context-based cancellation instead.
//
// Thread Safety: Safe to call concurrently (no-op).
func (s *AbortSignal) RemoveEventListener(eventType string, handler func(reason any)) {
	// Not implemented - see doc comment.
	// Go functions cannot be compared reliably, so the original implementation
	// using &h == &handler compared addresses of local variables, always false.
}

// ThrowIfAborted returns an error if the signal has been aborted.
//
// This is a convenience method that follows the AbortSignal.throwIfAborted()
// method from the DOM specification. Returns nil if not aborted.
//
// Thread Safety: Safe to call concurrently.
func (s *AbortSignal) ThrowIfAborted() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.aborted {
		return &AbortError{Reason: s.reason}
	}
	return nil
}

// abort is called by AbortController to abort the signal.
// This is an internal method.
func (s *AbortSignal) abort(reason any) {
	s.mu.Lock()

	// If already aborted, this is a no-op
	if s.aborted {
		s.mu.Unlock()
		return
	}

	s.aborted = true
	s.reason = reason

	// Copy handlers to invoke outside the lock
	handlers := make([]func(reason any), len(s.handlers))
	copy(handlers, s.handlers)
	s.mu.Unlock()

	// Invoke all handlers
	for _, handler := range handlers {
		// Handlers should not panic, but we don't recover here
		// to match JavaScript semantics where exceptions propagate
		handler(reason)
	}
}

// AbortController represents a controller object that allows aborting one or more
// asynchronous operations through its associated AbortSignal.
//
// This implementation follows the W3C DOM AbortController specification:
// https://dom.spec.whatwg.org/#interface-abortcontroller
//
// Thread Safety:
// AbortController is safe for concurrent access from multiple goroutines.
// The Abort() method can be called from any goroutine.
//
// Usage:
//
//	controller := eventloop.NewAbortController()
//	signal := controller.Signal()
//
//	// Pass signal to async operation
//	go func() {
//	    // Check periodically
//	    if signal.Aborted() {
//	        return // Operation cancelled
//	    }
//	    // Continue work...
//	}()
//
//	// Later, abort the operation
//	controller.Abort("Operation timed out")
type AbortController struct {
	signal *AbortSignal
}

// NewAbortController creates a new AbortController with a fresh AbortSignal.
//
// The returned controller can be used to abort operations that accept its
// associated Signal().
func NewAbortController() *AbortController {
	return &AbortController{
		signal: newAbortSignal(),
	}
}

// Signal returns the AbortSignal associated with this controller.
//
// The returned signal can be passed to asynchronous operations to allow
// them to be aborted when Abort() is called on the controller.
//
// Thread Safety: Safe to call concurrently. Always returns the same signal.
func (c *AbortController) Signal() *AbortSignal {
	return c.signal
}

// Abort aborts the controller's signal with the given reason.
//
// If reason is nil, a default AbortError is used as the reason.
//
// Once aborted, the signal's Aborted() method returns true, its Reason()
// method returns the abort reason, and all registered onabort handlers
// are invoked.
//
// Calling Abort() multiple times has no additional effect; the signal
// remains in its aborted state with the original reason.
//
// Thread Safety: Safe to call concurrently from any goroutine.
func (c *AbortController) Abort(reason any) {
	// If no reason provided, use a default AbortError
	if reason == nil {
		reason = &AbortError{Reason: "Aborted"}
	}
	c.signal.abort(reason)
}

// AbortError represents an error that occurs when an operation is aborted.
//
// This corresponds to the DOMException with name "AbortError" in browsers.
type AbortError struct {
	// Reason contains the abort reason provided to AbortController.Abort().
	Reason any
}

// Error implements the error interface.
func (e *AbortError) Error() string {
	if e.Reason == nil {
		return "AbortError: The operation was aborted"
	}
	if s, ok := e.Reason.(string); ok {
		return "AbortError: " + s
	}
	if err, ok := e.Reason.(error); ok {
		return "AbortError: " + err.Error()
	}
	return "AbortError: The operation was aborted"
}

// Is implements errors.Is support for AbortError.
func (e *AbortError) Is(target error) bool {
	_, ok := target.(*AbortError)
	return ok
}

// Unwrap returns the underlying error if Reason is an error type.
// This enables use with [errors.Is] and [errors.As] for error matching
// through the cause chain (ES2022 Error.cause compatibility).
//
// If Reason is not an error, returns nil.
func (e *AbortError) Unwrap() error {
	if err, ok := e.Reason.(error); ok {
		return err
	}
	return nil
}

// AbortTimeout creates an AbortController that will automatically abort after
// the specified duration.
//
// This is a convenience function similar to AbortSignal.timeout() in the DOM spec.
//
// Parameters:
//   - loop: The event loop to schedule the timeout on
//   - delayMs: Timeout duration in milliseconds
//
// Returns:
//   - The AbortController (for manual early-abort if needed)
//   - Error if scheduling on the loop fails
//
// Example:
//
//	controller, err := eventloop.AbortTimeout(loop, 5000) // 5 second timeout
//	if err != nil {
//	    return err
//	}
//	signal := controller.Signal()
//	// Pass signal to fetch or other async operation
func AbortTimeout(loop *Loop, delayMs int) (*AbortController, error) {
	controller := NewAbortController()

	_, err := loop.ScheduleTimer(time.Duration(delayMs)*time.Millisecond, func() {
		controller.Abort(&AbortError{Reason: "TimeoutError: The operation timed out"})
	})
	if err != nil {
		return nil, err
	}

	return controller, nil
}

// AbortAny creates a composite AbortSignal that aborts when ANY of the input
// signals abort.
//
// This implements the AbortSignal.any() static method from the DOM specification.
// The returned signal's reason will be the reason from the first signal to abort.
//
// If any input signal is already aborted, the returned signal will be immediately
// aborted with that signal's reason.
//
// Parameters:
//   - signals: A slice of AbortSignal pointers to monitor
//
// Returns:
//   - A new AbortSignal that aborts when any input signal aborts
//   - Returns an already-aborted signal if any input is already aborted
//   - Returns a never-aborted signal if the input slice is empty
//
// Thread Safety:
// AbortAny is safe to call from any goroutine. The returned signal is safe
// for concurrent access.
//
// Example:
//
//	controller1 := eventloop.NewAbortController()
//	controller2 := eventloop.NewAbortController()
//
//	combined := eventloop.AbortAny([]*eventloop.AbortSignal{
//	    controller1.Signal(),
//	    controller2.Signal(),
//	})
//
//	// combined.Aborted() becomes true when EITHER controller aborts
//	controller1.Abort("reason 1") // combined now aborted with "reason 1"
func AbortAny(signals []*AbortSignal) *AbortSignal {
	// Create a new signal for the composite
	composite := newAbortSignal()

	// Handle empty input - return a signal that will never abort
	if len(signals) == 0 {
		return composite
	}

	// Create a controller so we can abort the composite signal
	var abortOnce sync.Once

	// Check if any input signal is already aborted (fast path)
	for _, sig := range signals {
		if sig == nil {
			continue
		}
		if sig.Aborted() {
			// Already aborted - abort composite immediately with same reason
			composite.abort(sig.Reason())
			return composite
		}
	}

	// Register handlers on all signals to propagate abort
	for _, sig := range signals {
		if sig == nil {
			continue
		}

		// Capture sig in closure
		s := sig
		s.OnAbort(func(reason any) {
			abortOnce.Do(func() {
				composite.abort(reason)
			})
		})
	}

	return composite
}
