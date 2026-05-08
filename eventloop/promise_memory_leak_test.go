package eventloop

import (
	"context"
	"runtime"
	"testing"
	"time"
	"weak"
)

func TestResolvedPromiseChainsReleasePromises(t *testing.T) {
	loop := New()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	ready := make(chan struct{})
	if err := loop.Submit(func() { close(ready) }); err != nil {
		t.Fatalf("warmup Submit: %v", err)
	}
	waitContractSignal(t, ready, "promise-retention loop warmup")
	t.Cleanup(func() {
		if err := loop.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
		if err := waitContractValue(t, runDone, "promise-retention loop completion"); err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	js := NewJS(loop)

	const chainCount = 32
	references := make([]weak.Pointer[ChainedPromise], 0, chainCount*6)
	for value := range chainCount {
		references = append(references, settledPromiseChainReferences(t, js, value)...)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		runtime.GC()
		alive := 0
		for _, reference := range references {
			if reference.Value() != nil {
				alive++
			}
		}
		if alive == 0 {
			runtime.KeepAlive(js)
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("settled promise chains retained %d/%d promises", alive, len(references))
		}
		runtime.Gosched()
	}
}

func settledPromiseChainReferences(t *testing.T, js *JS, value int) []weak.Pointer[ChainedPromise] {
	t.Helper()
	const chainDepth = 5
	source, resolve, _ := js.NewChainedPromise()
	current := source
	references := make([]weak.Pointer[ChainedPromise], 0, chainDepth+1)
	references = append(references, weak.Make(source))
	for range chainDepth {
		current = current.Then(func(value any) any { return value }, nil)
		references = append(references, weak.Make(current))
	}
	result := current.ToChannel()
	resolve(value)
	if got := waitContractValue(t, result, "settled promise chain"); got != value {
		t.Fatalf("settled promise chain result = %#v, want %d", got, value)
	}
	runtime.KeepAlive(source)
	runtime.KeepAlive(current)
	return references
}

func TestRejectionTrackingCleanupUsesCheckpointBarrier(t *testing.T) {
	loop := New()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	ready := make(chan struct{})
	if err := loop.Submit(func() { close(ready) }); err != nil {
		t.Fatalf("warmup Submit: %v", err)
	}
	waitContractSignal(t, ready, "rejection-cleanup loop warmup")
	t.Cleanup(func() {
		if err := loop.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
		if err := waitContractValue(t, runDone, "rejection-cleanup loop completion"); err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	reported := make(chan any, 1)
	js := NewJS(loop, WithUnhandledRejection(func(reason any) {
		reported <- reason
	}))

	const promiseCount = 1000
	setupDone := make(chan struct{})
	if err := loop.Submit(func() {
		defer close(setupDone)
		for range promiseCount {
			p, _, reject := js.NewChainedPromise()
			reject("error")
			p.Then(nil, func(v any) any { return nil })
		}
	}); err != nil {
		t.Fatalf("Submit rejection setup: %v", err)
	}
	waitContractSignal(t, setupDone, "same-turn rejection handler setup")

	type trackingState struct {
		rejections    int
		handlerReady  int
		scheduled     bool
		running       bool
		rerun         bool
		fallbackRerun bool
	}
	observed := make(chan trackingState, 1)
	if err := loop.scheduleMicrotaskCheckpoint(func() {
		js.rejectionsMu.RLock()
		rejections := len(js.unhandledRejections)
		js.rejectionsMu.RUnlock()
		js.handlerReadyMu.Lock()
		handlerReady := len(js.handlerReadyChans)
		js.handlerReadyMu.Unlock()
		observed <- trackingState{
			rejections:    rejections,
			handlerReady:  handlerReady,
			scheduled:     js.checkRejectionScheduled.Load(),
			running:       js.checkRejectionRunning.Load(),
			rerun:         js.checkRejectionRerun.Load(),
			fallbackRerun: js.checkRejectionFallbackRerun.Load(),
		}
	}); err != nil {
		t.Fatalf("schedule cleanup checkpoint barrier: %v", err)
	}
	state := waitContractValue(t, observed, "rejection cleanup checkpoint barrier")
	if state != (trackingState{}) {
		t.Fatalf("rejection tracking after checkpoint barrier = %+v, want zero state", state)
	}
	select {
	case reason := <-reported:
		t.Fatalf("handled rejection was reported with reason %#v", reason)
	default:
	}
}

// TestPromiseMemoryLeak_HandlerFieldsCleared verifies that after settlement,
// the handler fields (h0, result-as-handlers) are properly zeroed,
// releasing closure references for garbage collection.
func TestPromiseMemoryLeak_HandlerFieldsCleared(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop)

	// Test 1: After resolve, h0 should be cleared (target becomes nil)
	p, resolve, _ := js.NewChainedPromise()
	p.Then(func(v any) any { return v }, nil)
	resolve("value")

	// Verify internal state
	p.mu.Lock()
	if p.h0.onFulfilled != nil || p.h0.onRejected != nil || p.h0.target != nil {
		t.Error("h0 should be zero-value after resolve")
	}
	p.mu.Unlock()

	// Test 2: After reject, same fields should be cleared
	p2, _, reject := js.NewChainedPromise()
	p2.Then(func(v any) any { return v }, func(v any) any { return v })
	reject("error")

	p2.mu.Lock()
	if p2.h0.onFulfilled != nil || p2.h0.onRejected != nil || p2.h0.target != nil {
		t.Error("h0 should be zero-value after reject")
	}
	p2.mu.Unlock()

	// Test 3: Multiple handlers — all should be cleared
	p3, resolve3, _ := js.NewChainedPromise()
	p3.Then(func(v any) any { return v }, nil)
	p3.Then(func(v any) any { return v }, nil)
	p3.Then(func(v any) any { return v }, nil)
	resolve3("value")

	p3.mu.Lock()
	// After resolve, result should be the settled value, not []handler
	if _, isHandlers := p3.result.([]handler); isHandlers {
		t.Error("result should not contain handlers after resolve")
	}
	if p3.result != any("value") {
		t.Errorf("result should be 'value', got: %v", p3.result)
	}
	p3.mu.Unlock()
}
