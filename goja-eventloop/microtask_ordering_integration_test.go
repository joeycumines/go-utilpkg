package gojaeventloop

import (
	"context"
	"strconv"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// TestGojaMicrotaskOrdering_PromiseBetweenTimers verifies Node.js v11+
// microtask ordering semantics: when a promise reaction is scheduled
// inside a timer callback, it runs before the next timer callback fires.
//
// Without per-callback draining, microtasks would be batched after all timers.
func TestGojaMicrotaskOrdering_PromiseBetweenTimers(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	vm := goja.New()
	adapter, err := New(loop, vm)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind adapter: %v", err)
	}

	var order []string
	done := make(chan struct{})

	vm.Set("pushOrder", func(s string) { order = append(order, s) })
	vm.Set("done", func() { close(done) })

	// Run JS BEFORE starting the loop to avoid concurrent goja access.
	// The setTimeout calls schedule timers via the loop, and the promise
	// reactions are queued as microtasks. When loop.Run starts, strict
	// microtask ordering ensures the promise runs between timer callbacks.
	jsCode := `
		setTimeout(function() {
			pushOrder("timer1");
			Promise.resolve().then(function() {
				pushOrder("microtask");
			});
		}, 0);

		setTimeout(function() {
			pushOrder("timer2");
			done();
		}, 0);
	`
	_, err = vm.RunString(jsCode)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() { _ = loop.Run(ctx) }()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("Timeout waiting for done signal")
	}

	expected := []string{"timer1", "microtask", "timer2"}
	if len(order) != len(expected) {
		t.Fatalf("Expected order %v, got %v", expected, order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d]: expected %q, got %q (full order: %v)", i, v, order[i], order)
		}
	}
}

// TestGojaMicrotaskOrdering_QueueMicrotaskBetweenTimers verifies that
// queueMicrotask() called inside a timer callback runs before the next
// timer callback, matching Node.js v11+ semantics.
func TestGojaMicrotaskOrdering_QueueMicrotaskBetweenTimers(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	vm := goja.New()
	adapter, err := New(loop, vm)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind adapter: %v", err)
	}

	var order []string
	done := make(chan struct{})

	vm.Set("pushOrder", func(s string) { order = append(order, s) })
	vm.Set("done", func() { close(done) })

	// Run JS BEFORE starting the loop.
	jsCode := `
		setTimeout(function() {
			pushOrder("timer1");
			queueMicrotask(function() {
				pushOrder("microtask");
			});
		}, 0);

		setTimeout(function() {
			pushOrder("timer2");
			done();
		}, 0);
	`
	_, err = vm.RunString(jsCode)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() { _ = loop.Run(ctx) }()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("Timeout waiting for done signal")
	}

	expected := []string{"timer1", "microtask", "timer2"}
	if len(order) != len(expected) {
		t.Fatalf("Expected order %v, got %v", expected, order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d]: expected %q, got %q (full order: %v)", i, v, order[i], order)
		}
	}
}

// TestGojaMicrotaskOrdering_ExhaustiveDrain verifies that a large batch of
// promise reaction microtasks (2000) are fully drained before the next
// timer phase fires.
//
// The inter-phase drain and exhaustive safety-net drain have no budget cap.
// The 2000 microtasks are scheduled during RunString
// (synchronously, before the loop starts). When loop.Run begins, the first
// tick drains all queued microtasks at the start-of-tick drainMicrotasks()
// call, before runTimers() fires the setTimeout callback.
func TestGojaMicrotaskOrdering_ExhaustiveDrain(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	vm := goja.New()
	adapter, err := New(loop, vm)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind adapter: %v", err)
	}

	var order []string
	done := make(chan struct{})

	vm.Set("pushOrder", func(s string) { order = append(order, s) })
	vm.Set("done", func() { close(done) })

	// Run JS BEFORE starting the loop. This schedules 2000 promise reaction
	// microtasks (via loop.ScheduleMicrotask through newPromiseJobEnqueuer)
	// and one setTimeout timer. When loop.Run starts, the first tick's
	// start-of-tick drainMicrotasks() drains all 2000 microtasks before
	// runTimers() fires the timer callback.
	const count = 2000
	jsCode := `
		var count = ` + strconv.Itoa(count) + `;
		for (var i = 0; i < count; i++) {
			Promise.resolve(i).then(function(v) {
				pushOrder("microtask-" + v);
			});
		}

		setTimeout(function() {
			pushOrder("timer");
			done();
		}, 0);
	`
	_, err = vm.RunString(jsCode)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	go func() { _ = loop.Run(ctx) }()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("Timeout waiting for done signal")
	}

	// All 2000 microtask handlers must execute BEFORE the timer fires.
	// The timer should be the last entry.
	if len(order) != count+1 {
		t.Fatalf("Expected %d entries (%d microtasks + 1 timer), got %d", count+1, count, len(order))
	}

	// Verify the timer is last.
	if order[count] != "timer" {
		t.Errorf("Expected order[%d] to be %q, got %q", count, "timer", order[count])
	}

	// Verify all microtasks ran before the timer, in order.
	for i := range count {
		expected := "microtask-" + strconv.Itoa(i)
		if order[i] != expected {
			t.Errorf("order[%d]: expected %q, got %q", i, expected, order[i])
			break
		}
	}
}

