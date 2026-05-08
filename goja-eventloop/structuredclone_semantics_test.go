package gojaeventloop

import "testing"

func TestWebStructuredCloneRejectsRetainedPlatformObjects(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			const controller = new AbortController();
			const values = [
				new Event("sample"),
				new CustomEvent("sample", { detail: 1 }),
				new EventTarget(),
				controller,
				controller.signal,
				performance,
				crypto,
			];
			const direct = [];
			const nested = [];
			for (const value of values) {
				try { structuredClone(value); direct.push("missing"); }
				catch (error) { direct.push(error.name); }
				try { structuredClone({ value }); nested.push("missing"); }
				catch (error) { nested.push(error.name); }
			}
			return direct.concat(nested).join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	const want = "DataCloneError,DataCloneError,DataCloneError,DataCloneError,DataCloneError,DataCloneError,DataCloneError," +
		"DataCloneError,DataCloneError,DataCloneError,DataCloneError,DataCloneError,DataCloneError,DataCloneError"
	if got := value.String(); got != want {
		t.Fatalf("retained platform object clone results = %q, want %q", got, want)
	}
}

func TestWebStructuredClonePlatformBrandsAreUnforgeable(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			const controller = new AbortController();
			const genuine = [
				new Event("sample"),
				new CustomEvent("sample", { detail: 1 }),
				new EventTarget(),
				controller,
				controller.signal,
				performance,
				crypto,
			];
			const fakes = genuine.map((item, index) => {
				const fake = Object.create(Object.getPrototypeOf(item));
				fake.marker = index + 1;
				return fake;
			});
			const fakeClones = fakes.map((fake) => structuredClone(fake));

			const errors = [];
			for (const item of genuine) {
				Object.setPrototypeOf(item, null);
				for (const input of [item, { item }]) {
					try { structuredClone(input); errors.push("missing"); }
					catch (error) { errors.push(error.name + ":" + error.code); }
				}
			}

			const exception = new DOMException("message", "DataError");
			Object.setPrototypeOf(exception, null);
			const exceptionClone = structuredClone(exception);
			return [
				errors.length,
				errors.every((result) => result === "DataCloneError:25"),
				fakeClones.map((clone) => clone.marker).join("/"),
				fakeClones.every((clone) => Object.getPrototypeOf(clone) === Object.prototype),
				exceptionClone instanceof DOMException,
				exceptionClone.name,
				exceptionClone.message,
			].join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := value.String(), "14,true,1/2/3/4/5/6/7,true,true,DataError,message"; got != want {
		t.Fatalf("platform structured clone brands = %q, want %q", got, want)
	}
}

func TestWebStructuredCloneGetterOrderAndEarlyRejection(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			let ordinaryGets = 0;
			let platformGets = 0;
			let globalGets = 0;
			let transferGets = 0;

			const ordinary = {};
			Object.defineProperty(ordinary, "value", {
				enumerable: true,
				get() { ordinaryGets++; return 7; },
			});
			const ordinaryClone = structuredClone(ordinary);

			Object.defineProperty(performance, "probe", {
				configurable: true,
				enumerable: true,
				get() { platformGets++; return "wrong"; },
			});
			let platformError = "missing";
			try { structuredClone(performance); }
			catch (error) { platformError = error.name; }

			Object.defineProperty(globalThis, "cloneProbe", {
				configurable: true,
				enumerable: true,
				get() { globalGets++; return "wrong"; },
			});
			let globalError = "missing";
			try { structuredClone(globalThis); }
			catch (error) { globalError = error.name; }

			const invalidTransfer = {};
			Object.defineProperty(invalidTransfer, "probe", {
				enumerable: true,
				get() { transferGets++; return "wrong"; },
			});
			let transferError = "missing";
			try { structuredClone(null, { transfer: [invalidTransfer] }); }
			catch (error) { transferError = error.name; }

			return [
				ordinaryClone.value,
				ordinaryGets,
				platformError,
				platformGets,
				globalError,
				globalGets,
				transferError,
				transferGets,
			].join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	const want = "7,1,DataCloneError,0,DataCloneError,0,DataCloneError,0"
	if got := value.String(); got != want {
		t.Fatalf("structured clone getter order = %q, want %q", got, want)
	}
}

