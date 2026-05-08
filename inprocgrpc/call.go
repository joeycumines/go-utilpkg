package inprocgrpc

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/joeycumines/go-inprocgrpc/internal/callopts"
	"github.com/joeycumines/go-inprocgrpc/internal/grpcutil"
	"github.com/joeycumines/go-inprocgrpc/internal/stream"
	"github.com/joeycumines/go-inprocgrpc/internal/transport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type rpcTarget struct {
	callback      StreamHandlerFunc
	unary         *grpc.MethodDesc
	stream        *grpc.StreamDesc
	service       any
	clientStreams bool
	serverStreams bool
	clientSend    bool
	clientReceive bool
}

type rpcInitResult struct {
	stream     *clientStreamAdapter
	err        error
	completion <-chan struct{}
}

// Invoke performs one unary RPC through the event-loop-owned transport.
func (c *Channel) Invoke(
	ctx context.Context,
	method string,
	request any,
	response any,
	opts ...grpc.CallOption,
) (err error) {
	if isNil(request) {
		return status.Error(codes.Internal, "request message is nil")
	}
	if isNil(response) {
		return status.Error(codes.Internal, "response message is nil")
	}
	method, service, operation, err := validateMethod(method)
	if err != nil {
		return err
	}
	target, err := c.lookupUnary(method, service, operation)
	if err != nil {
		return err
	}
	ctx, copts, err := prepareCall(ctx, method, opts)
	if err != nil {
		return err
	}
	client, err := c.startRPC(ctx, method, copts, target)
	if err != nil {
		return err
	}
	defer func() {
		<-client.life.finishClientObservation(err)
	}()
	if err = client.SendMsg(request); err != nil {
		return err
	}
	err = client.RecvMsg(response)
	return err
}

// NewStream creates one client stream through the event-loop-owned transport.
func (c *Channel) NewStream(
	ctx context.Context,
	desc *grpc.StreamDesc,
	method string,
	opts ...grpc.CallOption,
) (grpc.ClientStream, error) {
	if desc == nil {
		return nil, status.Error(codes.InvalidArgument, "stream descriptor is nil")
	}
	method, service, operation, err := validateMethod(method)
	if err != nil {
		return nil, err
	}
	target, err := c.lookupClientStream(
		method,
		service,
		operation,
		desc,
	)
	if err != nil {
		return nil, err
	}
	ctx, copts, err := prepareCall(ctx, method, opts)
	if err != nil {
		return nil, err
	}
	return c.startRPC(ctx, method, copts, target)
}

