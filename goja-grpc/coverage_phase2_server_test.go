package gojagrpc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/goja"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/anypb"
)

// ============================================================================
// A public property on an unrelated object cannot forge private status detail
// identity.
// ============================================================================

func TestExtractGoDetailsRejectsUnregisteredObject(t *testing.T) {
	env := newGrpcTestEnv(t)

	obj := env.runtime.NewObject()
	if err := obj.Set("_goDetails", env.runtime.ToValue("not a holder")); err != nil {
		t.Fatal(err)
	}

	result := env.grpcMod.extractGoDetails(obj)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

// ============================================================================
// Test: toWrappedMessage — not a proto.Message
//
// Covers: server.go line 510-512 (not a proto.Message)
// ============================================================================

func TestToWrappedMessage_NotProtoMessage(t *testing.T) {
	env := newGrpcTestEnv(t)

	desc, err := env.pbMod.FindDescriptor(protoreflect.FullName("testgrpc.EchoRequest"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msgDesc, ok := desc.(protoreflect.MessageDescriptor)
	if !(ok) {
		t.Fatalf("expected true")
	}

	_, wrapErr := env.grpcMod.toWrappedMessage("not a proto message", msgDesc)
	if wrapErr == nil {
		t.Fatalf("expected an error")
	}
	if !strings.Contains(wrapErr.Error(), "not a proto.Message") {
		t.Errorf("expected %q to contain %q", wrapErr.Error(), "not a proto.Message")
	}
}

// ============================================================================
// Same-wire protobuf messages with foreign descriptor identity are rejected.
// ============================================================================

func TestToWrappedMessageRejectsForeignSameWireDescriptor(t *testing.T) {
	env := newGrpcTestEnv(t)

	// anypb.Any is a generated (non-dynamicpb) proto.Message.
	// We need a descriptor for Any. Load a descriptor that has google.protobuf.Any.
	anyFDP := &descriptorpb.FileDescriptorProto{
		Name:    new("phase2_any.proto"),
		Package: new("phase2any"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: new("SimpleMsg"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("type_url"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName: new("typeUrl"),
					},
					{
						Name:     new("value"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName: new("value"),
					},
				},
			},
		},
	}
	fds := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{anyFDP}}
	data, err := proto.Marshal(fds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = env.pbMod.LoadDescriptorSetBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	desc, err := env.pbMod.FindDescriptor(protoreflect.FullName("phase2any.SimpleMsg"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msgDesc, ok := desc.(protoreflect.MessageDescriptor)
	if !(ok) {
		t.Fatalf("expected true")
	}

	// Create a generated proto message (anypb.Any) that has the same
	// wire format as SimpleMsg (type_url=field1, value=field2).
	anyMsg := &anypb.Any{
		TypeUrl: "test-url",
		Value:   []byte("test-value"),
	}

	if _, wrapErr := env.grpcMod.toWrappedMessage(anyMsg, msgDesc); wrapErr == nil ||
		!strings.Contains(wrapErr.Error(), "non-canonical descriptor") {
		t.Fatalf("same-wire foreign message error = %v, want identity rejection", wrapErr)
	}
}

// ============================================================================
// Test: Server handler receives no message (unary) — Recv error
//
// Uses Go-level channel access to send an empty unary RPC, causing the
// JS unary handler's Recv callback to fire with io.EOF.
//
// Covers: server.go lines 204-207 (makeUnaryHandler err != nil path)
// ============================================================================

func TestUnaryHandler_RecvError_NoMessage(t *testing.T) {
	env := newGrpcTestEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	setupDone := make(chan struct{}, 1)
	_ = env.runtime.Set("__ready", env.runtime.ToValue(func(_ goja.FunctionCall) goja.Value {
		select {
		case setupDone <- struct{}{}:
		default:
		}
		return goja.Undefined()
	}))

	runDone := make(chan error, 1)
	_ = env.loop.Submit(func() {
		_, err := env.runtime.RunString(`
			var server = grpc.createServer();
			server.addService('testgrpc.TestService', {
				echo: function(request, call) {
					var EchoResponse = pb.messageType('testgrpc.EchoResponse');
					var resp = new EchoResponse();
					resp.set('message', 'ok');
					return resp;
				},
				serverStream: function(request, call) {},
				clientStream: function(call) { return null; },
				bidiStream: function(call) {}
			});
			server.start();
			__ready();
		`)
		runDone <- err
	})

	go env.loop.Run(ctx)

	select {
	case <-setupDone:
	case <-ctx.Done():
		t.Fatal("timeout waiting for server setup")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for RunString")
	}

	// Use NewStream (streaming API) to send zero messages for a unary method.
	cs, err := env.channel.NewStream(ctx, &grpc.StreamDesc{}, "/testgrpc.TestService/Echo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Close send without sending any message — server handler gets io.EOF.
	err = cs.CloseSend()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Try to receive — should get an error because the handler finished with EOF.
	desc, findErr := env.pbMod.FindDescriptor(protoreflect.FullName("testgrpc.EchoResponse"))
	if findErr != nil {
		t.Fatalf("unexpected error: %v", findErr)
	}
	respMsg := dynamicpb.NewMessage(desc.(protoreflect.MessageDescriptor))
	err = cs.RecvMsg(respMsg)
	if err == nil {
		t.Fatalf("expected an error")
	}
}

// ============================================================================
// Test: Server-streaming handler receives no message — Recv error
//
// Covers: server.go lines 252-255 (makeServerStreamHandler err != nil path)
// ============================================================================

func TestServerStreamHandler_RecvError_NoMessage(t *testing.T) {
	env := newGrpcTestEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	setupDone := make(chan struct{}, 1)
	_ = env.runtime.Set("__ready", env.runtime.ToValue(func(_ goja.FunctionCall) goja.Value {
		select {
		case setupDone <- struct{}{}:
		default:
		}
		return goja.Undefined()
	}))

	runDone := make(chan error, 1)
	_ = env.loop.Submit(func() {
		_, err := env.runtime.RunString(`
			var server = grpc.createServer();
			server.addService('testgrpc.TestService', {
				echo: function(request, call) { return null; },
				serverStream: function(request, call) {
					var Item = pb.messageType('testgrpc.Item');
					var item = new Item();
					item.set('id', '1');
					item.set('name', 'test');
					call.send(item);
				},
				clientStream: function(call) { return null; },
				bidiStream: function(call) {}
			});
			server.start();
			__ready();
		`)
		runDone <- err
	})

	go env.loop.Run(ctx)

	select {
	case <-setupDone:
	case <-ctx.Done():
		t.Fatal("timeout waiting for server setup")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for RunString")
	}

	// Use NewStream for the server-streaming method but close without sending.
	cs, err := env.channel.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true}, "/testgrpc.TestService/ServerStream")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = cs.CloseSend()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	desc, findErr := env.pbMod.FindDescriptor(protoreflect.FullName("testgrpc.Item"))
	if findErr != nil {
		t.Fatalf("unexpected error: %v", findErr)
	}
	respMsg := dynamicpb.NewMessage(desc.(protoreflect.MessageDescriptor))
	err = cs.RecvMsg(respMsg)
	if err == nil {
		t.Fatalf("expected an error")
	}
}

// ============================================================================
// A generated message with foreign descriptor identity is rejected before a
// JavaScript handler observes it.
// ============================================================================

func TestUnaryHandler_ToWrappedMessageError(t *testing.T) {
	env := newGrpcTestEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	setupDone := make(chan struct{}, 1)
	_ = env.runtime.Set("__ready", env.runtime.ToValue(func(_ goja.FunctionCall) goja.Value {
		select {
		case setupDone <- struct{}{}:
		default:
		}
		return goja.Undefined()
	}))

	runDone := make(chan error, 1)
	_ = env.loop.Submit(func() {
		_, err := env.runtime.RunString(`
			var server = grpc.createServer();
			server.addService('testgrpc.TestService', {
				echo: function(request, call) {
					var EchoResponse = pb.messageType('testgrpc.EchoResponse');
					var resp = new EchoResponse();
					resp.set('message', 'ok');
					return resp;
				},
				serverStream: function(request, call) {},
				clientStream: function(call) { return null; },
				bidiStream: function(call) {}
			});
			server.start();
			__ready();
		`)
		runDone <- err
	})

	go env.loop.Run(ctx)

	select {
	case <-setupDone:
	case <-ctx.Done():
		t.Fatal("timeout waiting for server setup")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for RunString")
	}

	desc, findErr := env.pbMod.FindDescriptor(protoreflect.FullName("testgrpc.EchoResponse"))
	if findErr != nil {
		t.Fatalf("unexpected error: %v", findErr)
	}
	respMsg := dynamicpb.NewMessage(desc.(protoreflect.MessageDescriptor))

	anyMsg := &anypb.Any{TypeUrl: "test", Value: []byte("hello")}
	err := env.channel.Invoke(ctx, "/testgrpc.TestService/Echo", anyMsg, respMsg)
	if status.Code(err) != codes.Internal ||
		!strings.Contains(err.Error(), "non-canonical descriptor") {
		t.Fatalf("foreign request error = %v, want Internal identity rejection", err)
	}
}

// ============================================================================
// Test: Server addServerSend error — send after stream finished
//
// Covers: server.go line 459-460 (Send error in addServerSend)
// ============================================================================

func TestServerSend_StreamError(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Use an async handler that yields between sends via setTimeout.
	// After the client aborts, the context cancellation closes the
	// Responses channel, causing subsequent sends on the server side
	// to fail with an error (caught by the promise catch).
	//
	// Deterministic by construction: the server sends unboundedly (there is
	// no fixed count to race against the abort) and the client aborts only
	// after its first successful recv, which proves the server's first send
	// completed — so sendCount >= 1 is guaranteed before the abort, and a
	// later send is guaranteed to fail once the abort has landed.
	//
	// Covers: server.go line 459-460 (Send error in addServerSend)
	env.runOnLoop(t, `
		var sendCount = 0;
		var sendError = null;
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {
				var Item = pb.messageType('testgrpc.Item');
				return new Promise(function(resolve) {
					function sendOne(i) {
						var item = new Item();
						item.set('id', String(i));
						item.set('name', 'item');
						call.send(item).then(function() {
							sendCount = i + 1;
							// Yield to event loop between sends — allows abort cleanup.
							setTimeout(function() { sendOne(i + 1); }, 0);
						}).catch(function(e) {
							// Expected once the client aborts: the response
							// channel is closed, so this send fails. The
							// rejection must be handled or the adapter
							// treats it as a fatal unhandled rejection.
							sendError = e;
							resolve();
						});
					}
					sendOne(0);
				});
			},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test');

		var ctrl = new AbortController();
		client.serverStream(req, { signal: ctrl.signal }).then(function(stream) {
			var seen = false;
			(function recvLoop() {
				stream.recv().then(function(result) {
					if (result.done) { __done(); return; }
					if (!seen) {
						seen = true;
						// The first item was received, so the server's first
						// send completed (sendCount >= 1). Abort now: the
						// server keeps sending, so its next send
						// deterministically fails.
						ctrl.abort();
					}
					recvLoop();
				}).catch(function(err) {
					// Expected: abort error. Give handler time to hit send error.
					setTimeout(function() { __done(); }, 200);
				});
			})();
		}).catch(function(err) {
			setTimeout(function() { __done(); }, 200);
		});
	`, defaultTimeout)

	sendError := env.runtime.Get("sendError")
	if sendError == nil || isGojaUndefined(sendError) {
		t.Fatal("expected server-side send to fail after client abort")
	}
	sendCount := env.runtime.Get("sendCount")
	if sendCount == nil || sendCount.ToInteger() == 0 {
		t.Fatal("expected at least one successful send before the abort")
	}
}

// ============================================================================
// Test: finishUnaryResponse — Send error
//
// The client-streaming handler returns a response, but the stream's
// Send fails (e.g., because the client has disconnected).
//
// Covers: server.go line 598-601 (Send error in finishUnaryResponse)
// ============================================================================

func TestFinishUnaryResponse_SendError(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) {
				// Recv all messages. When done, DELAY before returning
				// the response. This delay allows the client's abort
				// to propagate and close the Responses channel before
				// finishUnaryResponse tries to Send.
				return (function loop() {
					return call.recv().then(function(result) {
						if (result.done) {
							return new Promise(function(resolve) {
								setTimeout(function() {
									var EchoResponse = pb.messageType('testgrpc.EchoResponse');
									var doneResp = new EchoResponse();
									doneResp.set('message', 'done');
									doneResp.set('code', 42);
									resolve(doneResp);
								}, 100);
							});
						}
						return loop();
					});
				})();
			},
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var ctrl = new AbortController();
		var Item = pb.messageType('testgrpc.Item');

		client.clientStream({ signal: ctrl.signal }).then(function(call) {
			// Send one message then close.
			var msg = new Item(); msg.set('id', '1'); msg.set('name', 'test');
			return call.send(msg).then(function() {
				// Abort, then close send. The handler will get done=true
				// and delay 100ms. During that delay, the abort's context
				// cancellation closes the Responses channel. When the
				// handler finally resolves, finishUnaryResponse's Send fails.
				ctrl.abort();
				return call.closeSend();
			});
		}).then(function() {
			// Give the handler time to finish its delayed response.
			setTimeout(function() { __done(); }, 300);
		}).catch(function(err) {
			setTimeout(function() { __done(); }, 300);
		});
	`, defaultTimeout)
}

// ============================================================================
// Test: ServerStream SendMsg error via abort before call
//
// Pre-aborts the signal before making a server-streaming call.
// This triggers SendMsg/CloseSend/NewStream errors in the goroutine.
//
// Covers: client.go lines 306-311 (SendMsg error), or 299-304 (streamErr)
// ============================================================================

func TestServerStream_AbortBeforeCall(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test');

		var error;
		var ctrl = new AbortController();
		ctrl.abort(); // Abort BEFORE call

		client.serverStream(req, { signal: ctrl.signal }).then(function(stream) {
			error = 'should not resolve';
			__done();
		}).catch(function(err) {
			error = err;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	// The error should be a GrpcError (cancelled) or similar
	if errObj, ok := errVal.(*goja.Object); ok {
		name := objGetString(errObj, "name")
		if name == "GrpcError" {
			code := errObj.Get("code").ToInteger()
			if got := code; got != int64(codes.Canceled) {
				t.Errorf("expected %v, got %v", int64(codes.Canceled), got)
			}
		}
	}
}

// ============================================================================
// Test: Server-streaming handler toWrappedMessage error
//
// Covers: server.go lines 258-261 (toWrappedMessage error in server stream handler)
// ============================================================================

func TestServerStreamHandler_ToWrappedMessageError(t *testing.T) {
	env := newGrpcTestEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	setupDone := make(chan struct{}, 1)
	_ = env.runtime.Set("__ready", env.runtime.ToValue(func(_ goja.FunctionCall) goja.Value {
		select {
		case setupDone <- struct{}{}:
		default:
		}
		return goja.Undefined()
	}))

	_ = env.loop.Submit(func() {
		env.runtime.RunString(`
			var server = grpc.createServer();
			server.addService('testgrpc.TestService', {
				echo: function(request, call) { return null; },
				serverStream: function(request, call) {
					var Item = pb.messageType('testgrpc.Item');
					var ssItem = new Item();
					ssItem.set('id', '1');
					ssItem.set('name', 'test');
					call.send(ssItem);
				},
				clientStream: function(call) { return null; },
				bidiStream: function(call) {}
			});
			server.start();
			__ready();
		`)
	})

	go env.loop.Run(ctx)

	select {
	case <-setupDone:
	case <-ctx.Done():
		t.Fatal("timeout")
	}

	// Create a server-streaming call via Go and send a generated message whose
	// descriptor identity does not match the method input.
	cs, err := env.channel.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true}, "/testgrpc.TestService/ServerStream")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	anyMsg := &anypb.Any{TypeUrl: "test-type", Value: []byte("test-data")}
	err = cs.SendMsg(anyMsg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if closeErr := cs.CloseSend(); closeErr != nil {
		if status.Code(closeErr) != codes.Internal ||
			!strings.Contains(closeErr.Error(), "non-canonical descriptor") {
			t.Fatalf("CloseSend error = %v, want Internal identity rejection", closeErr)
		}
		return
	}

	itemDesc, _ := env.pbMod.FindDescriptor(protoreflect.FullName("testgrpc.Item"))
	respMsg := dynamicpb.NewMessage(itemDesc.(protoreflect.MessageDescriptor))
	err = cs.RecvMsg(respMsg)
	if status.Code(err) != codes.Internal ||
		!strings.Contains(err.Error(), "non-canonical descriptor") {
		t.Fatalf("RecvMsg error = %v, want Internal identity rejection", err)
	}
}
