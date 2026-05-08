package inprocgrpc_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
)

// statsRecorder records all stats events for verification.
type statsRecorder struct {
	mu        sync.Mutex
	events    []stats.RPCStats
	tags      []stats.RPCTagInfo
	end       chan struct{}
	endPosted bool
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
	if _, ok := s.(*stats.End); ok && !r.endPosted {
		r.endPosted = true
		if r.end != nil {
			close(r.end)
		}
	}
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

func (r *statsRecorder) endSignal() <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.end == nil {
		r.end = make(chan struct{})
		if r.endPosted {
			close(r.end)
		}
	}
	return r.end
}

func waitStatsEnd(t *testing.T, recorder *statsRecorder) []stats.RPCStats {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-recorder.endSignal():
		return recorder.getEvents()
	case <-timer.C:
		t.Fatal("stats End was not published")
	}
	return nil
}

func statsNames(events []stats.RPCStats) []string {
	names := make([]string, 0, len(events))
	for _, event := range events {
		switch event.(type) {
		case *stats.Begin:
			names = append(names, "Begin")
		case *stats.InHeader:
			names = append(names, "InHeader")
		case *stats.InPayload:
			names = append(names, "InPayload")
		case *stats.InTrailer:
			names = append(names, "InTrailer")
		case *stats.OutHeader:
			names = append(names, "OutHeader")
		case *stats.OutPayload:
			names = append(names, "OutPayload")
		case *stats.OutTrailer:
			names = append(names, "OutTrailer")
		case *stats.End:
			names = append(names, "End")
		default:
			names = append(names, "unknown")
		}
	}
	return names
}

func requireStatsNames(t *testing.T, events []stats.RPCStats, expected ...string) {
	t.Helper()
	actual := statsNames(events)
	if len(actual) != len(expected) {
		t.Fatalf("stats = %v, want %v", actual, expected)
	}
	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("stats = %v, want %v", actual, expected)
		}
	}
}

func TestClientLocalSendFailureStats(t *testing.T) {
	clientStats := new(statsRecorder)
	cloneErr := errors.New("clone request failed")
	channel := newBareChannel(
		t,
		inprocgrpc.WithClientStatsHandler(clientStats),
		inprocgrpc.WithCloner(&conditionalCloner{cloneErr: cloneErr}),
	)
	channel.RegisterService(&testServiceDesc, &echoServer{})
	client, err := channel.NewStream(
		context.Background(),
		&grpc.StreamDesc{ClientStreams: true, ServerStreams: true},
		"/test.TestService/BidiStream",
	)
	if err != nil {
		t.Fatal(err)
	}
	err = client.SendMsg(new(wrapperspb.StringValue))
	if status.Code(err) != codes.Internal || !errors.Is(err, cloneErr) {
		t.Fatalf("SendMsg = %v, want wrapped Internal", err)
	}
	events := waitStatsEnd(t, clientStats)
	requireStatsNames(t, events, "Begin", "OutHeader", "End")
	end := events[len(events)-1].(*stats.End)
	if status.Code(end.Error) != codes.Internal {
		t.Fatalf("End error = %v, want Internal", end.Error)
	}
	if len(end.Trailer) != 0 {
		t.Fatalf("End trailer = %v, want empty", end.Trailer)
	}
}

func TestTrailersOnlyServerErrorStats(t *testing.T) {
	clientStats := new(statsRecorder)
	serverStats := new(statsRecorder)
	channel := newBareChannel(
		t,
		inprocgrpc.WithClientStatsHandler(clientStats),
		inprocgrpc.WithServerStatsHandler(serverStats),
	)
	desc := coverageServiceDesc(
		func(
			_ any,
			ctx context.Context,
			decode func(any) error,
			_ grpc.UnaryServerInterceptor,
		) (any, error) {
			if err := decode(new(wrapperspb.StringValue)); err != nil {
				return nil, err
			}
			if err := grpc.SetTrailer(
				ctx,
				metadata.Pairs("result", "denied"),
			); err != nil {
				return nil, err
			}
			return nil, status.Error(codes.PermissionDenied, "denied")
		},
		nil,
	)
	channel.RegisterService(&desc, &echoServer{})
	err := channel.Invoke(
		context.Background(),
		"/test.TestService/Unary",
		new(wrapperspb.StringValue),
		new(wrapperspb.StringValue),
	)
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("Invoke = %v, want PermissionDenied", err)
	}
	clientEvents := waitStatsEnd(t, clientStats)
	requireStatsNames(
		t,
		clientEvents,
		"Begin",
		"OutHeader",
		"OutPayload",
		"InTrailer",
		"End",
	)
	clientEnd := clientEvents[len(clientEvents)-1].(*stats.End)
	if values := clientEnd.Trailer.Get("result"); len(values) != 1 || values[0] != "denied" {
		t.Fatalf("End trailer = %v", clientEnd.Trailer)
	}
	serverEvents := waitStatsEnd(t, serverStats)
	requireStatsNames(
		t,
		serverEvents,
		"InHeader",
		"Begin",
		"InPayload",
		"OutTrailer",
		"End",
	)
}

