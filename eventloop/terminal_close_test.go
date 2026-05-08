package eventloop

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTerminalDrain_NonOwnerExternalEphemeralWorkRejected(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	stateTerminating := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() {
			close(stateTerminating)
		},
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	blockingTaskStarted := make(chan struct{})
	releaseBlockingTask := make(chan struct{})
	releaseBlocking := releaseSignalT(t, releaseBlockingTask)
	if err := loop.Submit(func() {
		close(blockingTaskStarted)
		<-releaseBlockingTask
	}); err != nil {
		t.Fatalf("submit blocking task: %v", err)
	}

	select {
	case <-blockingTaskStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("blocking task did not start")
	}

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(context.Background()) }()

	select {
	case <-stateTerminating:
	case <-time.After(5 * time.Second):
		releaseBlocking()
		t.Fatal("Shutdown did not enter StateTerminating")
	}

	if err := loop.Submit(func() {}); !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("external Submit during terminal drain err = %v, want ErrLoopTerminated", err)
	}
	if err := loop.SubmitInternal(func() {}); !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("external SubmitInternal during terminal drain err = %v, want ErrLoopTerminated", err)
	}
	if err := loop.ScheduleMicrotask(func() {}); !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("external ScheduleMicrotask during terminal drain err = %v, want ErrLoopTerminated", err)
	}
	if err := loop.ScheduleNextTick(func() {}); !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("external ScheduleNextTick during terminal drain err = %v, want ErrLoopTerminated", err)
	}

	releaseBlocking()

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("Shutdown returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not finish")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not finish")
	}
}

func TestTerminalDrain_CloseBypassesDrainAndRejectsCallbackContinuation(t *testing.T) {
	loop := New()
	terminalVisible := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		BeforeClosePromiseRejection: func() { close(terminalVisible) },
	}

	callbackStarted := make(chan struct{})
	releaseCallback := make(chan struct{})
	scheduleErr := make(chan error, 1)
	microtaskRan := make(chan struct{})
	if err := loop.Submit(func() {
		close(callbackStarted)
		<-releaseCallback
		scheduleErr <- loop.ScheduleMicrotask(func() { close(microtaskRan) })
	}); err != nil {
		t.Fatalf("submit blocking callback: %v", err)
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

	waitContractSignal(t, terminalVisible, "Close StateTerminated publication")

	close(releaseCallback)

	select {
	case err := <-scheduleErr:
		if !errors.Is(err, ErrLoopTerminated) {
			t.Fatalf("ScheduleMicrotask from callback after Close err = %v, want ErrLoopTerminated", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("callback did not attempt continuation schedule")
	}

	select {
	case <-microtaskRan:
		t.Fatal("microtask scheduled after Close unexpectedly ran")
	default:
	}

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not finish")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not finish")
	}
}

func TestTerminalDrain_CloseSkipsQueuedSubmitInternalAfterCurrentCallback(t *testing.T) {
	loop := New()

	releaseOuter := make(chan struct{})
	closeDone := make(chan error, 1)
	closeTerminated := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		BeforeClosePromiseRejection: func() { close(closeTerminated) },
	}

	directTaskRan := make(chan struct{}, 1)
	submitInternalDone := make(chan error, 1)
	if err := loop.Submit(func() {
		submitInternalDone <- loop.SubmitInternal(func() {
			directTaskRan <- struct{}{}
		})
		<-releaseOuter
	}); err != nil {
		t.Fatalf("submit outer callback: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	select {
	case err := <-submitInternalDone:
		if err != nil {
			t.Fatalf("SubmitInternal queued before Close = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SubmitInternal did not return")
	}
	go func() { closeDone <- loop.Close() }()
	waitContractSignal(t, closeTerminated, "Close terminal-state publication")
	close(releaseOuter)

	select {
	case <-directTaskRan:
		t.Fatal("queued SubmitInternal task ran after Close published StateTerminated")
	default:
	}

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not finish")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not finish")
	}
}

