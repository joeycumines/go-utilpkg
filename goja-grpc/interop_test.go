package gojagrpc

import (
	"context"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// ============================================================================
// T081: Go client -> JS server
// ============================================================================

// makeEchoRequest creates a dynamicpb EchoRequest with the given message.
func makeEchoRequest(t *testing.T, env *grpcTestEnv, message string) *dynamicpb.Message {
	t.Helper()
	desc, err := env.pbMod.FindDescriptor("testgrpc.EchoRequest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msgDesc := desc.(protoreflect.MessageDescriptor)
	msg := dynamicpb.NewMessage(msgDesc)
	msg.Set(msgDesc.Fields().ByName("message"), protoreflect.ValueOfString(message))
	return msg
}

// makeEchoResponse creates an empty dynamicpb EchoResponse.
func makeEchoResponse(t *testing.T, env *grpcTestEnv) *dynamicpb.Message {
	t.Helper()
	desc, err := env.pbMod.FindDescriptor("testgrpc.EchoResponse")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return dynamicpb.NewMessage(desc.(protoreflect.MessageDescriptor))
}

// makeItem creates a dynamicpb Item with the given id and name.
func makeItem(t *testing.T, env *grpcTestEnv, id string, name string) *dynamicpb.Message {
	t.Helper()
	desc, err := env.pbMod.FindDescriptor("testgrpc.Item")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msgDesc := desc.(protoreflect.MessageDescriptor)
	msg := dynamicpb.NewMessage(msgDesc)
	msg.Set(msgDesc.Fields().ByName("id"), protoreflect.ValueOfString(id))
	msg.Set(msgDesc.Fields().ByName("name"), protoreflect.ValueOfString(name))
	return msg
}

// makeEmptyItem creates an empty dynamicpb Item.
func makeEmptyItem(t *testing.T, env *grpcTestEnv) *dynamicpb.Message {
	t.Helper()
	desc, err := env.pbMod.FindDescriptor("testgrpc.Item")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return dynamicpb.NewMessage(desc.(protoreflect.MessageDescriptor))
}

// startJSServer starts the JS server on the event loop and returns
// a cancel function to stop the loop.
func startJSServer(t *testing.T, env *grpcTestEnv, jsCode string) (cancel context.CancelFunc, wait func()) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	serverReady := make(chan struct{}, 1)
	loopDone := make(chan error, 1)

	if err := env.loop.Submit(func() {
		_, jsErr := env.runtime.RunString(jsCode)
		if jsErr != nil {
			t.Errorf("JS server setup error: %v", jsErr)
		}
		close(serverReady)
	}); err != nil {
		t.Fatalf("submit server setup: %v", err)
	}

	go func() {
		loopDone <- env.loop.Run(ctx)
	}()

	<-serverReady

	wait = func() {
		cancel()
		<-loopDone
	}
	return cancel, wait
}

// --- Unary: Go client -> JS server ---

func TestGoClientJSServer_Unary(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	_, wait := startJSServer(t, env, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'go-called: ' + request.get('message'));
				resp.set('code', 77);
				return resp;
			},
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();
	`)
	defer wait()

	req := makeEchoRequest(t, env, "from-go")
	resp := makeEchoResponse(t, env)

	err := env.channel.Invoke(context.Background(), "/testgrpc.TestService/Echo", req, resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgField := resp.Descriptor().Fields().ByName("message")
	codeField := resp.Descriptor().Fields().ByName("code")
	if got := resp.Get(msgField).String(); got != "go-called: from-go" {
		t.Errorf("expected %v, got %v", "go-called: from-go", got)
	}
	if got := int32(resp.Get(codeField).Int()); got != int32(77) {
		t.Errorf("expected %v, got %v", int32(77), got)
	}
}

// --- Unary: Go client -> JS server error ---

func TestGoClientJSServer_UnaryError(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	_, wait := startJSServer(t, env, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				throw grpc.status.createError(grpc.status.PERMISSION_DENIED, 'access denied');
			},
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();
	`)
	defer wait()

	req := makeEchoRequest(t, env, "denied")
	resp := makeEchoResponse(t, env)

	err := env.channel.Invoke(context.Background(), "/testgrpc.TestService/Echo", req, resp)
	if err == nil {
		t.Fatalf("expected an error")
	}

	st, ok := status.FromError(err)
	if !(ok) {
		t.Fatalf("expected true")
	}
	if got := st.Code(); got != codes.PermissionDenied {
		t.Errorf("expected %v, got %v", codes.PermissionDenied, got)
	}
	if !strings.Contains(st.Message(), "access denied") {
		t.Errorf("expected %q to contain %q", st.Message(), "access denied")
	}
}

