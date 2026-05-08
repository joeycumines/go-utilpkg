package gojaeventloop

import (
	"testing"
)

// Timer, immediate, microtask, and Promise scheduling edge coverage.

func TestSetTimeout_NullArg(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { setTimeout(null, 0); var stErr = false; } catch(e) { var stErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("stErr").ToBoolean() {
		t.Error("Expected TypeError for setTimeout(null)")
	}
}

func TestSetInterval_NullArg(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { setInterval(null, 0); var siErr = false; } catch(e) { var siErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("siErr").ToBoolean() {
		t.Error("Expected TypeError for setInterval(null)")
	}
}

func TestQueueMicrotask_NullArg(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { queueMicrotask(null); var qmErr = false; } catch(e) { var qmErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("qmErr").ToBoolean() {
		t.Error("Expected TypeError for queueMicrotask(null)")
	}
}

func TestSetImmediate_NullArg(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { setImmediate(null); var simErr = false; } catch(e) { var simErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("simErr").ToBoolean() {
		t.Error("Expected TypeError for setImmediate(null)")
	}
}

func TestSetTimeout_NonFunctionArg(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { setTimeout(42, 0); var stNfErr = false; } catch(e) { var stNfErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("stNfErr").ToBoolean() {
		t.Error("Expected TypeError for setTimeout(42)")
	}
}

func TestSetTimeout_NegativeDelay(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var called = false;
		setTimeout(function() { called = true; }, -100);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
}

func TestSetInterval_NegativeDelay(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var count = 0;
		var id = setInterval(function() { count++; if (count >= 2) clearInterval(id); }, -50);
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 200)
}

func TestPromise_ExecutorThrows_CoverG3(t *testing.T) {
	adapter := coverSetupWithLoop(t)

	_, err := adapter.runtime.RunString(`
		var caught = false;
		var p = new Promise(function(resolve, reject) {
			throw new Error("executor error");
		});
		p.catch(function(e) { caught = true; });
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
}

func TestPromise_ConstructorNullExecutor(t *testing.T) {
	adapter := coverSetup(t)

	_, err := adapter.runtime.RunString(`
		try { new Promise(null); var pExErr = false; } catch(e) { var pExErr = true; }
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !adapter.runtime.Get("pExErr").ToBoolean() {
		t.Error("Expected TypeError for Promise(null)")
	}
}
