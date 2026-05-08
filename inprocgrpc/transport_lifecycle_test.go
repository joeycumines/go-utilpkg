package inprocgrpc_test

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
)

type droppedInitializationLoop struct {
	done      chan struct{}
	submitted chan struct{}
	once      sync.Once
}

func (l *droppedInitializationLoop) Submit(func()) error {
	l.once.Do(func() { close(l.submitted) })
	return nil
}

func (*droppedInitializationLoop) SubmitInternal(func()) error { return nil }
func (l *droppedInitializationLoop) Done() <-chan struct{}     { return l.done }

func TestNewStreamAcceptedInitializationDropSettles(t *testing.T) {
	loop := &droppedInitializationLoop{
		done:      make(chan struct{}),
		submitted: make(chan struct{}),
	}
	channel := mustNewChannel(t, inprocgrpc.WithLoop(loop))
	channel.RegisterService(&testServiceDesc, &echoServer{})

	result := make(chan error, 1)
	go func() {
		_, err := channel.NewStream(
			context.Background(),
			&grpc.StreamDesc{ClientStreams: true, ServerStreams: true},
			"/test.TestService/BidiStream",
		)
		result <- err
	}()
	<-loop.submitted
	close(loop.done)
	if err := <-result; status.Code(err) != codes.Unavailable {
		t.Fatalf("NewStream = %v, want Unavailable", err)
	}
}

type droppedTerminalLoop struct {
	done         chan struct{}
	submitted    chan struct{}
	terminalTask chan func()
}

type closeRaceLoop struct {
	done          chan struct{}
	pending       chan func()
	submitCalls   atomic.Int32
	internalCalls atomic.Int32
}

type immediateLoop struct {
	done      chan struct{}
	submitted chan struct{}
	tasks     chan func()
	stop      chan struct{}
	stopped   atomic.Bool
	stopOnce  sync.Once
}

func (l *immediateLoop) Submit(fn func()) error {
	return l.enqueue(func() {
		fn()
		l.submitted <- struct{}{}
	})
}

func (l *immediateLoop) Done() <-chan struct{} { return l.done }

func newImmediateLoop(t *testing.T) *immediateLoop {
	t.Helper()
	loop := &immediateLoop{
		done:      make(chan struct{}),
		submitted: make(chan struct{}, 8),
		tasks:     make(chan func(), 32),
		stop:      make(chan struct{}),
	}
	go loop.run()
	t.Cleanup(loop.close)
	return loop
}

func (l *immediateLoop) enqueue(fn func()) error {
	if l.stopped.Load() {
		return status.Error(codes.Unavailable, "loop stopped")
	}
	select {
	case l.tasks <- fn:
		return nil
	case <-l.stop:
		return status.Error(codes.Unavailable, "loop stopped")
	}
}

func (l *immediateLoop) run() {
	defer close(l.done)
	for {
		select {
		case fn := <-l.tasks:
			fn()
		case <-l.stop:
			return
		}
	}
}

func (l *immediateLoop) close() {
	l.stopOnce.Do(func() {
		l.stopped.Store(true)
		close(l.stop)
	})
	<-l.done
}

func (l *immediateLoop) SubmitInternal(fn func()) error {
	return l.enqueue(fn)
}

func (l *closeRaceLoop) Submit(fn func()) error {
	if l.submitCalls.Add(1) == 1 {
		fn()
		return nil
	}
	l.pending <- fn
	return nil
}

func (l *closeRaceLoop) SubmitInternal(fn func()) error {
	if l.internalCalls.Add(1) == 1 {
		fn()
	}
	return nil
}

func (l *closeRaceLoop) Done() <-chan struct{} { return l.done }

func (l *droppedTerminalLoop) Submit(fn func()) error {
	fn()
	l.submitted <- struct{}{}
	return nil
}

func (l *droppedTerminalLoop) SubmitInternal(fn func()) error {
	l.terminalTask <- fn
	return nil
}

func (l *droppedTerminalLoop) Done() <-chan struct{} { return l.done }

