package inprocgrpc_test

import (
	"context"
	"fmt"
	"io"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
)

func TestChannel_StatsHandler_HeadersTrailers(t *testing.T) {
	clientRec := &statsRecorder{}
	serverRec := &statsRecorder{}
	ch := newBareChannel(t, inprocgrpc.WithClientStatsHandler(clientRec), inprocgrpc.WithServerStatsHandler(serverRec))
	desc := testServiceDesc
	desc.Streams = []grpc.StreamDesc{
		{
			StreamName: "ServerStream",
			Handler: func(srv any, stream grpc.ServerStream) error {
				// Send explicit headers
				if err := stream.SendHeader(metadata.Pairs("svr-head", "hv")); err != nil {
					return err
				}
				stream.SetTrailer(metadata.Pairs("svr-trail", "tv"))
				in := new(wrapperspb.StringValue)
				if err := stream.RecvMsg(in); err != nil {
					return err
				}
				return stream.SendMsg(&wrapperspb.StringValue{Value: "ok"})
			},
			ServerStreams: true,
		},
	}
	ch.RegisterService(&desc, &echoServer{})

	stream, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
	}, "/test.TestService/ServerStream")
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SendMsg(&wrapperspb.StringValue{Value: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}

	// Read headers
	hdrs, err := stream.Header()
	if err != nil {
		t.Fatal(err)
	}
	if v := hdrs.Get("svr-head"); len(v) == 0 || v[0] != "hv" {
		t.Errorf("headers: %v", hdrs)
	}

	// Read response
	resp := new(wrapperspb.StringValue)
	if err := stream.RecvMsg(resp); err != nil {
		t.Fatal(err)
	}

	// Read EOF
	if err := stream.RecvMsg(new(wrapperspb.StringValue)); err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}

	// Check client stats events for InHeader
	var clientHasInHeader, clientHasInTrailer bool
	for _, ev := range clientRec.getEvents() {
		switch ev.(type) {
		case *stats.InHeader:
			clientHasInHeader = true
		case *stats.InTrailer:
			clientHasInTrailer = true
		}
	}
	if !clientHasInHeader {
		t.Error("client missing InHeader event")
	}
	if !clientHasInTrailer {
		t.Error("client missing InTrailer event")
	}

	// Check server stats events for OutHeader, OutPayload
	var serverHasOutHeader, serverHasOutPayload, serverHasOutTrailer bool
	for _, ev := range serverRec.getEvents() {
		switch ev.(type) {
		case *stats.OutHeader:
			serverHasOutHeader = true
		case *stats.OutPayload:
			serverHasOutPayload = true
		case *stats.OutTrailer:
			serverHasOutTrailer = true
		}
	}
	if !serverHasOutHeader {
		t.Error("server missing OutHeader event")
	}
	if !serverHasOutPayload {
		t.Error("server missing OutPayload event")
	}
	if !serverHasOutTrailer {
		t.Error("server missing OutTrailer event")
	}
}

func TestChannel_Stream_SetHeaderAfterSend(t *testing.T) {
	ch := newBareChannel(t)
	desc := testServiceDesc
	desc.Streams = []grpc.StreamDesc{
		{
			StreamName: "ServerStream",
			Handler: func(srv any, stream grpc.ServerStream) error {
				// Send a message first (implicitly sends headers)
				if err := stream.SendMsg(&wrapperspb.StringValue{Value: "first"}); err != nil {
					return err
				}
				// Now try to SetHeader - should fail
				err := stream.SetHeader(metadata.Pairs("late", "val"))
				if err == nil {
					return fmt.Errorf("expected error from SetHeader after send")
				}
				// SendHeader should also fail
				err = stream.SendHeader(metadata.Pairs("late2", "val2"))
				if err == nil {
					return fmt.Errorf("expected error from SendHeader after send")
				}
				return nil
			},
			ServerStreams: true,
		},
	}
	ch.RegisterService(&desc, &echoServer{})

	stream, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
	}, "/test.TestService/ServerStream")
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SendMsg(&wrapperspb.StringValue{Value: "trigger"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}

	// Read response
	resp := new(wrapperspb.StringValue)
	if err := stream.RecvMsg(resp); err != nil {
		t.Fatal(err)
	}

	// Read EOF
	if err := stream.RecvMsg(new(wrapperspb.StringValue)); err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
}

func TestChannel_Stream_SendNilMessage(t *testing.T) {
	ch := newBareChannel(t)
	desc := testServiceDesc
	desc.Streams = []grpc.StreamDesc{
		{
			StreamName: "ServerStream",
			Handler: func(srv any, stream grpc.ServerStream) error {
				// Consume client request first to prevent race
				if err := stream.RecvMsg(new(wrapperspb.StringValue)); err != nil {
					return err
				}
				// Try to send nil
				err := stream.SendMsg(nil)
				return err
			},
			ServerStreams: true,
		},
	}
	ch.RegisterService(&desc, &echoServer{})

	stream, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
	}, "/test.TestService/ServerStream")
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SendMsg(&wrapperspb.StringValue{Value: "trigger"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}

	// Server sent nil → should get some error
	resp := new(wrapperspb.StringValue)
	err = stream.RecvMsg(resp)
	if err == nil {
		t.Error("expected error")
	}
}

