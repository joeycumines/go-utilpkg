package gojagrpc

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/dop251/goja"
	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcmetadata "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// jsServer represents a builder for registering JavaScript gRPC
// server handlers on an [inprocgrpc.Channel].
type jsServer struct {
	m            *Module
	obj          *goja.Object
	services     []serviceRegistration
	interceptors []goja.Callable
	started      bool
}

// serviceRegistration tracks a service and its JS handler functions
// for deferred registration.
type serviceRegistration struct {
	sd       protoreflect.ServiceDescriptor
	handlers map[string]goja.Value // lowerCamelCase method name → handler
}

// jsCreateServer implements the JS-facing grpc.createServer() function.
// It returns a server builder object with addService() and start()
// methods.
func (m *Module) jsCreateServer(call goja.FunctionCall) goja.Value {
	srv := &jsServer{m: m}

	obj := m.runtime.NewObject()
	_ = obj.Set("addService", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return srv.addService(call)
	}))
	_ = obj.Set("addInterceptor", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return srv.addInterceptor(call)
	}))
	_ = obj.Set("start", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return srv.start(call)
	}))
	srv.obj = obj
	return obj
}

// addInterceptor registers a server interceptor factory function.
// Interceptors follow the connect-es onion pattern:
//
//	function(next) { return function(call) { return next(call) } }
//
// The call object passed to interceptors has:
//   - method (string): the full gRPC method name
//   - requestHeader (metadata): incoming client headers
//   - request (for unary/server-stream): the request message
//   - setHeader, sendHeader, setTrailer: response metadata methods
//   - send, recv (for applicable streaming types)
//
// Returns the server builder for chaining.
func (srv *jsServer) addInterceptor(call goja.FunctionCall) goja.Value {
	fn, ok := goja.AssertFunction(call.Argument(0))
	if !ok {
		panic(srv.m.runtime.NewTypeError("addInterceptor: argument must be a function"))
	}
	srv.interceptors = append(srv.interceptors, fn)
	return srv.obj
}

// addService registers JS handler functions for a protobuf service.
// Returns the server object for method chaining.
//
// JS: server.addService('my.package.ServiceName', { methodName: handler })
func (srv *jsServer) addService(call goja.FunctionCall) goja.Value {
	serviceName := call.Argument(0).String()
	handlersVal := call.Argument(1)

	sd, err := srv.m.resolveService(serviceName)
	if err != nil {
		panic(srv.m.runtime.NewTypeError(err.Error()))
	}

	handlersObj, ok := handlersVal.(*goja.Object)
	if !ok {
		panic(srv.m.runtime.NewTypeError("addService: handlers must be an object"))
	}

	handlers := make(map[string]goja.Value)
	methods := sd.Methods()
	for i := 0; i < methods.Len(); i++ {
		md := methods.Get(i)
		jsName := lowerFirst(string(md.Name()))
		handlerVal := handlersObj.Get(jsName)
		if handlerVal == nil || goja.IsUndefined(handlerVal) {
			panic(srv.m.runtime.NewTypeError("addService: missing handler for %q", jsName))
		}
		if _, fnOk := goja.AssertFunction(handlerVal); !fnOk {
			panic(srv.m.runtime.NewTypeError("addService: handler for %q is not a function", jsName))
		}
		handlers[jsName] = handlerVal
	}

	srv.services = append(srv.services, serviceRegistration{
		sd:       sd,
		handlers: handlers,
	})

	return srv.obj
}

