// Package inprocgrpc provides an in-process gRPC channel implementation
// driven by an [eventloop.Loop].
//
// The in-process channel makes RPCs that are effectively in-process function
// calls. All stream state is managed on the event loop goroutine, ensuring
// thread safety without mutexes on the fast path. Server handler goroutines
// communicate with the loop via blocking adapters wrapping a callback-based
// stream core.
//
// # Architecture
//
// A [Channel] is created via [NewChannel] with a running [eventloop.Loop]
// and optional configuration via [Option] functions. It serves as both a
// [grpc.ClientConnInterface] (client side) and a [grpc.ServiceRegistrar]
// (server side). Services are registered via [Channel.RegisterService], and
// RPCs are dispatched via [Channel.Invoke] (unary) and [Channel.NewStream]
// (streaming).
//
// # Message Isolation
//
// Because client and server share the same process, messages must be cloned to
// prevent concurrent mutation. The [Cloner] interface controls this behavior.
// The default [ProtoCloner] handles [proto.Message] instances; custom cloners
// can be provided via [WithCloner] for non-proto message types.
//
// # Context Handling
//
// Server handlers receive a context that:
//   - Inherits cancellation and deadline from the client context
//   - Does NOT inherit context values (prevents state leakage)
//   - Converts outgoing metadata to incoming metadata
//   - Sets peer info to "inproc:0"
//
// The original client context is retrievable on the server via [ClientContext].
//
// # Stats and Interceptors
//
// Server-side unary and stream interceptors are supported via
// [WithServerUnaryInterceptor] and [WithServerStreamInterceptor].
// Client and server stats handlers are supported via [WithClientStatsHandler]
// and [WithServerStatsHandler].
//
// # Thread Safety
//
// A [Channel] is safe for concurrent use from multiple goroutines.
// Multiple RPCs may be in-flight simultaneously. All state mutations
// occur on the event loop goroutine.
package inprocgrpc
