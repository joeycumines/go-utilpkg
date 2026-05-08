package inprocgrpc

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	eventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/go-inprocgrpc/internal/callopts"
	"github.com/joeycumines/go-inprocgrpc/internal/stream"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type barrierStatsHandler struct {
	entered chan struct{}
	release chan struct{}

	mu     sync.Mutex
	events []string
}

func (h *barrierStatsHandler) TagRPC(
	ctx context.Context,
	_ *stats.RPCTagInfo,
) context.Context {
	return ctx
}

func (h *barrierStatsHandler) HandleRPC(
	_ context.Context,
	event stats.RPCStats,
) {
	name := ""
	switch event.(type) {
	case *stats.Begin:
		name = "Begin"
	case *stats.OutHeader:
		name = "OutHeader"
	case *stats.OutPayload:
		name = "OutPayload"
	case *stats.OutTrailer:
		name = "OutTrailer"
	case *stats.End:
		name = "End"
	}
	h.mu.Lock()
	h.events = append(h.events, name)
	h.mu.Unlock()
	if name == "OutPayload" {
		close(h.entered)
		<-h.release
	}
}

func (*barrierStatsHandler) TagConn(
	ctx context.Context,
	_ *stats.ConnTagInfo,
) context.Context {
	return ctx
}

func (*barrierStatsHandler) HandleConn(context.Context, stats.ConnStats) {}

func (h *barrierStatsHandler) snapshot() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.events...)
}

type observableClientStream interface {
	grpc.ClientStream
	TerminalDone() <-chan struct{}
	TerminalResult() (error, bool)
	Done() <-chan struct{}
}

type reentrantClientStatsHandler struct {
	target   string
	action   func(observableClientStream) error
	entered  chan struct{}
	proceed  chan struct{}
	returned chan struct{}
	ended    chan struct{}

	mu          sync.Mutex
	client      observableClientStream
	result      error
	contextErr  error
	terminalErr error
	terminal    bool
	targetOnce  sync.Once
	proceedOnce sync.Once
	endOnce     sync.Once
}

func (h *reentrantClientStatsHandler) TagRPC(
	ctx context.Context,
	_ *stats.RPCTagInfo,
) context.Context {
	return ctx
}

func (h *reentrantClientStatsHandler) HandleRPC(
	_ context.Context,
	event stats.RPCStats,
) {
	name := ""
	switch event.(type) {
	case *stats.InHeader:
		name = "InHeader"
	case *stats.InPayload:
		name = "InPayload"
	case *stats.OutPayload:
		name = "OutPayload"
	case *stats.End:
		h.endOnce.Do(func() { close(h.ended) })
	}
	if name != h.target {
		return
	}
	h.targetOnce.Do(func() {
		close(h.entered)
		<-h.proceed
		h.mu.Lock()
		client := h.client
		h.mu.Unlock()
		result := h.action(client)
		contextErr := client.Context().Err()
		terminalErr, terminal := client.TerminalResult()
		h.mu.Lock()
		h.result = result
		h.contextErr = contextErr
		h.terminalErr = terminalErr
		h.terminal = terminal
		h.mu.Unlock()
		close(h.returned)
	})
}

func (*reentrantClientStatsHandler) TagConn(
	ctx context.Context,
	_ *stats.ConnTagInfo,
) context.Context {
	return ctx
}

func (*reentrantClientStatsHandler) HandleConn(
	context.Context,
	stats.ConnStats,
) {
}

func (h *reentrantClientStatsHandler) setClient(
	client grpc.ClientStream,
) observableClientStream {
	observed, ok := client.(observableClientStream)
	if !ok {
		panic("inprocgrpc: client stream lacks lifecycle observation")
	}
	h.mu.Lock()
	h.client = observed
	h.mu.Unlock()
	return observed
}

func (h *reentrantClientStatsHandler) snapshotResult() (
	error,
	error,
	error,
	bool,
) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.result, h.contextErr, h.terminalErr, h.terminal
}

func (h *reentrantClientStatsHandler) release() {
	h.proceedOnce.Do(func() { close(h.proceed) })
}

func newReentrantClientStatsHandler(
	target string,
	action func(observableClientStream) error,
) *reentrantClientStatsHandler {
	return &reentrantClientStatsHandler{
		target:   target,
		action:   action,
		entered:  make(chan struct{}),
		proceed:  make(chan struct{}),
		returned: make(chan struct{}),
		ended:    make(chan struct{}),
	}
}

func waitStatsBarrier(t *testing.T, signal <-chan struct{}, subject string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s did not complete", subject)
	}
}

