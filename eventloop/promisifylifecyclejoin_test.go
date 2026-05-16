package eventloop

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestPromisifyWorkerShutdownAcknowledgesActiveGracefulTermination(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	callWorkerShutdown := make(chan struct{})
	callWorkerShutdownFn := releaseSignalT(t, callWorkerShutdown)
	workerStarted := make(chan struct{})
	workerShutdownDone := make(chan error, 1)
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		close(workerStarted)
		<-callWorkerShutdown
		workerShutdownDone <- loop.Shutdown(context.Background())
		return "worker-returned", nil
	})
	waitContractSignal(t, workerStarted, "Promisify worker entry")

	terminalPublished := make(chan struct{})
	releaseTransition := make(chan struct{})
	releaseTransitionFn := releaseSignalT(t, releaseTransition)
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() {
			close(terminalPublished)
			<-releaseTransition
		},
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitLoopOwnerTurnT(t, loop)
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(context.Background()) }()
	waitContractSignal(t, terminalPublished, "graceful terminal publication")

	callWorkerShutdownFn()
	if err := waitContractValue(t, workerShutdownDone, "worker Shutdown request acknowledgement"); err != nil {
		t.Fatalf("worker Shutdown during graceful termination = %v, want nil", err)
	}
	releaseTransitionFn()
	if err := waitContractValue(t, shutdownDone, "external Shutdown completion"); err != nil {
		t.Fatalf("external Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result := waitContractValue(t, promise.ToChannel(), "Promisify settlement"); result != "worker-returned" {
		t.Fatalf("Promisify result = %v, want worker-returned", result)
	}
}

func TestPromisifyWorkerWinningCloseBeforeRun(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	workerCloseDone := make(chan error, 1)
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		workerCloseDone <- loop.Close()
		return "worker-returned", nil
	})
	if err := waitContractValue(t, workerCloseDone, "pre-Run worker Close request acknowledgement"); err != nil {
		t.Fatalf("pre-Run worker Close = %v, want nil request acknowledgement", err)
	}
	waitContractSignal(t, loop.terminalDone, "pre-Run worker Close completion")
	assertCloseSignals(t, loop)
	assertPromiseRejected(t, promise, ErrLoopTerminated)
}

func TestPromisifyWorkerWinningCloseDoesNotJoinLoopDependency(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	releaseCallback := make(chan struct{})
	releaseCallbackFn := releaseSignalT(t, releaseCallback)
	callbackStarted := make(chan struct{})
	if err := loop.Submit(func() {
		close(callbackStarted)
		<-releaseCallback
	}); err != nil {
		t.Fatalf("Submit callback dependency: %v", err)
	}

	closeWon := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterCloseStateTerminating: func() { close(closeWon) },
	}
	workerCloseDone := make(chan error, 1)
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		<-callbackStarted
		err := loop.Close()
		workerCloseDone <- err
		releaseCallbackFn()
		return "worker-returned", nil
	})

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, closeWon, "Promisify worker Close ownership")
	if err := waitContractValue(t, workerCloseDone, "worker Close request acknowledgement"); err != nil {
		t.Fatalf("worker-winning Close = %v, want nil request acknowledgement", err)
	}
	if err := waitContractValue(t, runDone, "Run completion after worker Close"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitContractSignal(t, loop.terminalDone, "worker-requested Close completion")
	assertPromiseRejected(t, promise, ErrLoopTerminated)
}

func TestPromisifyWorkerCloseAcknowledgesActiveImmediateTermination(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	callWorkerClose := make(chan struct{})
	callWorkerCloseFn := releaseSignalT(t, callWorkerClose)
	workerStarted := make(chan struct{})
	workerCloseDone := make(chan error, 1)
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		close(workerStarted)
		<-callWorkerClose
		workerCloseDone <- loop.Close()
		return "worker-returned", nil
	})
	waitContractSignal(t, workerStarted, "Promisify worker entry")

	sentinel := errors.New("injected terminal failure")
	loop.storeTerminalError(sentinel)
	terminalPublished := make(chan struct{})
	releaseTransition := make(chan struct{})
	releaseTransitionFn := releaseSignalT(t, releaseTransition)
	var terminalJoins atomic.Int32
	loop.testHooks = &loopTestHooks{
		AfterCloseStateTerminating: func() {
			close(terminalPublished)
			<-releaseTransition
		},
		BeforeTerminalJoin: func() { terminalJoins.Add(1) },
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	waitContractSignal(t, terminalPublished, "immediate terminal publication")
	callWorkerCloseFn()
	if err := waitContractValue(t, workerCloseDone, "worker same-mode Close acknowledgement"); err != nil {
		t.Fatalf("worker Close during immediate termination = %v, want nil", err)
	}
	if got := terminalJoins.Load(); got != 0 {
		t.Fatalf("worker same-mode Close terminal joins = %d, want 0", got)
	}
	select {
	case <-loop.terminalDone:
		t.Fatal("terminal completion published before winning Close was released")
	default:
	}

	releaseTransitionFn()
	if err := waitContractValue(t, closeDone, "winning Close completion"); !errors.Is(err, sentinel) {
		t.Fatalf("winning Close = %v, want injected terminal failure", err)
	}
	assertPromiseRejected(t, promise, ErrLoopTerminated)
}

