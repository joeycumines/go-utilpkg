package gojagrpc

import (
	"fmt"

	"github.com/joeycumines/goja"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type statusDetailHolder struct {
	details []*anypb.Any
}

func newStatusDetailStore(runtime *goja.Runtime) (*goja.Object, goja.Callable, goja.Callable, error) {
	constructor, ok := runtime.Intrinsic(goja.IntrinsicWeakMapConstructor)
	if !ok {
		return nil, nil, nil, fmt.Errorf("gojagrpc: WeakMap intrinsic is unavailable")
	}
	getValue, ok := runtime.Intrinsic(goja.IntrinsicWeakMapGet)
	if !ok {
		return nil, nil, nil, fmt.Errorf("gojagrpc: WeakMap.prototype.get intrinsic is unavailable")
	}
	get, ok := goja.AssertFunction(getValue)
	if !ok {
		return nil, nil, nil, fmt.Errorf("gojagrpc: WeakMap.prototype.get intrinsic is not callable")
	}
	setValue, ok := runtime.Intrinsic(goja.IntrinsicWeakMapSet)
	if !ok {
		return nil, nil, nil, fmt.Errorf("gojagrpc: WeakMap.prototype.set intrinsic is unavailable")
	}
	set, ok := goja.AssertFunction(setValue)
	if !ok {
		return nil, nil, nil, fmt.Errorf("gojagrpc: WeakMap.prototype.set intrinsic is not callable")
	}
	store, err := runtime.New(constructor)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("gojagrpc: initialize status detail store: %w", err)
	}
	return store, get, set, nil
}

// statusObject returns a goja.Object exposing all gRPC status codes
// and a createError factory function.
//
// JavaScript usage:
//
//	grpc.status.OK           // 0
//	grpc.status.NOT_FOUND    // 5
//	grpc.status.createError(code, message)             // → GrpcError object
//	grpc.status.createError(code, message, [detail])   // → with details
func (m *Module) statusObject() *goja.Object {
	obj := m.runtime.NewObject()

	// All 17 standard gRPC status codes.
	_ = obj.Set("OK", int32(codes.OK))
	_ = obj.Set("CANCELLED", int32(codes.Canceled))
	_ = obj.Set("UNKNOWN", int32(codes.Unknown))
	_ = obj.Set("INVALID_ARGUMENT", int32(codes.InvalidArgument))
	_ = obj.Set("DEADLINE_EXCEEDED", int32(codes.DeadlineExceeded))
	_ = obj.Set("NOT_FOUND", int32(codes.NotFound))
	_ = obj.Set("ALREADY_EXISTS", int32(codes.AlreadyExists))
	_ = obj.Set("PERMISSION_DENIED", int32(codes.PermissionDenied))
	_ = obj.Set("RESOURCE_EXHAUSTED", int32(codes.ResourceExhausted))
	_ = obj.Set("FAILED_PRECONDITION", int32(codes.FailedPrecondition))
	_ = obj.Set("ABORTED", int32(codes.Aborted))
	_ = obj.Set("OUT_OF_RANGE", int32(codes.OutOfRange))
	_ = obj.Set("UNIMPLEMENTED", int32(codes.Unimplemented))
	_ = obj.Set("INTERNAL", int32(codes.Internal))
	_ = obj.Set("UNAVAILABLE", int32(codes.Unavailable))
	_ = obj.Set("DATA_LOSS", int32(codes.DataLoss))
	_ = obj.Set("UNAUTHENTICATED", int32(codes.Unauthenticated))

	// createError(code, message, details?) → GrpcError object
	_ = obj.Set("createError", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		code := int32(call.Argument(0).ToInteger())
		message := call.Argument(1).String()

		// Optional third argument: array of protobuf message details.
		detailsArg := call.Argument(2)
		if detailsArg == nil || goja.IsUndefined(detailsArg) || goja.IsNull(detailsArg) {
			return m.newGrpcError(codes.Code(code), message)
		}

		arrObj, ok := detailsArg.(*goja.Object)
		if !ok || arrObj.ClassName() != "Array" {
			panic(m.runtime.NewTypeError("status details must be an array"))
		}

		lenVal := arrObj.Get("length")
		if lenVal == nil || goja.IsUndefined(lenVal) {
			panic(m.runtime.NewTypeError("status details must be an array"))
		}

		length := int(lenVal.ToInteger())
		if length == 0 {
			return m.newGrpcError(codes.Code(code), message)
		}

		details := make([]goja.Value, 0, length)
		for i := range length {
			details = append(details, arrObj.Get(fmt.Sprintf("%d", i)))
		}

		return m.newGrpcErrorWithDetails(codes.Code(code), message, details)
	}))

	return obj
}