func TestChannel_Stream_ClientSendAfterClose(t *testing.T) {
	ch := newTestChannel(t)
	stream, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		StreamName:    "BidiStream",
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	// Send after close should fail
	err = stream.SendMsg(&wrapperspb.StringValue{Value: "after close"})
	if err == nil {
		t.Error("expected error sending after CloseSend")
	}
}

func TestChannel_Stream_ClientSendNil(t *testing.T) {
	ch := newTestChannel(t)
	stream, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		StreamName:    "BidiStream",
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}
	err = stream.SendMsg(nil)
	if err == nil {
		t.Error("expected error sending nil")
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
}

func TestChannel_Invoke_WithHeaders_Unary(t *testing.T) {
	// Test the unary path with explicit SendHeader within handler
	ch := newBareChannel(t)
	desc := testServiceDesc
	desc.Methods = []grpc.MethodDesc{
		{
			MethodName: "Unary",
			Handler: func(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
				in := new(wrapperspb.StringValue)
				if err := dec(in); err != nil {
					return nil, err
				}
				_ = grpc.SendHeader(ctx, metadata.Pairs("h", "v"))
				grpc.SetTrailer(ctx, metadata.Pairs("t", "tv"))
				return &wrapperspb.StringValue{Value: "done"}, nil
			},
		},
	}
	ch.RegisterService(&desc, &echoServer{})

	var hdrs, tlrs metadata.MD
	req := &wrapperspb.StringValue{Value: "hi"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp,
		grpc.Header(&hdrs), grpc.Trailer(&tlrs))
	if err != nil {
		t.Fatal(err)
	}
	if v := hdrs.Get("h"); len(v) == 0 || v[0] != "v" {
		t.Errorf("headers: %v", hdrs)
	}
	if v := tlrs.Get("t"); len(v) == 0 || v[0] != "tv" {
		t.Errorf("trailers: %v", tlrs)
	}
}

func TestChannel_Invoke_HandlerReturnsNil(t *testing.T) {
	ch := newBareChannel(t)
	desc := testServiceDesc
	desc.Methods = []grpc.MethodDesc{
		{
			MethodName: "Unary",
			Handler: func(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
				in := new(wrapperspb.StringValue)
				if err := dec(in); err != nil {
					return nil, err
				}
				return nil, nil // return nil response without error
			},
		},
	}
	ch.RegisterService(&desc, &echoServer{})

	req := &wrapperspb.StringValue{Value: "hi"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp)
	if err == nil {
		t.Fatal("expected error for nil response")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal {
		t.Errorf("got %v, want Internal", st.Code())
	}
}

func TestChannel_Invoke_MalformedMethod(t *testing.T) {
	ch := newTestChannel(t)
	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)

	// Method with no slash separator after leading /
	err := ch.Invoke(context.Background(), "/justServiceName", req, resp)
	if err == nil {
		t.Fatal("expected error for malformed method")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got %v, want InvalidArgument", st.Code())
	}
}

func TestChannel_NewStream_MalformedMethod(t *testing.T) {
	ch := newTestChannel(t)
	_, err := ch.NewStream(context.Background(), &grpc.StreamDesc{}, "/justServiceName")
	if err == nil {
		t.Fatal("expected error for malformed method")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got %v, want InvalidArgument", st.Code())
	}
}

func TestChannel_RegisterService_InvalidHandlerType(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for invalid handler type")
		}
	}()
	ch := newBareChannel(t)
	// testServiceDesc has HandlerType: (*testServiceServer)(nil)
	// struct{}{} does NOT implement testServiceServer
	ch.RegisterService(&testServiceDesc, struct{}{})
}

func TestChannel_Invoke_ServerContextError(t *testing.T) {
	// When a server handler returns a raw context.Canceled error,
	// the client should receive it as a proper gRPC status error.
	ch := newBareChannel(t)
	desc := testServiceDesc
	desc.Methods = []grpc.MethodDesc{
		{
			MethodName: "Unary",
			Handler: func(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
				in := new(wrapperspb.StringValue)
				if err := dec(in); err != nil {
					return nil, err
				}
				return nil, context.Canceled // raw context error
			},
		},
	}
	ch.RegisterService(&desc, &echoServer{})

	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp)
	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %T: %v", err, err)
	}
	if st.Code() != codes.Canceled {
		t.Errorf("got %v, want Canceled", st.Code())
	}
}
