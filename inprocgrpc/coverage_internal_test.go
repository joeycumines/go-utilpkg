package inprocgrpc

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	eventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/go-inprocgrpc/internal/callopts"
	"github.com/joeycumines/go-inprocgrpc/internal/stream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding"
	grpcproto "google.golang.org/grpc/encoding/proto"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// mockStatsHandler records all stats events for verification.
type mockStatsHandler struct {
	mu     sync.Mutex
	events []stats.RPCStats
	tags   []*stats.RPCTagInfo
}

func (m *mockStatsHandler) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tags = append(m.tags, info)
	return context.WithValue(ctx, (*mockStatsHandler)(nil), "tagged")
}

func (m *mockStatsHandler) HandleRPC(_ context.Context, s stats.RPCStats) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, s)
}

func (m *mockStatsHandler) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}

func (m *mockStatsHandler) HandleConn(context.Context, stats.ConnStats) {}

var _ stats.Handler = (*mockStatsHandler)(nil)

// --- stats.go coverage: nil vs non-nil statsHandlerHelper ---

func TestStatsHandlerHelper_NilReceiver(t *testing.T) {
	// All 9 methods on a nil *statsHandlerHelper should be safe no-ops.
	var sh *statsHandlerHelper
	ctx := context.Background()

	// tagRPC should return ctx unchanged
	got := sh.tagRPC(ctx, "/test/Method")
	if got != ctx {
		t.Error("tagRPC on nil should return ctx unchanged")
	}

	// The remaining 8 void methods should not panic
	sh.begin(ctx, false, false)
	sh.begin(ctx, true, true)
	sh.end(ctx, nil)
	sh.end(ctx, errors.New("some error"))
	sh.inHeader(ctx, metadata.Pairs("k", "v"), "/m")
	sh.inPayload(ctx, "payload")
	sh.inTrailer(ctx, metadata.Pairs("t", "tv"))
	sh.outHeader(ctx, metadata.Pairs("h", "hv"))
	sh.outPayload(ctx, "payload")
	sh.outTrailer(ctx, metadata.Pairs("t", "tv"))
}

func TestStatsHandlerHelper_NonNil_AllMethods(t *testing.T) {
	mock := &mockStatsHandler{}
	sh := &statsHandlerHelper{handler: mock, isClient: true}
	ctx := context.Background()

	// tagRPC - should call handler.TagRPC and return the modified context
	ctx2 := sh.tagRPC(ctx, "/svc/Method")
	if ctx2 == ctx {
		t.Error("tagRPC should return a new context")
	}
	mock.mu.Lock()
	if len(mock.tags) != 1 {
		t.Errorf("expected 1 tag, got %d", len(mock.tags))
	} else if mock.tags[0].FullMethodName != "/svc/Method" {
		t.Errorf("tag method: %q", mock.tags[0].FullMethodName)
	}
	mock.mu.Unlock()

	// begin
	sh.begin(ctx2, true, false)
	assertLastEvent[*stats.Begin](t, mock, func(ev *stats.Begin) {
		if !ev.Client {
			t.Error("begin: Client should be true")
		}
		if !ev.IsClientStream {
			t.Error("begin: IsClientStream should be true")
		}
		if ev.IsServerStream {
			t.Error("begin: IsServerStream should be false")
		}
	})

	// end with error
	testErr := errors.New("test-err")
	sh.end(ctx2, testErr)
	assertLastEvent[*stats.End](t, mock, func(ev *stats.End) {
		if !ev.Client {
			t.Error("end: Client should be true")
		}
		if ev.Error != testErr {
			t.Errorf("end: Error = %v, want %v", ev.Error, testErr)
		}
	})

	// end without error
	sh.end(ctx2, nil)
	assertLastEvent[*stats.End](t, mock, func(ev *stats.End) {
		if ev.Error != nil {
			t.Errorf("end: Error should be nil, got %v", ev.Error)
		}
	})

	// inHeader
	hdr := metadata.Pairs("h", "v")
	sh.inHeader(ctx2, hdr, "/svc/Method")
	assertLastEvent[*stats.InHeader](t, mock, func(ev *stats.InHeader) {
		if !ev.Client {
			t.Error("inHeader: Client should be true")
		}
		if ev.FullMethod != "/svc/Method" {
			t.Errorf("inHeader: FullMethod = %q", ev.FullMethod)
		}
		if len(ev.Header.Get("h")) == 0 {
			t.Error("inHeader: missing header")
		}
	})

	// inPayload
	sh.inPayload(ctx2, "my-payload")
	assertLastEvent[*stats.InPayload](t, mock, func(ev *stats.InPayload) {
		if !ev.Client {
			t.Error("inPayload: Client should be true")
		}
		if ev.Payload != "my-payload" {
			t.Errorf("inPayload: Payload = %v", ev.Payload)
		}
	})

	// inTrailer
	tlr := metadata.Pairs("t", "tv")
	sh.inTrailer(ctx2, tlr)
	assertLastEvent[*stats.InTrailer](t, mock, func(ev *stats.InTrailer) {
		if !ev.Client {
			t.Error("inTrailer: Client should be true")
		}
		if len(ev.Trailer.Get("t")) == 0 {
			t.Error("inTrailer: missing trailer")
		}
	})

	// outHeader
	sh.outHeader(ctx2, hdr)
	assertLastEvent[*stats.OutHeader](t, mock, func(ev *stats.OutHeader) {
		if !ev.Client {
			t.Error("outHeader: Client should be true")
		}
	})

	// outPayload
	sh.outPayload(ctx2, "out-payload")
	assertLastEvent[*stats.OutPayload](t, mock, func(ev *stats.OutPayload) {
		if !ev.Client {
			t.Error("outPayload: Client should be true")
		}
		if ev.Payload != "out-payload" {
			t.Errorf("outPayload: Payload = %v", ev.Payload)
		}
	})

	// outTrailer
	sh.outTrailer(ctx2, tlr)
	assertLastEvent[*stats.OutTrailer](t, mock, func(ev *stats.OutTrailer) {
		if !ev.Client {
			t.Error("outTrailer: Client should be true")
		}
	})
}

