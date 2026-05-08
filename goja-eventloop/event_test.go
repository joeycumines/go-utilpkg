package gojaeventloop

import (
	"testing"

	"github.com/joeycumines/goja"
)

// ============================================================================
// Event JS Binding Tests
// ============================================================================

func TestEvent_Constructor(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		const event = new Event('click');
		if (event.type !== 'click') {
			throw new Error('type should be click');
		}
		if (event.bubbles !== false) {
			throw new Error('bubbles should be false by default');
		}
		if (event.cancelable !== false) {
			throw new Error('cancelable should be false by default');
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestEvent_ConstructorWithOptions(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		const event = new Event('submit', { bubbles: true, cancelable: true });
		if (event.type !== 'submit') {
			throw new Error('type should be submit');
		}
		if (event.bubbles !== true) {
			throw new Error('bubbles should be true');
		}
		if (event.cancelable !== true) {
			throw new Error('cancelable should be true');
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestEvent_PreventDefault(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		const event = new Event('submit', { cancelable: true });
		if (event.defaultPrevented !== false) {
			throw new Error('defaultPrevented should be false initially');
		}
		event.preventDefault();
		if (event.defaultPrevented !== true) {
			throw new Error('defaultPrevented should be true after preventDefault');
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestEventTarget_PassiveListenerCannotCancel(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	bindRetainedEventTestSurface(t, adapter)

	result, err := runtime.RunString(`
		const target = new EventTarget();
		const event = new Event("cancel", { cancelable: true });
		target.addEventListener("cancel", (value) => value.preventDefault(), { passive: true });
		[target.dispatchEvent(event), event.defaultPrevented].join(":");
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.String(), "true:false"; got != want {
		t.Fatalf("passive dispatch result = %q, want %q", got, want)
	}
}

func TestEventPinnedInterfaceCensusAndDescriptors(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	bindRetainedEventTestSurface(t, adapter)

	_, err = runtime.RunString(`
		for (const [constructor, name, length] of [
			[EventTarget, "EventTarget", 0], [Event, "Event", 1], [CustomEvent, "CustomEvent", 1],
		]) {
			if (constructor.name !== name || constructor.length !== length) throw new Error(name + " constructor metadata");
		}
		if (Object.getPrototypeOf(CustomEvent.prototype) !== Event.prototype) throw new Error("CustomEvent inheritance");
		for (const [prototype, tag] of [
			[EventTarget.prototype, "EventTarget"], [Event.prototype, "Event"], [CustomEvent.prototype, "CustomEvent"],
		]) {
			const descriptor = Object.getOwnPropertyDescriptor(prototype, Symbol.toStringTag);
			if (!descriptor || descriptor.value !== tag || descriptor.writable || descriptor.enumerable || !descriptor.configurable) {
				throw new Error(tag + " toStringTag descriptor");
			}
		}

		const accessors = ["type", "target", "srcElement", "currentTarget", "eventPhase", "timeStamp",
			"bubbles", "cancelable", "defaultPrevented", "composed"];
		for (const name of accessors) {
			const descriptor = Object.getOwnPropertyDescriptor(Event.prototype, name);
			if (!descriptor || descriptor.get.name !== "get " + name || descriptor.get.length !== 0 ||
				descriptor.set !== undefined || !descriptor.enumerable || !descriptor.configurable) {
				throw new Error(name + " accessor descriptor");
			}
		}
		for (const name of ["cancelBubble", "returnValue"]) {
			const descriptor = Object.getOwnPropertyDescriptor(Event.prototype, name);
			if (!descriptor || descriptor.get.name !== "get " + name || descriptor.get.length !== 0 ||
				descriptor.set.name !== "set " + name || descriptor.set.length !== 1 ||
				!descriptor.enumerable || !descriptor.configurable) {
				throw new Error(name + " accessor descriptor");
			}
		}
		for (const [name, length] of [["composedPath", 0], ["preventDefault", 0], ["stopPropagation", 0],
			["stopImmediatePropagation", 0], ["initEvent", 1]]) {
			const descriptor = Object.getOwnPropertyDescriptor(Event.prototype, name);
			if (!descriptor || descriptor.value.name !== name || descriptor.value.length !== length ||
				!descriptor.writable || !descriptor.enumerable || !descriptor.configurable) {
				throw new Error(name + " method descriptor");
			}
		}
		const detail = Object.getOwnPropertyDescriptor(CustomEvent.prototype, "detail");
		const initCustom = Object.getOwnPropertyDescriptor(CustomEvent.prototype, "initCustomEvent");
		if (!detail || detail.get.name !== "get detail" || detail.get.length !== 0 || detail.set !== undefined || !detail.enumerable || !detail.configurable) {
			throw new Error("detail descriptor");
		}
		if (!initCustom || initCustom.value.name !== "initCustomEvent" || initCustom.value.length !== 1 ||
			!initCustom.writable || !initCustom.enumerable || !initCustom.configurable) {
			throw new Error("initCustomEvent descriptor");
		}

		for (const [name, value] of [["NONE", 0], ["CAPTURING_PHASE", 1], ["AT_TARGET", 2], ["BUBBLING_PHASE", 3]]) {
			for (const holder of [Event, Event.prototype]) {
				const descriptor = Object.getOwnPropertyDescriptor(holder, name);
				if (!descriptor || descriptor.value !== value || descriptor.writable || !descriptor.enumerable || descriptor.configurable) {
					throw new Error(name + " constant descriptor");
				}
			}
		}

		const event = new Event("shape", { bubbles: true, cancelable: true, composed: true });
		const trusted = Object.getOwnPropertyDescriptor(event, "isTrusted");
		if (!trusted || trusted.get.name !== "get isTrusted" || trusted.get.length !== 0 || trusted.set !== undefined ||
			trusted.enumerable !== true || trusted.configurable !== false || event.isTrusted !== false) {
			throw new Error("isTrusted LegacyUnforgeable descriptor");
		}
		if (event.target !== null || event.srcElement !== null || event.currentTarget !== null || event.eventPhase !== Event.NONE ||
			event.composedPath().length !== 0 || !event.returnValue || event.cancelBubble || !event.composed) {
			throw new Error("initial event state");
		}
	`)
	if err != nil {
		t.Fatalf("Event pinned census: %v", err)
	}
}

func TestEventDispatchStateLegacyMembersInitializationAndCoercion(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	bindRetainedEventTestSurface(t, adapter)

	_, err = runtime.RunString(`
		const target = new EventTarget();
		const event = new Event("first", { cancelable: true, composed: true });
		const observations = [];
		let dispatch = 0;
		target.addEventListener("first", function(value) {
			dispatch++;
			observations.push(value.target === target, value.srcElement === target, value.currentTarget === target,
				value.eventPhase === Event.AT_TARGET, value.composedPath().length === 1 && value.composedPath()[0] === target);
			value.returnValue = false;
			if (dispatch === 1) value.stopImmediatePropagation();
		});
		target.addEventListener("first", function() { observations.push("second"); });
		if (target.dispatchEvent(event) !== false) throw new Error("first dispatch return");
		if (!event.defaultPrevented || event.returnValue || event.currentTarget !== null || event.eventPhase !== Event.NONE ||
			event.composedPath().length !== 0 || event.cancelBubble) throw new Error("post-dispatch state");
		if (target.dispatchEvent(event) !== false || observations.filter(value => value === "second").length !== 1) {
			throw new Error("redispatch stop flags/default cancellation");
		}
		if (!observations.slice(0, 5).every(Boolean)) throw new Error("during-dispatch state");

		event.initEvent("reset", false, false);
		if (event.type !== "reset" || event.bubbles || event.cancelable || event.defaultPrevented || !event.returnValue ||
			event.target !== null || event.srcElement !== null || event.composed !== true) throw new Error("initEvent state");
		const custom = new CustomEvent("custom", { detail: 1 });
		custom.initCustomEvent("updated", true, true, 9);
		if (custom.type !== "updated" || !custom.bubbles || !custom.cancelable || custom.detail !== 9) throw new Error("initCustomEvent state");

		const order = [];
		const type = { toString() { order.push("type"); return "ordered"; } };
		const init = {
			get bubbles() { order.push("bubbles"); return 1; },
			get cancelable() { order.push("cancelable"); return 1; },
			get composed() { order.push("composed"); return 1; },
			get detail() { order.push("detail"); return 1; },
		};
		new CustomEvent(type, init);
		if (order.join(",") !== "type,bubbles,cancelable,composed,detail") throw new Error("Web IDL conversion order: " + order);
		for (const run of [() => new Event(Symbol("x")), () => target.addEventListener(Symbol("x"), null)]) {
			let threw = false;
			try { run(); } catch (error) { threw = error instanceof TypeError; }
			if (!threw) throw new Error("DOMString accepted Symbol");
		}
	`)
	if err != nil {
		t.Fatalf("Event dispatch/legacy/init/coercion contract: %v", err)
	}
}

func TestEvent_StopPropagation(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		const event = new Event('click');
		event.stopPropagation();
		// Just verify it doesn't throw
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestEvent_StopImmediatePropagation(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`
		const target = new EventTarget();
		const order = [];

		target.addEventListener('test', function(e) {
			order.push(1);
			e.stopImmediatePropagation();
		});
		target.addEventListener('test', function(e) {
			order.push(2);
		});

		target.dispatchEvent(new Event('test'));

		order.length;
	`)
	if err != nil {
		t.Fatal(err)
	}

	if result.ToInteger() != 1 {
		t.Errorf("Only first listener should be called, got %d listeners called", result.ToInteger())
	}
}

func TestEvent_DispatchEvent_ReturnValue(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.RunString(`
		const target = new EventTarget();

		target.addEventListener('submit', function(e) {
			e.preventDefault();
		});

		const event = new Event('submit', { cancelable: true });
		const result = target.dispatchEvent(event);

		result;
	`)
	if err != nil {
		t.Fatal(err)
	}

	if result.ToBoolean() {
		t.Error("dispatchEvent should return false when event is canceled")
	}
}

func TestEvent_NoTypeArgument(t *testing.T) {
	loop, cleanup := testEventLoopSetup(t)
	defer cleanup()

	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	_, err = runtime.RunString(`
		try {
			new Event();
			throw new Error('Should have thrown');
		} catch (e) {
			if (!e.message.includes('requires a type')) {
				throw new Error('Wrong error: ' + e.message);
			}
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}
