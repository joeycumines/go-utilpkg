package promisealtone

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/joeycumines/go-eventloop"
)

// Result is an alias for eventloop.Result
type Result = eventloop.Result

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
	result Result

	// Optimization: Embedded first handler to avoid slice allocation for simple chains.
	// This covers the 99% case of linear chaining (A.Then(B).Then(C)).
	h0 handler

	// Additional handlers for branching logic (A.Then(B); A.Then(C)).
	// Only allocated if more than one handler is attached.
	handlers []handler

	// Pointer fields (8-byte aligned)
	js *eventloop.JS

	// Sync primitives
	mu sync.Mutex

	// Atomic state
	state atomic.Int32

	// Flags
	h0Used bool
	_      [3]byte // Padding
}

// handler represents a reaction to promise settlement.
// Optimized to point directly to the target promise, avoiding closure allocations.
type handler struct {
	onFulfilled func(Result) Result
	onRejected  func(Result) Result
	target      *Promise
}

// ResolveFunc is the function used to fulfill a promise.
type ResolveFunc func(Result)

// RejectFunc is the function used to reject a promise.
type RejectFunc func(Result)

// unique error to detect cycles efficiently
// var errCycle = fmt.Errorf("TypeError: Chaining cycle detected")

// New creates a new pending promise.
func New(js *eventloop.JS) (*Promise, ResolveFunc, RejectFunc) {
	p := newPromise(js)

	// Capture closures only for the root promise.
	// Chained promises will not use these pointers.
	resolve := func(value Result) {
		p.resolve(value)
	}

	reject := func(reason Result) {
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
func (p *Promise) Value() Result {
	if p.state.Load() != int32(Fulfilled) {
		return nil
	}
	return p.result
}

// Reason returns the rejection reason.
// Lock-free.
func (p *Promise) Reason() Result {
	if p.state.Load() != int32(Rejected) {
		return nil
	}
	return p.result
}

// Result returns the result (value or reason) if settled, or nil if pending.
// Lock-free.
func (p *Promise) Result() Result {
	if p.state.Load() == int32(Pending) {
		return nil
	}
	return p.result
}

// Then adds handlers.
func (p *Promise) Then(onFulfilled, onRejected func(Result) Result) *Promise {
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
		// Lazy init slice
		if p.handlers == nil {
			p.handlers = make([]handler, 0, 2)
		}
		p.handlers = append(p.handlers, h)
	}
	p.mu.Unlock()
}

// Catch adds a rejection handler.
func (p *Promise) Catch(onRejected func(Result) Result) *Promise {
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
	runFinally := func(res Result, isRej bool) {
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
		onFulfilled: func(v Result) Result {
			runFinally(v, false)
			return nil // Return ignored as we manually resolve `next`
		},
		onRejected: func(r Result) Result {
			runFinally(r, true)
			return nil
		},
		target: next,
	}

	p.addHandler(h)
	return next
}

func (p *Promise) resolve(value Result) {
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

	p.result = value
	p.state.Store(int32(Fulfilled))

	// Capture and clear handlers under lock
	h0 := p.h0
	useH0 := p.h0Used
	handlers := p.handlers

	// Release memory
	p.h0 = handler{}
	p.handlers = nil
	p.mu.Unlock()

	// Execute handlers outside lock
	if useH0 {
		p.scheduleHandler(h0, int32(Fulfilled), value)
	}
	for _, h := range handlers {
		p.scheduleHandler(h, int32(Fulfilled), value)
	}
}

func (p *Promise) reject(reason Result) {
	if p.state.Load() != int32(Pending) {
		return
	}

	p.mu.Lock()
	if p.state.Load() != int32(Pending) {
		p.mu.Unlock()
		return
	}

	p.result = reason
	p.state.Store(int32(Rejected))

	h0 := p.h0
	useH0 := p.h0Used
	handlers := p.handlers

	p.h0 = handler{}
	p.handlers = nil
	p.mu.Unlock()

	if useH0 {
		p.scheduleHandler(h0, int32(Rejected), reason)
	}
	for _, h := range handlers {
		p.scheduleHandler(h, int32(Rejected), reason)
	}
}

func (p *Promise) scheduleHandler(h handler, state int32, result Result) {
	if p.js == nil {
		// Fallback for no loop (testing?) or sync execution
		p.executeHandler(h, state, result)
		return
	}

	p.js.QueueMicrotask(func() {
		p.executeHandler(h, state, result)
	})
}

func (p *Promise) executeHandler(h handler, state int32, result Result) {
	var fn func(Result) Result

	// Determine which user callback to run
	if state == int32(Fulfilled) {
		fn = h.onFulfilled
	} else {
		fn = h.onRejected
	}

	// 1. If no handler, propagate state to target
	if fn == nil {
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
			h.target.reject(r)
		}
	}()

	res := fn(result)

	// 3. Resolve target with result
	// Note: If the handler returns a Promise, resolve() handles the unwrapping
	h.target.resolve(res)
}

// ToChannel returns a channel that receives the result.
func (p *Promise) ToChannel() <-chan Result {
	ch := make(chan Result, 1)
	p.Then(func(v Result) Result {
		ch <- v
		close(ch)
		return nil
	}, func(r Result) Result {
		ch <- r
		close(ch)
		return nil
	})
	return ch
}

// All waits for all promises.
func All(js *eventloop.JS, promises []*Promise) *Promise {
	result, resolve, reject := New(js)

	if len(promises) == 0 {
		resolve([]Result{})
		return result
	}

	results := make([]Result, len(promises))
	pending := int32(len(promises))

	// Optimization: Pre-allocate handlers?
	// The current logic is fine; New() creates accessors which have overhead,
	// but All() is inherently heavy due to slice management.

	for i, p := range promises {
		i := i // Capture
		p.Then(func(v Result) Result {
			results[i] = v
			if atomic.AddInt32(&pending, -1) == 0 {
				resolve(results)
			}
			return nil
		}, func(r Result) Result {
			// First rejection wins
			reject(r)
			return nil
		})
	}

	return result
}

// Helper to check interface compliance
var _ interface {
	State() PromiseState
	Value() Result
	Reason() Result
	Then(func(Result) Result, func(Result) Result) *Promise
	Catch(func(Result) Result) *Promise
	Finally(func()) *Promise
} = (*Promise)(nil)
