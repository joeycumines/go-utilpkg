package gojaeventloop

import (
	"context"
	"strings"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// ===============================================
// AbortController/AbortSignal Tests
// ===============================================

// TestAbortController_Basic tests basic AbortController functionality from JavaScript.
func TestAbortController_Basic(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Test creating AbortController
	_, err = runtime.RunString(`
		const controller = new AbortController();
		if (!controller.signal) {
			throw new Error("signal should exist");
		}
		if (controller.signal.aborted) {
			throw new Error("should not be aborted initially");
		}
	`)
	if err != nil {
		t.Fatalf("AbortController basic test failed: %v", err)
	}
}

// TestAbortController_Abort tests abort functionality from JavaScript.
func TestAbortController_Abort(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Test aborting
	_, err = runtime.RunString(`
		const controller = new AbortController();
		controller.abort("test reason");
		if (!controller.signal.aborted) {
			throw new Error("should be aborted after abort()");
		}
	`)
	if err != nil {
		t.Fatalf("AbortController abort test failed: %v", err)
	}
}

// TestAbortController_OnAbort tests onabort handler from JavaScript.
func TestAbortController_OnAbort(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Test onabort handler
	_, err = runtime.RunString(`
		const controller = new AbortController();
		let handlerCalled = false;
		controller.signal.onabort = function(reason) {
			handlerCalled = true;
		};
		controller.abort();
		if (!handlerCalled) {
			throw new Error("onabort handler should have been called");
		}
	`)
	if err != nil {
		t.Fatalf("AbortController onabort test failed: %v", err)
	}
}

// TestAbortController_AddEventListener tests addEventListener from JavaScript.
func TestAbortController_AddEventListener(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Test addEventListener
	_, err = runtime.RunString(`
		const controller = new AbortController();
		let eventReceived = false;
		controller.signal.addEventListener('abort', function(event) {
			eventReceived = true;
			if (event.type !== 'abort') {
				throw new Error("event type should be 'abort'");
			}
		});
		controller.abort();
		if (!eventReceived) {
			throw new Error("abort event should have been received");
		}
	`)
	if err != nil {
		t.Fatalf("AbortController addEventListener test failed: %v", err)
	}
}

// TestAbortController_ThrowIfAborted tests throwIfAborted from JavaScript.
func TestAbortController_ThrowIfAborted(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Test throwIfAborted
	_, err = runtime.RunString(`
		const controller = new AbortController();

		// Should not throw when not aborted
		try {
			controller.signal.throwIfAborted();
		} catch (e) {
			throw new Error("should not throw when not aborted");
		}

		// Should throw after abort
		controller.abort();
		let didThrow = false;
		try {
			controller.signal.throwIfAborted();
		} catch (e) {
			didThrow = true;
		}
		if (!didThrow) {
			throw new Error("should throw when aborted");
		}
	`)
	if err != nil {
		t.Fatalf("AbortController throwIfAborted test failed: %v", err)
	}
}

// TestAbortSignal_CannotConstruct tests that AbortSignal cannot be constructed directly.
func TestAbortSignal_CannotConstruct(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Test that AbortSignal cannot be constructed
	_, err = runtime.RunString(`
		let didThrow = false;
		try {
			new AbortSignal();
		} catch (e) {
			didThrow = true;
		}
		if (!didThrow) {
			throw new Error("AbortSignal should not be constructable");
		}
	`)
	if err != nil {
		t.Fatalf("AbortSignal construct test failed: %v", err)
	}
}

// ===============================================
// Performance API Tests
// ===============================================

// TestPerformance_Now tests performance.now() from JavaScript.
func TestPerformance_Now(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Test performance.now()
	_, err = runtime.RunString(`
		const t1 = performance.now();
		if (typeof t1 !== 'number') {
			throw new Error("performance.now() should return a number");
		}
		if (t1 < 0) {
			throw new Error("performance.now() should return non-negative value");
		}

		// Second call should be >= first
		const t2 = performance.now();
		if (t2 < t1) {
			throw new Error("performance.now() should be monotonically increasing");
		}
	`)
	if err != nil {
		t.Fatalf("performance.now() test failed: %v", err)
	}
}

// TestPerformance_TimeOrigin tests performance.timeOrigin from JavaScript.
func TestPerformance_TimeOrigin(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Test performance.timeOrigin
	_, err = runtime.RunString(`
		const origin = performance.timeOrigin;
		if (typeof origin !== 'number') {
			throw new Error("performance.timeOrigin should return a number");
		}
		if (origin <= 0) {
			throw new Error("performance.timeOrigin should be positive");
		}
	`)
	if err != nil {
		t.Fatalf("performance.timeOrigin test failed: %v", err)
	}
}

func TestPerformance_RetainedPrototypeAndBrand(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind performance: %v", err)
	}

	_, err = runtime.RunString(`
		if (!(performance instanceof Performance)) throw new Error("missing Performance brand");
		if (!(performance instanceof EventTarget)) throw new Error("missing EventTarget inheritance");
		if (Object.getPrototypeOf(performance) !== Performance.prototype) throw new Error("wrong prototype");
		if (Object.getPrototypeOf(Performance.prototype) !== EventTarget.prototype) throw new Error("wrong prototype inheritance");
		if (Object.keys(performance).length !== 0) throw new Error("performance has own enumerable properties");
		if (Object.prototype.toString.call(performance) !== "[object Performance]") throw new Error("wrong toStringTag");
		for (const name of ["mark", "measure", "getEntries", "getEntriesByType", "getEntriesByName", "clearMarks", "clearMeasures", "clearResourceTimings"]) {
			if (name in performance) throw new Error(name + " must not be installed");
		}
		if (Performance.name !== "Performance" || Performance.length !== 0) throw new Error("constructor metadata");
		const now = Object.getOwnPropertyDescriptor(Performance.prototype, "now");
		if (!now || typeof now.value !== "function" || now.value.name !== "now" || now.value.length !== 0 || !now.writable || !now.enumerable || !now.configurable) throw new Error("now descriptor");
		const origin = Object.getOwnPropertyDescriptor(Performance.prototype, "timeOrigin");
		if (!origin || typeof origin.get !== "function" || origin.get.name !== "get timeOrigin" || origin.get.length !== 0 || origin.set !== undefined || !origin.enumerable || !origin.configurable) throw new Error("timeOrigin descriptor");
		const toJSON = Object.getOwnPropertyDescriptor(Performance.prototype, "toJSON");
		if (!toJSON || toJSON.value.name !== "toJSON" || toJSON.value.length !== 0 || !toJSON.writable || !toJSON.enumerable || !toJSON.configurable) throw new Error("toJSON descriptor");
		const json = performance.toJSON();
		if (Object.getPrototypeOf(json) !== Object.prototype || Reflect.ownKeys(json).join() !== "timeOrigin" || json.timeOrigin !== performance.timeOrigin) {
			throw new Error("toJSON result");
		}
		function observe(call) {
			try { call(); return "missing"; }
			catch (error) { return [error.name, String(error.code), error.message].join(":"); }
		}
		const receiver = 'TypeError:undefined:Value of "this" must be of type Performance';
		for (const call of [
			() => Reflect.apply(Performance.prototype.now, {}, []),
			() => Reflect.apply(origin.get, {}, []),
			() => Reflect.apply(toJSON.value, {}, []),
		]) if (observe(call) !== receiver) throw new Error("receiver check: " + observe(call));
		if (observe(() => new Performance()) !== "TypeError:undefined:Illegal constructor") throw new Error("constructor check");
	`)
	if err != nil {
		t.Fatalf("performance retained profile: %v", err)
	}
}

