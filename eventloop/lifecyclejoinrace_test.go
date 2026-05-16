package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLifecycleExternalCloseJoinsAfterLifecycleRace(t *testing.T) {
	tests := []struct {
		name      string
		immediate bool
		winner    func(*Loop) error
	}{
		{
			name:   "graceful",
			winner: func(loop *Loop) error { return loop.Shutdown(context.Background()) },
		},
		{
			name:      "immediate",
			immediate: true,
			winner:    func(loop *Loop) error { return loop.Close() },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)
			sentinel := errors.New("injected terminal failure")
			loop.storeTerminalError(sentinel)

			loserBeforeLifecycleLock := make(chan struct{})
			releaseLoser := make(chan struct{})
			releaseLoserFn := releaseSignalT(t, releaseLoser)
			terminalPublished := make(chan struct{})
			releaseWinner := make(chan struct{})
			releaseWinnerFn := releaseSignalT(t, releaseWinner)
			terminalJoined := make(chan struct{})
			var lifecycleHookCalls atomic.Int32
			var joinedOnce sync.Once
			hooks := &loopTestHooks{
				BeforeCloseLifecycleLock: func() {
					if lifecycleHookCalls.Add(1) == 1 {
						close(loserBeforeLifecycleLock)
						<-releaseLoser
					}
				},
				BeforeTerminalJoin: func() {
					joinedOnce.Do(func() { close(terminalJoined) })
				},
			}
			if test.immediate {
				hooks.BeforeClosePromiseRejection = func() {
					close(terminalPublished)
					<-releaseWinner
				}
			} else {
				hooks.AfterShutdownStateTerminating = func() {
					close(terminalPublished)
					<-releaseWinner
				}
			}
			loop.testHooks = hooks

			loserDone := make(chan error, 1)
			go func() { loserDone <- loop.Close() }()
			waitContractSignal(t, loserBeforeLifecycleLock, "losing Close pre-lifecycle observation")
			winnerDone := make(chan error, 1)
			go func() { winnerDone <- test.winner(loop) }()
			waitContractSignal(t, terminalPublished, "winning terminal publication")
			releaseLoserFn()
			waitContractSignal(t, terminalJoined, "losing Close terminal join")
			select {
			case err := <-loserDone:
				t.Fatalf("losing Close returned before terminal completion: %v", err)
			default:
			}

			releaseWinnerFn()
			if err := waitContractValue(t, winnerDone, "winning terminal operation"); !errors.Is(err, sentinel) {
				t.Fatalf("winning terminal operation = %v, want injected failure", err)
			}
			if err := waitContractValue(t, loserDone, "joined Close terminal result"); !errors.Is(err, sentinel) {
				t.Fatalf("joined Close = %v, want injected failure", err)
			}
		})
	}
}

func TestLifecycleExternalCloseRetainsOpenProbeAfterCompletion(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	sentinel := errors.New("injected terminal failure")
	loop.storeTerminalError(sentinel)

	loserBeforeLifecycleLock := make(chan struct{})
	releaseLoser := make(chan struct{})
	releaseLoserFn := releaseSignalT(t, releaseLoser)
	terminalJoined := make(chan struct{})
	var joinedOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeCloseLifecycleLock: func() {
			close(loserBeforeLifecycleLock)
			<-releaseLoser
		},
		BeforeTerminalJoin: func() {
			joinedOnce.Do(func() { close(terminalJoined) })
		},
	}

	loserDone := make(chan error, 1)
	go func() { loserDone <- loop.Close() }()
	waitContractSignal(t, loserBeforeLifecycleLock, "losing Close pre-lifecycle observation")
	if err := loop.Shutdown(context.Background()); !errors.Is(err, sentinel) {
		t.Fatalf("winning Shutdown = %v, want injected terminal failure", err)
	}
	assertCloseSignals(t, loop)
	releaseLoserFn()
	waitContractSignal(t, terminalJoined, "completed terminal join after stale Close observation")
	if err := waitContractValue(t, loserDone, "joined Close published terminal result"); !errors.Is(err, sentinel) {
		t.Fatalf("joined Close = %v, want injected terminal failure", err)
	}
}

