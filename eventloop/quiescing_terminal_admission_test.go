package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSubmitToQueueAllowsPlainWorkDuringCurrentQuiescing(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)

	loop.beginQuiescing()

	var ran bool
	if err := loop.submitToQueue(func() { ran = true }); err != nil {
		t.Fatalf("plain submitToQueue during same-epoch quiescing returned %v", err)
	}
	loop.externalMu.Lock()
	commands := loop.commands.Len()
	loop.externalMu.Unlock()
	if commands != 1 {
		t.Fatalf("command ingress length = %d, want 1", commands)
	}
	loop.drainCommandIngress()
	loop.drainTerminalInternalQueue()
	if !ran {
		t.Fatal("queued plain work did not run when drained")
	}
}

func TestAutoExitTerminalAdmissionRejectsNonOwnerWork(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)

	triggerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer trigger: %v", err)
	}
	if err := loop.UnrefTimer(triggerID); err != nil {
		t.Fatalf("UnrefTimer trigger: %v", err)
	}

	enteredTermination := make(chan struct{})
	releaseTermination := make(chan struct{})
	release := contractRelease(t, releaseTermination)
	var hookOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeTerminateState: func() {
			hookOnce.Do(func() {
				close(enteredTermination)
				<-releaseTermination
			})
		},
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, enteredTermination, "auto-exit terminal admission")

	if state := loop.State(); state != StateRunning {
		t.Fatalf("state during committed auto-exit drain = %v, want StateRunning", state)
	}
	if !loop.terminalDrainActive() {
		t.Fatal("terminal drain was inactive before StateTerminated publication")
	}

	var callbackCalls atomic.Int32
	if id, scheduleErr := loop.ScheduleTimer(time.Hour, func() { callbackCalls.Add(1) }); id != 0 || scheduleErr != ErrLoopTerminated {
		t.Fatalf("ScheduleTimer during terminal drain = (%d, %v), want (0, ErrLoopTerminated)", id, scheduleErr)
	}
	if refErr := loop.RefTimer(triggerID); refErr != ErrLoopTerminated {
		t.Fatalf("RefTimer during terminal drain = %v, want ErrLoopTerminated", refErr)
	}
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		callbackCalls.Add(1)
		return nil, nil
	})
	assertPromiseRejected(t, promise, ErrLoopTerminated)
	if submitErr := loop.Submit(func() { callbackCalls.Add(1) }); submitErr != ErrLoopTerminated {
		t.Fatalf("Submit during terminal drain = %v, want ErrLoopTerminated", submitErr)
	}
	if got := callbackCalls.Load(); got != 0 {
		t.Fatalf("rejected callbacks ran %d time(s) before termination release", got)
	}

	release()
	if runErr := waitContractValue(t, runDone, "auto-exit terminal-drain Run completion"); runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}
	if state := loop.State(); state != StateTerminated {
		t.Fatalf("final state = %v, want StateTerminated", state)
	}
	if loop.terminalDrainActive() {
		t.Fatal("terminal drain remained active after Run completion")
	}
	if loop.Alive() {
		t.Fatal("Alive returned true after auto-exit termination")
	}
	if got := callbackCalls.Load(); got != 0 {
		t.Fatalf("rejected callbacks ran %d time(s) after termination", got)
	}
}

