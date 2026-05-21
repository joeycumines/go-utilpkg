package eventloop

import (
	"cmp"
	"slices"
)

// handler represents a reaction to promise settlement.
type handler struct {
	onFulfilled func(any) any
	onRejected  func(any) any
	target      *ChainedPromise
}

type handlerScheduleFailure struct {
	err         error
	result      any
	target      *ChainedPromise
	state       int32
	reportOwner rejectionReportOwner
	passThrough bool
}

type pendingPromiseReaction struct {
	source  *ChainedPromise
	failure handlerScheduleFailure
	seq     uint64
}

func (r pendingPromiseReaction) fail() {
	if r.source != nil {
		r.source.handleHandlerScheduleFailure(r.failure)
	}
}

const pendingReactionOverflowRetainLimit = 64

func (l *Loop) registerPendingPromiseReaction(target, source *ChainedPromise, failure handlerScheduleFailure) {
	if target == nil || source == nil {
		return
	}
	l.pendingReactionsMu.Lock()
	l.pendingReactionSeq++
	reaction := pendingPromiseReaction{source: source, failure: failure, seq: l.pendingReactionSeq}
	if l.pendingReactionTarget == nil {
		l.pendingReactionTarget = target
		l.pendingReaction = reaction
	} else {
		if l.pendingReactionOverflow == nil {
			l.pendingReactionOverflow = make(map[*ChainedPromise]pendingPromiseReaction)
		}
		l.pendingReactionOverflow[target] = reaction
		if size := len(l.pendingReactionOverflow); size > l.pendingReactionOverflowPeak {
			l.pendingReactionOverflowPeak = size
		}
	}
	l.pendingReactionsMu.Unlock()
}

func (l *Loop) claimPendingPromiseReaction(target *ChainedPromise) (pendingPromiseReaction, bool) {
	if target == nil {
		return pendingPromiseReaction{}, false
	}
	l.pendingReactionsMu.Lock()
	if l.pendingReactionTarget == target {
		reaction := l.pendingReaction
		l.pendingReactionTarget = nil
		l.pendingReaction = pendingPromiseReaction{}
		l.pendingReactionsMu.Unlock()
		return reaction, true
	}
	reaction, ok := l.pendingReactionOverflow[target]
	if ok {
		delete(l.pendingReactionOverflow, target)
		if len(l.pendingReactionOverflow) == 0 && l.pendingReactionOverflowPeak > pendingReactionOverflowRetainLimit {
			l.pendingReactionOverflow = nil
			l.pendingReactionOverflowPeak = 0
		}
	}
	l.pendingReactionsMu.Unlock()
	return reaction, ok
}

// takePendingPromiseReactions detaches every unclaimed reaction in registration
// order. Terminal cleanup drops even the bounded reusable overflow table before
// failure callbacks run outside ownership.
func (l *Loop) takePendingPromiseReactions() []pendingPromiseReaction {
	l.pendingReactionsMu.Lock()
	count := len(l.pendingReactionOverflow)
	if l.pendingReactionTarget != nil {
		count++
	}
	var reactions []pendingPromiseReaction
	if count != 0 {
		reactions = make([]pendingPromiseReaction, 0, count)
	}
	if l.pendingReactionTarget != nil {
		reactions = append(reactions, l.pendingReaction)
	}
	for target, reaction := range l.pendingReactionOverflow {
		delete(l.pendingReactionOverflow, target)
		reactions = append(reactions, reaction)
	}
	l.pendingReactionTarget = nil
	l.pendingReaction = pendingPromiseReaction{}
	l.pendingReactionOverflow = nil
	l.pendingReactionOverflowPeak = 0
	l.pendingReactionsMu.Unlock()
	slices.SortFunc(reactions, func(a, b pendingPromiseReaction) int {
		return cmp.Compare(a.seq, b.seq)
	})
	return reactions
}

func failPendingPromiseReactions(reactions []pendingPromiseReaction) {
	for _, reaction := range reactions {
		reaction.fail()
	}
}

