package eventloop

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// Result represents the value of a resolved or rejected promise.
// It can be any type, similar to JavaScript's dynamic typing.
// For fulfilled promises, this holds the success value.
// For rejected promises, this typically holds an error or rejection reason.
type Result = any

// PromiseState represents the lifecycle state of a [Promise].
// A promise starts in [Pending] state and transitions to either
// [Resolved] (also known as [Fulfilled]) or [Rejected].
// State transitions are irreversible.
type PromiseState int

const (
	// Pending indicates the promise operation is still in progress.
	// The promise has not yet been resolved or rejected.
	Pending PromiseState = iota

	// Resolved indicates the promise completed successfully with a value.
	// Fulfilled is an alias for Resolved, matching JavaScript terminology.
	Resolved

	// Rejected indicates the promise failed with a reason (typically an error).
	Rejected
)

const (
	// Fulfilled is an alias for [Resolved], matching the Promise/A+ specification.
	Fulfilled = Resolved
)

// Promise is a read-only view of a future result.
// It represents an asynchronous operation that will eventually complete
// with either a success value or a failure reason.
//
// For the full Promise/A+ implementation with Then/Catch/Finally chaining,
// see [ChainedPromise].
type Promise interface {
	// State returns the current [PromiseState] (Pending, Resolved, or Rejected).
	State() PromiseState

	// Result returns the result of the promise if settled, or nil if pending.
	// For resolved promises, returns the fulfillment value.
	// For rejected promises, returns the rejection reason.
	// Note: A resolved promise can legitimately have a nil result value.
	Result() Result

	// ToChannel returns a channel that will receive the result when the promise settles.
	// The channel is buffered (capacity 1) and will be closed after sending.
	// If the promise is already settled, returns a pre-filled channel.
	ToChannel() <-chan Result
}

// promise is the concrete implementation.
type promise struct {
	result      Result
	subscribers []chan Result // List of channels waiting for resolution
	state       PromiseState
	mu          sync.Mutex
}

var _ Promise = (*promise)(nil)

func (p *promise) State() PromiseState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

func (p *promise) Result() Result {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.result
}

// ToChannel returns a channel that will receive the result when settled.
func (p *promise) ToChannel() <-chan Result {
	p.mu.Lock()
	defer p.mu.Unlock()

	// If already settled, return a pre-filled, closed channel.
	if p.state != Pending {
		ch := make(chan Result, 1)
		ch <- p.result
		close(ch)
		return ch
	}

	ch := make(chan Result, 1)
	p.subscribers = append(p.subscribers, ch)
	return ch
}

// Resolve sets the promise state to Resolved and notifies all subscribers.
func (p *promise) Resolve(val Result) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != Pending {
		return
	}

	p.state = Fulfilled
	p.result = val
	p.fanOut()
}

// Reject sets the promise state to Rejected and notifies all subscribers.
func (p *promise) Reject(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.state != Pending {
		return
	}

	p.state = Rejected
	p.result = err
	p.fanOut()
}

// fanOut notifies all subscribers of the result and closes their channels.
// Must be called with p.mu held.
func (p *promise) fanOut() {
	for _, ch := range p.subscribers {
		select {
		case ch <- p.result:
		default:
			log.Printf("WARNING: eventloop: dropped promise result, channel full")
		}
		close(ch)
	}
	p.subscribers = nil // Release memory
}

// ============================================================================
// ChainedPromise Implementation (Task 1.6)
// ============================================================================

// ChainedPromise implements the Promise/A+ specification with [Then], [Catch], and [Finally].
//
// This is the JavaScript-compatible promise implementation with proper async semantics.
// All handler callbacks are scheduled as microtasks and executed on the event loop thread.
//
// Creating Promises:
//
//	promise, resolve, reject := js.NewChainedPromise()
//	go func() {
//	    result, err := doAsyncWork()
//	    if err != nil {
//	        reject(err)
//	    } else {
//	        resolve(result)
//	    }
//	}()
//
// Chaining:
//
//	promise.
//	    Then(func(v Result) Result {
//	        return transform(v)
//	    }, nil).
//	    Catch(func(r Result) Result {
//	        log.Printf("Error: %v", r)
//	        return nil // recover from error
//	    }).
//	    Finally(func() {
//	        cleanup()
//	    })
//
// Thread Safety:
//
// ChainedPromise is safe for concurrent use. The resolve/reject functions can be
// called from any goroutine, but handlers always execute on the event loop thread.
type ChainedPromise struct {
	value  Result
	reason Result

	// Pointer fields (all require 8-byte alignment, grouped last)
	js       *JS
	handlers []handler

	// Non-pointer, non-atomic fields (no pointer alignment needed)
	id uint64

	// Non-pointer synchronization primitives
	mu sync.RWMutex

	// Atomic state (requires 8-byte alignment, grouped)
	state atomic.Int32
	_     [4]byte // Padding to 8-byte

}