func TestLifecycleExternalShutdownJoinsAfterLifecycleRace(t *testing.T) {
	tests := []struct {
		name      string
		immediate bool
		winner    func(*Loop) error
	}{
		{
			name:   "graceful",
			winner: func(loop *Loop) error { return loop.Shutdown(context.Background()) },
		},
		{
			name:      "immediate",
			immediate: true,
			winner:    func(loop *Loop) error { return loop.Close() },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)
			sentinel := errors.New("injected terminal failure")
			loop.storeTerminalError(sentinel)

			loserBeforeLifecycleLock := make(chan struct{})
			releaseLoser := make(chan struct{})
			releaseLoserFn := releaseSignalT(t, releaseLoser)
			terminalPublished := make(chan struct{})
			releaseWinner := make(chan struct{})
			releaseWinnerFn := releaseSignalT(t, releaseWinner)
			terminalJoined := make(chan struct{})
			var lifecycleHookCalls atomic.Int32
			var joinedOnce sync.Once
			hooks := &loopTestHooks{
				BeforeShutdownLifecycleLock: func() {
					if lifecycleHookCalls.Add(1) == 1 {
						close(loserBeforeLifecycleLock)
						<-releaseLoser
					}
				},
				BeforeTerminalJoin: func() {
					joinedOnce.Do(func() { close(terminalJoined) })
				},
			}
			if test.immediate {
				hooks.BeforeClosePromiseRejection = func() {
					close(terminalPublished)
					<-releaseWinner
				}
			} else {
				hooks.AfterShutdownStateTerminating = func() {
					close(terminalPublished)
					<-releaseWinner
				}
			}
			loop.testHooks = hooks

			loserDone := make(chan error, 1)
			go func() { loserDone <- loop.Shutdown(context.Background()) }()
			waitContractSignal(t, loserBeforeLifecycleLock, "losing Shutdown pre-lifecycle observation")
			winnerDone := make(chan error, 1)
			go func() { winnerDone <- test.winner(loop) }()
			waitContractSignal(t, terminalPublished, "winning terminal publication")
			releaseLoserFn()
			waitContractSignal(t, terminalJoined, "losing Shutdown terminal join")
			select {
			case err := <-loserDone:
				t.Fatalf("losing Shutdown returned before terminal completion: %v", err)
			default:
			}

			releaseWinnerFn()
			if err := waitContractValue(t, winnerDone, "winning terminal operation"); !errors.Is(err, sentinel) {
				t.Fatalf("winning terminal operation = %v, want injected failure", err)
			}
			if err := waitContractValue(t, loserDone, "joined Shutdown terminal result"); !errors.Is(err, sentinel) {
				t.Fatalf("joined Shutdown = %v, want injected failure", err)
			}
		})
	}
}

