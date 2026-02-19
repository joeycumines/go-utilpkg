package promisealtone

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/joeycumines/go-eventloop"
)

// PromiseState is an alias for eventloop.PromiseState
type PromiseState = eventloop.PromiseState

const (
	Pending   = eventloop.Pending
	Resolved  = eventloop.Resolved
	Fulfilled = eventloop.Fulfilled
	Rejected  = eventloop.Rejected
)

// Promise is an optimized implementation of a Promise/A+ compatible promise.
// It prioritizes performance with lock-free reads, minimized allocations, and
// structure embedding for the common single-handler case.
type Promise struct { // betteralign:ignore
	// The value or reason of the promise.
	// Written ONCE before state transition to Fulfilled/Rejected.
	// Read safely without lock after checking state.
	result any

	// Optimization: Embedded first handler to avoid slice allocation for simple chains.
	// This covers the 99% case of linear chaining (A.Then(B).Then(C)).
	h0 handler

	// Pointer fields (8-byte aligned)
	js *eventloop.JS

	// Sync primitives
	mu sync.Mutex

	// Atomic state
	state atomic.Int32

	// Flags
	h0Used bool
	_      [3]byte // Padding to 64 bytes
}

// handler represents a reaction to promise settlement.
// Optimized to point directly to the target promise, avoiding closure allocations.
type handler struct {
	onFulfilled func(any) any
	onRejected  func(any) any
	target      *Promise
}

// ResolveFunc is the function used to fulfill a promise.
type ResolveFunc func(any)

// RejectFunc is the function used to reject a promise.
type RejectFunc func(any)

// New creates a new pending promise.
func New(js *eventloop.JS) (*Promise, ResolveFunc, RejectFunc) {
	p := newPromise(js)

	// Capture closures only for the root promise.
	// Chained promises will not use these pointers.
	resolve := func(value any) {
		p.resolve(value)
	}

	reject := func(reason any) {
		p.reject(reason)
	}

	return p, resolve, reject
}

// newPromise creates a lightweight promise without allocating closure wrappers.
func newPromise(js *eventloop.JS) *Promise {
	p := &Promise{
		js: js,
	}
	p.state.Store(int32(Pending))
	return p
}

// State returns the current state of the promise.
// Lock-free.
func (p *Promise) State() PromiseState {
	return PromiseState(p.state.Load())
}

// Value returns the fulfillment value.
// Lock-free.
func (p *Promise) Value() any {
	if p.state.Load() != int32(Fulfilled) {
		return nil
	}
	return p.result
}

// Reason returns the rejection reason.
// Lock-free.
func (p *Promise) Reason() any {
	if p.state.Load() != int32(Rejected) {
		return nil
	}
	return p.result
}

// Result returns the result (value or reason) if settled, or nil if pending.
// Lock-free.
func (p *Promise) Result() any {
	if p.state.Load() == int32(Pending) {
		return nil
	}
	return p.result
}

// Then adds handlers.
func (p *Promise) Then(onFulfilled, onRejected func(any) any) *Promise {
	// Create the next promise in the chain without allocating resolve/reject closures
	child := newPromise(p.js)

	h := handler{
		onFulfilled: onFulfilled,
		onRejected:  onRejected,
		target:      child,
	}

	p.addHandler(h)

	return child
}

