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
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type terminalDropLoop struct {
	done            chan struct{}
	terminalDropped chan struct{}
	tasks           chan func()
	stopCh          chan struct{}
	internalCalls   atomic.Int32
	dropOrdinal     int32
	stopped         atomic.Bool
	dropOnce        sync.Once
	stopOnce        sync.Once
}

func (l *terminalDropLoop) Submit(fn func()) error {
	return l.enqueue(fn)
}

func (l *terminalDropLoop) SubmitInternal(fn func()) error {
	if l.internalCalls.Add(1) == l.dropOrdinal {
		l.dropOnce.Do(func() { close(l.terminalDropped) })
		return nil
	}
	return l.enqueue(fn)
}

func (l *terminalDropLoop) Done() <-chan struct{} { return l.done }

func newTerminalDropLoop(
	t *testing.T,
	dropOrdinal int32,
) *terminalDropLoop {
	t.Helper()
	if dropOrdinal <= 0 {
		t.Fatal("terminal drop ordinal must be positive")
	}
	loop := &terminalDropLoop{
		done:            make(chan struct{}),
		terminalDropped: make(chan struct{}),
		tasks:           make(chan func(), 32),
		stopCh:          make(chan struct{}),
		dropOrdinal:     dropOrdinal,
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
	loop := newTerminalDropLoop(t, 1)
	channel := mustNewChannel(t,
		inprocgrpc.WithLoop(loop),
		inprocgrpc.WithStreamBuffer(1),
	)
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

func TestSchedulerLossDoesNotPublishDroppedPreparedUnaryResponse(t *testing.T) {
	loop := newTerminalDropLoop(t, 2)
	if err := loop.SubmitInternal(func() {}); err != nil {
		t.Fatal(err)
	}
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
	<-loop.terminalDropped
	loop.stop()
	if err := <-result; status.Code(err) != codes.Unavailable {
		t.Fatalf("Invoke = %v, want Unavailable", err)
	}
	if response.GetValue() != "" {
		t.Fatalf("dropped prepared response was copied: %q", response.GetValue())
	}
}
