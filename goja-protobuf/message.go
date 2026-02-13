package gojaprotobuf

import (
	"fmt"

	"github.com/dop251/goja"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// wrapMessage creates a JS object wrapping a [dynamicpb.Message] with
// get/set/has/clear/whichOneof/clearOneof methods and a $type
// read-only property.
func (m *Module) wrapMessage(msg *dynamicpb.Message) *goja.Object {
	obj := m.runtime.NewObject()
	msgDesc := msg.Descriptor()

	// Store the underlying proto message.
	_ = obj.Set("_pbMsg", msg)

	// $type — read-only accessor returning the fully-qualified name.
	_ = obj.DefineAccessorProperty("$type",
		m.runtime.ToValue(func(goja.FunctionCall) goja.Value {
			return m.runtime.ToValue(string(msgDesc.FullName()))
		}),
		nil,
		goja.FLAG_FALSE,
		goja.FLAG_TRUE,
	)

	// get(fieldName) — retrieve a field value.
	_ = obj.Set("get", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		name := call.Argument(0).String()
		fd := m.resolveField(msgDesc, name)

		if fd.IsList() {
			return m.wrapRepeatedField(msg, fd)
		}
		if fd.IsMap() {
			return m.wrapMapField(msg, fd)
		}
		if fd.Kind() == protoreflect.MessageKind || fd.Kind() == protoreflect.GroupKind {
			if !msg.Has(fd) {
				return goja.Null()
			}
		}
		return m.protoValueToGoja(msg.Get(fd), fd)
	}))

	// set(fieldName, value) — set a field value.
	_ = obj.Set("set", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		name := call.Argument(0).String()
		val := call.Argument(1)
		fd := m.resolveField(msgDesc, name)

		// null/undefined → clear the field.
		if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
			msg.Clear(fd)
			return goja.Undefined()
		}

		if fd.IsList() {
			// Clear existing, then populate from array.
			msg.Clear(fd)
			if err := m.setRepeatedFromGoja(msg, fd, val); err != nil {
				panic(m.runtime.NewTypeError(err.Error()))
			}
			return goja.Undefined()
		}
		if fd.IsMap() {
			msg.Clear(fd)
			if err := m.setMapFromGoja(msg, fd, val); err != nil {
				panic(m.runtime.NewTypeError(err.Error()))
			}
			return goja.Undefined()
		}

		pv, err := m.gojaToProtoValue(val, fd)
		if err != nil {
			panic(m.runtime.NewTypeError(err.Error()))
		}
		msg.Set(fd, pv)
		return goja.Undefined()
	}))

	// has(fieldName) — check whether a field has been set.
	_ = obj.Set("has", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		name := call.Argument(0).String()
		fd := m.resolveField(msgDesc, name)
		return m.runtime.ToValue(msg.Has(fd))
	}))

	// clear(fieldName) — clear a field to its default.
	_ = obj.Set("clear", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		name := call.Argument(0).String()
		fd := m.resolveField(msgDesc, name)
		msg.Clear(fd)
		return goja.Undefined()
	}))

	// whichOneof(oneofName) — return the name of the set oneof field, or undefined.
	_ = obj.Set("whichOneof", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		oneofName := call.Argument(0).String()
		od := msgDesc.Oneofs().ByName(protoreflect.Name(oneofName))
		if od == nil {
			panic(m.runtime.NewTypeError("oneof %q not found on message %q", oneofName, msgDesc.FullName()))
		}
		fd := msg.WhichOneof(od)
		if fd == nil {
			return goja.Undefined()
		}
		return m.runtime.ToValue(string(fd.Name()))
	}))

	// clearOneof(oneofName) — clear whichever field is set in a oneof group.
	_ = obj.Set("clearOneof", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		oneofName := call.Argument(0).String()
		od := msgDesc.Oneofs().ByName(protoreflect.Name(oneofName))
		if od == nil {
			panic(m.runtime.NewTypeError("oneof %q not found on message %q", oneofName, msgDesc.FullName()))
		}
		fd := msg.WhichOneof(od)
		if fd != nil {
			msg.Clear(fd)
		}
		return goja.Undefined()
	}))

	return obj
}

