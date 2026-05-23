package gojaeventloop

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	"github.com/joeycumines/logiface"
)

type adapterDiagnosticLogEvent struct {
	logiface.UnimplementedEvent
	level   logiface.Level
	message string
	err     error
	fields  map[string]any
}

func (e *adapterDiagnosticLogEvent) Level() logiface.Level { return e.level }

func (e *adapterDiagnosticLogEvent) AddField(key string, val any) {
	if e.fields == nil {
		e.fields = make(map[string]any)
	}
	e.fields[key] = val
}

func (e *adapterDiagnosticLogEvent) AddMessage(msg string) bool {
	e.message = msg
	return true
}

func (e *adapterDiagnosticLogEvent) AddError(err error) bool {
	e.err = err
	return true
}

func (e *adapterDiagnosticLogEvent) AddString(key string, val string) bool {
	e.AddField(key, val)
	return true
}

type adapterDiagnosticLogFactory struct{}

func (adapterDiagnosticLogFactory) NewEvent(level logiface.Level) *adapterDiagnosticLogEvent {
	return &adapterDiagnosticLogEvent{level: level}
}

type adapterDiagnosticLogRecord struct {
	level     logiface.Level
	message   string
	err       error
	component string
	callback  string
}

func newAdapterDiagnosticLoggedLoop(t *testing.T) (*goeventloop.Loop, <-chan adapterDiagnosticLogRecord) {
	t.Helper()
	records := make(chan adapterDiagnosticLogRecord, 16)
	logger := logiface.New[*adapterDiagnosticLogEvent](
		logiface.WithEventFactory[*adapterDiagnosticLogEvent](adapterDiagnosticLogFactory{}),
		logiface.WithWriter[*adapterDiagnosticLogEvent](logiface.NewWriterFunc(func(event *adapterDiagnosticLogEvent) error {
			record := adapterDiagnosticLogRecord{
				level:   event.level,
				message: event.message,
				err:     event.err,
			}
			if component, ok := event.fields["component"].(string); ok {
				record.component = component
			}
			if callback, ok := event.fields["callback"].(string); ok {
				record.callback = callback
			}
			records <- record
			return nil
		})),
	).Logger()
	loop, err := goeventloop.New(goeventloop.WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}
	return loop, records
}

func receiveAdapterDiagnosticLog(t *testing.T, records <-chan adapterDiagnosticLogRecord) adapterDiagnosticLogRecord {
	t.Helper()
	select {
	case record := <-records:
		return record
	case <-time.After(time.Second):
		t.Fatal("adapter diagnostic was not logged")
	}
	panic("unreachable")
}

