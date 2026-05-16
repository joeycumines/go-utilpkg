package eventloop

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestUnhandledRejectionReportClearsRunningTracking(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	ready := make(chan struct{})
	if err := loop.Submit(func() { close(ready) }); err != nil {
		t.Fatalf("warmup Submit: %v", err)
	}
	waitContractSignal(t, ready, "unhandled-rejection loop warmup")
	t.Cleanup(func() {
		if err := loop.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
		if err := waitContractValue(t, runDone, "unhandled-rejection loop completion"); err != nil {
			t.Errorf("Run: %v", err)
		}
	})

	type trackingState struct {
		rejections    int
		handlerReady  int
		scheduled     bool
		running       bool
		rerun         bool
		fallbackRerun bool
	}
	type result struct {
		state trackingState
		err   error
	}
	reason := errors.New("running rejection")
	observed := make(chan result, 1)
	var callbackCount atomic.Int32
	var js *JS
	js, err = NewJS(loop, WithUnhandledRejection(func(got any) {
		if got != reason {
			observed <- result{err: errors.New("unhandled-rejection callback received wrong reason")}
			return
		}
		callbackCount.Add(1)
		if err := loop.scheduleMicrotaskCheckpoint(func() {
			jsState := trackingState{
				scheduled:     false,
				running:       false,
				rerun:         false,
				fallbackRerun: false,
			}
			js.rejectionsMu.RLock()
			jsState.rejections = len(js.unhandledRejections)
			js.rejectionsMu.RUnlock()
			js.handlerReadyMu.Lock()
			jsState.handlerReady = len(js.handlerReadyChans)
			js.handlerReadyMu.Unlock()
			jsState.scheduled = js.checkRejectionScheduled.Load()
			jsState.running = js.checkRejectionRunning.Load()
			jsState.rerun = js.checkRejectionRerun.Load()
			jsState.fallbackRerun = js.checkRejectionFallbackRerun.Load()
			observed <- result{state: jsState}
		}); err != nil {
			observed <- result{err: err}
		}
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.Submit(func() {
		_, _, reject := js.NewChainedPromise()
		reject(reason)
	}); err != nil {
		t.Fatalf("Submit rejection: %v", err)
	}
	got := waitContractValue(t, observed, "post-report rejection tracking barrier")
	if got.err != nil {
		t.Fatalf("post-report barrier: %v", got.err)
	}
	if got.state != (trackingState{}) {
		t.Fatalf("rejection tracking after report = %+v, want zero state", got.state)
	}
	if got := callbackCount.Load(); got != 1 {
		t.Fatalf("unhandled-rejection callback count = %d, want 1", got)
	}
}
