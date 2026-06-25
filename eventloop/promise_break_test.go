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
