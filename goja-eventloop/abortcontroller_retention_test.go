package gojaeventloop

import (
	goruntime "runtime"
	"testing"
	"weak"

	"github.com/joeycumines/goja"
)

// Regression tests for the AbortController -> AbortSignal strong-retention
// contract. The signal JS object must live exactly as long as its controller
// (or a user-held reference), and must be collectible otherwise. A previous
// implementation stored only a weak.Pointer to the signal in the controller
// state, so after a Go GC cycle `controller.signal` returned nil and any JS
// use of it crashed the VM with a nil pointer dereference.
//
// GC mechanics relied on here (goja internals, not public API):
//
//   - Each JS snippet that must leave objects collectible is terminated with a
//     trailing `RunString("void 0")`. goja retains the last evaluated value in
//     vm.result and may retain stack references, so without the extra run the
//     object under test stays reachable and the collection assertions below
//     would fail — or, if someone deletes the trailing run later, flake.
//
//   - The hidden-state stores (Adapter.setHiddenState/hiddenState in
//     adapter.go) are JS WeakMaps keyed by the object. State attached to an
//     object is therefore collected together with it — there is no strong
//     Go-side map holding controller or signal state. Test
//     TestAbortControllerDroppedEverythingCollects pins exactly that: the new
//     strong controller -> signalObject edge must not leak once both JS
//     references are dropped.
//
//   - AbortSignal.timeout(100000) leaves a 100 s pending timer. That is safe
//     here because newBoundAdapterForNode26Test registers loop.Close()
//     (immediate termination: queued and pending timers are discarded, not
//     waited on) via t.Cleanup, and the timer callback holds only weak state
//     references, so the pending timer neither keeps the JS object alive nor
//     outlives the test.

func gcAbortRetention(t *testing.T) {
	t.Helper()
	for range 100 {
		goruntime.GC()
		goruntime.Gosched()
	}
}

