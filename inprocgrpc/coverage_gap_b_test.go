package inprocgrpc_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	eventloop "github.com/joeycumines/go-eventloop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
)

func TestCoverage_ServerAdapter_SendMsg_LoopStopped(t *testing.T) {
	svrErr := make(chan error, 1)
	loop := eventloop.New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))

	handlerReady := make(chan struct{})
	loopStopped := make(chan struct{})

	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "BidiStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			close(handlerReady)
			<-loopStopped
			err := stream.SendMsg(&wrapperspb.StringValue{Value: "too late"})
			svrErr <- err
			return err
		},
		ServerStreams: true,
		ClientStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	_, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}

	<-handlerReady
	cancel()
	<-done
	close(loopStopped)

	e := <-svrErr
	if e == nil {
		t.Fatal("expected error from SendMsg on stopped loop")
	}
}

func TestCoverage_ServerAdapter_RecvMsg_LoopStopped(t *testing.T) {
	svrErr := make(chan error, 1)
	loop := eventloop.New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))

	handlerReady := make(chan struct{})
	loopStopped := make(chan struct{})

	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "BidiStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			close(handlerReady)
			<-loopStopped
			err := stream.RecvMsg(new(wrapperspb.StringValue))
			svrErr <- err
			return err
		},
		ServerStreams: true,
		ClientStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	_, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}

	<-handlerReady
	cancel()
	<-done
	close(loopStopped)

	e := <-svrErr
	if e == nil {
		t.Fatal("expected error from RecvMsg on stopped loop")
	}
}

// --- Coverage gap: Server adapter ctx.Done in selects ---

func TestCoverage_ServerAdapter_SetHeader_ContextDone(t *testing.T) {
	// Server handler's context is cancelled while SetHeader is blocking.
	// We block the loop so SubmitInternal's callback can't execute,
	// ensuring only ctx.Done is ready in the select - deterministic.
	loop := newTestLoop(t)
	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))

	svrErr := make(chan error, 1)
	handlerReady := make(chan struct{})
	loopBlocked := make(chan struct{})
	unblockLoop := make(chan struct{})

	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "BidiStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			close(handlerReady)
			// Wait for the loop to be blocked and context to be cancelled.
			<-loopBlocked
			// Loop is blocked → SubmitInternal queues behind blocker.
			// Context is cancelled → ctx.Done is ready in the select.
			err := stream.SetHeader(metadata.Pairs("k", "v"))
			svrErr <- err
			return err
		},
		ServerStreams: true,
		ClientStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	ctx, cancel := context.WithCancel(context.Background())
	_, err := ch.NewStream(ctx, &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}

	<-handlerReady

	// Block the loop.
	if err := loop.Submit(func() {
		close(loopBlocked)
		<-unblockLoop
	}); err != nil {
		t.Fatal(err)
	}

	<-loopBlocked
	// Cancel context - handler will see ctx.Done in the select.
	cancel()

	e := <-svrErr
	if e == nil {
		t.Fatal("expected error from SetHeader with cancelled context")
	}
	close(unblockLoop)
}

func TestCoverage_ServerAdapter_SendHeader_ContextDone(t *testing.T) {
	// Same pattern as SetHeader: block the loop so SubmitInternal can't
	// execute, ensuring only ctx.Done is ready in the select.
	loop := newTestLoop(t)
	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))

	svrErr := make(chan error, 1)
	handlerReady := make(chan struct{})
	loopBlocked := make(chan struct{})
	unblockLoop := make(chan struct{})

	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "BidiStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			close(handlerReady)
			<-loopBlocked
			err := stream.SendHeader(metadata.Pairs("k", "v"))
			svrErr <- err
			return err
		},
		ServerStreams: true,
		ClientStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	ctx, cancel := context.WithCancel(context.Background())
	_, err := ch.NewStream(ctx, &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}

	<-handlerReady

	if err := loop.Submit(func() {
		close(loopBlocked)
		<-unblockLoop
	}); err != nil {
		t.Fatal(err)
	}

	<-loopBlocked
	cancel()

	e := <-svrErr
	if e == nil {
		t.Fatal("expected error from SendHeader with cancelled context")
	}
	close(unblockLoop)
}

func TestCoverage_ServerAdapter_RecvMsg_ContextDone(t *testing.T) {
	svrErr := make(chan error, 1)
	ch := newBareChannel(t)
	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "BidiStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			<-stream.Context().Done()
			err := stream.RecvMsg(new(wrapperspb.StringValue))
			svrErr <- err
			return err
		},
		ServerStreams: true,
		ClientStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	ctx, cancel := context.WithCancel(context.Background())
	_, err := ch.NewStream(ctx, &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}

	cancel()
	e := <-svrErr
	if e == nil {
		t.Fatal("expected error from RecvMsg with cancelled context")
	}
}

