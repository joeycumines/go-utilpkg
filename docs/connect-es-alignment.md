# connect-es Alignment Audit

This document maps [connect-es](https://connectrpc.com/docs/web/) features to the
goja-grpc implementation status. The goal is to provide a JavaScript gRPC
experience that feels natural to developers familiar with connect-es, adapted
for the in-process goja + eventloop environment.

> **Reference:** connect-es v2 ([@connectrpc/connect](https://www.npmjs.com/package/@connectrpc/connect))

## Scope Differences

| Aspect | connect-es | goja-grpc |
|---|---|---|
| Transport | HTTP/2, HTTP/1.1, gRPC, gRPC-web, Connect protocol | In-process via `inprocgrpc.Channel` (no network) |
| Runtime | Browser/Node.js | goja (Go-embedded JS engine) |
| Async model | Native async/await, AsyncIterable | Event loop promises (ChainedPromise) |
| Code generation | Compile-time from .proto | Runtime dynamic descriptors via goja-protobuf |
| Module system | ES modules, import | CommonJS require() via goja_nodejs |

## Client Features

| # | connect-es Feature | goja-grpc Status | Notes |
|---|---|---|---|
| C01 | `createClient(service, transport)` | ✅ Implemented | `grpc.createClient(serviceName)` — transport is implicit (bound Channel) |
| C02 | Unary RPC → Promise\<response\> | ✅ Implemented | `client.echo(req)` returns Promise |
| C03 | Server-streaming → AsyncIterable | ⚠️ Partial | Returns `stream.recv()` iterator pattern, not native AsyncIterable |
| C04 | Client-streaming → send/closeSend/response | ✅ Implemented | `call.send(msg)`, `call.closeSend()`, `call.response` |
| C05 | Bidi-streaming → send/recv/closeSend | ✅ Implemented | Full bidirectional support |
| C06 | `signal` option (AbortSignal cancellation) | ✅ Implemented | `{ signal: controller.signal }` → CANCELLED error |
| C07 | `timeoutMs` option | ❌ Not implemented | connect-es sends timeout header, creates deadline |
| C08 | `headers` option (request metadata) | ✅ Implemented | `{ metadata: md }` — uses gRPC metadata wrapper |
| C09 | `onHeader` callback (response headers) | ❌ Not implemented | connect-es: `{ onHeader: (h) => ... }` |
| C10 | `onTrailer` callback (response trailers) | ❌ Not implemented | connect-es: `{ onTrailer: (t) => ... }` |
| C11 | Response headers via interceptor | ❌ Not implemented | No interceptor support yet |
| C12 | Response trailers via interceptor | ❌ Not implemented | No interceptor support yet |
| C13 | Client interceptors | ❌ Not implemented | connect-es: `transport({ interceptors: [...] })` |
| C14 | `ConnectError` with code/message | ✅ Implemented | `GrpcError` with name/code/message properties |
| C15 | `ConnectError.from()` utility | ❌ Not implemented | Wraps unknown errors as ConnectError |
| C16 | Error `rawMessage` (without code prefix) | ❌ Not implemented | Minor: connect-es prefixes message with code |
| C17 | Error `metadata` (headers+trailers on error) | ❌ Not implemented | connect-es merges headers+trailers in error.metadata |
| C18 | Error `details` (google.rpc.Status details) | ❌ Not implemented | connect-es: `err.findDetails(Schema)` |
| C19 | `createCallbackClient()` | ❌ Not applicable | Callback-style client — not needed since we have promises |
| C20 | Context values (`contextValues` option) | ❌ Not implemented | connect-es: typed key-value context passing |

**Summary:** 7 of 20 client features implemented (35%).

## Server Features

| # | connect-es Feature | goja-grpc Status | Notes |
|---|---|---|---|
| S01 | Register service with handlers | ✅ Implemented | `server.addService(name, { method: fn })` |
| S02 | Unary handler (req, ctx) → response | ✅ Implemented | Handler returns response or Promise |
| S03 | Server-streaming handler + call.send() | ✅ Implemented | Sync/async handlers with call.send() |
| S04 | Client-streaming handler + call.recv() | ✅ Implemented | Promise-based recv() with {value, done} |
| S05 | Bidi-streaming handler | ✅ Implemented | Full send/recv in handler |
| S06 | `server.start()` | ✅ Implemented | Registers handlers on Channel |
| S07 | Throw error → gRPC status error | ✅ Implemented | `throw grpc.status.createError(code, msg)` |
| S08 | Error details in server errors | ❌ Not implemented | connect-es: throw ConnectError with details array |
| S09 | `context.requestHeader` access | ❌ Not implemented | connect-es: handler receives headers from request |
| S10 | `context.responseHeader.set()` | ❌ Not implemented | connect-es: set response headers from handler |
| S11 | `context.responseTrailer.set()` | ❌ Not implemented | connect-es: set response trailers from handler |
| S12 | Server interceptors | ❌ Not implemented | connect-es: `{ interceptors: [...] }` on router |
| S13 | Context values in handler | ❌ Not implemented | connect-es: access typed context values |
| S14 | `signal` in handler context | ❌ Not implemented | connect-es: handler gets AbortSignal for client disconnect |
| S15 | Method not implemented → UNIMPLEMENTED | ❌ Not implemented | connect-es: auto-responds if method omitted |

**Summary:** 7 of 15 server features implemented (47%).

## Metadata / Status

| # | connect-es Feature | goja-grpc Status | Notes |
|---|---|---|---|
| M01 | `metadata.create()` | ✅ Implemented | Creates empty metadata wrapper |
| M02 | `md.set(key, ...values)` | ✅ Implemented | Set with multiple values |
| M03 | `md.get(key)` | ✅ Implemented | Returns first value |
| M04 | `md.getAll(key)` | ✅ Implemented | Returns all values |
| M05 | `md.delete(key)` | ✅ Implemented | Remove key |
| M06 | `md.forEach(fn)` | ✅ Implemented | Iterate key-value pairs |
| M07 | `md.toObject()` | ✅ Implemented | Convert to plain object |
| M08 | Binary headers (`-bin` suffix) | ❌ Not implemented | connect-es: encodeBinaryHeader/decodeBinaryHeader |
| M09 | Status code constants | ✅ Implemented | All 17 standard codes |
| M10 | `status.createError(code, msg)` | ✅ Implemented | GrpcError factory |
| M11 | `GrpcError.toString()` | ✅ Implemented | `"GrpcError: Code: message"` |

**Summary:** 9 of 11 metadata features implemented (82%).

## Overall Summary

| Category | Implemented | Total | Coverage |
|---|---|---|---|
| Client | 7 | 20 | 35% |
| Server | 7 | 15 | 47% |
| Metadata/Status | 9 | 11 | 82% |
| **Total** | **23** | **46** | **50%** |

## Priority Implementation Order

Based on impact and difficulty:

1. **C07: timeoutMs** — Simple context.WithTimeout, high value
2. **C09/C10: onHeader/onTrailer callbacks** — Response metadata visibility
3. **S09/S10/S11: Request/response headers in handlers** — Server-side metadata
4. **C13/S12: Client/server interceptors** — Cross-cutting concerns (auth, logging, retry)
5. **C17/C18/S08: Error metadata and details** — Rich error information
6. **S14: Handler AbortSignal** — Client disconnect detection
7. **S15: Auto UNIMPLEMENTED** — Polish
8. **M08: Binary headers** — Niche but complete
9. **C15/C16/C20: Error utilities, context values** — Polish
