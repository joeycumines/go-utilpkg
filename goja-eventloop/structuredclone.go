package gojaeventloop

import (
	"errors"
	"strconv"
	"strings"

	"github.com/joeycumines/goja"
)

// structuredClone() Global Function

// structuredClone implements the host-provided subset of the HTML structured
// clone algorithm using Goja's native JavaScript objects. Unsupported clone or
// transfer inputs throw DOMException(DataCloneError), matching Node/browser
// observable error shape rather than silently dropping data.
func (a *Adapter) structuredClone(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 {
		panic(a.structuredCloneTypeError("The \"The value argument must be specified\" argument must be specified"))
	}

	state := &structuredCloneState{
		visited:   make(map[*goja.Object]goja.Value),
		transfers: make(map[*goja.Object]*structuredCloneTransfer),
	}
	if len(call.Arguments) > 1 {
		a.collectStructuredCloneTransfers(call.Argument(1), state)
	}

	cloned := a.structuredCloneValue(call.Argument(0), state)
	a.finalizeStructuredCloneTransfers(state)

	return cloned
}

type structuredCloneTransfer struct {
	source      goja.ArrayBuffer
	destination goja.ArrayBuffer
	clone       goja.Value
}

type structuredCloneState struct {
	visited       map[*goja.Object]goja.Value
	transfers     map[*goja.Object]*structuredCloneTransfer
	transferOrder []*structuredCloneTransfer
}

func (a *Adapter) finalizeStructuredCloneTransfers(state *structuredCloneState) {
	if state == nil || len(state.transferOrder) == 0 {
		return
	}

	for _, transfer := range state.transferOrder {
		if transfer == nil || transfer.source.Detached() {
			panic(a.dataCloneError("Cannot transfer object of unsupported type."))
		}
		snapshot := append([]byte(nil), transfer.source.Bytes()...)
		destination := transfer.destination.Bytes()
		if len(destination) != len(snapshot) {
			panic(a.dataCloneError("Cannot transfer object of unsupported type."))
		}
		copy(destination, snapshot)
		if !transfer.source.Detach() {
			panic(a.dataCloneError("Cannot transfer object of unsupported type."))
		}
	}
}

func (a *Adapter) dataCloneError(message string) goja.Value {
	return a.throwDOMException("DataCloneError", message)
}

func (a *Adapter) structuredCloneTypeError(message string) *goja.Object {
	return a.runtime.NewTypeError(message)
}

func (a *Adapter) panicJSException(err error) {
	if err == nil {
		return
	}
	var jsErr *goja.Exception
	if errors.As(err, &jsErr) {
		panic(jsErr.Value())
	}
	panic(a.runtime.NewTypeError(err.Error()))
}

func (a *Adapter) tryStructuredCloneIntrinsic(fn goja.Callable, receiver goja.Value, args ...goja.Value) (goja.Value, bool) {
	if fn == nil {
		return nil, false
	}
	var result goja.Value
	var err error
	exception := a.runtime.Try(func() {
		result, err = fn(receiver, args...)
		if err != nil {
			panic(err)
		}
	})
	if exception != nil {
		return nil, false
	}
	if result == nil {
		result = goja.Undefined()
	}
	return result, true
}

func (a *Adapter) callStructuredCloneIntrinsic(name string, fn goja.Callable, receiver goja.Value, args ...goja.Value) goja.Value {
	if fn == nil {
		panic(a.dataCloneError("structuredClone missing intrinsic " + name))
	}
	result, err := fn(receiver, args...)
	if err != nil {
		a.panicJSException(err)
	}
	if result == nil {
		return goja.Undefined()
	}
	return result
}