// handler represents a reaction to promise settlement.
type handler struct {
	onFulfilled func(Result) Result
	onRejected  func(Result) Result
	resolve     func(Result)
	reject      func(Result)
}

// ResolveFunc is the function used to fulfill a promise with a value.
// Calling resolve on an already-settled promise has no effect.
// Can be called from any goroutine.
type ResolveFunc func(Result)

// RejectFunc is the function used to reject a promise with a reason.
// Calling reject on an already-settled promise has no effect.
// Can be called from any goroutine.
type RejectFunc func(Result)

// NewChainedPromise creates a new pending promise along with resolve and reject functions.
//
// Returns:
//   - promise: The new [ChainedPromise] in Pending state
//   - resolve: Function to fulfill the promise with a value
//   - reject: Function to reject the promise with a reason
//
// Example:
//
//	promise, resolve, reject := js.NewChainedPromise()
//	go func() {
//	    result, err := doWork()
//	    if err != nil {
//	        reject(err)
//	    } else {
//	        resolve(result)
//	    }
//	}()
//
// The resolve and reject functions can be called from any goroutine.
// Only the first call has an effect; subsequent calls are ignored.
func (js *JS) NewChainedPromise() (*ChainedPromise, ResolveFunc, RejectFunc) {
	p := &ChainedPromise{
		// Start in Pending state (0)
		handlers: make([]handler, 0, 2),
		id:       js.nextTimerID.Add(1),
		js:       js,
	}
	p.state.Store(int32(Pending))

	resolve := func(value Result) {
		p.resolve(value, js)
	}

	reject := func(reason Result) {
		p.reject(reason, js)
	}

	return p, resolve, reject
}

// State returns the current [PromiseState] of this promise.
// Thread-safe and can be called from any goroutine.
func (p *ChainedPromise) State() PromiseState {
	return PromiseState(p.state.Load())
}

// Value returns the fulfillment value if the promise is fulfilled.
// Returns nil if the promise is pending or rejected.
// Thread-safe and can be called from any goroutine.
func (p *ChainedPromise) Value() Result {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.state.Load() != int32(Fulfilled) {
		return nil
	}
	return p.value
}

// Reason returns the rejection reason if the promise is rejected.
// Returns nil if the promise is pending or fulfilled.
// Thread-safe and can be called from any goroutine.
func (p *ChainedPromise) Reason() Result {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.state.Load() != int32(Rejected) {
		return nil
	}
	return p.reason
}

func (p *ChainedPromise) resolve(value Result, js *JS) {
	// Spec 2.3.1: If promise and x refer to the same object, reject promise with a TypeError.
	// (We can't easily check identity if value is wrapped, but basic check is good)
	if pr, ok := value.(*ChainedPromise); ok && pr == p {
		p.reject(fmt.Errorf("TypeError: Chaining cycle detected for promise #%d", p.id), js)
		return
	}

	// Spec 2.3.2: If x is a promise, adopt its state.
	if pr, ok := value.(*ChainedPromise); ok {
		// Wait for pr to settle, then resolve/reject p with the result
		// We use ThenWithJS to attach standard handlers
		pr.ThenWithJS(js,
			func(v Result) Result {
				p.resolve(v, js) // Recursive resolution (2.3.2.1)
				return nil
			},
			func(r Result) Result {
				p.reject(r, js) // (2.3.2.3)
				return nil
			},
		)
		return
	}

	if !p.state.CompareAndSwap(int32(Pending), int32(Fulfilled)) {
		// Already settled
		return
	}

	p.mu.Lock()
	p.value = value
	handlers := p.handlers
	p.handlers = nil // Clear handlers slice after copying to prevent memory leak
	p.mu.Unlock()

	// CLEANUP: Prevent leak on success - remove from promiseHandlers map
	// This fixes Memory Leak #1 from review.md Section 2.A
	if js != nil {
		js.promiseHandlersMu.Lock()
		delete(js.promiseHandlers, p.id)
		js.promiseHandlersMu.Unlock()
	}

	// Schedule handlers as microtasks
	for _, h := range handlers {
		// Promise/A+ 2.2.7.3: If onFulfilled is not a function and promise1 is fulfilled, promise2 must be fulfilled with the same value as promise1.
		if h.onFulfilled != nil {
			fn := h.onFulfilled
			result := h
			js.QueueMicrotask(func() {
				tryCall(fn, value, result.resolve, result.reject)
			})
		} else {
			// Propagate fulfillment
			h.resolve(value)
		}
	}
}

