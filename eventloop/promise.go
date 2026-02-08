package eventloop

import (
	"fmt"
	"log"
	"runtime"
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
	// creationStack stores the stack trace when the promise was created.
	// EXPAND-039: Only populated when debugMode is enabled on the loop.
	// Use [ChainedPromise.CreationStackTrace] to format as a string.
	creationStack []uintptr

	// Atomic state (requires 8-byte alignment)
	state atomic.Int32
	// h0Used tracks whether h0 has been assigned (replaces nil-target check).
	h0Used bool
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

	// EXPAND-039: Capture creation stack trace when debug mode is enabled
	if js.loop != nil && js.loop.debugMode {
		// Capture up to 32 stack frames, skip 2 (this function and runtime.Callers)
		pcs := make([]uintptr, 32)
		n := runtime.Callers(2, pcs)
		if n > 0 {
			p.creationStack = pcs[:n]
		}
	}

	resolve := func(value Result) {
		p.resolve(value)
	}

	reject := func(reason Result) {
		p.reject(reason)
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

// CreationStackTrace returns a formatted stack trace of where this promise was created.
//
// EXPAND-039: This method returns an empty string unless debug mode was enabled on the
// event loop when the promise was created. Use [WithDebugMode] to enable stack trace capture.
//
// The returned string contains one line per stack frame, formatted as:
//
//	package.function (file:line)
//
// Example:
//
//	loop, _ := eventloop.New(eventloop.WithDebugMode(true))
//	js, _ := eventloop.NewJS(loop)
//	promise, _, _ := js.NewChainedPromise()
//
//	fmt.Println(promise.CreationStackTrace())
//	// Output:
//	// main.createPromise (main.go:42)
//	// main.main (main.go:15)
//	// runtime.main (proc.go:271)
//
// This is useful for debugging "where did this promise come from?" issues,
// especially for unhandled rejections.
func (p *ChainedPromise) CreationStackTrace() string {
	if len(p.creationStack) == 0 {
		return ""
	}

	frames := runtime.CallersFrames(p.creationStack)
	var result string
	for {
		frame, more := frames.Next()
		if frame.Function != "" {
			if result != "" {
				result += "\n"
			}
			result += fmt.Sprintf("%s (%s:%d)", frame.Function, frame.File, frame.Line)
		}
		if !more {
			break
		}
	}
	return result
}

// addHandler attaches a handler to the promise. If the promise is already settled,
// the handler is scheduled immediately via microtask. If pending, the handler is
// stored for later execution when the promise settles.
//
// This method uses an optimistic lock-free check for the common case where
// the promise is already settled, avoiding lock acquisition entirely.
func (p *ChainedPromise) addHandler(h handler) {
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
		// Store additional handlers in p.result type-punned as []handler.
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

// scheduleHandler enqueues a handler for execution via microtask.
// If no JS adapter is available, executes synchronously.
func (p *ChainedPromise) scheduleHandler(h handler, state int32, result Result) {
	if p.js == nil {
		p.executeHandler(h, state, result)
		return
	}

	p.js.QueueMicrotask(func() {
		p.executeHandler(h, state, result)
	})
}

// executeHandler runs a single handler with the given state and result.
// Handles nil handlers (pass-through), panic recovery, and result propagation.
func (p *ChainedPromise) executeHandler(h handler, state int32, result Result) {
	var fn func(Result) Result

	if state == int32(Fulfilled) {
		fn = h.onFulfilled
	} else {
		fn = h.onRejected
	}

	// If no handler, propagate state to target (pass-through)
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

	// Run handler with panic protection
	defer func() {
		if r := recover(); r != nil {
			if h.target != nil {
				h.target.reject(PanicError{Value: r})
			}
		}
	}()

	res := fn(result)
	if h.target != nil {
		h.target.resolve(res)
	}
}

func (p *ChainedPromise) resolve(value Result) {
	// Spec 2.3.1: If promise and x refer to the same object, reject promise with a TypeError.
	if pr, ok := value.(*ChainedPromise); ok && pr == p {
		p.reject(fmt.Errorf("TypeError: Chaining cycle detected for promise #%d", p.id))
		return
	}

	// Spec 2.3.2: If x is a promise, adopt its state.
	// Use addHandler for zero-closure adoption (PromiseAltOne optimization).
	if pr, ok := value.(*ChainedPromise); ok {
		pr.addHandler(handler{target: p})
		return
	}

	p.mu.Lock()
	if p.state.Load() != int32(Pending) {
		p.mu.Unlock()
		return
	}

	h0 := p.h0
	useH0 := p.h0Used
	var handlers []handler

	// Extract handlers before they get overwritten with the actual result
	if useH0 && p.result != nil {
		handlers = p.result.([]handler)
	}

	// Extract channels for notification
	channels := p.channels
	p.channels = nil

	p.h0 = handler{}
	p.h0Used = false
	p.result = value
	p.state.Store(int32(Fulfilled))

	// Schedule handlers inside lock to guarantee ordering consistency
	// with concurrent addHandler calls (Promise/A+ §2.2.6).
	// This matches reject()'s T27 pattern.
	if useH0 {
		p.scheduleHandler(h0, int32(Fulfilled), value)
	}
	for _, h := range handlers {
		p.scheduleHandler(h, int32(Fulfilled), value)
	}

	// Notify all channels registered via ToChannel() while still holding
	// lock, matching reject()'s pattern for consistent channel behavior.
	for _, ch := range channels {
		select {
		case ch <- value:
		default:
		}
	}
	for _, ch := range channels {
		close(ch)
	}
	p.mu.Unlock()

	// CLEANUP: Prevent leak on success
	if p.js != nil {
		p.js.promiseHandlersMu.Lock()
		delete(p.js.promiseHandlers, p.id)
		p.js.promiseHandlersMu.Unlock()
	}
}

// reject transitions the promise to rejected state if it's still pending.
func (p *ChainedPromise) reject(reason Result) {
	p.mu.Lock()
	if p.state.Load() != int32(Pending) {
		p.mu.Unlock()
		return
	}

	// Snapshot handlers before clearing
	h0 := p.h0
	useH0 := p.h0Used
	var handlers []handler

	if useH0 && p.result != nil {
		handlers = p.result.([]handler)
	}

	// Extract channels for notification
	channels := p.channels
	p.channels = nil

	p.result = reason
	p.state.Store(int32(Rejected))

	// CRITICAL (T27): Schedule handler microtasks WHILE holding lock.
	// This ensures proper ordering: handler microtasks run before
	// checkUnhandledRejections, preventing false-positive reports.
	if useH0 {
		p.scheduleHandler(h0, int32(Rejected), reason)
	}
	for _, h := range handlers {
		p.scheduleHandler(h, int32(Rejected), reason)
	}

	// Clear handlers AFTER scheduling their microtasks
	p.h0 = handler{}
	p.h0Used = false

	// Notify all channels registered via ToChannel()
	for _, ch := range channels {
		select {
		case ch <- reason:
		default:
		}
	}
	for _, ch := range channels {
		close(ch)
	}

	p.mu.Unlock()

	// trackRejection AFTER releasing lock, AFTER scheduling handlers
	if p.js != nil {
		p.js.trackRejection(p.id, reason, p.creationStack)
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
		return p.thenStandalone(onFulfilled, onRejected)
	}

	child := &ChainedPromise{
		id: js.nextTimerID.Add(1),
		js: js,
	}
	child.state.Store(int32(Pending))

	p.addHandler(handler{
		onFulfilled: onFulfilled,
		onRejected:  onRejected,
		target:      child,
	})

	// Rejection tracking: register handler AFTER addHandler stores/schedules it.
	// Microtasks queued by addHandler won't execute until the current synchronous
	// code completes, so this registration happens before the handler runs.
	if onRejected != nil {
		p.registerRejectionHandler(js)
	}

	return child
}

// registerRejectionHandler tracks that a rejection handler has been attached
// to the parent promise. This is used by the unhandled rejection detection system
// to avoid false-positive reports.
func (p *ChainedPromise) registerRejectionHandler(js *JS) {
	currentState := PromiseState(p.state.Load())

	switch {
	case currentState == Fulfilled:
		// Fulfilled promises can never be rejected; clean up tracking
		js.promiseHandlersMu.Lock()
		delete(js.promiseHandlers, p.id)
		js.promiseHandlersMu.Unlock()

	case currentState == Rejected:
		// Register handler first, then verify it's still needed.
		// This order prevents a race where checkUnhandledRejections processes
		// and removes the entry from unhandledRejections between our check
		// and our set, leaving an orphaned promiseHandlers entry.
		js.promiseHandlersMu.Lock()
		js.promiseHandlers[p.id] = true
		js.promiseHandlersMu.Unlock()
		p.signalHandlerReady(js)

		// Double-check: if the rejection was already processed (removed from
		// unhandledRejections by checkUnhandledRejections running concurrently),
		// clean up our handler registration to prevent a map entry leak.
		js.rejectionsMu.RLock()
		_, isUnhandled := js.unhandledRejections[p.id]
		js.rejectionsMu.RUnlock()

		if !isUnhandled {
			js.promiseHandlersMu.Lock()
			delete(js.promiseHandlers, p.id)
			js.promiseHandlersMu.Unlock()
		}

	default: // Pending
		js.promiseHandlersMu.Lock()
		js.promiseHandlers[p.id] = true
		js.promiseHandlersMu.Unlock()
		p.signalHandlerReady(js)
	}
}

// signalHandlerReady signals that a rejection handler has been registered,
// allowing trackRejection's synchronization to proceed.
func (p *ChainedPromise) signalHandlerReady(js *JS) {
	js.handlerReadyMu.Lock()
	if ch, exists := js.handlerReadyChans[p.id]; exists {
		select {
		case <-ch:
			// Already closed
		default:
			close(ch)
		}
	}
	js.handlerReadyMu.Unlock()
}

// thenStandalone creates a child promise without JS adapter for basic operations.
// Uses addHandler internally for simplified code.
//
// NOTE: This code path is NOT Promise/A+ compliant - handlers execute synchronously
// when called on already-settled promises (since p.js is nil, scheduleHandler falls
// back to executeHandler). This is intentional for testing/fallback scenarios.
func (p *ChainedPromise) thenStandalone(onFulfilled, onRejected func(Result) Result) *ChainedPromise {
	child := &ChainedPromise{
		id: p.id + 1,
		js: nil,
	}
	child.state.Store(int32(Pending))

	p.addHandler(handler{
		onFulfilled: onFulfilled,
		onRejected:  onRejected,
		target:      child,
	})

	return child
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
// Go-specific behavior: If onFinally panics, the panic value is discarded and
// the original settlement is still propagated to the child promise. This differs
// from JavaScript's Promise.prototype.finally where a throw inside finally causes
// the returned promise to be rejected with the thrown value. The Go convention is
// that cleanup panics should not silently swallow the original result.
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
	var child *ChainedPromise
	if js != nil {
		child, _, _ = js.NewChainedPromise()
	} else {
		child = &ChainedPromise{
			id: p.id + 1,
			js: nil,
		}
		child.state.Store(int32(Pending))
	}

	if onFinally == nil {
		onFinally = func() {}
	}

	// Run onFinally, then propagate the original result.
	// If onFinally panics, we still propagate the original settlement
	// (Go panics in cleanup callbacks should not change the promise outcome).
	runFinally := func(res Result, isRej bool) {
		defer func() {
			if r := recover(); r != nil {
				// Panic in finally: still propagate original settlement.
				// This matches the Go convention that cleanup panics
				// should not silently swallow the original result.
				_ = r // panic value discarded
				if isRej {
					child.reject(res)
				} else {
					child.resolve(res)
				}
			}
		}()
		onFinally()
		if isRej {
			child.reject(res)
		} else {
			child.resolve(res)
		}
	}

	p.addHandler(handler{
		onFulfilled: func(v Result) Result {
			runFinally(v, false)
			return nil // Return ignored; child is resolved manually
		},
		onRejected: func(r Result) Result {
			runFinally(r, true)
			return nil // Return ignored; child is rejected manually
		},
		target: child,
	})

	// Track rejection handler (Finally always provides onRejected)
	if js != nil {
		p.registerRejectionHandler(js)
	}

	return child
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

// trackRejection tracks a rejected promise for unhandled rejection detection.
// This is called from the reject() method.
//
// This implementation ensures that checkUnhandledRejections runs AFTER all concurrent
// handler registrations from then() by using proper channel synchronization.
// Each rejection waits for a handler to be registered (or determines none will be)
// before checking for unhandled rejections.
//
// EXPAND-039: creationStack is passed to include in debug output for unhandled rejections.
func (js *JS) trackRejection(promiseID uint64, reason Result, creationStack []uintptr) {
	// Store rejection info
	info := &rejectionInfo{
		promiseID:     promiseID,
		reason:        reason,
		timestamp:     time.Now().UnixNano(),
		creationStack: creationStack, // EXPAND-039: Store for debug output
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

		// ALWAYS run checkUnhandledRejections to catch ALL pending unhandled
		// rejections, not just this promise's. Without this, concurrent rejections
		// where one has a handler and another doesn't could result in the unhandled
		// one never being reported (the CAS gate means only one microtask runs).
		//
		// Re-check loop: after resetting the CAS gate, verify no new rejections
		// arrived during our check. Without this, a rejection that arrives between
		// the snapshot in checkUnhandledRejections and the CAS reset would be
		// orphaned permanently (its CAS failed, and no subsequent check runs).
		for {
			js.checkUnhandledRejections()
			js.checkRejectionScheduled.Store(false)

			// Re-check: if new unhandled rejections arrived during our check,
			// they would have failed the CAS and not scheduled their own microtask.
			// Re-acquire the CAS and check again to prevent orphaning them.
			js.rejectionsMu.RLock()
			pending := len(js.unhandledRejections) > 0
			js.rejectionsMu.RUnlock()
			if !pending || !js.checkRejectionScheduled.CompareAndSwap(false, true) {
				break
			}
			// New rejections arrived during our check, loop to process them
		}
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
			// EXPAND-039: If debug mode captured a creation stack, wrap the reason
			// with debug info so the callback can access where the promise was created
			if len(info.creationStack) > 0 {
				stackTrace := formatCreationStack(info.creationStack)
				callback(&UnhandledRejectionDebugInfo{
					Reason:             info.reason,
					CreationStackTrace: stackTrace,
				})
			} else {
				callback(info.reason)
			}
		}

		// Clean up tracking for unhandled rejection
		js.rejectionsMu.Lock()
		delete(js.unhandledRejections, promiseID)
		js.rejectionsMu.Unlock()
	}
}

// rejectionInfo holds information about a rejected promise.
type rejectionInfo struct {
	reason        Result    // Largest field, put first
	creationStack []uintptr // EXPAND-039: Stack trace of where the promise was created
	promiseID     uint64
	timestamp     int64
}

// UnhandledRejectionDebugInfo is passed to [RejectionHandler] when debug mode is enabled
// and the promise has a creation stack trace.
//
// EXPAND-039: This type wraps the rejection reason and includes debug information
// about where the promise was created. This helps answer "where did this promise
// come from?" when debugging unhandled rejections.
//
// Users can type-assert the reason in their [RejectionHandler] callback to access
// the debug information:
//
//	js, _ := eventloop.NewJS(loop, eventloop.WithUnhandledRejection(func(r eventloop.Result) {
//	    if debug, ok := r.(*eventloop.UnhandledRejectionDebugInfo); ok {
//	        log.Printf("Unhandled rejection: %v\\nCreated at:\\n%s",
//	            debug.Reason, debug.CreationStackTrace)
//	    } else {
//	        log.Printf("Unhandled rejection: %v", r)
//	    }
//	}))
//
// If debug mode is not enabled or the promise has no creation stack,
// the callback receives the raw rejection reason without wrapping.
type UnhandledRejectionDebugInfo struct {
	// Reason is the original rejection value from the failed promise.
	Reason Result

	// CreationStackTrace is a formatted stack trace showing where the promise
	// was created. Each frame is on its own line in the format:
	//   package.function (file:line)
	CreationStackTrace string
}

// Error implements the error interface so UnhandledRejectionDebugInfo can be
// used as an error value when the underlying Reason is also an error.
func (u *UnhandledRejectionDebugInfo) Error() string {
	if err, ok := u.Reason.(error); ok {
		return err.Error()
	}
	return fmt.Sprintf("%v", u.Reason)
}

// Unwrap returns the underlying error if Reason is an error type.
// This enables [errors.Is] and [errors.As] to work through the wrapper.
func (u *UnhandledRejectionDebugInfo) Unwrap() error {
	if err, ok := u.Reason.(error); ok {
		return err
	}
	return nil
}

// formatCreationStack formats a slice of program counters as a stack trace string.
// EXPAND-039: Used by checkUnhandledRejections to format creation stack for debug output.
func formatCreationStack(pcs []uintptr) string {
	if len(pcs) == 0 {
		return ""
	}

	frames := runtime.CallersFrames(pcs)
	var result string
	for {
		frame, more := frames.Next()
		if frame.Function != "" {
			if result != "" {
				result += "\n"
			}
			result += fmt.Sprintf("%s (%s:%d)", frame.Function, frame.File, frame.Line)
		}
		if !more {
			break
		}
	}
	return result
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
		p.Then(
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
			func(r Result) Result {
				// Reject on first rejection
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
		p.Then(
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
		p.Then(
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
		p.Then(
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

// ============================================================================
// Promise.withResolvers (ES2024 API)
// ============================================================================

// PromiseWithResolvers represents the result of Promise.withResolvers().
// It provides a convenient way to create a promise along with its
// resolve and reject functions, without requiring an executor callback.
//
// This mirrors the ES2024 Promise.withResolvers() API, which returns
// an object with { promise, resolve, reject } properties.
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
	Promise *ChainedPromise

	// Resolve is the function that fulfills the Promise with a value.
	// Calling Resolve on an already-settled promise has no effect.
	// Can be called from any goroutine.
	Resolve ResolveFunc

	// Reject is the function that rejects the Promise with a reason.
	// Calling Reject on an already-settled promise has no effect.
	// Can be called from any goroutine.
	Reject RejectFunc
}

// WithResolvers creates a new pending promise along with its resolve and reject functions.
// This is the Go equivalent of ES2024's Promise.withResolvers().
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
//	func sendRequest(js *JS, id string, data any) *ChainedPromise {
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
	promise, resolve, reject := js.NewChainedPromise()
	return &PromiseWithResolvers{
		Promise: promise,
		Resolve: resolve,
		Reject:  reject,
	}
}
