package inprocgrpc_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	eventloop "github.com/joeycumines/go-eventloop"
	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
)

func TestRegisterStreamHandler_PanicsWithoutSlash(t *testing.T) {
	ch := newBareChannel(t)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		s := fmt.Sprint(r)
		if s != `inprocgrpc: method name must start with '/': "NoSlash"` {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()
	ch.RegisterStreamHandler("NoSlash", func(_ context.Context, _ *inprocgrpc.RPCStream) {})
}

func TestRegisterStreamHandler_PanicsNilHandler(t *testing.T) {
	ch := newBareChannel(t)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
	}()
	ch.RegisterStreamHandler("/test.Svc/Method", nil)
}

func TestRegisterStreamHandler_PanicsDuplicate(t *testing.T) {
	ch := newBareChannel(t)
	ch.RegisterStreamHandler("/test.Svc/Method", func(_ context.Context, _ *inprocgrpc.RPCStream) {})
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
	}()
	ch.RegisterStreamHandler("/test.Svc/Method", func(_ context.Context, _ *inprocgrpc.RPCStream) {})
}

// TestStreamHandler_Unary tests a non-blocking stream handler serving
// a unary-style RPC (one request, one response).
func TestStreamHandler_Unary(t *testing.T) {
	ch := newBareChannel(t)

	ch.RegisterStreamHandler("/test.Svc/Echo", func(ctx context.Context, s *inprocgrpc.RPCStream) {
		s.Recv().Recv(func(msg any, err error) {
			if err != nil {
				s.Finish(err)
				return
			}
			sv := msg.(*wrapperspb.StringValue)
			resp := &wrapperspb.StringValue{Value: "callback: " + sv.GetValue()}
			s.Send().Send(resp)
			s.Finish(nil)
		})
	})

	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	if err := ch.Invoke(context.Background(), "/test.Svc/Echo", req, resp); err != nil {
		t.Fatal(err)
	}
	if resp.GetValue() != "callback: hello" {
		t.Fatalf("unexpected response: %q", resp.GetValue())
	}
}

// TestStreamHandler_UnaryError tests a non-blocking handler returning
// an error via Finish.
func TestStreamHandler_UnaryError(t *testing.T) {
	ch := newBareChannel(t)

	ch.RegisterStreamHandler("/test.Svc/Fail", func(ctx context.Context, s *inprocgrpc.RPCStream) {
		s.Finish(status.Error(codes.NotFound, "not found"))
	})

	req := &wrapperspb.StringValue{Value: "x"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "/test.Svc/Fail", req, resp)
	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("not a status error: %v", err)
	}
	if st.Code() != codes.NotFound {
		t.Fatalf("unexpected code: %v", st.Code())
	}
}

// TestStreamHandler_SendThenFinishWithError tests that when a handler
// sends a response then finishes with an error, the error takes
// priority (standard gRPC unary semantics).
func TestStreamHandler_SendThenFinishWithError(t *testing.T) {
	ch := newBareChannel(t)

	ch.RegisterStreamHandler("/test.Svc/SendErr", func(ctx context.Context, s *inprocgrpc.RPCStream) {
		s.Recv().Recv(func(msg any, err error) {
			if err != nil {
				s.Finish(err)
				return
			}
			// Send a response, then finish with error.
			s.Send().Send(msg)
			s.Finish(status.Error(codes.Internal, "cleanup failed"))
		})
	})

	req := &wrapperspb.StringValue{Value: "x"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "/test.Svc/SendErr", req, resp)
	if err == nil {
		t.Fatal("expected error from Finish")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("not a status error: %v", err)
	}
	if st.Code() != codes.Internal {
		t.Fatalf("unexpected code: %v (msg: %s)", st.Code(), st.Message())
	}
}

