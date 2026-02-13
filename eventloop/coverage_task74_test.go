package eventloop

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// ===========================================================================
// PerformanceObserver — Buffered observe with pre-existing entries
// ===========================================================================

func Test_performanceObserver_BufferedWithEntries(t *testing.T) {
	perf := NewPerformance()

	// Create entries BEFORE observing
	perf.Mark("buffered-mark-1")
	perf.Mark("buffered-mark-2")

	var observedEntries []PerformanceEntry
	observer := newPerformanceObserver(perf, func(entries []PerformanceEntry, obs *performanceObserver) {
		observedEntries = append(observedEntries, entries...)
	})

	// Observe with Buffered=true should deliver existing "mark" entries
	observer.Observe(performanceObserverOptions{
		EntryTypes: []string{"mark"},
		Buffered:   true,
	})

	if len(observedEntries) != 2 {
		t.Errorf("Expected 2 buffered entries, got %d", len(observedEntries))
	}
	for _, e := range observedEntries {
		if e.EntryType != "mark" {
			t.Errorf("Expected mark entry, got %q", e.EntryType)
		}
	}
}

func Test_performanceObserver_BufferedFiltersMismatch(t *testing.T) {
	perf := NewPerformance()

	// Create mark entries
	perf.Mark("only-marks")

	var observedEntries []PerformanceEntry
	observer := newPerformanceObserver(perf, func(entries []PerformanceEntry, obs *performanceObserver) {
		observedEntries = append(observedEntries, entries...)
	})

	// Observe with Buffered=true for "measure" should NOT deliver "mark" entries
	observer.Observe(performanceObserverOptions{
		EntryTypes: []string{"measure"},
		Buffered:   true,
	})

	if len(observedEntries) != 0 {
		t.Errorf("Expected 0 entries (type mismatch), got %d", len(observedEntries))
	}
}

// ===========================================================================
// Standalone promise — ToChannel on standalone (no JS)
// ===========================================================================

func Test_ChainedPromise_ToChannel_Standalone(t *testing.T) {
	// Create standalone promise with no JS adapter
	p := &ChainedPromise{}
	p.state.Store(int32(Pending))

	go func() {
		p.resolve("hello")
	}()

	ch := p.ToChannel()
	result := <-ch
	if result != "hello" {
		t.Errorf("Expected 'hello', got %v", result)
	}
}

func Test_ChainedPromise_ToChannel_StandaloneSettled(t *testing.T) {
	// Create and immediately resolve
	p := &ChainedPromise{}
	p.state.Store(int32(Pending))
	p.resolve("already-done")

	// ToChannel on already-settled promise
	ch := p.ToChannel()
	result := <-ch
	if result != "already-done" {
		t.Errorf("Expected 'already-done', got %v", result)
	}
}

// ===========================================================================
// Promise — Resolve on already-settled (double resolve)
// ===========================================================================

func Test_ChainedPromise_DoubleResolve(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	t.Cleanup(func() { loop.Close() })

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS failed: %v", err)
	}

	p, resolve, reject := js.NewChainedPromise()

	resolve("first")
	resolve("second") // Should be no-op
	reject(nil)       // Should be no-op

	ch := p.ToChannel()
	result := <-ch
	if result != "first" {
		t.Errorf("Expected 'first', got %v", result)
	}
}

// ===========================================================================
// NewJS with nil option (resolveJSOptions nil skip path)
// ===========================================================================

func Test_NewJS_NilOption(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	t.Cleanup(func() { loop.Close() })

	// Passing nil option should be gracefully handled
	js, err := NewJS(loop, nil)
	if err != nil {
		t.Fatalf("NewJS with nil option failed: %v", err)
	}
	if js == nil {
		t.Error("Expected non-nil JS")
	}
}

// ===========================================================================
// QueueMetrics — UpdateInternal and UpdateMicrotask first-call path
// ===========================================================================

func Test_QueueMetrics_UpdateInternal_FirstCall(t *testing.T) {
	q := &QueueMetrics{}

	// First call initializes EMA
	q.UpdateInternal(10)
	if q.InternalCurrent != 10 {
		t.Errorf("Expected 10, got %d", q.InternalCurrent)
	}
	if q.InternalMax != 10 {
		t.Errorf("Expected max 10, got %d", q.InternalMax)
	}

	// Second call uses EMA formula
	q.UpdateInternal(20)
	if q.InternalMax != 20 {
		t.Errorf("Expected max 20, got %d", q.InternalMax)
	}
}

