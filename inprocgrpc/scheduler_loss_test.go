package inprocgrpc_test

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type terminalDropLoop struct {
	done            chan struct{}
	terminalDropped chan struct{}
	tasks           chan func()
	stopCh          chan struct{}
	dropArmed       atomic.Bool
	stopped         atomic.Bool
	dropOnce        sync.Once
	stopOnce        sync.Once
}

func (l *terminalDropLoop) Submit(fn func()) error {
	return l.enqueue(fn)
}

// SubmitInternal drops every internal submission made after arm, so the
// scheduler is lost at the armed landmark regardless of which internal turns
// (terminal owner, client receive, resubmissions) race for the next slot. A
// dropped submission returns nil: the caller fences it as accepted, and the
// turn never runs.
func (l *terminalDropLoop) SubmitInternal(fn func()) error {
	if l.dropArmed.Load() {
		l.dropOnce.Do(func() { close(l.terminalDropped) })
		return nil
	}
	return l.enqueue(fn)
}

func (l *terminalDropLoop) arm() { l.dropArmed.Store(true) }

func (l *terminalDropLoop) Done() <-chan struct{} { return l.done }

func newTerminalDropLoop(t *testing.T) *terminalDropLoop {
	t.Helper()
	loop := &terminalDropLoop{
		done:            make(chan struct{}),
		terminalDropped: make(chan struct{}),
		tasks:           make(chan func(), 32),
		stopCh:          make(chan struct{}),
	}
	go loop.run()
	t.Cleanup(loop.close)
	return loop
}

func (l *terminalDropLoop) enqueue(fn func()) error {
	if l.stopped.Load() {
		return status.Error(codes.Unavailable, "loop stopped")
	}
	select {
	case l.tasks <- fn:
		return nil
	case <-l.stopCh:
		return status.Error(codes.Unavailable, "loop stopped")
	}
}

func (l *terminalDropLoop) run() {
	defer close(l.done)
	for {
		select {
		case fn := <-l.tasks:
			fn()
		case <-l.stopCh:
			return
		}
	}
}

func (l *terminalDropLoop) stop() {
	l.stopOnce.Do(func() {
		l.stopped.Store(true)
		close(l.stopCh)
	})
}

func (l *terminalDropLoop) close() {
	l.stop()
	<-l.done
}

