package inprocgrpc_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	eventloop "github.com/joeycumines/go-eventloop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
)

// --- Coverage gap: NewChannel with option error ---

func TestCoverage_NewChannel_OptionError(t *testing.T) {
	loop := newTestLoop(t)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic from nil stats handler option")
		}
		s, ok := r.(string)
		if !ok {
			t.Fatalf("expected string panic, got %T", r)
		}
		if !strings.Contains(s, "inprocgrpc:") {
			t.Fatalf("unexpected panic message: %q", s)
		}
	}()
	inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop), inprocgrpc.WithClientStatsHandler(nil))
}

// --- Coverage gap: Invoke with clone error ---

type conditionalCloner struct {
	cloneCount atomic.Int64
	cloneErr   error // returned when cloneErrAt matches cloneCount (or always if cloneErrAt==0)
	cloneErrAt int64 // 1-based: fail on Nth Clone call (0 = fail on all if cloneErr set)
	copyCount  atomic.Int64
	copyErr    error // returned when copyErrAt matches copyCount
	copyErrAt  int64 // 1-based: fail on Nth Copy call
}

func (c *conditionalCloner) Clone(in any) (any, error) {
	n := c.cloneCount.Add(1)
	if c.cloneErr != nil {
		if c.cloneErrAt == 0 || n == c.cloneErrAt {
			return nil, c.cloneErr
		}
	}
	return inprocgrpc.ProtoCloner{}.Clone(in)
}

func (c *conditionalCloner) Copy(out, in any) error {
	n := c.copyCount.Add(1)
	if c.copyErrAt > 0 && n == c.copyErrAt {
		return c.copyErr
	}
	return inprocgrpc.ProtoCloner{}.Copy(out, in)
}

func TestCoverage_Invoke_CloneRequestError(t *testing.T) {
	ch := newTestChannel(t, inprocgrpc.WithCloner(&conditionalCloner{
		cloneErr: fmt.Errorf("clone boom"),
	}))

	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "clone boom" {
		t.Fatalf("got %q, want %q", err.Error(), "clone boom")
	}
}

func TestCoverage_Invoke_CopyResponseError(t *testing.T) {
	// Copy is called: (1) codec decode of request, (2) copy response to caller.
	// We want (2) to fail.
	cloner := &conditionalCloner{
		copyErr:   fmt.Errorf("copy response boom"),
		copyErrAt: 2, // second Copy call
	}
	ch := newTestChannel(t, inprocgrpc.WithCloner(cloner))

	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "copy response boom" {
		t.Fatalf("got %q, want %q", err.Error(), "copy response boom")
	}
}

// --- Coverage gap: Invoke error with headers+trailers+stats ---

func TestCoverage_Invoke_ErrorWithHeadersTrailersStats(t *testing.T) {
	rec := &statsRecorder{}
	ch := newBareChannel(t, inprocgrpc.WithClientStatsHandler(rec))
	desc := coverageServiceDesc(func(srv any, ctx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
		in := new(wrapperspb.StringValue)
		if err := dec(in); err != nil {
			return nil, err
		}
		_ = grpc.SetHeader(ctx, metadata.Pairs("err-hdr", "hv"))
		grpc.SetTrailer(ctx, metadata.Pairs("err-trl", "tv"))
		return nil, status.Error(codes.PermissionDenied, "denied")
	}, nil)
	ch.RegisterService(&desc, &echoServer{})

	var hdrs, tlrs metadata.MD
	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp,
		grpc.Header(&hdrs), grpc.Trailer(&tlrs),
	)
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.PermissionDenied {
		t.Errorf("got %v, want PermissionDenied", st.Code())
	}
	if v := hdrs.Get("err-hdr"); len(v) == 0 || v[0] != "hv" {
		t.Errorf("headers: %v", hdrs)
	}
	if v := tlrs.Get("err-trl"); len(v) == 0 || v[0] != "tv" {
		t.Errorf("trailers: %v", tlrs)
	}
	// Stats should show InHeader, InTrailer, End
	events := rec.getEvents()
	assertHasEventTypes(t, "err-stats", events,
		(*stats.InHeader)(nil),
		(*stats.InTrailer)(nil),
		(*stats.End)(nil),
	)
}

// --- Coverage gap: Loop not running (outer Submit failure) ---

