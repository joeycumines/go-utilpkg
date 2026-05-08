package gojaeventloop

import (
	"bytes"
	"context"
	"errors"
	"log"
	"runtime"
	"sync"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	"github.com/joeycumines/logiface"
)

func captureStandardLog(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	oldWriter := log.Writer()
	oldFlags := log.Flags()
	oldPrefix := log.Prefix()

	log.SetOutput(&buf)
	log.SetFlags(0)
	log.SetPrefix("")

	t.Cleanup(func() {
		log.SetOutput(oldWriter)
		log.SetFlags(oldFlags)
		log.SetPrefix(oldPrefix)
	})

	return &buf
}

func newDisabledDiagnosticLoop(t *testing.T) *goeventloop.Loop {
	t.Helper()
	logger := logiface.New[*adapterDiagnosticLogEvent](
		logiface.WithEventFactory[*adapterDiagnosticLogEvent](adapterDiagnosticLogFactory{}),
	).Logger()
	loop := goeventloop.New(goeventloop.WithLogger(logger))
	return loop
}

func newPanickingDiagnosticLoop(t *testing.T) *goeventloop.Loop {
	t.Helper()
	logger := logiface.New[*adapterDiagnosticLogEvent](
		logiface.WithEventFactory[*adapterDiagnosticLogEvent](adapterDiagnosticLogFactory{}),
		logiface.WithWriter[*adapterDiagnosticLogEvent](logiface.NewWriterFunc(func(*adapterDiagnosticLogEvent) error {
			panic("diagnostic logger panic")
		})),
	).Logger()
	loop := goeventloop.New(goeventloop.WithLogger(logger))
	return loop
}

func newGoexitDiagnosticLoop(t *testing.T) *goeventloop.Loop {
	t.Helper()
	logger := logiface.New[*adapterDiagnosticLogEvent](
		logiface.WithEventFactory[*adapterDiagnosticLogEvent](adapterDiagnosticLogFactory{}),
		logiface.WithWriter[*adapterDiagnosticLogEvent](logiface.NewWriterFunc(func(*adapterDiagnosticLogEvent) error {
			runtime.Goexit()
			return nil
		})),
	).Logger()
	loop := goeventloop.New(goeventloop.WithLogger(logger))
	return loop
}

func assertStandardLogUnused(t *testing.T, buf *bytes.Buffer) {
	t.Helper()
	if got := buf.String(); got != "" {
		t.Fatalf("adapter diagnostic wrote to process-global log: %q", got)
	}
}

func TestPromiseJobDiagnosticDropsWithoutLogger(t *testing.T) {
	buf := captureStandardLog(t)

	loop := goeventloop.New()
	defer loop.Close()

	reportPromiseJobError(loop, errors.New("boom"))
	assertStandardLogUnused(t, buf)
}

func TestPromiseJobDiagnosticDropsWithDisabledLogger(t *testing.T) {
	buf := captureStandardLog(t)

	loop := newDisabledDiagnosticLoop(t)
	defer loop.Close()

	reportPromiseJobError(loop, errors.New("disabled boom"))
	assertStandardLogUnused(t, buf)
}

func TestPromiseJobDiagnosticContainsStructuredLoggerPanic(t *testing.T) {
	buf := captureStandardLog(t)

	loop := newPanickingDiagnosticLoop(t)
	defer loop.Close()

	reportPromiseJobError(loop, errors.New("panic boom"))
	assertStandardLogUnused(t, buf)
}

func TestPromiseJobDiagnosticContainsStructuredLoggerGoexit(t *testing.T) {
	buf := captureStandardLog(t)
	loop := newGoexitDiagnosticLoop(t)
	defer loop.Close()

	reportPromiseJobError(loop, errors.New("goexit boom"))
	assertStandardLogUnused(t, buf)
}

func TestHostCallbackDiagnosticDropsWithoutLogger(t *testing.T) {
	buf := captureStandardLog(t)

	loop := goeventloop.New()
	defer loop.Close()

	adapter, err := New(loop, goja.New())
	if err != nil {
		t.Fatal(err)
	}

	adapter.reportHostCallbackError("setTimeout", errors.New("callback failed"))
	assertStandardLogUnused(t, buf)
}

