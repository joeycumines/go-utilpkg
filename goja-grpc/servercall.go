package gojagrpc

import (
	"errors"
	"fmt"
	"io"

	"github.com/joeycumines/goja"
	"google.golang.org/grpc/codes"
	grpcmetadata "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func (m *Module) newServerCallObject(rpc *serverRPC) *goja.Object {
	object := m.runtime.NewObject()
	incoming, _ := grpcmetadata.FromIncomingContext(rpc.ctx)
	_ = object.Set("requestHeader", m.newReadonlyMetadataWrapper(incoming))
	_ = object.Set("setHeader", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if metadata := m.metadataToGo(call.Argument(0)); metadata != nil {
			if err := rpc.stream.SetHeader(metadata.Copy()); err != nil {
				panic(m.runtime.NewGoError(err))
			}
		}
		return goja.Undefined()
	}))
	_ = object.Set("sendHeader", m.runtime.ToValue(func(goja.FunctionCall) goja.Value {
		rpc.stream.SendHeader()
		return goja.Undefined()
	}))
	_ = object.Set("setTrailer", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		if metadata := m.metadataToGo(call.Argument(0)); metadata != nil {
			rpc.stream.SetTrailer(metadata.Copy())
		}
		return goja.Undefined()
	}))
	return object
}

func (m *Module) addServerSend(
	callObject *goja.Object,
	rpc *serverRPC,
	output protoreflect.MessageDescriptor,
) {
	_ = callObject.Set("send", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		message, err := m.snapshotMessage(call.Argument(0), output)
		if err != nil {
			panic(m.runtime.NewTypeError("send: %s", err))
		}
		promise := m.newOwnerPromise(rpc.rootID, nil, nil)
		if !promise.admitted() {
			return promise.value
		}
		if err := rpc.send(message); err != nil {
			_ = m.rejectOwnerPromiseInline(promise.id, err)
			return promise.value
		}
		_ = m.resolveOwnerPromiseInline(promise.id, ownerEmptyResult{})
		return promise.value
	}))
}

func (m *Module) addServerRecv(
	callObject *goja.Object,
	rpc *serverRPC,
	input protoreflect.MessageDescriptor,
) {
	_ = callObject.Set("recv", m.runtime.ToValue(func(goja.FunctionCall) goja.Value {
		promise := m.newOwnerPromise(rpc.rootID, func(result ownerResult) any {
			object := m.runtime.NewObject()
			switch value := result.(type) {
			case ownerEmptyResult:
				_ = object.Set("done", true)
				_ = object.Set("value", goja.Undefined())
			case ownerMessageResult:
				message, err := m.wrapMessage(value.message, input)
				if err != nil {
					panic(err)
				}
				_ = object.Set("done", false)
				_ = object.Set("value", message)
			default:
				panic("gojagrpc: invalid server recv owner result")
			}
			return object
		}, nil)
		if !promise.admitted() {
			return promise.value
		}
		promiseID := promise.id
		settleTerminal := func(err error) {
			if errors.Is(err, io.EOF) || err == nil {
				if settleErr := m.resolveOwnerPromiseInline(
					promiseID,
					ownerEmptyResult{},
				); settleErr != nil {
					rpc.finish(settleErr)
				}
				return
			}
			if settleErr := m.rejectOwnerPromiseInline(promiseID, err); settleErr != nil {
				rpc.finish(settleErr)
			}
		}
		rpc.mu.Lock()
		if rpc.recvDone {
			err := rpc.recvErr
			rpc.mu.Unlock()
			settleTerminal(err)
			return promise.value
		}
		if rpc.recvPending {
			rpc.mu.Unlock()
			if settleErr := m.rejectOwnerPromiseInline(
				promiseID,
				errConcurrentRecv,
			); settleErr != nil {
				rpc.finish(settleErr)
			}
			return promise.value
		}
		rpc.recvPending = true
		rpc.mu.Unlock()

		rpc.stream.Recv().Recv(func(message any, recvErr error) {
			rpc.run(func() {
				if terminalErr, selected := rpc.stream.TerminalResult(); selected {
					rpc.mu.Lock()
					rpc.recvPending = false
					rpc.recvDone = true
					rpc.recvErr = terminalErr
					rpc.mu.Unlock()
					settleTerminal(terminalErr)
					return
				}
				rpc.mu.Lock()
				if recvErr != nil {
					rpc.recvPending = false
					rpc.recvDone = true
					rpc.recvErr = recvErr
				}
				rpc.mu.Unlock()
				if errors.Is(recvErr, io.EOF) {
					if err := m.resolveOwnerPromiseInline(
						promiseID,
						ownerEmptyResult{},
					); err != nil {
						rpc.finish(err)
					}
					return
				}
				if recvErr != nil {
					if err := m.rejectOwnerPromiseInline(
						promiseID,
						recvErr,
					); err != nil {
						rpc.finish(err)
					}
					return
				}
				protobufMessage, ok := message.(proto.Message)
				if !ok {
					recvErr = status.Error(
						codes.Internal,
						"recv conversion: received value is not a proto.Message",
					)
				} else if err := validateMessageDescriptor(
					protobufMessage,
					input,
				); err != nil {
					recvErr = status.Errorf(
						codes.Internal,
						"recv conversion: %v",
						err,
					)
				}
				if recvErr != nil {
					rpc.mu.Lock()
					rpc.recvPending = false
					rpc.recvDone = true
					rpc.recvErr = recvErr
					rpc.mu.Unlock()
					if settleErr := m.rejectOwnerPromiseInline(
						promiseID,
						recvErr,
					); settleErr != nil {
						rpc.finish(settleErr)
					} else {
						rpc.finish(recvErr)
					}
					return
				}
				if err := m.resolveOwnerPromiseInline(
					promiseID,
					ownerMessageResult{
						message: cloneOwnerMessage(protobufMessage),
					},
				); err != nil {
					rpc.finish(err)
				}
				rpc.mu.Lock()
				rpc.recvPending = false
				rpc.mu.Unlock()
			})
		})
		return promise.value
	}))
}

