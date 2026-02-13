package inprocgrpc_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
)

// --- Test service infrastructure ---

type testServiceServer interface {
	Unary(context.Context, *wrapperspb.StringValue) (*wrapperspb.StringValue, error)
	ServerStream(*wrapperspb.StringValue, grpc.ServerStream) error
	ClientStream(grpc.ServerStream) error
	BidiStream(grpc.ServerStream) error
}

var testServiceDesc = grpc.ServiceDesc{
	ServiceName: "test.TestService",
	HandlerType: (*testServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Unary",
			Handler:    testUnaryHandler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "ServerStream",
			Handler:       testServerStreamHandler,
			ServerStreams: true,
			ClientStreams: false,
		},
		{
			StreamName:    "ClientStream",
			Handler:       testClientStreamHandler,
			ServerStreams: false,
			ClientStreams: true,
		},
		{
			StreamName:    "BidiStream",
			Handler:       testBidiStreamHandler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "test.proto",
}

func testUnaryHandler(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	in := new(wrapperspb.StringValue)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(testServiceServer).Unary(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/test.TestService/Unary",
	}
	handler := func(ctx context.Context, req any) (any, error) {
		return srv.(testServiceServer).Unary(ctx, req.(*wrapperspb.StringValue))
	}
	return interceptor(ctx, in, info, handler)
}

func testServerStreamHandler(srv any, stream grpc.ServerStream) error {
	in := new(wrapperspb.StringValue)
	if err := stream.RecvMsg(in); err != nil {
		return err
	}
	return srv.(testServiceServer).ServerStream(in, stream)
}

func testClientStreamHandler(srv any, stream grpc.ServerStream) error {
	return srv.(testServiceServer).ClientStream(stream)
}

func testBidiStreamHandler(srv any, stream grpc.ServerStream) error {
	return srv.(testServiceServer).BidiStream(stream)
}

// --- echoServer implements testServiceServer ---

type echoServer struct{}

func (s *echoServer) Unary(ctx context.Context, req *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
	// Set headers from incoming metadata
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if vals := md.Get("test-header"); len(vals) > 0 {
			_ = grpc.SetHeader(ctx, metadata.Pairs("echo-header", vals[0]))
		}
	}
	grpc.SetTrailer(ctx, metadata.Pairs("echo-trailer", "trailer-value"))
	return &wrapperspb.StringValue{Value: "echo: " + req.GetValue()}, nil
}

func (s *echoServer) ServerStream(req *wrapperspb.StringValue, stream grpc.ServerStream) error {
	for i := range 3 {
		if err := stream.SendMsg(&wrapperspb.StringValue{
			Value: fmt.Sprintf("%s:%d", req.GetValue(), i),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *echoServer) ClientStream(stream grpc.ServerStream) error {
	var count int
	for {
		in := new(wrapperspb.StringValue)
		err := stream.RecvMsg(in)
		if err == io.EOF {
			return stream.SendMsg(&wrapperspb.StringValue{
				Value: fmt.Sprintf("received %d messages", count),
			})
		}
		if err != nil {
			return err
		}
		count++
	}
}

func (s *echoServer) BidiStream(stream grpc.ServerStream) error {
	for {
		in := new(wrapperspb.StringValue)
		err := stream.RecvMsg(in)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := stream.SendMsg(&wrapperspb.StringValue{
			Value: "bidi: " + in.GetValue(),
		}); err != nil {
			return err
		}
	}
}

// --- errorServer returns errors for testing error paths ---

type errorServer struct {
	echoServer
}

func (s *errorServer) Unary(_ context.Context, _ *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
	return nil, status.Error(codes.AlreadyExists, "already exists")
}

// --- Helper ---

// (newTestChannel and newBareChannel are in testhelper_test.go)

// --- Tests ---

func TestNewChannel_NilLoop(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for nil loop")
		}
		s, ok := r.(string)
		if !ok {
			t.Fatalf("unexpected panic type: %T", r)
		}
		// The panic message includes "inprocgrpc:" prefix from the panic formatting
		if s != "inprocgrpc: inprocgrpc: loop must be provided via WithLoop" &&
			s != "inprocgrpc: loop must be provided via WithLoop" {
			t.Fatalf("unexpected panic message: %q", s)
		}
	}()
	inprocgrpc.NewChannel()
}

func TestChannel_Invoke_Unary(t *testing.T) {
	ch := newTestChannel(t)
	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	ctx := metadata.NewOutgoingContext(context.Background(),
		metadata.Pairs("test-header", "header-value"))
	var respHeaders, respTrailers metadata.MD
	err := ch.Invoke(ctx, "/test.TestService/Unary", req, resp,
		grpc.Header(&respHeaders),
		grpc.Trailer(&respTrailers),
	)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if resp.GetValue() != "echo: hello" {
		t.Errorf("got %q, want %q", resp.GetValue(), "echo: hello")
	}
	if v := respHeaders.Get("echo-header"); len(v) == 0 || v[0] != "header-value" {
		t.Errorf("response headers: got %v", respHeaders)
	}
	if v := respTrailers.Get("echo-trailer"); len(v) == 0 || v[0] != "trailer-value" {
		t.Errorf("response trailers: got %v", respTrailers)
	}
}

func TestChannel_Invoke_Error(t *testing.T) {
	ch := newBareChannel(t)
	ch.RegisterService(&testServiceDesc, &errorServer{})
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
	if st.Code() != codes.AlreadyExists {
		t.Errorf("got %v, want AlreadyExists", st.Code())
	}
}

func TestChannel_Invoke_NilRequest(t *testing.T) {
	ch := newTestChannel(t)
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "/test.TestService/Unary", nil, resp)
	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestChannel_Invoke_UnimplementedService(t *testing.T) {
	ch := newTestChannel(t)
	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "/unknown.Service/Method", req, resp)
	st, _ := status.FromError(err)
	if st.Code() != codes.Unimplemented {
		t.Errorf("got %v, want Unimplemented", st.Code())
	}
}

