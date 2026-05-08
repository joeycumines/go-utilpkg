package eventloop

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"weak"

	"github.com/joeycumines/goroutineid"
)

type abortHandler struct {
	handler func(reason any)
	active  atomic.Bool
}

type abortAlgorithm struct {
	claim  func(reason any) *abortDispatch
	active atomic.Bool
}

// AbortSignal is a Go-native, concurrently safe cancellation signal controlled
// by an [AbortController].
//
// The API is inspired by the DOM AbortSignal shape, but callbacks receive the
// Go reason value directly and this type is not a DOM EventTarget. JavaScript
// runtime semantics belong to the goja-eventloop adapter.
//
// Usage:
//
//	controller := eventloop.NewAbortController()
//	signal := controller.Signal()
//
//	// Check if aborted
//	if signal.Aborted() {
//	    // Handle aborted state
//	}
//
//	// Add abort handler
//	signal.OnAbort(func(reason any) {
//	    fmt.Println("Aborted with reason:", reason)
//	})
//
//	// Abort the operation
//	controller.Abort("User cancelled")
type AbortSignal struct { //nolint:govet // betteralign:ignore
	handlers    []*abortHandler
	algorithms  []*abortAlgorithm
	reason      any
	mu          sync.RWMutex
	aborted     bool
	dispatching bool
}

type abortDispatch struct {
	reason    any
	signal    *AbortSignal
	cleanup   func()
	handlers  []*abortHandler
	children  []*abortDispatch
	completed bool
}

// newAbortSignal creates a new AbortSignal.
// This is an internal function; signals are created via AbortController.
func newAbortSignal() *AbortSignal {
	return &AbortSignal{}
}

// Aborted returns true if the signal has been aborted.
//
// Thread Safety: Safe to call concurrently.
func (s *AbortSignal) Aborted() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.aborted
}

// Reason returns the exact abort reason, or nil before the signal is aborted.
//
// [AbortController.Abort] replaces a nil reason with one default *[AbortError],
// so an aborted controller signal always has a non-nil reason.
//
// Thread Safety: Safe to call concurrently.
func (s *AbortSignal) Reason() any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.reason
}

// OnAbort registers a callback function to be invoked when the signal is aborted.
//
// If the signal is already aborted at the time of registration, the callback
// is invoked immediately with the current abort reason.
//
// Callbacks registered before settlement run in registration order. A callback
// registered while settlement callbacks are running joins the end of that same
// delivery. All callback references are detached before invocation. Callbacks
// run without the signal lock; if callbacks panic, delivery continues and,
// unless a later callback calls runtime.Goexit, the first panic is re-raised
// after every callback has run and cleanup has finished.
// A nil panic is canonicalized to *[runtime.PanicNilError], including when the
// process enables the legacy GODEBUG=panicnil=1 behavior. A later runtime.Goexit
// ends the remaining delivery but cannot suppress an earlier captured panic.
//
// Thread Safety: Safe to call concurrently.
func (s *AbortSignal) OnAbort(handler func(reason any)) {
	s.addAbortHandler(handler)
}

// ThrowIfAborted returns an error if the signal has been aborted.
//
// It returns nil before abort. If the stored reason is a non-nil error, the
// exact error is returned. Other reasons, including typed-nil errors, are
// wrapped once in *[AbortError].
//
// Thread Safety: Safe to call concurrently.
func (s *AbortSignal) ThrowIfAborted() error {
	s.mu.RLock()
	if !s.aborted {
		s.mu.RUnlock()
		return nil
	}
	reason := s.reason
	s.mu.RUnlock()

	if err := nonNilError(reason); err != nil {
		return err
	}
	return &AbortError{Reason: reason}
}

// abort is called by AbortController to abort the signal.
// This is an internal method.
func (s *AbortSignal) abort(reason any, cleanup func()) bool {
	dispatch, ok := s.beginAbort(reason, cleanup)
	if !ok {
		return false
	}
	runAbortDispatch(dispatch)
	return true
}

func (s *AbortSignal) beginAbort(reason any, cleanup func()) (*abortDispatch, bool) {
	s.mu.Lock()
	if s.aborted {
		s.mu.Unlock()
		return nil, false
	}

	s.aborted = true
	s.reason = reason
	s.dispatching = true
	dispatch := &abortDispatch{
		signal:   s,
		reason:   reason,
		handlers: s.handlers,
		cleanup:  cleanup,
	}
	s.handlers = nil
	algorithms := s.algorithms
	s.algorithms = nil
	for i, algorithm := range algorithms {
		if algorithm != nil && algorithm.active.CompareAndSwap(true, false) {
			if child := algorithm.claim(reason); child != nil {
				dispatch.children = append(dispatch.children, child)
			}
		}
		algorithms[i] = nil
	}
	s.mu.Unlock()
	return dispatch, true
}

