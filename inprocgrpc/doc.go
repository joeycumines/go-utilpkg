// Package inprocgrpc provides an in-process gRPC channel implementation
// driven by an [eventloop.Loop]. RPCs are effectively in-process function
// calls. Ordinary stream data and metadata publication is owner-confined;
// synchronized terminal claims and post-[eventloop.Loop.Done] recovery cover
// cancellation and scheduler loss.
//
// A [Channel] serves as both a [grpc.ClientConnInterface] and a
// [grpc.ServiceRegistrar]. Create one with [NewChannel], register
// services via [Channel.RegisterService], and issue RPCs via
// [Channel.Invoke] or [Channel.NewStream].
//
// # Message Isolation
//
// Messages are cloned between client and server to prevent concurrent
// mutation. The [Cloner] interface controls this; the default
// [ProtoCloner] handles [proto.Message] instances.
//
// # Context Handling
//
// Server handlers receive a context that inherits cancellation and
// deadline from the client but does not inherit values. Outgoing
// client metadata becomes incoming server metadata. The original
// client context is available via [ClientContext].
package inprocgrpc
