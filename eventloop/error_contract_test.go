package eventloop

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/joeycumines/logiface"
)

func TestPromiseReactionGoexitSettlement(t *testing.T) {
	t.Run("Then rejects its child", func(t *testing.T) {
		loop, js := newErrorContractJS(t)
		promise, resolve, _ := js.NewChainedPromise()
		child := promise.Then(func(any) any {
			runtime.Goexit()
			return nil
		}, nil)

		resolve("value")
		loop.tick()
		assertPromiseReason(t, child, ErrGoexit)
	})

	t.Run("Catch rejects its child", func(t *testing.T) {
		loop, js := newErrorContractJS(t)
		promise, _, reject := js.NewChainedPromise()
		child := promise.Catch(func(any) any {
			runtime.Goexit()
			return nil
		})

		reject("reason")
		loop.tick()
		assertPromiseReason(t, child, ErrGoexit)
	})

	for _, rejected := range []bool{false, true} {
		name := "fulfilled"
		if rejected {
			name = "rejected"
		}
		t.Run("Finally preserves "+name, func(t *testing.T) {
			loop, js := newErrorContractJS(t)
			promise, resolve, reject := js.NewChainedPromise()
			child := promise.Finally(func() { runtime.Goexit() })

			if rejected {
				reject("reason")
			} else {
				resolve("value")
			}
			loop.tick()

			if rejected {
				if child.State() != Rejected || child.Reason() != "reason" {
					t.Fatalf("Finally child = (%v, %#v), want Rejected reason", child.State(), child.Reason())
				}
			} else if child.State() != Fulfilled || child.Value() != "value" {
				t.Fatalf("Finally child = (%v, %#v), want Fulfilled value", child.State(), child.Value())
			}
		})
	}
}

func TestUnhandledRejectionGoexitReleasesAuxiliaryState(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	js, err := NewJS(loop, WithUnhandledRejection(func(any) { runtime.Goexit() }))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	promise := js.Reject("reason")
	js.handlerReadyMu.Lock()
	js.handlerReadyChans[promise] = make(chan struct{})
	js.handlerReadyMu.Unlock()

	loop.tick()

	js.handlerReadyMu.Lock()
	_, handlerReadyExists := js.handlerReadyChans[promise]
	js.handlerReadyMu.Unlock()
	if handlerReadyExists {
		t.Fatal("Goexit left handler-publication synchronization state")
	}
	waitUnhandledRejectionCheckOwnershipReleased(t, js)
	assertUnhandledRejectionTrackingDrained(t, js)
}

func TestUnhandledRejectionGoexitReschedulesNestedRejection(t *testing.T) {
	testUnhandledRejectionGoexitReschedulesNestedRejection(t, false)
}

func TestShutdownUnhandledRejectionGoexitReschedulesNestedRejection(t *testing.T) {
	testUnhandledRejectionGoexitReschedulesNestedRejection(t, true)
}

func testUnhandledRejectionGoexitReschedulesNestedRejection(t *testing.T, shutdown bool) {
	t.Helper()
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	reported := make(chan any, 2)
	var calls atomic.Int32
	var js *JS
	js, err = NewJS(loop, WithUnhandledRejection(func(reason any) {
		reported <- reason
		if calls.Add(1) == 1 {
			js.Reject("nested")
		}
		runtime.Goexit()
	}))
	if err != nil {
		t.Fatal(err)
	}
	js.Reject("initial")

	if shutdown {
		if err := loop.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	} else {
		loop.tick()
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("unhandled-rejection callback calls = %d, want 2; nested rejection was not rescheduled after Goexit", got)
	}
	if first := waitContractValue(t, reported, "initial unhandled rejection"); first != "initial" {
		t.Fatalf("first unhandled rejection = %v, want initial", first)
	}
	if second := waitContractValue(t, reported, "nested unhandled rejection"); second != "nested" {
		t.Fatalf("second unhandled rejection = %v, want nested", second)
	}
	waitUnhandledRejectionCheckOwnershipReleased(t, js)
	assertUnhandledRejectionTrackingDrained(t, js)
}

