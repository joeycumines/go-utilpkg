# Diff Summary vs `main`

Generated from `git diff main --stat`.

## Overview

| Metric | Value |
|--------|-------|
| Files changed | 255 |
| Insertions | ~41,545 |
| Deletions | ~1,160 |
| Net new lines | ~40,385 |

## Module Breakdown

### New Modules

| Module | Files | Description |
|--------|-------|-------------|
| `goja-protobuf` | 34 | Protocol Buffers support for goja (protobuf-es aligned) |
| `goja-grpc` | 38 | gRPC client/server for goja (connect-es aligned) |
| `goja-protojson` | 13 | Proto JSON encoding/decoding for goja |

### Significantly Modified Modules

| Module | Files Changed | Description |
|--------|---------------|-------------|
| `inprocgrpc` | 34 | In-process gRPC channel: complete rewrite with event-loop architecture |
| `eventloop` | 81 | Core event loop: promise combinators, pollers, abort signals, metrics |
| `goja-eventloop` | 6 | Adapter: promise integration, coverage improvements |

### Documentation (New)

| Path | Description |
|------|-------------|
| `docs/adr/decisions.md` | 6 Architecture Decision Records |
| `docs/concurrency-model.md` | Concurrency architecture guide |
| `docs/module-overview.md` | Module dependency diagram |
| `docs/performance-guide.md` | Performance tuning guide |
| `inprocgrpc/docs/` | Design, benchmarks (3 platforms), migration guide, tournament results |
| `goja-protobuf/docs/` | Design, benchmarks, protobuf-es alignment audit |
| `goja-grpc/docs/` | Design, benchmarks, connect-es alignment, error handling, stress results, reflection design, dial design |
| `goja-protojson/docs/` | Design document |

### Infrastructure

| File | Description |
|------|-------------|
| `go.work` | Workspace configuration with all modules |
| `project.mk` | Grit publishing destinations for new modules |
| `blueprint.json` | 305-task implementation plan |
| `WIP.md` | Session work-in-progress tracking |

## Changed File List

### `inprocgrpc/` (34 files)

New core implementation:
- `channel.go`, `channel_test.go` — Event-loop-driven channel
- `stream.go`, `stream_test.go` — StreamSender/StreamReceiver interfaces
- `handler.go`, `handler_test.go` — StreamHandlerFunc dispatch
- `options.go`, `options_test.go` — Interface-based option pattern
- `cloner.go`, `cloner_test.go` — Message isolation (ProtoCloner, CodecCloner)
- `context.go`, `context_test.go` — Server context with ClientContext
- `stats.go`, `stats_test.go` — Client/server stats handler support
- `serverstreamadapter.go` — Blocking adapter for grpc.ServerStream
- `clientstreamadapter.go` — Blocking adapter for grpc.ClientStream
- `stress_test.go` — 5 stress tests (1000 concurrent, 100 bidi, sustained throughput, leak, heap)
- `benchmark_test.go` — Performance benchmarks
- `internal/` — Internal packages (stream, transport, callopts, grpcutil, grpchantest)

### `goja-protobuf/` (34 files)

Complete protobuf-es aligned implementation:
- `module.go` — Module entry point, type registry
- `message.go`, `message_test.go` — Message wrapper (get/set/has/clear/whichOneof)
- `conversion.go`, `conversion_test.go` — Go↔JS type conversion
- `descriptors.go`, `descriptors_test.go` — Descriptor loading and resolution
- `serialize.go`, `serialize_test.go` — Binary + JSON serialization
- `types.go`, `types_test.go` — Repeated/map field proxies
- `helpers.go` — equals, clone, isMessage, isFieldSet, clearField
- `wellknown.go`, `wellknown_test.go` — Timestamp, Duration, Any helpers
- `utilities.go`, `utilities_test.go` — Utility functions
- `fuzz_test.go` — 2 fuzz tests (round-trip, random fields)
- `benchmark_test.go` — Performance benchmarks
- `example_test.go` — Testable examples

### `goja-grpc/` (38 files)

Complete connect-es aligned implementation:
- `module.go` — Module entry point
- `client.go` — Client proxy with all 4 RPC types
- `server.go` — Server builder with handler dispatch
- `service.go` — Service descriptor resolution
- `status.go`, `status_test.go` — gRPC status codes and GrpcError
- `metadata.go`, `metadata_test.go` — Metadata wrapper for JS
- `callopts.go` — Call options (signal, metadata, timeout, onHeader, onTrailer)
- `reflection.go`, `reflection_test.go` — gRPC reflection client
- `dial.go`, `dial_test.go` — grpc.Dial for external servers
- `stress_test.go` — 4 stress tests (JS 100 concurrent, Go→JS 100 concurrent, leak, heap)
- `benchmark_test.go` — Performance benchmarks
- Integration, interceptor, interop, edge, abort, error details tests

### `goja-protojson/` (13 files)

Proto JSON module:
- `module.go`, `module_test.go` — Module with marshal/unmarshal/format
- `marshal.go` — Marshal implementation
- `unmarshal.go` — Unmarshal implementation
- `options.go` — WithProtobuf option
- `example_test.go` — Testable examples

### `eventloop/` (81 files)

Promise combinators, abort signals, pollers, metrics, and extensive test coverage (see eventloop/CHANGELOG.md for details).

## Breaking Changes vs main

### `inprocgrpc`
- Complete architectural rewrite (event-loop-driven)
- `NewChannel` now takes `*eventloop.Loop` as first argument
- `NewChannel` panics on nil loop (previously returned error)
- `StreamReceiver.WaitForMessage` renamed to `Recv`
- `IsClosed` renamed to `Closed` on `StreamSender` and `StreamReceiver`
- `RegisterStreamHandler` is new (non-blocking handler pathway)
- Stats handler and interceptor support added via options

### New modules (no breaking changes — all new)
- `goja-protobuf` v0.0.0
- `goja-grpc` v0.0.0
- `goja-protojson` v0.0.0
