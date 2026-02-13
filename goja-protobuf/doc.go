// Package gojaprotobuf provides Protocol Buffers support for the [goja]
// JavaScript runtime, enabling JavaScript code to create, manipulate,
// serialize, and deserialize protobuf messages.
//
// # Why Dynamic Protobuf?
//
// This package uses dynamicpb.Message and dynamic descriptor loading to enable
// JavaScript code to operate on protobuf types without requiring Go code
// regeneration. When evaluating external JavaScript code, the protobuf types
// used by that code may not be known at Go compile-time. By loading descriptors
// at runtime from serialized FileDescriptorSet bytes, JavaScript can construct
// and manipulate protobuf messages without requiring the Go module to be
// recompiled with updated generated stubs. This provides the same flexibility
// that protobuf-es brings to JavaScript environments.
//
// # Overview
//
// The module exposes protobuf functionality through the [goja_nodejs/require]
// module system, making it available to JavaScript code via:
//
//	const pb = require('protobuf');
//
// # JavaScript API
//
// Descriptor loading:
//   - pb.loadDescriptorSet(bytes) — loads a serialized FileDescriptorSet
//   - pb.loadFileDescriptorProto(bytes) — loads a single FileDescriptorProto
//
// Message types:
//   - pb.messageType(fullName) — looks up a message type by fully-qualified name
//   - pb.enumType(fullName) — looks up an enum type by fully-qualified name
//
// Serialization:
//   - pb.encode(msg) — encodes a message to binary (Uint8Array)
//   - pb.decode(msgType, bytes) — decodes binary data to a message
//   - pb.toJSON(msg) — converts a message to its proto3 JSON representation
//   - pb.fromJSON(msgType, obj) — creates a message from a proto3 JSON object
//
// Message utilities:
//   - pb.equals(msg1, msg2) — compares two messages for structural equality
//   - pb.clone(msg) — creates a deep copy of a message
//   - pb.isMessage(value[, typeName]) — type guard for protobuf messages
//   - pb.isFieldSet(msg, fieldName) — checks whether a field has been explicitly set
//   - pb.clearField(msg, fieldName) — resets a field to its default value
//
// Well-known type helpers (protobuf-es aligned):
//   - pb.timestampNow() — creates a Timestamp for the current time
//   - pb.timestampFromDate(date) — creates a Timestamp from a JS Date
//   - pb.timestampDate(ts) — converts a Timestamp to a JS Date
//   - pb.timestampFromMs(ms) — creates a Timestamp from epoch milliseconds
//   - pb.timestampMs(ts) — extracts epoch milliseconds from a Timestamp
//   - pb.durationFromMs(ms) — creates a Duration from milliseconds
//   - pb.durationMs(dur) — extracts milliseconds from a Duration
//   - pb.anyPack(msgType, msg) — wraps a message into an Any
//   - pb.anyUnpack(any, msgType) — extracts a message from an Any
//   - pb.anyIs(any, typeNameOrMsgType) — checks if an Any contains a given type
//
// # Message Wrapper
//
// Messages returned by messageType constructors and decode are JavaScript
// objects with the following methods:
//   - msg.get(fieldName) — gets a field value
//   - msg.set(fieldName, value) — sets a field value
//   - msg.has(fieldName) — checks whether a field is set
//   - msg.clear(fieldName) — clears a field
//   - msg.whichOneof(name) — returns the set field name in a oneof group
//
// # Type Mapping
//
// Scalar protobuf types are mapped to JavaScript types:
//   - int32, sint32, sfixed32 → number
//   - int64, sint64, sfixed64 → number (or BigInt for large values)
//   - uint32, fixed32 → number
//   - uint64, fixed64 → number (or BigInt for large values)
//   - float, double → number
//   - bool → boolean
//   - string → string
//   - bytes → Uint8Array
//
// Repeated fields are exposed as array-like objects. Map fields are exposed
// as ES6 Map-like objects.
//
// # Usage
//
//	registry := require.NewRegistry()
//	registry.RegisterNativeModule("protobuf", gojaprotobuf.Require())
//
//	loop, _ := eventloop.New()
//	defer loop.Close()
//	rt := goja.New()
//	registry.Enable(rt)
//
//	loop.Submit(func() {
//	    rt.RunString(`
//	        const pb = require('protobuf');
//	        pb.loadDescriptorSet(descriptorBytes);
//	        const MyMsg = pb.messageType('my.package.MyMessage');
//	        const msg = new MyMsg();
//	        msg.set('name', 'hello');
//	        const encoded = pb.encode(msg);
//	    `)
//	})
//
// [goja]: github.com/dop251/goja
// [goja_nodejs/require]: github.com/dop251/goja_nodejs/require
package gojaprotobuf
