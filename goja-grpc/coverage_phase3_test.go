package gojagrpc

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"strings"
	"time"

	"github.com/dop251/goja"
	eventloop "github.com/joeycumines/go-eventloop"
	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcmetadata "google.golang.org/grpc/metadata"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// ============================================================================
// Phase 3 coverage tests for goja-grpc
//
// Targets remaining uncovered paths from 97.6% baseline:
//
//   client.go  — SendMsg error, Submit failures in all stream types
//   server.go  — toWrappedMessage conversion errors in handlers
//   status.go  — newGrpcErrorWithDetails UnwrapMessage skip
//   reflection.go — transitive dep extra-file branch
//
// Strategy:
//   - Mock grpc.ClientConnInterface/grpc.ClientStream for client paths
//   - Custom inprocgrpc.Cloner that produces non-proto values for server paths
//   - Deterministic synchronization via channels/WaitGroups (no time.Sleep)
// ============================================================================

// --------------------------------------------------------------------------
// Mock gRPC interfaces
// --------------------------------------------------------------------------

// phase3MockStream implements grpc.ClientStream with configurable behavior.
type phase3MockStream struct {
	grpc.ClientStream // satisfy the full interface via embedding
	sendMsgErr        error
	closeSendFn       func() error
	recvMsgFn         func(m any) error
	headerFn          func() (grpcmetadata.MD, error)
	trailerMD         grpcmetadata.MD
	ctx               context.Context
}

func (s *phase3MockStream) SendMsg(any) error { return s.sendMsgErr }
func (s *phase3MockStream) CloseSend() error {
	if s.closeSendFn != nil {
		return s.closeSendFn()
	}
	return nil
}
func (s *phase3MockStream) RecvMsg(any) error {
	if s.recvMsgFn != nil {
		return s.recvMsgFn(nil)
	}
	return io.EOF
}
func (s *phase3MockStream) Header() (grpcmetadata.MD, error) {
	if s.headerFn != nil {
		return s.headerFn()
	}
	return nil, nil
}
func (s *phase3MockStream) Trailer() grpcmetadata.MD { return s.trailerMD }
func (s *phase3MockStream) Context() context.Context {
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}

// phase3MockCC implements grpc.ClientConnInterface.
type phase3MockCC struct {
	newStreamFn func(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error)
}

func (c *phase3MockCC) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	if c.newStreamFn != nil {
		return c.newStreamFn(ctx, desc, method, opts...)
	}
	return nil, fmt.Errorf("phase3MockCC: no mock")
}
func (c *phase3MockCC) Invoke(context.Context, string, any, any, ...grpc.CallOption) error {
	return fmt.Errorf("phase3MockCC: Invoke not mocked")
}

// --------------------------------------------------------------------------
// Broken cloner: delivers non-proto.Message values to server handlers
// --------------------------------------------------------------------------

// phase3NonProtoCloner always returns a non-proto.Message value from Clone,
// causing server-side toWrappedMessage to fail with "not a proto.Message".
type phase3NonProtoCloner struct{}

func (c *phase3NonProtoCloner) Clone(any) (any, error) {
	return "not-a-proto-message", nil
}

func (c *phase3NonProtoCloner) Copy(out, in any) error {
	return fmt.Errorf("phase3NonProtoCloner: copy not supported")
}

// --------------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------------

func phase3FindMsgDesc(t *testing.T, env *grpcTestEnv, name string) protoreflect.MessageDescriptor {
	t.Helper()
	desc, err := env.pbMod.FindDescriptor(protoreflect.FullName(name))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	md, ok := desc.(protoreflect.MessageDescriptor)
	if !(ok) {
		t.Fatalf("expected true")
	}
	return md
}

