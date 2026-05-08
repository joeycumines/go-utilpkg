package eventloop

import (
	"runtime"
	"sync"
	"testing"
	"time"
	"weak"
)

func abortEventCapturePanic(fn func()) (value any) {
	defer func() {
		value = recover()
	}()
	fn()
	return nil
}
func waitAbortContractSignal(t *testing.T, signal <-chan struct{}, operation string) {
	t.Helper()
	waitAbortContractValue(t, signal, operation)
}

func abortContractRelease(t *testing.T, signal chan struct{}) func() {
	t.Helper()
	return contractRelease(t, signal)
}

func contractRelease(t *testing.T, signal chan struct{}) func() {
	t.Helper()
	var once sync.Once
	release := func() { once.Do(func() { close(signal) }) }
	t.Cleanup(release)
	return release
}

func waitAbortContractValue[T any](t *testing.T, values <-chan T, operation string) T {
	t.Helper()
	return waitContractValue(t, values, operation)
}

func waitContractSignal(t *testing.T, signal <-chan struct{}, operation string) {
	t.Helper()
	waitContractValue(t, signal, operation)
}

func waitContractValue[T any](t *testing.T, values <-chan T, operation string) T {
	t.Helper()
	value, _ := waitContractReceive(t, values, operation)
	return value
}

func waitLoopOwnerTurnT(t *testing.T, loop *Loop) {
	t.Helper()
	reached := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(reached) }); err != nil {
		t.Fatalf("SubmitInternal owner-turn barrier: %v", err)
	}
	waitContractSignal(t, reached, "loop owner turn")
}

func newIdleWaitBoundaryHooks() (*loopTestHooks, <-chan struct{}) {
	reached := make(chan struct{})
	var once sync.Once
	signal := func() { once.Do(func() { close(reached) }) }
	hooks := &loopTestHooks{}
	if fdPollingSupported {
		hooks.BeforePollIO = signal
	} else {
		hooks.BeforeFastPollWait = func(int) { signal() }
	}
	return hooks, reached
}

func waitContractReceive[T any](t *testing.T, values <-chan T, operation string) (T, bool) {
	t.Helper()
	select {
	case value, open := <-values:
		return value, open
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", operation)
		var zero T
		return zero, false
	}
}

func waitContractCollected[T any](t *testing.T, pointer weak.Pointer[T], keepAlive any) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		runtime.GC()
		if pointer.Value() == nil {
			runtime.KeepAlive(keepAlive)
			return
		}
		runtime.KeepAlive(keepAlive)
		if time.Now().After(deadline) {
			t.Fatal("retained object was not collected")
		}
		runtime.Gosched()
	}
}

// waitRefedTimerCount places an owner-turn barrier behind all previously
// accepted timer lifecycle commands, then reads the owner-maintained count.
func waitRefedTimerCount(t *testing.T, loop *Loop, expected int64) {
	t.Helper()
	observed := make(chan int64, 1)
	if err := loop.SubmitInternal(func() { observed <- loop.refedTimerCount.Load() }); err != nil {
		t.Fatalf("SubmitInternal timer-count barrier: %v", err)
	}
	if got := waitContractValue(t, observed, "timer-count owner barrier"); got != expected {
		t.Fatalf("refed timer count after owner barrier = %d, want %d", got, expected)
	}
}
