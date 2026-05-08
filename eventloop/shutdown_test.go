package eventloop

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/goroutineid"
)

// TestShutdownRace verifies that multiple concurrent Shutdown callers all
// return without hanging.
//
// External callers that overlap the winning Shutdown join the terminal-done
// publication and receive the stable graceful result without sharing the
// winner's context. A callback-local caller cannot join that barrier because
// its own callback may be part of the terminal worker drain; it acknowledges
// the already-committed graceful shutdown without blocking.
func TestShutdown_ReentrantFromLoopCallbackDuringExternalShutdown(t *testing.T) {
	loop := New()

	callbackStarted := make(chan struct{})
	callbackMayShutdown := make(chan struct{})
	var releaseOnce sync.Once
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() {
			releaseOnce.Do(func() { close(callbackMayShutdown) })
		},
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	callbackDone := make(chan error, 1)
	if err := loop.Submit(func() {
		close(callbackStarted)
		<-callbackMayShutdown
		callbackDone <- loop.Shutdown(context.Background())
	}); err != nil {
		t.Fatalf("Submit callback: %v", err)
	}

	select {
	case <-callbackStarted:
	case <-time.After(time.Second):
		t.Fatal("callback did not start")
	}

	externalDone := make(chan error, 1)
	go func() { externalDone <- loop.Shutdown(context.Background()) }()

	select {
	case err := <-callbackDone:
		if err != nil {
			t.Fatalf("callback-local Shutdown = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("callback-local Shutdown blocked behind external Shutdown")
	}

	select {
	case err := <-externalDone:
		if err != nil {
			t.Fatalf("external Shutdown = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("external Shutdown did not complete")
	}

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not exit after Shutdown")
	}
}

func TestShutdown_ReentrantFromLoopOwnedTerminalDrain(t *testing.T) {
	loop := New()

	blockingStarted := make(chan struct{})
	releaseBlocking := make(chan struct{})
	if err := loop.Submit(func() {
		close(blockingStarted)
		<-releaseBlocking
	}); err != nil {
		t.Fatalf("Submit blocking callback: %v", err)
	}

	type callbackResult struct {
		state    LoopState
		draining bool
		err      error
	}
	callbackDone := make(chan callbackResult, 1)

	transitioned := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() { close(transitioned) },
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case <-blockingStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("blocking callback did not start")
	}
	if err := loop.Submit(func() {
		callbackDone <- callbackResult{
			state:    loop.state.Load(),
			draining: loop.terminalDraining.Load(),
			err:      loop.Shutdown(context.Background()),
		}
	}); err != nil {
		close(releaseBlocking)
		t.Fatalf("Submit terminal-drain callback: %v", err)
	}

	externalDone := make(chan error, 1)
	go func() { externalDone <- loop.Shutdown(context.Background()) }()
	select {
	case <-transitioned:
	case <-time.After(5 * time.Second):
		close(releaseBlocking)
		t.Fatal("external Shutdown did not commit StateTerminating")
	}
	close(releaseBlocking)

	select {
	case result := <-callbackDone:
		if result.state != StateTerminated || !result.draining {
			t.Fatalf("callback lifecycle state = (%v, draining=%v), want (Terminated, true)", result.state, result.draining)
		}
		if result.err != nil {
			t.Fatalf("terminal-drain callback Shutdown = %v, want nil", result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("terminal-drain callback did not complete")
	}

	select {
	case err := <-externalDone:
		if err != nil {
			t.Fatalf("external Shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("external Shutdown did not complete")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not complete")
	}
}

func TestShutdown_LoopCallbackDuringCloseReturnsTerminated(t *testing.T) {
	loop := New()

	callbackStarted := make(chan struct{})
	callShutdown := make(chan struct{})
	callbackDone := make(chan error, 1)
	if err := loop.Submit(func() {
		close(callbackStarted)
		<-callShutdown
		callbackDone <- loop.Shutdown(context.Background())
	}); err != nil {
		t.Fatalf("Submit callback: %v", err)
	}

	closeTransitioned := make(chan struct{})
	releaseClose := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterCloseStateTerminating: func() {
			close(closeTransitioned)
			<-releaseClose
		},
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case <-callbackStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("callback did not start")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	select {
	case <-closeTransitioned:
	case <-time.After(5 * time.Second):
		close(callShutdown)
		close(releaseClose)
		t.Fatal("Close did not commit StateTerminating")
	}
	close(callShutdown)

	select {
	case err := <-callbackDone:
		if !errors.Is(err, ErrLoopTerminated) {
			close(releaseClose)
			t.Fatalf("callback-local Shutdown during Close = %v, want ErrLoopTerminated", err)
		}
	case <-time.After(5 * time.Second):
		close(releaseClose)
		t.Fatal("callback-local Shutdown blocked during Close")
	}
	close(releaseClose)

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not complete")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not complete")
	}
}

func TestShutdown_ReentrantDuringPreRunTerminalDrain(t *testing.T) {
	loop := New()

	callbackDone := make(chan error, 1)
	if err := loop.Submit(func() {
		callbackDone <- loop.Shutdown(context.Background())
	}); err != nil {
		t.Fatalf("Submit pre-run callback: %v", err)
	}

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(context.Background()) }()

	select {
	case err := <-callbackDone:
		if err != nil {
			t.Fatalf("recursive pre-run Shutdown = %v, want nil request acknowledgement", err)
		}
	case <-time.After(time.Second):
		t.Fatal("recursive pre-run Shutdown deadlocked during terminal drain")
	}

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("outer pre-run Shutdown = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("outer pre-run Shutdown did not complete")
	}
}

func TestShutdown_PreRunDrainsPromisifyDependencyBeforeWorkerWait(t *testing.T) {
	loop := New()

	callbackRan := make(chan struct{})
	submission := make(chan error, 1)
	workerDone := make(chan struct{})
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		err := loop.Submit(func() { close(callbackRan) })
		submission <- err
		if err != nil {
			return nil, err
		}
		<-callbackRan
		close(workerDone)
		return "dependency-complete", nil
	})

	select {
	case err := <-submission:
		if err != nil {
			t.Fatalf("Submit from Promisify worker: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify worker did not submit its dependency")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := loop.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	select {
	case <-workerDone:
	default:
		t.Fatal("Shutdown returned before the dependency-bound Promisify worker completed")
	}
	if promise.State() != Fulfilled {
		t.Fatalf("Promisify promise state = %v, want Fulfilled", promise.State())
	}
	if got := promise.Result(); got != "dependency-complete" {
		t.Fatalf("Promisify promise result = %v, want dependency-complete", got)
	}
	select {
	case <-loop.loopDone:
	default:
		t.Fatal("loopDone remained open after pre-Run Shutdown completed")
	}
	select {
	case <-loop.terminalDone:
	default:
		t.Fatal("terminalDone remained open after pre-Run Shutdown completed")
	}
}

func TestShutdown_PreRunDrainUsesDedicatedGoroutine(t *testing.T) {
	loop := New()

	callerID := goroutineid.Get()
	callbackID := make(chan int64, 1)
	if err := loop.Submit(func() { callbackID <- goroutineid.Get() }); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	select {
	case got := <-callbackID:
		if got == callerID {
			t.Fatalf("pre-Run callback goroutine = caller goroutine %d, want dedicated terminal finisher", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pre-Run terminal callback did not run")
	}
}

func TestShutdown_PreRunContextBoundsWorkerWaitAndCleanupContinues(t *testing.T) {
	loop := New()

	workerStarted := make(chan struct{})
	releaseWorker := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseWorker) }) }
	t.Cleanup(func() {
		release()
		select {
		case <-loop.terminalDone:
		case <-time.After(5 * time.Second):
			t.Error("pre-Run cleanup did not finish after worker release")
		}
	})

	loop.Promisify(context.Background(), func(context.Context) (any, error) {
		close(workerStarted)
		<-releaseWorker
		return "released", nil
	})
	select {
	case <-workerStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify worker did not start")
	}

	transitioned := make(chan struct{})
	requestOwnedDrain := make(chan bool, 1)
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() {
			requestOwnedDrain <- loop.isTerminalDrainOwner()
			close(transitioned)
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(ctx) }()
	select {
	case <-transitioned:
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not commit StateTerminating")
	}
	if owned := <-requestOwnedDrain; owned {
		t.Fatal("public Shutdown caller retained terminal-drain admission ownership")
	}

	// A concurrent external caller joins the independent terminal completion
	// with its own context bound, never the winning caller's context.
	concurrentCtx, cancelConcurrent := context.WithCancel(context.Background())
	cancelConcurrent()
	concurrentDone := make(chan error, 1)
	go func() { concurrentDone <- loop.Shutdown(concurrentCtx) }()
	select {
	case err := <-concurrentDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("concurrent Shutdown = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent Shutdown ignored its own canceled context")
	}

	cancel()
	select {
	case err := <-shutdownDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("winning Shutdown = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pre-Run Shutdown ignored caller cancellation while waiting for worker")
	}
	select {
	case <-loop.terminalDone:
		t.Fatal("terminalDone closed while Promisify worker remained blocked")
	default:
	}

	release()
	select {
	case <-loop.terminalDone:
	case <-time.After(5 * time.Second):
		t.Fatal("terminal cleanup did not continue after canceled Shutdown returned")
	}
	select {
	case <-loop.loopDone:
	default:
		t.Fatal("loopDone remained open after independent pre-Run cleanup")
	}
}

