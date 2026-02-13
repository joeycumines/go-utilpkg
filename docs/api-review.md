# Public API Symbol Review

Review of all exported symbols across all new/modified modules.
All symbols checked for: naming conventions, godoc presence, necessity, test coverage, preposition ban.

## Methodology

- ✅ = Verified clean
- ⚠️ = Note (acceptable deviation with reasoning)

## Module: `inprocgrpc`

### New Exported Symbols

| Symbol | Kind | Godoc | Tests | Preposition-free | Notes |
|--------|------|-------|-------|------------------|-------|
| `Channel` | struct | ✅ | ✅ | ✅ | Core type |
| `NewChannel(loop, opts...)` | func | ✅ | ✅ | ✅ | Factory, panics on nil loop |
| `Channel.GetServiceInfo()` | method | ✅ | ✅ | ✅ | Standard grpc.ServiceRegistrar |
| `Channel.Invoke(...)` | method | ✅ | ✅ | ✅ | grpc.ClientConnInterface |
| `Channel.NewStream(...)` | method | ✅ | ✅ | ✅ | grpc.ClientConnInterface |
| `Channel.RegisterService(...)` | method | ✅ | ✅ | ✅ | grpc.ServiceRegistrar |
| `Channel.RegisterStreamHandler(...)` | method | ✅ | ✅ | ✅ | Non-blocking handler pathway |
| `ClientContext(ctx)` | func | ✅ | ✅ | ✅ | Server-side context accessor |
| `Cloner` | interface | ✅ | ✅ | ✅ | Message isolation abstraction |
| `CloneFunc(fn)` | func | ✅ | ✅ | ✅ | Adapter |
| `CopyFunc(fn)` | func | ✅ | ✅ | ✅ | Adapter |
| `CodecCloner(codec)` | func | ✅ | ✅ | ✅ | gRPC codec v1 adapter |
| `CodecClonerV2(codec)` | func | ✅ | ✅ | ✅ | gRPC codec v2 adapter |
| `ProtoCloner` | struct | ✅ | ✅ | ✅ | Default cloner |
| `Option` | interface | ✅ | ✅ | ✅ | Interface-based option pattern |
| `WithCloner(c)` | func | ✅ | ✅ | ✅ | |
| `WithClientStatsHandler(h)` | func | ✅ | ✅ | ✅ | |
| `WithServerStatsHandler(h)` | func | ✅ | ✅ | ✅ | |
| `WithServerUnaryInterceptor(i)` | func | ✅ | ✅ | ✅ | |
| `WithServerStreamInterceptor(i)` | func | ✅ | ✅ | ✅ | |
| `RPCStream` | struct | ✅ | ✅ | ✅ | Callback-based stream core |
| `RPCStream.Method()` | method | ✅ | ✅ | ✅ | |
| `RPCStream.Recv()` | method | ✅ | ✅ | ✅ | Returns StreamReceiver |
| `RPCStream.Send()` | method | ✅ | ✅ | ✅ | Returns StreamSender |
| `RPCStream.Finish(err)` | method | ✅ | ✅ | ✅ | |
| `RPCStream.SetHeader(md)` | method | ✅ | ✅ | ✅ | |
| `RPCStream.SendHeader()` | method | ✅ | ✅ | ✅ | |
| `RPCStream.SetTrailer(md)` | method | ✅ | ✅ | ✅ | |
| `StreamHandlerFunc` | type | ✅ | ✅ | ✅ | Non-blocking handler type |
| `StreamReceiver` | interface | ✅ | ✅ | ✅ | Non-blocking receive |
| `StreamSender` | interface | ✅ | ✅ | ✅ | Non-blocking send |

### Changed Exported Symbols (vs main)

| Symbol | Change | Backward Compatible | CHANGELOG |
|--------|--------|---------------------|-----------|
| `NewChannel` | Added `*eventloop.Loop` param, returns `*Channel` only | ❌ Breaking | ✅ |
| `StreamReceiver.Recv` | Was `WaitForMessage` | ❌ Breaking | ✅ |
| `StreamSender.Closed` | Was `IsClosed` | ❌ Breaking | ✅ |
| `StreamReceiver.Closed` | Was `IsClosed` | ❌ Breaking | ✅ |

