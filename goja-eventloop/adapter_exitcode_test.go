package gojaeventloop

import (
	"context"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// runLoopToQuiescence runs loop until auto-exit returns and requires a nil
// error, so a failed Run cannot be mistaken for clean quiescence.
func runLoopToQuiescence(t *testing.T, loop *goeventloop.Loop) {
	t.Helper()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after natural quiescence")
	}
}

func TestAdapterExitCodeUnset(t *testing.T) {
	loop, err := goeventloop.New(goeventloop.WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loop.Close() })
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	code, ok := adapter.ExitCode()
	if ok {
		t.Fatalf("ExitCode before Bind = (%d, true), want ok false", code)
	}
	if code != 0 {
		t.Fatalf("ExitCode unset code = %d, want 0", code)
	}
}

func TestAdapterExitCodeUnsetAfterBind(t *testing.T) {
	_, _, _, adapter := newAutoExitAdapter(t)

	code, ok := adapter.ExitCode()
	if ok {
		t.Fatalf("ExitCode after Bind = (%d, true), want ok false", code)
	}
	if code != 0 {
		t.Fatalf("ExitCode unset code = %d, want 0", code)
	}
}

func TestAdapterExitCodeFromProcessExit(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loop.Close() })
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}

	_, err = runtime.RunString(`process.exit(7);`)
	if code, ok := processExitCode(err); !ok || code != 7 {
		t.Fatalf("RunString error = %v, want process exit signal code 7", err)
	}
	runtime.ClearInterrupt()

	code, ok := adapter.ExitCode()
	if !ok || code != 7 {
		t.Fatalf("ExitCode after process.exit(7) = (%d, %t), want (7, true)", code, ok)
	}
}

func TestAdapterExitCodeFromAssignedCode(t *testing.T) {
	_, loop, runtime, adapter := newAutoExitAdapter(t)

	if _, err := runtime.RunString(`process.exitCode = 3;`); err != nil {
		t.Fatalf("RunString: %v", err)
	}
	runLoopToQuiescence(t, loop)

	code, ok := adapter.ExitCode()
	if !ok || code != 3 {
		t.Fatalf("ExitCode after process.exitCode = 3 = (%d, %t), want (3, true)", code, ok)
	}
}

func TestAdapterExitCodeUnsetOnQuiescence(t *testing.T) {
	_, loop, runtime, adapter := newAutoExitAdapter(t)

	if _, err := runtime.RunString(`globalThis.settled = true;`); err != nil {
		t.Fatalf("RunString: %v", err)
	}
	runLoopToQuiescence(t, loop)

	code, ok := adapter.ExitCode()
	if ok {
		t.Fatalf("ExitCode after clean quiescence = (%d, true), want ok false", code)
	}
	if code != 0 {
		t.Fatalf("ExitCode clean-quiescence code = %d, want 0", code)
	}
}

func TestAdapterExitCodePanicsOnCopiedAdapter(t *testing.T) {
	_, _, _, adapter := newAutoExitAdapter(t)

	copied := copyAdapterValue(adapter)
	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("ExitCode on a copied Adapter did not panic")
			}
			const want = "goja-eventloop: ExitCode requires the adapter returned by New"
			if r != want {
				t.Fatalf("panic = %v, want %q", r, want)
			}
		}()
		copied.ExitCode()
	}()
}

func TestAdapterExitCodeNilAdapterPanics(t *testing.T) {
	var adapter *Adapter
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("ExitCode on a nil Adapter did not panic")
			}
		}()
		adapter.ExitCode()
	}()
}

func TestAdapterExitCodeReadableAfterLoopTermination(t *testing.T) {
	_, loop, runtime, adapter := newAutoExitAdapter(t)

	if _, err := runtime.RunString(`process.exitCode = 9;`); err != nil {
		t.Fatalf("RunString: %v", err)
	}
	runLoopToQuiescence(t, loop)

	// Settled exit codes stay readable from any goroutine.
	code, ok := adapter.ExitCode()
	if !ok || code != 9 {
		t.Fatalf("ExitCode after loop termination = (%d, %t), want (9, true)", code, ok)
	}
}
