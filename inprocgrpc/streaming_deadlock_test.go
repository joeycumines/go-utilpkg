package inprocgrpc_test

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
)

// onRecv fires before delegating, so it marks the wrapper being entered,
// not the underlying receive being registered.
type hookServerStream struct {
	grpc.ServerStream
	onRecv func()
}

func (s *hookServerStream) RecvMsg(m any) error {
	if s.onRecv != nil {
		s.onRecv()
	}
	return s.ServerStream.RecvMsg(m)
}

type bidiDeadlockServer struct {
	loop     *eventloop.Loop
	recvDone chan error
}

func (s *bidiDeadlockServer) Bidi(rawStream grpc.ServerStream) error {
	recvEntered := make(chan struct{})
	loopBarrier := make(chan struct{})
	stream := &hookServerStream{
		ServerStream: rawStream,
		onRecv: func() {
			close(recvEntered)
		},
	}
	go func() {
		in := new(wrapperspb.StringValue)
		s.recvDone <- stream.RecvMsg(in)
	}()

	<-recvEntered

	// Queued behind any receive the child already registered, so returning
	// proves RecvMsg registered on the loop.
	if err := s.loop.Submit(func() {
		close(loopBarrier)
	}); err != nil {
		return err
	}
	<-loopBarrier

	return nil
}

