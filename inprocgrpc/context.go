package inprocgrpc

import (
	"context"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

type clientContextKeyType struct{}

var clientContextKey clientContextKeyType

var inprocessPeer = peer.Peer{
	Addr:     inprocessAddr{},
	AuthInfo: inprocessAddr{},
}

type inprocessAddr struct{}

func (inprocessAddr) Network() string  { return "inproc" }
func (inprocessAddr) String() string   { return "0" }
func (inprocessAddr) AuthType() string { return "inproc" }

// makeServerContext creates a server context from a client context.
// The server context:
//   - Inherits cancellation/deadline from the client context
//   - Does NOT inherit context values (prevents state leakage)
//   - Converts outgoing metadata to incoming metadata
//   - Sets peer info to inproc:0
//   - Stores the original client context (retrievable via ClientContext)
func makeServerContext(ctx context.Context) context.Context {
	newCtx := context.Context(noValuesContext{ctx})
	if meta, ok := metadata.FromOutgoingContext(ctx); ok {
		newCtx = metadata.NewIncomingContext(newCtx, meta)
	}
	newCtx = peer.NewContext(newCtx, &inprocessPeer)
	newCtx = context.WithValue(newCtx, &clientContextKey, ctx)
	return newCtx
}

// ClientContext returns the original client context from a server context
// created by an in-process channel, or nil if the context was not created
// by an in-process channel.
func ClientContext(ctx context.Context) context.Context {
	if clientCtx, ok := ctx.Value(&clientContextKey).(context.Context); ok {
		return clientCtx
	}
	return nil
}

// noValuesContext wraps a context but prevents access to its values.
// This prevents leaking client-side state to the server.
// Cancellation and deadline are still propagated.
type noValuesContext struct {
	context.Context
}

func (ctx noValuesContext) Value(key any) any {
	return nil
}
