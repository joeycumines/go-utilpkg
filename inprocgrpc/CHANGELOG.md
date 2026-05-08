# Changelog

All notable changes to the `go-inprocgrpc` package will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- **Event-loop identity predicate** — `Channel.SharesLoop` lets runtime
  integrations authenticate the channel's concrete `eventloop.Loop` without
  exposing it through an inward accessor.

- **Bounded stream queues** — `WithStreamBuffer` configures finite
  per-direction capacity. Blocking gRPC adapters wait for credit; callback
  senders report `ResourceExhausted`.

- **Scheduler lifecycle seam** — Channels require the event loop's terminal
  `Done` signal, preventing admitted-but-discarded callbacks from leaving RPCs
  or initialization waiters unresolved.

- **`StreamHandlerFunc` callback-based handler type** — Non-blocking, event-loop-native handler
  for processing RPCs directly on the event loop goroutine. Registered via
  `Channel.RegisterStreamHandler()`. Supports all four RPC types (unary, server-stream,
  client-stream, bidi-stream) with callback-based `RPCStream` API.

- **`RPCStream` type** — Exposes non-blocking `StreamSender`/`StreamReceiver`
  interfaces for event-loop-native handlers. Data and graceful-completion
  methods are owner-only; `Abort` is a nonblocking, any-goroutine,
  first-terminal-wins failure path.

- **`StreamSender` / `StreamReceiver` interfaces** — Low-level non-blocking stream I/O with
  buffered message delivery and one-shot callback registration (`Recv`).

- **Server-side stats handler** — `WithServerStatsHandler(handler)` option for server-side
  RPC statistics/instrumentation. Supports `TagRPC`, `HandleRPC`, `InPayload`, `OutPayload`,
  `Begin`, and `End` stats events.

- **Client-side stats handler** — `WithClientStatsHandler(handler)` option for client-side
  RPC statistics/instrumentation with the same event support.

- **Server interceptors** — `WithServerUnaryInterceptor()` and
  `WithServerStreamInterceptor()` options for server-side interceptor
  registration.

- **Per-RPC credentials** — Support for `grpc.PerRPCCredentials` via `grpc.CallOption` in
  Invoke/NewStream.

### Changed

- **BREAKING: `NewChannel` now panics on nil loop** — Previously returned an error; now panics
  since a nil loop is always a programming error. Signature changed from
  `func NewChannel(*Loop, ...Option) (*Channel, error)` to
  `func NewChannel(...Option) *Channel`.

- **Exactly-once RPC termination** — Cancellation, deadlines, handler results,
  scheduler shutdown, cardinality violations, panics, and `runtime.Goexit`
  share one first-terminal-wins arbiter.

- **Publication-accurate stats** — Header and trailer events now represent
  actual publication instead of terminal cleanup. Trailers-only errors omit
  header events, local or scheduler failures do not synthesize metadata, and
  client `End` retains the received trailer snapshot.

- **Lossless isolation** — Requests, responses, and metadata cross the
  client/server boundary as independent values unless cloning is explicitly
  disabled.

- **BREAKING: API rename `WaitForMessage` → `Recv`** — `StreamReceiver`'s method renamed from
  `WaitForMessage` to `Recv` for idiomatic Go naming.

- **BREAKING: API rename `IsClosed` → `Closed`** — Both `StreamSender` and `StreamReceiver`
  methods renamed. Prepositions banned from public API names per project conventions.

### Fixed

- **Proper context error translation** — gRPC `DeadlineExceeded` and `Canceled` errors are
  now correctly propagated as status errors rather than raw context errors,
  including when wrapped.

- **Trailer delivery timing** — Trailers set between `Send` and `Finish` are now correctly
  included in the client's trailer metadata.
