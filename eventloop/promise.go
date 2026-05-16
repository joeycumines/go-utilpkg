package eventloop

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"weak"
)

// ============================================================================
// ChainedPromise Implementation
// ============================================================================

// ChainedPromise implements the package's Go-loop promise profile with [Then], [Catch], and [Finally].
//
// This implementation is modeled on Promise/A+ reaction ordering for this
// package's own promise values. It is not a full ECMAScript Promise
// implementation and does not assimilate arbitrary JavaScript thenables. All
// handler callbacks are scheduled as microtasks and executed on the event loop
// thread when a JS adapter is attached. If terminal callback admission discards
// an accepted non-nil handler before execution, its child rejects with
// [ErrLoopTerminated]; nil-handler pass-through retains the parent settlement.
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
//	    Then(func(v any) any {
//	        return transform(v)
//	    }, nil).
//	    Catch(func(r any) any {
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
// called from any goroutine. With a JS adapter attached, handlers execute as
// loop microtasks during normal [Loop.Run] execution; if scheduling is no longer
// possible during terminal paths, nil-handler pass-through may settle child
// promises synchronously to preserve the parent settlement.
//
// Resolving with another ChainedPromise adopts that source irrevocably. Sources
// owned by a different JS adapter settle the target through the source owner and
// enqueue target reactions through the target adapter. An adoption transfer
// accepted before immediate source-loop termination is recovered during source
// adapter cleanup rather than discarded with the ordinary microtask queue.
// Indirect native-promise cycles are deliberately left pending; only direct
// self-resolution is rejected.
//
// Performance Characteristics:
//   - ToChannel() uses JS.toChannels side table for direct notification without microtasks
//   - Debug stack traces stored in JS side table, only captured when debug mode is enabled
//   - Pointer identity (*ChainedPromise) used as map key instead of integer IDs
//   - result field reused for handler storage during pending state
//
// Memory Usage:
//   - Base struct: 64 bytes on 64-bit targets and 40 bytes on 32-bit targets
//   - Creation stack traces stored in JS.debugStacks side table (only when debug mode enabled)
//   - Promise identity uses pointer (*ChainedPromise) as map key instead of integer ID
//   - ToChannel() channels stored in JS.toChannels side table (not on the struct)
type ChainedPromise struct {
	// Pointer-bearing fields are grouped first for cache locality.
	result any
	js     *JS
	// h0 is the first handler (embedded to avoid slice allocation).
	// Most promises have only 1 handler.
	h0 handler

	// Atomic state
	state            atomic.Int32
	rejectionHandled rejectionStatus

	// Non-pointer synchronization primitives
	mu sync.Mutex
}

const (
	rejectionStatusHandled uint32 = 1 << iota
	rejectionStatusReported
)

type rejectionStatus struct {
	value atomic.Uint32
}

func (s *rejectionStatus) Load() bool {
	return s.value.Load()&rejectionStatusHandled != 0
}

func (s *rejectionStatus) Store(handled bool) {
	if handled {
		s.value.Or(rejectionStatusHandled)
		return
	}
	for {
		current := s.value.Load()
		if s.value.CompareAndSwap(current, current&^rejectionStatusHandled) {
			return
		}
	}
}

func (s *rejectionStatus) markReported() {
	s.value.Or(rejectionStatusReported)
}

func (s *rejectionStatus) reported() bool {
	return s.value.Load()&rejectionStatusReported != 0
}

type rejectionReportOwner uint8

const (
	rejectionReportUnowned rejectionReportOwner = iota
	rejectionReportPropagation
	rejectionReportChecker
)

// ResolveFunc is the function used to fulfill a promise with a value.
// Calling resolve on an already-settled promise has no effect.
// Can be called from any goroutine.
type ResolveFunc func(any)

// RejectFunc is the function used to reject a promise with a reason.
// Calling reject on an already-settled promise has no effect.
// Can be called from any goroutine.
type RejectFunc func(any)

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
		js: js,
	}
	p.state.Store(int32(Pending))

	// Capture creation stack trace when debug mode is enabled.
	// Stored in JS.debugStacks side table (keyed by weak.Pointer to avoid
	// pinning the promise in memory). runtime.AddCleanup removes the entry
	// when the promise is garbage collected.
	if js.loop != nil && js.loop.debugMode {
		var pcs [32]uintptr
		n := runtime.Callers(2, pcs[:])
		if n > 0 {
			wp := weak.Make(p)
			js.debugStacksMu.Lock()
			js.debugStacks[wp] = pcs[:n]
			js.debugStacksMu.Unlock()
			runtime.AddCleanup(p, func(key weak.Pointer[ChainedPromise]) {
				js.debugStacksMu.Lock()
				delete(js.debugStacks, key)
				js.debugStacksMu.Unlock()
			}, wp)
		}
	}

	resolve := func(value any) {
		p.resolve(value)
	}

	reject := func(reason any) {
		p.reject(reason)
	}

	return p, resolve, reject
}

// State returns the current [PromiseState] of this promise.
// Thread-safe and can be called from any goroutine.
func (p *ChainedPromise) State() PromiseState {
	return promiseState(p.state.Load())
}

