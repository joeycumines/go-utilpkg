// Package grpcutil provides gRPC utility functions for error translation
// and method lookup.
package grpcutil

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TranslateContextError converts context errors to gRPC status errors.
func TranslateContextError(err error) error {
	switch err {
	case context.DeadlineExceeded:
		return status.Error(codes.DeadlineExceeded, err.Error())
	case context.Canceled:
		return status.Error(codes.Canceled, err.Error())
	default:
		return err
	}
}

// FindUnaryMethod looks up a unary method by name from a service's method list.
func FindUnaryMethod(name string, methods []grpc.MethodDesc) *grpc.MethodDesc {
	for i := range methods {
		if methods[i].MethodName == name {
			return &methods[i]
		}
	}
	return nil
}

// FindStreamingMethod looks up a streaming method by name from a service's stream list.
func FindStreamingMethod(name string, streams []grpc.StreamDesc) *grpc.StreamDesc {
	for i := range streams {
		if streams[i].StreamName == name {
			return &streams[i]
		}
	}
	return nil
}
