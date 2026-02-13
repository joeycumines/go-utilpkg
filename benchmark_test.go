package gojagrpc

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	eventloop "github.com/joeycumines/go-eventloop"
	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// benchEnv creates a test environment for benchmarks.
func benchEnv(b *testing.B) *grpcTestEnv {
	b.Helper()
	loop, err := eventloop.New()
	require.NoError(b, err)
	runtime := goja.New()
	adapter, err := gojaeventloop.New(loop, runtime)
	require.NoError(b, err)
	require.NoError(b, adapter.Bind())
	channel := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop))
	pbMod, err := gojaprotobuf.New(runtime)
	require.NoError(b, err)
	_, err = pbMod.LoadDescriptorSetBytes(testGrpcDescriptorSetBytes())
	require.NoError(b, err)
	grpcMod, err := New(runtime,
		WithChannel(channel),
		WithProtobuf(pbMod),
		WithAdapter(adapter),
	)
	require.NoError(b, err)
	pbExports := runtime.NewObject()
	pbMod.SetupExports(pbExports)
	_ = runtime.Set("pb", pbExports)
	grpcExports := runtime.NewObject()
	grpcMod.setupExports(grpcExports)
	_ = runtime.Set("grpc", grpcExports)
	return &grpcTestEnv{
		loop:    loop,
		runtime: runtime,
		adapter: adapter,
		channel: channel,
		pbMod:   pbMod,
		grpcMod: grpcMod,
	}
}

// BenchmarkUnaryRPC benchmarks a full unary RPC round-trip (JS client → JS server).
func BenchmarkUnaryRPC(b *testing.B) {
	env := benchEnv(b)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	setupDone := make(chan struct{}, 1)
	_ = env.runtime.Set("__setupDone", env.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		select {
		case setupDone <- struct{}{}:
		default:
		}
		return goja.Undefined()
	}))

	// Set up server and client.
	_ = env.loop.Submit(func() {
		_, err := env.runtime.RunString(`
			const EchoReq = pb.messageType('testgrpc.EchoRequest');
			const EchoResp = pb.messageType('testgrpc.EchoResponse');

			const server = grpc.createServer();
			server.addService('testgrpc.TestService', {
				echo(request) {
					const resp = new EchoResp();
					resp.set('message', request.get('message'));
					return resp;
				},
				serverStream() {},
				clientStream() {},
				bidiStream() {},
			});
			server.start();

			const client = grpc.createClient('testgrpc.TestService');
			__setupDone();
		`)
		if err != nil {
			b.Fatalf("setup error: %v", err)
		}
	})

	loopDone := make(chan error, 1)
	go func() {
		loopDone <- env.loop.Run(ctx)
	}()

	select {
	case <-setupDone:
	case <-ctx.Done():
		b.Fatal("timeout waiting for setup")
	}

	// Now benchmark the RPC calls.
	b.ResetTimer()
	for b.Loop() {
		rpcDone := make(chan struct{}, 1)
		_ = env.loop.Submit(func() {
			_ = env.runtime.Set("__rpcDone", env.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
				rpcDone <- struct{}{}
				return goja.Undefined()
			}))
			_, _ = env.runtime.RunString(`
				(function() {
					var req = new EchoReq();
					req.set('message', 'bench');
					client.echo(req).then(function() { __rpcDone(); });
				})();
			`)
		})
		<-rpcDone
	}
	b.StopTimer()

	cancel()
	<-loopDone
}

// BenchmarkStatusCreate benchmarks creating gRPC error objects.
func BenchmarkStatusCreate(b *testing.B) {
	env := benchEnv(b)

	b.ResetTimer()
	for b.Loop() {
		_, _ = env.runtime.RunString(`grpc.status.createError(grpc.status.NOT_FOUND, 'not found')`)
	}
}

// BenchmarkMetadataCreate benchmarks creating and populating metadata.
func BenchmarkMetadataCreate(b *testing.B) {
	env := benchEnv(b)

	b.ResetTimer()
	for b.Loop() {
		_, _ = env.runtime.RunString(`
			(function() {
				var md = grpc.metadata.create();
				md.set('key1', 'val1');
				md.set('key2', 'val2');
				md.get('key1');
			})();
		`)
	}
}

// BenchmarkGoDirectUnaryRPC benchmarks a pure-Go unary RPC over the same
// inprocgrpc channel, using dynamicpb messages from the same descriptor set.
// This baseline isolates the JS bridge overhead when compared with
// BenchmarkUnaryRPC.
func BenchmarkGoDirectUnaryRPC(b *testing.B) {
	loop, err := eventloop.New()
	require.NoError(b, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go loop.Run(ctx)

	ch := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop))

	// Parse the test descriptor set to get message descriptors.
	var fds descriptorpb.FileDescriptorSet
	require.NoError(b, proto.Unmarshal(testGrpcDescriptorSetBytes(), &fds))
	files, err := protodesc.NewFiles(&fds)
	require.NoError(b, err)

	reqDescRaw, err := files.FindDescriptorByName("testgrpc.EchoRequest")
	require.NoError(b, err)
	reqMsgDesc := reqDescRaw.(protoreflect.MessageDescriptor)

	respDescRaw, err := files.FindDescriptorByName("testgrpc.EchoResponse")
	require.NoError(b, err)
	respMsgDesc := respDescRaw.(protoreflect.MessageDescriptor)

	// Register a Go handler as a standard grpc.ServiceDesc.
	ch.RegisterService(&grpc.ServiceDesc{
		ServiceName: "testgrpc.TestService",
		Methods: []grpc.MethodDesc{
			{
				MethodName: "Echo",
				Handler: func(srv any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
					in := dynamicpb.NewMessage(reqMsgDesc)
					if err := dec(in); err != nil {
						return nil, err
					}
					out := dynamicpb.NewMessage(respMsgDesc)
					msgField := respMsgDesc.Fields().ByName("message")
					out.Set(msgField, in.Get(reqMsgDesc.Fields().ByName("message")))
					return out, nil
				},
			},
		},
	}, nil)

	// Build request message.
	req := dynamicpb.NewMessage(reqMsgDesc)
	req.Set(reqMsgDesc.Fields().ByName("message"), protoreflect.ValueOfString("bench"))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		resp := dynamicpb.NewMessage(respMsgDesc)
		err := ch.Invoke(ctx, "/testgrpc.TestService/Echo", req, resp)
		if err != nil {
			b.Fatal(err)
		}
	}

	b.StopTimer()
	cancel()
}