func (a *Adapter) collectStructuredCloneTransfers(options goja.Value, state *structuredCloneState) {
	if options == nil || goja.IsUndefined(options) || goja.IsNull(options) {
		return
	}

	optionsObj, ok := options.(*goja.Object)
	if !ok || optionsObj == nil {
		panic(a.invalidArgumentTypeError("Failed to execute 'structuredClone': Options cannot be converted to a dictionary"))
	}
	transferVal := optionsObj.Get("transfer")
	if transferVal == nil || goja.IsUndefined(transferVal) {
		return
	}

	err := a.iterateValues(transferVal, func(index int, entry goja.Value) error {
		entryObj, ok := entry.(*goja.Object)
		if !ok {
			panic(a.structuredCloneTypeError("Failed to execute 'structuredClone': transfer in Options[" + strconv.Itoa(index) + "] is not an object."))
		}
		if entryObj.ExportType() != gojaArrayBufferReflectType {
			panic(a.dataCloneError("Found invalid value in transferList."))
		}
		buffer, ok := entry.Export().(goja.ArrayBuffer)
		if !ok {
			panic(a.dataCloneError("Found invalid value in transferList."))
		}
		if _, exists := state.transfers[entryObj]; exists {
			panic(a.dataCloneError("Transfer list contains duplicate ArrayBuffer"))
		}

		destination := a.runtime.NewArrayBuffer(make([]byte, len(buffer.Bytes())))
		transfer := &structuredCloneTransfer{
			source:      buffer,
			destination: destination,
			clone:       a.runtime.ToValue(destination),
		}
		state.transfers[entryObj] = transfer
		state.visited[entryObj] = transfer.clone
		state.transferOrder = append(state.transferOrder, transfer)
		return nil
	})
	if err != nil {
		a.panicJSException(err)
	}
}

// structuredCloneValue recursively clones a value.
// The state tracks object references to preserve cycles and transfer identity.
func (a *Adapter) structuredCloneValue(value goja.Value, state *structuredCloneState) goja.Value {
	// Handle null and undefined (pass-through)
	if value == nil || goja.IsNull(value) {
		return goja.Null()
	}
	if goja.IsUndefined(value) {
		return goja.Undefined()
	}
	if _, ok := value.(*goja.Symbol); ok {
		panic(a.dataCloneError("structuredClone cannot clone symbol values"))
	}

	obj, ok := value.(*goja.Object)
	if !ok {
		// Primitive values are immutable and do not need cloning.
		return value
	}

	// Check if we've already cloned this object (circular reference)
	if cloned, exists := state.visited[obj]; exists {
		return cloned
	}

	// Check for non-cloneable types first (functions)
	if isFunction(obj) {
		panic(a.dataCloneError("structuredClone cannot clone function objects"))
	}
	if _, isProxy := a.proxyTarget(obj); isProxy {
		panic(a.dataCloneError("structuredClone cannot clone Proxy objects"))
	}

	// Handle different object types
	return a.cloneObject(obj, state)
}

// isFunction checks if a Goja object is a function.
func isFunction(obj *goja.Object) bool {
	// Check if it's callable
	_, ok := goja.AssertFunction(obj)
	return ok
}

// cloneObject clones a Goja object based on its type.
func (a *Adapter) cloneObject(obj *goja.Object, state *structuredCloneState) goja.Value {
	if obj == a.runtime.GlobalObject() || a.isKnownUncloneableObject(obj) {
		panic(a.dataCloneError("structuredClone cannot clone " + obj.ClassName() + " objects"))
	}

	// Check for built-in types that need special handling
	if obj.ExportType() == gojaArrayBufferReflectType {
		if buffer, ok := obj.Export().(goja.ArrayBuffer); ok {
			return a.cloneArrayBuffer(obj, buffer, state)
		}
		panic(a.dataCloneError("structuredClone cannot read ArrayBuffer data"))
	}

	if a.isDataViewObject(obj) {
		return a.cloneDataView(obj, state)
	}

	if a.isTypedArrayObject(obj) {
		return a.cloneTypedArray(obj, state)
	}

	if _, ok := a.tryStructuredCloneIntrinsic(a.structuredCloneBrands.symbolValueOf, obj); ok {
		panic(a.dataCloneError("structuredClone cannot clone Symbol objects"))
	}
	if _, ok := a.tryStructuredCloneIntrinsic(a.structuredCloneBrands.bigintValueOf, obj); ok {
		return a.clonePrimitiveWrapper(obj, state, "BigInt", a.structuredCloneBrands.bigintValueOf)
	}

	switch obj.ClassName() {
	case "Boolean":
		return a.clonePrimitiveWrapper(obj, state, "Boolean", a.structuredCloneBrands.booleanValueOf)
	case "Number":
		return a.clonePrimitiveWrapper(obj, state, "Number", a.structuredCloneBrands.numberValueOf)
	case "String":
		return a.clonePrimitiveWrapper(obj, state, "String", a.structuredCloneBrands.stringValueOf)
	}

	// 1. Check for Date
	if a.isDateObject(obj) {
		return a.cloneDate(obj, state)
	}

	// 2. Check for RegExp
	if a.isRegExpObject(obj) {
		return a.cloneRegExp(obj, state)
	}

	// 3. Check for Map
	if a.isMapObject(obj) {
		return a.cloneMap(obj, state)
	}

	// 4. Check for Set
	if a.isSetObject(obj) {
		return a.cloneSet(obj, state)
	}

	// 5. Check for Array
	if a.isArrayObject(obj) {
		return a.cloneArray(obj, state)
	}

	// DOMException has native Error data but its own serialized fields and
	// legacy brand, so it must be handled before the generic Error branch.
	if _, ok := a.domExceptionState(obj); ok {
		return a.cloneDOMException(obj, state)
	}

	// 6. Check for Error objects
	if a.isErrorObject(obj) {
		return a.cloneError(obj, state)
	}

	if !obj.IsOrdinary() {
		panic(a.dataCloneError("structuredClone cannot clone " + obj.ClassName() + " objects"))
	}

	// 7. Default: plain object
	return a.clonePlainObject(obj, state)
}

