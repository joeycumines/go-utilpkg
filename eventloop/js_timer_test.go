package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestJSClearIntervalCancelsTimeoutID(t *testing.T) {
	loop := New()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	registerActiveLoopCleanupT(t, loop, runDone)
	waitLoopOwnerTurnT(t, loop)

	js := NewJS(loop)

	var callbackRan atomic.Bool
	id, err := js.SetTimeout(func() { callbackRan.Store(true) }, 60_000)
	if err != nil {
		t.Fatalf("SetTimeout failed: %v", err)
	}

	if err := js.ClearInterval(id); err != nil {
		t.Fatalf("ClearInterval(timeoutID) failed: %v", err)
	}

	js.timeoutsMu.RLock()
	_, timeoutExists := js.timeouts[id]
	js.timeoutsMu.RUnlock()
	if timeoutExists {
		t.Fatal("timeout ID remained in timeout registry after ClearInterval")
	}
	if callbackRan.Load() {
		t.Fatal("timeout callback ran after ClearInterval(timeoutID)")
	}
}

func TestJSClearTimeoutCancelsIntervalID(t *testing.T) {
	loop := New()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	registerActiveLoopCleanupT(t, loop, runDone)
	waitLoopOwnerTurnT(t, loop)

	js := NewJS(loop)

	var callbackRan atomic.Bool
	id, err := js.SetInterval(func() { callbackRan.Store(true) }, 60_000)
	if err != nil {
		t.Fatalf("SetInterval failed: %v", err)
	}

	if err := js.ClearTimeout(id); err != nil {
		t.Fatalf("ClearTimeout(intervalID) failed: %v", err)
	}

	js.intervalsMu.RLock()
	_, intervalExists := js.intervals[id]
	js.intervalsMu.RUnlock()
	if intervalExists {
		t.Fatal("interval ID remained in interval registry after ClearTimeout")
	}
	if callbackRan.Load() {
		t.Fatal("interval callback ran after ClearTimeout(intervalID)")
	}
}

