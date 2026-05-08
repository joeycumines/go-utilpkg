package gojaeventloop

import (
	"bytes"
	"strings"
	"testing"
)

// ===========================================================================
// formatCellValue — 61.5% coverage (exercise more type branches)
// ===========================================================================

func TestConsoleTable_VariousTypes(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	// Exercise int, bool, array, object, float, null in table cells
	_, err := adapter.runtime.RunString(`
		console.table([
			{ name: "str", value: "hello" },
			{ name: "bool", value: true },
			{ name: "int", value: 42 },
			{ name: "float", value: 3.14 },
			{ name: "null", value: null },
			{ name: "arr", value: [1, 2, 3] },
			{ name: "obj", value: { a: 1 } }
		]);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Error("Expected non-empty table output")
	}
	// Verify some expected content in the table
	if !strings.Contains(output, "hello") {
		t.Error("Expected 'hello' in table output")
	}
	if !strings.Contains(output, "true") {
		t.Error("Expected 'true' in table output")
	}
	if !strings.Contains(output, "null") {
		t.Error("Expected 'null' in table output")
	}
}

func TestConsoleTable_ObjectData(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.table({
			alice: { age: 30, role: "admin" },
			bob: { age: 25, role: "user" }
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "alice") {
		t.Error("Expected 'alice' in table output")
	}
	if !strings.Contains(output, "admin") {
		t.Error("Expected 'admin' in table output")
	}
}

func TestConsoleTable_ColumnFilter(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.table([
			{ name: "a", age: 1, role: "admin" },
			{ name: "b", age: 2, role: "user" }
		], ["name", "role"]);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "admin") {
		t.Error("Expected 'admin' in filtered table output")
	}
}

func TestConsoleTable_NullUndefined_CoverGap(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.table(null);
		console.table(undefined);
		console.table();
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestConsoleTable_Primitive(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.table("hello");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "hello") {
		t.Error("Expected 'hello' in output")
	}
}

func TestConsoleTable_EmptyArray(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`console.table([])`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestConsoleTable_EmptyObject(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`console.table({})`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestConsoleTable_NonObjectItems(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.table([1, "two", true, null]);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Values") {
		t.Error("Expected 'Values' column for non-object items")
	}
}

func TestConsoleTable_ObjectFilterNonExistent(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.table({
			a: { x: 1 },
			b: { x: 2 }
		}, ["nonexistent"]);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestConsoleTable_NilOutput_CoverGap(t *testing.T) {
	adapter := coverSetup(t)
	adapter.SetConsoleOutput(nil)

	_, err := adapter.runtime.RunString(`console.table([{a:1}])`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestConsoleTable_ObjectNonNestedValues(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.table({ a: 1, b: "two", c: true });
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Values") {
		t.Error("Expected 'Values' column for non-nested object")
	}
}

// ===========================================================================
// inspectValue — 75% coverage (exercise more branches)
// ===========================================================================

func TestConsoleDir_NestedObject(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.dir({
			arr: [1, 2, 3],
			obj: { nested: true },
			str: "hello",
			num: 42,
			flag: true,
			nil: null,
			deep: { a: { b: { c: 1 } } }
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "'hello'") {
		t.Errorf("Expected quoted string in dir output, got: %s", output)
	}
}

func TestConsoleDir_EmptyObject(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`console.dir({})`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if !strings.Contains(buf.String(), "{}") {
		t.Error("Expected '{}' for empty object")
	}
}

func TestConsoleDir_EmptyArray(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`console.dir([])`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if !strings.Contains(buf.String(), "[]") {
		t.Error("Expected '[]' for empty array")
	}
}

func TestConsoleDir_DeeplyNested(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	// maxDepth is 2 by default for inspectValue; depth>=maxDepth triggers "Object"/"Array(N)"
	_, err := adapter.runtime.RunString(`
		console.dir({ a: { b: { c: { d: 1 } } } });
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Object") {
		t.Error("Expected 'Object' for deeply nested obj beyond maxDepth")
	}
}

func TestConsoleDir_ArrayAtMaxDepth(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.dir({ a: { b: [1, 2, 3] } });
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Array(3)") {
		t.Error("Expected 'Array(3)' for array at maxDepth")
	}
}

func TestConsoleDir_Undefined(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`console.dir(undefined)`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !strings.Contains(buf.String(), "undefined") {
		t.Error("Expected 'undefined'")
	}
}

func TestConsoleDir_Null(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`console.dir(null)`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !strings.Contains(buf.String(), "null") {
		t.Error("Expected 'null'")
	}
}

func TestConsoleDir_NoArgs(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`console.dir()`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !strings.Contains(buf.String(), "undefined") {
		t.Error("Expected 'undefined' for no args")
	}
}

func TestConsoleDir_NilOutput_CoverGap(t *testing.T) {
	adapter := coverSetup(t)
	adapter.SetConsoleOutput(nil)

	_, err := adapter.runtime.RunString(`console.dir({a:1})`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// console group/clear/trace
// ===========================================================================

func TestConsoleGroup_IndentTracking(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.group("outer");
		console.group("inner");
		console.groupEnd();
		console.groupEnd();
		console.groupEnd(); // Extra groupEnd should not go negative
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "outer") {
		t.Error("Expected 'outer' in output")
	}
}

func TestConsoleGroupCollapsed_CoverGap(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.groupCollapsed("collapsed");
		console.groupCollapsed();
		console.groupEnd();
		console.groupEnd();
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestConsoleClear(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`console.clear()`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestConsoleTrace_WithMessage(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`console.trace("myTrace")`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if !strings.Contains(buf.String(), "Trace: myTrace") {
		t.Errorf("Expected 'Trace: myTrace', got: %s", buf.String())
	}
}

func TestConsoleTrace_NoMessage_CoverGap(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`console.trace()`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if !strings.Contains(buf.String(), "Trace") {
		t.Errorf("Expected 'Trace', got: %s", buf.String())
	}
}

func TestConsoleTrace_NilOutput_CoverGap(t *testing.T) {
	adapter := coverSetup(t)
	adapter.SetConsoleOutput(nil)

	_, err := adapter.runtime.RunString(`console.trace("msg")`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestConsoleClear_NilOutput_CoverGap(t *testing.T) {
	adapter := coverSetup(t)
	adapter.SetConsoleOutput(nil)

	_, err := adapter.runtime.RunString(`console.clear()`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestConsoleGroup_NilOutput(t *testing.T) {
	adapter := coverSetup(t)
	adapter.SetConsoleOutput(nil)

	_, err := adapter.runtime.RunString(`
		console.group("test");
		console.groupCollapsed("test2");
		console.groupEnd();
		console.groupEnd();
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// console.timeLog with extra data
// ===========================================================================

func TestConsoleTimeLog_WithExtraData(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.time("test");
		console.timeLog("test", "extra", "data");
		console.timeEnd("test");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "extra") {
		t.Errorf("Expected 'extra' in output, got: %s", output)
	}
}

// ===========================================================================
// console.assert with falsy/truthy
// ===========================================================================

func TestConsoleAssert_Truthy_CoverGap(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.assert(true, "should not print");
		console.assert(1, "also should not print");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if strings.Contains(buf.String(), "Assertion") {
		t.Error("Truthy assert should not produce output")
	}
}

func TestConsoleAssert_NoArgs(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`console.assert()`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !strings.Contains(buf.String(), "Assertion failed") {
		t.Error("No args assert should log failure")
	}
}