func Test_QueueMetrics_UpdateMicrotask_FirstCall(t *testing.T) {
	q := &QueueMetrics{}

	// First call initializes EMA
	q.UpdateMicrotask(5)
	if q.MicrotaskCurrent != 5 {
		t.Errorf("Expected 5, got %d", q.MicrotaskCurrent)
	}

	// Second call uses EMA formula
	q.UpdateMicrotask(15)
	if q.MicrotaskMax != 15 {
		t.Errorf("Expected max 15, got %d", q.MicrotaskMax)
	}
}

// ===========================================================================
// promise.Reject on already-settled (covers promise.go:118 early return)
// ===========================================================================

func Test_promise_RejectAlreadyResolved(t *testing.T) {
	p := &promise{state: Pending}
	p.Resolve("done")

	// Reject on already-resolved should be no-op
	p.Reject(errors.New("late error"))

	if p.state != Fulfilled {
		t.Errorf("Expected Fulfilled, got %v", p.state)
	}
	if p.result != "done" {
		t.Errorf("Expected 'done', got %v", p.result)
	}
}

func Test_promise_DoubleReject(t *testing.T) {
	p := &promise{state: Pending}
	p.Reject(errors.New("first"))

	// Second reject should be no-op
	p.Reject(errors.New("second"))

	if p.state != Rejected {
		t.Errorf("Expected Rejected, got %v", p.state)
	}
}

// ===========================================================================
// promise.fanOut channel full (covers promise.go:133 default branch)
// ===========================================================================

func Test_promise_FanOut_ChannelFull(t *testing.T) {
	p := &promise{state: Pending}

	// Subscribe and pre-fill the channel before resolve
	ch := p.ToChannel()

	// Now fill the subscriber channel from the outside (capacity 1 already taken by subscriber)
	// Actually, ToChannel returns a chan with cap=1, and subscriber is stored.
	// The channel isn't filled until Resolve is called. Let's do a different approach:
	// We manually add a subscriber that's already full.
	p.mu.Lock()
	fullCh := make(chan any, 1)
	fullCh <- "blocker" // Pre-fill the channel
	p.subscribers = append(p.subscribers, fullCh)
	p.mu.Unlock()

	// Resolve will try to send to fullCh (already full) -> triggers default branch
	p.Resolve("value")

	// The first subscriber (ch) should get the value
	result := <-ch
	if result != "value" {
		t.Errorf("Expected 'value', got %v", result)
	}

	// The full channel should still have the blocker, not the resolve value
	blocker := <-fullCh
	if blocker != "blocker" {
		t.Errorf("Expected 'blocker', got %v", blocker)
	}
}

// ===========================================================================
// ChainedPromise.addHandler on already-settled (fast path, no lock)
// ===========================================================================

func Test_ChainedPromise_ThenOnSettled(t *testing.T) {
	p := &ChainedPromise{}
	p.state.Store(int32(Pending))
	p.resolve("settled-value")

	// Then on already-settled triggers addHandler fast path
	var gotValue any
	var wg sync.WaitGroup
	wg.Add(1)

	child := p.thenStandalone(func(v any) any {
		gotValue = v
		wg.Done()
		return v
	}, nil)

	wg.Wait()
	if gotValue != "settled-value" {
		t.Errorf("Expected 'settled-value', got %v", gotValue)
	}
	if child == nil {
		t.Error("Expected non-nil child")
	}
}

func Test_ChainedPromise_CatchOnFulfilled(t *testing.T) {
	p := &ChainedPromise{}
	p.state.Store(int32(Pending))
	p.resolve("ok")

	// Catch on fulfilled promise: onRejected is not called, value passes through
	child := p.thenStandalone(nil, func(r any) any {
		t.Error("Should not be called on fulfilled promise")
		return nil
	})

	ch := child.ToChannel()
	result := <-ch
	if result != "ok" {
		t.Errorf("Expected 'ok' pass-through, got %v", result)
	}
}

// ===========================================================================
// ChainedPromise.Finally standalone (covers promise.go lines 714+)
// ===========================================================================

func Test_ChainedPromise_FinallyStandalone_Fulfilled(t *testing.T) {
	p := &ChainedPromise{}
	p.state.Store(int32(Pending))

	finallyCalled := false
	child := p.Finally(func() {
		finallyCalled = true
	})

	p.resolve("val")

	ch := child.ToChannel()
	result := <-ch
	if !finallyCalled {
		t.Error("Expected finally to be called")
	}
	if result != "val" {
		t.Errorf("Expected 'val', got %v", result)
	}
}