// reject transitions the promise to rejected state if it's still pending.
func (p *ChainedPromise) reject(reason Result, js *JS) {
	if !p.state.CompareAndSwap(int32(Pending), int32(Rejected)) {
		// Already settled
		return
	}

	p.mu.Lock()
	p.reason = reason
	handlers := p.handlers
	p.handlers = nil // Clear handlers slice after copying to prevent memory leak
	p.mu.Unlock()

	// Schedule handler microtasks FIRST
	// This ensures handlers are attached before unhandled rejection check runs
	for _, h := range handlers {
		// Promise/A+ 2.2.7.4: If onRejected is not a function and promise1 is rejected, promise2 must be rejected with the same reason as promise1.
		if h.onRejected != nil {
			fn := h.onRejected
			result := h
			js.QueueMicrotask(func() {
				tryCall(fn, reason, result.resolve, result.reject)
			})
		} else {
			// Propagate rejection
			h.reject(reason)
		}
	}

	// THEN schedule rejection check microtask (will run AFTER all handlers)
	// This fixes a timing race where check ran before handlers were scheduled
	js.trackRejection(p.id, reason)
}

// Then adds handlers to be called when the promise settles.
// Returns a new [ChainedPromise] that resolves with the result of the handler.
//
// Parameters:
//   - onFulfilled: Handler called with the fulfillment value. Can be nil.
//   - onRejected: Handler called with the rejection reason. Can be nil.
//
// Handler Return Values:
//   - If a handler returns a value, the returned promise resolves with that value
//   - If a handler panics, the returned promise rejects with the panic value
//   - If a handler is nil, the result passes through to the returned promise
//
// Handlers are always executed as microtasks on the event loop thread.
func (p *ChainedPromise) Then(onFulfilled, onRejected func(Result) Result) *ChainedPromise {
	js := p.js
	if js == nil {
		// No JS adapter available, create standalone promise without scheduling
		return p.thenStandalone(onFulfilled, onRejected)
	}
	return p.then(js, onFulfilled, onRejected)
}

// ThenWithJS adds handlers with explicit JS adapter.
func (p *ChainedPromise) ThenWithJS(js *JS, onFulfilled, onRejected func(Result) Result) *ChainedPromise {
	return p.then(js, onFulfilled, onRejected)
}

