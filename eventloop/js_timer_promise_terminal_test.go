package eventloop

import (
	"context"
	"errors"
	"math"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestJSTimerPromisesSettleOnTerminalCleanup(t *testing.T) {
	terminationCases := []struct {
		name      string
		terminate func(*Loop) error
	}{
		{
			name: "graceful-shutdown",
			terminate: func(loop *Loop) error {
				return loop.Shutdown(context.Background())
			},
		},
		{
			name:      "immediate-close",
			terminate: func(loop *Loop) error { return loop.Close() },
		},
	}

	for _, tc := range terminationCases {
		for _, running := range []bool{false, true} {
			name := "before-run"
			if running {
				name = "while-running"
			}
			t.Run(tc.name+"/"+name, func(t *testing.T) {
				loop, err := New()
				if err != nil {
					t.Fatal(err)
				}
				js, err := NewJS(loop, WithUnhandledRejectionFallback(UnhandledRejectionFallbackDisabled))
				if err != nil {
					t.Fatal(err)
				}

				sleep := js.Sleep(time.Hour)
				timeout := js.Timeout(time.Hour)
				sleepResult := sleep.ToChannel()
				timeoutResult := timeout.ToChannel()

				if state := sleep.State(); state != Pending {
					t.Fatalf("Sleep state before termination = %v, want Pending", state)
				}
				if state := timeout.State(); state != Pending {
					t.Fatalf("Timeout state before termination = %v, want Pending", state)
				}
				assertTimerPromiseRegistrySize(t, js, 2)

				var runDone chan error
				if running {
					runDone = make(chan error, 1)
					go func() { runDone <- loop.Run(context.Background()) }()
					waitLoopOwnerTurnT(t, loop)
				}

				if err := tc.terminate(loop); err != nil {
					t.Fatalf("termination = %v", err)
				}
				if running {
					select {
					case err := <-runDone:
						if err != nil {
							t.Fatalf("Run = %v", err)
						}
					case <-time.After(5 * time.Second):
						t.Fatal("Run did not return after termination")
					}
				}

				if state := sleep.State(); state != Fulfilled {
					t.Fatalf("Sleep state after termination = %v, want Fulfilled", state)
				}
				if value := sleep.Value(); value != nil {
					t.Fatalf("Sleep value after termination = %#v, want nil", value)
				}
				assertSingleTimerPromiseResult(t, "Sleep", sleepResult, nil)

				if state := timeout.State(); state != Rejected {
					t.Fatalf("Timeout state after termination = %v, want Rejected", state)
				}
				reason, ok := timeout.Reason().(*TimeoutError)
				if !ok {
					t.Fatalf("Timeout reason after termination = %T, want *TimeoutError", timeout.Reason())
				}
				if got, want := reason.Message, "timeout after 1h0m0s"; got != want {
					t.Fatalf("Timeout message after termination = %q, want %q", got, want)
				}
				assertSingleTimerPromiseResult(t, "Timeout", timeoutResult, reason)
				assertTimerPromiseRegistrySize(t, js, 0)
			})
		}
	}
}

func TestJSTimerPromiseTerminalReactionDisposition(t *testing.T) {
	terminationCases := []struct {
		name      string
		terminate func(*Loop) error
	}{
		{
			name: "graceful-shutdown",
			terminate: func(loop *Loop) error {
				return loop.Shutdown(context.Background())
			},
		},
		{
			name:      "immediate-close",
			terminate: func(loop *Loop) error { return loop.Close() },
		},
	}
	reactionCases := []struct {
		name          string
		timeout       bool
		terminalChild bool
		create        func(*JS, func(any) any) (*ChainedPromise, *ChainedPromise)
	}{
		{
			name:          "sleep-then-handler",
			terminalChild: true,
			create: func(js *JS, handler func(any) any) (*ChainedPromise, *ChainedPromise) {
				parent := js.Sleep(time.Hour)
				return parent, parent.Then(handler, nil)
			},
		},
		{
			name:          "timeout-catch-handler",
			timeout:       true,
			terminalChild: true,
			create: func(js *JS, handler func(any) any) (*ChainedPromise, *ChainedPromise) {
				parent := js.Timeout(time.Hour)
				return parent, parent.Catch(handler)
			},
		},
		{
			name: "sleep-nil-pass-through",
			create: func(js *JS, _ func(any) any) (*ChainedPromise, *ChainedPromise) {
				parent := js.Sleep(time.Hour)
				return parent, parent.Then(nil, nil)
			},
		},
		{
			name:    "timeout-nil-pass-through",
			timeout: true,
			create: func(js *JS, _ func(any) any) (*ChainedPromise, *ChainedPromise) {
				parent := js.Timeout(time.Hour)
				return parent, parent.Catch(nil)
			},
		},
	}

	for _, terminationCase := range terminationCases {
		for _, reactionCase := range reactionCases {
			t.Run(terminationCase.name+"/"+reactionCase.name, func(t *testing.T) {
				loop, err := New()
				if err != nil {
					t.Fatal(err)
				}
				registerLoopCleanupT(t, loop)
				js, err := NewJS(loop, WithUnhandledRejectionFallback(UnhandledRejectionFallbackDisabled))
				if err != nil {
					t.Fatal(err)
				}

				var handlerCalls atomic.Int32
				parent, child := reactionCase.create(js, func(any) any {
					handlerCalls.Add(1)
					return "handler result"
				})
				parentResult := parent.ToChannel()
				childResult := child.ToChannel()

				ownerStarted := make(chan struct{})
				if err := loop.Submit(func() { close(ownerStarted) }); err != nil {
					t.Fatalf("Submit owner barrier: %v", err)
				}
				runDone := make(chan error, 1)
				go func() { runDone <- loop.Run(context.Background()) }()
				waitContractSignal(t, ownerStarted, "timer-promise terminal-reaction owner barrier")

				terminalDone := make(chan error, 1)
				go func() { terminalDone <- terminationCase.terminate(loop) }()
				if err := waitContractValue(t, terminalDone, terminationCase.name+" completion"); err != nil {
					t.Fatalf("%s: %v", terminationCase.name, err)
				}
				if err := waitContractValue(t, runDone, terminationCase.name+" Run completion"); err != nil {
					t.Fatalf("Run after %s: %v", terminationCase.name, err)
				}

				if calls := handlerCalls.Load(); calls != 0 {
					t.Fatalf("terminally disposed handler calls = %d, want 0", calls)
				}
				parentOutcome := assertTimerPromiseTerminalParent(
					t,
					reactionCase.timeout,
					parent,
					parentResult,
				)
				if reactionCase.terminalChild {
					assertTimerPromiseTerminalChild(t, child, childResult)
					return
				}
				assertTimerPromiseTerminalPassThrough(t, parent, child, childResult, parentOutcome)
			})
		}
	}
}

func TestTerminalTransitionSettlesTimerPromiseNeededByRunningCallback(t *testing.T) {
	timerCases := []struct {
		name   string
		create func(*JS) *ChainedPromise
		assert func(*testing.T, *ChainedPromise)
	}{
		{
			name:   "sleep",
			create: func(js *JS) *ChainedPromise { return js.Sleep(time.Hour) },
			assert: assertSleepTerminalSettlement,
		},
		{
			name:   "timeout",
			create: func(js *JS) *ChainedPromise { return js.Timeout(time.Hour) },
			assert: assertTimeoutTerminalSettlement,
		},
	}
	terminationCases := []struct {
		name      string
		configure func(*Loop, chan struct{})
		terminate func(*Loop) error
	}{
		{
			name: "close",
			configure: func(loop *Loop, committed chan struct{}) {
				loop.testHooks = &loopTestHooks{
					BeforeClosePromiseRejection: func() { close(committed) },
				}
			},
			terminate: func(loop *Loop) error { return loop.Close() },
		},
		{
			name: "shutdown",
			configure: func(loop *Loop, committed chan struct{}) {
				loop.testHooks = &loopTestHooks{
					AfterShutdownStateTerminating: func() { close(committed) },
				}
			},
			terminate: func(loop *Loop) error { return loop.Shutdown(context.Background()) },
		},
	}
	for _, terminationCase := range terminationCases {
		for _, timerCase := range timerCases {
			t.Run(terminationCase.name+"/"+timerCase.name, func(t *testing.T) {
				loop, err := New()
				if err != nil {
					t.Fatal(err)
				}
				js, err := NewJS(loop, WithUnhandledRejectionFallback(UnhandledRejectionFallbackDisabled))
				if err != nil {
					t.Fatal(err)
				}
				promise := timerCase.create(js)
				result := promise.ToChannel()
				callbackStarted := make(chan struct{})
				callbackResult := make(chan any, 1)
				releaseCallback := make(chan struct{})
				terminalCommitted := make(chan struct{})
				terminationCase.configure(loop, terminalCommitted)
				if err := loop.Submit(func() {
					close(callbackStarted)
					select {
					case value := <-result:
						callbackResult <- value
					case <-releaseCallback:
						callbackResult <- releaseCallback
					}
				}); err != nil {
					t.Fatal(err)
				}

				runDone := make(chan error, 1)
				go func() { runDone <- loop.Run(context.Background()) }()
				waitContractSignal(t, callbackStarted, "timer-promise callback entry")
				terminalDone := make(chan error, 1)
				go func() { terminalDone <- terminationCase.terminate(loop) }()
				waitContractSignal(t, terminalCommitted, terminationCase.name+" terminal-state publication")

				select {
				case value := <-callbackResult:
					if value == releaseCallback {
						t.Fatal("callback escaped without terminal timer-promise settlement")
					}
				case <-time.After(2 * time.Second):
					close(releaseCallback)
					<-callbackResult
					<-runDone
					<-terminalDone
					t.Fatalf("%s waited for loopDone before settling the timer promise needed by the running callback", terminationCase.name)
				}
				if err := waitContractValue(t, runDone, "Run after timer-promise dependency settlement"); err != nil {
					t.Fatalf("Run: %v", err)
				}
				if err := waitContractValue(t, terminalDone, terminationCase.name+" after timer-promise dependency settlement"); err != nil {
					t.Fatalf("%s: %v", terminationCase.name, err)
				}
				timerCase.assert(t, promise)
				assertTimerPromiseRegistrySize(t, js, 0)
			})
		}
	}
}

func TestJSTimerPromisesSettleWhenTerminationWinsBeforeTimerPublication(t *testing.T) {
	timerCases := []struct {
		name   string
		create func(*JS) *ChainedPromise
		assert func(*testing.T, *ChainedPromise)
	}{
		{
			name:   "sleep",
			create: func(js *JS) *ChainedPromise { return js.Sleep(time.Hour) },
			assert: assertSleepTerminalSettlement,
		},
		{
			name:   "timeout",
			create: func(js *JS) *ChainedPromise { return js.Timeout(time.Hour) },
			assert: assertTimeoutTerminalSettlement,
		},
	}
	terminationCases := []struct {
		name      string
		terminate func(*Loop) error
	}{
		{
			name: "graceful-shutdown",
			terminate: func(loop *Loop) error {
				return loop.Shutdown(context.Background())
			},
		},
		{
			name:      "immediate-close",
			terminate: func(loop *Loop) error { return loop.Close() },
		},
	}

	for _, timerCase := range timerCases {
		for _, terminationCase := range terminationCases {
			t.Run(timerCase.name+"/"+terminationCase.name, func(t *testing.T) {
				loop, err := New()
				if err != nil {
					t.Fatal(err)
				}
				js, err := NewJS(loop, WithUnhandledRejectionFallback(UnhandledRejectionFallbackDisabled))
				if err != nil {
					t.Fatal(err)
				}

				registered := make(chan struct{})
				releasePublication := make(chan struct{})
				var registerOnce sync.Once
				loop.testHooks = &loopTestHooks{
					AfterJSTimerPromiseRegister: func() {
						registerOnce.Do(func() { close(registered) })
						<-releasePublication
					},
				}
				promiseResult := make(chan *ChainedPromise, 1)
				go func() { promiseResult <- timerCase.create(js) }()

				select {
				case <-registered:
				case <-time.After(5 * time.Second):
					close(releasePublication)
					t.Fatal("timer promise was not registered before publication")
				}
				assertTimerPromiseRegistrySize(t, js, 1)

				if err := terminationCase.terminate(loop); err != nil {
					close(releasePublication)
					t.Fatalf("termination = %v", err)
				}
				assertTimerPromiseRegistrySize(t, js, 0)
				close(releasePublication)

				var promise *ChainedPromise
				select {
				case promise = <-promiseResult:
				case <-time.After(5 * time.Second):
					t.Fatal("timer-promise constructor did not return after terminal cleanup")
				}
				timerCase.assert(t, promise)
				assertTimerPromiseRegistrySize(t, js, 0)
			})
		}
	}
}

func TestJSTimerPromisesSettleOnceWhenCallbackWinsTerminalRace(t *testing.T) {
	timerCases := []struct {
		name   string
		create func(*JS) *ChainedPromise
		assert func(*testing.T, *ChainedPromise)
	}{
		{
			name:   "sleep",
			create: func(js *JS) *ChainedPromise { return js.Sleep(0) },
			assert: assertSleepTerminalSettlement,
		},
		{
			name:   "timeout",
			create: func(js *JS) *ChainedPromise { return js.Timeout(0) },
			assert: assertTimeoutZeroSettlement,
		},
	}

	for _, timerCase := range timerCases {
		t.Run(timerCase.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			js, err := NewJS(loop, WithUnhandledRejectionFallback(UnhandledRejectionFallbackDisabled))
			if err != nil {
				t.Fatal(err)
			}

			callbackReached := make(chan struct{})
			releaseCallback := make(chan struct{})
			closeTerminated := make(chan struct{})
			var callbackOnce sync.Once
			loop.testHooks = &loopTestHooks{
				BeforeJSTimerPromiseCallbackFinish: func() {
					callbackOnce.Do(func() { close(callbackReached) })
					<-releaseCallback
				},
				BeforeClosePromiseRejection: func() { close(closeTerminated) },
			}
			promise := timerCase.create(js)
			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(context.Background()) }()

			select {
			case <-callbackReached:
			case <-time.After(5 * time.Second):
				close(releaseCallback)
				t.Fatal("native timer callback did not reach settlement boundary")
			}

			closeDone := make(chan error, 1)
			go func() { closeDone <- loop.Close() }()
			waitContractSignal(t, closeTerminated, "Close terminal-state publication")
			close(releaseCallback)

			select {
			case err := <-closeDone:
				if err != nil {
					t.Fatalf("Close = %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Close did not return after timer callback release")
			}
			select {
			case err := <-runDone:
				if err != nil {
					t.Fatalf("Run = %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Run did not return after Close")
			}

			timerCase.assert(t, promise)
			assertTimerPromiseRegistrySize(t, js, 0)
		})
	}
}

func TestJSTimerPromisesSettleAfterScheduleRejection(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	js, err := NewJS(loop, WithUnhandledRejectionFallback(UnhandledRejectionFallbackDisabled))
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.Close(); err != nil {
		t.Fatal(err)
	}

	assertTimerPromiseAdmissionError(t, "Sleep", js.Sleep(time.Hour), ErrLoopTerminated)
	assertTimerPromiseAdmissionError(t, "Timeout", js.Timeout(time.Hour), ErrLoopTerminated)
	assertTimerPromiseRegistrySize(t, js, 0)
}

func TestJSTimerPromisesPreserveTimerIDExhaustion(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	js, err := NewJS(loop, WithUnhandledRejectionFallback(UnhandledRejectionFallbackDisabled))
	if err != nil {
		t.Fatal(err)
	}
	loop.nextTimerID.Store(math.MaxUint64)

	assertTimerPromiseAdmissionError(t, "Sleep", js.Sleep(time.Hour), ErrTimerIDExhausted)
	assertTimerPromiseAdmissionError(t, "Timeout", js.Timeout(time.Hour), ErrTimerIDExhausted)
	assertTimerPromiseRegistrySize(t, js, 0)
}

func TestJSTimerPromiseTerminalSettlementSurvivesChannelOnlyGC(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	loop.testHooks = &loopTestHooks{
		AfterJSTerminalSettlementCollect: func() {
			runtime.GC()
			runtime.Gosched()
			runtime.GC()
		},
	}

	sleepResult, timeoutResult := func() (<-chan any, <-chan any) {
		js, err := NewJS(loop, WithUnhandledRejectionFallback(UnhandledRejectionFallbackDisabled))
		if err != nil {
			t.Fatal(err)
		}
		return js.Sleep(time.Hour).ToChannel(), js.Timeout(time.Hour).ToChannel()
	}()

	if err := loop.Close(); err != nil {
		t.Fatal(err)
	}

	assertSingleTimerPromiseResult(t, "Sleep", sleepResult, nil)
	timeoutValue := func() any {
		select {
		case result, ok := <-timeoutResult:
			if !ok {
				t.Fatal("Timeout result channel closed without a value")
			}
			return result
		default:
			t.Fatal("Timeout result was not published during terminal cleanup")
			return nil
		}
	}()
	if _, ok := timeoutValue.(*TimeoutError); !ok {
		t.Fatalf("Timeout result = %T, want *TimeoutError", timeoutValue)
	}
	select {
	case _, ok := <-timeoutResult:
		if ok {
			t.Fatal("Timeout result channel published more than one value")
		}
	default:
		t.Fatal("Timeout result channel remained open after terminal publication")
	}
}

func assertTimerPromiseTerminalParent(
	t *testing.T,
	timeout bool,
	parent *ChainedPromise,
	result <-chan any,
) any {
	t.Helper()
	if !timeout {
		if state := parent.State(); state != Fulfilled {
			t.Fatalf("Sleep parent state = %v, want Fulfilled", state)
		}
		if value := parent.Value(); value != nil {
			t.Fatalf("Sleep parent value = %#v, want nil", value)
		}
		assertSingleTimerPromiseResult(t, "Sleep parent", result, nil)
		return nil
	}

	if state := parent.State(); state != Rejected {
		t.Fatalf("Timeout parent state = %v, want Rejected", state)
	}
	reason, ok := parent.Reason().(*TimeoutError)
	if !ok {
		t.Fatalf("Timeout parent reason = %T, want *TimeoutError", parent.Reason())
	}
	if want := "timeout after 1h0m0s"; reason.Message != want {
		t.Fatalf("Timeout parent message = %q, want %q", reason.Message, want)
	}
	assertSingleTimerPromiseResult(t, "Timeout parent", result, reason)
	return reason
}

func assertTimerPromiseTerminalChild(t *testing.T, child *ChainedPromise, result <-chan any) {
	t.Helper()
	if state := child.State(); state != Rejected {
		t.Fatalf("terminal reaction child state = %v, want Rejected", state)
	}
	reason, ok := child.Reason().(error)
	if !ok || !errors.Is(reason, ErrLoopTerminated) {
		t.Fatalf("terminal reaction child reason = %T %v, want ErrLoopTerminated", child.Reason(), child.Reason())
	}
	assertSingleTimerPromiseResult(t, "terminal reaction child", result, reason)
}

func assertTimerPromiseTerminalPassThrough(
	t *testing.T,
	parent *ChainedPromise,
	child *ChainedPromise,
	result <-chan any,
	parentOutcome any,
) {
	t.Helper()
	if state := child.State(); state != parent.State() {
		t.Fatalf("pass-through child state = %v, want parent state %v", state, parent.State())
	}
	switch parent.State() {
	case Fulfilled:
		if value := child.Value(); value != parentOutcome {
			t.Fatalf("pass-through child value = %#v, want %#v", value, parentOutcome)
		}
	case Rejected:
		if reason := child.Reason(); reason != parentOutcome {
			t.Fatalf("pass-through child reason = %#v, want parent reason %#v", reason, parentOutcome)
		}
	default:
		t.Fatalf("pass-through parent state = %v, want settled", parent.State())
	}
	assertSingleTimerPromiseResult(t, "pass-through child", result, parentOutcome)
}

func assertSingleTimerPromiseResult(t *testing.T, name string, result <-chan any, want any) {
	t.Helper()

	select {
	case got, ok := <-result:
		if !ok {
			t.Fatalf("%s result channel closed without a value", name)
		}
		if got != want {
			t.Fatalf("%s result = %#v, want %#v", name, got, want)
		}
	default:
		t.Fatalf("%s result was not published during terminal cleanup", name)
	}

	select {
	case _, ok := <-result:
		if ok {
			t.Fatalf("%s result channel published more than one value", name)
		}
	default:
		t.Fatalf("%s result channel remained open after terminal publication", name)
	}
}

func assertSleepTerminalSettlement(t *testing.T, promise *ChainedPromise) {
	t.Helper()
	if state := promise.State(); state != Fulfilled {
		t.Fatalf("Sleep state = %v, want Fulfilled", state)
	}
	if value := promise.Value(); value != nil {
		t.Fatalf("Sleep value = %#v, want nil", value)
	}
	assertSingleTimerPromiseResult(t, "Sleep", promise.ToChannel(), nil)
}

func assertTimeoutTerminalSettlement(t *testing.T, promise *ChainedPromise) {
	assertTimeoutSettlement(t, promise, "timeout after 1h0m0s")
}

func assertTimeoutZeroSettlement(t *testing.T, promise *ChainedPromise) {
	assertTimeoutSettlement(t, promise, "timeout after 0s")
}

func assertTimerPromiseAdmissionError(
	t *testing.T,
	name string,
	promise *ChainedPromise,
	want error,
) {
	t.Helper()
	if state := promise.State(); state != Rejected {
		t.Fatalf("%s state = %v, want Rejected", name, state)
	}
	if reason := promise.Reason(); reason != want {
		t.Fatalf("%s reason = %#v, want %#v", name, reason, want)
	}
	assertSingleTimerPromiseResult(t, name, promise.ToChannel(), want)
}

func assertTimeoutSettlement(t *testing.T, promise *ChainedPromise, wantMessage string) {
	t.Helper()
	if state := promise.State(); state != Rejected {
		t.Fatalf("Timeout state = %v, want Rejected", state)
	}
	reason, ok := promise.Reason().(*TimeoutError)
	if !ok {
		t.Fatalf("Timeout reason = %T, want *TimeoutError", promise.Reason())
	}
	if reason.Message != wantMessage {
		t.Fatalf("Timeout message = %q, want %q", reason.Message, wantMessage)
	}
	assertSingleTimerPromiseResult(t, "Timeout", promise.ToChannel(), reason)
}

func assertTimerPromiseRegistrySize(t *testing.T, js *JS, want int) {
	t.Helper()
	js.timerPromisesMu.Lock()
	got := len(js.timerPromises)
	js.timerPromisesMu.Unlock()
	if got != want {
		t.Fatalf("timer-promise registry size = %d, want %d", got, want)
	}
}
