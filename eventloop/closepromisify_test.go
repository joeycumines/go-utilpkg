package eventloop

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestClosePreRunReturnsBeforePromisifyDependency(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	callbackRan := make(chan struct{})
	releaseWorker := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseWorker) }) }
	t.Cleanup(release)

	workerStarted := make(chan struct{})
	submitted := make(chan error, 1)
	workerReturned := make(chan struct{})
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		close(workerStarted)
		err := loop.Submit(func() { close(callbackRan) })
		submitted <- err
		if err != nil {
			return nil, err
		}
		select {
		case <-callbackRan:
		case <-releaseWorker:
		}
		close(workerReturned)
		return "released", nil
	})

	select {
	case <-workerStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify worker did not start")
	}
	select {
	case err := <-submitted:
		if err != nil {
			t.Fatalf("worker Submit: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify worker did not submit its callback dependency")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close waited for a pre-Run Promisify dependency it discarded")
	}

	select {
	case <-callbackRan:
		t.Fatal("pre-Run callback dependency ran during immediate Close")
	default:
	}
	select {
	case <-workerReturned:
		t.Fatal("Promisify worker returned before its external release")
	default:
	}
	assertPromiseRejected(t, promise, ErrLoopTerminated)
	assertCloseSignals(t, loop)

	release()
	select {
	case <-workerReturned:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify worker did not return after release")
	}
	waitPromisifyWorkersT(t, loop)
	assertPromiseRejected(t, promise, ErrLoopTerminated)
}

func TestCloseRunningReturnsBeforePromisifyInternalDependency(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	outerStarted := make(chan struct{})
	releaseOuter := make(chan struct{})
	releaseOuterFn := releaseSignalT(t, releaseOuter)
	if err := loop.Submit(func() {
		close(outerStarted)
		<-releaseOuter
	}); err != nil {
		t.Fatalf("Submit blocking callback: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case <-outerStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("blocking loop callback did not start")
	}

	dependencyRan := make(chan struct{})
	releaseWorker := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseWorker) }) }
	t.Cleanup(release)
	submitted := make(chan error, 1)
	workerReturned := make(chan struct{})
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		err := loop.SubmitInternal(func() { close(dependencyRan) })
		submitted <- err
		if err != nil {
			return nil, err
		}
		select {
		case <-dependencyRan:
		case <-releaseWorker:
		}
		close(workerReturned)
		return "released", nil
	})
	select {
	case err := <-submitted:
		if err != nil {
			releaseOuterFn()
			t.Fatalf("worker SubmitInternal: %v", err)
		}
	case <-time.After(5 * time.Second):
		releaseOuterFn()
		t.Fatal("Promisify worker did not submit its internal dependency")
	}

	closeTransitioned := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterCloseStateTerminating: func() { close(closeTransitioned) },
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	waitContractSignal(t, closeTransitioned, "Close StateTerminating publication")
	releaseOuterFn()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close waited for a running Promisify dependency it discarded")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not exit during Close")
	}

	select {
	case <-dependencyRan:
		t.Fatal("queued SubmitInternal dependency ran after immediate Close")
	default:
	}
	select {
	case <-workerReturned:
		t.Fatal("Promisify worker returned before its external release")
	default:
	}
	assertPromiseRejected(t, promise, ErrLoopTerminated)
	assertCloseSignals(t, loop)

	release()
	select {
	case <-workerReturned:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify worker did not return after release")
	}
	waitPromisifyWorkersT(t, loop)
	assertPromiseRejected(t, promise, ErrLoopTerminated)
}

