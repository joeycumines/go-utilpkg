package eventloop

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/logiface"
)

// testEvent is a minimal logiface.Event implementation for testing the
// structured logging paths (logCritical, logError).
type testEvent struct {
	logiface.UnimplementedEvent
	level   logiface.Level
	message string
	fields  map[string]any
}

type abnormalExitLogEvent struct {
	logiface.UnimplementedEvent
	level logiface.Level
	stage string
	exit  func()
}

func (e *abnormalExitLogEvent) Level() logiface.Level { return e.level }

func (e *abnormalExitLogEvent) AddString(string, string) bool {
	if e.stage == "AddString" {
		e.exit()
	}
	return e.stage != "AddField"
}

func (e *abnormalExitLogEvent) AddField(string, any) {
	if e.stage == "AddField" {
		e.exit()
	}
}

func (e *abnormalExitLogEvent) AddError(error) bool {
	if e.stage == "AddError" {
		e.exit()
	}
	return true
}

func (e *abnormalExitLogEvent) AddMessage(string) bool {
	if e.stage == "AddMessage" {
		e.exit()
	}
	return true
}

func (e *testEvent) Level() logiface.Level { return e.level }
func (e *testEvent) AddMessage(message string) bool {
	e.message = message
	return true
}
func (e *testEvent) AddField(key string, val any) {
	if e.fields == nil {
		e.fields = make(map[string]any)
	}
	e.fields[key] = val
}

// testEventFactory creates testEvent instances.
type testEventFactory struct {
	onNew func(logiface.Level) // callback when NewEvent is called
}

func (f *testEventFactory) NewEvent(level logiface.Level) *testEvent {
	if f.onNew != nil {
		f.onNew(level)
	}
	return &testEvent{level: level}
}

// testEventWriter writes testEvent instances.
type testEventWriter struct {
	onWrite func(*testEvent) error
}

func (w *testEventWriter) Write(event *testEvent) error {
	if w.onWrite != nil {
		return w.onWrite(event)
	}
	return nil
}

// --- logCritical coverage (20% → higher) ---

func TestLogCritical_WithEnabledLogger(t *testing.T) {
	expected := errors.New("test error")
	records := make(chan *testEvent, 1)

	writer := &testEventWriter{
		onWrite: func(event *testEvent) error {
			records <- event
			return nil
		},
	}
	factory := &testEventFactory{}

	typedLogger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](factory),
		logiface.WithWriter[*testEvent](writer),
	)

	// Convert to the generic Logger[Event] that Loop requires
	genericLogger := typedLogger.Logger()

	loop := New(WithLogger(genericLogger))
	registerLoopCleanupT(t, loop)

	loop.logCritical("test critical message", expected)

	event := waitContractValue(t, records, "critical diagnostic")
	if event.level != logiface.LevelCritical {
		t.Fatalf("level = %v, want %v", event.level, logiface.LevelCritical)
	}
	if event.message != "test critical message" {
		t.Fatalf("message = %q, want %q", event.message, "test critical message")
	}
	if len(event.fields) != 2 {
		t.Fatalf("fields = %#v, want exactly component and err", event.fields)
	}
	if got := event.fields["component"]; got != "eventloop" {
		t.Fatalf("component field = %#v, want eventloop", got)
	}
	if got := event.fields["err"]; got != expected {
		t.Fatalf("error field = %#v, want identity %p", got, expected)
	}
}

func TestLogCritical_WithPanickingLogger(t *testing.T) {
	called := false
	writer := &testEventWriter{
		onWrite: func(event *testEvent) error {
			called = true
			panic("logger panic")
		},
	}
	factory := &testEventFactory{}

	typedLogger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](factory),
		logiface.WithWriter[*testEvent](writer),
	)
	genericLogger := typedLogger.Logger()

	loop := New(WithLogger(genericLogger))
	registerLoopCleanupT(t, loop)

	// A panicking configured logger must not escape into loop control flow.
	loop.logCritical("test critical with panic", errors.New("test error"))
	if !called {
		t.Fatal("panicking logger writer was not invoked")
	}
}

