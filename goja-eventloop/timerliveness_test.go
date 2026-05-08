package gojaeventloop

import (
	"context"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

func TestNodeUnrefTimeoutOverdueBeforeRunDoesNotFire(t *testing.T) {
	ctx, loop, runtime, _ := newAutoExitAdapter(t)
	done := make(chan bool, 1)
	if err := runtime.Set("testDone", func(value bool) { done <- value }); err != nil {
		t.Fatalf("set testDone: %v", err)
	}
	if _, err := runtime.RunString(`
		let ran = false;
		setTimeout(function() { ran = true; }, 0).unref();
		process.on("exit", function() { testDone(ran); });
	`); err != nil {
		t.Fatalf("RunString: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	select {
	case ran := <-done:
		if ran {
			t.Fatal("unref timeout fired without refed work")
		}
	default:
		t.Fatal("process exit did not report timeout result")
	}
}

func TestNodeUnrefImmediateAloneDoesNotKeepLoopAlive(t *testing.T) {
	ctx, loop, runtime, _ := newAutoExitAdapter(t)
	done := make(chan bool, 1)
	if err := runtime.Set("testDone", func(value bool) { done <- value }); err != nil {
		t.Fatalf("set testDone: %v", err)
	}
	if _, err := runtime.RunString(`
		let ran = false;
		setImmediate(function() { ran = true; }).unref();
		process.on("exit", function() { testDone(ran); });
	`); err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ran := <-done; ran {
		t.Fatal("unref immediate fired without refed work")
	}
}

func TestNodeUnrefImmediateRunsBeforeRefedImmediate(t *testing.T) {
	ctx, loop, runtime, _ := newAutoExitAdapter(t)
	done := make(chan string, 1)
	if err := runtime.Set("testDone", func(value string) { done <- value }); err != nil {
		t.Fatalf("set testDone: %v", err)
	}
	if _, err := runtime.RunString(`
		const events = [];
		setImmediate(function() { events.push("unref"); }).unref();
		setImmediate(function() {
			events.push("refed");
			testDone(events.join(","));
		});
	`); err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := <-done; got != "unref,refed" {
		t.Fatalf("immediate order = %q, want %q", got, "unref,refed")
	}
}

func TestNodeImmediateHandledDirectThrowYieldsRemainingMicrotasks(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function() { events.push("u"); });
		setImmediate(function() {
			events.push("i1");
			process.nextTick(function() { events.push("n1"); });
			process.nextTick(function() { events.push("n2"); });
			Promise.resolve().then(function() { events.push("p"); });
			throw new Error("direct");
		});
		setImmediate(function() { events.push("i2"); });
	`)
	if want := "i1,u,i2,n1,n2,p"; got != want {
		t.Fatalf("handled direct immediate throw order = %q, want %q", got, want)
	}
}

func TestNodeImmediateHandledNextTickThrowYieldsRemainingMicrotasks(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function() { events.push("u"); });
		setImmediate(function() {
			events.push("i1");
			process.nextTick(function() {
				events.push("n1");
				throw new Error("nextTick");
			});
			process.nextTick(function() { events.push("n2"); });
			Promise.resolve().then(function() { events.push("p"); });
		});
		setImmediate(function() { events.push("i2"); });
	`)
	if want := "i1,n1,u,i2,n2,p"; got != want {
		t.Fatalf("handled nextTick immediate throw order = %q, want %q", got, want)
	}
}

