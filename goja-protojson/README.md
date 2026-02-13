# goja-protojson

Protocol Buffers JSON encoding and decoding for [Goja](https://github.com/dop251/goja) JavaScript engine.

This module wraps Go's standard
[`protojson`](https://pkg.go.dev/google.golang.org/protobuf/encoding/protojson)
package and exposes `marshal`, `unmarshal`, and `format` functions to
JavaScript code running in Goja.

## Relationship to goja-protobuf

[`goja-protobuf`](../goja-protobuf) provides `toJSON` and `fromJSON`
methods that convert between protobuf messages and native JavaScript
objects. In contrast, `goja-protojson` operates on **JSON strings**,
giving you direct control over the canonical Proto JSON wire format
including options like `emitDefaults`, `enumAsNumber`, and
`useProtoNames`.

Use `goja-protojson` when you need to produce or consume Proto JSON text
(for example, for REST APIs or logging). Use `goja-protobuf`'s
`toJSON`/`fromJSON` when you want to work with JS objects in memory.

## Installation

```bash
go get github.com/joeycumines/goja-protojson
```

## Usage

### Go Setup

```go
import (
    "github.com/dop251/goja"
    gojaprotobuf "github.com/joeycumines/goja-protobuf"
    gojaprotojson "github.com/joeycumines/goja-protojson"
)

rt := goja.New()

// Create protobuf module and load descriptors
pb, _ := gojaprotobuf.New(rt)
pb.LoadDescriptorSetBytes(descriptorBytes)

// Create protojson module
pj, _ := gojaprotojson.New(rt, gojaprotojson.WithProtobuf(pb))

// Wire exports
pbObj := rt.NewObject()
pb.SetupExports(pbObj)
rt.Set("pb", pbObj)

pjObj := rt.NewObject()
pj.SetupExports(pjObj)
rt.Set("protojson", pjObj)
```

Or use the `require()` pattern:

```go
registry := require.NewRegistry()
registry.RegisterNativeModule("protobuf", gojaprotobuf.Require())
registry.RegisterNativeModule("protojson", gojaprotojson.Require(
    gojaprotojson.WithProtobuf(pbModule),
))
```

### JavaScript API

#### `marshal(msg, opts?)`

Serializes a protobuf message to a JSON string.

```javascript
var Person = pb.messageType("example.Person");
var msg = new Person();
msg.set("name", "Alice");
msg.set("age", 30);

var json = protojson.marshal(msg);
// '{"name":"Alice", "age":30}'

// With options:
protojson.marshal(msg, {
    emitDefaults: true,   // Include fields with default values
    enumAsNumber: true,   // Use numeric enum values
    useProtoNames: true,  // Use snake_case field names
    indent: "  ",         // Pretty-print with indentation
});
```

#### `unmarshal(typeName, jsonStr, opts?)`

Deserializes a JSON string into a protobuf message.

```javascript
var msg = protojson.unmarshal("example.Person", '{"name":"Bob","age":25}');
msg.get("name"); // "Bob"
msg.get("age");  // 25

// With options:
protojson.unmarshal("example.Person", jsonStr, {
    discardUnknown: true,  // Silently ignore unknown fields
});
```

#### `format(msg)`

Convenience function: marshals the message with two-space indentation
and multiline output.

```javascript
var pretty = protojson.format(msg);
// {
//   "name": "Alice",
//   "age": 30
// }
```

## Marshal Options

| Option          | Type   | Description                                        |
| --------------- | ------ | -------------------------------------------------- |
| `emitDefaults`  | bool   | Include fields that have their default values       |
| `enumAsNumber`  | bool   | Emit enum values as numbers instead of strings      |
| `useProtoNames` | bool   | Use original proto field names (snake_case)         |
| `indent`        | string | Indentation string; enables multiline when non-empty|

## Unmarshal Options

| Option           | Type | Description                              |
| ---------------- | ---- | ---------------------------------------- |
| `discardUnknown` | bool | Silently discard unknown JSON fields      |

## Related Modules

- [goja-protobuf](../goja-protobuf) — Protobuf message creation, field access, encoding
- [goja-grpc](../goja-grpc) — gRPC client/server for Goja
- [inprocgrpc](../inprocgrpc) — In-process gRPC transport

## License

See [LICENSE](LICENSE).
