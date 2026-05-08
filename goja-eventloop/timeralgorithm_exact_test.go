package gojaeventloop

import (
	"errors"
	"testing"

	"github.com/joeycumines/goja"
)

func TestNodeTimerQueueInsertWritesFinalPositionOnce(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const seed = setTimeout(function() {}, 100);
		const prototype = Object.getPrototypeOf(seed._idlePrev);
		clearTimeout(seed);
		Object.defineProperty(prototype, "priorityQueuePosition", {
			configurable: true,
			get() { return this.__position; },
			set(value) {
				events.push(String(value));
				this.__position = value;
			},
		});
		const handle = setTimeout(function() {}, 101);
		clearTimeout(handle);
	`)
	if want := "null,1"; got != want {
		t.Fatalf("priority queue insert writes = %q, want %q", got, want)
	}
}

func TestTimerFactoryExceptionsAvoidThrownValueCoercion(t *testing.T) {
	runtime := goja.New()
	thrown, err := runtime.RunString(`
		globalThis.coercions = 0;
		globalThis.thrown = {
			[Symbol.toPrimitive]: function() {
				coercions++;
				return "coerced";
			},
		};
		thrown;
	`)
	if err != nil {
		t.Fatalf("install thrown value: %v", err)
	}
	assertSafe := func(t *testing.T, operation string, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("operation returned nil error")
		}
		if want := "goja-eventloop: " + operation + ": JavaScript exception"; err.Error() != want {
			t.Fatalf("factory error = %q, want %q", err.Error(), want)
		}
		if got := runtime.Get("coercions").ToInteger(); got != 0 {
			t.Fatalf("thrown value coercions = %d, want 0", got)
		}
		var exception *goja.Exception
		if !errors.As(err, &exception) {
			t.Fatalf("factory error %T does not preserve *goja.Exception", err)
		}
		if exception.Value() != thrown {
			t.Fatal("factory error did not preserve thrown value identity")
		}
	}
	t.Run("RunString error", func(t *testing.T) {
		_, runErr := runtime.RunString(`throw thrown`)
		assertSafe(t, "initialize timer-handle factory",
			wrapRuntimeError("initialize timer-handle factory", runErr))
	})
	t.Run("ambient Number ignored", func(t *testing.T) {
		if _, err = runtime.RunString(`
			globalThis.Number = {
				get MIN_SAFE_INTEGER() { throw thrown; },
			};
		`); err != nil {
			t.Fatalf("mutate factory intrinsic: %v", err)
		}
		adapter := &Adapter{
			runtime:                  runtime,
			objectGetOwnPropertyDesc: objectGetOwnPropertyDescriptor(runtime),
		}
		if err := adapter.initHandlePrototypes(); err != nil {
			t.Fatalf("initialize timer handles with replaced Number: %v", err)
		}
		if got := runtime.Get("coercions").ToInteger(); got != 0 {
			t.Fatalf("ambient Number replacement coerced thrown value %d times", got)
		}
	})
}

func TestNodeTimerConstructionExceptionsPreserveThrownIdentity(t *testing.T) {
	_, _, runtime, _ := newAutoExitAdapter(t)
	value, err := runtime.RunString(`
		const events = [];
		let coercions = 0;
		const thrown = {
			[Symbol.toPrimitive]: function() {
				coercions++;
				return "coerced";
			},
		};
		const timeoutSeed = setTimeout(function() {}, 1000);
		const Timeout = timeoutSeed.constructor;
		clearTimeout(timeoutSeed);
		const immediateSeed = setImmediate(function() {});
		const Immediate = immediateSeed.constructor;
		clearImmediate(immediateSeed);

		class ImmediateSetter extends Immediate {
			set _idleNext(value) { throw thrown; }
		}
		try {
			new ImmediateSetter(function() {});
		} catch (error) {
			events.push("immediate:" + (error === thrown));
		}

		class TimeoutSetter extends Timeout {
			set _idleTimeout(value) { throw thrown; }
		}
		try {
			new TimeoutSetter(function() {}, 1);
		} catch (error) {
			events.push("timeout:" + (error === thrown));
		}

		let writes = 0;
		let idleStart;
		Object.defineProperty(Timeout.prototype, "_idleStart", {
			configurable: true,
			get: function() { return idleStart; },
			set: function(value) {
				if (++writes === 2) throw thrown;
				idleStart = value;
			},
		});
		try {
			setTimeout(function() {}, 1);
		} catch (error) {
			events.push("activation:" + (error === thrown));
		} finally {
			delete Timeout.prototype._idleStart;
		}
		events.push("coercions:" + coercions);
		events.join(",");
	`)
	if err != nil {
		t.Fatalf("exercise timer construction exceptions: %v", err)
	}
	if want := "immediate:true,timeout:true,activation:true,coercions:0"; value.String() != want {
		t.Fatalf("timer construction exceptions = %q, want %q", value.String(), want)
	}
}

func TestWebAbortTimeoutDoesNotObserveTimeoutPrototype(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const seed = setTimeout(function() {}, 1000);
		const prototype = Object.getPrototypeOf(seed);
		clearTimeout(seed);
		const keepalive = setTimeout(function() {
			delete prototype.unref;
			delete prototype._idleStart;
			events.push("keepalive:" + hooks);
		}, 20);
		let hooks = 0;
		Object.defineProperty(prototype, "unref", {
			configurable: true,
			get: function() {
				hooks++;
				throw new Error("AbortSignal.timeout read Timeout.prototype.unref");
			},
		});
		Object.defineProperty(prototype, "_idleStart", {
			configurable: true,
			set: function() {
				hooks++;
				throw new Error("AbortSignal.timeout wrote through Timeout.prototype");
			},
		});
		const signal = AbortSignal.timeout(0);
		signal.addEventListener("abort", function() { events.push(signal.reason.name); });
		void keepalive;
	`)
	const want = "TimeoutError,keepalive:0"
	if got != want {
		t.Fatalf("AbortSignal.timeout private handle = %q, want %q", got, want)
	}
}