func TestChannel_Invoke_UnimplementedMethod(t *testing.T) {
	ch := newTestChannel(t)
	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "/test.TestService/UnknownMethod", req, resp)
	st, _ := status.FromError(err)
	if st.Code() != codes.Unimplemented {
		t.Errorf("got %v, want Unimplemented", st.Code())
	}
}

func TestChannel_Invoke_ContextCancel(t *testing.T) {
	ch := newBareChannel(t)
	blockDesc := testServiceDesc
	blockDesc.Methods = []grpc.MethodDesc{
		{
			MethodName: "Unary",
			Handler: func(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
				in := new(wrapperspb.StringValue)
				if err := dec(in); err != nil {
					return nil, err
				}
				<-ctx.Done()
				return nil, ctx.Err()
			},
		},
	}
	ch.RegisterService(&blockDesc, &echoServer{})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(ctx, "/test.TestService/Unary", req, resp)
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Canceled {
		t.Errorf("got %v, want Canceled", st.Code())
	}
}

func TestChannel_Invoke_Deadline(t *testing.T) {
	ch := newBareChannel(t)
	blockDesc := testServiceDesc
	blockDesc.Methods = []grpc.MethodDesc{
		{
			MethodName: "Unary",
			Handler: func(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
				in := new(wrapperspb.StringValue)
				if err := dec(in); err != nil {
					return nil, err
				}
				<-ctx.Done()
				return nil, ctx.Err()
			},
		},
	}
	ch.RegisterService(&blockDesc, &echoServer{})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(ctx, "/test.TestService/Unary", req, resp)
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.DeadlineExceeded {
		t.Errorf("got %v, want DeadlineExceeded", st.Code())
	}
}

func TestChannel_Invoke_MethodWithoutLeadingSlash(t *testing.T) {
	ch := newTestChannel(t)
	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "test.TestService/Unary", req, resp)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if resp.GetValue() != "echo: hello" {
		t.Errorf("got %q, want %q", resp.GetValue(), "echo: hello")
	}
}

func TestChannel_Invoke_MessageIsolation(t *testing.T) {
	ch := newTestChannel(t)
	req := &wrapperspb.StringValue{Value: "original"}
	resp := new(wrapperspb.StringValue)
	if err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp); err != nil {
		t.Fatal(err)
	}
	// Verify request wasn't mutated
	if req.GetValue() != "original" {
		t.Errorf("request was mutated: %q", req.GetValue())
	}
	// Verify we got a proper copy
	if !proto.Equal(resp, &wrapperspb.StringValue{Value: "echo: original"}) {
		t.Errorf("got %v", resp)
	}
}

