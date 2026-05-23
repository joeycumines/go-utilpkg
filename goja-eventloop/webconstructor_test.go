package gojaeventloop

import (
	"testing"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func TestWebConstructorsRequireNewAndPreserveSubclassing(t *testing.T) {
	adapter := coverSetup(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			const cases = [
				[AbortController, [], 0, value => value.signal instanceof AbortSignal],
				[EventTarget, [], 0, value => typeof value.dispatchEvent === "function"],
				[Event, ["event-type"], 1, value => value.type === "event-type"],
				[CustomEvent, ["custom-type", { detail: 42 }], 1, value => value.type === "custom-type" && value.detail === 42],
				[DOMException, ["message", "AbortError"], 0, value => value.message === "message" && value.name === "AbortError" && value.code === 20],
			];
			for (const [Constructor, args, length, valid] of cases) {
				let directError;
				try { Reflect.apply(Constructor, undefined, args); }
				catch (error) { directError = error; }
				if (!(directError instanceof TypeError)) return Constructor.name + ":direct";

				const instance = Reflect.construct(Constructor, args);
				if (!(instance instanceof Constructor) || Object.getPrototypeOf(instance) !== Constructor.prototype || !valid(instance)) {
					return Constructor.name + ":base";
				}

				class Derived extends Constructor {}
				const derived = Reflect.construct(Derived, args);
				if (!(derived instanceof Derived) || !(derived instanceof Constructor) || Object.getPrototypeOf(derived) !== Derived.prototype || !valid(derived)) {
					return Constructor.name + ":derived";
				}

				if (Constructor.name.length === 0 || Constructor.length !== length) return Constructor.name + ":shape";
				const prototype = Object.getOwnPropertyDescriptor(Constructor, "prototype");
				if (!prototype || prototype.value !== Constructor.prototype || prototype.writable || prototype.enumerable || prototype.configurable) {
					return Constructor.name + ":prototype";
				}
				const back = Object.getOwnPropertyDescriptor(Constructor.prototype, "constructor");
				if (!back || back.value !== Constructor || !back.writable || back.enumerable || !back.configurable) {
					return Constructor.name + ":constructor";
				}
			}
			const exception = new DOMException("message", "AbortError");
			if (!(exception instanceof Error) || !(exception instanceof DOMException)) return "DOMException:error-chain";
			return "ok";
		})()
	`)
	if err != nil {
		t.Fatalf("Web constructor contract: %v", err)
	}
	if got := value.String(); got != "ok" {
		t.Fatalf("Web constructor contract = %q, want ok", got)
	}
}

func TestWebConstructorGlobalsRollbackAfterLateBindFailure(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = loop.Close() })
	runtime := goja.New()
	installConformingHostSingletons(t, runtime)

	names := []string{"AbortController", "EventTarget", "Event", "CustomEvent", "DOMException"}
	want := make(map[string]observedDescriptor, len(names))
	for index, name := range names {
		value := runtime.NewObject()
		enumerable := goja.FLAG_FALSE
		if index%2 == 0 {
			enumerable = goja.FLAG_TRUE
		}
		if err := runtime.GlobalObject().DefineDataProperty(name, value, goja.FLAG_FALSE, enumerable, goja.FLAG_TRUE); err != nil {
			t.Fatalf("define %s: %v", name, err)
		}
		want[name] = observeDescriptor(t, runtime, runtime.GlobalObject(), name)
	}
	promise := runtime.Get("Promise").ToObject(runtime)
	if err := promise.DefineDataProperty("try", runtime.ToValue("blocked"), goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_TRUE); err != nil {
		t.Fatal(err)
	}

	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err == nil {
		t.Fatal("Bind succeeded despite the late non-configurable Promise.try conflict")
	}
	for _, name := range names {
		assertDescriptor(t, runtime, runtime.GlobalObject(), name, want[name])
	}
}
