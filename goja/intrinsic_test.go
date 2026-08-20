package goja

import "testing"

func TestRuntimeIntrinsicConstructorIdentity(t *testing.T) {
	runtime := New()
	tests := []struct {
		name                string
		id                  Intrinsic
		args                func() []Value
		constructionRejects bool
	}{
		{name: "Date", id: IntrinsicDateConstructor},
		{name: "RegExp", id: IntrinsicRegExpConstructor},
		{name: "Map", id: IntrinsicMapConstructor},
		{name: "Set", id: IntrinsicSetConstructor},
		{
			name: "DataView",
			id:   IntrinsicDataViewConstructor,
			args: func() []Value {
				return []Value{runtime.ToValue(runtime.NewArrayBuffer(nil))}
			},
		},
		{name: "Error", id: IntrinsicErrorConstructor},
		{name: "EvalError", id: IntrinsicEvalErrorConstructor},
		{name: "RangeError", id: IntrinsicRangeErrorConstructor},
		{name: "ReferenceError", id: IntrinsicReferenceErrorConstructor},
		{name: "SyntaxError", id: IntrinsicSyntaxErrorConstructor},
		{name: "TypeError", id: IntrinsicTypeErrorConstructor},
		{name: "URIError", id: IntrinsicURIErrorConstructor},
		{name: "Int8Array", id: IntrinsicInt8ArrayConstructor},
		{name: "Uint8Array", id: IntrinsicUint8ArrayConstructor},
		{name: "Uint8ClampedArray", id: IntrinsicUint8ClampedArrayConstructor},
		{name: "Int16Array", id: IntrinsicInt16ArrayConstructor},
		{name: "Uint16Array", id: IntrinsicUint16ArrayConstructor},
		{name: "Int32Array", id: IntrinsicInt32ArrayConstructor},
		{name: "Uint32Array", id: IntrinsicUint32ArrayConstructor},
		{name: "Float32Array", id: IntrinsicFloat32ArrayConstructor},
		{name: "Float64Array", id: IntrinsicFloat64ArrayConstructor},
		{name: "BigInt64Array", id: IntrinsicBigInt64ArrayConstructor},
		{name: "BigUint64Array", id: IntrinsicBigUint64ArrayConstructor},
		{
			name: "Promise",
			id:   IntrinsicPromiseConstructor,
			args: func() []Value {
				return []Value{runtime.ToValue(func(FunctionCall) Value { return Undefined() })}
			},
		},
		{name: "Symbol", id: IntrinsicSymbolConstructor, constructionRejects: true},
		{
			name: "AggregateError",
			id:   IntrinsicAggregateErrorConstructor,
			args: func() []Value {
				return []Value{runtime.NewArray()}
			},
		},
		{name: "WeakMap", id: IntrinsicWeakMapConstructor},
		{name: "WeakSet", id: IntrinsicWeakSetConstructor},
		{name: "String", id: IntrinsicStringConstructor},
	}

	type constructorState struct {
		constructor *Object
		prototype   *Object
	}
	states := make(map[string]constructorState, len(tests))
	for _, test := range tests {
		constructor := runtime.Get(test.name).ToObject(runtime)
		prototype, ok := constructor.Get("prototype").(*Object)
		if !ok || prototype == nil {
			t.Fatalf("%s prototype is unavailable", test.name)
		}
		states[test.name] = constructorState{constructor: constructor, prototype: prototype}
		if err := runtime.Set(test.name, runtime.NewObject()); err != nil {
			t.Fatal(err)
		}
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first, ok := runtime.Intrinsic(test.id)
			if !ok {
				t.Fatal("Intrinsic returned false")
			}
			second, ok := runtime.Intrinsic(test.id)
			if !ok || !first.SameAs(second) {
				t.Fatal("Intrinsic did not retain stable value identity")
			}
			if !first.SameAs(states[test.name].constructor) {
				t.Fatal("intrinsic differs from the original realm constructor")
			}
			if _, ok := AssertConstructor(first); !ok {
				t.Fatal("intrinsic is not a constructor")
			}
			var args []Value
			if test.args != nil {
				args = test.args()
			}
			if test.constructionRejects {
				// ECMAScript spec-non-constructable callables such as Symbol are
				// callable but reject `new`. Assert that construction rejects,
				// without asserting constructability either way: goja registers
				// these with a construct function that throws, so
				// AssertConstructor may report true even though `new` throws.
				if _, err := runtime.New(first, args...); err == nil {
					t.Fatal("New unexpectedly succeeded for non-constructable intrinsic")
				}
				return
			}
			if _, ok := AssertConstructor(first); !ok {
				t.Fatal("intrinsic is not a constructor")
			}
			object, err := runtime.New(first, args...)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if object.Prototype() != states[test.name].prototype {
				t.Fatal("intrinsic constructor used a replaced prototype")
			}
		})
	}
}