func TestNodeIntervalUnrefUsesSharedCarrier(t *testing.T) {
	ctx, loop, runtime, adapter := newAutoExitAdapter(t)
	done := make(chan bool, 1)
	if err := runtime.Set("testDone", func(value bool) { done <- value }); err != nil {
		t.Fatalf("set testDone: %v", err)
	}
	if _, err := runtime.RunString(`
		let ran = false;
		globalThis.intervalHandle = setInterval(function() { ran = true; }, 60000);
		intervalHandle.unref();
		process.on("exit", function() { testDone(ran); });
	`); err != nil {
		t.Fatalf("RunString: %v", err)
	}
	state, ok := adapter.timerStateObject(runtime.Get("intervalHandle").ToObject(runtime))
	if !ok || !state.active.Load() || state.refed.Load() {
		t.Fatal("unref interval native mirror is not active and unrefed")
	}
	adapter.timersMu.Lock()
	wake := adapter.timerBackendWake
	adapter.timersMu.Unlock()
	if wake == nil || wake.id == 0 || wake.refed {
		t.Fatalf("shared interval carrier = %#v, want published and unrefed", wake)
	}
	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ran := <-done; ran {
		t.Fatal("unref interval fired before auto-exit")
	}
}

func TestNodeLatentTimeoutClaimControlsActiveUnrefWork(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		setTimeout(function bootstrap() {
			const latent = new this.constructor(function() {}, 20, undefined, false, true);
			setTimeout(function() {
				events.push("timer");
				latent.unref();
			}, 20).unref();
		}, 0);
	`)
	if got != "timer" {
		t.Fatalf("latent Timeout liveness = %q, want %q", got, "timer")
	}
}

func TestNodeLatentRefedTimeoutDoesNotSynthesizeCarrier(t *testing.T) {
	ctx, loop, runtime, adapter := newAutoExitAdapter(t)
	if _, err := runtime.RunString(`
		globalThis.latentRuns = 0;
		globalThis.seed = setTimeout(function() {}, 5);
		globalThis.Timeout = seed.constructor;
		clearTimeout(seed);
	`); err != nil {
		t.Fatalf("create and clear seed Timeout: %v", err)
	}
	adapter.timersMu.Lock()
	staleWake := adapter.timerBackendWake
	adapter.timersMu.Unlock()
	if staleWake == nil || staleWake.id == 0 || staleWake.refed {
		t.Fatalf("cleared seed carrier = %#v, want one published unrefed stale carrier", staleWake)
	}
	if _, err := runtime.RunString(`
		globalThis.latent = new Timeout(function() { latentRuns++; }, 1000, undefined, false, true);
	`); err != nil {
		t.Fatalf("create latent refed Timeout: %v", err)
	}
	adapter.timersMu.Lock()
	latentWake := adapter.timerBackendWake
	adapter.timersMu.Unlock()
	if latentWake != staleWake || !latentWake.refed {
		t.Fatalf("latent ref transition carrier = %#v, want refed stale carrier %#v without a successor", latentWake, staleWake)
	}
	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := runtime.Get("latentRuns").ToInteger(); got != 0 {
		t.Fatalf("latent callback runs = %d, want 0", got)
	}
}

func TestNodeLatentTimeoutUnrefRefreshAndClearTransitions(t *testing.T) {
	for _, test := range []struct {
		name      string
		operation string
		wantRuns  int64
	}{
		{name: "unref", operation: "latent.unref()", wantRuns: 0},
		{name: "clear", operation: "clearTimeout(latent)", wantRuns: 0},
		{name: "refresh", operation: "latent.refresh()", wantRuns: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, loop, runtime, _ := newAutoExitAdapter(t)
			var runs int64
			if err := runtime.Set("recordLatent", func() { runs++ }); err != nil {
				t.Fatalf("set recordLatent: %v", err)
			}
			if _, err := runtime.RunString(`
				const seed = setTimeout(function() {}, 1);
				const Timeout = seed.constructor;
				clearTimeout(seed);
				const latent = new Timeout(recordLatent, 5, undefined, false, true);
				` + test.operation + `;
			`); err != nil {
				t.Fatalf("apply latent transition: %v", err)
			}
			if err := loop.Run(ctx); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if runs != test.wantRuns {
				t.Fatalf("latent callback runs = %d, want %d", runs, test.wantRuns)
			}
		})
	}
}

func TestNodeLatentRefDoesNotRearmAfterLastTimerListDrain(t *testing.T) {
	ctx, loop, runtime, adapter := newAutoExitAdapter(t)
	fired := make(chan struct{}, 1)
	scheduledFired := false
	if err := runtime.Set("recordScheduled", func() {
		scheduledFired = true
		fired <- struct{}{}
	}); err != nil {
		t.Fatalf("set recordScheduled: %v", err)
	}
	if _, err := runtime.RunString(`
		const seed = setTimeout(function() {}, 1);
		const Timeout = seed.constructor;
		clearTimeout(seed);
		globalThis.latentAfterDrain = new Timeout(function() {}, 1000, undefined, false, true);
		setTimeout(recordScheduled, 5);
	`); err != nil {
		t.Fatalf("create latent and scheduled timers: %v", err)
	}
	adapter.timersMu.Lock()
	activeWake := adapter.timerBackendWake
	adapter.timersMu.Unlock()
	if activeWake == nil || activeWake.id == 0 || !activeWake.refed {
		t.Fatalf("active timer carrier = %#v, want one published refed carrier", activeWake)
	}
	successorReservations := 0
	adapter.timerBackendHooks = &timerBackendTestHooks{
		afterSuccessorReservation: func() {
			if scheduledFired {
				successorReservations++
			}
		},
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(time.Second):
		if err := loop.Close(); err != nil {
			t.Errorf("Close after latent rearm timeout: %v", err)
		}
		<-runDone
		t.Fatal("latent ref rearmed a carrier after the last TimersList drained")
	}
	select {
	case <-fired:
	default:
		t.Fatal("scheduled timer did not fire before auto-exit")
	}
	adapter.timerBackendHooks = nil
	if successorReservations != 0 {
		t.Fatalf("post-drain carrier reservations = %d, want 0", successorReservations)
	}
}

func TestNodeLatentRefStateSurvivesEmptyDrainForLaterTimerList(t *testing.T) {
	ctx, loop, runtime, _ := newAutoExitAdapter(t)
	var runs int
	if err := runtime.Set("recordLaterTimer", func() { runs++ }); err != nil {
		t.Fatalf("set recordLaterTimer: %v", err)
	}
	if _, err := runtime.RunString(`
		const seed = setTimeout(function() {}, 5);
		const Timeout = seed.constructor;
		clearTimeout(seed);
		globalThis.latentAcrossDrain = new Timeout(function() {}, 1000, undefined, false, true);
	`); err != nil {
		t.Fatalf("create latent Timeout before empty drain: %v", err)
	}
	var callbackErr error
	if _, err := loop.ScheduleTimer(20*time.Millisecond, func() {
		_, callbackErr = runtime.RunString(`
			setTimeout(function() {
				recordLaterTimer();
				latentAcrossDrain.unref();
			}, 5).unref();
		`)
	}); err != nil {
		t.Fatalf("schedule non-timer keepalive: %v", err)
	}
	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if callbackErr != nil {
		t.Fatalf("schedule later timer list: %v", callbackErr)
	}
	if runs != 1 {
		t.Fatalf("later unref timer runs = %d, want 1 under preserved latent ref state", runs)
	}
}

func TestWebAbortSignalTimeoutZeroUsesTimerBackend(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const signal = AbortSignal.timeout(0);
		signal.addEventListener("abort", function() { events.push(signal.reason.name); });
		setTimeout(function() { events.push("keepalive"); }, 20);
	`)
	if got != "TimeoutError,keepalive" {
		t.Fatalf("AbortSignal.timeout lifecycle = %q, want %q", got, "TimeoutError,keepalive")
	}
}