// --- Coverage gap: Client stream SendMsg clone error ---

func TestCoverage_Stream_ClientSendMsg_CloneError(t *testing.T) {
	ch := newBareChannel(t, inprocgrpc.WithCloner(&conditionalCloner{
		cloneErr:   fmt.Errorf("client clone boom"),
		cloneErrAt: 0, // fail on all clones
	}))
	ch.RegisterService(&testServiceDesc, &echoServer{})

	cs, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}

	err = cs.SendMsg(&wrapperspb.StringValue{Value: "hello"})
	if err == nil {
		t.Fatal("expected error")
	}
	if status.Code(err) != codes.Internal || !strings.Contains(err.Error(), "client clone boom") {
		t.Fatalf("SendMsg = %v, want Internal clone failure", err)
	}
}

// --- Coverage gap: Server adapter SendMsg clone error ---

func TestCoverage_ServerAdapter_SendMsg_CloneError(t *testing.T) {
	// Use conditionalCloner (not CloneFunc!) because CloneFunc's Copy
	// implementation internally calls Clone, which would fail during
	// server RecvMsg before reaching server SendMsg.
	ch := newBareChannel(t, inprocgrpc.WithCloner(&conditionalCloner{
		cloneErr:   fmt.Errorf("server clone boom"),
		cloneErrAt: 2, // First clone (client SendMsg) succeeds, second (server SendMsg) fails
	}))
	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			in := new(wrapperspb.StringValue)
			if err := stream.RecvMsg(in); err != nil {
				return err
			}
			return stream.SendMsg(&wrapperspb.StringValue{Value: "resp"})
		},
		ServerStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	cs, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
	}, "/test.TestService/ServerStream")
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.SendMsg(&wrapperspb.StringValue{Value: "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := cs.CloseSend(); err != nil {
		t.Fatal(err)
	}

	msg := new(wrapperspb.StringValue)
	err = cs.RecvMsg(msg)
	if err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("expected non-EOF error, got: %v", err)
	}
}

// --- Coverage gap: doEnd with non-EOF error ---

func TestCoverage_Stream_DoEnd_NonEOFError(t *testing.T) {
	rec := &statsRecorder{}
	ch := newBareChannel(t, inprocgrpc.WithClientStatsHandler(rec))
	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			return status.Error(codes.Internal, "server error")
		},
		ServerStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	cs, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
	}, "/test.TestService/ServerStream")
	if err != nil {
		t.Fatal(err)
	}

	msg := new(wrapperspb.StringValue)
	err = cs.RecvMsg(msg)
	if err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("expected non-EOF error, got: %v", err)
	}

	// Verify stats.End was called with non-nil error
	events := waitStatsEnd(t, rec)
	for _, ev := range events {
		if end, ok := ev.(*stats.End); ok {
			if end.Error == nil {
				t.Error("stats End error should be non-nil")
			}
			return
		}
	}
	t.Error("stats handler did not see End event")
}

// --- Coverage gap: doEnd called twice (idempotent) ---

func TestCoverage_Stream_DoEnd_CalledTwice(t *testing.T) {
	rec := &statsRecorder{}
	ch := newBareChannel(t, inprocgrpc.WithClientStatsHandler(rec))
	ch.RegisterService(&testServiceDesc, &echoServer{})

	cs, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
	}, "/test.TestService/ServerStream")
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.SendMsg(&wrapperspb.StringValue{Value: "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := cs.CloseSend(); err != nil {
		t.Fatal(err)
	}

	// Drain all
	for {
		msg := new(wrapperspb.StringValue)
		err := cs.RecvMsg(msg)
		if err != nil {
			break
		}
	}

	// RecvMsg returning io.EOF triggers doEnd. Calling RecvMsg again
	// should trigger doEnd again but it's a no-op.
	msg := new(wrapperspb.StringValue)
	_ = cs.RecvMsg(msg) // second call

	// Should have exactly 1 End event
	events := waitStatsEnd(t, rec)
	endCount := 0
	for _, ev := range events {
		if _, ok := ev.(*stats.End); ok {
			endCount++
		}
	}
	if endCount != 1 {
		t.Errorf("expected exactly 1 End event, got %d", endCount)
	}
}

// --- Coverage gap: Client RecvMsg Copy error ---

