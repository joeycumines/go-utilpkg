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
	// Pointer fields (all require 8-byte alignment, grouped first for better cache locality)
	result Result
	js     *JS
	// h0 is the first handler (embedded to avoid slice allocation).
	// Most promises have only 1 handler.
	h0 handler
	// channels stores channels from ToChannel() calls
	// Set during pending state, cleared after settlement
	channels []chan Result

	// Atomic state (requires 8-byte alignment)
	state atomic.Int32
	// Non-pointer, non-atomic fields
	id uint64

	// Non-pointer synchronization primitives
	mu sync.Mutex
}

// handler represents a reaction to promise settlement.
type handler struct {
	onFulfilled func(Result) Result
	onRejected  func(Result) Result
	target      *ChainedPromise
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
		id: js.nextTimerID.Add(1),
		js: js,
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
	if p.state.Load() == int32(Fulfilled) {
		return p.result
	}
	return nil
}

// Reason returns the rejection reason if the promise is rejected.
// Returns nil if the promise is pending or fulfilled.
// Thread-safe and can be called from any goroutine.
func (p *ChainedPromise) Reason() Result {
	if p.state.Load() == int32(Rejected) {
		return p.result
	}
	return nil
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

	p.mu.Lock()
	if p.state.Load() != int32(Pending) {
		p.mu.Unlock()
		return
	}

	h0 := p.h0
	var handlers []handler

	// Extract handlers before they get overwritten with the actual result
	if p.result != nil {
		if hrs, ok := p.result.([]handler); ok {
			handlers = hrs
		}
	}

	// Extract channels for notification
	channels := p.channels
	p.channels = nil // Clear channels to prevent double-notification

	p.h0 = handler{}
	p.result = value
	p.state.Store(int32(Fulfilled))
	p.mu.Unlock()

	// Notify all channels registered via ToChannel()
	for _, ch := range channels {
		select {
		case ch <- value:
		default:
			// Channel might be full or closed, skip
		}
	}
	// Close all channels
	for _, ch := range channels {
		close(ch)
	}

	// CLEANUP: Prevent leak on success
	if js != nil {
		js.promiseHandlersMu.Lock()
		delete(js.promiseHandlers, p.id)
		js.promiseHandlersMu.Unlock()
	}

	process := func(h handler) {
		if h.onFulfilled != nil {
			fn := h.onFulfilled
			target := h.target
			if js != nil {
				js.QueueMicrotask(func() {
					tryCall(fn, value, target)
				})
			} else {
				tryCall(fn, value, target)
			}
		} else {
			// Propagate fulfillment
			h.target.resolve(value, h.target.js)
		}
	}

	if h0.target != nil {
		process(h0)
	}
	for _, h := range handlers {
		process(h)
	}
}

