package inprocgrpc_test

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
)

// parkedLoop is a serial fake loop whose runner can be parked by a submitted
// task, so later submissions queue deterministically behind it. busy is set
// for as long as the parked task occupies the runner; any internal submission
// observed while busy closes parkedSignal exactly once, which proves an owner
// turn was admitted and queued undelivered. Owner turns are submitted through
// SubmitInternal (rpcLifecycle.submitOwner), so the signal must live there.
type parkedLoop struct {
	done         chan struct{}
	busy         atomic.Bool
	parkedSignal chan struct{}
	signalOnce   sync.Once
	stop         chan struct{}
	tasks        chan func()
}

func newParkedLoop(t testing.TB) *parkedLoop {
	l := &parkedLoop{
		done:         make(chan struct{}),
		parkedSignal: make(chan struct{}),
		stop:         make(chan struct{}),
		tasks:        make(chan func(), 16),
	}
	runnerExited := make(chan struct{})
	go func() {
		defer close(runnerExited)
		for {
			select {
			case task := <-l.tasks:
				task()
			case <-l.stop:
				return
			}
		}
	}()
	t.Cleanup(func() {
		close(l.stop)
		<-runnerExited
	})
	return l
}

func (l *parkedLoop) Submit(task func()) error {
	select {
	case l.tasks <- task:
		return nil
	case <-l.stop:
		return status.Error(codes.Unavailable, "loop stopped")
	}
}

func (l *parkedLoop) SubmitInternal(task func()) error {
	// busy proves only that an owner turn was admitted and queued
	// undelivered; in this flow the queued turn is the decode's receive turn,
	// but the asserted invariant does not depend on which turn it is.
	if l.busy.Load() {
		l.signalOnce.Do(func() { close(l.parkedSignal) })
	}
	select {
	case l.tasks <- task:
		return nil
	case <-l.stop:
		return status.Error(codes.Unavailable, "loop stopped")
	}
}

func (l *parkedLoop) Done() <-chan struct{} { return l.done }

// Without the recvMu barrier the check reads recvCount as 0 and rejects a valid handler.
func TestUnary_AdmittedReceiveAtHandlerReturn_CardinalityError(t *testing.T) {
	loop := newParkedLoop(t)
	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))

	// The handler parks the loop runner, spawns the decode, waits until the
	// decode's owner turn is provably queued behind the parked task, then
	// returns while the receive is still undelivered. The unblocking task
	// runs only after the handler's defer has taken recvMu and blocked on it,
	// so the locked validation waits out the in-flight receive and must not
	// report a spurious cardinality failure.
	handlerReturning := make(chan struct{})
	unblock := make(chan struct{})

	desc := grpc.ServiceDesc{
		ServiceName: "test.OffLoopUnary",
		HandlerType: (*any)(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "Unary",
				Handler: func(
					_ any,
					_ context.Context,
					dec func(any) error,
					_ grpc.UnaryServerInterceptor,
				) (any, error) {
					// Park the loop runner: this task occupies it until the
					// unblock task below is queued behind it.
					parked := make(chan struct{})
					if err := loop.Submit(func() {
						loop.busy.Store(true)
						defer loop.busy.Store(false)
						close(parked)
						<-unblock
					}); err != nil {
						return nil, err
					}
					<-parked

					go func() {
						in := new(wrapperspb.StringValue)
						_ = dec(in)
					}()

					// Prove the receive's owner turn is queued, then return
					// with the receive still undelivered.
					select {
					case <-loop.parkedSignal:
					case <-time.After(5 * time.Second):
						t.Error("receive owner turn was never submitted")
						close(unblock)
						return nil, context.DeadlineExceeded
					}
					close(handlerReturning)
					return &wrapperspb.StringValue{Value: "ok"}, nil
				},
			},
		},
	}
	ch.RegisterService(&desc, struct{}{})

	invokeResult := make(chan error, 1)
	resp := new(wrapperspb.StringValue)
	go func() {
		invokeResult <- ch.Invoke(
			context.Background(),
			"/test.OffLoopUnary/Unary",
			&wrapperspb.StringValue{Value: "hi"},
			resp,
		)
	}()

	select {
	case <-handlerReturning:
	case <-time.After(5 * time.Second):
		t.Fatal("handler never reached its return path")
	}

	// The handler returned and its defer is now blocked taking recvMu (the
	// spawned decode registered the receive turn before returning, and
	// RecvMsg holds recvMu from entry until its owner turn completes).
	// Releasing the parked task lets the receive deliver, recvCount become 1,
	// recvMu release, and the locked validation observe the consumed request.
	close(unblock)

	select {
	case err := <-invokeResult:
		if err != nil {
			if status.Code(err) == codes.Internal &&
				strings.Contains(err.Error(), "must consume exactly one request") {
				t.Fatalf("spurious cardinality error on a valid handler: %v", err)
			}
			t.Fatalf("Invoke: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Invoke never completed")
	}
	if resp.GetValue() != "ok" {
		t.Fatalf("unexpected response: %q", resp.GetValue())
	}
}

// joinRecvStreamServer consumes its single request and waits for it, so the
// handler is unambiguously valid.
type joinRecvStreamServer struct{}

func (joinRecvStreamServer) Unary(
	_ context.Context,
	in *wrapperspb.StringValue,
) (*wrapperspb.StringValue, error) {
	return &wrapperspb.StringValue{Value: "ok"}, nil
}

func (s joinRecvStreamServer) ServerStream(
	_ *wrapperspb.StringValue,
	stream grpc.ServerStream,
) error {
	in := new(wrapperspb.StringValue)
	if err := stream.RecvMsg(in); err != nil {
		return err
	}
	return nil
}

func (joinRecvStreamServer) ClientStream(grpc.ServerStream) error { return nil }
func (joinRecvStreamServer) BidiStream(grpc.ServerStream) error   { return nil }

// A server-streaming handler that consumed its one request must never be
// failed with a cardinality error, however the receive and the handler
// return interleave.
func TestServerStream_JoinedReceive_NeverCardinalityError(t *testing.T) {
	loop := newTestLoop(t)
	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))

	desc := grpc.ServiceDesc{
		ServiceName: "test.JoinedRecv",
		HandlerType: (*any)(nil),
		Streams: []grpc.StreamDesc{{
			StreamName: "ServerStream",
			Handler: func(srv any, stream grpc.ServerStream) error {
				return srv.(joinRecvStreamServer).ServerStream(nil, stream)
			},
			ServerStreams: true,
			ClientStreams: false,
		}},
	}
	ch.RegisterService(&desc, joinRecvStreamServer{})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const iterations = 400
	for i := range iterations {
		cs, err := ch.NewStream(ctx, &desc.Streams[0], "/test.JoinedRecv/ServerStream")
		if err != nil {
			t.Fatalf("NewStream: %v", err)
		}
		if err := cs.SendMsg(&wrapperspb.StringValue{Value: "req"}); err != nil {
			t.Fatalf("SendMsg: %v", err)
		}
		_ = cs.CloseSend()

		streamErr := cs.RecvMsg(new(wrapperspb.StringValue))
		if streamErr == nil {
			t.Fatal("expected the stream to end with no response messages")
		}
		if status.Code(streamErr) == codes.Internal &&
			strings.Contains(streamErr.Error(), "must consume exactly one request") {
			t.Fatalf("iteration %d: spurious cardinality error on a valid handler: %v",
				i, streamErr)
		}
	}
}
