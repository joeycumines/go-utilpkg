package gojagrpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/dop251/goja"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcmetadata "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// jsCreateClient implements the JS-facing grpc.createClient(serviceName, opts)
// function. It resolves the service descriptor and builds a client
// object with a method for each RPC in the service.
//
// The optional second argument may contain:
//   - interceptors: An array of interceptor factory functions (connect-es pattern).
//     Each interceptor receives a next function and returns a wrapper function:
//     function(next) { return function(req) { return next(req); }; }
//     req has: method (string), message (protobuf), header (metadata wrapper).
//     Interceptors are applied to unary RPCs only.
func (m *Module) jsCreateClient(call goja.FunctionCall) goja.Value {
	serviceName := call.Argument(0).String()

	sd, err := m.resolveService(serviceName)
	if err != nil {
		panic(m.runtime.NewTypeError(err.Error()))
	}

	// Parse optional client options.
	var interceptors []goja.Callable
	var cc grpc.ClientConnInterface = m.channel // default: in-process channel
	if optsArg := call.Argument(1); optsArg != nil && !goja.IsUndefined(optsArg) && !goja.IsNull(optsArg) {
		if optsObj, ok := optsArg.(*goja.Object); ok {
			interceptors = m.parseInterceptors(optsObj)
			cc = m.parseChannelOpt(optsObj)
		}
	}

	client := m.runtime.NewObject()
	methods := sd.Methods()
	for i := 0; i < methods.Len(); i++ {
		md := methods.Get(i)
		jsName := lowerFirst(string(md.Name()))
		fullMethod := fmt.Sprintf("/%s/%s", sd.FullName(), md.Name())
		inputDesc := md.Input()
		outputDesc := md.Output()

		var fn goja.Value
		switch {
		case !md.IsStreamingClient() && !md.IsStreamingServer():
			fn = m.makeUnaryMethod(cc, fullMethod, inputDesc, outputDesc, interceptors)
		case !md.IsStreamingClient() && md.IsStreamingServer():
			fn = m.makeServerStreamMethod(cc, fullMethod, inputDesc, outputDesc)
		case md.IsStreamingClient() && !md.IsStreamingServer():
			fn = m.makeClientStreamMethod(cc, fullMethod, outputDesc)
		default:
			fn = m.makeBidiStreamMethod(cc, fullMethod, outputDesc)
		}

		_ = client.Set(jsName, fn)
	}

	return client
}

// parseInterceptors extracts an array of interceptor factory functions
// from the client options object.
func (m *Module) parseInterceptors(optsObj *goja.Object) []goja.Callable {
	val := optsObj.Get("interceptors")
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return nil
	}
	arrObj, ok := val.(*goja.Object)
	if !ok {
		panic(m.runtime.NewTypeError("interceptors must be an array"))
	}
	lenVal := arrObj.Get("length")
	if lenVal == nil || goja.IsUndefined(lenVal) {
		return nil
	}
	length := int(lenVal.ToInteger())
	if length == 0 {
		return nil
	}
	interceptors := make([]goja.Callable, 0, length)
	for i := 0; i < length; i++ {
		elemVal := arrObj.Get(fmt.Sprintf("%d", i))
		fn, fnOk := goja.AssertFunction(elemVal)
		if !fnOk {
			panic(m.runtime.NewTypeError("interceptor at index %d is not a function", i))
		}
		interceptors = append(interceptors, fn)
	}
	return interceptors
}

// ========================== Unary RPC ==========================

