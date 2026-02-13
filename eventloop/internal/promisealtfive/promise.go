// Package promisealtfive is a tournament snapshot of the original ChainedPromise
// implementation (before PromiseAltOne optimization patterns were ported).
//
// This package preserves the original ChainedPromise's internal mechanism:
//   - h0 uses target==nil check (no h0Used flag)
//   - resolve/reject take explicit js parameter
//   - Handler scheduling is inline in resolve/reject
//   - No addHandler method; handler storage is inlined in then()
//
// Simplified for tournament use: no rejection tracking, no debug mode,
// no direct ToChannel channels.
package promisealtfive

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/joeycumines/go-eventloop"
)

// PromiseState is an alias for eventloop.PromiseState.
type PromiseState = eventloop.PromiseState

const (
	Pending   = eventloop.Pending
	Resolved  = eventloop.Resolved
	Fulfilled = eventloop.Fulfilled
	Rejected  = eventloop.Rejected
)

// Promise is a snapshot of the original ChainedPromise implementation.
// It uses the pre-optimization handler model with nil-target checks
// and explicit js parameter passing in resolve/reject.
type Promise struct {
	// Fields mirror original ChainedPromise layout.
	result any
	js     *eventloop.JS
	h0     handler

	state atomic.Int32

	mu sync.Mutex
}

// handler represents a reaction to promise settlement.
type handler struct {
	onFulfilled func(any) any
	onRejected  func(any) any
	target      *Promise
}

// ResolveFunc is the function used to fulfill a promise.
type ResolveFunc func(any)

// RejectFunc is the function used to reject a promise.
type RejectFunc func(any)

// New creates a new pending promise. This mirrors ChainedPromise's NewChainedPromise
// but simplified for tournament use.
func New(js *eventloop.JS) (*Promise, ResolveFunc, RejectFunc) {
	p := &Promise{
		js: js,
	}
	p.state.Store(int32(Pending))

	resolve := func(value any) {
		p.resolve(value, js)
	}

	reject := func(reason any) {
		p.reject(reason, js)
	}

	return p, resolve, reject
}

// State returns the current state of the promise.
func (p *Promise) State() PromiseState {
	return PromiseState(p.state.Load())
}

// Value returns the fulfillment value.
func (p *Promise) Value() any {
	if p.state.Load() == int32(Fulfilled) {
		return p.result
	}
	return nil
}

// Reason returns the rejection reason.
func (p *Promise) Reason() any {
	if p.state.Load() == int32(Rejected) {
		return p.result
	}
	return nil
}

// Then adds handlers. This mirrors the original ChainedPromise.then() mechanism
// with inline handler storage (no addHandler method).
func (p *Promise) Then(onFulfilled, onRejected func(any) any) *Promise {
	js := p.js

	result := &Promise{
		js: js,
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
		// Re-check state after acquiring lock (race handling)
		p.mu.Lock()
		currentState = p.state.Load()

		if currentState != int32(Pending) {
			p.mu.Unlock()
			// Fall through to settled handling below
			if currentState == int32(Rejected) {
				r := p.Reason()
				if js != nil {
					js.QueueMicrotask(func() {
						tryCall(onRejected, r, result, js)
					})
				} else {
					tryCall(onRejected, r, result, js)
				}
				return result
			}
			// Fulfilled
			v := p.Value()
			if js != nil {
				js.QueueMicrotask(func() {
					tryCall(onFulfilled, v, result, js)
				})
			} else {
				tryCall(onFulfilled, v, result, js)
			}
			return result
		}

		// Still pending: store handler (original nil-target check pattern)
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
		// Already settled
		if currentState == int32(Rejected) {
			r := p.Reason()
			if js != nil {
				js.QueueMicrotask(func() {
					tryCall(onRejected, r, result, js)
				})
			} else {
				tryCall(onRejected, r, result, js)
			}
			return result
		}
		// Fulfilled
		v := p.Value()
		if js != nil {
			js.QueueMicrotask(func() {
				tryCall(onFulfilled, v, result, js)
			})
		} else {
			tryCall(onFulfilled, v, result, js)
		}
	}

	return result
}

// Catch adds a rejection handler.
func (p *Promise) Catch(onRejected func(any) any) *Promise {
	return p.Then(nil, onRejected)
}

