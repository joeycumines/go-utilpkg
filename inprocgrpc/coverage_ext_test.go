package inprocgrpc_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
)

// --- helpers ---

// failingCreds implements credentials.PerRPCCredentials and fails GetRequestMetadata.
type failingCreds struct {
	err error
}

func (c *failingCreds) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	return nil, c.err
}

func (c *failingCreds) RequireTransportSecurity() bool { return false }

var _ credentials.PerRPCCredentials = (*failingCreds)(nil)

// workingCreds returns metadata successfully.
type workingCreds struct {
	md map[string]string
}

func (c *workingCreds) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	return c.md, nil
}

func (c *workingCreds) RequireTransportSecurity() bool { return false }

var _ credentials.PerRPCCredentials = (*workingCreds)(nil)

// coverageServiceDesc builds a service desc with custom handlers per test.
func coverageServiceDesc(unaryHandler func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error), streams []grpc.StreamDesc) grpc.ServiceDesc {
	desc := grpc.ServiceDesc{
		ServiceName: "test.TestService",
		HandlerType: (*testServiceServer)(nil),
	}
	if unaryHandler != nil {
		desc.Methods = []grpc.MethodDesc{{
			MethodName: "Unary",
			Handler:    unaryHandler,
		}}
	}
	if streams != nil {
		desc.Streams = streams
	}
	return desc
}

// --- 1. Invoke with PerRPCCreds that fail GetRequestMetadata ---

func TestCoverage_Invoke_PerRPCCreds_FailGetMetadata(t *testing.T) {
	ch := newTestChannel(t)
	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp,
		grpc.PerRPCCredentials(&failingCreds{err: errors.New("creds broken")}),
	)
	if err == nil {
		t.Fatal("expected error from failing credentials")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %T: %v", err, err)
	}
	if st.Code() != codes.Unauthenticated {
		t.Errorf("got code %v, want Unauthenticated", st.Code())
	}
}

// --- 2. NewStream with PerRPCCreds ---

func TestCoverage_NewStream_PerRPCCreds_FailGetMetadata(t *testing.T) {
	ch := newTestChannel(t)
	_, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
	}, "/test.TestService/ServerStream",
		grpc.PerRPCCredentials(&failingCreds{err: errors.New("stream creds fail")}),
	)
	if err == nil {
		t.Fatal("expected error from failing credentials")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %T: %v", err, err)
	}
	if st.Code() != codes.Unauthenticated {
		t.Errorf("got code %v, want Unauthenticated", st.Code())
	}
}