func runAbortDispatch(root *abortDispatch) {
	dispatches := appendAbortDispatches(nil, root)
	panicState := abortPanicState{}
	completed := false
	defer func() {
		for _, dispatch := range dispatches {
			dispatch.abandon()
		}
		if !completed && panicState.panicked {
			panic(panicState.value)
		}
	}()

	for _, dispatch := range dispatches {
		panicState.capture(dispatch.runCleanup())
	}
	for _, dispatch := range dispatches {
		dispatch.runHandlers(&panicState)
	}

	completed = true
	if panicState.panicked {
		panic(panicState.value)
	}
}

type abortPanicState struct {
	value    any
	panicked bool
}

func (s *abortPanicState) capture(value any, panicked bool) {
	if panicked && !s.panicked {
		s.value = value
		s.panicked = true
	}
}

func appendAbortDispatches(dst []*abortDispatch, dispatch *abortDispatch) []*abortDispatch {
	if dispatch == nil {
		return dst
	}
	for i, child := range dispatch.children {
		dst = appendAbortDispatches(dst, child)
		dispatch.children[i] = nil
	}
	dispatch.children = nil
	return append(dst, dispatch)
}

func (d *abortDispatch) runCleanup() (any, bool) {
	cleanup := d.cleanup
	d.cleanup = nil
	if cleanup == nil {
		return nil, false
	}
	return invokeAbortHandler(func(any) { cleanup() }, nil)
}

func (d *abortDispatch) runHandlers(panicState *abortPanicState) {
	defer func() {
		if !d.completed {
			d.abandon()
		}
	}()
	handlers := d.handlers
	d.handlers = nil
	for {
		for i, entry := range handlers {
			if entry != nil && entry.active.CompareAndSwap(true, false) {
				panicState.capture(invokeAbortHandler(entry.handler, d.reason))
			}
			handlers[i] = nil
		}

		d.signal.mu.Lock()
		if len(d.signal.handlers) == 0 {
			d.signal.dispatching = false
			d.signal.mu.Unlock()
			d.completed = true
			return
		}
		handlers = d.signal.handlers
		d.signal.handlers = nil
		d.signal.mu.Unlock()
	}
}

func (d *abortDispatch) abandon() {
	if d == nil || d.completed {
		return
	}
	d.cleanup = nil
	d.signal.mu.Lock()
	queued := d.signal.handlers
	d.signal.handlers = nil
	d.signal.dispatching = false
	d.signal.mu.Unlock()
	for i, entry := range d.handlers {
		if entry != nil {
			entry.active.Store(false)
		}
		d.handlers[i] = nil
	}
	d.handlers = nil
	for i, entry := range queued {
		if entry != nil {
			entry.active.Store(false)
		}
		queued[i] = nil
	}
	d.completed = true
}

func (s *AbortSignal) addAbortHandler(handler func(reason any)) {
	if handler == nil {
		return
	}

	entry := &abortHandler{handler: handler}
	entry.active.Store(true)

	s.mu.Lock()
	if !s.aborted || s.dispatching {
		s.handlers = append(s.handlers, entry)
		s.mu.Unlock()
		return
	}
	reason := s.reason
	s.mu.Unlock()

	if entry.active.CompareAndSwap(true, false) {
		if value, panicked := invokeAbortHandler(handler, reason); panicked {
			panic(value)
		}
	}
}

func (s *AbortSignal) addAbortAlgorithm(claim func(reason any) *abortDispatch) func() {
	if claim == nil {
		return nil
	}

	algorithm := &abortAlgorithm{claim: claim}
	algorithm.active.Store(true)

	s.mu.Lock()
	if !s.aborted {
		s.algorithms = append(s.algorithms, algorithm)
		s.mu.Unlock()
		return func() { s.removeAbortAlgorithm(algorithm) }
	}
	reason := s.reason
	s.mu.Unlock()

	if algorithm.active.CompareAndSwap(true, false) {
		if dispatch := claim(reason); dispatch != nil {
			runAbortDispatch(dispatch)
		}
	}
	return nil
}