func TestLogCritical_WithGoexitLogger(t *testing.T) {
	called := make(chan struct{}, 1)
	typedLogger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](&testEventFactory{}),
		logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(*testEvent) error {
			called <- struct{}{}
			runtime.Goexit()
			return nil
		})),
	)
	loop := New(WithLogger(typedLogger.Logger()))
	registerLoopCleanupT(t, loop)

	returned := make(chan struct{})
	go func() {
		loop.logCritical("test critical with runtime.Goexit", errors.New("test error"))
		close(returned)
	}()
	waitContractSignal(t, called, "runtime.Goexit logger invocation")
	waitContractSignal(t, returned, "logCritical return after logger runtime.Goexit")
}

func TestLoopDiagnosticsAcceptNilLoggerReceiver(t *testing.T) {
	var logger *logiface.Logger[logiface.Event]
	loop := New(WithLogger(logger))
	registerLoopCleanupT(t, loop)

	loop.Log(logiface.LevelError, nil)
	loop.logError("nil logger error", errors.New("ignored"))
	loop.logCritical("nil logger critical", errors.New("ignored"))
}

func TestLoggerGoexitInsideCallbackDoesNotTerminateOwner(t *testing.T) {
	called := make(chan struct{}, 1)
	typedLogger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](&testEventFactory{}),
		logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(*testEvent) error {
			called <- struct{}{}
			runtime.Goexit()
			return nil
		})),
	)
	loop := New(WithLogger(typedLogger.Logger()))
	registerLoopCleanupT(t, loop)

	callbackReturned := make(chan struct{})
	if err := loop.Submit(func() {
		loop.logError("test callback diagnostic", nil)
		close(callbackReturned)
		if err := loop.Shutdown(context.Background()); err != nil {
			panic(err)
		}
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	waitContractSignal(t, called, "callback logger runtime.Goexit")
	waitContractSignal(t, callbackReturned, "callback continuation after logger runtime.Goexit")
	if err := waitContractValue(t, runDone, "Run after callback logger runtime.Goexit"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestLoggerCallbackRetainsActiveOwner(t *testing.T) {
	predicateCalled := make(chan struct{}, 1)
	aliveObserved := make(chan bool, 1)
	var loop *Loop
	typedLogger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](&testEventFactory{}),
		logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(*testEvent) error {
			aliveObserved <- loop.Alive()
			return nil
		})),
	)
	loop = New(WithLogger(typedLogger.Logger()))
	registerLoopCleanupT(t, loop)

	callbackReturned := make(chan struct{})
	if err := loop.Submit(func() {
		if err := loop.ScheduleImmediateRef(func() {}, func() bool {
			predicateCalled <- struct{}{}
			return true
		}); err != nil {
			panic(err)
		}
		loop.logError("test owner diagnostic", nil)
		close(callbackReturned)
		if err := loop.Shutdown(context.Background()); err != nil {
			panic(err)
		}
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	if !waitContractValue(t, aliveObserved, "logger-owned Alive result") {
		t.Fatal("logger-owned Alive returned false with a ref'd check callback")
	}
	waitContractSignal(t, predicateCalled, "logger-owned dynamic liveness predicate")
	waitContractSignal(t, callbackReturned, "callback after logger-owned Alive")
	if err := waitContractValue(t, runDone, "Run after logger-owned Alive"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestImmediateTerminalDependencyLoggerRetainsCompletionOwner(t *testing.T) {
	for _, test := range []struct {
		name    string
		reenter func(*Loop) error
		want    error
	}{
		{name: "close", reenter: func(loop *Loop) error { return loop.Close() }},
		{
			name:    "shutdown",
			reenter: func(loop *Loop) error { return loop.Shutdown(context.Background()) },
			want:    ErrLoopTerminated,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			lifecycleResult := make(chan error, 1)
			var loop *Loop
			logger := logiface.New[*testEvent](
				logiface.WithEventFactory[*testEvent](&testEventFactory{}),
				logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(*testEvent) error {
					lifecycleResult <- test.reenter(loop)
					return nil
				})),
			)
			loop = New(WithLogger(logger.Logger()))
			js := NewJS(loop, WithUnhandledRejectionFallback(UnhandledRejectionFallbackDisabled))
			result := js.Timeout(time.Hour).ToChannel()
			callbackStarted := make(chan struct{})
			if err := loop.Submit(func() {
				close(callbackStarted)
				<-result
			}); err != nil {
				t.Fatal(err)
			}

			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(context.Background()) }()
			waitContractSignal(t, callbackStarted, "terminal-dependency callback entry")
			closeDone := make(chan error, 1)
			go func() { closeDone <- loop.Close() }()

			select {
			case got := <-lifecycleResult:
				if !errors.Is(got, test.want) || (got == nil) != (test.want == nil) {
					t.Fatalf("%s from terminal-dependency logger = %v, want %v", test.name, got, test.want)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("%s from terminal-dependency logger joined its own completion", test.name)
			}
			if err := waitContractValue(t, runDone, "Run after terminal-dependency logger"); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if err := waitContractValue(t, closeDone, "Close after terminal-dependency logger"); err != nil {
				t.Fatalf("Close: %v", err)
			}
		})
	}
}