func TestNodeImmediateConstructorRefThrowRetainsNativeLiveness(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const seed = setImmediate(function() {});
		const Immediate = seed.constructor;
		clearImmediate(seed);
		class Sub extends Immediate {
			ref() {
				super.ref();
				events.push("ref");
				throw new Error("boom");
			}
		}
		try {
			new Sub(function() { events.push("callback"); });
		} catch (error) {
			events.push("caught");
		}
		setTimeout(function() {
			events.push("release");
			process.exit(0);
		}, 20).unref();
	`)
	if want := "ref,caught,release"; got != want {
		t.Fatalf("Immediate constructor ref throw liveness = %q, want %q", got, want)
	}
}

func TestNodeImmediateConstructorExecutesPartiallyAppendedHandle(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const first = setImmediate(function() { events.push("first"); });
		const Immediate = first.constructor;
		let next = null;
		Object.defineProperty(first, "_idleNext", {
			configurable: true,
			get: function() { return next; },
			set: function(value) {
				next = value;
				events.push("throw");
				throw new Error("append");
			},
		});
		try {
			new Immediate(function() { events.push("partial"); });
		} catch (error) {
			events.push("caught:" + error.message);
		}
		setTimeout(function() { events.push("last"); }, 20);
	`)
	if want := "throw,caught:append,first,partial,last"; got != want {
		t.Fatalf("partial Immediate append trace = %q, want %q", got, want)
	}
}

func TestNodeTimeoutConstructorRefSetterRetainsNativeLiveness(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const seed = setTimeout(function() {}, 1000);
		const Timeout = seed.constructor;
		const refed = Object.getOwnPropertySymbols(seed).find(function(symbol) {
			return symbol.description === "refed";
		});
		clearTimeout(seed);
		class Sub extends Timeout {
			set [refed](value) {
				events.push("setter:" + value);
				throw new Error("boom");
			}
		}
		try {
			new Sub(function() {}, 1, undefined, false, true);
		} catch (error) {
			events.push("caught:" + error.message);
		}
		setTimeout(function() { events.push("release"); }, 20).unref();
	`)
	if want := "setter:true,caught:boom,release"; got != want {
		t.Fatalf("Timeout constructor ref setter liveness = %q, want %q", got, want)
	}
}

func TestNodeTimerClearAcceptsCorruptedQueuePosition(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		setTimeout(function() { events.push("first"); }, 1);
		const corrupted = setTimeout(function() { events.push("corrupted"); }, 10);
		corrupted._idlePrev.priorityQueuePosition = 999;
		clearTimeout(corrupted);
		setTimeout(function() { events.push("last"); }, 20);
	`)
	if want := "first,last"; got != want {
		t.Fatalf("timer clear with corrupted queue position = %q, want %q", got, want)
	}
}