func TestNodeStructuredCloneTimerHandlesFollowVisibleState(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			const timeout = setTimeout(function() {}, 1000);
			const immediate = setImmediate(function() {});
			const errors = [];
			for (const handle of [timeout, immediate]) {
				try { structuredClone(handle); errors.push("missing"); }
				catch (error) { errors.push(error.name); }
			}
			clearTimeout(timeout);
			clearImmediate(immediate);
			const timeoutClone = structuredClone(timeout);
			const immediateClone = structuredClone(immediate);
			return [
				errors.join("/"),
				timeoutClone !== timeout,
				timeoutClone._onTimeout === null,
				immediateClone !== immediate,
				immediateClone._onImmediate === null,
			].join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := value.String(), "DataCloneError/DataCloneError,true,true,true,true"; got != want {
		t.Fatalf("timer handle clone lifecycle = %q, want %q", got, want)
	}
}

func TestWebStructuredClonePrimitiveWrappersAndSparseArrayLength(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			const bigint = Object(7n);
			bigint.extra = { marker: 1 };
			const bigintClone = structuredClone(bigint);
			let symbolError = "missing";
			try { structuredClone(Object(Symbol("sample"))); }
			catch (error) { symbolError = error.name; }

			const sparse = [];
			sparse.length = 4294967295;
			sparse[4294967294] = "tail";
			const sparseClone = structuredClone(sparse);
			return [
				bigintClone !== bigint,
				bigintClone.valueOf() === 7n,
				bigintClone.extra === undefined,
				symbolError,
				sparseClone.length,
				sparseClone[4294967294],
			].join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	if got, want := value.String(), "true,true,true,DataCloneError,4294967295,tail"; got != want {
		t.Fatalf("primitive wrapper and sparse array clone = %q, want %q", got, want)
	}
}

func TestWebStructuredCloneErrorDataConversionAndStackIsolation(t *testing.T) {
	adapter := newBoundAdapterForNode26Test(t)
	value, err := adapter.runtime.RunString(`
		(() => {
			let messageGets = 0;
			let stackGets = 0;

			const undefinedMessage = new Error("source");
			Object.defineProperty(undefinedMessage, "message", {
				value: undefined,
				writable: true,
				configurable: true,
			});
			const undefinedClone = structuredClone(undefinedMessage);

			const accessorMessage = new Error();
			Object.defineProperty(accessorMessage, "message", {
				configurable: true,
				get() { messageGets++; return "wrong"; },
			});
			const accessorClone = structuredClone(accessorMessage);

			let causeGets = 0;
			const accessorCause = new Error("cause");
			Object.defineProperty(accessorCause, "cause", {
				configurable: true,
				get() { causeGets++; return { wrong: true }; },
			});
			const accessorCauseClone = structuredClone(accessorCause);

			const dataCause = new Error("cause");
			const sourceCause = { marker: 1 };
			Object.defineProperty(dataCause, "cause", {
				value: sourceCause,
				writable: true,
				configurable: true,
			});
			const dataCauseClone = structuredClone(dataCause);

			const mutatingCauseError = new Error("cause order");
			mutatingCauseError.stack = "before";
			const mutatingCause = {};
			Object.defineProperty(mutatingCause, "value", {
				enumerable: true,
				get() {
					mutatingCauseError.stack = "after";
					return 2;
				},
			});
			mutatingCauseError.cause = mutatingCause;
			const mutatingCauseClone = structuredClone(mutatingCauseError);

			const stringStack = new Error("stack");
			Object.defineProperty(stringStack, "stack", {
				value: "source stack",
				writable: true,
				configurable: true,
			});
			const stringStackClone = structuredClone(stringStack);

			const objectStack = new Error("stack");
			Object.defineProperty(objectStack, "stack", {
				value: { wrong: true },
				writable: true,
				configurable: true,
			});
			const objectStackClone = structuredClone(objectStack);

			const accessorStack = new DOMException("message", "DataError");
			Object.defineProperty(accessorStack, "stack", {
				configurable: true,
				get() { stackGets++; return "wrong"; },
			});
			const accessorStackClone = structuredClone(accessorStack);

			return [
				Object.hasOwn(undefinedClone, "message"),
				undefinedClone.message,
				Object.hasOwn(accessorClone, "message"),
				messageGets,
				Object.hasOwn(accessorCauseClone, "cause"),
				causeGets,
				dataCauseClone.cause !== sourceCause,
				dataCauseClone.cause.marker,
				mutatingCauseClone.stack,
				mutatingCauseError.stack,
				mutatingCauseClone.cause.value,
				stringStackClone.stack,
				Object.hasOwn(objectStackClone, "stack"),
				objectStackClone.stack === undefined,
				stackGets,
				accessorStackClone.name,
				accessorStackClone.message,
			].join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	const want = "true,undefined,false,0,false,0,true,1,before,after,2,source stack,true,true,0,DataError,message"
	if got := value.String(); got != want {
		t.Fatalf("Error and DOMException clone semantics = %q, want %q", got, want)
	}
}