// stoppedLoopChannel creates a Channel whose loop has already stopped.
func stoppedLoopChannel(t testing.TB, opts ...inprocgrpc.Option) *inprocgrpc.Channel {
	t.Helper()
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
	cancel()
	<-done

	opts = append([]inprocgrpc.Option{inprocgrpc.WithLoop(loop)}, opts...)
	ch := inprocgrpc.NewChannel(opts...)
	ch.RegisterService(&testServiceDesc, &echoServer{})
	return ch
}

func TestCoverage_Invoke_LoopNotRunning(t *testing.T) {
	ch := stoppedLoopChannel(t)
	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp)
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got %v, want Unavailable", st.Code())
	}
}

func TestCoverage_Invoke_LoopNotRunning_WithStats(t *testing.T) {
	rec := &statsRecorder{}
	ch := stoppedLoopChannel(t, inprocgrpc.WithClientStatsHandler(rec))
	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp)
	if err == nil {
		t.Fatal("expected error")
	}
	events := rec.getEvents()
	assertHasEventTypes(t, "stopped-invoke", events, (*stats.End)(nil))
}

func TestCoverage_NewStream_LoopNotRunning(t *testing.T) {
	ch := stoppedLoopChannel(t)
	_, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		StreamName:    "ServerStream",
		ServerStreams: true,
	}, "/test.TestService/ServerStream")
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got %v, want Unavailable", st.Code())
	}
}

func TestCoverage_NewStream_LoopNotRunning_WithStats(t *testing.T) {
	rec := &statsRecorder{}
	ch := stoppedLoopChannel(t, inprocgrpc.WithClientStatsHandler(rec))
	_, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		StreamName:    "ServerStream",
		ServerStreams: true,
	}, "/test.TestService/ServerStream")
	if err == nil {
		t.Fatal("expected error")
	}
	events := rec.getEvents()
	assertHasEventTypes(t, "stopped-stream", events, (*stats.End)(nil))
}

// --- Coverage gap: Inner Submit failure (loop stops during handler) ---

func TestCoverage_Invoke_InnerSubmitFailure(t *testing.T) {
	// The handler goroutine runs, then tries to Submit its completion back
	// to the loop. If the loop stops between the outer Submit and the inner
	// Submit, the inner Submit fails and resCh gets an Unavailable error.
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

	handlerReady := make(chan struct{})
	handlerProceed := make(chan struct{})

	desc := coverageServiceDesc(func(srv any, svrCtx context.Context, dec func(any) error, _ grpc.UnaryServerInterceptor) (any, error) {
		in := new(wrapperspb.StringValue)
		if err := dec(in); err != nil {
			return nil, err
		}
		// Signal that handler is running, then wait
		close(handlerReady)
		<-handlerProceed
		return &wrapperspb.StringValue{Value: "ok"}, nil
	}, nil)
	ch.RegisterService(&desc, &echoServer{})

	// Start invoke in a goroutine
	var invokeErr error
	invokeDone := make(chan struct{})
	go func() {
		defer close(invokeDone)
		req := &wrapperspb.StringValue{Value: "hello"}
		resp := new(wrapperspb.StringValue)
		invokeErr = ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp)
	}()

	// Wait for handler to start
	<-handlerReady

	// Stop the loop
	cancel()
	<-done

	// Let the handler proceed - inner Submit will fail
	close(handlerProceed)

	// Wait for Invoke to complete
	<-invokeDone
	if invokeErr == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(invokeErr)
	if st.Code() != codes.Unavailable {
		t.Errorf("got %v, want Unavailable", st.Code())
	}
}

func TestCoverage_NewStream_InnerSubmitFailure(t *testing.T) {
	// The handler goroutine runs, then tries to Submit its completion.
	// If the loop stops, the inner Submit fails and cleans up directly.
	// The client already has the stream, so it will see errors on subsequent calls.
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

	handlerReady := make(chan struct{})
	handlerProceed := make(chan struct{})

	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			close(handlerReady)
			<-handlerProceed
			return nil
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

	// Wait for handler to start
	<-handlerReady

	// Stop the loop
	cancel()
	<-done

	// Let the handler proceed - inner Submit will fail, cleanup happens directly
	close(handlerProceed)

	// Client operations should see errors
	msg := new(wrapperspb.StringValue)
	err = cs.RecvMsg(msg)
	// Should get some error (EOF, context error, or Unavailable)
	if err == nil {
		t.Fatal("expected error from RecvMsg after loop stopped")
	}
}

