package gojaeventloop

import (
	"testing"
)

func TestPhase2_DOMException_ConstructorDefaults(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var e = new DOMException();
		if (e.name !== "Error") throw new Error("default name should be Error, got: " + e.name);
		if (e.message !== "") throw new Error("default message should be empty, got: " + e.message);
		if (e.code !== 0) throw new Error("default code should be 0, got: " + e.code);
	`)
	if err != nil {
		t.Fatalf("DOMException defaults failed: %v", err)
	}
}

func TestPhase2_DOMException_WithLegacyCode(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var e = new DOMException("msg", "NotFoundError");
		if (e.code !== 8) throw new Error("NotFoundError code should be 8, got: " + e.code);
		if (e.toString() !== "NotFoundError: msg") throw new Error("toString wrong: " + e.toString());
	`)
	if err != nil {
		t.Fatalf("DOMException legacy code failed: %v", err)
	}
}

func TestPhase2_AbortSignal_AnyEmpty(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var signal = AbortSignal.any([]);
		if (signal.aborted) throw new Error("empty signal should not be aborted");
	`)
	if err != nil {
		t.Fatalf("AbortSignal.any empty failed: %v", err)
	}
}

// AbortSignal.any with already-aborted signal
func TestPhase2_AbortSignal_AnyWithAborted(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var controller = new AbortController();
		controller.abort("test reason");
		var signal = AbortSignal.any([controller.signal]);
		if (!signal.aborted) throw new Error("composite should be aborted");
	`)
	if err != nil {
		t.Fatalf("AbortSignal.any with aborted failed: %v", err)
	}
}

// AbortSignal.any with non-signal argument
func TestPhase2_AbortSignal_AnyWithBadArg(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var caught = false;
		try {
			AbortSignal.any([{ aborted: false }]);
		} catch(e) {
			if (e instanceof TypeError) caught = true;
		}
		if (!caught) throw new Error("should throw TypeError for non-AbortSignal");
	`)
	if err != nil {
		t.Fatalf("AbortSignal.any bad arg failed: %v", err)
	}
}

// AbortSignal.any rejects null elements in the iterable
func TestPhase2_AbortSignal_AnyWithNull(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var caught = false;
		try { AbortSignal.any([null]); }
		catch (e) { caught = e instanceof TypeError; }
		if (!caught) throw new Error("should throw TypeError for null element");
	`)
	if err != nil {
		t.Fatalf("AbortSignal.any with null rejection failed: %v", err)
	}
}

func TestPhase2_AbortSignal_Timeout(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var signal = AbortSignal.timeout(50);
		if (signal.aborted) throw new Error("should not be aborted yet");
	`)
	if err != nil {
		t.Fatalf("AbortSignal.timeout failed: %v", err)
	}
}

func TestPhase2_DOMException_Constants(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		if (DOMException.INDEX_SIZE_ERR !== 1) throw new Error("INDEX_SIZE_ERR");
		if (DOMException.NOT_FOUND_ERR !== 8) throw new Error("NOT_FOUND_ERR");
		if (DOMException.ABORT_ERR !== 20) throw new Error("ABORT_ERR");
		if (DOMException.QUOTA_EXCEEDED_ERR !== 22) throw new Error("QUOTA_EXCEEDED_ERR");
	`)
	if err != nil {
		t.Fatalf("DOMException constants failed: %v", err)
	}
}

func TestPhase2_ThrowDOMException_Direct(t *testing.T) {
	adapter := coverSetup(t)
	// Exercise the throwDOMException via crypto quota exceeded - already tested
	// but let's also verify specific error properties
	_, err := adapter.runtime.RunString(`
		try {
			crypto.getRandomValues(new Uint8Array(65537));
		} catch(e) {
			if (e.code !== 22) throw new Error("code should be 22 (QuotaExceededError), got: " + e.code);
		}
	`)
	if err != nil {
		t.Fatalf("throwDOMException direct failed: %v", err)
	}
}

func TestPhase2_AbortSignal_Any_NonSignal(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		try {
			AbortSignal.any([{notASignal: true}]);
			throw new Error("should have thrown TypeError");
		} catch(e) {
			if (!(e instanceof TypeError)) throw new Error("wrong error: " + e.message);
		}
	`)
	if err != nil {
		t.Fatalf("AbortSignal.any non-signal failed: %v", err)
	}
}

func TestPhase2_AbortSignal_Any_ValidSignals(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var c1 = new AbortController();
		var c2 = new AbortController();
		var composite = AbortSignal.any([c1.signal, c2.signal]);
		if (composite.aborted) throw new Error("should not be aborted yet");

		c1.abort("reason1");
		if (!composite.aborted) throw new Error("composite should be aborted");
	`)
	if err != nil {
		t.Fatalf("AbortSignal.any valid signals failed: %v", err)
	}
}

func TestPhase2_AbortController_AbortReason(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var ac = new AbortController();
		ac.abort("my reason");
		if (!ac.signal.aborted) throw new Error("should be aborted");
		if (ac.signal.reason !== "my reason") throw new Error("reason: " + ac.signal.reason);

		// Abort again should be no-op
		ac.abort("second reason");
		if (ac.signal.reason !== "my reason") throw new Error("reason changed");
	`)
	if err != nil {
		t.Fatalf("AbortController abort reason failed: %v", err)
	}
}

func TestPhase2_AbortSignal_Any_WithNull(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var caught = false;
		try { AbortSignal.any([null, undefined]); }
		catch (e) { caught = e instanceof TypeError; }
		if (!caught) throw new Error("should throw TypeError for null/undefined elements");
	`)
	if err != nil {
		t.Fatalf("AbortSignal.any with null rejection failed: %v", err)
	}
}

// TestPhase2_AbortSignal_Timeout_Success exercises AbortSignal.timeout() with
// a valid timeout value (exercises the success path).
func TestPhase2_AbortSignal_Timeout_Success(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var signal = AbortSignal.timeout(1000);
		if (signal.aborted !== false) throw new Error("should not be aborted yet");
		// Just verify it was created without error
	`)
	if err != nil {
		t.Fatalf("AbortSignal.timeout failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 50)
}
