package inprocgrpc_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
)

type offLoopUnaryServer struct {
	loopBusy chan struct{}
	loopFree chan struct{}
}

func (s *offLoopUnaryServer) Unary(
	_ context.Context,
	in *wrapperspb.StringValue,
) (*wrapperspb.StringValue, error) {
	return &wrapperspb.StringValue{Value: "ok:" + in.GetValue()}, nil
}

func (s *offLoopUnaryServer) ServerStream(
	*wrapperspb.StringValue,
	grpc.ServerStream,
) error {
	return nil
}

func (s *offLoopUnaryServer) ClientStream(grpc.ServerStream) error { return nil }
func (s *offLoopUnaryServer) BidiStream(grpc.ServerStream) error   { return nil }

// Without the recvMu barrier the check reads recvCount as 0 and rejects a valid handler.
func TestUnary_AdmittedReceiveAtHandlerReturn_CardinalityError(t *testing.T) {
	loop := newTestLoop(t)
	ch := mustNewChannel(t, inprocgrpc.WithLoop(loop))

	srv := &offLoopUnaryServer{
		loopBusy: make(chan struct{}),
		loopFree: make(chan struct{}),
	}

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
					// Occupy the loop so the receive turn is admitted but cannot complete.
					if err := loop.Submit(func() {
						close(srv.loopBusy)
						<-srv.loopFree
					}); err != nil {
						return nil, err
					}
					<-srv.loopBusy

					go func() {
						in := new(wrapperspb.StringValue)
						_ = dec(in)
					}()

					// Let the decode register its owner turn behind the
					// blocked loop, then return while it is undelivered.
					time.Sleep(50 * time.Millisecond)
					go func() {
						time.Sleep(250 * time.Millisecond)
						close(srv.loopFree)
					}()
					return &wrapperspb.StringValue{Value: "ok"}, nil
				},
			},
		},
	}
	ch.RegisterService(&desc, srv)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp := new(wrapperspb.StringValue)
	err := ch.Invoke(
		ctx,
		"/test.OffLoopUnary/Unary",
		&wrapperspb.StringValue{Value: "hi"},
		resp,
	)
	if err != nil {
		if status.Code(err) == codes.Internal &&
			strings.Contains(err.Error(), "must consume exactly one request") {
			t.Fatalf("spurious cardinality error on a valid handler: %v", err)
		}
		t.Fatalf("Invoke: %v", err)
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
