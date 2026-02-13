package gojagrpc_test

import (
	"context"
	"fmt"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	eventloop "github.com/joeycumines/go-eventloop"
	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	gojagrpc "github.com/joeycumines/goja-grpc"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// exampleDescBytes returns a compiled FileDescriptorSet for examples.
func exampleGrpcDescBytes() []byte {
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{{
			Name:    proto.String("example.proto"),
			Package: proto.String("example"),
			Syntax:  proto.String("proto3"),
			MessageType: []*descriptorpb.DescriptorProto{{
				Name: proto.String("EchoRequest"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("message"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("message")},
				},
			}, {
				Name: proto.String("EchoResponse"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("message"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("message")},
				},
			}},
			Service: []*descriptorpb.ServiceDescriptorProto{{
				Name: proto.String("EchoService"),
				Method: []*descriptorpb.MethodDescriptorProto{{
					Name:       proto.String("Echo"),
					InputType:  proto.String(".example.EchoRequest"),
					OutputType: proto.String(".example.EchoResponse"),
				}},
			}},
		}},
	}
	data, err := proto.Marshal(fds)
	if err != nil {
		panic(err)
	}
	return data
}

func Example() {
	// Create event loop and goja runtime.
	loop, err := eventloop.New()
	if err != nil {
		panic(err)
	}

	rt := goja.New()
	adapter, err := gojaeventloop.New(loop, rt)
	if err != nil {
		panic(err)
	}
	if err := adapter.Bind(); err != nil {
		panic(err)
	}

	// Create in-process gRPC channel.
	channel := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop))

	// Create protobuf module and load descriptors.
	pbMod, err := gojaprotobuf.New(rt)
	if err != nil {
		panic(err)
	}
	if _, err := pbMod.LoadDescriptorSetBytes(exampleGrpcDescBytes()); err != nil {
		panic(err)
	}

	// Register gRPC module via require().
	registry := require.NewRegistry()
	registry.RegisterNativeModule("protobuf", gojaprotobuf.Require())
	registry.RegisterNativeModule("grpc", gojagrpc.Require(
		gojagrpc.WithChannel(channel),
		gojagrpc.WithProtobuf(pbMod),
		gojagrpc.WithAdapter(adapter),
	))
	registry.Enable(rt)

	// Make protobuf available to JS.
	pbExports := rt.NewObject()
	pbMod.SetupExports(pbExports)
	_ = rt.Set("pb", pbExports)

	// Set up done signal.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan string, 1)
	_ = rt.Set("__done", rt.ToValue(func(call goja.FunctionCall) goja.Value {
		done <- call.Argument(0).String()
		return goja.Undefined()
	}))

	// Run JavaScript: register server, create client, make RPC.
	_ = loop.Submit(func() {
		_, jsErr := rt.RunString(`
			const grpc = require('grpc');

			// Register echo server.
			const server = grpc.createServer();
			server.addService('example.EchoService', {
				echo(request) {
					const Resp = pb.messageType('example.EchoResponse');
					const resp = new Resp();
					resp.set('message', 'echo: ' + request.get('message'));
					return resp;
				}
			});
			server.start();

			// Create client and call.
			const client = grpc.createClient('example.EchoService');
			const Req = pb.messageType('example.EchoRequest');
			const req = new Req();
			req.set('message', 'hello');

			client.echo(req).then(resp => {
				__done(resp.get('message'));
			});
		`)
		if jsErr != nil {
			panic(jsErr)
		}
	})

	// Run event loop in background.
	loopDone := make(chan error, 1)
	go func() {
		loopDone <- loop.Run(ctx)
	}()

	select {
	case msg := <-done:
		cancel()
		<-loopDone
		fmt.Println(msg)
	case <-ctx.Done():
		panic("timeout")
	}
	// Output: echo: hello
}
