package eventloop

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestClosePromisifyConcurrentShutdownCloseWinner(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	releaseWorker := make(chan struct{})
	var releaseWorkerOnce sync.Once
	releaseWorkerFn := func() { releaseWorkerOnce.Do(func() { close(releaseWorker) }) }
	t.Cleanup(releaseWorkerFn)
	workerStarted := make(chan struct{})
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		close(workerStarted)
		<-releaseWorker
		return "released", nil
	})
	select {
	case <-workerStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify worker did not start")
	}

	closeTransitioned := make(chan struct{})
	releaseClose := make(chan struct{})
	terminalJoined := make(chan struct{})
	var releaseCloseOnce sync.Once
	var joinedOnce sync.Once
	releaseCloseFn := func() { releaseCloseOnce.Do(func() { close(releaseClose) }) }
	t.Cleanup(releaseCloseFn)
	loop.testHooks = &loopTestHooks{
		AfterCloseStateTerminating: func() {
			close(closeTransitioned)
			<-releaseClose
		},
		BeforeTerminalJoin: func() {
			joinedOnce.Do(func() { close(terminalJoined) })
		},
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	select {
	case <-closeTransitioned:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not win StateTerminating")
	}

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(context.Background()) }()
	waitContractSignal(t, terminalJoined, "Shutdown join of winning Close")
	select {
	case err := <-shutdownDone:
		t.Fatalf("Shutdown returned before winning Close completed: %v", err)
	default:
	}

	releaseCloseFn()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("winning Close waited for blocked Promisify worker")
	}
	if err := waitContractValue(t, shutdownDone, "joined Shutdown completion"); err != nil {
		t.Fatalf("joined Shutdown: %v", err)
	}
	assertPromiseRejected(t, promise, ErrLoopTerminated)
	assertCloseSignals(t, loop)

	releaseWorkerFn()
	waitPromisifyWorkersT(t, loop)
	assertPromiseRejected(t, promise, ErrLoopTerminated)
}

func TestClosePromisifyWorkerDoesNotJoinWinningShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	shutdownTransitioned := make(chan struct{})
	releaseShutdown := make(chan struct{})
	var releaseShutdownOnce sync.Once
	releaseShutdownFn := func() { releaseShutdownOnce.Do(func() { close(releaseShutdown) }) }
	t.Cleanup(releaseShutdownFn)
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() {
			close(shutdownTransitioned)
			<-releaseShutdown
		},
	}

	callClose := make(chan struct{})
	var callCloseOnce sync.Once
	callCloseFn := func() { callCloseOnce.Do(func() { close(callClose) }) }
	t.Cleanup(callCloseFn)
	workerStarted := make(chan struct{})
	workerReturned := make(chan struct{})
	workerCloseResult := make(chan error, 1)
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		close(workerStarted)
		<-callClose
		workerCloseResult <- loop.Close()
		close(workerReturned)
		return "released", nil
	})
	select {
	case <-workerStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify user function did not start")
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitLoopOwnerTurnT(t, loop)
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(context.Background()) }()
	select {
	case <-shutdownTransitioned:
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not win StateTerminating")
	}

	callCloseFn()
	select {
	case err := <-workerCloseResult:
		if !errors.Is(err, ErrLoopTerminated) {
			t.Fatalf("Promisify worker Close after Shutdown won = %v, want ErrLoopTerminated", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("non-winning Close joined graceful completion and waited on its own Promisify worker")
	}
	select {
	case <-workerReturned:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify user function did not return after non-winning Close")
	}

	releaseShutdownFn()
	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not complete after Promisify worker returned")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not exit during Shutdown")
	}
	select {
	case result := <-promise.ToChannel():
		if result != "released" {
			t.Fatalf("Promisify result = %v, want released", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify promise did not settle after graceful completion")
	}
}

func TestGracefulShutdownPromisifyRemainsLiveUntilWorkerReturns(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	releaseWorker := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseWorker) }) }
	t.Cleanup(release)
	workerStarted := make(chan struct{})
	workerReturned := make(chan struct{})
	shutdownTerminated := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterTerminateStateBeforeDrain: func() { close(shutdownTerminated) },
	}
	loop.Promisify(context.Background(), func(context.Context) (any, error) {
		close(workerStarted)
		<-releaseWorker
		close(workerReturned)
		return "released", nil
	})
	select {
	case <-workerStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify user function did not start")
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitLoopOwnerTurnT(t, loop)
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(context.Background()) }()
	waitContractSignal(t, shutdownTerminated, "Shutdown terminal-state publication")
	if got := loop.promisifyCount.Load(); got != 1 {
		t.Fatalf("promisifyCount during graceful completion = %d, want 1", got)
	}
	if !loop.Alive() {
		t.Fatal("Alive returned false while graceful Shutdown still had an admitted Promisify worker")
	}
	if !loop.HasMacrotaskWork() {
		t.Fatal("HasMacrotaskWork returned false while graceful Shutdown still had an admitted Promisify worker")
	}

	release()
	select {
	case <-workerReturned:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify user function did not return after release")
	}
	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not complete after Promisify worker returned")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not exit during graceful Shutdown")
	}
	if got := loop.promisifyCount.Load(); got != 0 {
		t.Fatalf("promisifyCount after graceful completion = %d, want 0", got)
	}
	if loop.Alive() {
		t.Fatal("Alive returned true after graceful Shutdown completed")
	}
	if loop.HasMacrotaskWork() {
		t.Fatal("HasMacrotaskWork returned true after graceful Shutdown completed")
	}
}
