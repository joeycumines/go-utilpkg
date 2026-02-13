# protobuf-es v2 Feature Alignment

This document maps protobuf-es v2 features to their goja-protobuf equivalents.

## Reference

- protobuf-es: https://github.com/bufbuild/protobuf-es (v2)
- goja-protobuf: Go-native Protobuf bindings for Goja JS runtime using dynamicpb

## Core Design Differences

| Aspect | protobuf-es | goja-protobuf |
|--------|------------|---------------|
| Schema source | Code-generated `*Schema` exports | Runtime descriptor loading via `loadDescriptorSet`/`loadFileDescriptorProto` |
| Message creation | `create(Schema, init?)` | `new (messageType(fullName))()` constructor |
| Field access | Plain property access (`msg.firstName`) | Method-based: `msg.get("first_name")`, `msg.set("first_name", v)` |
| Field names | lowerCamelCase | Proto source names (snake_case) |
| Type identity | `$typeName` string property | `$type` read-only accessor |

## Feature Matrix

### Descriptor/Registry Loading

| protobuf-es | goja-protobuf | Status |
|-------------|---------------|--------|
| `createFileRegistry(FileDescriptorSet)` | `loadDescriptorSet(bytes)` | ✅ Implemented |
| `createRegistry(Schema...)` | N/A (auto-registered on load) | ✅ Covered |
| Generated `file_*` exports | `loadFileDescriptorProto(bytes)` | ✅ Implemented |

### Message Construction

| protobuf-es | goja-protobuf | Status |
|-------------|---------------|--------|
| `create(Schema)` | `new (messageType(fullName))()` | ✅ Implemented |
| `create(Schema, init)` | Set fields via `msg.set()` after construction | ✅ Implemented (different style) |

### Serialization

| protobuf-es | goja-protobuf | Status |
|-------------|---------------|--------|
| `toBinary(Schema, msg)` | `encode(msg)` | ✅ Implemented |
| `fromBinary(Schema, bytes)` | `decode(msgType, bytes)` | ✅ Implemented |
| `toJson(Schema, msg)` | `toJSON(msg)` | ✅ Implemented |
| `fromJson(Schema, json)` | `fromJSON(msgType, obj)` | ✅ Implemented |
| `toJsonString(Schema, msg)` | `JSON.stringify(toJSON(msg))` | ✅ Composable |
| `fromJsonString(Schema, str)` | `fromJSON(msgType, JSON.parse(str))` | ✅ Composable |
| `mergeFromBinary(Schema, target, bytes)` | N/A | ❌ Not planned |
| `mergeFromJson(Schema, target, json)` | N/A | ❌ Not planned |
| Size-delimited streams | N/A | ❌ Not planned |

### Message Utilities

| protobuf-es | goja-protobuf | Status |
|-------------|---------------|--------|
| `equals(Schema, a, b)` | `equals(msg1, msg2)` | ✅ Implemented |
| `clone(Schema, msg)` | `clone(msg)` | ✅ Implemented |
| `isMessage(val, Schema?)` | `isMessage(val, typeName?)` | ✅ Implemented |

### Field Presence & Mutation

| protobuf-es | goja-protobuf | Status |
|-------------|---------------|--------|
| `isFieldSet(msg, Schema.field.x)` | `msg.has("field_name")` | ✅ Implemented |
| `clearField(msg, Schema.field.x)` | `msg.clear("field_name")` | ✅ Implemented |

### Enum Support

| protobuf-es | goja-protobuf | Status |
|-------------|---------------|--------|
| Generated TS enum | `enumType(fullName)` frozen object | ✅ Implemented |
| Bidirectional name↔number | name→number, number→name | ✅ Implemented |

### Oneof Support

| protobuf-es | goja-protobuf | Status |
|-------------|---------------|--------|
| Discriminated union `{ case, value }` | `msg.whichOneof("name")` + `msg.get()`/`msg.set()` | ✅ Implemented |
| N/A | `msg.clearOneof("name")` | ✅ Implemented (bonus) |

### Message Wrapper API

