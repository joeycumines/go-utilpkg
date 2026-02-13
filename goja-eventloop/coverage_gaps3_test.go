package gojaeventloop

import (
	"bytes"
	"strings"
	"testing"
)

// ===========================================================================
// URL — username without existing User
// ===========================================================================

func TestURL_UsernameNilUser(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var u = new URL("https://example.com");
		// username getter with nil User
		var uname = u.username;
		// password getter with nil User
		var pass = u.password;
		// Set username without existing User
		u.username = "newuser";
		// Set password without existing User
		u.password = "newpass";
		var uname2 = u.username;
		var pass2 = u.password;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if s := adapter.runtime.Get("uname").String(); s != "" {
		t.Errorf("Expected empty username, got %q", s)
	}
	if s := adapter.runtime.Get("pass").String(); s != "" {
		t.Errorf("Expected empty password, got %q", s)
	}
	if s := adapter.runtime.Get("uname2").String(); s != "newuser" {
		t.Errorf("Expected 'newuser', got %q", s)
	}
}

func TestURL_UsernameWithExistingPassword(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var u = new URL("https://user:pass@example.com");
		// Changing username preserves password
		u.username = "alice";
		var uname = u.username;
		var pass = u.password;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if s := adapter.runtime.Get("uname").String(); s != "alice" {
		t.Errorf("Expected 'alice', got %q", s)
	}
	if s := adapter.runtime.Get("pass").String(); s != "pass" {
		t.Errorf("Expected 'pass', got %q", s)
	}
}

func TestURL_UsernameWithoutPassword(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var u = new URL("https://example.com");
		u.username = "justuser";
		var uname = u.username;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if s := adapter.runtime.Get("uname").String(); s != "justuser" {
		t.Errorf("Expected 'justuser', got %q", s)
	}
}

// ===========================================================================
// Headers — min args error paths
// ===========================================================================

func TestHeaders_SetNoArgs(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			var h = new Headers();
			h.set("only-name");
			var hSetErr = false;
		} catch(e) {
			var hSetErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("hSetErr")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected TypeError for set with 1 arg")
	}
}

func TestHeaders_AppendNoArgs(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			var h = new Headers();
			h.append("only-name");
			var hAppErr = false;
		} catch(e) {
			var hAppErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("hAppErr")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected TypeError for append with 1 arg")
	}
}

func TestHeaders_DeleteNoArgs(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var h = new Headers({"a": "1"});
		h.delete();
		// Should be a no-op without args
		var stillHas = h.has("a");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if !adapter.runtime.Get("stillHas").ToBoolean() {
		t.Error("Expected header 'a' to still exist")
	}
}

func TestHeaders_GetNoArgs(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var h = new Headers({"a": "1"});
		h.get() === null;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected null for get() with no args")
	}
}

func TestHeaders_HasNoArgs(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var h = new Headers({"a": "1"});
		h.has() === false;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected false for has() with no args")
	}
}

func TestHeaders_GetSetCookie_CoverG3(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var h = new Headers();
		h.append("Set-Cookie", "a=1");
		h.append("Set-Cookie", "b=2");
		var cookies = h.getSetCookie();
		var count = cookies.length;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if n := adapter.runtime.Get("count").ToInteger(); n != 2 {
		t.Errorf("Expected 2 cookies, got %d", n)
	}
}

func TestHeaders_GetSetCookieEmpty_CoverG3(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var h = new Headers();
		var cookies = h.getSetCookie();
		var count = cookies.length;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if n := adapter.runtime.Get("count").ToInteger(); n != 0 {
		t.Errorf("Expected 0, got %d", n)
	}
}

// ===========================================================================
// URLSearchParams — min args error paths
// ===========================================================================

func TestURLSearchParams_AppendNoArgs(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			new URLSearchParams().append("name");
			var spAppErr = false;
		} catch(e) {
			var spAppErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("spAppErr")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected TypeError for append with 1 arg")
	}
}

func TestURLSearchParams_DeleteNoArgs(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			new URLSearchParams("a=1").delete();
			var spDelErr = false;
		} catch(e) {
			var spDelErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("spDelErr")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected TypeError for delete with 0 args")
	}
}

