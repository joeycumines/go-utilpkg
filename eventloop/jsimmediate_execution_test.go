package eventloop

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func runImmediateTestLoop(t *testing.T, loop *Loop) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		if err := waitContractValue(t, runDone, "immediate test Run completion"); !errors.Is(err, context.Canceled) {
			t.Errorf("Run: got %v, want %v", err, context.Canceled)
		}
	})
}

func TestJSSetImmediateExecutesAsynchronouslyAndCleansState(t *testing.T) {
	loop, js := newImmediateTestJS(t)
	var calls atomic.Int32
	callbackRan := make(chan struct{})
	id, err := js.SetImmediate(func() { calls.Add(1) })
	if err != nil {
		t.Fatalf("SetImmediate: %v", err)
	}
	if _, err := js.SetImmediate(func() { close(callbackRan) }); err != nil {
		t.Fatalf("SetImmediate completion sentinel: %v", err)
	}
	select {
	case <-callbackRan:
		t.Fatal("immediate callback ran synchronously")
	default:
	}
	js.setImmediateMu.RLock()
	state := js.setImmediateMap[id]
	js.setImmediateMu.RUnlock()
	if state == nil || state.cleared.Load() {
		t.Fatalf("pre-execution immediate state = %#v, want published and uncleared", state)
	}

	runImmediateTestLoop(t, loop)
	waitContractSignal(t, callbackRan, "immediate completion sentinel")
	if got := calls.Load(); got != 1 {
		t.Fatalf("immediate callback calls: got %d, want 1", got)
	}
	if !state.cleared.Load() {
		t.Fatal("executed immediate state remained uncleared")
	}
	js.setImmediateMu.RLock()
	_, retained := js.setImmediateMap[id]
	js.setImmediateMu.RUnlock()
	if retained {
		t.Fatal("executed immediate state remained registered")
	}
}

func TestJSSetImmediateQueuedDuringImmediateRollsForward(t *testing.T) {
	loop := New(WithAutoExit(true))
	js := NewJS(loop)
	var order []string
	if _, err := js.SetImmediate(func() {
		order = append(order, "outer")
		if _, err := js.SetImmediate(func() { order = append(order, "inner") }); err != nil {
			t.Errorf("inner SetImmediate: %v", err)
		}
		if _, err := js.SetTimeout(func() { order = append(order, "timeout") }, 0); err != nil {
			t.Errorf("SetTimeout: %v", err)
		}
	}); err != nil {
		t.Fatalf("outer SetImmediate: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []string{"outer", "inner", "timeout"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("immediate rollover order: got %v, want %v", order, want)
	}
}

func TestJSImmediatePanicStillCleansState(t *testing.T) {
	loop, js := newImmediateTestJS(t)
	id, err := js.SetImmediate(func() { panic("immediate panic") })
	if err != nil {
		t.Fatalf("SetImmediate: %v", err)
	}
	callbacksDrained := make(chan struct{})
	if _, err := js.SetImmediate(func() { close(callbacksDrained) }); err != nil {
		t.Fatalf("SetImmediate completion sentinel: %v", err)
	}

	runImmediateTestLoop(t, loop)
	waitContractSignal(t, callbacksDrained, "post-panic immediate callback")
	js.setImmediateMu.RLock()
	_, retained := js.setImmediateMap[id]
	js.setImmediateMu.RUnlock()
	if retained {
		t.Fatal("panicking immediate state remained registered")
	}
}

func TestJSClearImmediateAtExecutionBoundary(t *testing.T) {
	loop, js := newImmediateTestJS(t)
	callbackBoundary := make(chan struct{})
	releaseCallback := make(chan struct{})
	releaseCallbackOnce := contractRelease(t, releaseCallback)
	var hookOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeJSImmediatePublicationWait: func() {
			hookOnce.Do(func() {
				close(callbackBoundary)
				<-releaseCallback
			})
		},
	}
	var calls atomic.Int32
	id, err := js.SetImmediate(func() { calls.Add(1) })
	if err != nil {
		t.Fatalf("SetImmediate: %v", err)
	}
	drained := make(chan struct{})
	if _, err := js.SetImmediate(func() { close(drained) }); err != nil {
		t.Fatalf("SetImmediate completion sentinel: %v", err)
	}

	runImmediateTestLoop(t, loop)
	waitContractSignal(t, callbackBoundary, "immediate execution boundary")
	if err := js.ClearImmediate(id); err != nil {
		t.Fatalf("ClearImmediate: %v", err)
	}
	releaseCallbackOnce()
	waitContractSignal(t, drained, "post-clear immediate callback")
	if got := calls.Load(); got != 0 {
		t.Fatalf("callback calls after boundary clear: got %d, want 0", got)
	}
}

