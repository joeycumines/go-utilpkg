package eventloop

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/joeycumines/goroutineid"
)

func TestTerminalDrain_AutoExitCallbackCanScheduleNextTickAndMicrotask(t *testing.T) {
	loop := New(WithAutoExit(true))

	enteredTermination := make(chan struct{})
	releaseTermination := make(chan struct{})
	order, record := terminalDrainRecorder()
	var nextErr, microErr, timerErr error
	var timerID TimerID
	loop.testHooks = &loopTestHooks{
		BeforeTerminateState: func() {
			record("task")
			nextErr = loop.ScheduleNextTick(func() { record("nextTick") })
			microErr = loop.ScheduleMicrotask(func() { record("microtask") })
			timerID, timerErr = loop.ScheduleTimer(time.Hour, func() {})
			record("scheduled")
			close(enteredTermination)
			<-releaseTermination
		},
	}

	keepaliveID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()
	waitRefedTimerCount(t, loop, 1)

	if err := loop.UnrefTimer(keepaliveID); err != nil {
		t.Fatalf("UnrefTimer: %v", err)
	}

	select {
	case <-enteredTermination:
	case <-time.After(5 * time.Second):
		t.Fatal("termination hook did not trigger")
	}

	close(releaseTermination)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after terminal drain")
	}

	if nextErr != nil {
		t.Fatalf("ScheduleNextTick from terminal callback: %v", nextErr)
	}
	if microErr != nil {
		t.Fatalf("ScheduleMicrotask from terminal callback: %v", microErr)
	}
	terminalDrainRequireTimerRejected(t, timerID, timerErr)
	terminalDrainRequireOrder(t, order, []string{"task", "scheduled", "nextTick", "microtask"})
}