func TestPerformance_ForeignPairPreservationAndPartialRejection(t *testing.T) {
	for _, test := range []struct {
		name        string
		initializer string
		wantError   bool
	}{
		{name: "full pair", initializer: `globalThis.Performance = function ForeignPerformance() {}; globalThis.performance = Object.create(Performance.prototype);`},
		{name: "constructor only", initializer: `globalThis.Performance = function ForeignPerformance() {};`, wantError: true},
		{name: "singleton only", initializer: `globalThis.performance = { sentinel: true };`, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := goja.New()
			if _, err := runtime.RunString(test.initializer); err != nil {
				t.Fatalf("install foreign performance state: %v", err)
			}
			constructor := runtime.Get("Performance")
			singleton := runtime.Get("performance")
			_, preserved, err := coherentHostSingleton(runtime, "performance", "Performance")
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), "is partial") {
					t.Fatalf("coherent performance error = %v, want partial-pair error", err)
				}
			} else if err != nil {
				t.Fatalf("coherent performance: %v", err)
			} else if !preserved {
				t.Fatal("foreign performance pair was not recognized")
			}
			sameValue := func(left, right goja.Value) bool {
				if left == nil || right == nil {
					return left == nil && right == nil
				}
				return left.SameAs(right)
			}
			if !sameValue(runtime.Get("Performance"), constructor) || !sameValue(runtime.Get("performance"), singleton) {
				t.Fatal("pair inspection changed foreign globals")
			}
		})
	}
}

// TestAbortController_WithFetch tests AbortController with a simulated fetch-like operation.
func TestAbortController_WithFetch(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start the loop
	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	// Wait a bit for loop to start
	time.Sleep(10 * time.Millisecond)

	// Test AbortController with simulated fetch
	_, err = runtime.RunString(`
		const controller = new AbortController();
		const signal = controller.signal;

		// Simulate a cancellable operation
		let operationCompleted = false;
		let operationAborted = false;

		signal.addEventListener('abort', function() {
			operationAborted = true;
		});

		// Abort immediately
		controller.abort("User cancelled");

		// Check state
		if (!signal.aborted) {
			throw new Error("signal should be aborted");
		}
		if (!operationAborted) {
			throw new Error("abort handler should have been called");
		}
	`)
	if err != nil {
		t.Fatalf("AbortController with fetch test failed: %v", err)
	}

	// Shutdown
	_ = loop.Shutdown(context.Background())
	<-done
}