func TestCloseRejectsPromisifyAttemptAtWinningTransition(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	releaseCallbackFn := releaseSignalT(t, releaseCallback)
	if err := loop.Submit(func() {
		close(callbackEntered)
		<-releaseCallback
	}); err != nil {
		t.Fatalf("Submit blocking callback: %v", err)
	}
	closeTransitioned := make(chan struct{})
	releaseClose := make(chan struct{})
	releaseCloseFn := releaseSignalT(t, releaseClose)
	loop.testHooks = &loopTestHooks{
		AfterCloseStateTerminating: func() {
			close(closeTransitioned)
			<-releaseClose
		},
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, callbackEntered, "blocking loop callback")

	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	waitContractSignal(t, closeTransitioned, "Close winning transition")
	userFunctionRan := make(chan struct{}, 1)
	promisifyStarted := make(chan struct{})
	promisifyDone := make(chan Promise, 1)
	go func() {
		close(promisifyStarted)
		promisifyDone <- loop.Promisify(context.Background(), func(context.Context) (any, error) {
			userFunctionRan <- struct{}{}
			return nil, nil
		})
	}()
	waitContractSignal(t, promisifyStarted, "racing Promisify attempt")
	releaseCloseFn()
	rejected := waitContractValue(t, promisifyDone, "racing Promisify rejection")
	assertPromiseRejected(t, rejected, ErrLoopTerminated)
	select {
	case <-userFunctionRan:
		t.Fatal("Promisify entered its user function after Close won terminal admission")
	default:
	}

	releaseCallbackFn()
	if err := waitContractValue(t, closeDone, "Close completion"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := waitContractValue(t, runDone, "Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertCloseSignals(t, loop)
}

func TestCloseRejectsPromiseNeededByAdmittedCallbackBeforeLoopExit(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	releaseWorker := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseWorker) }) }
	t.Cleanup(release)

	workerStarted := make(chan struct{})
	workerReturned := make(chan struct{})
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		close(workerStarted)
		<-releaseWorker
		close(workerReturned)
		return "released", nil
	})
	select {
	case <-workerStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify worker did not start")
	}

	callbackStarted := make(chan struct{})
	callbackResult := make(chan any, 1)
	if err := loop.Submit(func() {
		close(callbackStarted)
		callbackResult <- <-promise.ToChannel()
	}); err != nil {
		t.Fatalf("Submit callback waiting on promise: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case <-callbackStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("callback waiting on promise did not start")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close waited for loopDone before rejecting the promise needed by the admitted callback")
	}
	select {
	case result := <-callbackResult:
		if !errorResultIs(result, ErrLoopTerminated) {
			t.Fatalf("callback promise result = %v, want ErrLoopTerminated", result)
		}
	default:
		t.Fatal("admitted callback did not observe terminal promise rejection before Close returned")
	}
	if err := waitContractValue(t, runDone, "Run completion after immediate cleanup"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	select {
	case <-workerReturned:
		t.Fatal("Promisify user function returned before its independent release")
	default:
	}
	assertPromiseRejected(t, promise, ErrLoopTerminated)
	assertCloseSignals(t, loop)

	release()
	select {
	case <-workerReturned:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify user function did not return after release")
	}
	waitPromisifyWorkersT(t, loop)
	assertPromiseRejected(t, promise, ErrLoopTerminated)
}

func TestCloseRejectsWorkerLoopAccessAfterReturn(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	type accessResult struct {
		submitErr    error
		internalErr  error
		timerErr     error
		jsTimerErr   error
		shutdownErr  error
		closeErr     error
		nestedState  PromiseState
		nestedResult any
		registryData int
		registryRing int
	}
	releaseWorker := make(chan struct{})
	releaseWorkerFn := releaseSignalT(t, releaseWorker)
	workerStarted := make(chan struct{})
	accessDone := make(chan accessResult, 1)
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		close(workerStarted)
		<-releaseWorker
		result := accessResult{
			submitErr:   loop.Submit(func() {}),
			internalErr: loop.SubmitInternal(func() {})}
		_, result.timerErr = loop.ScheduleTimer(0, func() {})
		_, result.jsTimerErr = js.SetTimeout(func() {}, 0)
		result.shutdownErr = loop.Shutdown(context.Background())
		result.closeErr = loop.Close()
		nested := loop.Promisify(context.Background(), func(context.Context) (any, error) {
			return "unexpected", nil
		})
		result.nestedState = nested.State()
		result.nestedResult = nested.Result()
		loop.registry.mu.RLock()
		result.registryData = len(loop.registry.data)
		result.registryRing = len(loop.registry.ring)
		loop.registry.mu.RUnlock()
		accessDone <- result
		return "released", nil
	})
	select {
	case <-workerStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify worker did not start")
	}

	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case result := <-accessDone:
		t.Fatalf("Promisify worker completed before release: %+v", result)
	default:
	}
	assertPromiseRejected(t, promise, ErrLoopTerminated)
	assertCloseSignals(t, loop)

	releaseWorkerFn()
	var result accessResult
	select {
	case result = <-accessDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify worker did not test post-Close access")
	}
	for name, err := range map[string]error{
		"Submit":         result.submitErr,
		"SubmitInternal": result.internalErr,
		"ScheduleTimer":  result.timerErr,
		"JS.SetTimeout":  result.jsTimerErr,
		"Shutdown":       result.shutdownErr,
		"Close":          result.closeErr,
	} {
		if !errors.Is(err, ErrLoopTerminated) {
			t.Errorf("%s after Close = %v, want ErrLoopTerminated", name, err)
		}
	}
	if result.nestedState != Rejected || !errorResultIs(result.nestedResult, ErrLoopTerminated) {
		t.Errorf("nested Promisify after Close = (state %v, result %v), want Rejected ErrLoopTerminated", result.nestedState, result.nestedResult)
	}
	if result.registryData != 0 || result.registryRing != 0 {
		t.Errorf("registry after rejected nested Promisify = (data %d, ring %d), want zero", result.registryData, result.registryRing)
	}
	waitPromisifyWorkersT(t, loop)
	assertPromiseRejected(t, promise, ErrLoopTerminated)
}

