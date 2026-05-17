package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCancelRunningTimerReleasesLiveness(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	callbackRan := make(chan struct{}, 1)
	id, err := loop.ScheduleTimer(time.Hour, func() { callbackRan <- struct{}{} })
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitLoopOwnerTurnT(t, loop)
	if got := loop.refedTimerCount.Load(); got != 1 {
		t.Fatalf("refedTimerCount before cancellation = %d, want 1", got)
	}
	if err := loop.CancelTimer(id); err != nil {
		t.Fatalf("CancelTimer: %v", err)
	}
	if err := waitContractValue(t, runDone, "single-cancel auto-exit completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	select {
	case <-callbackRan:
		t.Fatal("canceled timer callback ran")
	default:
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refedTimerCount after cancellation = %d, want 0", got)
	}
	if _, ok := loop.timerMap[id]; ok {
		t.Fatalf("canceled timer %d remained in timer map", id)
	}
	if loop.Alive() {
		t.Fatal("canceled timer retained loop liveness")
	}
}

func TestCancelTimerBeforeRunCancelsQueuedTimer(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	callbackRan := make(chan struct{}, 1)
	id, err := loop.ScheduleTimer(0, func() { callbackRan <- struct{}{} })
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	if err := loop.CancelTimer(id); err != nil {
		t.Fatalf("CancelTimer before Run: %v", err)
	}
	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	select {
	case <-callbackRan:
		t.Fatal("timer callback ran after pre-Run cancellation")
	default:
	}
}

func TestTimerRequestCancellationPrecedesLaterExecutionClaim(t *testing.T) {
	tests := []struct {
		name   string
		cancel func(LoopRequests, TimerID) error
	}{
		{name: "single", cancel: LoopRequests.CancelTimer},
		{
			name: "batch",
			cancel: func(requests LoopRequests, id TimerID) error {
				return requests.CancelTimers(id)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			loop, err := New(WithAutoExit(true), WithFastPathMode(FastPathDisabled))
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)
			claimReached := make(chan struct{})
			releaseClaim := make(chan struct{})
			releaseClaimFn := releaseSignalT(t, releaseClaim)
			var claimOnce sync.Once
			var fired atomic.Bool
			var timerID TimerID
			loop.testHooks = &loopTestHooks{
				BeforeTimerExecutionClaim: func(id TimerID) {
					if id == timerID {
						claimOnce.Do(func() {
							close(claimReached)
							<-releaseClaim
						})
					}
				},
			}

			timerID, err = loop.ScheduleTimer(0, func() { fired.Store(true) })
			if err != nil {
				t.Fatalf("ScheduleTimer: %v", err)
			}
			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(ctx) }()

			waitContractSignal(t, claimReached, "due timer execution-claim boundary")
			if err := test.cancel(loop.Requests(), timerID); err != nil {
				t.Fatalf("cancellation request: %v", err)
			}
			releaseClaimFn()

			if err := waitContractValue(t, runDone, "Run after pre-claim cancellation"); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if fired.Load() {
				t.Fatal("timer callback ran after cancellation acknowledgement preceded its execution claim")
			}
		})
	}
}

