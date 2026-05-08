package eventloop

import (
	"errors"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

func TestAbortControllerSignalSettlement(t *testing.T) {
	controller := NewAbortController()
	if controller == nil {
		t.Fatal("NewAbortController returned nil")
	}
	signal := controller.Signal()
	if signal == nil || signal != controller.Signal() {
		t.Fatal("Signal did not return one stable nonnil signal")
	}
	if signal.Aborted() || signal.Reason() != nil {
		t.Fatalf("new signal = (aborted=%v, reason=%#v), want (false, nil)", signal.Aborted(), signal.Reason())
	}
	if err := signal.ThrowIfAborted(); err != nil {
		t.Fatalf("ThrowIfAborted before settlement: %v", err)
	}

	signal.OnAbort(nil)
	firstReason := &struct{ label string }{label: "first"}
	var handlerCalls int
	var receivedReason any
	signal.OnAbort(func(reason any) {
		handlerCalls++
		receivedReason = reason
	})
	controller.Abort(firstReason)
	controller.Abort("second")
	controller.Abort("third")
	signal.OnAbort(nil)

	if !signal.Aborted() {
		t.Fatal("settled signal reported not aborted")
	}
	if reason := signal.Reason(); reason != firstReason {
		t.Fatalf("settled reason = %#v, want first reason %#v", reason, firstReason)
	}
	if handlerCalls != 1 || receivedReason != firstReason {
		t.Fatalf("handler delivery = (calls=%d, reason=%#v), want (1, %#v)", handlerCalls, receivedReason, firstReason)
	}
}

func TestAbortSignalConcurrentAccess(t *testing.T) {
	controller := NewAbortController()
	signal := controller.Signal()
	const handlerTotal = 100
	const readerTotal = 100
	const reason = "concurrent abort"
	start := make(chan struct{})
	startNow := contractRelease(t, start)
	ready := make(chan struct{}, handlerTotal+readerTotal+1)
	type readResult struct {
		aborted bool
		reason  any
		err     error
	}
	reads := make(chan readResult, readerTotal)
	var group sync.WaitGroup
	var handlerCalls atomic.Int32
	var reasonMismatches atomic.Int32

	for range handlerTotal {
		group.Go(func() {
			ready <- struct{}{}
			<-start
			signal.OnAbort(func(got any) {
				if got != reason {
					reasonMismatches.Add(1)
				}
				handlerCalls.Add(1)
			})
		})
	}
	for range readerTotal {
		group.Go(func() {
			ready <- struct{}{}
			<-start
			reads <- readResult{
				aborted: signal.Aborted(),
				reason:  signal.Reason(),
				err:     signal.ThrowIfAborted(),
			}
		})
	}
	group.Go(func() {
		ready <- struct{}{}
		<-start
		controller.Abort(reason)
	})
	for range handlerTotal + readerTotal + 1 {
		waitContractSignal(t, ready, "concurrent AbortSignal worker readiness")
	}
	startNow()
	workersDone := make(chan struct{})
	go func() {
		group.Wait()
		close(workersDone)
	}()
	waitContractSignal(t, workersDone, "concurrent AbortSignal operations")

	for range readerTotal {
		result := waitContractValue(t, reads, "concurrent AbortSignal read result")
		if result.reason != nil && result.reason != reason {
			t.Fatalf("concurrent Reason = %#v, want nil or %q", result.reason, reason)
		}
		if result.err != nil {
			abortErr, ok := result.err.(*AbortError)
			if !ok || abortErr.Reason != reason {
				t.Fatalf("concurrent ThrowIfAborted = %#v, want nil or AbortError with reason %q", result.err, reason)
			}
		}
		if result.aborted && (result.reason != reason || result.err == nil) {
			t.Fatalf("reads after observed abort = (reason=%#v, err=%#v), want exact settled reason and error", result.reason, result.err)
		}
		if result.reason == reason && result.err == nil {
			t.Fatal("ThrowIfAborted returned nil after Reason observed settlement")
		}
		if result.err == nil && (result.aborted || result.reason != nil) {
			t.Fatalf("nil ThrowIfAborted followed earlier settled observation: (aborted=%v, reason=%#v)", result.aborted, result.reason)
		}
	}

	if !signal.Aborted() || signal.Reason() != reason {
		t.Fatalf("final signal = (aborted=%v, reason=%#v), want (true, %q)", signal.Aborted(), signal.Reason(), reason)
	}
	if got := handlerCalls.Load(); got != handlerTotal {
		t.Fatalf("handler calls = %d, want %d", got, handlerTotal)
	}
	if got := reasonMismatches.Load(); got != 0 {
		t.Fatalf("handler reason mismatches = %d, want 0", got)
	}
}

func TestAbortSignalThrowIfAbortedPreservesErrorIdentity(t *testing.T) {
	reason := errors.New("stop")
	controller := NewAbortController()
	controller.Abort(reason)

	if got := controller.Signal().Reason(); got != reason {
		t.Fatalf("Reason = %#v, want exact error %#v", got, reason)
	}
	if got := controller.Signal().ThrowIfAborted(); got != reason {
		t.Fatalf("ThrowIfAborted = %#v, want exact error %#v", got, reason)
	}

	controller = NewAbortController()
	controller.Abort("stop")
	err := controller.Signal().ThrowIfAborted()
	abortErr, ok := err.(*AbortError)
	if !ok {
		t.Fatalf("ThrowIfAborted = %T, want *AbortError", err)
	}
	if abortErr.Reason != "stop" {
		t.Fatalf("AbortError.Reason = %#v, want %q", abortErr.Reason, "stop")
	}
	if _, nested := abortErr.Reason.(*AbortError); nested {
		t.Fatal("ThrowIfAborted nested an AbortError")
	}
}

func TestAbortSignalThrowIfAbortedReturnsDefaultReason(t *testing.T) {
	controller := NewAbortController()
	controller.Abort(nil)
	reason, ok := controller.Signal().Reason().(*AbortError)
	if !ok {
		t.Fatalf("Reason = %T, want *AbortError", controller.Signal().Reason())
	}
	if got := controller.Signal().ThrowIfAborted(); got != reason {
		t.Fatalf("ThrowIfAborted = %#v, want exact default reason %#v", got, reason)
	}
}

func TestAbortSignalSettlementRunsFIFOAndReraisesFirstPanic(t *testing.T) {
	controller := NewAbortController()
	signal := controller.Signal()
	firstMarker := errors.New("first handler panic")
	secondMarker := errors.New("second handler panic")
	order := make([]int, 0, 4)

	signal.OnAbort(func(any) {
		order = append(order, 1)
		signal.OnAbort(func(any) {
			order = append(order, 3)
		})
		panic(firstMarker)
	})
	signal.OnAbort(func(any) {
		order = append(order, 2)
		panic(secondMarker)
	})
	signal.OnAbort(func(any) {
		order = append(order, 4)
	})

	if got := abortEventCapturePanic(func() { controller.Abort("stop") }); got != firstMarker {
		t.Fatalf("panic = %#v, want first panic %#v", got, firstMarker)
	}
	if want := []int{1, 2, 4, 3}; !reflect.DeepEqual(order, want) {
		t.Fatalf("handler order = %v, want %v", order, want)
	}
	if got := len(signal.handlers); got != 0 {
		t.Fatalf("retained handlers after panic = %d, want 0", got)
	}
}

func TestAbortSignalEarlierPanicWinsLaterGoexit(t *testing.T) {
	controller := NewAbortController()
	marker := errors.New("first handler panic")
	goexitEntered := atomic.Bool{}
	laterCalls := atomic.Int32{}
	controller.Signal().OnAbort(func(any) { panic(marker) })
	controller.Signal().OnAbort(func(any) {
		goexitEntered.Store(true)
		runtime.Goexit()
	})
	controller.Signal().OnAbort(func(any) { laterCalls.Add(1) })

	type result struct {
		panicValue any
		returned   bool
	}
	results := make(chan result, 1)
	go func() {
		returned := false
		defer func() {
			results <- result{panicValue: recover(), returned: returned}
		}()
		controller.Abort("stop")
		returned = true
	}()

	got := waitAbortContractValue(t, results, "panic followed by Goexit")
	if got.panicValue != marker {
		t.Fatalf("panic after later Goexit = %#v, want first panic %#v (returned=%v)", got.panicValue, marker, got.returned)
	}
	if got.returned {
		t.Fatal("Abort returned after a handler called runtime.Goexit")
	}
	if !goexitEntered.Load() {
		t.Fatal("Goexit handler did not enter before the first panic was re-raised")
	}
	if got := laterCalls.Load(); got != 0 {
		t.Fatalf("handlers after Goexit = %d, want 0", got)
	}
	if controller.Signal().dispatching {
		t.Fatal("signal remained dispatching after panic/Goexit precedence")
	}
}

func TestAbortSignalCleanupPanicWinsHandlerGoexit(t *testing.T) {
	signal := newAbortSignal()
	marker := errors.New("cleanup panic")
	goexitEntered := atomic.Bool{}
	signal.OnAbort(func(any) {
		goexitEntered.Store(true)
		runtime.Goexit()
	})
	dispatch, ok := signal.beginAbort("stop", func() { panic(marker) })
	if !ok {
		t.Fatal("beginAbort did not claim an unsettled signal")
	}

	panicValues := make(chan any, 1)
	go func() {
		defer func() { panicValues <- recover() }()
		runAbortDispatch(dispatch)
	}()
	if got := waitAbortContractValue(t, panicValues, "cleanup panic followed by handler Goexit"); got != marker {
		t.Fatalf("panic after handler Goexit = %#v, want cleanup panic %#v", got, marker)
	}
	if !goexitEntered.Load() {
		t.Fatal("handler Goexit did not enter before the cleanup panic was re-raised")
	}
	if signal.dispatching {
		t.Fatal("signal remained dispatching after cleanup panic/handler Goexit")
	}
}

func TestAbortSignalCleanupPanicStillDeliversHandlers(t *testing.T) {
	signal := newAbortSignal()
	marker := errors.New("cleanup panic")
	var calls atomic.Int32
	signal.OnAbort(func(any) { calls.Add(1) })
	signal.OnAbort(func(any) { calls.Add(1) })
	dispatch, ok := signal.beginAbort("stop", func() { panic(marker) })
	if !ok {
		t.Fatal("beginAbort did not claim an unsettled signal")
	}
	if got := abortEventCapturePanic(func() { runAbortDispatch(dispatch) }); got != marker {
		t.Fatalf("panic = %#v, want cleanup panic %#v", got, marker)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("handler calls = %d, want 2 after cleanup panic", got)
	}
	if signal.dispatching {
		t.Fatal("signal remained dispatching after cleanup panic")
	}
}

func TestAbortDispatchDefensiveCleanup(t *testing.T) {
	t.Run("nil flatten", func(t *testing.T) {
		marker := &abortDispatch{}
		got := appendAbortDispatches([]*abortDispatch{marker}, nil)
		if len(got) != 1 || got[0] != marker {
			t.Fatalf("appendAbortDispatches with nil = %#v, want unchanged marker", got)
		}
	})

	t.Run("abandon", func(t *testing.T) {
		signal := newAbortSignal()
		current := &abortHandler{handler: func(any) {}}
		queued := &abortHandler{handler: func(any) {}}
		current.active.Store(true)
		queued.active.Store(true)
		signal.handlers = []*abortHandler{queued}
		signal.dispatching = true
		dispatch := &abortDispatch{
			signal:   signal,
			cleanup:  func() {},
			handlers: []*abortHandler{current},
		}

		dispatch.abandon()

		if !dispatch.completed || dispatch.cleanup != nil || dispatch.handlers != nil {
			t.Fatalf("abandoned dispatch = %#v, want completed with released callbacks", dispatch)
		}
		if signal.dispatching || signal.handlers != nil {
			t.Fatalf("abandoned signal state = dispatching %v, handlers %#v", signal.dispatching, signal.handlers)
		}
		if current.active.Load() || queued.active.Load() {
			t.Fatal("abandon left a current or queued handler active")
		}
		dispatch.abandon()
		var nilDispatch *abortDispatch
		nilDispatch.abandon()
	})
}

func TestAbortSignalGoexitStillReleasesDispatchState(t *testing.T) {
	controller := NewAbortController()
	signal := controller.Signal()
	returned := make(chan struct{})
	signal.OnAbort(func(any) {
		runtime.Goexit()
	})
	signal.OnAbort(func(any) {
		t.Error("handler after Goexit ran")
	})
	go func() {
		defer close(returned)
		controller.Abort("stop")
	}()
	waitAbortContractSignal(t, returned, "AbortSignal Goexit completion")

	signal.mu.RLock()
	dispatching := signal.dispatching
	handlerCount := len(signal.handlers)
	signal.mu.RUnlock()
	if dispatching || handlerCount != 0 {
		t.Fatalf("post-Goexit state = (dispatching=%v handlers=%d), want false/0", dispatching, handlerCount)
	}
	lateCalled := false
	signal.OnAbort(func(any) {
		lateCalled = true
	})
	if !lateCalled {
		t.Fatal("late handler was stranded after Goexit")
	}
}

func TestAbortSignalLateHandlerPanicPropagates(t *testing.T) {
	controller := NewAbortController()
	controller.Abort("reason")
	marker := &struct{ label string }{label: "late handler"}

	got := abortEventCapturePanic(func() {
		controller.Signal().OnAbort(func(any) { panic(marker) })
	})
	if got != marker {
		t.Fatalf("late handler panic = %#v, want marker %#v", got, marker)
	}

	called := false
	controller.Signal().OnAbort(func(any) { called = true })
	if !called {
		t.Fatal("late handler panic stranded a subsequent handler")
	}
}
