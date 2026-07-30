package goja

// Intrinsic identifies specific intrinsic object variants.
//
// https://tc39.es/ecma262/2024/#sec-well-known-intrinsic-objects
//
// N.B. This enumeration is not a complete implementation.
type Intrinsic uint16

const (
	IntrinsicInvalid Intrinsic = iota
	IntrinsicDateConstructor
	IntrinsicRegExpConstructor
	IntrinsicMapConstructor
	IntrinsicSetConstructor
	IntrinsicDataViewConstructor
	IntrinsicErrorConstructor
	IntrinsicEvalErrorConstructor
	IntrinsicRangeErrorConstructor
	IntrinsicReferenceErrorConstructor
	IntrinsicSyntaxErrorConstructor
	IntrinsicTypeErrorConstructor
	IntrinsicURIErrorConstructor
	IntrinsicInt8ArrayConstructor
	IntrinsicUint8ArrayConstructor
	IntrinsicUint8ClampedArrayConstructor
	IntrinsicInt16ArrayConstructor
	IntrinsicUint16ArrayConstructor
	IntrinsicInt32ArrayConstructor
	IntrinsicUint32ArrayConstructor
	IntrinsicFloat32ArrayConstructor
	IntrinsicFloat64ArrayConstructor
	IntrinsicBigInt64ArrayConstructor
	IntrinsicBigUint64ArrayConstructor
	IntrinsicPromiseConstructor
	IntrinsicSymbolConstructor
	IntrinsicAggregateErrorConstructor
	IntrinsicWeakMapConstructor
	IntrinsicWeakSetConstructor
	IntrinsicStringConstructor
	IntrinsicErrorPrototype

	IntrinsicDateGetTime
	IntrinsicRegExpSourceGetter
	IntrinsicRegExpGlobalGetter
	IntrinsicRegExpIgnoreCaseGetter
	IntrinsicRegExpMultilineGetter
	IntrinsicRegExpDotAllGetter
	IntrinsicRegExpUnicodeGetter
	IntrinsicRegExpStickyGetter
	IntrinsicDataViewBufferGetter
	IntrinsicDataViewByteOffsetGetter
	IntrinsicDataViewByteLengthGetter
	IntrinsicTypedArrayBufferGetter
	IntrinsicTypedArrayByteOffsetGetter
	IntrinsicTypedArrayLengthGetter
	IntrinsicTypedArrayNameGetter
	IntrinsicMapSizeGetter
	IntrinsicMapForEach
	IntrinsicMapSet
	IntrinsicSetSizeGetter
	IntrinsicSetForEach
	IntrinsicSetAdd
	IntrinsicWeakMapGet
	IntrinsicWeakMapSet
	IntrinsicWeakMapHas
	IntrinsicWeakSetAdd
	IntrinsicWeakSetHas
	IntrinsicWeakSetDelete
	IntrinsicBooleanValueOf
	IntrinsicNumberValueOf
	IntrinsicBigIntValueOf
	IntrinsicStringValueOf
	IntrinsicSymbolValueOf
	IntrinsicSymbolToString
	IntrinsicReflectApply
	IntrinsicReflectConstruct
	IntrinsicReflectDeleteProperty
	IntrinsicObjectCreate
	IntrinsicObjectDefineProperty
	IntrinsicObjectGetOwnPropertyDescriptor
	IntrinsicObjectGetOwnPropertyDescriptors
	IntrinsicObjectGetPrototypeOf
	IntrinsicObjectSetPrototypeOf
	IntrinsicFunctionBind
	IntrinsicFunctionToString
	IntrinsicArrayIndexOf
	IntrinsicArrayJoin
	IntrinsicArraySlice
	IntrinsicArraySplice
	IntrinsicMathMin
	IntrinsicMathMax
	IntrinsicMathTrunc
	IntrinsicStringSplit
)

// Intrinsic returns the runtime-bound canonical value identified by id.
//
// The lookup does not read JavaScript globals or prototype properties and does
// not execute JavaScript. Repeated successful lookups return the same value
// identity. It returns false for a nil Runtime or an unknown id.
//
// The returned value is the realm's canonical object identity. It is not
// frozen; JavaScript that has obtained a reference to the same object (for
// example through a prototype property) may still mutate the object's own
// properties.
//
// Access to Runtime and its values are subject to the calling goroutine
// constraints documented by Runtime.
func (r *Runtime) Intrinsic(id Intrinsic) (Value, bool) {
	if r == nil {
		return nil, false
	}
	if value := r.intrinsics[id]; value != nil {
		return value, true
	}
	value := r.newIntrinsic(id)
	if value == nil {
		return nil, false
	}
	if r.intrinsics == nil {
		r.intrinsics = make(map[Intrinsic]Value)
	}
	r.intrinsics[id] = value
	return value, true
}

