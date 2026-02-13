package gojagrpc

import (
	"context"
	"fmt"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// ======================== Test gRPC Server ==========================

// testDialServer starts a real gRPC server on a random TCP port using
// dynamicpb message types (no generated code). Returns the address
// and a stop function.
func testDialServer(t *testing.T) (addr string, stop func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)

	// Resolve message descriptors from our test proto.
	msgDescs := testMessageDescriptors(t)
	echoReqDesc := msgDescs["testgrpc.EchoRequest"]
	echoRespDesc := msgDescs["testgrpc.EchoResponse"]
	itemMsgDesc := msgDescs["testgrpc.Item"]

	s := grpc.NewServer()

	// Register TestService with manual ServiceDesc using dynamicpb.
	svcDesc := &grpc.ServiceDesc{
		ServiceName: "testgrpc.TestService",
		HandlerType: (*interface{})(nil),
		Methods: []grpc.MethodDesc{
			{
				MethodName: "Echo",
				Handler: func(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
					req := dynamicpb.NewMessage(echoReqDesc)
					if err := dec(req); err != nil {
						return nil, err
					}
					msg := req.Get(echoReqDesc.Fields().ByName("message")).String()
					resp := dynamicpb.NewMessage(echoRespDesc)
					resp.Set(echoRespDesc.Fields().ByName("message"), protoreflect.ValueOfString("dial-echo:"+msg))
					resp.Set(echoRespDesc.Fields().ByName("code"), protoreflect.ValueOfInt32(42))
					return resp, nil
				},
			},
		},
		Streams: []grpc.StreamDesc{
			{
				StreamName:    "ServerStream",
				ServerStreams: true,
				Handler: func(srv interface{}, stream grpc.ServerStream) error {
					req := dynamicpb.NewMessage(echoReqDesc)
					if err := stream.RecvMsg(req); err != nil {
						return err
					}
					msg := req.Get(echoReqDesc.Fields().ByName("message")).String()
					for i := 0; i < 3; i++ {
						item := dynamicpb.NewMessage(itemMsgDesc)
						item.Set(itemMsgDesc.Fields().ByName("id"), protoreflect.ValueOfString(fmt.Sprintf("%d", i)))
						item.Set(itemMsgDesc.Fields().ByName("name"), protoreflect.ValueOfString(fmt.Sprintf("%s-%d", msg, i)))
						if err := stream.SendMsg(item); err != nil {
							return err
						}
					}
					return nil
				},
			},
			{
				StreamName:    "ClientStream",
				ClientStreams: true,
				Handler: func(srv interface{}, stream grpc.ServerStream) error {
					var count int
					var lastID string
					for {
						item := dynamicpb.NewMessage(itemMsgDesc)
						if err := stream.RecvMsg(item); err != nil {
							if err == io.EOF {
								break
							}
							return err
						}
						count++
						lastID = item.Get(itemMsgDesc.Fields().ByName("id")).String()
					}
					resp := dynamicpb.NewMessage(echoRespDesc)
					resp.Set(echoRespDesc.Fields().ByName("message"), protoreflect.ValueOfString(fmt.Sprintf("received:%d:last:%s", count, lastID)))
					return stream.SendMsg(resp)
				},
			},
			{
				StreamName:    "BidiStream",
				ClientStreams: true,
				ServerStreams: true,
				Handler: func(srv interface{}, stream grpc.ServerStream) error {
					for {
						item := dynamicpb.NewMessage(itemMsgDesc)
						if err := stream.RecvMsg(item); err != nil {
							if err == io.EOF {
								return nil
							}
							return err
						}
						name := item.Get(itemMsgDesc.Fields().ByName("name")).String()
						echo := dynamicpb.NewMessage(itemMsgDesc)
						echo.Set(itemMsgDesc.Fields().ByName("id"), item.Get(itemMsgDesc.Fields().ByName("id")))
						echo.Set(itemMsgDesc.Fields().ByName("name"), protoreflect.ValueOfString("bidi-echo:"+name))
						if err := stream.SendMsg(echo); err != nil {
							return err
						}
					}
				},
			},
		},
		Metadata: "testgrpc.proto",
	}
	s.RegisterService(svcDesc, nil)

	go s.Serve(lis)

	return lis.Addr().String(), s.GracefulStop
}

