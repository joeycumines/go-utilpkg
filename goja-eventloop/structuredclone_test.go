package gojaeventloop

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// testSetup creates an event loop, adapter and runs it for testing.
func testSetup(t *testing.T) (*Adapter, func()) {
	t.Helper()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind adapter: %v", err)
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
			t.Log("loop did not stop in time")
		}
	}

	return adapter, cleanup
}

// TestStructuredClone_Undefined tests cloning undefined.
func TestStructuredClone_Undefined(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		structuredClone(undefined)
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !goja.IsUndefined(result) {
		t.Errorf("expected undefined, got %v", result.Export())
	}
}

// TestStructuredClone_Null tests cloning null.
func TestStructuredClone_Null(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		structuredClone(null)
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !goja.IsNull(result) {
		t.Errorf("expected null, got %v", result.Export())
	}
}

// TestStructuredClone_NoArgument tests cloning with no argument.
func TestStructuredClone_NoArgument(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		structuredClone()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !goja.IsUndefined(result) {
		t.Errorf("expected undefined, got %v", result.Export())
	}
}

// TestStructuredClone_Primitives tests cloning primitive types.
func TestStructuredClone_Primitives(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	tests := []struct {
		name     string
		input    string
		expected interface{}
	}{
		{"string", `"hello world"`, "hello world"},
		{"number_int", `42`, int64(42)},
		{"number_float", `3.14`, 3.14},
		{"boolean_true", `true`, true},
		{"boolean_false", `false`, false},
		{"zero", `0`, int64(0)},
		{"empty_string", `""`, ""},
		{"negative", `-123`, int64(-123)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := adapter.runtime.RunString(`
				(function() {
					var original = ` + tc.input + `;
					var cloned = structuredClone(original);
					return cloned;
				})()
			`)

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got := result.Export()
			if got != tc.expected {
				// Handle float64 comparison
				if f, ok := tc.expected.(float64); ok {
					if gf, ok := got.(float64); ok && gf == f {
						return
					}
				}
				// Handle int64 comparison with float64
				if i, ok := tc.expected.(int64); ok {
					if gf, ok := got.(float64); ok && gf == float64(i) {
						return
					}
				}
				t.Errorf("expected %v (%T), got %v (%T)", tc.expected, tc.expected, got, got)
			}
		})
	}
}

// TestStructuredClone_PlainObject tests cloning plain objects.
func TestStructuredClone_PlainObject(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var original = { name: "Alice", age: 30, active: true };
			var cloned = structuredClone(original);

			// Check that it's a new object
			var isDifferent = original !== cloned;

			// Check values are same
			var nameMatch = cloned.name === "Alice";
			var ageMatch = cloned.age === 30;
			var activeMatch = cloned.active === true;

			// Modify cloned and check original is unchanged
			cloned.name = "Bob";
			var originalUnchanged = original.name === "Alice";

			return isDifferent && nameMatch && ageMatch && activeMatch && originalUnchanged;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ToBoolean() != true {
		t.Errorf("plain object cloning failed")
	}
}

// TestStructuredClone_NestedObject tests cloning nested objects.
func TestStructuredClone_NestedObject(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var original = {
				outer: {
					inner: {
						value: 42
					}
				}
			};
			var cloned = structuredClone(original);

			// Check deep clone
			var outerDifferent = original.outer !== cloned.outer;
			var innerDifferent = original.outer.inner !== cloned.outer.inner;
			var valueMatch = cloned.outer.inner.value === 42;

			// Modify nested and check original unchanged
			cloned.outer.inner.value = 99;
			var originalUnchanged = original.outer.inner.value === 42;

			return outerDifferent && innerDifferent && valueMatch && originalUnchanged;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ToBoolean() != true {
		t.Errorf("nested object cloning failed")
	}
}

// TestStructuredClone_Array tests cloning arrays.
func TestStructuredClone_Array(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var original = [1, 2, 3, "four", true];
			var cloned = structuredClone(original);

			// Check that it's a new array
			var isDifferent = original !== cloned;

			// Check Array.isArray
			var isArray = Array.isArray(cloned);

			// Check length
			var lengthMatch = cloned.length === 5;

			// Check values
			var valuesMatch = cloned[0] === 1 &&
				cloned[1] === 2 &&
				cloned[2] === 3 &&
				cloned[3] === "four" &&
				cloned[4] === true;

			// Modify cloned and check original unchanged
			cloned[0] = 999;
			var originalUnchanged = original[0] === 1;

			return isDifferent && isArray && lengthMatch && valuesMatch && originalUnchanged;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ToBoolean() != true {
		t.Errorf("array cloning failed")
	}
}

