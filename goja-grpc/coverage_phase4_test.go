package gojagrpc

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/goja"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

func awaitPhase4ContextCanceled(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(defaultTimeout):
		t.Fatal("stream sender error did not cancel RPC context")
	}
}

func assertPhase4CleanupCalled(t *testing.T, cleanupCalls *atomic.Int32) {
	t.Helper()
	if got := cleanupCalls.Load(); got != 1 {
		t.Fatalf("sender terminal cleanup calls = %d, want 1", got)
	}
}

// ============================================================================
// Phase 4: Targeted coverage for remaining uncovered lines (98.7% → 99.6%).
//
// Covers:
//   client.go:306-311  — makeServerStreamMethod CloseSend error path
//   client.go:506-509  — newClientStreamCall sender: CloseSend error + Submit OK
//   client.go:668-671  — newBidiStream sender: CloseSend error + Submit OK
//   server.go           — exact message wrapping without marshal conversion
//   status.go:130-131  — newGrpcErrorWithDetails anypb.New error
//
// Synchronization strategy: blocking mock functions + channel signaling.
// No time.Sleep for synchronization. All tests are deterministic.
// ============================================================================

// ============================================================================
// Test: client.go:306-311 — makeServerStreamMethod CloseSend error
//
// Mock: NewStream succeeds, SendMsg succeeds, CloseSend FAILS.
// The goroutine hits the CloseSend error path and rejects the promise
// via submitOrRejectDirect.
// ============================================================================