func (s *AbortSignal) removeAbortAlgorithm(target *abortAlgorithm) {
	if target == nil || !target.active.Swap(false) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for i, algorithm := range s.algorithms {
		if algorithm != target {
			continue
		}
		copy(s.algorithms[i:], s.algorithms[i+1:])
		last := len(s.algorithms) - 1
		s.algorithms[last] = nil
		s.algorithms = s.algorithms[:last]
		if len(s.algorithms) == 0 {
			s.algorithms = nil
		}
		return
	}
}

func invokeAbortHandler(handler func(reason any), reason any) (value any, panicked bool) {
	return invokeCallback(func() { handler(reason) })
}

// AbortController settles its associated [AbortSignal].
//
// Abort is safe to call concurrently. Construct controllers with
// [NewAbortController]; the zero value is not usable.
//
// Usage:
//
//	controller := eventloop.NewAbortController()
//	signal := controller.Signal()
//
//	// Pass signal to async operation
//	go func() {
//	    // Check periodically
//	    if signal.Aborted() {
//	        return // Operation cancelled
//	    }
//	    // Continue work...
//	}()
//
//	// Later, abort the operation
//	controller.Abort("Operation timed out")
type AbortController struct {
	signal       *AbortSignal
	timeoutState *abortTimeoutState
}

// NewAbortController creates a new AbortController with a fresh AbortSignal.
//
// The returned controller can be used to abort operations that accept its
// associated Signal().
func NewAbortController() *AbortController {
	return &AbortController{
		signal: newAbortSignal(),
	}
}

// Signal returns the AbortSignal associated with this controller.
//
// The returned signal can be passed to asynchronous operations to allow
// them to be aborted when Abort() is called on the controller.
//
// Thread Safety: Safe to call concurrently. Always returns the same signal.
func (c *AbortController) Signal() *AbortSignal {
	return c.signal
}

// Abort aborts the controller's signal with the given reason.
//
// If reason is nil, a default AbortError is used as the reason.
//
// Once aborted, the signal's Aborted() method returns true, its Reason()
// method returns the abort reason, and registered handlers are delivered as
// documented by [AbortSignal.OnAbort]. A handler panic does not skip later
// handlers. runtime.Goexit ends the current delivery after internal cleanup; an
// earlier captured handler panic is still re-raised while the goroutine unwinds.
//
// Calling Abort() multiple times has no additional effect; the signal
// remains in its aborted state with the original reason.
//
// For a controller returned by [AbortTimeout], the timer callback or one Abort
// invocation atomically claims settlement. Every losing Abort waits until that
// winner publishes the signal's stable reason before returning. It does not wait
// for timer cleanup or handler delivery.
//
// Thread Safety: Safe to call concurrently from any goroutine.
func (c *AbortController) Abort(reason any) {
	// If no reason provided, use a default AbortError
	if reason == nil {
		reason = &AbortError{Reason: "Aborted"}
	}
	if state := c.timeoutState; state != nil {
		if !state.claimManual() {
			state.waitPublished()
			return
		}
		dispatch, ok := c.signal.beginAbort(reason, state.cancel)
		state.publish()
		if ok {
			runAbortDispatch(dispatch)
		}
		return
	}
	c.signal.abort(reason, nil)
}

// AbortError wraps a non-error Go abort reason or supplies the default reason.
type AbortError struct {
	// Reason contains the abort reason provided to AbortController.Abort().
	Reason any
}

// Error implements the error interface.
func (e *AbortError) Error() string {
	if e == nil || e.Reason == nil {
		return "AbortError: The operation was aborted"
	}
	if s, ok := e.Reason.(string); ok {
		return "AbortError: " + s
	}
	if err := nonNilError(e.Reason); err != nil {
		return "AbortError: " + err.Error()
	}
	return "AbortError: The operation was aborted"
}

// Is implements errors.Is support for AbortError.
func (e *AbortError) Is(target error) bool {
	match, ok := target.(*AbortError)
	return ok && match != nil
}

// Unwrap returns the underlying error if Reason is an error type.
// This enables use with [errors.Is] and [errors.As] for error matching
// through the cause chain (ES2022 Error.cause compatibility).
//
// If Reason is not an error, returns nil.
func (e *AbortError) Unwrap() error {
	if e == nil {
		return nil
	}
	return nonNilError(e.Reason)
}

