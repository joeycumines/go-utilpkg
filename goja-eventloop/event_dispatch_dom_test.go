package gojaeventloop

import (
	"testing"

	goeventloop "github.com/joeycumines/go-eventloop"
)

// TestPinnedDOMEventDispatchPropagationFlagsAndPhases covers DOM commit
// 8a5f57c61ca1de8dc21b7e114501b1b57882e935. Node v26.5.0 intentionally
// differs at this boundary, so this is Web-profile rather than Node evidence.
func TestPinnedDOMEventDispatchPropagationFlagsAndPhases(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		const out = [];

		for (const stop of ["stopPropagation", "stopImmediatePropagation"]) {
			const target = new EventTarget();
			const event = new Event("x");
			target.addEventListener("x", () => out.push("pre:" + stop));
			event[stop]();
			target.dispatchEvent(event);
			out.push("cleared:" + stop + ":" + event.cancelBubble);
		}

		const stopped = new EventTarget();
		stopped.addEventListener("x", event => { out.push("capture:first"); event.stopPropagation(); }, true);
		stopped.addEventListener("x", () => out.push("capture:second"), true);
		stopped.addEventListener("x", () => out.push("bubble"));
		stopped.dispatchEvent(new Event("x"));

		const immediate = new EventTarget();
		immediate.addEventListener("x", event => { out.push("immediate:first"); event.stopImmediatePropagation(); }, true);
		immediate.addEventListener("x", () => out.push("immediate:second"), true);
		immediate.addEventListener("x", () => out.push("immediate:bubble"));
		immediate.dispatchEvent(new Event("x"));

		out.join(",");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	want := "cleared:stopPropagation:false,cleared:stopImmediatePropagation:false," +
		"capture:first,capture:second,immediate:first"
	if got := value.String(); got != want {
		t.Fatalf("dispatch propagation order = %q, want %q", got, want)
	}
}

func TestPinnedDOMGoDispatchPropagationPhases(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	_, err := adapter.runtime.RunString(`
		globalThis.__goDispatchTarget = new EventTarget();
		globalThis.__goDispatchOrder = [];
		__goDispatchTarget.addEventListener("go", event => {
			__goDispatchOrder.push("capture:first");
			event.stopPropagation();
		}, true);
		__goDispatchTarget.addEventListener("go", () => __goDispatchOrder.push("capture:second"), true);
		__goDispatchTarget.addEventListener("go", () => __goDispatchOrder.push("bubble"));
	`)
	if err != nil {
		t.Fatalf("RunString setup: %v", err)
	}
	wrapper := adapter.eventTargetThis(adapter.runtime.Get("__goDispatchTarget"))
	if !wrapper.target.DispatchEvent(goeventloop.NewEvent("go")) {
		t.Fatal("Go event dispatch was canceled")
	}
	value, err := adapter.runtime.RunString(`__goDispatchOrder.join(",")`)
	if err != nil {
		t.Fatalf("RunString result: %v", err)
	}
	if got, want := value.String(), "capture:first,capture:second"; got != want {
		t.Fatalf("Go dispatch propagation order = %q, want %q", got, want)
	}
}

func TestPinnedDOMListenerSnapshotPerInvoke(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		const target = new EventTarget();
		const order = [];
		target.addEventListener("x", () => {
			order.push("capture");
			target.addEventListener("x", () => order.push("late-capture"), true);
			target.addEventListener("x", () => order.push("late-bubble"));
		}, true);
		target.addEventListener("x", () => order.push("bubble"));
		target.dispatchEvent(new Event("x"));
		order.join(",");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := value.String(), "capture,bubble,late-bubble"; got != want {
		t.Fatalf("listener snapshot order = %q, want %q", got, want)
	}
}

func TestPinnedDOMEventTrustedGetterIdentity(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	_, err := adapter.runtime.RunString(`
		const first = new Event("first");
		const second = new Event("second");
		globalThis.__trustedGetter = Object.getOwnPropertyDescriptor(first, "isTrusted").get;
		globalThis.__trustedGetterMatches = __trustedGetter ===
			Object.getOwnPropertyDescriptor(second, "isTrusted").get;
		globalThis.__trustedTarget = new EventTarget();
		__trustedTarget.addEventListener("host", event => {
			__trustedGetterMatches = __trustedGetterMatches && __trustedGetter ===
				Object.getOwnPropertyDescriptor(event, "isTrusted").get;
		});
	`)
	if err != nil {
		t.Fatalf("RunString setup: %v", err)
	}
	target := adapter.eventTargetThis(adapter.runtime.Get("__trustedTarget"))
	if !target.target.DispatchEvent(goeventloop.NewEvent("host")) {
		t.Fatal("host event dispatch was canceled")
	}
	if !adapter.runtime.Get("__trustedGetterMatches").ToBoolean() {
		t.Fatal("Event isTrusted getter identity differs between wrapped events")
	}
}

func TestPinnedDOMEventDictionaryConversion(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		const failures = [];
		for (const Constructor of [Event, CustomEvent]) {
			for (const value of [false, true, 0, 1, "", "x", Symbol("x")]) {
				let threw = false;
				try { new Constructor("x", value); } catch (err) { threw = err instanceof TypeError; }
				if (!threw) failures.push(Constructor.name + ":" + typeof value);
			}
			for (const value of [undefined, null, {}]) {
				try { new Constructor("x", value); } catch (err) { failures.push(Constructor.name + ":accepted"); }
			}
		}
		const order = [];
		new CustomEvent("x", {
			get bubbles() { order.push("bubbles"); return true; },
			get cancelable() { order.push("cancelable"); return true; },
			get composed() { order.push("composed"); return true; },
			get detail() { order.push("detail"); return 1; },
		});
		if (order.join(",") !== "bubbles,cancelable,composed,detail") failures.push("member-order");
		failures.join(",");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got := value.String(); got != "" {
		t.Fatalf("dictionary conversion failures: %s", got)
	}
}

func TestPinnedDOMEventListenerRequiredArguments(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		const target = new EventTarget();
		const failures = [];
		for (const name of ["addEventListener", "removeEventListener"]) {
			for (const args of [[], ["x"]]) {
				let threw = false;
				try { target[name](...args); } catch (err) { threw = err instanceof TypeError; }
				if (!threw) failures.push(name + ":" + args.length);
			}
			for (const listener of [undefined, null]) {
				try { target[name]("x", listener); } catch (err) { failures.push(name + ":nullish"); }
			}
		}
		failures.join(",");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got := value.String(); got != "" {
		t.Fatalf("required-argument failures: %s", got)
	}
}