// Finally adds a finally handler using the original two-handler approach.
func (p *Promise) Finally(onFinally func()) *Promise {
	js := p.js
	result := &Promise{
		js: js,
	}
	result.state.Store(int32(Pending))

	if onFinally == nil {
		onFinally = func() {}
	}

	handlerFunc := func(value any, isRejection bool, target *Promise) {
		onFinally()
		if isRejection {
			target.reject(value, target.js)
		} else {
			target.resolve(value, target.js)
		}
	}

	currentState := p.state.Load()

	if currentState == int32(Pending) {
		p.mu.Lock()
		currentState = p.state.Load()

		if currentState != int32(Pending) {
			p.mu.Unlock()
			if currentState == int32(Fulfilled) {
				handlerFunc(p.Value(), false, result)
			} else {
				handlerFunc(p.Reason(), true, result)
			}
			return result
		}

		// Original two-handler approach for Finally
		h1 := handler{
			onFulfilled: func(v any) any {
				handlerFunc(v, false, result)
				return nil
			},
			target: result,
		}
		h2 := handler{
			onRejected: func(r any) any {
				handlerFunc(r, true, result)
				return nil
			},
			target: result,
		}

		// Store h1
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

		// Store h2
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
	} else {
		if currentState == int32(Fulfilled) {
			handlerFunc(p.Value(), false, result)
		} else {
			handlerFunc(p.Reason(), true, result)
		}
	}

	return result
}

// resolve transitions the promise to fulfilled state.
// Takes explicit js parameter (original ChainedPromise pattern).
func (p *Promise) resolve(value any, js *eventloop.JS) {
	// Promise adoption
	if pr, ok := value.(*Promise); ok && pr == p {
		p.reject(fmt.Errorf("TypeError: Chaining cycle detected"), js)
		return
	}

	if pr, ok := value.(*Promise); ok {
		// Wait for pr to settle via Then (original ThenWithJS pattern)
		pr.Then(
			func(v any) any {
				p.resolve(v, js)
				return nil
			},
			func(r any) any {
				p.reject(r, js)
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

	if p.result != nil {
		if hrs, ok := p.result.([]handler); ok {
			handlers = hrs
		}
	}

	p.h0 = handler{}
	p.result = value
	p.state.Store(int32(Fulfilled))
	p.mu.Unlock()

	// Inline handler scheduling (original pattern, no scheduleHandler method)
	process := func(h handler) {
		if h.onFulfilled != nil {
			fn := h.onFulfilled
			target := h.target
			if js != nil {
				js.QueueMicrotask(func() {
					tryCall(fn, value, target, js)
				})
			} else {
				tryCall(fn, value, target, js)
			}
		} else {
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

// reject transitions the promise to rejected state.
// Takes explicit js parameter (original ChainedPromise pattern).
func (p *Promise) reject(reason any, js *eventloop.JS) {
	p.mu.Lock()
	if p.state.Load() != int32(Pending) {
		p.mu.Unlock()
		return
	}

	h0 := p.h0
	var handlers []handler

	if p.result != nil {
		if hrs, ok := p.result.([]handler); ok {
			handlers = hrs
		}
	}

	p.result = reason
	p.state.Store(int32(Rejected))

	// Inline handler scheduling (original pattern)
	process := func(h handler) {
		if h.onRejected != nil {
			fn := h.onRejected
			target := h.target
			if js != nil {
				js.QueueMicrotask(func() {
					tryCall(fn, reason, target, js)
				})
			} else {
				tryCall(fn, reason, target, js)
			}
		} else {
			h.target.reject(reason, h.target.js)
		}
	}

	if h0.target != nil {
		process(h0)
	}
	for _, h := range handlers {
		process(h)
	}

	p.h0 = handler{}
	p.mu.Unlock()
}

// ToChannel returns a channel that receives the result when settled.
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

// tryCall calls a handler function with panic recovery.
func tryCall(fn func(any) any, v any, target *Promise, js *eventloop.JS) {
	defer func() {
		if r := recover(); r != nil {
			target.reject(r, js)
		}
	}()

	if fn == nil {
		target.resolve(v, target.js)
		return
	}

	result := fn(v)
	target.resolve(result, target.js)
}