// testMessageDescriptors builds protoreflect.MessageDescriptor map
// for the test proto messages using protodesc.
func testMessageDescriptors(t *testing.T) map[string]protoreflect.MessageDescriptor {
	t.Helper()

	fdp := testGrpcFileDescriptorProto()

	// protodesc.NewFile builds a protoreflect.FileDescriptor from the
	// raw FileDescriptorProto. We pass nil for the resolver because
	// our test proto has no imports.
	file, err := protodesc.NewFile(fdp, nil)
	require.NoError(t, err)

	result := make(map[string]protoreflect.MessageDescriptor)
	for i := 0; i < file.Messages().Len(); i++ {
		md := file.Messages().Get(i)
		result[string(md.FullName())] = md
	}
	return result
}

// ============================================================================
// T235: Test: dial to real gRPC server (integration)
// ============================================================================

// TestDial_UnaryRPC verifies that a JS client can dial a real gRPC
// server and make a unary RPC.
func TestDial_UnaryRPC(t *testing.T) {
	addr, stop := testDialServer(t)
	defer stop()

	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, fmt.Sprintf(`
		var ch = grpc.dial('%s', { insecure: true });
		var client = grpc.createClient('testgrpc.TestService', { channel: ch });

		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'hello-dial');

		var result;
		client.echo(req).then(function(resp) {
			result = {
				message: resp.get('message'),
				code: resp.get('code')
			};
			ch.close();
			__done();
		}).catch(function(err) {
			result = { error: err.message };
			ch.close();
			__done();
		});
	`, addr), defaultTimeout)

	r := env.runtime.Get("result")
	require.NotNil(t, r)
	rObj := r.Export().(map[string]interface{})
	assert.Equal(t, "dial-echo:hello-dial", rObj["message"])
	assert.Equal(t, int64(42), rObj["code"])
}

// ============================================================================
// T236: Test: dial streaming RPCs
// ============================================================================

// TestDial_ServerStreamRPC verifies server-streaming over a dialed connection.
func TestDial_ServerStreamRPC(t *testing.T) {
	addr, stop := testDialServer(t)
	defer stop()

	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, fmt.Sprintf(`
		var ch = grpc.dial('%s', { insecure: true });
		var client = grpc.createClient('testgrpc.TestService', { channel: ch });

		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'stream');

		var items = [];
		client.serverStream(req).then(function(stream) {
			function pump() {
				stream.recv().then(function(r) {
					if (r.done) {
						ch.close();
						__done();
						return;
					}
					items.push(r.value.get('name'));
					pump();
				}).catch(function(err) {
					items.push('ERROR:' + err.message);
					ch.close();
					__done();
				});
			}
			pump();
		}).catch(function(err) {
			items.push('STREAM_ERROR:' + err.message);
			ch.close();
			__done();
		});
	`, addr), defaultTimeout)

	itemsVal := env.runtime.Get("items")
	require.NotNil(t, itemsVal)
	items := itemsVal.Export().([]interface{})
	require.Len(t, items, 3)
	assert.Equal(t, "stream-0", items[0])
	assert.Equal(t, "stream-1", items[1])
	assert.Equal(t, "stream-2", items[2])
}

// TestDial_ClientStreamRPC verifies client-streaming over a dialed connection.
func TestDial_ClientStreamRPC(t *testing.T) {
	addr, stop := testDialServer(t)
	defer stop()

	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, fmt.Sprintf(`
		var ch = grpc.dial('%s', { insecure: true });
		var client = grpc.createClient('testgrpc.TestService', { channel: ch });

		var Item = pb.messageType('testgrpc.Item');
		var result;
		client.clientStream().then(function(call) {
			var item1 = new Item();
			item1.set('id', 'A');
			item1.set('name', 'first');
			return call.send(item1).then(function() {
				var item2 = new Item();
				item2.set('id', 'B');
				item2.set('name', 'second');
				return call.send(item2);
			}).then(function() {
				return call.closeSend();
			}).then(function() {
				return call.response;
			});
		}).then(function(resp) {
			result = { message: resp.get('message') };
			ch.close();
			__done();
		}).catch(function(err) {
			result = { error: err.message };
			ch.close();
			__done();
		});
	`, addr), defaultTimeout)

	r := env.runtime.Get("result")
	require.NotNil(t, r)
	rObj := r.Export().(map[string]interface{})
	assert.Equal(t, "received:2:last:B", rObj["message"])
}

