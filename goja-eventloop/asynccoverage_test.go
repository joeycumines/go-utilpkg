package gojaeventloop

import (
	"context"
	"testing"
	"time"

	"github.com/joeycumines/goja"
)

// coverSetupWithLoop creates adapter WITHOUT starting the loop.
// Use coverRunLoopBriefly() after RunString to process async operations.
// This prevents data races between the loop goroutine and direct runtime access.
func coverSetupWithLoop(t *testing.T) *Adapter {
	t.Helper()
	return coverSetup(t)
}

// coverRunLoopBriefly starts the event loop, waits for the specified duration
// to allow async operations (timers, microtasks, promises) to process, then
// stops the loop and waits for it to finish. After this returns, it is safe
// to access adapter.runtime directly.
func coverRunLoopBriefly(t *testing.T, adapter *Adapter, waitMs int) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- adapter.loop.Run(ctx) }()
	time.Sleep(time.Duration(waitMs) * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("loop did not stop in time")
	}
}

// ===========================================================================
// Timer error paths — setInterval/setTimeout with nil Export
// ===========================================================================

func TestSetInterval_ErrorPaths(t *testing.T) {
	adapter := coverSetup(t)

	// setInterval with null function
	_, err := adapter.runtime.RunString(`
		try { setInterval(null, 10); } catch(e) { var err1 = true; }
	`)
	if err != nil {
		t.Fatalf("setInterval(null) failed: %v", err)
	}

	// setInterval with non-function
	_, err = adapter.runtime.RunString(`
		try { setInterval("not a function", 10); } catch(e) { var err2 = true; }
	`)
	if err != nil {
		t.Fatalf("setInterval(string) failed: %v", err)
	}

	// Negative delay clamped to 0
	_, err = adapter.runtime.RunString(`
		var intervalCalled = false;
		var iid = setInterval(function() { intervalCalled = true; }, -100);
		clearInterval(iid);
	`)
	if err != nil {
		t.Fatalf("setInterval with negative delay failed: %v", err)
	}
}

func TestQueueMicrotask_ErrorPaths(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { queueMicrotask(null); } catch(e) { var qErr1 = true; }
	`)
	if err != nil {
		t.Fatalf("queueMicrotask(null) failed: %v", err)
	}

	_, err = adapter.runtime.RunString(`
		try { queueMicrotask("not a function"); } catch(e) { var qErr2 = true; }
	`)
	if err != nil {
		t.Fatalf("queueMicrotask(string) failed: %v", err)
	}
}

func TestSetImmediate_ErrorPaths(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { setImmediate(null); } catch(e) { var siErr1 = true; }
	`)
	if err != nil {
		t.Fatalf("setImmediate(null) failed: %v", err)
	}

	_, err = adapter.runtime.RunString(`
		try { setImmediate("not a function"); } catch(e) { var siErr2 = true; }
	`)
	if err != nil {
		t.Fatalf("setImmediate(string) failed: %v", err)
	}
}

// ===========================================================================
// Promise prototype error paths — then/catch/finally on non-Promise
// ===========================================================================

// Native Promise.prototype receiver checks are covered by focused tests.

// ===========================================================================
// process.nextTick error paths
// ===========================================================================

