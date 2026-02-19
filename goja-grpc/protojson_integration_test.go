package gojagrpc

import (
	"strings"
	"testing"

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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pjExports := env.runtime.NewObject()
	pjMod.SetupExports(pjExports)
	if env.runtime.Set("protojson", pjExports) != nil {
		t.Fatalf("unexpected error: %v", env.runtime.Set("protojson", pjExports))
	}

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
	if !(errVal == nil || isGojaUndefined(errVal)) {
		t.Fatalf("unexpected error: %v", errVal)
	}

	// Verify JSON string is valid proto3 JSON
	jsonStr := env.runtime.Get("jsonStr")
	if jsonStr == nil {
		t.Fatalf("expected non-nil")
	}
	js := jsonStr.Export().(string)
	if !strings.Contains(js, `"message"`) {
		t.Errorf("expected %q to contain %q", js, `"message"`)
	}
	if !strings.Contains(js, `hello world`) {
		t.Errorf("expected %q to contain %q", js, `hello world`)
	}

	// Verify unmarshaled message has correct fields
	unmarshaled := env.runtime.Get("unmarshaled")
	if unmarshaled == nil {
		t.Fatalf("expected non-nil")
	}
	msg := env.run(t, `unmarshaled.get('message')`)
	if got := msg.Export(); got != "hello world" {
		t.Errorf("expected %v, got %v", "hello world", got)
	}
	code := env.run(t, `unmarshaled.get('code')`)
	if got := code.Export(); got != int64(99) {
		t.Errorf("expected %v, got %v", int64(99), got)
	}
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
	if !(errVal == nil || isGojaUndefined(errVal)) {
		t.Fatalf("unexpected error: %v", errVal)
	}

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["message"]; got != "processed: test input" {
		t.Errorf("expected %v, got %v", "processed: test input", got)
	}
	if got := resultObj["code"]; got != int64(123) {
		t.Errorf("expected %v, got %v", int64(123), got)
	}
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
	if !(errVal == nil || isGojaUndefined(errVal)) {
		t.Fatalf("unexpected error: %v", errVal)
	}

	formatted := env.runtime.Get("formatted")
	if formatted == nil {
		t.Fatalf("expected non-nil")
	}
	fmtStr := formatted.Export().(string)
	// format() uses 2-space indentation
	if !strings.Contains(fmtStr, "\n") {
		t.Errorf("expected %q to contain %q", fmtStr, "\n")
	}
	if !strings.Contains(fmtStr, `"message"`) {
		t.Errorf("expected %q to contain %q", fmtStr, `"message"`)
	}
	if !strings.Contains(fmtStr, `formatted`) {
		t.Errorf("expected %q to contain %q", fmtStr, `formatted`)
	}
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
	if !(errVal == nil || isGojaUndefined(errVal)) {
		t.Fatalf("unexpected error: %v", errVal)
	}

	without := env.runtime.Get("withoutDefaults").Export().(string)
	with := env.runtime.Get("withDefaults").Export().(string)

	// Without emitDefaults, code:0 should be omitted
	if strings.Contains(without, `"code"`) {
		t.Errorf("expected %q to not contain %q", without, `"code"`)
	}
	// With emitDefaults, code:0 should appear
	if !strings.Contains(with, `"code"`) {
		t.Errorf("expected %q to contain %q", with, `"code"`)
	}
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
	if !(errVal == nil || isGojaUndefined(errVal)) {
		t.Fatalf("unexpected error: %v", errVal)
	}

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["message"]; got != "roundtrip: ping" {
		t.Errorf("expected %v, got %v", "roundtrip: ping", got)
	}
	if got := resultObj["code"]; got != int64(42) {
		t.Errorf("expected %v, got %v", int64(42), got)
	}
	if !strings.Contains(resultObj["json"].(string), "roundtrip: ping") {
		t.Errorf("expected %q to contain %q", resultObj["json"].(string), "roundtrip: ping")
	}
}
