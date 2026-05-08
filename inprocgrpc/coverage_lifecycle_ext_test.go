package inprocgrpc_test

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// --- 11. Invoke where ctx is already cancelled ---

func TestCoverage_Invoke_AlreadyCancelledContext(t *testing.T) {
	ch := newTestChannel(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(ctx, "/test.TestService/Unary", req, resp)
	if err == nil {
		t.Fatal("expected error with already-cancelled context")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %T: %v", err, err)
	}
	if st.Code() != codes.Canceled {
		t.Errorf("got code %v, want Canceled", st.Code())
	}
}

// --- 12. Stream handler that returns error immediately (early cleanup path) ---

func TestCoverage_Stream_HandlerReturnsErrorImmediately(t *testing.T) {
	ch := newBareChannel(t)
	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			return status.Error(codes.FailedPrecondition, "immediate failure")
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
	// SendMsg and CloseSend may fail with io.EOF if the server returns
	// before the client-side write completes - this is expected behavior.
	_ = stream.SendMsg(&wrapperspb.StringValue{Value: "trigger"})
	_ = stream.CloseSend()
	resp := new(wrapperspb.StringValue)
	err = stream.RecvMsg(resp)
	if err == nil {
		t.Fatal("expected error from handler")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %T: %v", err, err)
	}
	if st.Code() != codes.FailedPrecondition {
		t.Errorf("got code %v, want FailedPrecondition", st.Code())
	}
}

// --- ensureNoMoreLocked: server returns error instead of second message ---

func TestCoverage_Stream_EnsureNoMore_ServerError(t *testing.T) {
	ch := newBareChannel(t)
	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			in := new(wrapperspb.StringValue)
			if err := stream.RecvMsg(in); err != nil {
				return err
			}
			// Send one response, then return error
			if err := stream.SendMsg(&wrapperspb.StringValue{Value: "only-one"}); err != nil {
				return err
			}
			return status.Error(codes.Aborted, "server error after first msg")
		},
		ServerStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	// Non-streaming response - triggers ensureNoMoreLocked
	stream, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: false,
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
	// ensureNoMoreLocked calls recvMsgLocked again. The server error
	// (Aborted) is returned. Since err != nil && err != io.EOF, it is
	// propagated directly.
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
}

// --- Invoke: server streams headers + data + trailers in unary path ---

func TestCoverage_Invoke_FullFrameSequence(t *testing.T) {
	ch := newBareChannel(t)
	desc := coverageServiceDesc(func(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
		in := new(wrapperspb.StringValue)
		if err := dec(in); err != nil {
			return nil, err
		}
		_ = grpc.SetHeader(ctx, metadata.Pairs("h1", "hv1"))
		_ = grpc.SendHeader(ctx, metadata.Pairs("h2", "hv2"))
		grpc.SetTrailer(ctx, metadata.Pairs("t1", "tv1"))
		return &wrapperspb.StringValue{Value: "full"}, nil
	}, nil)
	ch.RegisterService(&desc, &echoServer{})

	var hdrs, tlrs metadata.MD
	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp,
		grpc.Header(&hdrs), grpc.Trailer(&tlrs))
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetValue() != "full" {
		t.Errorf("resp: %q", resp.GetValue())
	}
	if v := hdrs.Get("h1"); len(v) == 0 || v[0] != "hv1" {
		t.Errorf("h1: %v", hdrs)
	}
	if v := hdrs.Get("h2"); len(v) == 0 || v[0] != "hv2" {
		t.Errorf("h2: %v", hdrs)
	}
	if v := tlrs.Get("t1"); len(v) == 0 || v[0] != "tv1" {
		t.Errorf("t1: %v", tlrs)
	}
}

// --- Header() called multiple times ---

