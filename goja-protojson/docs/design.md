# goja-protojson Design

## Overview

`goja-protojson` provides Protocol Buffers JSON encoding/decoding for the Goja JavaScript engine. It wraps Go's `google.golang.org/protobuf/encoding/protojson` package and exposes a clean JS API modelled after protobuf-es JSON serialization.

## Distinction from goja-protobuf toJSON/fromJSON

`goja-protobuf` has `toJSON(msg)` → JS object and `fromJSON(typeName, obj)` → message, which convert between protobuf messages and native JS objects.

`goja-protojson` operates on **JSON strings** directly:
- `marshal(msg)` → JSON string (for wire serialization)
- `unmarshal(typeName, jsonStr)` → message (from wire deserialization)

This is the canonical ProtoJSON format per the [Proto3 JSON Mapping spec](https://protobuf.dev/programming-guides/proto3/#json).

## JS API

```js
const protojson = require('protojson');

// Serialize message to ProtoJSON string
const jsonStr = protojson.marshal(msg);
const jsonStr = protojson.marshal(msg, {
  emitDefaults: true,    // emit fields with default values
  enumAsNumber: true,     // use enum numeric values instead of names
  useProtoNames: true,    // use proto field names (snake_case) instead of camelCase
  indent: '  '            // indentation string
});

// Deserialize ProtoJSON string to message
const msg = protojson.unmarshal('my.package.MyMessage', jsonStr);
const msg = protojson.unmarshal('my.package.MyMessage', jsonStr, {
  discardUnknown: true    // ignore unknown fields instead of erroring
});

// Pretty-print (convenience: marshal with indent='  ')
const prettyJson = protojson.format(msg);
```

## Go API

```go
// Module provides ProtoJSON encoding/decoding for a goja.Runtime.
type Module struct { ... }

// New creates a new Module.
func New(runtime *goja.Runtime, opts ...Option) (*Module, error)

// Option configures module behavior.
type Option interface { ... }

// WithProtobuf sets the protobuf module for type resolution.
func WithProtobuf(pb *gojaprotobuf.Module) Option

// Require returns a goja_nodejs require.ModuleLoader.
func Require(opts ...Option) func(runtime *goja.Runtime, module *goja.Object)

// SetupExports wires the module's JS API onto the given exports object.
func (m *Module) SetupExports(exports *goja.Object)
```

## Implementation Strategy

### marshal
1. Extract `*dynamicpb.Message` from goja wrapper via `protobuf.UnwrapMessage(val)`
2. Configure `protojson.MarshalOptions` from JS options object
3. Call `protojson.MarshalOptions.Marshal(msg)` → `[]byte`
4. Return string to JS

### unmarshal
1. Get message type name (string) and JSON string from JS args
2. Resolve message descriptor via protobuf module: `protobuf.FindDescriptor(name)`
3. Create `dynamicpb.NewMessage(descriptor)`
4. Configure `protojson.UnmarshalOptions` from JS options object
5. Call `protojson.UnmarshalOptions.Unmarshal([]byte(jsonStr), msg)`
6. Return wrapped message via `protobuf.WrapMessage(msg)`

### format
Sugar for `marshal(msg, {indent: '  '})`.

## Dependencies
- `github.com/dop251/goja`
- `github.com/joeycumines/goja-protobuf` — for message wrapping and type resolution
- `google.golang.org/protobuf/encoding/protojson`
