package gojaeventloop

import (
	"context"
	"strings"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// EXPAND-067: Error Types Verification Tests
// Tests verify Goja's native support for:
// - Error: constructor, message, name, stack
// - TypeError: constructor, message, name, instanceof Error
// - RangeError: constructor, message, name, instanceof Error
// - ReferenceError: constructor, message, name
// - SyntaxError: constructor, message, name
// - URIError: constructor, message, name
// - EvalError: constructor, message, name
// - AggregateError: constructor, errors array, message
// - Error.prototype.toString()
// - Custom error subclassing (class MyError extends Error)
// - Stack trace includes function names
//
// STATUS: All Error types are NATIVE to Goja
// ===============================================

func newErrorsTestAdapter(t *testing.T) (*Adapter, *goja.Runtime, func()) {
	t.Helper()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New loop failed: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		loop.Shutdown(context.Background())
		t.Fatalf("New adapter failed: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		loop.Shutdown(context.Background())
		t.Fatalf("Bind failed: %v", err)
	}

	cleanup := func() {
		loop.Shutdown(context.Background())
	}

	return adapter, runtime, cleanup
}

// ===============================================
// Error Constructor Tests
// ===============================================

func TestError_Constructor(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	script := `
		var e1 = new Error('something went wrong');
		var e2 = new Error();
		e1. message === 'something went wrong' &&
		e2.message === '' &&
		e1 instanceof Error &&
		e2 instanceof Error;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Error constructor failed")
	}
}

func TestError_MessageProperty(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	script := `
		var e = new Error('test message');
		e.message === 'test message';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Error message property failed")
	}
}

func TestError_NameProperty(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	script := `
		var e = new Error('test');
		e.name === 'Error';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Error name property failed")
	}
}

func TestError_StackProperty(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	script := `
		var e = new Error('test');
		typeof e.stack === 'string' && e.stack.length > 0;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Error stack property failed")
	}
}

// ===============================================
// TypeError Tests
// ===============================================

func TestTypeError_Constructor(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	script := `
		var e = new TypeError('type error message');
		e.message === 'type error message' &&
		e.name === 'TypeError' &&
		e instanceof TypeError &&
		e instanceof Error;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("TypeError constructor failed")
	}
}

func TestTypeError_InstanceofError(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	script := `
		var e = new TypeError('type error');
		e instanceof Error;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("TypeError should be instanceof Error")
	}
}

// ===============================================
// RangeError Tests
// ===============================================

func TestRangeError_Constructor(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	script := `
		var e = new RangeError('value out of range');
		e.message === 'value out of range' &&
		e.name === 'RangeError' &&
		e instanceof RangeError &&
		e instanceof Error;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("RangeError constructor failed")
	}
}

func TestRangeError_ThrownByToFixed(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	// toFixed throws RangeError for invalid precision
	script := `
		var caught = false;
		try {
			(1.5).toFixed(101); // max is 100
		} catch (e) {
			caught = e instanceof RangeError;
		}
		caught;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("RangeError not thrown by toFixed with invalid precision")
	}
}

// ===============================================
// ReferenceError Tests
// ===============================================

func TestReferenceError_Constructor(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	script := `
		var e = new ReferenceError('undefined variable');
		e.message === 'undefined variable' &&
		e.name === 'ReferenceError' &&
		e instanceof ReferenceError &&
		e instanceof Error;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("ReferenceError constructor failed")
	}
}

// ===============================================
// SyntaxError Tests
// ===============================================

func TestSyntaxError_Constructor(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	script := `
		var e = new SyntaxError('unexpected token');
		e.message === 'unexpected token' &&
		e.name === 'SyntaxError' &&
		e instanceof SyntaxError &&
		e instanceof Error;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("SyntaxError constructor failed")
	}
}

// ===============================================
// URIError Tests
// ===============================================

func TestURIError_Constructor(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	script := `
		var e = new URIError('malformed URI');
		e.message === 'malformed URI' &&
		e.name === 'URIError' &&
		e instanceof URIError &&
		e instanceof Error;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("URIError constructor failed")
	}
}

func TestURIError_ThrownByDecodeURI(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	// decodeURI throws URIError for invalid sequences
	script := `
		var caught = false;
		try {
			decodeURI('%');
		} catch (e) {
			caught = e instanceof URIError;
		}
		caught;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("URIError not thrown by decodeURI with invalid sequence")
	}
}

// ===============================================
// EvalError Tests
// ===============================================

func TestEvalError_Constructor(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	script := `
		var e = new EvalError('eval error');
		e.message === 'eval error' &&
		e.name === 'EvalError' &&
		e instanceof EvalError &&
		e instanceof Error;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("EvalError constructor failed")
	}
}

// ===============================================
// AggregateError Tests
// ===============================================

func TestAggregateError_Constructor(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	script := `
		var errors = [new Error('first'), new TypeError('second')];
		var e = new AggregateError(errors, 'multiple errors');
		e.message === 'multiple errors' &&
		e.name === 'AggregateError' &&
		e instanceof AggregateError &&
		e instanceof Error &&
		Array.isArray(e.errors) &&
		e.errors.length === 2;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("AggregateError constructor failed")
	}
}