// newGrpcError creates a JavaScript GrpcError object with the given
// gRPC status code and message. The object has name, code, message,
// and details properties. details is always an empty array unless
// created with [newGrpcErrorWithDetails].
func (m *Module) newGrpcError(code codes.Code, message string) *goja.Object {
	obj := m.runtime.NewObject()
	_ = obj.Set("name", "GrpcError")
	_ = obj.Set("code", int32(code))
	_ = obj.Set("message", message)
	_ = obj.Set("details", m.runtime.NewArray())
	_ = obj.Set("toString", m.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		return m.runtime.ToValue("GrpcError: " + codes.Code(code).String() + ": " + message)
	}))
	return obj
}

// newGrpcErrorWithDetails creates a GrpcError with attached details.
// The details are stored both as a JS array (for script access) and
// as a pre-converted []*anypb.Any (for Go-side status conversion).
func (m *Module) newGrpcErrorWithDetails(code codes.Code, message string, details []goja.Value) *goja.Object {
	obj := m.newGrpcError(code, message)

	// Build JS-visible details array.
	arr := m.runtime.NewArray()
	for i, d := range details {
		_ = arr.Set(fmt.Sprintf("%d", i), d)
	}
	_ = obj.Set("details", arr)

	// Pre-convert to *anypb.Any for Go-side extraction.
	var anyDetails []*anypb.Any
	for _, d := range details {
		msg, err := m.protobuf.UnwrapMessage(d)
		if err != nil {
			panic(m.runtime.NewTypeError("status detail: %s", err))
		}
		a, err := anypb.New(msg)
		if err != nil {
			panic(m.runtime.NewTypeError("status detail: %s", err))
		}
		anyDetails = append(anyDetails, a)
	}

	if err := m.storeStatusDetails(obj, anyDetails); err != nil {
		panic(m.runtime.NewGoError(err))
	}

	return obj
}

func (m *Module) storeStatusDetails(object *goja.Object, details []*anypb.Any) error {
	cloned := make([]*anypb.Any, 0, len(details))
	for _, detail := range details {
		if detail != nil {
			cloned = append(cloned, proto.Clone(detail).(*anypb.Any))
		}
	}
	holder := m.runtime.ToValue(&statusDetailHolder{details: cloned})
	if _, err := m.statusDetailSet(m.statusDetailStore, object, holder); err != nil {
		return fmt.Errorf("gojagrpc: store private status details: %w", err)
	}
	return nil
}

// extractGoDetails returns private, cloned detail protos.
func (m *Module) extractGoDetails(obj *goja.Object) []*anypb.Any {
	value, err := m.statusDetailGet(m.statusDetailStore, obj)
	if err != nil {
		panic(m.runtime.NewGoError(fmt.Errorf("gojagrpc: load private status details: %w", err)))
	}
	if value == nil || goja.IsUndefined(value) {
		return nil
	}
	holder, ok := value.Export().(*statusDetailHolder)
	if !ok || holder == nil {
		panic(m.runtime.NewGoError(fmt.Errorf("gojagrpc: invalid private status detail identity")))
	}
	cloned := make([]*anypb.Any, 0, len(holder.details))
	for _, detail := range holder.details {
		cloned = append(cloned, proto.Clone(detail).(*anypb.Any))
	}
	return cloned
}

// wrapStatusDetails converts a slice of *anypb.Any detail messages
// into a JS array of wrapped protobuf messages. Unknown details remain
// available losslessly as google.protobuf.Any wrappers.
func (m *Module) wrapStatusDetails(details []*anypb.Any) *goja.Object {
	arr := m.runtime.NewArray()
	for index, detail := range details {
		message, err := anypb.UnmarshalNew(detail, proto.UnmarshalOptions{
			Resolver: m.protobuf.TypeResolver(),
		})
		if err != nil {
			message = proto.Clone(detail)
		}
		value, wrapErr := m.wrapMessage(message, message.ProtoReflect().Descriptor())
		if wrapErr != nil {
			panic(wrapErr)
		}
		_ = arr.Set(fmt.Sprintf("%d", index), value)
	}
	return arr
}