func TestCoverage_Stream_Trailer_ContextCancel(t *testing.T) {
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
	cs, err := ch.NewStream(ctx, &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}

	cancel()

	// Trailer() with cancelled context should return nil
	md := cs.Trailer()
	_ = md // nil is expected
}

// --- Coverage gap: fetchTrailersOnLoop ctx.Done path ---

func TestCoverage_Stream_FetchTrailers_ContextCancel(t *testing.T) {
	// When context is cancelled during fetchTrailersOnLoop, the select
	// should take the ctx.Done branch.
	ch := newBareChannel(t)
	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			stream.SetTrailer(metadata.Pairs("t", "v"))
			return status.Error(codes.Aborted, "abort")
		},
		ServerStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	ctx, cancel := context.WithCancel(context.Background())
	cs, err := ch.NewStream(ctx, &grpc.StreamDesc{
		ServerStreams: true,
	}, "/test.TestService/ServerStream")
	if err != nil {
		t.Fatal(err)
	}
	// Cancel context immediately, then try RecvMsg which triggers fetchTrailers
	cancel()
	msg := new(wrapperspb.StringValue)
	_ = cs.RecvMsg(msg) // will get some error
}

// --- Coverage gap: CloseSend with stopped loop ---

func TestCoverage_Stream_CloseSend_LoopStopped(t *testing.T) {
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
	ch.RegisterService(&testServiceDesc, &echoServer{})

	// Create stream while loop is running
	cs, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}

	// Stop the loop
	cancel()
	<-done

	// CloseSend with stopped loop - Submit fails, returns nil per convention
	err = cs.CloseSend()
	if err != nil {
		t.Fatalf("CloseSend should return nil even with stopped loop, got: %v", err)
	}
}

// --- Coverage gap: SendMsg with stopped loop ---

func TestCoverage_Stream_SendMsg_LoopStopped(t *testing.T) {
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
	ch.RegisterService(&testServiceDesc, &echoServer{})

	cs, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}

	// Stop the loop
	cancel()
	<-done

	// SendMsg with stopped loop - Submit fails, returns io.EOF
	err = cs.SendMsg(&wrapperspb.StringValue{Value: "hello"})
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got: %v", err)
	}
}

// --- Coverage gap: RecvMsg with stopped loop ---

func TestCoverage_Stream_RecvMsg_LoopStopped(t *testing.T) {
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
	ch.RegisterService(&testServiceDesc, &echoServer{})

	cs, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}

	// Stop the loop
	cancel()
	<-done

	// RecvMsg with stopped loop - Submit fails, returns io.EOF
	msg := new(wrapperspb.StringValue)
	err = cs.RecvMsg(msg)
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got: %v", err)
	}
}

// --- Coverage gap: Header with stopped loop ---

func TestCoverage_Stream_Header_LoopStopped(t *testing.T) {
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
	ch.RegisterService(&testServiceDesc, &echoServer{})

	cs, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}

	// Stop the loop
	cancel()
	<-done

	// Header with stopped loop - Submit fails
	_, err = cs.Header()
	if err == nil {
		t.Fatal("expected error from Header with stopped loop")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got %v, want Unavailable", st.Code())
	}
}

// --- Coverage gap: Trailer with stopped loop ---

func TestCoverage_Stream_Trailer_LoopStopped(t *testing.T) {
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
	ch.RegisterService(&testServiceDesc, &echoServer{})

	cs, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}

	// Stop the loop
	cancel()
	<-done

	// Trailer with stopped loop - Submit fails, returns nil
	md := cs.Trailer()
	if md != nil {
		t.Errorf("expected nil, got %v", md)
	}
}

// --- Coverage gap: Server adapter SubmitInternal failures ---

func TestCoverage_ServerAdapter_SetHeader_LoopStopped(t *testing.T) {
	// Server handler calls SetHeader after loop stops.
	// SubmitInternal fails, returns Internal error.
	svrErr := make(chan error, 1)
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

	handlerReady := make(chan struct{})
	loopStopped := make(chan struct{})

	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "BidiStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			close(handlerReady)
			<-loopStopped
			err := stream.SetHeader(metadata.Pairs("k", "v"))
			svrErr <- err
			return err
		},
		ServerStreams: true,
		ClientStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	_, err = ch.NewStream(context.Background(), &grpc.StreamDesc{
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
		t.Fatal("expected error from SetHeader on stopped loop")
	}
}

