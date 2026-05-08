package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

type promiseFinallyTerminalCase struct {
	name        string
	reject      bool
	nilCallback bool
}

func promiseFinallyTerminalCases() []promiseFinallyTerminalCase {
	return []promiseFinallyTerminalCase{
		{name: "fulfilled/non-nil callback"},
		{name: "fulfilled/nil callback", nilCallback: true},
		{name: "rejected/non-nil callback", reject: true},
		{name: "rejected/nil callback", reject: true, nilCallback: true},
	}
}

func newTerminalFinallyReaction(
	t *testing.T,
	loop *Loop,
	testCase promiseFinallyTerminalCase,
) (*JS, *ChainedPromise, *ChainedPromise, <-chan any, *atomic.Int32, PromiseState, any) {
	t.Helper()
	js := NewJS(loop)
	source, resolveSource, rejectSource := js.NewChainedPromise()
	var callbackCalls atomic.Int32
	var callback func()
	if !testCase.nilCallback {
		callback = func() { callbackCalls.Add(1) }
	}
	child := source.Finally(callback)
	// This matrix observes the child directly; suppress unrelated unhandled-
	// rejection reporting without attaching another reaction to the queue.
	child.rejectionHandled.Store(true)
	resultChannel := child.ToChannel()
	if testCase.reject {
		reason := errors.New("source rejection")
		rejectSource(reason)
		return js, source, child, resultChannel, &callbackCalls, Rejected, reason
	}
	const value = "source fulfillment"
	resolveSource(value)
	return js, source, child, resultChannel, &callbackCalls, Fulfilled, value
}

func assertTerminalFinallyOutcome(
	t *testing.T,
	testCase promiseFinallyTerminalCase,
	source *ChainedPromise,
	child *ChainedPromise,
	resultChannel <-chan any,
	wantState PromiseState,
	wantResult any,
) {
	t.Helper()
	if source.State() != wantState {
		t.Fatalf("source state = %v, want %v", source.State(), wantState)
	}
	if wantState == Fulfilled && source.Value() != wantResult {
		t.Fatalf("source value = %#v, want %#v", source.Value(), wantResult)
	}
	if wantState == Rejected && source.Reason() != wantResult {
		t.Fatalf("source reason = %#v, want %#v", source.Reason(), wantResult)
	}
	result := waitContractValue(t, resultChannel, "terminal Finally result")
	if testCase.nilCallback {
		if child.State() != wantState {
			t.Fatalf("Finally(nil) state = %v, want %v", child.State(), wantState)
		}
		if wantState == Fulfilled && child.Value() != wantResult {
			t.Fatalf("Finally(nil) value = %#v, want %#v", child.Value(), wantResult)
		}
		if wantState == Rejected && child.Reason() != wantResult {
			t.Fatalf("Finally(nil) reason = %#v, want %#v", child.Reason(), wantResult)
		}
		if result != wantResult {
			t.Fatalf("Finally(nil) channel result = %#v, want %#v", result, wantResult)
		}
	} else {
		if child.State() != Rejected {
			t.Fatalf("terminally discarded Finally state = %v, want Rejected", child.State())
		}
		reason, ok := child.Reason().(error)
		if !ok || !errors.Is(reason, ErrLoopTerminated) {
			t.Fatalf("terminally discarded Finally reason = %T %v, want ErrLoopTerminated", child.Reason(), child.Reason())
		}
		resultErr, ok := result.(error)
		if !ok || !errors.Is(resultErr, ErrLoopTerminated) {
			t.Fatalf("terminally discarded Finally channel result = %T %v, want ErrLoopTerminated", result, result)
		}
	}
	assertPromiseResultChannelClosed(t, resultChannel)
}

func TestPromiseFinallyAcceptedNotDequeuedTerminalDisposition(t *testing.T) {
	for _, testCase := range promiseFinallyTerminalCases() {
		t.Run(testCase.name, func(t *testing.T) {
			loop := New(WithLogger(nil))
			registerLoopCleanupT(t, loop)
			js, source, child, resultChannel, callbackCalls, wantState, wantResult :=
				newTerminalFinallyReaction(t, loop, testCase)
			if got := pendingPromiseReactionCount(loop); got != 1 {
				t.Fatalf("pending Finally reactions before Close = %d, want 1", got)
			}
			if err := loop.Close(); err != nil {
				t.Fatal(err)
			}
			if got := callbackCalls.Load(); got != 0 {
				t.Fatalf("terminally discarded Finally callback calls = %d, want 0", got)
			}
			if got := pendingPromiseReactionCount(loop); got != 0 {
				t.Fatalf("pending Finally reactions after Close = %d, want 0", got)
			}
			waitTerminalUnhandledRejectionTrackingDrained(t, js)
			assertTerminalFinallyOutcome(t, testCase, source, child, resultChannel, wantState, wantResult)
		})
	}
}