func TestRuntimeIntrinsicMatchesRealmProperties(t *testing.T) {
	runtime := New()
	tests := []struct {
		expression string
		id         Intrinsic
	}{
		{expression: `Date.prototype.getTime`, id: IntrinsicDateGetTime},
		{expression: `Object.getOwnPropertyDescriptor(RegExp.prototype, "source").get`, id: IntrinsicRegExpSourceGetter},
		{expression: `Object.getOwnPropertyDescriptor(RegExp.prototype, "global").get`, id: IntrinsicRegExpGlobalGetter},
		{expression: `Object.getOwnPropertyDescriptor(RegExp.prototype, "ignoreCase").get`, id: IntrinsicRegExpIgnoreCaseGetter},
		{expression: `Object.getOwnPropertyDescriptor(RegExp.prototype, "multiline").get`, id: IntrinsicRegExpMultilineGetter},
		{expression: `Object.getOwnPropertyDescriptor(RegExp.prototype, "dotAll").get`, id: IntrinsicRegExpDotAllGetter},
		{expression: `Object.getOwnPropertyDescriptor(RegExp.prototype, "unicode").get`, id: IntrinsicRegExpUnicodeGetter},
		{expression: `Object.getOwnPropertyDescriptor(RegExp.prototype, "sticky").get`, id: IntrinsicRegExpStickyGetter},
		{expression: `Object.getOwnPropertyDescriptor(DataView.prototype, "buffer").get`, id: IntrinsicDataViewBufferGetter},
		{expression: `Object.getOwnPropertyDescriptor(DataView.prototype, "byteOffset").get`, id: IntrinsicDataViewByteOffsetGetter},
		{expression: `Object.getOwnPropertyDescriptor(DataView.prototype, "byteLength").get`, id: IntrinsicDataViewByteLengthGetter},
		{expression: `Object.getOwnPropertyDescriptor(Object.getPrototypeOf(Uint8Array.prototype), "buffer").get`, id: IntrinsicTypedArrayBufferGetter},
		{expression: `Object.getOwnPropertyDescriptor(Object.getPrototypeOf(Uint8Array.prototype), "byteOffset").get`, id: IntrinsicTypedArrayByteOffsetGetter},
		{expression: `Object.getOwnPropertyDescriptor(Object.getPrototypeOf(Uint8Array.prototype), "length").get`, id: IntrinsicTypedArrayLengthGetter},
		{expression: `Object.getOwnPropertyDescriptor(Object.getPrototypeOf(Uint8Array.prototype), Symbol.toStringTag).get`, id: IntrinsicTypedArrayNameGetter},
		{expression: `Object.getOwnPropertyDescriptor(Map.prototype, "size").get`, id: IntrinsicMapSizeGetter},
		{expression: `Map.prototype.forEach`, id: IntrinsicMapForEach},
		{expression: `Map.prototype.set`, id: IntrinsicMapSet},
		{expression: `Object.getOwnPropertyDescriptor(Set.prototype, "size").get`, id: IntrinsicSetSizeGetter},
		{expression: `Set.prototype.forEach`, id: IntrinsicSetForEach},
		{expression: `Set.prototype.add`, id: IntrinsicSetAdd},
		{expression: `WeakMap.prototype.has`, id: IntrinsicWeakMapHas},
		{expression: `WeakMap.prototype.get`, id: IntrinsicWeakMapGet},
		{expression: `WeakMap.prototype.set`, id: IntrinsicWeakMapSet},
		{expression: `WeakSet.prototype.add`, id: IntrinsicWeakSetAdd},
		{expression: `WeakSet.prototype.has`, id: IntrinsicWeakSetHas},
		{expression: `WeakSet.prototype.delete`, id: IntrinsicWeakSetDelete},
		{expression: `Boolean.prototype.valueOf`, id: IntrinsicBooleanValueOf},
		{expression: `Number.prototype.valueOf`, id: IntrinsicNumberValueOf},
		{expression: `BigInt.prototype.valueOf`, id: IntrinsicBigIntValueOf},
		{expression: `String.prototype.valueOf`, id: IntrinsicStringValueOf},
		{expression: `Symbol.prototype.valueOf`, id: IntrinsicSymbolValueOf},
		{expression: `Symbol.prototype.toString`, id: IntrinsicSymbolToString},
		{expression: `Reflect.apply`, id: IntrinsicReflectApply},
		{expression: `Reflect.construct`, id: IntrinsicReflectConstruct},
		{expression: `Reflect.deleteProperty`, id: IntrinsicReflectDeleteProperty},
		{expression: `Object.create`, id: IntrinsicObjectCreate},
		{expression: `Object.defineProperty`, id: IntrinsicObjectDefineProperty},
		{expression: `Object.getOwnPropertyDescriptor`, id: IntrinsicObjectGetOwnPropertyDescriptor},
		{expression: `Object.getOwnPropertyDescriptors`, id: IntrinsicObjectGetOwnPropertyDescriptors},
		{expression: `Object.getPrototypeOf`, id: IntrinsicObjectGetPrototypeOf},
		{expression: `Object.setPrototypeOf`, id: IntrinsicObjectSetPrototypeOf},
		{expression: `Function.prototype.bind`, id: IntrinsicFunctionBind},
		{expression: `Function.prototype.toString`, id: IntrinsicFunctionToString},
		{expression: `Array.prototype.indexOf`, id: IntrinsicArrayIndexOf},
		{expression: `Array.prototype.join`, id: IntrinsicArrayJoin},
		{expression: `Array.prototype.slice`, id: IntrinsicArraySlice},
		{expression: `Array.prototype.splice`, id: IntrinsicArraySplice},
		{expression: `Math.min`, id: IntrinsicMathMin},
		{expression: `Math.max`, id: IntrinsicMathMax},
		{expression: `Math.trunc`, id: IntrinsicMathTrunc},
		{expression: `String.prototype.split`, id: IntrinsicStringSplit},
		{expression: `Error.prototype`, id: IntrinsicErrorPrototype},
	}
	for _, test := range tests {
		t.Run(test.expression, func(t *testing.T) {
			property, err := runtime.RunString(test.expression)
			if err != nil {
				t.Fatal(err)
			}
			intrinsic, ok := runtime.Intrinsic(test.id)
			if !ok || !intrinsic.SameAs(property) {
				t.Fatalf("Intrinsic(%d) differs from realm property", test.id)
			}
		})
	}
}

