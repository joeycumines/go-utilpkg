# goja-protobuf

[![Go Reference](https://pkg.go.dev/badge/github.com/joeycumines/goja-protobuf.svg)](https://pkg.go.dev/github.com/joeycumines/goja-protobuf)

Protocol Buffers support for the [goja](https://github.com/dop251/goja) JavaScript runtime. Create, manipulate, serialize, and deserialize protobuf messages from JavaScript running in Go.

## Features

- **Full proto3 support**: All scalar types, enums, nested messages, repeated fields, map fields, oneof groups
- **Binary serialization**: Encode/decode protobuf wire format via `Uint8Array`
- **JSON serialization**: Proto3 canonical JSON including well-known type formats (Timestamp, Duration, Any, etc.)
- **Dynamic descriptors**: Load `.proto` definitions at runtime from serialized `FileDescriptorSet` or `FileDescriptorProto`
- **Type-safe wrappers**: JavaScript message objects with get/set/has/clear methods
- **BigInt support**: 64-bit integers outside the safe integer range automatically use BigInt
- **require() integration**: Standard goja module loading via `require('protobuf')`

## Installation

```sh
go get github.com/joeycumines/goja-protobuf
```

## Quick Start

```go
package main

import (
    "os"

    "github.com/dop251/goja"
    "github.com/dop251/goja_nodejs/require"
    gojaprotobuf "github.com/joeycumines/goja-protobuf"
)

func main() {
    registry := require.NewRegistry()
    registry.RegisterNativeModule("protobuf", gojaprotobuf.Require())

    rt := goja.New()
    registry.Enable(rt)

    // Load pre-compiled descriptor set
    descBytes, _ := os.ReadFile("myproto.pb")
    rt.Set("__descriptorBytes", rt.NewArrayBuffer(descBytes))

    rt.RunString(`
        const pb = require('protobuf');

        // Load proto definitions
        pb.loadDescriptorSet(__descriptorBytes);

        // Create and populate a message
        const MyMsg = pb.messageType('mypackage.MyMessage');
        const msg = new MyMsg();
        msg.set('name', 'hello');
        msg.set('count', 42);

        // Binary serialization
        const encoded = pb.encode(msg);
        const decoded = pb.decode(MyMsg, encoded);
        console.log(decoded.get('name')); // "hello"

        // JSON serialization
        const json = pb.toJSON(msg);
        console.log(JSON.stringify(json));
        const fromJson = pb.fromJSON(MyMsg, json);
    `)
}
```

## JavaScript API

### Descriptor Loading

```javascript
const pb = require('protobuf');

// Load a serialized FileDescriptorSet (protoc --descriptor_set_out)
pb.loadDescriptorSet(uint8ArrayOrArrayBuffer);

// Load a single FileDescriptorProto
pb.loadFileDescriptorProto(uint8ArrayOrArrayBuffer);
```

### Message Types

```javascript
// Look up a message type by fully-qualified name
const MyMsg = pb.messageType('mypackage.MyMessage');
const msg = new MyMsg();

// Field access
msg.set('field_name', value);
msg.get('field_name');
msg.has('field_name');    // boolean
msg.clear('field_name');

// Oneof support
msg.whichOneof('oneof_name');  // returns field name or undefined
msg.clearOneof('oneof_name');

// Type information
msg.get('$type');  // fully-qualified type name
```

### Enum Types

```javascript
const Status = pb.enumType('mypackage.Status');
// Status is a frozen object: { UNKNOWN: 0, ACTIVE: 1, 0: "UNKNOWN", 1: "ACTIVE" }
```

### Repeated Fields

```javascript
const list = msg.get('items');
list.length;          // number of elements
list.get(0);          // get by index
list.set(0, value);   // set by index
list.add(value);      // append
list.clear();         // remove all
list.forEach((val, i) => { ... });

// Set from array
msg.set('items', [value1, value2]);
```

### Map Fields

```javascript
const map = msg.get('labels');
map.size;              // number of entries
map.get('key');        // lookup
map.set('key', value); // insert/update
map.has('key');        // boolean
map.delete('key');     // remove
map.forEach((key, value) => { ... });
map.entries();         // iterator

// Set from object or Map
msg.set('labels', { key1: 'val1', key2: 'val2' });
msg.set('labels', new Map([['key1', 'val1']]));
```

### Serialization

```javascript
// Binary (wire format)
const bytes = pb.encode(msg);      // returns Uint8Array
const msg2 = pb.decode(MyMsg, bytes);

// JSON (proto3 canonical)
const json = pb.toJSON(msg);       // returns plain JS object
const msg3 = pb.fromJSON(MyMsg, json);
```

## Type Conversions

| Protobuf Type | JavaScript Type |
|---|---|
| int32, sint32, sfixed32 | number |
| int64, sint64, sfixed64 | number (BigInt if outside safe range) |
| uint32, fixed32 | number |
| uint64, fixed64 | number (BigInt if outside safe range) |
| float, double | number |
| bool | boolean |
| string | string |
| bytes | Uint8Array |
| enum | number (set accepts number or string name) |
| message | wrapped message object |
| repeated | array-like object |
| map | Map-like object |

## Go API

For programmatic use from Go code, the `Module` type provides direct access:

```go
mod, err := gojaprotobuf.New(runtime, gojaprotobuf.WithResolver(myTypes))
if err != nil {
    log.Fatal(err)
}

// Load descriptors from Go
names, err := mod.LoadDescriptorSetBytes(data)

// Wrap/unwrap messages for Go↔JS interop
jsObj := mod.WrapMessage(dynamicMsg)
goMsg, err := mod.UnwrapMessage(jsValue)

// Find descriptors
desc, err := mod.FindDescriptor("mypackage.MyMessage")
```

## License

[MIT](LICENSE)
