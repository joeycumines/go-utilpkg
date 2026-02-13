# Changelog

All notable changes to the `goja-protobuf` package will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased] - 2026-02-13

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

- **`FileResolver()` API** — Exported method returning a resolver that checks the module's
  local file registries first, then falls back to configured global registries. Essential for
  cross-module descriptor resolution (e.g., Go clients sending `dynamicpb.Message` to JS servers).

- **Interface-based options** — `WithResolver(*protoregistry.Types)` for custom type resolution.

- **Fuzz tests** — `FuzzEncodeDecodeRoundTrip` and `FuzzProtoMarshalUnmarshal` for
  robustness testing with random payloads.

- **`TypeResolver()` API** — Exported method returning a composite interface implementing
  `protoregistry.MessageTypeResolver` and `protoregistry.ExtensionTypeResolver`. Checks
  local types first, then falls back to global registries. Used by `goja-protojson` for
  `protojson.MarshalOptions.Resolver` and `protojson.UnmarshalOptions.Resolver`.

- **99.4% test coverage** — Comprehensive test suite covering all API functions, edge cases,
  error paths, type coercion, and boundary values.

### Fixed

- **`timestampFromMs` negative nanos** — Sub-second negative milliseconds (e.g., -500ms)
  now produce valid proto Timestamps with properly normalized nanos in [0, 999999999].
  Previously produced invalid negative nanos values.