// AbortTimeout creates an AbortController that aborts after delayMs milliseconds.
//
// Timeout settlement stores one *[TimeoutError], which [AbortSignal.Reason] and
// [AbortSignal.ThrowIfAborted] return by identity. The timer callback or one
// manual Abort invocation atomically claims settlement. A manual winner queues
// cancellation of the referenced timer so it no longer keeps the loop alive or
// retains its callback. Every losing Abort joins signal publication before
// returning without waiting for cleanup or handler delivery.
// Loop termination retires an unclaimed timer and releases the controller's loop
// reference without aborting the signal.
//
// Timeout handlers run on an isolated delegated-owner goroutine while the loop
// waits. Loop APIs treat that goroutine as the owner, preserving callback-local
// scheduling and lifecycle behavior while containing runtime.Goexit. A handler
// panic is relayed to the loop's normal callback panic boundary.
//
// AbortTimeout panics if loop was not created by [New], delayMs is negative, or
// the millisecond value cannot be represented as time.Duration. It returns an
// error when the loop's current lifecycle state rejects timer scheduling.
//
// Example:
//
//	controller, err := eventloop.AbortTimeout(loop, 5000) // 5 second timeout
//	if err != nil {
//	    return err
//	}
//	signal := controller.Signal()
//	// Pass signal to fetch or other async operation
func AbortTimeout(loop *Loop, delayMs int) (*AbortController, error) {
	if loop == nil || loop.state == nil || loop.commands == nil {
		panic("eventloop: AbortTimeout requires a Loop created by New")
	}
	if delayMs < 0 {
		panic("eventloop: negative AbortTimeout delay")
	}
	const maxDelayMillis = int64((1<<63 - 1) / int64(time.Millisecond))
	if int64(delayMs) > maxDelayMillis {
		panic("eventloop: AbortTimeout delay overflows time.Duration")
	}

	controller := NewAbortController()
	state := &abortTimeoutState{
		loop:      loop,
		published: make(chan struct{}),
	}
	timerID, err := loop.scheduleTimerRetire(time.Duration(delayMs)*time.Millisecond, func() {
		if !state.claimTimeout() {
			return
		}
		if loop.testHooks != nil && loop.testHooks.AfterAbortTimeoutClaim != nil {
			loop.testHooks.AfterAbortTimeoutClaim()
		}
		dispatchAbortTimeout(loop, controller.signal, state)
	}, state.release)
	if err != nil {
		return nil, err
	}
	state.setTimerID(timerID)
	controller.timeoutState = state

	return controller, nil
}

type abortTimeoutDispatchResult struct {
	panicValue any
	panicked   bool
}

func dispatchAbortTimeout(loop *Loop, signal *AbortSignal, state *abortTimeoutState) {
	result := make(chan abortTimeoutDispatchResult, 1)
	go func() {
		workerID := goroutineid.Get()
		ownerID := loop.loopGoroutineID.Swap(workerID)
		outcome := abortTimeoutDispatchResult{}
		defer func() {
			loop.loopGoroutineID.Store(ownerID)
			result <- outcome
		}()
		outcome.panicValue, outcome.panicked = invokeAbortHandler(func(any) {
			dispatch, ok := signal.beginAbort(&TimeoutError{}, state.release)
			state.publish()
			if ok {
				runAbortDispatch(dispatch)
			}
		}, nil)
	}()
	outcome := <-result
	if outcome.panicked {
		panic(outcome.panicValue)
	}
}

type abortTimeoutState struct {
	loop        *Loop
	published   chan struct{}
	timerID     TimerID
	mu          sync.Mutex
	publishOnce sync.Once
	winner      atomic.Uint32
}

const (
	abortTimeoutPending uint32 = iota
	abortTimeoutManual
	abortTimeoutTimer
)

func (s *abortTimeoutState) setTimerID(timerID TimerID) {
	s.mu.Lock()
	s.timerID = timerID
	s.mu.Unlock()
}

func (s *abortTimeoutState) claimManual() bool {
	if !s.winner.CompareAndSwap(abortTimeoutPending, abortTimeoutManual) {
		return false
	}
	s.mu.Lock()
	loop := s.loop
	s.mu.Unlock()
	if loop != nil && loop.testHooks != nil && loop.testHooks.AfterAbortTimeoutManualClaim != nil {
		loop.testHooks.AfterAbortTimeoutManualClaim()
	}
	return true
}

func (s *abortTimeoutState) claimTimeout() bool {
	s.mu.Lock()
	loop := s.loop
	s.mu.Unlock()
	if loop != nil && loop.testHooks != nil && loop.testHooks.BeforeAbortTimeoutClaim != nil {
		loop.testHooks.BeforeAbortTimeoutClaim()
	}
	return s.winner.CompareAndSwap(abortTimeoutPending, abortTimeoutTimer)
}

func (s *abortTimeoutState) publish() {
	s.publishOnce.Do(func() { close(s.published) })
}

