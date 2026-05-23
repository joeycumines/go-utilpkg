package gojaeventloop

import (
	"testing"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func TestJSErrorTypes_ExistViaGoja(t *testing.T) {
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
		t.Fatalf("Bind adapter: %v", err)
	}

	for _, errorType := range []string{"Error", "TypeError", "RangeError", "ReferenceError", "SyntaxError", "URIError", "EvalError"} {
		t.Run(errorType, func(t *testing.T) {
			result, err := runtime.RunString(`typeof ` + errorType + ` === "function"`)
			if err != nil {
				t.Fatalf("inspect %s: %v", errorType, err)
			}
			if !result.ToBoolean() {
				t.Fatalf("%s is not a constructor", errorType)
			}
		})
	}
}

func TestJSErrorTypes_CanThrow(t *testing.T) {
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
		t.Fatalf("Bind adapter: %v", err)
	}

	for _, errorType := range []string{"Error", "TypeError", "RangeError", "ReferenceError", "SyntaxError", "URIError", "EvalError"} {
		t.Run(errorType, func(t *testing.T) {
			if err := runtime.Set("errorType", errorType); err != nil {
				t.Fatalf("set error type: %v", err)
			}
			result, err := runtime.RunString(`(() => {
				const Constructor = globalThis[errorType];
				try { throw new Constructor("test error"); }
				catch (error) { return error.name === errorType && error.message === "test error"; }
			})()`)
			if err != nil {
				t.Fatalf("throw %s: %v", errorType, err)
			}
			if !result.ToBoolean() {
				t.Fatalf("%s did not retain its name and message when thrown", errorType)
			}
		})
	}
}

func TestJSErrorTypes_Instanceof(t *testing.T) {
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
		t.Fatalf("Bind adapter: %v", err)
	}

	for _, errorType := range []string{"TypeError", "RangeError", "ReferenceError", "SyntaxError", "URIError", "EvalError"} {
		t.Run(errorType, func(t *testing.T) {
			if err := runtime.Set("errorType", errorType); err != nil {
				t.Fatalf("set error type: %v", err)
			}
			result, err := runtime.RunString(`(() => {
				const Constructor = globalThis[errorType];
				const error = new Constructor("test");
				return error instanceof Error && error instanceof Constructor;
			})()`)
			if err != nil {
				t.Fatalf("construct %s: %v", errorType, err)
			}
			if !result.ToBoolean() {
				t.Fatalf("%s does not inherit Error", errorType)
			}
		})
	}
}
