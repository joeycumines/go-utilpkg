package eventloop

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func newImmediateTestJS(t *testing.T) (*Loop, *JS) {
	t.Helper()
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}
	return loop, js
}

func TestJSSetImmediateIDAllocation(t *testing.T) {
	_, js := newImmediateTestJS(t)

	first, err := js.SetImmediate(func() {})
	if err != nil {
		t.Fatalf("first SetImmediate: %v", err)
	}
	second, err := js.SetImmediate(func() {})
	if err != nil {
		t.Fatalf("second SetImmediate: %v", err)
	}
	if first <= 1<<48 {
		t.Fatalf("first immediate ID: got %d, want greater than %d", first, uint64(1<<48))
	}
	if second != first+1 {
		t.Fatalf("second immediate ID: got %d, want %d", second, first+1)
	}
}

func TestJSSetImmediateIDExhaustion(t *testing.T) {
	_, js := newImmediateTestJS(t)
	js.nextImmediateID.Store(maxSafeInteger)

	id, err := js.SetImmediate(func() {})
	if id != 0 || err != ErrImmediateIDExhausted {
		t.Fatalf("SetImmediate after ID exhaustion: got (%d, %v), want (0, %v)", id, err, ErrImmediateIDExhausted)
	}
}

func TestJSClearImmediateBeforeExecution(t *testing.T) {
	_, js := newImmediateTestJS(t)
	var calls atomic.Int32
	id, err := js.SetImmediate(func() { calls.Add(1) })
	if err != nil {
		t.Fatalf("SetImmediate: %v", err)
	}
	js.setImmediateMu.RLock()
	state := js.setImmediateMap[id]
	js.setImmediateMu.RUnlock()
	if state == nil {
		t.Fatal("immediate state was not published")
	}

	if err := js.ClearImmediate(id); err != nil {
		t.Fatalf("first ClearImmediate: %v", err)
	}
	if err := js.ClearImmediate(id); err != ErrImmediateNotFound {
		t.Fatalf("second ClearImmediate: got %v, want %v", err, ErrImmediateNotFound)
	}
	state.run()
	if got := calls.Load(); got != 0 {
		t.Fatalf("cleared immediate callback calls: got %d, want 0", got)
	}
}

func TestJSClearImmediateConcurrentSingleWinner(t *testing.T) {
	_, js := newImmediateTestJS(t)
	var calls atomic.Int32
	id, err := js.SetImmediate(func() { calls.Add(1) })
	if err != nil {
		t.Fatalf("SetImmediate: %v", err)
	}
	js.setImmediateMu.RLock()
	state := js.setImmediateMap[id]
	js.setImmediateMu.RUnlock()
	if state == nil {
		t.Fatal("immediate state was not published")
	}

	const callers = 16
	started := make(chan struct{}, callers)
	results := make(chan error, callers)
	js.setImmediateMu.Lock()
	for range callers {
		go func() {
			started <- struct{}{}
			results <- js.ClearImmediate(id)
		}()
	}
	for range callers {
		waitContractSignal(t, started, "concurrent ClearImmediate caller start")
	}
	js.setImmediateMu.Unlock()

	winners := 0
	for range callers {
		err := waitContractValue(t, results, "concurrent ClearImmediate result")
		switch {
		case err == nil:
			winners++
		case errors.Is(err, ErrImmediateNotFound):
		default:
			t.Fatalf("ClearImmediate result = %v, want nil or %v", err, ErrImmediateNotFound)
		}
	}
	if winners != 1 {
		t.Fatalf("successful ClearImmediate calls = %d, want 1", winners)
	}

	state.run()
	if got := calls.Load(); got != 0 {
		t.Fatalf("cleared immediate callback calls = %d, want 0", got)
	}
	js.setImmediateMu.RLock()
	_, retained := js.setImmediateMap[id]
	js.setImmediateMu.RUnlock()
	if retained {
		t.Fatal("cleared immediate remained registered")
	}
}

func TestJSClearImmediateUnknownID(t *testing.T) {
	_, js := newImmediateTestJS(t)
	if err := js.ClearImmediate(999_999); err != ErrImmediateNotFound {
		t.Fatalf("ClearImmediate unknown ID: got %v, want %v", err, ErrImmediateNotFound)
	}
}

func TestJSSetImmediateRejectsTerminatedLoop(t *testing.T) {
	loop, js := newImmediateTestJS(t)
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	id, err := js.SetImmediate(func() { t.Error("terminated immediate callback ran") })
	if id != 0 || err != ErrLoopTerminated {
		t.Fatalf("SetImmediate after Shutdown: got (%d, %v), want (0, %v)", id, err, ErrLoopTerminated)
	}
	js.setImmediateMu.RLock()
	remaining := len(js.setImmediateMap)
	js.setImmediateMu.RUnlock()
	if remaining != 0 {
		t.Fatalf("immediate registry entries after rejected admission: got %d, want 0", remaining)
	}
}

func TestJSImmediateStateRunsOnce(t *testing.T) {
	_, js := newImmediateTestJS(t)
	var calls atomic.Int32
	id, err := js.SetImmediate(func() { calls.Add(1) })
	if err != nil {
		t.Fatalf("SetImmediate: %v", err)
	}
	js.setImmediateMu.RLock()
	state := js.setImmediateMap[id]
	js.setImmediateMu.RUnlock()
	if state == nil || state.fn == nil || state.js != js || state.id != id || state.cleared.Load() {
		t.Fatalf("published immediate state = %#v, want initialized uncleared state", state)
	}

	state.run()
	state.run()
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
