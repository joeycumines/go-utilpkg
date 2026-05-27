package gojaeventloop

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func newBoundDelayAdapter(
	t *testing.T,
	options ...goeventloop.LoopOption,
) (*goeventloop.Loop, *goja.Runtime, *Adapter) {
	t.Helper()
	loop, err := goeventloop.New(options...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loop.Close() })
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	return loop, runtime, adapter
}

func exportedDelayPromise(t *testing.T, value goja.Value) *goja.Promise {
	t.Helper()
	object, ok := value.(*goja.Object)
	if !ok || object == nil {
		t.Fatalf("delay result = %T, want *goja.Object", value)
	}
	promise, ok := object.Export().(*goja.Promise)
	if !ok || promise == nil {
		t.Fatalf("delay export = %T, want *goja.Promise", object.Export())
	}
	return promise
}

func assertFulfilledDelay(t *testing.T, promise *goja.Promise) {
	t.Helper()
	if got := promise.State(); got != goja.PromiseStateFulfilled {
		t.Fatalf("delay state = %v, want fulfilled", got)
	}
	if result := promise.Result(); !goja.IsUndefined(result) {
		t.Fatalf("delay result = %v, want undefined", result)
	}
}

func awaitDelayLoopActive(t *testing.T, loop *goeventloop.Loop) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		state := loop.State()
		if state == goeventloop.StateRunning || state == goeventloop.StateSleeping {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("loop state = %v, want running or sleeping", state)
		}
		runtime.Gosched()
	}
}

func awaitDelayResult(t *testing.T, result <-chan error, operation string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not complete", operation)
		return nil
	}
}

func TestDelayNativePromiseIsAsynchronousAndRefed(t *testing.T) {
	loop, runtime, adapter := newBoundDelayAdapter(t, goeventloop.WithAutoExit(true))
	value, err := runtime.RunString(`
		globalThis.delayReactionCount = 0;
		globalThis.delayReactionValue = "unset";
		globalThis.delayPromise = delay(0);
		delayPromise.then(function(value) {
			delayReactionCount++;
			delayReactionValue = value;
		});
		delayPromise;
	`)
	if err != nil {
		t.Fatal(err)
	}
	promise := exportedDelayPromise(t, value)
	if got := promise.State(); got != goja.PromiseStatePending {
		t.Fatalf("pre-Run delay state = %v, want pending", got)
	}
	if got := runtime.Get("delayReactionCount").ToInteger(); got != 0 {
		t.Fatalf("pre-Run reaction count = %d, want 0", got)
	}

	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertFulfilledDelay(t, promise)
	if got := runtime.Get("delayReactionCount").ToInteger(); got != 1 {
		t.Fatalf("reaction count = %d, want 1", got)
	}
	if value := runtime.Get("delayReactionValue"); !goja.IsUndefined(value) {
		t.Fatalf("reaction value = %v, want undefined", value)
	}
	if adapter.delayHead != nil {
		t.Fatal("settled delay remains registered")
	}
}

func TestDelayTerminalCleanupFulfillsAcceptedPromise(t *testing.T) {
	for _, terminal := range []struct {
		name    string
		request func(*goeventloop.Loop) error
	}{
		{name: "Shutdown", request: func(loop *goeventloop.Loop) error {
			return loop.Shutdown(context.Background())
		}},
		{name: "Close", request: func(loop *goeventloop.Loop) error {
			return loop.Close()
		}},
	} {
		t.Run(terminal.name, func(t *testing.T) {
			loop, runtime, adapter := newBoundDelayAdapter(t)
			value, err := runtime.RunString(`
				globalThis.delayTerminalReactionCount = 0;
				globalThis.delayTerminalPromise = delay(60000);
				delayTerminalPromise.then(function() { delayTerminalReactionCount++; });
				delayTerminalPromise;
			`)
			if err != nil {
				t.Fatal(err)
			}
			promise := exportedDelayPromise(t, value)
			if promise.State() != goja.PromiseStatePending || adapter.delayHead == nil {
				t.Fatal("accepted delay was not pending and registered")
			}

			if err := terminal.request(loop); err != nil {
				t.Fatalf("%s: %v", terminal.name, err)
			}
			assertFulfilledDelay(t, promise)
			if got := runtime.Get("delayTerminalReactionCount").ToInteger(); got != 0 {
				t.Fatalf("terminal reaction count = %d, want 0", got)
			}
			if adapter.delayHead != nil {
				t.Fatal("terminal cleanup retained delay state")
			}
		})
	}
}

