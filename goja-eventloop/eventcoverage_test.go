package gojaeventloop

import (
	"testing"

	goeventloop "github.com/joeycumines/go-eventloop"
)

func TestPhase2_EventTarget_GoDispatchedEvent_WrapEventFallback(t *testing.T) {
	adapter := coverSetup(t)

	// Step 1: Create EventTarget in JS and add listener
	_, err := adapter.runtime.RunString(`
		var et = new EventTarget();
		var received = null;
		et.addEventListener("myevent", function(e) {
			received = e.type;
		});
	`)
	if err != nil {
		t.Fatalf("EventTarget setup failed: %v", err)
	}

	// Step 2: Extract the hidden Go EventTarget state from the adapter symbol.
	wrapper := adapter.eventTargetThis(adapter.runtime.Get("et"))

	// Step 3: Dispatch a Go event directly (NOT through JS dispatchEvent)
	// This bypasses the dispatchJSEvents.Store, triggering the wrapEvent fallback
	goEvent := goeventloop.NewEvent("myevent")
	wrapper.target.DispatchEvent(goEvent)

	// Step 4: Verify the listener was called with a wrapped event
	receivedVal := adapter.runtime.Get("received")
	if receivedVal == nil || receivedVal.String() != "myevent" {
		t.Errorf("expected received='myevent', got '%v'", receivedVal)
	}
}

// Same test but for the 'once' listener branch (line 2802)
func TestPhase2_EventTarget_GoDispatchedEvent_OnceFallback(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		var et2 = new EventTarget();
		var onceReceived = null;
		et2.addEventListener("test", function(e) {
			onceReceived = e.type;
		}, {once: true});
	`)
	if err != nil {
		t.Fatalf("EventTarget once setup failed: %v", err)
	}

	wrapper := adapter.eventTargetThis(adapter.runtime.Get("et2"))

	goEvent := goeventloop.NewEvent("test")
	wrapper.target.DispatchEvent(goEvent)

	receivedVal := adapter.runtime.Get("onceReceived")
	if receivedVal == nil || receivedVal.String() != "test" {
		t.Errorf("expected onceReceived='test', got '%v'", receivedVal)
	}
}

func TestPhase2_CustomEvent_NoTypeArg(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var caught = false;
		try {
			new CustomEvent();
		} catch(e) {
			if (e instanceof TypeError) caught = true;
		}
		if (!caught) throw new Error("CustomEvent() without type should throw TypeError");
	`)
	if err != nil {
		t.Fatalf("CustomEvent no type failed: %v", err)
	}
}

func TestPhase2_CustomEvent_NullDetail(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var ce = new CustomEvent("myevent");
		if (ce.detail !== null) throw new Error("default detail should be null");
	`)
	if err != nil {
		t.Fatalf("CustomEvent null detail failed: %v", err)
	}
}

func TestPhase2_Event_NoTypeArg(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var caught = false;
		try {
			new Event();
		} catch(e) {
			if (e instanceof TypeError) caught = true;
		}
		if (!caught) throw new Error("Event() without type should throw TypeError");
	`)
	if err != nil {
		t.Fatalf("Event no type failed: %v", err)
	}
}

func TestPhase2_Event_WithOptions(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var e = new Event("click", {bubbles: true, cancelable: true});
		if (!e.bubbles) throw new Error("bubbles should be true");
		if (!e.cancelable) throw new Error("cancelable should be true");
		if (e.defaultPrevented) throw new Error("defaultPrevented should be false");
	`)
	if err != nil {
		t.Fatalf("Event with options failed: %v", err)
	}
}

func TestPhase2_DispatchEvent_NullEvent(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var et = new EventTarget();
		var caught = false;
		try {
			et.dispatchEvent(null);
		} catch(e) {
			if (e instanceof TypeError) caught = true;
		}
		if (!caught) throw new Error("dispatchEvent(null) should throw TypeError");
	`)
	if err != nil {
		t.Fatalf("dispatchEvent null failed: %v", err)
	}
}