func (p *ChainedPromise) then(js *JS, onFulfilled, onRejected func(Result) Result) *ChainedPromise {
	result := &ChainedPromise{
		handlers: make([]handler, 0, 2),
		id:       js.nextTimerID.Add(1),
		js:       js,
	}
	result.state.Store(int32(Pending))

	resolve := func(value Result) {
		result.resolve(value, js)
	}

	reject := func(reason Result) {
		result.reject(reason, js)
	}

	h := handler{
		onFulfilled: onFulfilled,
		onRejected:  onRejected,
		resolve:     resolve,
		reject:      reject,
	}

	// Mark that this promise now has a handler attached
	if onRejected != nil {
		js.promiseHandlersMu.Lock()
		js.promiseHandlers[p.id] = true
		js.promiseHandlersMu.Unlock()
	}

	// Check current state
	currentState := p.state.Load()

	if currentState == int32(Pending) {
		// Pending: store handler
		p.mu.Lock()
		p.handlers = append(p.handlers, h)
		p.mu.Unlock()
	} else {
		// Already settled: retroactive cleanup for settled promises - This fixes Memory Leak #3 from review.md Section 2.A
		if onRejected != nil && currentState == int32(Fulfilled) {
			// Fulfilled promises don't need rejection tracking (can never be rejected)
			js.promiseHandlersMu.Lock()
			delete(js.promiseHandlers, p.id)
			js.promiseHandlersMu.Unlock()
		} else if onRejected != nil && currentState == int32(Rejected) {
			// Rejected promises: only track if currently unhandled
			js.rejectionsMu.RLock()
			_, isUnhandled := js.unhandledRejections[p.id]
			js.rejectionsMu.RUnlock()

			if !isUnhandled {
				// Already handled, remove tracking
				js.promiseHandlersMu.Lock()
				delete(js.promiseHandlers, p.id)
				js.promiseHandlersMu.Unlock()
			}

			// Schedule handler as microtask for already-rejected promise
			r := p.Reason()
			js.QueueMicrotask(func() {
				tryCall(onRejected, r, resolve, reject)
			})
			return result
		}

		// Schedule handler as microtask for already-fulfilled promise
		v := p.Value()
		js.QueueMicrotask(func() {
			tryCall(onFulfilled, v, resolve, reject)
		})
	}

	return result
}

// thenStandalone creates a child promise without JS adapter for basic operations.
//
// NOTE: This code path is NOT Promise/A+ compliant - handlers execute synchronously
// when called on already-settled promises. This is intentional for testing/fallback
// scenarios where a JS adapter is not available. Normal usage always goes through
// js.NewChainedPromise() which provides proper async semantics via microtasks.
//
// In production code, p.js should never be nil because promises are created
// via js.NewChainedPromise() which always sets the js field. This path is
// provided only for testing or future extensions where a standalone promise might be
// useful without an event loop.
func (p *ChainedPromise) thenStandalone(onFulfilled, onRejected func(Result) Result) *ChainedPromise {
	result := &ChainedPromise{
		handlers: make([]handler, 0, 2),
		id:       p.id + 1, // Simple ID generation for child
		js:       nil,
	}
	result.state.Store(int32(Pending))

	resolve := func(value Result) {
		if result.state.CompareAndSwap(int32(Pending), int32(Fulfilled)) {
			result.mu.Lock()
			result.value = value
			result.mu.Unlock()
		}
	}

	reject := func(reason Result) {
		if result.state.CompareAndSwap(int32(Pending), int32(Rejected)) {
			result.mu.Lock()
			result.reason = reason
			result.mu.Unlock()
		}
	}

	h := handler{
		onFulfilled: onFulfilled,
		onRejected:  onRejected,
		resolve:     resolve,
		reject:      reject,
	}

	// Check current state
	currentState := p.state.Load()

	if currentState == int32(Pending) {
		// Pending: store handler
		p.mu.Lock()
		p.handlers = append(p.handlers, h)
		p.mu.Unlock()
	} else {
		// Already settled: call handler synchronously (not spec-compliant)
		if currentState == int32(Fulfilled) && onFulfilled != nil {
			v := p.Value()
			tryCall(onFulfilled, v, resolve, reject)
		} else if currentState == int32(Rejected) && onRejected != nil {
			r := p.Reason()
			tryCall(onRejected, r, resolve, reject)
		}
	}

	return result
}

// Catch adds a rejection handler to the promise.
// Returns a new [ChainedPromise] that resolves with the result of the handler.
//
// This is equivalent to calling Then(nil, onRejected).
//
// Use Catch to recover from errors or transform rejection reasons:
//
//	promise.Catch(func(r Result) Result {
//	    log.Printf("Error: %v", r)
//	    return defaultValue // recover
//	})
func (p *ChainedPromise) Catch(onRejected func(Result) Result) *ChainedPromise {
	return p.Then(nil, onRejected)
}

