package promisealtfour

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

// Promise implements the Promise/A+ specification with Then, Catch, and Finally.
// This is the promisealtfour variant of the Main ChainedPromise implementation.
type Promise struct {
	value  any
	reason any

	// Pointer fields (all require 8-byte alignment, grouped last)
	js       *eventloop.JS
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
	onFulfilled func(any) any
	onRejected  func(any) any
	resolve     func(any)
	reject      func(any)
}

// ResolveFunc is the function used to fulfill a promise with a value.
type ResolveFunc func(any)

// RejectFunc is the function used to reject a promise with a reason.
type RejectFunc func(any)

// New creates a new pending promise along with resolve and reject functions.
func New(js *eventloop.JS) (*Promise, ResolveFunc, RejectFunc) {
	p := &Promise{
		// Start in Pending state (0)
		handlers: make([]handler, 0, 2),
		// id: js.nextTimerID // Cannot access private field. Use 0 or local counter if needed.
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

// State returns the current PromiseState of this promise.
func (p *Promise) State() PromiseState {
	return PromiseState(p.state.Load())
}

// Value returns the fulfillment value if the promise is fulfilled.
func (p *Promise) Value() any {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.state.Load() != int32(Fulfilled) {
		return nil
	}
	return p.value
}

// Reason returns the rejection reason if the promise is rejected.
func (p *Promise) Reason() any {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.state.Load() != int32(Rejected) {
		return nil
	}
	return p.reason
}

func (p *Promise) resolve(value any, js *eventloop.JS) {
	// Spec 2.3.1: If promise and x refer to the same object, reject promise with a TypeError.
	if pr, ok := value.(*Promise); ok && pr == p {
		p.reject(fmt.Errorf("TypeError: Chaining cycle detected for promise #%d", p.id), js)
		return
	}

	// Spec 2.3.2: If x is a promise, adopt its state.
	if pr, ok := value.(*Promise); ok {
		pr.ThenWithJS(js,
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

	if !p.state.CompareAndSwap(int32(Pending), int32(Fulfilled)) {
		return
	}

	p.mu.Lock()
	p.value = value
	handlers := p.handlers
	p.handlers = nil
	p.mu.Unlock()

	// CLEANUP: js.promiseHandlers not accessible. Skipping cleanup.

	// Schedule handlers as microtasks
	for _, h := range handlers {
		if h.onFulfilled != nil {
			fn := h.onFulfilled
			result := h
			js.QueueMicrotask(func() {
				tryCall(fn, value, result.resolve, result.reject)
			})
		} else {
			h.resolve(value)
		}
	}
}

func (p *Promise) reject(reason any, js *eventloop.JS) {
	if !p.state.CompareAndSwap(int32(Pending), int32(Rejected)) {
		return
	}

	p.mu.Lock()
	p.reason = reason
	handlers := p.handlers
	p.handlers = nil
	p.mu.Unlock()

	for _, h := range handlers {
		if h.onRejected != nil {
			fn := h.onRejected
			result := h
			js.QueueMicrotask(func() {
				tryCall(fn, reason, result.resolve, result.reject)
			})
		} else {
			h.reject(reason)
		}
	}

	// UNHANDLED REJECTION TRACKING: Removed as we cannot access js.trackRejection
}

// Then adds handlers to be called when the promise settles.
func (p *Promise) Then(onFulfilled, onRejected func(any) any) *Promise {
	js := p.js
	if js == nil {
		return p.thenStandalone(onFulfilled, onRejected)
	}
	return p.then(js, onFulfilled, onRejected)
}

// ThenWithJS adds handlers with explicit JS adapter.
func (p *Promise) ThenWithJS(js *eventloop.JS, onFulfilled, onRejected func(any) any) *Promise {
	return p.then(js, onFulfilled, onRejected)
}

func (p *Promise) then(js *eventloop.JS, onFulfilled, onRejected func(any) any) *Promise {
	result := &Promise{
		handlers: make([]handler, 0, 2),
		js:       js,
	}
	result.state.Store(int32(Pending))

	resolve := func(value any) {
		result.resolve(value, js)
	}

	reject := func(reason any) {
		result.reject(reason, js)
	}

	h := handler{
		onFulfilled: onFulfilled,
		onRejected:  onRejected,
		resolve:     resolve,
		reject:      reject,
	}

	// TRACKING: js.promiseHandlers not accessible. Skipping.

	currentState := p.state.Load()

	if currentState == int32(Pending) {
		p.mu.Lock()
		p.handlers = append(p.handlers, h)
		p.mu.Unlock()
	} else {
		// ALREADY SETTLED
		// TRACKING Cleanup: not accessible.

		if onRejected != nil && currentState == int32(Rejected) {
			// Schedule handler as microtask
			r := p.Reason()
			js.QueueMicrotask(func() {
				tryCall(onRejected, r, resolve, reject)
			})
			return result
		}

		v := p.Value()
		js.QueueMicrotask(func() {
			tryCall(onFulfilled, v, resolve, reject)
		})
	}

	return result
}

func (p *Promise) thenStandalone(onFulfilled, onRejected func(any) any) *Promise {
	result := &Promise{
		handlers: make([]handler, 0, 2),
		js:       nil,
	}
	result.state.Store(int32(Pending))

	resolve := func(value any) {
		if result.state.CompareAndSwap(int32(Pending), int32(Fulfilled)) {
			result.mu.Lock()
			result.value = value
			result.mu.Unlock()
		}
	}

	reject := func(reason any) {
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

	currentState := p.state.Load()

	if currentState == int32(Pending) {
		p.mu.Lock()
		p.handlers = append(p.handlers, h)
		p.mu.Unlock()
	} else {
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

func (p *Promise) Catch(onRejected func(any) any) *Promise {
	return p.Then(nil, onRejected)
}

func (p *Promise) Finally(onFinally func()) *Promise {
	js := p.js
	var result *Promise
	var resolve ResolveFunc
	var reject RejectFunc

	if js != nil {
		result, resolve, reject = New(js)
	} else {
		result = &Promise{
			handlers: make([]handler, 0, 2),
			js:       nil,
		}
		result.state.Store(int32(Pending))
		resolve = func(value any) {
			if result.state.CompareAndSwap(int32(Pending), int32(Fulfilled)) {
				result.mu.Lock()
				result.value = value
				result.mu.Unlock()
			}
		}
		reject = func(reason any) {
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

	// TRACKING: not accessible.

	handlerFunc := func(value any, isRejection bool, res ResolveFunc, rej RejectFunc) {
		onFinally()
		if isRejection {
			rej(value)
		} else {
			res(value)
		}
	}

	currentState := p.state.Load()

	if currentState == int32(Pending) {
		p.mu.Lock()
		p.handlers = append(p.handlers, handler{
			onFulfilled: func(v any) any {
				handlerFunc(v, false, resolve, reject)
				return nil
			},
			resolve: resolve,
			reject:  reject,
		})
		p.handlers = append(p.handlers, handler{
			onRejected: func(r any) any {
				handlerFunc(r, true, resolve, reject)
				return nil
			},
			resolve: resolve,
			reject:  reject,
		})
		p.mu.Unlock()
	} else {
		if currentState == int32(Fulfilled) {
			handlerFunc(p.Value(), false, resolve, reject)
		} else {
			handlerFunc(p.Reason(), true, resolve, reject)
		}
	}

	return result
}

func tryCall(fn func(any) any, v any, resolve ResolveFunc, reject RejectFunc) {
	defer func() {
		if r := recover(); r != nil {
			reject(r)
		}
	}()

	if fn == nil {
		resolve(v)
		return
	}

	result := fn(v)
	resolve(result)
}

// Result returns the result (value or reason) if settled, or nil if pending.
func (p *Promise) Result() any {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.state.Load() == int32(Pending) {
		return nil
	}
	if p.state.Load() == int32(Fulfilled) {
		return p.value
	}
	return p.reason
}

// All implementation
func All(js *eventloop.JS, promises []*Promise) *Promise {
	result, resolve, reject := New(js)

	if len(promises) == 0 {
		resolve(make([]any, 0))
		return result
	}

	var mu sync.Mutex
	var completed atomic.Int32
	values := make([]any, len(promises))
	hasRejected := atomic.Bool{}

	for i, p := range promises {
		idx := i
		p.ThenWithJS(js,
			func(v any) any {
				mu.Lock()
				values[idx] = v
				mu.Unlock()

				count := completed.Add(1)
				if count == int32(len(promises)) && !hasRejected.Load() {
					resolve(values)
				}
				return nil
			},
			nil,
		)

		p.ThenWithJS(js,
			nil,
			func(r any) any {
				if hasRejected.CompareAndSwap(false, true) {
					reject(r)
				}
				return nil
			},
		)
	}

	return result
}

// Race implementation
func Race(js *eventloop.JS, promises []*Promise) *Promise {
	result, resolve, reject := New(js)

	if len(promises) == 0 {
		return result
	}

	var settled atomic.Bool

	for _, p := range promises {
		p.ThenWithJS(js,
			func(v any) any {
				if settled.CompareAndSwap(false, true) {
					resolve(v)
				}
				return nil
			},
			func(r any) any {
				if settled.CompareAndSwap(false, true) {
					reject(r)
				}
				return nil
			},
		)
	}

	return result
}

// AllSettled implementation
func AllSettled(js *eventloop.JS, promises []*Promise) *Promise {
	if len(promises) == 0 {
		p := &Promise{
			handlers: make([]handler, 0),
			js:       js,
		}
		p.state.Store(int32(Fulfilled))
		p.value = make([]any, 0)
		return p
	}

	result, resolve, _ := New(js)

	var mu sync.Mutex
	var completed atomic.Int32
	results := make([]any, len(promises))

	for i, p := range promises {
		idx := i
		p.ThenWithJS(js,
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
		)
	}

	return result
}

// Any implementation
func Any(js *eventloop.JS, promises []*Promise) *Promise {
	result, resolve, reject := New(js)

	if len(promises) == 0 {
		reject(fmt.Errorf("AggregateError: No promises were provided"))
		return result
	}

	var mu sync.Mutex
	var rejected atomic.Int32
	rejections := make([]any, len(promises))
	var resolved atomic.Bool

	for i, p := range promises {
		idx := i
		p.ThenWithJS(js,
			func(v any) any {
				if resolved.CompareAndSwap(false, true) {
					resolve(v)
				}
				return nil
			},
			func(r any) any {
				mu.Lock()
				rejections[idx] = r
				mu.Unlock()

				count := rejected.Add(1)
				if count == int32(len(promises)) && !resolved.Load() {
					// Simply return slice of errors/results for now to avoid copying AggregateError structs
					reject(fmt.Errorf("AggregateError: All promises were rejected"))
				}
				return nil
			},
		)
	}

	return result
}
