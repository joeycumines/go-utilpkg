package eventloop

import (
	"runtime"
	"testing"
	"time"
	"weak"
)

type contractRetentionPayload struct {
	value int
	_     [32]byte
}

func newSettledSignalPayload() (*AbortSignal, weak.Pointer[contractRetentionPayload]) {
	payload := &contractRetentionPayload{value: 1}
	pointer := weak.Make(payload)
	controller := NewAbortController()
	signal := controller.Signal()
	signal.OnAbort(func(any) {
		payload.value++
	})
	controller.Abort("done")
	runtime.KeepAlive(payload)
	return signal, pointer
}

func newSettledCompositePointer() ([]*AbortSignal, weak.Pointer[AbortSignal]) {
	first := NewAbortController()
	second := NewAbortController()
	composite := AbortAny([]*AbortSignal{first.Signal(), second.Signal()})
	pointer := weak.Make(composite)
	first.Abort("done")
	runtime.KeepAlive(composite)
	return []*AbortSignal{first.Signal(), second.Signal()}, pointer
}

func newAbandonedPendingCompositePointer(signals []*AbortSignal) weak.Pointer[AbortSignal] {
	composite := AbortAny(signals)
	pointer := weak.Make(composite)
	runtime.KeepAlive(composite)
	return pointer
}

func pendingAbortAlgorithmCount(signal *AbortSignal) int {
	signal.mu.RLock()
	count := len(signal.algorithms)
	signal.mu.RUnlock()
	return count
}

func newSettledTimeoutLoop(t *testing.T, manual bool) (*AbortController, weak.Pointer[Loop]) {
	t.Helper()
	loop := New(WithAutoExit(true))
	pointer := weak.Make(loop)
	delay := 0
	if manual {
		delay = int(time.Hour / time.Millisecond)
	}
	controller, err := AbortTimeout(loop, delay)
	if err != nil {
		t.Fatal(err)
	}
	if manual {
		controller.Abort("manual")
		controller.timeoutState.mu.Lock()
		retainedLoop := controller.timeoutState.loop
		controller.timeoutState.mu.Unlock()
		if retainedLoop != nil {
			t.Fatal("manual timeout settlement retained its loop before terminal cleanup")
		}
	}
	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	runtime.KeepAlive(loop)
	return controller, pointer
}

func newTerminalTimeoutLoop(t *testing.T, running, graceful bool) (*AbortController, weak.Pointer[Loop]) {
	t.Helper()
	loop := New()
	pointer := weak.Make(loop)
	controller, err := AbortTimeout(loop, int(time.Hour/time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	var done chan error
	if running {
		ownerStarted := make(chan struct{})
		if err := loop.Submit(func() { close(ownerStarted) }); err != nil {
			t.Fatalf("Submit owner barrier: %v", err)
		}
		done = make(chan error, 1)
		go func() { done <- loop.Run(t.Context()) }()
		waitAbortContractSignal(t, ownerStarted, "owner callback after timer commit")
	}
	if graceful {
		if err := loop.Shutdown(t.Context()); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	} else if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if running {
		if err := waitAbortContractValue(t, done, "terminal timeout Loop.Run completion"); err != nil {
			t.Fatalf("Run: %v", err)
		}
	}
	runtime.KeepAlive(loop)
	return controller, pointer
}

func newManuallyCanceledTimeoutPointer(t *testing.T, loop *Loop) weak.Pointer[AbortSignal] {
	t.Helper()
	controller, err := AbortTimeout(loop, int(time.Hour/time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	signal := controller.Signal()
	pointer := weak.Make(signal)
	controller.Abort("manual")
	runtime.KeepAlive(signal)
	runtime.KeepAlive(controller)
	return pointer
}

func TestAbortAnyAbandonedPendingCompositesDetachSources(t *testing.T) {
	controllers := []*AbortController{NewAbortController(), NewAbortController(), NewAbortController()}
	sources := make([]*AbortSignal, len(controllers))
	for index, controller := range controllers {
		sources[index] = controller.Signal()
	}

	const compositeCount = 16
	pointers := make([]weak.Pointer[AbortSignal], compositeCount)
	for index := range pointers {
		pointers[index] = newAbandonedPendingCompositePointer(sources)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		runtime.GC()
		collected := 0
		for _, pointer := range pointers {
			if pointer.Value() == nil {
				collected++
			}
		}
		algorithmCounts := make([]int, len(sources))
		detached := true
		for index, source := range sources {
			algorithmCounts[index] = pendingAbortAlgorithmCount(source)
			detached = detached && algorithmCounts[index] == 0
		}
		if collected == compositeCount && detached {
			runtime.KeepAlive(controllers)
			return
		}
		runtime.KeepAlive(controllers)
		if time.Now().After(deadline) {
			t.Fatalf("abandoned AbortAny composites retained: collected=%d/%d source algorithms=%v", collected, compositeCount, algorithmCounts)
		}
		runtime.Gosched()
	}
}