// TestDial_BidiStreamRPC verifies bidi-streaming over a dialed connection.
func TestDial_BidiStreamRPC(t *testing.T) {
	addr, stop := testDialServer(t)
	defer stop()

	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, fmt.Sprintf(`
		var ch = grpc.dial('%s', { insecure: true });
		var client = grpc.createClient('testgrpc.TestService', { channel: ch });

		var Item = pb.messageType('testgrpc.Item');
		var results = [];
		client.bidiStream().then(function(stream) {
			var item1 = new Item();
			item1.set('id', '1');
			item1.set('name', 'ping');
			stream.send(item1).then(function() {
				return stream.recv();
			}).then(function(r) {
				results.push(r.value.get('name'));
				var item2 = new Item();
				item2.set('id', '2');
				item2.set('name', 'pong');
				return stream.send(item2);
			}).then(function() {
				return stream.recv();
			}).then(function(r) {
				results.push(r.value.get('name'));
				return stream.closeSend();
			}).then(function() {
				return stream.recv();
			}).then(function(r) {
				if (r.done) results.push('DONE');
				ch.close();
				__done();
			}).catch(function(err) {
				results.push('ERROR:' + err.message);
				ch.close();
				__done();
			});
		}).catch(function(err) {
			results.push('STREAM_ERROR:' + err.message);
			ch.close();
			__done();
		});
	`, addr), defaultTimeout)

	resultsVal := env.runtime.Get("results")
	require.NotNil(t, resultsVal)
	results := resultsVal.Export().([]interface{})
	require.Len(t, results, 3)
	assert.Equal(t, "bidi-echo:ping", results[0])
	assert.Equal(t, "bidi-echo:pong", results[1])
	assert.Equal(t, "DONE", results[2])
}

// ============================================================================
// T234: Connection lifecycle
// ============================================================================

// TestDial_Close verifies that close() works and target() returns
// the dial target.
func TestDial_Close(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.run(t, `
		var ch = grpc.dial('localhost:0', { insecure: true });
		var tgt = ch.target();
		ch.close();
	`)

	tgt := env.runtime.Get("tgt")
	require.NotNil(t, tgt)
	assert.Equal(t, "localhost:0", tgt.String())
}

// TestDial_ChannelOption verifies the { channel: ch } createClient option.
func TestDial_ChannelOption(t *testing.T) {
	addr, stop := testDialServer(t)
	defer stop()

	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, fmt.Sprintf(`
		var ch = grpc.dial('%s', { insecure: true });
		var client = grpc.createClient('testgrpc.TestService', { channel: ch });

		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'channel-opt');

		var result;
		client.echo(req).then(function(resp) {
			result = resp.get('message');
			ch.close();
			__done();
		}).catch(function(err) {
			result = 'error:' + err.message;
			ch.close();
			__done();
		});
	`, addr), defaultTimeout)

	r := env.runtime.Get("result")
	require.NotNil(t, r)
	assert.Equal(t, "dial-echo:channel-opt", r.String())
}

// ============================================================================
// T237: Test: dial error handling
// ============================================================================

// TestDial_EmptyTarget verifies that dial(”) throws a TypeError.
func TestDial_EmptyTarget(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	err := env.mustFail(t, `grpc.dial('')`)
	assert.Contains(t, err.Error(), "target must be a non-empty string")
}

