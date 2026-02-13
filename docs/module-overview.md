# Module Dependency Diagram

```
   ┌────────────────┐
   │  eventloop     │  (core event loop, timers, promises, I/O polling)
   └──────┬─────────┘
          │
   ┌──────┴─────────┐
   │  inprocgrpc    │  (in-process gRPC channel, event-loop-native)
   └──────┬─────────┘
          │
   ┌──────┴─────────┐     ┌───────────────-─┐
   │  goja-grpc     │────>│  goja-protobuf  │  (JS protobuf bridge)
   └────────────────┘     └──────┬──────────┘
                                 │
                          ┌──────┴─────────-─┐
                          │  goja-protojson  │  (JSON format support)
                          └─────────────────-┘
```

## Module Relationships

- **eventloop** → standalone, no dependencies on other modules
- **inprocgrpc** → depends on eventloop
- **goja-protobuf** → standalone, depends on goja and protobuf
- **goja-protojson** → depends on goja-protobuf
- **goja-grpc** → depends on inprocgrpc, goja-protobuf, goja-eventloop

## Individual Module READMEs

- [inprocgrpc](https://github.com/joeycumines/go-inprocgrpc) — Event-loop-driven in-process gRPC channel
- [goja-protobuf](https://github.com/joeycumines/goja-protobuf) — Protobuf bridge for Goja (protobuf-es inspired)
- [goja-grpc](https://github.com/joeycumines/goja-grpc) — gRPC client/server for Goja (connect-es inspired)
- [goja-protojson](https://github.com/joeycumines/goja-protojson) — Protobuf JSON marshaling for Goja
- [eventloop](https://github.com/joeycumines/go-eventloop) — High-performance JavaScript event loop