// makeUnaryMethod creates a JS function for a unary RPC call.
//
// JS: const response = await client.method(request, { onHeader, onTrailer })
//
// If interceptors are registered, each call builds an interceptor chain
// (connect-es pattern): the outermost interceptor wraps the next, down
// to the actual RPC. The request bundle {method, message, header} flows
// through the chain, allowing interceptors to modify metadata or the
// request message.
//
// Without interceptors, the call spawns a goroutine that invokes
// channel.Invoke (blocking), then resolves/rejects the returned promise.
func (m *Module) makeUnaryMethod(
	cc grpc.ClientConnInterface,
	fullMethod string,
	inputDesc, outputDesc protoreflect.MessageDescriptor,
	interceptors []goja.Callable,
) goja.Value {
	return m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		reqMsg, err := m.protobuf.UnwrapMessage(call.Argument(0))
		if err != nil {
			panic(m.runtime.NewTypeError("unary %s: %s", fullMethod, err))
		}

		copts := m.parseCallOpts(call, 1)

		if len(interceptors) == 0 {
			// Fast path: no interceptors, direct RPC.
			return m.executeUnaryRPC(cc, fullMethod, reqMsg, outputDesc, copts)
		}

		// --- Interceptor chain path ---

		// Build request bundle.
		reqBundle := m.runtime.NewObject()
		_ = reqBundle.Set("method", fullMethod)
		_ = reqBundle.Set("message", call.Argument(0))
		md, _ := grpcmetadata.FromOutgoingContext(copts.ctx)
		if md == nil {
			md = grpcmetadata.MD{}
		}
		_ = reqBundle.Set("header", m.newMetadataWrapper(md))

		// Build innermost function: the actual RPC.
		innerRPC := m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			bundle := call.Argument(0)
			if bundle == nil || goja.IsUndefined(bundle) {
				panic(m.runtime.NewTypeError("interceptor must call next with request"))
			}
			bundleObj := bundle.(*goja.Object)

			msgVal := bundleObj.Get("message")
			innerMsg, err := m.protobuf.UnwrapMessage(msgVal)
			if err != nil {
				panic(m.runtime.NewTypeError("interceptor: invalid message: %s", err))
			}

			headerVal := bundleObj.Get("header")
			innerMD := m.metadataToGo(headerVal)
			innerCtx := copts.ctx
			if innerMD != nil {
				innerCtx = grpcmetadata.NewOutgoingContext(innerCtx, innerMD)
			}

			innerCopts := &callOpts{
				ctx:       innerCtx,
				cancel:    copts.cancel,
				onHeader:  copts.onHeader,
				onTrailer: copts.onTrailer,
			}
			return m.executeUnaryRPC(cc, fullMethod, innerMsg, outputDesc, innerCopts)
		})

		// Build chain: right-to-left application of interceptors.
		var nextFn goja.Value = innerRPC
		for i := len(interceptors) - 1; i >= 0; i-- {
			interceptor := interceptors[i]
			wrapped, jsErr := interceptor(goja.Undefined(), nextFn)
			if jsErr != nil {
				panic(jsErr)
			}
			nextFn = wrapped
		}

		// Call the outermost wrapper with the request bundle.
		outerFn, ok := goja.AssertFunction(nextFn)
		if !ok {
			panic(m.runtime.NewTypeError("interceptor chain did not produce a callable"))
		}
		result, jsErr := outerFn(goja.Undefined(), reqBundle)
		if jsErr != nil {
			panic(jsErr)
		}
		return result
	})
}

// executeUnaryRPC performs the actual unary RPC call. It spawns a
// goroutine for the blocking Invoke, captures headers/trailers, and
// resolves/rejects the promise on the event loop.
func (m *Module) executeUnaryRPC(
	cc grpc.ClientConnInterface,
	fullMethod string,
	reqMsg *dynamicpb.Message,
	outputDesc protoreflect.MessageDescriptor,
	copts *callOpts,
) goja.Value {
	respMsg := dynamicpb.NewMessage(outputDesc)
	promise, resolve, reject := m.adapter.JS().NewChainedPromise()

	onHeader := copts.onHeader
	onTrailer := copts.onTrailer

	go func() {
		defer copts.cancel()

		var headerMD, trailerMD grpcmetadata.MD
		var grpcOpts []grpc.CallOption
		if onHeader != nil {
			grpcOpts = append(grpcOpts, grpc.Header(&headerMD))
		}
		if onTrailer != nil {
			grpcOpts = append(grpcOpts, grpc.Trailer(&trailerMD))
		}

		invokeErr := cc.Invoke(copts.ctx, fullMethod, reqMsg, respMsg, grpcOpts...)
		if submitErr := m.adapter.Loop().Submit(func() {
			m.invokeMetadataCallback(onHeader, headerMD)

			if invokeErr != nil {
				m.invokeMetadataCallback(onTrailer, trailerMD)
				reject(m.grpcErrorFromGoError(invokeErr))
				return
			}
			m.invokeMetadataCallback(onTrailer, trailerMD)
			resolve(m.protobuf.WrapMessage(respMsg))
		}); submitErr != nil {
			reject(fmt.Errorf("event loop not running"))
		}
	}()

	return m.adapter.GojaWrapPromise(promise)
}

