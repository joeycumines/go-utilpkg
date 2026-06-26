package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestFIFO_ConcurrentResolveAndThen verifies that when a promise is resolved
// from outside the loop goroutine, a concurrent Then() call cannot schedule
// its handler before pre-existing handlers (Promise/A+ §2.2.6).
//
// This test FAILS on the original code (observable violations under
// concurrent load) and PASSES on the fixed code (0% violations). It is
// the regression test for the FIFO fix.
func TestFIFO_ConcurrentResolveAndThen(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx)
	waitForRunning(t, loop)
	defer loop.Shutdown(context.Background())

	const numIterations = 50000
	const numPreHandlers = 50
	const numToChannelSubs = 100

	var violations atomic.Int64

	for iter := range numIterations {
		p, resolve, _ := js.NewChainedPromise()

		var orderMu sync.Mutex
		var execOrder []int
		handlerDone := make(chan struct{}, numPreHandlers+1)

		for i := range numPreHandlers {
			idx := i
			p.Then(func(v any) any {
				orderMu.Lock()
				execOrder = append(execOrder, idx)
				orderMu.Unlock()
				handlerDone <- struct{}{}
				return nil
			}, nil)
		}

		for range numToChannelSubs {
			p.ToChannel()
		}

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			resolve("value")
		}()

		go func() {
			defer wg.Done()
			p.Then(func(v any) any {
				orderMu.Lock()
				execOrder = append(execOrder, numPreHandlers)
				orderMu.Unlock()
				handlerDone <- struct{}{}
				return nil
			}, nil)
		}()

		wg.Wait()

		for range numPreHandlers + 1 {
			select {
			case <-handlerDone:
			case <-time.After(2 * time.Second):
				t.Fatalf("iter %d: timeout waiting for handlers", iter)
			}
		}

		orderMu.Lock()
		newPos := -1
		firstPos := -1
		for i, v := range execOrder {
			if v == numPreHandlers && newPos == -1 {
				newPos = i
			}
			if v == 0 && firstPos == -1 {
				firstPos = i
			}
		}
		orderMu.Unlock()

		if newPos != -1 && firstPos != -1 && newPos < firstPos {
			violations.Add(1)
		}
	}

	if v := violations.Load(); v > 0 {
		t.Fatalf("FIFO violations: %d/%d iterations. "+
			"Handler attached concurrently with resolve ran before pre-existing handlers.",
			v, numIterations)
	}
}

// TestFIFO_StandaloneNoDeadlock guards against a regression where the FIFO
// fix (scheduling handlers before state.Store) is applied to standalone
// promises (p.js == nil). For standalone promises, scheduleHandler runs
// executeHandler synchronously inside resolve()'s held lock. If a handler
// calls Then() on the same promise, addHandler would see Pending and block
// on p.mu → deadlock.
//
// This test passes on both original and fixed code. Its value is catching
// a future refactor that removes the p.js branch from resolve().
func TestFIFO_StandaloneNoDeadlock(t *testing.T) {
	p := &ChainedPromise{}
	p.state.Store(int32(Pending))

	p.Then(func(v any) any {
		p.Then(func(v any) any { return v }, nil)
		return nil
	}, nil)

	done := make(chan struct{})
	go func() {
		defer close(done)
		p.resolve(42)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("DEADLOCK: resolve() blocked because handler called Then() on same standalone promise")
	}
}

// TestFIFO_ConcurrentRejectAndThen verifies that when a promise is rejected
// from outside the loop goroutine, a concurrent Then(nil, handler) call
// cannot schedule its rejection handler before pre-existing rejection
// handlers (Promise/A+ §2.2.6). This mirrors TestFIFO_ConcurrentResolveAndThen
// but exercises the reject() path, which uses the same
// schedule-before-state.Store pattern for JS-backed promises.
func TestFIFO_ConcurrentRejectAndThen(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) {}))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx)
	waitForRunning(t, loop)
	defer loop.Shutdown(context.Background())

	const numIterations = 5000
	const numPreHandlers = 50

	var violations atomic.Int64

	for iter := range numIterations {
		p, _, reject := js.NewChainedPromise()

		var orderMu sync.Mutex
		var execOrder []int
		handlerDone := make(chan struct{}, numPreHandlers+1)

		for i := range numPreHandlers {
			idx := i
			p.Then(nil, func(v any) any {
				orderMu.Lock()
				execOrder = append(execOrder, idx)
				orderMu.Unlock()
				handlerDone <- struct{}{}
				return nil
			})
		}

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			reject("error")
		}()

		go func() {
			defer wg.Done()
			p.Then(nil, func(v any) any {
				orderMu.Lock()
				execOrder = append(execOrder, numPreHandlers)
				orderMu.Unlock()
				handlerDone <- struct{}{}
				return nil
			})
		}()

		wg.Wait()

		for range numPreHandlers + 1 {
			select {
			case <-handlerDone:
			case <-time.After(2 * time.Second):
				t.Fatalf("iter %d: timeout waiting for rejection handlers", iter)
			}
		}

		orderMu.Lock()
		newPos := -1
		firstPos := -1
		for i, v := range execOrder {
			if v == numPreHandlers && newPos == -1 {
				newPos = i
			}
			if v == 0 && firstPos == -1 {
				firstPos = i
			}
		}
		orderMu.Unlock()

		if newPos != -1 && firstPos != -1 && newPos < firstPos {
			violations.Add(1)
		}
	}

	if v := violations.Load(); v > 0 {
		t.Fatalf("FIFO violations: %d/%d iterations. "+
			"Rejection handler attached concurrently with reject ran before pre-existing rejection handlers.",
			v, numIterations)
	}
}