func TestJSClearImmediateAfterCallbackEntry(t *testing.T) {
	loop, js := newImmediateTestJS(t)
	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	releaseCallbackOnce := contractRelease(t, releaseCallback)
	id, err := js.SetImmediate(func() {
		close(callbackEntered)
		<-releaseCallback
	})
	if err != nil {
		t.Fatalf("SetImmediate: %v", err)
	}
	drained := make(chan struct{})
	if _, err := js.SetImmediate(func() { close(drained) }); err != nil {
		t.Fatalf("SetImmediate completion sentinel: %v", err)
	}

	runImmediateTestLoop(t, loop)
	waitContractSignal(t, callbackEntered, "immediate callback entry")
	if err := js.ClearImmediate(id); err != ErrImmediateNotFound {
		t.Fatalf("ClearImmediate after callback entry: got %v, want %v", err, ErrImmediateNotFound)
	}
	releaseCallbackOnce()
	waitContractSignal(t, drained, "post-entry-clear immediate callback")
}

func TestJSClearImmediateAfterExecutionClaim(t *testing.T) {
	loop, js := newImmediateTestJS(t)
	callbackClaimed := make(chan struct{})
	releaseCallback := make(chan struct{})
	releaseCallbackOnce := contractRelease(t, releaseCallback)
	var claimedID atomic.Uint64
	loop.testHooks = &loopTestHooks{
		BeforeJSImmediateCallbackEntry: func(id uint64) {
			claimedID.Store(id)
			close(callbackClaimed)
			<-releaseCallback
		},
	}
	callbackEntered := make(chan struct{})
	id, err := js.SetImmediate(func() { close(callbackEntered) })
	if err != nil {
		t.Fatalf("SetImmediate: %v", err)
	}

	runImmediateTestLoop(t, loop)
	waitContractSignal(t, callbackClaimed, "immediate execution claim")
	if got := claimedID.Load(); got != id {
		t.Fatalf("claimed immediate ID: got %d, want %d", got, id)
	}
	clearErr := js.ClearImmediate(id)
	releaseCallbackOnce()
	waitContractSignal(t, callbackEntered, "claimed immediate callback entry")
	if clearErr != ErrImmediateNotFound {
		t.Fatalf("ClearImmediate after execution claim: got %v, want %v", clearErr, ErrImmediateNotFound)
	}
}

func TestJSSetImmediateFIFO(t *testing.T) {
	loop, js := newImmediateTestJS(t)
	const callbackCount = 100
	order := make([]int, 0, callbackCount)
	done := make(chan struct{})
	for index := range callbackCount {
		if _, err := js.SetImmediate(func() {
			order = append(order, index)
			if len(order) == callbackCount {
				close(done)
			}
		}); err != nil {
			t.Fatalf("SetImmediate %d: %v", index, err)
		}
	}

	runImmediateTestLoop(t, loop)
	waitContractSignal(t, done, "FIFO immediate callbacks")
	for index, got := range order {
		if got != index {
			t.Fatalf("callback order at %d: got %d, want %d", index, got, index)
		}
	}
}