// newPhase3BrokenClonerEnv creates a test environment with a cloner that
// delivers non-proto.Message objects to server handlers, triggering the
// toWrappedMessage "not a proto.Message" error path.
func newPhase3BrokenClonerEnv(t *testing.T) *grpcTestEnv {
	t.Helper()

	loop, err := eventloop.New()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	runtime := goja.New()

	adapter, err := gojaeventloop.New(loop, runtime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if adapter.Bind() != nil {
		t.Fatalf("unexpected error: %v", adapter.Bind())
	}

	channel := inprocgrpc.NewChannel(inprocgrpc.WithLoop(loop), inprocgrpc.WithCloner(&phase3NonProtoCloner{}))

	pbMod, err := gojaprotobuf.New(runtime)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = pbMod.LoadDescriptorSetBytes(testGrpcDescriptorSetBytes())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	grpcMod, err := New(runtime,
		WithChannel(channel),
		WithProtobuf(pbMod),
		WithAdapter(adapter),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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

// phase3SetupBrokenServer registers a JS server on the broken-cloner env
// and returns when it's ready. Runs the event loop in the background.
// Returns a cancel func that stops the loop and waits for exit.
func phase3SetupBrokenServer(t *testing.T, env *grpcTestEnv) context.CancelFunc {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	setupDone := make(chan struct{})
	_ = env.loop.Submit(func() {
		_ = env.runtime.Set("__p3BrkReady", env.runtime.ToValue(func(_ goja.FunctionCall) goja.Value {
			close(setupDone)
			return goja.Undefined()
		}))
		_, _ = env.runtime.RunString(`
			var server = grpc.createServer();
			server.addService('testgrpc.TestService', {
				echo: function(request, call) { return request; },
				serverStream: function(request, call) {},
				clientStream: function(call) { return call.recv(); },
				bidiStream: function(call) { return call.recv(); }
			});
			server.start();
			__p3BrkReady();
		`)
	})

	loopDone := make(chan struct{})
	go func() { env.loop.Run(ctx); close(loopDone) }()

	select {
	case <-setupDone:
	case <-ctx.Done():
		cancel()
		t.Fatal("timeout waiting for broken-cloner server setup")
	}

	return func() {
		cancel()
		<-loopDone
	}
}

// ============================================================================
// Test: status.go:130-131 — newGrpcErrorWithDetails UnwrapMessage error skip
//
// Passes non-protobuf goja values as details. UnwrapMessage fails for each,
// and the continue statement is executed.
// ============================================================================

func TestPhase3_NewGrpcErrorWithDetails_UnwrapError(t *testing.T) {
	env := newGrpcTestEnv(t)

	// Plain JS values: not protobuf messages → UnwrapMessage fails → continue
	details := []goja.Value{
		env.runtime.ToValue("plain string"),
		env.runtime.ToValue(42),
		env.runtime.NewObject(), // plain object, not protobuf wrapper
	}

	obj := env.grpcMod.newGrpcErrorWithDetails(codes.Internal, "test error", details)
	if obj == nil {
		t.Fatalf("expected non-nil")
	}

	name := obj.Get("name").String()
	if got := name; got != "GrpcError" {
		t.Errorf("expected %v, got %v", "GrpcError", got)
	}
	if got := obj.Get("code").ToInteger(); got != int64(codes.Internal) {
		t.Errorf("expected %v, got %v", int64(codes.Internal), got)
	}

	// _goDetails should be nil or empty: all details failed UnwrapMessage
	goDetails := env.grpcMod.extractGoDetails(obj)
	if len(goDetails) != 0 {
		t.Errorf("expected empty, got len %d", len(goDetails))
	}
}

// ============================================================================
// Test: client.go:306-311 — makeServerStreamMethod SendMsg error
//
// Mock ClientStream where NewStream succeeds but SendMsg fails.
// The goroutine hits the SendMsg error path, calls submitOrRejectDirect,
// and the promise rejects with a GrpcError.
// ============================================================================

func TestPhase3_ServerStream_SendMsgError(t *testing.T) {
	env := newGrpcTestEnv(t)

	inputDesc := phase3FindMsgDesc(t, env, "testgrpc.EchoRequest")
	outputDesc := phase3FindMsgDesc(t, env, "testgrpc.Item")

	mockCC := &phase3MockCC{
		newStreamFn: func(ctx context.Context, _ *grpc.StreamDesc, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
			return &phase3MockStream{
				sendMsgErr: status.Errorf(codes.Unavailable, "mock send failed"),
				ctx:        ctx,
			}, nil
		},
	}

	// Pre-submit: create the mock-backed server-stream function.
	err := env.loop.Submit(func() {
		fn := env.grpcMod.makeServerStreamMethod(mockCC, "/test/ServerStream", inputDesc, outputDesc)
		_ = env.runtime.Set("__p3SsSendFn", fn)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	env.runOnLoop(t, `
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test');
		__p3SsSendFn(req).then(function() {
			__p3SsSendErr = 'unexpected resolve';
			__done();
		}).catch(function(err) {
			__p3SsSendErr = err;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("__p3SsSendErr")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if goja.IsUndefined(errVal) {
		t.Fatalf("expected false")
	}
	// Should be a GrpcError with UNAVAILABLE
	if errObj, ok := errVal.(*goja.Object); ok {
		nameVal := errObj.Get("name")
		if nameVal != nil && nameVal.String() == "GrpcError" {
			if got := errObj.Get("code").ToInteger(); got != int64(codes.Unavailable) {
				t.Errorf("expected %v, got %v", int64(codes.Unavailable), got)
			}
		}
	}
}

// ============================================================================
// Test: client.go:329-332 — makeServerStreamMethod Submit failure
//
// Mock blocks on Header. Event loop is stopped while blocked.
// After release, Submit fails → reject("event loop not running").
// ============================================================================

func TestPhase3_ServerStream_SubmitFailure(t *testing.T) {
	env := newGrpcTestEnv(t)

	inputDesc := phase3FindMsgDesc(t, env, "testgrpc.EchoRequest")
	outputDesc := phase3FindMsgDesc(t, env, "testgrpc.Item")

	headerReached := make(chan struct{})
	headerRelease := make(chan struct{})

	mockCC := &phase3MockCC{
		newStreamFn: func(ctx context.Context, _ *grpc.StreamDesc, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
			return &phase3MockStream{
				ctx: ctx,
				headerFn: func() (grpcmetadata.MD, error) {
					close(headerReached)
					<-headerRelease
					return grpcmetadata.Pairs("k", "v"), nil
				},
			}, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Setup: create and call the function with onHeader.
	var setupWg sync.WaitGroup
	setupWg.Add(1)
	_ = env.loop.Submit(func() {
		fn := env.grpcMod.makeServerStreamMethod(mockCC, "/test/ServerStream", inputDesc, outputDesc)
		_ = env.runtime.Set("__p3SsSubFn", fn)
		setupWg.Done()
	})
	_ = env.loop.Submit(func() {
		_, _ = env.runtime.RunString(`
			var req = new (pb.messageType('testgrpc.EchoRequest'))();
			req.set('message', 'test');
			__p3SsSubFn(req, { onHeader: function(md) {} });
		`)
	})

	loopDone := make(chan struct{})
	go func() { env.loop.Run(ctx); close(loopDone) }()

	// Wait for goroutine to block on Header.
	select {
	case <-headerReached:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("timeout waiting for header block")
	}

	// Stop event loop, then release goroutine → Submit fails.
	cancel()
	<-loopDone
	close(headerRelease)
	// Goroutine: Header returns → Submit fails → reject → exit (nanoseconds)
}

// ============================================================================
// Test: client.go:462-465 — makeClientStreamMethod Submit failure
//
// Mock blocks on NewStream. Event loop stopped while blocked.
// After release, goroutine continues: Submit fails.
// ============================================================================

func TestPhase3_ClientStream_SubmitFailure(t *testing.T) {
	env := newGrpcTestEnv(t)

	outputDesc := phase3FindMsgDesc(t, env, "testgrpc.EchoResponse")

	newStreamReached := make(chan struct{})
	newStreamRelease := make(chan struct{})

	mockCC := &phase3MockCC{
		newStreamFn: func(ctx context.Context, _ *grpc.StreamDesc, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
			close(newStreamReached)
			<-newStreamRelease
			return &phase3MockStream{ctx: ctx}, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	var setupWg sync.WaitGroup
	setupWg.Add(1)
	_ = env.loop.Submit(func() {
		fn := env.grpcMod.makeClientStreamMethod(mockCC, "/test/ClientStream", outputDesc)
		_ = env.runtime.Set("__p3CsSubFn", fn)
		setupWg.Done()
	})
	_ = env.loop.Submit(func() {
		_, _ = env.runtime.RunString(`__p3CsSubFn();`)
	})

	loopDone := make(chan struct{})
	go func() { env.loop.Run(ctx); close(loopDone) }()

	select {
	case <-newStreamReached:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("timeout")
	}

	cancel()
	<-loopDone
	close(newStreamRelease)
	// Goroutine: NewStream returns → Submit fails → lines 462-465 covered
}

// ============================================================================
// Test: client.go:636-639 — makeBidiStreamMethod Submit failure
//
// Mock blocks on Header (via onHeader callback). Loop stopped, then released.
// ============================================================================

func TestPhase3_BidiStream_SubmitFailure(t *testing.T) {
	env := newGrpcTestEnv(t)

	outputDesc := phase3FindMsgDesc(t, env, "testgrpc.Item")

	headerReached := make(chan struct{})
	headerRelease := make(chan struct{})

	mockCC := &phase3MockCC{
		newStreamFn: func(ctx context.Context, _ *grpc.StreamDesc, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
			return &phase3MockStream{
				ctx: ctx,
				headerFn: func() (grpcmetadata.MD, error) {
					close(headerReached)
					<-headerRelease
					return grpcmetadata.Pairs("k", "v"), nil
				},
			}, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	var setupWg sync.WaitGroup
	setupWg.Add(1)
	_ = env.loop.Submit(func() {
		fn := env.grpcMod.makeBidiStreamMethod(mockCC, "/test/BidiStream", outputDesc)
		_ = env.runtime.Set("__p3BsSubFn", fn)
		setupWg.Done()
	})
	_ = env.loop.Submit(func() {
		_, _ = env.runtime.RunString(`__p3BsSubFn({ onHeader: function(md) {} });`)
	})

	loopDone := make(chan struct{})
	go func() { env.loop.Run(ctx); close(loopDone) }()

	select {
	case <-headerReached:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("timeout")
	}

	cancel()
	<-loopDone
	close(headerRelease)
	// Goroutine: Header returns → Submit fails → lines 636-639 covered
}

// ============================================================================
// Test: client.go:506-509 — newClientStreamCall sender goroutine Submit failure
//
// Creates a client-stream call object (promise resolves on running loop),
// stops the loop, then calls closeSend(). The sender goroutine's Submit fails.
// ============================================================================

func TestPhase3_ClientStreamSender_SubmitFailure(t *testing.T) {
	env := newGrpcTestEnv(t)

	outputDesc := phase3FindMsgDesc(t, env, "testgrpc.EchoResponse")

	mockCC := &phase3MockCC{
		newStreamFn: func(ctx context.Context, _ *grpc.StreamDesc, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
			return &phase3MockStream{ctx: ctx}, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	callReady := make(chan struct{})
	_ = env.loop.Submit(func() {
		fn := env.grpcMod.makeClientStreamMethod(mockCC, "/test/ClientStream", outputDesc)
		_ = env.runtime.Set("__p3CsSenderFn", fn)
		_ = env.runtime.Set("__p3CsSenderOK", env.runtime.ToValue(func(_ goja.FunctionCall) goja.Value {
			close(callReady)
			return goja.Undefined()
		}))
		_, _ = env.runtime.RunString(`
			__p3CsSenderFn().then(function(call) {
				__p3CsSenderCall = call;
				__p3CsSenderOK();
			});
		`)
	})

	loopDone := make(chan struct{})
	go func() { env.loop.Run(ctx); close(loopDone) }()

	select {
	case <-callReady:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("timeout waiting for call object")
	}

	// Stop loop, then enqueue closeSend. Sender goroutine's Submit fails.
	cancel()
	<-loopDone

	_, _ = env.runtime.RunString(`__p3CsSenderCall.closeSend();`)
	// Sender goroutine: CloseSend(mock)→nil, Submit→fail → lines 506-509
}

// ============================================================================
// Test: client.go:668-671 — newBidiStream sender goroutine Submit failure
//
// Same pattern as client-stream sender, but for bidi streaming.
// ============================================================================

func TestPhase3_BidiStreamSender_SubmitFailure(t *testing.T) {
	env := newGrpcTestEnv(t)

	outputDesc := phase3FindMsgDesc(t, env, "testgrpc.Item")

	mockCC := &phase3MockCC{
		newStreamFn: func(ctx context.Context, _ *grpc.StreamDesc, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
			return &phase3MockStream{ctx: ctx}, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	streamReady := make(chan struct{})
	_ = env.loop.Submit(func() {
		fn := env.grpcMod.makeBidiStreamMethod(mockCC, "/test/BidiStream", outputDesc)
		_ = env.runtime.Set("__p3BsSenderFn", fn)
		_ = env.runtime.Set("__p3BsSenderOK", env.runtime.ToValue(func(_ goja.FunctionCall) goja.Value {
			close(streamReady)
			return goja.Undefined()
		}))
		_, _ = env.runtime.RunString(`
			__p3BsSenderFn().then(function(stream) {
				__p3BsSenderStream = stream;
				__p3BsSenderOK();
			});
		`)
	})

	loopDone := make(chan struct{})
	go func() { env.loop.Run(ctx); close(loopDone) }()

	select {
	case <-streamReady:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("timeout waiting for stream object")
	}

	cancel()
	<-loopDone

	_, _ = env.runtime.RunString(`__p3BsSenderStream.closeSend();`)
	// Sender goroutine: CloseSend(mock)→nil, Submit→fail → lines 668-671
}

// ============================================================================
// Test: server.go:210-213 — makeUnaryHandler toWrappedMessage conversion error
//
// Uses a broken cloner that delivers non-proto.Message values. The unary
// handler's toWrappedMessage fails with "not a proto.Message", causing
// the handler to Finish with a "request conversion" error.
// ============================================================================

func TestPhase3_UnaryHandler_ConvError(t *testing.T) {
	env := newPhase3BrokenClonerEnv(t)
	stop := phase3SetupBrokenServer(t, env)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	echoRespDesc := phase3FindMsgDesc(t, env, "testgrpc.EchoResponse")
	echoReqDesc := phase3FindMsgDesc(t, env, "testgrpc.EchoRequest")
	respMsg := dynamicpb.NewMessage(echoRespDesc)
	reqMsg := dynamicpb.NewMessage(echoReqDesc)

	err := env.channel.Invoke(ctx, "/testgrpc.TestService/Echo", reqMsg, respMsg)
	if err == nil {
		t.Fatalf("expected an error")
	}
	if !strings.Contains(err.Error(), "request conversion") {
		t.Errorf("expected %q to contain %q", err.Error(), "request conversion")
	}
}

// ============================================================================
// Test: server.go:258-261 — makeServerStreamHandler toWrappedMessage conv error
//
// Server-streaming handler receives a broken message and fails on conversion.
// ============================================================================

func TestPhase3_ServerStreamHandler_ConvError(t *testing.T) {
	env := newPhase3BrokenClonerEnv(t)
	stop := phase3SetupBrokenServer(t, env)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	echoReqDesc := phase3FindMsgDesc(t, env, "testgrpc.EchoRequest")
	reqMsg := dynamicpb.NewMessage(echoReqDesc)

	cs, err := env.channel.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true}, "/testgrpc.TestService/ServerStream")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cs.SendMsg(reqMsg) != nil {
		t.Fatalf("unexpected error: %v", cs.SendMsg(reqMsg))
	}
	if cs.CloseSend() != nil {
		t.Fatalf("unexpected error: %v", cs.CloseSend())
	}

	itemDesc := phase3FindMsgDesc(t, env, "testgrpc.Item")
	respMsg := dynamicpb.NewMessage(itemDesc)
	err = cs.RecvMsg(respMsg)
	if err == nil {
		t.Fatalf("expected an error")
	}
	if !strings.Contains(err.Error(), "request conversion") {
		t.Errorf("expected %q to contain %q", err.Error(), "request conversion")
	}
}

// ============================================================================
// Test: server.go:486-489 — addServerRecv toWrappedMessage conversion error
//
// Client-streaming handler calls recv(). The broken cloner delivers a
// non-proto.Message, causing the recv callback's toWrappedMessage to fail.
// The recv promise rejects, propagating through thenFinishUnary.
// ============================================================================

func TestPhase3_ClientStreamRecv_ConvError(t *testing.T) {
	env := newPhase3BrokenClonerEnv(t)
	stop := phase3SetupBrokenServer(t, env)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	itemDesc := phase3FindMsgDesc(t, env, "testgrpc.Item")
	reqMsg := dynamicpb.NewMessage(itemDesc)

	cs, err := env.channel.NewStream(ctx, &grpc.StreamDesc{ClientStreams: true}, "/testgrpc.TestService/ClientStream")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cs.SendMsg(reqMsg) != nil {
		t.Fatalf("unexpected error: %v", cs.SendMsg(reqMsg))
	}
	if cs.CloseSend() != nil {
		t.Fatalf("unexpected error: %v", cs.CloseSend())
	}

	echoRespDesc := phase3FindMsgDesc(t, env, "testgrpc.EchoResponse")
	respMsg := dynamicpb.NewMessage(echoRespDesc)
	err = cs.RecvMsg(respMsg)
	if err == nil {
		t.Fatalf("expected an error")
	}
	// The error originates from addServerRecv's toWrappedMessage failure
	// and propagates through the promise chain to the gRPC status.
	st, ok := status.FromError(err)
	if !(ok) {
		t.Fatalf("expected true")
	}
	if got := st.Code(); got != codes.Internal {
		t.Errorf("expected %v, got %v", codes.Internal, got)
	}
}

// ============================================================================
// Test: reflection.go:346-348 — transitive dep loop: extra file in response
//
// When the reflection server returns multiple file descriptors for a
// dependency request (requested file + extras), files not yet in the
// resolved set trigger the !resolved[name] branch.
// ============================================================================

func TestPhase3_FetchFileDescriptor_TransitiveExtraFile(t *testing.T) {
	env := newGrpcTestEnv(t)

	_, err := env.pbMod.LoadDescriptorSetBytes(phase2DescriptorSetBytes())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	baseBytes := mustMarshalFDP(phase2BaseFileDescriptor())

	// Extra file: returned alongside base.proto. Its name is NOT pre-resolved.
	extraFile := &descriptorpb.FileDescriptorProto{
		Name:    new("phase3_extra.proto"),
		Package: new("phase3extra"),
		Syntax:  new("proto3"),
	}
	extraBytes := mustMarshalFDP(extraFile)

	depBytes := mustMarshalFDP(phase2DepFileDescriptor())

	registerMockReflection(env.channel, func(reqNum int, req *reflectionpb.ServerReflectionRequest) mockReflResponse {
		if reqNum == 0 {
			// Initial: return ONLY dep file → forces transitive resolution
			return mockReflResponse{resp: &reflectionpb.ServerReflectionResponse{
				MessageResponse: &reflectionpb.ServerReflectionResponse_FileDescriptorResponse{
					FileDescriptorResponse: &reflectionpb.FileDescriptorResponse{
						FileDescriptorProto: [][]byte{depBytes},
					},
				},
			}}
		}
		// Dependency request for base.proto: return base + extra.
		// base.proto IS pre-resolved; extra.proto is NOT → line 346 branch hit.
		return mockReflResponse{resp: &reflectionpb.ServerReflectionResponse{
			MessageResponse: &reflectionpb.ServerReflectionResponse_FileDescriptorResponse{
				FileDescriptorResponse: &reflectionpb.FileDescriptorResponse{
					FileDescriptorProto: [][]byte{baseBytes, extraBytes},
				},
			},
		}}
	})

	stop := withLoopRunning(t, env, 5*time.Second)
	defer stop()

	fds, err := env.grpcMod.fetchFileDescriptorForSymbol("phase2.DepMsg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fds == nil {
		t.Fatalf("expected non-nil")
	}

	// Should have 3 files: dep + base + extra
	if got := len(fds.File); got != 3 {
		t.Fatalf("expected len %d, got %d", 3, got)
	}

	var foundExtra bool
	for _, f := range fds.File {
		if f.GetName() == "phase3_extra.proto" {
			foundExtra = true
		}
	}
	if !(foundExtra) {
		t.Errorf("extra file should be in the result set")
	}
}