func Test_ChainedPromise_FinallyStandalone_Rejected(t *testing.T) {
	p := &ChainedPromise{}
	p.state.Store(int32(Pending))

	finallyCalled := false
	child := p.Finally(func() {
		finallyCalled = true
	})

	p.reject("err")

	ch := child.ToChannel()
	result := <-ch
	if !finallyCalled {
		t.Error("Expected finally to be called")
	}
	if result != "err" {
		t.Errorf("Expected 'err', got %v", result)
	}
}

func Test_ChainedPromise_FinallyNilCallback(t *testing.T) {
	p := &ChainedPromise{}
	p.state.Store(int32(Pending))

	// nil callback should be replaced with no-op
	child := p.Finally(nil)
	p.resolve("ok")

	ch := child.ToChannel()
	result := <-ch
	if result != "ok" {
		t.Errorf("Expected 'ok', got %v", result)
	}
}

func Test_ChainedPromise_FinallyPanic(t *testing.T) {
	p := &ChainedPromise{}
	p.state.Store(int32(Pending))

	child := p.Finally(func() {
		panic("cleanup panic")
	})

	p.resolve("original")

	ch := child.ToChannel()
	result := <-ch
	// Even when finally panics, original value should propagate
	if result != "original" {
		t.Errorf("Expected 'original' after panic, got %v", result)
	}
}

// ===========================================================================
// pSquareQuantile edge cases (covers psquare.go lines 188-195)
// ===========================================================================

func Test_pSquareQuantile_ZeroObservations(t *testing.T) {
	ps := newPSquareQuantile(0.5)
	if q := ps.Quantile(); q != 0 {
		t.Errorf("Expected 0 for no observations, got %f", q)
	}
}

func Test_pSquareQuantile_FewObservations(t *testing.T) {
	ps := newPSquareQuantile(0.5)

	// 1 observation
	ps.Update(10)
	if q := ps.Quantile(); q != 10 {
		t.Errorf("Expected 10 with 1 obs, got %f", q)
	}

	// 2 observations
	ps.Update(20)
	q := ps.Quantile()
	if q < 10 || q > 20 {
		t.Errorf("Expected between 10-20 with 2 obs, got %f", q)
	}

	// 3 observations
	ps.Update(30)
	q = ps.Quantile()
	if q < 10 || q > 30 {
		t.Errorf("Expected between 10-30 with 3 obs, got %f", q)
	}

	// 4 observations (still < 5, uses init buffer)
	ps.Update(40)
	q = ps.Quantile()
	if q < 10 || q > 40 {
		t.Errorf("Expected between 10-40 with 4 obs, got %f", q)
	}
}

func Test_pSquareQuantile_P99_FewObs(t *testing.T) {
	ps := newPSquareQuantile(0.99)

	// With extreme quantile and few observations
	ps.Update(1)
	ps.Update(100)
	ps.Update(50)
	q := ps.Quantile()
	// index = int(float64(2) * 0.99) = int(1.98) = 1 => sorted[1]
	// sorted = [1, 50, 100], index=1 => 50
	if q != 50 {
		t.Errorf("Expected 50 for p99 with 3 obs, got %f", q)
	}
}

// ===========================================================================
// resolveJSOptions error path (covers js.go:53)
// ===========================================================================

func Test_resolveJSOptions_ErrorOption(t *testing.T) {
	errOption := &jsOptionImpl{
		applyJSOptionFunc: func(o *jsOptions) error {
			return errors.New("option error")
		},
	}

	_, err := resolveJSOptions([]JSOption{errOption})
	if err == nil {
		t.Error("Expected error from option")
	}
	if err.Error() != "option error" {
		t.Errorf("Expected 'option error', got %v", err)
	}
}

// ===========================================================================
// CancelTimer not found (covers loop.go:1737)
// ===========================================================================

func Test_CancelTimer_NotFound(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = loop.Run(ctx) }()
	t.Cleanup(func() { cancel(); loop.Close() })

	errCh := make(chan error, 1)
	if err := loop.SubmitInternal(func() {
		cancelErr := loop.CancelTimer(TimerID(99999))
		errCh <- cancelErr
	}); err != nil {
		t.Fatalf("SubmitInternal failed: %v", err)
	}

	cancelErr := <-errCh
	if !errors.Is(cancelErr, ErrTimerNotFound) {
		t.Errorf("Expected ErrTimerNotFound, got %v", cancelErr)
	}
}