func TestPhase2_DispatchEvent_NonEvent(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var et = new EventTarget();
		var caught = false;
		try {
			et.dispatchEvent({});
		} catch(e) {
			if (e instanceof TypeError) caught = true;
		}
		if (!caught) throw new Error("dispatchEvent({}) should throw TypeError");
	`)
	if err != nil {
		t.Fatalf("dispatchEvent non-Event failed: %v", err)
	}
}

func TestPhase2_DispatchEvent_UndefinedEvent(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var et = new EventTarget();
		var caught = false;
		try {
			et.dispatchEvent(undefined);
		} catch(e) {
			caught = true;
		}
		if (!caught) throw new Error("dispatchEvent(undefined) should throw");
	`)
	if err != nil {
		t.Fatalf("dispatchEvent undefined failed: %v", err)
	}
}

// Plain objects are not Events, even if they contain Event-looking fields.
func TestPhase2_DispatchEvent_FakeEvent(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var et = new EventTarget();
		var caught = false;
		try {
			et.dispatchEvent({ type: "test" });
		} catch(e) {
			if (e instanceof TypeError) caught = true;
		}
		if (!caught) throw new Error("dispatchEvent with plain object should throw TypeError");
	`)
	if err != nil {
		t.Fatalf("dispatchEvent fake event failed: %v", err)
	}
}

func TestPhase2_AddEventListener_NonCallable(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var et = new EventTarget();
		var rejected = false;
		try { et.addEventListener("click", "not a function"); }
		catch (error) { rejected = error instanceof TypeError; }
		if (!rejected) throw new Error("primitive listener was not rejected");
		et.addEventListener("click", null);
	`)
	if err != nil {
		t.Fatalf("addEventListener non-callable failed: %v", err)
	}
}

// removeEventListener with null
func TestPhase2_RemoveEventListener_Null(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var et = new EventTarget();
		// Should not panic
		et.removeEventListener("click", null);
	`)
	if err != nil {
		t.Fatalf("removeEventListener null failed: %v", err)
	}
}

func TestPhase2_EventTarget_ListenerOptions(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var et = new EventTarget();
		var count = 0;
		// once: true — should only fire once
		et.addEventListener("test", function() { count++; }, {once: true});
		et.dispatchEvent(new Event("test"));
		et.dispatchEvent(new Event("test"));
		if (count !== 1) throw new Error("once should fire 1 time, got: " + count);
	`)
	if err != nil {
		t.Fatalf("EventTarget once listener failed: %v", err)
	}
}

func TestPhase2_Event_AllProperties(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var e = new Event("click", {bubbles: true, cancelable: true});
		if (e.type !== "click") throw new Error("type: " + e.type);
		if (!e.bubbles) throw new Error("bubbles");
		if (!e.cancelable) throw new Error("cancelable");
		if (e.defaultPrevented) throw new Error("defaultPrevented initially");

		e.preventDefault();
		if (!e.defaultPrevented) throw new Error("defaultPrevented after");

		e.stopPropagation();
		e.stopImmediatePropagation();

		// target should be null (not dispatched)
		if (e.target !== null) throw new Error("target: " + e.target);
	`)
	if err != nil {
		t.Fatalf("Event all properties failed: %v", err)
	}
}

func TestPhase2_CustomEvent_Detail(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var ce = new CustomEvent("myevent", {detail: {x: 42}});
		if (ce.type !== "myevent") throw new Error("type: " + ce.type);
		if (ce.detail.x !== 42) throw new Error("detail.x: " + ce.detail.x);

		// CustomEvent without detail
		var ce2 = new CustomEvent("plain");
		if (ce2.detail !== null && ce2.detail !== undefined) {
			// detail defaults to null
		}
	`)
	if err != nil {
		t.Fatalf("CustomEvent detail failed: %v", err)
	}
}