func TestTerminalDrain_EndFunctionIdempotent(t *testing.T) {
	loop := New()
	end := loop.beginTerminalDrain()
	end()
	end()
}

func TestTerminalDrain_TryBeginCASFailureDoesNotRejectTimer(t *testing.T) {
	loop := New()
	loop.state.Store(StateRunning)
	end, ok := loop.tryBeginTerminalDrainTransition(StateSleeping, StateTerminating)
	if ok {
		end()
		t.Fatal("tryBeginTerminalDrainTransition unexpectedly succeeded")
	}
	if loop.terminalDraining.Load() {
		t.Fatal("terminalDraining was published after failed transition")
	}
	id, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer after failed drain transition: %v", err)
	}
	if id == 0 {
		t.Fatal("ScheduleTimer returned id 0 after failed drain transition")
	}
}

func TestTerminalDrain_AutoExitModeChangesOnlyAfterSuccessfulTransition(t *testing.T) {
	newPendingImmediate := func(t *testing.T) (*Loop, *atomic.Bool, *atomic.Bool) {
		t.Helper()
		loop := New()
		registerFDResourceCleanupT(t, loop)
		refed := new(atomic.Bool)
		ran := new(atomic.Bool)
		if err := loop.ScheduleImmediateRef(func() { ran.Store(true) }, refed.Load); err != nil {
			t.Fatal(err)
		}
		loop.drainCommandIngress()
		return loop, refed, ran
	}

	t.Run("failed-stale-transition-preserves-skip", func(t *testing.T) {
		loop, refed, ran := newPendingImmediate(t)
		finish := loop.beginAutoExitTerminalDrain()
		defer finish()
		if loop.hasLiveCheckJob(loop.snapshotOwnerCheckJobs()) {
			t.Fatal("final owner predicate evaluation = true, want false")
		}

		loop.state.Store(StateTerminated)
		refed.Store(true)
		if end, ok := loop.tryBeginTerminalDrainTransition(StateRunning, StateTerminating); ok {
			end()
			t.Fatal("stale graceful transition unexpectedly succeeded")
		}
		loop.drainTerminalCheckJobs()
		if ran.Load() {
			t.Fatal("failed stale transition revived final-false immediate")
		}
	})

	t.Run("successful-graceful-transition-upgrades-drain", func(t *testing.T) {
		loop, _, ran := newPendingImmediate(t)
		finish := loop.beginAutoExitTerminalDrain()
		defer finish()
		if loop.hasLiveCheckJob(loop.snapshotOwnerCheckJobs()) {
			t.Fatal("final owner predicate evaluation = true, want false")
		}

		loop.state.Store(StateRunning)
		end, ok := loop.tryBeginTerminalDrainTransition(StateRunning, StateTerminating)
		if !ok {
			t.Fatal("graceful transition did not upgrade active auto-exit drain")
		}
		loop.drainTerminalCheckJobs()
		end()
		if !ran.Load() {
			t.Fatal("successful graceful transition did not retain accepted check callback")
		}
	})
}

func terminalDrainRecorder() (func() []string, func(string)) {
	var mu sync.Mutex
	var order []string
	return func() []string {
			mu.Lock()
			defer mu.Unlock()
			return append([]string(nil), order...)
		}, func(entry string) {
			mu.Lock()
			order = append(order, entry)
			mu.Unlock()
		}
}

func terminalDrainRequireOrder(t *testing.T, order func() []string, want []string) {
	t.Helper()
	got := order()
	if !slices.Equal(got, want) {
		t.Fatalf("terminal drain order = %v, want %v", got, want)
	}
}

func terminalDrainRequireTimerRejected(t *testing.T, timerID TimerID, err error) {
	t.Helper()
	if err != ErrLoopTerminated {
		t.Fatalf("ScheduleTimer from terminal callback error = %v, want ErrLoopTerminated", err)
	}
	if timerID != 0 {
		t.Fatalf("ScheduleTimer from terminal callback timerID = %d, want 0", timerID)
	}
}