func (a *Adapter) isDataViewObject(obj *goja.Object) bool {
	if obj == nil || a.structuredCloneBrands == nil {
		return false
	}
	_, ok := a.tryStructuredCloneIntrinsic(a.structuredCloneBrands.dataViewBuffer, obj)
	return ok
}

func (a *Adapter) isTypedArrayObject(obj *goja.Object) bool {
	if obj == nil || a.structuredCloneBrands == nil || a.isDataViewObject(obj) {
		return false
	}
	_, ok := a.tryStructuredCloneIntrinsic(a.structuredCloneBrands.typedArrayBuffer, obj)
	return ok
}

func (a *Adapter) isKnownUncloneableObject(obj *goja.Object) bool {
	if obj == nil {
		return false
	}
	if obj.ExportType() == gojaPromiseReflectType {
		return true
	}
	for _, store := range []*goja.Object{
		a.eventTargetStateStore,
		a.eventStateStore,
		a.abortSignalStateStore,
		a.abortControllerStore,
		a.uncloneableStateStore,
	} {
		state := a.hiddenState(store, obj)
		if state != nil && !goja.IsUndefined(state) && !goja.IsNull(state) {
			return true
		}
	}
	if a.structuredCloneBrands == nil {
		return false
	}
	if _, ok := a.tryStructuredCloneIntrinsic(a.structuredCloneBrands.weakMapHas, obj, a.runtime.NewObject()); ok {
		return true
	}
	if _, ok := a.tryStructuredCloneIntrinsic(a.structuredCloneBrands.weakSetHas, obj, a.runtime.NewObject()); ok {
		return true
	}
	return false
}

func (a *Adapter) typedArrayConstructorName(obj *goja.Object) string {
	if obj == nil || a.structuredCloneBrands == nil {
		return ""
	}
	name, ok := a.tryStructuredCloneIntrinsic(a.structuredCloneBrands.typedArrayName, obj)
	if !ok || name == nil || goja.IsUndefined(name) || goja.IsNull(name) {
		return ""
	}
	return name.String()
}

func (a *Adapter) structuredCloneRegExpFlags(obj *goja.Object) string {
	if a.structuredCloneBrands == nil {
		return ""
	}
	var builder strings.Builder
	if a.callStructuredCloneIntrinsic("RegExp.prototype.global", a.structuredCloneBrands.regexpGlobal, obj).ToBoolean() {
		builder.WriteByte('g')
	}
	if a.callStructuredCloneIntrinsic("RegExp.prototype.ignoreCase", a.structuredCloneBrands.regexpIgnoreCase, obj).ToBoolean() {
		builder.WriteByte('i')
	}
	if a.callStructuredCloneIntrinsic("RegExp.prototype.multiline", a.structuredCloneBrands.regexpMultiline, obj).ToBoolean() {
		builder.WriteByte('m')
	}
	if a.callStructuredCloneIntrinsic("RegExp.prototype.dotAll", a.structuredCloneBrands.regexpDotAll, obj).ToBoolean() {
		builder.WriteByte('s')
	}
	if a.callStructuredCloneIntrinsic("RegExp.prototype.unicode", a.structuredCloneBrands.regexpUnicode, obj).ToBoolean() {
		builder.WriteByte('u')
	}
	if a.callStructuredCloneIntrinsic("RegExp.prototype.sticky", a.structuredCloneBrands.regexpSticky, obj).ToBoolean() {
		builder.WriteByte('y')
	}
	return builder.String()
}

func (a *Adapter) constructJS(ctorName string, args ...goja.Value) goja.Value {
	ctor, ok := a.structuredCloneConstructor(ctorName)
	if !ok {
		panic(a.dataCloneError("structuredClone cannot construct " + ctorName))
	}
	obj, err := ctor(nil, args...)
	if err != nil {
		a.panicJSException(err)
	}
	return obj
}