func TestShutdown_ContextBoundsPromisifyWaitAfterLoopExit(t *testing.T) {
	loop := New()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitLoopOwnerTurnT(t, loop)

	workerStarted := make(chan struct{})
	releaseWorker := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseWorker) }) }
	t.Cleanup(func() {
		release()
		select {
		case <-loop.terminalDone:
		case <-time.After(5 * time.Second):
			t.Error("running-loop cleanup did not finish after worker release")
		}
	})
	loop.Promisify(context.Background(), func(context.Context) (any, error) {
		close(workerStarted)
		<-releaseWorker
		return "released", nil
	})
	select {
	case <-workerStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify worker did not start")
	}

	ctx, cancel := context.WithCancel(context.Background())
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(ctx) }()
	select {
	case <-loop.loopDone:
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not exit after Shutdown transition")
	}
	select {
	case <-loop.terminalDone:
		t.Fatal("terminalDone closed before the blocked Promisify worker finished")
	default:
	}

	cancel()
	select {
	case err := <-shutdownDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Shutdown after loopDone = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown ignored caller cancellation after loopDone")
	}

	release()
	select {
	case <-loop.terminalDone:
	case <-time.After(5 * time.Second):
		t.Fatal("terminal cleanup did not finish after releasing Promisify worker")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	default:
		t.Fatal("Run result was not published after loopDone closed")
	}
}

func TestWaitShutdownCompletion_PrefersCompletedCleanupOverCanceledContext(t *testing.T) {
	loop := New()
	loop.closeTerminalDone()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := loop.waitShutdownCompletion(ctx); err != nil {
		t.Fatalf("waitShutdownCompletion with both signals ready = %v, want nil", err)
	}
}