func TestCloseTerminalRejectionWinsWorkerFallback(t *testing.T) {
	tests := []struct {
		name    string
		outcome func() (any, error)
	}{
		{
			name:    "success",
			outcome: func() (any, error) { return "worker-result", nil },
		},
		{
			name:    "error",
			outcome: func() (any, error) { return nil, errors.New("worker error") },
		},
		{
			name:    "panic",
			outcome: func() (any, error) { panic("worker panic") },
		},
		{
			name: "goexit",
			outcome: func() (any, error) {
				runtime.Goexit()
				return nil, errors.New("unreachable after runtime.Goexit")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testCloseTerminalWorkerFallbackT(t, test.outcome)
		})
	}
}

func TestCloseSkipsCommittedPromisifyWorkerBeforeUserEntry(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	workerReachedStart := make(chan struct{})
	releaseWorkerStart := make(chan struct{})
	releaseWorkerStartFn := releaseSignalT(t, releaseWorkerStart)
	loop.testHooks = &loopTestHooks{
		BeforePromisifyWorkerStart: func() {
			close(workerReachedStart)
			<-releaseWorkerStart
		},
	}
	userFunctionRan := make(chan struct{}, 1)
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		userFunctionRan <- struct{}{}
		return "unexpected", nil
	})
	select {
	case <-workerReachedStart:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify worker did not reach the user-entry claim")
	}

	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	assertCloseSignals(t, loop)
	assertPromiseRejected(t, promise, ErrLoopTerminated)
	releaseWorkerStartFn()
	waitPromisifyWorkersT(t, loop)
	select {
	case <-userFunctionRan:
		t.Fatal("Promisify user function first entered after immediate Close won")
	default:
	}
	assertPromiseRejected(t, promise, ErrLoopTerminated)
}

func TestCloseWinningTransitionOverridesPromisifyLiveness(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	workerStarted := make(chan struct{})
	releaseWorker := make(chan struct{})
	releaseWorkerFn := releaseSignalT(t, releaseWorker)
	loop.Promisify(context.Background(), func(context.Context) (any, error) {
		close(workerStarted)
		<-releaseWorker
		return nil, nil
	})
	select {
	case <-workerStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify worker did not start")
	}

	type liveness struct {
		alive     bool
		macros    bool
		state     LoopState
		immediate bool
	}
	observed := make(chan liveness, 1)
	releaseClose := make(chan struct{})
	releaseCloseFn := releaseSignalT(t, releaseClose)
	loop.testHooks = &loopTestHooks{
		AfterCloseStateTerminating: func() {
			observed <- liveness{
				alive:     loop.Alive(),
				macros:    loop.HasMacrotaskWork(),
				state:     loop.state.Load(),
				immediate: loop.immediateClose.Load(),
			}
			<-releaseClose
		},
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	select {
	case got := <-observed:
		if got.state != StateTerminating || !got.immediate {
			t.Fatalf("winning Close liveness boundary = %+v", got)
		}
		if got.alive || got.macros {
			t.Fatalf("winning Close liveness = Alive %v, HasMacrotaskWork %v; want both false", got.alive, got.macros)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not reach winning StateTerminating boundary")
	}
	releaseCloseFn()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return")
	}
	releaseWorkerFn()
	waitPromisifyWorkersT(t, loop)
}

func TestClosePreventsCompletedPromisifyWorkerWakeAfterResourceRelease(t *testing.T) {
	loop, err := New(WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	workerStarted := make(chan struct{})
	releaseWorker := make(chan struct{})
	releaseWorkerFn := releaseSignalT(t, releaseWorker)
	workerWakeReached := make(chan struct{})
	releaseWorkerWake := make(chan struct{})
	releaseWorkerWakeFn := releaseSignalT(t, releaseWorkerWake)
	var workerWakeOnce sync.Once
	var wakeCalls atomic.Int32
	loop.testHooks = &loopTestHooks{
		BeforePromisifyWorkerWake: func() {
			workerWakeOnce.Do(func() {
				close(workerWakeReached)
				<-releaseWorkerWake
			})
		},
		OnSubmitWakeup: func() { wakeCalls.Add(1) },
	}
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		close(workerStarted)
		<-releaseWorker
		return "ignored", nil
	})
	select {
	case <-workerStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify worker did not start")
	}
	releaseWorkerFn()
	select {
	case <-workerWakeReached:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify worker did not reach its final auto-exit wake")
	}
	// Ignore wake attempts made while publishing the worker result. The wake under
	// test is the final auto-exit wake after the worker count has been decremented.
	wakeCalls.Store(0)

	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	assertCloseSignals(t, loop)
	assertPromiseRejected(t, promise, ErrLoopTerminated)
	releaseWorkerWakeFn()
	waitPromisifyWorkersT(t, loop)
	if got := wakeCalls.Load(); got != 0 {
		t.Fatalf("post-Close Promisify worker submitted %d wakeups after resource release", got)
	}
}

