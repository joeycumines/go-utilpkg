package gojaeventloop

import (
	"context"
	"testing"
	"time"
	"weak"

	"github.com/joeycumines/goja"
)

// Identity tests for the AbortSignal strong-object contract (review-c finding
// 3). A non-aborted signal with abort listeners (or abort algorithms, or
// dependent observers) is retained per the DOM: it must not be
// garbage-collected, and dispatch must observe the exact JS object identity
// for this, event.target, and event.currentTarget — even after the JS
// reference is dropped and Go GC cycles run. The Go-side retention machinery
// pins the abortSignalState; these tests prove the exact *goja.Object is
// pinned with it (the eventTargetWrapper.object field is a weak pointer, so
// without the scoped strong object pin the state would survive while the
// object was swept, leaving dispatch with nil identities).
//
// Tests 1-3 discriminate the pin (they fail without it): the any composite,
// the timeout signal, and a timeout signal that is also the source of a
// retained composite (whose retention re-evaluation runs before its own
// dispatch). Tests 4-5 guard the release side (the object must become
// collectible once the retention condition ends).

const abortIdentityScript = `
	globalThis.__seen = {
		thisIsObject: typeof this === "object",
		thisIsSignal: this instanceof AbortSignal,
		thisIsTarget: this === e.target,
		thisIsCurrent: this === e.currentTarget,
	};
`

func assertAbortIdentity(t *testing.T, adapter *Adapter) {
	t.Helper()
	value, err := adapter.runtime.RunString(`
		globalThis.__seen !== null &&
		globalThis.__seen.thisIsObject === true &&
		globalThis.__seen.thisIsSignal === true &&
		globalThis.__seen.thisIsTarget === true &&
		globalThis.__seen.thisIsCurrent === true
	`)
	if err != nil {
		t.Fatal(err)
	}
	if !value.ToBoolean() {
		t.Fatalf(
			"abort listener observed wrong identities after GC: %v",
			mustRunString(adapter, `JSON.stringify(globalThis.__seen)`),
		)
	}
}

func mustRunString(adapter *Adapter, code string) goja.Value {
	value, err := adapter.runtime.RunString(code)
	if err != nil {
		panic(err)
	}
	return value
}

// A dropped composite signal (AbortSignal.any over a live controller signal)
// with an abort listener must survive Go GC cycles and dispatch the abort
// event with the exact object identity on this/target/currentTarget.
func TestAbortSignalAnyIdentityAfterGC(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	if _, err := adapter.runtime.RunString(`
		globalThis.__c = new AbortController();
		globalThis.__seen = null;
		(function() {
			var s = AbortSignal.any([globalThis.__c.signal]);
			s.addEventListener("abort", function(e) {
				` + abortIdentityScript + `
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
	assertAbortIdentity(t, adapter)
}

// A dropped AbortSignal.timeout signal with an abort listener must survive Go
// GC cycles and dispatch the timeout abort with the exact object identity on
// this/target/currentTarget.
func TestAbortSignalTimeoutIdentityAfterGC(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	done := make(chan struct{}, 1)
	if err := adapter.runtime.Set("__identityDone", func(goja.FunctionCall) goja.Value {
		select {
		case done <- struct{}{}:
		default:
		}
		return goja.Undefined()
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.runtime.RunString(`
		globalThis.__seen = null;
		(function() {
			var s = AbortSignal.timeout(20);
			s.addEventListener("abort", function(e) {
				` + abortIdentityScript + `
				__identityDone();
			});
			s = null;
		})();
		void 0;
	`); err != nil {
		t.Fatal(err)
	}
	gcAbortRetention(t)
	runDone := make(chan error, 1)
	go func() { runDone <- adapter.loop.Run(context.Background()) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout signal did not fire")
	}
	if err := adapter.loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("event loop did not exit")
	}
	assertAbortIdentity(t, adapter)
}

// A timeout signal that is also the source of a retained composite must
// dispatch its own abort with the exact object identity. beginAbortSignal
// re-evaluates the source's retention (via the dependent-link unlink ->
// adjustAbortDependentObservers -> refreshAbortTimeoutRetention) BEFORE the
// source's own dispatch runs; releasing the object pin there would let a Go
// GC sweep the object mid-dispatch, so the pin must survive until the
// dispatch completes.
func TestAbortSignalTimeoutSourceWithOwnListenerIdentityAfterGC(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	done := make(chan struct{}, 1)
	if err := adapter.runtime.Set("__identityDone", func(goja.FunctionCall) goja.Value {
		select {
		case done <- struct{}{}:
		default:
		}
		return goja.Undefined()
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.runtime.RunString(`
		globalThis.__seen = null;
		(function() {
			var s = AbortSignal.timeout(20);
			s.addEventListener("abort", function(e) {
				` + abortIdentityScript + `
				__identityDone();
			});
			var c = AbortSignal.any([s]);
			c.addEventListener("abort", function() {});
			s = null;
			c = null;
		})();
		void 0;
	`); err != nil {
		t.Fatal(err)
	}
	gcAbortRetention(t)
	runDone := make(chan error, 1)
	go func() { runDone <- adapter.loop.Run(context.Background()) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout signal did not fire")
	}
	if err := adapter.loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("event loop did not exit")
	}
	assertAbortIdentity(t, adapter)
}

// A composite signal with an abort listener becomes collectible once the
// listener is removed (the retention condition ends). The retained phase
// itself — the object surviving Go GC cycles with no external references —
// is proven by the identity tests above; this test guards the release side.
func TestAbortSignalAnyCollectibleAfterRetentionEnds(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	if _, err := adapter.runtime.RunString(`
		globalThis.__fn = null;
		(function() {
			var c = new AbortController();
			var s = AbortSignal.any([c.signal]);
			globalThis.__fn = function() {};
			s.addEventListener("abort", globalThis.__fn);
			globalThis.__ref = s;
			s = null;
			c = null;
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
	value = nil
	// End the retention: remove the listener through the Go-held object, then
	// drop the JS references and the Go reference; the object must be
	// collectible.
	fn := mustRunString(adapter, `globalThis.__fn`)
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
}

// A timeout signal with an abort listener becomes collectible after the
// timeout fires (the abort releases the retention pin).
func TestAbortSignalTimeoutCollectibleAfterAbort(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	done := make(chan struct{}, 1)
	if err := adapter.runtime.Set("__abortDone", func(goja.FunctionCall) goja.Value {
		select {
		case done <- struct{}{}:
		default:
		}
		return goja.Undefined()
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.runtime.RunString(`
		globalThis.__fn = null;
		(function() {
			var s = AbortSignal.timeout(20);
			globalThis.__fn = function() { __abortDone(); };
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
	ref := weak.Make(value.(*goja.Object))
	value = nil
	runDone := make(chan error, 1)
	go func() { runDone <- adapter.loop.Run(context.Background()) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout signal did not fire")
	}
	if err := adapter.loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("event loop did not exit")
	}
	// The signal is aborted: drop the JS references and the Go reference; the
	// object must now be collectible.
	if _, err := adapter.runtime.RunString(`globalThis.__fn = null; globalThis.__ref = null; void 0;`); err != nil {
		t.Fatal(err)
	}
	gcAbortRetention(t)
	if ref.Value() != nil {
		t.Fatal("aborted timeout signal remained retained after the timeout fired")
	}
}
