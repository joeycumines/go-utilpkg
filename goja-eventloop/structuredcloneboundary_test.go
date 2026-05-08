package gojaeventloop

import "testing"

// TestStructuredClone_Function_Throws tests that cloning functions throws DataCloneError.
func TestStructuredClone_Function_Throws(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			try {
				structuredClone(function() {});
				return false;
			} catch (e) {
				return e instanceof DOMException &&
					e.name === "DataCloneError" &&
					e.code === DOMException.DATA_CLONE_ERR;
			}
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Fatalf("expected DOMException DataCloneError for function clone")
	}
}

// TestStructuredClone_ObjectWithFunction_Throws tests that nested functions fail the whole clone.
func TestStructuredClone_ObjectWithFunction_Throws(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var original = {
				name: "test",
				getValue: function() { return 42; }
			};
			try {
				structuredClone(original);
				return false;
			} catch (e) {
				return e instanceof DOMException && e.name === "DataCloneError";
			}
		})()
	`)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ToBoolean() != true {
		t.Errorf("object with function should throw DataCloneError")
	}
}

func TestStructuredClone_ErrorAndEdgeSemantics(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			var cause = { marker: 1 };
			var originalError = new TypeError("boom");
			originalError.cause = cause;
			originalError.code = "E_BOOM";
			originalError.self = originalError;
			var clonedError = structuredClone(originalError);
			var errorOK = clonedError instanceof TypeError &&
				clonedError !== originalError &&
				clonedError.name === "TypeError" &&
				clonedError.message === "boom" &&
				clonedError.code === undefined &&
				clonedError.cause !== cause &&
				clonedError.cause.marker === 1 &&
				clonedError.self === undefined &&
				Object.keys(clonedError).length === 0;

			var invalidDate = new Date(NaN);
			var clonedDate = structuredClone(invalidDate);
			var dateOK = clonedDate instanceof Date && Number.isNaN(clonedDate.getTime());

			var re = /a+/g;
			re.lastIndex = 2;
			var clonedRe = structuredClone(re);
			var regexpOK = clonedRe instanceof RegExp &&
				clonedRe.source === "a+" &&
				clonedRe.flags === "g" &&
				clonedRe.lastIndex === 0;

			var withUndefined = {};
			withUndefined.value = undefined;
			var clonedUndefined = structuredClone(withUndefined);
			var undefinedOK = Object.prototype.hasOwnProperty.call(clonedUndefined, "value") &&
				clonedUndefined.value === undefined;

			var symbolOK = false;
			try {
				structuredClone(Symbol("x"));
			} catch (e) {
				symbolOK = e instanceof DOMException && e.name === "DataCloneError";
			}

			var dupTransferOK = false;
			try {
				var duplicate = new ArrayBuffer(1);
				structuredClone(duplicate, { transfer: [duplicate, duplicate] });
			} catch (e) {
				dupTransferOK = e instanceof DOMException &&
					e.name === "DataCloneError" &&
					e.message === "Transfer list contains duplicate ArrayBuffer";
			}

			return errorOK && dateOK && regexpOK && undefinedOK && symbolOK && dupTransferOK;
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.ToBoolean() {
		t.Fatalf("error/date/regexp/DataCloneError structuredClone checks failed")
	}
}

func TestStructuredClone_BuiltinExpandosExcluded(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	_, err := adapter.runtime.RunString(`
		const values = [
			new Date(1),
			/a/g,
			new Map([["k", "v"]]),
			new Set([1]),
			new Uint8Array([1, 2]),
			new DataView(new ArrayBuffer(2)),
		];
		for (const value of values) {
			value.expando = 1;
			const cloned = structuredClone(value);
			if (cloned.expando !== undefined || Object.keys(cloned).includes("expando")) {
				throw new Error(value.constructor.name + " expando was cloned");
			}
		}
	`)
	if err != nil {
		t.Fatalf("structuredClone built-in expando exclusion: %v", err)
	}
}

func TestStructuredClone_CapturedMapSetMutators(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	_, err := adapter.runtime.RunString(`
		const sourceMap = new Map([["key", { value: 1 }]]);
		const sourceSet = new Set([{ value: 2 }]);
		Map.prototype.set = function poisonedSet() { throw new Error("poisoned Map.set"); };
		Set.prototype.add = function poisonedAdd() { throw new Error("poisoned Set.add"); };
		const clonedMap = structuredClone(sourceMap);
		const clonedSet = structuredClone(sourceSet);
		const mapValue = [...clonedMap.values()][0];
		const setValue = [...clonedSet.values()][0];
		if (clonedMap.size !== 1 || mapValue.value !== 1 || mapValue === sourceMap.get("key")) {
			throw new Error("Map clone after mutator monkeypatch");
		}
		if (clonedSet.size !== 1 || setValue.value !== 2 || setValue === [...sourceSet][0]) {
			throw new Error("Set clone after mutator monkeypatch");
		}
	`)
	if err != nil {
		t.Fatalf("structuredClone captured Map/Set mutators: %v", err)
	}
}

func TestStructuredClone_Node26BrandAndProxyBoundaries(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			function cloneErrorName(value) {
				try { structuredClone(value); return "ok"; }
				catch (err) { return err.name; }
			}
			function plainCloneOK(value, ctor, marker) {
				value.marker = marker;
				var cloned = structuredClone(value);
				return cloned !== value && cloned.marker === marker && !(cloned instanceof ctor);
			}

			var spoof = { a: 1, constructor: { name: "WeakMap" } };
			var spoofClone = structuredClone(spoof);

			var originalObjectKeys = Object.keys;
			Object.keys = function() { throw new Error("Object.keys should not be consulted"); };
			var keysClone;
			try { keysClone = structuredClone({ b: 2 }); }
			finally { Object.keys = originalObjectKeys; }

			var fakeDateOK = plainCloneOK(Object.create(Date.prototype), Date, 10);
			var fakeMapOK = plainCloneOK(Object.create(Map.prototype), Map, 11);
			var fakeSetOK = plainCloneOK(Object.create(Set.prototype), Set, 12);
			var fakeDataViewOK = plainCloneOK(Object.create(DataView.prototype), DataView, 13);
			var fakeTypedArrayOK = plainCloneOK(Object.create(Uint8Array.prototype), Uint8Array, 14);
			var fakeErrorOK = plainCloneOK(Object.create(Error.prototype), Error, 15);

			var realDate = new Date(123);
			Object.setPrototypeOf(realDate, null);
			var clonedDate = structuredClone(realDate);
			var dateNullProtoOK = clonedDate instanceof Date && clonedDate.getTime() === 123;

			var realRegExp = /x+/gi;
			Object.setPrototypeOf(realRegExp, null);
			var clonedRegExp = structuredClone(realRegExp);
			var regexpNullProtoOK = clonedRegExp instanceof RegExp && clonedRegExp.source === "x+" && clonedRegExp.flags === "gi";

			var realMapValue = { nested: true };
			var realMap = new Map([["k", realMapValue]]);
			Object.setPrototypeOf(realMap, null);
			var clonedMap = structuredClone(realMap);
			var mapNullProtoOK = clonedMap instanceof Map && clonedMap.get("k") !== realMapValue && clonedMap.get("k").nested === true;

			var realSetValue = { nested: true };
			var realSet = new Set([realSetValue]);
			Object.setPrototypeOf(realSet, null);
			var clonedSet = structuredClone(realSet);
			var clonedSetValue = Array.from(clonedSet)[0];
			var setNullProtoOK = clonedSet instanceof Set && clonedSetValue !== realSetValue && clonedSetValue.nested === true;

			var realArray = [1, 2, 3];
			Object.setPrototypeOf(realArray, null);
			var clonedArray = structuredClone(realArray);
			var arrayNullProtoOK = Array.isArray(clonedArray) && clonedArray.length === 3 && clonedArray[2] === 3;

			var buffer = new ArrayBuffer(6);
			new Uint8Array(buffer).set([1, 2, 3, 4, 5, 6]);
			var realTypedArray = new Uint8Array(buffer, 1, 3);
			Object.setPrototypeOf(realTypedArray, null);
			var clonedTypedArray = structuredClone(realTypedArray);
			var typedArrayNullProtoOK = clonedTypedArray instanceof Uint8Array && clonedTypedArray.byteOffset === 1 && clonedTypedArray.length === 3 && clonedTypedArray[0] === 2;

			var realDataView = new DataView(buffer, 2, 2);
			Object.setPrototypeOf(realDataView, null);
			var clonedDataView = structuredClone(realDataView);
			var dataViewNullProtoOK = clonedDataView instanceof DataView && clonedDataView.byteOffset === 2 && clonedDataView.byteLength === 2 && clonedDataView.getUint8(0) === 3;

			var realError = new Error("boom");
			realError.extra = { nested: true };
			Object.setPrototypeOf(realError, null);
			var clonedError = structuredClone(realError);
			var errorNullProtoOK = clonedError instanceof Error && clonedError.message === "boom" && clonedError.extra === undefined;

			var realWeakMap = new WeakMap();
			Object.setPrototypeOf(realWeakMap, null);
			var realWeakSet = new WeakSet();
			Object.setPrototypeOf(realWeakSet, null);
			var realPromise = Promise.resolve(1);
			Object.setPrototypeOf(realPromise, null);

			return [
				cloneErrorName(new Proxy({ a: 1 }, {})),
				cloneErrorName(new WeakMap()),
				cloneErrorName(new WeakSet()),
				cloneErrorName(Promise.resolve(1)),
				cloneErrorName(realWeakMap),
				cloneErrorName(realWeakSet),
				cloneErrorName(realPromise),
				spoofClone !== spoof && spoofClone.a === 1 && spoofClone.constructor.name === "WeakMap",
				keysClone.b === 2,
				fakeDateOK,
				fakeMapOK,
				fakeSetOK,
				fakeDataViewOK,
				fakeTypedArrayOK,
				fakeErrorOK,
				dateNullProtoOK,
				regexpNullProtoOK,
				mapNullProtoOK,
				setNullProtoOK,
				arrayNullProtoOK,
				typedArrayNullProtoOK,
				dataViewNullProtoOK,
				errorNullProtoOK
			].join(",");
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "DataCloneError,DataCloneError,DataCloneError,DataCloneError,DataCloneError,DataCloneError,DataCloneError,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true,true"
	if got := result.String(); got != want {
		t.Fatalf("structuredClone brand/proxy boundaries = %q, want %q", got, want)
	}
}

func TestStructuredClone_ImmutableConstructorsAndTransferGetterTiming(t *testing.T) {
	adapter, cleanup := testSetup(t)
	defer cleanup()

	result, err := adapter.runtime.RunString(`
		(function() {
			const OriginalDataView = DataView;
			const OriginalUint8Array = Uint8Array;
			const OriginalTypeError = TypeError;
			const wrappers = [Object(true), Object(2), Object("three"), Object(4n)];
			const original = {
				date: new Date(5),
				re: /abc/gi,
				map: new Map([["x", { y: 1 }]]),
				set: new Set([{ z: 2 }]),
				view: new DataView(new ArrayBuffer(2)),
				u8: new Uint8Array([3, 4]),
				err: new TypeError("typed"),
			};
			const mutableConstructorError = new Error("mutable constructor used");
			for (const name of ["Object", "Date", "RegExp", "Map", "Set", "DataView", "Uint8Array", "TypeError", "Error"]) {
				globalThis[name] = function BrokenConstructor() { throw mutableConstructorError; };
			}
			const cloned = structuredClone(original);
			const clonedWrappers = wrappers.map((wrapper) => structuredClone(wrapper));
			const constructorsOK = cloned.date.getTime() === 5 &&
				cloned.re.source === "abc" && cloned.re.flags === "gi" &&
				cloned.map.get("x") !== original.map.get("x") && cloned.map.get("x").y === 1 &&
				Array.from(cloned.set)[0] !== Array.from(original.set)[0] && Array.from(cloned.set)[0].z === 2 &&
				cloned.view instanceof OriginalDataView && cloned.view.byteLength === 2 &&
				cloned.u8 instanceof OriginalUint8Array && cloned.u8[0] === 3 && cloned.u8[1] === 4 &&
				cloned.err instanceof OriginalTypeError && cloned.err.message === "typed" &&
				clonedWrappers[0].valueOf() === true &&
				clonedWrappers[1].valueOf() === 2 &&
				clonedWrappers[2].valueOf() === "three" &&
				clonedWrappers[3].valueOf() === 4n;

			const getterFirstBuffer = new ArrayBuffer(1);
			const getterFirstBytes = new OriginalUint8Array(getterFirstBuffer);
			getterFirstBytes[0] = 1;
			const getterFirst = structuredClone({
				get mutate() { getterFirstBytes[0] = 2; return true; },
				buffer: getterFirstBuffer,
			}, { transfer: [getterFirstBuffer] });

			const bufferFirstBuffer = new ArrayBuffer(1);
			const bufferFirstBytes = new OriginalUint8Array(bufferFirstBuffer);
			bufferFirstBytes[0] = 3;
			const bufferFirst = structuredClone({
				buffer: bufferFirstBuffer,
				get mutate() { bufferFirstBytes[0] = 4; return true; },
			}, { transfer: [bufferFirstBuffer] });

			const copyFirstBuffer = new ArrayBuffer(1);
			const copyFirstBytes = new OriginalUint8Array(copyFirstBuffer);
			copyFirstBytes[0] = 8;
			const copyFirst = structuredClone({
				buffer: copyFirstBuffer,
				get mutate() { copyFirstBytes[0] = 9; return true; },
			});

			const viewFirstBuffer = new ArrayBuffer(2);
			const viewFirstBytes = new OriginalUint8Array(viewFirstBuffer);
			viewFirstBytes.set([5, 6]);
			const viewFirst = structuredClone({
				view: viewFirstBytes,
				buffer: viewFirstBuffer,
				get mutate() { viewFirstBytes[0] = 7; return true; },
			}, { transfer: [viewFirstBuffer] });

			const failedBuffer = new ArrayBuffer(1);
			try {
				structuredClone({ buffer: failedBuffer, bad: function () {} }, { transfer: [failedBuffer] });
			} catch (error) {
				if (error.name !== "DataCloneError") throw error;
			}

			const transferOK =
				getterFirstBuffer.byteLength === 0 &&
				new OriginalUint8Array(getterFirst.buffer)[0] === 2 &&
				bufferFirstBuffer.byteLength === 0 &&
				new OriginalUint8Array(bufferFirst.buffer)[0] === 4 &&
				copyFirstBuffer.byteLength === 1 &&
				copyFirstBytes[0] === 9 &&
				new OriginalUint8Array(copyFirst.buffer)[0] === 8 &&
				viewFirstBuffer.byteLength === 0 &&
				viewFirst.view.buffer === viewFirst.buffer &&
				viewFirst.view[0] === 7 &&
				viewFirst.view[1] === 6 &&
				failedBuffer.byteLength === 1;
			return constructorsOK + ":" + transferOK;
		})()
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := result.String(), "true:true"; got != want {
		t.Fatalf("structuredClone mutable constructors / transfer timing = %q, want %q", got, want)
	}
}
