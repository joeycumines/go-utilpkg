package gojaeventloop

import (
	"context"
	"strings"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// testSetupDOMException creates an event loop, adapter and runs it for testing.
func testSetupDOMException(t *testing.T) (*Adapter, func()) {
	t.Helper()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	if err := runtime.Set("DOMException", adapter.domExceptionConstructor); err != nil {
		t.Fatalf("failed to install DOMException constructor: %v", err)
	}
	constructor := runtime.Get("DOMException").ToObject(runtime)
	if err := adapter.bindDOMExceptionConstants(constructor, constructorPrototype(runtime, "Error")); err != nil {
		t.Fatalf("failed to bind DOMException: %v", err)
	}
	if err := runtime.Set("structuredClone", adapter.structuredClone); err != nil {
		t.Fatalf("failed to install structuredClone: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	cleanup := func() {
		cancel()
		select {
		case <-runDone:
		case <-time.After(2 * time.Second):
			t.Error("loop did not stop in time")
		}
	}

	return adapter, cleanup
}

// TestDOMException_Constructor tests the DOMException constructor.
func TestDOMException_Constructor(t *testing.T) {
	adapter, cleanup := testSetupDOMException(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var ex = new DOMException();
			return ex !== null && ex !== undefined;
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("DOMException constructor failed")
	}
}

func TestDOMException_ConstructorName(t *testing.T) {
	adapter, cleanup := testSetupDOMException(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var ex = new DOMException('bad', 'DataCloneError');
			return DOMException.name + ':' + ex.constructor.name + ':' + (ex.constructor === DOMException);
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := result.String(), "DOMException:DOMException:true"; got != want {
		t.Fatalf("DOMException constructor name = %q, want %q", got, want)
	}
}

// TestDOMException_DefaultValues tests default property values.
func TestDOMException_DefaultValues(t *testing.T) {
	adapter, cleanup := testSetupDOMException(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var ex = new DOMException();
			var messageEmpty = ex.message === '';
			var nameError = ex.name === 'Error';
			var codeZero = ex.code === 0;
			return messageEmpty && nameError && codeZero;
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("DOMException default values failed")
	}
}

// TestDOMException_WithMessage tests constructor with message.
func TestDOMException_WithMessage(t *testing.T) {
	adapter, cleanup := testSetupDOMException(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var ex = new DOMException('Something went wrong');
			return ex.message === 'Something went wrong' &&
				ex.name === 'Error' &&
				ex.code === 0;
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("DOMException with message failed")
	}
}

// TestDOMException_WithMessageAndName tests constructor with message and name.
func TestDOMException_WithMessageAndName(t *testing.T) {
	adapter, cleanup := testSetupDOMException(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var ex = new DOMException('Invalid state', 'InvalidStateError');
			return ex.message === 'Invalid state' &&
				ex.name === 'InvalidStateError' &&
				ex.code === 11; // InvalidStateError = 11
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("DOMException with message and name failed")
	}
}

// TestDOMException_ToString tests the toString method.
func TestDOMException_ToString(t *testing.T) {
	adapter, cleanup := testSetupDOMException(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var ex = new DOMException('Test message', 'TestError');
			var str = ex.toString();
			return str === 'TestError: Test message';
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("DOMException.toString failed")
	}
}

// TestDOMException_KnownErrorCodes tests error codes for known error names.
func TestDOMException_KnownErrorCodes(t *testing.T) {
	adapter, cleanup := testSetupDOMException(t)
	defer cleanup()

	tests := []struct {
		name         string
		expectedCode int
	}{
		{"IndexSizeError", 1},
		{"HierarchyRequestError", 3},
		{"WrongDocumentError", 4},
		{"InvalidCharacterError", 5},
		{"NoModificationAllowedError", 7},
		{"NotFoundError", 8},
		{"NotSupportedError", 9},
		{"InUseAttributeError", 10},
		{"InvalidStateError", 11},
		{"SyntaxError", 12},
		{"InvalidModificationError", 13},
		{"NamespaceError", 14},
		{"InvalidAccessError", 15},
		{"TypeMismatchError", 17},
		{"SecurityError", 18},
		{"NetworkError", 19},
		{"AbortError", 20},
		{"URLMismatchError", 21},
		{"QuotaExceededError", 22},
		{"TimeoutError", 23},
		{"InvalidNodeTypeError", 24},
		{"DataCloneError", 25},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := adapter.runtime.RunString(`
				(function() {
					var ex = new DOMException('test', '` + tc.name + `');
					return ex.code;
				})()
			`)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			code := int(result.ToInteger())
			if code != tc.expectedCode {
				t.Errorf("expected code %d for %s, got %d", tc.expectedCode, tc.name, code)
			}
		})
	}
}

// TestDOMException_NewErrorNamesCodeZero tests that new error names have code 0.
func TestDOMException_NewErrorNamesCodeZero(t *testing.T) {
	adapter, cleanup := testSetupDOMException(t)
	defer cleanup()

	newErrorNames := []string{
		"EncodingError",
		"NotReadableError",
		"UnknownError",
		"ConstraintError",
		"DataError",
		"ReadOnlyError",
		"VersionError",
		"OperationError",
		"NotAllowedError",
	}

	for _, name := range newErrorNames {
		t.Run(name, func(t *testing.T) {
			result, err := adapter.runtime.RunString(`
				(function() {
					var ex = new DOMException('test', '` + name + `');
					return ex.code === 0 && ex.name === '` + name + `';
				})()
			`)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.ToBoolean() {
				t.Errorf("expected code 0 for %s", name)
			}
		})
	}
}

// TestDOMException_UnknownNameCodeZero tests that unknown names have code 0.
func TestDOMException_UnknownNameCodeZero(t *testing.T) {
	adapter, cleanup := testSetupDOMException(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var ex = new DOMException('test', 'UnknownCustomError');
			return ex.code === 0 && ex.name === 'UnknownCustomError';
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("DOMException with unknown name should have code 0")
	}
}

// TestDOMException_StaticConstants tests the static error code constants.
func TestDOMException_StaticConstants(t *testing.T) {
	adapter, cleanup := testSetupDOMException(t)
	defer cleanup()

	tests := []struct {
		constant string
		expected int
	}{
		{"INDEX_SIZE_ERR", 1},
		{"DOMSTRING_SIZE_ERR", 2},
		{"HIERARCHY_REQUEST_ERR", 3},
		{"WRONG_DOCUMENT_ERR", 4},
		{"INVALID_CHARACTER_ERR", 5},
		{"NO_DATA_ALLOWED_ERR", 6},
		{"NO_MODIFICATION_ALLOWED_ERR", 7},
		{"NOT_FOUND_ERR", 8},
		{"NOT_SUPPORTED_ERR", 9},
		{"INUSE_ATTRIBUTE_ERR", 10},
		{"INVALID_STATE_ERR", 11},
		{"SYNTAX_ERR", 12},
		{"INVALID_MODIFICATION_ERR", 13},
		{"NAMESPACE_ERR", 14},
		{"INVALID_ACCESS_ERR", 15},
		{"VALIDATION_ERR", 16},
		{"TYPE_MISMATCH_ERR", 17},
		{"SECURITY_ERR", 18},
		{"NETWORK_ERR", 19},
		{"ABORT_ERR", 20},
		{"URL_MISMATCH_ERR", 21},
		{"QUOTA_EXCEEDED_ERR", 22},
		{"TIMEOUT_ERR", 23},
		{"INVALID_NODE_TYPE_ERR", 24},
		{"DATA_CLONE_ERR", 25},
	}

	for _, tc := range tests {
		t.Run(tc.constant, func(t *testing.T) {
			result, err := adapter.runtime.RunString(`DOMException.` + tc.constant)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			code := int(result.ToInteger())
			if code != tc.expected {
				t.Errorf("expected DOMException.%s = %d, got %d", tc.constant, tc.expected, code)
			}
		})
	}
}

// TestDOMException_InstanceProperties tests that instance has all properties.
func TestDOMException_InstanceProperties(t *testing.T) {
	adapter, cleanup := testSetupDOMException(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var ex = new DOMException('Test', 'NotFoundError');
			return 'message' in ex &&
				'name' in ex &&
				'code' in ex &&
				'toString' in ex;
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("DOMException missing properties")
	}
}

// TestDOMException_CanThrow tests that DOMException can be thrown.
func TestDOMException_CanThrow(t *testing.T) {
	adapter, cleanup := testSetupDOMException(t)
	defer cleanup()

	_, err := adapter.runtime.RunString(`
		throw new DOMException('Test exception', 'NotFoundError');
	`)
	if err == nil {
		t.Fatalf("expected exception to be thrown")
	}
	errStr := err.Error()
	// Check that thrown error contains the exception info
	if !strings.Contains(errStr, "NotFoundError") && !strings.Contains(errStr, "Test exception") {
		t.Errorf("thrown exception should contain DOMException info, got: %v", err)
	}
}

// TestDOMException_CatchAndCheckProperties tests catching and inspecting DOMException.
func TestDOMException_CatchAndCheckProperties(t *testing.T) {
	adapter, cleanup := testSetupDOMException(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			try {
				throw new DOMException('Caught this', 'AbortError');
			} catch (e) {
				return e.message === 'Caught this' &&
					e.name === 'AbortError' &&
					e.code === 20;
			}
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("DOMException caught with wrong properties")
	}
}

// TestDOMException_UndefinedMessage tests constructor with undefined message.
func TestDOMException_UndefinedMessage(t *testing.T) {
	adapter, cleanup := testSetupDOMException(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var ex = new DOMException(undefined, 'TestError');
			return ex.message === '' && ex.name === 'TestError';
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("DOMException with undefined message failed")
	}
}

// TestDOMException_UndefinedName tests constructor with undefined name.
func TestDOMException_UndefinedName(t *testing.T) {
	adapter, cleanup := testSetupDOMException(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var ex = new DOMException('message', undefined);
			return ex.message === 'message' && ex.name === 'Error';
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("DOMException with undefined name failed")
	}
}

// TestDOMException_TypeCoercion tests that message and name are coerced to strings.
func TestDOMException_TypeCoercion(t *testing.T) {
	adapter, cleanup := testSetupDOMException(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var ex = new DOMException(42, 123);
			return ex.message === '42' && ex.name === '123';
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("DOMException type coercion failed")
	}
}

// TestDOMException_CompareWithStaticConstant tests comparing code with static constant.
func TestDOMException_CompareWithStaticConstant(t *testing.T) {
	adapter, cleanup := testSetupDOMException(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var ex = new DOMException('timeout', 'TimeoutError');
			return ex.code === DOMException.TIMEOUT_ERR;
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("DOMException code comparison with static constant failed")
	}
}

// TestDOMException_ThrowWithAbortError tests throwing with AbortError.
func TestDOMException_ThrowWithAbortError(t *testing.T) {
	adapter, cleanup := testSetupDOMException(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			try {
				var ex = new DOMException('The operation was aborted', 'AbortError');
				throw ex;
			} catch (e) {
				return e.name === 'AbortError' && e.code === DOMException.ABORT_ERR;
			}
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("DOMException AbortError throw/catch failed")
	}
}

// TestDOMException_MultipleInstances tests creating multiple instances.
func TestDOMException_MultipleInstances(t *testing.T) {
	adapter, cleanup := testSetupDOMException(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var ex1 = new DOMException('first', 'NotFoundError');
			var ex2 = new DOMException('second', 'TimeoutError');

			var different = ex1 !== ex2;
			var ex1Correct = ex1.message === 'first' && ex1.name === 'NotFoundError' && ex1.code === 8;
			var ex2Correct = ex2.message === 'second' && ex2.name === 'TimeoutError' && ex2.code === 23;

			return different && ex1Correct && ex2Correct;
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("DOMException multiple instances failed")
	}
}

// TestDOMException_ToStringEmpty tests toString with empty message.
func TestDOMException_ToStringEmpty(t *testing.T) {
	adapter, cleanup := testSetupDOMException(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var ex = new DOMException();
			return ex.toString() === 'Error';
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Errorf("DOMException.toString empty failed")
	}
}

func TestDOMException_PrototypeDescriptorsAndBrand(t *testing.T) {
	adapter, cleanup := testSetupDOMException(t)
	defer cleanup()

	_, err := adapter.runtime.RunString(`
		const ex = new DOMException("bad", "DataCloneError");
		if (!(ex instanceof DOMException)) throw new Error("missing DOMException brand");
		if (!(ex instanceof Error)) throw new Error("missing Error inheritance");
		if (Object.keys(ex).length !== 0) throw new Error("enumerable instance state leaked");
		if (Object.getOwnPropertyNames(ex).includes("_domException")) throw new Error("public state leaked");
		if (Object.prototype.toString.call(ex) !== "[object DOMException]") throw new Error("wrong toStringTag");
		if (typeof ex.stack !== "string" || !ex.stack.startsWith("DataCloneError: bad")) throw new Error("missing native Error stack");
		const stackDescriptor = Object.getOwnPropertyDescriptor(ex, "stack");
		if (!stackDescriptor || typeof stackDescriptor.value !== "string" || !stackDescriptor.writable || stackDescriptor.enumerable || !stackDescriptor.configurable) {
			throw new Error("stack descriptor mismatch");
		}
		if (DOMException.name !== "DOMException" || DOMException.length !== 0) throw new Error("constructor metadata");

		for (const name of ["message", "name", "code"]) {
			const descriptor = Object.getOwnPropertyDescriptor(DOMException.prototype, name);
			if (!descriptor || typeof descriptor.get !== "function") throw new Error(name + " getter missing");
			if (descriptor.set !== undefined || !descriptor.enumerable || !descriptor.configurable) {
				throw new Error(name + " descriptor mismatch");
			}
			let threw = false;
			try { Reflect.apply(descriptor.get, {}, []); } catch (err) { threw = err instanceof TypeError; }
			if (!threw) throw new Error(name + " getter accepted incompatible receiver");
		}

		for (const holder of [DOMException, DOMException.prototype]) {
			const descriptor = Object.getOwnPropertyDescriptor(holder, "DATA_CLONE_ERR");
			if (!descriptor || descriptor.value !== 25 || descriptor.writable || descriptor.configurable || !descriptor.enumerable) {
				throw new Error("constant descriptor mismatch");
			}
		}
	`)
	if err != nil {
		t.Fatalf("DOMException prototype contract: %v", err)
	}
}

func TestDOMException_StructuredClonePreservesSerializedState(t *testing.T) {
	adapter, cleanup := testSetupDOMException(t)
	defer cleanup()

	_, err := adapter.runtime.RunString(`
		const source = new DOMException("bad", "DataCloneError");
		source.stack = "pinned stack";
		source.expando = { ignored: true };
		Object.setPrototypeOf(source, null);
		const graph = { first: source, second: source };
		graph.self = graph;
		const cloned = structuredClone(graph);
		if (cloned === graph || cloned.self !== cloned) throw new Error("graph cycle");
		if (cloned.first !== cloned.second || cloned.first === source) throw new Error("DOMException identity");
		if (!(cloned.first instanceof DOMException) || !(cloned.first instanceof Error)) throw new Error("DOMException clone brand");
		if (cloned.first.name !== "DataCloneError" || cloned.first.message !== "bad" || cloned.first.code !== 25) {
			throw new Error("DOMException clone fields");
		}
		if (cloned.first.stack !== "pinned stack") throw new Error("DOMException clone stack");
		if (cloned.first.expando !== undefined) throw new Error("DOMException expando cloned");
	`)
	if err != nil {
		t.Fatalf("DOMException structured clone: %v", err)
	}
}