func TestChannel_NewStream_ServerStream(t *testing.T) {
	ch := newTestChannel(t)
	ctx := context.Background()
	stream, err := ch.NewStream(ctx, &grpc.StreamDesc{
		StreamName:    "ServerStream",
		ServerStreams: true,
	}, "/test.TestService/ServerStream")
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SendMsg(&wrapperspb.StringValue{Value: "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	var messages []string
	for {
		resp := new(wrapperspb.StringValue)
		err := stream.RecvMsg(resp)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		messages = append(messages, resp.GetValue())
	}
	if len(messages) != 3 {
		t.Fatalf("got %d messages, want 3", len(messages))
	}
	for i, msg := range messages {
		expected := fmt.Sprintf("hello:%d", i)
		if msg != expected {
			t.Errorf("message %d: got %q, want %q", i, msg, expected)
		}
	}
}

func TestChannel_NewStream_ClientStream(t *testing.T) {
	ch := newTestChannel(t)
	stream, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		StreamName:    "ClientStream",
		ClientStreams: true,
	}, "/test.TestService/ClientStream")
	if err != nil {
		t.Fatal(err)
	}
	for i := range 5 {
		if err := stream.SendMsg(&wrapperspb.StringValue{
			Value: fmt.Sprintf("msg%d", i),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	resp := new(wrapperspb.StringValue)
	if err := stream.RecvMsg(resp); err != nil {
		t.Fatal(err)
	}
	if resp.GetValue() != "received 5 messages" {
		t.Errorf("got %q", resp.GetValue())
	}
}

func TestChannel_NewStream_BidiStream(t *testing.T) {
	ch := newTestChannel(t)
	stream, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		StreamName:    "BidiStream",
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}
	// Send and receive interleaved
	for i := range 3 {
		if err := stream.SendMsg(&wrapperspb.StringValue{
			Value: fmt.Sprintf("msg%d", i),
		}); err != nil {
			t.Fatal(err)
		}
		resp := new(wrapperspb.StringValue)
		if err := stream.RecvMsg(resp); err != nil {
			t.Fatal(err)
		}
		expected := fmt.Sprintf("bidi: msg%d", i)
		if resp.GetValue() != expected {
			t.Errorf("got %q, want %q", resp.GetValue(), expected)
		}
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	// Should get EOF
	resp := new(wrapperspb.StringValue)
	if err := stream.RecvMsg(resp); err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
}

func TestChannel_NewStream_UnimplementedService(t *testing.T) {
	ch := newTestChannel(t)
	_, err := ch.NewStream(context.Background(), &grpc.StreamDesc{},
		"/unknown.Service/Method")
	st, _ := status.FromError(err)
	if st.Code() != codes.Unimplemented {
		t.Errorf("got %v, want Unimplemented", st.Code())
	}
}

func TestChannel_NewStream_UnimplementedMethod(t *testing.T) {
	ch := newTestChannel(t)
	_, err := ch.NewStream(context.Background(), &grpc.StreamDesc{},
		"/test.TestService/UnknownStream")
	st, _ := status.FromError(err)
	if st.Code() != codes.Unimplemented {
		t.Errorf("got %v, want Unimplemented", st.Code())
	}
}

func TestChannel_GetServiceInfo(t *testing.T) {
	ch := newTestChannel(t)
	info := ch.GetServiceInfo()
	if info == nil {
		t.Fatal("nil service info")
	}
	si, ok := info["test.TestService"]
	if !ok {
		t.Fatal("test.TestService not found")
	}
	if len(si.Methods) != 4 { // 1 unary + 3 streaming
		t.Errorf("got %d methods, want 4", len(si.Methods))
	}
}

func TestChannel_GetServiceInfo_Empty(t *testing.T) {
	ch := newBareChannel(t)
	info := ch.GetServiceInfo()
	if info != nil {
		t.Errorf("expected nil, got %v", info)
	}
}

func TestChannel_RegisterService_Duplicate(t *testing.T) {
	ch := newBareChannel(t)
	ch.RegisterService(&testServiceDesc, &echoServer{})
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic")
		}
	}()
	ch.RegisterService(&testServiceDesc, &echoServer{}) // should panic
}

func TestChannel_WithCloner(t *testing.T) {
	copied := false
	ch := newTestChannel(t, inprocgrpc.WithCloner(inprocgrpc.CopyFunc(func(out, in any) error {
		copied = true
		return inprocgrpc.ProtoCloner{}.Copy(out, in)
	})))
	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	if err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp); err != nil {
		t.Fatal(err)
	}
	if !copied {
		t.Error("Copy was not called")
	}
}

func TestChannel_WithServerUnaryInterceptor(t *testing.T) {
	var intercepted bool
	ch := newTestChannel(t, inprocgrpc.WithServerUnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		intercepted = true
		return handler(ctx, req)
	}))
	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	if err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp); err != nil {
		t.Fatal(err)
	}
	if !intercepted {
		t.Error("interceptor not called")
	}
}