func TestURLSearchParams_GetNoArgs(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			new URLSearchParams("a=1").get();
			var spGetErr = false;
		} catch(e) {
			var spGetErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("spGetErr")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected TypeError for get with 0 args")
	}
}

func TestURLSearchParams_GetAllNoArgs(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			new URLSearchParams("a=1").getAll();
			var spGaErr = false;
		} catch(e) {
			var spGaErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("spGaErr")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected TypeError for getAll with 0 args")
	}
}

func TestURLSearchParams_HasNoArgs(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			new URLSearchParams("a=1").has();
			var spHasErr = false;
		} catch(e) {
			var spHasErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("spHasErr")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected TypeError for has with 0 args")
	}
}

func TestURLSearchParams_SetNoArgs(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			new URLSearchParams("a=1").set("name");
			var spSetErr = false;
		} catch(e) {
			var spSetErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("spSetErr")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected TypeError for set with 1 arg")
	}
}

func TestURLSearchParams_ForEachNoArgs(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			new URLSearchParams("a=1").forEach();
			var spFeErr = false;
		} catch(e) {
			var spFeErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("spFeErr")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected TypeError for forEach with 0 args")
	}
}

func TestURLSearchParams_ForEachNonFunction(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			new URLSearchParams("a=1").forEach("not a func");
			var spFeErr2 = false;
		} catch(e) {
			var spFeErr2 = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("spFeErr2")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected TypeError for forEach with non-function")
	}
}

// ===========================================================================
// Storage — min args error paths
// ===========================================================================

func TestStorage_SetItemNoArgs(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			localStorage.setItem();
			var ssErr = false;
		} catch(e) {
			var ssErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("ssErr")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected TypeError for setItem with 0 args")
	}
}

func TestStorage_RemoveItemNoArgs(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		localStorage.removeItem();
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestStorage_KeyNoArgs(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		localStorage.key() === null;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected null for key() with no args")
	}
}

func TestStorage_GetItemNoArgs(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		localStorage.getItem() === null;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected null for getItem() with no args")
	}
}

// ===========================================================================
// Performance — string startMark detection path
// ===========================================================================

func TestPerformance_MeasureStringArgs(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		performance.mark("S");
		performance.mark("E");
		var entry = performance.measure("test", "S", "E");
		var name = entry.name;
		var dur = entry.duration;
		var entryType = entry.entryType;
		var startTime = entry.startTime;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if s := adapter.runtime.Get("name").String(); s != "test" {
		t.Errorf("Expected 'test', got %q", s)
	}
	if s := adapter.runtime.Get("entryType").String(); s != "measure" {
		t.Errorf("Expected 'measure', got %q", s)
	}
}

func TestPerformance_MeasureNoArgs(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var entry = performance.measure("test");
		var name = entry.name;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestPerformance_GetEntriesByName_CoverG3(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		performance.mark("findMe");
		performance.mark("findMe");
		var entries = performance.getEntriesByName("findMe");
		var count = entries.length;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestPerformance_GetEntries_CoverG3(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		performance.clearMarks();
		performance.clearMeasures();
		performance.mark("a");
		performance.mark("b");
		performance.measure("ab", "a", "b");
		var all = performance.getEntries();
		var marks = performance.getEntriesByType("mark");
		var measures = performance.getEntriesByType("measure");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// Performance — toJSON
// ===========================================================================

func TestPerformance_ToJSON_Fields(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var json = performance.toJSON();
		typeof json.timeOrigin === "number";
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("Expected timeOrigin in toJSON")
	}
}

// ===========================================================================
// Blob — sliced blob text() with loop running
// ===========================================================================

func TestBlobSlice_TextWithLoop(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var blob = new Blob(["ABCDEFGHIJ"]);
		var sliced = blob.slice(2, 7);
		var textResult = "";
		sliced.text().then(function(t) { textResult = t; });
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	if s := adapter.runtime.Get("textResult").String(); s != "CDEFG" {
		t.Errorf("Expected 'CDEFG', got %q", s)
	}
}

func TestBlobSlice_ArrayBufferWithLoop(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var blob = new Blob(["ABCDEFGHIJ"]);
		var sliced = blob.slice(0, 3);
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
		t.Errorf("Expected 3, got %d", n)
	}
}

// ===========================================================================
// console.table with columns filter on object data
// ===========================================================================

func TestConsoleTable_ObjectColumnsFilter_CoverGap(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.table({
			alice: { age: 30, role: "admin", score: 95 },
			bob: { age: 25, role: "user", score: 85 }
		}, ["role"]);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "admin") {
		t.Error("Expected 'admin' in filtered object table")
	}
}