func TestBidiStream_ConcurrentRecvOnHandlerReturn_Deadlock(t *testing.T) {
	loop := newTestLoop(t)
	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))

	srv := &bidiDeadlockServer{
		loop:     loop,
		recvDone: make(chan error, 1),
	}

	desc := grpc.ServiceDesc{
		ServiceName: "test.BidiService",
		HandlerType: (*any)(nil),
		Streams: []grpc.StreamDesc{
			{
				StreamName: "Bidi",
				Handler: func(srv any, stream grpc.ServerStream) error {
					return srv.(*bidiDeadlockServer).Bidi(stream)
				},
				ServerStreams: true,
				ClientStreams: true,
			},
		},
	}
	ch.RegisterService(&desc, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	clientStream, err := ch.NewStream(ctx, &desc.Streams[0], "/test.BidiService/Bidi")
	if err != nil {
		t.Fatalf("NewStream failed: %v", err)
	}

	select {
	case err := <-srv.recvDone:
		if err != io.EOF {
			t.Fatalf("expected io.EOF from server RecvMsg, got: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("deadlock detected: server RecvMsg never completed within timeout: %v", ctx.Err())
	}

	resp := new(wrapperspb.StringValue)
	if err := clientStream.RecvMsg(resp); err != io.EOF {
		t.Fatalf("expected io.EOF on client RecvMsg, got: %v", err)
	}
}

type clientStreamDeadlockServer struct {
	loop     *eventloop.Loop
	recvDone chan error
}

func (s *clientStreamDeadlockServer) ClientStream(rawStream grpc.ServerStream) error {
	recvEntered := make(chan struct{})
	loopBarrier := make(chan struct{})
	stream := &hookServerStream{
		ServerStream: rawStream,
		onRecv: func() {
			close(recvEntered)
		},
	}
	go func() {
		in := new(wrapperspb.StringValue)
		s.recvDone <- stream.RecvMsg(in)
	}()

	<-recvEntered

	if err := s.loop.Submit(func() {
		close(loopBarrier)
	}); err != nil {
		return err
	}
	<-loopBarrier

	return stream.SendMsg(&wrapperspb.StringValue{Value: "done"})
}

func TestClientStream_ConcurrentRecvOnHandlerReturn_Deadlock(t *testing.T) {
	loop := newTestLoop(t)
	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))

	srv := &clientStreamDeadlockServer{
		loop:     loop,
		recvDone: make(chan error, 1),
	}

	desc := grpc.ServiceDesc{
		ServiceName: "test.ClientStreamService",
		HandlerType: (*any)(nil),
		Streams: []grpc.StreamDesc{
			{
				StreamName: "ClientStream",
				Handler: func(srv any, stream grpc.ServerStream) error {
					return srv.(*clientStreamDeadlockServer).ClientStream(stream)
				},
				ServerStreams: false,
				ClientStreams: true,
			},
		},
	}
	ch.RegisterService(&desc, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	clientStream, err := ch.NewStream(ctx, &desc.Streams[0], "/test.ClientStreamService/ClientStream")
	if err != nil {
		t.Fatalf("NewStream failed: %v", err)
	}

	select {
	case err := <-srv.recvDone:
		if err != io.EOF {
			t.Fatalf("expected io.EOF from server RecvMsg, got: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("deadlock detected: server RecvMsg never completed within timeout: %v", ctx.Err())
	}

	resp := new(wrapperspb.StringValue)
	if err := clientStream.RecvMsg(resp); err != nil {
		t.Fatalf("expected response message, got: %v", err)
	}
	if resp.GetValue() != "done" {
		t.Fatalf("unexpected response value: %q", resp.GetValue())
	}

	if err := clientStream.RecvMsg(resp); err != io.EOF {
		t.Fatalf("expected io.EOF after response message, got: %v", err)
	}
}

func TestServerStream_UnconsumedRequest_CardinalityError(t *testing.T) {
	loop := newTestLoop(t)
	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))

	desc := grpc.ServiceDesc{
		ServiceName: "test.ServerStreamCardinalityService",
		HandlerType: (*any)(nil),
		Streams: []grpc.StreamDesc{
			{
				StreamName: "ServerStream",
				Handler: func(srv any, stream grpc.ServerStream) error {
					return nil
				},
				ServerStreams: true,
				ClientStreams: false,
			},
		},
	}
	ch.RegisterService(&desc, struct{}{})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	clientStream, err := ch.NewStream(ctx, &desc.Streams[0], "/test.ServerStreamCardinalityService/ServerStream")
	if err != nil {
		t.Fatalf("NewStream failed: %v", err)
	}

	// The handler may finish before this send is attempted, in which case
	// io.EOF is the correct gRPC result.
	if err := clientStream.SendMsg(&wrapperspb.StringValue{Value: "req"}); err != nil &&
		err != io.EOF {
		t.Fatalf("SendMsg failed: %v", err)
	}
	_ = clientStream.CloseSend()

	resp := new(wrapperspb.StringValue)
	err = clientStream.RecvMsg(resp)
	if err == nil {
		t.Fatalf("expected cardinality error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Internal {
		t.Fatalf("expected Internal error, got: %v", err)
	}
	if !strings.Contains(st.Message(), "method must consume exactly one request message") {
		t.Fatalf("unexpected error message: %q", st.Message())
	}
}

func TestClientStream_UnsentResponse_CardinalityError(t *testing.T) {
	loop := newTestLoop(t)
	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))

	desc := grpc.ServiceDesc{
		ServiceName: "test.ClientStreamCardinalityService",
		HandlerType: (*any)(nil),
		Streams: []grpc.StreamDesc{
			{
				StreamName: "ClientStream",
				Handler: func(srv any, stream grpc.ServerStream) error {
					for {
						in := new(wrapperspb.StringValue)
						if err := stream.RecvMsg(in); err != nil {
							if err == io.EOF {
								break
							}
							return err
						}
					}
					return nil
				},
				ServerStreams: false,
				ClientStreams: true,
			},
		},
	}
	ch.RegisterService(&desc, struct{}{})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	clientStream, err := ch.NewStream(ctx, &desc.Streams[0], "/test.ClientStreamCardinalityService/ClientStream")
	if err != nil {
		t.Fatalf("NewStream failed: %v", err)
	}

	if err := clientStream.SendMsg(&wrapperspb.StringValue{Value: "msg1"}); err != nil {
		t.Fatalf("SendMsg failed: %v", err)
	}
	_ = clientStream.CloseSend()

	resp := new(wrapperspb.StringValue)
	err = clientStream.RecvMsg(resp)
	if err == nil {
		t.Fatalf("expected cardinality error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Internal {
		t.Fatalf("expected Internal error, got: %v", err)
	}
	if !strings.Contains(st.Message(), "method must return exactly one response message") {
		t.Fatalf("unexpected error message: %q", st.Message())
	}
}

// The handler sends exactly one response, so it is valid and either outcome is
// correct: the message reaches the client, or the terminal claims first and the
// send is refused. The invariant is exclusivity — never both, and never neither.
func TestClientStream_NoResponseAfterCardinalityFailure(t *testing.T) {
	loop := newTestLoop(t)
	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))

	sendDone := make(chan error, 1)
	desc := grpc.ServiceDesc{
		ServiceName: "test.NoResponseAfterFailureService",
		HandlerType: (*any)(nil),
		Streams: []grpc.StreamDesc{
			{
				StreamName: "ClientStream",
				Handler: func(srv any, rawStream grpc.ServerStream) error {
					for {
						in := new(wrapperspb.StringValue)
						if err := rawStream.RecvMsg(in); err != nil {
							if err == io.EOF {
								break
							}
							return err
						}
					}

					go func() {
						sendDone <- rawStream.SendMsg(
							&wrapperspb.StringValue{Value: "late"},
						)
					}()

					return nil
				},
				ServerStreams: false,
				ClientStreams: true,
			},
		},
	}
	ch.RegisterService(&desc, struct{}{})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	clientStream, err := ch.NewStream(ctx, &desc.Streams[0], "/test.NoResponseAfterFailureService/ClientStream")
	if err != nil {
		t.Fatalf("NewStream failed: %v", err)
	}

	if err := clientStream.SendMsg(&wrapperspb.StringValue{Value: "req"}); err != nil {
		t.Fatalf("SendMsg failed: %v", err)
	}
	_ = clientStream.CloseSend()

	select {
	case <-sendDone:
	case <-ctx.Done():
		t.Fatalf("in-flight send never completed: %v", ctx.Err())
	}

	resp := new(wrapperspb.StringValue)
	err = clientStream.RecvMsg(resp)
	if err == nil {
		if resp.GetValue() != "late" {
			t.Fatalf("unexpected message: %q", resp.GetValue())
		}
		// The message consumed the unary response; the stream must then end.
		if endErr := clientStream.RecvMsg(resp); endErr != io.EOF {
			t.Fatalf("expected io.EOF after the message, got: %v", endErr)
		}
		return
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Internal {
		t.Fatalf("expected Internal error, got: %v", err)
	}
	if !strings.Contains(st.Message(), "method must return exactly one response message") {
		t.Fatalf("unexpected error message: %q", st.Message())
	}
	if resp.GetValue() == "late" {
		t.Fatal("client received the message alongside the cardinality error")
	}
}

