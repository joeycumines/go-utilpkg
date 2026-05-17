package gojagrpc

import (
	"context"
	"errors"
	"fmt"

	"github.com/joeycumines/goja"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcmetadata "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

var errMetadataCallbackExit = errors.New("gojagrpc: metadata callback exited without returning")

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
	m.mustOpen("createClient")
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
			fn = m.makeClientStreamMethod(cc, fullMethod, inputDesc, outputDesc)
		default:
			fn = m.makeBidiStreamMethod(cc, fullMethod, inputDesc, outputDesc)
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
	for i := range length {
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
		reqMsg, err := m.snapshotMessage(call.Argument(0), inputDesc)
		if err != nil {
			panic(m.runtime.NewTypeError("unary %s: %s", fullMethod, err))
		}

		if len(interceptors) == 0 {
			// Fast path: no interceptors, direct RPC.
			copts := m.parseCallOpts(call, 1)
			return m.executeUnaryRPC(cc, fullMethod, reqMsg, outputDesc, copts)
		}
		copts := m.parseCallOptsDeferred(call, 1)
		rpcOwnsOptions := false
		returned := false
		defer func() {
			reason := recover()
			if !returned || reason != nil {
				copts.finishOwner()
			}
			if reason != nil {
				panic(reason)
			}
		}()
		var optsObj *goja.Object
		var deferredOpts deferredCallOptions
		if optsVal := call.Argument(1); optsVal != nil && !goja.IsUndefined(optsVal) && !goja.IsNull(optsVal) {
			optsObj, _ = optsVal.(*goja.Object)
			deferredOpts = m.snapshotCallOptions(optsObj)
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
		nextCalled := false
		innerRPC := m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
			if nextCalled {
				panic(m.runtime.NewTypeError("interceptor next called multiple times"))
			}
			nextCalled = true

			bundle := call.Argument(0)
			if bundle == nil || goja.IsUndefined(bundle) {
				panic(m.runtime.NewTypeError("interceptor must call next with request"))
			}
			bundleObj, ok := bundle.(*goja.Object)
			if !ok {
				panic(m.runtime.NewTypeError("interceptor request must be an object"))
			}

			msgVal := bundleObj.Get("message")
			innerMsg, err := m.snapshotMessage(msgVal, inputDesc)
			if err != nil {
				panic(m.runtime.NewTypeError("interceptor: invalid message: %s", err))
			}

			headerVal := bundleObj.Get("header")
			innerMD := m.metadataToGo(headerVal)
			innerCtx := copts.ctx
			if innerMD != nil {
				innerCtx = grpcmetadata.NewOutgoingContext(innerCtx, innerMD)
			}

			copts.ctx = innerCtx
			m.applySnapshot(deferredOpts, copts)
			result := m.executeUnaryRPC(cc, fullMethod, innerMsg, outputDesc, copts)
			rpcOwnsOptions = true
			return result
		})

		// Build chain: right-to-left application of interceptors.
		nextFn := innerRPC
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
		if !nextCalled {
			object, objectOK := result.(*goja.Object)
			var then goja.Callable
			if objectOK {
				then, objectOK = goja.AssertFunction(object.Get("then"))
			}
			if !objectOK {
				copts.finishOwner()
			} else {
				cleanup := m.runtime.ToValue(func(goja.FunctionCall) goja.Value {
					if !rpcOwnsOptions {
						copts.finishOwner()
					}
					return goja.Undefined()
				})
				if _, err := then(result, cleanup, cleanup); err != nil {
					panic(err)
				}
			}
		}
		returned = true
		return result
	})
}

