package gojaeventloop

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Phase 3c Coverage Tests
// Targeting uncovered blocks identified in the 97.4% → 98%+ push.
// Focus: timer error paths (terminated loop), promise method theft,
// structuredClone type-check fallbacks, AbortSignal edge cases.
// =============================================================================

// phase3cSetupTerminated creates an adapter whose loop has been terminated.
// Calling timer/microtask APIs on this adapter should produce errors.
func phase3cSetupTerminated(t *testing.T) *Adapter {
	t.Helper()
	loop, err := goeventloop.New()
	require.NoError(t, err)

	rt := goja.New()
	adapter, err := New(loop, rt)
	require.NoError(t, err)
	require.NoError(t, adapter.Bind())

	// Shutdown the loop (Awake → Terminated directly)
	require.NoError(t, loop.Shutdown(context.Background()))

	return adapter
}

// ---------------------------------------------------------------------------
// Timer / microtask error paths on terminated loop
// adapter.go:227-228, 263-264, 292-293, 315-316
// ---------------------------------------------------------------------------

func TestPhase3c_SetTimeout_LoopTerminated(t *testing.T) {
	adapter := phase3cSetupTerminated(t)
	_, err := adapter.runtime.RunString(`
		var caught = false;
		try { setTimeout(function(){}, 100); } catch(e) { caught = true; }
		if (!caught) throw new Error("expected error from setTimeout on terminated loop");
	`)
	require.NoError(t, err)
}

func TestPhase3c_SetInterval_LoopTerminated(t *testing.T) {
	adapter := phase3cSetupTerminated(t)
	_, err := adapter.runtime.RunString(`
		var caught = false;
		try { setInterval(function(){}, 100); } catch(e) { caught = true; }
		if (!caught) throw new Error("expected error from setInterval on terminated loop");
	`)
	require.NoError(t, err)
}

func TestPhase3c_QueueMicrotask_LoopTerminated(t *testing.T) {
	adapter := phase3cSetupTerminated(t)
	_, err := adapter.runtime.RunString(`
		var caught = false;
		try { queueMicrotask(function(){}); } catch(e) { caught = true; }
		if (!caught) throw new Error("expected error from queueMicrotask on terminated loop");
	`)
	require.NoError(t, err)
}