func (a *Adapter) structuredCloneConstructor(name string) (goja.Constructor, bool) {
	if a == nil || a.structuredCloneBrands == nil || a.structuredCloneBrands.constructors == nil {
		return nil, false
	}
	ctor, ok := a.structuredCloneBrands.constructors[name]
	return ctor, ok && ctor != nil
}

func (a *Adapter) cloneArrayBuffer(obj *goja.Object, buffer goja.ArrayBuffer, state *structuredCloneState) goja.Value {
	if buffer.Detached() {
		panic(a.dataCloneError("structuredClone cannot clone a detached ArrayBuffer"))
	}
	bytes := append([]byte(nil), buffer.Bytes()...)
	cloned := a.runtime.ToValue(a.runtime.NewArrayBuffer(bytes))
	state.visited[obj] = cloned
	return cloned
}

func (a *Adapter) cloneDataView(obj *goja.Object, state *structuredCloneState) goja.Value {
	bufferVal := a.callStructuredCloneIntrinsic("DataView.prototype.buffer", a.structuredCloneBrands.dataViewBuffer, obj)
	clonedBuffer := a.structuredCloneValue(bufferVal, state)
	byteOffset := a.callStructuredCloneIntrinsic("DataView.prototype.byteOffset", a.structuredCloneBrands.dataViewByteOffset, obj).ToInteger()
	byteLength := a.callStructuredCloneIntrinsic("DataView.prototype.byteLength", a.structuredCloneBrands.dataViewByteLength, obj).ToInteger()
	cloned := a.constructJS("DataView", clonedBuffer, a.runtime.ToValue(byteOffset), a.runtime.ToValue(byteLength))
	state.visited[obj] = cloned
	return cloned
}

func (a *Adapter) cloneTypedArray(obj *goja.Object, state *structuredCloneState) goja.Value {
	constructorName := a.typedArrayConstructorName(obj)
	if constructorName == "" {
		panic(a.dataCloneError("structuredClone cannot identify typed array clone constructor"))
	}
	constructor, ok := a.structuredCloneConstructor(constructorName)
	if !ok {
		panic(a.dataCloneError("structuredClone cannot construct typed array clone"))
	}
	bufferVal := a.callStructuredCloneIntrinsic("TypedArray.prototype.buffer", a.structuredCloneBrands.typedArrayBuffer, obj)
	clonedBuffer := a.structuredCloneValue(bufferVal, state)
	byteOffset := a.callStructuredCloneIntrinsic("TypedArray.prototype.byteOffset", a.structuredCloneBrands.typedArrayByteOffset, obj).ToInteger()
	length := a.callStructuredCloneIntrinsic("TypedArray.prototype.length", a.structuredCloneBrands.typedArrayLength, obj).ToInteger()
	cloned, err := constructor(nil, clonedBuffer, a.runtime.ToValue(byteOffset), a.runtime.ToValue(length))
	if err != nil {
		a.panicJSException(err)
	}
	state.visited[obj] = cloned
	return cloned
}

func (a *Adapter) clonePrimitiveWrapper(obj *goja.Object, state *structuredCloneState, name string, valueOf goja.Callable) goja.Value {
	primitive := a.callStructuredCloneIntrinsic(name+".prototype.valueOf", valueOf, obj)
	cloned := primitive.ToObject(a.runtime)
	state.visited[obj] = cloned
	return cloned
}

// isDateObject checks if a Goja object is a Date instance.
func (a *Adapter) isDateObject(obj *goja.Object) bool {
	if obj == nil || a.structuredCloneBrands == nil {
		return false
	}
	_, ok := a.tryStructuredCloneIntrinsic(a.structuredCloneBrands.dateGetTime, obj)
	return ok
}

// cloneDate clones a Date object.
func (a *Adapter) cloneDate(obj *goja.Object, state *structuredCloneState) goja.Value {
	timeVal := a.callStructuredCloneIntrinsic("Date.prototype.getTime", a.structuredCloneBrands.dateGetTime, obj)
	milliseconds := timeVal.ToFloat()

	newDate := a.constructJS("Date", a.runtime.ToValue(milliseconds))

	// Register in visited map
	state.visited[obj] = newDate

	return newDate
}