func TestPhase4_ServerStream_CloseSendError(t *testing.T) {
	env := newGrpcTestEnv(t)

	inputDesc := phase3FindMsgDesc(t, env, "testgrpc.EchoRequest")
	outputDesc := phase3FindMsgDesc(t, env, "testgrpc.Item")

	mockCC := &phase3MockCC{
		newStreamFn: func(ctx context.Context, _ *grpc.StreamDesc, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
			return &phase3MockStream{
				sendMsgErr: nil, // SendMsg succeeds
				closeSendFn: func() error {
					return status.Errorf(codes.Internal, "close send failed")
				},
				ctx: ctx,
			}, nil
		},
	}

	// Pre-register the mock-backed server-stream function on the loop.
	err := env.loop.Submit(func() {
		fn := env.grpcMod.makeServerStreamMethod(mockCC, "/test/ServerStream", inputDesc, outputDesc)
		_ = env.runtime.Set("__p4SsCS", fn)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	env.runOnLoop(t, `
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test');
		__p4SsCS(req).then(function() {
			__p4SsCSerr = 'unexpected resolve';
			__done();
		}).catch(function(err) {
			__p4SsCSerr = err;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("__p4SsCSerr")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if goja.IsUndefined(errVal) {
		t.Fatalf("expected false")
	}
	if errObj, ok := errVal.(*goja.Object); ok {
		if nameVal := errObj.Get("name"); nameVal != nil && nameVal.String() == "GrpcError" {
			if got := errObj.Get("code").ToInteger(); got != int64(codes.Internal) {
				t.Errorf("expected %v, got %v", int64(codes.Internal), got)
			}
		}
	}
}

// ============================================================================
// Test: client.go:506-509 — newClientStreamCall sender goroutine:
// CloseSend returns error, Submit succeeds, callback hits closeErr != nil.
//
// The sender goroutine calls CloseSend which returns an error. Since the
// loop is still running, Submit succeeds and the callback executes the
// closeErr != nil branch (lines 506-509), rejecting the promise with a
// GrpcError.
// ============================================================================

func TestPhase4_ClientSender_CloseSendErrorWithSubmitSuccess(t *testing.T) {
	env := newGrpcTestEnv(t)

	inputDesc := phase3FindMsgDesc(t, env, "testgrpc.Item")
	outputDesc := phase3FindMsgDesc(t, env, "testgrpc.EchoResponse")
	ctxDone := make(chan struct{})

	mockCC := &phase3MockCC{
		newStreamFn: func(ctx context.Context, _ *grpc.StreamDesc, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
			go func() {
				<-ctx.Done()
				close(ctxDone)
			}()
			return &phase3MockStream{
				closeSendFn: func() error {
					return status.Errorf(codes.Unavailable, "close send failed")
				},
				recvMsgFn: func(any) error {
					// Block receiver goroutine to prevent early settlement
					// of the response promise interfering with closeSend.
					<-ctx.Done()
					return io.EOF
				},
				ctx: ctx,
			}, nil
		},
	}

	err := env.loop.Submit(func() {
		fn := env.grpcMod.makeClientStreamMethod(mockCC, "/test/ClientStream", inputDesc, outputDesc)
		_ = env.runtime.Set("__p4CSFn2", fn)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	env.runOnLoop(t, `
		__p4CSFn2().then(function(call) {
			call.closeSend().then(function() {
				__p4CSErr2 = 'unexpected resolve';
				__done();
			}).catch(function(err) {
				__p4CSErr2 = err;
				__done();
			});
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("__p4CSErr2")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if goja.IsUndefined(errVal) {
		t.Fatalf("expected false")
	}
	errStr := errVal.String()
	t.Logf("closeSend error: %s", errStr)
	if errObj, ok := errVal.(*goja.Object); ok {
		if nameVal := errObj.Get("name"); nameVal != nil && nameVal.String() == "GrpcError" {
			if got := errObj.Get("code").ToInteger(); got != int64(codes.Unavailable) {
				t.Errorf("expected %v, got %v", int64(codes.Unavailable), got)
			}
		}
	}
	awaitPhase4ContextCanceled(t, ctxDone)
}

func TestPhase4_ClientSender_SendMsgErrorWithSubmitSuccess(t *testing.T) {
	env := newGrpcTestEnv(t)

	inputDesc := phase3FindMsgDesc(t, env, "testgrpc.Item")
	outputDesc := phase3FindMsgDesc(t, env, "testgrpc.EchoResponse")
	ctxDone := make(chan struct{})

	mockCC := &phase3MockCC{
		newStreamFn: func(ctx context.Context, _ *grpc.StreamDesc, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
			go func() {
				<-ctx.Done()
				close(ctxDone)
			}()
			return &phase3MockStream{
				sendMsgErr: status.Errorf(codes.Unavailable, "send failed"),
				recvMsgFn: func(any) error {
					<-ctx.Done()
					return io.EOF
				},
				ctx: ctx,
			}, nil
		},
	}

	err := env.loop.Submit(func() {
		fn := env.grpcMod.makeClientStreamMethod(mockCC, "/test/ClientStream", inputDesc, outputDesc)
		_ = env.runtime.Set("__p4CSSendFn", fn)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	env.runOnLoop(t, `
		var Item = pb.messageType('testgrpc.Item');
		var req = new Item();
		req.set('name', 'send-error');
		__p4CSSendFn().then(function(call) {
			call.send(req).then(function() {
				__p4CSSendErr = 'unexpected resolve';
				__done();
			}).catch(function(err) {
				__p4CSSendErr = err;
				__done();
			});
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("__p4CSSendErr")
	if errVal == nil || goja.IsUndefined(errVal) {
		t.Fatalf("expected send error, got %v", errVal)
	}
	if errObj, ok := errVal.(*goja.Object); ok {
		if nameVal := errObj.Get("name"); nameVal != nil && nameVal.String() == "GrpcError" {
			if got := errObj.Get("code").ToInteger(); got != int64(codes.Unavailable) {
				t.Errorf("expected %v, got %v", int64(codes.Unavailable), got)
			}
		}
	}
	awaitPhase4ContextCanceled(t, ctxDone)
}

func TestPhase4_ClientSenderErrorsCleanupSignalWithSubmitSuccess(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
		stream func(context.Context) *phase3MockStream
	}{
		{
			name: "send",
			script: `
				var Item = pb.messageType('testgrpc.Item');
				var item = new Item();
				item.set('id', '1');
				item.set('name', 'send-error');
				__p4CSCleanupCall.send(item).then(function() {
					__p4CSCleanupErr = 'unexpected resolve';
					__done();
				}).catch(function(err) {
					__p4CSCleanupErr = err;
					__done();
				});
			`,
			stream: func(ctx context.Context) *phase3MockStream {
				return &phase3MockStream{sendMsgErr: status.Errorf(codes.Unavailable, "send failed"), ctx: ctx}
			},
		},
		{
			name: "closeSend",
			script: `
				__p4CSCleanupCall.closeSend().then(function() {
					__p4CSCleanupErr = 'unexpected resolve';
					__done();
				}).catch(function(err) {
					__p4CSCleanupErr = err;
					__done();
				});
			`,
			stream: func(ctx context.Context) *phase3MockStream {
				return &phase3MockStream{closeSendFn: func() error { return status.Errorf(codes.Unavailable, "close send failed") }, ctx: ctx}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newGrpcTestEnv(t)
			inputDesc := phase3FindMsgDesc(t, env, "testgrpc.Item")
			outputDesc := phase3FindMsgDesc(t, env, "testgrpc.EchoResponse")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var cleanupOnce sync.Once
			var cleanupCalls atomic.Int32
			releaseRecv := make(chan struct{})

			stream := tc.stream(ctx)
			stream.recvMsgFn = func(any) error {
				<-releaseRecv
				return io.EOF
			}
			err := env.loop.Submit(func() {
				cleanup := func() { cleanupOnce.Do(func() { cleanupCalls.Add(1) }) }
				options := &callOpts{
					module:        env.grpcMod,
					ctx:           ctx,
					cancel:        cancel,
					signalCleanup: cleanup,
				}
				if registerErr := options.register(); registerErr != nil {
					t.Errorf("register operation: %v", registerErr)
					return
				}
				_, projection, stateErr := newClientStreamHarness(
					env.grpcMod,
					stream,
					inputDesc,
					outputDesc,
					options,
				)
				if stateErr != nil {
					t.Errorf("new client stream state: %v", stateErr)
					return
				}
				callObj := env.grpcMod.newClientStreamCall(projection)
				_ = env.runtime.Set("__p4CSCleanupCall", callObj)
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			env.runOnLoop(t, tc.script, defaultTimeout)
			close(releaseRecv)

			errVal := env.runtime.Get("__p4CSCleanupErr")
			if errVal == nil || goja.IsUndefined(errVal) {
				t.Fatalf("expected sender error, got %v", errVal)
			}
			assertPhase4CleanupCalled(t, &cleanupCalls)
			if ctx.Err() == nil {
				t.Fatal("sender terminal error did not cancel context")
			}
		})
	}
}

// ============================================================================
// Test: client.go:668-671 — newBidiStream sender goroutine:
// CloseSend returns error, Submit succeeds, callback hits closeErr != nil.
//
// Same pattern as client-stream but for bidirectional streaming.
// ============================================================================

func TestPhase4_BidiSender_CloseSendErrorWithSubmitSuccess(t *testing.T) {
	env := newGrpcTestEnv(t)

	inputDesc := phase3FindMsgDesc(t, env, "testgrpc.Item")
	outputDesc := phase3FindMsgDesc(t, env, "testgrpc.Item")
	ctxDone := make(chan struct{})

	mockCC := &phase3MockCC{
		newStreamFn: func(ctx context.Context, _ *grpc.StreamDesc, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
			go func() {
				<-ctx.Done()
				close(ctxDone)
			}()
			return &phase3MockStream{
				closeSendFn: func() error {
					return status.Errorf(codes.Internal, "bidi close send failed")
				},
				ctx: ctx,
			}, nil
		},
	}

	err := env.loop.Submit(func() {
		fn := env.grpcMod.makeBidiStreamMethod(mockCC, "/test/BidiStream", inputDesc, outputDesc)
		_ = env.runtime.Set("__p4BSFn2", fn)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	env.runOnLoop(t, `
		__p4BSFn2().then(function(stream) {
			stream.closeSend().then(function() {
				__p4BSErr2 = 'unexpected resolve';
				__done();
			}).catch(function(err) {
				__p4BSErr2 = err;
				__done();
			});
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("__p4BSErr2")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if goja.IsUndefined(errVal) {
		t.Fatalf("expected false")
	}
	errStr := errVal.String()
	t.Logf("bidi closeSend error: %s", errStr)
	if errObj, ok := errVal.(*goja.Object); ok {
		if nameVal := errObj.Get("name"); nameVal != nil && nameVal.String() == "GrpcError" {
			if got := errObj.Get("code").ToInteger(); got != int64(codes.Internal) {
				t.Errorf("expected %v, got %v", int64(codes.Internal), got)
			}
		}
	}
	awaitPhase4ContextCanceled(t, ctxDone)
}

func TestPhase4_BidiSender_SendMsgErrorWithSubmitSuccess(t *testing.T) {
	env := newGrpcTestEnv(t)

	inputDesc := phase3FindMsgDesc(t, env, "testgrpc.Item")
	outputDesc := phase3FindMsgDesc(t, env, "testgrpc.Item")
	ctxDone := make(chan struct{})

	mockCC := &phase3MockCC{
		newStreamFn: func(ctx context.Context, _ *grpc.StreamDesc, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
			go func() {
				<-ctx.Done()
				close(ctxDone)
			}()
			return &phase3MockStream{
				sendMsgErr: status.Errorf(codes.Internal, "bidi send failed"),
				ctx:        ctx,
			}, nil
		},
	}

	err := env.loop.Submit(func() {
		fn := env.grpcMod.makeBidiStreamMethod(mockCC, "/test/BidiStream", inputDesc, outputDesc)
		_ = env.runtime.Set("__p4BSSendFn", fn)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	env.runOnLoop(t, `
		var Item = pb.messageType('testgrpc.Item');
		var req = new Item();
		req.set('name', 'bidi-send-error');
		__p4BSSendFn().then(function(stream) {
			stream.send(req).then(function() {
				__p4BSSendErr = 'unexpected resolve';
				__done();
			}).catch(function(err) {
				__p4BSSendErr = err;
				__done();
			});
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("__p4BSSendErr")
	if errVal == nil || goja.IsUndefined(errVal) {
		t.Fatalf("expected send error, got %v", errVal)
	}
	if errObj, ok := errVal.(*goja.Object); ok {
		if nameVal := errObj.Get("name"); nameVal != nil && nameVal.String() == "GrpcError" {
			if got := errObj.Get("code").ToInteger(); got != int64(codes.Internal) {
				t.Errorf("expected %v, got %v", int64(codes.Internal), got)
			}
		}
	}
	awaitPhase4ContextCanceled(t, ctxDone)
}

func TestPhase4_BidiSenderErrorsCleanupSignalWithSubmitSuccess(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
		stream *phase3MockStream
	}{
		{
			name: "send",
			script: `
				var Item = pb.messageType('testgrpc.Item');
				var item = new Item();
				item.set('id', '1');
				item.set('name', 'bidi-send-error');
				__p4BSCleanupStream.send(item).then(function() {
					__p4BSCleanupErr = 'unexpected resolve';
					__done();
				}).catch(function(err) {
					__p4BSCleanupErr = err;
					__done();
				});
			`,
			stream: &phase3MockStream{sendMsgErr: status.Errorf(codes.Internal, "bidi send failed")},
		},
		{
			name: "closeSend",
			script: `
				__p4BSCleanupStream.closeSend().then(function() {
					__p4BSCleanupErr = 'unexpected resolve';
					__done();
				}).catch(function(err) {
					__p4BSCleanupErr = err;
					__done();
				});
			`,
			stream: &phase3MockStream{closeSendFn: func() error { return status.Errorf(codes.Internal, "bidi close send failed") }},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newGrpcTestEnv(t)
			inputDesc := phase3FindMsgDesc(t, env, "testgrpc.Item")
			outputDesc := phase3FindMsgDesc(t, env, "testgrpc.Item")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var cleanupOnce sync.Once
			var cleanupCalls atomic.Int32

			stream := tc.stream
			stream.ctx = ctx
			err := env.loop.Submit(func() {
				cleanup := func() { cleanupOnce.Do(func() { cleanupCalls.Add(1) }) }
				options := &callOpts{
					module:        env.grpcMod,
					ctx:           ctx,
					cancel:        cancel,
					signalCleanup: cleanup,
				}
				if registerErr := options.register(); registerErr != nil {
					t.Errorf("register operation: %v", registerErr)
					return
				}
				_, projection, stateErr := newClientStreamHarness(
					env.grpcMod,
					stream,
					inputDesc,
					outputDesc,
					options,
				)
				if stateErr != nil {
					t.Errorf("new client stream state: %v", stateErr)
					return
				}
				streamObj := env.grpcMod.newBidiStreamObject(projection)
				_ = env.runtime.Set("__p4BSCleanupStream", streamObj)
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			env.runOnLoop(t, tc.script, defaultTimeout)

			errVal := env.runtime.Get("__p4BSCleanupErr")
			if errVal == nil || goja.IsUndefined(errVal) {
				t.Fatalf("expected sender error, got %v", errVal)
			}
			assertPhase4CleanupCalled(t, &cleanupCalls)
			if ctx.Err() == nil {
				t.Fatal("sender terminal error did not cancel context")
			}
		})
	}
}

// ============================================================================
// Exact message identity does not introduce a marshal/unmarshal conversion.
// Even an invalid string value remains unchanged at this ownership boundary.
// ============================================================================

func TestPhase4_ToWrappedMessage_PreservesExactMessageWithoutMarshal(t *testing.T) {
	env := newGrpcTestEnv(t)

	echoReqDesc := phase3FindMsgDesc(t, env, "testgrpc.EchoRequest")

	// Create a dynamicpb message with invalid UTF-8 in a string field.
	inner := dynamicpb.NewMessage(echoReqDesc)
	inner.Set(echoReqDesc.Fields().ByName("message"), protoreflect.ValueOfString("\xff\xfe"))

	// Wrap in nonDynamicMsg to bypass the dynamicpb fast path.
	wrapped := &nonDynamicMsg{Message: inner}

	object, err := env.grpcMod.toWrappedMessage(wrapped, echoReqDesc)
	if err != nil {
		t.Fatalf("toWrappedMessage: %v", err)
	}
	message, err := env.pbMod.UnwrapMessage(object)
	if err != nil {
		t.Fatalf("UnwrapMessage: %v", err)
	}
	if got := message.ProtoReflect().Get(echoReqDesc.Fields().ByName("message")).String(); got != "\xff\xfe" {
		t.Fatalf("wrapped string = %q, want exact invalid bytes", got)
	}
}

// ============================================================================
// Invalid status messages fail atomically instead of being silently omitted.
// ============================================================================

func TestPhase4_NewGrpcErrorWithDetails_AnypbNewError(t *testing.T) {
	env := newGrpcTestEnv(t)

	echoReqDesc := phase3FindMsgDesc(t, env, "testgrpc.EchoRequest")

	// Create a *dynamicpb.Message with invalid UTF-8 in a proto3 string field.
	msg := dynamicpb.NewMessage(echoReqDesc)
	msg.Set(echoReqDesc.Fields().ByName("message"), protoreflect.ValueOfString("\xff\xfe"))

	// Wrap it using the protobuf module so UnwrapMessage will succeed.
	wrappedObj, err := env.pbMod.WrapMessage(msg)
	if err != nil {
		t.Fatalf("WrapMessage: %v", err)
	}

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		env.grpcMod.newGrpcErrorWithDetails(codes.Internal, "test error", []goja.Value{wrappedObj})
	}()
	if recovered == nil || !strings.Contains(fmt.Sprint(recovered), "status detail") {
		t.Fatalf("invalid detail panic = %v, want status-detail failure", recovered)
	}
}
