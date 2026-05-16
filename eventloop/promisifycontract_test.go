package eventloop

import (
	"context"
	"testing"
	"time"
)

func TestPromisifyPreCanceledContextSkipsUserFunction(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	taskContext, cancel := context.WithCancel(context.Background())
	cancel()
	called := make(chan struct{}, 1)
	promise := loop.Promisify(taskContext, func(context.Context) (any, error) {
		called <- struct{}{}
		return "unexpected", nil
	})

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	if err := waitContractValue(t, runDone, "pre-canceled Promisify Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitPromisifyWorkersT(t, loop)
	assertPromisifyExactRejection(t, promise, context.Canceled)
	select {
	case <-called:
		t.Fatal("pre-canceled Promisify invoked the user function")
	default:
	}
	if got := loop.promisifyCount.Load(); got != 0 {
		t.Fatalf("promisifyCount = %d, want 0", got)
	}
}

func TestPromisifyActiveContextCancellation(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	taskContext, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	entered := make(chan context.Context, 1)
	returned := make(chan struct{})
	promise := loop.Promisify(taskContext, func(ctx context.Context) (any, error) {
		entered <- ctx
		<-ctx.Done()
		close(returned)
		return nil, ctx.Err()
	})
	if got := waitContractValue(t, entered, "Promisify user-function entry"); got != taskContext {
		t.Fatalf("Promisify context = %T %#v, want original context %T %#v", got, got, taskContext, taskContext)
	}
	cancel()
	waitContractSignal(t, returned, "context-canceled Promisify user-function return")

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	if err := waitContractValue(t, runDone, "context-canceled Promisify Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitPromisifyWorkersT(t, loop)
	assertPromisifyExactRejection(t, promise, context.Canceled)
	if got := loop.promisifyCount.Load(); got != 0 {
		t.Fatalf("promisifyCount = %d, want 0", got)
	}
}

func TestPromisifyDeadlineExceeded(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	taskContext, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	t.Cleanup(cancel)
	entered := make(chan context.Context, 1)
	returned := make(chan struct{})
	promise := loop.Promisify(taskContext, func(ctx context.Context) (any, error) {
		entered <- ctx
		<-ctx.Done()
		close(returned)
		return nil, ctx.Err()
	})
	if got := waitContractValue(t, entered, "deadline-bound Promisify user-function entry"); got != taskContext {
		t.Fatalf("Promisify context = %T %#v, want original context %T %#v", got, got, taskContext, taskContext)
	}
	waitContractSignal(t, returned, "deadline-bound Promisify user-function return")

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	if err := waitContractValue(t, runDone, "deadline-bound Promisify Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitPromisifyWorkersT(t, loop)
	assertPromisifyExactRejection(t, promise, context.DeadlineExceeded)
	if got := loop.promisifyCount.Load(); got != 0 {
		t.Fatalf("promisifyCount = %d, want 0", got)
	}
}

func TestPromisifyLivenessControlsAutoExit(t *testing.T) {
	if !fdPollingSupported {
		t.Skip("deterministic native-poll boundary is unavailable")
	}
	loop, err := New(WithAutoExit(true), WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	workerStarted := make(chan struct{})
	releaseWorker := make(chan struct{})
	releaseWorkerFn := releaseSignalT(t, releaseWorker)
	resultToken := &struct{ label string }{label: "Promisify result"}
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		close(workerStarted)
		<-releaseWorker
		return resultToken, nil
	})
	waitContractSignal(t, workerStarted, "Promisify worker entry")
	if got := loop.promisifyCount.Load(); got != 1 {
		t.Fatalf("promisifyCount = %d, want 1", got)
	}
	if !loop.Alive() || !loop.HasMacrotaskWork() {
		t.Fatal("in-flight Promisify worker did not publish liveness")
	}

	pollBoundary := make(chan struct{}, 1)
	loop.testHooks = &loopTestHooks{
		BeforePollIO: func() {
			select {
			case pollBoundary <- struct{}{}:
			default:
			}
		},
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, pollBoundary, "post-auto-exit-check native poll boundary")
	select {
	case err := <-runDone:
		t.Fatalf("Run auto-exited while Promisify worker was blocked: %v", err)
	default:
	}
	if !loop.Alive() || !loop.HasMacrotaskWork() {
		t.Fatal("Promisify liveness disappeared while its worker was blocked")
	}

	releaseWorkerFn()
	if err := waitContractValue(t, runDone, "Promisify-controlled auto-exit completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if state := promise.State(); state != Fulfilled || promise.Result() != resultToken {
		t.Fatalf("Promisify settlement = (%v, %T %#v), want Fulfilled result identity %p", state, promise.Result(), promise.Result(), resultToken)
	}
	if got := loop.promisifyCount.Load(); got != 0 {
		t.Fatalf("promisifyCount = %d, want 0", got)
	}
	if loop.Alive() || loop.HasMacrotaskWork() {
		t.Fatal("Promisify liveness remained after worker completion and auto-exit")
	}
}

func assertPromisifyExactRejection(t *testing.T, promise Promise, want error) {
	t.Helper()
	if state := promise.State(); state != Rejected {
		t.Fatalf("Promisify state = %v, want Rejected", state)
	}
	if got, ok := promise.Result().(error); !ok || got != want {
		t.Fatalf("Promisify result = %T %v, want exact %T %v", promise.Result(), promise.Result(), want, want)
	}
}
