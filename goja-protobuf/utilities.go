package gojaprotobuf

import (
	"github.com/dop251/goja"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/dynamicpb"
)

// jsEquals is the JS-facing implementation of pb.equals(msg1, msg2).
// It compares two wrapped protobuf messages for structural equality
// using [proto.Equal]. Both arguments must be wrapped messages of
// the same type.
func (m *Module) jsEquals(call goja.FunctionCall) goja.Value {
	msg1, err := m.unwrapMessage(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewTypeError("equals: first argument: %s", err))
	}

	msg2, err := m.unwrapMessage(call.Argument(1))
	if err != nil {
		panic(m.runtime.NewTypeError("equals: second argument: %s", err))
	}

	return m.runtime.ToValue(proto.Equal(msg1, msg2))
}

// jsClone is the JS-facing implementation of pb.clone(msg).
// It creates a deep copy of a wrapped protobuf message using
// [proto.Clone]. The returned message is independent — mutating the
// clone does not affect the original.
func (m *Module) jsClone(call goja.FunctionCall) goja.Value {
	msg, err := m.unwrapMessage(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewTypeError("clone: %s", err))
	}

	cloned := proto.Clone(msg).(*dynamicpb.Message)
	return m.wrapMessage(cloned)
}

// jsIsMessage is the JS-facing implementation of
// pb.isMessage(value[, typeName]).
//
// With one argument, it returns true if the value is a wrapped protobuf
// message object. With two arguments, it also checks that the message's
// fully-qualified type name matches the given string.
func (m *Module) jsIsMessage(call goja.FunctionCall) goja.Value {
	msg, err := m.unwrapMessage(call.Argument(0))
	if err != nil {
		return m.runtime.ToValue(false)
	}

	// If a second arg is provided and not undefined, check the type name.
	typeArg := call.Argument(1)
	if typeArg != nil && !goja.IsUndefined(typeArg) && !goja.IsNull(typeArg) {
		wantName := typeArg.String()
		gotName := string(msg.Descriptor().FullName())
		return m.runtime.ToValue(gotName == wantName)
	}

	return m.runtime.ToValue(true)
}

// jsIsFieldSet is the JS-facing implementation of
// pb.isFieldSet(msg, fieldName). It returns true if the named field has
// been explicitly set on the message. This mirrors protobuf-es's
// isFieldSet(msg, Schema.field.x) as a top-level convenience function.
func (m *Module) jsIsFieldSet(call goja.FunctionCall) goja.Value {
	msg, err := m.unwrapMessage(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewTypeError("isFieldSet: %s", err))
	}

	name := call.Argument(1).String()
	fd := m.resolveField(msg.Descriptor(), name)
	return m.runtime.ToValue(msg.Has(fd))
}

// jsClearField is the JS-facing implementation of
// pb.clearField(msg, fieldName). It resets the named field to its
// default value. This mirrors protobuf-es's clearField as a top-level
// convenience function.
func (m *Module) jsClearField(call goja.FunctionCall) goja.Value {
	msg, err := m.unwrapMessage(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewTypeError("clearField: %s", err))
	}

	name := call.Argument(1).String()
	fd := m.resolveField(msg.Descriptor(), name)
	msg.Clear(fd)
	return goja.Undefined()
}
