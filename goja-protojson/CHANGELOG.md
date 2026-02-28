# Changelog

All notable changes to the `goja-protojson` package will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- **Initial release** — Protobuf JSON marshaling/unmarshaling module for Goja, providing
  canonical protobuf JSON format support.

- **`protojson.marshal(msg)`** — Serializes a protobuf message to JSON string using
  `protojson.Marshal`. Returns canonical protobuf JSON format.

- **`protojson.unmarshal(Type, json)`** — Deserializes a JSON string to a protobuf message
  using `protojson.Unmarshal`.

- **`protojson.format(msg)`** — Human-readable JSON formatting with indentation.

### Fixed

- **Missing `Resolver` for `Any` types** — `protojson.MarshalOptions.Resolver` and
  `protojson.UnmarshalOptions.Resolver` are now set to the protobuf module's `TypeResolver()`.
  Previously, marshaling/unmarshaling messages containing `google.protobuf.Any` fields with
  locally-registered types would fail with "unable to resolve" errors.