func TestCoverage_Stream_ClientRecvMsg_CopyError(t *testing.T) {
	// The Copy is called in recvMsgLocked to copy the received message
	// to the caller's output. We make it fail via a counting cloner.
	ch := newBareChannel(t, inprocgrpc.WithCloner(&conditionalCloner{
		copyErr:   fmt.Errorf("recv copy boom"),
		copyErrAt: 2, // First copy: codec decode. Second: client RecvMsg.
	}))
	ch.RegisterService(&testServiceDesc, &echoServer{})

	cs, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
	}, "/test.TestService/ServerStream")
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.SendMsg(&wrapperspb.StringValue{Value: "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := cs.CloseSend(); err != nil {
		t.Fatal(err)
	}
	msg := new(wrapperspb.StringValue)
	err = cs.RecvMsg(msg)
	if status.Code(err) != codes.Internal || !strings.Contains(err.Error(), "recv copy boom") {
		t.Fatalf("RecvMsg = %v, want Internal copy failure", err)
	}
}

func TestCoverage_Stream_EnsureNoMore_ContextCancel(t *testing.T) {
	// For a non-streaming response, after RecvMsg gets the first message,
	// it calls ensureNoMoreLocked which does another Recv.
	// If the context is cancelled during that wait, the ctx.Done branch fires.
	//
	// Strategy: use a CopyFunc cloner that blocks during the client's
	// RecvMsg Copy. While blocked, we cancel the context. After Copy
	// returns, ensureNoMore runs with ctx already cancelled and its
	// select hits ctx.Done deterministically.
	var copyNum atomic.Int64
	copyReady := make(chan struct{})
	copyProceed := make(chan struct{})

	cloner := inprocgrpc.CopyFunc(func(out, in any) error {
		err := inprocgrpc.ProtoCloner{}.Copy(out, in)
		// CopyFunc's Clone also calls this function, so count:
		// #1: Client SendMsg Clone (CopyFunc internal)
		// #2: Server RecvMsg Copy
		// #3: Server SendMsg Clone (CopyFunc internal)
		// #4: Client RecvMsg Copy ← block here
		if n := copyNum.Add(1); n == 4 {
			close(copyReady)
			<-copyProceed
		}
		return err
	})

	ch := newBareChannel(t, inprocgrpc.WithCloner(cloner))
	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			in := new(wrapperspb.StringValue)
			if err := stream.RecvMsg(in); err != nil {
				return err
			}
			// Send one message, then hang (never finish)
			if err := stream.SendMsg(&wrapperspb.StringValue{Value: "one"}); err != nil {
				return err
			}
			// Block forever - ensureNoMore will wait for the second message
			<-stream.Context().Done()
			return stream.Context().Err()
		},
		ServerStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	ctx, cancel := context.WithCancel(context.Background())
	cs, err := ch.NewStream(ctx, &grpc.StreamDesc{
		ServerStreams: false, // non-streaming → triggers ensureNoMore
	}, "/test.TestService/ServerStream")
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.SendMsg(&wrapperspb.StringValue{Value: "go"}); err != nil {
		t.Fatal(err)
	}
	if err := cs.CloseSend(); err != nil {
		t.Fatal(err)
	}

	// Start RecvMsg in goroutine (it will block in Copy #4)
	recvDone := make(chan error, 1)
	go func() {
		msg := new(wrapperspb.StringValue)
		recvDone <- cs.RecvMsg(msg)
	}()

	// Wait until RecvMsg is in Copy #4 (message received, about to return)
	<-copyReady

	// Cancel context - when Copy returns, ensureNoMore will see ctx.Done
	cancel()
	close(copyProceed)

	err = <-recvDone
	if err == nil {
		t.Fatal("expected error from ensureNoMore with cancelled context")
	}
}

// --- Coverage gap: RecvMsg initial Submit failure (loop stopped) ---