func TestCoverage_NewStream_PerRPCCreds_Success(t *testing.T) {
	ch := newBareChannel(t)
	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			md, ok := metadata.FromIncomingContext(stream.Context())
			if !ok {
				return fmt.Errorf("no incoming metadata")
			}
			if vals := md.Get("auth-token"); len(vals) == 0 || vals[0] != "secret" {
				return fmt.Errorf("missing auth-token, got %v", md)
			}
			in := new(wrapperspb.StringValue)
			if err := stream.RecvMsg(in); err != nil {
				return err
			}
			return stream.SendMsg(&wrapperspb.StringValue{Value: "authed"})
		},
		ServerStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	stream, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
	}, "/test.TestService/ServerStream",
		grpc.PerRPCCredentials(&workingCreds{md: map[string]string{"auth-token": "secret"}}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SendMsg(&wrapperspb.StringValue{Value: "go"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	resp := new(wrapperspb.StringValue)
	if err := stream.RecvMsg(resp); err != nil {
		t.Fatal(err)
	}
	if resp.GetValue() != "authed" {
		t.Errorf("got %q", resp.GetValue())
	}
}

// --- 3. Streaming RPC where server calls grpc.SetTrailer ---

func TestCoverage_Stream_ServerSetTrailer(t *testing.T) {
	ch := newBareChannel(t)
	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			grpc.SetTrailer(stream.Context(), metadata.Pairs("svr-trailer", "tv"))
			in := new(wrapperspb.StringValue)
			if err := stream.RecvMsg(in); err != nil {
				return err
			}
			return stream.SendMsg(&wrapperspb.StringValue{Value: "done"})
		},
		ServerStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	var tlrs metadata.MD
	stream, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
	}, "/test.TestService/ServerStream",
		grpc.Trailer(&tlrs),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SendMsg(&wrapperspb.StringValue{Value: "trigger"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	resp := new(wrapperspb.StringValue)
	if err := stream.RecvMsg(resp); err != nil {
		t.Fatal(err)
	}
	// Drain EOF
	if err := stream.RecvMsg(new(wrapperspb.StringValue)); err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}
	if v := tlrs.Get("svr-trailer"); len(v) == 0 || v[0] != "tv" {
		t.Errorf("trailers: %v", tlrs)
	}
}

// --- 4. Unary RPC returning context error ---

func TestCoverage_Invoke_ServerReturnsDeadlineExceeded(t *testing.T) {
	ch := newBareChannel(t)
	desc := coverageServiceDesc(func(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
		in := new(wrapperspb.StringValue)
		if err := dec(in); err != nil {
			return nil, err
		}
		return nil, context.DeadlineExceeded
	}, nil)
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
	if st.Code() != codes.DeadlineExceeded {
		t.Errorf("got code %v, want DeadlineExceeded", st.Code())
	}
}

// --- 5. Server stream: sends data, SetTrailer, then returns error ---

func TestCoverage_Stream_FinishWithDataTrailersAndError(t *testing.T) {
	ch := newBareChannel(t)
	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			// Consume the client's request first
			if err := stream.RecvMsg(new(wrapperspb.StringValue)); err != nil {
				return err
			}
			// Send a data frame (implicitly sends headers)
			if err := stream.SendMsg(&wrapperspb.StringValue{Value: "data"}); err != nil {
				return err
			}
			// Set trailer
			stream.SetTrailer(metadata.Pairs("fin-trailer", "ftv"))
			// Return error
			return status.Error(codes.Aborted, "server aborted")
		},
		ServerStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	var tlrs metadata.MD
	stream, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
	}, "/test.TestService/ServerStream",
		grpc.Trailer(&tlrs),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SendMsg(&wrapperspb.StringValue{Value: "trigger"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}

	// Should get data frame
	resp := new(wrapperspb.StringValue)
	if err := stream.RecvMsg(resp); err != nil {
		t.Fatal(err)
	}
	if resp.GetValue() != "data" {
		t.Errorf("got %q", resp.GetValue())
	}

	// Next recv should get the error (trailers consumed implicitly)
	err = stream.RecvMsg(new(wrapperspb.StringValue))
	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %T: %v", err, err)
	}
	if st.Code() != codes.Aborted {
		t.Errorf("got code %v, want Aborted", st.Code())
	}
	if v := tlrs.Get("fin-trailer"); len(v) == 0 || v[0] != "ftv" {
		t.Errorf("trailers: %v", tlrs)
	}
}

// --- 6. Server stream: SendMsg on closed context ---

func TestCoverage_Stream_ServerSendMsg_CancelledContext(t *testing.T) {
	serverErr := make(chan error, 1)
	ch := newBareChannel(t)
	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "BidiStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			// Wait for context to be cancelled
			<-stream.Context().Done()
			// Try to send after cancel - should fail
			err := stream.SendMsg(&wrapperspb.StringValue{Value: "late"})
			serverErr <- err
			return stream.Context().Err()
		},
		ServerStreams: true,
		ClientStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := ch.NewStream(ctx, &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}
	_ = stream
	cancel()

	sErr := <-serverErr
	if sErr == nil {
		t.Error("expected error from SendMsg on cancelled context")
	}
}

// --- 7. Bidi stream: server sends >1 message, client expects unary response ---