// start registers all accumulated service handlers on the channel
// via [inprocgrpc.Channel.RegisterStreamHandler].
//
// JS: server.start()
func (srv *jsServer) start(call goja.FunctionCall) goja.Value {
	if srv.started {
		panic(srv.m.runtime.NewTypeError("server already started"))
	}
	srv.started = true

	for _, reg := range srv.services {
		methods := reg.sd.Methods()

		// Build a grpc.ServiceDesc for metadata exposure via
		// GetServiceInfo(). This enables gRPC reflection to discover
		// JS-registered services. The handlers listed here are never
		// called — stream handlers take priority in dispatch.
		grpcDesc := &grpc.ServiceDesc{
			ServiceName: string(reg.sd.FullName()),
		}

		for i := 0; i < methods.Len(); i++ {
			md := methods.Get(i)
			jsName := lowerFirst(string(md.Name()))
			fullMethod := fmt.Sprintf("/%s/%s", reg.sd.FullName(), md.Name())
			handlerVal := reg.handlers[jsName]

			var handlerFn inprocgrpc.StreamHandlerFunc
			switch {
			case !md.IsStreamingClient() && !md.IsStreamingServer():
				handlerFn = srv.makeUnaryHandler(fullMethod, md, handlerVal)
				grpcDesc.Methods = append(grpcDesc.Methods, grpc.MethodDesc{
					MethodName: string(md.Name()),
				})
			case !md.IsStreamingClient() && md.IsStreamingServer():
				handlerFn = srv.makeServerStreamHandler(fullMethod, md, handlerVal)
				grpcDesc.Streams = append(grpcDesc.Streams, grpc.StreamDesc{
					StreamName:    string(md.Name()),
					ServerStreams: true,
				})
			case md.IsStreamingClient() && !md.IsStreamingServer():
				handlerFn = srv.makeClientStreamHandler(fullMethod, md, handlerVal)
				grpcDesc.Streams = append(grpcDesc.Streams, grpc.StreamDesc{
					StreamName:    string(md.Name()),
					ClientStreams: true,
				})
			default:
				handlerFn = srv.makeBidiStreamHandler(fullMethod, md, handlerVal)
				grpcDesc.Streams = append(grpcDesc.Streams, grpc.StreamDesc{
					StreamName:    string(md.Name()),
					ServerStreams: true,
					ClientStreams: true,
				})
			}

			srv.m.channel.RegisterStreamHandler(fullMethod, handlerFn)
		}

		// Register service metadata (HandlerType nil skips type
		// check; stream handlers take priority in dispatch).
		srv.m.channel.RegisterService(grpcDesc, struct{}{})
	}

	return goja.Undefined()
}

// ========================= Unary Handler =========================

// makeUnaryHandler creates a [inprocgrpc.StreamHandlerFunc] for a
// unary RPC method. The JS handler receives (request, call) and
// must return a response message or a Promise resolving to one.
// If server interceptors are registered, they wrap the handler call
// in a connect-es onion chain.
func (srv *jsServer) makeUnaryHandler(
	fullMethod string,
	md protoreflect.MethodDescriptor,
	handlerVal goja.Value,
) inprocgrpc.StreamHandlerFunc {
	inputDesc := md.Input()
	outputDesc := md.Output()
	handlerFn, _ := goja.AssertFunction(handlerVal)

	return func(ctx context.Context, stream *inprocgrpc.RPCStream) {
		stream.Recv().Recv(func(msg any, err error) {
			if err != nil {
				stream.Finish(err)
				return
			}

			reqObj, convErr := srv.m.toWrappedMessage(msg, inputDesc)
			if convErr != nil {
				stream.Finish(status.Errorf(codes.Internal, "request conversion: %v", convErr))
				return
			}

			callObj := srv.m.newServerCallObject(ctx, stream)
			_ = callObj.Set("method", fullMethod)
			_ = callObj.Set("request", reqObj)

			result, jsErr := srv.buildServerChain(callObj, func() (goja.Value, error) {
				req := callObj.Get("request")
				return handlerFn(goja.Undefined(), req, callObj)
			})
			if jsErr != nil {
				stream.Finish(srv.m.jsErrorToGRPC(jsErr))
				return
			}

			if srv.m.isThenable(result) {
				srv.m.thenFinishUnary(result, stream, outputDesc)
			} else {
				srv.m.finishUnaryResponse(result, stream, outputDesc)
			}
		})
	}
}

// =================== Server-Streaming Handler ===================

// makeServerStreamHandler creates a [inprocgrpc.StreamHandlerFunc]
// for a server-streaming RPC. The JS handler receives (request, call)
// where call has a send(msg) method. The handler can be async.
func (srv *jsServer) makeServerStreamHandler(
	fullMethod string,
	md protoreflect.MethodDescriptor,
	handlerVal goja.Value,
) inprocgrpc.StreamHandlerFunc {
	inputDesc := md.Input()
	handlerFn, _ := goja.AssertFunction(handlerVal)

	return func(ctx context.Context, stream *inprocgrpc.RPCStream) {
		stream.Recv().Recv(func(msg any, err error) {
			if err != nil {
				stream.Finish(err)
				return
			}

			reqObj, convErr := srv.m.toWrappedMessage(msg, inputDesc)
			if convErr != nil {
				stream.Finish(status.Errorf(codes.Internal, "request conversion: %v", convErr))
				return
			}

			callObj := srv.m.newServerCallObject(ctx, stream)
			_ = callObj.Set("method", fullMethod)
			_ = callObj.Set("request", reqObj)
			srv.m.addServerSend(callObj, stream)

			result, jsErr := srv.buildServerChain(callObj, func() (goja.Value, error) {
				req := callObj.Get("request")
				return handlerFn(goja.Undefined(), req, callObj)
			})
			if jsErr != nil {
				stream.Finish(srv.m.jsErrorToGRPC(jsErr))
				return
			}

			if srv.m.isThenable(result) {
				srv.m.thenFinish(result, stream)
			} else {
				stream.Finish(nil)
			}
		})
	}
}

