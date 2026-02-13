# Changelog

All notable changes to the `go-inprocgrpc` package will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased] - 2026-02-13

### Added

- **`StreamHandlerFunc` callback-based handler type** — Non-blocking, event-loop-native handler
  for processing RPCs directly on the event loop goroutine. Registered via
  `Channel.RegisterStreamHandler()`. Supports all four RPC types (unary, server-stream,
  client-stream, bidi-stream) with callback-based `RPCStream` API.

- **`RPCStream` type** — Exposes non-blocking `StreamSender`/`StreamReceiver` interfaces for
  event-loop-native handlers. All methods must be called on the loop goroutine.

- **`StreamSender` / `StreamReceiver` interfaces** — Low-level non-blocking stream I/O with
  buffered message delivery and one-shot callback registration (`Recv`).

- **Server-side stats handler** — `WithServerStatsHandler(handler)` option for server-side
  RPC statistics/instrumentation. Supports `TagRPC`, `HandleRPC`, `InPayload`, `OutPayload`,
  `Begin`, and `End` stats events.

- **Client-side stats handler** — `WithClientStatsHandler(handler)` option for client-side
  RPC statistics/instrumentation with the same event support.

- **Client interceptors** — `WithUnaryInterceptor()` and `WithStreamInterceptor()` options
  for client-side interceptor chain registration.

- **Per-RPC credentials** — Support for `grpc.PerRPCCredentials` via `grpc.CallOption` in
  Invoke/NewStream.

- **Comprehensive stress tests** — 1000 concurrent unary RPCs, 100 concurrent bidi streams,
  sustained throughput (1.5M ops/5s), goroutine leak detection, heap allocation checks.

### Changed

- **BREAKING: `NewChannel` now panics on nil loop** — Previously returned an error; now panics
  since a nil loop is always a programming error. Signature changed from
  `func NewChannel(*Loop, ...Option) (*Channel, error)` to
  `func NewChannel(*Loop, ...Option) *Channel`.

- **BREAKING: API rename `WaitForMessage` → `Recv`** — `StreamReceiver`'s method renamed from
  `WaitForMessage` to `Recv` for idiomatic Go naming.

- **BREAKING: API rename `IsClosed` → `Closed`** — Both `StreamSender` and `StreamReceiver`
  methods renamed. Prepositions banned from public API names per project conventions.

- **100% test coverage** — Complete coverage of all code paths, including error paths,
  context cancellation, concurrent access, and platform-specific behavior.

### Fixed

- **Proper context error translation** — gRPC `DeadlineExceeded` and `Canceled` errors are
  now correctly propagated as status errors rather than raw context errors.

- **Trailer delivery timing** — Trailers set between `Send` and `Finish` are now correctly
  included in the client's trailer metadata.