// addHandler attaches a handler efficiently, handling strict ordering and concurrency.
func (p *Promise) addHandler(h handler) {
	// Optimistic check: if already settled, schedule immediately without lock.
	currentState := p.state.Load()
	if currentState != int32(Pending) {
		p.scheduleHandler(h, currentState, p.result)
		return
	}

	p.mu.Lock()
	// Re-check state under lock to avoid race
	currentState = p.state.Load()
	if currentState != int32(Pending) {
		p.mu.Unlock()
		p.scheduleHandler(h, currentState, p.result)
		return
	}

	if !p.h0Used {
		p.h0 = h
		p.h0Used = true
	} else {
		// Optimization: Store additional handlers in p.result type-punned as []handler
		// This saves 24 bytes in the Promise struct, fitting it into 64 bytes (1 cache line).
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
}

// Catch adds a rejection handler.
func (p *Promise) Catch(onRejected func(any) any) *Promise {
	return p.Then(nil, onRejected)
}

// Finally adds a finally handler.
func (p *Promise) Finally(onFinally func()) *Promise {
	if onFinally == nil {
		onFinally = func() {}
	}

	// We must wrap onFinally to forward results correctly.
	// This unfortunately requires closures, but Finally is less common in hot loops than Then.
	// Optimizing this further would require new fields in `handler` which bloats the struct for everyone.

	next := newPromise(p.js)

	// Logic: run onFinally. If successful, propagate original result.
	// If onFinally panics, reject with panic.

	// We reuse the standard handler mechanism but manually construct the callbacks

	// Helper to reduce code duplication
	runFinally := func(res any, isRej bool) {
		// Run finally block
		defer func() {
			if r := recover(); r != nil {
				next.reject(r)
			}
		}()
		onFinally()

		// If we are here, finally succeeded. Propagate original result.
		if isRej {
			next.reject(res)
		} else {
			next.resolve(res)
		}
	}

	h := handler{
		onFulfilled: func(v any) any {
			runFinally(v, false)
			return nil // Return ignored as we manually resolve `next`
		},
		onRejected: func(r any) any {
			runFinally(r, true)
			return nil
		},
		target: next,
	}

	p.addHandler(h)
	return next
}

func (p *Promise) resolve(value any) {
	// Check for chaining
	if pr, ok := value.(*Promise); ok {
		if pr == p {
			p.reject(fmt.Errorf("TypeError: Chaining cycle detected"))
			return
		}
		// Optimization: Link directly without intermediate closures
		pr.addHandler(handler{target: p})
		return
	}

	// Fast path check
	if p.state.Load() != int32(Pending) {
		return
	}

	p.mu.Lock()
	if p.state.Load() != int32(Pending) {
		p.mu.Unlock()
		return
	}

	// Capture and clear handlers under lock
	h0 := p.h0
	useH0 := p.h0Used

	// Extract extra handlers from result if present
	var handlers []handler
	if useH0 && p.result != nil {
		handlers = p.result.([]handler)
	}

	p.result = value
	p.state.Store(int32(Fulfilled))

	// Release memory
	p.h0 = handler{}
	p.mu.Unlock()

	// Execute handlers outside lock
	if useH0 {
		p.scheduleHandler(h0, int32(Fulfilled), value)
	}
	for _, h := range handlers {
		p.scheduleHandler(h, int32(Fulfilled), value)
	}
}

func (p *Promise) reject(reason any) {
	if p.state.Load() != int32(Pending) {
		return
	}

	p.mu.Lock()
	if p.state.Load() != int32(Pending) {
		p.mu.Unlock()
		return
	}

	// Capture handlers
	h0 := p.h0
	useH0 := p.h0Used

	// Extract extra handlers from result if present
	var handlers []handler
	if useH0 && p.result != nil {
		handlers = p.result.([]handler)
	}

	p.result = reason
	p.state.Store(int32(Rejected))

	p.h0 = handler{}
	p.mu.Unlock()

	if useH0 {
		p.scheduleHandler(h0, int32(Rejected), reason)
	}
	for _, h := range handlers {
		p.scheduleHandler(h, int32(Rejected), reason)
	}
}

func (p *Promise) scheduleHandler(h handler, state int32, result any) {
	if p.js == nil {
		// Fallback for no loop (testing?) or sync execution
		p.executeHandler(h, state, result)
		return
	}

	p.js.QueueMicrotask(func() {
		p.executeHandler(h, state, result)
	})
}

// Observe adds handlers without creating a child promise.
// This is useful for fire-and-forget scenarios or combinators like All.
func (p *Promise) Observe(onFulfilled, onRejected func(any) any) {
	h := handler{
		onFulfilled: onFulfilled,
		onRejected:  onRejected,
		target:      nil, // No target promise
	}
	p.addHandler(h)
}

func (p *Promise) executeHandler(h handler, state int32, result any) {
	var fn func(any) any

	// Determine which user callback to run
	if state == int32(Fulfilled) {
		fn = h.onFulfilled
	} else {
		fn = h.onRejected
	}

	// 1. If no handler, propagate state to target
	if fn == nil {
		if h.target == nil {
			return
		}
		if state == int32(Fulfilled) {
			h.target.resolve(result)
		} else {
			h.target.reject(result)
		}
		return
	}

	// 2. Run handler with panic protection
	defer func() {
		if r := recover(); r != nil {
			if h.target != nil {
				h.target.reject(r)
			}
		}
	}()

	res := fn(result)

	// 3. Resolve target with result
	if h.target != nil {
		h.target.resolve(res)
	}
}

// ToChannel returns a channel that receives the result.
func (p *Promise) ToChannel() <-chan any {
	ch := make(chan any, 1)
	p.Then(func(v any) any {
		ch <- v
		close(ch)
		return nil
	}, func(r any) any {
		ch <- r
		close(ch)
		return nil
	})
	return ch
}

// All waits for all promises.
func All(js *eventloop.JS, promises []*Promise) *Promise {
	result := newPromise(js)

	if len(promises) == 0 {
		result.resolve([]any{})
		return result
	}

	results := make([]any, len(promises))
	pending := int32(len(promises))

	// Optimization: onRejected is nil, so rejections are forwarded directly to result.
	// onFulfilled must be defined to aggregate results.

	for i, p := range promises {
		h := handler{
			onFulfilled: func(v any) any {
				results[i] = v
				if atomic.AddInt32(&pending, -1) == 0 {
					result.resolve(results)
				}
				return nil
			},
			onRejected: nil, // Forward rejection immediately
			target:     result,
		}
		p.addHandler(h)
	}

	return result
}

// Helper to check interface compliance
var _ interface {
	State() PromiseState
	Value() any
	Reason() any
	Then(func(any) any, func(any) any) *Promise
	Catch(func(any) any) *Promise
	Finally(func()) *Promise
} = (*Promise)(nil)

// Race returns a promise that settles as soon as any of the input promises settles.
func Race(js *eventloop.JS, promises []*Promise) *Promise {
	result := newPromise(js)

	if len(promises) == 0 {
		return result
	}

	// Optimization: Forward both fulfillment and rejection directly to result.
	// No closures allocated!
	h := handler{target: result}

	for _, p := range promises {
		p.addHandler(h)
	}

	return result
}

// AllSettled returns a promise that resolves when all input promises have settled.
func AllSettled(js *eventloop.JS, promises []*Promise) *Promise {
	result := newPromise(js)

	if len(promises) == 0 {
		result.resolve([]any{})
		return result
	}

	results := make([]any, len(promises))
	pending := int32(len(promises))

	// AllSettled needs to handle both paths to aggregate.
	// We can't forward directly.

	for i, p := range promises {
		checkDone := func() {
			if atomic.AddInt32(&pending, -1) == 0 {
				result.resolve(results)
			}
		}

		h := handler{
			onFulfilled: func(v any) any {
				results[i] = map[string]any{"status": "fulfilled", "value": v}
				checkDone()
				return nil
			},
			onRejected: func(r any) any {
				results[i] = map[string]any{"status": "rejected", "reason": r}
				checkDone()
				return nil
			},
			target: nil, // No direct forwarding
		}
		p.addHandler(h)
	}

	return result
}

// Any returns a promise that fulfills as soon as any of the input promises fulfills.
// If all input promises reject, it rejects with an AggregateError.
func Any(js *eventloop.JS, promises []*Promise) *Promise {
	result := newPromise(js)

	if len(promises) == 0 {
		result.reject(fmt.Errorf("AggregateError: All promises were rejected"))
		return result
	}

	errors := make([]any, len(promises))
	pending := int32(len(promises))

	// Optimization: onFulfilled is nil, so fulfillment is forwarded directly.
	// onRejected is handled to aggregate errors.

	for i, p := range promises {
		h := handler{
			onFulfilled: nil, // Forward fulfillment immediately
			onRejected: func(r any) any {
				errors[i] = r
				if atomic.AddInt32(&pending, -1) == 0 {
					result.reject(errors)
				}
				return nil
			},
			target: result,
		}
		p.addHandler(h)
	}

	return result
}

// String returns a string representation of the promise independent of its result value.
func (p *Promise) String() string {
	state := p.State()
	switch state {
	case Pending:
		return "Promise<Pending>"
	case Fulfilled:
		return fmt.Sprintf("Promise<Fulfilled: %v>", p.result) // Reading result is safe if Fulfilled
	case Rejected:
		return fmt.Sprintf("Promise<Rejected: %v>", p.result) // Reading result is safe if Rejected
	default:
		return "Promise<Unknown>"
	}
}

// Await blocks until the promise settles and returns the result or error.
// It uses a context for cancellation (abandonment).
//
// Note: This method blocks the calling goroutine. DO NOT call this from the event loop thread
// unless you are sure what you are doing, as it will deadlock if the promise relies on the loop.
func (p *Promise) Await(ctx interface {
	Done() <-chan struct{}
	Err() error
}) (any, error) {
	// Fast path
	state := p.State()
	if state == Fulfilled {
		return p.result, nil
	}
	if state == Rejected {
		// Return reason as error? OR reason as value?
		// Standard simple Await implies retrieving value or error.
		// If reason is error, we return it. If not, we wrap it?
		// Result is alias for any.
		// We return (Result, error).
		// If Fulfilled: (val, nil).
		// If Rejected: (nil, reason if error else fmt.Errorf("%v", reason))
		if err, ok := p.result.(error); ok {
			return nil, err
		}
		return nil, fmt.Errorf("%v", p.result)
	}

	ch := make(chan any, 1)
	errCh := make(chan any, 1)

	p.Observe(func(v any) any {
		ch <- v
		return nil
	}, func(r any) any {
		errCh <- r
		return nil
	})

	select {
	case v := <-ch:
		return v, nil
	case r := <-errCh:
		if err, ok := r.(error); ok {
			return nil, err
		}
		return nil, fmt.Errorf("%v", r)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