// =================== Client-Streaming Handler ===================

// makeClientStreamHandler creates a [inprocgrpc.StreamHandlerFunc]
// for a client-streaming RPC. The JS handler receives (call) where
// call has a recv() method returning Promise<{value, done}>.
// The handler must return the response (or a Promise<response>).
func (srv *jsServer) makeClientStreamHandler(
	fullMethod string,
	md protoreflect.MethodDescriptor,
	handlerVal goja.Value,
) inprocgrpc.StreamHandlerFunc {
	inputDesc := md.Input()
	outputDesc := md.Output()
	handlerFn, _ := goja.AssertFunction(handlerVal)

	return func(ctx context.Context, stream *inprocgrpc.RPCStream) {
		callObj := srv.m.newServerCallObject(ctx, stream)
		_ = callObj.Set("method", fullMethod)
		srv.m.addServerRecv(callObj, stream, inputDesc)

		result, jsErr := srv.buildServerChain(callObj, func() (goja.Value, error) {
			return handlerFn(goja.Undefined(), callObj)
		})
		if jsErr != nil {
			stream.Finish(srv.m.jsErrorToGRPC(jsErr))
			return
		}

		if srv.m.isThenable(result) {
			srv.m.thenFinishUnary(result, stream, outputDesc)
		} else {
			srv.m.finishUnaryResponse(result, stream, outputDesc)
		}
	}
}

// =================== Bidi-Streaming Handler ====================

// makeBidiStreamHandler creates a [inprocgrpc.StreamHandlerFunc]
// for a bidirectional-streaming RPC. The JS handler receives (call)
// which has send(msg) and recv() methods.
func (srv *jsServer) makeBidiStreamHandler(
	fullMethod string,
	md protoreflect.MethodDescriptor,
	handlerVal goja.Value,
) inprocgrpc.StreamHandlerFunc {
	inputDesc := md.Input()
	handlerFn, _ := goja.AssertFunction(handlerVal)

	return func(ctx context.Context, stream *inprocgrpc.RPCStream) {
		callObj := srv.m.newServerCallObject(ctx, stream)
		_ = callObj.Set("method", fullMethod)
		srv.m.addServerSend(callObj, stream)
		srv.m.addServerRecv(callObj, stream, inputDesc)

		result, jsErr := srv.buildServerChain(callObj, func() (goja.Value, error) {
			return handlerFn(goja.Undefined(), callObj)
		})
		if jsErr != nil {
			stream.Finish(srv.m.jsErrorToGRPC(jsErr))
			return
		}

		if srv.m.isThenable(result) {
			srv.m.thenFinish(result, stream)
		} else {
			stream.Finish(nil)
		}
	}
}

// buildServerChain invokes the handler through the server interceptor
// chain. If no interceptors are registered, it calls handlerInvocation
// directly. Otherwise, it builds a connect-es style onion chain where
// each interceptor wraps the next.
//
// The callObj must already have "method" and (for unary/server-stream)
// "request" set before calling this method.
//
// Must be called on the event loop goroutine.
func (srv *jsServer) buildServerChain(
	callObj *goja.Object,
	handlerInvocation func() (goja.Value, error),
) (goja.Value, error) {
	if len(srv.interceptors) == 0 {
		return handlerInvocation()
	}

	// Build innermost next: calls the actual JS handler.
	innerNext := srv.m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		result, jsErr := handlerInvocation()
		if jsErr != nil {
			panic(jsErr)
		}
		return result
	})

	// Build chain right-to-left.
	var nextFn goja.Value = innerNext
	for i := len(srv.interceptors) - 1; i >= 0; i-- {
		wrapped, err := srv.interceptors[i](goja.Undefined(), nextFn)
		if err != nil {
			return nil, err
		}
		nextFn = wrapped
	}

	outerFn, ok := goja.AssertFunction(nextFn)
	if !ok {
		return nil, fmt.Errorf("server interceptor chain did not produce a callable")
	}

	return outerFn(goja.Undefined(), callObj)
}