func TestPromiseJobEnqueuerReportsScheduleMicrotaskError(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	reported := make(chan error, 1)
	enqueue := newPromiseJobEnqueuer(loop, goja.New(), func(err error) {
		reported <- err
	})

	enqueue(func() {
		t.Fatal("promise job should not run when ScheduleMicrotask fails")
	})

	select {
	case err := <-reported:
		if !errors.Is(err, goeventloop.ErrLoopTerminated) {
			t.Fatalf("reported error = %v, want ErrLoopTerminated", err)
		}
		if !strings.Contains(err.Error(), "enqueue promise job") {
			t.Fatalf("reported error = %v, want enqueue context", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ScheduleMicrotask failure was not reported")
	}
}

func TestPromiseJobEnqueuerTerminalGateDropsJobs(t *testing.T) {
	for _, test := range []struct {
		name    string
		exiting bool
	}{
		{name: "exit already published", exiting: true},
		{name: "terminal scheduling race", exiting: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			loop, err := goeventloop.New()
			if err != nil {
				t.Fatal(err)
			}
			if err := loop.Close(); err != nil {
				t.Fatal(err)
			}
			var ran atomic.Bool
			var reported atomic.Int32
			enqueue := newPromiseJobEnqueuerWithGate(
				loop,
				goja.New(),
				func(error) { reported.Add(1) },
				func() bool { return test.exiting },
			)

			enqueue(func() { ran.Store(true) })

			if ran.Load() {
				t.Fatal("terminal promise job ran")
			}
			if got := reported.Load(); got != 0 {
				t.Fatalf("terminal promise-job reports = %d, want 0", got)
			}
			if loop.Alive() {
				t.Fatal("terminal promise job added liveness")
			}
		})
	}
}

func TestPromiseJobEnqueuerDefaultReporterUsesLoopLogger(t *testing.T) {
	loop, records := newAdapterDiagnosticLoggedLoop(t)
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	enqueue := newPromiseJobEnqueuer(loop, goja.New(), nil)
	enqueue(func() {
		t.Fatal("promise job should not run when ScheduleMicrotask fails")
	})

	record := receiveAdapterDiagnosticLog(t, records)
	if record.level != logiface.LevelError {
		t.Fatalf("logged level = %v, want %v", record.level, logiface.LevelError)
	}
	if record.message != "goja native promise job failed" {
		t.Fatalf("logged message = %q, want promise-job message", record.message)
	}
	if record.component != "goja-eventloop" {
		t.Fatalf("logged component = %q, want goja-eventloop", record.component)
	}
	if !errors.Is(record.err, goeventloop.ErrLoopTerminated) {
		t.Fatalf("logged error = %v, want ErrLoopTerminated", record.err)
	}
	if !strings.Contains(record.err.Error(), "enqueue promise job") {
		t.Fatalf("logged error = %v, want enqueue context", record.err)
	}
}

func TestPromiseJobEnqueuerReportsRunPromiseJobError(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}

	runtime := goja.New()
	reported := make(chan error, 1)
	runtime.SetPromiseJobEnqueuer(newPromiseJobEnqueuer(loop, runtime, func(err error) {
		reported <- err
	}))

	var reached atomic.Bool
	if err := runtime.Set("nativeHandler", func() {
		reached.Store(true)
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := runtime.RunString(`Promise.resolve().then(nativeHandler)`); err != nil {
		t.Fatal(err)
	}
	runtime.Interrupt("test-interrupt")

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		_ = loop.Run(ctx)
	}()

	select {
	case err := <-reported:
		if err == nil {
			t.Fatal("reported nil error")
		}
		if !strings.Contains(err.Error(), "run promise job") {
			t.Fatalf("reported error = %v, want run context", err)
		}
		if !strings.Contains(err.Error(), "test-interrupt") {
			t.Fatalf("reported error = %v, want interrupt reason", err)
		}
	case <-time.After(3 * time.Second):
		cancel()
		_ = loop.Shutdown(context.Background())
		t.Fatal("RunPromiseJob failure was not reported")
	}

	if reached.Load() {
		t.Fatal("native promise handler ran despite pending interrupt")
	}

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()
	_ = loop.Shutdown(shutdownCtx)

	select {
	case <-runDone:
	case <-time.After(time.Second):
		t.Fatal("loop did not stop")
	}
}

func TestPromiseJobErrorReporterUsesLoopLogger(t *testing.T) {
	loop, records := newAdapterDiagnosticLoggedLoop(t)
	defer loop.Shutdown(context.Background())

	adapter, err := New(loop, goja.New())
	if err != nil {
		t.Fatal(err)
	}

	var console bytes.Buffer
	adapter.SetConsoleOutput(&console)
	adapter.reportPromiseJobError(errors.New("boom"))

	record := receiveAdapterDiagnosticLog(t, records)
	if record.level != logiface.LevelError {
		t.Fatalf("logged level = %v, want %v", record.level, logiface.LevelError)
	}
	if record.message != "goja native promise job failed" {
		t.Fatalf("logged message = %q, want promise-job message", record.message)
	}
	if record.component != "goja-eventloop" {
		t.Fatalf("logged component = %q, want goja-eventloop", record.component)
	}
	if record.err == nil || record.err.Error() != "boom" {
		t.Fatalf("logged error = %v, want boom", record.err)
	}
	if console.Len() != 0 {
		t.Fatalf("promise-job diagnostic wrote to console output: %q", console.String())
	}

	adapter.SetConsoleOutput(nil)
	adapter.reportPromiseJobError(errors.New("still logged"))
	record = receiveAdapterDiagnosticLog(t, records)
	if record.err == nil || record.err.Error() != "still logged" {
		t.Fatalf("logged error after SetConsoleOutput(nil) = %v, want still logged", record.err)
	}
}