// --- Server-streaming: Go client -> JS server ---

func TestGoClientJSServer_ServerStream(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	_, wait := startJSServer(t, env, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {
				var Item = pb.messageType('testgrpc.Item');
				for (var i = 0; i < 3; i++) {
					var item = new Item();
					item.set('id', String(i));
					item.set('name', 'go-stream-' + i);
					call.send(item);
				}
			},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();
	`)
	defer wait()

	ctx := context.Background()
	desc := &grpc.StreamDesc{ServerStreams: true}
	cs, err := env.channel.NewStream(ctx, desc, "/testgrpc.TestService/ServerStream")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	req := makeEchoRequest(t, env, "list")
	if cs.SendMsg(req) != nil {
		t.Fatalf("unexpected error: %v", cs.SendMsg(req))
	}
	if cs.CloseSend() != nil {
		t.Fatalf("unexpected error: %v", cs.CloseSend())
	}

	var items []string
	for {
		item := makeEmptyItem(t, env)
		if recvErr := cs.RecvMsg(item); recvErr != nil {
			if recvErr == io.EOF {
				break
			}
			t.Fatalf("recv error: %v", recvErr)
		}
		nameField := item.Descriptor().Fields().ByName("name")
		items = append(items, item.Get(nameField).String())
	}

	if !reflect.DeepEqual(items, []string{"go-stream-0", "go-stream-1", "go-stream-2"}) {
		t.Errorf("expected %v, got %v", []string{"go-stream-0", "go-stream-1", "go-stream-2"}, items)
	}
}

// --- Client-streaming: Go client -> JS server ---

func TestGoClientJSServer_ClientStream(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	_, wait := startJSServer(t, env, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) {
				var names = [];
				return new Promise(function(resolve, reject) {
					function readLoop() {
						call.recv().then(function(result) {
							if (result.done) {
								var EchoResponse = pb.messageType('testgrpc.EchoResponse');
								var resp = new EchoResponse();
								resp.set('message', 'got: ' + names.join(','));
								resp.set('code', names.length);
								resolve(resp);
								return;
							}
							names.push(result.value.get('name'));
							readLoop();
						}).catch(reject);
					}
					readLoop();
				});
			},
			bidiStream: function(call) {}
		});
		server.start();
	`)
	defer wait()

	ctx := context.Background()
	desc := &grpc.StreamDesc{ClientStreams: true}
	cs, err := env.channel.NewStream(ctx, desc, "/testgrpc.TestService/ClientStream")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cs.SendMsg(makeItem(t, env, "1", "alpha")) != nil {
		t.Fatalf("unexpected error: %v", cs.SendMsg(makeItem(t, env, "1", "alpha")))
	}
	if cs.SendMsg(makeItem(t, env, "2", "beta")) != nil {
		t.Fatalf("unexpected error: %v", cs.SendMsg(makeItem(t, env, "2", "beta")))
	}
	if cs.CloseSend() != nil {
		t.Fatalf("unexpected error: %v", cs.CloseSend())
	}

	resp := makeEchoResponse(t, env)
	if cs.RecvMsg(resp) != nil {
		t.Fatalf("unexpected error: %v", cs.RecvMsg(resp))
	}

	msgField := resp.Descriptor().Fields().ByName("message")
	codeField := resp.Descriptor().Fields().ByName("code")
	if got := resp.Get(msgField).String(); got != "got: alpha,beta" {
		t.Errorf("expected %v, got %v", "got: alpha,beta", got)
	}
	if got := int32(resp.Get(codeField).Int()); got != int32(2) {
		t.Errorf("expected %v, got %v", int32(2), got)
	}
}