func (s *abortTimeoutState) waitPublished() {
	s.mu.Lock()
	loop := s.loop
	s.mu.Unlock()
	if loop != nil && loop.testHooks != nil && loop.testHooks.BeforeAbortTimeoutPublicationWait != nil {
		loop.testHooks.BeforeAbortTimeoutPublicationWait()
	}
	<-s.published
}

func (s *abortTimeoutState) cancel() {
	s.mu.Lock()
	loop := s.loop
	timerID := s.timerID
	s.loop = nil
	s.mu.Unlock()
	if loop != nil {
		_ = loop.queueTimerCancel(timerID)
	}
}

func (s *abortTimeoutState) release() {
	s.mu.Lock()
	s.loop = nil
	s.mu.Unlock()
}

// AbortAny creates a composite AbortSignal that aborts when any input aborts.
// The first observed source reason is preserved exactly.
//
// If any input signal is already aborted, the returned signal will be immediately
// aborted with that signal's reason.
//
// Nil inputs are ignored and duplicate signal pointers are monitored once. An
// empty or all-nil input produces a signal that never aborts. Settlement removes
// every internal source propagation link before composite callbacks run, so
// pending sources do not retain the settled composite. Once an unsettled
// composite becomes unreachable, runtime cleanup detaches those links.
// Source-to-composite propagation is claimed as part of source settlement,
// before source user callbacks run.
//
// Thread Safety:
// AbortAny is safe to call from any goroutine. The returned signal is safe
// for concurrent access.
//
// Example:
//
//	controller1 := eventloop.NewAbortController()
//	controller2 := eventloop.NewAbortController()
//
//	combined := eventloop.AbortAny([]*eventloop.AbortSignal{
//	    controller1.Signal(),
//	    controller2.Signal(),
//	})
//
//	// combined.Aborted() becomes true when EITHER controller aborts
//	controller1.Abort("reason 1") // combined now aborted with "reason 1"
func AbortAny(signals []*AbortSignal) *AbortSignal {
	composite := newAbortSignal()
	if len(signals) == 0 {
		return composite
	}

	state := newAbortAnyState(composite)
	seen := make(map[*AbortSignal]struct{}, len(signals))
	for _, sig := range signals {
		if sig == nil {
			continue
		}
		if _, ok := seen[sig]; ok {
			continue
		}
		seen[sig] = struct{}{}

		remove := sig.addAbortAlgorithm(state.claim)
		if !state.addRemoval(remove) {
			break
		}
	}

	if state.cleanupNeeded() {
		runtime.AddCleanup(composite, cleanupAbortAnyState, state)
	}
	runtime.KeepAlive(composite)
	return composite
}

type abortAnyState struct {
	signal   weak.Pointer[AbortSignal]
	removals []func()
	mu       sync.Mutex
	settled  bool
}

func newAbortAnyState(signal *AbortSignal) *abortAnyState {
	return &abortAnyState{signal: weak.Make(signal)}
}

func (s *abortAnyState) addRemoval(remove func()) bool {
	s.mu.Lock()
	if s.settled {
		s.mu.Unlock()
		if remove != nil {
			remove()
		}
		return false
	}
	if remove != nil {
		s.removals = append(s.removals, remove)
	}
	s.mu.Unlock()
	return true
}

func (s *abortAnyState) cleanupNeeded() bool {
	s.mu.Lock()
	needed := !s.settled && len(s.removals) != 0
	s.mu.Unlock()
	return needed
}

func cleanupAbortAnyState(state *abortAnyState) {
	state.abandon()
}

func (s *abortAnyState) abandon() {
	s.mu.Lock()
	if s.settled {
		s.mu.Unlock()
		return
	}
	s.settled = true
	removals := s.removals
	s.removals = nil
	s.mu.Unlock()

	runAbortRemovals(removals)
}

func (s *abortAnyState) claim(reason any) *abortDispatch {
	s.mu.Lock()
	if s.settled {
		s.mu.Unlock()
		return nil
	}
	s.settled = true
	signal := s.signal.Value()
	removals := s.removals
	s.removals = nil
	s.mu.Unlock()

	if signal == nil {
		runAbortRemovals(removals)
		return nil
	}
	dispatch, ok := signal.beginAbort(reason, func() { runAbortRemovals(removals) })
	runtime.KeepAlive(signal)
	if !ok {
		runAbortRemovals(removals)
		return nil
	}
	return dispatch
}

func runAbortRemovals(removals []func()) {
	for i, remove := range removals {
		remove()
		removals[i] = nil
	}
}