// =================== Server-Streaming RPC ====================

// makeServerStreamMethod creates a JS function for a server-streaming
// RPC call.
//
// JS:
//
//	const stream = await client.method(request, { onHeader, onTrailer });
//	while (true) {
//	    const { value, done } = await stream.recv();
//	    if (done) break;
//	}
//
// The returned promise resolves with a stream reader that has a
// recv() method. Each recv() call returns a promise resolving with
// {value, done}. If onHeader/onTrailer callbacks are set, onHeader is
// invoked asynchronously when response headers arrive, and onTrailer
// when the stream ends.
func (m *Module) makeServerStreamMethod(
	cc grpc.ClientConnInterface,
	fullMethod string,
	inputDesc, outputDesc protoreflect.MessageDescriptor,
) goja.Value {
	return m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		reqMsg, err := m.protobuf.UnwrapMessage(call.Argument(0))
		if err != nil {
			panic(m.runtime.NewTypeError("server-stream %s: %s", fullMethod, err))
		}

		copts := m.parseCallOpts(call, 1)
		onHeader := copts.onHeader
		onTrailer := copts.onTrailer
		desc := &grpc.StreamDesc{
			ServerStreams: true,
		}
		promise, resolve, reject := m.adapter.JS().NewChainedPromise()

		go func() {
			cs, streamErr := cc.NewStream(copts.ctx, desc, fullMethod)
			if streamErr != nil {
				copts.cancel()
				m.submitOrRejectDirect(reject, func() {
					reject(m.grpcErrorFromGoError(streamErr))
				})
				return
			}

			if sendErr := cs.SendMsg(reqMsg); sendErr != nil {
				copts.cancel()
				m.submitOrRejectDirect(reject, func() {
					reject(m.grpcErrorFromGoError(sendErr))
				})
				return
			}

			if closeErr := cs.CloseSend(); closeErr != nil {
				copts.cancel()
				m.submitOrRejectDirect(reject, func() {
					reject(m.grpcErrorFromGoError(closeErr))
				})
				return
			}

			// Fetch headers synchronously (blocks until available) before
			// delivering the reader. This guarantees the onHeader callback
			// fires before any recv() data, matching gRPC semantics.
			var headerMD grpcmetadata.MD
			if onHeader != nil {
				headerMD, _ = cs.Header()
			}

			// Stream established — deliver header callback and reader on the event loop.
			if submitErr := m.adapter.Loop().Submit(func() {
				if onHeader != nil && headerMD != nil {
					m.invokeMetadataCallback(onHeader, headerMD)
				}
				reader := m.newStreamReader(cs, outputDesc, copts.cancel, onTrailer)
				resolve(reader)
			}); submitErr != nil {
				copts.cancel()
				reject(fmt.Errorf("event loop not running"))
			}
		}()

		return m.adapter.GojaWrapPromise(promise)
	})
}