// promiseObserverFailure is a zero-allocation marker stored in the otherwise
// unused result slot of a private observer child. Its distinct pointer type
// identifies the public aggregate that owns infrastructure failures without
// adding a field to every promise or handler.
type promiseObserverFailure ChainedPromise

// addHandler attaches a handler to the promise. If the promise is already settled,
// the handler is scheduled immediately via microtask. If pending, the handler is
// stored for later execution when the promise settles.
//
// When handlesRejection is true, the unhandled-rejection tracker is updated
// before any concurrent rejection can report the parent as unhandled. For pending
// promises, the handler storage and promise-local handled registration are
// performed while holding p.mu so a concurrent reject cannot observe the stored
// handler before the handled state is visible.
//
// Handler registration always synchronizes through p.mu. Settlement publishes
// state before queuing the preexisting snapshot while retaining that lock, so a
// concurrent late handler cannot overtake earlier registrations.
func (p *ChainedPromise) addHandler(h handler, handlesRejection bool) handlerScheduleFailure {
	// Stable final states need no promise lock: result is immutable after the
	// release-store that publishes the stable state. Publishing sentinels take
	// the locked path so a late handler cannot overtake the preexisting snapshot.
	currentState := p.state.Load()
	if currentState == int32(Fulfilled) || currentState == int32(Rejected) {
		if handlesRejection && p.js != nil {
			p.registerRejectionHandler(p.js)
		}
		if currentState == int32(Rejected) && h.target != nil && h.onRejected == nil {
			return p.scheduleRejectionHandler(h, p.result, p.markPropagatedRejection())
		}
		return p.scheduleHandler(h, currentState, p.result)
	}
	if p.js != nil && p.js.loop != nil && p.js.loop.testHooks != nil && p.js.loop.testHooks.AfterPromiseHandlerPendingCheck != nil {
		p.js.loop.testHooks.AfterPromiseHandlerPendingCheck()
	}

	p.mu.Lock()
	currentState = p.state.Load()
	if !promisePending(currentState) {
		result := p.result
		p.mu.Unlock()
		return p.scheduleSettledHandler(h, handlesRejection, int32(promiseState(currentState)), result)
	}

	// h0 slot is unused when target is nil (no child promise chained).
	if p.h0.target == nil {
		p.h0 = h
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
	if handlesRejection && p.js != nil {
		if p.js.loop != nil && p.js.loop.testHooks != nil && p.js.loop.testHooks.BeforePromiseHandlerRegister != nil {
			p.js.loop.testHooks.BeforePromiseHandlerRegister()
		}
		p.registerRejectionHandler(p.js)
	}
	p.mu.Unlock()
	return handlerScheduleFailure{}
}

func (p *ChainedPromise) scheduleSettledHandler(h handler, handlesRejection bool, state int32, result any) handlerScheduleFailure {
	if handlesRejection && p.js != nil {
		p.registerRejectionHandler(p.js)
	}
	if state == int32(Rejected) && h.target != nil && h.onRejected == nil {
		return p.scheduleRejectionHandler(h, result, p.markPropagatedRejection())
	}
	return p.scheduleHandler(h, state, result)
}

// scheduleHandler enqueues a handler for execution via microtask.
// If no JS adapter is available, executes synchronously. When scheduling fails,
// it returns the failure for the caller to handle after any parent promise lock
// has been released; rejecting child promises synchronously while a settling
// parent is still locked can re-enter user diagnostics and deadlock.
func (p *ChainedPromise) scheduleHandler(h handler, state int32, result any) handlerScheduleFailure {
	failure := handlerScheduleFailure{target: h.target}
	if h.target != nil {
		var fn func(any) any
		if state == int32(Fulfilled) {
			fn = h.onFulfilled
		} else {
			fn = h.onRejected
		}
		if fn == nil {
			failure.state = state
			failure.result = result
			failure.passThrough = true
		}
	}
	return p.scheduleHandlerFailure(h, state, result, failure)
}

func (p *ChainedPromise) scheduleHandlerFailure(h handler, state int32, result any, failure handlerScheduleFailure) handlerScheduleFailure {
	if p.js == nil {
		p.executeHandler(h, state, result)
		return handlerScheduleFailure{}
	}

	if h.target == nil {
		if err := p.js.QueueMicrotask(func() {
			p.executeHandler(h, state, result)
		}); err != nil {
			failure.err = err
			return failure
		}
		return handlerScheduleFailure{}
	}

	failure.err = ErrLoopTerminated
	p.js.loop.registerPendingPromiseReaction(h.target, p, failure)
	if p.js.loop.testHooks != nil && p.js.loop.testHooks.AfterPromiseReactionRegister != nil {
		p.js.loop.testHooks.AfterPromiseReactionRegister()
	}
	if err := p.js.loop.schedulePromiseReaction(func() {
		p.executeHandler(h, state, result)
	}, h.target); err != nil {
		reaction, ok := p.js.loop.claimPendingPromiseReaction(h.target)
		if !ok {
			return handlerScheduleFailure{}
		}
		reaction.failure.err = err
		return reaction.failure
	}
	if p.js.loop != nil && p.js.loop.testHooks != nil && p.js.loop.testHooks.AfterPromiseHandlerScheduled != nil {
		p.js.loop.testHooks.AfterPromiseHandlerScheduled()
	}
	return handlerScheduleFailure{}
}

func (p *ChainedPromise) scheduleRejectionHandler(h handler, reason any, reportOwner rejectionReportOwner) handlerScheduleFailure {
	if h.target == nil || h.onRejected != nil {
		return p.scheduleHandler(h, int32(Rejected), reason)
	}
	target := h.target
	h.onRejected = func(value any) any {
		if p.claimAdoption(target) {
			p.settleAdoption(target, Rejected, value, reportOwner)
			return nil
		}
		if p.js != nil && target.state.Load() != int32(Pending) {
			return nil
		}
		p.propagateRejectionOwned(target, value, reportOwner)
		return nil
	}
	return p.scheduleHandlerFailure(h, int32(Rejected), reason, handlerScheduleFailure{
		target:      target,
		state:       int32(Rejected),
		result:      reason,
		reportOwner: reportOwner,
		passThrough: true,
	})
}

func (p *ChainedPromise) handleHandlerScheduleFailure(failure handlerScheduleFailure) {
	if failure.err == nil {
		return
	}
	if failure.target != nil {
		if observer, ok := takePromiseObserverFailure(failure.target); ok {
			// Internal observers have no public child. Complete their bookkeeping
			// target without manufacturing an inaccessible rejection, then route
			// the infrastructure failure to the public operation that owns it.
			failure.target.resolve(nil)
			observer.reject(failure.err)
			return
		}
		if p.claimAdoption(failure.target) {
			p.settleAdoption(failure.target, promiseState(failure.state), failure.result, failure.reportOwner)
			return
		}
		if p.js != nil && failure.target.state.Load() != int32(Pending) {
			return
		}
		if failure.passThrough {
			if failure.state == int32(Fulfilled) {
				if failure.target.state.Load() == promiseSettlementClaimed {
					failure.target.resolveClaimed(failure.result)
				} else {
					failure.target.resolve(failure.result)
				}
			} else {
				if failure.reportOwner == rejectionReportUnowned {
					p.propagateRejection(failure.target, failure.result)
				} else {
					p.propagateRejectionOwned(failure.target, failure.result, failure.reportOwner)
				}
			}
			return
		}
		failure.target.reject(failure.err)
		return
	}
	if p.js != nil && p.js.loop != nil {
		p.js.loop.logError("eventloop: failed to schedule promise handler microtask", failure.err)
	}
}

// executeHandler runs a single handler with the given state and result.
// Handles nil handlers (pass-through), panic recovery, and result propagation.
func (p *ChainedPromise) executeHandler(h handler, state int32, result any) {
	var fn func(any) any

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
		if p.claimAdoption(h.target) {
			p.settleAdoption(h.target, promiseState(state), result, rejectionReportUnowned)
			return
		}
		if p.js != nil && h.target.state.Load() != int32(Pending) {
			return
		}
		if state == int32(Fulfilled) {
			if h.target.state.Load() == promiseSettlementClaimed {
				h.target.resolveClaimed(result)
			} else {
				h.target.resolve(result)
			}
		} else {
			p.propagateRejection(h.target, result)
		}
		return
	}

	completed := false
	defer func() {
		if !completed && h.target != nil {
			if observer, ok := takePromiseObserverFailure(h.target); ok {
				h.target.resolve(nil)
				observer.reject(ErrGoexit)
				return
			}
			h.target.reject(ErrGoexit)
		}
	}()

	var res any
	panicValue, panicked := invokeCallback(func() { res = fn(result) })
	completed = true
	if panicked {
		if h.target != nil {
			err := PanicError{Value: panicValue}
			if observer, ok := takePromiseObserverFailure(h.target); ok {
				h.target.resolve(nil)
				observer.reject(err)
				return
			}
			h.target.reject(err)
		}
		return
	}
	if h.target != nil {
		h.target.resolve(res)
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
// For a loop-backed promise, accepted handlers participate in an exhaustive
// microtask checkpoint on the logical callback owner, including a dedicated
// pre-Run terminal drain. Graceful lifecycle control does not preempt that
// checkpoint. A non-nil handler denied by terminal callback admission rejects
// its child with [ErrLoopTerminated]. Terminal failures may synchronously
// propagate nil-handler pass-through settlements without running user handler
// code.
func (p *ChainedPromise) Then(onFulfilled, onRejected func(any) any) *ChainedPromise {
	js := p.js
	if js == nil {
		return p.thenStandalone(onFulfilled, onRejected)
	}

	child := &ChainedPromise{
		js: js,
	}
	child.state.Store(int32(Pending))

	handlesRejection := onRejected != nil
	failure := p.addHandler(handler{
		onFulfilled: onFulfilled,
		onRejected:  onRejected,
		target:      child,
	}, handlesRejection)
	p.handleHandlerScheduleFailure(failure)

	return child
}

// observeSettlement attaches internal handlers whose callback result is not
// exposed. Scheduling and execution failures reject the owning aggregate; the
// private bookkeeping child is fulfilled so it cannot become an orphaned
// unhandled rejection.
func (p *ChainedPromise) observeSettlement(onFulfilled, onRejected func(any) any, observer *ChainedPromise) *ChainedPromise {
	child := &ChainedPromise{
		js:     p.js,
		result: (*promiseObserverFailure)(observer),
	}
	child.state.Store(int32(Pending))

	failure := p.addHandler(handler{
		onFulfilled: onFulfilled,
		onRejected:  onRejected,
		target:      child,
	}, onRejected != nil)
	p.handleHandlerScheduleFailure(failure)
	return child
}

func takePromiseObserverFailure(target *ChainedPromise) (*ChainedPromise, bool) {
	target.mu.Lock()
	marker, ok := target.result.(*promiseObserverFailure)
	if ok {
		target.result = nil
	}
	target.mu.Unlock()
	return (*ChainedPromise)(marker), ok
}

func (p *ChainedPromise) propagateRejection(target *ChainedPromise, reason any) {
	p.propagateRejectionOwned(target, reason, p.markPropagatedRejection())
}

func (p *ChainedPromise) propagateRejectionOwned(target *ChainedPromise, reason any, reportOwner rejectionReportOwner) {
	if reportOwner == rejectionReportPropagation {
		reportOwner = p.claimPropagatedRejection()
	}
	targetClaimed := target.state.Load() == promiseSettlementClaimed
	if reportOwner == rejectionReportChecker && p.js != nil {
		// The parent checker already owns reporting. Suppress the derived child so
		// the same rejection does not produce both parent and child diagnostics.
		target.rejectionHandled.Store(true)
		target.rejectionHandled.markReported()
	}
	if targetClaimed {
		target.rejectClaimed(reason)
	} else {
		target.reject(reason)
	}
}

func (p *ChainedPromise) markPropagatedRejection() rejectionReportOwner {
	js := p.js
	if js == nil {
		return rejectionReportUnowned
	}
	owner := rejectionReportUnowned
	js.rejectionsMu.Lock()
	if info := js.unhandledRejections[p]; info != nil {
		info.propagationPending.Add(1)
		owner = rejectionReportPropagation
	}
	js.rejectionsMu.Unlock()
	if owner == rejectionReportUnowned && p.rejectionHandled.reported() {
		return rejectionReportChecker
	}
	return owner
}

func (p *ChainedPromise) claimPropagatedRejection() rejectionReportOwner {
	js := p.js
	if js == nil {
		return rejectionReportUnowned
	}
	owned := false
	js.rejectionsMu.Lock()
	if !p.rejectionHandled.reported() {
		if info := js.unhandledRejections[p]; info != nil {
			info.propagated.Store(true)
			delete(js.unhandledRejections, p)
			owned = true
		}
	}
	js.rejectionsMu.Unlock()
	if !owned {
		if p.rejectionHandled.reported() {
			return rejectionReportChecker
		}
		// Another propagation branch may already have consumed the parent record.
		// That transfers only its own descendant; this sibling must retain normal
		// unhandled-rejection ownership rather than impersonating the checker.
		return rejectionReportUnowned
	}
	js.handlerReadyMu.Lock()
	delete(js.handlerReadyChans, p)
	js.handlerReadyMu.Unlock()
	return rejectionReportPropagation
}

// registerRejectionHandler tracks that a rejection handler has been attached
// to the parent promise. This is used by the unhandled rejection detection system
// to avoid false-positive reports.
func (p *ChainedPromise) registerRejectionHandler(js *JS) {
	p.rejectionHandled.Store(true)

	currentState := promiseState(p.state.Load())

	switch {
	case currentState == Fulfilled:
		// Fulfilled promises can never be rejected; clean up synchronization state.
		js.handlerReadyMu.Lock()
		delete(js.handlerReadyChans, p)
		js.handlerReadyMu.Unlock()

	case currentState == Rejected:
		p.signalHandlerReady(js)

		// Double-check: if the rejection was already processed, clean up any
		// handler-publication synchronization state. The handled state itself lives
		// on the promise, so late-handler registration does not add strong roots.
		js.rejectionsMu.RLock()
		_, isUnhandled := js.unhandledRejections[p]
		js.rejectionsMu.RUnlock()

		if !isUnhandled {
			js.handlerReadyMu.Lock()
			delete(js.handlerReadyChans, p)
			js.handlerReadyMu.Unlock()
		}

	default: // Pending
		p.signalHandlerReady(js)
	}
}

// signalHandlerReady signals that a rejection handler has been registered,
// allowing trackRejection's synchronization to proceed.
func (p *ChainedPromise) signalHandlerReady(js *JS) {
	js.handlerReadyMu.Lock()
	if ch, exists := js.handlerReadyChans[p]; exists {
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
func (p *ChainedPromise) thenStandalone(onFulfilled, onRejected func(any) any) *ChainedPromise {
	child := &ChainedPromise{
		js: nil,
	}
	child.state.Store(int32(Pending))

	p.handleHandlerScheduleFailure(p.addHandler(handler{
		onFulfilled: onFulfilled,
		onRejected:  onRejected,
		target:      child,
	}, false))

	return child
}

// Catch adds a rejection handler to the promise.
// Returns a new [ChainedPromise] that resolves with the result of the handler.
//
// This is equivalent to calling Then(nil, onRejected).
//
// Use Catch to recover from errors or transform rejection reasons:
//
//	promise.Catch(func(r any) any {
//	    log.Printf("Error: %v", r)
//	    return defaultValue // recover
//	})
func (p *ChainedPromise) Catch(onRejected func(any) any) *ChainedPromise {
	return p.Then(nil, onRejected)
}

// Finally adds a handler that runs regardless of how the promise settles.
// Returns a new [ChainedPromise] that preserves the original settlement when
// the handler runs.
//
// Unlike Then/Catch, the onFinally callback receives no arguments and its
// return value is ignored. If terminal loop cleanup prevents a non-nil handler
// from running, the returned promise is rejected with [ErrLoopTerminated]. A
// nil handler is a pure pass-through and preserves the original settlement
// even when terminal cleanup disposes of its accepted reaction.
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
	if onFinally == nil {
		return p.Then(nil, nil)
	}

	js := p.js
	var child *ChainedPromise
	if js != nil {
		child, _, _ = js.NewChainedPromise()
	} else {
		child = &ChainedPromise{
			js: nil,
		}
		child.state.Store(int32(Pending))
	}

	// Run onFinally, then propagate the original result.
	// If onFinally panics, we still propagate the original settlement
	// (Go panics in cleanup callbacks should not change the promise outcome).
	runFinally := func(res any, isRej bool) {
		settle := func() {
			if isRej {
				child.reject(res)
			} else {
				child.resolve(res)
			}
		}
		completed := false
		defer func() {
			if !completed {
				settle()
			}
		}()
		_, _ = invokeCallback(onFinally)
		completed = true
		settle()
	}

	failure := p.addHandler(handler{
		onFulfilled: func(v any) any {
			runFinally(v, false)
			return nil // Return ignored; child is resolved manually
		},
		onRejected: func(r any) any {
			runFinally(r, true)
			return nil // Return ignored; child is rejected manually
		},
		target: child,
	}, js != nil)
	p.handleHandlerScheduleFailure(failure)

	return child
}

// ToChannel returns a channel that will receive the result when the promise settles.
// The channel is buffered (capacity 1) and will be closed after sending.
// If the promise is already settled, returns a pre-filled channel.
// Thread-safe and can be called from any goroutine.
//
// For JS-backed promises, channels are registered in the JS.toChannels side table
// and notified synchronously during resolve/reject (without going through the
// microtask queue). This ensures ToChannel works even when the event loop is not
// running.
//
// For standalone promises (p.js == nil), a handler-based fallback is used.
func (p *ChainedPromise) ToChannel() <-chan any {
	ch := make(chan any, 1)

	// Fast path: already settled (lock-free)
	currentState := p.state.Load()
	if !promisePending(currentState) {
		ch <- p.result
		close(ch)
		return ch
	}

	// Standalone promise (no JS adapter): use handler-based fallback.
	// Handlers execute synchronously for standalone promises (no microtask queue),
	// so this path works correctly without an event loop.
	if p.js == nil {
		return p.toChannelStandalonePromise(ch)
	}
	if p.js.loop != nil && p.js.loop.testHooks != nil && p.js.loop.testHooks.AfterPromiseToChannelPendingCheck != nil {
		p.js.loop.testHooks.AfterPromiseToChannelPendingCheck()
	}

	// JS-backed promise: register in side table for direct notification.
	// Lock ordering: p.mu → js.toChannelsMu (same order as resolve/reject).
	p.mu.Lock()
	// Double-check state under lock to handle the race where resolve/reject
	// ran between the fast-path check and lock acquisition.
	currentState = p.state.Load()
	if !promisePending(currentState) {
		p.mu.Unlock()
		ch <- p.result
		close(ch)
		return ch
	}
	p.js.toChannelsMu.Lock()
	p.js.toChannels[p] = append(p.js.toChannels[p], ch)
	p.js.toChannelsMu.Unlock()
	p.mu.Unlock()

	return ch
}

// toChannelStandalonePromise handles ToChannel for promises without a JS adapter.
// Uses addHandler with a dummy target to avoid interfering with the h0 slot check.
func (p *ChainedPromise) toChannelStandalonePromise(ch chan any) <-chan any {
	dummy := &ChainedPromise{}
	dummy.state.Store(int32(Pending))
	writeFn := func(v any) any {
		ch <- v
		close(ch)
		return nil
	}
	p.handleHandlerScheduleFailure(p.addHandler(handler{
		onFulfilled: writeFn,
		onRejected:  writeFn,
		target:      dummy,
	}, false))
	return ch
}
