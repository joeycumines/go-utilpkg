package inprocgrpc

import (
	"context"
	"fmt"
	"io"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/joeycumines/go-inprocgrpc/internal/callopts"
	"github.com/joeycumines/go-inprocgrpc/internal/grpcutil"
	"github.com/joeycumines/go-inprocgrpc/internal/stream"
	"github.com/joeycumines/go-inprocgrpc/internal/transport"
)

// Channel is a gRPC channel where RPCs amount to in-process method calls.
// It functions as both a [grpc.ClientConnInterface] (client side) and a
// [grpc.ServiceRegistrar] (server side).
//
// All RPC communication is event-loop-driven. The [Loop] owns
// all stream state and coordinates message delivery. Handler goroutines
// run concurrently with blocking adapters wrapping the callback-based
// stream core; completion is submitted back to the loop for thread safety.
//
// Create instances with [NewChannel]. The zero value is not usable.
type Channel struct {
	cloner         Cloner
	loop           Loop
	unaryInt       grpc.UnaryServerInterceptor
	streamInt      grpc.StreamServerInterceptor
	clientStats    *statsHandlerHelper
	serverStats    *statsHandlerHelper
	streamHandlers map[string]StreamHandlerFunc
	handlers       handlerMap
	cloneDisabled  bool
}

var (
	_ grpc.ClientConnInterface = (*Channel)(nil)
	_ grpc.ServiceRegistrar    = (*Channel)(nil)
)

// NewChannel creates a new event-loop-driven in-process gRPC channel.
//
// The loop must be provided via [WithLoop] option. The loop should be running
// before RPCs are initiated. All RPC state is managed on the loop goroutine.
//
// Options configure the event loop, cloner, interceptors, and stats handlers.
// If no cloner is specified, [ProtoCloner] is used. NewChannel panics if any
// option fails validation (invalid options are programming errors).
func NewChannel(opts ...Option) *Channel {
	cfg, err := resolveOptions(opts)
	if err != nil {
		panic(fmt.Sprintf("inprocgrpc: %s", err))
	}
	cloner := Cloner(ProtoCloner{})
	if cfg.cloner != nil {
		cloner = cfg.cloner
	}
	return &Channel{
		loop:          cfg.loop,
		cloner:        cloner,
		cloneDisabled: cfg.cloneDisabled,
		unaryInt:      cfg.unaryInterceptor,
		streamInt:     cfg.streamInterceptor,
		clientStats:   cfg.clientStats,
		serverStats:   cfg.serverStats,
	}
}

// RegisterService registers the given service and implementation. Like a normal
// gRPC server, only a single implementation is allowed for a particular service.
// Panics if a handler is already registered for the service.
func (c *Channel) RegisterService(desc *grpc.ServiceDesc, svr any) {
	c.handlers.registerService(desc, svr)
}

// GetServiceInfo returns information about registered services.
func (c *Channel) GetServiceInfo() map[string]grpc.ServiceInfo {
	return c.handlers.getServiceInfo()
}

// RegisterStreamHandler registers a non-blocking [StreamHandlerFunc]
// for the given full method name (e.g., "/pkg.Service/Method").
//
// Stream handlers are invoked directly on the event loop goroutine
// when a matching RPC arrives, providing a zero-overhead pathway for
// event-loop-native handlers (such as Goja/JS). They take priority
// over handlers registered via [Channel.RegisterService].
//
// Stream handlers bypass server interceptors and stats handlers.
// They operate at a lower level than the standard gRPC handler
// pipeline - interceptor and stats functionality should be
// implemented within the handler itself if needed.
//
// The method name must start with "/". RegisterStreamHandler panics
// if a stream handler is already registered for the given method.
//
// Stream handlers must not be registered concurrently with RPC
// dispatch. Register all handlers during setup, before starting
// RPCs - consistent with the [grpc.ServiceRegistrar] contract.
func (c *Channel) RegisterStreamHandler(method string, handler StreamHandlerFunc) {
	if len(method) == 0 || method[0] != '/' {
		panic(fmt.Sprintf("inprocgrpc: method name must start with '/': %q", method))
	}
	if handler == nil {
		panic("inprocgrpc: stream handler must not be nil")
	}
	if c.streamHandlers == nil {
		c.streamHandlers = make(map[string]StreamHandlerFunc)
	}
	if _, ok := c.streamHandlers[method]; ok {
		panic(fmt.Sprintf("inprocgrpc: stream handler already registered for %q", method))
	}
	c.streamHandlers[method] = handler
}