// newStreamReader creates a JS object with a recv() method that reads
// from a [grpc.ClientStream]. Each recv() call returns a promise
// resolving with {value: message, done: false} or
// {value: undefined, done: true} at end-of-stream.
//
// The ctxCancel function is called on EOF or error to release the
// context resources. If onTrailer is non-nil, it is invoked with
// a metadata wrapper when the stream terminates (before the final
// recv promise settles).
func (m *Module) newStreamReader(cs grpc.ClientStream, outputDesc protoreflect.MessageDescriptor, ctxCancel context.CancelFunc, onTrailer goja.Callable) *goja.Object {
	reader := m.runtime.NewObject()
	trailerOnce := new(sync.Once)

	_ = reader.Set("recv", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		p, res, rej := m.adapter.JS().NewChainedPromise()
		go func() {
			respMsg := dynamicpb.NewMessage(outputDesc)
			recvErr := cs.RecvMsg(respMsg)

			// Fetch trailers off-loop (only once, on stream end).
			var trailerMD grpcmetadata.MD
			var hasTrailer bool
			if recvErr != nil {
				trailerOnce.Do(func() {
					if onTrailer != nil {
						trailerMD = cs.Trailer()
						hasTrailer = true
					}
				})
			}

			if submitErr := m.adapter.Loop().Submit(func() {
				if recvErr != nil {
					ctxCancel()
					if hasTrailer {
						m.invokeMetadataCallback(onTrailer, trailerMD)
					}
					if recvErr == io.EOF {
						result := m.runtime.NewObject()
						_ = result.Set("done", true)
						_ = result.Set("value", goja.Undefined())
						res(result)
					} else {
						rej(m.grpcErrorFromGoError(recvErr))
					}
					return
				}
				result := m.runtime.NewObject()
				_ = result.Set("done", false)
				_ = result.Set("value", m.protobuf.WrapMessage(respMsg))
				res(result)
			}); submitErr != nil {
				ctxCancel()
				rej(fmt.Errorf("event loop not running"))
			}
		}()
		return m.adapter.GojaWrapPromise(p)
	}))

	return reader
}

// =================== Client-Streaming RPC ====================

// makeClientStreamMethod creates a JS function for a client-streaming
// RPC call.
//
// JS:
//
//	const call = await client.method({ onHeader, onTrailer });
//	await call.send(msg1);
//	await call.send(msg2);
//	await call.closeSend();
//	const response = await call.response;
//
// The returned promise resolves with a call object that has send(),
// closeSend(), and a response property (a Promise). If onHeader/onTrailer
// callbacks are set, onHeader is invoked when response headers arrive,
// and onTrailer when the response is received.
func (m *Module) makeClientStreamMethod(
	cc grpc.ClientConnInterface,
	fullMethod string,
	outputDesc protoreflect.MessageDescriptor,
) goja.Value {
	return m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		copts := m.parseCallOpts(call, 0)
		onHeader := copts.onHeader
		onTrailer := copts.onTrailer
		desc := &grpc.StreamDesc{
			ClientStreams: true,
		}

		promise, resolve, reject := m.adapter.JS().NewChainedPromise()

		go func() {
			cs, streamErr := cc.NewStream(copts.ctx, desc, fullMethod)
			if streamErr != nil {
				copts.cancel()
				m.submitOrRejectDirect(reject, func() {
					reject(m.grpcErrorFromGoError(streamErr))
				})
				return
			}

			// For client-streaming, the server handler has already run
			// during NewStream and may have called sendHeader(). However,
			// cs.Header() blocks via loop Submit, so we fetch headers
			// asynchronously to avoid holding up the call object delivery.
			if onHeader != nil {
				go func() {
					md, err := cs.Header()
					if err != nil || md == nil {
						return
					}
					m.adapter.Loop().Submit(func() {
						m.invokeMetadataCallback(onHeader, md)
					})
				}()
			}

			if submitErr := m.adapter.Loop().Submit(func() {
				callObj := m.newClientStreamCall(cs, outputDesc, copts.cancel, onTrailer)
				resolve(callObj)
			}); submitErr != nil {
				copts.cancel()
				reject(fmt.Errorf("event loop not running"))
			}
		}()

		return m.adapter.GojaWrapPromise(promise)
	})
}