func TestTerminalDrain_GracefulBacklogUsesTickNonTimerOrder(t *testing.T) {
	loop := New()

	runEntered := make(chan struct{})
	releaseRun := make(chan struct{})
	shutdownCommitted := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterRunStateRunningBeforeStart: func() {
			close(runEntered)
			<-releaseRun
		},
		AfterShutdownStateTerminating: func() {
			close(shutdownCommitted)
		},
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, runEntered, "Run StateRunning publication")

	order, record := terminalDrainRecorder()
	var checkMicroErr, closeMicroErr, internalMicroErr, externalMicroErr error
	if err := loop.ScheduleImmediate(func() {
		record("check")
		checkMicroErr = loop.ScheduleMicrotask(func() { record("check-micro") })
	}); err != nil {
		t.Fatalf("ScheduleImmediate: %v", err)
	}
	if err := loop.ScheduleCloseCallback(func() {
		record("close")
		closeMicroErr = loop.ScheduleMicrotask(func() { record("close-micro") })
	}); err != nil {
		t.Fatalf("ScheduleCloseCallback: %v", err)
	}
	if err := loop.SubmitInternal(func() {
		record("internal")
		internalMicroErr = loop.ScheduleMicrotask(func() { record("internal-micro") })
	}); err != nil {
		t.Fatalf("SubmitInternal: %v", err)
	}
	if err := loop.Submit(func() {
		record("external")
		externalMicroErr = loop.ScheduleMicrotask(func() { record("external-micro") })
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if _, err := loop.ScheduleTimer(0, func() { record("timer") }); err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(context.Background()) }()
	waitContractSignal(t, shutdownCommitted, "Shutdown StateTerminating publication")
	close(releaseRun)

	if err := waitContractValue(t, runDone, "Run graceful terminal drain"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := waitContractValue(t, shutdownDone, "Shutdown graceful terminal drain"); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	for name, err := range map[string]error{
		"check":    checkMicroErr,
		"close":    closeMicroErr,
		"internal": internalMicroErr,
		"external": externalMicroErr,
	} {
		if err != nil {
			t.Errorf("%s callback ScheduleMicrotask: %v", name, err)
		}
	}
	terminalDrainRequireOrder(t, order, []string{
		"check", "check-micro",
		"close", "close-micro",
		"internal", "internal-micro",
		"external", "external-micro",
	})
	if got := order(); slices.Contains(got, "timer") {
		t.Fatalf("timer ran during graceful terminal drain: order=%v", got)
	}
}

func TestTerminalDrain_FinishRejectsDiagnosticsAfterEmptySnapshot(t *testing.T) {
	loop := New()

	hookEntered := make(chan struct{})
	scheduleResult := make(chan bool, 1)
	loop.testHooks = &loopTestHooks{
		BeforeTerminalDrainFinish: func() {
			close(hookEntered)
			go func() {
				scheduleResult <- loop.scheduleTerminalDiagnostic(func() {
					t.Error("late terminal diagnostic executed after empty snapshot")
				})
			}()
		},
	}

	finish := loop.beginTerminalDrain()
	finish()

	select {
	case <-hookEntered:
	case <-time.After(time.Second):
		t.Fatal("terminal drain finish hook did not run")
	}

	select {
	case accepted := <-scheduleResult:
		if accepted {
			t.Fatal("terminal diagnostic was accepted after empty snapshot began closing the drain")
		}
	case <-time.After(time.Second):
		t.Fatal("late terminal diagnostic scheduling did not complete")
	}
}

func TestTerminalDrain_PreRunCallbackCloseIsReentrant(t *testing.T) {
	loop := New()

	closeErr := make(chan error, 1)
	if err := loop.Submit(func() {
		closeErr <- loop.Close()
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	shutdownErr := make(chan error, 1)
	go func() { shutdownErr <- loop.Shutdown(context.Background()) }()

	select {
	case err := <-closeErr:
		if !errors.Is(err, ErrReentrantClose) {
			t.Fatalf("Close from pre-Run terminal-drain callback = %v, want ErrReentrantClose", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close from pre-Run terminal-drain callback deadlocked")
	}

	select {
	case err := <-shutdownErr:
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not complete after reentrant Close")
	}
}

func TestTerminalDrain_AdmissionWaitsForDrainPublication(t *testing.T) {
	loop := New()

	loop.state.Store(StateRunning)
	loop.terminalDrainMu.Lock()
	loop.state.Store(StateTerminating)
	drainSync := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		BeforeTerminalEphemeralDrainSync: func() { close(drainSync) },
	}

	gid := make(chan int64, 1)
	scheduleDone := make(chan error, 1)
	go func() {
		id := goroutineid.Get()
		loop.loopGoroutineID.Store(id)
		gid <- id
		scheduleDone <- loop.ScheduleMicrotask(func() {})
	}()

	owner := waitContractValue(t, gid, "terminal drain owner publication")
	waitContractSignal(t, drainSync, "terminal ephemeral drain synchronization")

	done := make(chan struct{})
	loop.terminalDrainDone = done
	loop.terminalDrainOwner.Store(owner)
	loop.terminalDraining.Store(true)
	loop.terminalDrainMu.Unlock()

	select {
	case err := <-scheduleDone:
		if err != nil {
			t.Fatalf("ScheduleMicrotask after synchronized drain publication: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ScheduleMicrotask did not resume after terminal drain publication")
	}
	if loop.popOwnerPromiseMicrotask().fn == nil {
		t.Fatal("terminal microtask continuation was not admitted")
	}
	loop.finishTerminalDrain(done)
}

func TestTerminalDrain_ContextCancelCallbackCanScheduleNextTickAndMicrotask(t *testing.T) {
	loop := New()

	enteredTermination := make(chan struct{})
	releaseTermination := make(chan struct{})
	order, record := terminalDrainRecorder()
	var nextErr, microErr, timerErr error
	var timerID TimerID
	loop.testHooks = &loopTestHooks{
		BeforeTerminateState: func() {
			record("task")
			nextErr = loop.ScheduleNextTick(func() { record("nextTick") })
			microErr = loop.ScheduleMicrotask(func() { record("microtask") })
			timerID, timerErr = loop.ScheduleTimer(time.Hour, func() {})
			record("scheduled")
			close(enteredTermination)
			<-releaseTermination
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- loop.Run(ctx) }()
	waitLoopOwnerTurnT(t, loop)

	cancel()

	select {
	case <-enteredTermination:
	case <-time.After(5 * time.Second):
		t.Fatal("termination hook did not trigger")
	}

	close(releaseTermination)

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after context cancellation drain")
	}

	if nextErr != nil {
		t.Fatalf("ScheduleNextTick from context terminal callback: %v", nextErr)
	}
	if microErr != nil {
		t.Fatalf("ScheduleMicrotask from context terminal callback: %v", microErr)
	}
	terminalDrainRequireTimerRejected(t, timerID, timerErr)
	terminalDrainRequireOrder(t, order, []string{"task", "scheduled", "nextTick", "microtask"})
}

func TestTerminalDrain_PublicShutdownCallbackCanScheduleNextTickAndMicrotask(t *testing.T) {
	loop := New()
	terminating := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() { close(terminating) },
	}

	taskStarted := make(chan struct{})
	releaseTask := make(chan struct{})
	if err := loop.Submit(func() {
		close(taskStarted)
		<-releaseTask
	}); err != nil {
		t.Fatalf("blocking Submit: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- loop.Run(context.Background()) }()

	select {
	case <-taskStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("blocking task did not start")
	}

	order, record := terminalDrainRecorder()
	var nextErr, microErr, checkpointErr, timerErr error
	var timerID TimerID
	if err := loop.Submit(func() {
		record("task")
		nextErr = loop.ScheduleNextTick(func() { record("nextTick") })
		microErr = loop.ScheduleMicrotask(func() { record("microtask") })
		checkpointErr = loop.scheduleMicrotaskCheckpoint(func() { record("checkpoint") })
		timerID, timerErr = loop.ScheduleTimer(time.Hour, func() {})
		record("scheduled")
	}); err != nil {
		close(releaseTask)
		t.Fatalf("queued terminal Submit before public Shutdown: %v", err)
	}

	shutdownErr := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownErr <- loop.Shutdown(ctx)
	}()

	waitContractSignal(t, terminating, "Shutdown StateTerminating publication")

	close(releaseTask)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return during public Shutdown")
	}

	select {
	case err := <-shutdownErr:
		if err != nil {
			t.Fatalf("Shutdown returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not return")
	}

	if nextErr != nil {
		t.Fatalf("ScheduleNextTick from public shutdown terminal callback: %v", nextErr)
	}
	if microErr != nil {
		t.Fatalf("ScheduleMicrotask from public shutdown terminal callback: %v", microErr)
	}
	if checkpointErr != nil {
		t.Fatalf("scheduleMicrotaskCheckpoint from public shutdown terminal callback: %v", checkpointErr)
	}
	terminalDrainRequireTimerRejected(t, timerID, timerErr)
	terminalDrainRequireOrder(t, order, []string{"task", "scheduled", "nextTick", "microtask", "checkpoint"})
}

func TestTerminalDrain_PublicShutdownQueuedCallbackRunsOnLoopGoroutine(t *testing.T) {
	loop := New()
	terminating := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() { close(terminating) },
	}

	var loopGID int64
	var terminalGID int64
	blockingTaskStarted := make(chan struct{})
	releaseBlockingTask := make(chan struct{})
	if err := loop.Submit(func() {
		loopGID = goroutineid.Get()
		close(blockingTaskStarted)
		<-releaseBlockingTask
	}); err != nil {
		t.Fatalf("blocking Submit: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case <-blockingTaskStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("blocking task did not start")
	}

	terminalRan := make(chan struct{})
	if err := loop.Submit(func() {
		terminalGID = goroutineid.Get()
		if !loop.isLoopThread() {
			t.Errorf("terminal callback did not run on loop goroutine")
		}
		close(terminalRan)
	}); err != nil {
		t.Fatalf("queued terminal Submit: %v", err)
	}

	shutdownErr := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownErr <- loop.Shutdown(ctx)
	}()

	waitContractSignal(t, terminating, "Shutdown StateTerminating publication")

	close(releaseBlockingTask)

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return during public Shutdown")
	}
	select {
	case err := <-shutdownErr:
		if err != nil {
			t.Fatalf("Shutdown returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not return")
	}
	select {
	case <-terminalRan:
	default:
		t.Fatal("queued terminal callback did not run")
	}
	if terminalGID == 0 || loopGID == 0 {
		t.Fatalf("goroutine ids were not captured: loop=%d terminal=%d", loopGID, terminalGID)
	}
	if terminalGID != loopGID {
		t.Fatalf("terminal callback goroutine = %d, want loop goroutine %d", terminalGID, loopGID)
	}
}

func TestTerminalDrain_PublicShutdownOwnerAllowsOnlyMicrotaskContinuations(t *testing.T) {
	loop := New()
	terminating := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() { close(terminating) },
	}

	blockingTaskStarted := make(chan struct{})
	releaseBlockingTask := make(chan struct{})
	if err := loop.Submit(func() {
		close(blockingTaskStarted)
		<-releaseBlockingTask
	}); err != nil {
		t.Fatalf("submit blocking task: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	select {
	case <-blockingTaskStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("blocking task did not start")
	}

	order, record := terminalDrainRecorder()
	var submitErr, internalErr, microErr, nextTickErr, checkpointErr, immediateErr, closeErr error
	if err := loop.Submit(func() {
		record("owner")
		submitErr = loop.Submit(func() { record("submit") })
		internalErr = loop.SubmitInternal(func() { record("internal") })
		microErr = loop.ScheduleMicrotask(func() { record("microtask") })
		nextTickErr = loop.ScheduleNextTick(func() { record("nextTick") })
		checkpointErr = loop.ScheduleMicrotaskCheckpoint(func() { record("checkpoint") })
		immediateErr = loop.ScheduleImmediate(func() { record("immediate") })
		closeErr = loop.ScheduleCloseCallback(func() { record("close") })
		record("scheduled")
	}); err != nil {
		close(releaseBlockingTask)
		t.Fatalf("queued terminal Submit before public Shutdown: %v", err)
	}

	shutdownErr := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownErr <- loop.Shutdown(ctx)
	}()

	waitContractSignal(t, terminating, "Shutdown StateTerminating publication")

	close(releaseBlockingTask)

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return during public Shutdown")
	}
	select {
	case err := <-shutdownErr:
		if err != nil {
			t.Fatalf("Shutdown returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not return")
	}

	if !errors.Is(submitErr, ErrLoopTerminated) {
		t.Fatalf("Submit from owner terminal-drain callback err = %v, want ErrLoopTerminated", submitErr)
	}
	if !errors.Is(internalErr, ErrLoopTerminated) {
		t.Fatalf("SubmitInternal from owner terminal-drain callback err = %v, want ErrLoopTerminated", internalErr)
	}
	if microErr != nil {
		t.Fatalf("ScheduleMicrotask from owner terminal-drain callback: %v", microErr)
	}
	if nextTickErr != nil {
		t.Fatalf("ScheduleNextTick from owner terminal-drain callback: %v", nextTickErr)
	}
	if checkpointErr != nil {
		t.Fatalf("ScheduleMicrotaskCheckpoint from owner terminal-drain callback: %v", checkpointErr)
	}
	if !errors.Is(immediateErr, ErrLoopTerminated) {
		t.Fatalf("ScheduleImmediate from owner terminal-drain callback err = %v, want ErrLoopTerminated", immediateErr)
	}
	if !errors.Is(closeErr, ErrLoopTerminated) {
		t.Fatalf("ScheduleCloseCallback from owner terminal-drain callback err = %v, want ErrLoopTerminated", closeErr)
	}

	got := order()
	for _, want := range []string{"owner", "scheduled", "microtask", "nextTick", "checkpoint"} {
		if !slices.Contains(got, want) {
			t.Fatalf("terminal owner microtask continuation %q did not run; order=%v", want, got)
		}
	}
	for _, rejected := range []string{"submit", "internal", "immediate", "close"} {
		if slices.Contains(got, rejected) {
			t.Fatalf("terminal owner queue continuation %q unexpectedly ran; order=%v", rejected, got)
		}
	}
}

func TestTerminalDrain_PublicShutdownRunningCallbackCanScheduleContinuationAfterTerminating(t *testing.T) {
	loop := New()

	stateTerminating := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() {
			close(stateTerminating)
		},
	}

	order, record := terminalDrainRecorder()
	callbackStarted := make(chan struct{})
	callbackDone := make(chan struct{})
	var nextErr, microErr, checkpointErr error
	if err := loop.Submit(func() {
		close(callbackStarted)
		<-stateTerminating
		record("task")
		nextErr = loop.ScheduleNextTick(func() { record("nextTick") })
		microErr = loop.ScheduleMicrotask(func() { record("microtask") })
		checkpointErr = loop.scheduleMicrotaskCheckpoint(func() { record("checkpoint") })
		record("scheduled")
		close(callbackDone)
	}); err != nil {
		t.Fatalf("blocking Submit: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	select {
	case <-callbackStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("running callback did not start")
	}

	shutdownErr := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownErr <- loop.Shutdown(ctx)
	}()

	select {
	case <-callbackDone:
	case <-time.After(5 * time.Second):
		t.Fatal("running callback did not finish after StateTerminating")
	}

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return during public Shutdown")
	}
	select {
	case err := <-shutdownErr:
		if err != nil {
			t.Fatalf("Shutdown returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not return")
	}

	if nextErr != nil {
		t.Fatalf("ScheduleNextTick from running callback after StateTerminating: %v", nextErr)
	}
	if microErr != nil {
		t.Fatalf("ScheduleMicrotask from running callback after StateTerminating: %v", microErr)
	}
	if checkpointErr != nil {
		t.Fatalf("scheduleMicrotaskCheckpoint from running callback after StateTerminating: %v", checkpointErr)
	}
	terminalDrainRequireOrder(t, order, []string{"task", "scheduled", "nextTick", "microtask", "checkpoint"})
}

func TestTerminalDrain_ShutdownDrainCallbackCanScheduleNextTickAndMicrotask(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	order, record := terminalDrainRecorder()
	var nextErr, microErr, timerErr error
	var timerID TimerID
	if err := loop.Submit(func() {
		record("task")
		nextErr = loop.ScheduleNextTick(func() { record("nextTick") })
		microErr = loop.ScheduleMicrotask(func() { record("microtask") })
		timerID, timerErr = loop.ScheduleTimer(time.Hour, func() {})
		record("scheduled")
	}); err != nil {
		t.Fatalf("Submit before shutdown: %v", err)
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	if err := loop.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	if nextErr != nil {
		t.Fatalf("ScheduleNextTick from shutdown terminal callback: %v", nextErr)
	}
	if microErr != nil {
		t.Fatalf("ScheduleMicrotask from shutdown terminal callback: %v", microErr)
	}
	terminalDrainRequireTimerRejected(t, timerID, timerErr)
	terminalDrainRequireOrder(t, order, []string{"task", "scheduled", "nextTick", "microtask"})
}

func TestTerminalDrain_PublicShutdownContextCancelDoesNotOverwriteOwner(t *testing.T) {
	loop := New()

	stateTerminating := make(chan struct{})
	releaseShutdownHook := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() {
			close(stateTerminating)
			<-releaseShutdownHook
		},
	}

	blockingTaskStarted := make(chan struct{})
	releaseBlockingTask := make(chan struct{})
	if err := loop.Submit(func() {
		close(blockingTaskStarted)
		<-releaseBlockingTask
	}); err != nil {
		t.Fatalf("blocking Submit: %v", err)
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(runCtx) }()
	select {
	case <-blockingTaskStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("blocking task did not start")
	}

	order, record := terminalDrainRecorder()
	var nextErr, microErr, timerErr error
	var timerID TimerID
	if err := loop.Submit(func() {
		record("task")
		nextErr = loop.ScheduleNextTick(func() { record("nextTick") })
		microErr = loop.ScheduleMicrotask(func() { record("microtask") })
		timerID, timerErr = loop.ScheduleTimer(time.Hour, func() {})
		record("scheduled")
	}); err != nil {
		close(releaseBlockingTask)
		t.Fatalf("queued terminal Submit before public Shutdown: %v", err)
	}

	shutdownErr := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownErr <- loop.Shutdown(ctx)
	}()

	select {
	case <-stateTerminating:
	case <-time.After(5 * time.Second):
		close(releaseBlockingTask)
		t.Fatal("public Shutdown did not enter StateTerminating")
	}

	cancelRun()
	close(releaseBlockingTask)
	close(releaseShutdownHook)

	select {
	case err := <-runDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after public Shutdown/context cancellation race")
	}
	select {
	case err := <-shutdownErr:
		if err != nil {
			t.Fatalf("Shutdown returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not return after context cancellation race")
	}

	if nextErr != nil {
		t.Fatalf("ScheduleNextTick after owner race: %v", nextErr)
	}
	if microErr != nil {
		t.Fatalf("ScheduleMicrotask after owner race: %v", microErr)
	}
	terminalDrainRequireTimerRejected(t, timerID, timerErr)
	terminalDrainRequireOrder(t, order, []string{"task", "scheduled", "nextTick", "microtask"})
}

func TestTerminalDrain_ShutdownTimeoutDoesNotEndLoopOwnedTerminalDrain(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	runCtx := t.Context()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(runCtx) }()

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

	microtaskRan := make(chan struct{})
	scheduleErr := make(chan error, 1)
	if err := loop.Submit(func() {
		scheduleErr <- loop.ScheduleMicrotask(func() {
			close(microtaskRan)
		})
	}); err != nil {
		releaseBlocking()
		t.Fatalf("submit terminal-drain task: %v", err)
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancelShutdown()
	shutdownErr := loop.Shutdown(shutdownCtx)
	if !errors.Is(shutdownErr, context.DeadlineExceeded) {
		releaseBlocking()
		t.Fatalf("Shutdown err = %v, want context deadline exceeded", shutdownErr)
	}

	releaseBlocking()

	select {
	case err := <-scheduleErr:
		if err != nil {
			t.Fatalf("terminal-drain callback could not enqueue microtask: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("terminal-drain callback did not run")
	}

	select {
	case <-microtaskRan:
	case <-time.After(5 * time.Second):
		t.Fatal("microtask scheduled by accepted terminal-drain callback did not run")
	}

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after terminal drain")
	}
}