// TestNode26NextTickPromiseQueueMicrotaskOrder covers the authenticated exact
// Node.js v26.5.0 node-ordering fixture. A nextTick queued
// before promise jobs runs first, promise and queueMicrotask jobs share FIFO
// microtask order, and a nextTick queued from inside a promise job waits until
// the current microtask checkpoint completes.
func TestNode26NextTickPromiseQueueMicrotaskOrder(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = loop.Shutdown(context.Background()) }()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	done := make(chan string, 1)
	if err := runtime.Set("testDone", func(value string) { done <- value }); err != nil {
		t.Fatalf("set testDone: %v", err)
	}
	_, err = runtime.RunString(`
		const events = [];
		process.nextTick(function() { events.push("nextTick1"); });
		Promise.resolve().then(function() {
			events.push("promise1");
			process.nextTick(function() { events.push("nextTick2"); });
			queueMicrotask(function() { events.push("queueMicrotask2"); });
		});
		queueMicrotask(function() { events.push("queueMicrotask1"); });
		setImmediate(function() { testDone(events.join(",")); });
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	select {
	case got := <-done:
		want := "nextTick1,promise1,queueMicrotask1,queueMicrotask2,nextTick2"
		if got != want {
			t.Fatalf("nextTick/promise/queueMicrotask order = %q, want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("Timeout waiting for nextTick/promise/queueMicrotask assertion")
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Shutdown")
	}
}

// TestGojaImmediateMicrotaskCheckpoint verifies Node.js check-phase semantics:
// each setImmediate callback is its own native check callback, so process.nextTick
// and Promise jobs scheduled by one immediate drain before the next immediate can
// run. The Promise job clears the second immediate; batched adapter-level
// flushing would run that second callback before the clearImmediate call happens.
func TestGojaImmediateMicrotaskCheckpoint(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = loop.Shutdown(context.Background()) }()

	vm := goja.New()
	adapter, err := New(loop, vm)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind adapter: %v", err)
	}

	done := make(chan string, 1)
	if err := vm.Set("testDone", func(value string) { done <- value }); err != nil {
		t.Fatalf("Failed to bind testDone: %v", err)
	}

	_, err = vm.RunString(`
		const events = [];
		let second;
		setImmediate(function first() {
			events.push("first");
			process.nextTick(function tickAfterFirst() {
				events.push("tick1");
			});
			Promise.resolve().then(function promiseAfterFirst() {
				events.push("promise1");
				clearImmediate(second);
			});
		});
		second = setImmediate(function secondShouldBeCleared() {
			events.push("second-ran");
		});
		setImmediate(function third() {
			events.push("third");
			Promise.resolve().then(function promiseAfterThird() {
				events.push("promise3");
			});
			setImmediate(function nested() {
				events.push("nested");
				testDone(events.join(","));
			});
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	var got string
	select {
	case got = <-done:
	case <-ctx.Done():
		t.Fatal("Timeout waiting for immediate checkpoint assertion")
	}

	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Shutdown")
	}

	if want := "first,tick1,promise1,third,promise3,nested"; got != want {
		t.Fatalf("immediate checkpoint order = %q, want %q", got, want)
	}
}

// TestGojaImmediateRollover verifies that immediates scheduled while the check
// phase is already running are not appended to the active phase snapshot. They
// must run after all already-queued peer immediates, in the next loop iteration.
func TestGojaImmediateRollover(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = loop.Shutdown(context.Background()) }()

	vm := goja.New()
	adapter, err := New(loop, vm)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind adapter: %v", err)
	}

	done := make(chan string, 1)
	if err := vm.Set("testDone", func(value string) { done <- value }); err != nil {
		t.Fatalf("Failed to bind testDone: %v", err)
	}

	_, err = vm.RunString(`
		const events = [];
		setImmediate(function outer() {
			events.push("outer");
			setImmediate(function inner() { events.push("inner"); });
		});
		setImmediate(function peer() {
			events.push("peer");
			setImmediate(function sentinel() {
				events.push("sentinel");
				testDone(events.join(","));
			});
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	var got string
	select {
	case got = <-done:
	case <-ctx.Done():
		t.Fatal("Timeout waiting for immediate rollover assertion")
	}

	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Shutdown")
	}

	if want := "outer,peer,inner,sentinel"; got != want {
		t.Fatalf("immediate rollover order = %q, want %q", got, want)
	}
}

func TestNode26TimeoutScheduledInsideImmediateUsesValidOrdering(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = loop.Shutdown(context.Background()) }()

	vm := goja.New()
	adapter, err := New(loop, vm)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind adapter: %v", err)
	}

	done := make(chan string, 1)
	if err := vm.Set("testDone", func(value string) { done <- value }); err != nil {
		t.Fatalf("Failed to bind testDone: %v", err)
	}

	_, err = vm.RunString(`
		const events = [];
		let remaining = 2;
		function finish() {
			remaining -= 1;
			if (remaining === 0) testDone(events.join(","));
		}
		setImmediate(function() {
			events.push("immediate1");
			setImmediate(function() {
				events.push("immediate1-inner");
				finish();
			});
			setTimeout(function() {
				events.push("timeout-from-immediate");
				finish();
			}, 0);
		});
		setImmediate(function() { events.push("immediate2"); });
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	var got string
	select {
	case got = <-done:
	case <-ctx.Done():
		t.Fatal("Timeout waiting for timeout-from-immediate ordering assertion")
	}

	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Shutdown")
	}

	switch got {
	case "immediate1,immediate2,immediate1-inner,timeout-from-immediate",
		"immediate1,immediate2,timeout-from-immediate,immediate1-inner":
		return
	default:
		t.Fatalf("timeout scheduled inside immediate order = %q, want a Node v26 valid order", got)
	}
}

