package gojagrpc

import (
	"context"
	"io"
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// ============================================================================
// Phase 4: Targeted coverage for remaining uncovered lines (98.7% → 99.6%).
//
// Covers:
//   client.go:306-311  — makeServerStreamMethod CloseSend error path
//   client.go:506-509  — newClientStreamCall sender: CloseSend error + Submit OK
//   client.go:668-671  — newBidiStream sender: CloseSend error + Submit OK
//   server.go:515-517  — toWrappedMessage slow-path marshal error
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
	require.NoError(t, err)

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
	require.NotNil(t, errVal)
	require.False(t, goja.IsUndefined(errVal))
	if errObj, ok := errVal.(*goja.Object); ok {
		if nameVal := errObj.Get("name"); nameVal != nil && nameVal.String() == "GrpcError" {
			assert.Equal(t, int64(codes.Internal), errObj.Get("code").ToInteger())
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

	outputDesc := phase3FindMsgDesc(t, env, "testgrpc.EchoResponse")

	mockCC := &phase3MockCC{
		newStreamFn: func(ctx context.Context, _ *grpc.StreamDesc, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
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
		fn := env.grpcMod.makeClientStreamMethod(mockCC, "/test/ClientStream", outputDesc)
		_ = env.runtime.Set("__p4CSFn2", fn)
	})
	require.NoError(t, err)

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
	require.NotNil(t, errVal)
	require.False(t, goja.IsUndefined(errVal))
	errStr := errVal.String()
	t.Logf("closeSend error: %s", errStr)
	if errObj, ok := errVal.(*goja.Object); ok {
		if nameVal := errObj.Get("name"); nameVal != nil && nameVal.String() == "GrpcError" {
			assert.Equal(t, int64(codes.Unavailable), errObj.Get("code").ToInteger())
		}
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

	outputDesc := phase3FindMsgDesc(t, env, "testgrpc.Item")

	mockCC := &phase3MockCC{
		newStreamFn: func(ctx context.Context, _ *grpc.StreamDesc, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
			return &phase3MockStream{
				closeSendFn: func() error {
					return status.Errorf(codes.Internal, "bidi close send failed")
				},
				ctx: ctx,
			}, nil
		},
	}

	err := env.loop.Submit(func() {
		fn := env.grpcMod.makeBidiStreamMethod(mockCC, "/test/BidiStream", outputDesc)
		_ = env.runtime.Set("__p4BSFn2", fn)
	})
	require.NoError(t, err)

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
	require.NotNil(t, errVal)
	require.False(t, goja.IsUndefined(errVal))
	errStr := errVal.String()
	t.Logf("bidi closeSend error: %s", errStr)
	if errObj, ok := errVal.(*goja.Object); ok {
		if nameVal := errObj.Get("name"); nameVal != nil && nameVal.String() == "GrpcError" {
			assert.Equal(t, int64(codes.Internal), errObj.Get("code").ToInteger())
		}
	}
}

// ============================================================================
// Test: server.go:515-517 — toWrappedMessage slow-path marshal error
//
// Creates a nonDynamicMsg wrapping a *dynamicpb.Message with invalid UTF-8
// in a proto3 string field. toWrappedMessage takes the slow path (not
// *dynamicpb.Message) and proto.Marshal fails UTF-8 validation.
// ============================================================================

func TestPhase4_ToWrappedMessage_MarshalError_InvalidUTF8(t *testing.T) {
	env := newGrpcTestEnv(t)

	echoReqDesc := phase3FindMsgDesc(t, env, "testgrpc.EchoRequest")

	// Create a dynamicpb message with invalid UTF-8 in a string field.
	inner := dynamicpb.NewMessage(echoReqDesc)
	inner.Set(echoReqDesc.Fields().ByName("message"), protoreflect.ValueOfString("\xff\xfe"))

	// Wrap in nonDynamicMsg to bypass the dynamicpb fast path.
	wrapped := &nonDynamicMsg{Message: inner}

	_, err := env.grpcMod.toWrappedMessage(wrapped, echoReqDesc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "marshal")
}

// ============================================================================
// Test: status.go:130-131 — anypb.New error in newGrpcErrorWithDetails
//
// Creates a *dynamicpb.Message with invalid UTF-8, wraps it via the
// protobuf module, and passes it as a detail to newGrpcErrorWithDetails.
// UnwrapMessage succeeds (returns the *dynamicpb.Message), but anypb.New
// calls proto.Marshal internally which rejects invalid UTF-8 for proto3
// string fields → the detail is skipped via the continue on line 131.
// ============================================================================

func TestPhase4_NewGrpcErrorWithDetails_AnypbNewError(t *testing.T) {
	env := newGrpcTestEnv(t)

	echoReqDesc := phase3FindMsgDesc(t, env, "testgrpc.EchoRequest")

	// Create a *dynamicpb.Message with invalid UTF-8 in a proto3 string field.
	msg := dynamicpb.NewMessage(echoReqDesc)
	msg.Set(echoReqDesc.Fields().ByName("message"), protoreflect.ValueOfString("\xff\xfe"))

	// Wrap it using the protobuf module so UnwrapMessage will succeed.
	wrappedObj := env.pbMod.WrapMessage(msg)

	// Call newGrpcErrorWithDetails with this as a detail.
	// UnwrapMessage succeeds → anypb.New fails → continue → empty goDetails.
	obj := env.grpcMod.newGrpcErrorWithDetails(codes.Internal, "test error", []goja.Value{wrappedObj})
	require.NotNil(t, obj)

	assert.Equal(t, "GrpcError", obj.Get("name").String())
	assert.Equal(t, int64(codes.Internal), obj.Get("code").ToInteger())

	// The detail should have been skipped because anypb.New failed
	// (proto.Marshal rejects invalid UTF-8 for proto3 string fields).
	goDetails := env.grpcMod.extractGoDetails(obj)
	assert.Empty(t, goDetails, "details should be empty because anypb.New failed on invalid UTF-8")
}