// TestStructuredClone_NestedArray tests cloning arrays with nested objects.
func TestStructuredClone_NestedArray(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var original = [{ id: 1 }, { id: 2 }, [3, 4, 5]];
			var cloned = structuredClone(original);

			// Deep clone verification
			var objDifferent = original[0] !== cloned[0];
			var nestedArrDifferent = original[2] !== cloned[2];
			var valuesMatch = cloned[0].id === 1 && cloned[1].id === 2 && cloned[2][1] === 4;

			// Modify and verify isolation
			cloned[0].id = 999;
			var originalUnchanged = original[0].id === 1;

			return objDifferent && nestedArrDifferent && valuesMatch && originalUnchanged;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ToBoolean() != true {
		t.Errorf("nested array cloning failed")
	}
}

// TestStructuredClone_Date tests cloning Date objects.
func TestStructuredClone_Date(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var original = new Date(1609459200000); // 2021-01-01 00:00:00 UTC
			var cloned = structuredClone(original);

			// Check that it's a new Date
			var isDifferent = original !== cloned;

			// Check it's still a Date
			var isDate = cloned instanceof Date;

			// Check same time value
			var timeMatch = cloned.getTime() === 1609459200000;

			// Modify cloned and check original unchanged
			cloned.setTime(0);
			var originalUnchanged = original.getTime() === 1609459200000;

			return isDifferent && isDate && timeMatch && originalUnchanged;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ToBoolean() != true {
		t.Errorf("Date cloning failed")
	}
}

// TestStructuredClone_RegExp tests cloning RegExp objects.
func TestStructuredClone_RegExp(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var original = /hello/gi;
			var cloned = structuredClone(original);

			// Check that it's a new RegExp
			var isDifferent = original !== cloned;

			// Check it's still a RegExp
			var isRegExp = cloned instanceof RegExp;

			// Check source and flags preserved
			var sourceMatch = cloned.source === "hello";
			var flagsMatch = cloned.flags === "gi";

			// Verify it works
			var works = cloned.test("HELLO");

			return isDifferent && isRegExp && sourceMatch && flagsMatch && works;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ToBoolean() != true {
		t.Errorf("RegExp cloning failed")
	}
}

// TestStructuredClone_Map tests cloning Map objects.
func TestStructuredClone_Map(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var original = new Map([
				["key1", "value1"],
				["key2", { nested: true }]
			]);
			var cloned = structuredClone(original);

			// Check that it's a new Map
			var isDifferent = original !== cloned;

			// Check it's still a Map
			var isMap = cloned instanceof Map;

			// Check size preserved
			var sizeMatch = cloned.size === 2;

			// Check values
			var key1Match = cloned.get("key1") === "value1";
			var key2Match = cloned.get("key2") && cloned.get("key2").nested === true;

			// Check nested object is cloned (different reference)
			var nestedDifferent = cloned.get("key2") !== original.get("key2");

			// Modify cloned and check original unchanged
			cloned.set("key1", "modified");
			var originalUnchanged = original.get("key1") === "value1";

			return isDifferent && isMap && sizeMatch && key1Match && key2Match && nestedDifferent && originalUnchanged;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ToBoolean() != true {
		t.Errorf("Map cloning failed")
	}
}

// TestStructuredClone_Set tests cloning Set objects.
func TestStructuredClone_Set(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var obj = { id: 1 };
			var original = new Set([1, 2, "three", obj]);
			var cloned = structuredClone(original);

			// Check that it's a new Set
			var isDifferent = original !== cloned;

			// Check it's still a Set
			var isSet = cloned instanceof Set;

			// Check size preserved
			var sizeMatch = cloned.size === 4;

			// Check primitives preserved
			var has1 = cloned.has(1);
			var has2 = cloned.has(2);
			var has_three = cloned.has("three");

			// Note: the object is cloned, so it won't be strictly equal
			// We just verify size includes it

			// Modify cloned and check original unchanged
			cloned.add(999);
			var originalUnchanged = original.size === 4;

			return isDifferent && isSet && sizeMatch && has1 && has2 && has_three && originalUnchanged;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ToBoolean() != true {
		t.Errorf("Set cloning failed")
	}
}

// TestStructuredClone_CircularReference tests cloning objects with circular references.
func TestStructuredClone_CircularReference(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var original = { name: "circular" };
			original.self = original; // Create circular reference

			var cloned = structuredClone(original);

			// Check that it's a new object
			var isDifferent = original !== cloned;

			// Check circular reference is preserved
			var hasCircular = cloned.self === cloned;

			// Check original circular reference is still intact
			var originalIntact = original.self === original;

			// Name is cloned correctly
			var nameMatch = cloned.name === "circular";

			return isDifferent && hasCircular && originalIntact && nameMatch;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ToBoolean() != true {
		t.Errorf("circular reference cloning failed")
	}
}

// TestStructuredClone_DeepCircularReference tests cloning with deeply nested circular references.
func TestStructuredClone_DeepCircularReference(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var a = { name: "a" };
			var b = { name: "b", ref: a };
			a.ref = b; // a -> b -> a (circular)

			var cloned = structuredClone(a);

			// Check structure is correct
			var aNameMatch = cloned.name === "a";
			var bNameMatch = cloned.ref.name === "b";

			// Check circular reference is preserved
			var circular = cloned.ref.ref === cloned;

			// Check different from original
			var aDifferent = cloned !== a;
			var bDifferent = cloned.ref !== b;

			return aNameMatch && bNameMatch && circular && aDifferent && bDifferent;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ToBoolean() != true {
		t.Errorf("deep circular reference cloning failed")
	}
}