// Same exclusivity contract as the test above: exactly one response means the
// handler is valid, so the client sees the message or the status, never both.
func TestClientStream_InFlightSendOnHandlerReturn_CardinalityError(t *testing.T) {
	loop := newTestLoop(t)
	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))

	loopBlocked := make(chan struct{})
	unblockLoop := make(chan struct{})
	sendDone := make(chan error, 1)

	desc := grpc.ServiceDesc{
		ServiceName: "test.ClientStreamInFlightService",
		HandlerType: (*any)(nil),
		Streams: []grpc.StreamDesc{
			{
				StreamName: "ClientStream",
				Handler: func(srv any, rawStream grpc.ServerStream) error {
					for {
						in := new(wrapperspb.StringValue)
						if err := rawStream.RecvMsg(in); err != nil {
							if err == io.EOF {
								break
							}
							return err
						}
					}

					// Hold the loop so the send cannot run first.
					if err := loop.Submit(func() {
						close(loopBlocked)
						<-unblockLoop
					}); err != nil {
						return err
					}
					<-loopBlocked

					go func() {
						sendDone <- rawStream.SendMsg(
							&wrapperspb.StringValue{Value: "in-flight"},
						)
					}()

					// Return without a completed response message.
					return nil
				},
				ServerStreams: false,
				ClientStreams: true,
			},
		},
	}
	ch.RegisterService(&desc, struct{}{})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	clientStream, err := ch.NewStream(ctx, &desc.Streams[0], "/test.ClientStreamInFlightService/ClientStream")
	if err != nil {
		t.Fatalf("NewStream failed: %v", err)
	}

	if err := clientStream.SendMsg(&wrapperspb.StringValue{Value: "req"}); err != nil {
		t.Fatalf("SendMsg failed: %v", err)
	}
	_ = clientStream.CloseSend()

	// Wait for the handler's loop task so the send stays queued behind it.
	select {
	case <-loopBlocked:
	case <-ctx.Done():
		t.Fatalf("handler never reached the loop barrier: %v", ctx.Err())
	}
	close(unblockLoop)

	select {
	case <-sendDone:
	case <-ctx.Done():
		t.Fatalf("in-flight send never completed: %v", ctx.Err())
	}

	resp := new(wrapperspb.StringValue)
	err = clientStream.RecvMsg(resp)
	if err == nil {
		if resp.GetValue() != "in-flight" {
			t.Fatalf("unexpected message: %q", resp.GetValue())
		}
		if endErr := clientStream.RecvMsg(resp); endErr != io.EOF {
			t.Fatalf("expected io.EOF after the message, got: %v", endErr)
		}
		return
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Internal {
		t.Fatalf("expected Internal error, got: %v", err)
	}
	if !strings.Contains(st.Message(), "method must return exactly one response message") {
		t.Fatalf("unexpected error message: %q", st.Message())
	}
	if resp.GetValue() == "in-flight" {
		t.Fatal("client received the message alongside the cardinality error")
	}
}