func TestHostCallbackDiagnosticDropsWithDisabledLogger(t *testing.T) {
	buf := captureStandardLog(t)

	loop := newDisabledDiagnosticLoop(t)
	defer loop.Close()

	adapter, err := New(loop, goja.New())
	if err != nil {
		t.Fatal(err)
	}

	adapter.reportHostCallbackError("queueMicrotask", errors.New("disabled callback failed"))
	assertStandardLogUnused(t, buf)
}

func TestHostCallbackDiagnosticContainsStructuredLoggerPanic(t *testing.T) {
	buf := captureStandardLog(t)

	loop := newPanickingDiagnosticLoop(t)
	defer loop.Close()

	adapter, err := New(loop, goja.New())
	if err != nil {
		t.Fatal(err)
	}

	adapter.reportHostCallbackError("setImmediate", errors.New("panic callback failed"))
	assertStandardLogUnused(t, buf)
}

func TestHostCallbackDiagnosticContainsStructuredLoggerGoexit(t *testing.T) {
	buf := captureStandardLog(t)
	loop := newGoexitDiagnosticLoop(t)
	defer loop.Close()

	adapter, err := New(loop, goja.New())
	if err != nil {
		t.Fatal(err)
	}

	adapter.reportHostCallbackError("setImmediate", errors.New("goexit callback failed"))
	assertStandardLogUnused(t, buf)
}

func TestHostCallbackLoggerGoexitDoesNotSuppressFatalExit(t *testing.T) {
	loop := newGoexitDiagnosticLoop(t)
	defer loop.Close()
	adapter, err := New(loop, goja.New())
	if err != nil {
		t.Fatal(err)
	}

	adapter.handleHostCallbackError("setTimeout", errors.New("fatal callback failed"), "uncaughtException")

	if !adapter.exiting.Load() {
		t.Fatal("adapter did not enter exiting state after logger runtime.Goexit")
	}
	if got := adapter.currentExitCode(); got != 1 {
		t.Fatalf("exit code = %d, want 1", got)
	}
}

func TestHostCallbackLoggerReentrantShutdownRetainsLoopRole(t *testing.T) {
	var loop *goeventloop.Loop
	shutdownCtx, cancelShutdown := context.WithCancel(context.Background())
	defer cancelShutdown()
	shutdownResult := make(chan error, 1)
	var shutdownOnce sync.Once
	logger := logiface.New[*adapterDiagnosticLogEvent](
		logiface.WithEventFactory[*adapterDiagnosticLogEvent](adapterDiagnosticLogFactory{}),
		logiface.WithWriter[*adapterDiagnosticLogEvent](logiface.NewWriterFunc(func(*adapterDiagnosticLogEvent) error {
			shutdownOnce.Do(func() {
				shutdownResult <- loop.Shutdown(shutdownCtx)
			})
			return nil
		})),
	).Logger()
	loop = goeventloop.New(goeventloop.WithLogger(logger))
	adapter, err := New(loop, goja.New())
	if err != nil {
		t.Fatal(err)
	}

	callbackDone := make(chan struct{})
	if err := loop.Submit(func() {
		adapter.handleHostCallbackError("setImmediate", errors.New("fatal callback failed"), "uncaughtException")
		close(callbackDone)
	}); err != nil {
		t.Fatal(err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	select {
	case err := <-shutdownResult:
		if err != nil {
			t.Fatalf("logger Shutdown = %v, want nil", err)
		}
	case <-time.After(time.Second):
		cancelShutdown()
		err := <-shutdownResult
		t.Fatalf("logger Shutdown joined its interrupted callback: %v", err)
	}
	select {
	case <-callbackDone:
	case <-time.After(time.Second):
		t.Fatal("host callback did not continue after logger Shutdown")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not finish after logger-initiated Shutdown")
	}
	if got := adapter.currentExitCode(); got != 1 {
		t.Fatalf("exit code = %d, want 1", got)
	}
}
