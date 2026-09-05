package eventloop

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestAbortIntegrationSignalObservedAcrossPromiseChain(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	controller := NewAbortController()
	signal := controller.Signal()
	stageTwoReady := make(chan struct{})
	releaseStageTwo := make(chan struct{})
	releaseStageTwoOnce := contractRelease(t, releaseStageTwo)
	chainResults := make(chan any, 1)

	first, resolve, _ := js.NewChainedPromise()
	second := first.Then(func(value any) any { return value }, nil)
	third := second.Then(func(value any) any {
		close(stageTwoReady)
		<-releaseStageTwo
		if err := signal.ThrowIfAborted(); err != nil {
			return err
		}
		return value
	}, nil)
	third.Then(func(value any) any {
		chainResults <- value
		return value
	}, nil)

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	resolve("initial value")
	waitContractSignal(t, stageTwoReady, "second promise-chain stage")
	controller.Abort("abort during chain")
	releaseStageTwoOnce()

	result := waitContractValue(t, chainResults, "promise-chain abort observation")
	abortErr, ok := result.(*AbortError)
	if !ok {
		t.Fatalf("chain result: got %T, want *AbortError", result)
	}
	if abortErr.Reason != "abort during chain" {
		t.Fatalf("chain abort reason: got %#v, want %q", abortErr.Reason, "abort during chain")
	}

	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAbortIntegrationWithPromisify(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	running := make(chan struct{})
	if err := loop.Submit(func() { close(running) }); err != nil {
		t.Fatalf("Submit startup probe: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, running, "startup probe")

	t.Run("manual abort", func(t *testing.T) {
		controller := NewAbortController()
		operationCtx, cancelOperation := context.WithCancel(context.Background())
		t.Cleanup(cancelOperation)
		controller.Signal().OnAbort(func(any) { cancelOperation() })

		workerStarted := make(chan struct{})
		promise := loop.Promisify(operationCtx, func(ctx context.Context) (any, error) {
			close(workerStarted)
			<-ctx.Done()
			return nil, ctx.Err()
		})
		waitContractSignal(t, workerStarted, "Promisify worker entry")
		controller.Abort("abort promisify")

		result := waitContractValue(t, promise.ToChannel(), "manually aborted Promisify settlement")
		if resultErr, ok := result.(error); !ok || !errors.Is(resultErr, context.Canceled) {
			t.Fatalf("Promisify result: got %#v, want %v", result, context.Canceled)
		}
		if state := promise.State(); state != Rejected {
			t.Fatalf("Promisify state: got %v, want %v", state, Rejected)
		}
	})

	t.Run("timeout abort", func(t *testing.T) {
		operationCtx, cancelOperation := context.WithCancel(context.Background())
		t.Cleanup(cancelOperation)
		workerStarted := make(chan struct{})
		promise := loop.Promisify(operationCtx, func(ctx context.Context) (any, error) {
			close(workerStarted)
			<-ctx.Done()
			return nil, ctx.Err()
		})
		waitContractSignal(t, workerStarted, "Promisify worker entry")

		controller, err := AbortTimeout(loop, 0)
		if err != nil {
			t.Fatalf("AbortTimeout: %v", err)
		}
		controller.Signal().OnAbort(func(any) { cancelOperation() })

		result := waitContractValue(t, promise.ToChannel(), "timeout-aborted Promisify settlement")
		if resultErr, ok := result.(error); !ok || !errors.Is(resultErr, context.Canceled) {
			t.Fatalf("Promisify result: got %#v, want %v", result, context.Canceled)
		}
		if _, ok := controller.Signal().Reason().(*TimeoutError); !ok {
			t.Fatalf("timeout abort reason: got %T, want *TimeoutError", controller.Signal().Reason())
		}
	})

	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAbortIntegrationLargeHandlerList(t *testing.T) {
	controller := NewAbortController()
	const handlerCount = 10_000
	var calls atomic.Int32

	for range handlerCount {
		controller.Signal().OnAbort(func(any) { calls.Add(1) })
	}
	controller.Abort("large handler test")

	if got := calls.Load(); got != handlerCount {
		t.Fatalf("handler calls: got %d, want %d", got, handlerCount)
	}
}

func TestAbortIntegrationNestedControllers(t *testing.T) {
	outer := NewAbortController()
	inner := NewAbortController()
	outer.Signal().OnAbort(func(reason any) { inner.Abort(reason) })

	var innerCalls atomic.Int32
	inner.Signal().OnAbort(func(any) { innerCalls.Add(1) })
	outer.Abort("cascade abort")

	if got := innerCalls.Load(); got != 1 {
		t.Fatalf("inner handler calls: got %d, want 1", got)
	}
	if got := inner.Signal().Reason(); got != "cascade abort" {
		t.Fatalf("inner reason: got %#v, want %q", got, "cascade abort")
	}
}
