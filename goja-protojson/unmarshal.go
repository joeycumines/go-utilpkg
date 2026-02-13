package gojaprotojson

import (
	"github.com/dop251/goja"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// jsUnmarshal implements protojson.unmarshal(typeName, jsonStr, opts?).
// Returns a wrapped protobuf message.
func (m *Module) jsUnmarshal(call goja.FunctionCall) goja.Value {
	typeArg := call.Argument(0)
	if typeArg == nil || goja.IsUndefined(typeArg) || goja.IsNull(typeArg) {
		panic(m.runtime.NewTypeError("unmarshal: type name is required"))
	}
	typeName := typeArg.String()

	jsonArg := call.Argument(1)
	if jsonArg == nil || goja.IsUndefined(jsonArg) || goja.IsNull(jsonArg) {
		panic(m.runtime.NewTypeError("unmarshal: JSON string is required"))
	}
	jsonStr := jsonArg.String()

	desc, err := m.protobuf.FindDescriptor(protoreflect.FullName(typeName))
	if err != nil {
		panic(m.runtime.NewTypeError("unmarshal: unknown type %q: %s", typeName, err))
	}

	msgDesc, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		panic(m.runtime.NewTypeError("unmarshal: %q is not a message type", typeName))
	}

	opts := m.parseUnmarshalOptions(call.Argument(2))

	msg := dynamicpb.NewMessage(msgDesc)
	if err := opts.Unmarshal([]byte(jsonStr), msg); err != nil {
		panic(m.runtime.NewTypeError("unmarshal: %s", err))
	}

	return m.protobuf.WrapMessage(msg)
}

// parseUnmarshalOptions extracts UnmarshalOptions from a JS options object.
// Supported options:
//   - discardUnknown (bool): silently discard unknown fields
func (m *Module) parseUnmarshalOptions(val goja.Value) protojson.UnmarshalOptions {
	opts := protojson.UnmarshalOptions{
		Resolver: m.protobuf.TypeResolver(),
	}

	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return opts
	}

	obj := val.ToObject(m.runtime)

	if v := obj.Get("discardUnknown"); v != nil && !goja.IsUndefined(v) {
		opts.DiscardUnknown = v.ToBoolean()
	}

	return opts
}
