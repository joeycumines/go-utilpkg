package gojaeventloop

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// crypto.getRandomValues() Tests
// ===============================================

// Helper: create a bound adapter without starting the loop.
// All tests in this file only call runtime.RunString (no async/timers),
// so they don't need loop.Run().
func newBoundCryptoTestAdapter(t *testing.T) (*goeventloop.Loop, *goja.Runtime) {
	t.Helper()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("New loop failed: %v", err)
	}
	t.Cleanup(func() { loop.Shutdown(context.Background()) })

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter failed: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}
	return loop, runtime
}

func TestGetRandomValues_Uint8Array(t *testing.T) {
	_, runtime := newBoundCryptoTestAdapter(t)

	result, err := runtime.RunString(`
		var arr = new Uint8Array(16);
		var ret = crypto.getRandomValues(arr);
		// Should return the same array
		ret === arr;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("getRandomValues did not return the same array")
	}

	// Verify at least some bytes are non-zero (statistically)
	result2, err := runtime.RunString(`
		var nonZero = 0;
		for (var i = 0; i < arr.length; i++) {
			if (arr[i] !== 0) nonZero++;
		}
		nonZero > 0;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result2.ToBoolean() {
		t.Error("All 16 bytes are zero - extremely unlikely with crypto random")
	}
}

func TestGetRandomValues_Uint16Array(t *testing.T) {
	_, runtime := newBoundCryptoTestAdapter(t)

	result, err := runtime.RunString(`
		var arr = new Uint16Array(8);
		var ret = crypto.getRandomValues(arr);
		ret === arr && arr.length === 8;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("getRandomValues with Uint16Array failed")
	}
}

func TestGetRandomValues_Uint32Array(t *testing.T) {
	_, runtime := newBoundCryptoTestAdapter(t)

	result, err := runtime.RunString(`
		var arr = new Uint32Array(4);
		var ret = crypto.getRandomValues(arr);
		ret === arr && arr.length === 4;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("getRandomValues with Uint32Array failed")
	}
}

func TestGetRandomValues_Int8Array(t *testing.T) {
	_, runtime := newBoundCryptoTestAdapter(t)

	result, err := runtime.RunString(`
		var arr = new Int8Array(16);
		var ret = crypto.getRandomValues(arr);
		ret === arr && arr.length === 16;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("getRandomValues with Int8Array failed")
	}
}

func TestGetRandomValues_Int16Array(t *testing.T) {
	_, runtime := newBoundCryptoTestAdapter(t)

	result, err := runtime.RunString(`
		var arr = new Int16Array(8);
		var ret = crypto.getRandomValues(arr);
		ret === arr && arr.length === 8;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("getRandomValues with Int16Array failed")
	}
}

func TestGetRandomValues_Int32Array(t *testing.T) {
	_, runtime := newBoundCryptoTestAdapter(t)

	result, err := runtime.RunString(`
		var arr = new Int32Array(4);
		var ret = crypto.getRandomValues(arr);
		ret === arr && arr.length === 4;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("getRandomValues with Int32Array failed")
	}
}

func TestGetRandomValues_Uint8ClampedArray(t *testing.T) {
	_, runtime := newBoundCryptoTestAdapter(t)

	result, err := runtime.RunString(`
		var arr = new Uint8ClampedArray(16);
		var ret = crypto.getRandomValues(arr);
		ret === arr && arr.length === 16;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("getRandomValues with Uint8ClampedArray failed")
	}
}

func TestGetRandomValues_EmptyArray(t *testing.T) {
	_, runtime := newBoundCryptoTestAdapter(t)

	result, err := runtime.RunString(`
		var arr = new Uint8Array(0);
		var ret = crypto.getRandomValues(arr);
		ret === arr && arr.length === 0;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("getRandomValues with empty array failed")
	}
}

func TestGetRandomValues_MaxSize(t *testing.T) {
	_, runtime := newBoundCryptoTestAdapter(t)

	// 65536 bytes is the max (should succeed)
	result, err := runtime.RunString(`
		var arr = new Uint8Array(65536);
		var ret = crypto.getRandomValues(arr);
		ret === arr && arr.length === 65536;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("getRandomValues with 65536 bytes should succeed")
	}
}

func TestGetRandomValues_QuotaExceeded(t *testing.T) {
	_, runtime := newBoundCryptoTestAdapter(t)

	// 65537 bytes exceeds the limit
	_, err := runtime.RunString(`
		crypto.getRandomValues(new Uint8Array(65537));
	`)
	if err == nil {
		t.Error("Expected QuotaExceededError for >65536 bytes")
		return
	}
	// Verify the error message mentions QuotaExceededError
	errStr := err.Error()
	if !containsStr(errStr, "QuotaExceeded") {
		t.Errorf("Expected QuotaExceededError, got: %s", errStr)
	}
}

func TestGetRandomValues_QuotaExceeded_LargeUint32(t *testing.T) {
	_, runtime := newBoundCryptoTestAdapter(t)

	// 16385 × 4 = 65540 > 65536
	_, err := runtime.RunString(`
		crypto.getRandomValues(new Uint32Array(16385));
	`)
	if err == nil {
		t.Error("Expected QuotaExceededError for Uint32Array with >65536 bytes")
	}
}

func TestGetRandomValues_TypeError_NoArgs(t *testing.T) {
	_, runtime := newBoundCryptoTestAdapter(t)

	_, err := runtime.RunString(`
		crypto.getRandomValues();
	`)
	if err == nil {
		t.Error("Expected TypeError for missing argument")
	}
}

func TestGetRandomValues_TypeError_Null(t *testing.T) {
	_, runtime := newBoundCryptoTestAdapter(t)

	_, err := runtime.RunString(`
		crypto.getRandomValues(null);
	`)
	if err == nil {
		t.Error("Expected TypeError for null argument")
	}
}

func TestGetRandomValues_TypeError_Number(t *testing.T) {
	_, runtime := newBoundCryptoTestAdapter(t)

	_, err := runtime.RunString(`
		crypto.getRandomValues(42);
	`)
	if err == nil {
		t.Error("Expected TypeError for number argument")
	}
}

func TestGetRandomValues_TypeError_PlainArray(t *testing.T) {
	_, runtime := newBoundCryptoTestAdapter(t)

	_, err := runtime.RunString(`
		crypto.getRandomValues([1, 2, 3]);
	`)
	if err == nil {
		t.Error("Expected TypeError for plain array argument")
	}
}

func TestGetRandomValues_TypeError_Float32Array(t *testing.T) {
	_, runtime := newBoundCryptoTestAdapter(t)

	_, err := runtime.RunString(`
		crypto.getRandomValues(new Float32Array(4));
	`)
	if err == nil {
		t.Error("Expected TypeError for Float32Array")
	}
}

func TestGetRandomValues_TypeError_Float64Array(t *testing.T) {
	_, runtime := newBoundCryptoTestAdapter(t)

	_, err := runtime.RunString(`
		crypto.getRandomValues(new Float64Array(4));
	`)
	if err == nil {
		t.Error("Expected TypeError for Float64Array")
	}
}

func TestGetRandomValues_Randomness(t *testing.T) {
	_, runtime := newBoundCryptoTestAdapter(t)

	// Generate two arrays and verify they are different
	result, err := runtime.RunString(`
		var a = new Uint8Array(32);
		var b = new Uint8Array(32);
		crypto.getRandomValues(a);
		crypto.getRandomValues(b);
		// Compare: at least one byte should differ
		var differ = false;
		for (var i = 0; i < 32; i++) {
			if (a[i] !== b[i]) {
				differ = true;
				break;
			}
		}
		differ;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Two 32-byte random arrays are identical - extremely unlikely")
	}
}

func TestGetRandomValues_FunctionExists(t *testing.T) {
	_, runtime := newBoundCryptoTestAdapter(t)

	result, err := runtime.RunString(`typeof crypto.getRandomValues`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if result.String() != "function" {
		t.Errorf("Expected crypto.getRandomValues to be 'function', got: %s", result.String())
	}
}

func TestGetRandomValues_ReturnsSameObject(t *testing.T) {
	_, runtime := newBoundCryptoTestAdapter(t)

	result, err := runtime.RunString(`
		var arr = new Uint8Array(8);
		var ret = crypto.getRandomValues(arr);
		// Verify it's the exact same object (identity check)
		ret === arr;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("getRandomValues should return the same TypedArray object")
	}
}

func TestGetRandomValues_ModifiesInPlace(t *testing.T) {
	_, runtime := newBoundCryptoTestAdapter(t)

	result, err := runtime.RunString(`
		var arr = new Uint8Array(16);
		// Verify all zeros initially
		var allZero = true;
		for (var i = 0; i < arr.length; i++) {
			if (arr[i] !== 0) allZero = false;
		}
		if (!allZero) throw new Error("Array not initialized to zeros");

		crypto.getRandomValues(arr);

		// Check that SOMETHING changed
		var hasNonZero = false;
		for (var i = 0; i < arr.length; i++) {
			if (arr[i] !== 0) hasNonZero = true;
		}
		hasNonZero;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("array should have been modified in-place with random data")
	}
}

// containsStr is a simple helper for string containment check.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