func TestStatsHandlerHelper_NonNil_ServerSide(t *testing.T) {
	// Verify isClient=false propagation
	mock := &mockStatsHandler{}
	sh := &statsHandlerHelper{handler: mock, isClient: false}
	ctx := context.Background()

	sh.begin(ctx, false, true)
	assertLastEvent[*stats.Begin](t, mock, func(ev *stats.Begin) {
		if ev.Client {
			t.Error("begin: Client should be false for server")
		}
		if ev.IsClientStream {
			t.Error("begin: IsClientStream should be false")
		}
		if !ev.IsServerStream {
			t.Error("begin: IsServerStream should be true")
		}
	})

	sh.end(ctx, nil)
	assertLastEvent[*stats.End](t, mock, func(ev *stats.End) {
		if ev.Client {
			t.Error("end: Client should be false for server")
		}
	})

	sh.inPayload(ctx, "x")
	assertLastEvent[*stats.InPayload](t, mock, func(ev *stats.InPayload) {
		if ev.Client {
			t.Error("inPayload: Client should be false for server")
		}
	})

	sh.outPayload(ctx, "y")
	assertLastEvent[*stats.OutPayload](t, mock, func(ev *stats.OutPayload) {
		if ev.Client {
			t.Error("outPayload: Client should be false for server")
		}
	})
}

// assertLastEvent checks that the last recorded event in the mock is of type T
// and runs fn on it for additional verification.
func assertLastEvent[T stats.RPCStats](t *testing.T, mock *mockStatsHandler, fn func(T)) {
	t.Helper()
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.events) == 0 {
		t.Fatal("no events recorded")
	}
	last := mock.events[len(mock.events)-1]
	ev, ok := last.(T)
	if !ok {
		t.Fatalf("last event is %T, want %T", last, *new(T))
	}
	fn(ev)
}

// --- ProtoCloner non-proto fallback paths ---

// nonProtoMsg is a simple struct that does NOT implement proto.Message.
type nonProtoMsg struct {
	Value string
}

func TestProtoCloner_Clone_NonProto_CodecFallback(t *testing.T) {
	// ProtoCloner.Clone for non-proto should fall back to codecClonerV2.
	// Since the proto codec cannot marshal a random struct, this exercises
	// the codec branch. The proto codec will error, which is expected.
	c := ProtoCloner{}
	_, err := c.Clone(&nonProtoMsg{Value: "hello"})
	if err == nil {
		t.Fatal("expected error cloning non-proto with proto codec")
	}
	// The error comes from the codec trying to marshal a non-proto type.
	// This confirms the fallback path was taken.
}

