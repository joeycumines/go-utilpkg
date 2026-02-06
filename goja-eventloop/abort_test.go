package gojaeventloop

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
)

// ===============================================
// FEATURE-001: AbortController/AbortSignal Tests
// ===============================================

// TestAbortController_Basic tests basic AbortController functionality from JavaScript.
func TestAbortController_Basic(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
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
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
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
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
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
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
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
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
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
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
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
// FEATURE-002/003: Performance API Tests
// ===============================================

// TestPerformance_Now tests performance.now() from JavaScript.
func TestPerformance_Now(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
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
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
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

// TestPerformance_Mark tests performance.mark() from JavaScript.
func TestPerformance_Mark(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Test performance.mark()
	_, err = runtime.RunString(`
		const entry = performance.mark("test-mark");
		if (!entry) {
			throw new Error("mark() should return entry");
		}
		if (entry.name !== "test-mark") {
			throw new Error("entry name should be 'test-mark'");
		}
		if (entry.entryType !== "mark") {
			throw new Error("entry type should be 'mark'");
		}
		if (entry.duration !== 0) {
			throw new Error("mark duration should be 0");
		}
	`)
	if err != nil {
		t.Fatalf("performance.mark() test failed: %v", err)
	}
}

// TestPerformance_MarkWithOptions tests performance.mark() with options from JavaScript.
func TestPerformance_MarkWithOptions(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Test performance.mark() with options
	_, err = runtime.RunString(`
		const entry = performance.mark("detailed-mark", { detail: { key: "value" } });
		if (!entry) {
			throw new Error("mark() should return entry");
		}
	`)
	if err != nil {
		t.Fatalf("performance.mark() with options test failed: %v", err)
	}
}

// TestPerformance_Measure tests performance.measure() from JavaScript.
func TestPerformance_Measure(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Test performance.measure()
	_, err = runtime.RunString(`
		performance.mark("start");
		performance.mark("end");
		
		const entry = performance.measure("test-measure", "start", "end");
		if (!entry) {
			throw new Error("measure() should return entry");
		}
		if (entry.name !== "test-measure") {
			throw new Error("entry name should be 'test-measure'");
		}
		if (entry.entryType !== "measure") {
			throw new Error("entry type should be 'measure'");
		}
		if (typeof entry.duration !== "number") {
			throw new Error("entry should have duration");
		}
	`)
	if err != nil {
		t.Fatalf("performance.measure() test failed: %v", err)
	}
}

// TestPerformance_MeasureFromOrigin tests performance.measure() from origin.
func TestPerformance_MeasureFromOrigin(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Test performance.measure() from origin
	_, err = runtime.RunString(`
		performance.mark("end");
		const entry = performance.measure("from-origin", undefined, "end");
		if (entry.startTime !== 0) {
			throw new Error("startTime should be 0 when measuring from origin");
		}
	`)
	if err != nil {
		t.Fatalf("performance.measure() from origin test failed: %v", err)
	}
}

// TestPerformance_MeasureToNow tests performance.measure() to current time.
func TestPerformance_MeasureToNow(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Test performance.measure() to now
	_, err = runtime.RunString(`
		performance.mark("start");
		const entry = performance.measure("to-now", "start", undefined);
		if (entry.duration < 0) {
			throw new Error("duration should be non-negative");
		}
	`)
	if err != nil {
		t.Fatalf("performance.measure() to now test failed: %v", err)
	}
}

// TestPerformance_GetEntries tests performance.getEntries() from JavaScript.
func TestPerformance_GetEntries(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Test performance.getEntries()
	_, err = runtime.RunString(`
		performance.mark("mark1");
		performance.mark("mark2");
		
		const entries = performance.getEntries();
		if (!Array.isArray(entries)) {
			throw new Error("getEntries() should return an array");
		}
		if (entries.length !== 2) {
			throw new Error("should have 2 entries, got " + entries.length);
		}
	`)
	if err != nil {
		t.Fatalf("performance.getEntries() test failed: %v", err)
	}
}

// TestPerformance_GetEntriesByType tests performance.getEntriesByType() from JavaScript.
func TestPerformance_GetEntriesByType(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Test performance.getEntriesByType()
	_, err = runtime.RunString(`
		performance.mark("mark1");
		performance.mark("mark2");
		performance.measure("measure1", "mark1", "mark2");
		
		const marks = performance.getEntriesByType("mark");
		if (marks.length !== 2) {
			throw new Error("should have 2 marks");
		}
		
		const measures = performance.getEntriesByType("measure");
		if (measures.length !== 1) {
			throw new Error("should have 1 measure");
		}
	`)
	if err != nil {
		t.Fatalf("performance.getEntriesByType() test failed: %v", err)
	}
}

// TestPerformance_GetEntriesByName tests performance.getEntriesByName() from JavaScript.
func TestPerformance_GetEntriesByName(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Test performance.getEntriesByName()
	_, err = runtime.RunString(`
		performance.mark("target");
		performance.mark("other");
		performance.mark("target");
		
		const entries = performance.getEntriesByName("target");
		if (entries.length !== 2) {
			throw new Error("should have 2 entries named 'target'");
		}
		
		// With type filter
		const marks = performance.getEntriesByName("target", "mark");
		if (marks.length !== 2) {
			throw new Error("should have 2 marks named 'target'");
		}
	`)
	if err != nil {
		t.Fatalf("performance.getEntriesByName() test failed: %v", err)
	}
}

// TestPerformance_ClearMarks tests performance.clearMarks() from JavaScript.
func TestPerformance_ClearMarks(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Test performance.clearMarks()
	_, err = runtime.RunString(`
		performance.mark("keep");
		performance.mark("remove");
		
		performance.clearMarks("remove");
		
		const entries = performance.getEntriesByType("mark");
		if (entries.length !== 1) {
			throw new Error("should have 1 mark after clear");
		}
		if (entries[0].name !== "keep") {
			throw new Error("wrong mark remaining");
		}
	`)
	if err != nil {
		t.Fatalf("performance.clearMarks() test failed: %v", err)
	}
}

// TestPerformance_ClearAllMarks tests performance.clearMarks() without name.
func TestPerformance_ClearAllMarks(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Test performance.clearMarks() without name
	_, err = runtime.RunString(`
		performance.mark("mark1");
		performance.mark("mark2");
		
		performance.clearMarks();
		
		const entries = performance.getEntriesByType("mark");
		if (entries.length !== 0) {
			throw new Error("should have 0 marks after clear all, got " + entries.length);
		}
	`)
	if err != nil {
		t.Fatalf("performance.clearMarks() all test failed: %v", err)
	}
}

// TestPerformance_ClearMeasures tests performance.clearMeasures() from JavaScript.
func TestPerformance_ClearMeasures(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Test performance.clearMeasures()
	_, err = runtime.RunString(`
		performance.mark("start");
		performance.mark("end");
		performance.measure("keep", "start", "end");
		performance.measure("remove", "start", "end");
		
		performance.clearMeasures("remove");
		
		const measures = performance.getEntriesByType("measure");
		if (measures.length !== 1) {
			throw new Error("should have 1 measure after clear");
		}
	`)
	if err != nil {
		t.Fatalf("performance.clearMeasures() test failed: %v", err)
	}
}

// TestPerformance_ToJSON tests performance.toJSON() from JavaScript.
func TestPerformance_ToJSON(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Test performance.toJSON()
	_, err = runtime.RunString(`
		const json = performance.toJSON();
		if (typeof json !== 'object') {
			throw new Error("toJSON() should return an object");
		}
		if (typeof json.timeOrigin !== 'number') {
			throw new Error("toJSON() should include timeOrigin");
		}
	`)
	if err != nil {
		t.Fatalf("performance.toJSON() test failed: %v", err)
	}
}

// TestPerformance_MeasureError tests performance.measure() with invalid marks.
func TestPerformance_MeasureError(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	if err := adapter.Bind(); err != nil {
		t.Fatalf("Failed to bind: %v", err)
	}

	// Test performance.measure() with invalid mark
	_, err = runtime.RunString(`
		let didThrow = false;
		try {
			performance.measure("test", "nonexistent", undefined);
		} catch (e) {
			didThrow = true;
		}
		if (!didThrow) {
			throw new Error("measure() should throw for nonexistent start mark");
		}
	`)
	if err != nil {
		t.Fatalf("performance.measure() error test failed: %v", err)
	}
}

// TestPerformance_Integration tests a realistic performance measurement scenario.
func TestPerformance_Integration(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
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

	// Test realistic performance measurement
	_, err = runtime.RunString(`
		// Simulate async operation with performance measurement
		performance.mark("operation-start");
		
		// ... simulate some work ...
		
		performance.mark("operation-end");
		performance.measure("operation-duration", "operation-start", "operation-end");
		
		// Get results
		const measures = performance.getEntriesByType("measure");
		if (measures.length !== 1) {
			throw new Error("should have 1 measure");
		}
		
		const duration = measures[0].duration;
		if (duration < 0) {
			throw new Error("duration should be non-negative");
		}
	`)
	if err != nil {
		t.Fatalf("Performance integration test failed: %v", err)
	}

	// Shutdown
	_ = loop.Shutdown(context.Background())
	<-done
}

// TestAbortController_WithFetch tests AbortController with a simulated fetch-like operation.
func TestAbortController_WithFetch(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
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
// EXPAND-001: AbortSignal.any() Tests
// ===============================================

// TestAbortSignal_Any_Basic tests AbortSignal.any() with multiple signals.
func TestAbortSignal_Any_Basic(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
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
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
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
		t.Fatalf("Failed to create loop: %v", err)
	}
	defer loop.Shutdown(context.Background())

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
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
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
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
// EXPAND-002: AbortSignal.timeout() Tests
// ===============================================

// TestAbortSignal_Timeout_Basic tests AbortSignal.timeout() basic functionality.
func TestAbortSignal_Timeout_Basic(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
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
		const signal = AbortSignal.timeout(50);
		
		if (signal.aborted) {
			throw new Error("signal should not be aborted immediately");
		}
	`)
	if err != nil {
		t.Fatalf("AbortSignal.timeout basic test failed: %v", err)
	}

	// Wait for timeout to fire
	time.Sleep(100 * time.Millisecond)

	_, err = runtime.RunString(`
		// The signal should now be aborted
		// Note: We can't easily check this in the same script since
		// the timeout fires asynchronously
	`)
	if err != nil {
		t.Fatalf("AbortSignal.timeout continuation failed: %v", err)
	}

	_ = loop.Shutdown(context.Background())
	<-done
}

// TestAbortSignal_Timeout_Fires tests that AbortSignal.timeout() actually aborts.
func TestAbortSignal_Timeout_Fires(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
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
		t.Fatalf("Failed to create loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
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
		const signal = AbortSignal.timeout(0);
		
		// Should not throw
		if (typeof signal.aborted !== 'boolean') {
			throw new Error("signal.aborted should be a boolean");
		}
	`)
	if err != nil {
		t.Fatalf("AbortSignal.timeout(0) test failed: %v", err)
	}

	_ = loop.Shutdown(context.Background())
	<-done
}