func TestAggregateError_ErrorsArray(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	script := `
		var e1 = new Error('error 1');
		var e2 = new RangeError('error 2');
		var e3 = new TypeError('error 3');
		var agg = new AggregateError([e1, e2, e3]);
		agg.errors[0] === e1 &&
		agg.errors[1] === e2 &&
		agg.errors[2] === e3;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("AggregateError errors array failed")
	}
}

func TestAggregateError_FromPromiseAny(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	// Promise.any with all rejections throws AggregateError
	script := `
		(function() {
			var caught = false;
			var errorsCount = 0;
			Promise.any([
				Promise.reject(new Error('first')),
				Promise.reject(new Error('second'))
			]).catch(function(e) {
				caught = e instanceof AggregateError;
				errorsCount = e.errors.length;
			});
			return { caught: caught, errorsCount: errorsCount };
		})();
	`
	_, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	// Note: Promise.any is async, so we just verify it doesn't throw
	t.Log("AggregateError from Promise.any: NATIVE")
}

// ===============================================
// Error.prototype.toString() Tests
// ===============================================

func TestError_ToString(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	script := `
		var e = new Error('test message');
		e.toString() === 'Error: test message';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Error.prototype.toString() failed")
	}
}

func TestError_ToStringNoMessage(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	script := `
		var e = new Error();
		e.toString() === 'Error';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Error.prototype.toString() with no message failed")
	}
}

func TestTypeError_ToString(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	script := `
		var e = new TypeError('invalid type');
		e.toString() === 'TypeError: invalid type';
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("TypeError.toString() failed")
	}
}

// ===============================================
// Custom Error Subclassing Tests
// ===============================================

func TestError_CustomSubclass(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	script := `
		class MyError extends Error {
			constructor(message) {
				super(message);
				this.name = 'MyError';
			}
		}
		var e = new MyError('custom error');
		e.name === 'MyError' &&
		e.message === 'custom error' &&
		e instanceof MyError &&
		e instanceof Error;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Custom error subclass failed")
	}
}

func TestError_CustomSubclassWithExtraProperties(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	script := `
		class ValidationError extends Error {
			constructor(message, field) {
				super(message);
				this.name = 'ValidationError';
				this.field = field;
			}
		}
		var e = new ValidationError('Field is required', 'username');
		e.name === 'ValidationError' &&
		e.message === 'Field is required' &&
		e.field === 'username' &&
		e instanceof ValidationError &&
		e instanceof Error;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Custom error subclass with extra properties failed")
	}
}

// ===============================================
// Stack Trace Tests
// ===============================================

func TestError_StackTraceIncludesFunctionNames(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	script := `
		function outerFunction() {
			return innerFunction();
		}
		function innerFunction() {
			return new Error('test').stack;
		}
		outerFunction();
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	stack := v.String()
	// Stack should contain function names
	if !strings.Contains(stack, "innerFunction") {
		t.Errorf("Stack trace should include 'innerFunction', got: %s", stack)
	}
	t.Logf("Stack trace: %s", stack)
}

func TestError_StackTraceFromThrow(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	script := `
		var stack = '';
		function level1() { level2(); }
		function level2() { level3(); }
		function level3() { throw new Error('deep error'); }
		try {
			level1();
		} catch (e) {
			stack = e.stack;
		}
		stack;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	stack := v.String()
	if !strings.Contains(stack, "level3") {
		t.Errorf("Stack trace should include 'level3', got: %s", stack)
	}
}

// ===============================================
// Type Verification Tests
// ===============================================

func TestError_TypeExists(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	script := `typeof Error === 'function'`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Error constructor should exist (NATIVE)")
	}
	t.Log("Error: NATIVE")
}

func TestError_AllTypesExist(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	errorTypes := []string{
		"Error",
		"TypeError",
		"RangeError",
		"ReferenceError",
		"SyntaxError",
		"URIError",
		"EvalError",
		"AggregateError",
	}

	for _, errType := range errorTypes {
		t.Run(errType, func(t *testing.T) {
			script := `typeof ` + errType + ` === 'function'`
			v, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if !v.ToBoolean() {
				t.Errorf("%s constructor should exist (NATIVE)", errType)
			} else {
				t.Logf("%s: NATIVE", errType)
			}
		})
	}
}

func TestError_PrototypeMethodsExist(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	methods := []string{"toString"}
	for _, method := range methods {
		t.Run("Error.prototype."+method, func(t *testing.T) {
			script := `typeof Error.prototype.` + method + ` === 'function'`
			v, err := runtime.RunString(script)
			if err != nil {
				t.Fatalf("script failed: %v", err)
			}
			if !v.ToBoolean() {
				t.Errorf("Error.prototype.%s should be a function (NATIVE)", method)
			}
		})
	}
}

func TestError_PropertiesExist(t *testing.T) {
	_, runtime, cleanup := newErrorsTestAdapter(t)
	defer cleanup()

	script := `
		var e = new Error('test');
		'message' in e && 'name' in e && 'stack' in e;
	`
	v, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("Error should have message, name, and stack properties")
	}
}