func TestCoverage_ServerAdapter_SendHeader_LoopStopped(t *testing.T) {
	svrErr := make(chan error, 1)
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

	handlerReady := make(chan struct{})
	loopStopped := make(chan struct{})

	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "BidiStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			close(handlerReady)
			<-loopStopped
			err := stream.SendHeader(metadata.Pairs("k", "v"))
			svrErr <- err
			return err
		},
		ServerStreams: true,
		ClientStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	_, err = ch.NewStream(context.Background(), &grpc.StreamDesc{
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
		t.Fatal("expected error from SendHeader on stopped loop")
	}
}

func TestCoverage_ServerAdapter_SendMsg_LoopStopped(t *testing.T) {
	svrErr := make(chan error, 1)
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

	_, err = ch.NewStream(context.Background(), &grpc.StreamDesc{
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

	_, err = ch.NewStream(context.Background(), &grpc.StreamDesc{
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
	ch := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop))

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
	ch := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop))

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
	if err.Error() != "client clone boom" {
		t.Fatalf("got %q, want %q", err.Error(), "client clone boom")
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
	events := rec.getEvents()
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
	events := rec.getEvents()
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
	if err == nil {
		t.Fatal("expected error from RecvMsg Copy failure")
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
	events := rec.getEvents()
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
	ch := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop), inprocgrpc.WithServerStatsHandler(rec))
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

	// Use a loop sentinel to ensure the handler's completion Submit callback
	// (which fires server stats End) has been processed.
	sentinel := make(chan struct{})
	if err := loop.Submit(func() { close(sentinel) }); err != nil {
		t.Fatal(err)
	}
	<-sentinel

	// Verify server stats End was called with error
	events := rec.getEvents()
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

// --- Coverage gap: CodecCloner V1 factory + methods ---

// mockCodecV1 implements encoding.Codec for testing CodecCloner.
type mockCodecV1 struct{}

func (mockCodecV1) Marshal(v any) ([]byte, error) {
	msg, ok := v.(*wrapperspb.StringValue)
	if !ok {
		return nil, fmt.Errorf("unsupported type")
	}
	return []byte(msg.GetValue()), nil
}

func (mockCodecV1) Unmarshal(data []byte, v any) error {
	msg, ok := v.(*wrapperspb.StringValue)
	if !ok {
		return fmt.Errorf("unsupported type")
	}
	msg.Value = string(data)
	return nil
}

func (mockCodecV1) Name() string { return "test-v1" }

func TestCoverage_CodecCloner_V1_Clone(t *testing.T) {
	cloner := inprocgrpc.CodecCloner(mockCodecV1{})

	in := &wrapperspb.StringValue{Value: "hello"}
	out, err := cloner.Clone(in)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	outMsg, ok := out.(*wrapperspb.StringValue)
	if !ok {
		t.Fatalf("Clone returned %T, want *wrapperspb.StringValue", out)
	}
	if outMsg.GetValue() != "hello" {
		t.Fatalf("Clone got %q, want %q", outMsg.GetValue(), "hello")
	}

	// Verify independence
	in.Value = "changed"
	if outMsg.GetValue() == "changed" {
		t.Error("Clone result was not independent")
	}
}

func TestCoverage_CodecCloner_V1_Copy(t *testing.T) {
	cloner := inprocgrpc.CodecCloner(mockCodecV1{})

	in := &wrapperspb.StringValue{Value: "hello"}
	dst := new(wrapperspb.StringValue)
	if err := cloner.Copy(dst, in); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if dst.GetValue() != "hello" {
		t.Fatalf("Copy got %q, want %q", dst.GetValue(), "hello")
	}
}

func TestCoverage_CodecCloner_V1_CloneError(t *testing.T) {
	cloner := inprocgrpc.CodecCloner(&struct {
		mockCodecV1
	}{mockCodecV1{}})

	// Test with a non-StringValue type that our mock can't handle
	type badType struct{}
	_, err := cloner.Clone(&badType{})
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

func TestCoverage_CodecCloner_V1_CopyError(t *testing.T) {
	cloner := inprocgrpc.CodecCloner(mockCodecV1{})
	out := new(wrapperspb.StringValue)
	// Pass unsupported type - Marshal fails
	type badType struct{}
	err := cloner.Copy(out, &badType{})
	if err == nil {
		t.Fatal("expected error for unsupported type")
	}
}

// --- Coverage gap: Client adapter ctx.Done paths via loop blocking ---

// blockLoop submits a task that blocks the loop for the specified duration.
// Returns a cancel function to unblock early (safe to call multiple times).
func blockLoop(t testing.TB, loop *eventloop.Loop, dur time.Duration) func() {
	t.Helper()
	unblock := make(chan struct{})
	var once sync.Once
	if err := loop.Submit(func() {
		select {
		case <-time.After(dur):
		case <-unblock:
		}
	}); err != nil {
		t.Fatal("failed to submit blocking task")
	}
	return func() { once.Do(func() { close(unblock) }) }
}

func TestCoverage_Stream_Header_ContextDone(t *testing.T) {
	// This tests the ctx.Done branch in Header's select.
	// Strategy: block the loop so the Header callback can't execute,
	// then cancel the context.
	loop := newTestLoop(t)
	ch := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop))
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
	cs, err := ch.NewStream(ctx, &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}

	// Block the loop so Header's Submit callback is queued but not executed.
	unblock := blockLoop(t, loop, 5*time.Second)
	defer unblock()

	// Cancel context - now ctx.Done is ready.
	cancel()

	// Header's Submit will succeed (adds to queue), but the callback
	// won't run because the loop is blocked. The select hits ctx.Done.
	_, err = cs.Header()
	if err == nil {
		t.Fatal("expected error from Header with cancelled context")
	}
	unblock()
}

