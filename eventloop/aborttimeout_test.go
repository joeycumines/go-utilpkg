package eventloop

import (
	"errors"
	"reflect"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/logiface"
)

func TestAbortTimeoutReasonAndThrowIdentity(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	controller, err := AbortTimeout(loop, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}

	reason, ok := controller.Signal().Reason().(*TimeoutError)
	if !ok {
		t.Fatalf("timeout reason = %T, want *TimeoutError", controller.Signal().Reason())
	}
	if got := controller.Signal().ThrowIfAborted(); got != reason {
		t.Fatalf("ThrowIfAborted = %#v, want exact timeout reason %#v", got, reason)
	}
}

func TestAbortTimeoutManualAbortReleasesTimerLiveness(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	controller, err := AbortTimeout(loop, int(time.Hour/time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	controller.Abort("manual")

	done := make(chan error, 1)
	go func() { done <- loop.Run(t.Context()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("manual abort left timeout timer live")
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refed timer count = %d, want 0", got)
	}
}

func TestAbortTimeoutManualClaimSuppressesDetachedTimer(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	controller, err := AbortTimeout(loop, 0)
	if err != nil {
		t.Fatal(err)
	}
	timerDetached := make(chan struct{})
	releaseTimer := make(chan struct{})
	releaseTimerNow := abortContractRelease(t, releaseTimer)
	timeoutClaimed := atomic.Bool{}
	loop.testHooks = &loopTestHooks{BeforeAbortTimeoutClaim: func() {
		close(timerDetached)
		<-releaseTimer
	}, AfterAbortTimeoutClaim: func() {
		timeoutClaimed.Store(true)
	}}

	done := make(chan error, 1)
	go func() { done <- loop.Run(t.Context()) }()
	waitAbortContractSignal(t, timerDetached, "detached timeout callback")
	controller.Abort("manual")
	releaseTimerNow()
	if err := waitAbortContractValue(t, done, "Run after detached timeout suppression"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := controller.Signal().Reason(); got != "manual" {
		t.Fatalf("reason = %#v, want %q", got, "manual")
	}
	if got := controller.timeoutState.winner.Load(); got != abortTimeoutManual {
		t.Fatalf("timeout winner = %d, want manual claim %d", got, abortTimeoutManual)
	}
	if timeoutClaimed.Load() {
		t.Fatal("losing timeout callback crossed the successful-claim boundary")
	}
}

func TestAbortTimeoutTimerClaimPublishesBeforeManualAbortReturns(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	controller, err := AbortTimeout(loop, 0)
	if err != nil {
		t.Fatal(err)
	}
	timerClaimed := make(chan struct{})
	releaseTimer := make(chan struct{})
	releaseTimerNow := abortContractRelease(t, releaseTimer)
	manualWaiting := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterAbortTimeoutClaim: func() {
			close(timerClaimed)
			<-releaseTimer
		},
		BeforeAbortTimeoutPublicationWait: func() { close(manualWaiting) },
	}
	var handlerCalls atomic.Int32
	controller.Signal().OnAbort(func(any) {
		handlerCalls.Add(1)
	})

	done := make(chan error, 1)
	go func() { done <- loop.Run(t.Context()) }()
	waitAbortContractSignal(t, timerClaimed, "timer claim")
	manualReturned := make(chan struct{})
	go func() {
		controller.Abort("manual")
		close(manualReturned)
	}()
	waitAbortContractSignal(t, manualWaiting, "losing manual publication wait")
	select {
	case <-manualReturned:
		t.Fatal("manual Abort returned from a confirmed publication wait before publication")
	default:
	}
	if got := controller.timeoutState.winner.Load(); got != abortTimeoutTimer {
		t.Fatalf("timeout winner = %d, want timer claim %d", got, abortTimeoutTimer)
	}
	if controller.Signal().Aborted() {
		t.Fatal("timeout signal published before the timer claim was released")
	}
	releaseTimerNow()
	waitAbortContractSignal(t, manualReturned, "manual Abort after timeout publication")
	if err := waitAbortContractValue(t, done, "Run after timeout publication"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, ok := controller.Signal().Reason().(*TimeoutError); !ok {
		t.Fatalf("reason = %T, want *TimeoutError", controller.Signal().Reason())
	}
	if got := handlerCalls.Load(); got != 1 {
		t.Fatalf("handler calls = %d, want exactly 1", got)
	}
}

func TestAbortTimeoutTimerPublicationDoesNotWaitForHandler(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	controller, err := AbortTimeout(loop, 0)
	if err != nil {
		t.Fatal(err)
	}
	timerClaimed := make(chan struct{})
	releaseTimer := make(chan struct{})
	releaseTimerNow := abortContractRelease(t, releaseTimer)
	manualWaiting := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterAbortTimeoutClaim: func() {
			close(timerClaimed)
			<-releaseTimer
		},
		BeforeAbortTimeoutPublicationWait: func() { close(manualWaiting) },
	}
	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	releaseHandlerNow := abortContractRelease(t, releaseHandler)
	controller.Signal().OnAbort(func(any) {
		close(handlerStarted)
		<-releaseHandler
	})
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(t.Context()) }()
	waitAbortContractSignal(t, timerClaimed, "timer claim before blocked handler")
	manualReturned := make(chan struct{})
	go func() {
		controller.Abort("loser")
		close(manualReturned)
	}()
	waitAbortContractSignal(t, manualWaiting, "manual publication wait before blocked handler")
	releaseTimerNow()
	waitAbortContractSignal(t, handlerStarted, "blocked timeout handler start")
	waitAbortContractSignal(t, manualReturned, "losing manual return during blocked timeout handler")
	select {
	case err := <-runDone:
		t.Fatalf("Run returned while timeout handler remained blocked: %v", err)
	default:
	}
	if _, ok := controller.Signal().Reason().(*TimeoutError); !ok {
		t.Fatalf("reason = %T, want *TimeoutError", controller.Signal().Reason())
	}
	releaseHandlerNow()
	if err := waitAbortContractValue(t, runDone, "Run after blocked timeout handler release"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAbortTimeoutManualClaimPublishesBeforeLosingManualReturns(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	controller, err := AbortTimeout(loop, int(time.Hour/time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	firstClaimed := make(chan struct{})
	releaseFirst := make(chan struct{})
	releaseFirstNow := abortContractRelease(t, releaseFirst)
	secondWaiting := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterAbortTimeoutManualClaim: func() {
			close(firstClaimed)
			<-releaseFirst
		},
		BeforeAbortTimeoutPublicationWait: func() { close(secondWaiting) },
	}
	winnerReason := errors.New("first manual winner")
	loserReason := errors.New("second manual loser")
	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	releaseHandlerNow := abortContractRelease(t, releaseHandler)
	var handlerCalls atomic.Int32
	controller.Signal().OnAbort(func(any) {
		handlerCalls.Add(1)
		close(handlerStarted)
		<-releaseHandler
	})
	firstDone := make(chan struct{})
	go func() {
		controller.Abort(winnerReason)
		close(firstDone)
	}()
	waitAbortContractSignal(t, firstClaimed, "first manual claim")
	secondDone := make(chan struct{})
	go func() {
		controller.Abort(loserReason)
		close(secondDone)
	}()
	waitAbortContractSignal(t, secondWaiting, "second manual publication wait")
	select {
	case <-secondDone:
		t.Fatal("losing manual Abort returned before the winner published")
	default:
	}
	releaseFirstNow()
	waitAbortContractSignal(t, handlerStarted, "winning manual handler")
	waitAbortContractSignal(t, secondDone, "losing manual return after publication")
	select {
	case <-firstDone:
		t.Fatal("winning manual Abort returned before its handler completed")
	default:
	}
	if got := controller.Signal().Reason(); got != winnerReason {
		t.Fatalf("reason = %#v, want exact first manual reason %#v", got, winnerReason)
	}
	if got := handlerCalls.Load(); got != 1 {
		t.Fatalf("handler calls = %d, want exactly 1", got)
	}
	releaseHandlerNow()
	waitAbortContractSignal(t, firstDone, "winning manual return")
}

func TestAbortTimeoutManualPublicationReleasesEveryLoser(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	controller, err := AbortTimeout(loop, int(time.Hour/time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	firstClaimed := make(chan struct{})
	releaseFirst := make(chan struct{})
	releaseFirstNow := abortContractRelease(t, releaseFirst)
	const loserCount = 8
	allWaiting := make(chan struct{})
	var waiting atomic.Int32
	loop.testHooks = &loopTestHooks{
		AfterAbortTimeoutManualClaim: func() {
			close(firstClaimed)
			<-releaseFirst
		},
		BeforeAbortTimeoutPublicationWait: func() {
			if waiting.Add(1) == loserCount {
				close(allWaiting)
			}
		},
	}
	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	releaseHandlerNow := abortContractRelease(t, releaseHandler)
	controller.Signal().OnAbort(func(any) {
		close(handlerStarted)
		<-releaseHandler
	})
	winnerReason := errors.New("manual winner")
	winnerDone := make(chan struct{})
	go func() {
		controller.Abort(winnerReason)
		close(winnerDone)
	}()
	waitAbortContractSignal(t, firstClaimed, "manual winner claim")
	losersReturned := make(chan struct{}, loserCount)
	for i := range loserCount {
		go func(reason int) {
			controller.Abort(reason)
			losersReturned <- struct{}{}
		}(i)
	}
	waitAbortContractSignal(t, allWaiting, "all losing manual publication waits")
	select {
	case <-losersReturned:
		t.Fatal("a losing manual Abort returned before publication")
	default:
	}
	releaseFirstNow()
	waitAbortContractSignal(t, handlerStarted, "manual winner blocked handler")
	for range loserCount {
		waitAbortContractSignal(t, losersReturned, "losing manual return after publication")
	}
	select {
	case <-winnerDone:
		t.Fatal("winning manual Abort returned before its handler")
	default:
	}
	if got := controller.Signal().Reason(); got != winnerReason {
		t.Fatalf("reason = %#v, want exact winner %#v", got, winnerReason)
	}
	releaseHandlerNow()
	waitAbortContractSignal(t, winnerDone, "manual winner return")
}

func TestAbortTimeoutSettlementReleasesLoopReference(t *testing.T) {
	for _, manual := range []bool{false, true} {
		name := "timeout"
		if manual {
			name = "manual"
		}
		t.Run(name, func(t *testing.T) {
			controller, pointer := newSettledTimeoutLoop(t, manual)
			waitContractCollected(t, pointer, controller)
		})
	}
}

func TestAbortTimeoutTerminalDiscardReleasesLoopReference(t *testing.T) {
	for _, graceful := range []bool{false, true} {
		terminalName := "close"
		if graceful {
			terminalName = "shutdown"
		}
		for _, running := range []bool{false, true} {
			runName := "pre-run"
			if running {
				runName = "running"
			}
			t.Run(terminalName+"/"+runName, func(t *testing.T) {
				controller, pointer := newTerminalTimeoutLoop(t, running, graceful)
				if controller.Signal().Aborted() {
					t.Fatalf("terminal discard aborted signal with reason %#v", controller.Signal().Reason())
				}
				if got := controller.Signal().Reason(); got != nil {
					t.Fatalf("terminal discard reason = %#v, want nil", got)
				}
				waitContractCollected(t, pointer, controller)
				manualReason := errors.New("manual settlement after terminal discard")
				var calls atomic.Int32
				controller.Signal().OnAbort(func(reason any) {
					if reason != manualReason {
						t.Errorf("handler reason = %#v, want exact manual reason %#v", reason, manualReason)
					}
					calls.Add(1)
				})
				controller.Abort(manualReason)
				if got := controller.Signal().Reason(); got != manualReason {
					t.Fatalf("post-terminal manual reason = %#v, want %#v", got, manualReason)
				}
				if got := calls.Load(); got != 1 {
					t.Fatalf("post-terminal manual handler calls = %d, want 1", got)
				}
			})
		}
	}
}

func TestAbortTimeoutClaimLinearizesBeforeConcurrentClose(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	controller, err := AbortTimeout(loop, 0)
	if err != nil {
		t.Fatal(err)
	}
	timerClaimed := make(chan struct{})
	releaseTimer := make(chan struct{})
	releaseTimerNow := abortContractRelease(t, releaseTimer)
	closeOwned := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterAbortTimeoutClaim: func() {
			close(timerClaimed)
			<-releaseTimer
		},
		BeforeClosePromiseRejection: func() { close(closeOwned) },
	}
	var handlerCalls atomic.Int32
	controller.Signal().OnAbort(func(any) { handlerCalls.Add(1) })
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(t.Context()) }()
	waitAbortContractSignal(t, timerClaimed, "timer claim")
	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	waitAbortContractSignal(t, closeOwned, "Close terminal ownership")
	releaseTimerNow()
	if err := waitAbortContractValue(t, closeDone, "Close after claimed timeout"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := waitAbortContractValue(t, runDone, "Run after claimed timeout"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, ok := controller.Signal().Reason().(*TimeoutError); !ok {
		t.Fatalf("reason = %T, want winning *TimeoutError", controller.Signal().Reason())
	}
	if got := handlerCalls.Load(); got != 1 {
		t.Fatalf("handler calls = %d, want exactly 1", got)
	}
}

func TestAbortTimeoutClaimLinearizesBeforeConcurrentShutdown(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	controller, err := AbortTimeout(loop, 0)
	if err != nil {
		t.Fatal(err)
	}
	timerClaimed := make(chan struct{})
	releaseTimer := make(chan struct{})
	releaseTimerNow := abortContractRelease(t, releaseTimer)
	shutdownOwned := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterAbortTimeoutClaim: func() {
			close(timerClaimed)
			<-releaseTimer
		},
		AfterShutdownStateTerminating: func() { close(shutdownOwned) },
	}
	var handlerCalls atomic.Int32
	controller.Signal().OnAbort(func(any) { handlerCalls.Add(1) })
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(t.Context()) }()
	waitAbortContractSignal(t, timerClaimed, "timer claim")
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(t.Context()) }()
	waitAbortContractSignal(t, shutdownOwned, "Shutdown terminal ownership")
	releaseTimerNow()
	if err := waitAbortContractValue(t, shutdownDone, "Shutdown after claimed timeout"); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitAbortContractValue(t, runDone, "Run after claimed timeout Shutdown"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, ok := controller.Signal().Reason().(*TimeoutError); !ok {
		t.Fatalf("reason = %T, want winning *TimeoutError", controller.Signal().Reason())
	}
	if got := handlerCalls.Load(); got != 1 {
		t.Fatalf("handler calls = %d, want exactly 1", got)
	}
}

func TestAbortTimeoutManualCancellationReleasesSignalWhileLoopLives(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	keepAliveID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatal(err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(t.Context()) }()
	ownerReady := make(chan struct{})
	if err := loop.Submit(func() { close(ownerReady) }); err != nil {
		t.Fatalf("Submit owner barrier: %v", err)
	}
	waitAbortContractSignal(t, ownerReady, "live loop owner barrier")
	pointer := newManuallyCanceledTimeoutPointer(t, loop)
	probeID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer probe: %v", err)
	}
	if err := loop.CancelTimer(probeID); err != nil {
		t.Fatalf("CancelTimer probe after timeout cancellation: %v", err)
	}
	waitContractCollected(t, pointer, loop)
	if state := LoopState(loop.state.Load()); state == StateTerminating || state == StateTerminated {
		t.Fatalf("loop terminated before live-loop retention proof: %v", state)
	}
	if err := loop.CancelTimer(keepAliveID); err != nil {
		t.Fatalf("CancelTimer keepalive: %v", err)
	}
	if err := waitAbortContractValue(t, runDone, "auto-exit after live-loop retention proof"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestAbortTimeoutHandlerGoexitDoesNotStrandLoop(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	controller, err := AbortTimeout(loop, 0)
	if err != nil {
		t.Fatal(err)
	}
	controller.Signal().OnAbort(func(any) {
		runtime.Goexit()
	})
	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refed timer count = %d, want 0", got)
	}
	if got := len(loop.timerMap); got != 0 {
		t.Fatalf("timer map entries = %d, want 0", got)
	}
	controller.Signal().mu.RLock()
	dispatching := controller.Signal().dispatching
	controller.Signal().mu.RUnlock()
	if dispatching {
		t.Fatal("timeout signal remained in dispatch after handler Goexit")
	}
}

func TestAbortTimeoutHandlerExitPublishesToWaitingLoser(t *testing.T) {
	tests := []struct {
		name    string
		handler func(any)
	}{
		{name: "panic", handler: func(any) { panic(errors.New("timeout handler panic")) }},
		{name: "Goexit", handler: func(any) { runtime.Goexit() }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New(WithAutoExit(true))
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)
			controller, err := AbortTimeout(loop, 0)
			if err != nil {
				t.Fatal(err)
			}
			timerClaimed := make(chan struct{})
			releaseTimer := make(chan struct{})
			releaseTimerNow := abortContractRelease(t, releaseTimer)
			manualWaiting := make(chan struct{})
			loop.testHooks = &loopTestHooks{
				AfterAbortTimeoutClaim: func() {
					close(timerClaimed)
					<-releaseTimer
				},
				BeforeAbortTimeoutPublicationWait: func() { close(manualWaiting) },
			}
			controller.Signal().OnAbort(test.handler)
			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(t.Context()) }()
			waitAbortContractSignal(t, timerClaimed, "timer claim before handler exit")
			manualReturned := make(chan struct{})
			go func() {
				controller.Abort("loser")
				close(manualReturned)
			}()
			waitAbortContractSignal(t, manualWaiting, "manual publication wait before handler exit")
			releaseTimerNow()
			waitAbortContractSignal(t, manualReturned, "losing manual return after handler exit publication")
			if err := waitAbortContractValue(t, runDone, "Run after timeout handler exit"); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if _, ok := controller.Signal().Reason().(*TimeoutError); !ok {
				t.Fatalf("reason = %T, want *TimeoutError", controller.Signal().Reason())
			}
		})
	}
}

func TestAbortTimeoutHandlerRetainsLoopOwnerSemantics(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	controller, err := AbortTimeout(loop, 0)
	if err != nil {
		t.Fatal(err)
	}
	callbackResult := make(chan error, 1)
	controller.Signal().OnAbort(func(any) {
		if err := loop.Close(); !errors.Is(err, ErrReentrantClose) {
			callbackResult <- errors.New("callback-local Close did not return ErrReentrantClose")
			return
		}
		callbackResult <- loop.Shutdown(t.Context())
	})

	done := make(chan error, 1)
	go func() { done <- loop.Run(t.Context()) }()
	select {
	case err := <-callbackResult:
		if err != nil {
			t.Fatalf("callback lifecycle result: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout handler deadlocked in loop lifecycle call")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not finish after callback-local Shutdown")
	}
}

func TestAbortTimeoutHandlerSchedulesOwnerLocalWorkAndRelaysPanic(t *testing.T) {
	panicValues := make(chan any, 1)
	typedLogger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](&testEventFactory{}),
		logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(event *testEvent) error {
			if value, ok := event.fields["panic"]; ok {
				panicValues <- value
			}
			return nil
		})),
	)
	loop, err := New(WithAutoExit(true), WithLogger(typedLogger.Logger()))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	controller, err := AbortTimeout(loop, 0)
	if err != nil {
		t.Fatal(err)
	}
	marker := errors.New("timeout handler panic")
	order := make(chan string, 3)
	controller.Signal().OnAbort(func(any) {
		if !loop.ownsLocalQueues() {
			t.Error("timeout handler did not own loop-local queues")
		}
		if err := loop.ScheduleMicrotask(func() { order <- "microtask" }); err != nil {
			t.Errorf("ScheduleMicrotask: %v", err)
		}
		if err := loop.ScheduleNextTick(func() { order <- "nextTick" }); err != nil {
			t.Errorf("ScheduleNextTick: %v", err)
		}
		if got := loop.commands.Len(); got != 0 {
			t.Errorf("owner-local scheduling used %d command-ingress entries", got)
		}
		if _, err := loop.ScheduleTimer(0, func() {
			if !loop.ownsLocalQueues() {
				t.Error("physical owner was not restored after delegated panic")
			}
			order <- "timer"
		}); err != nil {
			t.Errorf("ScheduleTimer: %v", err)
		}
		if got := loop.commands.Len(); got != 0 {
			t.Errorf("owner-local timer scheduling used %d command-ingress entries", got)
		}
		panic(marker)
	})
	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	close(order)
	var got []string
	for value := range order {
		got = append(got, value)
	}
	if want := []string{"nextTick", "microtask", "timer"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("scheduled order after relayed panic = %v, want %v", got, want)
	}
	if got := waitAbortContractValue(t, panicValues, "relayed panic log event"); got != marker {
		t.Fatalf("safeExecute panic = %#v, want exact marker %#v", got, marker)
	}
}