// isRegExpObject checks if a Goja object is a RegExp instance.
func (a *Adapter) isRegExpObject(obj *goja.Object) bool {
	return obj != nil && obj.ClassName() == "RegExp"
}

// cloneRegExp clones a RegExp object.
func (a *Adapter) cloneRegExp(obj *goja.Object, state *structuredCloneState) goja.Value {
	source := a.callStructuredCloneIntrinsic("RegExp.prototype.source", a.structuredCloneBrands.regexpSource, obj).String()
	flags := a.structuredCloneRegExpFlags(obj)

	newRegexp := a.constructJS("RegExp", a.runtime.ToValue(source), a.runtime.ToValue(flags))

	// Register in visited map
	state.visited[obj] = newRegexp

	return newRegexp
}

// isMapObject checks if a Goja object is a Map instance.
func (a *Adapter) isMapObject(obj *goja.Object) bool {
	if obj == nil || a.structuredCloneBrands == nil {
		return false
	}
	_, ok := a.tryStructuredCloneIntrinsic(a.structuredCloneBrands.mapSize, obj)
	return ok
}

// cloneMap clones a Map object.
func (a *Adapter) cloneMap(obj *goja.Object, state *structuredCloneState) goja.Value {
	// Create a new Map using JS: new Map()
	newMapVal := a.constructJS("Map")
	newMapObj := newMapVal.ToObject(a.runtime)

	// Register in visited map BEFORE iterating to handle circular references
	state.visited[obj] = newMapVal

	type entry struct {
		key   goja.Value
		value goja.Value
	}
	var entries []entry
	callback := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		entries = append(entries, entry{key: call.Argument(1), value: call.Argument(0)})
		return goja.Undefined()
	})
	a.callStructuredCloneIntrinsic("Map.prototype.forEach", a.structuredCloneBrands.mapForEach, obj, callback)
	for _, item := range entries {
		clonedKey := a.structuredCloneValue(item.key, state)
		clonedValue := a.structuredCloneValue(item.value, state)
		a.callStructuredCloneIntrinsic("Map.prototype.set", a.structuredCloneBrands.mapSet, newMapObj, clonedKey, clonedValue)
	}

	return newMapVal
}

// isSetObject checks if a Goja object is a Set instance.
func (a *Adapter) isSetObject(obj *goja.Object) bool {
	if obj == nil || a.structuredCloneBrands == nil {
		return false
	}
	_, ok := a.tryStructuredCloneIntrinsic(a.structuredCloneBrands.setSize, obj)
	return ok
}

// cloneSet clones a Set object.
func (a *Adapter) cloneSet(obj *goja.Object, state *structuredCloneState) goja.Value {
	// Create a new Set using JS: new Set()
	newSetVal := a.constructJS("Set")
	newSetObj := newSetVal.ToObject(a.runtime)

	// Register in visited map BEFORE iterating to handle circular references
	state.visited[obj] = newSetVal

	var values []goja.Value
	callback := a.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		values = append(values, call.Argument(0))
		return goja.Undefined()
	})
	a.callStructuredCloneIntrinsic("Set.prototype.forEach", a.structuredCloneBrands.setForEach, obj, callback)
	for _, value := range values {
		clonedValue := a.structuredCloneValue(value, state)
		a.callStructuredCloneIntrinsic("Set.prototype.add", a.structuredCloneBrands.setAdd, newSetObj, clonedValue)
	}

	return newSetVal
}

// isArrayObject checks if a Goja object is an Array.
func (a *Adapter) isArrayObject(obj *goja.Object) bool {
	return obj != nil && obj.IsECMAScriptArray()
}

// cloneArray clones an Array object.
func (a *Adapter) cloneArray(obj *goja.Object, state *structuredCloneState) goja.Value {
	length := obj.Get("length").ToInteger()

	// Create a new array
	newArr := a.runtime.NewArray()
	if err := newArr.Set("length", length); err != nil {
		a.panicJSException(err)
	}

	// Register in visited map BEFORE iterating to handle circular references
	state.visited[obj] = newArr
	a.cloneEnumerableStringProperties(obj, newArr, state, nil)

	return newArr
}

// isErrorObject checks if a Goja object is an Error instance.
func (a *Adapter) isErrorObject(obj *goja.Object) bool {
	return obj != nil && obj.ClassName() == "Error"
}

