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

	// isErrorObject is exercised via structuredClone — Error objects are cloned
	// with name/message preserved.
	for _, errorType := range []string{"Error", "TypeError", "RangeError", "ReferenceError"} {
		result, err := adapter.runtime.RunString(`
			(function() {
				var original = new ` + errorType + `("test");
				var cloned = structuredClone(original);
				return cloned !== original && cloned.name === original.name && cloned.message === "test";
			})()
		`)
		if err != nil {
			t.Fatalf("RunString failed for %s: %v", errorType, err)
		}
		if !result.ToBoolean() {
			t.Errorf("Expected structuredClone(%s) to preserve Error details", errorType)
		}
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
