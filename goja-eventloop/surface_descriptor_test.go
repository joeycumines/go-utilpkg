package gojaeventloop

import "testing"

func TestRetainedNodeSurfaceDescriptors(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		const failures = [];
		const check = (condition, label) => { if (!condition) failures.push(label); };

		const consoleDescriptor = Object.getOwnPropertyDescriptor(globalThis, "console");
		check(consoleDescriptor.writable && consoleDescriptor.enumerable && consoleDescriptor.configurable,
			"global console descriptor");
		for (const name of [
			"time", "timeEnd", "timeLog", "count", "countReset", "assert", "table",
			"group", "groupCollapsed", "groupEnd", "trace", "clear", "dir",
		]) {
			const descriptor = Object.getOwnPropertyDescriptor(console, name);
			check(descriptor && descriptor.writable && descriptor.enumerable && descriptor.configurable,
				"console." + name + " descriptor");
			check(descriptor && descriptor.value.name === name && descriptor.value.length === 0,
				"console." + name + " function shape");
		}

		for (const [holder, name] of [
			[AbortSignal.prototype, "reason"],
			[AbortSignal.prototype, "throwIfAborted"],
			[AbortSignal, "abort"],
			[AbortSignal, "any"],
			[AbortSignal, "timeout"],
		]) {
			const descriptor = Object.getOwnPropertyDescriptor(holder, name);
			check(descriptor && descriptor.enumerable && descriptor.configurable,
				"AbortSignal " + name + " descriptor");
		}

		const dispose = Object.getOwnPropertyDescriptor(Symbol, "dispose");
		check(dispose && typeof dispose.value === "symbol" &&
			!dispose.writable && !dispose.enumerable && !dispose.configurable,
			"Symbol.dispose descriptor");
		failures.join(",");
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got := value.String(); got != "" {
		t.Fatalf("retained descriptor failures: %s", got)
	}
}
