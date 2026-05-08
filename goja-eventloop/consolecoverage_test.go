package gojaeventloop

import (
	"bytes"
	"strings"
	"testing"
)

func TestPhase2_ConsoleTable_FormatAllTypes(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.consoleOutput = &buf
	// Array of mixed-type entries exercises the "Values" column path
	// when items are not objects
	_, err := adapter.runtime.RunString(`
		console.table([
			"hello",
			42,
			3.14,
			true,
			null,
			[1, 2, 3],
			{nested: "obj"}
		]);
	`)
	if err != nil {
		t.Fatalf("console.table mixed types failed: %v", err)
	}
	output := buf.String()
	// Verify some expected cell values
	if !strings.Contains(output, "hello") {
		t.Error("expected 'hello' in output")
	}
	if !strings.Contains(output, "42") {
		t.Error("expected '42' in output")
	}
	if !strings.Contains(output, "3.14") {
		t.Error("expected '3.14' in output")
	}
	if !strings.Contains(output, "true") {
		t.Error("expected 'true' in output")
	}
	if !strings.Contains(output, "null") {
		t.Error("expected 'null' in output")
	}
}

// console.table with object data, nested object values
func TestPhase2_ConsoleTable_ObjectWithNestedValues(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.consoleOutput = &buf
	_, err := adapter.runtime.RunString(`
		console.table({
			row1: "simple string",
			row2: 99.5,
			row3: true,
			row4: null,
			row5: [10, 20],
			row6: {inner: "val"}
		});
	`)
	if err != nil {
		t.Fatalf("console.table object with nested values failed: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "row1") {
		t.Error("expected 'row1' in output")
	}
}

// console.table with array of objects that have sub-array/sub-object values
func TestPhase2_ConsoleTable_ArrayOfObjectsWithArrayValues(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.consoleOutput = &buf
	_, err := adapter.runtime.RunString(`
		console.table([
			{name: "Alice", scores: [100, 95], meta: {rank: 1}},
			{name: "Bob", scores: [80, 85], meta: {rank: 2}}
		]);
	`)
	if err != nil {
		t.Fatalf("console.table array of objects with arrays failed: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "Alice") {
		t.Error("expected 'Alice' in output")
	}
	// scores should show as Array(2) and meta as Object
	if !strings.Contains(output, "Array(2)") {
		t.Error("expected 'Array(2)' in output")
	}
	if !strings.Contains(output, "Object") {
		t.Error("expected 'Object' in output")
	}
}

func TestPhase2_ConsoleGroupEnd_AtZero(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.consoleOutput = &buf
	_, err := adapter.runtime.RunString(`
		// Call groupEnd without any group - should not go negative
		console.groupEnd();
		console.groupEnd();
		console.trace("still works");
	`)
	if err != nil {
		t.Fatalf("console.groupEnd at zero failed: %v", err)
	}
	if !strings.Contains(buf.String(), "still works") {
		t.Error("console.trace should still work after excess groupEnd")
	}
}

func TestPhase2_ConsoleTrace_NilOutput(t *testing.T) {
	adapter := coverSetup(t)
	adapter.consoleOutput = nil
	// Should not panic when output is nil
	_, err := adapter.runtime.RunString(`
		console.trace("test");
		console.dir("test");
		console.table([1, 2]);
		console.group("g");
		console.groupEnd();
	`)
	if err != nil {
		t.Fatalf("console with nil output failed: %v", err)
	}
}

func TestPhase2_ConsoleTrace(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.consoleOutput = &buf
	_, err := adapter.runtime.RunString(`
		console.trace("trace message");
	`)
	if err != nil {
		t.Fatalf("console.trace failed: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "Trace: trace message") {
		t.Error("expected trace output")
	}
}

func TestPhase2_ConsoleTrace_NoMessage(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.consoleOutput = &buf
	_, err := adapter.runtime.RunString(`
		console.trace();
	`)
	if err != nil {
		t.Fatalf("console.trace without message failed: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "Trace") {
		t.Error("expected Trace output")
	}
}

func TestPhase2_ConsoleDir_NoArgs(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.consoleOutput = &buf
	_, err := adapter.runtime.RunString(`
		console.dir();
	`)
	if err != nil {
		t.Fatalf("console.dir no args failed: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "undefined") {
		t.Error("expected 'undefined' in output")
	}
}

func TestPhase2_ConsoleClear(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.consoleOutput = &buf
	_, err := adapter.runtime.RunString(`
		console.clear();
	`)
	if err != nil {
		t.Fatalf("console.clear failed: %v", err)
	}
	output := buf.String()
	if len(output) == 0 {
		t.Error("expected some output from clear")
	}
}

// console.clear with nil output
func TestPhase2_ConsoleClear_NilOutput(t *testing.T) {
	adapter := coverSetup(t)
	adapter.consoleOutput = nil
	_, err := adapter.runtime.RunString(`
		console.clear();
	`)
	if err != nil {
		t.Fatalf("console.clear with nil output failed: %v", err)
	}
}

func TestPhase2_ConsoleCount(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.consoleOutput = &buf
	_, err := adapter.runtime.RunString(`
		console.count("myLabel");
		console.count("myLabel");
		console.count("myLabel");
		console.countReset("myLabel");
		console.count("myLabel");
	`)
	if err != nil {
		t.Fatalf("console.count failed: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "myLabel: 3") {
		t.Error("expected 'myLabel: 3' in output")
	}
	// After reset, should be 1 again
	if !strings.Contains(output, "myLabel: 1") {
		t.Error("expected 'myLabel: 1' after reset")
	}
}

func TestPhase2_ConsoleGroup_Nesting(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.consoleOutput = &buf
	_, err := adapter.runtime.RunString(`
		console.group("Level 1");
		console.trace("Inside level 1");
		console.group("Level 2");
		console.trace("Inside level 2");
		console.groupEnd();
		console.trace("Back to level 1");
		console.groupEnd();
		console.trace("Back to top");
	`)
	if err != nil {
		t.Fatalf("console.group nesting failed: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "Back to top") {
		t.Error("expected 'Back to top' in output")
	}
}

func TestPhase2_ConsoleDir_NullValue(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.consoleOutput = &buf
	_, err := adapter.runtime.RunString(`
		console.dir(null);
	`)
	if err != nil {
		t.Fatalf("console.dir null failed: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "null") {
		t.Error("expected 'null' in output")
	}
}

func TestPhase2_ConsoleTable_Primitive(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.consoleOutput = &buf
	_, err := adapter.runtime.RunString(`
		console.table("just a string");
	`)
	if err != nil {
		t.Fatalf("console.table primitive failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected some output")
	}
}

func TestPhase2_ConsoleTable_EmptyArray(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.consoleOutput = &buf
	_, err := adapter.runtime.RunString(`
		console.table([]);
	`)
	if err != nil {
		t.Fatalf("console.table empty array failed: %v", err)
	}
}

func TestPhase2_ConsoleTable_Null(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.consoleOutput = &buf
	_, err := adapter.runtime.RunString(`
		console.table(null);
	`)
	if err != nil {
		t.Fatalf("console.table null failed: %v", err)
	}
}

func TestPhase2_ConsoleTable_WithColumns(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.consoleOutput = &buf
	_, err := adapter.runtime.RunString(`
		console.table([{a: 1, b: 2, c: 3}, {a: 4, b: 5, c: 6}], ["a", "c"]);
	`)
	if err != nil {
		t.Fatalf("console.table with columns failed: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "a") {
		t.Error("expected 'a' column in output")
	}
}

func TestPhase2_ConsoleDir_DeepObject(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.consoleOutput = &buf
	_, err := adapter.runtime.RunString(`
		console.dir({
			str: "hello",
			num: 42,
			flt: 3.14,
			bool: true,
			arr: [1, "two", {three: 3}],
			nested: { a: { b: 1 } },
			nullVal: null,
			undefVal: undefined
		});
	`)
	if err != nil {
		t.Fatalf("console.dir deep object failed: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "hello") {
		t.Error("expected 'hello' in dir output")
	}
	if !strings.Contains(output, "42") {
		t.Error("expected '42' in dir output")
	}
}

func TestPhase2_ConsoleTime_Full(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.consoleOutput = &buf
	_, err := adapter.runtime.RunString(`
		console.time("op");
		console.timeLog("op", "checkpoint");
		console.timeEnd("op");
		// timeEnd for non-started timer
		console.timeEnd("nonexistent");
		// timeLog for non-started timer
		console.timeLog("nonexistent");
	`)
	if err != nil {
		t.Fatalf("console.time full failed: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "op") {
		t.Error("expected timer output")
	}
}

func TestPhase2_Console_Table_ColumnFilter_Array(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)
	_, err := adapter.runtime.RunString(`
		console.table([{name: "Alice", age: 30, city: "NYC"}, {name: "Bob", age: 25, city: "LA"}], ["name", "age"]);
	`)
	if err != nil {
		t.Fatalf("console.table column filter failed: %v", err)
	}
	out := buf.String()
	// Column filter should include name and age but NOT city
	if !strings.Contains(out, "name") {
		t.Errorf("expected 'name' column in output: %s", out)
	}
	if !strings.Contains(out, "age") {
		t.Errorf("expected 'age' column in output: %s", out)
	}
	if strings.Contains(out, "city") {
		t.Errorf("'city' should be filtered out: %s", out)
	}
}

func TestPhase2_Console_Table_ColumnFilter_Object(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)
	_, err := adapter.runtime.RunString(`
		console.table({row1: {x: 10, y: 20, z: 30}, row2: {x: 40, y: 50, z: 60}}, ["x", "z"]);
	`)
	if err != nil {
		t.Fatalf("console.table column filter object failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "x") {
		t.Errorf("expected 'x' column: %s", out)
	}
	// y should not appear as column header
	// (it may appear in index column as "(index)" but not as data column heading)
	if strings.Contains(out, "| y ") {
		t.Errorf("'y' should be filtered out: %s", out)
	}
}

func TestPhase2_Console_Table_NonObjectValues(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)
	_, err := adapter.runtime.RunString(`
		console.table(["hello", 42, true, null]);
	`)
	if err != nil {
		t.Fatalf("console.table non-object values failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Values") {
		t.Errorf("expected 'Values' column: %s", out)
	}
}

func TestPhase2_Console_Table_NestedArray(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)
	_, err := adapter.runtime.RunString(`
		console.table([{arr: [1,2,3], obj: {a: 1}}]);
	`)
	if err != nil {
		t.Fatalf("console.table nested failed: %v", err)
	}
	out := buf.String()
	// Array values should show as "Array(3)" in table
	if !strings.Contains(out, "Array(3)") {
		t.Errorf("expected 'Array(3)' in output: %s", out)
	}
	// Object values should show as "Object"
	if !strings.Contains(out, "Object") {
		t.Errorf("expected 'Object' in output: %s", out)
	}
}

func TestPhase2_Console_Table_Primitive(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)
	_, err := adapter.runtime.RunString(`
		console.table("just a string");
	`)
	if err != nil {
		t.Fatalf("console.table primitive failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "just a string") {
		t.Errorf("expected primitive string in output: %s", out)
	}
}

func TestPhase2_Console_Trace_NilOutput(t *testing.T) {
	adapter := coverSetup(t)
	adapter.SetConsoleOutput(nil)
	// Should not panic when output is nil
	_, err := adapter.runtime.RunString(`
		console.trace("should not panic");
	`)
	if err != nil {
		t.Fatalf("console.trace nil output failed: %v", err)
	}
}

func TestPhase2_Console_Clear_NilOutput(t *testing.T) {
	adapter := coverSetup(t)
	adapter.SetConsoleOutput(nil)
	_, err := adapter.runtime.RunString(`
		console.clear();
	`)
	if err != nil {
		t.Fatalf("console.clear nil output failed: %v", err)
	}
}

func TestPhase2_Console_GroupEnd_AtZeroRepeat(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)
	_, err := adapter.runtime.RunString(`
		// Call groupEnd multiple times at zero indentation
		console.groupEnd();
		console.groupEnd();
		console.groupEnd();
		// Then group/groupEnd to verify correct behavior
		console.group("test");
		console.dir({inside: true});
		console.groupEnd();
		console.dir({outside: true});
	`)
	if err != nil {
		t.Fatalf("console.groupEnd at zero failed: %v", err)
	}
}

func TestPhase2_Console_Dir_DeepObject(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)
	_, err := adapter.runtime.RunString(`
		console.dir({
			str: "hello",
			num: 3.14,
			int: 42,
			bool: true,
			arr: [1, 2, 3],
			nested: {a: {b: {c: "deep"}}},
			empty_obj: {},
			empty_arr: [],
			null_val: null
		});
	`)
	if err != nil {
		t.Fatalf("console.dir deep failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "hello") {
		t.Errorf("expected 'hello' in dir output: %s", out)
	}
	if !strings.Contains(out, "3.14") {
		t.Errorf("expected '3.14' in dir output: %s", out)
	}
}

func TestPhase2_Console_Table_Object_NonObjectValues(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)
	_, err := adapter.runtime.RunString(`
		console.table({a: 1, b: "hello", c: true});
	`)
	if err != nil {
		t.Fatalf("console.table object non-object values failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Values") {
		t.Errorf("expected 'Values' column: %s", out)
	}
}

func TestPhase2_Console_Table_FormatStrings(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)
	// Exercise console.table with boolean and float values in cells
	_, err := adapter.runtime.RunString(`
		console.table([{flag: true, score: 3.14, count: 42}]);
	`)
	if err != nil {
		t.Fatalf("console.table format strings failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "true") {
		t.Errorf("expected 'true' in output: %s", out)
	}
	if !strings.Contains(out, "3.14") {
		t.Errorf("expected '3.14' in output: %s", out)
	}
}

func TestPhase2_Console_Assert(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)
	_, err := adapter.runtime.RunString(`
		console.assert(true, "this should not appear");
		console.assert(false, "assertion failed message");
		console.assert(false); // no message
	`)
	if err != nil {
		t.Fatalf("console.assert failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Assertion failed") {
		t.Errorf("expected assertion failed: %s", out)
	}
}

func TestPhase2_Console_CountReset(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)
	_, err := adapter.runtime.RunString(`
		console.count("myCounter");
		console.count("myCounter");
		console.countReset("myCounter");
		console.count("myCounter"); // should be back to 1
	`)
	if err != nil {
		t.Fatalf("console.countReset failed: %v", err)
	}
	out := buf.String()
	// After reset, the count should restart at 1
	if !strings.Contains(out, "myCounter: 1") {
		t.Errorf("expected reset counter output: %s", out)
	}
}

func TestPhase2_Console_Trace_Anonymous(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)
	_, err := adapter.runtime.RunString(`
		// Call trace from top level (no function name)
		console.trace();
	`)
	if err != nil {
		t.Fatalf("console.trace anonymous failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Trace") {
		t.Errorf("expected 'Trace' in output: %s", out)
	}
}

func TestPhase2_Console_Trace_WithMessage(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)
	_, err := adapter.runtime.RunString(`
		console.trace("my trace message");
	`)
	if err != nil {
		t.Fatalf("console.trace with message failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "my trace message") {
		t.Errorf("expected trace message: %s", out)
	}
}

func TestPhase2_Console_Dir_MaxDepth(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)
	_, err := adapter.runtime.RunString(`
		console.dir({a: {b: {c: {d: {e: {f: "deep"}}}}}}, {depth: 2});
	`)
	if err != nil {
		t.Fatalf("console.dir max depth failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Object") {
		t.Errorf("expected 'Object' truncation: %s", out)
	}
}

func TestPhase2_Console_Assert_FalsyValues(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)
	_, err := adapter.runtime.RunString(`
		console.assert(0, "zero is falsy");
		console.assert("", "empty string is falsy");
		console.assert(null, "null is falsy");
		console.assert(undefined, "undefined is falsy");
		console.assert(1, "one is truthy - should NOT print");
	`)
	if err != nil {
		t.Fatalf("console.assert falsy values failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "zero is falsy") {
		t.Errorf("expected zero assertion: %s", out)
	}
}

func TestPhase2_Console_Table_EmptyArray2(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)
	_, err := adapter.runtime.RunString(`
		console.table([]);
		console.table({});
	`)
	if err != nil {
		t.Fatalf("console.table empty data failed: %v", err)
	}
}

// TestPhase2_Console_Trace_FromAnonymous exercises the funcName==""
// path in console.trace (lines 2016-2018) by calling from an anonymous function.
func TestPhase2_Console_Trace_FromAnonymous(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		// Call console.trace from various anonymous contexts
		(function() { console.trace(); })();

		// Arrow function (also anonymous in goja)
		var fn = () => { console.trace("arrow"); };
		fn();

		// Nested anonymous
		(function() {
			(function() {
				console.trace("nested");
			})();
		})();
	`)
	if err != nil {
		t.Fatalf("Console trace from anonymous failed: %v", err)
	}
}