func TestAcceptedTerminalCallbackDropAllowsBufferedDrain(t *testing.T) {
	loop := &droppedTerminalLoop{
		done:         make(chan struct{}),
		submitted:    make(chan struct{}, 8),
		terminalTask: make(chan func(), 8),
	}
	channel := mustNewChannel(t, inprocgrpc.WithLoop(loop))
	channel.RegisterStreamHandler(
		"/test.Svc/DropTerminal",
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
				stream.Finish(status.Error(codes.Aborted, "late"))
			})
		},
	)

	client, err := channel.NewStream(
		context.Background(),
		&grpc.StreamDesc{ServerStreams: true},
		"/test.Svc/DropTerminal",
	)
	if err != nil {
		t.Fatal(err)
	}
	<-loop.submitted
	if err := client.SendMsg(&wrapperspb.StringValue{Value: "request"}); err != nil {
		t.Fatal(err)
	}
	<-loop.submitted
	delayedTerminal := <-loop.terminalTask
	response := new(wrapperspb.StringValue)
	if err := client.RecvMsg(response); err != nil {
		t.Fatal(err)
	}
	<-loop.submitted
	if response.GetValue() != "request" {
		t.Fatalf("response = %q, want request", response.GetValue())
	}

	// A terminal callback accepted before the draining receive may still arrive
	// late. Its obsolete turn must be rejected without retrying or changing the
	// already applied terminal result.
	delayedTerminal()
	select {
	case <-loop.terminalTask:
		t.Fatal("obsolete terminal callback retried after data-owner takeover")
	default:
	}
	err = client.RecvMsg(new(wrapperspb.StringValue))
	close(loop.done)
	if err != io.EOF {
		t.Fatalf("terminal RecvMsg = %v, want EOF", err)
	}
}

func TestCloseSendAcceptedTerminalRaces(t *testing.T) {
	for _, name := range []string{
		"ack before terminal",
		"terminal before ack",
		"context before ack",
	} {
		t.Run(name, func(t *testing.T) {
			loop := &closeRaceLoop{
				done:    make(chan struct{}),
				pending: make(chan func(), 1),
			}
			channel := mustNewChannel(t, inprocgrpc.WithLoop(loop))
			channel.RegisterStreamHandler(
				"/test.Svc/CloseRace",
				func(context.Context, *inprocgrpc.RPCStream) {},
			)
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			client, err := channel.NewStream(
				ctx,
				&grpc.StreamDesc{ClientStreams: true, ServerStreams: true},
				"/test.Svc/CloseRace",
			)
			if err != nil {
				t.Fatal(err)
			}
			result := make(chan error, 1)
			go func() { result <- client.CloseSend() }()
			pending := <-loop.pending
			switch name {
			case "ack before terminal":
				pending()
				err = <-result
				close(loop.done)
			case "terminal before ack":
				close(loop.done)
				err = <-result
			case "context before ack":
				cancel()
				err = <-result
				pending()
				close(loop.done)
			default:
				t.Fatalf("unknown test case %q", name)
			}
			if err != nil {
				t.Fatalf("CloseSend = %v, want nil", err)
			}
			if second := client.CloseSend(); second != nil {
				t.Fatalf("second CloseSend = %v, want nil", second)
			}
		})
	}
}

func TestPendingClientSendReportsEOFWhenGracefulCompletionDiscardsIt(t *testing.T) {
	loop := newImmediateLoop(t)
	channel := mustNewChannel(t,
		inprocgrpc.WithLoop(loop),
		inprocgrpc.WithStreamBuffer(1),
	)
	handler := make(chan *inprocgrpc.RPCStream, 1)
	channel.RegisterStreamHandler(
		"/test.Svc/PendingRequest",
		func(_ context.Context, stream *inprocgrpc.RPCStream) {
			handler <- stream
		},
	)
	client, err := channel.NewStream(
		context.Background(),
		&grpc.StreamDesc{ClientStreams: true, ServerStreams: true},
		"/test.Svc/PendingRequest",
	)
	if err != nil {
		t.Fatal(err)
	}
	stream := <-handler
	<-loop.submitted
	if err := client.SendMsg(&wrapperspb.StringValue{Value: "buffered"}); err != nil {
		t.Fatal(err)
	}
	<-loop.submitted

	second := make(chan error, 1)
	go func() {
		second <- client.SendMsg(&wrapperspb.StringValue{Value: "pending"})
	}()
	<-loop.submitted
	if err := loop.Submit(func() { stream.Finish(nil) }); err != nil {
		t.Fatal(err)
	}
	<-loop.submitted
	if err := <-second; err != io.EOF {
		t.Fatalf("pending SendMsg = %v, want EOF", err)
	}
	if err := client.RecvMsg(new(wrapperspb.StringValue)); err != io.EOF {
		t.Fatalf("terminal RecvMsg = %v, want EOF", err)
	}
}