// TestStreamHandler_UnaryWithMetadata tests headers and trailers flow
// through the stream handler's SetHeader/SetTrailer/SendHeader.
// Specifically verifies trailers set AFTER Send but BEFORE Finish are
// correctly captured (regression test for trailer-timing fix).
func TestStreamHandler_UnaryWithMetadata(t *testing.T) {
	ch := newBareChannel(t)

	ch.RegisterStreamHandler("/test.Svc/Meta", func(ctx context.Context, s *inprocgrpc.RPCStream) {
		s.SetHeader(metadata.Pairs("resp-header", "hval"))
		s.SendHeader()
		s.Recv().Recv(func(msg any, err error) {
			if err != nil {
				s.Finish(err)
				return
			}
			s.Send().Send(msg)
			// Set trailer AFTER sending response but BEFORE Finish.
			s.SetTrailer(metadata.Pairs("resp-trailer", "tval"))
			s.Finish(nil)
		})
	})

	var header, trailer metadata.MD
	req := &wrapperspb.StringValue{Value: "meta"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "/test.Svc/Meta", req, resp,
		grpc.Header(&header), grpc.Trailer(&trailer))
	if err != nil {
		t.Fatal(err)
	}
	if v := header.Get("resp-header"); len(v) == 0 || v[0] != "hval" {
		t.Fatalf("unexpected header: %v", header)
	}
	if v := trailer.Get("resp-trailer"); len(v) == 0 || v[0] != "tval" {
		t.Fatalf("unexpected trailer: %v", trailer)
	}
}

// TestStreamHandler_BidiStream tests a non-blocking handler with
// bidirectional streaming.
func TestStreamHandler_BidiStream(t *testing.T) {
	ch := newBareChannel(t)

	// Echo handler: for each received message, send it back.
	ch.RegisterStreamHandler("/test.Svc/BidiEcho", func(ctx context.Context, s *inprocgrpc.RPCStream) {
		var recv func()
		recv = func() {
			s.Recv().Recv(func(msg any, err error) {
				if err == io.EOF {
					s.Finish(nil)
					return
				}
				if err != nil {
					s.Finish(err)
					return
				}
				s.Send().Send(msg)
				recv() // wait for next message
			})
		}
		recv()
	})

	bidiDesc := grpc.StreamDesc{
		StreamName:    "BidiEcho",
		ServerStreams: true,
		ClientStreams: true,
	}

	stream, err := ch.NewStream(context.Background(), &bidiDesc, "/test.Svc/BidiEcho")
	if err != nil {
		t.Fatal(err)
	}

	// Send 5 messages, receive 5 echoes.
	for i := range 5 {
		msg := &wrapperspb.StringValue{Value: fmt.Sprintf("msg-%d", i)}
		if err := stream.SendMsg(msg); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
		resp := new(wrapperspb.StringValue)
		if err := stream.RecvMsg(resp); err != nil {
			t.Fatalf("recv %d: %v", i, err)
		}
		expected := fmt.Sprintf("msg-%d", i)
		if resp.GetValue() != expected {
			t.Fatalf("recv %d: got %q, want %q", i, resp.GetValue(), expected)
		}
	}

	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	// Should get EOF after CloseSend.
	resp := new(wrapperspb.StringValue)
	if err := stream.RecvMsg(resp); err != io.EOF {
		t.Fatalf("expected EOF, got: %v", err)
	}
}