func TestAutoExitFinalRecheckAcceptsEphemeralWorkAndAbortsTermination(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)

	var hookErr error
	var submitErr error
	var microtaskErr error
	var nextTickErr error
	var checkpointErr error
	var immediateErr error
	var closeErr error
	var ran atomic.Int32
	var injected atomic.Bool
	loop.testHooks = &loopTestHooks{
		BeforeAutoExitCommit: func() {
			if !injected.CompareAndSwap(false, true) {
				return
			}
			done := make(chan struct{})
			go func() {
				submitErr = loop.Submit(func() { ran.Add(1) })
				microtaskErr = loop.ScheduleMicrotask(func() { ran.Add(1) })
				nextTickErr = loop.ScheduleNextTick(func() { ran.Add(1) })
				checkpointErr = loop.ScheduleMicrotaskCheckpoint(func() { ran.Add(1) })
				immediateErr = loop.ScheduleImmediate(func() { ran.Add(1) })
				closeErr = loop.ScheduleCloseCallback(func() { ran.Add(1) })
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				hookErr = errors.New("non-owner terminal admission attempts did not return")
			}
		},
	}

	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if hookErr != nil {
		t.Fatal(hookErr)
	}
	if submitErr != nil {
		t.Fatalf("Submit error = %v, want nil", submitErr)
	}
	if microtaskErr != nil {
		t.Fatalf("ScheduleMicrotask error = %v, want nil", microtaskErr)
	}
	if nextTickErr != nil {
		t.Fatalf("ScheduleNextTick error = %v, want nil", nextTickErr)
	}
	if checkpointErr != nil {
		t.Fatalf("ScheduleMicrotaskCheckpoint error = %v, want nil", checkpointErr)
	}
	if immediateErr != nil {
		t.Fatalf("ScheduleImmediate error = %v, want nil", immediateErr)
	}
	if closeErr != nil {
		t.Fatalf("ScheduleCloseCallback error = %v, want nil", closeErr)
	}
	if ran.Load() != 6 {
		t.Fatalf("accepted callbacks ran %d time(s), want 6", ran.Load())
	}
}

func TestAutoExitPostFinalAliveIngressAbortsBeforeTerminalAdmission(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)

	var hookErr error
	var submitErr error
	var immediateErr error
	var closeErr error
	var timerErr error
	var cancelErr error
	var ran atomic.Int32
	var injected atomic.Bool
	loop.testHooks = &loopTestHooks{
		AfterAutoExitFinalAliveCheck: func() {
			if !injected.CompareAndSwap(false, true) {
				return
			}
			done := make(chan struct{})
			go func() {
				submitErr = loop.Submit(func() {
					ran.Add(1)
					var id TimerID
					id, timerErr = loop.ScheduleTimer(time.Hour, func() {})
					if timerErr == nil {
						cancelErr = loop.CancelTimer(id)
					}
				})
				immediateErr = loop.ScheduleImmediate(func() { ran.Add(1) })
				closeErr = loop.ScheduleCloseCallback(func() { ran.Add(1) })
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				hookErr = errors.New("post-final-Alive Submit did not return")
			}
		},
	}

	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if hookErr != nil {
		t.Fatal(hookErr)
	}
	if submitErr != nil {
		t.Fatalf("Submit error = %v, want nil", submitErr)
	}
	if immediateErr != nil {
		t.Fatalf("ScheduleImmediate error = %v, want nil", immediateErr)
	}
	if closeErr != nil {
		t.Fatalf("ScheduleCloseCallback error = %v, want nil", closeErr)
	}
	if timerErr != nil {
		t.Fatalf("ScheduleTimer from post-final-Alive work = %v, want nil", timerErr)
	}
	if cancelErr != nil {
		t.Fatalf("CancelTimer from post-final-Alive work = %v, want nil", cancelErr)
	}
	if ran.Load() != 3 {
		t.Fatalf("post-final-Alive callbacks ran %d time(s), want 3", ran.Load())
	}
}