func TestDelayTerminalCleanupWhileRunning(t *testing.T) {
	for _, terminal := range []struct {
		name    string
		request func(*goeventloop.Loop) error
	}{
		{name: "Shutdown", request: func(loop *goeventloop.Loop) error {
			return loop.Shutdown(context.Background())
		}},
		{name: "Close", request: func(loop *goeventloop.Loop) error {
			return loop.Close()
		}},
	} {
		t.Run(terminal.name, func(t *testing.T) {
			loop, runtime, adapter := newBoundDelayAdapter(t)
			value, err := runtime.RunString(`
				globalThis.delayRunningReactionCount = 0;
				globalThis.delayRunningPromise = delay(60000);
				delayRunningPromise.then(function() { delayRunningReactionCount++; });
				delayRunningPromise;
			`)
			if err != nil {
				t.Fatal(err)
			}
			promise := exportedDelayPromise(t, value)
			runResult := make(chan error, 1)
			go func() { runResult <- loop.Run(context.Background()) }()
			awaitDelayLoopActive(t, loop)

			if err := terminal.request(loop); err != nil {
				t.Fatalf("%s: %v", terminal.name, err)
			}
			if err := awaitDelayResult(t, runResult, "Run"); err != nil &&
				!errors.Is(err, goeventloop.ErrLoopTerminated) {
				t.Fatalf("Run: %v", err)
			}
			assertFulfilledDelay(t, promise)
			if got := runtime.Get("delayRunningReactionCount").ToInteger(); got != 0 {
				t.Fatalf("terminal reaction count = %d, want 0", got)
			}
			if adapter.delayHead != nil {
				t.Fatal("terminal cleanup retained delay state")
			}
		})
	}
}

func TestDelayPostTerminalRejectsImmediately(t *testing.T) {
	loop, runtime, adapter := newBoundDelayAdapter(t)
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	value, err := runtime.RunString(`
		globalThis.delayPostTerminalReactionCount = 0;
		globalThis.delayPostTerminalPromise = delay(0);
		delayPostTerminalPromise.catch(function() { delayPostTerminalReactionCount++; });
		delayPostTerminalPromise;
	`)
	if err != nil {
		t.Fatal(err)
	}
	promise := exportedDelayPromise(t, value)
	if got := promise.State(); got != goja.PromiseStateRejected {
		t.Fatalf("post-terminal delay state = %v, want rejected", got)
	}
	reason, ok := promise.Result().Export().(error)
	if !ok || !errors.Is(reason, goeventloop.ErrLoopTerminated) {
		t.Fatalf("post-terminal reason = %#v, want ErrLoopTerminated", promise.Result().Export())
	}
	if got := runtime.Get("delayPostTerminalReactionCount").ToInteger(); got != 0 {
		t.Fatalf("post-terminal reaction count = %d, want 0", got)
	}
	if adapter.delayHead != nil {
		t.Fatal("post-terminal delay was registered")
	}
}

func TestDelaySettlementClaimOrdering(t *testing.T) {
	newState := func(calls *int) *adapterDelayState {
		return &adapterDelayState{
			resolve: func(any) error {
				*calls++
				return nil
			},
			reject: func(any) error {
				*calls++
				return nil
			},
		}
	}
	t.Run("expiry then cleanup", func(t *testing.T) {
		adapter := new(Adapter)
		calls := 0
		state := newState(&calls)
		adapter.registerDelay(state)
		if !adapter.finishDelay(state, false, goja.Undefined()) {
			t.Fatal("expiry did not claim delay")
		}
		if states := adapter.takeDelayStates(); len(states) != 0 {
			t.Fatalf("cleanup collected %d settled delays", len(states))
		}
		if calls != 1 || adapter.delayHead != nil || state.linked || state.resolve != nil || state.reject != nil {
			t.Fatalf("settled state = calls:%d head:%p state:%+v", calls, adapter.delayHead, state)
		}
	})
	t.Run("cleanup then stale expiry", func(t *testing.T) {
		adapter := new(Adapter)
		calls := 0
		state := newState(&calls)
		adapter.registerDelay(state)
		states := adapter.takeDelayStates()
		if len(states) != 1 || states[0] != state {
			t.Fatalf("cleanup states = %#v", states)
		}
		adapter.settleClaimedDelay(state, false, goja.Undefined())
		if adapter.finishDelay(state, true, errors.New("stale")) {
			t.Fatal("stale expiry claimed cleaned delay")
		}
		if calls != 1 || adapter.delayHead != nil || state.linked || state.resolve != nil || state.reject != nil {
			t.Fatalf("cleaned state = calls:%d head:%p state:%+v", calls, adapter.delayHead, state)
		}
	})
	t.Run("expiry twice", func(t *testing.T) {
		adapter := new(Adapter)
		calls := 0
		state := newState(&calls)
		adapter.registerDelay(state)
		if !adapter.finishDelay(state, false, goja.Undefined()) {
			t.Fatal("first expiry did not claim delay")
		}
		if adapter.finishDelay(state, true, errors.New("duplicate")) {
			t.Fatal("duplicate expiry claimed delay")
		}
		if calls != 1 {
			t.Fatalf("settlement calls = %d, want 1", calls)
		}
	})
}
