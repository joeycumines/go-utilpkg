package eventloop

import (
	"slices"
	"sync"
	"sync/atomic"
)

// Promise Combinators
// ============================================================================

// All returns a promise that resolves when all input promises resolve.
//
// Behavior:
//   - If promises is empty, resolves immediately with an empty slice
//   - Resolves with a slice of values in the same order as the input promises
//   - Rejects immediately when any promise rejects, with that promise's reason
//   - Rejects with [NilPromiseError] before attachment if an input is nil
//   - Rejects with [ErrLoopTerminated] if an input reaction cannot execute and
//     that failure wins the returned promise's settlement claim
//
// Example:
//
//	p1, resolve1, _ := js.NewPromise()
//	p2, resolve2, _ := js.NewPromise()
//	go func() {
//	    resolve1("a")
//	    resolve2("b")
//	}()
//	// result will be []any{"a", "b"}
//	result := js.All(p1, p2)
func (js *JS) All(promises ...*Promise) *Promise {
	result, resolve, reject := js.NewPromise()

	// Handle empty array - resolve immediately with empty array
	if len(promises) == 0 {
		resolve(make([]any, 0))
		return result
	}
	if err := validatePromiseInputs(promises); err != nil {
		reject(err)
		return result
	}

	// Track completion
	var mu sync.Mutex
	var completed atomic.Int32
	values := make([]any, len(promises))
	// Attach handlers to each promise
	for i, p := range promises {
		idx := i // Capture index
		p.observeSettlement(
			func(v any) any {
				// Store value in correct position
				mu.Lock()
				values[idx] = v
				mu.Unlock()

				// Check if all promises resolved
				count := completed.Add(1)
				if count == int32(len(promises)) {
					resolve(values)
				}
				return nil
			},
			func(r any) any {
				// Reject on first rejection
				reject(r)
				return nil
			},
			result,
		)
	}

	return result
}

// Race returns a promise that settles as soon as any of the input promises settles.
//
// Behavior:
//   - If promises is empty, the returned promise never settles (remains pending)
//   - Settles with the value/reason of the first input reaction to claim the
//     returned promise
//   - Ignores subsequent settlements from other promises
//   - Rejects with [NilPromiseError] before attachment if an input is nil
//   - Rejects with [ErrLoopTerminated] if an input reaction cannot execute and
//     that failure wins the returned promise's settlement claim
//
// Use Race for timeout patterns:
//
//	timeout, _, rejectTimeout := js.NewPromise()
//	go func() {
//	    time.Sleep(5 * time.Second)
//	    rejectTimeout(errors.New("timeout"))
//	}()
//	result := js.Race(actualWork, timeout)
func (js *JS) Race(promises ...*Promise) *Promise {
	result, resolve, reject := js.NewPromise()

	// Handle empty array - never settles
	if len(promises) == 0 {
		return result
	}
	if err := validatePromiseInputs(promises); err != nil {
		reject(err)
		return result
	}

	// Attach handlers to each promise (first to settle wins)
	for _, p := range promises {
		p.observeSettlement(
			func(v any) any {
				resolve(v)
				return nil
			},
			func(r any) any {
				reject(r)
				return nil
			},
			result,
		)
	}

	return result
}

// AllSettled returns a promise that resolves when all input promises have settled.
//
// Unlike [JS.All], this waits for every valid promise to complete. A nil input
// rejects the returned promise before any source handlers are attached.
// The promise fulfills with a slice of outcome objects:
//
//	// For fulfilled promises:
//	map[string]interface{}{"status": "fulfilled", "value": <value>}
//
//	// For rejected promises:
//	map[string]interface{}{"status": "rejected", "reason": <reason>}
//
// Behavior:
//   - If promises is empty, resolves immediately with an empty slice
//   - Resolves for every valid input; a nil input rejects before attachment
//   - Results are in the same order as the input promises
//   - Rejects with [ErrLoopTerminated] if an input reaction cannot execute and
//     that failure wins the returned promise's settlement claim
func (js *JS) AllSettled(promises ...*Promise) *Promise {
	result, resolve, reject := js.NewPromise()

	if len(promises) == 0 {
		resolve(make([]any, 0))
		return result
	}
	if err := validatePromiseInputs(promises); err != nil {
		reject(err)
		return result
	}

	// Track completion
	var mu sync.Mutex
	var completed atomic.Int32
	results := make([]any, len(promises))
	for i, p := range promises {
		idx := i // Capture index
		p.observeSettlement(
			func(v any) any {
				mu.Lock()
				results[idx] = map[string]any{
					"status": "fulfilled",
					"value":  v,
				}
				mu.Unlock()

				count := completed.Add(1)
				if count == int32(len(promises)) {
					resolve(results)
				}
				return nil
			},
			func(r any) any {
				mu.Lock()
				results[idx] = map[string]any{
					"status": "rejected",
					"reason": r,
				}
				mu.Unlock()

				count := completed.Add(1)
				if count == int32(len(promises)) {
					resolve(results)
				}
				return nil
			},
			result,
		)
	}

	return result
}