func TestPromisifyWorkerLifecycleResultAfterLifecycleRace(t *testing.T) {
	tests := []struct {
		name            string
		worker          func(*Loop) error
		winner          func(*Loop) error
		workerClose     bool
		winnerImmediate bool
		want            error
	}{
		{
			name:        "close_loses_to_graceful",
			worker:      func(loop *Loop) error { return loop.Close() },
			winner:      func(loop *Loop) error { return loop.Shutdown(context.Background()) },
			workerClose: true,
			want:        ErrLoopTerminated,
		},
		{
			name:            "close_loses_to_immediate",
			worker:          func(loop *Loop) error { return loop.Close() },
			winner:          func(loop *Loop) error { return loop.Close() },
			workerClose:     true,
			winnerImmediate: true,
		},
		{
			name:   "shutdown_loses_to_graceful",
			worker: func(loop *Loop) error { return loop.Shutdown(context.Background()) },
			winner: func(loop *Loop) error { return loop.Shutdown(context.Background()) },
		},
		{
			name:            "shutdown_loses_to_immediate",
			worker:          func(loop *Loop) error { return loop.Shutdown(context.Background()) },
			winner:          func(loop *Loop) error { return loop.Close() },
			winnerImmediate: true,
			want:            ErrLoopTerminated,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)

			callWorker := make(chan struct{})
			callWorkerFn := releaseSignalT(t, callWorker)
			workerStarted := make(chan struct{})
			workerDone := make(chan error, 1)
			promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
				close(workerStarted)
				<-callWorker
				workerDone <- test.worker(loop)
				return "worker-returned", nil
			})
			waitContractSignal(t, workerStarted, "Promisify worker entry")

			workerBeforeLifecycleLock := make(chan struct{})
			releaseWorkerLifecycleLock := make(chan struct{})
			releaseWorkerLifecycleLockFn := releaseSignalT(t, releaseWorkerLifecycleLock)
			winnerPublished := make(chan struct{})
			releaseWinner := make(chan struct{})
			releaseWinnerFn := releaseSignalT(t, releaseWinner)
			var lifecycleHookCalls atomic.Int32
			var terminalJoins atomic.Int32
			pauseWorker := func() {
				if lifecycleHookCalls.Add(1) == 1 {
					close(workerBeforeLifecycleLock)
					<-releaseWorkerLifecycleLock
				}
			}
			hooks := &loopTestHooks{
				BeforeTerminalJoin: func() { terminalJoins.Add(1) },
			}
			if test.workerClose {
				hooks.BeforeCloseLifecycleLock = pauseWorker
			} else {
				hooks.BeforeShutdownLifecycleLock = pauseWorker
			}
			if test.winnerImmediate {
				hooks.BeforeClosePromiseRejection = func() {
					close(winnerPublished)
					<-releaseWinner
				}
			} else {
				hooks.AfterShutdownStateTerminating = func() {
					close(winnerPublished)
					<-releaseWinner
				}
			}
			loop.testHooks = hooks

			callWorkerFn()
			waitContractSignal(t, workerBeforeLifecycleLock, "worker pre-lifecycle observation")
			winnerDone := make(chan error, 1)
			go func() { winnerDone <- test.winner(loop) }()
			waitContractSignal(t, winnerPublished, "winning terminal publication")
			releaseWorkerLifecycleLockFn()
			err = waitContractValue(t, workerDone, "worker locked-recheck lifecycle result")
			if test.want == nil {
				if err != nil {
					t.Fatalf("worker lifecycle result = %v, want nil", err)
				}
			} else if !errors.Is(err, test.want) {
				t.Fatalf("worker lifecycle result = %v, want %v", err, test.want)
			}
			if got := terminalJoins.Load(); got != 0 {
				t.Fatalf("worker locked-recheck terminal joins = %d, want 0", got)
			}
			select {
			case <-loop.terminalDone:
				t.Fatal("terminal completion published before winning operation was released")
			default:
			}

			releaseWinnerFn()
			if err := waitContractValue(t, winnerDone, "winning terminal completion"); err != nil {
				t.Fatalf("winning terminal operation: %v", err)
			}
			if test.winnerImmediate {
				assertPromiseRejected(t, promise, ErrLoopTerminated)
			} else if result := waitContractValue(t, promise.ToChannel(), "graceful Promisify settlement"); result != "worker-returned" {
				t.Fatalf("Promisify result = %v, want worker-returned", result)
			}
		})
	}
}

