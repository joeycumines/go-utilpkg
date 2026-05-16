package gojaeventloop

import (
	"context"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func newCapturedErrorBoundaryAdapter(t *testing.T, loop *goeventloop.Loop) (*Adapter, *goja.Runtime) {
	t.Helper()
	runtime := goja.New()
	_, err := runtime.RunString(`
		const NativeError = Error;
		globalThis.NativeError = NativeError;
		globalThis.capturedErrorThrows = false;
		globalThis.capturedErrorCalls = 0;
		globalThis.capturedErrorCoercions = 0;
		globalThis.capturedErrorThrown = {
			[Symbol.toPrimitive]() {
				capturedErrorCoercions++;
				throw new NativeError("captured Error exception was coerced");
			}
		};
		function CapturedError(...args) {
			capturedErrorCalls++;
			if (capturedErrorThrows) throw capturedErrorThrown;
			return Reflect.construct(NativeError, args, NativeError);
		}
		Object.defineProperty(CapturedError, "name", { value: "Error" });
		CapturedError.prototype = NativeError.prototype;
		globalThis.Error = CapturedError;
	`)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	return adapter, runtime
}

func TestRuntimePrimordialsPreserveGlobalErrorReplacement(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loop.Close() })
	_, runtime := newCapturedErrorBoundaryAdapter(t, loop)
	value, err := runtime.RunString(`
		capturedErrorThrows = true;
		const exception = new DOMException("sample", "AbortError");
		[
			Error === CapturedError,
			exception instanceof NativeError,
			Object.getPrototypeOf(DOMException.prototype) === NativeError.prototype,
			capturedErrorCalls,
			capturedErrorCoercions,
		].join(":");
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := value.String(), "true:true:true:0:0"; got != want {
		t.Fatalf("intrinsic Error boundary = %q, want %q", got, want)
	}
}

func TestAdapterSubmitJavaScriptExceptionUsesAdapterBoundary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	loop, records := newAdapterDiagnosticLoggedLoop(t)
	adapter, runtime := newCapturedErrorBoundaryAdapter(t, loop)
	done := make(chan string, 1)
	if err := runtime.Set("testDone", func(value string) { done <- value }); err != nil {
		t.Fatal(err)
	}
	_, err := runtime.RunString(`
		const submitTarget = {};
		Object.defineProperty(submitTarget, "value", {
			get() { throw capturedErrorThrown; }
		});
		process.on("uncaughtException", function(error, origin) {
			testDone([error === capturedErrorThrown, origin, capturedErrorCoercions].join(":"));
		});
	`)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Submit(func(runtime *goja.Runtime) {
		runtime.Get("submitTarget").ToObject(runtime).Get("value")
	}); err != nil {
		t.Fatal(err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	select {
	case got := <-done:
		if want := "true:uncaughtException:0"; got != want {
			t.Fatalf("Adapter.Submit exception boundary = %q, want %q", got, want)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for Adapter.Submit exception")
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Run did not return")
	}
	if got := runtime.Get("capturedErrorCoercions").ToInteger(); got != 0 {
		t.Fatalf("Adapter.Submit exception was coerced %d times", got)
	}
	select {
	case record := <-records:
		t.Fatalf("unexpected raw core or adapter diagnostic: %#v", record)
	default:
	}
}

func TestProcessExitThrowingExitingSetterPreservesExactException(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, runtime := newCapturedErrorBoundaryAdapter(t, loop)
	value, err := runtime.RunString(`
		capturedErrorThrows = true;
		Object.defineProperty(process, "_exiting", {
			configurable: true,
			get() { return false; },
			set() { throw capturedErrorThrown; }
		});
		let caught;
		try { process.exit(23); } catch (error) { caught = error; }
		[caught === capturedErrorThrown, capturedErrorCoercions, process.exitCode,
		 process._exiting].join(":");
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := value.String(), "true:0:23:false"; got != want {
		t.Fatalf("process.exit setter exception = %q, want %q", got, want)
	}
	if adapter.exiting.Load() {
		t.Fatal("failed process.exit marked adapter exiting")
	}
	if adapter.exitEmitted.Load() {
		t.Fatal("failed process.exit marked exit emitted")
	}
	if got := loop.State(); got != goeventloop.StateAwake {
		t.Fatalf("failed process.exit loop state = %s, want awake", got)
	}
}