func TestAutoExitAbortsWhenShutdownOwnsTerminalDrainBeforeCommit(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)

	shutdownDone := make(chan error, 1)
	terminalPublished := make(chan struct{})
	type terminalPublication struct {
		reached  bool
		state    LoopState
		draining bool
	}
	publicationDone := make(chan terminalPublication, 1)
	var injected atomic.Bool
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() { close(terminalPublished) },
		AfterAutoExitFinalAliveCheck: func() {
			if !injected.CompareAndSwap(false, true) {
				return
			}
			go func() { shutdownDone <- loop.Shutdown(context.Background()) }()
			publication := terminalPublication{}
			select {
			case <-terminalPublished:
				publication.reached = true
				publication.state = loop.State()
				publication.draining = loop.terminalDraining.Load()
			case <-time.After(5 * time.Second):
			}
			publicationDone <- publication
		},
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	publication := waitContractValue(t, publicationDone, "Shutdown terminal-drain publication")
	if !publication.reached {
		t.Fatal("Shutdown terminal-drain publication did not occur")
	}
	if publication.state != StateTerminating || !publication.draining {
		t.Fatalf("Shutdown publication = (state %v, draining %v), want (StateTerminating, true)", publication.state, publication.draining)
	}
	if err := waitContractValue(t, runDone, "Run after Shutdown raced auto-exit commit"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := waitContractValue(t, shutdownDone, "Shutdown after auto-exit commit race"); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if !injected.Load() {
		t.Fatal("auto-exit final Alive hook did not run")
	}
	if loop.terminalDraining.Load() {
		t.Fatal("terminalDraining remained true after Shutdown-owned terminal drain completed")
	}
	if _, active := loop.terminalDrainWaiter(); active {
		t.Fatal("terminal drain waiter remained active after Shutdown-owned terminal drain completed")
	}
}

func TestLoopRejectAllPendingPromisesRunsOnce(t *testing.T) {
	loop := New()

	errFirst := errors.New("first terminal rejection")
	errSecond := errors.New("second terminal rejection")
	_, first := loop.registry.NewPromise()
	loop.rejectAllPendingPromises(errFirst)

	if first.State() != Rejected {
		t.Fatalf("first promise state = %v, want Rejected", first.State())
	}
	if first.Result() != errFirst {
		t.Fatalf("first promise reason = %v, want %v", first.Result(), errFirst)
	}

	_, second := loop.registry.NewPromise()
	loop.rejectAllPendingPromises(errSecond)
	if second.State() != Pending {
		t.Fatalf("second promise state = %v, want Pending because rejectAllOnce already fired", second.State())
	}
}

func TestAutoExitTerminalDiagnosticCanUseLivenessAPIWithoutDeadlock(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)

	diagnosticDone := make(chan error, 1)
	var hookErr error
	loop.testHooks = &loopTestHooks{
		BeforeTerminateState: func() {
			if ok := loop.scheduleTerminalDiagnostic(func() {
				_, err := loop.ScheduleTimer(time.Hour, func() {})
				diagnosticDone <- err
			}); !ok {
				hookErr = errors.New("terminal diagnostic was not accepted during active terminal drain")
			}
		},
	}

	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if hookErr != nil {
		t.Fatal(hookErr)
	}

	select {
	case err := <-diagnosticDone:
		if err != ErrLoopTerminated {
			t.Fatalf("ScheduleTimer from terminal diagnostic = %v, want ErrLoopTerminated", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("terminal diagnostic liveness API call did not return; possible livenessMu deadlock")
	}
}

func TestStateAwakeShutdownExternalCloseJoinsCompletion(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	callbackStarted := make(chan struct{})
	releaseCallback := make(chan struct{})
	releaseCallbackFn := releaseSignalT(t, releaseCallback)
	terminalJoined := make(chan struct{})
	var joinedOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeTerminalJoin: func() {
			joinedOnce.Do(func() { close(terminalJoined) })
		},
	}
	if err := loop.Submit(func() {
		close(callbackStarted)
		<-releaseCallback
	}); err != nil {
		t.Fatalf("Submit before Run: %v", err)
	}

	shutdownDone := make(chan error, 1)
	shutdownExited := make(chan struct{})
	t.Cleanup(func() {
		releaseCallbackFn()
		select {
		case <-shutdownExited:
		case <-time.After(5 * time.Second):
			t.Error("cleanup timed out joining Shutdown caller")
		}
	})
	go func() {
		defer close(shutdownExited)
		shutdownDone <- loop.Shutdown(context.Background())
	}()

	select {
	case <-callbackStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("StateAwake Shutdown did not start draining accepted callback")
	}

	select {
	case <-loop.loopDone:
		t.Fatal("loopDone closed before StateAwake Shutdown drain completed")
	default:
	}
	select {
	case <-loop.terminalDone:
		t.Fatal("terminalDone closed before StateAwake Shutdown drain completed")
	default:
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	waitContractSignal(t, terminalJoined, "Close join of StateAwake Shutdown")
	select {
	case err := <-closeDone:
		t.Fatalf("Close returned before StateAwake Shutdown completion: %v", err)
	default:
	}
	select {
	case <-loop.terminalDone:
		t.Fatal("terminalDone closed before the blocked Shutdown callback was released")
	default:
	}
	releaseCallbackFn()

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not return after releasing callback")
	}
	if err := waitContractValue(t, closeDone, "joined Close completion"); err != nil {
		t.Fatalf("joined Close: %v", err)
	}
	select {
	case <-loop.loopDone:
	case <-time.After(time.Second):
		t.Fatal("loopDone was not closed after StateAwake Shutdown cleanup")
	}
	select {
	case <-loop.terminalDone:
	case <-time.After(time.Second):
		t.Fatal("terminalDone was not closed after StateAwake Shutdown cleanup")
	}
}