func TestCancelTimerResultPrecedesLaterExecutionClaim(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := New(WithAutoExit(true), WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	claimReached := make(chan struct{})
	releaseClaim := make(chan struct{})
	releaseClaimFn := releaseSignalT(t, releaseClaim)
	ingressDrained := make(chan struct{})
	releaseDrain := make(chan struct{})
	releaseDrainFn := releaseSignalT(t, releaseDrain)
	commandPublished := make(chan struct{})
	var fired atomic.Bool
	var timerID TimerID
	loop.testHooks = &loopTestHooks{
		BeforeTimerExecutionClaim: func(id TimerID) {
			if id == timerID {
				close(claimReached)
				<-releaseClaim
			}
		},
		AfterTimerExecutionIngressDrain: func(id TimerID) {
			if id == timerID {
				close(ingressDrained)
				<-releaseDrain
			}
		},
		AfterSynchronousTimerCommandPublish: func(kind loopCommandKind) {
			if kind == loopCommandTimerCancel {
				close(commandPublished)
			}
		},
	}

	timerID, err = loop.ScheduleTimer(0, func() { fired.Store(true) })
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	waitContractSignal(t, claimReached, "due timer execution-claim boundary")
	cancelDone := make(chan error, 1)
	go func() { cancelDone <- loop.CancelTimer(timerID) }()
	waitContractSignal(t, commandPublished, "result-bearing cancellation publication")
	releaseClaimFn()
	waitContractSignal(t, ingressDrained, "result-bearing cancellation drain")
	if err := waitContractValue(t, cancelDone, "CancelTimer result before callback claim"); err != nil {
		t.Fatalf("CancelTimer: %v", err)
	}
	releaseDrainFn()

	if err := waitContractValue(t, runDone, "Run after result-bearing cancellation"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if fired.Load() {
		t.Fatal("timer callback ran after exact cancellation result preceded its execution claim")
	}
}

func TestTimerRequestCancellationAfterExecutionClaimDoesNotSuppressCallback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop, err := New(WithAutoExit(true), WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	claimCommitted := make(chan struct{})
	releaseCallback := make(chan struct{})
	releaseCallbackFn := releaseSignalT(t, releaseCallback)
	var claimOnce sync.Once
	var fired atomic.Bool
	var timerID TimerID
	loop.testHooks = &loopTestHooks{
		BeforeTimerPublicationWait: func(id TimerID) {
			if id == timerID {
				claimOnce.Do(func() {
					close(claimCommitted)
					<-releaseCallback
				})
			}
		},
	}

	timerID, err = loop.ScheduleTimer(0, func() { fired.Store(true) })
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	waitContractSignal(t, claimCommitted, "committed timer execution claim")
	if err := loop.Requests().CancelTimer(timerID); err != nil {
		t.Fatalf("CancelTimer request after claim: %v", err)
	}
	releaseCallbackFn()

	if err := waitContractValue(t, runDone, "Run after post-claim cancellation"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !fired.Load() {
		t.Fatal("post-claim cancellation suppressed the already-claimed timer callback")
	}
}

func TestJSClearTimeoutBeforeRunCancelsQueuedTimer(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	callbackRan := make(chan struct{}, 1)
	id, err := js.SetTimeout(func() { callbackRan <- struct{}{} }, 0)
	if err != nil {
		t.Fatalf("SetTimeout: %v", err)
	}
	if err := js.ClearTimeout(id); err != nil {
		t.Fatalf("ClearTimeout before Run: %v", err)
	}
	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	select {
	case <-callbackRan:
		t.Fatal("JS timeout callback ran after pre-Run cancellation")
	default:
	}
}

func TestScheduleTimerCancelAfterExpiration(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	fired := make(chan struct{})
	id, err := loop.ScheduleTimer(0, func() { close(fired) })
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, fired, "expired timer callback")
	waitLoopOwnerTurnT(t, loop)
	if err := loop.RefTimer(id); err != nil {
		t.Fatalf("RefTimer after callback: %v", err)
	}
	if err := loop.UnrefTimer(id); err != nil {
		t.Fatalf("UnrefTimer after callback: %v", err)
	}
	type expiredTimerState struct {
		present  bool
		refCount int64
	}
	observed := make(chan expiredTimerState, 1)
	if err := loop.SubmitInternal(func() {
		_, present := loop.timerMap[id]
		observed <- expiredTimerState{present: present, refCount: loop.refedTimerCount.Load()}
	}); err != nil {
		t.Fatalf("expired timer observation: %v", err)
	}
	state := waitContractValue(t, observed, "expired timer state")
	if state.present || state.refCount != 0 {
		t.Fatalf("expired timer state = (present=%v, refs=%d), want (false, 0)", state.present, state.refCount)
	}
	if err := loop.CancelTimer(id); err != ErrTimerNotFound {
		t.Fatalf("CancelTimer after callback = %v, want ErrTimerNotFound", err)
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "expired-timer Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestCancelTimerMissingBeforeRun(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	if err := loop.CancelTimer(1); err != ErrTimerNotFound {
		t.Fatalf("CancelTimer before Run = %v, want %v", err, ErrTimerNotFound)
	}
}

func TestAwaitCancelTimerResultReleasesAfterClose(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	result := make(chan error)
	waitDone := make(chan error, 1)
	go func() { waitDone <- loop.awaitCancelTimerResult(result) }()
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := waitContractValue(t, waitDone, "pending timer result release"); err != ErrLoopTerminated {
		t.Fatalf("awaitCancelTimerResult after Close = %v, want ErrLoopTerminated", err)
	}
}

func TestTerminalTransitionReleasesTimerCommandNeededByRunningCallback(t *testing.T) {
	commandCases := []struct {
		name            string
		kind            loopCommandKind
		count           int
		wantTerminalErr error
		call            func(*Loop, []TimerID) []error
	}{
		{
			name:  "ref",
			kind:  loopCommandTimerRef,
			count: 1,
			call: func(loop *Loop, ids []TimerID) []error {
				return []error{loop.RefTimer(ids[0])}
			},
		},
		{
			name:  "unref",
			kind:  loopCommandTimerUnref,
			count: 1,
			call: func(loop *Loop, ids []TimerID) []error {
				return []error{loop.UnrefTimer(ids[0])}
			},
		},
		{
			name:            "cancel",
			kind:            loopCommandTimerCancel,
			count:           1,
			wantTerminalErr: ErrLoopTerminated,
			call: func(loop *Loop, ids []TimerID) []error {
				return []error{loop.CancelTimer(ids[0])}
			},
		},
		{
			name:            "cancel batch",
			kind:            loopCommandTimerCancelBatch,
			count:           2,
			wantTerminalErr: ErrLoopTerminated,
			call: func(loop *Loop, ids []TimerID) []error {
				return loop.CancelTimers(ids...)
			},
		},
	}
	terminationCases := []struct {
		name      string
		configure func(*loopTestHooks, chan struct{})
		terminate func(*Loop) error
	}{
		{
			name: "close",
			configure: func(hooks *loopTestHooks, committed chan struct{}) {
				hooks.BeforeClosePromiseRejection = func() { close(committed) }
			},
			terminate: func(loop *Loop) error { return loop.Close() },
		},
		{
			name: "shutdown",
			configure: func(hooks *loopTestHooks, committed chan struct{}) {
				hooks.AfterShutdownStateTerminating = func() { close(committed) }
			},
			terminate: func(loop *Loop) error { return loop.Shutdown(context.Background()) },
		},
	}

	for _, terminationCase := range terminationCases {
		for _, commandCase := range commandCases {
			t.Run(terminationCase.name+"/"+commandCase.name, func(t *testing.T) {
				loop, err := New()
				if err != nil {
					t.Fatal(err)
				}
				ids := make([]TimerID, commandCase.count)
				for index := range ids {
					ids[index], err = loop.ScheduleTimer(time.Hour, func() {})
					if err != nil {
						t.Fatalf("ScheduleTimer %d: %v", index, err)
					}
				}

				commandPublished := make(chan struct{})
				terminalCommitted := make(chan struct{})
				hooks := &loopTestHooks{
					AfterSynchronousTimerCommandPublish: func(kind loopCommandKind) {
						if kind == commandCase.kind {
							close(commandPublished)
						}
					},
				}
				terminationCase.configure(hooks, terminalCommitted)
				loop.testHooks = hooks

				callbackResult := make(chan []error, 1)
				releaseCallback := make(chan struct{})
				commandDone := make(chan []error, 1)
				if err := loop.Submit(func() {
					go func() { commandDone <- commandCase.call(loop, ids) }()
					select {
					case result := <-commandDone:
						callbackResult <- result
					case <-releaseCallback:
						callbackResult <- nil
					}
				}); err != nil {
					t.Fatal(err)
				}

				runDone := make(chan error, 1)
				go func() { runDone <- loop.Run(context.Background()) }()
				waitContractSignal(t, commandPublished, "result-bearing timer command publication")
				terminalDone := make(chan error, 1)
				go func() { terminalDone <- terminationCase.terminate(loop) }()
				waitContractSignal(t, terminalCommitted, terminationCase.name+" terminal-state publication")

				select {
				case results := <-callbackResult:
					if len(results) != commandCase.count {
						t.Fatalf("timer command returned %d results, want %d", len(results), commandCase.count)
					}
					for index, result := range results {
						want := commandCase.wantTerminalErr
						if !errors.Is(result, want) || (result == nil) != (want == nil) {
							t.Fatalf("timer command result %d = %v, want %v", index, result, want)
						}
					}
				case <-time.After(2 * time.Second):
					close(releaseCallback)
					<-callbackResult
					<-runDone
					<-terminalDone
					<-commandDone
					t.Fatalf("%s waited for the running callback before releasing %s", terminationCase.name, commandCase.name)
				}
				if err := waitContractValue(t, runDone, "Run after timer-command dependency release"); err != nil {
					t.Fatalf("Run: %v", err)
				}
				if err := waitContractValue(t, terminalDone, terminationCase.name+" after timer-command dependency release"); err != nil {
					t.Fatalf("%s: %v", terminationCase.name, err)
				}
			})
		}
	}
}

func TestTerminalRefDependencyCannotOverrideLaterOwnerUnref(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	id, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.UnrefTimer(id); err != nil {
		t.Fatal(err)
	}

	refPublished := make(chan struct{})
	var publishedOnce sync.Once
	terminalRefCount := make(chan int64, 1)
	var drainOnce sync.Once
	loop.testHooks = &loopTestHooks{
		AfterSynchronousTimerCommandPublish: func(kind loopCommandKind) {
			if kind == loopCommandTimerRef {
				publishedOnce.Do(func() { close(refPublished) })
			}
		},
		BeforeTerminalDrainFinish: func() {
			drainOnce.Do(func() { terminalRefCount <- loop.refedTimerCount.Load() })
		},
	}

	type callbackOutcome struct {
		refErr   error
		unrefErr error
	}
	callbackDone := make(chan callbackOutcome, 1)
	if err := loop.Submit(func() {
		refDone := make(chan error, 1)
		go func() { refDone <- loop.RefTimer(id) }()
		refErr := <-refDone
		callbackDone <- callbackOutcome{refErr: refErr, unrefErr: loop.UnrefTimer(id)}
	}); err != nil {
		t.Fatal(err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, refPublished, "ordinary RefTimer publication")
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(context.Background()) }()

	outcome := waitContractValue(t, callbackDone, "terminal RefTimer dependency release")
	if outcome.refErr != nil {
		t.Errorf("RefTimer result = %v, want nil", outcome.refErr)
	}
	if outcome.unrefErr != nil {
		t.Errorf("later owner UnrefTimer = %v, want nil", outcome.unrefErr)
	}
	if err := waitContractValue(t, runDone, "Run after terminal RefTimer release"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := waitContractValue(t, shutdownDone, "Shutdown after terminal RefTimer release"); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if got := waitContractValue(t, terminalRefCount, "terminal timer ref count"); got != 0 {
		t.Fatalf("terminal refed timer count = %d, want 0 after later owner UnrefTimer", got)
	}
}

func TestTerminalTransitionPreservesTimerResultAppliedBeforeDependencyScan(t *testing.T) {
	for _, test := range []struct {
		name     string
		kind     loopCommandKind
		count    int
		call     func(*Loop, []TimerID) []error
		expected []error
	}{
		{
			name:  "ref",
			kind:  loopCommandTimerRef,
			count: 1,
			call: func(loop *Loop, ids []TimerID) []error {
				return []error{loop.RefTimer(ids[0])}
			},
			expected: []error{nil},
		},
		{
			name:  "unref",
			kind:  loopCommandTimerUnref,
			count: 1,
			call: func(loop *Loop, ids []TimerID) []error {
				return []error{loop.UnrefTimer(ids[0])}
			},
			expected: []error{nil},
		},
		{
			name:  "cancel",
			kind:  loopCommandTimerCancel,
			count: 1,
			call: func(loop *Loop, ids []TimerID) []error {
				return []error{loop.CancelTimer(ids[0])}
			},
			expected: []error{nil},
		},
		{
			name:  "cancel batch",
			kind:  loopCommandTimerCancelBatch,
			count: 2,
			call: func(loop *Loop, ids []TimerID) []error {
				return loop.CancelTimers(ids[0], ids[0], ids[1])
			},
			expected: []error{nil, ErrTimerNotFound, nil},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)
			commandPopped := make(chan struct{})
			releaseApply := make(chan struct{})
			release := contractRelease(t, releaseApply)
			terminalCommitted := make(chan struct{})
			var poppedOnce sync.Once
			loop.testHooks = &loopTestHooks{
				AfterCommandIngressPopBeforeApply: func(kind loopCommandKind) {
					if kind == test.kind {
						poppedOnce.Do(func() { close(commandPopped) })
						<-releaseApply
					}
				},
				AfterShutdownStateTerminating: func() { close(terminalCommitted) },
			}
			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(context.Background()) }()
			waitLoopOwnerTurnT(t, loop)

			ids := make([]TimerID, test.count)
			for index := range ids {
				ids[index], err = loop.ScheduleTimer(time.Hour, func() {})
				if err != nil {
					t.Fatalf("ScheduleTimer %d: %v", index, err)
				}
			}
			waitLoopOwnerTurnT(t, loop)

			operationDone := make(chan []error, 1)
			go func() { operationDone <- test.call(loop, ids) }()
			waitContractSignal(t, commandPopped, "timer command owner claim")
			shutdownDone := make(chan error, 1)
			go func() { shutdownDone <- loop.Shutdown(context.Background()) }()
			waitContractSignal(t, terminalCommitted, "graceful terminal commitment")
			release()

			results := waitContractValue(t, operationDone, "owner-applied timer result")
			if len(results) != len(test.expected) {
				t.Fatalf("result count = %d, want %d", len(results), len(test.expected))
			}
			for index, expected := range test.expected {
				if !errors.Is(results[index], expected) || (results[index] == nil) != (expected == nil) {
					t.Fatalf("result %d = %v, want %v", index, results[index], expected)
				}
			}
			if err := waitContractValue(t, runDone, "Run after owner-applied timer result"); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if err := waitContractValue(t, shutdownDone, "Shutdown after owner-applied timer result"); err != nil {
				t.Fatalf("Shutdown: %v", err)
			}
		})
	}
}