func TestChannel_WithServerStreamInterceptor(t *testing.T) {
	var intercepted bool
	ch := newTestChannel(t, inprocgrpc.WithServerStreamInterceptor(func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		intercepted = true
		return handler(srv, ss)
	}))
	stream, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
	}, "/test.TestService/ServerStream")
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SendMsg(&wrapperspb.StringValue{Value: "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	// Drain responses
	for {
		resp := new(wrapperspb.StringValue)
		if err := stream.RecvMsg(resp); err != nil {
			break
		}
	}
	if !intercepted {
		t.Error("interceptor not called")
	}
}

func TestClientContext(t *testing.T) {
	type contextKey string
	const key contextKey = "test-key"
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
				// Server context should NOT have client values
				if v := ctx.Value(key); v != nil {
					return nil, fmt.Errorf("server ctx has client value: %v", v)
				}
				// But ClientContext should give us the original
				clientCtx := inprocgrpc.ClientContext(ctx)
				if clientCtx == nil {
					return nil, errors.New("ClientContext returned nil")
				}
				if v, ok := clientCtx.Value(key).(string); !ok || v != "test-value" {
					return nil, fmt.Errorf("ClientContext missing value: %v", v)
				}
				return &wrapperspb.StringValue{Value: "ok"}, nil
			},
		},
	}
	ch.RegisterService(&desc, &echoServer{})
	ctx := context.WithValue(context.Background(), key, "test-value")
	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	if err := ch.Invoke(ctx, "/test.TestService/Unary", req, resp); err != nil {
		t.Fatal(err)
	}
	if resp.GetValue() != "ok" {
		t.Errorf("got %q", resp.GetValue())
	}
}

func TestClientContext_NotInProcess(t *testing.T) {
	ctx := context.Background()
	if c := inprocgrpc.ClientContext(ctx); c != nil {
		t.Errorf("expected nil, got %v", c)
	}
}

func TestChannel_ConcurrentRPCs(t *testing.T) {
	ch := newTestChannel(t)
	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := &wrapperspb.StringValue{Value: fmt.Sprintf("msg%d", i)}
			resp := new(wrapperspb.StringValue)
			if err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp); err != nil {
				errs <- fmt.Errorf("rpc %d: %w", i, err)
				return
			}
			expected := fmt.Sprintf("echo: msg%d", i)
			if resp.GetValue() != expected {
				errs <- fmt.Errorf("rpc %d: got %q, want %q", i, resp.GetValue(), expected)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestChannel_NewStream_ContextCancel(t *testing.T) {
	ch := newBareChannel(t)
	desc := testServiceDesc
	desc.Streams = []grpc.StreamDesc{
		{
			StreamName: "BidiStream",
			Handler: func(srv any, stream grpc.ServerStream) error {
				// Block until context is cancelled
				<-stream.Context().Done()
				return stream.Context().Err()
			},
			ServerStreams: true,
			ClientStreams: true,
		},
	}
	ch.RegisterService(&desc, &echoServer{})
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := ch.NewStream(ctx, &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	time.Sleep(10 * time.Millisecond) // Give goroutine time to notice
	resp := new(wrapperspb.StringValue)
	err = stream.RecvMsg(resp)
	if err == nil {
		t.Error("expected error after cancel")
	}
}

func TestChannel_NewStream_MetadataPropagation(t *testing.T) {
	ch := newBareChannel(t)
	desc := testServiceDesc
	desc.Streams = []grpc.StreamDesc{
		{
			StreamName: "ServerStream",
			Handler: func(srv any, stream grpc.ServerStream) error {
				// Read incoming metadata
				md, ok := metadata.FromIncomingContext(stream.Context())
				if !ok {
					return errors.New("no incoming metadata")
				}
				vals := md.Get("test-key")
				if len(vals) == 0 || vals[0] != "test-val" {
					return fmt.Errorf("missing metadata: %v", md)
				}
				// Send header and trailer
				if err := stream.SendHeader(metadata.Pairs("resp-header", "hval")); err != nil {
					return err
				}
				stream.SetTrailer(metadata.Pairs("resp-trailer", "tval"))
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
	ctx := metadata.NewOutgoingContext(context.Background(),
		metadata.Pairs("test-key", "test-val"))
	stream, err := ch.NewStream(ctx, &grpc.StreamDesc{
		ServerStreams: true,
	}, "/test.TestService/ServerStream")
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SendMsg(&wrapperspb.StringValue{Value: "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	// Check headers
	hdrs, err := stream.Header()
	if err != nil {
		t.Fatal(err)
	}
	if vals := hdrs.Get("resp-header"); len(vals) == 0 || vals[0] != "hval" {
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
	// Check trailers
	tlrs := stream.Trailer()
	if vals := tlrs.Get("resp-trailer"); len(vals) == 0 || vals[0] != "tval" {
		t.Errorf("trailers: %v", tlrs)
	}
}
