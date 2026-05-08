package gojaeventloop

import (
	"context"
	"testing"

	goeventloop "github.com/joeycumines/go-eventloop"
)

func TestPhase2_Symbol_ForAndKeyFor(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var s1 = Symbol.for("shared");
		var s2 = Symbol.for("shared");
		if (s1 !== s2) throw new Error("Symbol.for should return same symbol");
		var key = Symbol.keyFor(s1);
		if (key !== "shared") throw new Error("Symbol.keyFor wrong: " + key);
	`)
	if err != nil {
		t.Fatalf("Symbol.for and keyFor failed: %v", err)
	}
}

func TestPhase2_Symbol_KeyForUnregistered(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var s = Symbol("local");
		var key = Symbol.keyFor(s);
		if (key !== undefined) throw new Error("unregistered symbol should return undefined");
	`)
	if err != nil {
		t.Fatalf("Symbol.keyFor unregistered failed: %v", err)
	}
}

func TestPhase2_Performance_Now(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var t1 = performance.now();
		if (typeof t1 !== "number" || t1 < 0) throw new Error("performance.now should return non-negative number");
	`)
	if err != nil {
		t.Fatalf("performance.now failed: %v", err)
	}
}

func TestPhase2_Atob_Btoa(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var encoded = btoa("Hello World");
		if (encoded !== "SGVsbG8gV29ybGQ=") throw new Error("btoa wrong: " + encoded);
		var decoded = atob(encoded);
		if (decoded !== "Hello World") throw new Error("atob wrong: " + decoded);
	`)
	if err != nil {
		t.Fatalf("atob/btoa failed: %v", err)
	}
}

func TestPhase2_Atob_InvalidBase64(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		var caught = false;
		try { atob("!!!invalid!!!"); } catch(e) { caught = true; }
		if (!caught) throw new Error("atob should throw for invalid base64");
	`)
	if err != nil {
		t.Fatalf("atob invalid base64 failed: %v", err)
	}
}

func TestPhase2_ProcessNextTick(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var tickVal = null;
		process.nextTick(function() { tickVal = "ticked"; });
	`)
	if err != nil {
		t.Fatalf("process.nextTick setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
	val := adapter.runtime.Get("tickVal")
	if val == nil || val.String() != "ticked" {
		t.Errorf("expected 'ticked', got '%v'", val)
	}
}

func TestPhase2_Delay(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var delayResult = null;
		delay(10).then(function() { delayResult = "done"; });
	`)
	if err != nil {
		t.Fatalf("delay setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
	val := adapter.runtime.Get("delayResult")
	if val == nil || val.String() != "done" {
		t.Errorf("expected 'done', got '%v'", val)
	}
}

func TestPhase2_Fetch_Omitted(t *testing.T) {
	adapter := coverSetup(t)
	value, err := adapter.runtime.RunString(`typeof fetch`)
	if err != nil {
		t.Fatalf("fetch omission check failed: %v", err)
	}
	if got, want := value.String(), "undefined"; got != want {
		t.Fatalf("typeof fetch = %q, want %q", got, want)
	}
}

func TestPhase2_SetInterval_ClearInterval(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var count = 0;
		var id = setInterval(function() { count++; }, 10);
		setTimeout(function() { clearInterval(id); }, 50);
	`)
	if err != nil {
		t.Fatalf("setInterval/clearInterval setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 200)
	val := adapter.runtime.Get("count")
	if val == nil || val.ToInteger() < 1 {
		t.Error("expected count > 0 from interval")
	}
}

func TestPhase2_ClearTimeout_NonExistent(t *testing.T) {
	adapter := coverSetup(t)
	_, err := adapter.runtime.RunString(`
		clearTimeout(99999); // Non-existent ID — should be silently ignored
	`)
	if err != nil {
		t.Fatalf("clearTimeout nonexistent failed: %v", err)
	}
}

func TestPhase2_SetImmediate(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var immVal = null;
		setImmediate(function() { immVal = "immediate"; });
	`)
	if err != nil {
		t.Fatalf("setImmediate setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
	val := adapter.runtime.Get("immVal")
	if val == nil || val.String() != "immediate" {
		t.Errorf("expected 'immediate', got '%v'", val)
	}
}

func TestPhase2_QueueMicrotask(t *testing.T) {
	adapter := coverSetupWithLoop(t)
	_, err := adapter.runtime.RunString(`
		var microVal = null;
		queueMicrotask(function() { microVal = "micro"; });
	`)
	if err != nil {
		t.Fatalf("queueMicrotask setup failed: %v", err)
	}
	coverRunLoopBriefly(t, adapter, 100)
	val := adapter.runtime.Get("microVal")
	if val == nil || val.String() != "micro" {
		t.Errorf("expected 'micro', got '%v'", val)
	}
}

func TestPhase2_New_NilLoop(t *testing.T) {
	defer assertAdapterPanic(t, "nil loop")
	_, _ = New(nil, nil)
}

func TestPhase2_New_NilRuntime(t *testing.T) {
	loop := goeventloop.New()
	defer loop.Shutdown(context.Background())
	defer assertAdapterPanic(t, "nil runtime")
	_, _ = New(loop, nil)
}