func TestNodeTimerQueuePercolateDownSetterPrecedesHeapWrite(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const root = setTimeout(function() {}, 50000);
		const middle = setTimeout(function() {}, 60000);
		const bottom = setTimeout(function() {}, 70000);
		const list = middle._idlePrev;
		let position = list.priorityQueuePosition;
		let reentered = false;
		Object.defineProperty(list, "priorityQueuePosition", {
			configurable: true,
			get: function() { return position; },
			set: function(value) {
				events.push("set:" + value);
				if (!reentered) {
					reentered = true;
					const injected = setTimeout(function() {}, 55000);
					events.push("injected:" + injected._idlePrev.priorityQueuePosition);
				}
				position = value;
			},
		});
		clearTimeout(root);
		clearTimeout(middle);
		clearTimeout(bottom);
		setImmediate(function() { process.exit(0); });
	`)
	if want := "set:1,injected:1"; got != want {
		t.Fatalf("reentrant queue setter trace = %q, want %q", got, want)
	}
}

func TestNodeTimerQueuePercolateDownCachesComparedChild(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const root = setTimeout(function() {}, 50000);
		const left = setTimeout(function() {}, 60000);
		const right = setTimeout(function() {}, 70000);
		const bottom = setTimeout(function() {}, 80000);
		const leftList = left._idlePrev;
		const rightList = right._idlePrev;
		const bottomList = bottom._idlePrev;
		let reentered = false;
		let injectedList;
		Object.defineProperty(bottomList, "expiry", {
			configurable: true,
			get: function() {
				events.push("get-bottom");
				if (!reentered) {
					reentered = true;
					const injected = setTimeout(function() {}, 55000);
					injectedList = injected._idlePrev;
					events.push("injected:" + injectedList.priorityQueuePosition);
				}
				return 80000;
			},
		});
		clearTimeout(root);
		events.push("pos:" + [
			leftList.priorityQueuePosition,
			rightList.priorityQueuePosition,
			bottomList.priorityQueuePosition,
			injectedList.priorityQueuePosition,
		].join("/"));
		setImmediate(function() { process.exit(0); });
	`)
	const want = "get-bottom,get-bottom,injected:1,pos:1/3/2/1"
	if got != want {
		t.Fatalf("reentrant queue comparison trace = %q, want %q", got, want)
	}
}

func TestNodeTimerRefreshExecutesPartiallyInsertedGenericReceiver(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const seed = setTimeout(function() { events.push("seed"); }, 1);
		const refed = Object.getOwnPropertySymbols(seed).find(function(symbol) {
			return symbol.description === "refed";
		});
		const generic = Object.create(seed);
		generic[refed] = false;
		generic._idleTimeout = 1;
		generic._idleNext = null;
		generic._idlePrev = null;
		generic._onTimeout = function() { events.push("generic"); };
		generic._timerArgs = undefined;
		generic._repeat = null;
		generic._destroyed = false;
		const list = seed._idlePrev;
		let next = list._idleNext;
		let first = true;
		Object.defineProperty(list, "_idleNext", {
			configurable: true,
			get: function() { return next; },
			set: function(value) {
				next = value;
				if (first) {
					first = false;
					events.push("throw");
					throw new Error("append");
				}
			},
		});
		try {
			generic.refresh();
		} catch (error) {
			events.push("caught:" + error.message);
		}
		setTimeout(function() { events.push("last"); }, 20);
	`)
	if want := "throw,caught:append,seed,generic,last"; got != want {
		t.Fatalf("partial generic refresh trace = %q, want %q", got, want)
	}
}

func TestNodeTimerRefreshExecutesPartiallyReinsertedRetiredHandle(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const revived = setTimeout(function() { events.push("old"); }, 1);
		clearTimeout(revived);
		const seed = setTimeout(function() { events.push("seed"); }, 5);
		const list = seed._idlePrev;
		let next = list._idleNext;
		let first = true;
		Object.defineProperty(list, "_idleNext", {
			configurable: true,
			get: function() { return next; },
			set: function(value) {
				next = value;
				if (first) {
					first = false;
					events.push("throw");
					throw new Error("append");
				}
			},
		});
		revived._idleTimeout = 5;
		revived._onTimeout = function() { events.push("revived"); };
		try {
			revived.refresh();
		} catch (error) {
			events.push("caught:" + error.message);
		}
		setTimeout(function() { events.push("last"); }, 20);
	`)
	const want = "throw,caught:append,seed,revived,last"
	if got != want {
		t.Fatalf("partial retired refresh trace = %q, want %q", got, want)
	}
}