// TestStructuredClone_Function_Throws tests that cloning functions throws TypeError.
func TestStructuredClone_Function_Throws(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	_, err := adapter.runtime.RunString(`
		structuredClone(function() {})
	`)

	if err == nil {
		t.Fatalf("expected error when cloning function, got none")
	}

	// Check it's a TypeError
	errStr := err.Error()
	if !containsSubstring(errStr, "TypeError") && !containsSubstring(errStr, "cannot clone functions") {
		t.Errorf("expected TypeError about functions, got: %v", err)
	}
}

// TestStructuredClone_ObjectWithFunction_SkipsFunction tests that object properties that are functions are skipped.
func TestStructuredClone_ObjectWithFunction_SkipsFunction(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var original = {
				name: "test",
				getValue: function() { return 42; }
			};
			var cloned = structuredClone(original);

			// Name should be cloned
			var nameMatch = cloned.name === "test";

			// Function should be skipped (undefined in cloned)
			var funcSkipped = cloned.getValue === undefined;

			return nameMatch && funcSkipped;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ToBoolean() != true {
		t.Errorf("object with function cloning failed")
	}
}

// TestStructuredClone_EmptyObject tests cloning empty object.
func TestStructuredClone_EmptyObject(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var original = {};
			var cloned = structuredClone(original);

			var isDifferent = original !== cloned;
			var isEmpty = Object.keys(cloned).length === 0;

			return isDifferent && isEmpty;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ToBoolean() != true {
		t.Errorf("empty object cloning failed")
	}
}

// TestStructuredClone_EmptyArray tests cloning empty array.
func TestStructuredClone_EmptyArray(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var original = [];
			var cloned = structuredClone(original);

			var isDifferent = original !== cloned;
			var isArray = Array.isArray(cloned);
			var isEmpty = cloned.length === 0;

			return isDifferent && isArray && isEmpty;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ToBoolean() != true {
		t.Errorf("empty array cloning failed")
	}
}

// TestStructuredClone_EmptyMap tests cloning empty Map.
func TestStructuredClone_EmptyMap(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var original = new Map();
			var cloned = structuredClone(original);

			var isDifferent = original !== cloned;
			var isMap = cloned instanceof Map;
			var isEmpty = cloned.size === 0;

			return isDifferent && isMap && isEmpty;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ToBoolean() != true {
		t.Errorf("empty Map cloning failed")
	}
}

// TestStructuredClone_EmptySet tests cloning empty Set.
func TestStructuredClone_EmptySet(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var original = new Set();
			var cloned = structuredClone(original);

			var isDifferent = original !== cloned;
			var isSet = cloned instanceof Set;
			var isEmpty = cloned.size === 0;

			return isDifferent && isSet && isEmpty;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ToBoolean() != true {
		t.Errorf("empty Set cloning failed")
	}
}

// TestStructuredClone_ComplexNested tests cloning complex nested structures.
func TestStructuredClone_ComplexNested(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var original = {
				array: [1, 2, { nested: "value" }],
				map: new Map([["key", { data: 42 }]]),
				set: new Set([1, 2, 3]),
				date: new Date(1000000000000),
				regex: /test/i
			};

			var cloned = structuredClone(original);

			// All should be different references
			var arrayDiff = cloned.array !== original.array;
			var mapDiff = cloned.map !== original.map;
			var setDiff = cloned.set !== original.set;
			var dateDiff = cloned.date !== original.date;
			var regexDiff = cloned.regex !== original.regex;

			// Values should be preserved
			var arrayOk = cloned.array[2].nested === "value";
			var mapOk = cloned.map.get("key").data === 42;
			var setOk = cloned.set.has(2);
			var dateOk = cloned.date.getTime() === 1000000000000;
			var regexOk = cloned.regex.test("TEST");

			return arrayDiff && mapDiff && setDiff && dateDiff && regexDiff &&
				   arrayOk && mapOk && setOk && dateOk && regexOk;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ToBoolean() != true {
		t.Errorf("complex nested cloning failed")
	}
}

// TestStructuredClone_ArrayWithCircularMap tests arrays containing maps with circular refs.
func TestStructuredClone_ArrayWithCircularMap(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var arr = [1, 2, 3];
			var map = new Map();
			map.set("array", arr);
			arr.push(map); // arr[3] = map, which has arr

			var cloned = structuredClone(arr);

			// Verify structure
			var isArray = Array.isArray(cloned);
			var hasMap = cloned[3] instanceof Map;

			// Verify circular: cloned[3].get("array") should be cloned itself
			var circular = cloned[3].get("array") === cloned;

			return isArray && hasMap && circular;
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ToBoolean() != true {
		t.Errorf("array with circular map cloning failed")
	}
}

// containsSubstring is a helper to check if a string contains a substring.
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