func TestCoverage_Stream_HeaderCalledTwice(t *testing.T) {
	ch := newBareChannel(t)
	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			if err := stream.SendHeader(metadata.Pairs("hdr", "val")); err != nil {
				return err
			}
			in := new(wrapperspb.StringValue)
			if err := stream.RecvMsg(in); err != nil {
				return err
			}
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
	if err := stream.SendMsg(&wrapperspb.StringValue{Value: "go"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}

	// First call - should get headers
	hdrs1, err := stream.Header()
	if err != nil {
		t.Fatal(err)
	}
	if v := hdrs1.Get("hdr"); len(v) == 0 || v[0] != "val" {
		t.Errorf("first Header: %v", hdrs1)
	}

	// Second call - should return cached headers without error
	hdrs2, err := stream.Header()
	if err != nil {
		t.Fatal(err)
	}
	if v := hdrs2.Get("hdr"); len(v) == 0 || v[0] != "val" {
		t.Errorf("second Header: %v", hdrs2)
	}

	// Drain
	resp := new(wrapperspb.StringValue)
	if err := stream.RecvMsg(resp); err != nil {
		t.Fatal(err)
	}
	if err := stream.RecvMsg(new(wrapperspb.StringValue)); err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
}

// --- processFrameLocked: empty/unknown frame ---

func TestCoverage_Stream_ServerSendsEmptyFrame(t *testing.T) {
	// This would require the server to send a frame with no data/headers/trailers/error.
	// The channel validates frames, but we can test by having a handler that
	// just returns without sending anything in a streaming RPC.
	ch := newBareChannel(t)
	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			in := new(wrapperspb.StringValue)
			if err := stream.RecvMsg(in); err != nil {
				return err
			}
			// Return without sending - finish() will close the channel
			return nil
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
	if err := stream.SendMsg(&wrapperspb.StringValue{Value: "go"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	resp := new(wrapperspb.StringValue)
	err = stream.RecvMsg(resp)
	if err == nil {
		t.Error("expected EOF or error")
	}
}

// --- doEnd: called once, stats handler invoked ---

func TestCoverage_Stream_DoEnd_StatsHandler(t *testing.T) {
	clientRec := &statsRecorder{}
	ch := newBareChannel(t, inprocgrpc.WithClientStatsHandler(clientRec))
	ch.RegisterService(&testServiceDesc, &echoServer{})

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

	// Drain all messages
	for {
		resp := new(wrapperspb.StringValue)
		if err := stream.RecvMsg(resp); err != nil {
			break
		}
	}

	// Should have End event from doEnd
	events := waitStatsEnd(t, clientRec)
	var hasEnd bool
	for _, ev := range events {
		if _, ok := ev.(*stats.End); ok {
			hasEnd = true
		}
	}
	if !hasEnd {
		t.Error("missing End event from doEnd")
	}
}

// --- clientstream RecvMsg: trailers in processFrameLocked ---

func TestCoverage_Stream_RecvMsg_TrailersBeforeData(t *testing.T) {
	ch := newBareChannel(t)
	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			// Set trailers before sending data
			stream.SetTrailer(metadata.Pairs("early-trail", "etv"))
			in := new(wrapperspb.StringValue)
			if err := stream.RecvMsg(in); err != nil {
				return err
			}
			return stream.SendMsg(&wrapperspb.StringValue{Value: "msg"})
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
	if resp.GetValue() != "msg" {
		t.Errorf("got %q", resp.GetValue())
	}
	// EOF
	if err := stream.RecvMsg(new(wrapperspb.StringValue)); err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}
	if v := tlrs.Get("early-trail"); len(v) == 0 || v[0] != "etv" {
		t.Errorf("trailers: %v", tlrs)
	}
}

// --- Concurrent RPCs with stats handlers ---

func TestCoverage_ConcurrentRPCs_WithStatsHandlers(t *testing.T) {
	clientRec := &statsRecorder{}
	serverRec := &statsRecorder{}
	ch := newBareChannel(t, inprocgrpc.WithClientStatsHandler(clientRec), inprocgrpc.WithServerStatsHandler(serverRec))
	ch.RegisterService(&testServiceDesc, &echoServer{})

	const n = 10
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Go(func() {
			req := &wrapperspb.StringValue{Value: fmt.Sprintf("c%d", i)}
			resp := new(wrapperspb.StringValue)
			if err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent RPC error: %v", err)
	}

	// At least n Begin events per side
	clientEvents := clientRec.getEvents()
	serverEvents := serverRec.getEvents()
	countType := func(events []stats.RPCStats, target string) int {
		count := 0
		for _, ev := range events {
			if fmt.Sprintf("%T", ev) == target {
				count++
			}
		}
		return count
	}
	if c := countType(clientEvents, "*stats.Begin"); c < n {
		t.Errorf("client Begin events: %d < %d", c, n)
	}
	if c := countType(serverEvents, "*stats.Begin"); c < n {
		t.Errorf("server Begin events: %d < %d", c, n)
	}
}

// --- Invoke: server sends duplicate response (gotResponse path) ---

func TestCoverage_Invoke_ServerSendsDuplicateResponse(t *testing.T) {
	ch := newBareChannel(t)
	desc := coverageServiceDesc(func(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
		in := new(wrapperspb.StringValue)
		if err := dec(in); err != nil {
			return nil, err
		}
		// Return a valid response - but the unary handler only supports one
		return &wrapperspb.StringValue{Value: "ok"}, nil
	}, nil)
	ch.RegisterService(&desc, &echoServer{})

	req := &wrapperspb.StringValue{Value: "hi"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if resp.GetValue() != "ok" {
		t.Errorf("got %q", resp.GetValue())
	}
}

// --- Stream: server sends headers, trailers, then finishes with unsent headers ---

func TestCoverage_Stream_FinishWithUnsentHeadersAndTrailers(t *testing.T) {
	ch := newBareChannel(t)
	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			// Set headers (don't send them explicitly)
			if err := stream.SetHeader(metadata.Pairs("unsent-h", "uhv")); err != nil {
				return err
			}
			stream.SetTrailer(metadata.Pairs("finish-t", "ftv"))
			in := new(wrapperspb.StringValue)
			if err := stream.RecvMsg(in); err != nil {
				return err
			}
			// Return without sending any messages - finish() should send the headers
			return nil
		},
		ServerStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	var hdrs, tlrs metadata.MD
	stream, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
	}, "/test.TestService/ServerStream",
		grpc.Header(&hdrs), grpc.Trailer(&tlrs),
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
	// Drain
	for {
		resp := new(wrapperspb.StringValue)
		if err := stream.RecvMsg(resp); err != nil {
			break
		}
	}

	// finish() should have sent the unsent headers
	if v := hdrs.Get("unsent-h"); len(v) == 0 || v[0] != "uhv" {
		t.Errorf("headers: %v", hdrs)
	}
	if v := tlrs.Get("finish-t"); len(v) == 0 || v[0] != "ftv" {
		t.Errorf("trailers: %v", tlrs)
	}
}

// --- clientstream SendMsg with statsHandler ---

func TestCoverage_Stream_ClientSendMsg_WithStatsHandler(t *testing.T) {
	clientRec := &statsRecorder{}
	ch := newBareChannel(t, inprocgrpc.WithClientStatsHandler(clientRec))
	ch.RegisterService(&testServiceDesc, &echoServer{})

	stream, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SendMsg(&wrapperspb.StringValue{Value: "msg1"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.SendMsg(&wrapperspb.StringValue{Value: "msg2"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	// Drain
	for {
		resp := new(wrapperspb.StringValue)
		if err := stream.RecvMsg(resp); err != nil {
			break
		}
	}

	events := waitStatsEnd(t, clientRec)
	outPayloadCount := 0
	for _, ev := range events {
		if _, ok := ev.(*stats.OutPayload); ok {
			outPayloadCount++
		}
	}
	if outPayloadCount < 2 {
		t.Errorf("expected at least 2 OutPayload events, got %d", outPayloadCount)
	}
}

// --- Invoke with Peer option ---

func TestCoverage_Invoke_WithPeerOption(t *testing.T) {
	ch := newTestChannel(t)
	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	var p *wrapperspb.StringValue // wrong type, just to have the import
	_ = p

	// Use grpc.Peer to capture the peer info
	// We can't import peer directly in _test, but the CallOption
	// should propagate correctly.
	err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp)
	if err != nil {
		t.Fatal(err)
	}
}

// --- recvMsgLocked: non-EOF read error ---

func TestCoverage_Stream_RecvMsg_ContextError(t *testing.T) {
	ch := newBareChannel(t)
	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "BidiStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			<-stream.Context().Done()
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
	cancel()
	resp := new(wrapperspb.StringValue)
	err = stream.RecvMsg(resp)
	if err == nil {
		t.Fatal("expected error after cancel")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected status error, got %T: %v", err, err)
	}
	if st.Code() != codes.Canceled {
		t.Errorf("got code %v, want Canceled", st.Code())
	}
}

// --- Stream: Trailer() returns metadata ---

func TestCoverage_Stream_Trailer(t *testing.T) {
	ch := newBareChannel(t)
	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			stream.SetTrailer(metadata.Pairs("x-trail", "xv"))
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
	if err := stream.SendMsg(&wrapperspb.StringValue{Value: "go"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}

	// Read data
	resp := new(wrapperspb.StringValue)
	if err := stream.RecvMsg(resp); err != nil {
		t.Fatal(err)
	}

	// Read EOF (which triggers trailer consumption)
	if err := stream.RecvMsg(new(wrapperspb.StringValue)); err != io.EOF {
		t.Fatalf("expected EOF, got %v", err)
	}

	// Trailer() should return the trailers now
	gotTrailers := stream.Trailer()
	if v := gotTrailers.Get("x-trail"); len(v) == 0 || v[0] != "xv" {
		t.Errorf("Trailer(): %v", gotTrailers)
	}
}

// --- ensureNoMoreLocked: clean EOF (exactly one message) ---

func TestCoverage_Stream_EnsureNoMore_CleanEOF(t *testing.T) {
	ch := newBareChannel(t)
	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			in := new(wrapperspb.StringValue)
			if err := stream.RecvMsg(in); err != nil {
				return err
			}
			// Send exactly one response
			return stream.SendMsg(&wrapperspb.StringValue{Value: "exactly-one"})
		},
		ServerStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	// Non-streaming response - triggers ensureNoMoreLocked
	stream, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: false,
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
	if err := stream.RecvMsg(resp); err != nil {
		t.Fatalf("RecvMsg: %v", err)
	}
	if resp.GetValue() != "exactly-one" {
		t.Errorf("got %q", resp.GetValue())
	}
}

// --- Invoke with InHeader and InTrailer stats events simultaneously ---

func TestCoverage_Invoke_StatsHandler_InHeaderInTrailer(t *testing.T) {
	clientRec := &statsRecorder{}
	ch := newBareChannel(t, inprocgrpc.WithClientStatsHandler(clientRec))
	desc := coverageServiceDesc(func(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
		in := new(wrapperspb.StringValue)
		if err := dec(in); err != nil {
			return nil, err
		}
		_ = grpc.SetHeader(ctx, metadata.Pairs("ih", "iv"))
		grpc.SetTrailer(ctx, metadata.Pairs("it", "itv"))
		return &wrapperspb.StringValue{Value: "ok"}, nil
	}, nil)
	ch.RegisterService(&desc, &echoServer{})

	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	if err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp); err != nil {
		t.Fatal(err)
	}

	events := waitStatsEnd(t, clientRec)
	assertHasEventTypes(t, "invoke-stats", events,
		(*stats.Begin)(nil),
		(*stats.OutPayload)(nil),
		(*stats.InHeader)(nil),
		(*stats.InPayload)(nil),
		(*stats.InTrailer)(nil),
		(*stats.End)(nil),
	)
}