// The controller stays alive while every reference to its signal is dropped.
// The signal must remain reachable (and usable) via the controller.
func TestAbortControllerSignalSurvivesGC(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	if _, err := adapter.runtime.RunString(`
		globalThis.__c = new AbortController();
		globalThis.__c.signal; // evaluate the accessor once
		void 0;
	`); err != nil {
		t.Fatal(err)
	}
	gcAbortRetention(t)

	// Pre-fix this sequence crashed the VM (nil pointer dereference).
	value, err := adapter.runtime.RunString(`globalThis.__c.signal.aborted === false`)
	if err != nil {
		t.Fatalf("signal accessor after GC: %v", err)
	}
	if !value.ToBoolean() {
		t.Fatal("signal accessor returned a non-signal value after GC")
	}

	// The signal must also remain usable for listener registration after GC.
	if _, err := adapter.runtime.RunString(`
		globalThis.__events = [];
		globalThis.__c.signal.addEventListener("abort", function(e) {
			globalThis.__events.push(e.type);
		});
		void 0;
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.runtime.RunString(`globalThis.__c.abort(new Error("boom")); void 0`); err != nil {
		t.Fatalf("abort after GC: %v", err)
	}
	value, err = adapter.runtime.RunString(`
		globalThis.__c.signal.aborted === true && globalThis.__events.join(",") === "abort"
	`)
	if err != nil {
		t.Fatalf("post-abort reads after GC: %v", err)
	}
	if !value.ToBoolean() {
		t.Fatal("abort did not propagate to the controller signal after GC")
	}
}

// Repeated access to controller.signal must yield the same object across GC.
func TestAbortControllerSignalIdentityStableAcrossGC(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	if _, err := adapter.runtime.RunString(`
		globalThis.__c = new AbortController();
		globalThis.__s1 = globalThis.__c.signal;
		void 0;
	`); err != nil {
		t.Fatal(err)
	}
	gcAbortRetention(t)
	value, err := adapter.runtime.RunString(`globalThis.__c.signal === globalThis.__s1`)
	if err != nil {
		t.Fatal(err)
	}
	if !value.ToBoolean() {
		t.Fatal("controller.signal returned a different object after GC")
	}
}

// A user-held signal must survive the collection of its controller.
func TestAbortControllerSignalUserHeldSurvivesControllerDrop(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	if _, err := adapter.runtime.RunString(`
		globalThis.__s = (function() {
			var c = new AbortController();
			var s = c.signal;
			c = null;
			return s;
		})();
		void 0;
	`); err != nil {
		t.Fatal(err)
	}
	gcAbortRetention(t)
	value, err := adapter.runtime.RunString(`globalThis.__s.aborted === false`)
	if err != nil {
		t.Fatalf("user-held signal after controller drop: %v", err)
	}
	if !value.ToBoolean() {
		t.Fatal("user-held signal unusable after controller drop")
	}
}

// Dropping both the controller and the signal must release both objects
// (no leak from the strong controller -> signal edge).
func TestAbortControllerDroppedEverythingCollects(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	if _, err := adapter.runtime.RunString(`
		(function() {
			var c = new AbortController();
			var s = c.signal;
			globalThis.__c = c;
			globalThis.__s = s;
		})();
		void 0;
	`); err != nil {
		t.Fatal(err)
	}
	controllerValue, err := adapter.runtime.RunString(`globalThis.__c`)
	if err != nil {
		t.Fatal(err)
	}
	signalValue, err := adapter.runtime.RunString(`globalThis.__s`)
	if err != nil {
		t.Fatal(err)
	}
	controllerRef := weak.Make(controllerValue.(*goja.Object))
	signalRef := weak.Make(signalValue.(*goja.Object))
	controllerValue, signalValue = nil, nil
	if _, err := adapter.runtime.RunString(`globalThis.__c = null; globalThis.__s = null; void 0`); err != nil {
		t.Fatal(err)
	}
	gcAbortRetention(t)
	if controllerRef.Value() != nil {
		t.Fatal("dropped AbortController object remained strongly retained")
	}
	if signalRef.Value() != nil {
		t.Fatal("dropped AbortSignal object remained strongly retained")
	}
}

// Static-created signals remain usable while held from JS.
func TestAbortSignalStaticsUsableAfterGC(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	if _, err := adapter.runtime.RunString(`
		globalThis.__a = AbortSignal.abort(new Error("e1"));
		globalThis.__t = AbortSignal.timeout(100000);
		globalThis.__y = AbortSignal.any([globalThis.__t]);
		void 0;
	`); err != nil {
		t.Fatal(err)
	}
	gcAbortRetention(t)
	value, err := adapter.runtime.RunString(`
		globalThis.__a.aborted === true &&
		globalThis.__t.aborted === false &&
		globalThis.__y.aborted === false
	`)
	if err != nil {
		t.Fatalf("static signal reads after GC: %v", err)
	}
	if !value.ToBoolean() {
		t.Fatal("static-created signal unusable after GC")
	}
}

// A dropped composite signal (with an abort listener on a live controller
// source) is still reachable through the Go-side retention machinery, so the
// source abort propagates into the registered listener after GC.
func TestAbortSignalAnyCompositePropagationAfterGC(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	if _, err := adapter.runtime.RunString(`
		globalThis.__c = new AbortController();
		globalThis.__events = [];
		(function() {
			var s = AbortSignal.any([globalThis.__c.signal]);
			s.addEventListener("abort", function(e) {
				globalThis.__events.push(e.type);
			});
			s = null;
		})();
		void 0;
	`); err != nil {
		t.Fatal(err)
	}
	gcAbortRetention(t)
	if _, err := adapter.runtime.RunString(`globalThis.__c.abort(new Error("boom")); void 0`); err != nil {
		t.Fatalf("abort after GC: %v", err)
	}
	value, err := adapter.runtime.RunString(`globalThis.__events.join(",")`)
	if err != nil {
		t.Fatal(err)
	}
	if got := value.String(); got != "abort" {
		t.Fatalf("composite abort events after GC = %q, want %q", got, "abort")
	}
}

// Dropped standalone signals and event targets must still be collectible
// (the weak design exists to prevent leaks).
func TestAbortSignalAndEventTargetCollectibleWhenDropped(t *testing.T) {
	t.Run("AbortSignal.abort", func(t *testing.T) {
		adapter := newBoundAdapterForNode26Test(t)
		value, err := adapter.runtime.RunString(`AbortSignal.abort(new Error("x"))`)
		if err != nil {
			t.Fatal(err)
		}
		ref := weak.Make(value.(*goja.Object))
		value = nil
		if _, err := adapter.runtime.RunString(`void 0`); err != nil {
			t.Fatal(err)
		}
		gcAbortRetention(t)
		if ref.Value() != nil {
			t.Fatal("dropped AbortSignal.abort object remained strongly retained")
		}
	})
	t.Run("AbortSignal.timeout", func(t *testing.T) {
		adapter := newBoundAdapterForNode26Test(t)
		value, err := adapter.runtime.RunString(`AbortSignal.timeout(100000)`)
		if err != nil {
			t.Fatal(err)
		}
		ref := weak.Make(value.(*goja.Object))
		value = nil
		if _, err := adapter.runtime.RunString(`void 0`); err != nil {
			t.Fatal(err)
		}
		gcAbortRetention(t)
		if ref.Value() != nil {
			t.Fatal("dropped AbortSignal.timeout object remained strongly retained")
		}
	})
	t.Run("EventTarget", func(t *testing.T) {
		adapter := newBoundAdapterForNode26Test(t)
		value, err := adapter.runtime.RunString(`new EventTarget()`)
		if err != nil {
			t.Fatal(err)
		}
		ref := weak.Make(value.(*goja.Object))
		value = nil
		if _, err := adapter.runtime.RunString(`void 0`); err != nil {
			t.Fatal(err)
		}
		gcAbortRetention(t)
		if ref.Value() != nil {
			t.Fatal("dropped EventTarget object remained strongly retained")
		}
	})
	t.Run("composite-with-listener", func(t *testing.T) {
		adapter := newBoundAdapterForNode26Test(t)
		if _, err := adapter.runtime.RunString(`
			globalThis.__fn = null;
			(function() {
				var s = AbortSignal.any([AbortSignal.timeout(100000)]);
				globalThis.__fn = function() {};
				s.addEventListener("abort", globalThis.__fn);
				globalThis.__ref = s;
				s = null;
			})();
			void 0;
		`); err != nil {
			t.Fatal(err)
		}
		value, err := adapter.runtime.RunString(`globalThis.__ref`)
		if err != nil {
			t.Fatal(err)
		}
		obj := value.(*goja.Object)
		ref := weak.Make(obj)
		// A non-aborted signal with abort listeners is retained per the DOM
		// and must NOT be garbage-collected while the listener is attached —
		// the old assertion here (that the dropped composite with a listener
		// is collected) contradicted that contract and the identity tests in
		// abort_signal_identity_test.go. This test guards the release side:
		// removing the listener ends the retention and the object must then
		// be collectible.
		fn, err := adapter.runtime.RunString(`globalThis.__fn`)
		if err != nil {
			t.Fatal(err)
		}
		remove, ok := goja.AssertFunction(obj.Get("removeEventListener"))
		if !ok {
			t.Fatal("removeEventListener is not callable")
		}
		if _, err := remove(obj, adapter.runtime.ToValue("abort"), fn); err != nil {
			t.Fatal(err)
		}
		if _, err := adapter.runtime.RunString(`globalThis.__fn = null; globalThis.__ref = null; void 0;`); err != nil {
			t.Fatal(err)
		}
		obj = nil
		gcAbortRetention(t)
		if ref.Value() != nil {
			t.Fatal("composite signal remained retained after its listener was removed")
		}
	})
}