func newInternalTestChannel(
	t *testing.T,
	options ...ChannelOption,
) *Channel {
	t.Helper()
	loop := eventloop.New()
	started := make(chan error, 1)
	if err := loop.Submit(func() {
		_, err := loop.ScheduleTimer(time.Hour, func() {})
		started <- err
	}); err != nil {
		t.Fatalf("queue event loop startup: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = loop.Run(ctx)
	}()
	if err := <-started; err != nil {
		cancel()
		<-done
		t.Fatalf("schedule event loop keepalive: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		<-done
	})
	options = append([]ChannelOption{WithLoop(loop)}, options...)
	return NewChannel(options...)
}

func TestPreparedStatsAckGatesReleaseNotTerminalResult(t *testing.T) {
	loop := &capturedTerminalLoop{
		done:      make(chan struct{}),
		internal:  make(chan func(), 4),
		submitted: make(chan struct{}, 4),
	}
	state := stream.NewRPCState("/test.Service/Call", 1)
	life := newRPCLifecycle(loop, state, nil, false, true)
	handler := &barrierStatsHandler{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	helper := &statsHandlerHelper{handler: handler}
	serverStats, err := helper.startRPC(
		context.Background(),
		state.Method,
		false,
		false,
	)
	if err != nil {
		t.Fatal(err)
	}
	life.setServer(func() {}, serverStats)
	client := &clientStreamAdapter{
		ctx:       context.Background(),
		callerCtx: context.Background(),
		loop:      loop,
		life:      life,
		state:     state,
		copts:     new(callopts.CallOptions),
		stats:     nil,
		method:    state.Method,
	}
	response := &wrapperspb.StringValue{Value: "response"}
	if !life.serverFinishPrepared(nil, &terminalPreparation{
		headers:      metadata.Pairs("header", "value"),
		trailers:     metadata.Pairs("trailer", "value"),
		response:     response,
		statsPayload: response,
		sendResponse: true,
	}) {
		t.Fatal("prepared completion did not win")
	}
	(<-loop.internal)()
	select {
	case <-handler.entered:
	case <-time.After(time.Second):
		t.Fatal("OutPayload did not enter")
	}
	select {
	case <-client.TerminalDone():
	default:
		t.Fatal("TerminalDone remained open behind observational stats")
	}
	if terminalErr, terminal := client.TerminalResult(); !terminal ||
		terminalErr != nil {
		t.Fatalf("TerminalResult = %v, %v, want nil, true", terminalErr, terminal)
	}
	headers, err := client.Header()
	values := headers.Get("header")
	if err != nil || len(values) != 1 || values[0] != "value" {
		t.Fatalf("Header = %v, %v", headers, err)
	}
	select {
	case <-client.Done():
		t.Fatal("Done closed before stats acknowledgement")
	default:
	}
	var (
		delivered   any
		receiveErr  error
		terminalErr error
	)
	if !life.submitExternalOwner(
		"test response drain",
		func(rpcOwnerCapability) {
			state.Responses.Recv(func(message any, recvErr error) {
				delivered = message
				receiveErr = recvErr
			})
			state.Responses.Recv(func(_ any, recvErr error) {
				terminalErr = recvErr
			})
		},
	) {
		t.Fatal("response drain was not admitted")
	}
	if receiveErr != nil {
		t.Fatalf("response receive = %v", receiveErr)
	}
	if delivered != response {
		t.Fatalf("response identity = %p, want %p", delivered, response)
	}
	if terminalErr != io.EOF {
		t.Fatalf("response terminal = %v, want EOF", terminalErr)
	}
	close(handler.release)
	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("Done remained open after stats acknowledgement")
	}
	events := handler.snapshot()
	want := []string{
		"Begin",
		"OutHeader",
		"OutPayload",
		"OutTrailer",
		"End",
	}
	if len(events) != len(want) {
		t.Fatalf("stats events = %v, want %v", events, want)
	}
	for index := range want {
		if events[index] != want[index] {
			t.Fatalf("stats events = %v, want %v", events, want)
		}
	}
}

func TestClientOutPayloadMayReenterTerminalReceive(t *testing.T) {
	handler := newReentrantClientStatsHandler(
		"OutPayload",
		func(client observableClientStream) error {
			return client.RecvMsg(new(wrapperspb.StringValue))
		},
	)
	t.Cleanup(handler.release)
	channel := newInternalTestChannel(t, WithClientStatsHandler(handler))
	aborted := make(chan struct{})
	channel.RegisterService(&grpc.ServiceDesc{
		ServiceName: "test.StatsService",
		Streams: []grpc.StreamDesc{{
			StreamName:    "ReentrantTerminal",
			ClientStreams: true,
			ServerStreams: true,
			Handler: func(_ any, stream grpc.ServerStream) error {
				message := new(wrapperspb.StringValue)
				if err := stream.RecvMsg(message); err != nil {
					return err
				}
				close(aborted)
				return status.Error(codes.Aborted, "selected")
			},
		}},
	}, struct{}{})
	raw, err := channel.NewStream(
		context.Background(),
		&grpc.StreamDesc{ClientStreams: true, ServerStreams: true},
		"/test.StatsService/ReentrantTerminal",
	)
	if err != nil {
		t.Fatal(err)
	}
	client := handler.setClient(raw)
	sendResult := make(chan error, 1)
	go func() {
		sendResult <- client.SendMsg(new(wrapperspb.StringValue))
	}()
	waitStatsBarrier(t, aborted, "server abort")
	waitStatsBarrier(t, handler.entered, "OutPayload entry")
	waitStatsBarrier(t, client.TerminalDone(), "terminal result")
	select {
	case <-client.Done():
		t.Fatal("Done closed while OutPayload was blocked")
	default:
	}
	handler.release()
	waitStatsBarrier(t, handler.returned, "reentrant RecvMsg")
	if err := <-sendResult; err != nil {
		t.Fatalf("SendMsg = %v", err)
	}
	result, contextErr, terminalErr, terminal := handler.snapshotResult()
	if status.Code(result) != codes.Aborted {
		t.Fatalf("reentrant RecvMsg = %v, want Aborted", result)
	}
	if contextErr == nil {
		t.Fatal("client context was not canceled before RecvMsg returned")
	}
	if !terminal || status.Code(terminalErr) != codes.Aborted {
		t.Fatalf("TerminalResult = %v, %v, want Aborted, true",
			terminalErr,
			terminal,
		)
	}
	waitStatsBarrier(t, handler.ended, "client stats End")
	waitStatsBarrier(t, client.Done(), "client Done")
}

func TestClientInboundStatsMayReenterLocalFailure(t *testing.T) {
	for _, event := range []string{"InHeader", "InPayload"} {
		t.Run(event, func(t *testing.T) {
			handler := newReentrantClientStatsHandler(
				event,
				func(client observableClientStream) error {
					return client.SendMsg(
						&wrapperspb.StringValue{Value: "too large"},
					)
				},
			)
			t.Cleanup(handler.release)
			channel := newInternalTestChannel(t, WithClientStatsHandler(handler))
			channel.RegisterService(&grpc.ServiceDesc{
				ServiceName: "test.StatsService",
				Streams: []grpc.StreamDesc{{
					StreamName:    "ReentrantFailure",
					ClientStreams: true,
					ServerStreams: true,
					Handler: func(_ any, stream grpc.ServerStream) error {
						message := new(wrapperspb.StringValue)
						if err := stream.RecvMsg(message); err != nil {
							return err
						}
						if err := stream.SetHeader(
							metadata.Pairs("header", "value"),
						); err != nil {
							return err
						}
						if err := stream.SendMsg(
							new(wrapperspb.StringValue),
						); err != nil {
							return err
						}
						return nil
					},
				}},
			}, struct{}{})
			raw, err := channel.NewStream(
				context.Background(),
				&grpc.StreamDesc{
					ClientStreams: true,
					ServerStreams: true,
				},
				"/test.StatsService/ReentrantFailure",
				grpc.MaxCallSendMsgSize(0),
			)
			if err != nil {
				t.Fatal(err)
			}
			client := handler.setClient(raw)
			if err := client.SendMsg(new(wrapperspb.StringValue)); err != nil {
				t.Fatalf("initial SendMsg = %v", err)
			}
			receiveResult := make(chan error, 1)
			go func() {
				receiveResult <- client.RecvMsg(new(wrapperspb.StringValue))
			}()
			waitStatsBarrier(t, handler.entered, event+" entry")
			select {
			case <-handler.ended:
				t.Fatal("End ran while inbound stats callback was blocked")
			default:
			}
			handler.release()
			waitStatsBarrier(t, handler.returned, "reentrant SendMsg")
			if err := <-receiveResult; err != nil {
				t.Fatalf("response RecvMsg = %v", err)
			}
			result, contextErr, terminalErr, terminal :=
				handler.snapshotResult()
			if status.Code(result) != codes.ResourceExhausted {
				t.Fatalf("reentrant SendMsg = %v, want ResourceExhausted",
					result,
				)
			}
			if contextErr == nil {
				t.Fatal("client context was not canceled before SendMsg returned")
			}
			if !terminal || terminalErr != nil {
				t.Fatalf("TerminalResult = %v, %v, want nil, true",
					terminalErr,
					terminal,
				)
			}
			err = client.RecvMsg(new(wrapperspb.StringValue))
			if status.Code(err) != codes.ResourceExhausted {
				t.Fatalf("terminal RecvMsg = %v, want ResourceExhausted", err)
			}
			waitStatsBarrier(t, handler.ended, "client stats End")
			waitStatsBarrier(t, client.Done(), "client Done")
		})
	}
}
