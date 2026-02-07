// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package gojaeventloop

import (
	"bytes"
	"strings"
	"testing"
)

// --- wrapEvent / wrapEventWithObject coverage (0% → high) ---

func TestWrapEvent_AllAccessors(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	// Exercise every accessor property and method on a wrapped Event object
	result, err := adapter.runtime.RunString(`
		var e = new Event('click', { bubbles: true, cancelable: true });
		var results = {};
		results.type = e.type;
		results.bubbles = e.bubbles;
		results.cancelable = e.cancelable;
		results.defaultPrevented = e.defaultPrevented;
		results.target = e.target; // null for non-dispatched events
		e.preventDefault();
		results.defaultPreventedAfter = e.defaultPrevented;
		e.stopPropagation();
		e.stopImmediatePropagation();
		JSON.stringify(results);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	s := result.String()
	if s == "" {
		t.Fatal("Empty result")
	}
	t.Logf("Event accessors result: %s", s)
}

func TestWrapEvent_TargetNullForNonDispatched(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		var e = new Event('test');
		e.target === null;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected e.target === null for non-dispatched event")
	}
}

func TestWrapEvent_DispatchFiresListener(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		var target = new EventTarget();
		var received = null;
		target.addEventListener('test', function(e) {
			received = {
				type: e.type,
				bubbles: e.bubbles,
				cancelable: e.cancelable,
				defaultPrevented: e.defaultPrevented
			};
		});
		target.dispatchEvent(new Event('test', { bubbles: true, cancelable: true }));
		JSON.stringify(received);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	s := result.String()
	t.Logf("Dispatch result: %s", s)
	if s == "null" || s == "" {
		t.Error("Listener was not called")
	}
}

func TestWrapEvent_PreventDefault(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		var target = new EventTarget();
		var prevented = false;
		target.addEventListener('test', function(e) {
			e.preventDefault();
			prevented = e.defaultPrevented;
		});
		target.dispatchEvent(new Event('test', { cancelable: true }));
		prevented;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected defaultPrevented to be true after preventDefault()")
	}
}

// --- exportGojaValue coverage (42.9% → higher) ---

func TestExportGojaValue_NilValue(t *testing.T) {
	val, ok := exportGojaValue(nil)
	if ok || val != nil {
		t.Errorf("Expected (nil, false) for nil input, got (%v, %v)", val, ok)
	}
}

func TestExportGojaValue_UndefinedValue(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	undef := adapter.runtime.GlobalObject().Get("undefined")
	val, ok := exportGojaValue(undef)
	if ok || val != nil {
		t.Errorf("Expected (nil, false) for undefined, got (%v, %v)", val, ok)
	}
}

func TestExportGojaValue_NullValue(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	null := adapter.runtime.ToValue(nil)
	val, ok := exportGojaValue(null)
	if ok || val != nil {
		t.Errorf("Expected (nil, false) for null, got (%v, %v)", val, ok)
	}
}

func TestExportGojaValue_ErrorObject(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	errObj, err := adapter.runtime.RunString(`new Error("test error")`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val, ok := exportGojaValue(errObj)
	if !ok {
		t.Error("Expected ok=true for Error object")
	}
	if val == nil {
		t.Error("Expected non-nil value for Error object")
	}
}

func TestExportGojaValue_TypeError(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	errObj, err := adapter.runtime.RunString(`new TypeError("type error")`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val, ok := exportGojaValue(errObj)
	if !ok {
		t.Error("Expected ok=true for TypeError")
	}
	if val == nil {
		t.Error("Expected non-nil value for TypeError")
	}
}

func TestExportGojaValue_RangeError(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	errObj, err := adapter.runtime.RunString(`new RangeError("range error")`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val, ok := exportGojaValue(errObj)
	if !ok {
		t.Error("Expected ok=true for RangeError")
	}
	if val == nil {
		t.Error("Expected non-nil value for RangeError")
	}
}

func TestExportGojaValue_ReferenceError(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	errObj, err := adapter.runtime.RunString(`new ReferenceError("ref error")`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val, ok := exportGojaValue(errObj)
	if !ok {
		t.Error("Expected ok=true for ReferenceError")
	}
	if val == nil {
		t.Error("Expected non-nil value for ReferenceError")
	}
}

func TestExportGojaValue_PlainObject(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	obj, err := adapter.runtime.RunString(`({name: "not an error"})`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	_, ok := exportGojaValue(obj)
	if ok {
		t.Error("Expected ok=false for plain object with 'name' property that isn't an error type")
	}
}

func TestExportGojaValue_PrimitiveString(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	str := adapter.runtime.ToValue("hello")
	_, ok := exportGojaValue(str)
	if ok {
		t.Error("Expected ok=false for primitive string")
	}
}

func TestExportGojaValue_PrimitiveNumber(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	num := adapter.runtime.ToValue(42)
	_, ok := exportGojaValue(num)
	if ok {
		t.Error("Expected ok=false for primitive number")
	}
}

// --- formatCellValue coverage (exercising nested objects and edge cases) ---

func TestFormatCellValue_EdgeCases(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	// Test console.table with various types to exercise formatCellValue branches
	_, err := adapter.runtime.RunString(`
		// Objects with nested structures
		console.table([
			{ name: "test", value: { nested: true } },
			{ name: "arr", value: [1, 2, 3] },
			{ name: "null", value: null },
			{ name: "undef", value: undefined },
			{ name: "num", value: 42 },
			{ name: "bool", value: true },
			{ name: "str", value: "hello" },
		]);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Expected console.table to produce output")
	}
}

