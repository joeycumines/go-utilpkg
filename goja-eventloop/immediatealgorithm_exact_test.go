package gojaeventloop

import "testing"

func TestNodeHandledImmediateThrowQueuesCycleNoop(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function() {});
		setImmediate(function() { throw new Error("handled"); });
		setImmediate(function() {
			const first = setImmediate(function() {});
			const second = setImmediate(function() {
				events.push(String(first._idlePrev !== null));
			});
		});
	`)
	if want := "true"; got != want {
		t.Fatalf("handled Immediate cycle predecessor = %q, want %q", got, want)
	}
}

func TestNodeHandledMalformedImmediateQueuesCycleNoop(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function(error) {
			events.push("u:" + error.message);
		});
		const malformed = setImmediate(function() {
			events.push("malformed-callback");
		});
		Object.defineProperty(malformed, "_argv", {
			configurable: true,
			get() {
				events.push("argv");
				throw new Error("argv");
			},
		});
		setImmediate(function peer() {
			events.push("peer");
			const first = setImmediate(function firstUser() {
				events.push("first");
			});
			setImmediate(function secondUser() {
				const previous = first._idlePrev;
				const refed = Object.getOwnPropertySymbols(previous).find(function(symbol) {
					return symbol.description === "refed";
				});
				events.push([
					previous !== null,
					previous !== first,
					previous._destroyed,
					previous._onImmediate,
					previous._idleNext === first,
					first._idlePrev === previous,
					previous[refed],
				].map(String).join(":"));
			});
		});
	`)
	if want := "argv,u:argv,peer,first,true:true:true:null:true:true:null"; got != want {
		t.Fatalf("handled malformed Immediate cycle linkage = %q, want %q", got, want)
	}
}

func TestNodeImmediateArgumentGetterThrowAdvancesTraversal(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function(error) { events.push("u:" + error.message); });
		const first = setImmediate(function() { events.push("first"); });
		Object.defineProperty(first, "_argv", {
			configurable: true,
			get() { throw new Error("argv"); },
		});
		setImmediate(function() { events.push("peer"); });
		setTimeout(function() { events.push("keep"); }, 10);
	`)
	if want := "u:argv,peer,keep"; got != want {
		t.Fatalf("Immediate argument getter throw = %q, want %q", got, want)
	}
}

func TestNodeImmediateSkippedTraversalReleasesNativeMirror(t *testing.T) {
	ctx, loop, runtime, adapter := newAutoExitAdapter(t)
	count := make(chan int, 1)
	if err := runtime.Set("captureImmediateCount", func() {
		adapter.immediatesMu.Lock()
		count <- len(adapter.immediates)
		adapter.immediatesMu.Unlock()
	}); err != nil {
		t.Fatalf("install Immediate mirror observer: %v", err)
	}
	if _, err := runtime.RunString(`
		const first = setImmediate(function() { first._idleNext = null; });
		setImmediate(function() {}).unref();
		setTimeout(captureImmediateCount, 10);
	`); err != nil {
		t.Fatalf("schedule Immediate traversal: %v", err)
	}
	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := <-count; got != 0 {
		t.Fatalf("consumed Immediate native mirrors = %d, want 0", got)
	}
}

func TestNodeDestroyedImmediateRefUsesGlobalLiveness(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		setImmediate(function() { events.push("pending"); }).unref();
		const canceled = setImmediate(function() { events.push("canceled"); });
		const refed = Object.getOwnPropertySymbols(canceled).find(function(symbol) {
			return symbol.description === "refed";
		});
		clearImmediate(canceled);
		canceled[refed] = false;
		canceled.ref();
		setTimeout(function() {
			events.push("release");
			canceled.unref();
		}, 10).unref();
	`)
	if want := "pending,release"; got != want {
		t.Fatalf("destroyed Immediate liveness = %q, want %q", got, want)
	}
}