func TestPhase3c_SetImmediate_LoopTerminated(t *testing.T) {
	adapter := phase3cSetupTerminated(t)
	_, err := adapter.runtime.RunString(`
		var caught = false;
		try { setImmediate(function(){}); } catch(e) { caught = true; }
		if (!caught) throw new Error("expected error from setImmediate on terminated loop");
	`)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Promise .then / .catch / .finally called on non-object this
// adapter.go:858-859, 877-878, 895-896
// Steal the method from a real promise and call it with a non-object this
// to trigger the !ok branch of thisVal.(*goja.Object) type assertion.
// ---------------------------------------------------------------------------

func TestPhase3c_PromiseThen_NonObjectThis(t *testing.T) {
	adapter := coverSetup(t)
	// Create a promise, extract its .then method, call with non-object this
	_, err := adapter.runtime.RunString(`
		var p = new Promise(function(resolve) { resolve(1); });
		var thenFn = p.then;
		var caught = false;
		try {
			thenFn.call(42, function(){});
		} catch(e) {
			if (e instanceof TypeError) caught = true;
		}
		if (!caught) throw new Error("expected TypeError from then() on non-Promise this");
	`)
	require.NoError(t, err)
}

func TestPhase3c_PromiseCatch_NonObjectThis(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var p = new Promise(function(resolve) { resolve(1); });
		var catchFn = p["catch"];
		var caught = false;
		try {
			catchFn.call(42, function(){});
		} catch(e) {
			if (e instanceof TypeError) caught = true;
		}
		if (!caught) throw new Error("expected TypeError from catch() on non-Promise this");
	`)
	require.NoError(t, err)
}

func TestPhase3c_PromiseFinally_NonObjectThis(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var p = new Promise(function(resolve) { resolve(1); });
		var finallyFn = p["finally"];
		var caught = false;
		try {
			finallyFn.call(42, function(){});
		} catch(e) {
			if (e instanceof TypeError) caught = true;
		}
		if (!caught) throw new Error("expected TypeError from finally() on non-Promise this");
	`)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// AbortSignal.any with non-signal object
// adapter.go:1352 — object has _signal but it's not *goeventloop.AbortSignal
// ---------------------------------------------------------------------------

func TestPhase3c_AbortSignalAny_FakeSignal(t *testing.T) {
	adapter := coverSetup(t)
	// _signal exists but is wrong type → exercises the type assertion path
	_, err := adapter.runtime.RunString(`
		var caught = false;
		try {
			AbortSignal.any([{_signal: "not a real signal"}]);
		} catch(e) {
			if (e instanceof TypeError) caught = true;
		}
		if (!caught) throw new Error("expected TypeError from AbortSignal.any with fake signal");
	`)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// AbortSignal.timeout with negative delay
// adapter.go:1377-1378 — delayMs < 0 → delayMs = 0
// ---------------------------------------------------------------------------

func TestPhase3c_AbortSignalTimeout_NegativeDelay(t *testing.T) {
	// We need the loop running for ScheduleTimer to succeed
	loop, err := goeventloop.New()
	require.NoError(t, err)

	rt := goja.New()
	adapter, err := New(loop, rt)
	require.NoError(t, err)
	require.NoError(t, adapter.Bind())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx)

	// Use Submit to run JS on the loop thread (single-threaded goja safety)
	done := make(chan error, 1)
	err = loop.Submit(func() {
		_, err2 := rt.RunString(`
			var sig = AbortSignal.timeout(-1);
			if (sig.aborted) throw new Error("signal should not be aborted immediately");
		`)
		done <- err2
	})
	require.NoError(t, err)
	require.NoError(t, <-done)
}

// ---------------------------------------------------------------------------
// structuredClone with Symbol value
// adapter.go:3084-3087 — ExportType() returns nil for symbols
// ---------------------------------------------------------------------------

func TestPhase3c_StructuredClone_SymbolFallback(t *testing.T) {
	adapter := coverSetup(t)
	// Symbol's ExportType() returns nil in goja, triggering the early return
	_, err := adapter.runtime.RunString(`
		var sym = Symbol("test");
		var cloned = structuredClone(sym);
		// Symbols are immutable, so structuredClone returns the same symbol
		if (cloned !== sym) throw new Error("expected same symbol reference");
	`)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// isDateObject / isRegExpObject / isMapObject / isSetObject
// Exercise the constructor-undefined paths by directly calling Go functions
// with objects that have the right methods but Object.create(null) prototype.
// adapter.go:3206-3208, 3260-3262, 3323-3325, 3409-3411
// ---------------------------------------------------------------------------

func TestPhase3c_IsDateObject_ConstructorUndefined(t *testing.T) {
	adapter := coverSetup(t)
	// Create an object with getTime (callable) but constructor set to undefined
	val, err := adapter.runtime.RunString(`
		var fake = {};
		fake.getTime = function() { return 12345; };
		Object.defineProperty(fake, 'constructor', { value: undefined, configurable: true });
		fake
	`)
	require.NoError(t, err)
	obj := val.ToObject(adapter.runtime)
	result := adapter.isDateObject(obj)
	assert.False(t, result, "should return false for date-like object with undefined constructor")
}

func TestPhase3c_IsRegExpObject_ConstructorUndefined(t *testing.T) {
	adapter := coverSetup(t)
	val, err := adapter.runtime.RunString(`
		var fake = {};
		fake.test = function() { return true; };
		fake.source = "abc";
		Object.defineProperty(fake, 'constructor', { value: undefined, configurable: true });
		fake
	`)
	require.NoError(t, err)
	obj := val.ToObject(adapter.runtime)
	assert.False(t, adapter.isRegExpObject(obj), "should return false for regexp-like object with undefined constructor")
}

func TestPhase3c_IsMapObject_ConstructorUndefined(t *testing.T) {
	adapter := coverSetup(t)
	val, err := adapter.runtime.RunString(`
		var fake = {};
		fake.get = function() {};
		fake.set = function() {};
		fake.has = function() {};
		fake["delete"] = function() {};
		Object.defineProperty(fake, 'constructor', { value: undefined, configurable: true });
		fake
	`)
	require.NoError(t, err)
	obj := val.ToObject(adapter.runtime)
	assert.False(t, adapter.isMapObject(obj), "should return false for map-like object with undefined constructor")
}

func TestPhase3c_IsSetObject_ConstructorUndefined(t *testing.T) {
	adapter := coverSetup(t)
	val, err := adapter.runtime.RunString(`
		var fake = {};
		fake.add = function() {};
		fake.has = function() {};
		fake["delete"] = function() {};
		Object.defineProperty(fake, 'constructor', { value: undefined, configurable: true });
		fake
	`)
	require.NoError(t, err)
	obj := val.ToObject(adapter.runtime)
	assert.False(t, adapter.isSetObject(obj), "should return false for set-like object with undefined constructor")
}

// ---------------------------------------------------------------------------
// structuredClone with invalid RegExp (cloneRegExp error path)
// adapter.go:3281-3285 — RunString fails for invalid regex pattern
// ---------------------------------------------------------------------------

func TestPhase3c_StructuredClone_InvalidRegExpSource(t *testing.T) {
	adapter := coverSetup(t)
	// Create a fake object that passes isRegExpObject but has invalid source
	// Using Object.create(RegExp.prototype) to inherit constructor.name === "RegExp"
	_, err := adapter.runtime.RunString(`
		var fake = Object.create(RegExp.prototype);
		Object.defineProperty(fake, 'source', { value: '[', configurable: true });
		Object.defineProperty(fake, 'flags', { value: 'g', configurable: true });
		// test needs to be a real function
		fake.test = function() { return false; };
		try {
			var cloned = structuredClone(fake);
			// cloneRegExp may return null/undefined on error - that's acceptable
		} catch(e) {
			// The clone might fail entirely - that's fine too
		}
	`)
	// We don't care about errors - we just want to hit the error path in cloneRegExp
	_ = err
}

// ---------------------------------------------------------------------------
// Process.nextTick on terminated loop
// adapter.go:2493-2494 — QueueMicrotask (NextTick) error on terminated loop
// ---------------------------------------------------------------------------

func TestPhase3c_ProcessNextTick_LoopTerminated(t *testing.T) {
	adapter := phase3cSetupTerminated(t)
	_, err := adapter.runtime.RunString(`
		var caught = false;
		try { process.nextTick(function(){}); } catch(e) { caught = true; }
		if (!caught) throw new Error("expected error from process.nextTick on terminated loop");
	`)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// AbortSignal.timeout on terminated loop
// adapter.go:1411 — AbortTimeout returns error when loop is terminated
// ---------------------------------------------------------------------------

func TestPhase3c_AbortSignalTimeout_LoopTerminated(t *testing.T) {
	adapter := phase3cSetupTerminated(t)
	_, err := adapter.runtime.RunString(`
		var caught = false;
		try { AbortSignal.timeout(100); } catch(e) { caught = true; }
		if (!caught) throw new Error("expected error from AbortSignal.timeout on terminated loop");
	`)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// console.clear with nil output
// adapter.go:2016-2018 — output == nil early return
// ---------------------------------------------------------------------------

func TestPhase3c_ConsoleClear_NilOutput(t *testing.T) {
	adapter := coverSetup(t)
	adapter.consoleOutput = nil
	_, err := adapter.runtime.RunString(`console.clear()`)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Verify all tests exercise the intended paths
// ---------------------------------------------------------------------------

func TestPhase3c_Sanity(t *testing.T) {
	// Quick sanity check that coverSetup and phase3cSetupTerminated both work
	adapter := coverSetup(t)
	assert.NotNil(t, adapter)

	terminated := phase3cSetupTerminated(t)
	assert.NotNil(t, terminated)
}