// --- inspectValue coverage (deep object inspection) ---

func TestInspectValue_CircularReference(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	// Test console.dir with circular references
	// After Export(), circular refs become nil in Go, so inspectValue handles map with nil values
	_, err := adapter.runtime.RunString(`
		var obj = { name: "circular" };
		obj.self = obj;
		console.dir(obj);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Expected console.dir to produce output")
	}
	t.Logf("inspectValue output for circular ref: %s", buf.String())
}

func TestInspectValue_DeeplyNested(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		var deep = { a: { b: { c: { d: "deep" } } } };
		console.dir(deep);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Expected console.dir to produce output")
	}
	output := buf.String()
	// inspectValue with maxDepth=2 truncates nested objects beyond depth 2 to "Object"
	if !strings.Contains(output, "a:") || !strings.Contains(output, "b:") {
		t.Errorf("Expected output to contain nested keys, got: %s", output)
	}
	t.Logf("inspectValue output for deep nesting: %s", output)
}

// --- generateTableFromObject coverage ---

func TestGenerateTableFromObject_WithSparseData(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		// Object with mixed keys and missing values
		console.table({
			row1: { a: 1, b: 2 },
			row2: { a: 3, c: 4 },
			row3: { b: 5, c: 6 },
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Expected console.table to produce output")
	}
	output := buf.String()
	// Verify the table contains our row keys
	if !strings.Contains(output, "row1") || !strings.Contains(output, "row2") || !strings.Contains(output, "row3") {
		t.Errorf("Expected output to contain row keys, got: %s", output)
	}
	t.Logf("console.table output:\n%s", output)
}

// --- isErrorObject / isSetObject coverage ---

func TestIsErrorObject_ViaStructuredClone(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	// isErrorObject is exercised via structuredClone — Error objects cannot be cloned
	// per spec, and cloneObject checks isErrorObject before throwing TypeError.
	for _, errorType := range []string{"Error", "TypeError", "RangeError", "ReferenceError"} {
		result, err := adapter.runtime.RunString(`
			(function() {
				try {
					structuredClone(new ` + errorType + `("test"));
					return "no error";
				} catch (e) {
					return e.message;
				}
			})()
		`)
		if err != nil {
			t.Fatalf("RunString failed for %s: %v", errorType, err)
		}
		// Should throw a TypeError about not being able to clone Error objects
		if result.String() == "no error" {
			t.Errorf("Expected structuredClone(%s) to throw, but it didn't", errorType)
		}
		t.Logf("structuredClone(%s) threw: %s", errorType, result.String())
	}
}

func TestIsSetObject_ViaStructuredClone(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	// isSetObject is exercised via structuredClone — Set objects go through cloneSet path
	result, err := adapter.runtime.RunString(`
		var original = new Set([1, 2, 3]);
		var cloned = structuredClone(original);
		// Verify the clone is a distinct Set with same values
		cloned.has(1) && cloned.has(2) && cloned.has(3) && cloned.size === 3;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected structuredClone(Set) to produce a valid clone")
	}

	// Also test Map cloning (isMapObject path)
	result2, err := adapter.runtime.RunString(`
		var origMap = new Map([["a", 1], ["b", 2]]);
		var clonedMap = structuredClone(origMap);
		clonedMap.get("a") === 1 && clonedMap.get("b") === 2 && clonedMap.size === 2;
	`)
	if err != nil {
		t.Fatalf("RunString failed for Map: %v", err)
	}
	if !result2.ToBoolean() {
		t.Error("Expected structuredClone(Map) to produce a valid clone")
	}
}

// --- bindSymbol coverage ---

func TestBindSymbol_SymbolFor(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		var s1 = Symbol.for('test');
		var s2 = Symbol.for('test');
		s1 === s2; // Should be true - same registry key
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected Symbol.for to return same symbol for same key")
	}
}

func TestBindSymbol_SymbolKeyFor(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		var s = Symbol.for('myKey');
		Symbol.keyFor(s);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if result.String() != "myKey" {
		t.Errorf("Expected 'myKey', got %q", result.String())
	}
}

func TestBindSymbol_KeyForUnregistered(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		var s = Symbol('unregistered');
		Symbol.keyFor(s) === undefined;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected undefined for unregistered symbol")
	}
}
