# gRPC Dial Design — Connecting to External Servers

## Overview

The current goja-grpc module uses `inprocgrpc.Channel` exclusively for all
RPC communication. This design explores adding `grpc.NewClient`-based
connections for calling external gRPC servers from JavaScript.

## Current Architecture

```
JS Code → createClient(serviceName) → Module.channel (inprocgrpc.Channel) → Handler
```

The `Module` stores one `*inprocgrpc.Channel` and all clients use it.

## Proposed Architecture

```
JS Code → createClient(serviceName, { channel: dialChannel }) → dialChannel → External Server
JS Code → createClient(serviceName)                           → module.channel (in-proc)
```

### Key Design Decisions

1. **Channel abstraction**: Both `inprocgrpc.Channel` and `grpc.ClientConn`
   implement `grpc.ClientConnInterface` (Invoke + NewStream). The client code
   already uses these methods. A thin wrapper maps `dial()` results into
   a compatible interface.

2. **JS API**: `grpc.dial(target, opts?)` returns a channel object that can be
   passed as `{ channel: ch }` to `createClient`. This keeps the client
   creation API consistent.

3. **Connection lifecycle**: `channel.close()` wraps `ClientConn.Close()`.
   The channel tracks whether it's closed and returns errors on subsequent use.

## JS API

### `grpc.dial(target, opts?)`

Creates a connection to an external gRPC server.

```javascript
const ch = grpc.dial('localhost:50051');
const client = grpc.createClient('my.Service', { channel: ch });

// ... use client ...

ch.close();
```

### Options

| Option      | Type   | Description                                  |
| ----------- | ------ | -------------------------------------------- |
| `insecure`  | bool   | Use insecure (plaintext) connection          |
| `authority` | string | Override :authority header                   |

TLS support deferred to a future iteration.

### Connection Object Methods

- `close()` — Close the connection.
- `target()` — Returns the dial target string.

## Implementation Plan

1. Create `dial.go` with `jsDial` function
2. Create `dialchannel` wrapper that implements the channel interface
3. Modify `createClient` to accept optional `channel` in client options
4. Add tests with a real `grpc.Server` on loopback

## Feasibility Notes

- `grpc.NewClient` (Go 1.63+) replaces the deprecated `grpc.Dial`
- `grpc.ClientConn` implements `grpc.ClientConnInterface` which has
  `Invoke(ctx, method, args, reply, opts)` and `NewStream(ctx, desc, method, opts)`
- The inprocgrpc.Channel's `Invoke` and `NewStream` have the same signature
- The main difference: real gRPC requires actual proto-registered message types
  for codec operations. dynamicpb messages work with the proto codec.

## Risk Assessment

- **Proto codec compatibility**: `dynamicpb.Message` implements `proto.Message`,
  so the standard proto codec should handle serialization. ✅
- **Event loop blocking**: `Invoke` and `NewStream` are goroutine-safe and
  our client code already runs them off-loop in goroutines. ✅
- **Connection errors**: Need proper error handling for DNS resolution,
  TLS handshake, connection refused, etc. The existing `grpcErrorFromGoError`
  maps gRPC status codes. ✅