// --- Bidi-streaming: Go client -> JS server ---

func TestGoClientJSServer_BidiStream(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	_, wait := startJSServer(t, env, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {
				return new Promise(function(resolve, reject) {
					function readLoop() {
						call.recv().then(function(result) {
							if (result.done) {
								resolve();
								return;
							}
							var Item = pb.messageType('testgrpc.Item');
							var echo = new Item();
							echo.set('id', result.value.get('id'));
							echo.set('name', 'go-echo-' + result.value.get('name'));
							call.send(echo);
							readLoop();
						}).catch(reject);
					}
					readLoop();
				});
			}
		});
		server.start();
	`)
	defer wait()

	ctx := context.Background()
	desc := &grpc.StreamDesc{ClientStreams: true, ServerStreams: true}
	cs, err := env.channel.NewStream(ctx, desc, "/testgrpc.TestService/BidiStream")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cs.SendMsg(makeItem(t, env, "1", "x")) != nil {
		t.Fatalf("unexpected error: %v", cs.SendMsg(makeItem(t, env, "1", "x")))
	}
	if cs.SendMsg(makeItem(t, env, "2", "y")) != nil {
		t.Fatalf("unexpected error: %v", cs.SendMsg(makeItem(t, env, "2", "y")))
	}
	if cs.CloseSend() != nil {
		t.Fatalf("unexpected error: %v", cs.CloseSend())
	}

	var names []string
	for {
		item := makeEmptyItem(t, env)
		if recvErr := cs.RecvMsg(item); recvErr != nil {
			if recvErr == io.EOF {
				break
			}
			t.Fatalf("recv error: %v", recvErr)
		}
		nameField := item.Descriptor().Fields().ByName("name")
		names = append(names, item.Get(nameField).String())
	}

	if !reflect.DeepEqual(names, []string{"go-echo-x", "go-echo-y"}) {
		t.Errorf("expected %v, got %v", []string{"go-echo-x", "go-echo-y"}, names)
	}
}

// ============================================================================
// T082: JS client -> Go server
// ============================================================================

// TestJSClientGoServer_Unary registers a Go server handler, then calls
// it from JS.
func TestJSClientGoServer_Unary(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Register a Go handler on the channel.
	env.channel.RegisterStreamHandler("/testgrpc.TestService/Echo", func(ctx context.Context, stream *inprocgrpc.RPCStream) {
		// Read request
		stream.Recv().Recv(func(msg any, err error) {
			if err != nil {
				stream.Finish(err)
				return
			}
			reqMsg := msg.(proto.Message)
			reqBytes, _ := proto.Marshal(reqMsg)

			// Decode into dynamicpb to read fields
			reqDesc, _ := env.pbMod.FindDescriptor("testgrpc.EchoRequest")
			reqDyn := dynamicpb.NewMessage(reqDesc.(protoreflect.MessageDescriptor))
			proto.Unmarshal(reqBytes, reqDyn)

			// Build response
			respDesc, _ := env.pbMod.FindDescriptor("testgrpc.EchoResponse")
			respDyn := dynamicpb.NewMessage(respDesc.(protoreflect.MessageDescriptor))
			msgField := respDyn.Descriptor().Fields().ByName("message")
			codeField := respDyn.Descriptor().Fields().ByName("code")

			inMsg := reqDyn.Get(reqDyn.Descriptor().Fields().ByName("message")).String()
			respDyn.Set(msgField, protoreflect.ValueOfString("go-server: "+inMsg))
			respDyn.Set(codeField, protoreflect.ValueOfInt32(42))

			stream.Send().Send(respDyn)
			stream.Finish(nil)
		})
	})

	env.runOnLoop(t, `
		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'from-js');
		var result;
		client.echo(req).then(function(resp) {
			result = { message: resp.get('message'), code: resp.get('code') };
			__done();
		}).catch(function(err) {
			result = { error: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if resultObj["error"] != nil {
		t.Errorf("expected nil, got %v", resultObj["error"])
	}
	if got := resultObj["message"]; got != "go-server: from-js" {
		t.Errorf("expected %v, got %v", "go-server: from-js", got)
	}
	if got := resultObj["code"]; got != int64(42) {
		t.Errorf("expected %v, got %v", int64(42), got)
	}
}

// TestJSClientGoServer_ServerStream registers a Go server-stream handler.
func TestJSClientGoServer_ServerStream(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.channel.RegisterStreamHandler("/testgrpc.TestService/ServerStream", func(ctx context.Context, stream *inprocgrpc.RPCStream) {
		stream.Recv().Recv(func(msg any, err error) {
			if err != nil {
				stream.Finish(err)
				return
			}
			// Send 3 items
			itemDesc, _ := env.pbMod.FindDescriptor("testgrpc.Item")
			for i := range 3 {
				item := dynamicpb.NewMessage(itemDesc.(protoreflect.MessageDescriptor))
				item.Set(item.Descriptor().Fields().ByName("id"), protoreflect.ValueOfString("go-id"))
				item.Set(item.Descriptor().Fields().ByName("name"),
					protoreflect.ValueOfString("go-item-"+string(rune('A'+i))))
				stream.Send().Send(item)
			}
			stream.Finish(nil)
		})
	})

	env.runOnLoop(t, `
		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'go-list');
		var items = [];
		client.serverStream(req).then(function(stream) {
			function readNext() {
				stream.recv().then(function(result) {
					if (result.done) {
						__done();
						return;
					}
					items.push(result.value.get('name'));
					readNext();
				}).catch(function(err) {
					items.push('error: ' + err.message);
					__done();
				});
			}
			readNext();
		}).catch(function(err) {
			items.push('open-error: ' + err.message);
			__done();
		});
	`, defaultTimeout)

	items := env.runtime.Get("items")
	if items == nil {
		t.Fatalf("expected non-nil")
	}
	arr := items.Export().([]any)
	if got := len(arr); got != 3 {
		t.Errorf("expected %v, got %v", 3, got)
	}
	if got := arr[0]; got != "go-item-A" {
		t.Errorf("expected %v, got %v", "go-item-A", got)
	}
	if got := arr[1]; got != "go-item-B" {
		t.Errorf("expected %v, got %v", "go-item-B", got)
	}
	if got := arr[2]; got != "go-item-C" {
		t.Errorf("expected %v, got %v", "go-item-C", got)
	}
}

// TestJSClientGoServer_UnaryError tests Go server returning gRPC error.
func TestJSClientGoServer_UnaryError(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.channel.RegisterStreamHandler("/testgrpc.TestService/Echo", func(ctx context.Context, stream *inprocgrpc.RPCStream) {
		stream.Recv().Recv(func(msg any, err error) {
			if err != nil {
				stream.Finish(err)
				return
			}
			stream.Finish(status.Errorf(codes.Unavailable, "service down"))
		})
	})

	env.runOnLoop(t, `
		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test');
		var error;
		client.echo(req).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { name: err.name, code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["name"]; got != "GrpcError" {
		t.Errorf("expected %v, got %v", "GrpcError", got)
	}
	if got := resultObj["code"]; got != int64(14) {
		t.Errorf("expected %v, got %v", int64(14), got)
	}
	if !strings.Contains(resultObj["message"].(string), "service down") {
		t.Errorf("expected %q to contain %q", resultObj["message"], "service down")
	}
}