func TestProtoCloner_Copy_NonProto_CodecFallback(t *testing.T) {
	c := ProtoCloner{}
	src := &nonProtoMsg{Value: "hello"}
	dst := &nonProtoMsg{}
	err := c.Copy(dst, src)
	if err == nil {
		t.Fatal("expected error copying non-proto with proto codec")
	}
}

func TestProtoCloner_Copy_MixedTypes(t *testing.T) {
	// One proto and one non-proto - exercises the inOk!=outOk branch.
	c := ProtoCloner{}

	// proto in, non-proto out
	err := c.Copy(&nonProtoMsg{}, &wrapperspb.StringValue{Value: "x"})
	if err == nil {
		t.Fatal("expected error with mixed types (proto in, non-proto out)")
	}

	// non-proto in, proto out
	err = c.Copy(&wrapperspb.StringValue{}, &nonProtoMsg{Value: "x"})
	if err == nil {
		t.Fatal("expected error with mixed types (non-proto in, proto out)")
	}
}

// --- CloneFunc error path ---

func TestCloneFunc_ErrorPath(t *testing.T) {
	expectedErr := errors.New("clone failed")
	c := CloneFunc(func(in any) (any, error) {
		return nil, expectedErr
	})

	// Clone itself should return the error
	_, err := c.Clone(&wrapperspb.StringValue{Value: "x"})
	if !errors.Is(err, expectedErr) {
		t.Errorf("Clone: got %v, want %v", err, expectedErr)
	}

	// Copy (derived from Clone) should also propagate the error
	dst := new(wrapperspb.StringValue)
	err = c.Copy(dst, &wrapperspb.StringValue{Value: "x"})
	if !errors.Is(err, expectedErr) {
		t.Errorf("Copy: got %v, want %v", err, expectedErr)
	}
}

// --- CopyFunc error path ---

func TestCopyFunc_ErrorPath(t *testing.T) {
	expectedErr := errors.New("copy failed")
	c := CopyFunc(func(out, in any) error {
		return expectedErr
	})

	// Copy itself should return the error
	dst := new(wrapperspb.StringValue)
	err := c.Copy(dst, &wrapperspb.StringValue{Value: "x"})
	if !errors.Is(err, expectedErr) {
		t.Errorf("Copy: got %v, want %v", err, expectedErr)
	}

	// Clone (derived from Copy) should also propagate the error
	_, err = c.Clone(&wrapperspb.StringValue{Value: "x"})
	if !errors.Is(err, expectedErr) {
		t.Errorf("Clone: got %v, want %v", err, expectedErr)
	}
}

// --- CodecCloner (v1) coverage ---

func TestCodecClonerV1_Clone_Error(t *testing.T) {
	codec := encoding.GetCodec(grpcproto.Name)
	if codec == nil {
		t.Skip("proto v1 codec not available")
	}
	c := CodecCloner(codec)
	// Attempt to clone a non-proto type - should fail at marshal
	_, err := c.Clone(&nonProtoMsg{Value: "x"})
	if err == nil {
		t.Fatal("expected error cloning non-proto with v1 codec")
	}
}

func TestCodecClonerV1_Copy_Error(t *testing.T) {
	codec := encoding.GetCodec(grpcproto.Name)
	if codec == nil {
		t.Skip("proto v1 codec not available")
	}
	c := CodecCloner(codec)
	err := c.Copy(&nonProtoMsg{}, &nonProtoMsg{Value: "x"})
	if err == nil {
		t.Fatal("expected error copying non-proto with v1 codec")
	}
}

// --- CodecClonerV2 coverage ---

func TestCodecClonerV2_Clone_Error(t *testing.T) {
	codec := encoding.GetCodecV2(grpcproto.Name)
	if codec == nil {
		t.Skip("proto v2 codec not available")
	}
	c := CodecClonerV2(codec)
	_, err := c.Clone(&nonProtoMsg{Value: "x"})
	if err == nil {
		t.Fatal("expected error cloning non-proto with v2 codec")
	}
}

