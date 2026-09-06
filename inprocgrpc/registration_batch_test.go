package inprocgrpc_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestRegisterBatchCollisionPublishesNothing(t *testing.T) {
	channel := newTestChannel(t)
	unique := &grpc.ServiceDesc{
		ServiceName: "batch.Unique",
		Methods:     []grpc.MethodDesc{{MethodName: "Unary"}},
	}
	err := channel.RegisterBatch(inprocgrpc.RegistrationBatch{
		Services: []inprocgrpc.ServiceRegistration{
			{Descriptor: unique, Implementation: struct{}{}},
			{Descriptor: &testServiceDesc, Implementation: &echoServer{}},
		},
		StreamHandlers: []inprocgrpc.StreamHandlerRegistration{{
			Method: "/batch.Unique/Unary",
			Handler: func(context.Context, *inprocgrpc.RPCStream) {
				t.Error("handler from failed batch was invoked")
			},
		}},
	})
	if err == nil {
		t.Fatal("batch collision was accepted")
	}
	if _, exists := channel.GetServiceInfo()["batch.Unique"]; exists {
		t.Fatal("failed batch published its unique service")
	}
	err = channel.Invoke(
		context.Background(),
		"/batch.Unique/Unary",
		&wrapperspb.StringValue{},
		&wrapperspb.StringValue{},
	)
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("failed batch method = %v, want Unimplemented", err)
	}
	response := new(wrapperspb.StringValue)
	if err := channel.Invoke(
		context.Background(),
		"/test.TestService/Unary",
		&wrapperspb.StringValue{Value: "original"},
		response,
	); err != nil {
		t.Fatal(err)
	}
	if response.Value != "echo: original" {
		t.Fatalf("original service response = %q", response.Value)
	}
}

func TestRegisterBatchStreamCollisionPublishesNothing(t *testing.T) {
	channel := newBareChannel(t)
	channel.RegisterStreamHandler(
		"/batch.Seed/Unary",
		func(context.Context, *inprocgrpc.RPCStream) {},
	)
	unique := &grpc.ServiceDesc{ServiceName: "batch.Late"}
	err := channel.RegisterBatch(inprocgrpc.RegistrationBatch{
		Services: []inprocgrpc.ServiceRegistration{{
			Descriptor: unique, Implementation: struct{}{},
		}},
		StreamHandlers: []inprocgrpc.StreamHandlerRegistration{
			{
				Method:  "/batch.Late/Unary",
				Handler: func(context.Context, *inprocgrpc.RPCStream) {},
			},
			{
				Method:  "/batch.Seed/Unary",
				Handler: func(context.Context, *inprocgrpc.RPCStream) {},
			},
		},
	})
	if err == nil {
		t.Fatal("existing stream-handler collision was accepted")
	}
	if _, exists := channel.GetServiceInfo()["batch.Late"]; exists {
		t.Fatal("failed batch published its unique service")
	}
}

func TestRegisterBatchRejectsInvalidShapeBeforePublication(t *testing.T) {
	channel := newBareChannel(t)
	requirePanicContains(t, "duplicate stream handler", func() {
		_ = channel.RegisterBatch(inprocgrpc.RegistrationBatch{
			Services: []inprocgrpc.ServiceRegistration{{
				Descriptor:     &grpc.ServiceDesc{ServiceName: "batch.Shape"},
				Implementation: struct{}{},
			}},
			StreamHandlers: []inprocgrpc.StreamHandlerRegistration{
				{
					Method:  "/batch.Shape/Unary",
					Handler: func(context.Context, *inprocgrpc.RPCStream) {},
				},
				{
					Method:  "/batch.Shape/Unary",
					Handler: func(context.Context, *inprocgrpc.RPCStream) {},
				},
			},
		})
	})
	if info := channel.GetServiceInfo(); len(info) != 0 {
		t.Fatalf("invalid batch published services: %v", info)
	}
	for _, method := range []string{
		"",
		"/",
		"/service",
		"/service/",
		"/service/method/extra",
	} {
		t.Run(method, func(t *testing.T) {
			requirePanicContains(t, "must have form", func() {
				_ = channel.RegisterBatch(
					inprocgrpc.RegistrationBatch{
						StreamHandlers: []inprocgrpc.StreamHandlerRegistration{{
							Method: method,
							Handler: func(
								context.Context,
								*inprocgrpc.RPCStream,
							) {
							},
						}},
					},
				)
			})
		})
	}
}

func TestRegisterBatchSynchronizesReaders(t *testing.T) {
	channel := newBareChannel(t)
	const count = 64
	channel.RegisterStreamHandler(
		"/batch.Stable/Unary",
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
	start := make(chan struct{})
	readerErrors := make(chan error, 8)
	var readers sync.WaitGroup
	for range 8 {
		readers.Go(func() {
			<-start
			for range count {
				_ = channel.GetServiceInfo()
				response := new(wrapperspb.StringValue)
				if err := channel.Invoke(
					context.Background(),
					"/batch.Stable/Unary",
					&wrapperspb.StringValue{Value: "stable"},
					response,
				); err != nil {
					readerErrors <- err
					return
				}
				if response.Value != "stable" {
					readerErrors <- fmt.Errorf(
						"stable response = %q",
						response.Value,
					)
					return
				}
			}
		})
	}
	close(start)
	for index := range count {
		name := fmt.Sprintf("batch.Race%d", index)
		if err := channel.RegisterBatch(inprocgrpc.RegistrationBatch{
			Services: []inprocgrpc.ServiceRegistration{{
				Descriptor: &grpc.ServiceDesc{
					ServiceName: name,
					Methods: []grpc.MethodDesc{{
						MethodName: "Unary",
					}},
				},
				Implementation: struct{}{},
			}},
			StreamHandlers: []inprocgrpc.StreamHandlerRegistration{{
				Method:  "/" + name + "/Unary",
				Handler: func(context.Context, *inprocgrpc.RPCStream) {},
			}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	readers.Wait()
	close(readerErrors)
	for err := range readerErrors {
		t.Fatal(err)
	}
	if got := len(channel.GetServiceInfo()); got != count {
		t.Fatalf("registered services = %d, want %d", got, count)
	}
}
