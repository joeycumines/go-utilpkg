package gojaprotojson

import (
	"github.com/dop251/goja"
	"google.golang.org/protobuf/encoding/protojson"
)

// jsMarshal implements protojson.marshal(msg, opts?).
// Returns a JSON string representation of the protobuf message.
func (m *Module) jsMarshal(call goja.FunctionCall) goja.Value {
	msg, err := m.protobuf.UnwrapMessage(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewTypeError("marshal: %s", err))
	}

	opts := m.parseMarshalOptions(call.Argument(1))

	data, err := opts.Marshal(msg)
	if err != nil {
		panic(m.runtime.NewTypeError("marshal: %s", err))
	}

	return m.runtime.ToValue(string(data))
}

// jsFormat implements protojson.format(msg).
// Convenience wrapper: marshal with indent="  ".
func (m *Module) jsFormat(call goja.FunctionCall) goja.Value {
	msg, err := m.protobuf.UnwrapMessage(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewTypeError("format: %s", err))
	}

	opts := protojson.MarshalOptions{
		Multiline: true,
		Indent:    "  ",
		Resolver:  m.protobuf.TypeResolver(),
	}

	data, err := opts.Marshal(msg)
	if err != nil {
		panic(m.runtime.NewTypeError("format: %s", err))
	}

	return m.runtime.ToValue(string(data))
}

// parseMarshalOptions extracts MarshalOptions from a JS options object.
// Supported options:
//   - emitDefaults (bool): emit fields with default values
//   - enumAsNumber (bool): use enum numeric values instead of names
//   - useProtoNames (bool): use proto field names (snake_case) instead of camelCase
//   - indent (string): indentation string; if set, enables multiline
func (m *Module) parseMarshalOptions(val goja.Value) protojson.MarshalOptions {
	opts := protojson.MarshalOptions{
		Resolver: m.protobuf.TypeResolver(),
	}

	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return opts
	}

	obj := val.ToObject(m.runtime)

	if v := obj.Get("emitDefaults"); v != nil && !goja.IsUndefined(v) {
		opts.EmitDefaultValues = v.ToBoolean()
	}
	if v := obj.Get("enumAsNumber"); v != nil && !goja.IsUndefined(v) {
		opts.UseEnumNumbers = v.ToBoolean()
	}
	if v := obj.Get("useProtoNames"); v != nil && !goja.IsUndefined(v) {
		opts.UseProtoNames = v.ToBoolean()
	}
	if v := obj.Get("indent"); v != nil && !goja.IsUndefined(v) {
		opts.Indent = v.String()
		if opts.Indent != "" {
			opts.Multiline = true
		}
	}

	return opts
}