func TestCloseUnhandledRejectionGoexitHandsNestedRejectionToFallback(t *testing.T) {
	logRecords := make(chan *testEvent, 8)
	logger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](&testEventFactory{}),
		logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(event *testEvent) error {
			logRecords <- event
			return nil
		})),
	).Logger()
	loop, err := New(WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	initialStarted := make(chan struct{})
	releaseInitial := make(chan struct{})
	releaseInitialFn := releaseSignalT(t, releaseInitial)
	closeTransitioned := make(chan struct{})
	reported := make(chan any, 3)
	loop.testHooks = &loopTestHooks{
		AfterCloseStateTerminating: func() { close(closeTransitioned) },
	}
	var js *JS
	js, err = NewJS(loop,
		WithUnhandledRejection(func(reason any) {
			reported <- reason
			if reason == "initial" {
				js.Reject("nested")
				close(initialStarted)
				<-releaseInitial
				runtime.Goexit()
			}
		}),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
	)
	if err != nil {
		t.Fatal(err)
	}
	js.Reject("initial")

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, initialStarted, "initial unhandled-rejection callback")
	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	waitContractSignal(t, closeTransitioned, "Close StateTerminating publication")
	releaseInitialFn()

	if err := waitContractValue(t, closeDone, "Close completion after rejection Goexit"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := waitContractValue(t, runDone, "Run completion after rejection Goexit"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if first := waitContractValue(t, reported, "initial unhandled rejection"); first != "initial" {
		t.Fatalf("first unhandled rejection = %v, want initial", first)
	}
	if second := waitContractValue(t, reported, "nested terminal fallback"); second != "nested" {
		t.Fatalf("second unhandled rejection = %v, want nested", second)
	}
	waitTerminalUnhandledRejectionTrackingDrained(t, js)
	select {
	case reason := <-reported:
		t.Fatalf("duplicate unhandled rejection after Close/Goexit handoff: %v", reason)
	default:
	}
	for {
		select {
		case event := <-logRecords:
			if event.message == "eventloop: failed to reschedule unhandled rejection check microtask" {
				t.Fatalf("expected terminal fallback emitted a reschedule failure diagnostic: fields=%v", event.fields)
			}
		default:
			return
		}
	}
}

func TestRunJoinsPollFailureWithConcurrentContextCancellation(t *testing.T) {
	if !fdPollingSupported {
		t.Skip("native poll failures are unavailable on task-only targets")
	}
	sentinel := errors.New("injected poll failure")
	ctx, cancel := context.WithCancel(context.Background())
	records := make(chan *testEvent, 2)
	logger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](&testEventFactory{}),
		logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(event *testEvent) error {
			records <- event
			return nil
		})),
	).Logger()
	loop, err := New(WithFastPathMode(FastPathDisabled), WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	loop.testHooks = &loopTestHooks{
		PollIO: func(int) (int, error) { return 0, nil },
		PollError: func() error {
			cancel()
			return sentinel
		},
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	err = waitContractValue(t, runDone, "poll-error Run completion")
	if !errors.Is(err, context.Canceled) || !errors.Is(err, sentinel) {
		t.Fatalf("Run error = %v, want context.Canceled joined with sentinel", err)
	}
	event := waitContractValue(t, records, "poll-error critical diagnostic")
	if event.level != logiface.LevelCritical {
		t.Fatalf("diagnostic level = %v, want %v", event.level, logiface.LevelCritical)
	}
	if event.message != "pollIO failed" {
		t.Fatalf("diagnostic message = %q, want %q", event.message, "pollIO failed")
	}
	if got := event.fields["component"]; got != "eventloop" {
		t.Fatalf("diagnostic component = %#v, want eventloop", got)
	}
	loggedErr, ok := event.fields["err"].(error)
	if !ok || !errors.Is(loggedErr, sentinel) {
		t.Fatalf("diagnostic error = %#v, want %v", event.fields["err"], sentinel)
	}
	select {
	case unexpected := <-records:
		t.Fatalf("unexpected additional diagnostic: level=%v message=%q fields=%#v", unexpected.level, unexpected.message, unexpected.fields)
	default:
	}
}

func TestInternalPollGoexitDoesNotEscape(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	loop.executePollInternal(func() { runtime.Goexit() })
	var subsequent atomic.Bool
	loop.executePollInternal(func() { subsequent.Store(true) })
	if !subsequent.Load() {
		t.Fatal("internal callback worker was not replaced after runtime.Goexit")
	}
}

func TestRequiredCallbacksRejectNilSynchronously(t *testing.T) {
	loop, js := newErrorContractJS(t)
	tests := []struct {
		name string
		call func()
	}{
		{name: "Submit", call: func() { _ = loop.Submit(nil) }},
		{name: "SubmitInternal", call: func() { _ = loop.SubmitInternal(nil) }},
		{name: "ScheduleImmediate", call: func() { _ = loop.ScheduleImmediate(nil) }},
		{name: "ScheduleImmediateRef", call: func() { _ = loop.ScheduleImmediateRef(nil, nil) }},
		{name: "ScheduleCloseCallback", call: func() { _ = loop.ScheduleCloseCallback(nil) }},
		{name: "ScheduleMicrotask", call: func() { _ = loop.ScheduleMicrotask(nil) }},
		{name: "ScheduleMicrotaskCheckpoint", call: func() { _ = loop.ScheduleMicrotaskCheckpoint(nil) }},
		{name: "ScheduleNextTick", call: func() { _ = loop.ScheduleNextTick(nil) }},
		{name: "ScheduleTimer", call: func() { _, _ = loop.ScheduleTimer(0, nil) }},
		{name: "QueueMicrotask", call: func() { _ = js.QueueMicrotask(nil) }},
		{name: "NextTick", call: func() { _ = js.NextTick(nil) }},
		{name: "SetTimeout", call: func() { _, _ = js.SetTimeout(nil, 0) }},
		{name: "SetInterval", call: func() { _, _ = js.SetInterval(nil, 0) }},
		{name: "SetImmediate", call: func() { _, _ = js.SetImmediate(nil) }},
		{name: "Promisify", call: func() { loop.Promisify(context.Background(), nil) }},
		{name: "Try", call: func() { js.Try(nil) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := captureErrorContractPanic(test.call); got == nil {
				t.Fatalf("%s accepted a nil callback", test.name)
			}
		})
	}
}

func TestReadinessMethodsRequireConstructedLoop(t *testing.T) {
	receivers := []struct {
		name string
		loop *Loop
	}{
		{name: "nil"},
		{name: "zero value", loop: &Loop{}},
	}
	methods := []struct {
		name string
		call func(*Loop)
	}{
		{name: "RegisterFD", call: func(loop *Loop) { _ = loop.RegisterFD(0, EventRead, func(IOEvents) {}) }},
		{name: "ModifyFD", call: func(loop *Loop) { _ = loop.ModifyFD(0, EventRead) }},
		{name: "UnregisterFD", call: func(loop *Loop) { _ = loop.UnregisterFD(0) }},
	}
	for _, receiver := range receivers {
		for _, method := range methods {
			t.Run(receiver.name+"/"+method.name, func(t *testing.T) {
				panicValue := captureErrorContractPanic(func() { method.call(receiver.loop) })
				panicErr, ok := panicValue.(error)
				if !ok || !errors.Is(panicErr, errLoopUninitialized) {
					t.Fatalf("panic = %T %#v, want error matching errLoopUninitialized", panicValue, panicValue)
				}
				want := "eventloop: " + method.name + ": " + errLoopUninitialized.Error()
				if got := panicErr.Error(); got != want {
					t.Fatalf("panic error = %q, want %q", got, want)
				}
			})
		}
	}
}

func TestPromiseResolutionErrorIdentities(t *testing.T) {
	_, js := newErrorContractJS(t)

	self, resolveSelf, _ := js.NewChainedPromise()
	resolveSelf(self)
	assertPromiseReason(t, self, ErrPromiseSelfResolution)

	typedNil, resolveTypedNil, _ := js.NewChainedPromise()
	var source *ChainedPromise
	resolveTypedNil(source)
	assertPromiseReason(t, typedNil, ErrPromiseNilAdoption)
}

func newErrorContractJS(t *testing.T) (*Loop, *JS) {
	t.Helper()
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	return loop, js
}

func assertPromiseReason(t *testing.T, promise *ChainedPromise, target error) {
	t.Helper()
	if promise.State() != Rejected {
		t.Fatalf("promise state = %v, want Rejected", promise.State())
	}
	reason, ok := promise.Reason().(error)
	if !ok || !errors.Is(reason, target) {
		t.Fatalf("promise reason = %T %#v, want %v", promise.Reason(), promise.Reason(), target)
	}
}

func captureErrorContractPanic(fn func()) (value any) {
	defer func() { value = recover() }()
	fn()
	return nil
}
