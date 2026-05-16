package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCallbackMicrotaskBarrier(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	hooks, pollReached := newIdleWaitBoundaryHooks()
	loop.testHooks = hooks
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, pollReached, "initial idle wait")

	var order []string
	var orderMu sync.Mutex
	var callbacks sync.WaitGroup
	callbacks.Add(3)
	taskA := func() {
		orderMu.Lock()
		order = append(order, "task-a")
		orderMu.Unlock()
		if err := loop.ScheduleMicrotask(func() {
			orderMu.Lock()
			order = append(order, "microtask")
			orderMu.Unlock()
			callbacks.Done()
		}); err != nil {
			t.Errorf("ScheduleMicrotask: %v", err)
		}
		callbacks.Done()
	}
	taskB := func() {
		orderMu.Lock()
		order = append(order, "task-b")
		orderMu.Unlock()
		callbacks.Done()
	}

	loop.externalMu.Lock()
	loop.enqueueCommandLocked(loopCommand{kind: loopCommandExternal, fn: taskA})
	loop.enqueueCommandLocked(loopCommand{kind: loopCommandExternal, fn: taskB})
	loop.externalMu.Unlock()
	loop.wakeAfterIngress()

	callbacksDone := make(chan struct{})
	go func() {
		callbacks.Wait()
		close(callbacksDone)
	}()
	select {
	case <-callbacksDone:
	case <-time.After(5 * time.Second):
		t.Fatal("callbacks did not complete")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	if err := loop.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}

	orderMu.Lock()
	defer orderMu.Unlock()
	want := []string{"task-a", "microtask", "task-b"}
	if len(order) != len(want) {
		t.Fatalf("order length = %d, want %d: %v", len(order), len(want), order)
	}
	for index := range want {
		if order[index] != want[index] {
			t.Fatalf("order[%d] = %q, want %q: %v", index, order[index], want[index], order)
		}
	}
}

func TestMicrotaskCheckpointExhaustsQueue(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	hooks, pollReached := newIdleWaitBoundaryHooks()
	loop.testHooks = hooks
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, pollReached, "initial idle wait")

	const total = 2000
	var executed atomic.Int32
	done := make(chan struct{})
	if err := loop.Submit(func() {
		for range total {
			if err := loop.ScheduleMicrotask(func() {
				if executed.Add(1) == total {
					close(done)
				}
			}); err != nil {
				t.Errorf("ScheduleMicrotask: %v", err)
			}
		}
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("microtask checkpoint did not exhaust the queue")
	}
	if got := executed.Load(); got != total {
		t.Fatalf("executed = %d, want %d", got, total)
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	if err := loop.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}
}