// TestStreamHandler_ServerStream tests a callback-based handler for
// server-streaming RPCs.
func TestStreamHandler_ServerStream(t *testing.T) {
	ch := newBareChannel(t)

	ch.RegisterStreamHandler("/test.Svc/ServerStream", func(ctx context.Context, s *inprocgrpc.RPCStream) {
		s.Recv().Recv(func(msg any, err error) {
			if err != nil {
				s.Finish(err)
				return
			}
			// Send 3 copies of the request.
			for i := range 3 {
				sv := msg.(*wrapperspb.StringValue)
				s.Send().Send(&wrapperspb.StringValue{
					Value: fmt.Sprintf("%s:%d", sv.GetValue(), i),
				})
			}
			s.Finish(nil)
		})
	})

	ssDesc := grpc.StreamDesc{
		StreamName:    "ServerStream",
		ServerStreams: true,
	}

	stream, err := ch.NewStream(context.Background(), &ssDesc, "/test.Svc/ServerStream")
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SendMsg(&wrapperspb.StringValue{Value: "data"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}

	for i := range 3 {
		resp := new(wrapperspb.StringValue)
		if err := stream.RecvMsg(resp); err != nil {
			t.Fatalf("recv %d: %v", i, err)
		}
		expected := fmt.Sprintf("data:%d", i)
		if resp.GetValue() != expected {
			t.Fatalf("recv %d: got %q, want %q", i, resp.GetValue(), expected)
		}
	}
	resp := new(wrapperspb.StringValue)
	if err := stream.RecvMsg(resp); err != io.EOF {
		t.Fatalf("expected EOF, got: %v", err)
	}
}

// TestStreamHandler_CoexistsWithBlockingHandler tests that callback-based
// and blocking handlers can coexist on the same Channel.
func TestStreamHandler_CoexistsWithBlockingHandler(t *testing.T) {
	ch := newBareChannel(t)

	// Register a blocking handler via RegisterService.
	ch.RegisterService(&testServiceDesc, &echoServer{})

	// Register a callback handler for a different method on the same Channel.
	ch.RegisterStreamHandler("/test.Special/Callback", func(ctx context.Context, s *inprocgrpc.RPCStream) {
		s.Recv().Recv(func(msg any, err error) {
			if err != nil {
				s.Finish(err)
				return
			}
			sv := msg.(*wrapperspb.StringValue)
			s.Send().Send(&wrapperspb.StringValue{Value: "callback: " + sv.GetValue()})
			s.Finish(nil)
		})
	})

	// Test the blocking handler (unary).
	req := &wrapperspb.StringValue{Value: "blocking"}
	resp := new(wrapperspb.StringValue)
	if err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp); err != nil {
		t.Fatal(err)
	}
	if resp.GetValue() != "echo: blocking" {
		t.Fatalf("blocking: got %q", resp.GetValue())
	}

	// Test the callback handler (unary via Invoke).
	req2 := &wrapperspb.StringValue{Value: "async"}
	resp2 := new(wrapperspb.StringValue)
	if err := ch.Invoke(context.Background(), "/test.Special/Callback", req2, resp2); err != nil {
		t.Fatal(err)
	}
	if resp2.GetValue() != "callback: async" {
		t.Fatalf("callback: got %q", resp2.GetValue())
	}
}

// TestStreamHandler_StreamTakesPriorityOverService verifies that a stream
// handler registered for a method takes priority over a service handler
// for the same method path.
func TestStreamHandler_StreamTakesPriorityOverService(t *testing.T) {
	ch := newBareChannel(t)

	// Register both a service handler and a stream handler for the same method.
	ch.RegisterService(&testServiceDesc, &echoServer{})
	ch.RegisterStreamHandler("/test.TestService/Unary", func(ctx context.Context, s *inprocgrpc.RPCStream) {
		s.Recv().Recv(func(msg any, err error) {
			if err != nil {
				s.Finish(err)
				return
			}
			sv := msg.(*wrapperspb.StringValue)
			s.Send().Send(&wrapperspb.StringValue{Value: "stream-priority: " + sv.GetValue()})
			s.Finish(nil)
		})
	})

	req := &wrapperspb.StringValue{Value: "test"}
	resp := new(wrapperspb.StringValue)
	if err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp); err != nil {
		t.Fatal(err)
	}
	// The stream handler should take priority.
	if resp.GetValue() != "stream-priority: test" {
		t.Fatalf("expected stream handler priority, got %q", resp.GetValue())
	}
}