func TestNodeDirectAndGenericTimeoutRefreshActivateExactlyOnce(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const seed = setTimeout(function() {}, 1000);
		const Timeout = seed.constructor;
		clearTimeout(seed);
		const direct = new Timeout(function() { events.push("direct"); }, 5, undefined, false, true);
		direct.refresh();
		const generic = {
			_idleTimeout: 5,
			_destroyed: true,
			_onTimeout() { events.push("generic"); },
			_timerArgs: undefined,
			_repeat: null,
			_idleNext: null,
			_idlePrev: null,
		};
		Timeout.prototype.refresh.call(generic);
	`)
	if got != "direct,generic" {
		t.Fatalf("refresh activation = %q, want %q", got, "direct,generic")
	}
}

func TestNodeDestroyedTimeoutRefreshReinitializesObservableIdentity(t *testing.T) {
	ctx, loop, runtime, adapter := newAutoExitAdapter(t)
	if _, err := runtime.RunString(`
		globalThis.events = [];
		globalThis.numericRuns = 0;
		globalThis.destroyedRefreshHandle = setTimeout(function() { numericRuns++; }, 1);
		const numericOld = +destroyedRefreshHandle;
		(async function() {
			const wait = (msecs) => new Promise((resolve) => setTimeout(resolve, msecs));
			await wait(10);
			destroyedRefreshHandle.refresh();
			const numericNew = +destroyedRefreshHandle;
			clearTimeout(numericNew);
			await wait(10);

			let objectRuns = 0;
			const objectHandle = setTimeout(function() { objectRuns++; }, 1);
			const objectOld = +objectHandle;
			await wait(10);
			objectHandle.refresh();
			const objectNew = +objectHandle;
			clearTimeout(objectHandle);
			await wait(10);

			events.push(numericOld !== numericNew, numericRuns, objectOld !== objectNew, objectRuns);
		})();
	`); err != nil {
		t.Fatalf("schedule destroyed refresh probes: %v", err)
	}
	before, ok := adapter.timerStateObject(runtime.Get("destroyedRefreshHandle").ToObject(runtime))
	if !ok || before == nil {
		t.Fatal("destroyed refresh handle has no private timer state")
	}
	privateID := before.id
	if err := loop.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	after, ok := adapter.timerStateObject(runtime.Get("destroyedRefreshHandle").ToObject(runtime))
	if !ok || after != before || after.id != privateID {
		var afterID uint64
		if after != nil {
			afterID = after.id
		}
		t.Fatalf("private timer identity changed across refresh: before=%p/%d after=%p/%d", before, privateID, after, afterID)
	}
	value, err := runtime.RunString(`events.join(",")`)
	if err != nil {
		t.Fatalf("read destroyed refresh results: %v", err)
	}
	if got, want := value.String(), "true,2,true,1"; got != want {
		t.Fatalf("destroyed refresh identity = %q, want %q", got, want)
	}
}

func TestNodeTimerRetirementUsesPreCallbackAsyncID(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		let oldId;
		const handle = setTimeout(function() {
			const asyncId = Object.getOwnPropertySymbols(this).find(function(symbol) {
				return symbol.description === "asyncId";
			});
			this[asyncId] = 9007199254740990;
			setImmediate(function() {
				clearTimeout(oldId);
				events.push(typeof handle._onTimeout, handle._destroyed);
			});
		}, 1);
		oldId = +handle;
	`)
	if want := "function,true"; got != want {
		t.Fatalf("post-callback async ID retirement = %q, want %q", got, want)
	}
}

