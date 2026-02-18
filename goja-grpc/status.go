package gojagrpc

import (
	"fmt"
	"strings"

	"github.com/dop251/goja"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/anypb"
)

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
		if !ok {
			return m.newGrpcError(codes.Code(code), message)
		}

		lenVal := arrObj.Get("length")
		if lenVal == nil || goja.IsUndefined(lenVal) {
			return m.newGrpcError(codes.Code(code), message)
		}

		length := int(lenVal.ToInteger())
		if length == 0 {
			return m.newGrpcError(codes.Code(code), message)
		}

		details := make([]goja.Value, 0, length)
		for i := range length {
			elemVal := arrObj.Get(fmt.Sprintf("%d", i))
			if elemVal != nil && !goja.IsUndefined(elemVal) {
				details = append(details, elemVal)
			}
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

// goDetailsHolder wraps []*anypb.Any for storage on a GrpcError
// object. Using a struct prevents goja from converting the slice
// into a JS array during set/get.
type goDetailsHolder struct {
	details []*anypb.Any
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
			continue
		}
		a, err := anypb.New(msg)
		if err != nil {
			continue
		}
		anyDetails = append(anyDetails, a)
	}

	_ = obj.Set("_goDetails", &goDetailsHolder{details: anyDetails})

	return obj
}

// extractGoDetails extracts pre-converted detail protos from a
// GrpcError object (stored by newGrpcErrorWithDetails). Returns nil
// if no details are present.
func (m *Module) extractGoDetails(obj *goja.Object) []*anypb.Any {
	goDetailsVal := obj.Get("_goDetails")
	if goDetailsVal == nil || goja.IsUndefined(goDetailsVal) || goja.IsNull(goDetailsVal) {
		return nil
	}

	holder, ok := goDetailsVal.Export().(*goDetailsHolder)
	if !ok {
		return nil
	}
	return holder.details
}

// wrapStatusDetails converts a slice of *anypb.Any detail messages
// into a JS array of wrapped protobuf messages. Types that cannot be
// resolved are silently skipped.
func (m *Module) wrapStatusDetails(details []*anypb.Any) *goja.Object {
	arr := m.runtime.NewArray()
	idx := 0
	for _, any := range details {
		typeURL := any.GetTypeUrl()
		// Strip "type.googleapis.com/" prefix.
		fullName := typeURL
		if i := strings.LastIndex(typeURL, "/"); i >= 0 {
			fullName = typeURL[i+1:]
		}

		desc, err := m.protobuf.FindDescriptor(protoreflect.FullName(fullName))
		if err != nil {
			continue // Unknown type — skip.
		}

		md, ok := desc.(protoreflect.MessageDescriptor)
		if !ok {
			continue
		}

		dynMsg := dynamicpb.NewMessage(md)
		if err := anypb.UnmarshalTo(any, dynMsg, proto.UnmarshalOptions{}); err != nil {
			continue
		}

		_ = arr.Set(fmt.Sprintf("%d", idx), m.protobuf.WrapMessage(dynMsg))
		idx++
	}
	return arr
}