// ===========================================================================
// CancelTimers with mix of valid/invalid (covers loop.go:1813-1819)
// ===========================================================================

func Test_CancelTimers_MixedIDs(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = loop.Run(ctx) }()
	t.Cleanup(func() { cancel(); loop.Close() })

	type result struct {
		errs []error
	}

	resultCh := make(chan result, 1)
	if err := loop.SubmitInternal(func() {
		// Schedule two timers
		id1, _ := loop.ScheduleTimer(time.Hour, func() {})
		id2, _ := loop.ScheduleTimer(time.Hour, func() {})

		// Cancel with mix: valid, invalid, valid
		errs := loop.CancelTimers([]TimerID{id1, TimerID(99999), id2})
		resultCh <- result{errs: errs}
	}); err != nil {
		t.Fatalf("SubmitInternal failed: %v", err)
	}

	r := <-resultCh
	if len(r.errs) != 3 {
		t.Fatalf("Expected 3 errors, got %d", len(r.errs))
	}
	if r.errs[0] != nil {
		t.Errorf("Expected nil for id1, got %v", r.errs[0])
	}
	if !errors.Is(r.errs[1], ErrTimerNotFound) {
		t.Errorf("Expected ErrTimerNotFound for invalid, got %v", r.errs[1])
	}
	if r.errs[2] != nil {
		t.Errorf("Expected nil for id2, got %v", r.errs[2])
	}
}

// ===========================================================================
// CancelTimers empty (covers loop.go:1762 early return)
// ===========================================================================

func Test_CancelTimers_Empty(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	t.Cleanup(func() { loop.Close() })

	errs := loop.CancelTimers(nil)
	if errs != nil {
		t.Errorf("Expected nil for empty CancelTimers, got %v", errs)
	}
}

// ===========================================================================
// CancelTimer/CancelTimers on non-running loop (covers loop.go:1722/1767)
// ===========================================================================

func Test_CancelTimer_NotRunning(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	// Close immediately, don't start
	loop.Close()

	cancelErr := loop.CancelTimer(TimerID(1))
	if cancelErr == nil {
		t.Error("Expected error on non-running loop")
	}
}

func Test_CancelTimers_NotRunning(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	loop.Close()

	errs := loop.CancelTimers([]TimerID{TimerID(1), TimerID(2)})
	if len(errs) != 2 {
		t.Fatalf("Expected 2 errors, got %d", len(errs))
	}
	for i, e := range errs {
		if e == nil {
			t.Errorf("Expected error at index %d, got nil", i)
		}
	}
}

// ===========================================================================
// safeExecuteFn nil (covers loop.go:1858)
// ===========================================================================

func Test_safeExecuteFn_Nil(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	t.Cleanup(func() { loop.Close() })

	// Should not panic
	loop.safeExecuteFn(nil)
}

// ===========================================================================
// safeExecuteFn panic recovery (covers loop.go:1865-1867)
// ===========================================================================

func Test_safeExecuteFn_PanicRecovery(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	t.Cleanup(func() { loop.Close() })

	// Should recover and not propagate panic
	loop.safeExecuteFn(func() {
		panic("test panic")
	})
}

// ===========================================================================
// safeExecute nil (covers loop.go:1828 nil check)
// ===========================================================================

func Test_safeExecute_Nil(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	t.Cleanup(func() { loop.Close() })

	// Should not panic
	loop.safeExecute(nil)
}

// ===========================================================================
// ChainedPromise addHandler with multiple handlers (covers h0 + additional slice)
// ===========================================================================

func Test_ChainedPromise_MultipleHandlers(t *testing.T) {
	p := &ChainedPromise{}
	p.state.Store(int32(Pending))

	var results []any
	var mu sync.Mutex

	// Add 3 handlers while still pending -> fills h0 slot + additional handlers slice
	for i := 0; i < 3; i++ {
		val := i
		p.thenStandalone(func(v any) any {
			mu.Lock()
			results = append(results, val)
			mu.Unlock()
			return v
		}, nil)
	}

	p.resolve("multi")

	// Wait for handlers to propagate (standalone is synchronous)
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(results) != 3 {
		t.Errorf("Expected 3 handler calls, got %d", len(results))
	}
}

// ===========================================================================
// promise.ToChannel on already-settled (covers promise.go:88-92)
// ===========================================================================

func Test_promise_ToChannel_AlreadySettled(t *testing.T) {
	p := &promise{state: Pending}
	p.Resolve("done")

	ch := p.ToChannel()
	result := <-ch
	if result != "done" {
		t.Errorf("Expected 'done', got %v", result)
	}
}

