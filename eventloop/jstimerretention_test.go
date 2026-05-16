package eventloop

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"weak"
)

type intervalRetentionPayload struct {
	value int
	_     [32]byte
}

type intervalRetentionState struct {
	clearErr      error
	checkpointErr error
	adapterLive   bool
	nativeLive    bool
	refedTimers   int64
}

func TestSelfCancelingIntervalReleasesHandleAndCapture(t *testing.T) {
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
	waitContractSignal(t, ready, "interval-retention loop warmup")
	t.Cleanup(func() {
		if err := loop.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
		if err := waitContractValue(t, runDone, "interval-retention loop completion"); err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	pointer := selfCancelingIntervalPointer(t, loop, js)
	waitContractCollected(t, pointer, js)
}

func selfCancelingIntervalPointer(t *testing.T, loop *Loop, js *JS) weak.Pointer[intervalRetentionPayload] {
	t.Helper()
	payload := &intervalRetentionPayload{value: 1}
	pointer := weak.Make(payload)
	published := make(chan struct{})
	releasePublication := releaseSignalT(t, published)
	result := make(chan intervalRetentionState, 1)
	var adapterID atomic.Uint64
	var nativeID atomic.Uint64
	var callbackCount atomic.Int32

	id, err := js.SetInterval(func() {
		<-published
		payload.value++
		if callbackCount.Add(1) != 3 {
			return
		}
		state := intervalRetentionState{clearErr: js.ClearInterval(adapterID.Load())}
		state.checkpointErr = loop.scheduleMicrotaskCheckpoint(func() {
			js.intervalsMu.RLock()
			_, state.adapterLive = js.intervals[adapterID.Load()]
			js.intervalsMu.RUnlock()
			_, state.nativeLive = loop.timerMap[TimerID(nativeID.Load())]
			state.refedTimers = loop.refedTimerCount.Load()
			result <- state
		})
		if state.checkpointErr != nil {
			result <- state
		}
	}, 1)
	if err != nil {
		t.Fatalf("SetInterval: %v", err)
	}
	adapterID.Store(id)
	js.intervalsMu.RLock()
	interval := js.intervals[id]
	js.intervalsMu.RUnlock()
	if interval == nil {
		t.Fatal("SetInterval did not publish adapter state")
	}
	nativeID.Store(interval.currentLoopTimerID.Load())
	releasePublication()

	state := waitContractValue(t, result, "self-canceling interval cleanup barrier")
	if state.clearErr != nil {
		t.Fatalf("ClearInterval from callback: %v", state.clearErr)
	}
	if state.checkpointErr != nil {
		t.Fatalf("interval cleanup checkpoint: %v", state.checkpointErr)
	}
	if state.adapterLive || state.nativeLive || state.refedTimers != 0 {
		t.Fatalf("interval state after self-cancel: adapterLive=%v nativeLive=%v refedTimers=%d", state.adapterLive, state.nativeLive, state.refedTimers)
	}
	if got := callbackCount.Load(); got != 3 {
		t.Fatalf("interval callback count = %d, want 3", got)
	}
	runtime.KeepAlive(payload)
	return pointer
}