func TestNodeTimerRetirementDoesNotRereadCachedAsyncID(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		setTimeout(function() {
			events.push("timer");
			const asyncId = Object.getOwnPropertySymbols(this).find(function(symbol) {
				return symbol.description === "asyncId";
			});
			Object.defineProperty(this, asyncId, {
				configurable: true,
				get() {
					events.push("get");
					throw new Error("async ID reread");
				},
			});
		}, 1);
		setTimeout(function() { events.push("keep"); }, 10);
	`)
	if want := "timer,keep"; got != want {
		t.Fatalf("cached async ID retirement = %q, want %q", got, want)
	}
}

func TestNodeSkippedTimerRunsCheckpointBeforeDuePeer(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const skipped = setTimeout(function() {}, 1);
		Object.defineProperty(skipped, "_onTimeout", {
			configurable: true,
			get() {
				events.push("get");
				process.nextTick(function() { events.push("n"); });
				Promise.resolve().then(function() { events.push("p"); });
				return null;
			},
		});
		setTimeout(function() { events.push("peer"); }, 1);
	`)
	if want := "get,n,p,peer"; got != want {
		t.Fatalf("skipped timer checkpoint = %q, want %q", got, want)
	}
}

func TestNodeTimerAlgorithmsIgnoreMutableCollectionPrototypes(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const mapGet = Map.prototype.get;
		const mapSet = Map.prototype.set;
		const mapDelete = Map.prototype.delete;
		const arrayPush = Array.prototype.push;
		const arrayPop = Array.prototype.pop;
		Map.prototype.get = null;
		Map.prototype.set = null;
		Map.prototype.delete = null;
		Array.prototype.push = null;
		Array.prototype.pop = null;
		setTimeout(function() {
			Map.prototype.get = mapGet;
			Map.prototype.set = mapSet;
			Map.prototype.delete = mapDelete;
			Array.prototype.push = arrayPush;
			Array.prototype.pop = arrayPop;
			events.push("timer");
		}, 1);
	`)
	if want := "timer"; got != want {
		t.Fatalf("timer result under mutable collection prototypes = %q, want %q", got, want)
	}
}

func TestNodeTimerQueueWritesOnlyFinalReplacementPosition(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const root = setTimeout(function() {}, 50);
		const middle = setTimeout(function() {}, 60);
		const bottom = setTimeout(function() {}, 70);
		const list = bottom._idlePrev;
		let position = list.priorityQueuePosition;
		const writes = [];
		Object.defineProperty(list, "priorityQueuePosition", {
			configurable: true,
			get() { return position; },
			set(value) { writes.push(value); position = value; },
		});
		clearTimeout(root);
		events.push(writes.join(":"), String(position));
		clearTimeout(middle);
		clearTimeout(bottom);
	`)
	if want := "2,2"; got != want {
		t.Fatalf("priority queue replacement writes = %q, want %q", got, want)
	}
}