func TestLifecycleJoinedShutdownAfterLifecycleRaceRetainsContextBound(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	loserBeforeLifecycleLock := make(chan struct{})
	releaseLoser := make(chan struct{})
	releaseLoserFn := releaseSignalT(t, releaseLoser)
	terminalPublished := make(chan struct{})
	releaseWinner := make(chan struct{})
	releaseWinnerFn := releaseSignalT(t, releaseWinner)
	terminalJoined := make(chan struct{})
	var joinedOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeShutdownLifecycleLock: func() {
			close(loserBeforeLifecycleLock)
			<-releaseLoser
		},
		BeforeClosePromiseRejection: func() {
			close(terminalPublished)
			<-releaseWinner
		},
		BeforeTerminalJoin: func() {
			joinedOnce.Do(func() { close(terminalJoined) })
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	loserDone := make(chan error, 1)
	go func() { loserDone <- loop.Shutdown(ctx) }()
	waitContractSignal(t, loserBeforeLifecycleLock, "losing Shutdown pre-lifecycle observation")
	winnerDone := make(chan error, 1)
	go func() { winnerDone <- loop.Close() }()
	waitContractSignal(t, terminalPublished, "winning immediate terminal publication")
	cancel()
	releaseLoserFn()
	waitContractSignal(t, terminalJoined, "context-bounded Shutdown terminal join")
	if err := waitContractValue(t, loserDone, "context-bounded joined Shutdown"); !errors.Is(err, context.Canceled) {
		t.Fatalf("joined Shutdown = %v, want context.Canceled", err)
	}
	select {
	case err := <-winnerDone:
		t.Fatalf("winning Close returned before release: %v", err)
	default:
	}

	releaseWinnerFn()
	if err := waitContractValue(t, winnerDone, "winning Close completion"); err != nil {
		t.Fatalf("winning Close: %v", err)
	}
}

func TestLifecycleJoinedShutdownAfterLifecycleRacePrefersPublishedTerminalResult(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	sentinel := errors.New("injected terminal failure")
	loop.storeTerminalError(sentinel)

	loserBeforeLifecycleLock := make(chan struct{})
	releaseLoser := make(chan struct{})
	releaseLoserFn := releaseSignalT(t, releaseLoser)
	terminalJoined := make(chan struct{})
	var joinedOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeShutdownLifecycleLock: func() {
			close(loserBeforeLifecycleLock)
			<-releaseLoser
		},
		BeforeTerminalJoin: func() {
			joinedOnce.Do(func() { close(terminalJoined) })
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	loserDone := make(chan error, 1)
	go func() { loserDone <- loop.Shutdown(ctx) }()
	waitContractSignal(t, loserBeforeLifecycleLock, "losing Shutdown pre-lifecycle observation")
	if err := loop.Close(); !errors.Is(err, sentinel) {
		t.Fatalf("winning Close = %v, want injected terminal failure", err)
	}
	assertCloseSignals(t, loop)
	releaseLoserFn()
	waitContractSignal(t, terminalJoined, "completed terminal join after stale Shutdown observation")
	if err := waitContractValue(t, loserDone, "joined Shutdown published terminal result"); !errors.Is(err, sentinel) {
		t.Fatalf("joined Shutdown = %v, want injected terminal failure", err)
	}
}

func TestLifecycleJoinedShutdownContextBoundaryPrefersPublishedTerminalResult(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	sentinel := errors.New("injected terminal failure")
	loop.storeTerminalError(sentinel)

	terminalPublished := make(chan struct{})
	releaseWinner := make(chan struct{})
	releaseWinnerFn := releaseSignalT(t, releaseWinner)
	terminalJoined := make(chan struct{})
	contextSelected := make(chan struct{})
	releaseContextRecheck := make(chan struct{})
	releaseContextRecheckFn := releaseSignalT(t, releaseContextRecheck)
	var joinedOnce sync.Once
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() {
			close(terminalPublished)
			<-releaseWinner
		},
		BeforeTerminalJoin: func() {
			joinedOnce.Do(func() { close(terminalJoined) })
		},
		AfterShutdownJoinContext: func() {
			close(contextSelected)
			<-releaseContextRecheck
		},
	}

	winnerDone := make(chan error, 1)
	go func() { winnerDone <- loop.Shutdown(context.Background()) }()
	waitContractSignal(t, terminalPublished, "graceful terminal publication")

	ctx, cancel := context.WithCancel(context.Background())
	joinedDone := make(chan error, 1)
	go func() { joinedDone <- loop.Shutdown(ctx) }()
	waitContractSignal(t, terminalJoined, "context-bounded terminal join")
	cancel()
	waitContractSignal(t, contextSelected, "joined Shutdown context selection")

	releaseWinnerFn()
	waitContractSignal(t, loop.terminalDone, "terminal publication at context boundary")
	releaseContextRecheckFn()
	if err := waitContractValue(t, winnerDone, "winning Shutdown terminal result"); !errors.Is(err, sentinel) {
		t.Fatalf("winning Shutdown = %v, want injected terminal failure", err)
	}
	if err := waitContractValue(t, joinedDone, "context-boundary terminal result"); !errors.Is(err, sentinel) {
		t.Fatalf("joined Shutdown = %v, want injected terminal failure", err)
	}
}
