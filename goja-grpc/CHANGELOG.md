# Changelog

All notable changes to the `goja-grpc` package will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Changed

- Runtime-sensitive work now composes through the exact bound
  `goja-eventloop.Adapter`: worker goroutines retain Go-only transport data,
  while Goja value creation and Promise settlement run under the logical
  adapter owner.
- Construction now rejects a protobuf module owned by another Goja runtime and
  an in-process channel dispatched by another event loop, before any exports or
  handlers can be mutated.
- Client requests are cloned and descriptor-validated on-owner before transport
  workers start. Streaming sends and receives use bounded, serialized
  coordinators with deterministic post-terminal behavior.
- Reflection operations now accept `timeoutMs` and `AbortSignal` options.
- Module shutdown is idempotent, cancels active operations, and closes only
  connections created by the module. Event-loop terminal cleanup triggers the
  same release path, and all behavior admissions reject after closure.
- Export installation is atomic; direct Go installation reports errors and
  `require` publishes only a fully constructed export object.
- Protobuf messages retain exact runtime registry identity across transport,
  including generated message types and unknown fields. Status-detail branding
  uses private weak identity and rejects invalid details without omission.

### Removed

- The raw `Module.Runtime()` accessor. Construction validates the supplied
  runtime against the bound adapter without exposing ownership-sensitive
  internals afterward.

### Added

- **Initial release** — JavaScript gRPC clients and in-process servers for the
  Goja runtime, using `inprocgrpc.Channel` as the default transport.

- **`Module.DisposeServices`** — retires every server registration whose gRPC
  service fully-qualified name appears in `services`, returning the number of
  plans this call actually retired. The in-process channel entries (stream
  handler and service entry) are removed synchronously so a fresh registration
  of the same service succeeds — the module delete/recreate lifecycle that
  previously bricked on "stream handler already registered". Retirement is
  scoped per service: when one `server.start` admission published several
  services, disposing a subset leaves co-rooted siblings fully live, and a
  supervisor root is disposed only once every service on it has been retired.
  Retirement is serialized against compound server admissions through the
  supervisor boundary (the same mutex `Module.Close` holds), so channel entries
  can never outlive their plans. The deeper owner-side disposal (root
  retirement and disposer execution) is scheduled best-effort and not awaited,
  so disposal is safe whether or not the event loop is currently running.
  Unlike `Module.Close`, retired methods report `Unimplemented` rather than
  `Unavailable`; missing services are silent no-ops.
- **Client API** — `grpc.createClient(serviceName, options?)` returns a client
  with methods matching the service definition:
  - Unary RPCs return `Promise<Message>`
  - Server-streaming RPCs expose `recv()`
  - Client-streaming RPCs expose `send()`, `closeSend()`, and `response`
  - Bidirectional RPCs expose `send()`, `recv()`, and `closeSend()`
  - A `{ channel: grpc.dial(...) }` option selects an external connection
- **Server API** — `grpc.createServer()`, `server.addService(name, handlers)`,
  and `server.start()` provide all four RPC shapes:
  - Unary: `function(request, call) → message|Promise<message>`
  - Server-stream: `function(request, call)` with `call.send(msg)`
  - Client-stream: `function(call)` with `call.recv()`
  - Bidi-stream: `function(call)` with `call.send()` and `call.recv()`
- **`grpc.dial(target, options?)`** — Creates an external gRPC client
  connection:
  - `target`: host:port address
  - `options.insecure`: use plaintext (no TLS)
  - `options.authority`: override `:authority` header
  - Returns channel object with `close()` and `target()` methods
- **Client interceptors** — Unary clients accept an `interceptors` array of
  connect-es-style factories through `createClient` options.
- **Server interceptors** — `server.addInterceptor(factory)` adds
  connect-es-style middleware.
- **Metadata support** —
  - Client call options accept `metadata`, `onHeader`, and `onTrailer`
  - Server: `call.requestHeader.get(key)`, `call.setHeader(metadata)`,
    `call.sendHeader()`, `call.setTrailer(metadata)`
  - Full round-trip header/trailer propagation
- **Error handling** —
  - `grpc.status.createError(code, message)` for structured gRPC errors
  - Direct status constants (OK, CANCELLED, UNKNOWN, etc.)
  - Error detail support via `error.details`
  - Proper error propagation between JS and Go
- **AbortSignal support** — Client RPCs accept
  `{ signal: abortController.signal }`:
  - Pre-aborted signals reject immediately
  - Mid-RPC abort cancels in-flight operations
  - Works with all four RPC types
- **gRPC reflection** — `grpc.enableReflection()` registers the in-process
  reflection service. `grpc.createReflectionClient()` returns
  `listServices()`, `describeService(name)`, and `describeType(name)` methods.
- **Lifecycle control** — `grpc.close()` cancels module-owned operations and
  closes module-created dial connections.

### Technical Notes

- All JS APIs run under the bound adapter's serialized logical callback owner;
  the event loop may transfer that role between physical goroutines
- Shares the runtime-bound protobuf module identity across generated and dynamic
  messages
- Compatible with both `inprocgrpc.Channel` and `grpc.ClientConn` via
  `grpc.ClientConnInterface`