| protobuf-es | goja-protobuf | Status |
|-------------|---------------|--------|
| `msg.$typeName` | `msg.$type` (read-only accessor) | ✅ Implemented |
| `msg.field` (property) | `msg.get("field")` | ✅ Implemented (different style) |
| `msg.field = v` (assignment) | `msg.set("field", v)` | ✅ Implemented (different style) |

### Repeated Fields

| protobuf-es | goja-protobuf | Status |
|-------------|---------------|--------|
| `msg.field` → `T[]` | `msg.get("field")` → wrapper | ✅ Implemented |
| Array methods | `.get(i)`, `.set(i,v)`, `.add(v)`, `.clear()`, `.forEach(cb)`, `.length` | ✅ Implemented |

### Map Fields

| protobuf-es | goja-protobuf | Status |
|-------------|---------------|--------|
| `msg.field` → `Record<K,V>` | `msg.get("field")` → wrapper | ✅ Implemented |
| Map methods | `.get(k)`, `.set(k,v)`, `.has(k)`, `.delete(k)`, `.forEach(cb)`, `.entries()`, `.size` | ✅ Implemented |

### Reflection API

| protobuf-es | goja-protobuf | Status |
|-------------|---------------|--------|
| `reflect(Schema, msg)` → ReflectMessage | Go-side via `FindDescriptor` + `UnwrapMessage` | ✅ Partial (Go API) |
| DescFile, DescMessage, etc. | Go descriptors via `FindDescriptor` | ✅ Partial (Go API) |

### Well-Known Type Helpers

| protobuf-es | goja-protobuf | Status |
|-------------|---------------|--------|
| `timestampNow()` | `timestampNow()` | ✅ Implemented |
| `timestampFromDate(date)` | `timestampFromDate(date)` | ✅ Implemented |
| `timestampDate(ts)` | `timestampDate(ts)` | ✅ Implemented |
| `timestampFromMs(ms)` | `timestampFromMs(ms)` | ✅ Implemented |
| `timestampMs(ts)` | `timestampMs(ts)` | ✅ Implemented |
| `durationFromMs(ms)` | `durationFromMs(ms)` | ✅ Implemented |
| `durationMs(dur)` | `durationMs(dur)` | ✅ Implemented |
| `anyPack(Schema, msg)` | `anyPack(msgType, msg)` | ✅ Implemented |
| `anyUnpack(any, Schema)` | `anyUnpack(any, msgType)` | ✅ Implemented |
| `anyIs(any, Schema/typeName)` | `anyIs(any, typeNameOrMsgType)` | ✅ Implemented |

### Extensions

| protobuf-es | goja-protobuf | Status |
|-------------|---------------|--------|
| `setExtension(msg, ext, v)` | N/A | ❌ Not planned |
| `getExtension(msg, ext)` | N/A | ❌ Not planned |
| `hasExtension(msg, ext)` | N/A | ❌ Not planned |
| `clearExtension(msg, ext)` | N/A | ❌ Not planned |

### Code Generation / Plugin Framework

| protobuf-es | goja-protobuf | Status |
|-------------|---------------|--------|
| `protoc-gen-es` | N/A (runtime-only) | N/A |
| `@bufbuild/protoplugin` | N/A (runtime-only) | N/A |

### Additional protobuf-es Features

| Feature | goja-protobuf | Status |
|---------|---------------|--------|
| JSON types (`json_types=true`) | N/A | N/A |
| Valid types (`valid_types`) | N/A | N/A |
| Base64 encoding utilities | N/A | ❌ Not planned |
| BinaryReader/BinaryWriter | N/A | ❌ Not planned |
| Custom options via getOption/hasOption | Go-side only | ✅ Partial |

## Summary

**Implemented**: 32 features (core serialization, message manipulation, field presence, oneofs, maps, repeated fields, enums, registries, equals, clone, isMessage, isFieldSet, clearField, well-known type helpers: timestampNow, timestampFromDate, timestampDate, timestampFromMs, timestampMs, durationFromMs, durationMs, anyPack, anyUnpack, anyIs)

**Not planned**: Extensions JS API, merge operations, size-delimited streams, code generation, BinaryReader/Writer, base64 utilities. These are either out of scope for a runtime-only library or trivially composable from existing primitives.
