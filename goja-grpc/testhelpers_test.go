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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// grpcTestEnv provides a fully wired test environment with event loop,
// goja runtime, protobuf module (with loaded test descriptors), and
// gRPC module. Suitable for both synchronous tests (status, metadata)
// and asynchronous tests (RPCs).
type grpcTestEnv struct {
	loop    *eventloop.Loop
	runtime *goja.Runtime
	adapter *gojaeventloop.Adapter
	channel *inprocgrpc.Channel
	pbMod   *gojaprotobuf.Module
	grpcMod *Module
}

// newGrpcTestEnv creates the full test environment. The event loop is
// created but NOT started — call [grpcTestEnv.runOnLoop] for async
// tests, or [grpcTestEnv.run] for synchronous JS evaluation.
func newGrpcTestEnv(t *testing.T) *grpcTestEnv {
	t.Helper()

	loop, err := eventloop.New()
	require.NoError(t, err)

	runtime := goja.New()

	adapter, err := gojaeventloop.New(loop, runtime)
	require.NoError(t, err)
	require.NoError(t, adapter.Bind())

	channel := inprocgrpc.NewChannel(loop)

	pbMod, err := gojaprotobuf.New(runtime)
	require.NoError(t, err)

	// Load test service descriptors.
	_, err = pbMod.LoadDescriptorSetBytes(testGrpcDescriptorSetBytes())
	require.NoError(t, err)

	grpcMod, err := New(runtime,
		WithChannel(channel),
		WithProtobuf(pbMod),
		WithAdapter(adapter),
	)
	require.NoError(t, err)

	// Wire JS exports.
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

// run executes JS code synchronously on the runtime (no event loop).
// Suitable for testing pure synchronous APIs like status and metadata.
func (e *grpcTestEnv) run(t *testing.T, code string) goja.Value {
	t.Helper()
	v, err := e.runtime.RunString(code)
	require.NoError(t, err)
	return v
}

// mustFail runs JS code and asserts that it throws an exception.
func (e *grpcTestEnv) mustFail(t *testing.T, code string) error {
	t.Helper()
	_, err := e.runtime.RunString(code)
	require.Error(t, err)
	return err
}

// runOnLoop submits JS code for execution on the event loop, then
// runs the loop until the JS code signals completion via __done().
//
// The JS code MUST call __done() when all async operations have completed.
// Store results in global variables (e.g. "result", "error") which
// can be read from e.runtime after this call returns.
func (e *grpcTestEnv) runOnLoop(t *testing.T, code string, timeout time.Duration) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	done := make(chan struct{}, 1)
	var jsErr error

	_ = e.runtime.Set("__done", e.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		select {
		case done <- struct{}{}:
		default:
		}
		return goja.Undefined()
	}))

	if submitErr := e.loop.Submit(func() {
		_, jsErr = e.runtime.RunString(code)
	}); submitErr != nil {
		t.Fatalf("submit error: %v", submitErr)
	}

	// Run loop in background goroutine.
	loopDone := make(chan error, 1)
	go func() {
		loopDone <- e.loop.Run(ctx)
	}()

	// Wait for JS to signal completion or timeout.
	select {
	case <-done:
		cancel()   // Stop the event loop.
		<-loopDone // Wait for loop goroutine to finish.
	case err := <-loopDone:
		if jsErr != nil {
			t.Fatalf("JS error: %v", jsErr)
		}
		if err != nil && ctx.Err() == nil {
			t.Fatalf("loop error: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("timeout waiting for __done()")
	}

	if jsErr != nil {
		t.Fatalf("JS error: %v", jsErr)
	}
}

// shutdown cleanly shuts down the event loop.
func (e *grpcTestEnv) shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	e.loop.Shutdown(ctx)
}

// ======================== Test Descriptors ==========================

// testGrpcDescriptorSetBytes returns the serialized FileDescriptorSet
// for the test gRPC service, including EchoRequest, EchoResponse, Item
// messages and a TestService with all four RPC types.
func testGrpcDescriptorSetBytes() []byte {
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{testGrpcFileDescriptorProto()},
	}
	data, err := proto.Marshal(fds)
	if err != nil {
		panic("testGrpcDescriptorSetBytes: " + err.Error())
	}
	return data
}

// testGrpcFileDescriptorProto builds the file descriptor for package
// "testgrpc" containing:
//
//   - EchoRequest  { string message = 1; }
//   - EchoResponse { string message = 1; int32 code = 2; }
//   - Item         { string id = 1; string name = 2; }
//   - TestService
//   - Echo(EchoRequest) returns (EchoResponse)                 — unary
//   - ServerStream(EchoRequest) returns (stream Item)          — server-streaming
//   - ClientStream(stream Item) returns (EchoResponse)         — client-streaming
//   - BidiStream(stream Item) returns (stream Item)            — bidi-streaming
func testGrpcFileDescriptorProto() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:    proto.String("testgrpc.proto"),
		Package: proto.String("testgrpc"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			echoRequestDesc(),
			echoResponseDesc(),
			itemDesc(),
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			testServiceDesc(),
		},
	}
}

func echoRequestDesc() *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name: proto.String("EchoRequest"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{
				Name:     proto.String("message"),
				Number:   proto.Int32(1),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				JsonName: proto.String("message"),
			},
		},
	}
}

func echoResponseDesc() *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name: proto.String("EchoResponse"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{
				Name:     proto.String("message"),
				Number:   proto.Int32(1),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				JsonName: proto.String("message"),
			},
			{
				Name:     proto.String("code"),
				Number:   proto.Int32(2),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
				Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				JsonName: proto.String("code"),
			},
		},
	}
}

func itemDesc() *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name: proto.String("Item"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{
				Name:     proto.String("id"),
				Number:   proto.Int32(1),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				JsonName: proto.String("id"),
			},
			{
				Name:     proto.String("name"),
				Number:   proto.Int32(2),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				JsonName: proto.String("name"),
			},
		},
	}
}

func testServiceDesc() *descriptorpb.ServiceDescriptorProto {
	return &descriptorpb.ServiceDescriptorProto{
		Name: proto.String("TestService"),
		Method: []*descriptorpb.MethodDescriptorProto{
			{
				Name:       proto.String("Echo"),
				InputType:  proto.String(".testgrpc.EchoRequest"),
				OutputType: proto.String(".testgrpc.EchoResponse"),
			},
			{
				Name:            proto.String("ServerStream"),
				InputType:       proto.String(".testgrpc.EchoRequest"),
				OutputType:      proto.String(".testgrpc.Item"),
				ServerStreaming: proto.Bool(true),
			},
			{
				Name:            proto.String("ClientStream"),
				InputType:       proto.String(".testgrpc.Item"),
				OutputType:      proto.String(".testgrpc.EchoResponse"),
				ClientStreaming: proto.Bool(true),
			},
			{
				Name:            proto.String("BidiStream"),
				InputType:       proto.String(".testgrpc.Item"),
				OutputType:      proto.String(".testgrpc.Item"),
				ClientStreaming: proto.Bool(true),
				ServerStreaming: proto.Bool(true),
			},
		},
	}
}