func prepareCall(
	ctx context.Context,
	method string,
	opts []grpc.CallOption,
) (context.Context, *callopts.CallOptions, error) {
	if ctx == nil {
		return nil, nil, status.Error(codes.InvalidArgument, "context is nil")
	}
	copts := callopts.GetCallOptions(opts)
	copts.SetPeer(&inprocessPeer)
	credentialCtx := ctx
	err := containRPCOperation("per-RPC credentials", func() error {
		var credentialErr error
		credentialCtx, credentialErr = callopts.ApplyPerRPCCreds(
			ctx,
			copts,
			fmt.Sprintf("inproc:0%s", method),
			true,
		)
		if credentialErr != nil {
			return normalizeRPCError(credentialErr)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return credentialCtx, copts, nil
}

func (c *Channel) lookupUnary(method, service, operation string) (rpcTarget, error) {
	c.registrationMu.RLock()
	defer c.registrationMu.RUnlock()
	if callback, ok := c.streamHandlers[method]; ok {
		return rpcTarget{callback: callback}, nil
	}
	desc, handler := c.handlers.queryService(service)
	if desc == nil {
		return rpcTarget{}, status.Errorf(codes.Unimplemented, "service %s not implemented", service)
	}
	methodDesc := grpcutil.FindUnaryMethod(operation, desc.Methods)
	if methodDesc == nil {
		return rpcTarget{}, status.Errorf(
			codes.Unimplemented,
			"method %s/%s not implemented",
			service,
			operation,
		)
	}
	return rpcTarget{unary: methodDesc, service: handler}, nil
}

func (c *Channel) lookupClientStream(
	method string,
	service string,
	operation string,
	clientDesc *grpc.StreamDesc,
) (rpcTarget, error) {
	c.registrationMu.RLock()
	defer c.registrationMu.RUnlock()
	if callback, ok := c.streamHandlers[method]; ok {
		return rpcTarget{
			callback:      callback,
			clientStreams: clientDesc.ClientStreams,
			serverStreams: clientDesc.ServerStreams,
			clientSend:    clientDesc.ClientStreams,
			clientReceive: clientDesc.ServerStreams,
		}, nil
	}
	desc, handler := c.handlers.queryService(service)
	if desc == nil {
		return rpcTarget{}, status.Errorf(
			codes.Unimplemented,
			"service %s not implemented",
			service,
		)
	}
	if streamDesc := grpcutil.FindStreamingMethod(
		operation,
		desc.Streams,
	); streamDesc != nil {
		return rpcTarget{
			stream:        streamDesc,
			service:       handler,
			clientStreams: streamDesc.ClientStreams,
			serverStreams: streamDesc.ServerStreams,
			clientSend:    clientDesc.ClientStreams,
			clientReceive: clientDesc.ServerStreams,
		}, nil
	}
	if !clientDesc.ClientStreams && !clientDesc.ServerStreams {
		if methodDesc := grpcutil.FindUnaryMethod(
			operation,
			desc.Methods,
		); methodDesc != nil {
			// Intentional in-process extension: a false/false client descriptor
			// may drive a generated unary MethodDesc through one SendMsg and one
			// RecvMsg. A registered stream of the same name always wins above.
			return rpcTarget{unary: methodDesc, service: handler}, nil
		}
	}
	return rpcTarget{}, status.Errorf(
		codes.Unimplemented,
		"method %s/%s not implemented",
		service,
		operation,
	)
}

func (c *Channel) startRPC(
	ctx context.Context,
	method string,
	copts *callopts.CallOptions,
	target rpcTarget,
) (*clientStreamAdapter, error) {
	if err := ctx.Err(); err != nil {
		return nil, normalizeRPCError(err)
	}
	callback := target.callback != nil
	var clientStats *rpcStats
	if !callback && c.clientStats != nil {
		var statsErr error
		ctx, statsErr = c.clientStats.tagRPCSafe(ctx, method)
		if statsErr != nil {
			return nil, statsErr
		}
		clientStats, statsErr = c.clientStats.startRPC(
			ctx,
			method,
			target.clientStreams,
			target.serverStreams,
		)
		if statsErr != nil {
			return nil, statsErr
		}
		if outgoing, ok := metadata.FromOutgoingContext(ctx); ok {
			_ = clientStats.outHeader(outgoing)
		} else {
			_ = clientStats.outHeader(nil)
		}
	}
	clientCtx, clientCancel := context.WithCancel(ctx)
	initCh := make(chan rpcInitResult, 1)

	if err := c.loop.Submit(func() {
		state := stream.NewRPCState(method, c.streamBuffer)
		life := newRPCLifecycle(
			c.loop,
			state,
			clientCancel,
			true,
			true,
		)
		life.clientStats = clientStats
		initializationError := func(failure error) rpcInitResult {
			life.serverAbort(failure)
			if winner, terminal := life.terminalSelection(); terminal {
				return rpcInitResult{
					err:        winner,
					completion: life.finishClientObservation(winner),
				}
			}
			return rpcInitResult{
				err:        failure,
				completion: life.finishClientObservation(failure),
			}
		}

		client := &clientStreamAdapter{
			ctx:           clientCtx,
			callerCtx:     ctx,
			cancel:        clientCancel,
			loop:          c.loop,
			life:          life,
			state:         state,
			cloner:        c.cloner,
			copts:         copts,
			stats:         clientStats,
			method:        method,
			cloneDisabled: c.cloneDisabled,
			clientStreams: target.clientSend,
			serverStreams: target.clientReceive,
		}
		initializationComplete := false
		defer func() {
			if initializationComplete {
				return
			}
			failure := internalRPCError("RPC initialization", recover())
			initCh <- initializationError(failure)
		}()

		serverBase, serverCancel := context.WithCancel(makeServerContext(clientCtx))
		var serverAdapter *serverStreamAdapter
		serverCtx := serverBase
		var unaryTransport *transport.UnaryServerTransportStream
		switch {
		case target.unary != nil:
			unaryTransport = &transport.UnaryServerTransportStream{Name: method}
			serverCtx = grpc.NewContextWithServerTransportStream(serverCtx, unaryTransport)
			serverAdapter = c.newServerAdapter(serverCtx, life, state, target)
		case target.stream != nil:
			serverAdapter = c.newServerAdapter(serverCtx, life, state, target)
			serverTransport := &transport.ServerTransportStream{
				Name:   method,
				Stream: serverAdapter,
			}
			serverCtx = grpc.NewContextWithServerTransportStream(serverCtx, serverTransport)
			serverAdapter.ctx = serverCtx
		}

		life.setServer(serverCancel, nil)
		if !callback && c.serverStats != nil {
			life.beginServerSetup()
		}
		life.watch(clientCtx)
		completeInitialization := func(
			serverCtx context.Context,
			handlerCancel context.CancelFunc,
			serverStats *rpcStats,
		) {
			if serverAdapter != nil {
				serverAdapter.ctx = serverCtx
				serverAdapter.stats = serverStats
			}
			life.setServer(handlerCancel, serverStats)
			var (
				callbackStream *RPCStream
				callbackTurn   *CallbackTurn
			)
			if target.callback != nil {
				callbackStream, callbackTurn = c.admitCallback(
					life,
					state,
					target,
				)
				if callbackTurn == nil {
					initCh <- initializationError(unavailableError())
					return
				}
			}
			initCh <- rpcInitResult{stream: client}
			switch {
			case target.callback != nil:
				c.startCallback(
					serverCtx,
					callbackStream,
					callbackTurn,
					target,
				)
			case target.unary != nil:
				c.startUnary(serverAdapter, unaryTransport, life, target)
			case target.stream != nil:
				c.startStream(serverAdapter, life, method, target)
			}
		}
		if !callback && c.serverStats != nil {
			obligation, obligationErr := life.beginStatsObligation()
			if obligationErr != nil {
				life.finishServerSetup(serverCancel, nil)
				initializationComplete = true
				completeInitialization(serverCtx, serverCancel, nil)
				return
			}
			initializationComplete = true
			go func() {
				defer obligation.complete()
				incoming, _ := metadata.FromIncomingContext(serverCtx)
				incoming = cloneMetadata(incoming)
				serverStatsCtx, statsErr := c.serverStats.tagRPCSafe(
					rpcStatsContext{
						values:  serverCtx,
						control: ctx,
					},
					method,
				)
				var serverStats *rpcStats
				if statsErr == nil {
					serverStats = c.serverStats.newRPCStats(
						serverStatsCtx,
						method,
					)
					_ = serverStats.inHeader(incoming, method)
					statsErr = serverStats.begin(
						target.clientStreams,
						target.serverStreams,
					)
				}
				if statsErr != nil {
					if serverStats != nil {
						serverStats.runner.abort()
					}
					life.finishServerSetup(serverCancel, nil)
					initCh <- initializationError(statsErr)
					return
				}
				// A successful Begin owns an End obligation even when the
				// owner continuation is rejected or accepted but lost.
				// Transfer that lifecycle ownership now; adapter publication
				// and handler startup remain owner-confined below.
				handlerCtx, handlerCancel := context.WithCancel(serverStatsCtx)
				life.finishServerSetup(handlerCancel, serverStats)
				serverCancel()
				submission, admitted := life.scheduleOwnerSubmission(
					"RPC initialization continuation",
					func(rpcOwnerCapability) {
						completeInitialization(
							handlerCtx,
							handlerCancel,
							serverStats,
						)
					},
				)
				if !admitted {
					failure := life.schedulerError()
					initCh <- initializationError(failure)
					return
				}
				if failure := <-submission; failure != nil {
					initCh <- initializationError(failure)
				}
			}()
			return
		}
		initializationComplete = true
		completeInitialization(serverCtx, serverCancel, nil)
	}); err != nil {
		clientStats.end(unavailableError())
		clientCancel()
		return nil, unavailableError()
	}

	completeFailure := func(result rpcInitResult) error {
		if result.completion != nil {
			<-result.completion
		}
		clientCancel()
		return result.err
	}
	select {
	case result := <-initCh:
		if result.err != nil {
			return nil, completeFailure(result)
		}
		return result.stream, nil
	case <-ctx.Done():
		select {
		case result := <-initCh:
			if result.err != nil {
				return nil, completeFailure(result)
			}
			return result.stream, nil
		default:
		}
		clientStats.end(ctx.Err())
		clientCancel()
		return nil, normalizeRPCError(ctx.Err())
	case <-c.loop.Done():
		select {
		case result := <-initCh:
			if result.err != nil {
				return nil, completeFailure(result)
			} else {
				return result.stream, nil
			}
		default:
		}
		clientStats.end(unavailableError())
		clientCancel()
		return nil, unavailableError()
	}
}

func (c *Channel) newServerAdapter(
	ctx context.Context,
	life *rpcLifecycle,
	state *stream.RPCState,
	target rpcTarget,
) *serverStreamAdapter {
	return &serverStreamAdapter{
		ctx:           ctx,
		loop:          c.loop,
		life:          life,
		state:         state,
		cloner:        c.cloner,
		cloneDisabled: c.cloneDisabled,
		clientStreams: target.clientStreams,
		serverStreams: target.serverStreams,
	}
}

func (c *Channel) admitCallback(
	life *rpcLifecycle,
	state *stream.RPCState,
	target rpcTarget,
) (*RPCStream, *CallbackTurn) {
	rpcStream := &RPCStream{
		state:         state,
		life:          life,
		cloner:        c.cloner,
		cloneDisabled: c.cloneDisabled,
		clientStreams: target.clientStreams,
		serverStreams: target.serverStreams,
	}
	turn, admitted := rpcStream.AdmitCallback()
	if !admitted {
		return rpcStream, nil
	}
	return rpcStream, turn
}

func (c *Channel) startCallback(
	ctx context.Context,
	rpcStream *RPCStream,
	turn *CallbackTurn,
	target rpcTarget,
) {
	turn.Run(func() {
		target.callback(ctx, rpcStream)
	})
}

func (c *Channel) startUnary(
	server *serverStreamAdapter,
	transportStream *transport.UnaryServerTransportStream,
	life *rpcLifecycle,
	target rpcTarget,
) {
	go func() {
		var (
			handlerErr error
			response   any
			returned   bool
		)
		defer func() {
			panicValue := recover()
			switch {
			case panicValue != nil:
				handlerErr = internalRPCError("unary handler", panicValue)
			case !returned:
				handlerErr = internalRPCError("unary handler", nil)
			}
			transportStream.Finish()
			headers := cloneMetadata(transportStream.GetHeaders())
			trailers := cloneMetadata(transportStream.GetTrailers())
			var cloned any
			if requestErr := server.validateRequestCardinality(); requestErr != nil &&
				(handlerErr == nil || errors.Is(handlerErr, io.EOF)) {
				// Generated unary decoders report io.EOF when the client closes
				// without its required request. Expose the transport cardinality
				// contract instead of leaking EOF as status Unknown.
				handlerErr = requestErr
			}
			if handlerErr == nil {
				if isNil(response) {
					handlerErr = cardinalityError(
						"handler returned neither error nor response message",
					)
				} else if c.cloneDisabled {
					cloned = response
				} else {
					result := cloneMessageSafe(
						"clone response",
						c.cloner,
						response,
					)
					cloned = result.value
					if result.err != nil {
						handlerErr = result.err
					}
				}
			}
			life.serverFinishPrepared(handlerErr, &terminalPreparation{
				headers:      headers,
				trailers:     trailers,
				response:     cloned,
				statsPayload: response,
				sendResponse: handlerErr == nil,
			})
		}()

		response, handlerErr = target.unary.Handler(
			target.service,
			server.ctx,
			server.RecvMsg,
			c.unaryInt,
		)
		returned = true
	}()
}

func (c *Channel) startStream(
	server *serverStreamAdapter,
	life *rpcLifecycle,
	method string,
	target rpcTarget,
) {
	go func() {
		returned := false
		var handlerErr error
		defer recoverHandler(
			handlerSubject(method),
			&returned,
			&handlerErr,
			func(err error) { life.serverFinish(err) },
		)
		if c.streamInt != nil {
			info := &grpc.StreamServerInfo{
				FullMethod:     method,
				IsClientStream: target.clientStreams,
				IsServerStream: target.serverStreams,
			}
			handlerErr = c.streamInt(
				target.service,
				server,
				info,
				target.stream.Handler,
			)
		} else {
			handlerErr = target.stream.Handler(target.service, server)
		}
		if handlerErr == nil {
			handlerErr = server.validateCardinality()
		}
		returned = true
	}()
}
