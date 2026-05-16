package gojaeventloop

import (
	"context"
	"testing"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// coverSetup creates a minimal adapter for coverage tests.
// Unlike testSetup, this does NOT start the loop (for sync-only tests).
func coverSetup(t *testing.T) *Adapter {
	t.Helper()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { loop.Shutdown(context.Background()) })

	rt := goja.New()
	adapter, err := New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("failed to bind adapter: %v", err)
	}
	return adapter
}

// ===========================================================================
// fetch boundary — intentionally omitted
// ===========================================================================

func TestFetchIsOmitted(t *testing.T) {
	adapter := coverSetup(t)
	value, err := adapter.runtime.RunString(`typeof fetch`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if got, want := value.String(), "undefined"; got != want {
		t.Fatalf("typeof fetch = %q, want %q", got, want)
	}
}

func TestBindPreservesHostFetch(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = loop.Close() }()
	runtime := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Set("fetch", func(goja.FunctionCall) goja.Value { return runtime.ToValue("host-fetch") }); err != nil {
		t.Fatalf("set host fetch: %v", err)
	}
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	value, err := runtime.RunString(`fetch()`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if got, want := value.String(), "host-fetch"; got != want {
		t.Fatalf("host fetch result = %q, want %q", got, want)
	}
}

func TestBindPreservesNativeSymbolRegistry(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loop.Close() })
	runtime := goja.New()
	if err != nil {
		t.Fatal(err)
	}
	symbol := runtime.Get("Symbol")
	symbolObject := symbol.ToObject(runtime)
	registryLookup := symbolObject.Get("for")
	registryKey := symbolObject.Get("keyFor")

	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind adapter: %v", err)
	}
	if !runtime.Get("Symbol").SameAs(symbol) || !runtime.Get("Symbol").ToObject(runtime).Get("for").SameAs(registryLookup) ||
		!runtime.Get("Symbol").ToObject(runtime).Get("keyFor").SameAs(registryKey) {
		t.Fatal("Bind replaced native Symbol registry intrinsics")
	}

	result, err := runtime.RunString(`
		if (typeof Symbol !== "function" || typeof Symbol.for !== "function" || typeof Symbol.keyFor !== "function") {
			throw new Error("native Symbol registry is unavailable");
		}
		const ordinary = Symbol.for("test");
		const special = Symbol.for("key with spaces and emojis 🎉");
		const empty = Symbol.for("");
		ordinary === Symbol.for("test") &&
		ordinary !== special &&
		empty === Symbol.for("") &&
		Symbol.keyFor(ordinary) === "test" &&
		Symbol.keyFor(special) === "key with spaces and emojis 🎉" &&
		Symbol.keyFor(empty) === "" &&
		Symbol.keyFor(Symbol("local")) === undefined;
	`)
	if err != nil {
		t.Fatalf("RunString failed: %v", err)
	}
	if !result.ToBoolean() {
		t.Error("native Symbol registry behavior changed across Bind")
	}
}