// sendOp represents a queued send or closeSend operation for a
// client-streaming or bidirectional-streaming RPC.
type sendOp struct {
	msg     *dynamicpb.Message // nil for closeSend
	resolve func(any)
	reject  func(any)
}

// newClientStreamCall creates a JS call object for a client-streaming
// RPC. It starts a sender goroutine that serializes send/closeSend
// operations, and a receiver goroutine that waits for the server
// response. If onTrailer is non-nil, it is invoked when the response
// is received (before the response promise settles).
//
// Must be called on the event loop goroutine.
func (m *Module) newClientStreamCall(cs grpc.ClientStream, outputDesc protoreflect.MessageDescriptor, ctxCancel context.CancelFunc, onTrailer goja.Callable) *goja.Object {
	callObj := m.runtime.NewObject()

	// Response promise — resolved when the server sends its response.
	responsePromise, responseResolve, responseReject := m.adapter.JS().NewChainedPromise()

	// Channel for serializing send operations. Buffered to avoid
	// blocking the event loop on normal-sized bursts.
	sendCh := make(chan sendOp, 64)

	// Sender goroutine: processes sends in order.
	go func() {
		for op := range sendCh {
			if op.msg == nil {
				// closeSend
				closeErr := cs.CloseSend()
				opResolve := op.resolve
				opReject := op.reject
				if submitErr := m.adapter.Loop().Submit(func() {
					if closeErr != nil {
						opReject(m.grpcErrorFromGoError(closeErr))
						return
					}
					opResolve(goja.Undefined())
				}); submitErr != nil {
					op.reject(fmt.Errorf("event loop not running"))
				}
				return // No more sends after closeSend.
			}
			sendErr := cs.SendMsg(op.msg)
			opResolve := op.resolve
			opReject := op.reject
			if submitErr := m.adapter.Loop().Submit(func() {
				if sendErr != nil {
					opReject(m.grpcErrorFromGoError(sendErr))
					return
				}
				opResolve(goja.Undefined())
			}); submitErr != nil {
				op.reject(fmt.Errorf("event loop not running"))
			}
		}
	}()

	// Receiver goroutine: waits for the single server response.
	go func() {
		respMsg := dynamicpb.NewMessage(outputDesc)
		recvErr := cs.RecvMsg(respMsg)

		// Fetch trailers off-loop before submitting to event loop.
		var trailerMD grpcmetadata.MD
		if onTrailer != nil {
			trailerMD = cs.Trailer()
		}

		if submitErr := m.adapter.Loop().Submit(func() {
			ctxCancel()
			m.invokeMetadataCallback(onTrailer, trailerMD)
			if recvErr != nil {
				responseReject(m.grpcErrorFromGoError(recvErr))
				return
			}
			responseResolve(m.protobuf.WrapMessage(respMsg))
		}); submitErr != nil {
			ctxCancel()
			responseReject(fmt.Errorf("event loop not running"))
		}
	}()

	// send(msg) → Promise<void>
	_ = callObj.Set("send", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		msg, err := m.protobuf.UnwrapMessage(call.Argument(0))
		if err != nil {
			panic(m.runtime.NewTypeError("send: %s", err))
		}
		p, res, rej := m.adapter.JS().NewChainedPromise()
		sendCh <- sendOp{msg: msg, resolve: res, reject: rej}
		return m.adapter.GojaWrapPromise(p)
	}))

	// closeSend() → Promise<void>
	_ = callObj.Set("closeSend", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		p, res, rej := m.adapter.JS().NewChainedPromise()
		sendCh <- sendOp{msg: nil, resolve: res, reject: rej}
		return m.adapter.GojaWrapPromise(p)
	}))

	// response — Promise that resolves with the server's response.
	_ = callObj.Set("response", m.adapter.GojaWrapPromise(responsePromise))

	return callObj
}

// ================ Bidirectional-Streaming RPC ================