func TestCallbackBackpressureAndFirstTerminalResult(t *testing.T) {
	channel := newBareChannel(t, inprocgrpc.WithStreamBuffer(1))
	sendResults := make(chan [2]error, 1)
	channel.RegisterStreamHandler(
		"/test.Svc/Backpressure",
		func(_ context.Context, stream *inprocgrpc.RPCStream) {
			stream.Recv().Recv(func(message any, err error) {
				if err != nil {
					stream.Finish(err)
					return
				}
				first := stream.Send().Send(message)
				second := stream.Send().Send(message)
				sendResults <- [2]error{first, second}
				stream.Finish(second)
				stream.Finish(status.Error(codes.Aborted, "late"))
			})
		},
	)

	client, err := channel.NewStream(
		context.Background(),
		&grpc.StreamDesc{ServerStreams: true},
		"/test.Svc/Backpressure",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SendMsg(&wrapperspb.StringValue{Value: "request"}); err != nil {
		t.Fatal(err)
	}
	results := <-sendResults
	if results[0] != nil {
		t.Fatalf("first Send = %v", results[0])
	}
	if status.Code(results[1]) != codes.ResourceExhausted {
		t.Fatalf("second Send = %v, want ResourceExhausted", results[1])
	}
	if err := client.RecvMsg(new(wrapperspb.StringValue)); err != nil {
		t.Fatalf("buffered RecvMsg: %v", err)
	}
	if err := client.RecvMsg(new(wrapperspb.StringValue)); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("terminal RecvMsg = %v, want ResourceExhausted", err)
	}
}

func TestUnaryHandlerAbnormalExitSettlesInternal(t *testing.T) {
	tests := []struct {
		name string
		run  func()
		want string
	}{
		{name: "panic", run: func() { panic("boom") }, want: "panicked"},
		{name: "Goexit", run: runtime.Goexit, want: "exited without returning"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := newBareChannel(t)
			desc := grpc.ServiceDesc{
				ServiceName: "test.Abnormal",
				Methods: []grpc.MethodDesc{{
					MethodName: "Unary",
					Handler: func(
						any,
						context.Context,
						func(any) error,
						grpc.UnaryServerInterceptor,
					) (any, error) {
						test.run()
						return new(wrapperspb.StringValue), nil
					},
				}},
			}
			channel.RegisterService(&desc, struct{}{})
			err := channel.Invoke(
				context.Background(),
				"/test.Abnormal/Unary",
				new(wrapperspb.StringValue),
				new(wrapperspb.StringValue),
			)
			if status.Code(err) != codes.Internal || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Invoke = %v, want Internal containing %q", err, test.want)
			}
		})
	}
}

func TestStreamInterceptorAbnormalExitSettlesInternal(t *testing.T) {
	tests := []struct {
		name string
		run  func()
		want string
	}{
		{name: "panic", run: func() { panic("boom") }, want: "panicked"},
		{name: "Goexit", run: runtime.Goexit, want: "exited without returning"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := newTestChannel(
				t,
				inprocgrpc.WithServerStreamInterceptor(func(
					any,
					grpc.ServerStream,
					*grpc.StreamServerInfo,
					grpc.StreamHandler,
				) error {
					test.run()
					return nil
				}),
			)
			client, err := channel.NewStream(
				context.Background(),
				&grpc.StreamDesc{ClientStreams: true, ServerStreams: true},
				"/test.TestService/BidiStream",
			)
			if err != nil {
				t.Fatal(err)
			}
			err = client.RecvMsg(new(wrapperspb.StringValue))
			if status.Code(err) != codes.Internal || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("RecvMsg = %v, want Internal containing %q", err, test.want)
			}
		})
	}
}

