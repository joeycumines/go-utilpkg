package inprocgrpc_test

import (
	"context"

	"fmt"

	"sync"
	"sync/atomic"
	"testing"
	"time"

	eventloop "github.com/joeycumines/go-eventloop"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/protobuf/types/known/wrapperspb"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
)

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
	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))
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
	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))
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
	if err != nil {
		t.Fatalf("CloseSend = %v, want nil", err)
	}
	unblock()
}

func TestCoverage_Stream_SendMsg_ContextDone(t *testing.T) {
	loop := newTestLoop(t)
	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))
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

	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop), inprocgrpc.WithCloner(cloner))

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
	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))
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
	loop := eventloop.New()
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

	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop), inprocgrpc.WithCloner(cloner))

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
