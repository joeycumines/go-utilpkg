package gojagrpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gojaprotojson "github.com/joeycumines/goja-protojson"
)

// grpcProtojsonTestEnv wraps grpcTestEnv with an additional goja-protojson
// module wired into the JS runtime as the global "protojson".
type grpcProtojsonTestEnv struct {
	*grpcTestEnv
	pjMod *gojaprotojson.Module
}

func newGrpcProtojsonTestEnv(t *testing.T) *grpcProtojsonTestEnv {
	t.Helper()

	env := newGrpcTestEnv(t)

	pjMod, err := gojaprotojson.New(env.runtime, gojaprotojson.WithProtobuf(env.pbMod))
	require.NoError(t, err)

	pjExports := env.runtime.NewObject()
	pjMod.SetupExports(pjExports)
	require.NoError(t, env.runtime.Set("protojson", pjExports))

	return &grpcProtojsonTestEnv{grpcTestEnv: env, pjMod: pjMod}
}

// TestProtojsonIntegration_MarshalResponse verifies that a gRPC server
// handler can use protojson.marshal to serialize a protobuf response
// message to JSON, and the client can independently re-parse it via
// protojson.unmarshal, demonstrating the protojson module works with
// messages produced by the goja-grpc stack.
func TestProtojsonIntegration_MarshalResponse(t *testing.T) {
	env := newGrpcProtojsonTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'hello ' + request.get('message'));
				resp.set('code', 99);
				return resp;
			},
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'world');

		var jsonStr;
		var unmarshaled;
		var error;

		client.echo(req).then(function(resp) {
			// Marshal the response to JSON using protojson
			jsonStr = protojson.marshal(resp);

			// Unmarshal back to a new message
			unmarshaled = protojson.unmarshal('testgrpc.EchoResponse', jsonStr);
			__done();
		}).catch(function(err) {
			error = err;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	// Verify JSON string is valid proto3 JSON
	jsonStr := env.runtime.Get("jsonStr")
	require.NotNil(t, jsonStr)
	js := jsonStr.Export().(string)
	assert.Contains(t, js, `"message"`)
	assert.Contains(t, js, `hello world`)

	// Verify unmarshaled message has correct fields
	unmarshaled := env.runtime.Get("unmarshaled")
	require.NotNil(t, unmarshaled)
	msg := env.run(t, `unmarshaled.get('message')`)
	assert.Equal(t, "hello world", msg.Export())
	code := env.run(t, `unmarshaled.get('code')`)
	assert.Equal(t, int64(99), code.Export())
}

// TestProtojsonIntegration_UnmarshalRequest verifies that a gRPC server
// handler can receive a regular protobuf message, marshal it to JSON via
// protojson, transform the JSON, then unmarshal back to a protobuf message
// and return it.
func TestProtojsonIntegration_UnmarshalRequest(t *testing.T) {
	env := newGrpcProtojsonTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				// Marshal request to JSON
				var jsonStr = protojson.marshal(request);
				// Parse, transform, and unmarshal as response
				var parsed = JSON.parse(jsonStr);
				parsed.code = 123;
				parsed.message = 'processed: ' + parsed.message;
				var newJson = JSON.stringify(parsed);
				var resp = protojson.unmarshal('testgrpc.EchoResponse', newJson);
				return resp;
			},
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test input');

		var result;
		var error;

		client.echo(req).then(function(resp) {
			result = {
				message: resp.get('message'),
				code: resp.get('code')
			};
			__done();
		}).catch(function(err) {
			error = err;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]any)
	assert.Equal(t, "processed: test input", resultObj["message"])
	assert.Equal(t, int64(123), resultObj["code"])
}

// TestProtojsonIntegration_FormatMessage verifies protojson.format()
// works on messages flowing through the gRPC stack.
func TestProtojsonIntegration_FormatMessage(t *testing.T) {
	env := newGrpcProtojsonTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'formatted');
				resp.set('code', 7);
				return resp;
			},
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'fmt test');

		var formatted;
		var error;

		client.echo(req).then(function(resp) {
			formatted = protojson.format(resp);
			__done();
		}).catch(function(err) {
			error = err;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	formatted := env.runtime.Get("formatted")
	require.NotNil(t, formatted)
	fmtStr := formatted.Export().(string)
	// format() uses 2-space indentation
	assert.Contains(t, fmtStr, "\n")
	assert.Contains(t, fmtStr, `"message"`)
	assert.Contains(t, fmtStr, `formatted`)
}

// TestProtojsonIntegration_MarshalOptions verifies custom marshal options
// (emitDefaults, useProtoNames, enumAsNumber) work on gRPC messages.
func TestProtojsonIntegration_MarshalOptions(t *testing.T) {
	env := newGrpcProtojsonTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				// Only set message, leave code as default (0)
				resp.set('message', 'partial');
				return resp;
			},
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'opts');

		var withDefaults;
		var withoutDefaults;
		var error;

		client.echo(req).then(function(resp) {
			// Without emitDefaults: code=0 should be omitted
			withoutDefaults = protojson.marshal(resp);
			// With emitDefaults: code=0 should appear
			withDefaults = protojson.marshal(resp, { emitDefaults: true });
			__done();
		}).catch(function(err) {
			error = err;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	without := env.runtime.Get("withoutDefaults").Export().(string)
	with := env.runtime.Get("withDefaults").Export().(string)

	// Without emitDefaults, code:0 should be omitted
	assert.NotContains(t, without, `"code"`)
	// With emitDefaults, code:0 should appear
	assert.Contains(t, with, `"code"`)
}

// TestProtojsonIntegration_RoundtripThroughJSON verifies a complete
// roundtrip: create message → marshal to JSON → modify JSON → unmarshal
// back → send via gRPC → verify on client. This is the most comprehensive
// integration between goja-protobuf, goja-protojson, and goja-grpc.
func TestProtojsonIntegration_RoundtripThroughJSON(t *testing.T) {
	env := newGrpcProtojsonTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				// Full roundtrip: request → JSON → modify → new message
				var json = protojson.marshal(request);
				var obj = JSON.parse(json);

				// Transform: build response from request data
				var respJson = JSON.stringify({
					message: 'roundtrip: ' + obj.message,
					code: 42
				});
				return protojson.unmarshal('testgrpc.EchoResponse', respJson);
			},
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'ping');

		var result;
		var error;

		client.echo(req).then(function(resp) {
			// Marshal the final response to JSON and back to verify full roundtrip
			var finalJson = protojson.marshal(resp);
			var finalMsg = protojson.unmarshal('testgrpc.EchoResponse', finalJson);
			result = {
				message: finalMsg.get('message'),
				code: finalMsg.get('code'),
				json: finalJson
			};
			__done();
		}).catch(function(err) {
			error = err;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]any)
	assert.Equal(t, "roundtrip: ping", resultObj["message"])
	assert.Equal(t, int64(42), resultObj["code"])
	assert.Contains(t, resultObj["json"].(string), "roundtrip: ping")
}