// makeBidiStreamMethod creates a JS function for a bidirectional-streaming
// RPC call.
//
// JS:
//
//	const stream = await client.method({ onHeader, onTrailer });
//	await stream.send(msg);
//	const { value, done } = await stream.recv();
//	await stream.closeSend();
//
// The returned promise resolves with a stream object that has send(),
// closeSend(), and recv() methods. If onHeader/onTrailer callbacks are
// set, onHeader is invoked when response headers arrive, and onTrailer
// when the stream ends.
func (m *Module) makeBidiStreamMethod(
	cc grpc.ClientConnInterface,
	fullMethod string,
	outputDesc protoreflect.MessageDescriptor,
) goja.Value {
	return m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		copts := m.parseCallOpts(call, 0)
		onHeader := copts.onHeader
		onTrailer := copts.onTrailer
		desc := &grpc.StreamDesc{
			ClientStreams: true,
			ServerStreams: true,
		}

		promise, resolve, reject := m.adapter.JS().NewChainedPromise()

		go func() {
			cs, streamErr := cc.NewStream(copts.ctx, desc, fullMethod)
			if streamErr != nil {
				copts.cancel()
				m.submitOrRejectDirect(reject, func() {
					reject(m.grpcErrorFromGoError(streamErr))
				})
				return
			}

			// For bidi-streaming, fetch headers synchronously before
			// delivering the stream object. This matches server-stream
			// behavior: headers are guaranteed available before recv().
			var headerMD grpcmetadata.MD
			if onHeader != nil {
				headerMD, _ = cs.Header()
			}

			if submitErr := m.adapter.Loop().Submit(func() {
				if onHeader != nil && headerMD != nil {
					m.invokeMetadataCallback(onHeader, headerMD)
				}
				streamObj := m.newBidiStream(cs, outputDesc, copts.cancel, onTrailer)
				resolve(streamObj)
			}); submitErr != nil {
				copts.cancel()
				reject(fmt.Errorf("event loop not running"))
			}
		}()

		return m.adapter.GojaWrapPromise(promise)
	})
}

// newBidiStream creates a JS stream object for a bidirectional-streaming
// RPC. It starts a sender goroutine that serializes send/closeSend
// operations, and exposes a recv() method for reading server messages.
// If onTrailer is non-nil, it is invoked with a metadata wrapper when
// the stream terminates (before the final recv promise settles).
//
// Must be called on the event loop goroutine.
func (m *Module) newBidiStream(cs grpc.ClientStream, outputDesc protoreflect.MessageDescriptor, ctxCancel context.CancelFunc, onTrailer goja.Callable) *goja.Object {
	streamObj := m.runtime.NewObject()
	trailerOnce := new(sync.Once)

	// Channel for serializing send operations.
	sendCh := make(chan sendOp, 64)

	// Sender goroutine: processes sends in order.
	go func() {
		for op := range sendCh {
			if op.msg == nil {
				closeErr := cs.CloseSend()
				opResolve := op.resolve
				opReject := op.reject
				if submitErr := m.adapter.Loop().Submit(func() {
					if closeErr != nil {
						opReject(m.grpcErrorFromGoError(closeErr))
						return
					}
					opResolve(goja.Undefined())
				}); submitErr != nil {
					op.reject(fmt.Errorf("event loop not running"))
				}
				return
			}
			sendErr := cs.SendMsg(op.msg)
			opResolve := op.resolve
			opReject := op.reject
			if submitErr := m.adapter.Loop().Submit(func() {
				if sendErr != nil {
					opReject(m.grpcErrorFromGoError(sendErr))
					return
				}
				opResolve(goja.Undefined())
			}); submitErr != nil {
				op.reject(fmt.Errorf("event loop not running"))
			}
		}
	}()

	// send(msg) → Promise<void>
	_ = streamObj.Set("send", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		msg, err := m.protobuf.UnwrapMessage(call.Argument(0))
		if err != nil {
			panic(m.runtime.NewTypeError("send: %s", err))
		}
		p, res, rej := m.adapter.JS().NewChainedPromise()
		sendCh <- sendOp{msg: msg, resolve: res, reject: rej}
		return m.adapter.GojaWrapPromise(p)
	}))

	// closeSend() → Promise<void>
	_ = streamObj.Set("closeSend", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		p, res, rej := m.adapter.JS().NewChainedPromise()
		sendCh <- sendOp{msg: nil, resolve: res, reject: rej}
		return m.adapter.GojaWrapPromise(p)
	}))

	// recv() → Promise<{value, done}>
	_ = streamObj.Set("recv", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		p, res, rej := m.adapter.JS().NewChainedPromise()
		go func() {
			respMsg := dynamicpb.NewMessage(outputDesc)
			recvErr := cs.RecvMsg(respMsg)

			// Fetch trailers off-loop (only once, on stream end).
			var trailerMD grpcmetadata.MD
			var hasTrailer bool
			if recvErr != nil {
				trailerOnce.Do(func() {
					if onTrailer != nil {
						trailerMD = cs.Trailer()
						hasTrailer = true
					}
				})
			}

			if submitErr := m.adapter.Loop().Submit(func() {
				if recvErr != nil {
					if hasTrailer {
						m.invokeMetadataCallback(onTrailer, trailerMD)
					}
					if recvErr == io.EOF {
						result := m.runtime.NewObject()
						_ = result.Set("done", true)
						_ = result.Set("value", goja.Undefined())
						res(result)
					} else {
						rej(m.grpcErrorFromGoError(recvErr))
					}
					return
				}
				result := m.runtime.NewObject()
				_ = result.Set("done", false)
				_ = result.Set("value", m.protobuf.WrapMessage(respMsg))
				res(result)
			}); submitErr != nil {
				rej(fmt.Errorf("event loop not running"))
			}
		}()
		return m.adapter.GojaWrapPromise(p)
	}))

	return streamObj
}