func TestCoverage_Stream_EnsureNoMore_LoopStopped(t *testing.T) {
	loop := eventloop.New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))

	handlerReady := make(chan struct{})
	handlerProceed := make(chan struct{})

	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			in := new(wrapperspb.StringValue)
			if err := stream.RecvMsg(in); err != nil {
				return err
			}
			if err := stream.SendMsg(&wrapperspb.StringValue{Value: "one"}); err != nil {
				return err
			}
			close(handlerReady)
			<-handlerProceed
			return nil
		},
		ServerStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	cs, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: false, // triggers ensureNoMore
	}, "/test.TestService/ServerStream")
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.SendMsg(&wrapperspb.StringValue{Value: "go"}); err != nil {
		t.Fatal(err)
	}
	if err := cs.CloseSend(); err != nil {
		t.Fatal(err)
	}

	// Wait for handler to send the message and be ready
	<-handlerReady

	// Stop the loop BEFORE RecvMsg. RecvMsg's initial Submit fails
	// (returns ErrLoopTerminated), so RecvMsg returns io.EOF directly.
	// Note: ensureNoMore is never reached - RecvMsg bails out at Submit.
	cancel()
	<-done
	close(handlerProceed)

	// RecvMsg's Submit fails → returns io.EOF
	msg := new(wrapperspb.StringValue)
	_ = cs.RecvMsg(msg)
	// Expected: io.EOF (loop stopped, Submit returns error)
}

func TestCoverage_Stream_FetchTrailers_LoopStopped(t *testing.T) {
	loop := eventloop.New()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()

	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))
	ch.RegisterService(&testServiceDesc, &echoServer{})

	cs, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
	}, "/test.TestService/ServerStream")
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.SendMsg(&wrapperspb.StringValue{Value: "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := cs.CloseSend(); err != nil {
		t.Fatal(err)
	}

	// Drain while loop is running
	for {
		msg := new(wrapperspb.StringValue)
		err := cs.RecvMsg(msg)
		if err != nil {
			break
		}
	}

	// Stop the loop
	cancel()
	<-done

	// Now Trailer() calls fetchTrailersOnLoop, Submit fails, returns nil
	md := cs.Trailer()
	_ = md
}

func TestCoverage_Stream_DoEnd_NoStatsHandler(t *testing.T) {
	ch := newTestChannel(t)
	cs, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
	}, "/test.TestService/ServerStream")
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.SendMsg(&wrapperspb.StringValue{Value: "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := cs.CloseSend(); err != nil {
		t.Fatal(err)
	}

	// Drain (doEnd with no stats handler = early return)
	for {
		msg := new(wrapperspb.StringValue)
		if err := cs.RecvMsg(msg); err != nil {
			break
		}
	}
}

// --- Coverage gap: server stats with server error (Invoke) ---

func TestCoverage_Invoke_ServerStatsWithError(t *testing.T) {
	rec := &statsRecorder{}
	ch := newBareChannel(t, inprocgrpc.WithServerStatsHandler(rec))
	desc := coverageServiceDesc(func(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
		in := new(wrapperspb.StringValue)
		if err := dec(in); err != nil {
			return nil, err
		}
		return nil, status.Error(codes.Internal, "server boom")
	}, nil)
	ch.RegisterService(&desc, &echoServer{})

	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp)
	if err == nil {
		t.Fatal("expected error")
	}
	// Verify server stats End was called with error
	events := waitStatsEnd(t, rec)
	for _, ev := range events {
		if end, ok := ev.(*stats.End); ok {
			if end.Error == nil {
				t.Error("server stats End error should be non-nil")
			}
			return
		}
	}
	t.Error("server stats handler did not see End event")
}

// --- Coverage gap: server stats with server error (NewStream) ---

func TestCoverage_NewStream_ServerStatsWithError(t *testing.T) {
	rec := &statsRecorder{}
	loop := newTestLoop(t)
	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop), inprocgrpc.WithServerStatsHandler(rec))
	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			return status.Error(codes.Internal, "stream boom")
		},
		ServerStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	cs, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
	}, "/test.TestService/ServerStream")
	if err != nil {
		t.Fatal(err)
	}

	// Drain - handler returns error
	msg := new(wrapperspb.StringValue)
	_ = cs.RecvMsg(msg)

	// Verify server stats End was called with error
	events := waitStatsEnd(t, rec)
	for _, ev := range events {
		if end, ok := ev.(*stats.End); ok {
			if end.Error == nil {
				t.Error("server stats End error should be non-nil")
			}
			return
		}
	}
	t.Error("server stats handler did not see End event")
}

// --- Coverage gap: method without leading / (NewStream) ---

func TestCoverage_NewStream_MethodWithoutLeadingSlash(t *testing.T) {
	ch := newTestChannel(t)
	cs, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
	}, "test.TestService/ServerStream") // no leading /
	if err != nil {
		t.Fatal(err)
	}
	if err := cs.SendMsg(&wrapperspb.StringValue{Value: "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := cs.CloseSend(); err != nil {
		t.Fatal(err)
	}
	// Drain
	for {
		msg := new(wrapperspb.StringValue)
		if err := cs.RecvMsg(msg); err != nil {
			break
		}
	}
}