func TestNodeNaturalTimerListDrainIgnoresMutablePosition(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const first = setTimeout(function() { events.push("first"); }, 1);
		first._idlePrev.priorityQueuePosition = 999;
		setTimeout(function() { events.push("second"); }, 10);
	`)
	if want := "first,second"; got != want {
		t.Fatalf("timer list drain with mutable position = %q, want %q", got, want)
	}
}

func TestNodeTimerAndImmediateSubclassesPreserveConstructedReceiver(t *testing.T) {
	_, _, runtime, _ := newAutoExitAdapter(t)
	value, err := runtime.RunString(`
		(function() {
			const timeoutSeed = setTimeout(function() {}, 1000);
			const Timeout = timeoutSeed.constructor;
			clearTimeout(timeoutSeed);
			const immediateSeed = setImmediate(function() {});
			const Immediate = immediateSeed.constructor;
			clearImmediate(immediateSeed);

			class TimeoutSubclass extends Timeout {}
			class ImmediateSubclass extends Immediate {}
			const timeout = new TimeoutSubclass(function() {}, 1000, undefined, false, true);
			const immediate = new ImmediateSubclass(function() {});
			const result = [
				timeout instanceof TimeoutSubclass,
				Object.getPrototypeOf(timeout) === TimeoutSubclass.prototype,
				timeout.constructor === TimeoutSubclass,
				immediate instanceof ImmediateSubclass,
				Object.getPrototypeOf(immediate) === ImmediateSubclass.prototype,
				immediate.constructor === ImmediateSubclass,
			];
			clearTimeout(timeout);
			clearImmediate(immediate);
			return result.join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("construct timer subclasses: %v", err)
	}
	if got, want := value.String(), "true,true,true,true,true,true"; got != want {
		t.Fatalf("timer subclass construction = %q, want %q", got, want)
	}
}

func TestNodeImmediateBatchPreservesExecutedLinks(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		let first;
		let second;
		function links() {
			return [
				first._idlePrev === null,
				first._idleNext === second,
				second._idlePrev === first,
				second._idleNext === null,
			].join(":");
		}
		first = setImmediate(function() { events.push("first-" + links()); });
		second = setImmediate(function() { events.push("second-" + links()); });
	`)
	if want := "first-true:true:true:true,second-true:true:true:true"; got != want {
		t.Fatalf("executed Immediate links = %q, want %q", got, want)
	}
}

func TestNodeTimerCallbackIgnoresOwnCallProperty(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		function callback() { events.push("timer"); }
		callback.call = null;
		setTimeout(callback, 0);
	`)
	if want := "timer"; got != want {
		t.Fatalf("timer callback with own call property = %q, want %q", got, want)
	}
}

