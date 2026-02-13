package inprocgrpc_test

import (
	"context"
	"io"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/protobuf/types/known/wrapperspb"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
)

// statsRecorder records all stats events for verification.
type statsRecorder struct {
	mu     sync.Mutex
	events []stats.RPCStats
	tags   []stats.RPCTagInfo
}

func (r *statsRecorder) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tags = append(r.tags, *info)
	return ctx
}

func (r *statsRecorder) HandleRPC(ctx context.Context, s stats.RPCStats) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, s)
}

func (r *statsRecorder) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}

func (r *statsRecorder) HandleConn(context.Context, stats.ConnStats) {}

func (r *statsRecorder) getEvents() []stats.RPCStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]stats.RPCStats, len(r.events))
	copy(out, r.events)
	return out
}

var _ stats.Handler = (*statsRecorder)(nil)

func TestChannel_WithClientStatsHandler_Unary(t *testing.T) {
	recorder := &statsRecorder{}
	ch := newBareChannel(t, inprocgrpc.WithClientStatsHandler(recorder))
	ch.RegisterService(&testServiceDesc, &echoServer{})

	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	if err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp); err != nil {
		t.Fatal(err)
	}

	events := recorder.getEvents()
	if len(events) == 0 {
		t.Fatal("no stats events")
	}

	// Check for Begin, OutPayload, InPayload, End
	var hasBegin, hasOutPayload, hasInPayload, hasEnd bool
	for _, ev := range events {
		switch ev.(type) {
		case *stats.Begin:
			hasBegin = true
		case *stats.OutPayload:
			hasOutPayload = true
		case *stats.InPayload:
			hasInPayload = true
		case *stats.End:
			hasEnd = true
		}
	}
	if !hasBegin {
		t.Error("missing Begin event")
	}
	if !hasOutPayload {
		t.Error("missing OutPayload event")
	}
	if !hasInPayload {
		t.Error("missing InPayload event")
	}
	if !hasEnd {
		t.Error("missing End event")
	}

	// Check tags
	if len(recorder.tags) == 0 {
		t.Error("no tags")
	}
}

func TestChannel_WithServerStatsHandler_Unary(t *testing.T) {
	recorder := &statsRecorder{}
	ch := newBareChannel(t, inprocgrpc.WithServerStatsHandler(recorder))
	ch.RegisterService(&testServiceDesc, &echoServer{})

	req := &wrapperspb.StringValue{Value: "hello"}
	resp := new(wrapperspb.StringValue)
	if err := ch.Invoke(context.Background(), "/test.TestService/Unary", req, resp); err != nil {
		t.Fatal(err)
	}

	events := recorder.getEvents()
	var hasBegin, hasEnd bool
	for _, ev := range events {
		switch ev.(type) {
		case *stats.Begin:
			hasBegin = true
		case *stats.End:
			hasEnd = true
		}
	}
	if !hasBegin {
		t.Error("missing Begin event")
	}
	if !hasEnd {
		t.Error("missing End event")
	}
}

func TestChannel_WithClientStatsHandler_Stream(t *testing.T) {
	recorder := &statsRecorder{}
	ch := newBareChannel(t, inprocgrpc.WithClientStatsHandler(recorder))
	ch.RegisterService(&testServiceDesc, &echoServer{})

	stream, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		StreamName:    "ServerStream",
		ServerStreams: true,
	}, "/test.TestService/ServerStream")
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SendMsg(&wrapperspb.StringValue{Value: "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	for {
		resp := new(wrapperspb.StringValue)
		if err := stream.RecvMsg(resp); err != nil {
			break
		}
	}

	events := recorder.getEvents()
	if len(events) == 0 {
		t.Fatal("no stats events")
	}
	var hasBegin bool
	for _, ev := range events {
		if _, ok := ev.(*stats.Begin); ok {
			hasBegin = true
		}
	}
	if !hasBegin {
		t.Error("missing Begin event")
	}
}

func TestChannel_WithServerStatsHandler_Stream(t *testing.T) {
	recorder := &statsRecorder{}
	ch := newBareChannel(t, inprocgrpc.WithServerStatsHandler(recorder))
	ch.RegisterService(&testServiceDesc, &echoServer{})

	stream, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		StreamName:    "ServerStream",
		ServerStreams: true,
	}, "/test.TestService/ServerStream")
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SendMsg(&wrapperspb.StringValue{Value: "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	for {
		resp := new(wrapperspb.StringValue)
		if err := stream.RecvMsg(resp); err != nil {
			break
		}
	}

	events := recorder.getEvents()
	if len(events) == 0 {
		t.Fatal("no stats events")
	}
}

func TestChannel_NewStream_StreamHeaderExplicit(t *testing.T) {
	ch := newBareChannel(t)
	desc := testServiceDesc
	desc.Streams = []grpc.StreamDesc{
		{
			StreamName: "ServerStream",
			Handler: func(srv any, stream grpc.ServerStream) error {
				if err := stream.SetHeader(metadata.Pairs("h1", "v1")); err != nil {
					return err
				}
				if err := stream.SendHeader(metadata.Pairs("h2", "v2")); err != nil {
					return err
				}
				stream.SetTrailer(metadata.Pairs("t1", "tv1"))
				in := new(wrapperspb.StringValue)
				if err := stream.RecvMsg(in); err != nil {
					return err
				}
				return stream.SendMsg(&wrapperspb.StringValue{Value: "ok"})
			},
			ServerStreams: true,
		},
	}
	ch.RegisterService(&desc, &echoServer{})

	stream, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		ServerStreams: true,
	}, "/test.TestService/ServerStream")
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SendMsg(&wrapperspb.StringValue{Value: "hello"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}

	hdrs, err := stream.Header()
	if err != nil {
		t.Fatal(err)
	}
	if v := hdrs.Get("h1"); len(v) == 0 || v[0] != "v1" {
		t.Errorf("h1: %v", hdrs)
	}
	if v := hdrs.Get("h2"); len(v) == 0 || v[0] != "v2" {
		t.Errorf("h2: %v", hdrs)
	}

	resp := new(wrapperspb.StringValue)
	if err := stream.RecvMsg(resp); err != nil {
		t.Fatal(err)
	}

	// Read EOF
	if err := stream.RecvMsg(new(wrapperspb.StringValue)); err != io.EOF {
		t.Errorf("expected EOF, got %v", err)
	}

	tlrs := stream.Trailer()
	if v := tlrs.Get("t1"); len(v) == 0 || v[0] != "tv1" {
		t.Errorf("trailers: %v", tlrs)
	}
}

func TestChannel_NewStream_CloseSendIdempotent(t *testing.T) {
	ch := newTestChannel(t)
	stream, err := ch.NewStream(context.Background(), &grpc.StreamDesc{
		StreamName:    "BidiStream",
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	// Second close should also succeed (idempotent)
	if err := stream.CloseSend(); err != nil {
		t.Errorf("second CloseSend: %v", err)
	}
}

func TestChannel_NewStream_StreamContext(t *testing.T) {
	ch := newTestChannel(t)
	ctx := context.Background()
	stream, err := ch.NewStream(ctx, &grpc.StreamDesc{
		StreamName:    "BidiStream",
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.TestService/BidiStream")
	if err != nil {
		t.Fatal(err)
	}
	// Stream context should not be nil
	if stream.Context() == nil {
		t.Error("stream context is nil")
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
}