// ===============================================
// AbortSignal.any() Tests
// ===============================================

// TestAbortSignal_Any_Basic tests AbortSignal.any() with multiple signals.
func TestAbortSignal_Any_Basic(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const c1 = new AbortController();
		const c2 = new AbortController();

		const combined = AbortSignal.any([c1.signal, c2.signal]);

		if (combined.aborted) {
			throw new Error("combined should not be aborted initially");
		}

		// Abort first controller
		c1.abort("first reason");

		if (!combined.aborted) {
			throw new Error("combined should be aborted after c1 aborts");
		}
	`)
	if err != nil {
		t.Fatalf("AbortSignal.any basic test failed: %v", err)
	}
}

// TestAbortSignal_Any_AlreadyAborted tests AbortSignal.any() with pre-aborted signal.
func TestAbortSignal_Any_AlreadyAborted(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const c1 = new AbortController();
		const c2 = new AbortController();

		// Abort c1 before creating combined
		c1.abort("pre-aborted");

		const combined = AbortSignal.any([c1.signal, c2.signal]);

		if (!combined.aborted) {
			throw new Error("combined should be immediately aborted");
		}
	`)
	if err != nil {
		t.Fatalf("AbortSignal.any pre-aborted test failed: %v", err)
	}
}

// TestAbortSignal_Any_Empty tests AbortSignal.any() with empty array.
func TestAbortSignal_Any_Empty(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const combined = AbortSignal.any([]);

		if (combined.aborted) {
			throw new Error("combined with empty array should not be aborted");
		}
	`)
	if err != nil {
		t.Fatalf("AbortSignal.any empty test failed: %v", err)
	}
}

// TestAbortSignal_Any_OnAbort tests AbortSignal.any() with onabort handler.
func TestAbortSignal_Any_OnAbort(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)

	_, err = runtime.RunString(`
		const c1 = new AbortController();
		const c2 = new AbortController();

		const combined = AbortSignal.any([c1.signal, c2.signal]);

		let handlerCalled = false;
		combined.onabort = function(reason) {
			handlerCalled = true;
		};

		c2.abort("test");

		if (!handlerCalled) {
			throw new Error("onabort handler should have been called");
		}
	`)
	if err != nil {
		t.Fatalf("AbortSignal.any onabort test failed: %v", err)
	}

	_ = loop.Shutdown(context.Background())
	<-done
}

// ===============================================
// AbortSignal.timeout() Tests
// ===============================================

// TestAbortSignal_Timeout_Basic tests AbortSignal.timeout() basic functionality.
func TestAbortSignal_Timeout_Basic(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		globalThis.timeoutSignal = AbortSignal.timeout(50);

		if (timeoutSignal.aborted) {
			throw new Error("signal should not be aborted immediately");
		}
	`)
	if err != nil {
		t.Fatalf("AbortSignal.timeout basic test failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	// Wait for timeout to fire
	time.Sleep(100 * time.Millisecond)
	_ = loop.Shutdown(context.Background())
	<-done

	_, err = runtime.RunString(`
		if (!timeoutSignal.aborted) {
			throw new Error("signal should be aborted after timeout");
		}
	`)
	if err != nil {
		t.Fatalf("AbortSignal.timeout continuation failed: %v", err)
	}
}

// TestAbortSignal_Timeout_Fires tests that AbortSignal.timeout() actually aborts.
func TestAbortSignal_Timeout_Fires(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Create a promise that resolves when timeout fires
	runtime.Set("testResolve", nil)
	runtime.Set("testResult", nil)

	_, err = runtime.RunString(`
		let resolve;
		const promise = new Promise(r => { resolve = r; });

		const signal = AbortSignal.timeout(30);
		signal.onabort = function() {
			resolve(signal.aborted);
		};

		testResolve = resolve;
	`)
	if err != nil {
		t.Fatalf("AbortSignal.timeout setup failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	// Wait for timeout to fire
	time.Sleep(100 * time.Millisecond)

	_ = loop.Shutdown(context.Background())
	<-done
}

// TestAbortSignal_Timeout_Zero tests AbortSignal.timeout(0).
func TestAbortSignal_Timeout_Zero(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	_, err = runtime.RunString(`
		const signal = AbortSignal.timeout(0);

		// Should not throw
		if (typeof signal.aborted !== 'boolean') {
			throw new Error("signal.aborted should be a boolean");
		}
	`)
	if err != nil {
		t.Fatalf("AbortSignal.timeout(0) test failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- loop.Run(ctx)
	}()

	_ = loop.Shutdown(context.Background())
	<-done
}
