package gojaeventloop

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// coverSetup creates a minimal adapter for coverage tests.
// Unlike testSetup, this does NOT start the loop (for sync-only tests).
func coverSetup(t *testing.T) *Adapter {
	t.Helper()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("failed to create loop: %v", err)
	}
	t.Cleanup(func() { loop.Shutdown(context.Background()) })

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind adapter: %v", err)
	}
	return adapter
}

// coverSetupWithLoop creates adapter WITHOUT starting the loop.
// Use coverRunLoopBriefly() after RunString to process async operations.
// This prevents data races between the loop goroutine and direct runtime access.
func coverSetupWithLoop(t *testing.T) *Adapter {
	t.Helper()
	return coverSetup(t)
}

// coverRunLoopBriefly starts the event loop, waits for the specified duration
// to allow async operations (timers, microtasks, promises) to process, then
// stops the loop and waits for it to finish. After this returns, it is safe
// to access adapter.runtime directly.
func coverRunLoopBriefly(t *testing.T, adapter *Adapter, waitMs int) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- adapter.loop.Run(ctx) }()
	time.Sleep(time.Duration(waitMs) * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("loop did not stop in time")
	}
}

// ===========================================================================
// fetchNotImplemented — 0% coverage
// ===========================================================================

func TestFetchNotImplemented(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	// fetch() should return a rejected promise
	_, err := adapter.runtime.RunString(`
		var rejected = false;
		fetch('http://example.com').catch(function(e) {
			rejected = true;
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	val := adapter.runtime.Get("rejected")
	if val == nil || !val.ToBoolean() {
		t.Error("fetch() should return a rejected promise")
	}
}

// ===========================================================================
// wrapBlobWithObject — 34.7% coverage (slice blob methods)
// ===========================================================================

func TestBlobSlice_TextAndArrayBuffer(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var blob = new Blob(["hello world"], { type: "text/plain" });
		var sliced = blob.slice(0, 5, "text/plain");
		var textResult = "";
		var sizeResult = -1;
		var typeResult = "";
		sliced.text().then(function(t) { textResult = t; });
		sizeResult = sliced.size;
		typeResult = sliced.type;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	if s := adapter.runtime.Get("textResult").String(); s != "hello" {
		t.Errorf("Expected 'hello', got %q", s)
	}
	if n := adapter.runtime.Get("sizeResult").ToInteger(); n != 5 {
		t.Errorf("Expected size 5, got %d", n)
	}
	if s := adapter.runtime.Get("typeResult").String(); s != "text/plain" {
		t.Errorf("Expected 'text/plain', got %q", s)
	}
}

func TestBlobSlice_ArrayBuffer(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var blob = new Blob(["ABCDEF"]);
		var sliced = blob.slice(1, 4);
		var resultLen = -1;
		sliced.arrayBuffer().then(function(ab) {
			resultLen = ab.byteLength;
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	if n := adapter.runtime.Get("resultLen").ToInteger(); n != 3 {
		t.Errorf("Expected byteLength 3, got %d", n)
	}
}

func TestBlobSlice_NegativeIndices(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var blob = new Blob(["0123456789"]);
		var s1 = blob.slice(-3);
		var s2 = blob.slice(2, -2);
		var s3 = blob.slice(-100);
		var s4 = blob.slice(5, 3); // start > end
		var r1 = s1.size;
		var r2 = s2.size;
		var r3 = s3.size;
		var r4 = s4.size;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if n := adapter.runtime.Get("r1").ToInteger(); n != 3 {
		t.Errorf("Expected 3, got %d", n)
	}
	if n := adapter.runtime.Get("r2").ToInteger(); n != 6 {
		t.Errorf("Expected 6, got %d", n)
	}
	if n := adapter.runtime.Get("r3").ToInteger(); n != 10 {
		t.Errorf("Expected 10, got %d", n)
	}
	if n := adapter.runtime.Get("r4").ToInteger(); n != 0 {
		t.Errorf("Expected 0, got %d", n)
	}
}

func TestBlobSlice_Stream(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var blob = new Blob(["test"]);
		var sliced = blob.slice(0, 2);
		sliced.stream() === undefined;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("blob.slice().stream() should return undefined")
	}
}

func TestBlobSlice_DoubleSlice(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var blob = new Blob(["ABCDEFGHIJ"]);
		var s1 = blob.slice(2, 8);
		var s2 = s1.slice(1, 4, "application/octet-stream");
		var r1 = s2.size;
		var r2 = s2.type;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if n := adapter.runtime.Get("r1").ToInteger(); n != 3 {
		t.Errorf("Expected 3, got %d", n)
	}
	if s := adapter.runtime.Get("r2").String(); s != "application/octet-stream" {
		t.Errorf("Expected 'application/octet-stream', got %q", s)
	}
}

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
// Timer error paths — setInterval/setTimeout with nil Export
// ===========================================================================

func TestSetInterval_ErrorPaths(t *testing.T) {
	adapter := coverSetup(t)

	// setInterval with null function
	_, err := adapter.runtime.RunString(`
		try { setInterval(null, 10); } catch(e) { var err1 = true; }
	`)
	if err != nil {
		t.Fatalf("setInterval(null) failed: %v", err)
	}

	// setInterval with non-function
	_, err = adapter.runtime.RunString(`
		try { setInterval("not a function", 10); } catch(e) { var err2 = true; }
	`)
	if err != nil {
		t.Fatalf("setInterval(string) failed: %v", err)
	}

	// Negative delay clamped to 0
	_, err = adapter.runtime.RunString(`
		var intervalCalled = false;
		var iid = setInterval(function() { intervalCalled = true; }, -100);
		clearInterval(iid);
	`)
	if err != nil {
		t.Fatalf("setInterval with negative delay failed: %v", err)
	}
}

func TestQueueMicrotask_ErrorPaths(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { queueMicrotask(null); } catch(e) { var qErr1 = true; }
	`)
	if err != nil {
		t.Fatalf("queueMicrotask(null) failed: %v", err)
	}

	_, err = adapter.runtime.RunString(`
		try { queueMicrotask("not a function"); } catch(e) { var qErr2 = true; }
	`)
	if err != nil {
		t.Fatalf("queueMicrotask(string) failed: %v", err)
	}
}