// TestFIFO_OffLoopResolveOrdering verifies that resolving a JS-backed promise
// from an EXTERNAL goroutine (not the loop thread) preserves FIFO handler order:
// all N pre-attached handlers run in attach order 0..N-1, none dropped/reordered.
//
// This is ORTHOGONAL coverage to the Promise/A+ §2.2.6 concurrent-Then race,
// which is guarded by TestFIFO_ConcurrentResolveAndThen (the schedule-before-
// state.Store fix). Pre-attached handlers are scheduled in attach order
// regardless of the store-vs-schedule ordering (this test passes even if that
// fix is reverted), so it does NOT discriminate that fix. It only verifies the
// off-loop resolution path drains the pre-attached handlers in attach order.
func TestFIFO_OffLoopResolveOrdering(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx)
	waitForRunning(t, loop)
	defer loop.Shutdown(context.Background())

	const numHandlers = 64
	p, resolve, _ := js.NewChainedPromise()

	var mu sync.Mutex
	var order []int
	var count atomic.Int64
	done := make(chan struct{})

	for i := range numHandlers {
		idx := i
		p.Then(func(any) any {
			mu.Lock()
			order = append(order, idx)
			mu.Unlock()
			if count.Add(1) == int64(numHandlers) {
				close(done)
			}
			return nil
		}, nil)
	}

	// Resolve from an external goroutine (off-loop).
	go func() { resolve("value") }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		mu.Lock()
		snapshot := append([]int(nil), order...)
		mu.Unlock()
		t.Fatalf("Timeout: only %d/%d handlers ran. Order: %v", len(snapshot), numHandlers, snapshot)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(order) != numHandlers {
		t.Fatalf("Expected %d handlers, got %d: %v", numHandlers, len(order), order)
	}
	for i, v := range order {
		if v != i {
			t.Fatalf("FIFO violation (off-loop resolve): order[%d]=%d, expected %d. Full order: %v",
				i, v, i, order)
		}
	}
}

// TestFIFO_HandlerThenOnSamePromiseDuringResolve verifies that a handler which
// re-enters Then() on the SAME promise during its own execution does not
// deadlock and that the late-attached handler still runs. When handler-1 calls
// p.Then, addHandler either takes the optimistic settled-state fast path (if
// resolve() has already published state) or briefly waits on p.mu until resolve()
// finishes publishing — neither case deadlocks, and handler-2 is scheduled as a
// microtask that runs afterward.
func TestFIFO_HandlerThenOnSamePromiseDuringResolve(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx)
	waitForRunning(t, loop)
	defer loop.Shutdown(context.Background())

	p, resolve, _ := js.NewChainedPromise()

	var mu sync.Mutex
	var order []string
	done := make(chan struct{})

	appendOrder := func(s string) {
		mu.Lock()
		order = append(order, s)
		mu.Unlock()
	}

	// Pre-attached handler re-enters Then on the same promise once it runs.
	p.Then(func(any) any {
		appendOrder("handler-1")
		// p is settled now; this schedules handler-2 as a microtask.
		p.Then(func(any) any {
			appendOrder("handler-2-from-handler-1")
			close(done)
			return nil
		}, nil)
		return nil
	}, nil)

	go func() { resolve("value") }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		mu.Lock()
		snapshot := append([]string(nil), order...)
		mu.Unlock()
		t.Fatalf("Timeout / deadlock: order: %v", snapshot)
	}

	mu.Lock()
	defer mu.Unlock()

	expected := []string{"handler-1", "handler-2-from-handler-1"}
	if len(order) != len(expected) {
		t.Fatalf("Expected %d events, got %d: %v", len(expected), len(order), order)
	}
	for i, ev := range expected {
		if order[i] != ev {
			t.Errorf("order[%d]: expected %q, got %q", i, ev, order[i])
		}
	}
}

// TestToChannel_OrderingVsHandlers verifies that a ToChannel subscriber and
// promise handlers both receive the resolution value with no deadlock or hang,
// covering review-02 §4.3. notifyToChannels runs synchronously under p.mu inside
// resolve/reject; handler microtasks are queued before notifyToChannels, so the
// relative order of handler execution vs the channel send is not guaranteed.
// This test only asserts that both the channel and the handler observe the value
// (not any ordering between them).
func TestToChannel_OrderingVsHandlers(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx)
	waitForRunning(t, loop)
	defer loop.Shutdown(context.Background())

	p, resolve, _ := js.NewChainedPromise()

	// Subscribe BEFORE resolving (exercises the side-table registration path).
	ch := p.ToChannel()

	var mu sync.Mutex
	var handlerVal any
	done := make(chan struct{})

	// Attach a handler that records the value and signals completion.
	p.Then(func(v any) any {
		mu.Lock()
		handlerVal = v
		mu.Unlock()
		close(done)
		return nil
	}, nil)

	go func() { resolve("the-value") }()

	// ToChannel is buffered (cap 1) so this receive cannot deadlock; it returns
	// the value pushed by notifyToChannels inside resolve.
	var channelVal any
	select {
	case channelVal = <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for ToChannel value")
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for handler to run")
	}

	mu.Lock()
	hv := handlerVal
	mu.Unlock()

	if channelVal != "the-value" {
		t.Errorf("ToChannel value: expected %q, got %v", "the-value", channelVal)
	}
	if hv != "the-value" {
		t.Errorf("handler value: expected %q, got %v", "the-value", hv)
	}
}
