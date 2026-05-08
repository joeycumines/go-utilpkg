package gojaeventloop

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/joeycumines/goja"
)

func TestRuntimeExceptionErrorDoesNotUnwrapOffOwner(t *testing.T) {
	runtime := goja.New()
	sentinel := errors.New("sentinel")
	thrown := runtime.NewGoError(sentinel)
	var valueReads atomic.Int32
	getter := runtime.ToValue(func(goja.FunctionCall) goja.Value {
		valueReads.Add(1)
		return runtime.ToValue(sentinel)
	})
	if err := thrown.DefineAccessorProperty("value", getter, nil, goja.FLAG_TRUE, goja.FLAG_FALSE); err != nil {
		t.Fatal(err)
	}

	exception := runtime.Try(func() { panic(thrown) })
	if exception == nil || exception.Value() != thrown {
		t.Fatal("runtime did not preserve the exact thrown GoError")
	}
	if _, ok := processExitCode(exception); ok {
		t.Fatal("Goja exception was mistaken for the native process-exit sentinel")
	}
	if got := valueReads.Load(); got != 0 {
		t.Fatalf("processExitCode read Goja exception value %d times", got)
	}
	err := wrapRuntimeException("test operation", exception)
	if got := err.Error(); got != "goja-eventloop: test operation: JavaScript exception" {
		t.Fatalf("Error() = %q", got)
	}
	var extracted *goja.Exception
	if !errors.As(err, &extracted) || extracted != exception {
		t.Fatal("errors.As did not preserve the exact Goja exception")
	}

	result := make(chan bool, 1)
	go func() { result <- errors.Is(err, sentinel) }()
	if <-result {
		t.Fatal("runtime exception wrapper unexpectedly unwraps the thrown Go value")
	}
	if got := valueReads.Load(); got != 0 {
		t.Fatalf("errors.Is read Goja exception value %d times off-owner", got)
	}
}

func TestOwnerAPIInternalDataPropertiesBypassPrototypeSetters(t *testing.T) {
	tests := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name: "invalid callback error code",
			script: `
				let setterCalls = 0;
				Object.defineProperty(TypeError.prototype, "code", {
					set() { setterCalls++; throw new Error("prototype setter called"); }, configurable: true
				});
				let error;
				try { queueMicrotask(1); } catch (caught) { error = caught; }
				const descriptor = Object.getOwnPropertyDescriptor(error, "code");
				[error.name, error.code, setterCalls, descriptor.writable,
				 descriptor.enumerable, descriptor.configurable].join(":");
			`,
			expected: "TypeError:ERR_INVALID_ARG_TYPE:0:true:true:true",
		},
		{
			name: "Performance toJSON result",
			script: `
				let setterCalls = 0;
				Object.defineProperty(Object.prototype, "timeOrigin", {
					set() { setterCalls++; throw new Error("prototype setter called"); }, configurable: true
				});
				const result = performance.toJSON();
				const descriptor = Object.getOwnPropertyDescriptor(result, "timeOrigin");
				[setterCalls, typeof descriptor.value, descriptor.writable,
				 descriptor.enumerable, descriptor.configurable].join(":");
			`,
			expected: "0:number:true:true:true",
		},
		{
			name: "process exit type error code",
			script: `
				let setterCalls = 0;
				Object.defineProperty(TypeError.prototype, "code", {
					set() { setterCalls++; throw new Error("prototype setter called"); }, configurable: true
				});
				let error;
				try { process.exit("invalid"); } catch (caught) { error = caught; }
				const descriptor = Object.getOwnPropertyDescriptor(error, "code");
				[error.name, error.code, setterCalls, descriptor.writable,
				 descriptor.enumerable, descriptor.configurable].join(":");
			`,
			expected: "TypeError:ERR_INVALID_ARG_TYPE:0:true:true:true",
		},
		{
			name: "process exit range error code",
			script: `
				let setterCalls = 0;
				Object.defineProperty(RangeError.prototype, "code", {
					set() { setterCalls++; throw new Error("prototype setter called"); }, configurable: true
				});
				let error;
				try { process.exit(1.5); } catch (caught) { error = caught; }
				const descriptor = Object.getOwnPropertyDescriptor(error, "code");
				[error.name, error.code, setterCalls, descriptor.writable,
				 descriptor.enumerable, descriptor.configurable].join(":");
			`,
			expected: "RangeError:ERR_OUT_OF_RANGE:0:true:true:true",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := newBoundAdapterForNode26Test(t)
			value, err := adapter.runtime.RunString(test.script)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := value.String(), test.expected; got != want {
				t.Fatalf("owner API exception result = %q, want %q", got, want)
			}
		})
	}
}
