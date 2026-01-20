package eventloop

import (
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// Result represents the value of a resolved or rejected promise.
// It can be any type.
type Result any

// PromiseState represents the lifecycle state of a Promise.
type PromiseState int

const (
	// Pending indicates the promise operation is still initializing or ongoing.
	Pending PromiseState = iota
	// Resolved indicates the promise operation completed successfully.
	// Fulfilled is an alias for compatibility with JavaScript spec.
	Resolved
	Fulfilled = Resolved
	// Rejected indicates the promise operation failed with an error.
	Rejected
)

// Promise is a read-only view of a future result.
type Promise interface {
	// State returns the current state of the promise.
	State() PromiseState

	// Result returns the result of the promise if settled, or nil if pending.
	// Note: A resolved promise can also have a nil result.
	Result() Result

	// ToChannel returns a channel that will receive the result when settled.
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

// ChainedPromise implements the Promise/A+ specification with Then/Catch/Finally.
// This is the JS-compatible promise implementation with proper async semantics.
type ChainedPromise struct {
	// Atomic state (requires 8-byte alignment, grouped)
	state atomic.Int32
	_     [4]byte // Padding to 8-byte

	// Non-pointer, non-atomic fields (no pointer alignment needed)
	id uint64

	// Non-pointer synchronization primitives
	mu sync.RWMutex

	// Pointer fields (all require 8-byte alignment, grouped last)
	js       *JS
	value    Result
	reason   Result
	handlers []handler
}

// handler represents a reaction to promise settlement.
type handler struct {
	onFulfilled func(Result) Result
	onRejected  func(Result) Result
	resolve     func(Result)
	reject      func(Result)
}

// ResolveFunc resolves a promise with a value.
type ResolveFunc func(Result)

// RejectFunc rejects a promise with a reason.
type RejectFunc func(Result)

// NewChainedPromise creates a new pending promise along with resolve and reject functions.
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

// State returns the current state of the promise.
func (p *ChainedPromise) State() PromiseState {
	return PromiseState(p.state.Load())
}

// Value returns the fulfillment value if the promise is fulfilled.
// Returns nil if the promise is pending or rejected.
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
func (p *ChainedPromise) Reason() Result {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.state.Load() != int32(Rejected) {
		return nil
	}
	return p.reason
}

// resolve transitions the promise to fulfilled state if it's still pending.
func (p *ChainedPromise) resolve(value Result, js *JS) {
	if !p.state.CompareAndSwap(int32(Pending), int32(Fulfilled)) {
		// Already settled
		return
	}

	p.mu.Lock()
	p.value = value
	handlers := p.handlers
	p.mu.Unlock()

	// Schedule handlers as microtasks
	for _, h := range handlers {
		if h.onFulfilled != nil {
			fn := h.onFulfilled
			result := h
			js.QueueMicrotask(func() {
				tryCall(fn, value, result.resolve, result.reject)
			})
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
	p.mu.Unlock()

	// Track unhandled rejection
	js.trackRejection(p.id, reason)

	// Schedule handlers as microtasks
	for _, h := range handlers {
		if h.onRejected != nil {
			fn := h.onRejected
			result := h
			js.QueueMicrotask(func() {
				tryCall(fn, reason, result.resolve, result.reject)
			})
		}
	}
}

// Then adds handlers to be called when the promise resolves or rejects.
// Returns a new promise that resolves with the result of the handler.
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
		js.promiseHandlers.Store(p.id, true)
	}

	// Check current state
	currentState := p.state.Load()

	if currentState == int32(Pending) {
		// Pending: store handler
		p.mu.Lock()
		p.handlers = append(p.handlers, h)
		p.mu.Unlock()
	} else {
		// Already settled: schedule handler as microtask
		if currentState == int32(Fulfilled) && onFulfilled != nil {
			v := p.Value()
			js.QueueMicrotask(func() {
				tryCall(onFulfilled, v, resolve, reject)
			})
		} else if currentState == int32(Rejected) && onRejected != nil {
			r := p.Reason()
			js.QueueMicrotask(func() {
				tryCall(onRejected, r, resolve, reject)
			})
		}
	}

	return result
}

// thenStandalone creates a child promise without JS adapter for basic operations.
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
// Returns a new promise that resolves with the result of the handler.
func (p *ChainedPromise) Catch(onRejected func(Result) Result) *ChainedPromise {
	return p.Then(nil, onRejected)
}

// Finally adds a handler that runs regardless of settlement.
// Returns a new promise that preserves the original settlement.
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
	js.unhandledRejections.Store(promiseID, info)

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

	js.unhandledRejections.Range(func(key, value interface{}) bool {
		promiseID := key.(uint64)
		handledAny, exists := js.promiseHandlers.Load(promiseID)

		// If we can't find a handler, or it's explicitly marked as unhandled, report it
		if !exists || !handledAny.(bool) {
			if callback != nil {
				info := value.(*rejectionInfo)
				callback(info.reason)
			}
		}

		// Clean up tracking
		js.unhandledRejections.Delete(promiseID)
		js.promiseHandlers.Delete(promiseID)
		return true
	})
}

// rejectionInfo holds information about a rejected promise.
type rejectionInfo struct {
	reason    Result // Largest field, put first
	promiseID uint64
	timestamp int64
}
