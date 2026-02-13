# goja-protobuf Design Document

## Overview

`goja-protobuf` provides Protocol Buffers support for the [goja](https://github.com/dop251/goja)
JavaScript runtime. It enables JavaScript code to load protobuf descriptors, create and manipulate
dynamic messages, and serialize/deserialize using both binary and JSON formats.

The module is exposed through the `goja_nodejs/require` module system as `require('protobuf')`.

## Go API Surface

### Types

```go
// Module is the core type that bridges protobuf functionality to the goja runtime.
// Each Module instance is bound to a single goja.Runtime.
type Module struct {
    // unexported fields:
    // runtime    *goja.Runtime
    // resolver   *protoregistry.Types   — where to find types (default: GlobalTypes)
    // files      *protoregistry.Files   — where to find file descriptors (default: GlobalFiles)
    // localTypes *protoregistry.Types   — types registered via loadDescriptorSet
    // localFiles *protoregistry.Files   — file descriptors registered via loadDescriptorSet
}

// Option configures a Module instance. Options are applied during construction.
type Option interface {
    applyOption(*moduleOptions) error
}
```

### Functions

```go
// New creates a new Module bound to the given goja runtime.
// Panics if runtime is nil (programming error / invariant violation).
// Returns error if option validation fails (user-input error).
func New(runtime *goja.Runtime, opts ...Option) (*Module, error)

// Require returns a require.ModuleLoader that initialises the protobuf
// module when loaded by a goja.Runtime. The integrator registers the
// loader under whatever module name they choose.
func Require(opts ...Option) require.ModuleLoader

// WithResolver configures a custom type resolver. Defaults to protoregistry.GlobalTypes.
func WithResolver(resolver *protoregistry.Types) Option

// WithFiles configures a custom file registry. Defaults to protoregistry.GlobalFiles.
func WithFiles(files *protoregistry.Files) Option
```

## JS API Surface

All functionality is accessed via `require('protobuf')`:

```javascript
const pb = require('protobuf');
```

### Descriptor Loading

```javascript
// Loads a serialized google.protobuf.FileDescriptorSet (Uint8Array or ArrayBuffer).
// Registers all contained message types, enum types, and services.
// Returns an array of fully-qualified type names that were registered.
pb.loadDescriptorSet(bytes)

// Loads a single serialized google.protobuf.FileDescriptorProto (Uint8Array or ArrayBuffer).
// Convenience method for loading individual .proto files.
pb.loadFileDescriptorProto(bytes)
```

### Type Lookup

```javascript
// Returns a MessageType constructor for the given fully-qualified name.
// The constructor creates new message instances: new MsgType()
// Throws TypeError if the type is not found.
const MyMsg = pb.messageType('my.package.MyMessage');
const msg = new MyMsg();

// Returns an EnumType object mapping name↔number for the given fully-qualified name.
// Throws TypeError if the type is not found.
const MyEnum = pb.enumType('my.package.MyEnum');
// MyEnum.VALUE_NAME → 1
// MyEnum[1] → 'VALUE_NAME'
```

### Serialization

```javascript
// Encodes a message to binary format. Returns Uint8Array.
const bytes = pb.encode(msg);

// Decodes binary data into a message of the given type.
// msgType is a constructor from messageType().
const msg = pb.decode(MyMsg, bytes);

// Converts a message to its proto3 JSON representation (plain JS object).
const obj = pb.toJSON(msg);

// Creates a message from a proto3 JSON object.
// msgType is a constructor from messageType().
const msg = pb.fromJSON(MyMsg, obj);
```

## Type Mapping

### Scalar Types

| Proto Type | JS Type | Notes |
|---|---|---|
| `int32`, `sint32`, `sfixed32` | `number` | |
| `int64`, `sint64`, `sfixed64` | `number` or `BigInt` | BigInt for values outside safe integer range |
| `uint32`, `fixed32` | `number` | |
| `uint64`, `fixed64` | `number` or `BigInt` | BigInt for values outside safe integer range |
| `float`, `double` | `number` | |
| `bool` | `boolean` | |
| `string` | `string` | |
| `bytes` | `Uint8Array` | |

### Composite Types

| Proto Type | JS Type | Notes |
|---|---|---|
| Message field | Wrapped message object | Recursive wrapping |
| Enum field | `number` | Numeric enum value; set accepts number or string name |
| Repeated field | Array-like object | `add()`, `get(i)`, `set(i, v)`, `length`, `clear()`, iterable |
| Map field | Map-like object | `get(k)`, `set(k, v)`, `has(k)`, `delete(k)`, `size`, `forEach()`, `entries()` |
| Oneof group | Via `whichOneof(name)` | Returns set field name or `undefined` |

### 64-bit Integer Strategy

For 64-bit integer fields (`int64`, `uint64`, etc.):
- **Read**: If the value fits within `Number.MIN_SAFE_INTEGER` to `Number.MAX_SAFE_INTEGER`, return `number`. Otherwise return `BigInt`.
- **Write**: Accept both `number` and `BigInt`. Validate range for the target proto type.

## Message Wrapper Design

Messages are backed by `dynamicpb.Message` and exposed to JS as objects with a field-access API:

```javascript
const msg = new MyMsg();

// Field access
msg.get('field_name')          // returns field value (converted to JS type)
msg.set('field_name', value)   // sets field value (converted from JS type)
msg.has('field_name')          // returns boolean (meaningful for optional/message fields)
msg.clear('field_name')        // clears field to default value

// Oneof support
msg.whichOneof('oneof_name')   // returns field name string or undefined
msg.clearOneof('oneof_name')   // clears whichever oneof field is set

// Identity
msg.$type                      // fully-qualified message type name (string, read-only)
```

### Field Name Resolution

Field names use the proto field name (snake_case), not the JSON name (camelCase).
This matches the protobuf reflection API and avoids ambiguity.

### Nested Message Fields

- `get()` on a message field returns a wrapped sub-message (lazy allocation).
- `set()` accepts a wrapped message or a plain JS object. Plain objects are
  converted field-by-field using the target message's descriptor.
- Setting `null` or `undefined` clears the field.

### Default Values

- Unset scalar fields return proto3 default values (zero, empty string, false).
- Unset message fields return `null` (not a default message).
- `has()` returns `false` for unset fields in proto3 (except optional/message fields).

## Options Pattern

Follows the interface-based option pattern from `inprocgrpc/options.go`:

```go
// moduleOptions holds resolved configuration.
type moduleOptions struct {
    resolver *protoregistry.Types
    files    *protoregistry.Files
}

// Option configures a Module instance.
type Option interface {
    applyOption(*moduleOptions) error
}

// optionFunc implements Option via a closure.
type optionFunc struct {
    fn func(*moduleOptions) error
}

func (o *optionFunc) applyOption(opts *moduleOptions) error {
    return o.fn(opts)
}
```

### Defaults

- `resolver`: `protoregistry.GlobalTypes` — the process-wide protobuf type registry.
- `files`: `protoregistry.GlobalFiles` — the process-wide protobuf file registry.

Custom registries allow isolation between Module instances, useful when
multiple JS runtimes need independent type registries.

## Error Handling Strategy

### Go-side Errors

- `New()` **panics** on nil runtime (programming error / invariant violation).
- `New()` **returns error** for option validation failures (user-input errors).
- `Require()` captures options for deferred application. If options fail during
  `require(...)`, the error is thrown as a JS exception (via goja panic).

### JS-side Errors

All JS-facing functions throw typed errors:

| Condition | Error Type | Example |
|---|---|---|
| Type not found | `TypeError` | `messageType('no.Such.Type')` |
| Invalid argument type | `TypeError` | `encode('not a message')` |
| Decode failure | `Error` | `decode(MyMsg, corruptBytes)` |
| Descriptor parse failure | `Error` | `loadDescriptorSet(invalidBytes)` |
| Field not found | `TypeError` | `msg.get('no_such_field')` |
| Type mismatch on set | `TypeError` | `msg.set('int_field', 'string')` |
| Not yet implemented | `TypeError` | Stub methods before full implementation |

Errors are thrown using goja's panic mechanism (`runtime.NewTypeError(...)` or
`runtime.NewGoError(err)`), which goja catches and converts to JS exceptions.
This integrates naturally with try/catch in JS code.