func TestCodecClonerV2_Copy_Error(t *testing.T) {
	codec := encoding.GetCodecV2(grpcproto.Name)
	if codec == nil {
		t.Skip("proto v2 codec not available")
	}
	c := CodecClonerV2(codec)
	err := c.Copy(&nonProtoMsg{}, &nonProtoMsg{Value: "x"})
	if err == nil {
		t.Fatal("expected error copying non-proto with v2 codec")
	}
}

// --- codecClonerV1 Copy/Clone success paths ---

func TestCodecClonerV1_RoundTrip(t *testing.T) {
	codec := encoding.GetCodec(grpcproto.Name)
	if codec == nil {
		t.Skip("proto v1 codec not available")
	}
	c := codecClonerV1{codec: codec}

	orig := &wrapperspb.StringValue{Value: "round-trip"}

	// Clone
	cloned, err := c.Clone(orig)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if cloned.(*wrapperspb.StringValue).GetValue() != "round-trip" {
		t.Errorf("Clone value: %q", cloned.(*wrapperspb.StringValue).GetValue())
	}

	// Copy
	dst := new(wrapperspb.StringValue)
	if err := c.Copy(dst, orig); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if dst.GetValue() != "round-trip" {
		t.Errorf("Copy value: %q", dst.GetValue())
	}

	// Verify independence
	cloned.(*wrapperspb.StringValue).Value = "mutated"
	dst.Value = "mutated2"
	if orig.GetValue() != "round-trip" {
		t.Error("original was mutated")
	}
}

// --- codecClonerV2 Copy/Clone success + error paths ---

func TestCodecClonerV2_RoundTrip(t *testing.T) {
	codec := encoding.GetCodecV2(grpcproto.Name)
	if codec == nil {
		t.Skip("proto v2 codec not available")
	}
	c := codecClonerV2{codec: codec}

	orig := &wrapperspb.Int64Value{Value: 42}

	// Clone
	cloned, err := c.Clone(orig)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if cloned.(*wrapperspb.Int64Value).GetValue() != 42 {
		t.Errorf("Clone value: %d", cloned.(*wrapperspb.Int64Value).GetValue())
	}

	// Copy
	dst := new(wrapperspb.Int64Value)
	if err := c.Copy(dst, orig); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if dst.GetValue() != 42 {
		t.Errorf("Copy value: %d", dst.GetValue())
	}
}

// --- funcCloner Clone/Copy delegation ---

func TestFuncCloner_CloneAndCopy(t *testing.T) {
	c := funcCloner{
		cloneFn: func(in any) (any, error) {
			// Return a copy made by reflect
			v := reflect.New(reflect.TypeOf(in).Elem())
			v.Elem().Set(reflect.ValueOf(in).Elem())
			return v.Interface(), nil
		},
		copyFn: func(out, in any) error {
			reflect.ValueOf(out).Elem().Set(reflect.ValueOf(in).Elem())
			return nil
		},
	}

	orig := &wrapperspb.StringValue{Value: "test"}
	cloned, err := c.Clone(orig)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if cloned.(*wrapperspb.StringValue).GetValue() != "test" {
		t.Error("clone mismatch")
	}

	dst := new(wrapperspb.StringValue)
	if err := c.Copy(dst, orig); err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if dst.GetValue() != "test" {
		t.Error("copy mismatch")
	}
}

// --- ProtoCloner fallback: no codec available ---

func TestProtoCloner_Clone_NonProto_NoCodec(t *testing.T) {
	// Override getCodecV2 to return nil, simulating no proto codec.
	old := getCodecV2
	getCodecV2 = func(string) encoding.CodecV2 { return nil }
	defer func() { getCodecV2 = old }()

	_, err := ProtoCloner{}.Clone(&nonProtoMsg{Value: "x"})
	if err == nil || !strings.Contains(err.Error(), "no codec found") {
		t.Fatalf("expected 'no codec found' error, got: %v", err)
	}
}

func TestProtoCloner_Copy_NonProto_NoCodec(t *testing.T) {
	old := getCodecV2
	getCodecV2 = func(string) encoding.CodecV2 { return nil }
	defer func() { getCodecV2 = old }()

	err := ProtoCloner{}.Copy(&nonProtoMsg{}, &nonProtoMsg{Value: "x"})
	if err == nil || !strings.Contains(err.Error(), "no codec found") {
		t.Fatalf("expected 'no codec found' error, got: %v", err)
	}
}