// Any returns a promise that resolves when any input promise resolves.
//
// Behavior:
//   - If promises is empty, rejects immediately with [AggregateError]
//   - Resolves with the value of the first fulfillment reaction to claim the
//     returned promise
//   - Rejects with [AggregateError] only if ALL promises reject
//   - Rejects with [NilPromiseError] before attachment if an input is nil
//   - Rejects with [ErrLoopTerminated] if an input reaction cannot execute and
//     that failure wins the returned promise's settlement claim
//
// Use Any when you need at least one success:
//
//	// Try multiple data sources, use first successful response
//	result := js.Any(source1, source2, source3)
func (js *JS) Any(promises ...*Promise) *Promise {
	result, resolve, reject := js.NewPromise()

	// Handle empty array - reject immediately
	if len(promises) == 0 {
		reject(&AggregateError{
			Errors:  make([]any, 0),
			Message: "All promises were rejected",
		})
		return result
	}
	if err := validatePromiseInputs(promises); err != nil {
		reject(err)
		return result
	}

	var mu sync.Mutex
	var rejected atomic.Int32
	rejections := make([]any, len(promises))
	// Attach handlers to each promise
	for i, p := range promises {
		idx := i // Capture index
		p.observeSettlement(
			func(v any) any {
				resolve(v)
				return nil
			},
			func(r any) any {
				mu.Lock()
				rejections[idx] = r
				mu.Unlock()

				count := rejected.Add(1)
				// If all rejected and none resolved, aggregate errors
				if count == int32(len(promises)) {
					mu.Lock()
					errors := slices.Clone(rejections)
					mu.Unlock()
					reject(&AggregateError{
						Errors:  errors,
						Message: "All promises were rejected",
					})
				}
				return nil
			},
			result,
		)
	}

	return result
}

func validatePromiseInputs(promises []*Promise) error {
	for index, promise := range promises {
		if promise == nil {
			return &NilPromiseError{Index: index}
		}
	}
	return nil
}

// ============================================================================
// Promise.withResolvers-shaped Go helper
// ============================================================================

// PromiseWithResolvers is a Go result shaped like Promise.withResolvers().
// It provides a convenient way to create a promise along with its
// resolve and reject functions, without requiring an executor callback.
//
// Its fields model the ES2024 {promise, resolve, reject} result shape without
// claiming to implement an ECMAScript API.
//
// Example:
//
//	resolvers := js.WithResolvers()
//	go func() {
//	    result, err := doAsyncWork()
//	    if err != nil {
//	        resolvers.Reject(err)
//	    } else {
//	        resolvers.Resolve(result)
//	    }
//	}()
//	resolvers.Promise.Then(handleResult, handleError)
//
// Thread Safety:
//
// All fields are safe for concurrent use. The Promise, Resolve, and Reject
// fields can be accessed from any goroutine.
type PromiseWithResolvers struct {
	// Promise is the pending promise associated with this resolvers object.
	// It will be resolved or rejected when Resolve or Reject is called.
	Promise *Promise

	// Resolve is the function that fulfills the Promise with a value.
	// Calling Resolve on an already-settled promise has no effect.
	// Can be called from any goroutine.
	Resolve ResolveFunc

	// Reject is the function that rejects the Promise with a reason.
	// Calling Reject on an already-settled promise has no effect.
	// Can be called from any goroutine.
	Reject RejectFunc
}

// WithResolvers creates a new pending promise along with its resolve and reject
// functions. The Go result is modeled on the ES2024
// {promise, resolve, reject} shape.
//
// Unlike the constructor pattern (new Promise(executor)), withResolvers returns
// the promise and its resolve/reject functions directly, making it easier to
// use in scenarios where the executor pattern is awkward:
//
//   - When you need to resolve/reject from outside the executor scope
//   - When integrating with callback-based APIs
//   - When building custom promise-based abstractions
//
// Returns:
//   - PromiseWithResolvers containing the Promise, Resolve, and Reject fields
//
// Example - Timer with cancellation:
//
//	func delayWithCancel(js *JS, ms int) (*PromiseWithResolvers) {
//	    r := js.WithResolvers()
//	    go func() {
//	        time.Sleep(time.Duration(ms) * time.Millisecond)
//	        r.Resolve(nil)
//	    }()
//	    return r
//	}
//
//	// Usage:
//	timer := delayWithCancel(js, 1000)
//	// Cancel early:
//	timer.Reject(errors.New("cancelled"))
//
// Example - Request/Response correlation:
//
//	pending := make(map[string]*PromiseWithResolvers)
//
//	func sendRequest(js *JS, id string, data any) *Promise {
//	    r := js.WithResolvers()
//	    pending[id] = r
//	    conn.Send(id, data)
//	    return r.Promise
//	}
//
//	func onResponse(id string, result any) {
//	    if r, ok := pending[id]; ok {
//	        r.Resolve(result)
//	        delete(pending, id)
//	    }
//	}
func (js *JS) WithResolvers() *PromiseWithResolvers {
	promise, resolve, reject := js.NewPromise()
	return &PromiseWithResolvers{
		Promise: promise,
		Resolve: resolve,
		Reject:  reject,
	}
}