// Invoke satisfies [grpc.ClientConnInterface] and supports sending unary RPCs.
// All stream state is managed on the event loop goroutine.
func (c *Channel) Invoke(ctx context.Context, method string, req, resp any, opts ...grpc.CallOption) error {
	copts := callopts.GetCallOptions(opts)
	copts.SetPeer(&inprocessPeer)

	if isNil(req) {
		return status.Errorf(codes.Internal, "request message is nil")
	}

	if len(method) == 0 || method[0] != '/' {
		method = "/" + method
	}
	ctx, err := callopts.ApplyPerRPCCreds(ctx, copts, fmt.Sprintf("inproc:0%s", method), true)
	if err != nil {
		return err
	}

	strs := strings.SplitN(method[1:], "/", 2)
	if len(strs) != 2 {
		return status.Errorf(codes.InvalidArgument, "malformed method name: %s", method)
	}

	// Check for callback-based stream handler first.
	if sh, ok := c.streamHandlers[method]; ok {
		return c.invokeStreamHandler(ctx, method, req, resp, copts, sh)
	}

	serviceName := strs[0]
	methodName := strs[1]
	sd, handler := c.handlers.queryService(serviceName)
	if sd == nil {
		return status.Errorf(codes.Unimplemented, "service %s not implemented", serviceName)
	}
	md := grpcutil.FindUnaryMethod(methodName, sd.Methods)
	if md == nil {
		return status.Errorf(codes.Unimplemented, "method %s/%s not implemented", serviceName, methodName)
	}

	// Clone request on caller goroutine (off-loop).
	var clonedReq any
	var cloneErr error
	if c.cloneDisabled {
		clonedReq = req
	} else {
		clonedReq, cloneErr = c.cloner.Clone(req)
		if cloneErr != nil {
			return cloneErr
		}
	}

	// Client stats: tag and begin.
	if c.clientStats != nil {
		ctx = c.clientStats.tagRPC(ctx, method)
		c.clientStats.begin(ctx, false, false)
		c.clientStats.outPayload(ctx, req)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type invokeResult struct {
		err error
	}
	resCh := make(chan invokeResult, 1)

	submitErr := c.loop.Submit(func() {
		// On the loop: set up server context and handler.
		uts := transport.UnaryServerTransportStream{Name: method}
		svrCtx := grpc.NewContextWithServerTransportStream(makeServerContext(ctx), &uts)

		codec := func(out any) error {
			if c.cloneDisabled {
				shallowCopy(out, clonedReq)
				return nil
			}
			return c.cloner.Copy(out, clonedReq)
		}

		// Server stats: tag and begin.
		if c.serverStats != nil {
			svrCtx = c.serverStats.tagRPC(svrCtx, method)
			c.serverStats.begin(svrCtx, false, false)
		}

		serverCtx := svrCtx
		serverStats := c.serverStats
		cloner := c.cloner

		// Launch handler goroutine. The handler runs off-loop;
		// its completion is submitted back to the loop.
		go func() {
			defer uts.Finish()
			v, svrErr := md.Handler(handler, serverCtx, codec, c.unaryInt)
			if err := c.loop.Submit(func() {
				if svrErr != nil {
					// Handler failed.
					if serverStats != nil {
						serverStats.end(serverCtx, svrErr)
					}
					if h := uts.GetHeaders(); len(h) > 0 {
						copts.SetHeaders(h)
						if c.clientStats != nil {
							c.clientStats.inHeader(ctx, h, method)
						}
					}
					if t := uts.GetTrailers(); len(t) > 0 {
						copts.SetTrailers(t)
						if c.clientStats != nil {
							c.clientStats.inTrailer(ctx, t)
						}
					}
					resCh <- invokeResult{err: grpcutil.TranslateContextError(svrErr)}
					return
				}

				// Handler succeeded - send response back.
				if serverStats != nil {
					serverStats.end(serverCtx, nil)
				}
				if isNil(v) {
					resCh <- invokeResult{err: status.Error(codes.Internal,
						"handler returned neither error nor response message")}
					return
				}
				if h := uts.GetHeaders(); len(h) > 0 {
					copts.SetHeaders(h)
					if c.clientStats != nil {
						c.clientStats.inHeader(ctx, h, method)
					}
				}
				if c.cloneDisabled {
					shallowCopy(resp, v)
				} else if copyErr := cloner.Copy(resp, v); copyErr != nil {
					resCh <- invokeResult{err: copyErr}
					return
				}
				if c.clientStats != nil {
					c.clientStats.inPayload(ctx, v)
				}
				if t := uts.GetTrailers(); len(t) > 0 {
					copts.SetTrailers(t)
					if c.clientStats != nil {
						c.clientStats.inTrailer(ctx, t)
					}
				}
				resCh <- invokeResult{err: nil}
			}); err != nil {
				// Loop terminated - unblock the caller.
				resCh <- invokeResult{err: status.Error(codes.Unavailable, "event loop not running")}
			}
		}()
	})

	if submitErr != nil {
		if c.clientStats != nil {
			c.clientStats.end(ctx, submitErr)
		}
		return status.Error(codes.Unavailable, "event loop not running")
	}

	var resultErr error
	select {
	case r := <-resCh:
		resultErr = r.err
	case <-ctx.Done():
		resultErr = grpcutil.TranslateContextError(ctx.Err())
	}

	if c.clientStats != nil {
		c.clientStats.end(ctx, resultErr)
	}
	return resultErr
}

// NewStream satisfies [grpc.ClientConnInterface] and supports sending streaming RPCs.
// All stream state is managed on the event loop goroutine.
func (c *Channel) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	copts := callopts.GetCallOptions(opts)
	copts.SetPeer(&inprocessPeer)

	if len(method) == 0 || method[0] != '/' {
		method = "/" + method
	}
	ctx, err := callopts.ApplyPerRPCCreds(ctx, copts, fmt.Sprintf("inproc:0%s", method), true)
	if err != nil {
		return nil, err
	}

	strs := strings.SplitN(method[1:], "/", 2)
	if len(strs) != 2 {
		return nil, status.Errorf(codes.InvalidArgument, "malformed method name: %s", method)
	}

	// Check for callback-based stream handler first.
	if sh, ok := c.streamHandlers[method]; ok {
		return c.newStreamWithHandler(ctx, desc, method, copts, sh)
	}

	serviceName := strs[0]
	methodName := strs[1]
	sd, handler := c.handlers.queryService(serviceName)
	if sd == nil {
		return nil, status.Errorf(codes.Unimplemented, "service %s not implemented", serviceName)
	}
	smd := grpcutil.FindStreamingMethod(methodName, sd.Streams)
	if smd == nil {
		return nil, status.Errorf(codes.Unimplemented, "method %s/%s not implemented", serviceName, methodName)
	}

	// Client stats: tag and begin.
	if c.clientStats != nil {
		ctx = c.clientStats.tagRPC(ctx, method)
		c.clientStats.begin(ctx, smd.ClientStreams, smd.ServerStreams)
	}

	ctx, cancel := context.WithCancel(ctx)

	initCh := make(chan *clientStreamAdapter, 1)

	submitErr := c.loop.Submit(func() {
		// On the loop: create RPCState and adapters.
		state := &stream.RPCState{
			Method: method,
		}

		svrCtx, svrCancel := context.WithCancel(makeServerContext(ctx))

		ssAdapter := &serverStreamAdapter{
			ctx:           svrCtx,
			loop:          c.loop,
			state:         state,
			cloner:        c.cloner,
			cloneDisabled: c.cloneDisabled,
			stats:         c.serverStats,
			method:        method,
		}

		sts := &transport.ServerTransportStream{Name: method, Stream: ssAdapter}
		ssAdapter.ctx = grpc.NewContextWithServerTransportStream(svrCtx, sts)

		if c.serverStats != nil {
			ssAdapter.ctx = c.serverStats.tagRPC(ssAdapter.ctx, method)
			c.serverStats.begin(ssAdapter.ctx, smd.ClientStreams, smd.ServerStreams)
		}

		// Launch handler goroutine.
		serverCtx := ssAdapter.ctx
		serverStats := c.serverStats
		streamInterceptor := c.streamInt

		go func() {
			var svrErr error
			if streamInterceptor != nil {
				info := grpc.StreamServerInfo{
					FullMethod:     method,
					IsClientStream: smd.ClientStreams,
					IsServerStream: smd.ServerStreams,
				}
				svrErr = streamInterceptor(handler, ssAdapter, &info, smd.Handler)
			} else {
				svrErr = smd.Handler(handler, ssAdapter)
			}
			if err := c.loop.Submit(func() {
				state.FinishWithTrailers(svrErr)
				if serverStats != nil {
					serverStats.outTrailer(serverCtx, state.ResponseTrailers)
					serverStats.end(serverCtx, svrErr)
				}
				svrCancel()
			}); err != nil {
				// Loop terminated - just cancel the server context.
				// Direct access to state is unsafe because the cleanup
				// goroutine's Submit callback may still be executing
				// concurrently on the loop goroutine.
				svrCancel()
			}
		}()

		csAdapter := &clientStreamAdapter{
			ctx:            ctx,
			cancel:         cancel,
			loop:           c.loop,
			state:          state,
			cloner:         c.cloner,
			cloneDisabled:  c.cloneDisabled,
			method:         method,
			copts:          copts,
			stats:          c.clientStats,
			responseStream: desc.ServerStreams,
		}
		csAdapter.setFinalizer()

		// Watch for context cancellation to close streams.
		go func() {
			select {
			case <-ctx.Done():
				c.loop.Submit(func() {
					if !state.Requests.Closed() {
						state.Requests.Close(ctx.Err())
					}
					if !state.Responses.Closed() {
						state.Responses.Close(ctx.Err())
					}
				})
			case <-svrCtx.Done():
				// Server done - no cleanup needed.
			}
		}()

		initCh <- csAdapter
	})

	if submitErr != nil {
		cancel()
		if c.clientStats != nil {
			c.clientStats.end(ctx, submitErr)
		}
		return nil, status.Error(codes.Unavailable, "event loop not running")
	}

	return <-initCh, nil
}