func TestAbortTimeoutRejectsStaticContractViolations(t *testing.T) {
	for _, test := range []struct {
		name string
		loop *Loop
	}{
		{name: "nil loop"},
		{name: "zero loop", loop: &Loop{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			const want = "eventloop: AbortTimeout requires a Loop created by New"
			if got := abortEventCapturePanic(func() { _, _ = AbortTimeout(test.loop, 1) }); got != want {
				t.Fatalf("panic = %#v, want %q", got, want)
			}
		})
	}

	t.Run("negative delay", func(t *testing.T) {
		loop, err := New()
		if err != nil {
			t.Fatal(err)
		}
		registerLoopCleanupT(t, loop)
		if got := abortEventCapturePanic(func() { _, _ = AbortTimeout(loop, -1) }); got == nil {
			t.Fatal("AbortTimeout(loop, -1) did not panic")
		}
	})

	if strconv.IntSize == 64 {
		t.Run("duration overflow", func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)
			maxDelayMillis := int64((1<<63 - 1) / int64(time.Millisecond))
			if got := abortEventCapturePanic(func() {
				_, _ = AbortTimeout(loop, int(maxDelayMillis+1))
			}); got == nil {
				t.Fatal("overflowing AbortTimeout delay did not panic")
			}
		})
	}
}

func TestAbortTimeoutTerminatedLoopReturnsLifecycleError(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	controller, err := AbortTimeout(loop, 0)
	if controller != nil {
		t.Fatalf("controller = %#v, want nil", controller)
	}
	if !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("error = %v, want %v", err, ErrLoopTerminated)
	}
}
