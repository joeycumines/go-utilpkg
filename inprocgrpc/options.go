package inprocgrpc

import (
	"errors"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"
)

// Loop is the interface required by inprocgrpc for event loop integration.
// It provides methods for submitting tasks to the event loop for execution.
type Loop interface {
	// Submit submits a task to the external queue for execution on the loop.
	// A nil result means the task was admitted. It may execute once in an owner
	// scope, or Done may prove that it can no longer execute so inprocgrpc can
	// transfer exclusive ownership to recovery. While the scheduler remains
	// live, an abnormal callback must not prevent later admitted work.
	// Returns ErrLoopTerminated if the loop has been shut down.
	Submit(func()) error

	// SubmitInternal submits a task to the internal priority queue.
	// These tasks are processed before external tasks.
	// It has the same owner-survival requirement as Submit.
	// Returns ErrLoopTerminated if the loop has been shut down.
	SubmitInternal(func()) error

	// Done closes after scheduler terminal cleanup, when no callback accepted
	// by Submit or SubmitInternal can still execute.
	Done() <-chan struct{}
}

type channelConfig struct {
	loop              Loop
	cloner            Cloner
	unaryInterceptor  grpc.UnaryServerInterceptor
	streamInterceptor grpc.StreamServerInterceptor
	clientStats       *statsHandlerHelper
	serverStats       *statsHandlerHelper
	streamBuffer      int
	cloneDisabled     bool
}

// ChannelOption configures a [Channel]. Values are produced by the With*
// constructors in this package. [NewChannel] panics if an option is nil or
// invalid.
type ChannelOption interface {
	applyChannelOption(*channelConfig) error
}

// ClonerOption configures message cloning for a [Channel].
type ClonerOption struct{ cloner Cloner }

// WithCloner configures the [Cloner] used for message isolation between
// client and server. If not set, [ProtoCloner] is used by default.
func WithCloner(cloner Cloner) *ClonerOption { return &ClonerOption{cloner: cloner} }

func (o *ClonerOption) applyChannelOption(cfg *channelConfig) error {
	if o == nil || isNil(o.cloner) {
		return errors.New("cloner must not be nil")
	}
	cfg.cloner = o.cloner
	return nil
}

// ServerUnaryInterceptorOption configures a unary server interceptor.
type ServerUnaryInterceptorOption struct {
	interceptor grpc.UnaryServerInterceptor
}

// WithServerUnaryInterceptor configures a non-nil server-side unary
// interceptor for all RPCs dispatched through the channel.
func WithServerUnaryInterceptor(interceptor grpc.UnaryServerInterceptor) *ServerUnaryInterceptorOption {
	return &ServerUnaryInterceptorOption{interceptor: interceptor}
}

func (o *ServerUnaryInterceptorOption) applyChannelOption(cfg *channelConfig) error {
	if o == nil || o.interceptor == nil {
		return errors.New("unary server interceptor must not be nil")
	}
	cfg.unaryInterceptor = o.interceptor
	return nil
}

// ServerStreamInterceptorOption configures a streaming server interceptor.
type ServerStreamInterceptorOption struct {
	interceptor grpc.StreamServerInterceptor
}

// WithServerStreamInterceptor configures a non-nil server-side stream
// interceptor for all streaming RPCs dispatched through the channel.
func WithServerStreamInterceptor(interceptor grpc.StreamServerInterceptor) *ServerStreamInterceptorOption {
	return &ServerStreamInterceptorOption{interceptor: interceptor}
}

func (o *ServerStreamInterceptorOption) applyChannelOption(cfg *channelConfig) error {
	if o == nil || o.interceptor == nil {
		return errors.New("stream server interceptor must not be nil")
	}
	cfg.streamInterceptor = o.interceptor
	return nil
}

// ClientStatsHandlerOption configures client-side RPC stats.
type ClientStatsHandlerOption struct{ handler stats.Handler }

// WithClientStatsHandler configures a client-side stats handler.
// The handler must not be nil.
// [stats.Handler.TagConn] and [stats.Handler.HandleConn] will not be called.
func WithClientStatsHandler(h stats.Handler) *ClientStatsHandlerOption {
	return &ClientStatsHandlerOption{handler: h}
}

