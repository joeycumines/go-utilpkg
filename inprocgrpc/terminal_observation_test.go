package inprocgrpc

import (
	"context"
	"testing"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/go-inprocgrpc/internal/stream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestCallerCancellationAfterServerAbortKeepsServerStatus(t *testing.T) {
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	runCtx, stop := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(runCtx) }()
	t.Cleanup(func() {
		stop()
		<-runDone
	})

	channel := NewChannel(WithLoop(loop))
	callCtx, cancelCall := context.WithCancel(context.Background())
	channel.RegisterStreamHandler(
		"/test.Service/Unary",
		func(_ context.Context, stream *RPCStream) {
			stream.Recv().Recv(func(_ any, err error) {
				if err != nil {
					stream.Abort(err)
					return
				}
				stream.Abort(status.Error(codes.Unavailable, "server stopped"))
				cancelCall()
			})
		},
	)

	err = channel.Invoke(
		callCtx,
		"/test.Service/Unary",
		&wrapperspb.StringValue{Value: "request"},
		new(wrapperspb.StringValue),
	)
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("Invoke = %v, want Unavailable", err)
	}
}

func TestSelectedServerAbortBeatsClosedLoopAndCaller(t *testing.T) {
	loop := newDroppedOwnerLoop()
	state := stream.NewRPCState("/test.Service/Unary", 1)
	callCtx, cancelCall := context.WithCancel(context.Background())
	life := newRPCLifecycle(loop, state, cancelCall)
	selected := status.Error(codes.Unavailable, "server selected")
	if !life.serverAbort(selected) {
		t.Fatal("server abort did not win terminal selection")
	}
	cancelCall()
	loop.close()

	client := &clientStreamAdapter{
		ctx:       callCtx,
		callerCtx: callCtx,
		loop:      loop,
		life:      life,
		state:     state,
	}
	result := client.schedulerReceiveResult()
	if status.Code(result.err) != codes.Unavailable {
		t.Fatalf("scheduler result = %v, want selected Unavailable", result.err)
	}
}