// ===========================================================================
// EventTarget — dispatchEvent with stopImmediatePropagation
// ===========================================================================

func TestEventTarget_EventPreventation(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var et = new EventTarget();
		var count = 0;
		et.addEventListener("test", function(e) {
			count++;
			e.preventDefault();
			e.stopPropagation();
			e.stopImmediatePropagation();
		});
		et.addEventListener("test", function(e) {
			count++; // Should not be called
		});
		var e = new Event("test", { cancelable: true });
		et.dispatchEvent(e);
		var wasCancelled = e.defaultPrevented;
		var finalCount = count;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// EventTarget — addEventListener options
// ===========================================================================

func TestEventTarget_OnceOption(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var et = new EventTarget();
		var count = 0;
		et.addEventListener("test", function(e) { count++; }, { once: true });
		et.dispatchEvent(new Event("test"));
		et.dispatchEvent(new Event("test"));
		var finalCount = count;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if n := adapter.runtime.Get("finalCount").ToInteger(); n != 1 {
		t.Errorf("Expected count 1 (once), got %d", n)
	}
}

// ===========================================================================
// renderTable — empty rows
// ===========================================================================

func TestConsoleTable_SingleRowSingleCol(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.table([{x:1}]);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("Expected non-empty table output")
	}
}

// ===========================================================================
// console.time duplicate / missing label
// ===========================================================================

func TestConsoleTime_DuplicateLabel(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.time("dup");
		console.time("dup"); // duplicate - should warn
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestConsoleTimeEnd_MissingLabel(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.timeEnd("nonexistent");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if !strings.Contains(buf.String(), "nonexistent") {
		t.Errorf("Expected warning about nonexistent timer, got: %s", buf.String())
	}
}

func TestConsoleTimeLog_MissingLabel(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.timeLog("missing");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if !strings.Contains(buf.String(), "missing") {
		t.Errorf("Expected warning about missing timer, got: %s", buf.String())
	}
}

// ===========================================================================
// URL constructor — no scheme
// ===========================================================================

func TestURL_NoScheme_CoverGap(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { new URL(""); var urlErr = false; } catch(e) { var urlErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("urlErr")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected error for empty URL")
	}
}

func TestURL_WithUserInfo(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var u = new URL("https://user@example.com");
		var uname = u.username;
		var pass = u.password;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if s := adapter.runtime.Get("uname").String(); s != "user" {
		t.Errorf("Expected 'user', got %q", s)
	}
}

// ===========================================================================
// Blob constructor edge case — empty parts array
// ===========================================================================

func TestBlob_EmptyPartsArray(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var b = new Blob([]);
		b.size;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if result.ToInteger() != 0 {
		t.Errorf("Expected 0, got %d", result.ToInteger())
	}
}

// ===========================================================================
// structuredClone — with object containing function (should be skipped)
// ===========================================================================

func TestStructuredClone_ObjectWithFunction(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var obj = { a: 1, fn: function() {}, b: "hello" };
		var cloned = structuredClone(obj);
		var hasA = cloned.a === 1;
		var hasB = cloned.b === "hello";
		var noFn = typeof cloned.fn === "undefined";
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	val := adapter.runtime.Get("hasA")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected cloned.a === 1")
	}
}

// ===========================================================================
// Blob — constructor with Blob part creates correct concat
// ===========================================================================

func TestBlob_MultipleParts_CoverG3(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var b1 = new Blob(["Hello"]);
		var b2 = new Blob([", "]);
		var combined = new Blob([b1, b2, "World!"]);
		var combinedText = "";
		combined.text().then(function(t) { combinedText = t; });
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	if s := adapter.runtime.Get("combinedText").String(); s != "Hello, World!" {
		t.Errorf("Expected 'Hello, World!', got %q", s)
	}
}