func TestLoggerDropsRecursiveDiagnostics(t *testing.T) {
	var calls atomic.Int32
	var loop *Loop
	typedLogger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](&testEventFactory{}),
		logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(*testEvent) error {
			calls.Add(1)
			loop.logError("recursive diagnostic", nil)
			return nil
		})),
	)
	loop = New(WithLogger(typedLogger.Logger()))
	registerLoopCleanupT(t, loop)

	returned := make(chan struct{})
	go func() {
		loop.logCritical("outer diagnostic", nil)
		close(returned)
	}()
	waitContractSignal(t, returned, "recursive logger suppression")
	if got := calls.Load(); got != 1 {
		t.Fatalf("logger calls = %d, want 1", got)
	}
}

func TestLoggerGoexitStagesDoNotTerminateCaller(t *testing.T) {
	for _, stage := range []string{"factory", "modifier", "writer", "releaser"} {
		t.Run(stage, func(t *testing.T) {
			var calls atomic.Int32
			factory := &testEventFactory{}
			writer := &testEventWriter{}
			options := []logiface.Option[*testEvent]{
				logiface.WithEventFactory[*testEvent](factory),
				logiface.WithWriter[*testEvent](writer),
			}
			goexit := func() {
				calls.Add(1)
				runtime.Goexit()
			}
			switch stage {
			case "factory":
				factory.onNew = func(logiface.Level) { goexit() }
			case "modifier":
				options = append(options, logiface.WithModifier[*testEvent](
					logiface.NewModifierFunc(func(*testEvent) error {
						goexit()
						return nil
					}),
				))
			case "writer":
				writer.onWrite = func(*testEvent) error {
					goexit()
					return nil
				}
			case "releaser":
				options = append(options, logiface.WithEventReleaser[*testEvent](
					logiface.NewEventReleaserFunc(func(*testEvent) { goexit() }),
				))
			}

			logger := logiface.New[*testEvent](options...)
			loop := New(WithLogger(logger.Logger()))
			registerLoopCleanupT(t, loop)
			returned := make(chan struct{})
			go func() {
				loop.logError("logger stage runtime.Goexit", nil)
				close(returned)
			}()
			waitContractSignal(t, returned, stage+" runtime.Goexit containment")
			if got := calls.Load(); got != 1 {
				t.Fatalf("%s calls = %d, want 1", stage, got)
			}
		})
	}
}

func TestLoggerEventAbnormalExitDoesNotTerminateCaller(t *testing.T) {
	exitCases := []struct {
		name string
		exit func()
	}{
		{name: "panic", exit: func() { panic("event method panic") }},
		{name: "goexit", exit: runtime.Goexit},
	}
	for _, exitCase := range exitCases {
		for _, stage := range []string{"AddString", "AddField", "AddError", "AddMessage"} {
			t.Run(exitCase.name+"/"+stage, func(t *testing.T) {
				var calls atomic.Int32
				var writes atomic.Int32
				logger := logiface.New[*abnormalExitLogEvent](
					logiface.WithEventFactory[*abnormalExitLogEvent](
						logiface.NewEventFactoryFunc(func(level logiface.Level) *abnormalExitLogEvent {
							return &abnormalExitLogEvent{
								level: level,
								stage: stage,
								exit: func() {
									calls.Add(1)
									exitCase.exit()
								},
							}
						}),
					),
					logiface.WithWriter[*abnormalExitLogEvent](logiface.NewWriterFunc(func(*abnormalExitLogEvent) error {
						writes.Add(1)
						return nil
					})),
				)
				loop := New(WithLogger(logger.Logger()))
				registerLoopCleanupT(t, loop)

				type callResult struct {
					panicValue any
					returned   bool
				}
				finished := make(chan callResult, 1)
				go func() {
					result := callResult{}
					defer func() {
						result.panicValue = recover()
						finished <- result
					}()
					loop.logCritical("event method abnormal exit", errors.New("diagnostic error"))
					result.returned = true
				}()
				result := waitContractValue(t, finished, exitCase.name+" from "+stage+" containment")
				if !result.returned {
					t.Fatalf("%s escaped %s: panic=%#v", stage, exitCase.name, result.panicValue)
				}
				if got := calls.Load(); got != 1 {
					t.Fatalf("%s calls = %d, want 1", stage, got)
				}
				if got := writes.Load(); got != 0 {
					t.Fatalf("writer calls after %s = %d, want 0", stage, got)
				}
			})
		}
	}
}