// unwrapMessage extracts a [dynamicpb.Message] from a JS value that was
// created by [Module.wrapMessage].
func (m *Module) unwrapMessage(val goja.Value) (*dynamicpb.Message, error) {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil, fmt.Errorf("expected protobuf message, got null/undefined")
	}
	obj, ok := val.(*goja.Object)
	if !ok {
		return nil, fmt.Errorf("expected protobuf message object")
	}

	msgVal := obj.Get("_pbMsg")
	if msgVal == nil || goja.IsUndefined(msgVal) {
		return nil, fmt.Errorf("not a protobuf message wrapper")
	}

	msg, ok := msgVal.Export().(*dynamicpb.Message)
	if !ok || msg == nil {
		return nil, fmt.Errorf("not a protobuf message wrapper")
	}
	return msg, nil
}

// resolveField looks up a field descriptor by proto field name. Panics
// with a JS TypeError if the field is not found.
func (m *Module) resolveField(msgDesc protoreflect.MessageDescriptor, name string) protoreflect.FieldDescriptor {
	fd := msgDesc.Fields().ByName(protoreflect.Name(name))
	if fd == nil {
		panic(m.runtime.NewTypeError("field %q not found on message %q", name, msgDesc.FullName()))
	}
	return fd
}

// ---------- Repeated field wrapper ----------

// wrapRepeatedField creates a JS object with array-like methods that
// operates on a repeated protobuf field.
func (m *Module) wrapRepeatedField(msg *dynamicpb.Message, fd protoreflect.FieldDescriptor) *goja.Object {
	obj := m.runtime.NewObject()

	// length — read-only dynamic accessor.
	_ = obj.DefineAccessorProperty("length",
		m.runtime.ToValue(func(goja.FunctionCall) goja.Value {
			return m.runtime.ToValue(msg.Get(fd).List().Len())
		}),
		nil,
		goja.FLAG_FALSE,
		goja.FLAG_TRUE,
	)

	// get(index) — retrieve element at index.
	_ = obj.Set("get", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		idx := int(call.Argument(0).ToInteger())
		list := msg.Get(fd).List()
		if idx < 0 || idx >= list.Len() {
			return goja.Undefined()
		}
		return m.protoValueToGoja(list.Get(idx), fd)
	}))

	// set(index, value) — replace element at index.
	_ = obj.Set("set", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		idx := int(call.Argument(0).ToInteger())
		val := call.Argument(1)
		if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
			panic(m.runtime.NewTypeError("cannot set null/undefined in repeated field"))
		}
		list := msg.Mutable(fd).List()
		if idx < 0 || idx >= list.Len() {
			panic(m.runtime.NewTypeError("index %d out of bounds (length %d)", idx, list.Len()))
		}
		pv, err := m.gojaToProtoValue(val, fd)
		if err != nil {
			panic(m.runtime.NewTypeError(err.Error()))
		}
		list.Set(idx, pv)
		return goja.Undefined()
	}))

	// add(value) — append element.
	_ = obj.Set("add", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		val := call.Argument(0)
		if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
			panic(m.runtime.NewTypeError("cannot add null/undefined to repeated field"))
		}
		list := msg.Mutable(fd).List()
		pv, err := m.gojaToProtoValue(val, fd)
		if err != nil {
			panic(m.runtime.NewTypeError(err.Error()))
		}
		list.Append(pv)
		return goja.Undefined()
	}))

	// clear() — remove all elements.
	_ = obj.Set("clear", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		msg.Mutable(fd).List().Truncate(0)
		return goja.Undefined()
	}))

	// forEach(callback) — iterate elements.
	_ = obj.Set("forEach", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		callback, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(m.runtime.NewTypeError("forEach requires a function"))
		}
		list := msg.Get(fd).List()
		for i := 0; i < list.Len(); i++ {
			val := m.protoValueToGoja(list.Get(i), fd)
			if _, err := callback(goja.Undefined(), val, m.runtime.ToValue(i)); err != nil {
				panic(err)
			}
		}
		return goja.Undefined()
	}))

	return obj
}

// ---------- Map field wrapper ----------

