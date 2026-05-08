package inprocgrpc_test

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestUnaryCallMessageLimits(t *testing.T) {
	tests := []struct {
		name   string
		option grpc.CallOption
	}{
		{
			name:   "send",
			option: grpc.MaxCallSendMsgSize(1),
		},
		{
			name:   "receive",
			option: grpc.MaxCallRecvMsgSize(1),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := newTestChannel(t)
			err := channel.Invoke(
				context.Background(),
				"/test.TestService/Unary",
				&wrapperspb.StringValue{Value: "larger than one byte"},
				new(wrapperspb.StringValue),
				test.option,
			)
			if status.Code(err) != codes.ResourceExhausted {
				t.Fatalf("Invoke = %v, want ResourceExhausted", err)
			}
		})
	}
}

func TestStreamCallMessageLimits(t *testing.T) {
	t.Run("send", func(t *testing.T) {
		channel := newTestChannel(t)
		client, err := channel.NewStream(
			context.Background(),
			&grpc.StreamDesc{ClientStreams: true, ServerStreams: true},
			"/test.TestService/BidiStream",
			grpc.MaxCallSendMsgSize(1),
		)
		if err != nil {
			t.Fatal(err)
		}
		err = client.SendMsg(&wrapperspb.StringValue{Value: "larger than one byte"})
		if status.Code(err) != codes.ResourceExhausted {
			t.Fatalf("SendMsg = %v, want ResourceExhausted", err)
		}
	})
	t.Run("receive", func(t *testing.T) {
		channel := newTestChannel(t)
		client, err := channel.NewStream(
			context.Background(),
			&grpc.StreamDesc{ServerStreams: true},
			"/test.TestService/ServerStream",
			grpc.MaxCallRecvMsgSize(1),
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := client.SendMsg(&wrapperspb.StringValue{Value: "request"}); err != nil {
			t.Fatal(err)
		}
		err = client.RecvMsg(new(wrapperspb.StringValue))
		if status.Code(err) != codes.ResourceExhausted {
			t.Fatalf("RecvMsg = %v, want ResourceExhausted", err)
		}
	})
}

func TestExplicitZeroMessageLimit(t *testing.T) {
	channel := newTestChannel(t)
	if err := channel.Invoke(
		context.Background(),
		"/test.TestService/Unary",
		new(wrapperspb.StringValue),
		new(wrapperspb.StringValue),
		grpc.MaxCallSendMsgSize(0),
	); err != nil {
		t.Fatalf("zero-byte request: %v", err)
	}
	err := channel.Invoke(
		context.Background(),
		"/test.TestService/Unary",
		&wrapperspb.StringValue{Value: "x"},
		new(wrapperspb.StringValue),
		grpc.MaxCallSendMsgSize(0),
	)
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("non-empty request = %v, want ResourceExhausted", err)
	}
}
