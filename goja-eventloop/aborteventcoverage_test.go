package gojaeventloop

import "testing"

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
		try { AbortSignal.timeout(-1); var typeErr = false; }
		catch (e) { var typeErr = e.name === "TypeError"; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("typeErr").ToBoolean() {
		t.Error("Expected TypeError for negative AbortSignal.timeout delay")
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