// TestDial_InvalidChannel verifies that { channel: 42 } throws.
func TestDial_InvalidChannel(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	err := env.mustFail(t, `grpc.createClient('testgrpc.TestService', { channel: 42 })`)
	assert.Contains(t, err.Error(), "channel must be a dial() result")
}

// TestDial_ConnectionRefused verifies that dialing a non-existent
// server produces a sensible error on the first RPC.
func TestDial_ConnectionRefused(t *testing.T) {
	// Find an unused port by binding and then closing.
	lis, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	deadAddr := lis.Addr().String()
	lis.Close()

	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, fmt.Sprintf(`
		var ch = grpc.dial('%s', { insecure: true });
		var client = grpc.createClient('testgrpc.TestService', { channel: ch });

		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'should-fail');

		var result;
		client.echo(req).then(function(resp) {
			result = { unexpected: true };
			ch.close();
			__done();
		}).catch(function(err) {
			result = { name: err.name, code: err.code, message: err.message };
			ch.close();
			__done();
		});
	`, deadAddr), defaultTimeout)

	r := env.runtime.Get("result")
	require.NotNil(t, r)
	rObj := r.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", rObj["name"])
	// UNAVAILABLE (14) for connection refused.
	assert.Equal(t, int64(14), rObj["code"])
}

// TestDial_Authority verifies the authority option is passed through.
func TestDial_Authority(t *testing.T) {
	addr, stop := testDialServer(t)
	defer stop()

	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// The authority option should not break the connection.
	env.runOnLoop(t, fmt.Sprintf(`
		var ch = grpc.dial('%s', { insecure: true, authority: 'custom.authority' });
		var client = grpc.createClient('testgrpc.TestService', { channel: ch });

		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'authority-test');

		var result;
		client.echo(req).then(function(resp) {
			result = resp.get('message');
			ch.close();
			__done();
		}).catch(function(err) {
			result = 'error:' + err.message;
			ch.close();
			__done();
		});
	`, addr), defaultTimeout)

	r := env.runtime.Get("result")
	require.NotNil(t, r)
	assert.Equal(t, "dial-echo:authority-test", r.String())
}

// TestDial_NoTransportSecurity verifies dial without insecure throws.
func TestDial_NoTransportSecurity(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// grpc.NewClient requires explicit transport security.
	err := env.mustFail(t, `grpc.dial('localhost:0')`)
	assert.Contains(t, err.Error(), "no transport security set")
}

// TestDial_NullOptions verifies dial with null options throws (no transport security).
func TestDial_NullOptions(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	err := env.mustFail(t, `grpc.dial('localhost:0', null)`)
	assert.Contains(t, err.Error(), "no transport security set")
}

// TestDial_ChannelNull verifies null channel falls back to in-proc.
func TestDial_ChannelNull(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// { channel: null } should fall back to in-process.
	env.run(t, `
		var client = grpc.createClient('testgrpc.TestService', { channel: null });
	`)
}

// TestDial_ChannelMissingConn verifies that an object without _conn throws.
func TestDial_ChannelMissingConn(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	err := env.mustFail(t, `grpc.createClient('testgrpc.TestService', { channel: {} })`)
	assert.Contains(t, err.Error(), "channel must be a dial() result")
}

// TestDial_AbortSignal verifies that AbortSignal works with dialed connections.
func TestDial_AbortSignal(t *testing.T) {
	addr, stop := testDialServer(t)
	defer stop()

	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, fmt.Sprintf(`
		var ch = grpc.dial('%s', { insecure: true });
		var client = grpc.createClient('testgrpc.TestService', { channel: ch });

		var controller = new AbortController();
		controller.abort(); // Pre-abort.

		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'abort-dial');

		var result;
		client.echo(req, { signal: controller.signal }).then(function(resp) {
			result = { unexpected: true };
			ch.close();
			__done();
		}).catch(function(err) {
			result = { name: err.name, code: err.code };
			ch.close();
			__done();
		});
	`, addr), defaultTimeout)

	r := env.runtime.Get("result")
	require.NotNil(t, r)
	rObj := r.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", rObj["name"])
	assert.Equal(t, int64(1), rObj["code"]) // CANCELLED
}
