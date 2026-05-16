package tournament

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/go-eventloop/internal/tournamenttest"
)

func TestPromiseRaceZeroInputs(t *testing.T) {
	for _, implementation := range PromiseImplementations() {
		if implementation.Race == nil {
			continue
		}
		t.Run(implementation.Name, func(t *testing.T) {
			_, js := startPromiseAdapterTestLoop(t)
			promise, settle := implementation.Race(js, 0)
			if promise == nil || settle == nil {
				t.Fatal("Race(0) returned a nil promise or settlement function")
			}
			drained, err := settle()
			if err != nil {
				t.Fatalf("settle Race(0): %v", err)
			}
			waitPromiseAdapterTestSignal(t, drained, "Race(0) settlement barrier")
		})
	}
}

func TestPromiseRaceSettlementDrainsEveryInput(t *testing.T) {
	for _, implementation := range PromiseImplementations() {
		if implementation.Race == nil {
			continue
		}
		t.Run(implementation.Name, func(t *testing.T) {
			_, js := startPromiseAdapterTestLoop(t)
			promise, settle := implementation.Race(js, 100)
			observed := make(chan any, 1)
			promise.Then(func(value any) any {
				observed <- value
				return value
			}, func(reason any) any {
				observed <- reason
				return reason
			})
			drained, err := settle()
			if err != nil {
				t.Fatalf("settle Race(100): %v", err)
			}
			waitPromiseAdapterTestSignal(t, drained, "Race(100) input-reaction barrier")
			select {
			case value := <-observed:
				if value != 1 {
					t.Fatalf("Race(100) result = %#v, want 1", value)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Race(100) result observation timed out")
			}
		})
	}
}

func TestSettlePromiseRaceInputsSettlesEveryInput(t *testing.T) {
	_, js := startPromiseAdapterTestLoop(t)
	calls := make([]int, 100)
	resolvers := make([]eventloop.ResolveFunc, len(calls))
	for index := range resolvers {
		resolvers[index] = func(any) { calls[index]++ }
	}
	drained, err := settlePromiseRaceInputs(js, resolvers)()
	if err != nil {
		t.Fatalf("settle race inputs: %v", err)
	}
	waitPromiseAdapterTestSignal(t, drained, "race input settlement barrier")
	for index, count := range calls {
		if count != 1 {
			t.Fatalf("resolver %d call count = %d, want 1", index, count)
		}
	}
}

func startPromiseAdapterTestLoop(t *testing.T) (*eventloop.Loop, *eventloop.JS) {
	t.Helper()
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	t.Cleanup(func() {
		result := tournamenttest.Terminate(loop, runDone, 5*time.Second)
		if result.ShutdownErr != nil && !errors.Is(result.ShutdownErr, eventloop.ErrLoopTerminated) {
			t.Errorf("Shutdown() failed: %v", result.ShutdownErr)
		}
		if result.CloseErr != nil && !errors.Is(result.CloseErr, eventloop.ErrLoopTerminated) {
			t.Errorf("fallback Close() failed: %v", result.CloseErr)
		}
		if result.RunErr != nil {
			t.Errorf("Run() failed: %v", result.RunErr)
		}
	})
	ready := make(chan struct{})
	if err := loop.Submit(func() { close(ready) }); err != nil {
		t.Fatalf("Submit readiness callback: %v", err)
	}
	waitPromiseAdapterTestSignal(t, ready, "loop readiness")
	js, err := eventloop.NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS: %v", err)
	}
	return loop, js
}

func waitPromiseAdapterTestSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s timed out", name)
	}
}