// reject transitions the promise to rejected state if it's still pending.
func (p *ChainedPromise) reject(reason Result, js *JS) {
	p.mu.Lock()
	if p.state.Load() != int32(Pending) {
		p.mu.Unlock()
		return
	}

	// CRITICAL FIX (T27): Snapshot handlers BEFORE clearing them
	// then() runs concurrently: checks state, then acquires p.mu
	// If then() sees Pending and stores handler AFTER we clear p.h0
	// but BEFORE release lock, that handler must be processed too
	// Solution: Don't clear handlers until AFTER processing them
	h0 := p.h0
	var handlers []handler

	// Extract handlers before they get overwritten with the actual reason
	if p.result != nil {
		if hrs, ok := p.result.([]handler); ok {
			handlers = hrs
		}
	}

	// Extract channels for notification
	channels := p.channels
	p.channels = nil // Clear channels to prevent double-notification

	// Set reason BEFORE processing handlers (needed for error messages)
	p.result = reason
	p.state.Store(int32(Rejected))

	// CRITICAL FIX (T27): Process handlers BEFORE clearing them
	// This ensures any handlers added by concurrent then() are processed
	// then() checks p.state at line ~457, then acquires p.mu at line ~462
	// If then() won the race and added handler, we MUST process it
	process := func(h handler) {
		if h.onRejected != nil {
			fn := h.onRejected
			target := h.target
			if js != nil {
				js.QueueMicrotask(func() {
					tryCall(fn, reason, target)
				})
			} else {
				tryCall(fn, reason, target)
			}
		} else {
			// Propagate rejection
			h.target.reject(reason, h.target.js)
		}
	}

	// CRITICAL FIX (T27): Enqueue handler microtasks FIRST (while holding lock)
	// THEN release lock to allow then() to add more handlers
	// THEN call trackRejection() (which enqueues checkUnhandledRejections)
	//
	// Microtask ordering after this fix:
	// 1. Handler microtasks from this loop (queued at line ~388-389)
	// 2. Late-subscriber microtasks from then() (queued at line ~492)
	// 3. checkUnhandledRejections microtask from trackRejection (queued at line ~737)
	//
	// This ensures checkUnhandledRejections sees handlers added by concurrent then()
	if h0.target != nil {
		process(h0)
	}
	for _, h := range handlers {
		process(h)
	}

	// Clear handlers AFTER enqueuing their microtasks
	p.h0 = handler{}

	// Notify all channels registered via ToChannel()
	for _, ch := range channels {
		select {
		case ch <- reason:
		default:
			// Channel might be full or closed, skip
		}
	}
	// Close all channels
	for _, ch := range channels {
		close(ch)
	}

	p.mu.Unlock()

	// NOW call trackRejection (AFTER releasing lock, AFTER enqueuing handlers)
	// This queues checkUnhandledRejections to run AFTER all handler microtasks
	if js != nil {
		js.trackRejection(p.id, reason)
	}
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
		id: js.nextTimerID.Add(1),
		js: js,
	}
	result.state.Store(int32(Pending))

	h := handler{
		onFulfilled: onFulfilled,
		onRejected:  onRejected,
		target:      result,
	}

	// NOTE: We DO NOT set js.promiseHandlers[p.id] = true here anymore.
	// Instead, we set it AFTER the handler is stored, inside the critical section.
	// This ensures trackRejection sees the handler BEFORE checking unhandled rejections.

	// Check current state
	currentState := p.state.Load()

	if currentState == int32(Pending) {
		// CRITICAL FIX (T27): Re-check state after acquiring lock to handle race
		// then() checks p.state above, then acquires p.mu
		// If reject() runs concurrently, it may have left Rejected state
		// Snapshot taken when Pending is now stale
		// Solution: Re-read state under lock to catch this race
		p.mu.Lock()
		currentState = p.state.Load()

		// If state changed from Pending to [Rejected|Fulfilled] during race,
		// follow the "already settled" path instead
		if currentState != int32(Pending) {
			p.mu.Unlock()
			// Fall through to "already settled" handling below
			if onRejected != nil && currentState == int32(Fulfilled) {
				// Fulfilled promises don't need rejection tracking
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
				// ALSO signal handler ready for trackRejection synchronization
				if onRejected != nil {
					js.promiseHandlersMu.Lock()
					js.promiseHandlers[p.id] = true
					js.promiseHandlersMu.Unlock()
					// Signal that handler is registered (for trackRejection synchronization)
					js.handlerReadyMu.Lock()
					if ch, exists := js.handlerReadyChans[p.id]; exists {
						// Close channel to signal handler registration
						// Use select with default to avoid blocking if no one is waiting
						select {
						case <-ch:
							// Already closed
						default:
							close(ch)
						}
					}
					js.handlerReadyMu.Unlock()
				}

				r := p.Reason()
				js.QueueMicrotask(func() {
					tryCall(onRejected, r, result)
				})
				return result
			}

			// Schedule handler as microtask for already-fulfilled promise
			v := p.Value()
			js.QueueMicrotask(func() {
				tryCall(onFulfilled, v, result)
			})
			return result
		}

		// Still pending: store handler FIRST, then mark as tracked
		if p.h0.target == nil {
			p.h0 = h
		} else {
			var handlers []handler
			if p.result == nil {
				handlers = make([]handler, 0, 2)
			} else {
				handlers = p.result.([]handler)
			}
			handlers = append(handlers, h)
			p.result = handlers
		}
		p.mu.Unlock()

		// NOW mark as tracked AFTER storing the handler
		// This ensures trackRejection sees the handler before checking
		if onRejected != nil {
			js.promiseHandlersMu.Lock()
			js.promiseHandlers[p.id] = true
			js.promiseHandlersMu.Unlock()

			// Signal that handler is registered (for trackRejection synchronization)
			js.handlerReadyMu.Lock()
			if ch, exists := js.handlerReadyChans[p.id]; exists {
				// Close channel to signal handler registration
				// Use select with default to avoid blocking if no one is waiting
				select {
				case <-ch:
					// Already closed
				default:
					close(ch)
				}
			}
			js.handlerReadyMu.Unlock()
		}
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
			// ONLY track if currently unhandled (for trackRejection synchronization)
			if isUnhandled {
				js.promiseHandlersMu.Lock()
				js.promiseHandlers[p.id] = true
				js.promiseHandlersMu.Unlock()
				// Signal that handler is registered (for trackRejection synchronization)
				js.handlerReadyMu.Lock()
				if ch, exists := js.handlerReadyChans[p.id]; exists {
					// Close channel to signal handler registration
					// Use select with default to avoid blocking if no one is waiting
					select {
					case <-ch:
						// Already closed
					default:
						close(ch)
					}
				}
				js.handlerReadyMu.Unlock()
			}

			r := p.Reason()
			js.QueueMicrotask(func() {
				tryCall(onRejected, r, result)
			})
			return result
		}

		// Schedule handler as microtask for already-fulfilled promise
		v := p.Value()
		js.QueueMicrotask(func() {
			tryCall(onFulfilled, v, result)
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
		id: p.id + 1, // Simple ID generation for child
		js: nil,
	}
	result.state.Store(int32(Pending))

	h := handler{
		onFulfilled: onFulfilled,
		onRejected:  onRejected,
		target:      result,
	}

	// Check current state
	currentState := p.state.Load()

	if currentState == int32(Pending) {
		// Pending: store handler
		p.mu.Lock()
		if p.h0.target == nil {
			p.h0 = h
		} else {
			var handlers []handler
			if p.result == nil {
				handlers = make([]handler, 0, 2)
			} else {
				handlers = p.result.([]handler)
			}
			handlers = append(handlers, h)
			p.result = handlers
		}
		p.mu.Unlock()
	} else {
		// Already settled: call handler synchronously (not spec-compliant)
		if currentState == int32(Fulfilled) {
			v := p.Value()
			if onFulfilled != nil {
				tryCall(onFulfilled, v, result)
			} else {
				// Nil handler means pass-through
				result.resolve(v, result.js)
			}
		} else if currentState == int32(Rejected) {
			r := p.Reason()
			if onRejected != nil {
				tryCall(onRejected, r, result)
			} else {
				// Nil handler means pass-through rejection
				result.reject(r, result.js)
			}
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
	if js != nil {
		result, _, _ = js.NewChainedPromise()
	} else {
		result = &ChainedPromise{
			id: p.id + 1,
			js: nil,
		}
		result.state.Store(int32(Pending))
	}

	if onFinally == nil {
		onFinally = func() {}
	}

	// NOTE: We DO NOT set js.promiseHandlers[p.id] = true here anymore.
	// Instead, we set it AFTER the handlers are stored, inside the critical section.
	// This ensures trackRejection sees the handler BEFORE checking unhandled rejections.

	// Create handler that runs onFinally then forwards result
	handlerFunc := func(value Result, isRejection bool, target *ChainedPromise) {
		onFinally()
		if isRejection {
			target.reject(value, target.js)
		} else {
			target.resolve(value, target.js)
		}
	}

	// Check current state
	currentState := p.state.Load()

	if currentState == int32(Pending) {
		// Pending: store custom handlers
		p.mu.Lock()
		// We need to duplicate handler logic for both success and failure paths
		// because finally needs to run in both cases
		h1 := handler{
			onFulfilled: func(v Result) Result {
				handlerFunc(v, false, result)
				return nil // Result already delivered
			},
			target: result,
		}
		h2 := handler{
			onRejected: func(r Result) Result {
				handlerFunc(r, true, result)
				return nil
			},
			target: result,
		}

		// Append h1
		if p.h0.target == nil {
			p.h0 = h1
		} else {
			var handlers []handler
			if p.result == nil {
				handlers = make([]handler, 0, 2)
			} else {
				handlers = p.result.([]handler)
			}
			handlers = append(handlers, h1)
			p.result = handlers
		}

		// Append h2
		// Re-read slice in case it changed/allocated
		if p.h0.target == nil {
			p.h0 = h2
		} else {
			var handlers []handler
			if p.result == nil {
				handlers = make([]handler, 0, 2)
			} else {
				handlers = p.result.([]handler)
			}
			handlers = append(handlers, h2)
			p.result = handlers
		}
		p.mu.Unlock()

		// NOW mark as tracked AFTER storing the handlers
		// This ensures trackRejection sees the handler before checking
		js.promiseHandlersMu.Lock()
		js.promiseHandlers[p.id] = true
		js.promiseHandlersMu.Unlock()

		// Signal that handler is registered (for trackRejection synchronization)
		js.handlerReadyMu.Lock()
		if ch, exists := js.handlerReadyChans[p.id]; exists {
			// Close channel to signal handler registration
			// Use select with default to avoid blocking if no one is waiting
			select {
			case <-ch:
				// Already closed
			default:
				close(ch)
			}
		}
		js.handlerReadyMu.Unlock()
	} else {
		// Already settled: run onFinally and forward result
		if currentState == int32(Fulfilled) {
			handlerFunc(p.Value(), false, result)
		} else {
			handlerFunc(p.Reason(), true, result)
		}
	}

	return result
}

// ToChannel returns a channel that will receive the result when the promise settles.
// The channel is buffered (capacity 1) and will be closed after sending.
// If the promise is already settled, returns a pre-filled channel.
// Thread-safe and can be called from any goroutine.
func (p *ChainedPromise) ToChannel() <-chan Result {
	ch := make(chan Result, 1)

	currentState := p.state.Load()
	if currentState != int32(Pending) {
		// Already settled, send result immediately
		ch <- p.result
		close(ch)
		return ch
	}

	// Pending: set up callback to send result when settled
	p.mu.Lock()
	// Double-check state after acquiring lock
	if p.state.Load() != int32(Pending) {
		p.mu.Unlock()
		ch <- p.result
		close(ch)
		return ch
	}

	// Store the channel
	p.channels = append(p.channels, ch)
	p.mu.Unlock()

	return ch
}

// tryCall calls a function with the given value, resolving or rejecting the target.
func tryCall(fn func(Result) Result, v Result, target *ChainedPromise) {
	defer func() {
		if r := recover(); r != nil {
			// Wrap panic value in PanicError to provide consistent error interface
			panicErr := PanicError{Value: r}
			target.reject(panicErr, target.js)
		}
	}()

	// CRITICAL: Check for nil handler before calling
	if fn == nil {
		// No handler means pass-through
		target.resolve(v, target.js)
		return
	}

	result := fn(v)
	target.resolve(result, target.js)
}

// trackRejection tracks a rejected promise for unhandled rejection detection.
// This is called from the reject() method.
//
// This implementation ensures that checkUnhandledRejections runs AFTER all concurrent
// handler registrations from then() by using proper channel synchronization.
// Each rejection waits for a handler to be registered (or determines none will be)
// before checking for unhandled rejections.
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

	// CRITICAL FIX: Use atomic counter to prevent duplicate microtasks
	// checkUnhandledRejections checks all unhandled rejections, so we only need
	// ONE scheduled check running at a time, not one per rejection
	if !js.checkRejectionScheduled.CompareAndSwap(false, true) {
		// Another check is already scheduled, this rejection will be caught by it
		return
	}

	// Create a channel for this rejection to signal handler registration
	// Multiple rejections to the same promise share this channel
	handlerReady := make(chan struct{})

	// Try to store the channel so then() can signal when handler is registered
	// Use atomic compare-and-swap to avoid races with concurrent rejections
	handlerKey := promiseID

	js.handlerReadyMu.Lock()
	// Check if another rejection already stored a channel
	if _, exists := js.handlerReadyChans[handlerKey]; !exists {
		// No channel yet, store ours
		js.handlerReadyChans[handlerKey] = handlerReady
	}
	js.handlerReadyMu.Unlock()

	// Schedule the check to wait for handler registration
	// This is queued via ScheduleMicrotask which is called BEFORE QueueMicrotask
	// in the overall sequence, ensuring proper ordering
	//
	// CRITICAL: We wait for handler registration (via channel) to prevent
	// false positives where checkUnhandledRejections runs before the handler
	// is registered in promiseHandlers. However, we also check promiseHandlers
	// after the wait to handle the case where no handler is ever attached.
	js.loop.ScheduleMicrotask(func() {
		// Check if we should wait for a handler
		js.handlerReadyMu.Lock()
		ch, exists := js.handlerReadyChans[handlerKey]
		if exists {
			// Remove from map so then() knows we're done waiting
			delete(js.handlerReadyChans, handlerKey)
		}
		js.handlerReadyMu.Unlock()

		if exists && ch == handlerReady {
			// Wait for handler registration signal with timeout
			// Use a timeout to avoid deadlocks when no handler is attached
			select {
			case <-handlerReady:
				// Handler was registered
			case <-time.After(10 * time.Millisecond):
				// Timeout - no handler registered yet
			}
		}

		// Check if handler exists in promiseHandlers
		// If it does, skip the check (it will be handled properly)
		// If it doesn't, run the check (to catch truly unhandled rejections)
		js.promiseHandlersMu.RLock()
		_, handlerExists := js.promiseHandlers[promiseID]
		js.promiseHandlersMu.RUnlock()

		if !handlerExists {
			// No handler registered, run check to catch unhandled rejection
			js.checkUnhandledRejections()
		}
		// If handler exists, skip the check - the handler will be processed
		// and a subsequent check will clean up

		// Mark as completed (allow next one to be scheduled)
		js.checkRejectionScheduled.Store(false)
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

		js.promiseHandlersMu.Lock()
		handled, exists := js.promiseHandlers[promiseID]

		// If a handler exists, clean up tracking now (handled rejection)
		if exists && handled {
			delete(js.promiseHandlers, promiseID)
			js.promiseHandlersMu.Unlock()

			// Remove from unhandled rejections but DON'T report it
			js.rejectionsMu.Lock()
			delete(js.unhandledRejections, promiseID)
			js.rejectionsMu.Unlock()
			continue
		}
		js.promiseHandlersMu.Unlock()

		// No handler found - report unhandled rejection
		if callback != nil {
			callback(info.reason)
		}

		// Clean up tracking for unhandled rejection
		js.rejectionsMu.Lock()
		delete(js.unhandledRejections, promiseID)
		js.rejectionsMu.Unlock()
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
	// Handle empty array - create resolved promise directly
	if len(promises) == 0 {
		// Create a ChainedPromise in resolved state
		p := &ChainedPromise{
			js: js,
		}
		p.state.Store(int32(Fulfilled))
		p.result = make([]Result, 0)
		return p
	}

	result, resolve, _ := js.NewChainedPromise()

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