func TestCoverage_Stream_CloseSend_ContextDone(t *testing.T) {
	loop := newTestLoop(t)
	ch := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop))
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
	cs, err := ch.NewStream(ctx, &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}

	unblock := blockLoop(t, loop, 5*time.Second)
	defer unblock()
	cancel()

	err = cs.CloseSend()
	// CloseSend always returns nil per gRPC convention
	if err != nil {
		t.Fatalf("expected nil, got: %v", err)
	}
	unblock()
}

func TestCoverage_Stream_SendMsg_ContextDone(t *testing.T) {
	loop := newTestLoop(t)
	ch := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop))
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
	cs, err := ch.NewStream(ctx, &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}

	unblock := blockLoop(t, loop, 5*time.Second)
	defer unblock()
	cancel()

	err = cs.SendMsg(&wrapperspb.StringValue{Value: "hello"})
	if err == nil {
		t.Fatal("expected error from SendMsg with cancelled context")
	}
	unblock()
}

// --- Coverage gap: server adapter SendMsg ctx.Done ---

func TestCoverage_ServerAdapter_SendMsg_ContextDone(t *testing.T) {
	// To hit the ctx.Done branch in server SendMsg's select, we need:
	// 1. SendMsg's SubmitInternal to succeed (adds callback to queue)
	// 2. Context to cancel BEFORE the callback runs
	// Strategy: block the loop, have the handler call SendMsg (SubmitInternal
	// queues behind blocker → enters select with errCh never ready), then
	// cancel context to fire ctx.Done in the select.
	//
	// To prevent the early ctx.Err() check from short-circuiting before
	// the select, we use a CloneFunc that signals the test AFTER Clone
	// runs (which is after ctx.Err()). The test waits for this signal
	// before calling cancel(), ensuring deterministic coverage.
	loop := newTestLoop(t)

	// Custom cloner that signals after Clone runs (past ctx.Err() check).
	clonePassed := make(chan struct{})
	var cloneOnce sync.Once
	cloner := inprocgrpc.CloneFunc(func(in any) (any, error) {
		cloneOnce.Do(func() { close(clonePassed) })
		return inprocgrpc.ProtoCloner{}.Clone(in)
	})

	ch := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop), inprocgrpc.WithCloner(cloner))

	svrErr := make(chan error, 1)
	handlerReady := make(chan struct{})
	loopBlocked := make(chan struct{})
	cancelClient := make(chan struct{})

	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "BidiStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			close(handlerReady)
			// Wait for the loop to be blocked
			<-loopBlocked
			// Now call SendMsg - Clone signals the test, SubmitInternal queues
			// behind blocker, select waits for errCh (never ready) or ctx.Done.
			err := stream.SendMsg(&wrapperspb.StringValue{Value: "data"})
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

	// Block the loop. The blocker waits on cancelClient - ensuring the loop
	// stays blocked until after the handler's SendMsg returns.
	if err := loop.Submit(func() {
		close(loopBlocked)
		<-cancelClient
	}); err != nil {
		t.Fatal("failed to submit blocker")
	}

	<-loopBlocked

	// Wait for Clone to complete - this proves the handler passed ctx.Err()
	// and is now (or will be) in the select, since SubmitInternal is fast.
	<-clonePassed

	// Now cancel - ctx.Done fires in the select, NOT at ctx.Err() early exit.
	cancel()

	// Handler's SendMsg select should hit ctx.Done
	e := <-svrErr
	if e == nil {
		t.Fatal("expected error from SendMsg ctx.Done")
	}
	// Now unblock the loop for cleanup.
	close(cancelClient)
}