func TestProcessNextTick_ErrorPaths(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { process.nextTick(null); } catch(e) { var ntErr1 = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}

	_, err = adapter.runtime.RunString(`
		try { process.nextTick("not a function"); } catch(e) { var ntErr2 = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// consumeIterable — error paths
// ===========================================================================

func TestConsumeIterable_NullUndefined(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			Promise.all(null);
		} catch(e) {
			// Might be caught or rejected
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestConsumeIterable_NonIterable(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try {
			Promise.all(42);
		} catch(e) {
			var nonIterErr = true;
		}
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// resolveThenable — various paths
// ===========================================================================

func TestResolveThenable_NullUndefined(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		// Promise.resolve with null/undefined
		var r1, r2;
		Promise.resolve(null).then(function(v) { r1 = v; });
		Promise.resolve(undefined).then(function(v) { r2 = v; });
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
}

func TestResolveThenable_NonObjectWithThen(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		// A non-object that has a then property (e.g. string) should not be treated as thenable
		var resolvedVal;
		Promise.resolve("stringValue").then(function(v) { resolvedVal = v; });
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	val := adapter.runtime.Get("resolvedVal")
	if val == nil || val.String() != "stringValue" {
		t.Errorf("Expected 'stringValue', got %v", val)
	}
}

func TestResolveThenable_ThenableThrows(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var rejected = false;
		var thenable = {
			then: function(resolve, reject) {
				throw new Error("thenable error");
			}
		};
		Promise.resolve(thenable).catch(function(e) { rejected = true; });
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	val := adapter.runtime.Get("rejected")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected thenable throw to reject")
	}
}

func TestResolveThenable_ThenNotFunction(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var resolvedVal;
		var obj = { then: "not a function" };
		Promise.resolve(obj).then(function(v) { resolvedVal = v; });
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
}

// ===========================================================================
// Native Promise AggregateError behavior
// ===========================================================================

func TestConvertToGojaValue_AggregateError(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var errResult;
		Promise.any([
			Promise.reject("e1"),
			Promise.reject("e2")
		]).catch(function(e) {
			errResult = {
				name: e.name,
				hasErrors: Array.isArray(e.errors),
				count: e.errors ? e.errors.length : 0
			};
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	val := adapter.runtime.Get("errResult")
	if val == nil || goja.IsUndefined(val) {
		t.Fatal("errResult is nil/undefined")
	}
	obj := val.ToObject(adapter.runtime)
	if obj.Get("name").String() != "AggregateError" {
		t.Error("Expected AggregateError name")
	}
}

// ===========================================================================
// gojaVoidFuncToHandler — non-function input
// ===========================================================================

func TestGojaVoidFuncToHandler_NonFunction(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		// finally with non-function argument should still work (noop)
		var finallyResult;
		Promise.resolve(42).finally("not a function").then(function(v) {
			finallyResult = v;
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
}

// ===========================================================================
// gojaFuncToHandler — handler returns wrapped promise
// ===========================================================================

func TestGojaFuncToHandler_ReturnsPromise(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var chainResult;
		Promise.resolve(1).then(function(v) {
			return Promise.resolve(v + 1);
		}).then(function(v) {
			chainResult = v;
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	val := adapter.runtime.Get("chainResult")
	if val == nil || val.ToInteger() != 2 {
		t.Errorf("Expected 2, got %v", val)
	}
}

// ===========================================================================
// Promise.reject with Error object — preserves .message
// ===========================================================================

func TestPromiseReject_ErrorPreservesMessage(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var errMsg;
		Promise.reject(new Error("test error")).catch(function(e) {
			errMsg = e.message;
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	val := adapter.runtime.Get("errMsg")
	if val == nil || val.String() != "test error" {
		t.Errorf("Expected 'test error', got %v", val)
	}
}

func TestPromiseReject_WrappedPromise(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var isPromise;
		var p = Promise.resolve(42);
		Promise.reject(p).catch(function(reason) {
			// reason should be the promise object itself
			isPromise = reason !== undefined && reason !== null;
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	val := adapter.runtime.Get("isPromise")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected promise as rejection reason")
	}
}

// ===========================================================================
// Promise executor throws
// ===========================================================================

func TestPromise_ExecutorThrows(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var caught;
		new Promise(function(resolve, reject) {
			throw new Error("executor error");
		}).catch(function(e) {
			caught = true;
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	val := adapter.runtime.Get("caught")
	if val == nil || !val.ToBoolean() {
		t.Error("Expected rejection from executor throw")
	}
}

func TestPromise_ExecutorNullReject(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { new Promise(null); } catch(e) { var nullExecErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

func TestPromise_ExecutorNonFunction(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { new Promise("not a function"); } catch(e) { var nonFuncExecErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
}

// ===========================================================================
// Native Promise.allSettled result conversion
// ===========================================================================

func TestGojaFuncToHandler_MapWithWrappedPromise(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var result;
		Promise.allSettled([
			Promise.resolve(1),
			Promise.reject("err")
		]).then(function(results) {
			result = results;
		});
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)

	val := adapter.runtime.Get("result")
	if val == nil || goja.IsUndefined(val) {
		t.Error("Expected allSettled results")
	}
}
