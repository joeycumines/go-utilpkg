package eventloop

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestCloseHandsRejectedNormalRejectionHandlerToTerminalFallback(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	handlerStarted := make(chan any, 1)
	handlerDone := make(chan struct{})
	closeReturned := make(chan struct{})
	js := NewJS(loop,
		WithUnhandledRejection(func(reason any) {
			handlerStarted <- reason
			<-closeReturned
			close(handlerDone)
		}),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
	)

	handlerAdmissionReached := make(chan struct{})
	releaseAdmission := make(chan struct{})
	var releaseAdmissionOnce sync.Once
	var handlerAdmissionOnce sync.Once
	closeTransitioned := make(chan struct{})
	releaseAdmissionFn := func() { releaseAdmissionOnce.Do(func() { close(releaseAdmission) }) }
	t.Cleanup(releaseAdmissionFn)
	loop.testHooks = &loopTestHooks{
		BeforeUnhandledRejectionCallback: func() {
			handlerAdmissionOnce.Do(func() {
				close(handlerAdmissionReached)
				<-releaseAdmission
			})
		},
		AfterCloseStateTerminating: func() { close(closeTransitioned) },
	}
	_, _, reject := js.NewChainedPromise()
	reject("normal-checkpoint")

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case <-handlerAdmissionReached:
	case <-time.After(5 * time.Second):
		t.Fatal("normal unhandled-rejection check did not reach user callback admission")
	}

	closeDone := make(chan error, 1)
	go func() {
		err := loop.Close()
		close(closeReturned)
		closeDone <- err
	}()
	waitContractSignal(t, closeTransitioned, "Close StateTerminating publication")
	releaseAdmissionFn()

	select {
	case reason := <-handlerStarted:
		if reason != "normal-checkpoint" {
			t.Fatalf("terminal fallback reason = %v, want normal-checkpoint", reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("rejected normal handler was not handed to terminal fallback")
	}
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close remained blocked by the isolated terminal fallback handler")
	}
	select {
	case <-handlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("terminal fallback handler did not finish after Close returned")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not exit after Close")
	}
	waitTerminalUnhandledRejectionTrackingDrained(t, js)
}