// ===========================================================================
// ErrTimerNotFound and ErrLoopNotRunning type checks
// ===========================================================================

func Test_ErrorValues(t *testing.T) {
	if ErrTimerNotFound == nil {
		t.Error("ErrTimerNotFound should not be nil")
	}
	if ErrLoopNotRunning == nil {
		t.Error("ErrLoopNotRunning should not be nil")
	}
	if ErrLoopTerminated == nil {
		t.Error("ErrLoopTerminated should not be nil")
	}
}

// ===========================================================================
// runTimers with canceled timer (covers loop.go:1633-1640)
// ===========================================================================

func Test_runTimers_CanceledTimer(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = loop.Run(ctx) }()
	t.Cleanup(func() { cancel(); loop.Close() })

	done := make(chan struct{})
	if err := loop.SubmitInternal(func() {
		// Schedule timer with very short delay
		id, err := loop.ScheduleTimer(time.Millisecond, func() {
			t.Error("Canceled timer should not fire")
		})
		if err != nil {
			t.Errorf("ScheduleTimer failed: %v", err)
			close(done)
			return
		}

		// Cancel it
		if err := loop.CancelTimer(id); err != nil {
			t.Errorf("CancelTimer failed: %v", err)
		}

		// Schedule another timer that WILL fire to prove loop continues
		_, err = loop.ScheduleTimer(50*time.Millisecond, func() {
			close(done)
		})
		if err != nil {
			t.Errorf("ScheduleTimer 2 failed: %v", err)
			close(done)
		}
	}); err != nil {
		t.Fatalf("SubmitInternal failed: %v", err)
	}

	select {
	case <-done:
		// OK
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for timer")
	}
}

// ===========================================================================
// executeHandler — panic in handler propagates to target as PanicError
// ===========================================================================

func Test_ChainedPromise_HandlerPanic(t *testing.T) {
	p := &ChainedPromise{}
	p.state.Store(int32(Pending))

	child := p.thenStandalone(func(v any) any {
		panic("handler panic")
	}, nil)

	p.resolve("trigger")

	ch := child.ToChannel()
	result := <-ch

	// The child should be rejected with a PanicError
	if pe, ok := result.(PanicError); ok {
		if pe.Value != "handler panic" {
			t.Errorf("Expected panic value 'handler panic', got %v", pe.Value)
		}
	} else {
		t.Errorf("Expected PanicError, got %T: %v", result, result)
	}
}

// ===========================================================================
// executeHandler — rejected promise with no onRejected (pass-through)
// ===========================================================================

func Test_ChainedPromise_RejectPassThrough(t *testing.T) {
	p := &ChainedPromise{}
	p.state.Store(int32(Pending))

	// Then with only onFulfilled — rejection should pass through
	child := p.thenStandalone(func(v any) any {
		t.Error("Should not be called")
		return nil
	}, nil)

	p.reject("some-error")

	ch := child.ToChannel()
	result := <-ch
	if result != "some-error" {
		t.Errorf("Expected 'some-error' pass-through, got %v", result)
	}
}

// ===========================================================================
// ChainedPromise — resolve with another promise (spec 2.3.2 adoption)
// ===========================================================================

func Test_ChainedPromise_ResolveWithPromise(t *testing.T) {
	p1 := &ChainedPromise{}
	p1.state.Store(int32(Pending))

	p2 := &ChainedPromise{}
	p2.state.Store(int32(Pending))

	// Resolve p1 with p2 -> p1 should adopt p2's state
	p1.resolve(p2)

	// p2 resolves later
	p2.resolve("adopted-value")

	ch := p1.ToChannel()
	result := <-ch
	if result != "adopted-value" {
		t.Errorf("Expected 'adopted-value', got %v", result)
	}
}

// ===========================================================================
// ChainedPromise — resolve with self (TypeError cycle detection, spec 2.3.1)
// ===========================================================================

func Test_ChainedPromise_ResolveWithSelf(t *testing.T) {
	p := &ChainedPromise{}
	p.state.Store(int32(Pending))

	// Resolving with self should reject with TypeError
	p.resolve(p)

	ch := p.ToChannel()
	result := <-ch
	if err, ok := result.(error); ok {
		if err == nil || len(err.Error()) == 0 {
			t.Error("Expected non-empty TypeError")
		}
	} else {
		t.Errorf("Expected error result, got %T: %v", result, result)
	}
}

// ===========================================================================
// chunkedIngress — edge cases
// ===========================================================================