func (r *Runtime) intrinsicObject(id Intrinsic) *Object {
	value, _ := r.Intrinsic(id)
	object, _ := value.(*Object)
	return object
}

func (r *Runtime) newIntrinsic(id Intrinsic) Value {
	switch id {
	case IntrinsicDateConstructor:
		return r.getDate()
	case IntrinsicRegExpConstructor:
		return r.getRegExp()
	case IntrinsicMapConstructor:
		return r.getMap()
	case IntrinsicSetConstructor:
		return r.getSet()
	case IntrinsicDataViewConstructor:
		return r.getDataView()
	case IntrinsicErrorConstructor:
		return r.getError()
	case IntrinsicEvalErrorConstructor:
		return r.getEvalError()
	case IntrinsicRangeErrorConstructor:
		return r.getRangeError()
	case IntrinsicReferenceErrorConstructor:
		return r.getReferenceError()
	case IntrinsicSyntaxErrorConstructor:
		return r.getSyntaxError()
	case IntrinsicTypeErrorConstructor:
		return r.getTypeError()
	case IntrinsicURIErrorConstructor:
		return r.getURIError()
	case IntrinsicInt8ArrayConstructor:
		return r.getInt8Array()
	case IntrinsicUint8ArrayConstructor:
		return r.getUint8Array()
	case IntrinsicUint8ClampedArrayConstructor:
		return r.getUint8ClampedArray()
	case IntrinsicInt16ArrayConstructor:
		return r.getInt16Array()
	case IntrinsicUint16ArrayConstructor:
		return r.getUint16Array()
	case IntrinsicInt32ArrayConstructor:
		return r.getInt32Array()
	case IntrinsicUint32ArrayConstructor:
		return r.getUint32Array()
	case IntrinsicFloat32ArrayConstructor:
		return r.getFloat32Array()
	case IntrinsicFloat64ArrayConstructor:
		return r.getFloat64Array()
	case IntrinsicBigInt64ArrayConstructor:
		return r.getBigInt64Array()
	case IntrinsicBigUint64ArrayConstructor:
		return r.getBigUint64Array()
	case IntrinsicPromiseConstructor:
		return r.getPromise()
	case IntrinsicSymbolConstructor:
		return r.getSymbol()
	case IntrinsicAggregateErrorConstructor:
		return r.getAggregateError()
	case IntrinsicWeakMapConstructor:
		return r.getWeakMap()
	case IntrinsicWeakSetConstructor:
		return r.getWeakSet()
	case IntrinsicStringConstructor:
		return r.getString()
	case IntrinsicErrorPrototype:
		return r.getErrorPrototype()
	case IntrinsicDateGetTime:
		return r.newNativeFunc(r.dateproto_getTime, "getTime", 0)
	case IntrinsicRegExpSourceGetter:
		return r.newNativeFunc(r.regexpproto_getSource, "get source", 0)
	case IntrinsicRegExpGlobalGetter:
		return r.newNativeFunc(r.regexpproto_getGlobal, "get global", 0)
	case IntrinsicRegExpIgnoreCaseGetter:
		return r.newNativeFunc(r.regexpproto_getIgnoreCase, "get ignoreCase", 0)
	case IntrinsicRegExpMultilineGetter:
		return r.newNativeFunc(r.regexpproto_getMultiline, "get multiline", 0)
	case IntrinsicRegExpDotAllGetter:
		return r.newNativeFunc(r.regexpproto_getDotAll, "get dotAll", 0)
	case IntrinsicRegExpUnicodeGetter:
		return r.newNativeFunc(r.regexpproto_getUnicode, "get unicode", 0)
	case IntrinsicRegExpStickyGetter:
		return r.newNativeFunc(r.regexpproto_getSticky, "get sticky", 0)
	case IntrinsicDataViewBufferGetter:
		return r.newNativeFunc(r.dataViewProto_getBuffer, "get buffer", 0)
	case IntrinsicDataViewByteOffsetGetter:
		return r.newNativeFunc(r.dataViewProto_getByteOffset, "get byteOffset", 0)
	case IntrinsicDataViewByteLengthGetter:
		return r.newNativeFunc(r.dataViewProto_getByteLen, "get byteLength", 0)
	case IntrinsicTypedArrayBufferGetter:
		return r.newNativeFunc(r.typedArrayProto_getBuffer, "get buffer", 0)
	case IntrinsicTypedArrayByteOffsetGetter:
		return r.newNativeFunc(r.typedArrayProto_getByteOffset, "get byteOffset", 0)
	case IntrinsicTypedArrayLengthGetter:
		return r.newNativeFunc(r.typedArrayProto_getLength, "get length", 0)
	case IntrinsicTypedArrayNameGetter:
		return r.newNativeFunc(r.typedArrayProto_toStringTag, "get [Symbol.toStringTag]", 0)
	case IntrinsicMapSizeGetter:
		return r.newNativeFunc(r.mapProto_getSize, "get size", 0)
	case IntrinsicMapForEach:
		return r.newNativeFunc(r.mapProto_forEach, "forEach", 1)
	case IntrinsicMapSet:
		return r.newNativeFunc(r.mapProto_set, "set", 2)
	case IntrinsicSetSizeGetter:
		return r.newNativeFunc(r.setProto_getSize, "get size", 0)
	case IntrinsicSetForEach:
		return r.newNativeFunc(r.setProto_forEach, "forEach", 1)
	case IntrinsicSetAdd:
		return r.newNativeFunc(r.setProto_add, "add", 1)
	case IntrinsicWeakMapGet:
		return r.newNativeFunc(r.weakMapProto_get, "get", 1)
	case IntrinsicWeakMapSet:
		return r.newNativeFunc(r.weakMapProto_set, "set", 2)
	case IntrinsicWeakMapHas:
		return r.newNativeFunc(r.weakMapProto_has, "has", 1)
	case IntrinsicWeakSetAdd:
		return r.newNativeFunc(r.weakSetProto_add, "add", 1)
	case IntrinsicWeakSetHas:
		return r.newNativeFunc(r.weakSetProto_has, "has", 1)
	case IntrinsicWeakSetDelete:
		return r.newNativeFunc(r.weakSetProto_delete, "delete", 1)
	case IntrinsicBooleanValueOf:
		return r.newNativeFunc(r.booleanproto_valueOf, "valueOf", 0)
	case IntrinsicNumberValueOf:
		return r.newNativeFunc(r.numberproto_valueOf, "valueOf", 0)
	case IntrinsicBigIntValueOf:
		return r.newNativeFunc(r.bigintproto_valueOf, "valueOf", 0)
	case IntrinsicStringValueOf:
		return r.newNativeFunc(r.stringproto_valueOf, "valueOf", 0)
	case IntrinsicSymbolValueOf:
		return r.newNativeFunc(r.symbolproto_valueOf, "valueOf", 0)
	case IntrinsicSymbolToString:
		return r.newNativeFunc(r.symbolproto_tostring, "toString", 0)
	case IntrinsicReflectApply:
		return r.newNativeFunc(r.builtin_reflect_apply, "apply", 3)
	case IntrinsicReflectConstruct:
		return r.newNativeFunc(r.builtin_reflect_construct, "construct", 2)
	case IntrinsicReflectDeleteProperty:
		return r.newNativeFunc(r.builtin_reflect_deleteProperty, "deleteProperty", 2)
	case IntrinsicObjectCreate:
		return r.newNativeFunc(r.object_create, "create", 2)
	case IntrinsicObjectDefineProperty:
		return r.newNativeFunc(r.object_defineProperty, "defineProperty", 3)
	case IntrinsicObjectGetOwnPropertyDescriptor:
		return r.newNativeFunc(r.object_getOwnPropertyDescriptor, "getOwnPropertyDescriptor", 2)
	case IntrinsicObjectGetOwnPropertyDescriptors:
		return r.newNativeFunc(r.object_getOwnPropertyDescriptors, "getOwnPropertyDescriptors", 1)
	case IntrinsicObjectGetPrototypeOf:
		return r.newNativeFunc(r.object_getPrototypeOf, "getPrototypeOf", 1)
	case IntrinsicObjectSetPrototypeOf:
		return r.newNativeFunc(r.object_setPrototypeOf, "setPrototypeOf", 2)
	case IntrinsicFunctionBind:
		return r.newNativeFunc(r.functionproto_bind, "bind", 1)
	case IntrinsicFunctionToString:
		return r.newNativeFunc(r.functionproto_toString, "toString", 0)
	case IntrinsicArrayIndexOf:
		return r.newNativeFunc(r.arrayproto_indexOf, "indexOf", 1)
	case IntrinsicArrayJoin:
		return r.newNativeFunc(r.arrayproto_join, "join", 1)
	case IntrinsicArraySlice:
		return r.newNativeFunc(r.arrayproto_slice, "slice", 2)
	case IntrinsicArraySplice:
		return r.newNativeFunc(r.arrayproto_splice, "splice", 2)
	case IntrinsicMathMin:
		return r.newNativeFunc(r.math_min, "min", 2)
	case IntrinsicMathMax:
		return r.newNativeFunc(r.math_max, "max", 2)
	case IntrinsicMathTrunc:
		return r.newNativeFunc(r.math_trunc, "trunc", 1)
	case IntrinsicStringSplit:
		return r.newNativeFunc(r.stringproto_split, "split", 2)
	default:
		return nil
	}
}