func TestRuntimeIntrinsicDoesNotReadGlobalBindings(t *testing.T) {
	runtime := New()
	_, err := runtime.RunString(`
		globalThis.intrinsicGlobalReads = 0;
		globalThis.intrinsicGlobalSentinel = {};
		for (const name of [
			"Date", "RegExp", "Map", "Set", "DataView",
			"Error", "EvalError", "RangeError", "ReferenceError", "SyntaxError", "TypeError", "URIError",
			"Int8Array", "Uint8Array", "Uint8ClampedArray", "Int16Array", "Uint16Array",
			"Int32Array", "Uint32Array", "Float32Array", "Float64Array", "BigInt64Array", "BigUint64Array",
			"Promise", "Symbol", "AggregateError", "WeakMap", "WeakSet", "String",
		]) {
			Object.defineProperty(globalThis, name, {
				configurable: true,
				get() {
					intrinsicGlobalReads++;
					throw intrinsicGlobalSentinel;
				},
			});
		}
	`)
	if err != nil {
		t.Fatal(err)
	}

	for id := IntrinsicDateConstructor; id <= IntrinsicStringConstructor; id++ {
		if value, ok := runtime.Intrinsic(id); !ok || value == nil {
			t.Fatalf("Intrinsic(%d) = %v, %t", id, value, ok)
		}
	}
	if got := runtime.Get("intrinsicGlobalReads").ToInteger(); got != 0 {
		t.Fatalf("global getters ran %d times", got)
	}
}

func TestRuntimeIntrinsicErrorPrototypeDoesNotReadGlobalError(t *testing.T) {
	runtime := New()
	errorConstructor := runtime.Get("Error").ToObject(runtime)
	want := errorConstructor.Get("prototype")
	if _, cached := runtime.intrinsics[IntrinsicErrorPrototype]; cached {
		t.Fatal("Error prototype intrinsic was cached before the lookup under test")
	}
	reads := 0
	sentinel := runtime.NewObject()
	getter := runtime.ToValue(func(FunctionCall) Value {
		reads++
		panic(sentinel)
	})
	if err := runtime.GlobalObject().DefineAccessorProperty(
		"Error",
		getter,
		nil,
		FLAG_FALSE,
		FLAG_TRUE,
	); err != nil {
		t.Fatal(err)
	}

	got, ok := runtime.Intrinsic(IntrinsicErrorPrototype)
	if !ok || !got.SameAs(want) {
		t.Fatalf("IntrinsicErrorPrototype = %v, %t; want original realm prototype", got, ok)
	}
	again, ok := runtime.Intrinsic(IntrinsicErrorPrototype)
	if !ok || !again.SameAs(got) {
		t.Fatal("IntrinsicErrorPrototype did not retain stable realm identity")
	}
	if reads != 0 {
		t.Fatalf("global Error getter ran %d times", reads)
	}
}

// TestRuntimeIntrinsicErrorPrototypeStableUnderMutation verifies that
// IntrinsicErrorPrototype returns the original realm Error.prototype identity
// even after JavaScript mutates Error.prototype's own properties and attempts
// to reassign Error.prototype (which is non-writable per spec).
func TestRuntimeIntrinsicErrorPrototypeStableUnderMutation(t *testing.T) {
	runtime := New()
	first, ok := runtime.Intrinsic(IntrinsicErrorPrototype)
	if !ok {
		t.Fatal("IntrinsicErrorPrototype returned false")
	}
	realmProto := runtime.Get("Error").ToObject(runtime).Get("prototype")
	if !first.SameAs(realmProto) {
		t.Fatal("IntrinsicErrorPrototype differs from realm Error.prototype")
	}
	// Error.prototype is non-writable, non-configurable, non-enumerable per spec.
	errorCtor := runtime.Get("Error").ToObject(runtime)
	protoDesc, hasProtoDesc := errorCtor.OwnPropertyDescriptor("prototype")
	if !hasProtoDesc {
		t.Fatal("Error.prototype property missing on Error constructor")
	}
	if protoDesc.Writable != FLAG_FALSE {
		t.Fatalf("Error.prototype is writable: %+v", protoDesc)
	}
	// Mutate Error.prototype's own state.
	if _, err := runtime.RunString(`
		Error.prototype.extra = 1;
		Error.prototype = {};
	`); err != nil {
		t.Fatal(err)
	}
	// The reassignment must not have replaced the prototype (non-writable).
	after := errorCtor.Get("prototype")
	if !after.SameAs(realmProto) {
		t.Fatal("Error.prototype identity changed after reassignment attempt")
	}
	// The intrinsic lookup must still return the same canonical identity.
	again, ok := runtime.Intrinsic(IntrinsicErrorPrototype)
	if !ok || !again.SameAs(first) {
		t.Fatal("IntrinsicErrorPrototype changed identity after Error.prototype mutation")
	}
	// The mutation to the object's own state is visible through the intrinsic,
	// demonstrating that the intrinsic is canonical identity, not a frozen copy.
	if got := first.ToObject(runtime).Get("extra").ToInteger(); got != 1 {
		t.Fatalf("Error.prototype.extra via intrinsic = %d, want 1", got)
	}
}

