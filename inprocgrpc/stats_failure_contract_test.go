package inprocgrpc_test

import (
	"context"
	"io"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type panicStatsHandler struct {
	event       string
	goexit      bool
	nilTag      bool
	typedNilTag bool
	calls       atomic.Int32
	beginCalls  atomic.Int32
	endCalls    atomic.Int32
}

func (h *panicStatsHandler) TagRPC(
	ctx context.Context,
	_ *stats.RPCTagInfo,
) context.Context {
	if h.event == "TagRPC" {
		h.calls.Add(1)
		if h.nilTag {
			return nil
		}
		if h.typedNilTag {
			return (*nilStatsContext)(nil)
		}
		if h.goexit {
			runtime.Goexit()
		}
		panic("stats TagRPC failure")
	}
	return ctx
}

type nilStatsContext struct{}

func (*nilStatsContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (*nilStatsContext) Done() <-chan struct{}       { return nil }
func (*nilStatsContext) Err() error                  { return nil }
func (*nilStatsContext) Value(any) any               { return nil }

func (h *panicStatsHandler) HandleRPC(_ context.Context, event stats.RPCStats) {
	name := ""
	switch event.(type) {
	case *stats.Begin:
		name = "Begin"
		h.beginCalls.Add(1)
	case *stats.InHeader:
		name = "InHeader"
	case *stats.InPayload:
		name = "InPayload"
	case *stats.InTrailer:
		name = "InTrailer"
	case *stats.OutHeader:
		name = "OutHeader"
	case *stats.OutPayload:
		name = "OutPayload"
	case *stats.End:
		name = "End"
		h.endCalls.Add(1)
	}
	if name == h.event {
		h.calls.Add(1)
		if h.goexit {
			runtime.Goexit()
		}
		panic("stats " + name + " failure")
	}
}

func (*panicStatsHandler) TagConn(
	ctx context.Context,
	_ *stats.ConnTagInfo,
) context.Context {
	return ctx
}

func (*panicStatsHandler) HandleConn(context.Context, stats.ConnStats) {}

type recordingUnaryServer struct {
	echoServer
	received chan string
	err      error
}

type metadataUnaryServer struct {
	echoServer
}

func (*metadataUnaryServer) Unary(
	ctx context.Context,
	request *wrapperspb.StringValue,
) (*wrapperspb.StringValue, error) {
	if err := grpc.SendHeader(ctx, metadata.Pairs("header", "value")); err != nil {
		return nil, err
	}
	grpc.SetTrailer(ctx, metadata.Pairs("trailer", "value"))
	return &wrapperspb.StringValue{Value: request.GetValue()}, nil
}

func (s *recordingUnaryServer) Unary(
	_ context.Context,
	request *wrapperspb.StringValue,
) (*wrapperspb.StringValue, error) {
	if s.received != nil {
		select {
		case s.received <- request.GetValue():
		default:
		}
	}
	if s.err != nil {
		return nil, s.err
	}
	return &wrapperspb.StringValue{Value: request.GetValue()}, nil
}

func requireLoopUsable(t *testing.T, loop interface{ Submit(func()) error }) {
	t.Helper()
	done := make(chan struct{})
	if err := loop.Submit(func() { close(done) }); err != nil {
		t.Fatalf("submit loop sentinel: %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("loop sentinel did not execute")
	}
}

func TestStatsTagRPCAbnormalExitFailsConstruction(t *testing.T) {
	for _, side := range []string{"client", "server"} {
		for _, goexit := range []bool{false, true} {
			name := side + "/panic"
			if goexit {
				name = side + "/Goexit"
			}
			t.Run(name, func(t *testing.T) {
				loop := newTestLoop(t)
				handler := &panicStatsHandler{
					event:  "TagRPC",
					goexit: goexit,
				}
				options := []inprocgrpc.ChannelOption{inprocgrpc.WithLoop(loop)}
				if side == "client" {
					options = append(
						options,
						inprocgrpc.WithClientStatsHandler(handler),
					)
				} else {
					options = append(
						options,
						inprocgrpc.WithServerStatsHandler(handler),
					)
				}
				channel := mustNewChannel(t, options...)
				channel.RegisterService(&testServiceDesc, &echoServer{})
				err := channel.Invoke(
					context.Background(),
					"/test.TestService/Unary",
					new(wrapperspb.StringValue),
					new(wrapperspb.StringValue),
				)
				if status.Code(err) != codes.Internal {
					t.Fatalf("Invoke = %v, want Internal", err)
				}
				if got := handler.calls.Load(); got != 1 {
					t.Fatalf("TagRPC attempts = %d, want 1", got)
				}
				if got := handler.endCalls.Load(); got != 0 {
					t.Fatalf("End attempts = %d, want 0", got)
				}
				requireLoopUsable(t, loop)
			})
		}
	}
}

func TestStatsTagRPCNilContextFailsConstruction(t *testing.T) {
	for _, side := range []string{"client", "server"} {
		for _, typed := range []bool{false, true} {
			name := side + "/nil"
			if typed {
				name = side + "/typed nil"
			}
			t.Run(name, func(t *testing.T) {
				loop := newTestLoop(t)
				handler := &panicStatsHandler{
					event:       "TagRPC",
					nilTag:      !typed,
					typedNilTag: typed,
				}
				options := []inprocgrpc.ChannelOption{
					inprocgrpc.WithLoop(loop),
				}
				if side == "client" {
					options = append(
						options,
						inprocgrpc.WithClientStatsHandler(handler),
					)
				} else {
					options = append(
						options,
						inprocgrpc.WithServerStatsHandler(handler),
					)
				}
				channel := mustNewChannel(t, options...)
				channel.RegisterService(&testServiceDesc, &echoServer{})
				err := channel.Invoke(
					context.Background(),
					"/test.TestService/Unary",
					new(wrapperspb.StringValue),
					new(wrapperspb.StringValue),
				)
				statusValue := status.Convert(err)
				if statusValue.Code() != codes.Internal ||
					statusValue.Message() !=
						"stats TagRPC returned nil context" {
					t.Fatalf("Invoke = %v, want nil-context Internal", err)
				}
				if got := handler.calls.Load(); got != 1 {
					t.Fatalf("TagRPC attempts = %d, want 1", got)
				}
				if got := handler.beginCalls.Load(); got != 0 {
					t.Fatalf("Begin attempts = %d, want 0", got)
				}
				if got := handler.endCalls.Load(); got != 0 {
					t.Fatalf("End attempts = %d, want 0", got)
				}
				requireLoopUsable(t, loop)
			})
		}
	}
}

func TestStatsBeginAbnormalExitFailsInitialization(t *testing.T) {
	for _, side := range []string{"client", "server"} {
		for _, goexit := range []bool{false, true} {
			name := side + "/panic"
			if goexit {
				name = side + "/Goexit"
			}
			t.Run(name, func(t *testing.T) {
				loop := newTestLoop(t)
				handler := &panicStatsHandler{
					event:  "Begin",
					goexit: goexit,
				}
				options := []inprocgrpc.ChannelOption{
					inprocgrpc.WithLoop(loop),
				}
				if side == "client" {
					options = append(
						options,
						inprocgrpc.WithClientStatsHandler(handler),
					)
				} else {
					options = append(
						options,
						inprocgrpc.WithServerStatsHandler(handler),
					)
				}
				channel := mustNewChannel(t, options...)
				channel.RegisterService(&testServiceDesc, &echoServer{})

				ctx, cancel := context.WithTimeout(
					context.Background(),
					2*time.Second,
				)
				defer cancel()
				err := channel.Invoke(
					ctx,
					"/test.TestService/Unary",
					&wrapperspb.StringValue{Value: "request"},
					new(wrapperspb.StringValue),
				)
				if status.Code(err) != codes.Internal {
					t.Fatalf("Invoke = %v, want Internal", err)
				}
				if got := handler.calls.Load(); got != 1 {
					t.Fatalf("Begin abnormal calls = %d, want 1", got)
				}
				if got := handler.endCalls.Load(); got != 0 {
					t.Fatalf("End calls = %d, want 0", got)
				}
				requireLoopUsable(t, loop)
			})
		}
	}
}

func TestServerStatsInHeaderAbnormalExitStillBegins(t *testing.T) {
	for _, goexit := range []bool{false, true} {
		name := "panic"
		if goexit {
			name = "Goexit"
		}
		t.Run(name, func(t *testing.T) {
			loop := newTestLoop(t)
			handler := &panicStatsHandler{
				event:  "InHeader",
				goexit: goexit,
			}
			channel := mustNewChannel(
				t,
				inprocgrpc.WithLoop(loop),
				inprocgrpc.WithServerStatsHandler(handler),
			)
			channel.RegisterService(&testServiceDesc, &echoServer{})
			err := channel.Invoke(
				context.Background(),
				"/test.TestService/Unary",
				&wrapperspb.StringValue{Value: "request"},
				new(wrapperspb.StringValue),
			)
			if err != nil {
				t.Fatalf("Invoke = %v, want nil", err)
			}
			if got := handler.calls.Load(); got != 1 {
				t.Fatalf("InHeader attempts = %d, want 1", got)
			}
			if got := handler.beginCalls.Load(); got != 1 {
				t.Fatalf("Begin attempts = %d, want 1", got)
			}
			deadline := time.Now().Add(2 * time.Second)
			for handler.endCalls.Load() == 0 &&
				time.Now().Before(deadline) {
				runtime.Gosched()
			}
			if got := handler.endCalls.Load(); got != 1 {
				t.Fatalf("End attempts = %d, want 1", got)
			}
			requireLoopUsable(t, loop)
		})
	}
}

func TestServerStatsConstructionFailureMatchesClientEnd(t *testing.T) {
	for _, event := range []string{"TagRPC", "Begin"} {
		for _, goexit := range []bool{false, true} {
			name := event + "/panic"
			if goexit {
				name = event + "/Goexit"
			}
			t.Run(name, func(t *testing.T) {
				loop := newTestLoop(t)
				clientStats := new(statsRecorder)
				serverStats := &panicStatsHandler{
					event:  event,
					goexit: goexit,
				}
				channel := mustNewChannel(
					t,
					inprocgrpc.WithLoop(loop),
					inprocgrpc.WithClientStatsHandler(clientStats),
					inprocgrpc.WithServerStatsHandler(serverStats),
				)
				channel.RegisterService(&testServiceDesc, &echoServer{})
				err := channel.Invoke(
					context.Background(),
					"/test.TestService/Unary",
					new(wrapperspb.StringValue),
					new(wrapperspb.StringValue),
				)
				if status.Code(err) != codes.Internal {
					t.Fatalf("Invoke = %v, want Internal", err)
				}
				events := waitStatsEnd(t, clientStats)
				end := events[len(events)-1].(*stats.End)
				if end.Error == nil ||
					end.Error.Error() != err.Error() ||
					status.Code(end.Error) != status.Code(err) {
					t.Fatalf(
						"client End error = %v, want exact %v",
						end.Error,
						err,
					)
				}
				requireLoopUsable(t, loop)
			})
		}
	}
}

func TestClientStatsOutPayloadPanicPreservesAcceptedRequest(t *testing.T) {
	loop := newTestLoop(t)
	handler := &panicStatsHandler{event: "OutPayload"}
	channel := mustNewChannel(
		t,
		inprocgrpc.WithLoop(loop),
		inprocgrpc.WithClientStatsHandler(handler),
	)
	received := make(chan string, 1)
	channel.RegisterService(
		&testServiceDesc,
		&recordingUnaryServer{received: received},
	)

	err := channel.Invoke(
		context.Background(),
		"/test.TestService/Unary",
		&wrapperspb.StringValue{Value: "accepted"},
		new(wrapperspb.StringValue),
	)
	if err != nil {
		t.Fatalf("Invoke = %v, want nil", err)
	}
	select {
	case value := <-received:
		if value != "accepted" {
			t.Fatalf("received request = %q, want accepted", value)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("accepted request was not delivered after stats panic")
	}
	if got := handler.calls.Load(); got != 1 {
		t.Fatalf("OutPayload panic calls = %d, want 1", got)
	}
	requireLoopUsable(t, loop)
}

func TestClientInboundStatsAbnormalExitFailsReceive(t *testing.T) {
	for _, event := range []string{"InHeader", "InPayload", "InTrailer"} {
		for _, goexit := range []bool{false, true} {
			name := event + "/panic"
			if goexit {
				name = event + "/Goexit"
			}
			t.Run(name, func(t *testing.T) {
				loop := newTestLoop(t)
				handler := &panicStatsHandler{
					event:  event,
					goexit: goexit,
				}
				channel := mustNewChannel(
					t,
					inprocgrpc.WithLoop(loop),
					inprocgrpc.WithClientStatsHandler(handler),
				)
				channel.RegisterService(
					&testServiceDesc,
					new(metadataUnaryServer),
				)
				err := channel.Invoke(
					context.Background(),
					"/test.TestService/Unary",
					&wrapperspb.StringValue{Value: "request"},
					new(wrapperspb.StringValue),
				)
				if err != nil {
					t.Fatalf("Invoke = %v, want nil", err)
				}
				if got := handler.calls.Load(); got != 1 {
					t.Fatalf("%s abnormal calls = %d, want 1", event, got)
				}
				deadline := time.Now().Add(2 * time.Second)
				for handler.endCalls.Load() == 0 &&
					time.Now().Before(deadline) {
					runtime.Gosched()
				}
				if got := handler.endCalls.Load(); got != 1 {
					t.Fatalf("End calls = %d, want 1", got)
				}
				requireLoopUsable(t, loop)
			})
		}
	}
}

func TestUnaryServerStatsPanicSettlesInternal(t *testing.T) {
	for _, event := range []string{"OutHeader", "OutPayload"} {
		t.Run(event, func(t *testing.T) {
			loop := newTestLoop(t)
			handler := &panicStatsHandler{event: event}
			channel := mustNewChannel(
				t,
				inprocgrpc.WithLoop(loop),
				inprocgrpc.WithServerStatsHandler(handler),
			)
			channel.RegisterService(&testServiceDesc, &echoServer{})

			err := channel.Invoke(
				context.Background(),
				"/test.TestService/Unary",
				&wrapperspb.StringValue{Value: "request"},
				new(wrapperspb.StringValue),
			)
			if err != nil {
				t.Fatalf("Invoke = %v, want nil", err)
			}
			deadline := time.Now().Add(2 * time.Second)
			for handler.calls.Load() == 0 &&
				time.Now().Before(deadline) {
				runtime.Gosched()
			}
			if got := handler.calls.Load(); got != 1 {
				t.Fatalf("%s panic calls = %d, want 1", event, got)
			}
			requireLoopUsable(t, loop)
		})
	}
}

func TestStreamingServerStatsOutPayloadPanicPreservesAcceptedResponse(t *testing.T) {
	loop := newTestLoop(t)
	handler := &panicStatsHandler{event: "OutPayload"}
	channel := mustNewChannel(
		t,
		inprocgrpc.WithLoop(loop),
		inprocgrpc.WithServerStatsHandler(handler),
	)
	channel.RegisterService(&testServiceDesc, &echoServer{})

	stream, err := channel.NewStream(
		context.Background(),
		&grpc.StreamDesc{ServerStreams: true},
		"/test.TestService/ServerStream",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.SendMsg(&wrapperspb.StringValue{Value: "request"}); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	response := new(wrapperspb.StringValue)
	if err := stream.RecvMsg(response); err != nil {
		t.Fatalf("accepted response RecvMsg = %v, want nil", err)
	}
	if response.GetValue() != "request:0" {
		t.Fatalf("accepted response = %q, want request:0", response.GetValue())
	}
	for {
		err := stream.RecvMsg(new(wrapperspb.StringValue))
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("drain RecvMsg = %v, want nil or EOF", err)
		}
	}
	if got := handler.calls.Load(); got != 1 {
		t.Fatalf("OutPayload panic calls = %d, want 1", got)
	}
	requireLoopUsable(t, loop)
}

func TestServerStatsEndPanicPreservesSelectedStatus(t *testing.T) {
	for _, goexit := range []bool{false, true} {
		name := "panic"
		if goexit {
			name = "Goexit"
		}
		t.Run(name, func(t *testing.T) {
			loop := newTestLoop(t)
			handler := &panicStatsHandler{event: "End", goexit: goexit}
			channel := mustNewChannel(
				t,
				inprocgrpc.WithLoop(loop),
				inprocgrpc.WithServerStatsHandler(handler),
			)
			channel.RegisterService(
				&testServiceDesc,
				&recordingUnaryServer{
					err: status.Error(codes.PermissionDenied, "denied"),
				},
			)

			err := channel.Invoke(
				context.Background(),
				"/test.TestService/Unary",
				&wrapperspb.StringValue{Value: "request"},
				new(wrapperspb.StringValue),
			)
			if status.Code(err) != codes.PermissionDenied {
				t.Fatalf("Invoke = %v, want PermissionDenied", err)
			}
			deadline := time.Now().Add(2 * time.Second)
			for handler.calls.Load() == 0 &&
				time.Now().Before(deadline) {
				runtime.Gosched()
			}
			if got := handler.calls.Load(); got != 1 {
				t.Fatalf("End abnormal calls = %d, want 1", got)
			}
			requireLoopUsable(t, loop)
		})
	}
}

var _ stats.Handler = (*panicStatsHandler)(nil)