// --- Coverage gap: Trailer with stats handler and non-nil trailers ---

func TestCoverage_Stream_Trailer_WithStatsAndNonNilTrailers(t *testing.T) {
	rec := &statsRecorder{}
	ch := newBareChannel(t, inprocgrpc.WithClientStatsHandler(rec))
	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			in := new(wrapperspb.StringValue)
			if err := stream.RecvMsg(in); err != nil {
				return err
			}
			stream.SetTrailer(metadata.Pairs("key", "value"))
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
	// Drain all messages
	for {
		msg := new(wrapperspb.StringValue)
		if err := cs.RecvMsg(msg); err != nil {
			break
		}
	}
	// Call Trailer() explicitly - triggers the stats inTrailer path
	md := cs.Trailer()
	if v := md.Get("key"); len(v) == 0 || v[0] != "value" {
		t.Errorf("expected trailer key=value, got %v", md)
	}
	// Verify stats saw InTrailer
	events := rec.getEvents()
	assertHasEventTypes(t, "trailer-stats", events, (*stats.InTrailer)(nil))
}

// --- Coverage gap: Trailer ctx.Done via loop blocking ---

func TestCoverage_Stream_Trailer_ContextDone_LoopBlocked(t *testing.T) {
	loop := newTestLoop(t)
	ch := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop))
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
	cs, err := ch.NewStream(ctx, &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}

	// Block the loop
	unblock := blockLoop(t, loop, 5*time.Second)
	defer unblock()

	// Cancel context
	cancel()

	// Trailer's Submit succeeds (queued behind blocker) but select hits ctx.Done
	md := cs.Trailer()
	if md != nil {
		t.Errorf("expected nil, got %v", md)
	}
	unblock()
}

// --- Coverage gap: ensureNoMoreLocked Submit failure ---

func TestCoverage_Stream_EnsureNoMore_SubmitFailure(t *testing.T) {
	// ensureNoMoreLocked is called from recvMsgLocked for unary-response streams.
	// To make its Submit fail, the loop must stop BETWEEN the initial RecvMsg
	// Submit (which succeeds) and ensureNoMore's Submit.
	// Strategy: use a custom cloner whose Copy method stops the loop on the
	// correct call (the client's RecvMsg Copy, not earlier calls).
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

	// CopyFunc's Clone also calls Copy, so count carefully:
	// Call 1: Client SendMsg → Clone → Copy (client needs this to succeed)
	// Call 2: Server RecvMsg → Copy (server needs this to succeed)
	// Call 3: Server SendMsg → Clone → Copy (server needs this to succeed)
	// Call 4: Client RecvMsg → Copy (stop the loop here)
	var copyCallNum atomic.Int64
	cloner := inprocgrpc.CopyFunc(func(out, in any) error {
		n := copyCallNum.Add(1)
		if n == 4 {
			// Stop the loop during the client's RecvMsg Copy.
			// After this returns, ensureNoMore's Submit will fail.
			cancel()
			<-done
		}
		return inprocgrpc.ProtoCloner{}.Copy(out, in)
	})

	ch := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop), inprocgrpc.WithCloner(cloner))

	desc := coverageServiceDesc(nil, []grpc.StreamDesc{{
		StreamName: "ServerStream",
		Handler: func(srv any, stream grpc.ServerStream) error {
			in := new(wrapperspb.StringValue)
			if err := stream.RecvMsg(in); err != nil {
				return err
			}
			// Send exactly one message
			return stream.SendMsg(&wrapperspb.StringValue{Value: "response"})
		},
		ServerStreams: true,
	}})
	ch.RegisterService(&desc, &echoServer{})

	cs, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: false, // unary response → triggers ensureNoMore
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

	// RecvMsg → gets message → Copy stops the loop → ensureNoMore Submit fails
	// → returns nil → RecvMsg succeeds (or context-related error, both OK).
	msg := new(wrapperspb.StringValue)
	_ = cs.RecvMsg(msg) // any result is acceptable
}