// ====================== Server Helpers =========================

// newServerCallObject creates the base call object passed to JS
// handlers. It exposes:
//   - requestHeader: read-only metadata wrapper with incoming client headers
//   - setHeader(metadata): buffer response headers (like grpc.SetHeader)
//   - sendHeader(): flush buffered headers immediately (like grpc.SendHeader)
//   - setTrailer(metadata): set response trailers (like grpc.SetTrailer)
//
// It can be extended with send/recv methods by the caller.
func (m *Module) newServerCallObject(ctx context.Context, stream *inprocgrpc.RPCStream) *goja.Object {
	callObj := m.runtime.NewObject()

	// requestHeader — read-only metadata wrapper with incoming headers.
	inMD, _ := grpcmetadata.FromIncomingContext(ctx)
	if inMD == nil {
		inMD = grpcmetadata.MD{}
	}
	_ = callObj.Set("requestHeader", m.newMetadataWrapper(inMD))

	// setHeader(metadata) — buffer response headers.
	_ = callObj.Set("setHeader", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		md := m.metadataToGo(call.Argument(0))
		if md != nil {
			if err := stream.SetHeader(md); err != nil {
				panic(m.runtime.NewGoError(err))
			}
		}
		return goja.Undefined()
	}))

	// sendHeader() — flush buffered headers immediately.
	_ = callObj.Set("sendHeader", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		stream.SendHeader()
		return goja.Undefined()
	}))

	// setTrailer(metadata) — set response trailers.
	_ = callObj.Set("setTrailer", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		md := m.metadataToGo(call.Argument(0))
		if md != nil {
			stream.SetTrailer(md)
		}
		return goja.Undefined()
	}))

	return callObj
}

// addServerSend adds a send(msg) method to a server call object.
// send() is synchronous because RPCStream.Send is non-blocking
// on the event loop.
func (m *Module) addServerSend(callObj *goja.Object, stream *inprocgrpc.RPCStream) {
	_ = callObj.Set("send", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		msg, err := m.protobuf.UnwrapMessage(call.Argument(0))
		if err != nil {
			panic(m.runtime.NewTypeError("send: %s", err))
		}
		if sendErr := stream.Send().Send(msg); sendErr != nil {
			panic(m.runtime.NewGoError(sendErr))
		}
		return goja.Undefined()
	}))
}

// addServerRecv adds a recv() method to a server call object.
// recv() returns a Promise that resolves with {value, done} when
// the next client message arrives (or EOF).
func (m *Module) addServerRecv(callObj *goja.Object, stream *inprocgrpc.RPCStream, inputDesc protoreflect.MessageDescriptor) {
	_ = callObj.Set("recv", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		p, res, rej := m.adapter.JS().NewChainedPromise()
		stream.Recv().Recv(func(msg any, err error) {
			// This callback fires on the event loop.
			if err != nil {
				if err == io.EOF {
					result := m.runtime.NewObject()
					_ = result.Set("done", true)
					_ = result.Set("value", goja.Undefined())
					res(result)
				} else {
					rej(m.grpcErrorFromGoError(err))
				}
				return
			}
			reqObj, convErr := m.toWrappedMessage(msg, inputDesc)
			if convErr != nil {
				rej(m.grpcErrorFromGoError(status.Errorf(codes.Internal, "recv conversion: %v", convErr)))
				return
			}
			result := m.runtime.NewObject()
			_ = result.Set("done", false)
			_ = result.Set("value", reqObj)
			res(result)
		})
		return m.adapter.GojaWrapPromise(p)
	}))
}

// toWrappedMessage converts any proto.Message to a JS-wrapped
// *dynamicpb.Message. Fast path if already a *dynamicpb.Message,
// otherwise serializes and deserializes.
func (m *Module) toWrappedMessage(msg any, desc protoreflect.MessageDescriptor) (*goja.Object, error) {
	protoMsg, ok := msg.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("received value is not a proto.Message")
	}

	// Fast path: already a dynamicpb.Message.
	if dyn, dynOk := protoMsg.(*dynamicpb.Message); dynOk {
		return m.protobuf.WrapMessage(dyn), nil
	}

	// Slow path: serialize + deserialize to dynamicpb.
	data, err := proto.Marshal(protoMsg)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	dyn := dynamicpb.NewMessage(desc)
	if err := proto.Unmarshal(data, dyn); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return m.protobuf.WrapMessage(dyn), nil
}

// ==================== Promise Resolution =====================