func Test_chunkedIngress_PopEmpty(t *testing.T) {
	q := newChunkedIngressWithSize(16)
	fn, ok := q.Pop()
	if ok || fn != nil {
		t.Errorf("Expected empty Pop, got ok=%v fn!=nil:%v", ok, fn != nil)
	}
}

func Test_chunkedIngress_PushPop(t *testing.T) {
	q := newChunkedIngressWithSize(4)

	executed := 0
	// Push more than chunk size to test multi-chunk
	for i := 0; i < 10; i++ {
		q.Push(func() { executed++ })
	}

	if q.Length() != 10 {
		t.Errorf("Expected length 10, got %d", q.Length())
	}

	count := 0
	for {
		fn, ok := q.Pop()
		if !ok {
			break
		}
		fn()
		count++
	}
	if count != 10 {
		t.Errorf("Expected 10 pops, got %d", count)
	}
	if executed != 10 {
		t.Errorf("Expected 10 executions, got %d", executed)
	}
}

func Test_chunkedIngress_PushPopAlternating(t *testing.T) {
	q := newChunkedIngressWithSize(4)

	// Push 2, pop 1, alternating
	for i := 0; i < 20; i++ {
		val := i
		q.Push(func() { _ = val })
		q.Push(func() { _ = val })
		fn, ok := q.Pop()
		if !ok {
			t.Fatalf("Expected Pop at i=%d", i)
		}
		fn()
	}

	// Drain remaining
	count := 0
	for {
		_, ok := q.Pop()
		if !ok {
			break
		}
		count++
	}
	if count != 20 {
		t.Errorf("Expected 20 remaining, got %d", count)
	}
}

// ===========================================================================
// QueueMetrics — Ingress
// ===========================================================================

func Test_QueueMetrics_UpdateIngress(t *testing.T) {
	q := &QueueMetrics{}

	q.UpdateIngress(5)
	if q.IngressCurrent != 5 {
		t.Errorf("Expected 5, got %d", q.IngressCurrent)
	}
	if q.IngressMax != 5 {
		t.Errorf("Expected max 5, got %d", q.IngressMax)
	}

	q.UpdateIngress(10)
	if q.IngressMax != 10 {
		t.Errorf("Expected max 10, got %d", q.IngressMax)
	}
}

// ===========================================================================
// NewPerformance and getEntries
// ===========================================================================

func Test_Performance_GetEntries(t *testing.T) {
	perf := NewPerformance()
	perf.Mark("m1")
	perf.Mark("m2")
	perf.Measure("measure1", "m1", "m2")

	entries := perf.GetEntries()
	if len(entries) < 3 {
		t.Errorf("Expected at least 3 entries, got %d", len(entries))
	}

	// GetEntriesByType
	marks := perf.GetEntriesByType("mark")
	if len(marks) != 2 {
		t.Errorf("Expected 2 marks, got %d", len(marks))
	}

	measures := perf.GetEntriesByType("measure")
	if len(measures) != 1 {
		t.Errorf("Expected 1 measure, got %d", len(measures))
	}

	// GetEntriesByName
	m1 := perf.GetEntriesByName("m1")
	if len(m1) != 1 {
		t.Errorf("Expected 1 entry named 'm1', got %d", len(m1))
	}
}

// ===========================================================================
// Performance ClearMarks / ClearMeasures
// ===========================================================================

func Test_Performance_ClearMarks(t *testing.T) {
	perf := NewPerformance()
	perf.Mark("m1")
	perf.Mark("m2")

	perf.ClearMarks("")
	marks := perf.GetEntriesByType("mark")
	if len(marks) != 0 {
		t.Errorf("Expected 0 marks after clear, got %d", len(marks))
	}
}

func Test_Performance_ClearMeasures(t *testing.T) {
	perf := NewPerformance()
	perf.Mark("start")
	perf.Mark("end")
	perf.Measure("m", "start", "end")

	perf.ClearMeasures("")
	measures := perf.GetEntriesByType("measure")
	if len(measures) != 0 {
		t.Errorf("Expected 0 measures after clear, got %d", len(measures))
	}
}

// ===========================================================================
// Performance timeOrigin
// ===========================================================================

func Test_Performance_TimeOrigin(t *testing.T) {
	perf := NewPerformance()
	to := perf.TimeOrigin()
	if to <= 0 {
		t.Error("Expected positive TimeOrigin")
	}
	now := perf.Now()
	if now < 0 {
		t.Error("Expected non-negative Now()")
	}
}
