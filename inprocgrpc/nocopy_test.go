package inprocgrpc_test

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/wrapperspb"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
)

func TestChannel_WithCloneDisabled(t *testing.T) {
	ch := newTestChannel(t, inprocgrpc.WithCloneDisabled())

	val := "hello"
	req := &wrapperspb.StringValue{Value: val}
	resp := new(wrapperspb.StringValue)

	if err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp); err != nil {
		t.Fatal(err)
	}

	if resp.GetValue() != "echo: hello" {
		t.Errorf("got %q", resp.GetValue())
	}
}

// TestChannel_WithCloneDisabled_SharedMemory verifies that we are indeed sharing memory
// for pointer fields.
func TestChannel_WithCloneDisabled_SharedMemory(t *testing.T) {
	ch := newBareChannel(t, inprocgrpc.WithCloneDisabled())

	service := &testBytesService{}
	desc := grpc.ServiceDesc{
		ServiceName: "test.TestBytesService",
		HandlerType: (*testBytesServer)(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "Unary",
				Handler:    testBytesUnaryHandler,
			},
		},
		Streams:  []grpc.StreamDesc{},
		Metadata: "test.proto",
	}

	ch.RegisterService(&desc, service)

	// Create a byte slice that we expect to be modified in place.
	original := []byte("original")
	req := &wrapperspb.BytesValue{Value: original}
	resp := new(wrapperspb.BytesValue)

	// Invoke synchronously.
	if err := ch.Invoke(context.Background(), "/test.TestBytesService/Unary", req, resp); err != nil {
		t.Fatal(err)
	}

	// Check if the original slice backing array was modified.
	if string(req.Value) != "modified" {
		t.Errorf("expected request value to be modified to 'modified', got %q", string(req.Value))
	}
}

type testBytesServer interface {
	Unary(context.Context, *wrapperspb.BytesValue) (*wrapperspb.BytesValue, error)
}

type testBytesService struct{}

func (s *testBytesService) Unary(ctx context.Context, req *wrapperspb.BytesValue) (*wrapperspb.BytesValue, error) {
	// Modify the input slice directly.
	// Since we disabled cloning, this should reflect in the caller's slice if they share backing array.
	// We use copy to modify the existing backing array without reallocating.
	if len(req.Value) >= 8 {
		copy(req.Value, []byte("modified"))
	}
	return &wrapperspb.BytesValue{Value: req.Value}, nil
}

func testBytesUnaryHandler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(wrapperspb.BytesValue)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(testBytesServer).Unary(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/test.TestBytesService/Unary",
	}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(testBytesServer).Unary(ctx, req.(*wrapperspb.BytesValue))
	}
	return interceptor(ctx, in, info, handler)
}