func TestNodeTimerListRetirementUsesCachedDuration(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const first = setTimeout(function() { events.push("first"); }, 1);
		const list = first._idlePrev;
		let reads = 0;
		Object.defineProperty(list, "msecs", {
			configurable: true,
			get() {
				reads++;
				return reads % 2 === 1 ? 1 : 2;
			},
		});
		setTimeout(function() {
			events.push("last", "reads=" + reads);
		}, 20);
	`)
	if want := "first,last,reads=1"; got != want {
		t.Fatalf("cached timer-list duration = %q, want %q", got, want)
	}
}

func TestNodeTimerUnenrollDeletesObservableListDuration(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const stale = setTimeout(function() { events.push("stale"); }, 50);
		stale._idlePrev.msecs = 60;
		clearTimeout(stale);
		setTimeout(function() { events.push("replacement"); }, 50);
	`)
	if got != "" {
		t.Fatalf("mutated timer-list duration callbacks = %q, want empty", got)
	}
}

func TestNodeTimerListPeekTreatsUndefinedAsEmpty(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const broken = setTimeout(function() { events.push("broken"); }, 1);
		broken._idlePrev._idlePrev = undefined;
		setTimeout(function() { events.push("last"); }, 20);
	`)
	if want := "last"; got != want {
		t.Fatalf("undefined timer-list peek = %q, want %q", got, want)
	}
}

func TestNodeHandledTimerThrowConsumesOneSkippedCheckpoint(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function() { events.push("u"); });
		setImmediate(function() {
			events.push("i");
			setTimeout(function() {
				events.push("t1");
				process.nextTick(function() { events.push("n"); });
				Promise.resolve().then(function() { events.push("p"); });
				throw new Error("boom");
			}, 0);
			const skipped = setTimeout(function() { events.push("bad"); }, 0);
			skipped._onTimeout = null;
			setTimeout(function() { events.push("peer"); }, 0);
		});
	`)
	if want := "i,t1,u,n,p,peer"; got != want {
		t.Fatalf("handled throw skipped checkpoint = %q, want %q", got, want)
	}
}

func TestNodeHandledTickThrowAdvancesPastSkippedTimer(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function() { events.push("u"); });
		setImmediate(function() {
			events.push("i");
			setTimeout(function() {
				events.push("t1");
				process.nextTick(function() {
					events.push("n1");
					throw new Error("boom");
				});
				process.nextTick(function() { events.push("n2"); });
				Promise.resolve().then(function() { events.push("p"); });
			}, 0);
			const skipped = setTimeout(function() { events.push("bad"); }, 0);
			skipped._onTimeout = null;
			setTimeout(function() { events.push("peer"); }, 0);
		});
	`)
	if want := "i,t1,n1,u,n2,p,peer"; got != want {
		t.Fatalf("handled tick throw skipped checkpoint = %q, want %q", got, want)
	}
}

func TestNodeTimerSelectionConsumesPriorYield(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function() { events.push("u"); });
		setTimeout(function() {
			events.push("t0");
			process.nextTick(function() {
				events.push("n1");
				throw new Error("boom");
			});
			process.nextTick(function() { events.push("n2"); });
			Promise.resolve().then(function() { events.push("p"); });
		}, 1);
		const skipped = setTimeout(function() { events.push("bad"); }, 20);
		skipped._onTimeout = null;
		setTimeout(function() { events.push("peer"); }, 20);
	`)
	if want := "t0,n1,u,n2,p,peer"; got != want {
		t.Fatalf("fresh timer selection checkpoint = %q, want %q", got, want)
	}
}

func TestNodeFutureTimerWakeResumesHandledDirectThrowCheckpoint(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function() { events.push("u"); });
		setTimeout(function() {
			events.push("t0");
			process.nextTick(function() { events.push("n"); });
			Promise.resolve().then(function() { events.push("p"); });
			throw new Error("direct");
		}, 1);
		setTimeout(function() { events.push("peer"); }, 20);
	`)
	if want := "t0,u,n,p,peer"; got != want {
		t.Fatalf("future wake direct-throw checkpoint = %q, want %q", got, want)
	}
}

func TestNodeFutureTimerWakeResumesHandledTickThrowCheckpoint(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function() { events.push("u"); });
		setTimeout(function() {
			events.push("t0");
			process.nextTick(function() {
				events.push("n1");
				throw new Error("tick");
			});
			process.nextTick(function() { events.push("n2"); });
			Promise.resolve().then(function() { events.push("p"); });
		}, 1);
		setTimeout(function() { events.push("peer"); }, 20);
	`)
	if want := "t0,n1,u,n2,p,peer"; got != want {
		t.Fatalf("future wake tick-throw checkpoint = %q, want %q", got, want)
	}
}