// TestStreamHandler_ConcurrentUnary tests concurrent unary RPCs through
// a non-blocking stream handler.
func TestStreamHandler_ConcurrentUnary(t *testing.T) {
	ch := newBareChannel(t)

	ch.RegisterStreamHandler("/test.Svc/Echo", func(ctx context.Context, s *inprocgrpc.RPCStream) {
		s.Recv().Recv(func(msg any, err error) {
			if err != nil {
				s.Finish(err)
				return
			}
			s.Send().Send(msg)
			s.Finish(nil)
		})
	})

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := &wrapperspb.StringValue{Value: fmt.Sprintf("req-%d", i)}
			resp := new(wrapperspb.StringValue)
			if err := ch.Invoke(context.Background(), "/test.Svc/Echo", req, resp); err != nil {
				t.Errorf("invoke %d: %v", i, err)
				return
			}
			if resp.GetValue() != fmt.Sprintf("req-%d", i) {
				t.Errorf("invoke %d: got %q", i, resp.GetValue())
			}
		}(i)
	}
	wg.Wait()
}

// TestStreamSender_CloseAndClosed exercises StreamSender.Close and
// StreamSender.Closed via the RPCStream API.
func TestStreamSender_CloseAndClosed(t *testing.T) {
	ch := newBareChannel(t)

	var senderClosed atomic.Bool
	ch.RegisterStreamHandler("/test.Svc/CloseTest", func(ctx context.Context, s *inprocgrpc.RPCStream) {
		sender := s.Send()
		if sender.Closed() {
			t.Error("sender should not be closed initially")
		}
		sender.Close(nil)
		senderClosed.Store(sender.Closed())
		s.Finish(status.Error(codes.OK, ""))
	})

	req := &wrapperspb.StringValue{Value: "x"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "/test.Svc/CloseTest", req, resp)
	// The handler closes responses early and then calls Finish - the
	// client gets an EOF (translated into nil by Invoke for unary).
	_ = err
	if !senderClosed.Load() {
		t.Fatal("sender.Closed() should be true after Close")
	}
}

// TestStreamReceiver_Closed exercises StreamReceiver.Closed via the
// RPCStream API after the request stream has been fully consumed.
func TestStreamReceiver_Closed(t *testing.T) {
	ch := newBareChannel(t)

	var receiverClosed atomic.Bool
	ch.RegisterStreamHandler("/test.Svc/RecvCloseTest", func(ctx context.Context, s *inprocgrpc.RPCStream) {
		recv := s.Recv()
		// Before reading: the request stream was closed by Invoke, but
		// there's a buffered message - Closed may be true (close is
		// already queued) but we still get a message.
		s.Recv().Recv(func(msg any, err error) {
			if err != nil {
				s.Finish(err)
				return
			}
			// After consuming the message, drain the EOF.
			s.Recv().Recv(func(_ any, _ error) {
				receiverClosed.Store(recv.Closed())
				s.Send().Send(msg)
				s.Finish(nil)
			})
		})
	})

	req := &wrapperspb.StringValue{Value: "y"}
	resp := new(wrapperspb.StringValue)
	if err := ch.Invoke(context.Background(), "/test.Svc/RecvCloseTest", req, resp); err != nil {
		t.Fatal(err)
	}
	if !receiverClosed.Load() {
		t.Fatal("recv.Closed() should be true after draining all messages")
	}
}

// TestRPCStream_Method tests that RPCStream.Method returns the correct value.
func TestRPCStream_Method(t *testing.T) {
	ch := newBareChannel(t)

	var gotMethod string
	ch.RegisterStreamHandler("/test.Svc/Check", func(ctx context.Context, s *inprocgrpc.RPCStream) {
		gotMethod = s.Method()
		s.Recv().Recv(func(msg any, err error) {
			if err != nil {
				s.Finish(err)
				return
			}
			s.Send().Send(msg)
			s.Finish(nil)
		})
	})

	req := &wrapperspb.StringValue{Value: "x"}
	resp := new(wrapperspb.StringValue)
	if err := ch.Invoke(context.Background(), "/test.Svc/Check", req, resp); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "/test.Svc/Check" {
		t.Fatalf("unexpected method: %q", gotMethod)
	}
}

