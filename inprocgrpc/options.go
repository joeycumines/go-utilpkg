package inprocgrpc

import (
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
)

// Loop is the interface required by inprocgrpc for event loop integration.
// It provides methods for submitting tasks to the event loop for execution.
type Loop interface {
	// Submit submits a task to the external queue for execution on the loop.
	// Returns ErrLoopTerminated if the loop has been shut down.
	Submit(func()) error

	// SubmitInternal submits a task to the internal priority queue.
	// These tasks are processed before external tasks.
	// Returns ErrLoopTerminated if the loop has been shut down.
	SubmitInternal(func()) error
}

// channelOptions holds configuration for a [Channel] instance.
// Fields are ordered for optimal struct alignment.
type channelOptions struct {
	loop              Loop
	cloner            Cloner
	unaryInterceptor  grpc.UnaryServerInterceptor
	streamInterceptor grpc.StreamServerInterceptor
	clientStats       *statsHandlerHelper
	serverStats       *statsHandlerHelper
}

// Option configures a [Channel] instance. Options are applied during
// channel construction.
type Option interface {
	applyOption(*channelOptions) error
}

// channelOptionImpl implements [Option] via a closure.
type channelOptionImpl struct {
	fn func(*channelOptions) error
}

func (o *channelOptionImpl) applyOption(opts *channelOptions) error {
	return o.fn(opts)
}

// WithCloner configures the [Cloner] used for message isolation between
// client and server. If not set, [ProtoCloner] is used by default.
func WithCloner(cloner Cloner) Option {
	return &channelOptionImpl{fn: func(opts *channelOptions) error {
		opts.cloner = cloner
		return nil
	}}
}

// WithServerUnaryInterceptor configures a server-side unary interceptor
// for all RPCs dispatched through the channel.
func WithServerUnaryInterceptor(interceptor grpc.UnaryServerInterceptor) Option {
	return &channelOptionImpl{fn: func(opts *channelOptions) error {
		opts.unaryInterceptor = interceptor
		return nil
	}}
}

// WithServerStreamInterceptor configures a server-side stream interceptor
// for all streaming RPCs dispatched through the channel.
func WithServerStreamInterceptor(interceptor grpc.StreamServerInterceptor) Option {
	return &channelOptionImpl{fn: func(opts *channelOptions) error {
		opts.streamInterceptor = interceptor
		return nil
	}}
}

// WithClientStatsHandler configures a client-side stats handler.
// The handler must not be nil.
// [stats.Handler.TagConn] and [stats.Handler.HandleConn] will not be called.
func WithClientStatsHandler(h stats.Handler) Option {
	return &channelOptionImpl{fn: func(opts *channelOptions) error {
		if h == nil {
			return errors.New("inprocgrpc: client stats handler must not be nil")
		}
		opts.clientStats = &statsHandlerHelper{handler: h, isClient: true}
		return nil
	}}
}

// WithServerStatsHandler configures a server-side stats handler.
// The handler must not be nil.
// [stats.Handler.TagConn] and [stats.Handler.HandleConn] will not be called.
func WithServerStatsHandler(h stats.Handler) Option {
	return &channelOptionImpl{fn: func(opts *channelOptions) error {
		if h == nil {
			return errors.New("inprocgrpc: server stats handler must not be nil")
		}
		opts.serverStats = &statsHandlerHelper{handler: h, isClient: false}
		return nil
	}}
}

// WithLoop configures the event loop for the channel.
// The loop must not be nil.
func WithLoop(loop Loop) Option {
	return &channelOptionImpl{fn: func(opts *channelOptions) error {
		if loop == nil {
			return errors.New("inprocgrpc: loop must not be nil")
		}
		opts.loop = loop
		return nil
	}}
}

// resolveOptions applies the given options to a default [channelOptions].
func resolveOptions(opts []Option) (*channelOptions, error) {
	cfg := &channelOptions{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt.applyOption(cfg); err != nil {
			return nil, err
		}
	}
	if cfg.loop == nil {
		return nil, errors.New("inprocgrpc: loop must be provided via WithLoop")
	}
	return cfg, nil
}