func TestStateTerminatingRejectsNonOwnerEphemeralWork(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	started := make(chan struct{})
	release := make(chan struct{})
	releaseFn := releaseSignalT(t, release)
	type admissionResult struct {
		completed     bool
		submitErr     error
		microtaskErr  error
		nextTickErr   error
		checkpointErr error
		immediateErr  error
		closeErr      error
	}
	admissionDone := make(chan admissionResult, 1)
	var ran atomic.Int32
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() {
			attemptsDone := make(chan admissionResult, 1)
			go func() {
				attemptsDone <- admissionResult{
					completed:     true,
					submitErr:     loop.Submit(func() { ran.Add(1) }),
					microtaskErr:  loop.ScheduleMicrotask(func() { ran.Add(1) }),
					nextTickErr:   loop.ScheduleNextTick(func() { ran.Add(1) }),
					checkpointErr: loop.ScheduleMicrotaskCheckpoint(func() { ran.Add(1) }),
					immediateErr:  loop.ScheduleImmediate(func() { ran.Add(1) }),
					closeErr:      loop.ScheduleCloseCallback(func() { ran.Add(1) }),
				}
			}()
			select {
			case result := <-attemptsDone:
				admissionDone <- result
			case <-time.After(5 * time.Second):
				admissionDone <- admissionResult{}
			}
			releaseFn()
		},
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	if err := loop.Submit(func() {
		close(started)
		<-release
	}); err != nil {
		t.Fatalf("blocking Submit: %v", err)
	}
	waitContractSignal(t, started, "blocking callback entry")

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(context.Background()) }()
	result := waitContractValue(t, admissionDone, "non-owner StateTerminating admission attempts")
	if !result.completed {
		t.Fatal("non-owner StateTerminating admission attempts did not return")
	}
	if err := waitContractValue(t, shutdownDone, "StateTerminating Shutdown completion"); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "StateTerminating Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.submitErr != ErrLoopTerminated {
		t.Fatalf("Submit error = %v, want ErrLoopTerminated", result.submitErr)
	}
	if result.microtaskErr != ErrLoopTerminated {
		t.Fatalf("ScheduleMicrotask error = %v, want ErrLoopTerminated", result.microtaskErr)
	}
	if result.nextTickErr != ErrLoopTerminated {
		t.Fatalf("ScheduleNextTick error = %v, want ErrLoopTerminated", result.nextTickErr)
	}
	if result.checkpointErr != ErrLoopTerminated {
		t.Fatalf("ScheduleMicrotaskCheckpoint error = %v, want ErrLoopTerminated", result.checkpointErr)
	}
	if result.immediateErr != ErrLoopTerminated {
		t.Fatalf("ScheduleImmediate error = %v, want ErrLoopTerminated", result.immediateErr)
	}
	if result.closeErr != ErrLoopTerminated {
		t.Fatalf("ScheduleCloseCallback error = %v, want ErrLoopTerminated", result.closeErr)
	}
	if ran.Load() != 0 {
		t.Fatalf("terminal-rejected callbacks ran %d time(s)", ran.Load())
	}
}