func TestCoverage_Stream_EnsureNoMoreLocked_ServerSendsMultiple(t *testing.T) {
	ch := newBareChannel(t)
	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			in := new(wrapperspb.StringValue)
			if err := stream.RecvMsg(in); err != nil {
				return err
			}
			// Send two responses (client expects only one for non-streaming)
			if err := stream.SendMsg(&wrapperspb.StringValue{Value: "first"}); err != nil {
				return err
			}
			if err := stream.SendMsg(&wrapperspb.StringValue{Value: "second"}); err != nil {
				return err
			}
			return nil
		},
		ServerStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	// desc.ServerStreams=false to trigger ensureNoMoreLocked
	stream, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: false, // unary response expected
	}, "/test.TestService/ServerStream")
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SendMsg(&wrapperspb.StringValue{Value: "go"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	resp := new(wrapperspb.StringValue)
	err = stream.RecvMsg(resp)
	if err == nil {
		t.Fatal("expected error from >1 response")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %T: %v", err, err)
	}
	if st.Code() != codes.Internal {
		t.Errorf("got code %v, want Internal", st.Code())
	}
}

// --- 8. Header() called and server sends data without explicit headers ---

func TestCoverage_Stream_Header_NoExplicitHeaders(t *testing.T) {
	ch := newBareChannel(t)
	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			in := new(wrapperspb.StringValue)
			if err := stream.RecvMsg(in); err != nil {
				return err
			}
			// Send data without SendHeader - no explicit headers frame
			return stream.SendMsg(&wrapperspb.StringValue{Value: "data"})
		},
		ServerStreams: true,
	}})
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

	// Call Header() - server didn't send explicit headers, so we should get
	// nil headers but no error (the data frame is saved as last).
	hdrs, err := stream.Header()
	if err != nil {
		t.Fatalf("Header: %v", err)
	}
	if hdrs != nil {
		t.Errorf("expected nil headers, got %v", hdrs)
	}

	// RecvMsg should still work - uses the saved frame
	resp := new(wrapperspb.StringValue)
	if err := stream.RecvMsg(resp); err != nil {
		t.Fatalf("RecvMsg: %v", err)
	}
	if resp.GetValue() != "data" {
		t.Errorf("got %q", resp.GetValue())
	}

	// EOF
	if err := stream.RecvMsg(new(wrapperspb.StringValue)); err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
}

// --- 9. Invoke: server returns error AND sends trailers simultaneously ---

func TestCoverage_Invoke_ErrorWithTrailers(t *testing.T) {
	ch := newBareChannel(t)
	desc := coverageServiceDesc(func(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
		in := new(wrapperspb.StringValue)
		if err := dec(in); err != nil {
			return nil, err
		}
		grpc.SetTrailer(ctx, metadata.Pairs("err-trailer", "etv"))
		return nil, status.Error(codes.NotFound, "not found")
	}, nil)
	ch.RegisterService(&desc, &echoServer{})

	var tlrs metadata.MD
	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp,
		grpc.Trailer(&tlrs),
	)
	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %T: %v", err, err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("got code %v, want NotFound", st.Code())
	}
	if v := tlrs.Get("err-trailer"); len(v) == 0 || v[0] != "etv" {
		t.Errorf("trailers: %v", tlrs)
	}
}

// --- 10. Channel with both client AND server stats handlers ---