// Finally adds a handler that runs regardless of how the promise settles.
// Returns a new [ChainedPromise] that preserves the original settlement.
//
// Unlike Then/Catch, the onFinally callback receives no arguments and its
// return value is ignored. The promise returned by Finally will settle with
// the same value/reason as the original promise.
//
// Use Finally for cleanup operations:
//
//	promise.
//	    Then(processResult, nil).
//	    Finally(func() {
//	        closeResources()
//	    })
func (p *ChainedPromise) Finally(onFinally func()) *ChainedPromise {
	js := p.js
	var result *ChainedPromise
	var resolve ResolveFunc
	var reject RejectFunc

	if js != nil {
		result, resolve, reject = js.NewChainedPromise()
	} else {
		result = &ChainedPromise{
			handlers: make([]handler, 0, 2),
			id:       p.id + 1,
			js:       nil,
		}
		result.state.Store(int32(Pending))
		resolve = func(value Result) {
			if result.state.CompareAndSwap(int32(Pending), int32(Fulfilled)) {
				result.mu.Lock()
				result.value = value
				result.mu.Unlock()
			}
		}
		reject = func(reason Result) {
			if result.state.CompareAndSwap(int32(Pending), int32(Rejected)) {
				result.mu.Lock()
				result.reason = reason
				result.mu.Unlock()
			}
		}
	}

	if onFinally == nil {
		onFinally = func() {}
	}

	// Mark that this promise now has a handler attached
	// Finally counts as handling rejection (it runs whether fulfilled or rejected)
	if js != nil {
		js.promiseHandlersMu.Lock()
		js.promiseHandlers[p.id] = true
		js.promiseHandlersMu.Unlock()
	}

	// Create handler that runs onFinally then forwards result
	handlerFunc := func(value Result, isRejection bool, res ResolveFunc, rej RejectFunc) {
		onFinally()
		if isRejection {
			rej(value)
		} else {
			res(value)
		}
	}

	// Check current state
	currentState := p.state.Load()

	if currentState == int32(Pending) {
		// Pending: store custom handlers
		p.mu.Lock()
		// We need to duplicate handler logic for both success and failure paths
		// because finally needs to run in both cases
		p.handlers = append(p.handlers, handler{
			onFulfilled: func(v Result) Result {
				handlerFunc(v, false, resolve, reject)
				return nil // Result already delivered
			},
			resolve: resolve,
			reject:  reject,
		})
		p.handlers = append(p.handlers, handler{
			onRejected: func(r Result) Result {
				handlerFunc(r, true, resolve, reject)
				return nil
			},
			resolve: resolve,
			reject:  reject,
		})
		p.mu.Unlock()
	} else {
		// Already settled: run onFinally and forward result
		if currentState == int32(Fulfilled) {
			handlerFunc(p.Value(), false, resolve, reject)
		} else {
			handlerFunc(p.Reason(), true, resolve, reject)
		}
	}

	return result
}

// tryCall calls a function with the given value, resolving or rejecting on error/panic.
func tryCall(fn func(Result) Result, v Result, resolve ResolveFunc, reject RejectFunc) {
	defer func() {
		if r := recover(); r != nil {
			reject(r)
		}
	}()

	// CRITICAL: Check for nil handler before calling
	if fn == nil {
		// No handler means pass-through
		resolve(v)
		return
	}

	result := fn(v)
	resolve(result)
}

// trackRejection tracks a rejected promise for unhandled rejection detection.
// This is called from the reject() method.
func (js *JS) trackRejection(promiseID uint64, reason Result) {
	// Store rejection info
	info := &rejectionInfo{
		promiseID: promiseID,
		reason:    reason,
		timestamp: time.Now().UnixNano(),
	}
	js.rejectionsMu.Lock()
	js.unhandledRejections[promiseID] = info
	js.rejectionsMu.Unlock()

	// Schedule a microtask to check if this rejection was handled
	js.loop.ScheduleMicrotask(func() {
		js.checkUnhandledRejections()
	})
}