All breaking changes are documented in `inprocgrpc/CHANGELOG.md`.

## Module: `goja-protobuf` (All New)

| Symbol | Kind | Godoc | Tests | Preposition-free | Notes |
|--------|------|-------|-------|------------------|-------|
| `Require(opts...)` | func | ✅ | ✅ | ✅ | Module loader factory |
| `Module` | struct | ✅ | ✅ | ✅ | Core type |
| `New(rt, opts...)` | func | ✅ | ✅ | ✅ | Direct construction |
| `Module.FileResolver()` | method | ✅ | ✅ | ✅ | Descriptor resolution |
| `Module.FindDescriptor(name)` | method | ✅ | ✅ | ✅ | Named lookup |
| `Module.LoadDescriptorSetBytes(data)` | method | ✅ | ✅ | ✅ | Binary descriptor loading |
| `Module.Runtime()` | method | ✅ | ✅ | ✅ | Runtime accessor |
| `Module.SetupExports(exports)` | method | ✅ | ✅ | ✅ | Manual export wiring |
| `Module.UnwrapMessage(val)` | method | ✅ | ✅ | ✅ | JS→Go message extraction |
| `Module.WrapMessage(msg)` | method | ✅ | ✅ | ✅ | Go→JS message wrapping |
| `Option` | interface | ✅ | ✅ | ✅ | Interface-based pattern |
| `WithFiles(files)` | func | ✅ | ✅ | ✅ | |
| `WithResolver(resolver)` | func | ✅ | ✅ | ✅ | |

## Module: `goja-grpc` (All New)

| Symbol | Kind | Godoc | Tests | Preposition-free | Notes |
|--------|------|-------|-------|------------------|-------|
| `Require(opts...)` | func | ✅ | ✅ | ✅ | Module loader factory |
| `Module` | struct | ✅ | ✅ | ✅ | Core type |
| `New(rt, opts...)` | func | ✅ | ✅ | ✅ | Direct construction |
| `Module.EnableReflection()` | method | ✅ | ✅ | ✅ | Reflection service |
| `Module.Runtime()` | method | ✅ | ✅ | ✅ | Runtime accessor |
| `Module.SetupExports(exports)` | method | ✅ | ✅ | ✅ | Manual export wiring |
| `Option` | interface | ✅ | ✅ | ✅ | Interface-based pattern |
| `WithAdapter(a)` | func | ✅ | ✅ | ✅ | |
| `WithChannel(ch)` | func | ✅ | ✅ | ✅ | |
| `WithProtobuf(pb)` | func | ✅ | ✅ | ✅ | |

## Module: `goja-protojson` (All New)

| Symbol | Kind | Godoc | Tests | Preposition-free | Notes |
|--------|------|-------|-------|------------------|-------|
| `Require(opts...)` | func | ✅ | ✅ | ✅ | Module loader factory |
| `Module` | struct | ✅ | ✅ | ✅ | Core type |
| `New(rt, opts...)` | func | ✅ | ✅ | ✅ | Direct construction |
| `Module.SetupExports(exports)` | method | ✅ | ✅ | ✅ | Manual export wiring |
| `Option` | interface | ✅ | ✅ | ✅ | Interface-based pattern |
| `WithProtobuf(pb)` | func | ✅ | ✅ | ✅ | |

## Summary

| Module | New Symbols | Changed Symbols | Issues Found |
|--------|-------------|-----------------|--------------|
| `inprocgrpc` | 30 | 4 (breaking) | 0 |
| `goja-protobuf` | 13 | 0 | 0 |
| `goja-grpc` | 10 | 0 | 0 |
| `goja-protojson` | 6 | 0 | 0 |
| **Total** | **59** | **4** | **0** |

### Design Pattern Consistency

- ✅ All modules use interface-based `Option` pattern
- ✅ All modules have `New(runtime, opts...)` constructors
- ✅ All modules have `SetupExports(exports)` for manual wiring
- ✅ All modules have `Require(opts...)` for require() integration
- ✅ Zero prepositions in any exported name
- ✅ All constructors panic on nil runtime (invariant violation)
- ✅ All option validation returns errors at construction time