// errorCloner is a test Cloner that can be configured to fail on Clone or Copy.
type errorCloner struct {
	cloneErr error
	copyErr  error
}

func (e *errorCloner) Clone(in any) (any, error) {
	if e.cloneErr != nil {
		return nil, e.cloneErr
	}
	return inprocgrpc.ProtoCloner{}.Clone(in)
}

func (e *errorCloner) Copy(out, in any) error {
	if e.copyErr != nil {
		return e.copyErr
	}
	return inprocgrpc.ProtoCloner{}.Copy(out, in)
}

// TestStreamHandler_UnaryCloneError covers the clone-error path in
// invokeStreamHandler (channel.go: cloneErr != nil).
func TestStreamHandler_UnaryCloneError(t *testing.T) {
	cloneErr := errors.New("clone failed")
	ch := newBareChannel(t, inprocgrpc.WithCloner(&errorCloner{cloneErr: cloneErr}))

	ch.RegisterStreamHandler("/test.Svc/Echo", func(ctx context.Context, s *inprocgrpc.RPCStream) {
		s.Recv().Recv(func(msg any, err error) {
			if err != nil {
				s.Finish(err)
				return
			}
			s.Send().Send(msg)
			s.Finish(nil)
		})
	})

	req := &wrapperspb.StringValue{Value: "x"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "/test.Svc/Echo", req, resp)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, cloneErr) {
		t.Fatalf("expected errors.Is(err, cloneErr), got: %v", err)
	}
}

// TestStreamHandler_UnaryCopyError covers the copy-error path in
// invokeStreamHandler (channel.go: copyErr != nil).
func TestStreamHandler_UnaryCopyError(t *testing.T) {
	copyErr := errors.New("copy failed")
	ch := newBareChannel(t, inprocgrpc.WithCloner(&errorCloner{copyErr: copyErr}))

	ch.RegisterStreamHandler("/test.Svc/Echo", func(ctx context.Context, s *inprocgrpc.RPCStream) {
		s.Recv().Recv(func(msg any, err error) {
			if err != nil {
				s.Finish(err)
				return
			}
			s.Send().Send(msg)
			s.Finish(nil)
		})
	})

	req := &wrapperspb.StringValue{Value: "x"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "/test.Svc/Echo", req, resp)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, copyErr) {
		t.Fatalf("expected errors.Is(err, copyErr), got: %v", err)
	}
}