// checkUnhandledRejections checks for rejections without handlers and reports them.
func (js *JS) checkUnhandledRejections() {
	// Get the unhandled rejection callback if any
	js.mu.Lock()
	callback := js.unhandledCallback
	js.mu.Unlock()

	// Collect snapshot of rejections to iterate safely
	js.rejectionsMu.RLock()
	// Early exit
	if len(js.unhandledRejections) == 0 {
		js.rejectionsMu.RUnlock()
		return
	}

	snapshot := make([]*rejectionInfo, 0, len(js.unhandledRejections))
	for _, info := range js.unhandledRejections {
		snapshot = append(snapshot, info)
	}
	js.rejectionsMu.RUnlock()

	// Process snapshot
	for _, info := range snapshot {
		promiseID := info.promiseID

		js.promiseHandlersMu.RLock()
		handled, exists := js.promiseHandlers[promiseID]
		js.promiseHandlersMu.RUnlock()

		// If we can't find a handler, or it's explicitly marked as unhandled (though map is bool true), report it
		if !exists || !handled {
			if callback != nil {
				callback(info.reason)
			}
		}

		// Clean up tracking
		js.rejectionsMu.Lock()
		delete(js.unhandledRejections, promiseID)
		js.rejectionsMu.Unlock()

		js.promiseHandlersMu.Lock()
		delete(js.promiseHandlers, promiseID)
		js.promiseHandlersMu.Unlock()
	}
}

// rejectionInfo holds information about a rejected promise.
type rejectionInfo struct {
	reason    Result // Largest field, put first
	promiseID uint64
	timestamp int64
}

// ============================================================================
// Promise Combinators (Task 3.x)
// ============================================================================

// All returns a promise that resolves when all input promises resolve.
//
// Behavior:
//   - If promises is empty, resolves immediately with an empty slice
//   - Resolves with a slice of values in the same order as the input promises
//   - Rejects immediately when any promise rejects, with that promise's reason
//
// Example:
//
//	p1, resolve1, _ := js.NewChainedPromise()
//	p2, resolve2, _ := js.NewChainedPromise()
//	go func() {
//	    resolve1("a")
//	    resolve2("b")
//	}()
//	// result will be []Result{"a", "b"}
//	result := js.All([]*ChainedPromise{p1, p2})
func (js *JS) All(promises []*ChainedPromise) *ChainedPromise {
	result, resolve, reject := js.NewChainedPromise()

	// Handle empty array - resolve immediately with empty array
	if len(promises) == 0 {
		resolve(make([]Result, 0))
		return result
	}

	// Track completion
	var mu sync.Mutex
	var completed atomic.Int32
	values := make([]Result, len(promises))
	hasRejected := atomic.Bool{}

	// Attach handlers to each promise
	for i, p := range promises {
		idx := i // Capture index
		p.ThenWithJS(js,
			func(v Result) Result {
				// Store value in correct position
				mu.Lock()
				values[idx] = v
				mu.Unlock()

				// Check if all promises resolved
				count := completed.Add(1)
				if count == int32(len(promises)) && !hasRejected.Load() {
					resolve(values)
				}
				return nil
			},
			nil,
		)

		// Reject on first rejection
		p.ThenWithJS(js,
			nil,
			func(r Result) Result {
				if hasRejected.CompareAndSwap(false, true) {
					reject(r)
				}
				return nil
			},
		)
	}

	return result
}

// Race returns a promise that settles as soon as any of the input promises settles.
//
// Behavior:
//   - If promises is empty, the returned promise never settles (remains pending)
//   - Settles with the value/reason of the first promise to settle
//   - Ignores subsequent settlements from other promises
//
// Use Race for timeout patterns:
//
//	timeout, _, rejectTimeout := js.NewChainedPromise()
//	go func() {
//	    time.Sleep(5 * time.Second)
//	    rejectTimeout(errors.New("timeout"))
//	}()
//	result := js.Race([]*ChainedPromise{actualWork, timeout})
func (js *JS) Race(promises []*ChainedPromise) *ChainedPromise {
	result, resolve, reject := js.NewChainedPromise()

	// Handle empty array - never settles
	if len(promises) == 0 {
		return result
	}

	var settled atomic.Bool

	// Attach handlers to each promise (first to settle wins)
	for _, p := range promises {
		p.ThenWithJS(js,
			func(v Result) Result {
				if settled.CompareAndSwap(false, true) {
					resolve(v)
				}
				return nil
			},
			func(r Result) Result {
				if settled.CompareAndSwap(false, true) {
					reject(r)
				}
				return nil
			},
		)
	}

	return result
}