func (a *Adapter) cloneError(obj *goja.Object, state *structuredCloneState) goja.Value {
	ctorName := "Error"
	nameValue := obj.Get("name")
	if name, ok := primitiveString(nameValue); ok {
		switch name {
		case "Error", "EvalError", "RangeError", "ReferenceError", "SyntaxError", "TypeError", "URIError":
			if _, ok := a.structuredCloneConstructor(name); ok {
				ctorName = name
			}
		}
	}
	messageDescriptor := a.ownStructuredCloneDescriptor(obj, "message")
	messageIsData := descriptorHasDataValue(messageDescriptor)
	var cloned goja.Value
	if messageIsData {
		message := messageDescriptor.Value
		if message == nil {
			message = goja.Undefined()
		}
		cloned = a.constructJS(ctorName, a.runtime.ToValue(a.webIDLString(message)))
	} else {
		cloned = a.constructJS(ctorName)
	}
	clonedObj := cloned.ToObject(a.runtime)
	state.visited[obj] = cloned

	// Snapshot optional stack data before recursively serializing accompanying
	// cause data, whose getters may mutate the source Error.
	if messageDescriptor == nil || messageIsData {
		a.cloneErrorStack(obj, clonedObj)
	}
	if descriptor := a.ownStructuredCloneDescriptor(obj, "cause"); descriptor != nil && descriptorHasDataValue(descriptor) {
		cause := descriptor.Value
		if cause == nil {
			cause = goja.Undefined()
		}
		if err := clonedObj.DefineDataProperty("cause", a.structuredCloneValue(cause, state), goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE); err != nil {
			a.panicJSException(err)
		}
	}
	return cloned
}

func (a *Adapter) cloneDOMException(obj *goja.Object, state *structuredCloneState) goja.Value {
	source, ok := a.domExceptionState(obj)
	if !ok {
		panic(a.dataCloneError("structuredClone cannot read DOMException state"))
	}
	cloned := a.newDOMExceptionObject(source.message, source.name, source.prototype, source.prototype)
	state.visited[obj] = cloned

	a.cloneErrorStack(obj, cloned)
	return cloned
}

func (a *Adapter) cloneErrorStack(source, target *goja.Object) {
	descriptor := a.ownStructuredCloneDescriptor(source, "stack")
	if descriptor == nil || !descriptorHasDataValue(descriptor) {
		return
	}
	stack := goja.Value(goja.Undefined())
	if value := descriptor.Value; value != nil {
		if text, ok := primitiveString(value); ok {
			stack = a.runtime.ToValue(text)
		}
	}
	if err := target.DefineDataProperty("stack", stack, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_FALSE); err != nil {
		a.panicJSException(err)
	}
}

func (a *Adapter) hasOwnStructuredCloneProperty(obj *goja.Object, name string) bool {
	return a.ownStructuredCloneDescriptor(obj, name) != nil
}

func (a *Adapter) ownStructuredCloneDescriptor(obj *goja.Object, name string) *goja.PropertyDescriptor {
	if obj == nil {
		return nil
	}
	descriptor, ok := obj.OwnPropertyDescriptor(name)
	if !ok {
		return nil
	}
	return &descriptor
}

func descriptorHasDataValue(descriptor *goja.PropertyDescriptor) bool {
	return descriptor != nil && descriptor.IsData()
}

// clonePlainObject clones a plain JavaScript object.
func (a *Adapter) clonePlainObject(obj *goja.Object, state *structuredCloneState) goja.Value {
	// Create a new object
	newObj := a.runtime.NewObject()

	// Register in visited map BEFORE iterating to handle circular references
	state.visited[obj] = newObj
	a.cloneEnumerableStringProperties(obj, newObj, state, nil)

	return newObj
}

func (a *Adapter) ownEnumerableStringKeys(obj *goja.Object) []string {
	if obj == nil {
		return nil
	}
	var keys []string
	if ex := a.runtime.Try(func() { keys = obj.Keys() }); ex != nil {
		a.panicJSException(ex)
	}
	return keys
}

func (a *Adapter) cloneEnumerableStringProperties(source, target *goja.Object, state *structuredCloneState, skip func(string) bool) {
	for _, key := range a.ownEnumerableStringKeys(source) {
		if skip != nil && skip(key) {
			continue
		}
		if !a.hasOwnStructuredCloneProperty(source, key) {
			continue
		}
		value := source.Get(key)
		if value == nil {
			value = goja.Undefined()
		}
		clonedValue := a.structuredCloneValue(value, state)
		if err := target.DefineDataProperty(key, clonedValue, goja.FLAG_TRUE, goja.FLAG_TRUE, goja.FLAG_TRUE); err != nil {
			a.panicJSException(err)
		}
	}
}
