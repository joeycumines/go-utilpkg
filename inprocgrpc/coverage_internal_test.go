package inprocgrpc

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	eventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/go-inprocgrpc/internal/callopts"
	"github.com/joeycumines/go-inprocgrpc/internal/stream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding"
	grpcproto "google.golang.org/grpc/encoding/proto"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// mockStatsHandler records all stats events for verification.
type mockStatsHandler struct {
	mu        sync.Mutex
	events    []stats.RPCStats
	tags      []*stats.RPCTagInfo
	end       chan struct{}
	endPosted bool
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
	if _, ok := s.(*stats.End); ok && !m.endPosted {
		m.endPosted = true
		if m.end != nil {
			close(m.end)
		}
	}
}

func (m *mockStatsHandler) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}

func (m *mockStatsHandler) HandleConn(context.Context, stats.ConnStats) {}

var _ stats.Handler = (*mockStatsHandler)(nil)

func (m *mockStatsHandler) endSignal() <-chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.end == nil {
		m.end = make(chan struct{})
		if m.endPosted {
			close(m.end)
		}
	}
	return m.end
}

func waitMockStatsEnd(t *testing.T, handler *mockStatsHandler) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-handler.endSignal():
	case <-timer.C:
		t.Fatal("stats End was not published")
	}
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

	state := stream.NewRPCState("/test/Method", 1)
	adapter := &clientStreamAdapter{
		ctx:       context.Background(),
		callerCtx: context.Background(),
		loop:      loop,
		life:      newRPCLifecycle(loop, state, nil),
		state:     state,
		copts:     callopts.GetCallOptions(nil),
	}

	// Start Header() - it will Submit to the loop to register HeaderWaiter.
	headerDone := make(chan struct{})
	var headerErr error
	go func() {
		defer close(headerDone)
		_, headerErr = adapter.Header()
	}()

	// Poll on the loop until HeaderWaiter is registered, then call
	// Complete with an error. This is deterministic because
	// each poll callback runs on the loop goroutine, and once the
	// Header goroutine's Submit callback registers HeaderWaiter, the
	// next poll sees it and delivers the error synchronously - no race.
	// The HeadersSent guard ensures previously-queued polls stop after
	// Complete has already fired.
	var poll func()
	poll = func() {
		if err := loop.Submit(func() {
			if state.HeaderWaiter != nil {
				// Waiter is registered - deliver the error.
				state.Complete(status.Error(codes.Internal, "no headers for you"))
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