func TestJSSetTimeoutPublicationPrecedesCallback(t *testing.T) {
	loop := New()

	hookEntered := make(chan struct{})
	callbackWaiting := make(chan struct{})
	releaseHook := make(chan struct{})
	var releaseHookOnce sync.Once
	defer releaseHookOnce.Do(func() { close(releaseHook) })
	loop.testHooks = &loopTestHooks{
		BeforeJSTimeoutRegistryPublish: func(uint64) {
			close(hookEntered)
			<-releaseHook
		},
		BeforeJSTimeoutPublicationWait: func() { close(callbackWaiting) },
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	registerActiveLoopCleanupT(t, loop, runDone)
	waitLoopOwnerTurnT(t, loop)

	js := NewJS(loop)

	callbackRan := make(chan struct{})
	setTimeoutDone := make(chan struct {
		id  uint64
		err error
	}, 1)
	go func() {
		id, err := js.SetTimeout(func() { close(callbackRan) }, 0)
		setTimeoutDone <- struct {
			id  uint64
			err error
		}{id: id, err: err}
	}()

	select {
	case <-hookEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("SetTimeout did not reach pre-publication hook")
	}
	select {
	case <-callbackWaiting:
	case <-time.After(5 * time.Second):
		t.Fatal("native timeout callback did not reach its publication wait")
	}
	select {
	case <-callbackRan:
		t.Fatal("timeout callback entered before SetTimeout published its adapter handle")
	default:
	}

	releaseHookOnce.Do(func() { close(releaseHook) })

	var result struct {
		id  uint64
		err error
	}
	select {
	case result = <-setTimeoutDone:
	case <-time.After(5 * time.Second):
		t.Fatal("SetTimeout did not return after pre-publication hook release")
	}
	if result.err != nil {
		t.Fatalf("SetTimeout failed: %v", result.err)
	}
	select {
	case <-callbackRan:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout callback did not run after SetTimeout published its adapter handle")
	}

	js.timeoutsMu.RLock()
	_, exists := js.timeouts[result.id]
	js.timeoutsMu.RUnlock()
	if exists {
		t.Fatal("fired timeout ID remained in timeout registry after callback-before-publication interleaving")
	}
	if err := js.ClearTimeout(result.id); !errors.Is(err, ErrTimerNotFound) {
		t.Fatalf("ClearTimeout(fired ID) = %v, want ErrTimerNotFound", err)
	}
}

func TestJSSetIntervalPublicationPrecedesCallback(t *testing.T) {
	loop := New()

	hookEntered := make(chan struct{})
	callbackWaiting := make(chan struct{})
	releaseHook := make(chan struct{})
	var releaseHookOnce sync.Once
	defer releaseHookOnce.Do(func() { close(releaseHook) })
	hookState := make(chan struct {
		id      uint64
		state   *intervalState
		initial TimerID
	}, 1)
	var callbackWaitOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeJSIntervalTimerIDPublish: func(id uint64, state *intervalState, initial TimerID) {
			hookState <- struct {
				id      uint64
				state   *intervalState
				initial TimerID
			}{id: id, state: state, initial: initial}
			close(hookEntered)
			<-releaseHook
		},
		BeforeJSIntervalPublicationWait: func() {
			callbackWaitOnce.Do(func() { close(callbackWaiting) })
		},
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	registerActiveLoopCleanupT(t, loop, runDone)
	waitLoopOwnerTurnT(t, loop)

	js := NewJS(loop)

	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	var releaseCallbackOnce sync.Once
	defer releaseCallbackOnce.Do(func() { close(releaseCallback) })
	var fireCount atomic.Int32
	setIntervalDone := make(chan struct {
		id  uint64
		err error
	}, 1)
	go func() {
		id, err := js.SetInterval(func() {
			if fireCount.Add(1) == 1 {
				close(callbackEntered)
				<-releaseCallback
			}
		}, 0)
		setIntervalDone <- struct {
			id  uint64
			err error
		}{id: id, err: err}
	}()

	select {
	case <-hookEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("SetInterval did not reach pre-publication hook")
	}
	var publication struct {
		id      uint64
		state   *intervalState
		initial TimerID
	}
	select {
	case publication = <-hookState:
	case <-time.After(5 * time.Second):
		t.Fatal("SetInterval hook did not expose publication state")
	}
	select {
	case <-callbackWaiting:
	case <-time.After(5 * time.Second):
		t.Fatal("native interval callback did not reach its publication wait")
	}
	select {
	case <-callbackEntered:
		t.Fatal("interval callback entered before SetInterval published its adapter handle")
	default:
	}

	releaseHookOnce.Do(func() { close(releaseHook) })

	var result struct {
		id  uint64
		err error
	}
	select {
	case result = <-setIntervalDone:
	case <-time.After(5 * time.Second):
		t.Fatal("SetInterval did not return after pre-publication hook release")
	}
	if result.err != nil {
		t.Fatalf("SetInterval failed: %v", result.err)
	}
	if result.id != publication.id {
		t.Fatalf("SetInterval returned id %d, hook observed id %d", result.id, publication.id)
	}
	select {
	case <-callbackEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("interval callback did not enter after SetInterval published its adapter handle")
	}

	if got := TimerID(publication.state.currentLoopTimerID.Load()); got != publication.initial {
		t.Fatalf("native interval timer ID = %d, want initial repeating timer ID %d", got, publication.initial)
	}

	releaseCallbackOnce.Do(func() { close(releaseCallback) })
	if err := js.ClearInterval(result.id); err != nil {
		t.Fatalf("ClearInterval after pre-publication first fire failed: %v", err)
	}

}

func TestJSSetIntervalExternalCancelWhileCallbackBlockedCancelsNativeTimer(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	js := NewJS(loop)
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	var fireCount atomic.Int32
	entered := make(chan struct{})
	releaseCallback := make(chan struct{})
	releaseCallbackFn := releaseSignalT(t, releaseCallback)
	var enteredOnce sync.Once
	id, err := js.SetInterval(func() {
		if fireCount.Add(1) != 1 {
			return
		}
		enteredOnce.Do(func() { close(entered) })
		<-releaseCallback
	}, 1)
	if err != nil {
		t.Fatalf("SetInterval: %v", err)
	}
	js.intervalsMu.RLock()
	interval := js.intervals[id]
	js.intervalsMu.RUnlock()
	if interval == nil {
		t.Fatal("SetInterval did not publish adapter state")
	}
	nativeID := TimerID(interval.currentLoopTimerID.Load())
	if nativeID == 0 {
		t.Fatal("SetInterval did not publish native timer ID")
	}

	waitContractSignal(t, entered, "blocked interval callback entry")
	clearDone := make(chan error, 1)
	go func() { clearDone <- js.ClearInterval(id) }()
	if err := waitContractValue(t, clearDone, "external ClearInterval admission"); err != nil {
		t.Fatalf("ClearInterval: %v", err)
	}

	type cleanupState struct {
		adapterPresent bool
		nativePresent  bool
		refedTimers    int64
		fireCount      int32
	}
	cleanupObserved := make(chan cleanupState, 1)
	if err := loop.SubmitInternal(func() {
		js.intervalsMu.RLock()
		_, adapterPresent := js.intervals[id]
		js.intervalsMu.RUnlock()
		_, nativePresent := loop.timerMap[nativeID]
		cleanupObserved <- cleanupState{
			adapterPresent: adapterPresent,
			nativePresent:  nativePresent,
			refedTimers:    loop.refedTimerCount.Load(),
			fireCount:      fireCount.Load(),
		}
	}); err != nil {
		t.Fatalf("cleanup observation admission: %v", err)
	}
	releaseCallbackFn()
	state := waitContractValue(t, cleanupObserved, "post-cancellation owner barrier")
	if state.adapterPresent || state.nativePresent || state.refedTimers != 0 || state.fireCount != 1 {
		t.Fatalf("interval after external clear = (adapter=%v, native=%v, refs=%d, fires=%d), want (false, false, 0, 1)", state.adapterPresent, state.nativePresent, state.refedTimers, state.fireCount)
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "externally canceled interval Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestJSTimerReEntrancy(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)
	js := NewJS(loop)

	order := make(chan string, 2)
	scheduleErr := make(chan error, 1)
	_, err := js.SetTimeout(func() {
		order <- "first"
		if _, err := js.SetTimeout(func() { order <- "second" }, 0); err != nil {
			select {
			case scheduleErr <- err:
			default:
			}
		}
	}, 0)
	if err != nil {
		t.Fatalf("SetTimeout: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	want := []string{"first", "second"}
	for i, expected := range want {
		select {
		case got := <-order:
			if got != expected {
				t.Fatalf("timer %d fired as %q, want %q", i, got, expected)
			}
		case err := <-scheduleErr:
			t.Fatalf("nested SetTimeout failed: %v", err)
		case <-time.After(2 * time.Second):
			t.Fatalf("timer %d did not fire", i)
		}
	}
	if err := waitContractValue(t, runDone, "nested zero-delay timer auto-exit"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	select {
	case err := <-scheduleErr:
		t.Fatalf("nested SetTimeout failed: %v", err)
	default:
	}
}