func TestPromisifyWorkerShutdownDoesNotJoinImmediateTermination(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	callShutdown := make(chan struct{})
	callShutdownFn := releaseSignalT(t, callShutdown)
	workerStarted := make(chan struct{})
	workerShutdownDone := make(chan error, 1)
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		close(workerStarted)
		<-callShutdown
		workerShutdownDone <- loop.Shutdown(context.Background())
		return "worker-returned", nil
	})
	waitContractSignal(t, workerStarted, "Promisify worker entry")

	terminalPublished := make(chan struct{})
	releaseTransition := make(chan struct{})
	releaseTransitionFn := releaseSignalT(t, releaseTransition)
	loop.testHooks = &loopTestHooks{
		AfterCloseStateTerminating: func() {
			close(terminalPublished)
			<-releaseTransition
		},
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	waitContractSignal(t, terminalPublished, "immediate terminal publication")

	callShutdownFn()
	if err := waitContractValue(t, workerShutdownDone, "worker Shutdown during immediate termination"); !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("worker Shutdown = %v, want ErrLoopTerminated", err)
	}
	releaseTransitionFn()
	if err := waitContractValue(t, closeDone, "immediate Close completion"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	assertPromiseRejected(t, promise, ErrLoopTerminated)
}

func TestPromisifyWorkerLifecycleResultsWhileCompletionOpen(t *testing.T) {
	tests := []struct {
		name         string
		winner       func(*Loop) error
		immediate    bool
		wantShutdown error
		wantClose    error
	}{
		{
			name:         "graceful",
			winner:       func(loop *Loop) error { return loop.Shutdown(context.Background()) },
			wantShutdown: nil,
			wantClose:    ErrLoopTerminated,
		},
		{
			name:         "immediate",
			winner:       func(loop *Loop) error { return loop.Close() },
			immediate:    true,
			wantShutdown: ErrLoopTerminated,
			wantClose:    nil,
		},
	}

	type lifecycleResults struct {
		shutdown error
		close    error
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)

			callLifecycle := make(chan struct{})
			callLifecycleFn := releaseSignalT(t, callLifecycle)
			workerStarted := make(chan struct{})
			workerResults := make(chan lifecycleResults, 1)
			promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
				close(workerStarted)
				<-callLifecycle
				workerResults <- lifecycleResults{
					shutdown: loop.Shutdown(context.Background()),
					close:    loop.Close(),
				}
				return "worker-returned", nil
			})
			waitContractSignal(t, workerStarted, "Promisify worker entry")

			terminalPublished := make(chan struct{})
			releaseTransition := make(chan struct{})
			releaseTransitionFn := releaseSignalT(t, releaseTransition)
			var terminalJoins atomic.Int32
			hooks := &loopTestHooks{
				BeforeTerminalJoin: func() { terminalJoins.Add(1) },
			}
			if test.immediate {
				hooks.BeforeClosePromiseRejection = func() {
					close(terminalPublished)
					<-releaseTransition
				}
			} else {
				hooks.AfterTerminateStateBeforeDrain = func() {
					close(terminalPublished)
					<-releaseTransition
				}
			}
			loop.testHooks = hooks

			winnerDone := make(chan error, 1)
			go func() { winnerDone <- test.winner(loop) }()
			waitContractSignal(t, terminalPublished, "StateTerminated publication")
			if state := loop.State(); state != StateTerminated {
				t.Fatalf("state at worker lifecycle calls = %v, want StateTerminated", state)
			}
			select {
			case <-loop.terminalDone:
				t.Fatal("terminalDone closed before worker lifecycle calls")
			default:
			}

			callLifecycleFn()
			got := waitContractValue(t, workerResults, "worker StateTerminated lifecycle results")
			if test.wantShutdown == nil {
				if got.shutdown != nil {
					t.Fatalf("worker Shutdown = %v, want nil", got.shutdown)
				}
			} else if !errors.Is(got.shutdown, test.wantShutdown) {
				t.Fatalf("worker Shutdown = %v, want %v", got.shutdown, test.wantShutdown)
			}
			if test.wantClose == nil {
				if got.close != nil {
					t.Fatalf("worker Close = %v, want nil", got.close)
				}
			} else if !errors.Is(got.close, test.wantClose) {
				t.Fatalf("worker Close = %v, want %v", got.close, test.wantClose)
			}
			if got := terminalJoins.Load(); got != 0 {
				t.Fatalf("worker lifecycle terminal joins = %d, want 0", got)
			}

			releaseTransitionFn()
			if err := waitContractValue(t, winnerDone, "winning terminal operation"); err != nil {
				t.Fatalf("winning terminal operation: %v", err)
			}
			if test.immediate {
				assertPromiseRejected(t, promise, ErrLoopTerminated)
			} else if result := waitContractValue(t, promise.ToChannel(), "graceful worker settlement"); result != "worker-returned" {
				t.Fatalf("Promisify result = %v, want worker-returned", result)
			}
		})
	}
}
