package grpcutil

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestTranslateContextError_Canceled(t *testing.T) {
	st, _ := status.FromError(TranslateContextError(context.Canceled))
	if st.Code() != codes.Canceled {
		t.Errorf("got %v", st.Code())
	}
}

func TestTranslateContextError_Deadline(t *testing.T) {
	st, _ := status.FromError(TranslateContextError(context.DeadlineExceeded))
	if st.Code() != codes.DeadlineExceeded {
		t.Errorf("got %v", st.Code())
	}
}

func TestTranslateContextError_Other(t *testing.T) {
	err := status.Error(codes.Internal, "x")
	if TranslateContextError(err) != err {
		t.Error("should pass through")
	}
}

func TestFindUnaryMethod(t *testing.T) {
	methods := []grpc.MethodDesc{{MethodName: "A"}, {MethodName: "B"}}
	if m := FindUnaryMethod("B", methods); m == nil || m.MethodName != "B" {
		t.Error("not found")
	}
	if FindUnaryMethod("C", methods) != nil {
		t.Error("found nonexistent")
	}
}

func TestFindStreamingMethod(t *testing.T) {
	streams := []grpc.StreamDesc{{StreamName: "X"}, {StreamName: "Y"}}
	if s := FindStreamingMethod("X", streams); s == nil || s.StreamName != "X" {
		t.Error("not found")
	}
	if FindStreamingMethod("Z", streams) != nil {
		t.Error("found nonexistent")
	}
}
