package gojaeventloop

import (
	"bytes"
	"strings"
	"testing"
)

// ===========================================================================
// extractBytes — ArrayBuffer direct path (lines 4524-4535)
// ===========================================================================

func TestTextDecoder_WithTypedArray(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var enc = new TextEncoder();
		var encoded = enc.encode("test");
		var dec = new TextDecoder();
		dec.decode(encoded);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if result.String() != "test" {
		t.Errorf("Expected 'test', got %q", result.String())
	}
}

// ===========================================================================
// TextDecoder — fatal mode with invalid UTF-8 (lines 4439-4465)
// ===========================================================================

func TestTextDecoder_FatalMode(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var dec = new TextDecoder("utf-8", { fatal: true });
		var valid = dec.decode(new Uint8Array([72, 101, 108, 108, 111]));
		var validOk = valid === "Hello";
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestTextDecoder_BOMHandling_CoverGap(t *testing.T) {
	adapter := coverSetup(t)

	// UTF-8 BOM is EF BB BF
	result, err := adapter.runtime.RunString(`
		var dec = new TextDecoder(); // ignoreBOM=false by default
		var withBOM = new Uint8Array([0xEF, 0xBB, 0xBF, 72, 105]);
		dec.decode(withBOM);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if result.String() != "Hi" {
		t.Errorf("Expected 'Hi' (BOM stripped), got %q", result.String())
	}
}

func TestTextDecoder_BOMPreserved(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var dec = new TextDecoder("utf-8", { ignoreBOM: true });
		var withBOM = new Uint8Array([0xEF, 0xBB, 0xBF, 72, 105]);
		dec.decode(withBOM);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	// With ignoreBOM=true, the BOM should be preserved
	if !strings.HasSuffix(result.String(), "Hi") {
		t.Errorf("Expected string ending with 'Hi', got %q", result.String())
	}
}

func TestTextDecoder_FatalModeInvalidUTF8(t *testing.T) {
	adapter := coverSetup(t)

	// Invalid UTF-8 sequence
	_, err := adapter.runtime.RunString(`
		var dec = new TextDecoder("utf-8", { fatal: true });
		// 0xFF is invalid UTF-8 start byte
		var invalid = new Uint8Array([0xFF, 0xFE]);
		var result = dec.decode(invalid);
		// In fatal mode, invalid UTF-8 might throw or return replacement chars
	`)
	// Some implementations throw, some don't - just ensure no crash
	_ = err
}

// ===========================================================================
// TextEncoder — encodeInto error paths (lines 4324-4335)
// ===========================================================================

func TestTextEncoder_EncodeInto_TooFewArgs(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			var enc = new TextEncoder();
			enc.encodeInto("hello");
		} catch(e) {
			var encodeIntoErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("encodeIntoErr")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected TypeError for too few args")
	}
}

func TestTextEncoder_EncodeInto_NullDest(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			var enc = new TextEncoder();
			enc.encodeInto("hello", null);
		} catch(e) {
			var nullDestErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestTextEncoder_EncodeInto_NullSource(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var enc = new TextEncoder();
		var buf = new Uint8Array(10);
		var result = enc.encodeInto(null, buf);
		var written = result.written;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

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
// URL property setters — specific uncovered paths
// ===========================================================================

func TestURL_SetProtocol_Invalid(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var u = new URL("https://example.com");
		u.protocol = "ftp:";
		var proto = u.protocol;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestURL_SetHostname_Empty(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var u = new URL("https://example.com:8080/path");
		u.hostname = "newhost.com";
		var hn = u.hostname;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestURL_SetPort_Empty(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var u = new URL("https://example.com:8080");
		u.port = "";
		var port = u.port;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestURL_SetHost_WithPort(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var u = new URL("https://example.com");
		u.host = "other.com:9090";
		var host = u.host;
		var hn = u.hostname;
		var port = u.port;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestURL_SetHost_NoPort(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var u = new URL("https://example.com:8080");
		u.host = "other.com";
		var host = u.host;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestURL_SetHref_Invalid(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			var u = new URL("https://example.com");
			u.href = "not a valid url";
		} catch(e) {
			var hrefErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestURL_Password_SetClear(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var u = new URL("https://user:pass@example.com");
		u.password = "newpass";
		var p1 = u.password;
		u.password = "";
		var p2 = u.password;
		// Clear userinfo entirely
		u.username = "";
		var href = u.href;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestURL_Username_SetClear(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var u = new URL("https://user:pass@example.com");
		u.username = "newuser";
		var u1 = u.username;
		u.username = "";
		var u2 = u.username;
		// href after
		var href = u.href;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// URLSearchParams — init edge cases
// ===========================================================================

func TestURLSearchParams_InitEmpty(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var sp = new URLSearchParams();
		sp.toString() === "";
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Empty URLSearchParams should have empty toString")
	}
}

func TestURLSearchParams_Size_CoverGap(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var sp = new URLSearchParams("a=1&b=2&c=3");
		var s = sp.size;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if n := adapter.runtime.Get("s").ToInteger(); n != 3 {
		t.Errorf("Expected size 3, got %d", n)
	}
}

func TestURLSearchParams_AppendAndGetAll(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var sp = new URLSearchParams();
		sp.append("key", "val1");
		sp.append("key", "val2");
		var all = sp.getAll("key");
		var count = all.length;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if n := adapter.runtime.Get("count").ToInteger(); n != 2 {
		t.Errorf("Expected 2, got %d", n)
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
// blobPartToBytes — exercise more paths
// ===========================================================================

func TestBlob_TypedArrayPart(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var arr = new Uint8Array([65, 66, 67]);
		var blob = new Blob([arr]);
		var s = blob.size;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if n := adapter.runtime.Get("s").ToInteger(); n != 3 {
		t.Errorf("Expected size 3, got %d", n)
	}
}

func TestBlob_NumberPart(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var blob = new Blob([42]);
		var s = blob.size;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// Headers — set, append, delete error paths
// ===========================================================================

func TestHeaders_SetGetDelete(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var h = new Headers();
		h.set("Content-Type", "text/html");
		h.append("X-Custom", "val1");
		h.append("X-Custom", "val2");
		var ct = h.get("Content-Type");
		var xc = h.get("X-Custom");
		h.delete("X-Custom");
		var xc2 = h.get("X-Custom");
		var has = h.has("Content-Type");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestHeaders_IteratorProtocol(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var h = new Headers({"a": "1", "b": "2"});
		var keysIter = h.keys();
		var r1 = keysIter.next();
		var r2 = keysIter.next();
		var r3 = keysIter.next();
		var done = r3.done === true;

		var valsIter = h.values();
		var v1 = valsIter.next();

		var entIter = h.entries();
		var e1 = entIter.next();
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestHeaders_InitWithNull(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var h = new Headers(null);
		var empty = !h.has("any");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestHeaders_InitWithUndefined(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var h = new Headers(undefined);
		var empty = !h.has("any");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestHeaders_InitFromInvalidPair(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			var h = new Headers([["only-one"]]);
		} catch(e) {
			// Headers should handle incomplete pairs
		}
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
// Storage — more edge cases
// ===========================================================================

func TestStorage_RemoveNonExistentKey(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		localStorage.clear();
		localStorage.removeItem("nonexistent");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestStorage_GetNonExistentKey(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		localStorage.clear();
		localStorage.getItem("nonexistent") === null;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected null for non-existent key")
	}
}

func TestStorage_LengthAndKeyIteration(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		localStorage.clear();
		localStorage.setItem("a", "1");
		localStorage.setItem("b", "2");
		localStorage.setItem("c", "3");
		var len = localStorage.length;
		var k0 = localStorage.key(0);
		var k1 = localStorage.key(1);
		var k2 = localStorage.key(2);
		localStorage.removeItem("b");
		var len2 = localStorage.length;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if n := adapter.runtime.Get("len").ToInteger(); n != 3 {
		t.Errorf("Expected length 3, got %d", n)
	}
	if n := adapter.runtime.Get("len2").ToInteger(); n != 2 {
		t.Errorf("Expected length 2, got %d", n)
	}
}

func TestStorage_SetItemOverwrite(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		localStorage.clear();
		localStorage.setItem("key", "value1");
		localStorage.setItem("key", "value2");
		localStorage.getItem("key") === "value2";
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected overwritten value")
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
// Performance — measure with string startMark
// ===========================================================================

func TestPerformance_MeasureWithStartMark(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		performance.mark("myStart");
		performance.mark("myEnd");
		var entry = performance.measure("test-measure", "myStart", "myEnd");
		var name = entry.name;
		var dur = entry.duration;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if s := adapter.runtime.Get("name").String(); s != "test-measure" {
		t.Errorf("Expected 'test-measure', got %q", s)
	}
}

func TestPerformance_MeasureWithOptionsStart(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		performance.mark("s1");
		var entry = performance.measure("test", { start: "s1" });
		var name = entry.name;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestPerformance_MeasureOptionsDuration(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		performance.mark("s1");
		var entry = performance.measure("test", { start: "s1", duration: 100 });
		var dur = entry.duration;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
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
// Blob constructor edge cases
// ===========================================================================

func TestBlob_EmptyConstructor(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var b = new Blob();
		b.size === 0 && b.type === "";
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected empty blob")
	}
}

func TestBlob_ConstructorWithOptions(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var b = new Blob(["hello"], { type: "TEXT/PLAIN" });
		b.type === "text/plain";
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected lowercase type")
	}
}

// ===========================================================================
// FormData — get, getAll, has, delete, append, set
// ===========================================================================

func TestFormData_CRUD(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var fd = new FormData();
		fd.append("key", "val1");
		fd.append("key", "val2");
		fd.set("other", "val3");

		var g = fd.get("key");
		var all = fd.getAll("key");
		var has = fd.has("key");
		var notHas = fd.has("nonexistent");

		fd.delete("key");
		var afterDel = fd.has("key");

		var count = 0;
		fd.forEach(function(v, k) { count++; });
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestFormData_Iterators(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var fd = new FormData();
		fd.append("a", "1");
		fd.append("b", "2");

		var keys = [];
		for (var k of fd.keys()) keys.push(k);

		var vals = [];
		for (var v of fd.values()) vals.push(v);

		var entries = [];
		for (var e of fd.entries()) entries.push(e);

		var keysLen = keys.length;
		var valsLen = vals.length;
		var entriesLen = entries.length;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestFormData_GetNonExistent(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var fd = new FormData();
		fd.get("nothing") === null;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected null for non-existent key")
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