func TestPhase2_EventTarget_DispatchAndReceive(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var et = new EventTarget();
		var receivedType = "";
		var receivedDetail = null;

		et.addEventListener("custom", function(e) {
			receivedType = e.type;
			if (e.detail) receivedDetail = e.detail;
		});

		// Dispatch CustomEvent
		et.dispatchEvent(new CustomEvent("custom", {detail: {msg: "hello"}}));
		if (receivedType !== "custom") throw new Error("type: " + receivedType);
		if (!receivedDetail || receivedDetail.msg !== "hello") throw new Error("detail: " + JSON.stringify(receivedDetail));
	`)
	if err != nil {
		t.Fatalf("EventTarget dispatch and receive failed: %v", err)
	}
}

func TestPhase2_EventTarget_RemoveByReference(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var et = new EventTarget();
		var count = 0;
		var handler = function() { count++; };
		et.addEventListener("ev", handler);
		et.dispatchEvent(new Event("ev"));
		if (count !== 1) throw new Error("first: " + count);

		et.removeEventListener("ev", handler);
		et.dispatchEvent(new Event("ev"));
		if (count !== 1) throw new Error("after remove: " + count);
	`)
	if err != nil {
		t.Fatalf("EventTarget remove by reference failed: %v", err)
	}
}

func TestPhase2_Event_StopImmediatePropagation(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var et = new EventTarget();
		var results = [];
		et.addEventListener("ev", function(e) {
			results.push("first");
			e.stopImmediatePropagation();
		});
		et.addEventListener("ev", function(e) {
			results.push("second");
		});
		et.dispatchEvent(new Event("ev"));
		if (results.length !== 1 || results[0] !== "first") {
			throw new Error("stop imm: " + JSON.stringify(results));
		}
	`)
	if err != nil {
		t.Fatalf("Event stopImmediatePropagation failed: %v", err)
	}
}

// TestPhase2_DispatchEvent_BadInternalEvent exercises the error path when
// dispatchEvent receives objects that are not Event instances.
func TestPhase2_DispatchEvent_BadInternalEvent(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var target = new EventTarget();

		// Event-looking ordinary properties are ignored.
		try {
			target.dispatchEvent({ type: "test", target: target });
			throw new Error("should have thrown");
		} catch(e) {
			if (!(e instanceof TypeError)) throw new Error("wrong error: " + e);
		}

		// Missing hidden Event state also throws.
		try {
			target.dispatchEvent({ type: "test" });
			throw new Error("should have thrown for missing Event state");
		} catch(e) {
			if (!(e instanceof TypeError)) throw new Error("wrong error: " + e);
		}
	`)
	if err != nil {
		t.Fatalf("DispatchEvent bad internal event failed: %v", err)
	}
}

// TestPhase2_Event_Target_During_Dispatch exercises the event.target
// accessor when Target is non-nil (line 2943) by reading e.target
// inside a listener callback.
func TestPhase2_Event_Target_During_Dispatch(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var targetAccessed = false;
		var target = new EventTarget();
		target.addEventListener("test", function(e) {
			// During dispatch, e.target should be accessible
			var t = e.target;
			targetAccessed = true;
		});
		target.dispatchEvent(new Event("test"));
		if (!targetAccessed) throw new Error("listener not called");
	`)
	if err != nil {
		t.Fatalf("Event target during dispatch failed: %v", err)
	}
}

// TestPhase2_CustomEvent_Detail_Value exercises the CustomEvent.detail
// accessor when detail has a non-null value (line 3035).
func TestPhase2_CustomEvent_Detail_Value(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var target = new EventTarget();
		var detailVal = null;
		target.addEventListener("myevent", function(e) {
			detailVal = e.detail;
		});
		var ev = new CustomEvent("myevent", { detail: { key: "value" } });
		target.dispatchEvent(ev);
		if (detailVal === null) throw new Error("detail was null");
		if (detailVal.key !== "value") throw new Error("wrong detail: " + JSON.stringify(detailVal));
	`)
	if err != nil {
		t.Fatalf("CustomEvent detail value failed: %v", err)
	}
}

// TestPhase2_Event_Bubbles_Cancelable exercises event properties
// that may not have been accessed in other tests.
func TestPhase2_Event_Bubbles_Cancelable(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var ev = new Event("click", { bubbles: true, cancelable: true });
		if (ev.bubbles !== true) throw new Error("bubbles wrong");
		if (ev.cancelable !== true) throw new Error("cancelable wrong");

		ev.preventDefault();
		if (ev.defaultPrevented !== true) throw new Error("defaultPrevented wrong");

		// stopPropagation shouldn't throw
		ev.stopPropagation();
	`)
	if err != nil {
		t.Fatalf("Event bubbles/cancelable failed: %v", err)
	}
}