// ===================== Error Conversion ======================

// grpcErrorFromGoError converts a Go error to a JS GrpcError object.
// If the error contains a gRPC status, the code, message, and details
// are extracted. Context cancellation maps to CANCELLED, deadline
// exceeded maps to DEADLINE_EXCEEDED. Otherwise, codes.Internal is used.
//
// Must be called on the event loop goroutine (creates goja objects).
func (m *Module) grpcErrorFromGoError(err error) *goja.Object {
	s, ok := status.FromError(err)
	if ok {
		obj := m.newGrpcError(s.Code(), s.Message())
		// Extract details from the status proto.
		if sp := s.Proto(); sp != nil && len(sp.GetDetails()) > 0 {
			_ = obj.Set("details", m.wrapStatusDetails(sp.GetDetails()))
		}
		return obj
	}
	if errors.Is(err, context.Canceled) {
		return m.newGrpcError(codes.Canceled, err.Error())
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return m.newGrpcError(codes.DeadlineExceeded, err.Error())
	}
	return m.newGrpcError(codes.Internal, err.Error())
}

// =================== Internal Helpers =======================

// invokeMetadataCallback calls a JS callback function with a metadata
// wrapper argument. If fn is nil, this is a no-op. If md is nil, an
// empty metadata wrapper is used (the callback always receives an
// object, never undefined).
//
// Must be called on the event loop goroutine.
func (m *Module) invokeMetadataCallback(fn goja.Callable, md grpcmetadata.MD) {
	if fn == nil {
		return
	}
	if md == nil {
		md = grpcmetadata.MD{}
	}
	_, _ = fn(goja.Undefined(), m.newMetadataWrapper(md))
}

// submitOrRejectDirect attempts to submit fn to the event loop. If
// the loop is not running, it rejects the promise with a plain error
// directly from the goroutine (safe because reject is thread-safe).
func (m *Module) submitOrRejectDirect(reject func(any), fn func()) {
	if submitErr := m.adapter.Loop().Submit(fn); submitErr != nil {
		reject(fmt.Errorf("event loop not running"))
	}
}