func TestSchedulerLossPreservesAcceptedGracefulResponse(t *testing.T) {
	loop := newTerminalDropLoop(t)
	channel := mustNewChannel(t,
		inprocgrpc.WithLoop(loop),
		inprocgrpc.WithStreamBuffer(1),
	)
	sent := make(chan struct{})
	release := make(chan struct{})
	channel.RegisterStreamHandler(
		"/test.Svc/BufferedResponse",
		func(_ context.Context, stream *inprocgrpc.RPCStream) {
			stream.Recv().Recv(func(message any, err error) {
				if err != nil {
					stream.Finish(err)
					return
				}
				if err := stream.Send().Send(message); err != nil {
					stream.Finish(err)
					return
				}
				// The response is buffered; hold the terminal until the drop
				// is armed, so the loss cannot race an earlier owner turn.
				close(sent)
				<-release
				stream.Finish(nil)
			})
		},
	)
	client, err := channel.NewStream(
		context.Background(),
		&grpc.StreamDesc{ServerStreams: true},
		"/test.Svc/BufferedResponse",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SendMsg(&wrapperspb.StringValue{Value: "lost"}); err != nil {
		t.Fatal(err)
	}
	<-sent
	loop.arm()
	close(release)
	<-loop.terminalDropped
	loop.stop()

	response := new(wrapperspb.StringValue)
	err = client.RecvMsg(response)
	if err != nil {
		t.Fatalf("RecvMsg = %v, want nil", err)
	}
	if response.GetValue() != "lost" {
		t.Fatalf("recovered response = %q, want lost", response.GetValue())
	}
	if err := client.RecvMsg(new(wrapperspb.StringValue)); err != io.EOF {
		t.Fatalf("terminal RecvMsg = %v, want EOF", err)
	}
}

// A unary RPC whose scheduler is lost before the request is delivered never
// admits a terminal claim: loop death selects a scheduler-origin terminal, so
// Invoke reports Unavailable and the handler's response — never prepared — is
// not published.
func TestSchedulerLossDoesNotPublishUnaryResponse(t *testing.T) {
	loop := newTerminalDropLoop(t)
	loop.arm()
	channel := mustNewChannel(t, inprocgrpc.WithLoop(loop))
	channel.RegisterService(&testServiceDesc, &echoServer{})

	result := make(chan error, 1)
	response := new(wrapperspb.StringValue)
	go func() {
		result <- channel.Invoke(
			context.Background(),
			"/test.TestService/Unary",
			&wrapperspb.StringValue{Value: "prepared"},
			response,
		)
	}()
	// The first internal submission is an owner turn (server receive or
	// client receive); dropping it strands delivery, so no handler return,
	// no preparation, and no claim can occur.
	<-loop.terminalDropped
	loop.stop()
	if err := <-result; status.Code(err) != codes.Unavailable {
		t.Fatalf("Invoke = %v, want Unavailable", err)
	}
	if response.GetValue() != "" {
		t.Fatalf("dropped prepared response was copied: %q", response.GetValue())
	}
}

// A valid streaming RPC that loses its scheduler after the handler consumed
// its request and finished must report nil header and trailer metadata, the
// same as the live terminal-owner path. Recovery materializes metadata through
// metadata.Join and DetachPostDone, which never return nil, so without
// normalization the client observes an empty non-nil MD.
func TestSchedulerLossPreservesNilHeaderTrailerOnRecovery(t *testing.T) {
	loop := newTerminalDropLoop(t)
	channel := mustNewChannel(t,
		inprocgrpc.WithLoop(loop),
		inprocgrpc.WithStreamBuffer(1),
	)
	consumed := make(chan struct{})
	release := make(chan struct{})
	desc := grpc.ServiceDesc{
		ServiceName: "test.Svc",
		HandlerType: (*any)(nil),
		Streams: []grpc.StreamDesc{{
			StreamName: "NilMeta",
			Handler: func(_ any, stream grpc.ServerStream) error {
				if err := stream.RecvMsg(new(wrapperspb.StringValue)); err != nil {
					return err
				}
				// The request is consumed; hold the terminal until the drop
				// is armed, so the loss cannot race an earlier owner turn.
				close(consumed)
				<-release
				return nil
			},
			ServerStreams: true,
			ClientStreams: false,
		}},
	}
	channel.RegisterService(&desc, struct{}{})

	// grpc.Header/grpc.Trailer call options pin the same nil guarantee on the
	// callopts copy path (callopts.SetHeaders/SetTrailers), which is where an
	// empty non-nil MD from recovery would first become client-visible.
	var callHeaders, callTrailers metadata.MD
	client, err := channel.NewStream(
		context.Background(),
		&desc.Streams[0],
		"/test.Svc/NilMeta",
		grpc.Header(&callHeaders),
		grpc.Trailer(&callTrailers),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SendMsg(&wrapperspb.StringValue{Value: "req"}); err != nil {
		t.Fatal(err)
	}
	_ = client.CloseSend()
	// The handler consumed the request and is parked; once released, it
	// finishes without further loop interaction, and the loop stops after the
	// first post-arm drop, so the loss is in effect for the rest of the RPC
	// whichever internal turn would have run next.
	<-consumed
	loop.arm()
	close(release)
	<-loop.terminalDropped
	loop.stop()

	if err := client.RecvMsg(new(wrapperspb.StringValue)); err != io.EOF {
		t.Fatalf("terminal RecvMsg = %v, want EOF", err)
	}
	headers, err := client.Header()
	if err != nil {
		t.Fatalf("Header: %v", err)
	}
	if headers != nil {
		t.Errorf("recovered headers = %#v, want nil", headers)
	}
	if trailers := client.Trailer(); trailers != nil {
		t.Errorf("recovered trailers = %#v, want nil", trailers)
	}
	if callHeaders != nil {
		t.Errorf(
			"recovered call-option headers = %#v, want nil",
			callHeaders,
		)
	}
	if callTrailers != nil {
		t.Errorf(
			"recovered call-option trailers = %#v, want nil",
			callTrailers,
		)
	}
}