// TestGojaStartupDueTimerCanBeatImmediate verifies the Node startup boundary
// where a top-level setTimeout(..., 0) may run before setImmediate if the
// timer's normalized 1ms threshold has elapsed before the first event-loop
// iteration starts. Without the startup internal-queue drain, the adapter's
// timer registration is not visible until after the first check phase and
// setImmediate deterministically wins even when the timer is already due.
func TestGojaStartupDueTimerCanBeatImmediate(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = loop.Shutdown(context.Background()) }()

	vm := goja.New()
	adapter, err := New(loop, vm)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind adapter: %v", err)
	}

	done := make(chan string, 1)
	if err := vm.Set("testDone", func(value string) { done <- value }); err != nil {
		t.Fatalf("Failed to bind testDone: %v", err)
	}

	_, err = vm.RunString(`
		const events = [];
		setTimeout(function timeout() { events.push("timeout"); }, 0);
		setImmediate(function immediate() {
			events.push("immediate");
			testDone(events.join(","));
		});
	`)
	if err != nil {
		t.Fatalf("Failed to run JS: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	var got string
	select {
	case got = <-done:
	case <-ctx.Done():
		t.Fatal("Timeout waiting for startup timer/immediate assertion")
	}

	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Shutdown")
	}

	if want := "timeout,immediate"; got != want {
		t.Fatalf("startup timer/immediate order = %q, want %q", got, want)
	}
}