func TestInboundPayloadUsesCallerObjects(t *testing.T) {
	clientStats := new(statsRecorder)
	serverStats := new(statsRecorder)
	channel := newBareChannel(
		t,
		inprocgrpc.WithClientStatsHandler(clientStats),
		inprocgrpc.WithServerStatsHandler(serverStats),
	)
	var decoded *wrapperspb.StringValue
	desc := coverageServiceDesc(
		func(
			_ any,
			_ context.Context,
			decode func(any) error,
			_ grpc.UnaryServerInterceptor,
		) (any, error) {
			decoded = new(wrapperspb.StringValue)
			if err := decode(decoded); err != nil {
				return nil, err
			}
			return &wrapperspb.StringValue{Value: decoded.GetValue()}, nil
		},
		nil,
	)
	channel.RegisterService(&desc, &echoServer{})
	response := new(wrapperspb.StringValue)
	if err := channel.Invoke(
		context.Background(),
		"/test.TestService/Unary",
		&wrapperspb.StringValue{Value: "payload"},
		response,
	); err != nil {
		t.Fatal(err)
	}
	clientPayload := false
	for _, event := range waitStatsEnd(t, clientStats) {
		if payload, ok := event.(*stats.InPayload); ok {
			clientPayload = true
			if payload.Payload != response {
				t.Fatalf("client InPayload = %p, want response %p", payload.Payload, response)
			}
		}
	}
	if !clientPayload {
		t.Fatal("client InPayload was not published")
	}
	serverPayload := false
	for _, event := range waitStatsEnd(t, serverStats) {
		if payload, ok := event.(*stats.InPayload); ok {
			serverPayload = true
			if payload.Payload != decoded {
				t.Fatalf("server InPayload = %p, want decoded %p", payload.Payload, decoded)
			}
		}
	}
	if !serverPayload {
		t.Fatal("server InPayload was not published")
	}
}

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

	events := waitStatsEnd(t, recorder)
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

func TestUnaryStatsOrderAndCount(t *testing.T) {
	clientRecorder := &statsRecorder{}
	serverRecorder := &statsRecorder{}
	channel := newBareChannel(
		t,
		inprocgrpc.WithClientStatsHandler(clientRecorder),
		inprocgrpc.WithServerStatsHandler(serverRecorder),
	)
	channel.RegisterService(&testServiceDesc, &echoServer{})
	if err := channel.Invoke(
		context.Background(),
		"/test.TestService/Unary",
		&wrapperspb.StringValue{Value: "request"},
		new(wrapperspb.StringValue),
	); err != nil {
		t.Fatal(err)
	}

	assertStatsOrder(t, waitStatsEnd(t, clientRecorder), []string{
		"Begin",
		"OutHeader",
		"OutPayload",
		"InHeader",
		"InPayload",
		"InTrailer",
		"End",
	})
	assertStatsOrder(t, waitStatsEnd(t, serverRecorder), []string{
		"InHeader",
		"Begin",
		"InPayload",
		"OutHeader",
		"OutPayload",
		"OutTrailer",
		"End",
	})
}

func assertStatsOrder(t *testing.T, events []stats.RPCStats, want []string) {
	t.Helper()
	got := make([]string, 0, len(events))
	for _, event := range events {
		switch event.(type) {
		case *stats.Begin:
			got = append(got, "Begin")
		case *stats.End:
			got = append(got, "End")
		case *stats.InHeader:
			got = append(got, "InHeader")
		case *stats.InPayload:
			got = append(got, "InPayload")
		case *stats.InTrailer:
			got = append(got, "InTrailer")
		case *stats.OutHeader:
			got = append(got, "OutHeader")
		case *stats.OutPayload:
			got = append(got, "OutPayload")
		case *stats.OutTrailer:
			got = append(got, "OutTrailer")
		default:
			t.Fatalf("unexpected stats event %T", event)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("stats order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("stats order = %v, want %v", got, want)
		}
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
