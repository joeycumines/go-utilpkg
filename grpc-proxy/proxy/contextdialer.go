package proxy

import (
	"context"
	"net"
	"time"
)

// ContextDialer is for use with grpc.WithContextDialer.
type ContextDialer func(ctx context.Context, addr string) (conn net.Conn, err error)

var dialer net.Dialer

// DialTCP is a convenience function, for use with [DialWithCancel].
func DialTCP(ctx context.Context, addr string) (net.Conn, error) {
	if ctx == nil {
		panic("grpc-proxy: DialTCP called with nil context")
	}
	return dialer.DialContext(ctx, "tcp", addr)
}

var _ ContextDialer = DialTCP

// DialWithCancel wraps a dialer function to ensure that it respects the provided context.
func DialWithCancel(ctx context.Context, dialer ContextDialer) ContextDialer {
	if ctx == nil {
		panic("grpc-proxy: DialWithCancel called with nil context")
	}
	if dialer == nil {
		panic("grpc-proxy: DialWithCancel called with nil dialer")
	}
	return func(ctx2 context.Context, addr string) (net.Conn, error) {
		if ctx2.Err() != nil {
			return nil, ctx2.Err()
		}
		if ctx.Err() != nil {
			return nil, context.Canceled
		}
		ctx2, cancel := context.WithCancel(ctx2)
		defer cancel()
		defer context.AfterFunc(ctx, cancel)() // stop on exit
		return dialer(ctx2, addr)
	}
}

// DialWithTimeout wraps a dialer function to ensure that it respects the provided timeout.
func DialWithTimeout(timeout time.Duration, dialer ContextDialer) ContextDialer {
	return func(ctx context.Context, addr string) (net.Conn, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return dialer(ctx, addr)
	}
}