func TestLoggerModifierErrorDoesNotReenterLogger(t *testing.T) {
	sentinel := errors.New("injected modifier failure")
	var modifierCalls atomic.Int32
	var writerCalls atomic.Int32
	logger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](&testEventFactory{}),
		logiface.WithModifier[*testEvent](logiface.NewModifierFunc(func(*testEvent) error {
			modifierCalls.Add(1)
			return sentinel
		})),
		logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(*testEvent) error {
			writerCalls.Add(1)
			return nil
		})),
	)
	loop := New(WithLogger(logger.Logger()))
	registerLoopCleanupT(t, loop)
	returned := make(chan struct{})
	go func() {
		loop.logError("modifier failure", nil)
		close(returned)
	}()
	waitContractSignal(t, returned, "logger modifier error containment")
	if got := modifierCalls.Load(); got != 1 {
		t.Fatalf("modifier calls = %d, want 1", got)
	}
	if got := writerCalls.Load(); got != 0 {
		t.Fatalf("writer calls = %d, want 0", got)
	}
}

func TestLogCallbackErrorWithEnabledLogger(t *testing.T) {
	records := make(chan *testEvent, 1)
	panicValue := &struct{ Code int }{Code: 42}

	writer := &testEventWriter{
		onWrite: func(event *testEvent) error {
			records <- event
			return nil
		},
	}
	factory := &testEventFactory{}

	typedLogger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](factory),
		logiface.WithWriter[*testEvent](writer),
	)
	genericLogger := typedLogger.Logger()

	loop := New(WithLogger(genericLogger))
	registerLoopCleanupT(t, loop)

	loop.logCallbackError("test error message", panicValue)

	event := waitContractValue(t, records, "callback-error diagnostic")
	if event.level != logiface.LevelError {
		t.Fatalf("level = %v, want %v", event.level, logiface.LevelError)
	}
	if event.message != "test error message" {
		t.Fatalf("message = %q, want %q", event.message, "test error message")
	}
	if len(event.fields) != 2 {
		t.Fatalf("fields = %#v, want exactly component and panic", event.fields)
	}
	if got := event.fields["component"]; got != "eventloop" {
		t.Fatalf("component field = %#v, want eventloop", got)
	}
	if got := event.fields["panic"]; got != panicValue {
		t.Fatalf("panic field = %#v, want identity %p", got, panicValue)
	}
}

func TestLogUsesConfiguredLogger(t *testing.T) {
	records := make(chan *testEvent, 1)
	typedLogger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](&testEventFactory{}),
		logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(event *testEvent) error {
			records <- event
			return nil
		})),
	)

	loop := New(WithLogger(typedLogger.Logger()))
	registerLoopCleanupT(t, loop)

	loop.Log(logiface.LevelWarning, logiface.ModifierFunc[logiface.Event](func(event logiface.Event) error {
		addLogMessage(event, "public diagnostic")
		return nil
	}))

	event := waitContractValue(t, records, "public Log diagnostic")
	if event.level != logiface.LevelWarning {
		t.Fatalf("level = %v, want %v", event.level, logiface.LevelWarning)
	}
	if event.message != "public diagnostic" {
		t.Fatalf("message = %q, want public diagnostic", event.message)
	}
}

func TestLogNilLoopPanics(t *testing.T) {
	defer func() {
		if got := recover(); got != "eventloop: nil Loop" {
			t.Fatalf("panic = %#v, want %q", got, "eventloop: nil Loop")
		}
	}()
	var loop *Loop
	loop.Log(logiface.LevelError, nil)
}

