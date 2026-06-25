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
// Without strict per-callback draining, microtasks would be batched and
// run after all timers, producing ["timer1", "timer2", "microtask"].
// With WithStrictMicrotaskOrdering(true), the order is ["timer1",
// "microtask", "timer2"].
func TestGojaMicrotaskOrdering_PromiseBetweenTimers(t *testing.T) {
	loop, err := goeventloop.New(goeventloop.WithStrictMicrotaskOrdering(true))
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	vm := goja.New()
	adapter, err := New(loop, vm)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
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
//
// Uses WithStrictMicrotaskOrdering(true) for per-callback draining.
func TestGojaMicrotaskOrdering_QueueMicrotaskBetweenTimers(t *testing.T) {
	loop, err := goeventloop.New(goeventloop.WithStrictMicrotaskOrdering(true))
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	vm := goja.New()
	adapter, err := New(loop, vm)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
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
// This does NOT require WithStrictMicrotaskOrdering(true) because the
// inter-phase drain and the exhaustive safety-net drain (no budget cap)
// are unconditional. The 2000 microtasks are scheduled during RunString
// (synchronously, before the loop starts). When loop.Run begins, the first
// tick drains all queued microtasks at the start-of-tick drainMicrotasks()
// call, before runTimers() fires the setTimeout callback.
func TestGojaMicrotaskOrdering_ExhaustiveDrain(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	vm := goja.New()
	adapter, err := New(loop, vm)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
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
	for i := 0; i < count; i++ {
		expected := "microtask-" + strconv.Itoa(i)
		if order[i] != expected {
			t.Errorf("order[%d]: expected %q, got %q", i, expected, order[i])
			break
		}
	}
}
