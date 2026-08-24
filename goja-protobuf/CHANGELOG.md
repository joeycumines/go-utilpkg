# Changelog

All notable changes to the `goja-protobuf` package will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Changed

- **Base-owned descriptor files admit additive metadata** — loading a
  descriptor set that redeclares a file the base registry already owns (e.g.
  `google/protobuf/descriptor.proto` as emitted by buf, whose custom
  `FileOptions` extension ranges make the bytes differ) no longer fails a
  strict byte comparison. The incoming file is accepted when the base
  semantically covers it: every declared path, package, symbol, and extension
  is already present in the base with a matching kind, and the canonical base
  identity keeps serving resolution. A genuine redefinition — one introducing a
  symbol or extension the base lacks, or changing a symbol's kind — is still
  rejected and rolls back transactionally.

### Added

- **Initial release** — Go-native protobuf bridge for the Goja JavaScript engine, modeled
  after [protobuf-es](https://github.com/bufbuild/protobuf-es).

- **Message API** — `pb.messageType(name)` returns a constructor. Instances support:
  - `msg.get(field)` / `msg.set(field, value)` — field access
  - `msg.has(field)` / `msg.toObject()` — presence check and export
  - Field access uses `protoreflect` descriptors for type safety

- **Binary encoding** — `pb.encode(msg)` / `pb.decode(Type, bytes)` for protobuf wire format
  serialization/deserialization using `proto.Marshal`/`proto.Unmarshal`.

- **JSON support** — `pb.toJSON(msg)` / `pb.fromJSON(Type, json)` for canonical protobuf JSON
  encoding using `protojson.Marshal`/`protojson.Unmarshal`.

- **Enum support** — `pb.enumType(name)` provides access to enum values by name or number.

- **Well-known type helpers** —
  - `pb.timestampNow()` / `pb.timestampFromDate(date)` / `pb.timestampDate(msg)` /
    `pb.timestampFromMs(ms)` — Timestamp utilities
  - Proper wrapping/unwrapping of `google.protobuf.Duration`, `google.protobuf.Struct`,
    `google.protobuf.Value`, `google.protobuf.Any`

- **Utility functions** —
  - `pb.equals(a, b)` — deep equality via `proto.Equal`
  - `pb.clone(msg)` — deep copy via `proto.Clone`
  - `pb.isMessage(value)` — type check
  - `pb.isFieldSet(msg, field)` — field presence
  - `pb.clearField(msg, field)` — field clearing

- **Descriptor loading** — `pb.loadDescriptorSet(bytes)` and
  `pb.loadFileDescriptorProto(bytes)` for runtime descriptor registration.

- **`FileResolver()` API** — Exported method returning the runtime's canonical
  local and immutable base descriptor graph. Essential for cross-module
  descriptor resolution and gRPC reflection.

- **Interface-based options** — `WithResolver(*protoregistry.Types)` for custom type resolution.

- **`TypeResolver()` API** — Exported composite message and extension resolver,
  including extension enumeration. It checks live runtime-local types before
  the immutable construction snapshot and composes with `goja-protojson` and
  gRPC reflection.

### Fixed

- **Canonical runtime identity** — Direct construction and `require()` now share
  one runtime-scoped file/type/extension graph. Generated messages retain their
  concrete identity, while foreign same-name descriptors and forged wrappers or
  constructors are rejected.
- **Immutable base registry ownership** — The first module for a runtime
  snapshots configured registry membership. Later caller mutation cannot
  introduce unvalidated shadows; runtime descriptor transactions remain live
  and shared.
- **Atomic descriptor loading** — Descriptor sets are order-independent,
  register nested and top-level extensions, swap as one transaction, treat
  exact duplicates as idempotent, and reject divergent paths or symbols without
  partial registration.
- **Lossless conversion** — Unsafe, fractional, and non-finite JavaScript
  numbers no longer silently corrupt 64-bit values or map keys. BigInt and exact
  decimal strings cover the full signed and unsigned ranges.
- **Transactional collections** — Repeated and map replacements validate into
  temporary values before mutation. Map wrappers now implement the iterable
  protocol without reserving the valid plain-object key `entries`.
- **Owner-truthful Go API** — `OwnsRuntime` replaces raw runtime exposure;
  `WrapMessage` validates descriptor identity and returns dynamic errors;
  `SetupExports` performs atomic, idempotent installation and returns errors.
- **`timestampFromMs` negative nanos** — Sub-second negative milliseconds (e.g., -500ms)
  now produce valid proto Timestamps with properly normalized nanos in [0, 999999999].
  Previously produced invalid negative nanos values.