// invokeStreamHandler handles unary RPCs dispatched to a callback-based
// [StreamHandlerFunc]. It creates an [RPCStream], delivers the request,
// calls the handler on the loop, and waits for the response.
func (c *Channel) invokeStreamHandler(
	ctx context.Context,
	method string,
	req, resp any,
	copts *callopts.CallOptions,
	handler StreamHandlerFunc,
) error {
	// Clone request off-loop.
	var clonedReq any
	var cloneErr error
	if c.cloneDisabled {
		clonedReq = req
	} else {
		clonedReq, cloneErr = c.cloner.Clone(req)
		if cloneErr != nil {
			return cloneErr
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type invokeResult struct {
		err error
	}
	resCh := make(chan invokeResult, 1)

	submitErr := c.loop.Submit(func() {
		state := &stream.RPCState{Method: method}
		svrCtx := makeServerContext(ctx)

		// Deliver the cloned request and close the request stream.
		state.Requests.Send(clonedReq)
		state.Requests.Close(nil)

		rpcStream := &RPCStream{state: state}

		// Register a response waiter that copies the response and
		// unblocks the caller.
		cloner := c.cloner
		state.Responses.Recv(func(msg any, err error) {
			if err != nil {
				copts.SetHeaders(state.ResponseHeaders)
				copts.SetTrailers(state.ResponseTrailers)
				resCh <- invokeResult{err: grpcutil.TranslateContextError(err)}
				return
			}
			if c.cloneDisabled {
				shallowCopy(resp, msg)
			} else if copyErr := cloner.Copy(resp, msg); copyErr != nil {
				resCh <- invokeResult{err: copyErr}
				return
			}
			// Drain the EOF from Finish. Capture headers/trailers
			// here, after Finish has been called, so that any trailers
			// set between Send and Finish are included.
			state.Responses.Recv(func(_ any, finishErr error) {
				copts.SetHeaders(state.ResponseHeaders)
				copts.SetTrailers(state.ResponseTrailers)
				if finishErr != nil && finishErr != io.EOF {
					// Handler sent a response then finished with an
					// error - the error takes priority (standard gRPC
					// unary semantics).
					resCh <- invokeResult{err: grpcutil.TranslateContextError(finishErr)}
				} else {
					resCh <- invokeResult{err: nil}
				}
			})
		})

		// Invoke the handler on the loop goroutine.
		handler(svrCtx, rpcStream)
	})

	if submitErr != nil {
		return status.Error(codes.Unavailable, "event loop not running")
	}

	select {
	case r := <-resCh:
		return r.err
	case <-ctx.Done():
		return grpcutil.TranslateContextError(ctx.Err())
	}
}

// newStreamWithHandler handles streaming RPCs dispatched to a callback-based
// [StreamHandlerFunc]. It creates an [RPCStream], calls the handler on
// the loop, and returns a [clientStreamAdapter] for client-side I/O.
func (c *Channel) newStreamWithHandler(
	ctx context.Context,
	desc *grpc.StreamDesc,
	method string,
	copts *callopts.CallOptions,
	handler StreamHandlerFunc,
) (grpc.ClientStream, error) {
	ctx, cancel := context.WithCancel(ctx)

	initCh := make(chan *clientStreamAdapter, 1)

	submitErr := c.loop.Submit(func() {
		state := &stream.RPCState{Method: method}
		svrCtx, svrCancel := context.WithCancel(makeServerContext(ctx))

		rpcStream := &RPCStream{state: state, onFinish: svrCancel}

		csAdapter := &clientStreamAdapter{
			ctx:            ctx,
			cancel:         cancel,
			loop:           c.loop,
			state:          state,
			cloner:         c.cloner,
			cloneDisabled:  c.cloneDisabled,
			method:         method,
			copts:          copts,
			stats:          nil, // stream handlers bypass stats
			responseStream: desc.ServerStreams,
		}
		csAdapter.setFinalizer()

		// Watch for context cancellation.
		go func() {
			select {
			case <-ctx.Done():
				c.loop.Submit(func() {
					if !state.Requests.Closed() {
						state.Requests.Close(ctx.Err())
					}
					if !state.Responses.Closed() {
						state.Responses.Close(ctx.Err())
					}
				})
			case <-svrCtx.Done():
			}
		}()

		// Invoke handler on the loop.
		handler(svrCtx, rpcStream)

		initCh <- csAdapter
	})

	if submitErr != nil {
		cancel()
		return nil, status.Error(codes.Unavailable, "event loop not running")
	}

	return <-initCh, nil
}