func TestCloseTerminalFallbackUpgradesActiveNormalRejectionCheck(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	reasons := make(chan any, 2)
	terminalHandlerStarted := make(chan struct{})
	terminalHandlerDone := make(chan struct{})
	closeReturned := make(chan struct{})
	js := NewJS(loop,
		WithUnhandledRejection(func(reason any) {
			reasons <- reason
			if reason == "late-terminal" {
				close(terminalHandlerStarted)
				<-closeReturned
				close(terminalHandlerDone)
			}
		}),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
	)

	checkCleared := make(chan struct{})
	fallbackRerun := make(chan struct{})
	closeTerminated := make(chan struct{})
	releaseCheck := make(chan struct{})
	releaseCheckFn := releaseSignalT(t, releaseCheck)
	var checkOnce sync.Once
	var fallbackOnce sync.Once
	loop.testHooks = &loopTestHooks{
		AfterUnhandledRejectionCheckClear: func() {
			checkOnce.Do(func() {
				close(checkCleared)
				<-releaseCheck
			})
		},
		AfterUnhandledRejectionFallbackRerun: func() {
			fallbackOnce.Do(func() { close(fallbackRerun) })
		},
		BeforeClosePromiseRejection: func() { close(closeTerminated) },
	}

	_, _, rejectInitial := js.NewChainedPromise()
	rejectInitial("initial")
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case <-checkCleared:
	case <-time.After(5 * time.Second):
		t.Fatal("normal rejection checker did not pause after clearing its scheduled flag")
	}
	select {
	case reason := <-reasons:
		if reason != "initial" {
			t.Fatalf("initial unhandled reason = %v, want initial", reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("normal rejection checker did not report the initial rejection")
	}

	closeDone := make(chan error, 1)
	go func() {
		err := loop.Close()
		close(closeReturned)
		closeDone <- err
	}()
	waitContractSignal(t, closeTerminated, "Close terminal-state publication")

	// This post-terminal rejection cannot schedule a loop checkpoint. Its
	// fallback collides with the paused normal checker and must upgrade that
	// checker's handling of this rejection instead of being reduced to a
	// mode-less rerun through the closed normal callback gate.
	_, _, rejectLate := js.NewChainedPromise()
	rejectLate("late-terminal")
	select {
	case <-fallbackRerun:
	case <-time.After(5 * time.Second):
		t.Fatal("terminal fallback did not collide with the active normal checker")
	}
	releaseCheckFn()

	select {
	case <-terminalHandlerStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("terminal fallback did not start after normal-checker handoff")
	}
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return while the isolated terminal fallback handler waited for it")
	}
	select {
	case <-terminalHandlerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("terminal fallback handler did not complete after Close returned")
	}
	select {
	case reason := <-reasons:
		if reason != "late-terminal" {
			t.Fatalf("fallback unhandled reason = %v, want late-terminal", reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("terminal fallback reason was not delivered")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Close")
	}
	select {
	case reason := <-reasons:
		t.Fatalf("unexpected duplicate unhandled rejection: %v", reason)
	default:
	}
	waitTerminalUnhandledRejectionTrackingDrained(t, js)
}

func TestTerminalFallbackTakesOwnershipAfterNormalCheckerExit(t *testing.T) {
	t.Run("isolated", func(t *testing.T) {
		testTerminalFallbackTakesOwnershipAfterNormalCheckerExit(t, UnhandledRejectionFallbackIsolated, true)
	})
	t.Run("disabled", func(t *testing.T) {
		testTerminalFallbackTakesOwnershipAfterNormalCheckerExit(t, UnhandledRejectionFallbackDisabled, false)
	})
}

func testTerminalFallbackTakesOwnershipAfterNormalCheckerExit(t *testing.T, fallbackMode UnhandledRejectionFallbackMode, wantLateCallback bool) {
	t.Helper()
	loop := New()
	registerLoopCleanupT(t, loop)

	reasons := make(chan any, 2)
	js := NewJS(
		loop,
		WithUnhandledRejection(func(reason any) { reasons <- reason }),
		WithUnhandledRejectionFallback(fallbackMode),
	)

	checkCleared := make(chan struct{})
	releaseCheck := make(chan struct{})
	releaseCheckFn := releaseSignalT(t, releaseCheck)
	rerunRequestReached := make(chan struct{})
	releaseRerunRequest := make(chan struct{})
	releaseRerunRequestFn := releaseSignalT(t, releaseRerunRequest)
	var checkOnce sync.Once
	var requestOnce sync.Once
	loop.testHooks = &loopTestHooks{
		AfterUnhandledRejectionCheckClear: func() {
			checkOnce.Do(func() {
				close(checkCleared)
				<-releaseCheck
			})
		},
		BeforeUnhandledRejectionRerunRequest: func() {
			requestOnce.Do(func() {
				close(rerunRequestReached)
				<-releaseRerunRequest
			})
		},
	}

	_, _, rejectInitial := js.NewChainedPromise()
	rejectInitial("initial")
	normalCheckDone := make(chan struct{})
	go func() {
		js.runRejectionCheck()
		close(normalCheckDone)
	}()
	select {
	case <-checkCleared:
	case <-time.After(5 * time.Second):
		t.Fatal("normal rejection checker did not pause before ownership release")
	}
	select {
	case reason := <-reasons:
		if reason != "initial" {
			t.Fatalf("initial unhandled reason = %v, want initial", reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("normal rejection checker did not report the initial rejection")
	}

	_, _, rejectLate := js.NewChainedPromise()
	rejectLate("late-terminal")
	fallbackDone := make(chan struct{})
	go func() {
		js.runUnhandledRejectionFallback()
		close(fallbackDone)
	}()
	select {
	case <-rerunRequestReached:
	case <-time.After(5 * time.Second):
		t.Fatal("terminal fallback did not pause before rerun ownership synchronization")
	}

	// Let the normal checker exit before the fallback publishes a rerun. The
	// fallback must recheck under the ownership mutex, take ownership itself, and
	// process the terminal record rather than leaving a lost rerun bit.
	releaseCheckFn()
	waitContractSignal(t, normalCheckDone, "normal rejection checker ownership release")
	releaseRerunRequestFn()

	select {
	case <-fallbackDone:
	case <-time.After(5 * time.Second):
		t.Fatal("terminal fallback did not return after taking checker ownership")
	}
	if wantLateCallback {
		select {
		case reason := <-reasons:
			if reason != "late-terminal" {
				t.Fatalf("fallback unhandled reason = %v, want late-terminal", reason)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("terminal fallback was lost after the normal checker exited")
		}
	} else {
		select {
		case reason := <-reasons:
			t.Fatalf("disabled terminal fallback invoked callback with %v", reason)
		default:
		}
	}
	waitUnhandledRejectionCheckOwnershipReleased(t, js)
	assertUnhandledRejectionTrackingDrained(t, js)
}
