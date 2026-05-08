package gojaeventloop

import (
	"testing"
)

// Event, abort, and DOM exception control edge coverage.

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
		try { AbortSignal.timeout(-10); var typeErr = false; }
		catch (e) { var typeErr = e.name === "TypeError"; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("typeErr").ToBoolean() {
		t.Error("Expected TypeError for negative AbortSignal.timeout delay")
	}
}

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
		try { AbortSignal.any([null]); var typeErr = false; }
		catch (e) { var typeErr = e instanceof TypeError; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("typeErr").ToBoolean() {
		t.Error("Expected TypeError")
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

func TestEventTarget_RemoveNonFunction(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var et = new EventTarget();
		var rejected = false;
		try { et.removeEventListener("test", "not a function"); }
		catch (error) { rejected = error instanceof TypeError; }
		if (!rejected) throw new Error("primitive listener was not rejected");
		et.removeEventListener("test", null);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

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