// wrapMapField creates a JS object with Map-like methods that operates
// on a map protobuf field.
func (m *Module) wrapMapField(msg *dynamicpb.Message, fd protoreflect.FieldDescriptor) *goja.Object {
	obj := m.runtime.NewObject()
	keyDesc := fd.MapKey()
	valueDesc := fd.MapValue()

	// size — read-only dynamic accessor.
	_ = obj.DefineAccessorProperty("size",
		m.runtime.ToValue(func(goja.FunctionCall) goja.Value {
			return m.runtime.ToValue(msg.Get(fd).Map().Len())
		}),
		nil,
		goja.FLAG_FALSE,
		goja.FLAG_TRUE,
	)

	// get(key) — retrieve value for key.
	_ = obj.Set("get", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		k := call.Argument(0)
		mk, err := m.gojaToProtoMapKey(k, keyDesc)
		if err != nil {
			panic(m.runtime.NewTypeError(err.Error()))
		}
		protoMap := msg.Get(fd).Map()
		v := protoMap.Get(mk)
		if !v.IsValid() {
			return goja.Undefined()
		}
		return m.protoValueToGoja(v, valueDesc)
	}))

	// set(key, value) — store value for key. Null/undefined removes the entry.
	_ = obj.Set("set", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		k := call.Argument(0)
		v := call.Argument(1)
		mk, err := m.gojaToProtoMapKey(k, keyDesc)
		if err != nil {
			panic(m.runtime.NewTypeError(err.Error()))
		}
		// Null/undefined value means "delete this key" — consistent
		// with setMapFromGoja which skips null/undefined values.
		if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
			msg.Mutable(fd).Map().Clear(mk)
			return goja.Undefined()
		}
		mv, err := m.gojaToProtoValue(v, valueDesc)
		if err != nil {
			panic(m.runtime.NewTypeError(err.Error()))
		}
		msg.Mutable(fd).Map().Set(mk, mv)
		return goja.Undefined()
	}))

	// has(key) — check if key exists.
	_ = obj.Set("has", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		k := call.Argument(0)
		mk, err := m.gojaToProtoMapKey(k, keyDesc)
		if err != nil {
			panic(m.runtime.NewTypeError(err.Error()))
		}
		return m.runtime.ToValue(msg.Get(fd).Map().Has(mk))
	}))

	// delete(key) — remove entry.
	_ = obj.Set("delete", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		k := call.Argument(0)
		mk, err := m.gojaToProtoMapKey(k, keyDesc)
		if err != nil {
			panic(m.runtime.NewTypeError(err.Error()))
		}
		msg.Mutable(fd).Map().Clear(mk)
		return goja.Undefined()
	}))

	// forEach(callback) — iterate entries as (value, key).
	_ = obj.Set("forEach", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		callback, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(m.runtime.NewTypeError("forEach requires a function"))
		}
		protoMap := msg.Get(fd).Map()
		protoMap.Range(func(mk protoreflect.MapKey, v protoreflect.Value) bool {
			jk := m.mapKeyToGoja(mk, keyDesc)
			jv := m.protoValueToGoja(v, valueDesc)
			if _, err := callback(goja.Undefined(), jv, jk); err != nil {
				panic(err)
			}
			return true
		})
		return goja.Undefined()
	}))

	// entries() — return an iterator of [key, value] pairs.
	_ = obj.Set("entries", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		var pairs []goja.Value
		protoMap := msg.Get(fd).Map()
		protoMap.Range(func(mk protoreflect.MapKey, v protoreflect.Value) bool {
			pair := m.runtime.NewArray()
			_ = pair.Set("0", m.mapKeyToGoja(mk, keyDesc))
			_ = pair.Set("1", m.protoValueToGoja(v, valueDesc))
			pairs = append(pairs, pair)
			return true
		})

		// Build a simple iterator object.
		idx := 0
		iter := m.runtime.NewObject()
		_ = iter.Set("next", m.runtime.ToValue(func(goja.FunctionCall) goja.Value {
			result := m.runtime.NewObject()
			if idx >= len(pairs) {
				_ = result.Set("done", true)
				_ = result.Set("value", goja.Undefined())
			} else {
				_ = result.Set("done", false)
				_ = result.Set("value", pairs[idx])
				idx++
			}
			return result
		}))
		return iter
	}))

	return obj
}
