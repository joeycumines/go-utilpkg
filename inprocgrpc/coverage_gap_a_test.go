package inprocgrpc_test

import (
	"context"

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

// --- Coverage gap: NewChannel with option error ---

func TestCoverage_NewChannel_OptionError(t *testing.T) {
	loop := newTestLoop(t)
	requirePanicContains(t, "stats handler", func() {
		inprocgrpc.NewChannel(
			inprocgrpc.WithLoop(loop),
			inprocgrpc.WithClientStatsHandler(nil),
		)
	})
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
	if status.Code(err) != codes.Internal || !strings.Contains(err.Error(), "clone boom") {
		t.Fatalf("Invoke = %v, want Internal clone failure", err)
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
	if status.Code(err) != codes.Internal || !strings.Contains(err.Error(), "copy response boom") {
		t.Fatalf("Invoke = %v, want Internal copy failure", err)
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
	events := waitStatsEnd(t, rec)
	assertHasEventTypes(t, "err-stats", events,
		(*stats.InHeader)(nil),
		(*stats.InTrailer)(nil),
		(*stats.End)(nil),
	)
}

// --- Coverage gap: Loop not running (outer Submit failure) ---

// stoppedLoopChannel creates a Channel whose loop has already stopped.
func stoppedLoopChannel(
	t testing.TB,
	opts ...inprocgrpc.ChannelOption,
) *inprocgrpc.Channel {
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

	opts = append(
		[]inprocgrpc.ChannelOption{inprocgrpc.WithLoop(loop)},
		opts...,
	)
	ch := mustNewChannel(t, opts...)
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
	events := waitStatsEnd(t, rec)
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
	events := waitStatsEnd(t, rec)
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

	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))

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

	// Stop the loop and wait for terminal cleanup to fully complete.
	cancel()
	<-done
	// Loop.Run returning does not imply Loop.Done() is closed (terminal
	// completion is published by an independent finisher). The handler's
	// later claim consults Loop.Done() to decide whether the scheduler
	// terminal must win; waiting here makes that decision deterministic
	// instead of racing the finisher.
	<-loop.Done()

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

	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))

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

	// Stop the loop and wait for terminal cleanup to fully complete.
	cancel()
	<-done
	// Loop.Run returning does not imply Loop.Done() is closed (terminal
	// completion is published by an independent finisher). The handler's
	// later claim consults Loop.Done() to decide whether the scheduler
	// terminal must win; waiting here makes that decision deterministic
	// instead of racing the finisher.
	<-loop.Done()

	// Let the handler proceed - inner Submit will fail, cleanup happens directly
	close(handlerProceed)

	// Scheduler termination precedes the handler's later terminal attempt.
	msg := new(wrapperspb.StringValue)
	err = cs.RecvMsg(msg)
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("RecvMsg after loop stopped = %v, want Unavailable", err)
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

	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))
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

	// CloseSend follows grpc.ClientStream's best-effort, always-nil contract.
	err = cs.CloseSend()
	if err != nil {
		t.Fatalf("CloseSend = %v, want nil", err)
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

	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))
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

	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))
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

	// RecvMsg with a stopped loop reports transport loss, not clean EOF.
	msg := new(wrapperspb.StringValue)
	err = cs.RecvMsg(msg)
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("RecvMsg = %v, want Unavailable", err)
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

	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))
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

	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))
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

	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))

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

	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))

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