// TestStreamHandler_UnaryDeadlineExceeded covers the ctx.Done() select case
// in invokeStreamHandler when the handler never responds.
func TestStreamHandler_UnaryDeadlineExceeded(t *testing.T) {
	ch := newBareChannel(t)

	// Handler that never responds (simulating a slow/stuck handler).
	ch.RegisterStreamHandler("/test.Svc/Slow", func(ctx context.Context, s *inprocgrpc.RPCStream) {
		// Do nothing - never calls Finish, never sends a response.
		// The deadline expiry should unblock the caller.
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req := &wrapperspb.StringValue{Value: "x"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(ctx, "/test.Svc/Slow", req, resp)
	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("not a status error: %v", err)
	}
	if st.Code() != codes.DeadlineExceeded {
		t.Fatalf("unexpected code: %v", st.Code())
	}
}

// TestStreamHandler_StreamContextCancellation verifies that cancelling the
// client context while a streaming RPC is open results in a Canceled status
// error from RecvMsg. The handler returns immediately without calling Finish,
// so context cancellation is the only mechanism that closes the streams.
//
// Multiple iterations are used because the context cancellation watcher
// goroutine in newStreamWithHandler has a select between ctx.Done() and
// svrCtx.Done(), which are both signalled when ctx is cancelled (since
// svrCtx derives from ctx). Running multiple iterations ensures the
// ctx.Done() case body is exercised at least once.
func TestStreamHandler_StreamContextCancellation(t *testing.T) {
	ch := newBareChannel(t)

	ch.RegisterStreamHandler("/test.Svc/SlowStream", func(ctx context.Context, s *inprocgrpc.RPCStream) {
		// Return immediately without calling Finish or sending any
		// messages. The streams remain open, and only context
		// cancellation (or GC) will close them.
	})

	bidiDesc := grpc.StreamDesc{
		StreamName:    "SlowStream",
		ServerStreams: true,
		ClientStreams: true,
	}

	for range 50 {
		ctx, cancel := context.WithCancel(context.Background())

		stream, err := ch.NewStream(ctx, &bidiDesc, "/test.Svc/SlowStream")
		if err != nil {
			cancel()
			t.Fatal(err)
		}

		// Cancel the context, then RecvMsg should return a cancellation error.
		cancel()

		resp := new(wrapperspb.StringValue)
		err = stream.RecvMsg(resp)
		if err == nil {
			t.Fatal("expected error after context cancellation")
		}
		st, ok := status.FromError(err)
		if !ok {
			t.Fatalf("not a status error: %v", err)
		}
		if st.Code() != codes.Canceled {
			t.Fatalf("unexpected code: %v", st.Code())
		}
	}
}

// TestStreamHandler_InvokeLoopStopped covers the submitErr path in
// invokeStreamHandler when the event loop has been stopped.
func TestStreamHandler_InvokeLoopStopped(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	ch := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop))
	ch.RegisterStreamHandler("/test.Svc/Echo", func(_ context.Context, s *inprocgrpc.RPCStream) {
		s.Recv().Recv(func(msg any, recvErr error) {
			if recvErr != nil {
				s.Finish(recvErr)
				return
			}
			s.Send().Send(msg)
			s.Finish(nil)
		})
	})

	// Stop the loop.
	cancel()
	<-done

	// Now Invoke should fail with Unavailable.
	req := &wrapperspb.StringValue{Value: "x"}
	resp := new(wrapperspb.StringValue)
	invokeErr := ch.Invoke(context.Background(), "/test.Svc/Echo", req, resp)
	if invokeErr == nil {
		t.Fatal("expected error when loop is stopped")
	}
	st, ok := status.FromError(invokeErr)
	if !ok {
		t.Fatalf("not a status error: %v", invokeErr)
	}
	if st.Code() != codes.Unavailable {
		t.Fatalf("unexpected code: %v", st.Code())
	}
}

// TestStreamHandler_NewStreamLoopStopped covers the submitErr path in
// newStreamWithHandler when the event loop has been stopped.
func TestStreamHandler_NewStreamLoopStopped(t *testing.T) {
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	ch := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop))
	ch.RegisterStreamHandler("/test.Svc/Echo", func(_ context.Context, s *inprocgrpc.RPCStream) {
		s.Recv().Recv(func(msg any, recvErr error) {
			if recvErr != nil {
				s.Finish(recvErr)
				return
			}
			s.Send().Send(msg)
			s.Finish(nil)
		})
	})

	// Stop the loop.
	cancel()
	<-done

	// Now NewStream should fail with Unavailable.
	bidiDesc := grpc.StreamDesc{
		StreamName:    "Echo",
		ServerStreams: true,
		ClientStreams: true,
	}
	_, streamErr := ch.NewStream(context.Background(), &bidiDesc, "/test.Svc/Echo")
	if streamErr == nil {
		t.Fatal("expected error when loop is stopped")
	}
	st, ok := status.FromError(streamErr)
	if !ok {
		t.Fatalf("not a status error: %v", streamErr)
	}
	if st.Code() != codes.Unavailable {
		t.Fatalf("unexpected code: %v", st.Code())
	}
}
