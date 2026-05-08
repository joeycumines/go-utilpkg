package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestTerminalCompletionOwnerLifecycleReentry(t *testing.T) {
	tests := []struct {
		name        string
		winner      func(*Loop) error
		reenter     func(*Loop) error
		wantReentry error
	}{
		{
			name:        "graceful_Shutdown",
			winner:      func(loop *Loop) error { return loop.Shutdown(context.Background()) },
			reenter:     func(loop *Loop) error { return loop.Shutdown(context.Background()) },
			wantReentry: nil,
		},
		{
			name:        "graceful_Close",
			winner:      func(loop *Loop) error { return loop.Shutdown(context.Background()) },
			reenter:     func(loop *Loop) error { return loop.Close() },
			wantReentry: ErrLoopTerminated,
		},
		{
			name:        "immediate_Shutdown",
			winner:      func(loop *Loop) error { return loop.Close() },
			reenter:     func(loop *Loop) error { return loop.Shutdown(context.Background()) },
			wantReentry: ErrLoopTerminated,
		},
		{
			name:        "immediate_Close",
			winner:      func(loop *Loop) error { return loop.Close() },
			reenter:     func(loop *Loop) error { return loop.Close() },
			wantReentry: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop := New()

			ownerObserved := make(chan bool, 1)
			reentryResult := make(chan error, 1)
			postPublicationResult := make(chan error, 1)
			var terminalJoins atomic.Int32
			loop.testHooks = &loopTestHooks{
				BeforeCloseFDLock: func() {
					ownerObserved <- loop.isTerminalCompletionOwner()
					reentryResult <- test.reenter(loop)
				},
				BeforeTerminalJoin: func() { terminalJoins.Add(1) },
				AfterTerminalDoneClose: func() {
					postPublicationResult <- test.reenter(loop)
				},
			}

			winnerDone := make(chan error, 1)
			go func() { winnerDone <- test.winner(loop) }()
			if !waitContractValue(t, ownerObserved, "terminal completion ownership") {
				t.Fatal("terminal cleanup callback did not observe completion ownership")
			}
			got := waitContractValue(t, reentryResult, "terminal completion owner lifecycle reentry")
			if test.wantReentry == nil {
				if got != nil {
					t.Fatalf("terminal completion owner reentry = %v, want nil", got)
				}
			} else if !errors.Is(got, test.wantReentry) {
				t.Fatalf("terminal completion owner reentry = %v, want %v", got, test.wantReentry)
			}
			if err := waitContractValue(t, winnerDone, "winning terminal completion"); err != nil {
				t.Fatalf("winning terminal operation: %v", err)
			}
			if err := waitContractValue(t, postPublicationResult, "post-publication lifecycle reentry"); !errors.Is(err, ErrLoopTerminated) {
				t.Fatalf("post-publication lifecycle reentry = %v, want ErrLoopTerminated", err)
			}
			if got := terminalJoins.Load(); got != 0 {
				t.Fatalf("terminal completion owner reentry joins = %d, want 0", got)
			}
			if owner := loop.terminalCompletionOwner.Load(); owner != 0 {
				t.Fatalf("terminal completion owner after publication = %d, want 0", owner)
			}
			assertCloseSignals(t, loop)
		})
	}
}

func TestLifecycleAutoExitPublishesLoopDoneBeforeTerminalDone(t *testing.T) {
	loop := New(WithAutoExit(true))
	publicationChecked := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterTerminalDoneClose: func() {
			select {
			case <-loop.loopDone:
			default:
				t.Error("terminalDone closed before loopDone")
			}
			close(publicationChecked)
		},
	}
	if err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	waitContractSignal(t, publicationChecked, "auto-exit completion publication ordering")
}

func TestLifecycleClosePublishesAggregateTerminalError(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	sentinel := errors.New("injected terminal failure")
	loop.storeTerminalError(sentinel)

	terminalPublished := make(chan struct{})
	releaseTransition := make(chan struct{})
	releaseTransitionFn := releaseSignalT(t, releaseTransition)
	terminalJoined := make(chan struct{})
	var joinedOnce sync.Once
	loop.testHooks = &loopTestHooks{
		AfterCloseStateTerminating: func() {
			close(terminalPublished)
			<-releaseTransition
		},
		BeforeTerminalJoin: func() {
			joinedOnce.Do(func() { close(terminalJoined) })
		},
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	waitContractSignal(t, terminalPublished, "Close terminal publication")
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(context.Background()) }()
	waitContractSignal(t, terminalJoined, "Shutdown aggregate-result join")
	releaseTransitionFn()
	if err := waitContractValue(t, closeDone, "winning Close terminal result"); !errors.Is(err, sentinel) {
		t.Fatalf("winning Close = %v, want injected terminal failure", err)
	}
	if err := waitContractValue(t, shutdownDone, "joined Shutdown terminal result"); !errors.Is(err, sentinel) {
		t.Fatalf("joined Shutdown = %v, want injected terminal failure", err)
	}
}

func TestLifecycleCloseWaitsForRunOwnerPublishedBeforeRunStarted(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	runTransitioned := make(chan struct{})
	releaseRun := make(chan struct{})
	releaseRunFn := releaseSignalT(t, releaseRun)
	loop.testHooks = &loopTestHooks{
		AfterRunStateRunningBeforeStart: func() {
			close(runTransitioned)
			<-releaseRun
		},
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, runTransitioned, "Run StateRunning publication")

	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	select {
	case err := <-closeDone:
		t.Fatalf("Close returned before the Run owner exited: %v", err)
	default:
	}
	releaseRunFn()
	if err := waitContractValue(t, runDone, "Run completion after Close"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := waitContractValue(t, closeDone, "Close completion after Run owner exit"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	assertCloseSignals(t, loop)
}
