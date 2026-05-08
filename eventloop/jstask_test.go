package eventloop

import (
	"context"
	"slices"
	"testing"
)

func newRunningMicrotaskJS(t *testing.T) (*Loop, *JS) {
	t.Helper()
	loop := New()
	js := NewJS(loop)
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	ready := make(chan struct{})
	if err := loop.Submit(func() { close(ready) }); err != nil {
		t.Fatalf("warmup Submit: %v", err)
	}
	waitContractSignal(t, ready, "microtask test loop warmup")
	t.Cleanup(func() {
		if err := loop.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
		if err := waitContractValue(t, runDone, "microtask test loop completion"); err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	return loop, js
}

func TestJSQueueMicrotaskExecutes(t *testing.T) {
	_, js := newRunningMicrotaskJS(t)
	callbackRan := make(chan struct{})
	if err := js.QueueMicrotask(func() { close(callbackRan) }); err != nil {
		t.Fatalf("QueueMicrotask: %v", err)
	}
	waitContractSignal(t, callbackRan, "QueueMicrotask callback")
}

func TestJSQueueMicrotaskPanicDoesNotStopCheckpoint(t *testing.T) {
	loop, js := newRunningMicrotaskJS(t)
	firstEntered := make(chan struct{})
	continued := make(chan struct{})
	scheduled := make(chan error, 1)
	if err := loop.Submit(func() {
		if err := js.QueueMicrotask(func() {
			close(firstEntered)
			panic("microtask panic")
		}); err != nil {
			scheduled <- err
			return
		}
		if err := js.QueueMicrotask(func() { close(continued) }); err != nil {
			scheduled <- err
			return
		}
		scheduled <- nil
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := waitContractValue(t, scheduled, "panicking microtask scheduling"); err != nil {
		t.Fatalf("QueueMicrotask: %v", err)
	}
	waitContractSignal(t, firstEntered, "panicking microtask entry")
	waitContractSignal(t, continued, "microtask after panic")
}

func TestJSMicrotaskBeforeTimer(t *testing.T) {
	loop, js := newRunningMicrotaskJS(t)
	order := make([]string, 0, 2)
	callbacksDone := make(chan struct{})
	record := func(name string) {
		order = append(order, name)
		if len(order) == 2 {
			close(callbacksDone)
		}
	}
	scheduled := make(chan error, 1)
	if err := loop.Submit(func() {
		if _, err := js.SetTimeout(func() { record("timer") }, 0); err != nil {
			scheduled <- err
			return
		}
		if err := js.QueueMicrotask(func() { record("microtask") }); err != nil {
			scheduled <- err
			return
		}
		scheduled <- nil
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := waitContractValue(t, scheduled, "timer and microtask scheduling"); err != nil {
		t.Fatalf("scheduling timer and microtask: %v", err)
	}
	waitContractSignal(t, callbacksDone, "timer and microtask callbacks")
	if !slices.Equal(order, []string{"microtask", "timer"}) {
		t.Fatalf("callback order = %v, want [microtask timer]", order)
	}
}

func TestJSNextTickAndMicrotaskPriorityAndFIFO(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)
	js := NewJS(loop)
	order := make([]string, 0, 5)
	scheduled := make(chan error, 1)
	var reaction *ChainedPromise
	if err := loop.Submit(func() {
		if err := js.NextTick(func() { order = append(order, "nextTick-1") }); err != nil {
			scheduled <- err
			return
		}
		if err := js.NextTick(func() { order = append(order, "nextTick-2") }); err != nil {
			scheduled <- err
			return
		}
		if err := js.QueueMicrotask(func() { order = append(order, "queueMicrotask-1") }); err != nil {
			scheduled <- err
			return
		}
		if err := js.QueueMicrotask(func() { order = append(order, "queueMicrotask-2") }); err != nil {
			scheduled <- err
			return
		}
		reaction = js.Resolve("promise").Then(func(value any) any {
			order = append(order, value.(string))
			return "reaction complete"
		}, nil)
		scheduled <- nil
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := waitContractValue(t, scheduled, "nextTick and microtask scheduling"); err != nil {
		t.Fatalf("scheduling owner work: %v", err)
	}
	if !slices.Equal(order, []string{"nextTick-1", "nextTick-2", "queueMicrotask-1", "queueMicrotask-2", "promise"}) {
		t.Fatalf("callback order = %v, want nextTick FIFO before microtasks", order)
	}
	if reaction == nil || reaction.State() != Fulfilled || reaction.Value() != "reaction complete" {
		t.Fatalf("Promise reaction = %v, want fulfilled reaction result", reaction)
	}
}