// The second send aborts the RPC, so the client sees neither message.
func TestClientStream_SecondResponseRejected(t *testing.T) {
	loop := newTestLoop(t)
	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))

	sends := make(chan [2]error, 1)
	desc := grpc.ServiceDesc{
		ServiceName: "test.SecondResponseService",
		HandlerType: (*any)(nil),
		Streams: []grpc.StreamDesc{
			{
				StreamName: "ClientStream",
				Handler: func(srv any, rawStream grpc.ServerStream) error {
					for {
						in := new(wrapperspb.StringValue)
						if err := rawStream.RecvMsg(in); err != nil {
							if err == io.EOF {
								break
							}
							return err
						}
					}
					first := rawStream.SendMsg(
						&wrapperspb.StringValue{Value: "first"},
					)
					second := rawStream.SendMsg(
						&wrapperspb.StringValue{Value: "second"},
					)
					sends <- [2]error{first, second}
					return nil
				},
				ServerStreams: false,
				ClientStreams: true,
			},
		},
	}
	ch.RegisterService(&desc, struct{}{})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	clientStream, err := ch.NewStream(ctx, &desc.Streams[0], "/test.SecondResponseService/ClientStream")
	if err != nil {
		t.Fatalf("NewStream failed: %v", err)
	}
	if err := clientStream.SendMsg(&wrapperspb.StringValue{Value: "req"}); err != nil {
		t.Fatalf("SendMsg failed: %v", err)
	}
	_ = clientStream.CloseSend()

	var results [2]error
	select {
	case results = <-sends:
	case <-ctx.Done():
		t.Fatalf("handler never reported its sends: %v", ctx.Err())
	}
	if results[0] != nil {
		t.Fatalf("first send failed: %v", results[0])
	}
	if results[1] == nil {
		t.Fatalf("second send succeeded on a unary-response method")
	}
	if !strings.Contains(
		status.Convert(results[1]).Message(),
		"method returned more than one response message",
	) {
		t.Fatalf("unexpected second send error: %v", results[1])
	}

	resp := new(wrapperspb.StringValue)
	err = clientStream.RecvMsg(resp)
	if err == nil {
		t.Fatalf("client received a message from an aborted RPC: %q",
			resp.GetValue())
	}
	if !strings.Contains(
		status.Convert(err).Message(),
		"more than one",
	) {
		t.Fatalf("expected the cardinality error, got: %v", err)
	}
}

// A streaming RPC whose handler sends no metadata must leave the client's
// headers and trailers nil, not empty-but-non-nil: SetHeaders/SetTrailers
// join into the state, so passing nil would manufacture an empty MD.
func TestStream_NoMetadata_LeavesNilHeaderAndTrailer(t *testing.T) {
	loop := newTestLoop(t)
	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))

	desc := grpc.ServiceDesc{
		ServiceName: "test.NoMetadata",
		HandlerType: (*any)(nil),
		Streams: []grpc.StreamDesc{{
			StreamName: "ServerStream",
			Handler: func(srv any, stream grpc.ServerStream) error {
				in := new(wrapperspb.StringValue)
				if err := stream.RecvMsg(in); err != nil {
					return err
				}
				return nil
			},
			ServerStreams: true,
			ClientStreams: false,
		}},
	}
	ch.RegisterService(&desc, struct{}{})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	clientStream, err := ch.NewStream(ctx, &desc.Streams[0], "/test.NoMetadata/ServerStream")
	if err != nil {
		t.Fatalf("NewStream failed: %v", err)
	}
	if err := clientStream.SendMsg(&wrapperspb.StringValue{Value: "req"}); err != nil {
		t.Fatalf("SendMsg failed: %v", err)
	}
	_ = clientStream.CloseSend()
	if err := clientStream.RecvMsg(new(wrapperspb.StringValue)); err == nil {
		t.Fatal("expected the stream to end with no response messages")
	}

	headers, headerErr := clientStream.Header()
	if headerErr != nil {
		t.Fatalf("Header: %v", headerErr)
	}
	if headers != nil {
		t.Errorf("headers = %#v, want nil", headers)
	}
	if trailers := clientStream.Trailer(); trailers != nil {
		t.Errorf("trailers = %#v, want nil", trailers)
	}
}