func TestCloseAllowsClaimedPromisifyWorkerFirstInstructionAfterReturn(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	entryClaimed := make(chan struct{})
	releaseEntry := make(chan struct{})
	releaseEntryFn := releaseSignalT(t, releaseEntry)
	loop.testHooks = &loopTestHooks{
		AfterPromisifyWorkerEntryClaim: func() {
			close(entryClaimed)
			<-releaseEntry
		},
	}
	closeReturned := make(chan struct{})
	firstInstructionAfterClose := make(chan bool, 1)
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		select {
		case <-closeReturned:
			firstInstructionAfterClose <- true
		default:
			firstInstructionAfterClose <- false
		}
		return "ignored", nil
	})
	select {
	case <-entryClaimed:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify worker did not claim user-function entry")
	}

	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	close(closeReturned)
	assertCloseSignals(t, loop)
	assertPromiseRejected(t, promise, ErrLoopTerminated)
	releaseEntryFn()
	select {
	case afterClose := <-firstInstructionAfterClose:
		if !afterClose {
			t.Fatal("claimed Promisify worker entered its user function before Close returned")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("claimed Promisify worker did not enter its user function after release")
	}
	waitPromisifyWorkersT(t, loop)
	assertPromiseRejected(t, promise, ErrLoopTerminated)
}

func testCloseTerminalWorkerFallbackT(t *testing.T, outcome func() (any, error)) {
	t.Helper()
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	releaseWorker := make(chan struct{})
	releaseWorkerFn := releaseSignalT(t, releaseWorker)
	workerStarted := make(chan struct{})
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		close(workerStarted)
		<-releaseWorker
		return outcome()
	})
	select {
	case <-workerStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify worker did not start")
	}

	pollPaused := make(chan struct{})
	releasePoll := make(chan struct{})
	releasePollFn := releaseSignalT(t, releasePoll)
	rejectionReached := make(chan struct{})
	releaseRejection := make(chan struct{})
	releaseRejectionFn := releaseSignalT(t, releaseRejection)
	var pauseOnce sync.Once
	loop.testHooks = &loopTestHooks{
		PrePollSleep: func() {
			pauseOnce.Do(func() { close(pollPaused) })
			<-releasePoll
		},
		BeforeClosePromiseRejection: func() {
			close(rejectionReached)
			<-releaseRejection
		},
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case <-pollPaused:
	case <-time.After(5 * time.Second):
		releasePollFn()
		t.Fatal("loop did not pause before native poll")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	select {
	case <-rejectionReached:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not pause before registered-promise rejection")
	}

	// Close has published terminal state, but registry rejection is blocked.
	// Release and join the worker so its direct fallback settlement definitely
	// executes first. Terminal-aware fallback must still reject the promise;
	// direct outcome settlement would make this assertion fail deterministically.
	releaseWorkerFn()
	waitPromisifyWorkersT(t, loop)
	assertPromiseRejected(t, promise, ErrLoopTerminated)
	releaseRejectionFn()

	releasePollFn()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not complete after loop owner release")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not complete after Close")
	}
	assertPromiseRejected(t, promise, ErrLoopTerminated)
}