func TestNodeTimerAlgorithmThrowCheckpointsBeforeRetry(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function() {
			events.push("u");
			process.nextTick(function() { events.push("n"); });
			Promise.resolve().then(function() { events.push("p"); });
		});
		const handle = setTimeout(function() { events.push("callback"); }, 1);
		const list = handle._idlePrev;
		let reads = 0;
		let expiry = list.expiry;
		Object.defineProperty(list, "expiry", {
			configurable: true,
			get: function() {
				events.push("get" + (++reads));
				if (reads === 1) throw new Error("expiry");
				return expiry;
			},
			set: function(value) { expiry = value; },
		});
	`)
	if want := "get1,u,n,p,get2,callback"; got != want {
		t.Fatalf("timer algorithm retry checkpoint = %q, want %q", got, want)
	}
}

func TestNodeImmediateSubclassRefOverrideControlsInitialLiveness(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const seed = setImmediate(function() {});
		const Immediate = seed.constructor;
		clearImmediate(seed);
		class Sub extends Immediate {
			ref() {
				events.push("override-ref");
				return this;
			}
		}
		const value = new Sub(function() { events.push("callback"); });
		events.push(String(value.hasRef()));
		setTimeout(function() { events.push("keep"); }, 10);
	`)
	if want := "override-ref,false,callback,keep"; got != want {
		t.Fatalf("Immediate subclass initial ref = %q, want %q", got, want)
	}
}

func TestNodeImmediateSubclassRefOverrideCannotReconcileNativeLiveness(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		setTimeout(function() {
			const seed = setImmediate(function() {});
			const Immediate = seed.constructor;
			clearImmediate(seed);
			class Sub extends Immediate {
				ref() {
					const refed = Object.getOwnPropertySymbols(this).find(function(symbol) {
						return symbol.description === "refed";
					});
					this[refed] = true;
					events.push("override");
					return this;
				}
			}
			new Sub(function() { events.push("callback"); });
		}, 1);
	`)
	if want := "override"; got != want {
		t.Fatalf("Immediate override native liveness = %q, want %q", got, want)
	}
}

func TestNodeImmediateMalformedIteratorUsesCapturedString(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const originalString = String;
		const seed = setImmediate(function() {});
		const Immediate = seed.constructor;
		clearImmediate(seed);
		const args = {
			[Symbol.iterator]: function() {
				return { next: function() { return 1; } };
			},
		};
		process.on("uncaughtException", function(error) {
			events.push(error.message);
		});
		new Immediate(function() { events.push("callback"); }, args);
		globalThis.String = function() {
			events.push("String");
			return "tampered";
		};
		setImmediate(function() {
			globalThis.String = originalString;
			events.push("peer");
		});
	`)
	if want := "Iterator result 1 is not an object,peer"; got != want {
		t.Fatalf("Immediate malformed iterator diagnostic = %q, want %q", got, want)
	}
}

func TestNodeTimerCallbackReadsAsyncSymbolsInOrder(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const handle = setTimeout(function() { events.push("callback"); }, 1);
		for (const symbol of Object.getOwnPropertySymbols(handle)) {
			if (symbol.description !== "kAsyncContextFrame" && symbol.description !== "triggerId") continue;
			const value = handle[symbol];
			Object.defineProperty(handle, symbol, {
				configurable: true,
				get: function() {
					events.push("get:" + symbol.description);
					return value;
				},
			});
		}
	`)
	if want := "get:kAsyncContextFrame,get:triggerId,callback"; got != want {
		t.Fatalf("Timeout async symbol reads = %q, want %q", got, want)
	}
}

func TestNodeImmediateCallbackReadsAsyncSymbolsInOrder(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const handle = setImmediate(function() { events.push("callback"); });
		for (const symbol of Object.getOwnPropertySymbols(handle)) {
			if (!["refed", "kAsyncContextFrame", "asyncId", "triggerId"].includes(symbol.description)) continue;
			let value = handle[symbol];
			Object.defineProperty(handle, symbol, {
				configurable: true,
				get: function() {
					events.push("get:" + symbol.description);
					return value;
				},
				set: function(next) {
					events.push("set:" + symbol.description);
					value = next;
				},
			});
		}
		setImmediate(function() { events.push("peer"); });
	`)
	const want = "get:refed,set:refed,get:kAsyncContextFrame,get:asyncId,get:triggerId,callback,peer"
	if got != want {
		t.Fatalf("Immediate async symbol reads = %q, want %q", got, want)
	}
}