// --- fetchTrailersOnLoop: Submit failure (loop stopped) ---

func TestFetchTrailersOnLoop_SubmitFailure(t *testing.T) {
	// Create and stop a loop.
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
	<-done // loop is now stopped

	// Create a clientStreamAdapter with the stopped loop.
	adapter := &clientStreamAdapter{
		ctx:   context.Background(),
		loop:  loop,
		state: &stream.RPCState{},
		copts: callopts.GetCallOptions(nil),
	}

	// fetchTrailersOnLoop's Submit fails → just returns.
	adapter.fetchTrailersOnLoop() // should not panic
}

// --- fetchTrailersOnLoop: ctx.Done path ---

func TestFetchTrailersOnLoop_ContextDone(t *testing.T) {
	// Create a running loop but block it, so the Submit callback is queued.
	// Cancel the adapter's context so the select hits ctx.Done.
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	loopCtx := t.Context()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = loop.Run(loopCtx)
	}()

	// Block the loop.
	blocker := make(chan struct{})
	if err := loop.Submit(func() {
		<-blocker
	}); err != nil {
		t.Fatal(err)
	}

	// Create adapter with a cancelled context.
	adapterCtx, adapterCancel := context.WithCancel(context.Background())
	adapterCancel() // cancel immediately

	adapter := &clientStreamAdapter{
		ctx:   adapterCtx,
		loop:  loop,
		state: &stream.RPCState{},
		copts: callopts.GetCallOptions(nil),
	}

	// fetchTrailersOnLoop: Submit succeeds (queued behind blocker), but
	// ctx.Done is ready in the select → goes through the ctx.Done case.
	adapter.fetchTrailersOnLoop()

	// Unblock the loop for cleanup.
	close(blocker)
}

// --- Header() error-from-waiter path (clientstreamadapter.go) ---

func TestClientStreamAdapter_Header_ErrorFromWaiter(t *testing.T) {
	// Covers the r.err != nil branch in clientStreamAdapter.Header().
	// Uses internal access to state.HeaderWaiter for deterministic
	// ordering - no timing dependency.
	loop, err := eventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	loopCtx, loopCancel := context.WithCancel(context.Background())
	defer loopCancel()
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(loopCtx)
	}()

	state := &stream.RPCState{Method: "/test/Method"}
	adapter := &clientStreamAdapter{
		ctx:   context.Background(),
		loop:  loop,
		state: state,
		copts: callopts.GetCallOptions(nil),
	}

	// Start Header() - it will Submit to the loop to register HeaderWaiter.
	headerDone := make(chan struct{})
	var headerErr error
	go func() {
		defer close(headerDone)
		_, headerErr = adapter.Header()
	}()

	// Poll on the loop until HeaderWaiter is registered, then call
	// FinishWithTrailers with an error. This is deterministic because
	// each poll callback runs on the loop goroutine, and once the
	// Header goroutine's Submit callback registers HeaderWaiter, the
	// next poll sees it and delivers the error synchronously - no race.
	// The HeadersSent guard ensures previously-queued polls stop after
	// FinishWithTrailers has already fired.
	var poll func()
	poll = func() {
		if err := loop.Submit(func() {
			if state.HeaderWaiter != nil {
				// Waiter is registered - deliver the error.
				state.FinishWithTrailers(status.Error(codes.Internal, "no headers for you"))
				return
			}
			if state.HeadersSent {
				// Already finished - stop polling.
				return
			}
			// Not yet - re-poll.
			poll()
		}); err != nil {
			// Loop is terminating - expected during cleanup.
			return
		}
	}
	poll()

	<-headerDone
	if headerErr == nil {
		t.Fatal("expected error from Header when waiter receives error")
	}
	st, ok := status.FromError(headerErr)
	if !ok {
		t.Fatalf("expected status error, got %v", headerErr)
	}
	if st.Code() != codes.Internal {
		t.Errorf("got code %v, want Internal", st.Code())
	}

	// Ensure the loop goroutine exits cleanly.
	loopCancel()
	<-loopDone
}