// AllSettled returns a promise that resolves when all input promises have settled.
//
// Unlike [JS.All], this never rejects - it waits for all promises to complete.
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
//   - Always resolves (never rejects)
//   - Results are in the same order as the input promises
func (js *JS) AllSettled(promises []*ChainedPromise) *ChainedPromise {
	result, resolve, _ := js.NewChainedPromise()

	// Handle empty array - resolve immediately with empty array
	if len(promises) == 0 {
		resolve(make([]Result, 0))
		return result
	}

	// Track completion
	var mu sync.Mutex
	var completed atomic.Int32
	results := make([]Result, len(promises))

	for i, p := range promises {
		idx := i // Capture index
		p.ThenWithJS(js,
			func(v Result) Result {
				mu.Lock()
				results[idx] = map[string]interface{}{
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
			func(r Result) Result {
				mu.Lock()
				results[idx] = map[string]interface{}{
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
		)
	}

	return result
}

// Any returns a promise that resolves when any input promise resolves.
//
// Behavior:
//   - If promises is empty, rejects immediately with [AggregateError]
//   - Resolves with the value of the first promise to resolve
//   - Rejects with [AggregateError] only if ALL promises reject
//
// Use Any when you need at least one success:
//
//	// Try multiple data sources, use first successful response
//	result := js.Any([]*ChainedPromise{source1, source2, source3})
func (js *JS) Any(promises []*ChainedPromise) *ChainedPromise {
	result, resolve, reject := js.NewChainedPromise()

	// Handle empty array - reject immediately
	if len(promises) == 0 {
		reject(&AggregateError{
			Errors: []error{&ErrNoPromiseResolved{}},
		})
		return result
	}

	var mu sync.Mutex
	var rejected atomic.Int32
	rejections := make([]Result, len(promises))
	var resolved atomic.Bool

	// Attach handlers to each promise
	for i, p := range promises {
		idx := i // Capture index
		p.ThenWithJS(js,
			func(v Result) Result {
				if resolved.CompareAndSwap(false, true) {
					resolve(v)
				}
				return nil
			},
			func(r Result) Result {
				mu.Lock()
				rejections[idx] = r
				mu.Unlock()

				count := rejected.Add(1)
				// If all rejected and none resolved, aggregate errors
				if count == int32(len(promises)) && !resolved.Load() {
					// Convert rejections to error interface
					errors := make([]error, len(rejections))
					for i, r := range rejections {
						if err, ok := r.(error); ok {
							errors[i] = err
						} else {
							errors[i] = &ErrorWrapper{Value: r}
						}
					}
					reject(&AggregateError{
						Errors:  errors,
						Message: "All promises were rejected",
					})
				}
				return nil
			},
		)
	}

	return result
}

// AggregateError represents an error thrown when [JS.Any] fails because
// all input promises were rejected.
//
// The Errors field contains the rejection reasons from all failed promises,
// preserving the order of the input promises array.
//
// Example:
//
//	promise := js.Any([]*ChainedPromise{
//	    js.Reject(errors.New("error 1")),
//	    js.Reject(errors.New("error 2")),
//	})
//	promise.Catch(func(r Result) Result {
//	    if agg, ok := r.(*AggregateError); ok {
//	        fmt.Printf("All failed. Errors:\n")
//	        for i, err := range agg.Errors {
//	            fmt.Printf("  [%d] %v\n", i, err)
//	        }
//	    }
//	    return nil
//	})
type AggregateError struct {
	// Message matches standard JS AggregateError property
	Message string
	// Errors contains all rejection reasons from failed promises.
	// The order matches the input promises array to [JS.Any].
	Errors []error
}

// Error implements the error interface.
// Returns "All promises were rejected" as a generic message.
// Individual rejection reasons can be accessed via the [Errors] field.
func (e *AggregateError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "All promises were rejected"
}

// ErrNoPromiseResolved indicates that [JS.Any] was called with an empty array.
type ErrNoPromiseResolved struct{}

// Error implements the error interface.
func (e *ErrNoPromiseResolved) Error() string {
	return "No promises were provided"
}

// ErrorWrapper wraps a non-error value as an error for [AggregateError] compatibility.
type ErrorWrapper struct {
	// Value is the original non-error rejection reason.
	Value Result
}

// Error implements the error interface.
func (e *ErrorWrapper) Error() string {
	return fmt.Sprintf("%v", e.Value)
}