func TestNodeImmediatePreCallbackThrowRetainsOutstandingLiveness(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		process.on("uncaughtException", function(error) {
			events.push("u:" + error.message);
		});
		const first = setImmediate(function() { events.push("first"); });
		let reads = 0;
		let destroyed = false;
		Object.defineProperty(first, "_destroyed", {
			configurable: true,
			get: function() {
				if (++reads === 1) throw new Error("destroyed");
				return destroyed;
			},
			set: function(value) { destroyed = value; },
		});
		setImmediate(function() { events.push("peer"); });
		setTimeout(function() {
			events.push("release", "reads=" + reads);
			process.exit(0);
		}, 20).unref();
	`)
	if want := "u:destroyed,release,reads=1"; got != want {
		t.Fatalf("Immediate pre-callback throw liveness = %q, want %q", got, want)
	}
}

func TestNodeImmediateResumedDestroyedHeadUsesMissingPrevious(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		let second;
		let errors = 0;
		process.on("uncaughtException", function(error) {
			events.push("u" + (++errors) + ":" + error.message);
			if (errors === 1) {
				second._destroyed = true;
			} else {
				process.exit(0);
			}
		});
		setImmediate(function() {
			events.push("first");
			throw new Error("first-error");
		});
		second = setImmediate(function() { events.push("second"); });
		setImmediate(function() { events.push("third"); });
		setTimeout(function() { process.exit(0); }, 20);
	`)
	const want = "first,u1:first-error,u2:Cannot read properties of undefined (reading '_idleNext')"
	if got != want {
		t.Fatalf("Immediate resumed destroyed head = %q, want %q", got, want)
	}
}

func TestNodeImmediateResumedClearedHeadUsesMissingPrevious(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		let second;
		let errors = 0;
		process.on("uncaughtException", function(error) {
			events.push("u" + (++errors) + ":" + error.message);
			if (errors === 1) {
				clearImmediate(second);
			} else {
				process.exit(0);
			}
		});
		setImmediate(function() {
			events.push("first");
			throw new Error("first-error");
		});
		second = setImmediate(function() { events.push("second"); });
		setImmediate(function() { events.push("third"); });
		setTimeout(function() { process.exit(0); }, 20);
	`)
	const want = "first,u1:first-error,u2:Cannot read properties of undefined (reading '_idleNext')"
	if got != want {
		t.Fatalf("Immediate resumed cleared head = %q, want %q", got, want)
	}
}

func TestNodeImmediateCheckpointClearSkipsWithPreviousHandle(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		let second;
		setImmediate(function() {
			events.push("first");
			queueMicrotask(function() {
				clearImmediate(second);
				events.push("microtask");
			});
		});
		second = setImmediate(function() { events.push("second"); });
		setImmediate(function() { events.push("third"); });
	`)
	if want := "first,microtask,third"; got != want {
		t.Fatalf("Immediate checkpoint clear trace = %q, want %q", got, want)
	}
}

func TestNodeImmediateFirstCleanupThrowLosesOldQueue(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		let handled = false;
		process.on("uncaughtException", function(error) {
			events.push("u:" + error.message);
			if (!handled) {
				handled = true;
				setImmediate(function() { events.push("new"); });
			}
		});
		const first = setImmediate(function() { events.push("callback"); });
		let callback = first._onImmediate;
		let writes = 0;
		Object.defineProperty(first, "_onImmediate", {
			configurable: true,
			get: function() {
				events.push("get");
				return callback;
			},
			set: function() {
				if (++writes === 1) {
					events.push("set");
					throw new Error("setter");
				}
			},
		});
		setImmediate(function() { events.push("old-peer"); });
		setTimeout(function() { events.push("timeout"); }, 20);
	`)
	const want = "get,callback,set,u:setter,new,timeout"
	if got != want {
		t.Fatalf("Immediate cleanup failure trace = %q, want %q", got, want)
	}
}

func TestNodeImmediateTraversalUsesVisibleLinks(t *testing.T) {
	got := runAutoExitProcessScript(t, `
		globalThis.events = [];
		const first = setImmediate(function() {
			events.push("first");
			first._idleNext = null;
		});
		setImmediate(function() { events.push("second"); }).unref();
		setTimeout(function() { events.push("keep"); }, 10);
	`)
	if want := "first,keep"; got != want {
		t.Fatalf("Immediate visible-link traversal = %q, want %q", got, want)
	}
}