// executeUnaryRPC performs the actual unary RPC call. It spawns a
// goroutine for the blocking Invoke, captures headers/trailers, and
// resolves or rejects the Promise under the logical adapter owner.
func (m *Module) executeUnaryRPC(
	cc grpc.ClientConnInterface,
	fullMethod string,
	reqMsg proto.Message,
	outputDesc protoreflect.MessageDescriptor,
	copts *callOpts,
) goja.Value {
	respMsg := dynamicpb.NewMessage(outputDesc)
	project := func(result ownerResult) any {
		value, ok := result.(ownerUnaryResult)
		if !ok {
			panic("gojagrpc: invalid unary owner result")
		}
		if callbackErr := m.invokeMetadataCallbackID(value.onHeader, value.header); callbackErr != nil {
			panic(callbackErr)
		}
		if callbackErr := m.invokeMetadataCallbackID(value.onTrailer, value.trailer); callbackErr != nil {
			panic(callbackErr)
		}
		message, wrapErr := m.wrapMessage(value.message, outputDesc)
		if wrapErr != nil {
			panic(wrapErr)
		}
		return message
	}
	reject := func(result ownerResult) any {
		if value, ok := result.(ownerUnaryResult); ok {
			if callbackErr := m.invokeMetadataCallbackID(value.onHeader, value.header); callbackErr != nil {
				panic(callbackErr)
			}
			if callbackErr := m.invokeMetadataCallbackID(value.onTrailer, value.trailer); callbackErr != nil {
				panic(callbackErr)
			}
		}
		return m.grpcErrorFromGoError(ownerResultError(result))
	}
	promise := m.newOwnerPromise(copts.rootID, project, reject)
	if !promise.admitted() {
		copts.finishOwner()
		return promise.value
	}
	if err := copts.register(); err != nil {
		_ = m.rejectOwnerPromiseInline(promise.id, err)
		copts.finishOwner()
		return promise.value
	}

	onHeader := copts.headerCallback()
	onTrailer := copts.trailerCallback()
	promiseID := promise.id
	root := copts.workerRoot()
	defaultInproc := m.defaultInprocChannel(cc)

	go func() {
		boundary := rootWorkerBoundary{root: root, promise: promiseID}
		boundary.run(func() {
			var headerMD, trailerMD grpcmetadata.MD
			var grpcOpts []grpc.CallOption
			if onHeader != (ownerCallbackID{}) {
				grpcOpts = append(grpcOpts, grpc.Header(&headerMD))
			}
			if onTrailer != (ownerCallbackID{}) {
				grpcOpts = append(grpcOpts, grpc.Trailer(&trailerMD))
			}

			var (
				callSnapshot workerErrorSnapshot
				disposition  error
				noTransport  bool
				lifecycle    transportLifecycle
			)
			if defaultInproc {
				stream, streamErr := cc.NewStream(
					root.control.ctx,
					&grpc.StreamDesc{},
					fullMethod,
					grpcOpts...,
				)
				if streamErr != nil {
					callSnapshot = snapshotWorkerError(streamErr)
					noTransport = true
				} else {
					var bindErr error
					lifecycle, bindErr = bindClientTransport(
						root.control,
						stream,
						true,
					)
					boundary.transportBound = true
					if bindErr != nil {
						callSnapshot = snapshotWorkerError(bindErr)
					} else if sendErr := stream.SendMsg(reqMsg); sendErr != nil {
						callSnapshot = snapshotWorkerError(sendErr)
					} else if recvErr := stream.RecvMsg(respMsg); recvErr != nil {
						callSnapshot = snapshotWorkerError(recvErr)
					}
					if lifecycle != nil {
						<-lifecycle.TerminalDone()
						terminalErr, terminalKnown := lifecycle.TerminalResult()
						if !terminalKnown {
							terminalErr = status.Error(
								codes.Internal,
								"inproc terminal signal closed without a result",
							)
							if callSnapshot.err() == nil {
								callSnapshot = snapshotWorkerError(terminalErr)
							}
						}
						disposition = canonicalWorkerError(terminalErr)
					}
				}
			} else {
				bindErr := root.control.bindRelease(nil)
				boundary.transportBound = true
				if bindErr != nil {
					callSnapshot = snapshotWorkerError(bindErr)
				} else {
					callSnapshot = snapshotWorkerError(cc.Invoke(
						root.control.ctx,
						fullMethod,
						reqMsg,
						respMsg,
						grpcOpts...,
					))
				}
				disposition = callSnapshot.err()
			}
			callErr := callSnapshot.err()
			if disposition == nil && callErr != nil {
				disposition = callErr
			}
			result := ownerUnaryResult{
				message:   cloneOwnerMessage(respMsg),
				header:    headerMD.Copy(),
				trailer:   trailerMD.Copy(),
				status:    callSnapshot.result().status,
				onHeader:  onHeader,
				onTrailer: onTrailer,
			}
			var settleErr error
			if callErr != nil {
				settleErr = root.owner.settleOwnerPromise(promiseID, result, true)
			} else {
				settleErr = root.owner.resolveOwnerPromise(promiseID, result)
			}
			if settleErr != nil {
				disposition = settleErr
			}
			if noTransport {
				root.failConstruction(disposition)
				return
			}
			root.finish(disposition)
		})
	}()

	return promise.value
}

// ===================== Error Conversion ======================

// grpcErrorFromGoError converts a Go error to a JS GrpcError object.
// If the error contains a gRPC status, the code, message, and details
// are extracted. Context cancellation maps to CANCELLED, deadline
// exceeded maps to DEADLINE_EXCEEDED. Otherwise, codes.Internal is used.
//
// Must be called by the current logical adapter callback owner because it
// creates Goja objects.
func (m *Module) grpcErrorFromGoError(err error) *goja.Object {
	s, ok := status.FromError(err)
	if ok {
		obj := m.newGrpcError(s.Code(), s.Message())
		// Extract details from the status proto.
		if sp := s.Proto(); sp != nil && len(sp.GetDetails()) > 0 {
			_ = obj.Set("details", m.wrapStatusDetails(sp.GetDetails()))
			if err := m.storeStatusDetails(obj, sp.GetDetails()); err != nil {
				panic(m.runtime.NewGoError(err))
			}
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
// Must be called by the current logical adapter callback owner.
func (m *Module) invokeMetadataCallback(fn goja.Callable, md grpcmetadata.MD) (err error) {
	if fn == nil {
		return nil
	}
	if md == nil {
		md = grpcmetadata.MD{}
	}
	returned := false
	defer func() {
		reason := recover()
		if reason != nil {
			panic(reason)
		}
		if !returned {
			panic(errMetadataCallbackExit)
		}
	}()
	_, err = fn(goja.Undefined(), m.newMetadataWrapper(md))
	returned = true
	return err
}

// submitOrRejectDirect attempts to submit fn under the logical adapter owner.
// If owner submission fails, it invokes the adapter-backed rejection admission
// from the worker without accessing Goja there; native settlement still
// depends on a successful owner submission.
func (m *Module) submitOrRejectDirect(reject func(any), fn func()) {
	m.submitOrRejectDirectCleanup(reject, nil, fn)
}

func (m *Module) submitOrRejectDirectCleanup(reject func(any), cleanup func(), fn func()) {
	if submitErr := m.submit(fn); submitErr != nil {
		if cleanup != nil {
			cleanup()
		}
		reject(fmt.Errorf("event loop not running"))
	}
}