// TestRuntimeIntrinsicMethodSharedIdentity demonstrates that an intrinsic
// method object is the same identity as the realm prototype property, so
// JavaScript-added own properties on the function object are visible on the
// intrinsic. This documents the not-frozen hardening boundary: the lookup is
// protected, but the intrinsic object's own state is mutable by JS holding a
// reference.
func TestRuntimeIntrinsicMethodSharedIdentity(t *testing.T) {
	runtime := New()
	sliceIntrinsic, ok := runtime.Intrinsic(IntrinsicArraySlice)
	if !ok {
		t.Fatal("IntrinsicArraySlice returned false")
	}
	realmSlice := runtime.Get("Array").ToObject(runtime).Get("prototype").ToObject(runtime).Get("slice")
	if !sliceIntrinsic.SameAs(realmSlice) {
		t.Fatal("IntrinsicArraySlice differs from Array.prototype.slice")
	}
	if _, err := runtime.RunString(`Array.prototype.slice.marker = "polluted";`); err != nil {
		t.Fatal(err)
	}
	if got := sliceIntrinsic.ToObject(runtime).Get("marker").String(); got != "polluted" {
		t.Fatalf("intrinsic slice marker = %q, want %q", got, "polluted")
	}
	// A subsequent intrinsic lookup returns the same (mutated) identity.
	again, ok := runtime.Intrinsic(IntrinsicArraySlice)
	if !ok || !again.SameAs(sliceIntrinsic) {
		t.Fatal("IntrinsicArraySlice identity changed after mutation")
	}
}

func TestRuntimeIntrinsicDoesNotReadPrototypeProperties(t *testing.T) {
	runtime := New()
	_, err := runtime.RunString(`
		globalThis.intrinsicPrototypeCalls = 0;
		globalThis.intrinsicPrototypeSentinel = {};
		const defineProperty = Object.defineProperty;
		const poison = {
			get() {
				intrinsicPrototypeCalls++;
				throw intrinsicPrototypeSentinel;
			},
			configurable: true,
		};
		const method = function () {
			intrinsicPrototypeCalls++;
			throw intrinsicPrototypeSentinel;
		};
		for (const [prototype, names] of [
			[Date.prototype, ["getTime"]],
			[RegExp.prototype, ["source", "global", "ignoreCase", "multiline", "dotAll", "unicode", "sticky"]],
			[DataView.prototype, ["buffer", "byteOffset", "byteLength"]],
			[Object.getPrototypeOf(Uint8Array.prototype), ["buffer", "byteOffset", "length", Symbol.toStringTag]],
			[Map.prototype, ["size"]],
			[Set.prototype, ["size"]],
		]) {
			for (const name of names) defineProperty(prototype, name, poison);
		}
		for (const [prototype, names] of [
			[Map.prototype, ["forEach", "set"]],
			[Set.prototype, ["forEach", "add"]],
			[WeakMap.prototype, ["get", "set", "has"]],
			[WeakSet.prototype, ["add", "has", "delete"]],
			[Boolean.prototype, ["valueOf"]],
			[Number.prototype, ["valueOf"]],
			[BigInt.prototype, ["valueOf"]],
			[String.prototype, ["valueOf", "split"]],
			[Symbol.prototype, ["valueOf", "toString"]],
			[Reflect, ["apply", "construct", "deleteProperty"]],
			[Object, ["create", "defineProperty", "getOwnPropertyDescriptor", "getOwnPropertyDescriptors", "getPrototypeOf", "setPrototypeOf"]],
			[Function.prototype, ["bind", "toString"]],
			[Array.prototype, ["indexOf", "join", "slice", "splice"]],
			[Math, ["min", "max", "trunc"]],
		]) {
			for (const name of names) defineProperty(prototype, name, {
				value: method,
				configurable: true,
				writable: true,
			});
		}
	`)
	if err != nil {
		t.Fatal(err)
	}

	for id := IntrinsicDateGetTime; id <= IntrinsicStringSplit; id++ {
		value, ok := runtime.Intrinsic(id)
		if !ok {
			t.Fatalf("Intrinsic(%d) returned false", id)
		}
		if _, ok := AssertFunction(value); !ok {
			t.Fatalf("Intrinsic(%d) is not callable", id)
		}
		again, ok := runtime.Intrinsic(id)
		if !ok || !value.SameAs(again) {
			t.Fatalf("Intrinsic(%d) did not retain stable value identity", id)
		}
	}
	if got := runtime.Get("intrinsicPrototypeCalls").ToInteger(); got != 0 {
		t.Fatalf("poisoned prototype functions ran %d times", got)
	}
}