func (o *ClientStatsHandlerOption) applyChannelOption(cfg *channelConfig) error {
	if o == nil || isNil(o.handler) {
		return errors.New("client stats handler must not be nil")
	}
	cfg.clientStats = &statsHandlerHelper{handler: o.handler, isClient: true}
	return nil
}

// ServerStatsHandlerOption configures server-side RPC stats.
type ServerStatsHandlerOption struct{ handler stats.Handler }

// WithServerStatsHandler configures a server-side stats handler.
// The handler must not be nil.
// [stats.Handler.TagConn] and [stats.Handler.HandleConn] will not be called.
func WithServerStatsHandler(h stats.Handler) *ServerStatsHandlerOption {
	return &ServerStatsHandlerOption{handler: h}
}

func (o *ServerStatsHandlerOption) applyChannelOption(cfg *channelConfig) error {
	if o == nil || isNil(o.handler) {
		return errors.New("server stats handler must not be nil")
	}
	cfg.serverStats = &statsHandlerHelper{handler: o.handler, isClient: false}
	return nil
}

// LoopOption configures the scheduler used by a [Channel].
type LoopOption struct{ loop Loop }

// WithLoop configures the event loop for the channel. The loop and its stable
// terminal-cleanup signal returned by [Loop.Done] must not be nil.
func WithLoop(loop Loop) *LoopOption { return &LoopOption{loop: loop} }

func (o *LoopOption) applyChannelOption(cfg *channelConfig) error {
	if o == nil || isNil(o.loop) {
		return errors.New("loop must not be nil")
	}
	if o.loop.Done() == nil {
		return errors.New("loop Done signal must not be nil")
	}
	cfg.loop = o.loop
	return nil
}

// CloneDisabledOption disables message cloning for a [Channel].
type CloneDisabledOption struct{}

// WithCloneDisabled disables the default behavior of cloning messages
// passed between client and server.
//
// SAFETY WARNING: This option removes the isolation between client and server.
// If the client modifies a message after sending it (but before the server processes it),
// or if the server retains a reference to the request message and the client modifies it,
// data races or unexpected behavior may occur. This mode is unsafe by default and
// should only be used when the caller guarantees ownership transfer or immutable messages.
func WithCloneDisabled() *CloneDisabledOption { return &CloneDisabledOption{} }

func (o *CloneDisabledOption) applyChannelOption(cfg *channelConfig) error {
	if o == nil {
		return errors.New("clone-disabled option must not be nil")
	}
	cfg.cloneDisabled = true
	return nil
}

// StreamBufferOption configures the finite message capacity of each RPC
// direction. A full buffer applies backpressure to blocking gRPC adapters and
// returns ResourceExhausted to non-blocking callback senders.
type StreamBufferOption struct{ size int }

// WithStreamBuffer configures the positive per-direction buffered message
// count. New channels use a capacity of 16 when this option is omitted.
func WithStreamBuffer(size int) *StreamBufferOption {
	return &StreamBufferOption{size: size}
}

func (o *StreamBufferOption) applyChannelOption(cfg *channelConfig) error {
	if o == nil {
		return errors.New("stream buffer option must not be nil")
	}
	if o.size <= 0 {
		return fmt.Errorf("stream buffer must be positive, got %d", o.size)
	}
	cfg.streamBuffer = o.size
	return nil
}

func resolveOptions(opts []ChannelOption) (*channelConfig, error) {
	cfg := &channelConfig{streamBuffer: 16}
	for index, opt := range opts {
		if opt == nil {
			return nil, fmt.Errorf("channel option %d is nil", index)
		}
		if err := opt.applyChannelOption(cfg); err != nil {
			return nil, fmt.Errorf(
				"channel option %d: %w",
				index,
				err,
			)
		}
	}
	if cfg.loop == nil {
		return nil, errors.New("loop must be provided via WithLoop")
	}
	return cfg, nil
}

var (
	_ ChannelOption = (*ClonerOption)(nil)
	_ ChannelOption = (*ServerUnaryInterceptorOption)(nil)
	_ ChannelOption = (*ServerStreamInterceptorOption)(nil)
	_ ChannelOption = (*ClientStatsHandlerOption)(nil)
	_ ChannelOption = (*ServerStatsHandlerOption)(nil)
	_ ChannelOption = (*LoopOption)(nil)
	_ ChannelOption = (*CloneDisabledOption)(nil)
	_ ChannelOption = (*StreamBufferOption)(nil)
)