// isThenable checks if a goja.Value has a callable "then" method.
func (m *Module) isThenable(val goja.Value) bool {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		return false
	}
	obj, ok := val.(*goja.Object)
	if !ok {
		return false
	}
	thenVal := obj.Get("then")
	if thenVal == nil || goja.IsUndefined(thenVal) {
		return false
	}
	_, ok = goja.AssertFunction(thenVal)
	return ok
}

// thenFinishUnary chains .then/.catch on a Promise to send a unary
// response and finish the stream.
func (m *Module) thenFinishUnary(promiseVal goja.Value, stream *inprocgrpc.RPCStream, outputDesc protoreflect.MessageDescriptor) {
	obj := promiseVal.(*goja.Object)
	thenVal := obj.Get("then")
	thenFn, _ := goja.AssertFunction(thenVal)

	onFulfilled := m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		m.finishUnaryResponse(call.Argument(0), stream, outputDesc)
		return goja.Undefined()
	})
	onRejected := m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		stream.Finish(m.jsValueToGRPCError(call.Argument(0)))
		return goja.Undefined()
	})

	_, _ = thenFn(promiseVal, onFulfilled, onRejected)
}

// thenFinish chains .then/.catch on a Promise to finish the stream
// without sending a response (used for streaming handlers).
func (m *Module) thenFinish(promiseVal goja.Value, stream *inprocgrpc.RPCStream) {
	obj := promiseVal.(*goja.Object)
	thenVal := obj.Get("then")
	thenFn, _ := goja.AssertFunction(thenVal)

	onFulfilled := m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		stream.Finish(nil)
		return goja.Undefined()
	})
	onRejected := m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		stream.Finish(m.jsValueToGRPCError(call.Argument(0)))
		return goja.Undefined()
	})

	_, _ = thenFn(promiseVal, onFulfilled, onRejected)
}

// finishUnaryResponse wraps the JS return value as a protobuf message,
// sends it, and finishes the stream.
func (m *Module) finishUnaryResponse(val goja.Value, stream *inprocgrpc.RPCStream, outputDesc protoreflect.MessageDescriptor) {
	if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
		stream.Finish(status.Errorf(codes.Internal, "handler returned nil/undefined"))
		return
	}

	msg, err := m.protobuf.UnwrapMessage(val)
	if err != nil {
		stream.Finish(status.Errorf(codes.Internal, "handler response: %v", err))
		return
	}

	_ = outputDesc // reserved for future type validation

	if sendErr := stream.Send().Send(msg); sendErr != nil {
		stream.Finish(status.Errorf(codes.Internal, "send response: %v", sendErr))
		return
	}
	stream.Finish(nil)
}

// ===================== Error Conversion ========================

// jsErrorToGRPC converts a goja exception (from a JS throw/reject)
// to a gRPC status error. If the thrown value is a GrpcError, its
// code and message are preserved.
func (m *Module) jsErrorToGRPC(err error) error {
	var jsExcept *goja.Exception
	if errors.As(err, &jsExcept) {
		return m.jsValueToGRPCError(jsExcept.Value())
	}
	return status.Errorf(codes.Internal, "%v", err)
}

// jsValueToGRPCError converts a JS rejection value to a gRPC error.
// If the value is a GrpcError object, extracts code, message, and
// optional details. Otherwise maps to codes.Internal.
func (m *Module) jsValueToGRPCError(val goja.Value) error {
	if val == nil || goja.IsUndefined(val) {
		return status.Errorf(codes.Internal, "unknown error")
	}

	obj, ok := val.(*goja.Object)
	if !ok {
		return status.Errorf(codes.Internal, "%s", val.String())
	}

	// Check for GrpcError
	nameVal := obj.Get("name")
	if nameVal != nil && nameVal.String() == "GrpcError" {
		codeVal := obj.Get("code")
		msgVal := obj.Get("message")
		code := codes.Code(codeVal.ToInteger())
		msg := msgVal.String()

		// Check for details.
		anyDetails := m.extractGoDetails(obj)
		if len(anyDetails) > 0 {
			st := status.New(code, msg)
			stProto := st.Proto()
			stProto.Details = anyDetails
			st = status.FromProto(stProto)
			return st.Err()
		}

		return status.Errorf(code, "%s", msg)
	}

	// Generic JS error
	msgVal := obj.Get("message")
	if msgVal != nil && !goja.IsUndefined(msgVal) {
		return status.Errorf(codes.Internal, "%s", msgVal.String())
	}
	return status.Errorf(codes.Internal, "%s", val.String())
}
