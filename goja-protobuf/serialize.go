package gojaprotobuf

import (
	"github.com/dop251/goja"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/dynamicpb"
)

// jsEncode is the JS-facing implementation of pb.encode(msg).
// It serialises a wrapped message to binary format and returns a
// Uint8Array.
func (m *Module) jsEncode(call goja.FunctionCall) goja.Value {
	msg, err := m.unwrapMessage(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewTypeError("encode: %s", err))
	}

	data, err := proto.Marshal(msg)
	if err != nil {
		panic(m.runtime.NewGoError(err))
	}

	return m.newUint8Array(data)
}

// jsDecode is the JS-facing implementation of pb.decode(msgType, bytes).
// It deserialises binary data into a new message of the given type.
// msgType must be a constructor returned by messageType().
func (m *Module) jsDecode(call goja.FunctionCall) goja.Value {
	msgDesc, err := m.extractMessageDesc(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewTypeError("decode: %s", err))
	}

	data, err := m.extractBytes(call.Argument(1))
	if err != nil {
		panic(m.runtime.NewTypeError("decode: %s", err))
	}

	msg := dynamicpb.NewMessage(msgDesc)
	if err := proto.Unmarshal(data, msg); err != nil {
		panic(m.runtime.NewGoError(err))
	}

	return m.wrapMessage(msg)
}

// jsToJSON is the JS-facing implementation of pb.toJSON(msg).
// It converts a wrapped message to its proto3 JSON representation
// (as a plain JS object).
func (m *Module) jsToJSON(call goja.FunctionCall) goja.Value {
	msg, err := m.unwrapMessage(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewTypeError("toJSON: %s", err))
	}

	opts := protojson.MarshalOptions{
		Resolver: m.typeResolver(),
	}
	jsonBytes, err := opts.Marshal(msg)
	if err != nil {
		panic(m.runtime.NewGoError(err))
	}

	// Use JSON.parse to convert the JSON string into a JS object.
	jsonParseVal := m.runtime.Get("JSON").ToObject(m.runtime).Get("parse")
	parseFn, ok := goja.AssertFunction(jsonParseVal)
	if !ok {
		panic(m.runtime.NewTypeError("toJSON: JSON.parse is not available"))
	}
	result, err := parseFn(goja.Undefined(), m.runtime.ToValue(string(jsonBytes)))
	if err != nil {
		panic(m.runtime.NewGoError(err))
	}
	return result
}

// jsFromJSON is the JS-facing implementation of
// pb.fromJSON(msgType, obj). It creates a message from a proto3 JSON
// object. msgType must be a constructor returned by messageType().
func (m *Module) jsFromJSON(call goja.FunctionCall) goja.Value {
	msgDesc, err := m.extractMessageDesc(call.Argument(0))
	if err != nil {
		panic(m.runtime.NewTypeError("fromJSON: %s", err))
	}

	obj := call.Argument(1)

	// Use JSON.stringify to convert the JS object to a JSON string.
	jsonStringifyVal := m.runtime.Get("JSON").ToObject(m.runtime).Get("stringify")
	stringifyFn, ok := goja.AssertFunction(jsonStringifyVal)
	if !ok {
		panic(m.runtime.NewTypeError("fromJSON: JSON.stringify is not available"))
	}
	jsonStr, err := stringifyFn(goja.Undefined(), obj)
	if err != nil {
		panic(m.runtime.NewGoError(err))
	}

	msg := dynamicpb.NewMessage(msgDesc)
	uOpts := protojson.UnmarshalOptions{
		Resolver:       m.typeResolver(),
		DiscardUnknown: true,
	}
	if err := uOpts.Unmarshal([]byte(jsonStr.String()), msg); err != nil {
		panic(m.runtime.NewGoError(err))
	}

	return m.wrapMessage(msg)
}