func TestCallbackAbnormalExitSettlesAndLoopContinues(t *testing.T) {
	tests := []struct {
		name string
		run  func()
		want string
	}{
		{name: "panic", run: func() { panic("boom") }, want: "panicked"},
		{name: "Goexit", run: runtime.Goexit, want: "exited without returning"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := newBareChannel(t)
			channel.RegisterStreamHandler(
				"/test.Callback/Abnormal",
				func(_ context.Context, stream *inprocgrpc.RPCStream) {
					stream.Recv().Recv(func(any, error) { test.run() })
				},
			)
			channel.RegisterStreamHandler(
				"/test.Callback/Echo",
				func(_ context.Context, stream *inprocgrpc.RPCStream) {
					stream.Recv().Recv(func(message any, err error) {
						if err == nil {
							err = stream.Send().Send(message)
						}
						stream.Finish(err)
					})
				},
			)
			channel.RegisterStreamHandler(
				"/test.Callback/CloseAbnormal",
				func(_ context.Context, stream *inprocgrpc.RPCStream) {
					stream.Recv().Recv(func(any, error) { test.run() })
				},
			)

			err := channel.Invoke(
				context.Background(),
				"/test.Callback/Abnormal",
				&wrapperspb.StringValue{Value: "request"},
				new(wrapperspb.StringValue),
			)
			if status.Code(err) != codes.Internal || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("abnormal Invoke = %v, want Internal containing %q", err, test.want)
			}

			client, err := channel.NewStream(
				context.Background(),
				&grpc.StreamDesc{ClientStreams: true, ServerStreams: true},
				"/test.Callback/CloseAbnormal",
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := client.CloseSend(); err != nil {
				t.Fatalf("CloseSend during abnormal callback: %v", err)
			}
			err = client.RecvMsg(new(wrapperspb.StringValue))
			if status.Code(err) != codes.Internal || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("close-callback RecvMsg = %v, want Internal containing %q", err, test.want)
			}

			response := new(wrapperspb.StringValue)
			if err := channel.Invoke(
				context.Background(),
				"/test.Callback/Echo",
				&wrapperspb.StringValue{Value: "still running"},
				response,
			); err != nil {
				t.Fatalf("follow-up Invoke: %v", err)
			}
			if response.GetValue() != "still running" {
				t.Fatalf("follow-up response = %q", response.GetValue())
			}
		})
	}
}

func TestSuccessfulUnaryMustConsumeOneRequest(t *testing.T) {
	channel := newBareChannel(t)
	desc := grpc.ServiceDesc{
		ServiceName: "test.Cardinality",
		Methods: []grpc.MethodDesc{{
			MethodName: "Unary",
			Handler: func(
				any,
				context.Context,
				func(any) error,
				grpc.UnaryServerInterceptor,
			) (any, error) {
				return new(wrapperspb.StringValue), nil
			},
		}},
	}
	channel.RegisterService(&desc, struct{}{})
	err := channel.Invoke(
		context.Background(),
		"/test.Cardinality/Unary",
		new(wrapperspb.StringValue),
		new(wrapperspb.StringValue),
	)
	if status.Code(err) != codes.Internal {
		t.Fatalf("Invoke = %v, want Internal", err)
	}
}