func TestNodeHandledTimerThrowRetainsListGeneration(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		let sentinel;
		process.on("uncaughtException", function() {
			const replacement = setTimeout(function() { events.push("replacement"); }, 1);
			events.push(replacement._idlePrev === sentinel ? "same" : "different");
		});
		const throwing = setTimeout(function() { throw new Error("boom"); }, 1);
		sentinel = throwing._idlePrev;
		setTimeout(function() { events.push("keep"); }, 10);
	`)
	if want := "same,replacement,keep"; got != want {
		t.Fatalf("handled timer throw generation = %q, want %q", got, want)
	}
}

func TestNodeHandledTimerThrowRetriesWithFixedNow(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function() {
			events.push("u");
			setImmediate(function() { events.push("i"); });
		});
		setTimeout(function() {
			events.push("t1");
			const until = performance.now() + 35;
			while (performance.now() < until) {}
			throw new Error("slow");
		}, 5);
		setTimeout(function() { events.push("t2"); }, 20);
		setTimeout(function() { events.push("keep"); }, 80);
	`)
	if want := "t1,u,i,t2,keep"; got != want {
		t.Fatalf("handled timer retry clock order = %q, want %q", got, want)
	}
}

func TestNodeTimerCheckpointRunsAfterSoleListRetires(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		let sentinel;
		const first = setTimeout(function() {
			process.nextTick(function() {
				const replacement = setTimeout(function() { events.push("replacement"); }, 1);
				events.push(replacement._idlePrev === sentinel ? "same" : "different");
			});
		}, 1);
		sentinel = first._idlePrev;
		setTimeout(function() { events.push("keep"); }, 20);
	`)
	if want := "different,replacement,keep"; got != want {
		t.Fatalf("post-list timer checkpoint = %q, want %q", got, want)
	}
}

func TestNodeTimerCheckpointFollowsNextPeerDiff(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		let second;
		setImmediate(function() {
			setTimeout(function() {
				process.nextTick(function() {
					second.refresh();
					setImmediate(function() { events.push("i"); });
				});
			}, 1);
			second = setTimeout(function() { events.push("second"); }, 1);
			setTimeout(function() { events.push("keep"); }, 20);
		});
	`)
	if want := "second,i,keep"; got != want {
		t.Fatalf("timer peer diff checkpoint = %q, want %q", got, want)
	}
}

func TestNodeTimerBoundaryCheckpointPreservesNewCarrier(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		setTimeout(function() {
			process.nextTick(function() {
				setTimeout(function() { events.push("later"); }, 5);
			});
		}, 1);
	`)
	if want := "later"; got != want {
		t.Fatalf("post-boundary timer carrier = %q, want %q", got, want)
	}
}

func TestNodeTimerBoundaryResumesHandledThrowCheckpoint(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function() {
			events.push("u");
			setImmediate(function() { events.push("i"); });
		});
		setTimeout(function() {
			events.push("t1");
			process.nextTick(function() { events.push("n"); });
			Promise.resolve().then(function() { events.push("p"); });
			throw new Error("boom");
		}, 1);
		setTimeout(function() { events.push("keep"); }, 20);
	`)
	if want := "t1,u,n,p,i,keep"; got != want {
		t.Fatalf("handled timer boundary checkpoint = %q, want %q", got, want)
	}
}