func TestRuntimeIntrinsicInvalid(t *testing.T) {
	var nilRuntime *Runtime
	if value, ok := nilRuntime.Intrinsic(IntrinsicDateConstructor); ok || value != nil {
		t.Fatalf("nil Runtime Intrinsic = %v, %t", value, ok)
	}
	if value, ok := New().Intrinsic(0); ok || value != nil {
		t.Fatalf("unknown Intrinsic = %v, %t", value, ok)
	}
	if value, ok := New().Intrinsic(Intrinsic(^uint16(0))); ok || value != nil {
		t.Fatalf("out-of-range Intrinsic = %v, %t", value, ok)
	}
}

func TestRuntimeIntrinsicRealmIsolationAndMutation(t *testing.T) {
	firstRuntime := New()
	secondRuntime := New()
	first := make(map[Intrinsic]Value)
	for id := IntrinsicDateConstructor; id <= IntrinsicStringSplit; id++ {
		value, ok := firstRuntime.Intrinsic(id)
		if !ok {
			t.Fatalf("first Intrinsic(%d) returned false", id)
		}
		other, ok := secondRuntime.Intrinsic(id)
		if !ok {
			t.Fatalf("second Intrinsic(%d) returned false", id)
		}
		if value.SameAs(other) {
			t.Fatalf("Intrinsic(%d) leaked identity between runtimes", id)
		}
		first[id] = value
	}

	_, err := firstRuntime.RunString(`
		for (const [prototype, names] of [
			[Date.prototype, ["getTime"]],
			[RegExp.prototype, ["source", "global", "ignoreCase", "multiline", "dotAll", "unicode", "sticky"]],
			[DataView.prototype, ["buffer", "byteOffset", "byteLength"]],
			[Object.getPrototypeOf(Uint8Array.prototype), ["buffer", "byteOffset", "length", Symbol.toStringTag]],
			[Map.prototype, ["size", "forEach", "set"]],
			[Set.prototype, ["size", "forEach", "add"]],
			[WeakMap.prototype, ["get", "set", "has"]],
			[WeakSet.prototype, ["add", "has", "delete"]],
			[Boolean.prototype, ["valueOf"]],
			[Number.prototype, ["valueOf"]],
			[BigInt.prototype, ["valueOf"]],
			[String.prototype, ["valueOf", "split"]],
			[Symbol.prototype, ["valueOf", "toString"]],
			[Reflect, ["apply", "construct", "deleteProperty"]],
			[Object, ["create", "defineProperty", "getOwnPropertyDescriptor", "getOwnPropertyDescriptors", "getPrototypeOf", "setPrototypeOf"]],
			[Function.prototype, ["bind", "toString"]],
			[Array.prototype, ["indexOf", "join", "slice", "splice"]],
			[Math, ["min", "max", "trunc"]],
		]) {
			for (const name of names) delete prototype[name];
		}
		for (const name of [
			"Date", "RegExp", "Map", "Set", "DataView",
			"Error", "EvalError", "RangeError", "ReferenceError", "SyntaxError", "TypeError", "URIError",
			"Int8Array", "Uint8Array", "Uint8ClampedArray", "Int16Array", "Uint16Array",
			"Int32Array", "Uint32Array", "Float32Array", "Float64Array", "BigInt64Array", "BigUint64Array",
			"Promise", "Symbol", "AggregateError", "WeakMap", "WeakSet", "String",
		]) {
			delete globalThis[name];
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range first {
		got, ok := firstRuntime.Intrinsic(id)
		if !ok || !got.SameAs(want) {
			t.Fatalf("Intrinsic(%d) changed after mutation", id)
		}
		other, ok := secondRuntime.Intrinsic(id)
		if !ok || got.SameAs(other) {
			t.Fatalf("Intrinsic(%d) contaminated another runtime", id)
		}
	}

	date, err := firstRuntime.New(first[IntrinsicDateConstructor], firstRuntime.ToValue(42))
	if err != nil {
		t.Fatal(err)
	}
	getTime, _ := AssertFunction(first[IntrinsicDateGetTime])
	result, err := getTime(date)
	if err != nil || result.ToInteger() != 42 {
		t.Fatalf("Date intrinsic call = %v, %v", result, err)
	}
	mapObject, err := firstRuntime.New(first[IntrinsicMapConstructor])
	if err != nil {
		t.Fatal(err)
	}
	mapSet, _ := AssertFunction(first[IntrinsicMapSet])
	if _, err := mapSet(mapObject, firstRuntime.ToValue("key"), firstRuntime.ToValue("value")); err != nil {
		t.Fatalf("Map intrinsic call: %v", err)
	}
}

func TestObjectOwnPropertyDescriptor(t *testing.T) {
	runtime := New()
	getterCalls := 0
	getter := runtime.ToValue(func() Value {
		getterCalls++
		return Undefined()
	})
	object := runtime.NewObject()
	if err := object.DefineDataProperty("data", runtime.ToValue(42), FLAG_FALSE, FLAG_TRUE, FLAG_FALSE); err != nil {
		t.Fatal(err)
	}
	if err := object.DefineAccessorProperty("accessor", getter, nil, FLAG_FALSE, FLAG_TRUE); err != nil {
		t.Fatal(err)
	}

	data, ok := object.OwnPropertyDescriptor("data")
	if !ok || !data.IsData() || data.IsAccessor() || data.Value.ToInteger() != 42 ||
		data.Writable != FLAG_FALSE || data.Configurable != FLAG_TRUE || data.Enumerable != FLAG_FALSE {
		t.Fatalf("data descriptor = %+v, %t", data, ok)
	}
	accessor, ok := object.OwnPropertyDescriptor("accessor")
	if !ok || !accessor.IsAccessor() || accessor.IsData() || !accessor.Getter.SameAs(getter) ||
		!IsUndefined(accessor.Setter) || accessor.Configurable != FLAG_FALSE || accessor.Enumerable != FLAG_TRUE {
		t.Fatalf("accessor descriptor = %+v, %t", accessor, ok)
	}
	if getterCalls != 0 {
		t.Fatalf("ordinary getter ran %d times", getterCalls)
	}
	if err := object.DefineDataProperty("undefined", Undefined(), FLAG_TRUE, FLAG_FALSE, FLAG_TRUE); err != nil {
		t.Fatal(err)
	}
	undefined, ok := object.OwnPropertyDescriptor("undefined")
	if !ok || !undefined.IsData() || !IsUndefined(undefined.Value) ||
		undefined.Writable != FLAG_TRUE || undefined.Configurable != FLAG_FALSE || undefined.Enumerable != FLAG_TRUE {
		t.Fatalf("undefined descriptor = %+v, %t", undefined, ok)
	}
	if descriptor, ok := object.OwnPropertyDescriptor("missing"); ok || !descriptor.Empty() {
		t.Fatalf("missing descriptor = %+v, %t", descriptor, ok)
	}
	var nilObject *Object
	if descriptor, ok := nilObject.OwnPropertyDescriptor("data"); ok || !descriptor.Empty() {
		t.Fatalf("nil Object descriptor = %+v, %t", descriptor, ok)
	}
}

// TestObjectOwnPropertyDescriptorNilDataValue exercises the nil-value hardening
// in OwnPropertyDescriptor. An internal valueProperty with accessor==false and
// a nil value (which the ordinary defineOwnProperty path normalizes away, but
// which exotic producers could in principle produce) must surface as a data
// descriptor whose Value is undefined rather than a nil Go interface.
func TestObjectOwnPropertyDescriptorNilDataValue(t *testing.T) {
	runtime := New()
	object := runtime.NewObject()
	// Directly install a data property whose value is a nil Go interface, as an
	// exotic internal producer might. This bypasses defineOwnProperty
	// normalization to reach the OwnPropertyDescriptor data branch directly.
	if bo, ok := object.self.(*baseObject); ok {
		bo._put("genny", &valueProperty{
			writable:     true,
			configurable: true,
			enumerable:   true,
			// value left nil, accessor false
		})
	} else {
		t.Skipf("object self is %T, not *baseObject", object.self)
	}

	descriptor, ok := object.OwnPropertyDescriptor("genny")
	if !ok {
		t.Fatal("OwnPropertyDescriptor returned false for nil-valued data property")
	}
	if !descriptor.IsData() {
		t.Fatalf("nil-valued data property is not data: %+v", descriptor)
	}
	if descriptor.IsAccessor() {
		t.Fatalf("nil-valued data property is accessor: %+v", descriptor)
	}
	if descriptor.Value == nil {
		t.Fatal("descriptor.Value is a nil Go interface; expected _undefined")
	}
	if !IsUndefined(descriptor.Value) {
		t.Fatalf("descriptor.Value = %v, want undefined", descriptor.Value)
	}
	if !descriptor.Value.SameAs(Undefined()) {
		t.Fatal("descriptor.Value is not SameAs Undefined()")
	}
	// Calling a method on the Value must not panic with a nil-interface
	// dereference.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic using nil-normalized Value: %v", r)
			}
		}()
		if descriptor.Value.ToInteger() != 0 {
			t.Fatalf("undefined.ToInteger() = %d, want 0", descriptor.Value.ToInteger())
		}
	}()
}

func TestObjectOwnPropertyDescriptorIndexedObjects(t *testing.T) {
	runtime := New()
	array := runtime.NewArray("value")
	descriptor, ok := array.OwnPropertyDescriptor("0")
	if !ok || !descriptor.IsData() || descriptor.Value.String() != "value" ||
		descriptor.Writable != FLAG_TRUE || descriptor.Configurable != FLAG_TRUE || descriptor.Enumerable != FLAG_TRUE {
		t.Fatalf("array descriptor = %+v, %t", descriptor, ok)
	}
	typed, err := runtime.RunString(`new Uint8Array([7])`)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, ok = typed.ToObject(runtime).OwnPropertyDescriptor("0")
	if !ok || !descriptor.IsData() || descriptor.Value.ToInteger() != 7 ||
		descriptor.Writable != FLAG_TRUE || descriptor.Configurable != FLAG_TRUE || descriptor.Enumerable != FLAG_TRUE {
		t.Fatalf("typed-array descriptor = %+v, %t", descriptor, ok)
	}
	dynamic := runtime.NewDynamicArray(&testDynArray{r: runtime, a: []Value{runtime.ToValue(9)}})
	descriptor, ok = dynamic.OwnPropertyDescriptor("0")
	if !ok || !descriptor.IsData() {
		t.Fatalf("dynamic-array descriptor = %+v, %t", descriptor, ok)
	}
}

func TestObjectIsExtensible(t *testing.T) {
	runtime := New()
	object := runtime.NewObject()
	if !object.IsExtensible() {
		t.Fatal("new ordinary object is not extensible")
	}
	if err := runtime.Set("objectUnderTest", object); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.RunString(`Object.preventExtensions(globalThis.objectUnderTest)`); err != nil {
		t.Fatal(err)
	}
	if object.IsExtensible() {
		t.Fatal("non-extensible ordinary object reports extensible")
	}
	var nilObject *Object
	if nilObject.IsExtensible() {
		t.Fatal("nil Object reports extensible")
	}
}

func TestObjectIsExtensibleProxy(t *testing.T) {
	runtime := New()
	target := runtime.NewObject()
	if err := runtime.Set("objectUnderTest", target); err != nil {
		t.Fatal(err)
	}
	value, err := runtime.RunString(`
		globalThis.proxyTrapCalls = 0;
		new Proxy(objectUnderTest, {
			isExtensible(target) {
				proxyTrapCalls++;
				return Reflect.isExtensible(target);
			},
		});
	`)
	if err != nil {
		t.Fatal(err)
	}
	proxy := value.ToObject(runtime)
	if !proxy.IsExtensible() {
		t.Fatal("proxy reports non-extensible")
	}
	if got := runtime.Get("proxyTrapCalls").ToInteger(); got != 1 {
		t.Fatalf("isExtensible trap calls = %d, want 1", got)
	}
	thrown, err := runtime.RunString(`globalThis.isExtensibleThrown = {}; isExtensibleThrown`)
	if err != nil {
		t.Fatal(err)
	}
	throwing, err := runtime.RunString(`
		new Proxy({}, {
			isExtensible() {
				throw isExtensibleThrown;
			},
		});
	`)
	if err != nil {
		t.Fatal(err)
	}
	exception := runtime.Try(func() {
		throwing.ToObject(runtime).IsExtensible()
	})
	if exception == nil || !exception.Value().SameAs(thrown) {
		t.Fatalf("throwing proxy exception = %v, want exact sentinel", exception)
	}
}

func TestObjectOwnPropertyDescriptorProxyResults(t *testing.T) {
	runtime := New()
	_, err := runtime.RunString(`
		globalThis.proxyDescriptorGetter = function () {};
		globalThis.proxyDescriptorSetter = function (_) {};
	`)
	if err != nil {
		t.Fatal(err)
	}
	getter := runtime.Get("proxyDescriptorGetter")
	setter := runtime.Get("proxyDescriptorSetter")
	tests := []struct {
		name       string
		descriptor string
		assert     func(*testing.T, PropertyDescriptor)
	}{
		{
			name:       "generic becomes data",
			descriptor: `{ configurable: true }`,
			assert: func(t *testing.T, descriptor PropertyDescriptor) {
				if !descriptor.IsData() || descriptor.IsAccessor() || !IsUndefined(descriptor.Value) ||
					descriptor.Writable != FLAG_FALSE || descriptor.Configurable != FLAG_TRUE ||
					descriptor.Enumerable != FLAG_FALSE {
					t.Fatalf("descriptor = %+v", descriptor)
				}
			},
		},
		{
			name:       "undefined accessor",
			descriptor: `{ get: undefined, configurable: true }`,
			assert: func(t *testing.T, descriptor PropertyDescriptor) {
				if !descriptor.IsAccessor() || descriptor.IsData() ||
					!IsUndefined(descriptor.Getter) || !IsUndefined(descriptor.Setter) ||
					descriptor.Configurable != FLAG_TRUE || descriptor.Enumerable != FLAG_FALSE {
					t.Fatalf("descriptor = %+v", descriptor)
				}
			},
		},
		{
			name:       "getter",
			descriptor: `{ get: proxyDescriptorGetter, enumerable: true, configurable: true }`,
			assert: func(t *testing.T, descriptor PropertyDescriptor) {
				if !descriptor.IsAccessor() || !descriptor.Getter.SameAs(getter) ||
					!IsUndefined(descriptor.Setter) || descriptor.Enumerable != FLAG_TRUE ||
					descriptor.Configurable != FLAG_TRUE {
					t.Fatalf("descriptor = %+v", descriptor)
				}
			},
		},
		{
			name:       "setter",
			descriptor: `{ set: proxyDescriptorSetter, configurable: true }`,
			assert: func(t *testing.T, descriptor PropertyDescriptor) {
				if !descriptor.IsAccessor() || !descriptor.Setter.SameAs(setter) ||
					!IsUndefined(descriptor.Getter) || descriptor.Configurable != FLAG_TRUE {
					t.Fatalf("descriptor = %+v", descriptor)
				}
			},
		},
		{
			name:       "undefined data",
			descriptor: `{ value: undefined, writable: true, configurable: true }`,
			assert: func(t *testing.T, descriptor PropertyDescriptor) {
				if !descriptor.IsData() || descriptor.IsAccessor() || !IsUndefined(descriptor.Value) ||
					descriptor.Writable != FLAG_TRUE || descriptor.Configurable != FLAG_TRUE {
					t.Fatalf("descriptor = %+v", descriptor)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value, err := runtime.RunString(`
				new Proxy({}, {
					getOwnPropertyDescriptor() {
						return ` + test.descriptor + `;
					},
				});
			`)
			if err != nil {
				t.Fatal(err)
			}
			descriptor, ok := value.ToObject(runtime).OwnPropertyDescriptor("value")
			if !ok {
				t.Fatal("OwnPropertyDescriptor returned false")
			}
			test.assert(t, descriptor)
		})
	}
}

func TestObjectOwnPropertyDescriptorProxyTrap(t *testing.T) {
	runtime := New()
	value, err := runtime.RunString(`
		globalThis.ownDescriptorSentinel = {};
		new Proxy({}, {
			getOwnPropertyDescriptor() {
				throw ownDescriptorSentinel;
			},
		});
	`)
	if err != nil {
		t.Fatal(err)
	}
	proxy := value.ToObject(runtime)
	exception := runtime.Try(func() {
		proxy.OwnPropertyDescriptor("value")
	})
	if exception == nil || !exception.Value().SameAs(runtime.Get("ownDescriptorSentinel")) {
		t.Fatalf("proxy trap exception = %v", exception)
	}
}

// TestObjectOwnPropertyDescriptorGoProxyTrap exercises the Go-side
// ProxyTrapConfig.GetOwnPropertyDescriptor path through the
// descriptorValueProperty helper, covering the descriptor shapes a Go host
// might return: generic, data (including undefined value), accessor (getter,
// setter, undefined getter), and a non-callable getter that must TypeError.
func TestObjectOwnPropertyDescriptorGoProxyTrap(t *testing.T) {
	runtime := New()
	getter := runtime.ToValue(func(FunctionCall) Value { return runtime.ToValue(1) })
	setter := runtime.ToValue(func(FunctionCall) Value { return Undefined() })

	tests := []struct {
		name   string
		desc   PropertyDescriptor
		assert func(*testing.T, PropertyDescriptor)
		throws bool
	}{
		{
			name: "generic becomes data",
			desc: PropertyDescriptor{Configurable: FLAG_TRUE},
			assert: func(t *testing.T, d PropertyDescriptor) {
				if !d.IsData() || d.IsAccessor() || !IsUndefined(d.Value) ||
					d.Writable != FLAG_FALSE || d.Configurable != FLAG_TRUE || d.Enumerable != FLAG_FALSE {
					t.Fatalf("descriptor = %+v", d)
				}
			},
		},
		{
			name: "undefined data value",
			desc: PropertyDescriptor{Value: Undefined(), Writable: FLAG_TRUE, Configurable: FLAG_TRUE},
			assert: func(t *testing.T, d PropertyDescriptor) {
				if !d.IsData() || d.IsAccessor() || !IsUndefined(d.Value) ||
					d.Writable != FLAG_TRUE || d.Configurable != FLAG_TRUE {
					t.Fatalf("descriptor = %+v", d)
				}
			},
		},
		{
			name: "data value",
			desc: PropertyDescriptor{Value: runtime.ToValue(42), Writable: FLAG_TRUE, Configurable: FLAG_TRUE, Enumerable: FLAG_TRUE},
			assert: func(t *testing.T, d PropertyDescriptor) {
				if !d.IsData() || d.IsAccessor() || d.Value.ToInteger() != 42 ||
					d.Writable != FLAG_TRUE || d.Configurable != FLAG_TRUE || d.Enumerable != FLAG_TRUE {
					t.Fatalf("descriptor = %+v", d)
				}
			},
		},
		{
			name: "accessor getter only",
			desc: PropertyDescriptor{Getter: getter, Configurable: FLAG_TRUE, Enumerable: FLAG_TRUE},
			assert: func(t *testing.T, d PropertyDescriptor) {
				if !d.IsAccessor() || d.IsData() || !d.Getter.SameAs(getter) ||
					!IsUndefined(d.Setter) || d.Configurable != FLAG_TRUE || d.Enumerable != FLAG_TRUE {
					t.Fatalf("descriptor = %+v", d)
				}
			},
		},
		{
			name: "accessor setter only",
			desc: PropertyDescriptor{Setter: setter, Configurable: FLAG_TRUE},
			assert: func(t *testing.T, d PropertyDescriptor) {
				if !d.IsAccessor() || d.IsData() || !d.Setter.SameAs(setter) ||
					!IsUndefined(d.Getter) || d.Configurable != FLAG_TRUE {
					t.Fatalf("descriptor = %+v", d)
				}
			},
		},
		{
			name: "undefined accessor getter",
			desc: PropertyDescriptor{Getter: Undefined(), Configurable: FLAG_TRUE},
			assert: func(t *testing.T, d PropertyDescriptor) {
				if !d.IsAccessor() || d.IsData() || !IsUndefined(d.Getter) ||
					!IsUndefined(d.Setter) || d.Configurable != FLAG_TRUE {
					t.Fatalf("descriptor = %+v", d)
				}
			},
		},
		{
			name:   "non-callable getter typeerrors",
			desc:   PropertyDescriptor{Getter: runtime.ToValue(123), Configurable: FLAG_TRUE},
			throws: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := runtime.NewObject()
			desc := test.desc
			proxy := runtime.NewProxy(target, &ProxyTrapConfig{
				GetOwnPropertyDescriptor: func(target *Object, prop string) PropertyDescriptor {
					return desc
				},
			}).proxy.val

			if test.throws {
				exception := runtime.Try(func() {
					proxy.OwnPropertyDescriptor("value")
				})
				if exception == nil {
					t.Fatal("expected TypeError for non-callable getter, got nil")
				}
				return
			}

			descriptor, ok := proxy.OwnPropertyDescriptor("value")
			if !ok {
				t.Fatal("OwnPropertyDescriptor returned false")
			}
			test.assert(t, descriptor)
		})
	}
}