func TestCoverage_BothStatsHandlers(t *testing.T) {
	clientRec := &statsRecorder{}
	serverRec := &statsRecorder{}
	ch := newBareChannel(t, inprocgrpc.WithClientStatsHandler(clientRec), inprocgrpc.WithServerStatsHandler(serverRec))
	ch.RegisterService(&testServiceDesc, &echoServer{})

	// Unary RPC
	req := &wrapperspb.StringValue{Value: "stats-test"}
	resp := new(wrapperspb.StringValue)
	if err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp); err != nil {
		t.Fatal(err)
	}
	if resp.GetValue() != "echo: stats-test" {
		t.Errorf("got %q", resp.GetValue())
	}

	// Check client-side events
	clientEvents := waitStatsEnd(t, clientRec)
	assertHasEventTypes(t, "client", clientEvents,
		(*stats.Begin)(nil),
		(*stats.OutPayload)(nil),
		(*stats.InPayload)(nil),
		(*stats.End)(nil),
	)

	// Check server-side events
	serverEvents := waitStatsEnd(t, serverRec)
	assertHasEventTypes(t, "server", serverEvents,
		(*stats.Begin)(nil),
		(*stats.End)(nil),
	)

	// Check both recorders have tag events
	clientRec.mu.Lock()
	clientTags := len(clientRec.tags)
	clientRec.mu.Unlock()
	if clientTags == 0 {
		t.Error("client: no tags")
	}

	serverRec.mu.Lock()
	serverTags := len(serverRec.tags)
	serverRec.mu.Unlock()
	if serverTags == 0 {
		t.Error("server: no tags")
	}
}

func TestCoverage_BothStatsHandlers_Stream(t *testing.T) {
	clientRec := &statsRecorder{}
	serverRec := &statsRecorder{}
	ch := newBareChannel(t, inprocgrpc.WithClientStatsHandler(clientRec), inprocgrpc.WithServerStatsHandler(serverRec))

	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			if err := stream.SendHeader(metadata.Pairs("sh", "sv")); err != nil {
				return err
			}
			in := new(wrapperspb.StringValue)
			if err := stream.RecvMsg(in); err != nil {
				return err
			}
			stream.SetTrailer(metadata.Pairs("st", "stv"))
			return stream.SendMsg(&wrapperspb.StringValue{Value: "streamed"})
		},
		ServerStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	stream, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
	}, "/test.TestService/ServerStream")
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SendMsg(&wrapperspb.StringValue{Value: "hi"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}

	if _, err := stream.Header(); err != nil {
		t.Fatal(err)
	}

	resp := new(wrapperspb.StringValue)
	if err := stream.RecvMsg(resp); err != nil {
		t.Fatal(err)
	}
	// EOF
	if err := stream.RecvMsg(new(wrapperspb.StringValue)); err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}

	// Verify client stats include InHeader, InPayload, InTrailer
	clientEvents := waitStatsEnd(t, clientRec)
	assertHasEventTypes(t, "client-stream", clientEvents,
		(*stats.Begin)(nil),
		(*stats.OutPayload)(nil),
		(*stats.InHeader)(nil),
		(*stats.InPayload)(nil),
		(*stats.InTrailer)(nil),
		(*stats.End)(nil),
	)

	// Verify server stats include OutHeader, OutPayload, OutTrailer, InPayload
	serverEvents := waitStatsEnd(t, serverRec)
	assertHasEventTypes(t, "server-stream", serverEvents,
		(*stats.Begin)(nil),
		(*stats.OutHeader)(nil),
		(*stats.InPayload)(nil),
		(*stats.OutPayload)(nil),
		(*stats.OutTrailer)(nil),
		(*stats.End)(nil),
	)
}

// assertHasEventTypes checks that events contain at least one instance of each expected type.
func assertHasEventTypes(t *testing.T, prefix string, events []stats.RPCStats, expectedTypes ...stats.RPCStats) {
	t.Helper()
	for _, expected := range expectedTypes {
		typeName := fmt.Sprintf("%T", expected)
		found := false
		for _, ev := range events {
			if fmt.Sprintf("%T", ev) == typeName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: missing event type %s (have: %v)", prefix, typeName, eventTypeNames(events))
		}
	}
}

func eventTypeNames(events []stats.RPCStats) []string {
	names := make([]string, len(events))
	for i, ev := range events {
		names[i] = fmt.Sprintf("%T", ev)
	}
	return names
}