func TestLogCallbackErrorWithPanickingLogger(t *testing.T) {
	called := false
	writer := &testEventWriter{
		onWrite: func(event *testEvent) error {
			called = true
			panic("logger panic")
		},
	}
	factory := &testEventFactory{}

	typedLogger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](factory),
		logiface.WithWriter[*testEvent](writer),
	)
	genericLogger := typedLogger.Logger()

	loop := New(WithLogger(genericLogger))
	registerLoopCleanupT(t, loop)

	// A panicking configured logger must not escape into loop control flow.
	loop.logCallbackError("test error with panic", "test panic value")
	if !called {
		t.Fatal("panicking logger writer was not invoked")
	}
}

func TestPromiseScheduleFailureUsesInstanceLogger(t *testing.T) {
	records := make(chan *testEvent, 1)
	logger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](&testEventFactory{}),
		logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(event *testEvent) error {
			records <- event
			return nil
		})),
	).Logger()
	loop := New(WithLogger(logger))
	registerLoopCleanupT(t, loop)

	expected := errors.New("schedule failed")
	promise := &ChainedPromise{js: &JS{loop: loop}}
	promise.handleHandlerScheduleFailure(handlerScheduleFailure{err: expected})

	event := waitContractValue(t, records, "promise schedule-failure diagnostic")
	if event.level != logiface.LevelError {
		t.Fatalf("level = %v, want %v", event.level, logiface.LevelError)
	}
	if event.message != "eventloop: failed to schedule promise handler microtask" {
		t.Fatalf("message = %q", event.message)
	}
	got, ok := event.fields["err"].(error)
	if !ok || !errors.Is(got, expected) {
		t.Fatalf("error field = %#v, want %v", event.fields["err"], expected)
	}
	if got := event.fields["component"]; got != "eventloop" {
		t.Fatalf("component field = %#v, want eventloop", got)
	}
}

func TestUnhandledRejectionDiagnosticUsesReasonField(t *testing.T) {
	records := make(chan *testEvent, 2)
	reported := make(chan any, 1)
	logger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](&testEventFactory{}),
		logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(event *testEvent) error {
			records <- event
			return nil
		})),
	).Logger()
	loop := New(WithLogger(logger))
	registerLoopCleanupT(t, loop)
	js := NewJS(loop,
		WithUnhandledRejection(func(reason any) { reported <- reason }),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackDisabled),
	)
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reason := struct{ Code int }{Code: 42}
	_, _, reject := js.NewChainedPromise()
	reject(reason)

	event := waitContractValue(t, records, "unhandled-rejection diagnostic")
	if event.level != logiface.LevelError {
		t.Fatalf("level = %v, want %v", event.level, logiface.LevelError)
	}
	if event.message != "eventloop: unhandled rejection after loop termination (fallback callback disabled)" {
		t.Fatalf("message = %q", event.message)
	}
	if len(event.fields) != 2 {
		t.Fatalf("fields = %#v, want exactly component and reason", event.fields)
	}
	if got := event.fields["component"]; got != "eventloop" {
		t.Fatalf("component field = %#v, want eventloop", got)
	}
	if got := event.fields["reason"]; got != reason {
		t.Fatalf("reason field = %#v, want %#v", got, reason)
	}
	select {
	case extra := <-records:
		t.Fatalf("unexpected additional diagnostic: level=%v message=%q fields=%#v", extra.level, extra.message, extra.fields)
	default:
	}
	select {
	case got := <-reported:
		t.Fatalf("disabled fallback invoked rejection handler with %#v", got)
	default:
	}
	js.rejectionsMu.RLock()
	rejectionCount := len(js.unhandledRejections)
	js.rejectionsMu.RUnlock()
	js.handlerReadyMu.Lock()
	readyCount := len(js.handlerReadyChans)
	js.handlerReadyMu.Unlock()
	if rejectionCount != 0 || readyCount != 0 ||
		js.checkRejectionScheduled.Load() || js.checkRejectionRunning.Load() ||
		js.checkRejectionRerun.Load() || js.checkRejectionFallbackRerun.Load() {
		t.Fatalf("rejection tracking retained state: rejections=%d ready=%d scheduled=%v running=%v rerun=%v fallbackRerun=%v",
			rejectionCount,
			readyCount,
			js.checkRejectionScheduled.Load(),
			js.checkRejectionRunning.Load(),
			js.checkRejectionRerun.Load(),
			js.checkRejectionFallbackRerun.Load(),
		)
	}
}
