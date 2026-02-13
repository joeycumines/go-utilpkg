# Changelog

All notable changes to the `goja-grpc` package will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased] - 2026-02-13

### Added

- **Initial release** — JavaScript gRPC client/server for the Goja runtime, modeled after
  [connect-es](https://github.com/connectrpc/connect-es) and using `inprocgrpc.Channel`
  for event-loop-native transport.

- **Client API** — `grpc.createClient(serviceName, [options])` returns a client with methods
  matching the service definition:
  - Unary RPCs return `Promise<Message>`
  - Server-streaming RPCs return async-iterable stream objects
  - Client-streaming RPCs return writable streams with `send(msg)` and `close()`
  - Bidirectional streaming RPCs return full-duplex stream objects
  - Optional `{ channel: dialChannel }` for external connections

- **Server API** — `grpc.createServer()` → `server.addService(name, handlers)` →
  `server.start()`. Handlers:
  - Unary: `function(request, call) → message|Promise<message>`
  - Server-stream: `function(request, call)` with `call.send(msg)`
  - Client-stream: `function(call)` with async iteration
  - Bidi-stream: `function(call)` with send/receive

- **`grpc.dial(target, options)`** — Connect to external gRPC servers:
  - `target`: host:port address
  - `options.insecure`: use plaintext (no TLS)
  - `options.authority`: override `:authority` header
  - Returns channel object with `close()` and `target()` methods
  - Use with `grpc.createClient(service, { channel: ch })`

- **Client interceptors** — `grpc.setClientInterceptor(fn)` for request/response
  transformation. Interceptors receive `(method, request, next)` and can modify or
  observe RPC flow.

- **Server interceptors** — `server.setInterceptor(fn)` for server-side middleware.
  Chainable. Receives `(call, next)`.

- **Metadata support** —
  - Client: `call.setRequestHeader(key, value)`, `call.getResponseHeader(key)`,
    `call.getResponseTrailer(key)`
  - Server: `call.requestHeader.get(key)`, `call.setHeader(metadata)`,
    `call.sendHeader()`, `call.setTrailer(metadata)`
  - Full round-trip header/trailer propagation

- **Error handling** —
  - `grpc.status.createError(code, message)` for structured gRPC errors
  - `grpc.status.codes` enum (OK, CANCELLED, UNKNOWN, etc.)
  - Error detail support via `error.details`
  - Proper error propagation between JS and Go

- **AbortSignal support** — All client RPCs accept `{ signal: abortController.signal }`:
  - Pre-aborted signals reject immediately
  - Mid-RPC abort cancels in-flight operations
  - Works with all four RPC types

- **gRPC reflection** — `grpc.getServiceMethods(serviceName)` for runtime service
  introspection. Includes method names, types (unary/stream), and descriptors.

- **Stress tests** — 100 concurrent JS RPCs, 100 concurrent Go→JS RPCs,
  goroutine leak detection, heap allocation profiling.

- **99.6% test coverage** across all source files.

### Technical Notes

- All JS APIs run on the event loop goroutine (single-threaded JS semantics)
- Uses `dynamicpb.Message` internally — no code generation required
- Compatible with both `inprocgrpc.Channel` and `grpc.ClientConn` via
  `grpc.ClientConnInterface`
