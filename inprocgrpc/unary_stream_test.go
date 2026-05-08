package inprocgrpc_test

import (
	"context"
	"io"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type lifecycleClientStream interface {
	grpc.ClientStream
	TerminalDone() <-chan struct{}
	TerminalResult() (error, bool)
	Done() <-chan struct{}
}

func TestNewStreamGeneratedUnaryLifecycle(t *testing.T) {
	channel := newTestChannel(t)
	raw, err := channel.NewStream(
		context.Background(),
		new(grpc.StreamDesc),
		"/test.TestService/Unary",
	)
	if err != nil {
		t.Fatal(err)
	}
	client, ok := raw.(lifecycleClientStream)
	if !ok {
		t.Fatal("unary client stream lacks lifecycle observation")
	}
	if err := client.SendMsg(
		&wrapperspb.StringValue{Value: "stream unary"},
	); err != nil {
		t.Fatalf("SendMsg = %v", err)
	}
	if err := client.SendMsg(new(wrapperspb.StringValue)); status.Code(err) !=
		codes.Internal {
		t.Fatalf("second SendMsg = %v, want Internal", err)
	}
	response := new(wrapperspb.StringValue)
	if err := client.RecvMsg(response); err != nil {
		t.Fatalf("RecvMsg = %v", err)
	}
	if response.GetValue() != "echo: stream unary" {
		t.Fatalf("response = %q", response.GetValue())
	}
	if err := client.RecvMsg(new(wrapperspb.StringValue)); err != io.EOF {
		t.Fatalf("second RecvMsg = %v, want EOF", err)
	}
	select {
	case <-client.TerminalDone():
	case <-time.After(time.Second):
		t.Fatal("TerminalDone remained open")
	}
	if terminalErr, terminal := client.TerminalResult(); !terminal ||
		terminalErr != nil {
		t.Fatalf("TerminalResult = %v, %v, want nil, true",
			terminalErr,
			terminal,
		)
	}
	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("Done remained open")
	}
}

func TestNewStreamGeneratedUnaryRequiresRequest(t *testing.T) {
	channel := newTestChannel(t)
	client, err := channel.NewStream(
		context.Background(),
		new(grpc.StreamDesc),
		"/test.TestService/Unary",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CloseSend(); err != nil {
		t.Fatalf("CloseSend = %v", err)
	}
	err = client.RecvMsg(new(wrapperspb.StringValue))
	if status.Code(err) != codes.Internal {
		t.Fatalf("RecvMsg = %v, want Internal cardinality failure", err)
	}
}

func TestNewStreamGeneratedUnaryLookupFailures(t *testing.T) {
	channel := newTestChannel(t)
	for _, method := range []string{
		"/missing.Service/Unary",
		"/test.TestService/Missing",
	} {
		t.Run(method, func(t *testing.T) {
			_, err := channel.NewStream(
				context.Background(),
				new(grpc.StreamDesc),
				method,
			)
			if status.Code(err) != codes.Unimplemented {
				t.Fatalf("NewStream = %v, want Unimplemented", err)
			}
		})
	}
}

func TestNewStreamGeneratedUnaryAbortReleasesContexts(t *testing.T) {
	serverCanceled := make(chan struct{})
	channel := newBareChannel(t)
	desc := coverageServiceDesc(
		func(
			_ any,
			ctx context.Context,
			decode func(any) error,
			_ grpc.UnaryServerInterceptor,
		) (any, error) {
			request := new(wrapperspb.StringValue)
			if err := decode(request); err != nil {
				return nil, err
			}
			go func() {
				<-ctx.Done()
				close(serverCanceled)
			}()
			return nil, status.Error(codes.PermissionDenied, "denied")
		},
		nil,
	)
	channel.RegisterService(&desc, &echoServer{})
	raw, err := channel.NewStream(
		context.Background(),
		new(grpc.StreamDesc),
		"/test.TestService/Unary",
	)
	if err != nil {
		t.Fatal(err)
	}
	client := raw.(lifecycleClientStream)
	if err := client.SendMsg(new(wrapperspb.StringValue)); err != nil {
		t.Fatal(err)
	}
	err = client.RecvMsg(new(wrapperspb.StringValue))
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("RecvMsg = %v, want PermissionDenied", err)
	}
	waitUnaryLifecycleSignal(t, client.Context().Done(), "client context")
	terminalErr, terminal := client.TerminalResult()
	if !terminal || status.Code(terminalErr) != codes.PermissionDenied {
		t.Fatalf("TerminalResult = %v, %v, want PermissionDenied, true",
			terminalErr,
			terminal,
		)
	}
	waitUnaryLifecycleSignal(t, serverCanceled, "server context")
	waitUnaryLifecycleSignal(t, client.Done(), "Done")
}

func TestNewStreamGeneratedUnaryCallerCancelReleasesContexts(t *testing.T) {
	handlerStarted := make(chan struct{})
	serverCanceled := make(chan struct{})
	channel := newBareChannel(t)
	desc := coverageServiceDesc(
		func(
			_ any,
			ctx context.Context,
			decode func(any) error,
			_ grpc.UnaryServerInterceptor,
		) (any, error) {
			request := new(wrapperspb.StringValue)
			if err := decode(request); err != nil {
				return nil, err
			}
			close(handlerStarted)
			<-ctx.Done()
			close(serverCanceled)
			return nil, ctx.Err()
		},
		nil,
	)
	channel.RegisterService(&desc, &echoServer{})
	ctx, cancel := context.WithCancel(context.Background())
	raw, err := channel.NewStream(
		ctx,
		new(grpc.StreamDesc),
		"/test.TestService/Unary",
	)
	if err != nil {
		t.Fatal(err)
	}
	client := raw.(lifecycleClientStream)
	if err := client.SendMsg(new(wrapperspb.StringValue)); err != nil {
		t.Fatal(err)
	}
	waitUnaryLifecycleSignal(t, handlerStarted, "unary handler")
	cancel()
	err = client.RecvMsg(new(wrapperspb.StringValue))
	if status.Code(err) != codes.Canceled {
		t.Fatalf("RecvMsg = %v, want Canceled", err)
	}
	waitUnaryLifecycleSignal(t, client.Context().Done(), "client context")
	terminalErr, terminal := client.TerminalResult()
	if !terminal || status.Code(terminalErr) != codes.Canceled {
		t.Fatalf("TerminalResult = %v, %v, want Canceled, true",
			terminalErr,
			terminal,
		)
	}
	waitUnaryLifecycleSignal(t, serverCanceled, "server context")
	waitUnaryLifecycleSignal(t, client.Done(), "Done")
}

func waitUnaryLifecycleSignal(
	t *testing.T,
	signal <-chan struct{},
	subject string,
) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s remained open", subject)
	}
}
