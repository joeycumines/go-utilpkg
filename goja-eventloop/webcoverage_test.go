package gojaeventloop

import "testing"

// ===========================================================================
// bindCrypto — getRandomValues edge cases
// ===========================================================================

func TestCryptoGetRandomValues_Float64ArrayReject(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			crypto.getRandomValues(new Float64Array(4));
			var float64Passed = true;
		} catch(e) {
			var float64Rejected = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("float64Rejected")
	if val == nil || !val.ToBoolean() {
		t.Error("Float64Array should be rejected")
	}
}

func TestCryptoGetRandomValues_Float32ArrayReject(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			crypto.getRandomValues(new Float32Array(4));
			var float32Passed = true;
		} catch(e) {
			var float32Rejected = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("float32Rejected")
	if val == nil || !val.ToBoolean() {
		t.Error("Float32Array should be rejected")
	}
}

func TestCryptoGetRandomValues_NoArgs(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			crypto.getRandomValues();
			var noArgsOk = true;
		} catch(e) {
			var noArgsErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("noArgsErr")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected error with no args")
	}
}

func TestCryptoGetRandomValues_NullArg(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			crypto.getRandomValues(null);
		} catch(e) {
			var nullArgErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("nullArgErr")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected error with null arg")
	}
}

func TestCryptoGetRandomValues_QuotaExceeded(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			crypto.getRandomValues(new Uint8Array(65537));
		} catch(e) {
			var quotaErr = e.name === 'QuotaExceededError' || e.toString().indexOf('QuotaExceeded') >= 0;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("quotaErr")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected QuotaExceededError")
	}
}

func TestCryptoGetRandomValues_PlainObject(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			crypto.getRandomValues({});
		} catch(e) {
			var plainObjErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("plainObjErr")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected error with plain object")
	}
}

// ===========================================================================
// DOMException — uncovered branches
// ===========================================================================

func TestDOMException_DefaultValues_CoverGap(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var ex = new DOMException();
		var name = ex.name;
		var msg = ex.message;
		var code = ex.code;
		var str = ex.toString();
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if s := adapter.runtime.Get("name").String(); s != "Error" {
		t.Errorf("Expected 'Error', got %q", s)
	}
	if s := adapter.runtime.Get("msg").String(); s != "" {
		t.Errorf("Expected empty message, got %q", s)
	}
}

func TestDOMException_AllErrors(t *testing.T) {
	adapter := coverSetup(t)

	codes := []string{
		"IndexSizeError", "HierarchyRequestError", "WrongDocumentError",
		"InvalidCharacterError", "NoModificationAllowedError", "NotFoundError",
		"NotSupportedError", "InUseAttributeError", "InvalidStateError",
		"SyntaxError", "InvalidModificationError", "NamespaceError",
		"InvalidAccessError", "TypeMismatchError", "SecurityError",
		"NetworkError", "AbortError", "URLMismatchError",
		"QuotaExceededError", "TimeoutError", "InvalidNodeTypeError",
		"DataCloneError", "EncodingError",
	}
	for _, code := range codes {
		_, err := adapter.runtime.RunString(`new DOMException("msg", "` + code + `")`)
		if err != nil {
			t.Errorf("Failed to create DOMException(%s): %v", code, err)
		}
	}
}

// ===========================================================================
// structuredClone — edge case types
// ===========================================================================

func TestStructuredClone_ErrorObject(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var err = new Error("oops");
		err.extra = 42;
		var cloned = structuredClone(err);
		var scErrOk = cloned !== err && cloned instanceof Error && cloned.message === "oops" && cloned.extra === undefined;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("scErrOk")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected Error object clone to preserve its serialized message and exclude expandos")
	}
}

// ===========================================================================
// performance.toJSON
// ===========================================================================

func TestPerformance_ToJSON_CoverGap(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var json = performance.toJSON();
		typeof json === 'object';
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected object from toJSON")
	}
}