func (m *Module) toWrappedMessage(
	message any,
	descriptor protoreflect.MessageDescriptor,
) (*goja.Object, error) {
	protobufMessage, ok := message.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("received value is not a proto.Message")
	}
	if err := validateMessageDescriptor(protobufMessage, descriptor); err != nil {
		return nil, err
	}
	return m.wrapMessage(protobufMessage, descriptor)
}

func (m *Module) finishHandler(
	result goja.Value,
	rpc *serverRPC,
	output protoreflect.MessageDescriptor,
	unary bool,
) {
	if result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
		if unary {
			rpc.finish(status.Error(codes.Internal, "handler returned nil/undefined"))
		} else {
			rpc.finish(nil)
		}
		return
	}
	object, ok := result.(*goja.Object)
	if !ok {
		if unary {
			m.finishUnaryResponse(result, rpc, output)
		} else {
			rpc.finish(nil)
		}
		return
	}
	thenValue := object.Get("then")
	then, thenable := goja.AssertFunction(thenValue)
	if !thenable {
		if unary {
			m.finishUnaryResponse(result, rpc, output)
		} else {
			rpc.finish(nil)
		}
		return
	}

	fulfilled := m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		value := call.Argument(0)
		rpc.schedule(func() {
			if unary {
				m.finishUnaryResponse(value, rpc, output)
			} else {
				rpc.finish(nil)
			}
		})
		return goja.Undefined()
	})
	rejected := m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		value := call.Argument(0)
		rpc.schedule(func() {
			rpc.finish(m.jsValueToGRPCError(value))
		})
		return goja.Undefined()
	})
	if _, err := then(result, fulfilled, rejected); err != nil {
		rpc.finish(m.jsErrorToGRPC(err))
	}
}

// isThenable reports whether value has a callable then property. It must run
// under the runtime owner because reading the property can execute JavaScript.
func (m *Module) isThenable(value goja.Value) bool {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return false
	}
	object, ok := value.(*goja.Object)
	if !ok {
		return false
	}
	_, ok = goja.AssertFunction(object.Get("then"))
	return ok
}

func (m *Module) finishUnaryResponse(
	value goja.Value,
	rpc *serverRPC,
	output protoreflect.MessageDescriptor,
) {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		rpc.finish(status.Error(codes.Internal, "handler returned nil/undefined"))
		return
	}
	message, err := m.snapshotMessage(value, output)
	if err != nil {
		rpc.finish(status.Errorf(codes.Internal, "handler response: %v", err))
		return
	}
	if err := rpc.send(message); err != nil {
		rpc.finish(status.Errorf(codes.Internal, "send response: %v", err))
		return
	}
	rpc.finish(nil)
}

func (m *Module) jsErrorToGRPC(err error) error {
	var exception *goja.Exception
	if errors.As(err, &exception) {
		return m.jsValueToGRPCError(exception.Value())
	}
	return status.Errorf(codes.Internal, "%v", err)
}

func (m *Module) jsValueToGRPCError(value goja.Value) error {
	if value == nil || goja.IsUndefined(value) {
		return status.Error(codes.Internal, "unknown error")
	}
	object, ok := value.(*goja.Object)
	if !ok {
		return status.Errorf(codes.Internal, "%s", value.String())
	}
	if name := object.Get("name"); name != nil && name.String() == "GrpcError" {
		code := codes.Code(object.Get("code").ToInteger())
		message := object.Get("message").String()
		details := m.extractGoDetails(object)
		if len(details) == 0 {
			return status.Error(code, message)
		}
		statusProto := status.New(code, message).Proto()
		statusProto.Details = details
		return status.FromProto(statusProto).Err()
	}
	if message := object.Get("message"); message != nil &&
		!goja.IsUndefined(message) {
		return status.Errorf(codes.Internal, "%s", message.String())
	}
	return status.Errorf(codes.Internal, "%s", value.String())
}
