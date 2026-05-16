package eventloop

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestQuiescenceHandlerCancellationWinsBeforeAutoExitCommit(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	ctx, cancel := context.WithCancel(context.Background())
	loop.SetQuiescenceHandler(func() bool {
		cancel()
		return false
	})

	if err := loop.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run after quiescence-handler cancellation = %v, want context.Canceled", err)
	}
}

func TestAbortedAutoExitCommitClearsQuiescingBeforeNextHandler(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	var (
		armed             atomic.Bool
		predicatePhase    atomic.Int32
		quiescenceCalls   atomic.Int32
		timerRan          atomic.Bool
		timerAdmissionErr error
	)
	loop.testHooks = &loopTestHooks{
		AfterAutoExitFinalAliveCheck: func() {
			if armed.CompareAndSwap(false, true) {
				predicatePhase.Store(1)
			}
		},
	}
	if err := loop.ScheduleImmediateRef(func() {}, func() bool {
		phase := predicatePhase.Load()
		if phase == 0 {
			return false
		}
		return predicatePhase.Add(1) == 3
	}); err != nil {
		t.Fatalf("ScheduleImmediateRef: %v", err)
	}
	loop.SetQuiescenceHandler(func() bool {
		switch quiescenceCalls.Add(1) {
		case 1:
			return false
		case 2:
			_, timerAdmissionErr = loop.ScheduleTimer(0, func() { timerRan.Store(true) })
			return true
		default:
			return false
		}
	})

	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := quiescenceCalls.Load(); got < 2 {
		t.Fatalf("quiescence handler calls = %d, want at least 2", got)
	}
	if timerAdmissionErr != nil {
		t.Fatalf("ScheduleTimer from handler after aborted auto-exit commit = %v, want nil", timerAdmissionErr)
	}
	if !timerRan.Load() {
		t.Fatal("timer accepted after aborted auto-exit commit did not run")
	}
}

func TestContextCancellationWinsAtFinalAutoExitAdmission(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	ctx, cancel := context.WithCancel(context.Background())
	var boundaryReached bool
	loop.testHooks = &loopTestHooks{
		BeforeAutoExitTerminalDrainCommit: func() {
			boundaryReached = true
			cancel()
		},
	}

	if err := loop.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run after final-admission cancellation = %v, want context.Canceled", err)
	}
	if !boundaryReached {
		t.Fatal("auto-exit did not reach its final terminal-admission boundary")
	}
}

func TestContextCancellationAfterAutoExitAdmissionKeepsCleanCompletion(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	ctx, cancel := context.WithCancel(context.Background())
	var boundaryReached bool
	loop.testHooks = &loopTestHooks{
		BeforeTerminateState: func() {
			boundaryReached = true
			cancel()
		},
	}

	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run after committed auto-exit cancellation = %v, want nil", err)
	}
	if !boundaryReached {
		t.Fatal("auto-exit did not pass its terminal-admission boundary")
	}
}