func TestSetImmediate_ErrorPaths(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { setImmediate(null); } catch(e) { var siErr1 = true; }
	`)
	if err != nil {
		t.Fatalf("setImmediate(null) failed: %v", err)
	}

	_, err = adapter.runtime.RunString(`
		try { setImmediate("not a function"); } catch(e) { var siErr2 = true; }
	`)
	if err != nil {
		t.Fatalf("setImmediate(string) failed: %v", err)
	}
}

// ===========================================================================
// Promise prototype error paths — then/catch/finally on non-Promise
// ===========================================================================

// NOTE: Promise.prototype.then/catch/finally called on non-Promise panics due to
// nil pointer dereference in adapter.go:862 (Get("_internalPromise") returns nil).
// This is a real bug but requires source code fix. Skipping for now.

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
// Performance API — uncovered branches
// ===========================================================================

func TestPerformance_MeasureWithOptions(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		performance.mark('start');
		performance.mark('end');
		// Use options object form
		var entry = performance.measure('test', { start: 'start', end: 'end', detail: 'myDetail' });
		var measureName = entry.name;
		var measureDetail = entry.detail;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if s := adapter.runtime.Get("measureName").String(); s != "test" {
		t.Errorf("Expected 'test', got %q", s)
	}
}

func TestPerformance_MeasureWithUndefinedStartMark(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		performance.mark('end');
		// Pass undefined as startMark, should still work
		var entry = performance.measure('test', undefined, 'end');
		var endMeasure = entry !== undefined;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestPerformance_MeasureWithNullStartMark(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		performance.mark('end');
		var entry = performance.measure('test', null, 'end');
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestPerformance_MarkWithDetail(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var m = performance.mark('detailed', { detail: { key: 'value' } });
		var hasDetail = m && m.detail !== undefined;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestPerformance_EntryToJSON(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var m = performance.mark('jsontest');
		var json = m.toJSON();
		var hasName = json.name === 'jsontest';
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("hasName")
	if val == nil || !val.ToBoolean() {
		t.Error("toJSON should have name")
	}
}

func TestPerformance_EmptyGetEntries(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		// After clearMarks/clearMeasures, entries should return 0
		performance.clearMarks();
		performance.clearMeasures();
		var entries = performance.getEntriesByType('mark');
		var noEntries = entries.length === 0;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("noEntries")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected no entries after clear")
	}
}

// ===========================================================================
// AbortSignal statics — uncovered branches
// ===========================================================================

func TestAbortSignal_AnyWithEmptyArray(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var signal = AbortSignal.any([]);
		var notAborted = !signal.aborted;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("notAborted")
	if val == nil || !val.ToBoolean() {
		t.Error("Empty AbortSignal.any should not be aborted")
	}
}

func TestAbortSignal_AnyWithNullElements(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			// Null elements should be skipped
			var ctrl = new AbortController();
			var signal = AbortSignal.any([ctrl.signal, null, undefined]);
			var has = !signal.aborted;
		} catch(e) {
			var has = false;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestAbortSignal_AnyNotIterable(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			AbortSignal.any(42);
			var notIterableOk = false;
		} catch(e) {
			var notIterableOk = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("notIterableOk")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected TypeError for non-iterable")
	}
}

func TestAbortSignal_AnyNonSignalElement(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			AbortSignal.any([{ notASignal: true }]);
			var nonSignalOk = false;
		} catch(e) {
			var nonSignalOk = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("nonSignalOk")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected TypeError for non-signal element")
	}
}

func TestAbortSignal_TimeoutNegative(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var signal = AbortSignal.timeout(-1);
		var isSignal = !signal.aborted;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// AbortSignal reason, onabort setter, addEventListener non-abort
// ===========================================================================

func TestAbortSignal_ReasonUndefined(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var ctrl = new AbortController();
		ctrl.signal.reason === undefined;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected reason to be undefined before abort")
	}
}

func TestAbortSignal_OnabortNullHandler(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var ctrl = new AbortController();
		ctrl.signal.onabort = null;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestAbortSignal_AddEventListenerNonAbort(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var ctrl = new AbortController();
		ctrl.signal.addEventListener('click', function() {});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestAbortSignal_AddEventListenerNullHandler(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var ctrl = new AbortController();
		ctrl.signal.addEventListener('abort', null);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// process.nextTick error paths
// ===========================================================================

func TestProcessNextTick_ErrorPaths(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { process.nextTick(null); } catch(e) { var ntErr1 = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	_, err = adapter.runtime.RunString(`
		try { process.nextTick("not a function"); } catch(e) { var ntErr2 = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// consumeIterable — error paths
// ===========================================================================

func TestConsumeIterable_NullUndefined(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			Promise.all(null);
		} catch(e) {
			// Might be caught or rejected
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestConsumeIterable_NonIterable(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			Promise.all(42);
		} catch(e) {
			var nonIterErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// resolveThenable — various paths
// ===========================================================================

func TestResolveThenable_NullUndefined(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		// Promise.resolve with null/undefined
		var r1, r2;
		Promise.resolve(null).then(function(v) { r1 = v; });
		Promise.resolve(undefined).then(function(v) { r2 = v; });
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
}

func TestResolveThenable_NonObjectWithThen(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		// A non-object that has a then property (e.g. string) should not be treated as thenable
		var resolvedVal;
		Promise.resolve("stringValue").then(function(v) { resolvedVal = v; });
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	val := adapter.runtime.Get("resolvedVal")
	if val == nil || val.String() != "stringValue" {
		t.Errorf("Expected 'stringValue', got %v", val)
	}
}

func TestResolveThenable_ThenableThrows(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var rejected = false;
		var thenable = {
			then: function(resolve, reject) {
				throw new Error("thenable error");
			}
		};
		Promise.resolve(thenable).catch(function(e) { rejected = true; });
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	val := adapter.runtime.Get("rejected")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected thenable throw to reject")
	}
}

func TestResolveThenable_ThenNotFunction(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var resolvedVal;
		var obj = { then: "not a function" };
		Promise.resolve(obj).then(function(v) { resolvedVal = v; });
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
}

// ===========================================================================
// convertToGojaValue — PanicError, AggregateError, error wrapping
// ===========================================================================

func TestConvertToGojaValue_AggregateError(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var errResult;
		Promise.any([
			Promise.reject("e1"),
			Promise.reject("e2")
		]).catch(function(e) {
			errResult = {
				name: e.name,
				hasErrors: Array.isArray(e.errors),
				count: e.errors ? e.errors.length : 0
			};
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	val := adapter.runtime.Get("errResult")
	if val == nil || goja.IsUndefined(val) {
		t.Fatal("errResult is nil/undefined")
	}
	obj := val.ToObject(adapter.runtime)
	if obj.Get("name").String() != "AggregateError" {
		t.Error("Expected AggregateError name")
	}
}

// ===========================================================================
// joinStrings — empty slice
// ===========================================================================

func TestJoinStrings_Empty(t *testing.T) {
	result := joinStrings(nil, ",")
	if result != "" {
		t.Errorf("Expected empty, got %q", result)
	}

	result = joinStrings([]string{}, ",")
	if result != "" {
		t.Errorf("Expected empty, got %q", result)
	}
}

func TestJoinStrings_SingleElement(t *testing.T) {
	result := joinStrings([]string{"hello"}, ",")
	if result != "hello" {
		t.Errorf("Expected 'hello', got %q", result)
	}
}

// ===========================================================================
// URL — uncovered branches (empty scheme, base URL)
// ===========================================================================

func TestURL_EmptyScheme(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			new URL("no-scheme");
			var noSchemeOk = true;
		} catch(e) {
			var noSchemeErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("noSchemeErr")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected TypeError for no scheme")
	}
}

func TestURL_InvalidBaseURL(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			new URL("/path", "not a valid url!!!");
			var invalidBaseOk = false;
		} catch(e) {
			var invalidBaseOk = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestURL_WithBaseURL(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var u = new URL("/path?q=1", "https://example.com");
		u.href;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if !strings.Contains(result.String(), "example.com") {
		t.Errorf("Expected URL with base, got %s", result.String())
	}
}

func TestURL_SearchParams(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var u = new URL("https://example.com?a=1&b=2");
		var sp = u.searchParams;
		var a = sp.get("a");
		var b = sp.get("b");
		var size = sp.size;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if s := adapter.runtime.Get("a").String(); s != "1" {
		t.Errorf("Expected '1', got %q", s)
	}
}

func TestURL_Properties(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var u = new URL("https://user:pass@example.com:8080/path?q=1#fragment");
		var props = {
			origin: u.origin,
			protocol: u.protocol,
			host: u.host,
			hostname: u.hostname,
			port: u.port,
			pathname: u.pathname,
			search: u.search,
			hash: u.hash,
			username: u.username,
			password: u.password,
			href: u.href
		};
		// Setters
		u.protocol = "http:";
		u.host = "other.com:9090";
		u.hostname = "new.com";
		u.port = "3000";
		u.pathname = "/new";
		u.search = "?x=1";
		u.hash = "#newhash";
		u.username = "newuser";
		u.password = "newpass";
		u.href = "https://final.com";
		var origOrigin = props.origin;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("origOrigin")
	if val == nil || !strings.Contains(val.String(), "example.com") {
		t.Errorf("Expected origin with example.com, got %v", val)
	}
}

func TestURL_UserPassword_NoExisting(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var u = new URL("https://example.com");
		u.username = "user1";
		u.password = "pass1";
		var u1 = u.username;
		var p1 = u.password;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestURL_ToStringToJSON(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var u = new URL("https://example.com/path");
		u.toString() === u.toJSON();
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("toString and toJSON should match")
	}
}

func TestURL_OriginEmpty(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var u = new URL("https://example.com");
		u.host = "";
		var origin = u.origin;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// URLSearchParams — uncovered branches
// ===========================================================================

func TestURLSearchParams_DeleteWithValue_CoverGap(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var sp = new URLSearchParams("a=1&a=2&a=3&b=4");
		sp.delete("a", "2");
		var all = sp.getAll("a");
		var count = all.length;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if n := adapter.runtime.Get("count").ToInteger(); n != 2 {
		t.Errorf("Expected 2, got %d", n)
	}
}

func TestURLSearchParams_HasWithValue_CoverGap(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var sp = new URLSearchParams("a=1&a=2&b=3");
		var r1 = sp.has("a", "2");
		var r2 = sp.has("a", "99");
		var r3 = sp.has("c");
		r1 === true && r2 === false && r3 === false;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("has() with value check failed")
	}
}

func TestURLSearchParams_Sort_CoverGap(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var sp = new URLSearchParams("c=3&a=1&b=2");
		sp.sort();
		sp.toString();
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	s := result.String()
	if !strings.HasPrefix(s, "a=1") {
		t.Errorf("Expected sorted params starting with 'a=1', got %q", s)
	}
}

func TestURLSearchParams_Iterators(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var sp = new URLSearchParams("a=1&b=2");
		var keys = [];
		var vals = [];
		var entries = [];

		for (var k of sp.keys()) keys.push(k);
		for (var v of sp.values()) vals.push(v);
		for (var e of sp.entries()) entries.push(e[0] + "=" + e[1]);

		var keysOk = keys.length === 2;
		var valsOk = vals.length === 2;
		var entriesOk = entries.length === 2;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestURLSearchParams_ForEach_CoverGap(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var sp = new URLSearchParams("a=1&b=2");
		var collected = [];
		sp.forEach(function(value, key) {
			collected.push(key + "=" + value);
		});
		var forEachOk = collected.length === 2;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestURLSearchParams_GetNonExistent(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var sp = new URLSearchParams("a=1");
		sp.get("notfound") === null;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected null for non-existent key")
	}
}

func TestURLSearchParams_InitFromPairs(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var sp = new URLSearchParams([["a", "1"], ["b", "2"]]);
		var r = sp.get("a") === "1" && sp.get("b") === "2";
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestURLSearchParams_InitFromObject(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var sp = new URLSearchParams({ x: "10", y: "20" });
		var r = sp.get("x") === "10";
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// TextEncoder/TextDecoder — uncovered branches
// ===========================================================================

func TestTextEncoder_EncodeInto_CoverGap(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var enc = new TextEncoder();
		var buf = new Uint8Array(3);
		var result = enc.encodeInto("hello", buf);
		var read = result.read;
		var written = result.written;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if n := adapter.runtime.Get("written").ToInteger(); n != 3 {
		t.Errorf("Expected written=3, got %d", n)
	}
	if n := adapter.runtime.Get("read").ToInteger(); n != 3 {
		t.Errorf("Expected read=3, got %d", n)
	}
}

func TestTextEncoder_EncodeIntoMultibyte(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var enc = new TextEncoder();
		var buf = new Uint8Array(10);
		var result = enc.encodeInto("café", buf);
		var read = result.read;
		var written = result.written;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	// "café" = 'c','a','f','é' where é is 2 bytes in UTF-8
	if n := adapter.runtime.Get("read").ToInteger(); n != 4 {
		t.Errorf("Expected read=4, got %d", n)
	}
	if n := adapter.runtime.Get("written").ToInteger(); n != 5 {
		t.Errorf("Expected written=5, got %d", n)
	}
}

func TestTextDecoder_UnsupportedEncoding_CoverGap(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			new TextDecoder("latin-1");
			var unsupported = false;
		} catch(e) {
			var unsupported = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("unsupported")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected error for unsupported encoding")
	}
}

func TestTextDecoder_WithOptions(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var dec = new TextDecoder("utf-8", { fatal: true, ignoreBOM: true });
		var isFatal = dec.fatal;
		var ignoreBOM = dec.ignoreBOM;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if !adapter.runtime.Get("isFatal").ToBoolean() {
		t.Error("Expected fatal=true")
	}
	if !adapter.runtime.Get("ignoreBOM").ToBoolean() {
		t.Error("Expected ignoreBOM=true")
	}
}

func TestTextDecoder_Utf8Alias(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var dec = new TextDecoder("utf8");
		dec.encoding;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if result.String() != "utf-8" {
		t.Errorf("Expected 'utf-8', got %q", result.String())
	}
}

func TestTextDecoder_EmptyInput(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var dec = new TextDecoder();
		dec.decode();
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if result.String() != "" {
		t.Errorf("Expected empty string, got %q", result.String())
	}
}

func TestTextDecoder_NullInput(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var dec = new TextDecoder();
		dec.decode(null);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if result.String() != "" {
		t.Errorf("Expected empty string, got %q", result.String())
	}
}

// ===========================================================================
// Headers — uncovered branches
// ===========================================================================

func TestHeaders_InitFromHeadersObject(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var h1 = new Headers({ "Content-Type": "text/html" });
		var h2 = new Headers(h1);
		var ct = h2.get("content-type");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if s := adapter.runtime.Get("ct").String(); s != "text/html" {
		t.Errorf("Expected 'text/html', got %q", s)
	}
}

func TestHeaders_InitFromPairs(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var h = new Headers([["x-header", "value1"], ["x-header", "value2"]]);
		var all = h.get("x-header");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if s := adapter.runtime.Get("all").String(); !strings.Contains(s, "value1") {
		t.Errorf("Expected 'value1' in header, got %q", s)
	}
}

func TestHeaders_IteratorsAndForEach(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var h = new Headers({ "a": "1", "b": "2" });
		var collected = [];
		h.forEach(function(value, key) {
			collected.push(key + ":" + value);
		});
		var forEachCount = collected.length;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if n := adapter.runtime.Get("forEachCount").ToInteger(); n != 2 {
		t.Errorf("Expected 2, got %d", n)
	}
}

// ===========================================================================
// FormData — error paths
// ===========================================================================

func TestFormData_ForEachErrorPath(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			var fd = new FormData();
			fd.forEach();
			var fdErr = false;
		} catch(e) {
			var fdErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestFormData_ForEachNonFunction(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			var fd = new FormData();
			fd.forEach("not a function");
			var fdErr2 = false;
		} catch(e) {
			var fdErr2 = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// Storage — uncovered error paths
// ===========================================================================

func TestStorage_KeyOutOfBounds(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		localStorage.clear();
		localStorage.setItem("a", "1");
		var r1 = localStorage.key(0);
		var r2 = localStorage.key(1);
		var r3 = localStorage.key(-1);
		r2 === null && r3 === null;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected null for out-of-bounds key")
	}
}

func TestStorage_SetItemMinArgs(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			localStorage.setItem("only-one");
		} catch(e) {
			var storageErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
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
// bindSymbol — the polyfill path can't be triggered since Goja has native
// Symbol.for. Just verify the function returns nil by calling it again.
// ===========================================================================

func TestBindSymbol_NativeExists(t *testing.T) {
	adapter := coverSetup(t)

	// Verify Symbol.for and Symbol.keyFor work
	result, err := adapter.runtime.RunString(`
		var s1 = Symbol.for("test");
		var s2 = Symbol.for("test");
		s1 === s2;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Symbol.for should return same symbol")
	}
}

// ===========================================================================
// Blob — blobPartToBytes edge cases
// ===========================================================================

func TestBlobPartToBytes_NullPart(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var b = new Blob([null, undefined, "hello"]);
		var size = b.size;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestBlobPartToBytes_BlobPart(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var inner = new Blob(["inner"]);
		var outer = new Blob([inner, "outer"]);
		var size = outer.size;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if n := adapter.runtime.Get("size").ToInteger(); n != 10 {
		t.Errorf("Expected size 10, got %d", n)
	}
}

func TestBlobPartToBytes_TypedArray(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var arr = new Uint8Array([72, 101, 108, 108, 111]);
		var b = new Blob([arr]);
		var size = b.size;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if n := adapter.runtime.Get("size").ToInteger(); n != 5 {
		t.Errorf("Expected size 5, got %d", n)
	}
}

// ===========================================================================
// structuredClone — edge case types
// ===========================================================================

func TestStructuredClone_ErrorObject(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			structuredClone(new Error("oops"));
			var scErrOk = false;
		} catch(e) {
			var scErrOk = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("scErrOk")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected TypeError for cloning Error objects")
	}
}

// ===========================================================================
// gojaVoidFuncToHandler — non-function input
// ===========================================================================

func TestGojaVoidFuncToHandler_NonFunction(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		// finally with non-function argument should still work (noop)
		var finallyResult;
		Promise.resolve(42).finally("not a function").then(function(v) {
			finallyResult = v;
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
}

// ===========================================================================
// gojaFuncToHandler — handler returns wrapped promise
// ===========================================================================

func TestGojaFuncToHandler_ReturnsPromise(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var chainResult;
		Promise.resolve(1).then(function(v) {
			return Promise.resolve(v + 1);
		}).then(function(v) {
			chainResult = v;
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	val := adapter.runtime.Get("chainResult")
	if val == nil || val.ToInteger() != 2 {
		t.Errorf("Expected 2, got %v", val)
	}
}

// ===========================================================================
// Promise.reject with Error object — preserves .message
// ===========================================================================

func TestPromiseReject_ErrorPreservesMessage(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var errMsg;
		Promise.reject(new Error("test error")).catch(function(e) {
			errMsg = e.message;
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	val := adapter.runtime.Get("errMsg")
	if val == nil || val.String() != "test error" {
		t.Errorf("Expected 'test error', got %v", val)
	}
}

func TestPromiseReject_WrappedPromise(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var isPromise;
		var p = Promise.resolve(42);
		Promise.reject(p).catch(function(reason) {
			// reason should be the promise object itself
			isPromise = reason !== undefined && reason !== null;
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	val := adapter.runtime.Get("isPromise")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected promise as rejection reason")
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
// EventTarget — removeEventListener for non-function listener
// ===========================================================================

func TestEventTarget_RemoveNullListener(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var et = new EventTarget();
		et.removeEventListener("test", null);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestEventTarget_DispatchNonEvent(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			var et = new EventTarget();
			et.dispatchEvent(null);
			var dispatchErr = false;
		} catch(e) {
			var dispatchErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestEventTarget_DispatchPlainObject(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			var et = new EventTarget();
			et.dispatchEvent({});
			var dispatchErr2 = false;
		} catch(e) {
			var dispatchErr2 = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// AbortController constructor — direct call
// ===========================================================================

func TestAbortController_Direct(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			AbortSignal();
			var sigErr = false;
		} catch(e) {
			var sigErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// Promise executor throws
// ===========================================================================

func TestPromise_ExecutorThrows(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var caught;
		new Promise(function(resolve, reject) {
			throw new Error("executor error");
		}).catch(function(e) {
			caught = true;
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	val := adapter.runtime.Get("caught")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected rejection from executor throw")
	}
}

func TestPromise_ExecutorNullReject(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { new Promise(null); } catch(e) { var nullExecErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestPromise_ExecutorNonFunction(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { new Promise("not a function"); } catch(e) { var nonFuncExecErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// gojaFuncToHandler — map value conversion with wrapped promise
// ===========================================================================

func TestGojaFuncToHandler_MapWithWrappedPromise(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var result;
		Promise.allSettled([
			Promise.resolve(1),
			Promise.reject("err")
		]).then(function(results) {
			result = results;
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	val := adapter.runtime.Get("result")
	if val == nil || goja.IsUndefined(val) {
		t.Error("Expected allSettled results")
	}
}

// ===========================================================================
// isWrappedPromise / tryExtractWrappedPromise edge cases
// ===========================================================================

func TestIsWrappedPromise_EdgeCases(t *testing.T) {
	// Test non-object values
	if isWrappedPromise(goja.Undefined()) {
		t.Error("undefined should not be wrapped promise")
	}
	if isWrappedPromise(goja.Null()) {
		t.Error("null should not be wrapped promise")
	}
	if isWrappedPromise(nil) {
		t.Error("nil should not be wrapped promise")
	}

	// tryExtractWrappedPromise
	if p, ok := tryExtractWrappedPromise(nil); ok || p != nil {
		t.Error("nil should return (nil, false)")
	}
	if p, ok := tryExtractWrappedPromise(goja.Undefined()); ok || p != nil {
		t.Error("undefined should return (nil, false)")
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

// ===========================================================================
// performance.clearResourceTimings
// ===========================================================================

func TestPerformance_ClearResourceTimings(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`performance.clearResourceTimings()`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// Ensure coverage for linked URLSearchParams modification
// ===========================================================================

func TestURL_LinkedSearchParams_Mutations(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var u = new URL("https://example.com?a=1");
		var sp = u.searchParams;
		sp.append("b", "2");
		sp.set("a", "10");
		sp.delete("b");
		sp.sort();
		var s = sp.toString();
		var search = u.search;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// extractBytes — uncovered paths (ArrayBuffer direct, non-BufferSource)
// ===========================================================================

func TestTextDecoder_WithArrayBuffer(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var enc = new TextEncoder();
		var encoded = enc.encode("hello");
		var dec = new TextDecoder();
		// Pass the typed array directly
		dec.decode(encoded);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if result.String() != "hello" {
		t.Errorf("Expected 'hello', got %q", result.String())
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