// Value returns the fulfillment value if the promise is fulfilled.
// Returns nil if the promise is pending or rejected.
// Thread-safe and can be called from any goroutine.
func (p *ChainedPromise) Value() any {
	if promiseState(p.state.Load()) == Fulfilled {
		return p.result
	}
	return nil
}

// Reason returns the rejection reason if the promise is rejected.
// Returns nil if the promise is pending or fulfilled.
// Thread-safe and can be called from any goroutine.
func (p *ChainedPromise) Reason() any {
	if promiseState(p.state.Load()) == Rejected {
		return p.result
	}
	return nil
}

// CreationStackTrace returns a formatted stack trace of where this promise was created.
//
// This method returns an empty string unless debug mode was enabled on the
// event loop when the promise was created. Use [WithDebugMode] to enable stack trace capture.
//
// The returned string contains one line per stack frame, formatted as:
//
//	package.function (file:line)
//
// Example:
//
//	loop, err := eventloop.New(eventloop.WithDebugMode(true))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	js, err := eventloop.NewJS(loop)
//	if err != nil {
//	    log.Fatal(err)
//	}
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
	if p.js == nil {
		return ""
	}

	wp := weak.Make(p)
	p.js.debugStacksMu.Lock()
	stack := p.js.debugStacks[wp]
	p.js.debugStacksMu.Unlock()

	if len(stack) == 0 {
		return ""
	}

	frames := runtime.CallersFrames(stack)
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

// reject transitions the promise to rejected state if it's still pending.
func (p *ChainedPromise) reject(reason any) {
	if !p.state.CompareAndSwap(int32(Pending), promiseSettlementClaimed) {
		return
	}
	p.rejectClaimed(reason)
}

func (p *ChainedPromise) rejectClaimed(reason any) {
	if p.js != nil && p.js.loop != nil && p.js.loop.testHooks != nil && p.js.loop.testHooks.BeforePromiseRejectLock != nil {
		p.js.loop.testHooks.BeforePromiseRejectLock()
	}
	p.mu.Lock()
	if p.state.Load() != promiseSettlementClaimed {
		p.mu.Unlock()
		return
	}

	// Snapshot handlers before clearing
	h0 := p.h0
	useH0 := p.h0.target != nil
	var handlers []handler
	var scheduleFailures []handlerScheduleFailure
	rejectionRecorded := false
	reportOwner := rejectionReportUnowned

	if useH0 && p.result != nil {
		handlers = p.result.([]handler)
	}
	propagationPending := useH0 && h0.onRejected == nil
	if !propagationPending {
		for _, h := range handlers {
			if h.target != nil && h.onRejected == nil {
				propagationPending = true
				break
			}
		}
	}

	p.h0 = handler{} // Clears h0
	p.result = reason

	if p.js != nil {
		// Record rejection bookkeeping and publish state before any reaction can
		// execute. Holding p.mu through existing-handler scheduling preserves FIFO
		// against concurrent late handler registration.
		p.js.recordRejection(p, reason)
		rejectionRecorded = true
		if propagationPending {
			// A queued pass-through will transfer this rejection to its child.
			// Publish that intent before Rejected becomes visible so an already
			// active checker cannot report both parent and child.
			reportOwner = p.markPropagatedRejection()
		}
		if p.js.loop != nil && p.js.loop.testHooks != nil && p.js.loop.testHooks.AfterPromiseRejectionRecorded != nil {
			p.js.loop.testHooks.AfterPromiseRejectionRecorded()
		}
		p.state.Store(promiseRejectedPublishing)
		if useH0 {
			if failure := p.scheduleRejectionHandler(h0, reason, reportOwner); failure.err != nil {
				scheduleFailures = append(scheduleFailures, failure)
			}
		}
		for _, h := range handlers {
			if failure := p.scheduleRejectionHandler(h, reason, reportOwner); failure.err != nil {
				scheduleFailures = append(scheduleFailures, failure)
			}
		}
	} else {
		p.state.Store(promiseRejectedPublishing)
	}

	if p.js != nil {
		p.js.notifyToChannels(p, reason)
	}
	p.state.Store(int32(Rejected))
	p.mu.Unlock()
	if p.js != nil && p.js.loop != nil && p.js.loop.testHooks != nil && p.js.loop.testHooks.AfterPromiseRejectedStateStore != nil {
		p.js.loop.testHooks.AfterPromiseRejectedStateStore()
	}

	if p.js == nil {
		if useH0 {
			p.handleHandlerScheduleFailure(p.scheduleRejectionHandler(h0, reason, rejectionReportUnowned))
		}
		for _, h := range handlers {
			p.handleHandlerScheduleFailure(p.scheduleRejectionHandler(h, reason, rejectionReportUnowned))
		}
	}

	for _, failure := range scheduleFailures {
		p.handleHandlerScheduleFailure(failure)
	}

	// Schedule the unhandled-rejection check AFTER releasing lock, AFTER scheduling
	// handlers. The rejection record itself was inserted before publishing
	// Rejected, closing the concurrent late-handler registration window.
	if p.js != nil {
		if rejectionRecorded {
			p.js.scheduleRejectionCheck(p)
		} else {
			p.js.trackRejection(p, reason)
		}
	}
}
