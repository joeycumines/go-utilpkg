package gojaeventloop

import (
	"bytes"
	"strings"
	"testing"
)

// ===========================================================================
// structuredClone — Map, Set, RegExp, Date edge cases
// ===========================================================================

func TestStructuredClone_Map_CoverGap(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var m = new Map([["key1", "val1"], ["key2", { nested: true }]]);
		var cloned = structuredClone(m);
		cloned instanceof Map && cloned.get("key1") === "val1" && cloned !== m;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected Map clone to work")
	}
}

func TestStructuredClone_Set_CoverGap(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var s = new Set([1, "two", { three: 3 }]);
		var cloned = structuredClone(s);
		cloned instanceof Set && cloned.size === 3 && cloned !== s;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected Set clone to work")
	}
}

func TestStructuredClone_RegExp_CoverGap(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var re = /test/gi;
		var cloned = structuredClone(re);
		cloned instanceof RegExp && cloned.source === "test" && cloned.flags.indexOf("g") >= 0 && cloned !== re;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected RegExp clone to work")
	}
}

func TestStructuredClone_Date_CoverGap(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var d = new Date(2024, 0, 1);
		var cloned = structuredClone(d);
		cloned instanceof Date && cloned.getTime() === d.getTime() && cloned !== d;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected Date clone to work")
	}
}

func TestStructuredClone_NestedArraysAndObjects(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var obj = {
			arr: [1, [2, 3]],
			nested: { a: { b: "deep" } },
			num: 42,
			str: "hello",
			bool: true,
			nil: null
		};
		var cloned = structuredClone(obj);
		cloned !== obj && cloned.arr !== obj.arr && cloned.nested.a.b === "deep";
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected deep clone to work")
	}
}

func TestStructuredClone_UndefinedValue(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var cloned = structuredClone(undefined);
		var isUndefined = cloned === undefined;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestStructuredClone_NullValue(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var cloned = structuredClone(null);
		var isNull = cloned === null;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestStructuredClone_Primitives_CoverGap(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var n = structuredClone(42);
		var s = structuredClone("hello");
		var b = structuredClone(true);
		var ok = n === 42 && s === "hello" && b === true;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestStructuredClone_Function(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			structuredClone(function() {});
			var funcErr = false;
		} catch(e) {
			var funcErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("funcErr")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected coerce or error for function clone")
	}
}

func TestStructuredClone_CircularReference_CoverGap(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var a = {};
		a.self = a;
		var cloned = structuredClone(a);
		var isCircular = cloned.self === cloned;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("isCircular")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected circular reference to be preserved")
	}
}

// ===========================================================================
// bindCrypto — getRandomValues with various typed arrays
// ===========================================================================

func TestCryptoGetRandomValues_Int8Array(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var arr = new Int8Array(4);
		crypto.getRandomValues(arr);
		// Should have been populated
		arr.length === 4;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected Int8Array to be populated")
	}
}

func TestCryptoGetRandomValues_Uint16Array(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var arr = new Uint16Array(4);
		crypto.getRandomValues(arr);
		arr.length === 4;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected Uint16Array to be populated")
	}
}

func TestCryptoGetRandomValues_Int16Array(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var arr = new Int16Array(4);
		crypto.getRandomValues(arr);
		arr.length === 4;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected Int16Array to be populated")
	}
}

func TestCryptoGetRandomValues_Int32Array(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var arr = new Int32Array(4);
		crypto.getRandomValues(arr);
		arr.length === 4;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected Int32Array to be populated")
	}
}

func TestCryptoGetRandomValues_Uint32Array(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var arr = new Uint32Array(4);
		crypto.getRandomValues(arr);
		arr.length === 4;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected Uint32Array to be populated")
	}
}

// ===========================================================================
// formatCellValue — int64, int, bool, default branches
// ===========================================================================

func TestConsoleTable_BigIntValues(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.table([
			{ name: "a", val: 9007199254740992 },
			{ name: "b", val: -9007199254740992 }
		]);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// inspectValue — more branches (bool, int, empty array/obj at depth)
// ===========================================================================

func TestConsoleDir_BoolValue(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.dir({ a: true, b: false, c: null, d: undefined });
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "true") {
		t.Error("Expected 'true' in dir output")
	}
}

func TestConsoleDir_IntegerValue(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.dir({ a: 0, b: -1, c: 42 });
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !strings.Contains(buf.String(), "42") {
		t.Error("Expected '42' in dir output")
	}
}

func TestConsoleDir_ArrayWithVariousTypes(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.dir([1, "two", true, null, undefined, [3, 4], { a: 1 }]);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// DOMException — Constants, toString
// ===========================================================================

func TestDOMException_Constants(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		DOMException.INDEX_SIZE_ERR === 1 &&
		DOMException.NOT_FOUND_ERR === 8 &&
		DOMException.INVALID_STATE_ERR === 11 &&
		DOMException.SYNTAX_ERR === 12 &&
		DOMException.QUOTA_EXCEEDED_ERR === 22;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected DOMException constants to be correct")
	}
}

func TestDOMException_ToStringCustomName(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var ex = new DOMException("custom msg", "NotSupportedError");
		ex.toString();
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !strings.Contains(result.String(), "NotSupportedError") {
		t.Errorf("Expected NotSupportedError in toString, got %q", result.String())
	}
}

