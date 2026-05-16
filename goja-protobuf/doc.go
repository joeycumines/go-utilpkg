// Package gojaprotobuf provides Protocol Buffers support for the [goja]
// JavaScript runtime, enabling JavaScript code to create, manipulate,
// serialize, and deserialize protobuf messages.
//
// # Generated and Dynamic Protobuf
//
// A runtime has one canonical file, message, enum, and extension identity.
// Generated messages retain their concrete Go type and instance. Descriptor
// sets add dynamic types through an atomic, order-independent transaction, so
// JavaScript can also use schemas that were not known when the Go program was
// compiled. Conflicting schemas are rejected without changing the active
// registry snapshot. The first module for a runtime snapshots configured base
// registry membership; later module-owned descriptor loads remain live and
// shared by every module bound to that runtime.
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
//   - int64, sint64, sfixed64 → safe number or BigInt
//   - uint32, fixed32 → number
//   - uint64, fixed64 → safe number or BigInt
//   - float, double → number
//   - bool → boolean
//   - string → string
//   - bytes → Uint8Array
//
// Repeated fields are exposed as array-like objects. Map fields are exposed
// as ES6 Map-like objects.
//
// Setters accept 64-bit integers as safe integer numbers, BigInt values, or
// exact decimal strings. Unsafe, fractional, or non-finite numbers are rejected.
// All operations that touch JavaScript values must execute on the owning Goja
// goroutine.
//
// # Usage
//
//	registry := require.NewRegistry()
//	registry.RegisterNativeModule("protobuf", gojaprotobuf.Require())
//
//	loop, err := eventloop.New()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	runDone := make(chan error, 1)
//	go func() {
//	    runDone <- loop.Run(context.Background())
//	}()
//	rt := goja.New()
//	registry.Enable(rt)
//
//	result := make(chan error, 1)
//	if err := loop.Submit(func() {
//	    _, err := rt.RunString(`
//	        const pb = require('protobuf');
//	        pb.loadDescriptorSet(descriptorBytes);
//	        const MyMsg = pb.messageType('my.package.MyMessage');
//	        const msg = new MyMsg();
//	        msg.set('name', 'hello');
//	        const encoded = pb.encode(msg);
//	    `)
//	    result <- err
//	}); err != nil {
//	    log.Fatal(err)
//	}
//	if err := <-result; err != nil {
//	    log.Fatal(err)
//	}
//	if err := loop.Close(); err != nil {
//	    log.Fatal(err)
//	}
//	if err := <-runDone; err != nil {
//	    log.Fatal(err)
//	}
//
// [goja]: github.com/joeycumines/goja
// [goja_nodejs/require]: github.com/joeycumines/goja_nodejs/require
package gojaprotobuf