func TestSuccessfulServerStreamMustReceiveOneRequest(t *testing.T) {
	channel := newBareChannel(t)
	desc := grpc.ServiceDesc{
		ServiceName: "test.RequestCardinality",
		Streams: []grpc.StreamDesc{{
			StreamName:    "ServerStream",
			ServerStreams: true,
			Handler: func(_ any, stream grpc.ServerStream) error {
				recvErr := stream.RecvMsg(new(wrapperspb.StringValue))
				if recvErr != io.EOF {
					return fmt.Errorf("RecvMsg = %v, want EOF", recvErr)
				}
				return stream.SendMsg(
					&wrapperspb.StringValue{Value: "invalid success"},
				)
			},
		}},
	}
	channel.RegisterService(&desc, struct{}{})
	client, err := channel.NewStream(
		context.Background(),
		&grpc.StreamDesc{ServerStreams: true},
		"/test.RequestCardinality/ServerStream",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CloseSend(); err != nil {
		t.Fatal(err)
	}
	err = client.RecvMsg(new(wrapperspb.StringValue))
	if err == nil {
		err = client.RecvMsg(new(wrapperspb.StringValue))
	}
	if status.Code(err) != codes.Internal ||
		!strings.Contains(err.Error(), "consume exactly one request") {
		t.Fatalf("RecvMsg = %v, want request-cardinality Internal", err)
	}
}

func TestSuccessfulCallbackMustReceiveOneRequest(t *testing.T) {
	channel := newBareChannel(t)
	channel.RegisterStreamHandler(
		"/test.CallbackCardinality/Unary",
		func(_ context.Context, stream *inprocgrpc.RPCStream) {
			if err := stream.Send().Send(new(wrapperspb.StringValue)); err != nil {
				stream.Abort(err)
				return
			}
			stream.Finish(nil)
		},
	)
	err := channel.Invoke(
		context.Background(),
		"/test.CallbackCardinality/Unary",
		new(wrapperspb.StringValue),
		new(wrapperspb.StringValue),
	)
	if status.Code(err) != codes.Internal ||
		!strings.Contains(err.Error(), "consume exactly one request") {
		t.Fatalf("Invoke = %v, want request-cardinality Internal", err)
	}
}

func TestSuccessfulCallbackReceivesExactlyOneRequest(t *testing.T) {
	channel := newBareChannel(t)
	channel.RegisterStreamHandler(
		"/test.CallbackCardinality/Echo",
		func(_ context.Context, stream *inprocgrpc.RPCStream) {
			stream.Recv().Recv(func(message any, err error) {
				if err != nil {
					stream.Abort(err)
					return
				}
				if err := stream.Send().Send(message); err != nil {
					stream.Abort(err)
					return
				}
				stream.Finish(nil)
			})
		},
	)
	response := new(wrapperspb.StringValue)
	if err := channel.Invoke(
		context.Background(),
		"/test.CallbackCardinality/Echo",
		&wrapperspb.StringValue{Value: "accepted"},
		response,
	); err != nil {
		t.Fatal(err)
	}
	if response.Value != "accepted" {
		t.Fatalf("response = %q", response.Value)
	}
}

func TestStreamSenderCloseSelectsRPCOutcome(t *testing.T) {
	channel := newBareChannel(t)
	channel.RegisterStreamHandler(
		"/test.CallbackCardinality/Close",
		func(_ context.Context, stream *inprocgrpc.RPCStream) {
			stream.Recv().Recv(func(_ any, err error) {
				if err != nil {
					stream.Abort(err)
					return
				}
				stream.Send().Close(nil)
				stream.Abort(status.Error(codes.Aborted, "late abort"))
			})
		},
	)
	client, err := channel.NewStream(
		context.Background(),
		&grpc.StreamDesc{ServerStreams: true},
		"/test.CallbackCardinality/Close",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SendMsg(new(wrapperspb.StringValue)); err != nil {
		t.Fatal(err)
	}
	if err := client.CloseSend(); err != nil {
		t.Fatal(err)
	}
	if err := client.RecvMsg(new(wrapperspb.StringValue)); err != io.EOF {
		t.Fatalf("RecvMsg = %v, want graceful EOF", err)
	}
}

func TestWrappedContextErrorsRetainStatus(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code codes.Code
	}{
		{
			name: "canceled",
			err:  fmt.Errorf("wrapped cancellation: %w", context.Canceled),
			code: codes.Canceled,
		},
		{
			name: "deadline",
			err:  fmt.Errorf("wrapped deadline: %w", context.DeadlineExceeded),
			code: codes.DeadlineExceeded,
		},
	}
	for _, test := range tests {
		t.Run(test.name+"/unary", func(t *testing.T) {
			channel := newBareChannel(t)
			desc := grpc.ServiceDesc{
				ServiceName: "test.WrappedUnary",
				Methods: []grpc.MethodDesc{{
					MethodName: "Call",
					Handler: func(
						_ any,
						_ context.Context,
						decode func(any) error,
						_ grpc.UnaryServerInterceptor,
					) (any, error) {
						if err := decode(new(wrapperspb.StringValue)); err != nil {
							return nil, err
						}
						return nil, test.err
					},
				}},
			}
			channel.RegisterService(&desc, struct{}{})
			err := channel.Invoke(
				context.Background(),
				"/test.WrappedUnary/Call",
				new(wrapperspb.StringValue),
				new(wrapperspb.StringValue),
			)
			if status.Code(err) != test.code {
				t.Fatalf("Invoke = %v, want %v", err, test.code)
			}
		})
		t.Run(test.name+"/stream", func(t *testing.T) {
			channel := newBareChannel(t)
			desc := grpc.ServiceDesc{
				ServiceName: "test.WrappedStream",
				Streams: []grpc.StreamDesc{{
					StreamName:    "Call",
					ServerStreams: true,
					Handler: func(_ any, stream grpc.ServerStream) error {
						if err := stream.RecvMsg(new(wrapperspb.StringValue)); err != nil {
							return err
						}
						return test.err
					},
				}},
			}
			channel.RegisterService(&desc, struct{}{})
			client, err := channel.NewStream(
				context.Background(),
				&grpc.StreamDesc{ServerStreams: true},
				"/test.WrappedStream/Call",
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := client.SendMsg(new(wrapperspb.StringValue)); err != nil {
				t.Fatal(err)
			}
			err = client.RecvMsg(new(wrapperspb.StringValue))
			if status.Code(err) != test.code {
				t.Fatalf("RecvMsg = %v, want %v", err, test.code)
			}
		})
	}
}