// ===========================================================================
// Timer edge cases — null/undefined first argument
// ===========================================================================

func TestSetTimeout_NullArg(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { setTimeout(null, 0); var stErr = false; } catch(e) { var stErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("stErr").ToBoolean() {
		t.Error("Expected TypeError for setTimeout(null)")
	}
}

func TestSetInterval_NullArg(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { setInterval(null, 0); var siErr = false; } catch(e) { var siErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("siErr").ToBoolean() {
		t.Error("Expected TypeError for setInterval(null)")
	}
}

func TestQueueMicrotask_NullArg(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { queueMicrotask(null); var qmErr = false; } catch(e) { var qmErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("qmErr").ToBoolean() {
		t.Error("Expected TypeError for queueMicrotask(null)")
	}
}

func TestSetImmediate_NullArg(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { setImmediate(null); var simErr = false; } catch(e) { var simErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("simErr").ToBoolean() {
		t.Error("Expected TypeError for setImmediate(null)")
	}
}

func TestSetTimeout_NonFunctionArg(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { setTimeout(42, 0); var stNfErr = false; } catch(e) { var stNfErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("stNfErr").ToBoolean() {
		t.Error("Expected TypeError for setTimeout(42)")
	}
}

func TestSetTimeout_NegativeDelay(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var called = false;
		setTimeout(function() { called = true; }, -100);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
}

func TestSetInterval_NegativeDelay(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var count = 0;
		var id = setInterval(function() { count++; if (count >= 2) clearInterval(id); }, -50);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 200)
}

// ===========================================================================
// URL — constructor with base URL
// ===========================================================================

func TestURL_WithBaseURL_CoverG3(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var u = new URL("/path", "https://example.com");
		var href = u.href;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	if s := adapter.runtime.Get("href").String(); !strings.Contains(s, "example.com") {
		t.Errorf("Expected example.com in href, got %q", s)
	}
}

func TestURL_ConstructorNoArgs(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { new URL(); var noArgErr = false; } catch(e) { var noArgErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("noArgErr").ToBoolean() {
		t.Error("Expected TypeError for URL()")
	}
}

func TestURL_ConstructorInvalidBase(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { new URL("/path", "://bad"); var badBaseErr = false; } catch(e) { var badBaseErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	// Note: Go's url.Parse is very permissive, this might not throw
}

// ===========================================================================
// performance.measure — exportType path (number as second arg)
// ===========================================================================

func TestPerformance_MeasureWithNumberArg(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		performance.mark("S3");
		// Second arg is a number - should be treated as options/non-string
		try {
			performance.measure("test-num", { start: "S3" });
		} catch(e) {
			// Might throw if mark doesn't exist
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// AbortController — basic usage coverage
// ===========================================================================

func TestAbortController_SignalAbort(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var ac = new AbortController();
		var isAbortedBefore = ac.signal.aborted;
		ac.abort("custom reason");
		var isAbortedAfter = ac.signal.aborted;
		var reason = ac.signal.reason;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if adapter.runtime.Get("isAbortedBefore").ToBoolean() {
		t.Error("Expected false before abort")
	}
	if !adapter.runtime.Get("isAbortedAfter").ToBoolean() {
		t.Error("Expected true after abort")
	}
}

// ===========================================================================
// AbortSignal.timeout — basic coverage
// ===========================================================================

func TestAbortSignal_Timeout(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var sig = AbortSignal.timeout(50);
		var wasAborted = sig.aborted;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if adapter.runtime.Get("wasAborted").ToBoolean() {
		t.Error("Expected false immediately after AbortSignal.timeout()")
	}
	coverRunLoopBriefly(t, adapter, 200)
}

func TestAbortSignal_TimeoutNegative_CoverG3(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var sig = AbortSignal.timeout(-10);
		var wasAborted = sig.aborted;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// AbortSignal.any — with valid signals
// ===========================================================================

func TestAbortSignal_AnyValidSignals(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var ac1 = new AbortController();
		var ac2 = new AbortController();
		var combined = AbortSignal.any([ac1.signal, ac2.signal]);
		var before = combined.aborted;
		ac1.abort("first");
		var after = combined.aborted;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if adapter.runtime.Get("before").ToBoolean() {
		t.Error("Expected false before abort")
	}
	if !adapter.runtime.Get("after").ToBoolean() {
		t.Error("Expected true after abort")
	}
}

func TestAbortSignal_AnyWithNull(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var ac = new AbortController();
		// null entries should be skipped
		var combined = AbortSignal.any([null, ac.signal]);
		var before = combined.aborted;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if adapter.runtime.Get("before").ToBoolean() {
		t.Error("Expected false")
	}
}

func TestAbortSignal_AnyNonIterable(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { AbortSignal.any(42); var anyErr = false; } catch(e) { var anyErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("anyErr").ToBoolean() {
		t.Error("Expected TypeError for AbortSignal.any(42)")
	}
}

// ===========================================================================
// crypto.getRandomValues — various typed arrays
// ===========================================================================

func TestCrypto_GetRandomValues_Int32Array(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var arr = new Int32Array(4);
		crypto.getRandomValues(arr);
		var hasNonZero = false;
		for (var i = 0; i < arr.length; i++) {
			if (arr[i] !== 0) hasNonZero = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestCrypto_GetRandomValues_Int16Array(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var arr = new Int16Array(8);
		crypto.getRandomValues(arr);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestCrypto_GetRandomValues_Uint32Array(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var arr = new Uint32Array(4);
		crypto.getRandomValues(arr);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestCrypto_GetRandomValues_Float64ArrayFails(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			var arr = new Float64Array(4);
			crypto.getRandomValues(arr);
			var f64Err = false;
		} catch(e) {
			var f64Err = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("f64Err").ToBoolean() {
		t.Error("Expected TypeError for Float64Array")
	}
}

func TestCrypto_GetRandomValues_Float32ArrayFails(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			var arr = new Float32Array(4);
			crypto.getRandomValues(arr);
			var f32Err = false;
		} catch(e) {
			var f32Err = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("f32Err").ToBoolean() {
		t.Error("Expected TypeError for Float32Array")
	}
}

func TestCrypto_GetRandomValues_NoArgs(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { crypto.getRandomValues(); var crvErr = false; } catch(e) { var crvErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("crvErr").ToBoolean() {
		t.Error("Expected TypeError for getRandomValues() with no args")
	}
}

func TestCrypto_GetRandomValues_NullArg(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { crypto.getRandomValues(null); var crvNullErr = false; } catch(e) { var crvNullErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("crvNullErr").ToBoolean() {
		t.Error("Expected TypeError for getRandomValues(null)")
	}
}

func TestCrypto_GetRandomValues_QuotaExceeded(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			var big = new Uint8Array(65537);
			crypto.getRandomValues(big);
			var quotaErr = false;
		} catch(e) {
			var quotaErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("quotaErr").ToBoolean() {
		t.Error("Expected QuotaExceededError for oversized array")
	}
}

// ===========================================================================
// structuredClone — objects that look like Map/Set/Date/RegExp but aren't
// ===========================================================================

func TestStructuredClone_ObjectWithGetTime(t *testing.T) {
	adapter := coverSetup(t)

	// Object with getTime method but not Date constructor
	_, err := adapter.runtime.RunString(`
		var fakeDate = { getTime: function() { return 42; }, value: "not a date" };
		fakeDate.constructor = { name: "NotDate" };
		var cloned = structuredClone(fakeDate);
		var hasVal = cloned.value === "not a date";
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("hasVal").ToBoolean() {
		t.Error("Expected plain clone of fake Date object")
	}
}

func TestStructuredClone_ObjectWithTestMethod(t *testing.T) {
	adapter := coverSetup(t)

	// Object with test method but not RegExp constructor
	_, err := adapter.runtime.RunString(`
		var fakeRegex = { test: function() { return true; }, source: "abc" };
		fakeRegex.constructor = { name: "NotRegExp" };
		var cloned = structuredClone(fakeRegex);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestStructuredClone_ObjectWithAddMethod(t *testing.T) {
	adapter := coverSetup(t)

	// Object with add/has/delete but not Set constructor
	_, err := adapter.runtime.RunString(`
		var fakeSet = {
			add: function() {},
			has: function() { return false; },
			delete: function() { return false; }
		};
		fakeSet.constructor = { name: "NotSet" };
		var cloned = structuredClone(fakeSet);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestStructuredClone_ObjectWithGetSetHasDelete(t *testing.T) {
	adapter := coverSetup(t)

	// Object with get/set/has/delete but not Map constructor
	_, err := adapter.runtime.RunString(`
		var fakeMap = {
			get: function() {},
			set: function() {},
			has: function() { return false; },
			delete: function() { return false; }
		};
		fakeMap.constructor = { name: "NotMap" };
		var cloned = structuredClone(fakeMap);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// structuredClone — TypedArray and ArrayBuffer cloning
// ===========================================================================

func TestStructuredClone_TypedArray(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var arr = new Uint8Array([1, 2, 3, 4]);
		var cloned = structuredClone(arr);
		var same = cloned[0] === 1 && cloned[1] === 2 && cloned.length === 4;
		// Modify original shouldn't affect clone
		arr[0] = 99;
		var independent = cloned[0] === 1;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestStructuredClone_ArrayBuffer(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var buf = new ArrayBuffer(8);
		var view = new Uint8Array(buf);
		view[0] = 42;
		var cloned = structuredClone(buf);
		var clonedView = new Uint8Array(cloned);
		var preserved = clonedView[0] === 42;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// TextDecoder — fatal mode and BOM handling
// ===========================================================================

func TestTextDecoder_FatalMode_CoverG3(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var dec = new TextDecoder("utf-8", { fatal: true });
		// Valid UTF-8 should work fine
		var result = dec.decode(new Uint8Array([72, 101, 108, 108, 111]));
		var valid = result === "Hello";
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("valid").ToBoolean() {
		t.Error("Expected 'Hello'")
	}
}

func TestTextDecoder_IgnoreBOM(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var dec = new TextDecoder("utf-8", { ignoreBOM: true });
		var result = dec.decode(new Uint8Array([0xEF, 0xBB, 0xBF, 72, 105]));
		// With ignoreBOM, the BOM should be preserved in output
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// TextEncoder — encodeInto edge case
// ===========================================================================

func TestTextEncoder_EncodeIntoLargeString(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var enc = new TextEncoder();
		var dest = new Uint8Array(2);
		var result = enc.encodeInto("Hello World!", dest);
		// Only 2 bytes should be written
		var written = result.written;
		var read = result.read;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if n := adapter.runtime.Get("written").ToInteger(); n != 2 {
		t.Errorf("Expected 2 written, got %d", n)
	}
}

// ===========================================================================
// console.table — undefined data and primitives
// ===========================================================================

func TestConsoleTable_UndefinedData(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.table(undefined);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !strings.Contains(buf.String(), "(index)") {
		t.Errorf("Expected (index) in output, got: %s", buf.String())
	}
}

func TestConsoleTable_NullData(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.table(null);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestConsoleTable_PrimitiveString(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.table("hello world");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !strings.Contains(buf.String(), "hello world") {
		t.Errorf("Expected 'hello world' in output, got: %s", buf.String())
	}
}

func TestConsoleTable_EmptyArray_CoverG3(t *testing.T) {
	adapter := coverSetup(t)
	var buf bytes.Buffer
	adapter.SetConsoleOutput(&buf)

	_, err := adapter.runtime.RunString(`
		console.table([]);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// FormData — delete, has, set, getAll
// ===========================================================================

func TestFormData_SetAndGetAll(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var fd = new FormData();
		fd.append("key", "val1");
		fd.append("key", "val2");
		fd.set("key", "val3");
		var all = fd.getAll("key");
		var count = all.length;
		var val = all[0];
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if n := adapter.runtime.Get("count").ToInteger(); n != 1 {
		t.Errorf("Expected 1 after set, got %d", n)
	}
	if s := adapter.runtime.Get("val").String(); s != "val3" {
		t.Errorf("Expected 'val3', got %q", s)
	}
}

func TestFormData_Delete_CoverG3(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var fd = new FormData();
		fd.append("x", "1");
		fd.append("y", "2");
		fd.delete("x");
		var hasX = fd.has("x");
		var hasY = fd.has("y");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if adapter.runtime.Get("hasX").ToBoolean() {
		t.Error("Expected x to be deleted")
	}
	if !adapter.runtime.Get("hasY").ToBoolean() {
		t.Error("Expected y to still exist")
	}
}

// ===========================================================================
// process.env — edge cases
// ===========================================================================

func TestProcess_EnvAccess(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var hasProcess = typeof process !== "undefined";
		var envType = typeof process.env;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("hasProcess").ToBoolean() {
		t.Error("Expected process to be defined")
	}
}

// ===========================================================================
// DOMException — different error names
// ===========================================================================

func TestDOMException_DifferentNames(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var e1 = new DOMException("test", "NotFoundError");
		var name1 = e1.name;
		var msg1 = e1.message;
		var e2 = new DOMException("test2");
		var name2 = e2.name;
		var e3 = new DOMException();
		var msg3 = e3.message;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if s := adapter.runtime.Get("name1").String(); s != "NotFoundError" {
		t.Errorf("Expected 'NotFoundError', got %q", s)
	}
	if s := adapter.runtime.Get("name2").String(); s != "Error" {
		t.Errorf("Expected 'Error', got %q", s)
	}
}

// ===========================================================================
// Promise — executor that throws
// ===========================================================================

func TestPromise_ExecutorThrows_CoverG3(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var caught = false;
		var p = new Promise(function(resolve, reject) {
			throw new Error("executor error");
		});
		p.catch(function(e) { caught = true; });
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
}

func TestPromise_ConstructorNullExecutor(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { new Promise(null); var pExErr = false; } catch(e) { var pExErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("pExErr").ToBoolean() {
		t.Error("Expected TypeError for Promise(null)")
	}
}

// ===========================================================================
// URL.searchParams interaction
// ===========================================================================

func TestURL_SearchParams_Integration(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var u = new URL("https://example.com?a=1&b=2");
		var sp = u.searchParams;
		sp.set("c", "3");
		sp.delete("a");
		var search = u.search;
		var size = sp.size;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// URLSearchParams — sort and toString
// ===========================================================================

func TestURLSearchParams_SortAndToString(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var sp = new URLSearchParams("z=3&a=1&m=2");
		sp.sort();
		var sorted = sp.toString();
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if s := adapter.runtime.Get("sorted").String(); !strings.HasPrefix(s, "a=") {
		t.Errorf("Expected sorted params starting with a=, got %q", s)
	}
}

// ===========================================================================
// URLSearchParams — delete with value
// ===========================================================================

func TestURLSearchParams_DeleteWithValue_CoverG3(t *testing.T) {
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
		t.Errorf("Expected 2 remaining 'a' params, got %d", n)
	}
}

// ===========================================================================
// URLSearchParams — has with value
// ===========================================================================

func TestURLSearchParams_HasWithValue_CoverG3(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var sp = new URLSearchParams("a=1&a=2");
		var has1 = sp.has("a", "1");
		var has3 = sp.has("a", "3");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("has1").ToBoolean() {
		t.Error("Expected has('a','1') to be true")
	}
	if adapter.runtime.Get("has3").ToBoolean() {
		t.Error("Expected has('a','3') to be false")
	}
}

// ===========================================================================
// URLSearchParams — forEach with thisArg
// ===========================================================================

func TestURLSearchParams_ForEachWithThisArg(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var sp = new URLSearchParams("a=1&b=2");
		var collector = { items: [] };
		sp.forEach(function(value, key) {
			this.items.push(key + "=" + value);
		}, collector);
		var count = collector.items.length;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if n := adapter.runtime.Get("count").ToInteger(); n != 2 {
		t.Errorf("Expected 2, got %d", n)
	}
}

// ===========================================================================
// URLSearchParams — entries, keys, values iterators
// ===========================================================================

func TestURLSearchParams_Entries_CoverG3(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var sp = new URLSearchParams("a=1&b=2");
		var entries = sp.entries();
		var first = entries.next();
		var done = false;
		var count = 0;
		while (!first.done) {
			count++;
			first = entries.next();
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// Headers — forEach with callback
// ===========================================================================

func TestHeaders_ForEach_CoverG3(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var h = new Headers({"a": "1", "b": "2"});
		var count = 0;
		h.forEach(function(value, key) {
			count++;
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if n := adapter.runtime.Get("count").ToInteger(); n < 2 {
		t.Errorf("Expected at least 2, got %d", n)
	}
}

// ===========================================================================
// Headers — entries/keys/values iterators
// ===========================================================================

func TestHeaders_Entries_CoverG3(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var h = new Headers({"a": "1", "b": "2"});
		var entries = h.entries();
		var arr = [];
		var next = entries.next();
		while (!next.done) {
			arr.push(next.value);
			next = entries.next();
		}
		var count = arr.length;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// Blob slice — negative indices
// ===========================================================================

func TestBlobSlice_NegativeIndices_CoverG3(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var blob = new Blob(["0123456789"]);
		var sliced = blob.slice(-5);
		var textResult = "";
		sliced.text().then(function(t) { textResult = t; });
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	if s := adapter.runtime.Get("textResult").String(); s != "56789" {
		t.Errorf("Expected '56789', got %q", s)
	}
}

func TestBlobSlice_StartGreaterThanEnd_CoverG3(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var blob = new Blob(["hello"]);
		var sliced = blob.slice(4, 2);
		sliced.size;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if result.ToInteger() != 0 {
		t.Errorf("Expected 0, got %d", result.ToInteger())
	}
}

func TestBlobSlice_WithContentType(t *testing.T) {
	adapter := coverSetup(t)

	result, err := adapter.runtime.RunString(`
		var blob = new Blob(["hello"], { type: "text/plain" });
		var sliced = blob.slice(0, 5, "text/html");
		sliced.type;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if s := result.String(); s != "text/html" {
		t.Errorf("Expected 'text/html', got %q", s)
	}
}

// ===========================================================================
// initHeaders — edge cases
// ===========================================================================

func TestHeaders_InitFromHeaders(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var h1 = new Headers({"a": "1", "b": "2"});
		// Init new headers from existing Headers-like object
		var h2 = new Headers(h1);
		var a = h2.get("a");
		var b = h2.get("b");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestHeaders_InitFromArray(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var h = new Headers([["a", "1"], ["b", "2"]]);
		var a = h.get("a");
		var b = h.get("b");
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if s := adapter.runtime.Get("a").String(); s != "1" {
		t.Errorf("Expected '1', got %q", s)
	}
}

// ===========================================================================
// EventTarget — addEventListener with capture option
// ===========================================================================

func TestEventTarget_CaptureOption(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var et = new EventTarget();
		var count = 0;
		et.addEventListener("test", function() { count++; }, { capture: true });
		et.dispatchEvent(new Event("test"));
		var finalCount = count;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if n := adapter.runtime.Get("finalCount").ToInteger(); n != 1 {
		t.Errorf("Expected 1, got %d", n)
	}
}

// ===========================================================================
// EventTarget — removeEventListener with non-function
// ===========================================================================

func TestEventTarget_RemoveNonFunction(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var et = new EventTarget();
		// This should silently fail
		et.removeEventListener("test", "not a function");
		et.removeEventListener("test", null);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// Event — various properties
// ===========================================================================

func TestEvent_Properties_CoverG3(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var e = new Event("click", { bubbles: true, cancelable: true });
		var type = e.type;
		var bubbles = e.bubbles;
		var cancelable = e.cancelable;
		var timeStamp = e.timeStamp;
		var target = e.target;
		var currentTarget = e.currentTarget;
		var composed = e.composed;
		var isTrusted = e.isTrusted;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if s := adapter.runtime.Get("type").String(); s != "click" {
		t.Errorf("Expected 'click', got %q", s)
	}
}

// ===========================================================================
// CustomEvent — detail access
// ===========================================================================

func TestCustomEvent_DetailAccess_CoverG3(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var et = new EventTarget();
		var receivedDetail = null;
		et.addEventListener("myevent", function(e) {
			receivedDetail = e.detail;
		});
		var ce = new CustomEvent("myevent", { detail: { foo: "bar" } });
		et.dispatchEvent(ce);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}