### Invariant: No Silent Failures

- No function silently ignores errors.
- Failed operations always throw.
- Invalid states are detected eagerly, not lazily.

## Module Lifecycle

1. **Registration**: The integrator calls
   `registry.RegisterNativeModule("protobuf", gojaprotobuf.Require(opts...))`,
   choosing their own module name.

2. **Instantiation**: When JS calls `require('protobuf')`, the loader creates
   a new `Module` instance using the runtime and the captured options.

3. **Descriptor Loading**: JS code calls `loadDescriptorSet()` or
   `loadFileDescriptorProto()` to register protobuf type definitions.
   Types are registered into the module's `localTypes` and `localFiles` registries.

4. **Type Resolution**: `messageType()` and `enumType()` first check
   `localTypes`, then fall back to the configured `resolver` (default: GlobalTypes).

5. **Usage**: JS code creates, populates, and serializes messages using the
   wrapper API and serialization functions.

## Dependencies

| Dependency | Purpose |
|---|---|
| `github.com/dop251/goja` | JavaScript runtime for Go |
| `github.com/dop251/goja_nodejs/require` | Node.js-style require() module system |
| `google.golang.org/protobuf/proto` | Binary serialization |
| `google.golang.org/protobuf/encoding/protojson` | JSON serialization |
| `google.golang.org/protobuf/reflect/protodesc` | Building file descriptors from protos |
| `google.golang.org/protobuf/reflect/protoreflect` | Protobuf reflection API |
| `google.golang.org/protobuf/reflect/protoregistry` | Type and file registries |
| `google.golang.org/protobuf/types/descriptorpb` | FileDescriptorSet/FileDescriptorProto types |
| `google.golang.org/protobuf/types/dynamicpb` | Dynamic message creation |