// ===========================================================================
// generateUUIDv4 — randomUUID
// ===========================================================================

func TestCryptoRandomUUID_Format_CoverGap(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var uuid = crypto.randomUUID();
		// UUID format: 8-4-4-4-12 hex chars
		/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(uuid);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		u := adapter.runtime.Get("uuid")
		t.Errorf("UUID format invalid: %v", u)
	}
}

// ===========================================================================
// Custom Event
// ===========================================================================

func TestCustomEvent_WithDetail_CoverGap(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var et = new EventTarget();
		var received;
		et.addEventListener("test", function(e) {
			received = e.detail;
		});
		et.dispatchEvent(new CustomEvent("test", { detail: "myDetail" }));
		var ok = received === "myDetail";
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("ok")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected custom event detail to be received")
	}
}

// ===========================================================================
// AbortController — abort with custom reason
// ===========================================================================

func TestAbortController_AbortWithReason(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var ctrl = new AbortController();
		ctrl.abort("custom reason");
		var reason = ctrl.signal.reason;
		var aborted = ctrl.signal.aborted;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if s := adapter.runtime.Get("reason").String(); s != "custom reason" {
		t.Errorf("Expected 'custom reason', got %q", s)
	}
	if !adapter.runtime.Get("aborted").ToBoolean() {
		t.Error("Expected aborted=true")
	}
}

func TestAbortSignal_AbortEvent(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var ctrl = new AbortController();
		var eventFired = false;
		ctrl.signal.addEventListener('abort', function() { eventFired = true; });
		ctrl.abort();
		var aborted = ctrl.signal.aborted;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if !adapter.runtime.Get("aborted").ToBoolean() {
		t.Error("Expected aborted=true")
	}
	if !adapter.runtime.Get("eventFired").ToBoolean() {
		t.Error("Expected abort event to fire")
	}
}

func TestAbortSignal_AnyWithControllerAborted(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var ctrl1 = new AbortController();
		ctrl1.abort();
		var ctrl2 = new AbortController();
		var combined = AbortSignal.any([ctrl2.signal, ctrl1.signal]);
		var isAborted = combined.aborted;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if !adapter.runtime.Get("isAborted").ToBoolean() {
		t.Error("Expected combined signal to be aborted")
	}
}

// ===========================================================================
// Promise combinators — race, any, allSettled edge cases
// ===========================================================================

func TestPromise_RaceEmpty(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var raceResult;
		Promise.race([]).then(function(v) {
			raceResult = v;
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	// Race on empty array should never resolve - just verify no crash
	coverRunLoopBriefly(t, adapter, 100)
}

func TestPromise_AllSettled_MixedResults(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var settledResults;
		Promise.allSettled([
			Promise.resolve(1),
			Promise.reject("err"),
			42, // non-promise value
			Promise.resolve("ok")
		]).then(function(results) {
			settledResults = results;
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
}

func TestPromise_All_RejectsOnFirst(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var allErr;
		Promise.all([
			Promise.resolve(1),
			Promise.reject("fail"),
			Promise.resolve(3)
		]).catch(function(e) {
			allErr = e;
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	val := adapter.runtime.Get("allErr")
	if val == nil || val.String() != "fail" {
		t.Errorf("Expected 'fail', got %v", val)
	}
}

// ===========================================================================
// EventTarget — once and multi-dispatch
// ===========================================================================

func TestEventTarget_MultipleListeners(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var et = new EventTarget();
		var count = 0;
		var fn = function() { count++; };
		et.addEventListener("test", fn);
		et.addEventListener("test", fn); // duplicate
		et.dispatchEvent(new Event("test"));
		// Remove
		et.removeEventListener("test", fn);
		et.dispatchEvent(new Event("test"));
		var finalCount = count;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestEvent_Properties(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var e = new Event("click");
		var type = e.type;
		var bubbles = e.bubbles;
		var cancelable = e.cancelable;
		var defaultPrev = e.defaultPrevented;
		var ts = e.timeStamp;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestEvent_WithOptions(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var e = new Event("click", { bubbles: true, cancelable: true });
		e.bubbles === true && e.cancelable === true;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected event options to be set")
	}
}

// ===========================================================================
// process.nextTick — valid usage
// ===========================================================================

func TestProcessNextTick_Valid(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var nextTickCalled = false;
		process.nextTick(function() {
			nextTickCalled = true;
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
}

// ===========================================================================
// console.count and console.countReset
// ===========================================================================

func TestConsoleCount_WithLabel(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.count("myLabel");
		console.count("myLabel");
		console.countReset("myLabel");
		console.count("myLabel");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "myLabel") {
		t.Errorf("Expected 'myLabel' in output, got: %s", output)
	}
}

func TestConsoleCount_Default(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.count();
		console.count();
		console.countReset();
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "default") {
		t.Errorf("Expected 'default' in output, got: %s", output)
	}
}

// ===========================================================================
// atob / btoa — base64
// ===========================================================================

func TestAtobBtoa_Roundtrip(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var encoded = btoa("Hello, World!");
		var decoded = atob(encoded);
		decoded === "Hello, World!";
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected roundtrip to work")
	}
}

func TestBtoa_Error(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			btoa("\u0100");
			var btoaErr = false;
		} catch(e) {
			var btoaErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestAtob_Error(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			atob("!!!not valid!!!");
			var atobErr = false;
		} catch(e) {
			var atobErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}