func TestPromiseFinallyDequeuedAdmissionLossTerminalDisposition(t *testing.T) {
	for _, testCase := range promiseFinallyTerminalCases() {
		t.Run(testCase.name, func(t *testing.T) {
			loop := New(WithLogger(nil))
			registerLoopCleanupT(t, loop)
			admissionEntered := make(chan struct{})
			releaseAdmission := make(chan struct{})
			releaseAdmissionFn := releaseSignalT(t, releaseAdmission)
			closeTransitioned := make(chan struct{})
			var admissionOnce sync.Once
			loop.testHooks = &loopTestHooks{
				BeforeCallbackAdmission: func() {
					admissionOnce.Do(func() {
						close(admissionEntered)
						<-releaseAdmission
					})
				},
				AfterCloseStateTerminating: func() { close(closeTransitioned) },
			}

			js, source, child, resultChannel, callbackCalls, wantState, wantResult :=
				newTerminalFinallyReaction(t, loop, testCase)
			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(context.Background()) }()
			waitContractSignal(t, admissionEntered, "dequeued Finally reaction admission")
			closeDone := make(chan error, 1)
			go func() { closeDone <- loop.Close() }()
			waitContractSignal(t, closeTransitioned, "Close callback gate transition")
			if got := pendingPromiseReactionCount(loop); got != 0 {
				t.Fatalf("dequeued Finally reaction remained pending during admission loss: %d", got)
			}
			if state := child.State(); state != Pending {
				t.Fatalf("dequeued Finally child settled before admission decision: %v", state)
			}
			releaseAdmissionFn()
			if err := waitContractValue(t, closeDone, "Close after dequeued Finally reaction"); err != nil {
				t.Fatalf("Close: %v", err)
			}
			if err := waitContractValue(t, runDone, "Run after dequeued Finally reaction"); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if got := callbackCalls.Load(); got != 0 {
				t.Fatalf("terminally denied Finally callback calls = %d, want 0", got)
			}
			if got := pendingPromiseReactionCount(loop); got != 0 {
				t.Fatalf("pending Finally reactions after admission loss = %d, want 0", got)
			}
			waitTerminalUnhandledRejectionTrackingDrained(t, js)
			assertTerminalFinallyOutcome(t, testCase, source, child, resultChannel, wantState, wantResult)
		})
	}
}

func TestPromiseFinally_TerminatedLoop(t *testing.T) {
	loop := New()
	js := NewJS(loop, WithUnhandledRejection(func(any) {}))
	parent := js.Resolve("value")
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	var called atomic.Bool
	child := parent.Finally(func() { called.Store(true) })
	if called.Load() {
		t.Fatal("Finally callback ran after loop termination")
	}
	if child.State() != Rejected {
		t.Fatalf("child state = %v, want Rejected", child.State())
	}
	reason, ok := child.Reason().(error)
	if !ok || !errors.Is(reason, ErrLoopTerminated) {
		t.Fatalf("child reason = %v, want ErrLoopTerminated", child.Reason())
	}
}

func TestPromiseFinally_PanicInHandler(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	js := NewJS(loop)

	parent, resolve, _ := js.NewChainedPromise()
	var calls atomic.Int32
	child := parent.Finally(func() {
		calls.Add(1)
		panic("finally panic")
	})
	resolve("value")
	loop.tick()

	if calls.Load() != 1 {
		t.Fatalf("Finally calls = %d, want 1", calls.Load())
	}
	if parent.State() != Fulfilled || parent.Value() != "value" {
		t.Fatalf("parent = (%v, %#v), want (Fulfilled, value)", parent.State(), parent.Value())
	}
	if child.State() != Fulfilled || child.Value() != "value" {
		t.Fatalf("child = (%v, %#v), want (Fulfilled, value)", child.State(), child.Value())
	}
}