func TestNodeNestedProxyTimeoutRefTransitionsOnce(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		setTimeout(function bootstrap() {
			const latent = new this.constructor(function() {}, 20, undefined, false, true);
			const proxy = new Proxy(new Proxy(latent, {}), {});
			proxy.unref();
			proxy.ref();
			setTimeout(function() {
				events.push("timer");
				proxy.unref();
			}, 20).unref();
		}, 0);
	`)
	if got != "timer" {
		t.Fatalf("nested Proxy timeout lifecycle = %q, want %q", got, "timer")
	}
}

func TestNodeSameDurationPeerPrecedesIntervalRepeat(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		let count = 0;
		const interval = setInterval(function intervalCallback() {
			count++;
			events.push("interval-" + count);
			if (count === 1) {
				const until = performance.now() + 5;
				while (performance.now() < until) {}
			} else {
				clearInterval(interval);
			}
		}, 20);
		setTimeout(function peer() { events.push("peer"); }, 20);
	`)
	if got != "interval-1,peer,interval-2" {
		t.Fatalf("same-duration timer order = %q, want %q", got, "interval-1,peer,interval-2")
	}
}

func TestNodeTimerHandledDirectThrowYieldsPeerBeforeMicrotasks(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function() { events.push("u"); });
		setImmediate(function() {
			events.push("i");
			setTimeout(function() {
				events.push("t1");
				throw new Error("direct");
			}, 0);
			setTimeout(function() { events.push("t2"); }, 0);
		});
	`)
	if want := "i,t1,u,t2"; got != want {
		t.Fatalf("handled direct timer throw order = %q, want %q", got, want)
	}
}

func TestNodeTimerHandledNextTickThrowYieldsPeerBeforeRemainingMicrotasks(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function() { events.push("u"); });
		setImmediate(function() {
			events.push("i");
			setTimeout(function() {
				events.push("t1");
				process.nextTick(function() {
					events.push("n1");
					throw new Error("nextTick");
				});
				process.nextTick(function() { events.push("n2"); });
				Promise.resolve().then(function() { events.push("p"); });
			}, 0);
			setTimeout(function() { events.push("t2"); }, 0);
		});
	`)
	if want := "i,t1,n1,u,t2,n2,p"; got != want {
		t.Fatalf("handled nextTick timer throw order = %q, want %q", got, want)
	}
}

func TestNodeIntervalDelayAnchorsCallbackStart(t *testing.T) {
	for _, test := range []struct {
		repeat  float64
		elapsed float64
		want    int
	}{
		{repeat: 10, want: 10},
		{repeat: 10, elapsed: 3.2, want: 7},
		{repeat: 10, elapsed: 10, want: 1},
		{repeat: 5.9, want: 5},
		{repeat: 0.9, want: 1},
		{repeat: nodeTimerDelayMax + 10, want: nodeTimerDelayMax},
	} {
		if got := nodeIntervalDelay(test.repeat, test.elapsed); got != test.want {
			t.Fatalf("nodeIntervalDelay(%v, %v) = %d, want %d", test.repeat, test.elapsed, got, test.want)
		}
	}
}

func newAutoExitAdapter(t *testing.T) (context.Context, *goeventloop.Loop, *goja.Runtime, *Adapter) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	loop := goeventloop.New(goeventloop.WithAutoExit(true))
	t.Cleanup(func() { _ = loop.Close() })
	runtime := goja.New()
	adapter, err := New(loop, runtime)
	if err != nil {
		t.Fatalf("New adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	return ctx, loop, runtime, adapter
}
