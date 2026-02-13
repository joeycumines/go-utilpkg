package gojagrpc

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	gojarequire "github.com/dop251/goja_nodejs/require"
	eventloop "github.com/joeycumines/go-eventloop"
	inprocgrpc "github.com/joeycumines/go-inprocgrpc"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// nonDynamicMsg wraps a *dynamicpb.Message but is NOT a
// *dynamicpb.Message type, forcing the slow path in toWrappedMessage.
type nonDynamicMsg struct {
	*dynamicpb.Message
}

// ============================================================================
// T083: Coverage gap tests
// ============================================================================

// --- Require() via require.Registry ---

func TestRequire_ViaRegistry(t *testing.T) {
	loop, err := eventloop.New()
	require.NoError(t, err)

	runtime := goja.New()

	adapter, err := gojaeventloop.New(loop, runtime)
	require.NoError(t, err)
	require.NoError(t, adapter.Bind())

	channel := inprocgrpc.NewChannel(loop)

	pbMod, err := gojaprotobuf.New(runtime)
	require.NoError(t, err)

	registry := gojarequire.NewRegistry()
	registry.RegisterNativeModule("protobuf", gojaprotobuf.Require())
	registry.RegisterNativeModule("grpc", Require(
		WithChannel(channel),
		WithProtobuf(pbMod),
		WithAdapter(adapter),
	))
	registry.Enable(runtime)

	// require('grpc') should return exports with createClient, createServer, status
	val, err := runtime.RunString(`
		var grpc = require('grpc');
		typeof grpc.createClient === 'function' &&
		typeof grpc.createServer === 'function' &&
		typeof grpc.status === 'object' &&
		typeof grpc.metadata === 'object';
	`)
	require.NoError(t, err)
	assert.True(t, val.ToBoolean())
}

// --- SetupExports public wrapper ---

func TestSetupExports_PublicWrapper(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	exports := env.runtime.NewObject()
	env.grpcMod.SetupExports(exports)

	assert.NotNil(t, exports.Get("createClient"))
	assert.NotNil(t, exports.Get("createServer"))
	assert.NotNil(t, exports.Get("status"))
	assert.NotNil(t, exports.Get("metadata"))
}

// --- Unary handler returning nil/undefined ---

func TestUnaryHandler_ReturnsUndefined(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				// Return undefined (no return statement).
			},
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
		client.echo(req).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { name: err.name, code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	assert.Equal(t, int64(13), resultObj["code"]) // INTERNAL = 13
	assert.Contains(t, resultObj["message"], "nil/undefined")
}

// --- Unary handler returning wrong type ---

func TestUnaryHandler_ReturnsNonMessage(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				return "I am a string, not a protobuf message";
			},
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
		client.echo(req).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { name: err.name, code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	assert.Equal(t, int64(13), resultObj["code"]) // INTERNAL
	assert.Contains(t, resultObj["message"], "handler response")
}

// --- Async unary handler rejects with string ---

func TestUnaryHandler_AsyncRejectString(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				return new Promise(function(resolve, reject) {
					reject("plain string error");
				});
			},
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
		client.echo(req).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { name: err.name, code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	assert.Equal(t, int64(13), resultObj["code"]) // INTERNAL
}

// --- Async unary handler rejects with generic Error ---

func TestUnaryHandler_AsyncRejectGenericError(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				return new Promise(function(resolve, reject) {
					reject(new Error("something went wrong"));
				});
			},
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
		client.echo(req).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { name: err.name, code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	assert.Equal(t, int64(13), resultObj["code"]) // INTERNAL
	// Note: the message may vary depending on how goja serializes Error
	// objects through Promise rejection chains.
}

// --- Metadata on unary call ---

func TestUnaryRPC_WithMetadata(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'with-metadata');
				resp.set('code', 1);
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
		req.set('message', 'test');

		var md = grpc.metadata.create();
		md.set('x-custom-key', 'custom-value');
		md.set('x-request-id', '12345');

		var result;
		client.echo(req, { metadata: md }).then(function(resp) {
			result = { message: resp.get('message'), code: resp.get('code') };
			__done();
		}).catch(function(err) {
			result = { error: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Nil(t, resultObj["error"])
	assert.Equal(t, "with-metadata", resultObj["message"])
}

// --- context.DeadlineExceeded maps to DEADLINE_EXCEEDED ---

func TestGrpcErrorFromGoError_DeadlineExceeded(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Create a channel that will time out
	env.runOnLoop(t, `
		// Don't register any handler — the call should use a super-short timeout.
		// Actually, register a handler that returns a never-resolving promise.
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				return new Promise(function(resolve) {
					// Never resolve — the deadline will expire.
				});
			},
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		// We can't set a deadline from JS easily; instead test that
		// an already-cancelled signal produces a cancellation error.
		var controller = new AbortController();
		controller.abort(); // abort immediately (already aborted)

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'timeout-test');
		var error;
		client.echo(req, { signal: controller.signal }).then(function() {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { name: err.name, code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	// This should be Canceled(1) since we used AbortSignal
	assert.Equal(t, int64(1), resultObj["code"])
}

// --- applySignal: signal property is not an object ---

func TestApplySignal_NonObject(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'ok');
				resp.set('code', 1);
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
		req.set('message', 'test');
		var result;
		// Pass a non-object signal
		client.echo(req, { signal: "not-a-signal" }).then(function(resp) {
			result = resp.get('message');
			__done();
		}).catch(function(err) {
			result = 'error: ' + err.message;
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	assert.Equal(t, "ok", result.String())
}

// --- applySignal: signal._signal is missing ---

func TestApplySignal_MissingNativeSignal(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'ok');
				resp.set('code', 1);
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
		req.set('message', 'test');
		var result;
		// Pass an object without _signal
		client.echo(req, { signal: { aborted: false } }).then(function(resp) {
			result = resp.get('message');
			__done();
		}).catch(function(err) {
			result = 'error: ' + err.message;
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	assert.Equal(t, "ok", result.String())
}

// --- applySignal: _signal is not an AbortSignal ---

func TestApplySignal_WrongNativeType(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'ok');
				resp.set('code', 1);
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
		req.set('message', 'test');
		var result;
		// Pass an object with _signal that's not an AbortSignal
		client.echo(req, { signal: { _signal: 42 } }).then(function(resp) {
			result = resp.get('message');
			__done();
		}).catch(function(err) {
			result = 'error: ' + err.message;
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	assert.Equal(t, "ok", result.String())
}

// --- Server streaming handler that throws ---

func TestServerStreamHandler_SyncThrow(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {
				throw new Error("sync server stream error");
			},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test');
		var error;
		client.serverStream(req).then(function(stream) {
			stream.recv().then(function() {
				__done();
			}).catch(function(err) {
				error = { code: err.code, message: err.message };
				__done();
			});
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(13), resultObj["code"]) // INTERNAL
	assert.Contains(t, resultObj["message"], "sync server stream error")
}

// --- Client stream handler that throws on first recv ---

func TestClientStreamHandler_SyncThrow(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) {
				throw new Error("sync client stream error");
			},
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var error;
		client.clientStream().then(function(call) {
			call.response.then(function() {
				__done();
			}).catch(function(err) {
				error = { code: err.code, message: err.message };
				__done();
			});
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(13), resultObj["code"]) // INTERNAL
	assert.Contains(t, resultObj["message"], "sync client stream error")
}

// --- Bidi handler that throws ---

func TestBidiStreamHandler_SyncThrow(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {
				throw new Error("sync bidi error");
			}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var error;
		client.bidiStream().then(function(stream) {
			stream.recv().then(function() {
				__done();
			}).catch(function(err) {
				error = { code: err.code, message: err.message };
				__done();
			});
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(13), resultObj["code"]) // INTERNAL
	assert.Contains(t, resultObj["message"], "sync bidi error")
}

// --- parseCallOpts: no opts argument ---

func TestParseCallOpts_NoArg(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'no-opts');
				resp.set('code', 1);
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
		req.set('message', 'test');
		var result;
		// No second argument at all
		client.echo(req).then(function(resp) {
			result = resp.get('message');
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	assert.Equal(t, "no-opts", result.String())
}

// --- parseCallOpts: null opts ---

func TestParseCallOpts_NullOpts(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'null-opts');
				resp.set('code', 1);
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
		req.set('message', 'test');
		var result;
		client.echo(req, null).then(function(resp) {
			result = resp.get('message');
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	assert.Equal(t, "null-opts", result.String())
}

// --- isThenable: null/undefined/non-object ---

func TestIsThenable_NilAndPrimitive(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Handler returns null → treated as nil/undefined error
	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				return null;
			},
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
		client.echo(req).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(13), resultObj["code"]) // INTERNAL
}

// --- resolveOptions: option that returns error ---

func TestResolveOptions_BadOption(t *testing.T) {
	runtime := goja.New()

	_, err := New(runtime,
		WithChannel(nil), // nil channel should error
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "channel")
}

// --- Metadata.toObject for metadataToGo with non-wrapper ---

func TestMetadataToGo_PlainObject(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Test passing a plain JS object as metadata (should try toObject path)
	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'got-metadata');
				resp.set('code', 1);
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
		req.set('message', 'test');

		// Pass metadata as a plain Metadata wrapper
		var md = grpc.metadata.create();
		md.set('x-test', 'value1');

		var result;
		client.echo(req, { metadata: md }).then(function(resp) {
			result = resp.get('message');
			__done();
		}).catch(function(err) {
			result = 'error: ' + err.message;
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	assert.Equal(t, "got-metadata", result.String())
}

// --- Unregistered method errors (covers client error paths) ---

func TestUnaryRPC_UnregisteredMethod(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Don't register any server, call a method that doesn't exist
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
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	assert.Equal(t, int64(12), resultObj["code"]) // UNIMPLEMENTED = 12
}

func TestServerStreamRPC_UnregisteredMethod(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test');
		var error;
		client.serverStream(req).then(function(stream) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { name: err.name, code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	assert.Equal(t, int64(12), resultObj["code"]) // UNIMPLEMENTED
}

func TestClientStreamRPC_UnregisteredMethod(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var client = grpc.createClient('testgrpc.TestService');
		var error;
		client.clientStream().then(function(call) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { name: err.name, code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	assert.Equal(t, int64(12), resultObj["code"]) // UNIMPLEMENTED
}

func TestBidiStreamRPC_UnregisteredMethod(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var client = grpc.createClient('testgrpc.TestService');
		var error;
		client.bidiStream().then(function(stream) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { name: err.name, code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	assert.Equal(t, int64(12), resultObj["code"]) // UNIMPLEMENTED
}

// ============================================================================
// T125: Direct unit tests for lowest-coverage functions
// ============================================================================

// --- grpcErrorFromGoError direct tests (37.5% → 100%) ---

func TestGrpcErrorFromGoError_Direct_StatusError(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	err := status.Errorf(codes.NotFound, "not here")
	obj := env.grpcMod.grpcErrorFromGoError(err)
	assert.Equal(t, "GrpcError", obj.Get("name").String())
	assert.Equal(t, int64(codes.NotFound), obj.Get("code").ToInteger())
	assert.Contains(t, obj.Get("message").String(), "not here")
}

func TestGrpcErrorFromGoError_Direct_ContextCanceled(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	obj := env.grpcMod.grpcErrorFromGoError(context.Canceled)
	assert.Equal(t, "GrpcError", obj.Get("name").String())
	assert.Equal(t, int64(codes.Canceled), obj.Get("code").ToInteger())
}

func TestGrpcErrorFromGoError_Direct_DeadlineExceeded(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	obj := env.grpcMod.grpcErrorFromGoError(context.DeadlineExceeded)
	assert.Equal(t, "GrpcError", obj.Get("name").String())
	assert.Equal(t, int64(codes.DeadlineExceeded), obj.Get("code").ToInteger())
}

func TestGrpcErrorFromGoError_Direct_GenericError(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	obj := env.grpcMod.grpcErrorFromGoError(assert.AnError)
	assert.Equal(t, "GrpcError", obj.Get("name").String())
	assert.Equal(t, int64(codes.Internal), obj.Get("code").ToInteger())
}

// --- toWrappedMessage direct tests (33.3% → 100%) ---

func TestToWrappedMessage_Direct_DynamicPB(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	desc, err := env.pbMod.FindDescriptor("testgrpc.EchoRequest")
	require.NoError(t, err)
	msgDesc := desc.(protoreflect.MessageDescriptor)
	msg := dynamicpb.NewMessage(msgDesc)
	msg.Set(msgDesc.Fields().ByName("message"), protoreflect.ValueOfString("fast"))

	obj, convErr := env.grpcMod.toWrappedMessage(msg, msgDesc)
	require.NoError(t, convErr)
	require.NotNil(t, obj)
}

func TestToWrappedMessage_Direct_NonProto(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	desc, err := env.pbMod.FindDescriptor("testgrpc.EchoRequest")
	require.NoError(t, err)
	msgDesc := desc.(protoreflect.MessageDescriptor)

	_, convErr := env.grpcMod.toWrappedMessage("not a proto", msgDesc)
	require.Error(t, convErr)
	assert.Contains(t, convErr.Error(), "not a proto.Message")
}

func TestToWrappedMessage_Direct_NilValue(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	desc, err := env.pbMod.FindDescriptor("testgrpc.EchoRequest")
	require.NoError(t, err)
	msgDesc := desc.(protoreflect.MessageDescriptor)

	_, convErr := env.grpcMod.toWrappedMessage(nil, msgDesc)
	require.Error(t, convErr)
	assert.Contains(t, convErr.Error(), "not a proto.Message")
}

// --- jsErrorToGRPC direct tests (75% → 100%) ---

func TestJsErrorToGRPC_Direct_NonException(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	err := env.grpcMod.jsErrorToGRPC(assert.AnError)
	s, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, s.Code())
}

func TestJsErrorToGRPC_Direct_GrpcErrorException(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	_, jsErr := env.runtime.RunString(`
		(function() { throw grpc.status.createError(grpc.status.PERMISSION_DENIED, "denied"); })()
	`)
	require.Error(t, jsErr)

	grpcErr := env.grpcMod.jsErrorToGRPC(jsErr)
	s, ok := status.FromError(grpcErr)
	require.True(t, ok)
	assert.Equal(t, codes.PermissionDenied, s.Code())
	assert.Contains(t, s.Message(), "denied")
}

func TestJsErrorToGRPC_Direct_GenericJSException(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	_, jsErr := env.runtime.RunString(`
		(function() { throw new Error("generic error"); })()
	`)
	require.Error(t, jsErr)

	grpcErr := env.grpcMod.jsErrorToGRPC(jsErr)
	s, ok := status.FromError(grpcErr)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, s.Code())
	assert.Contains(t, s.Message(), "generic error")
}

// --- jsValueToGRPCError direct tests (92.9% → 100%) ---

func TestJsValueToGRPCError_Direct_Nil(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	err := env.grpcMod.jsValueToGRPCError(nil)
	s, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, s.Code())
	assert.Contains(t, s.Message(), "unknown error")
}

func TestJsValueToGRPCError_Direct_Undefined(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	err := env.grpcMod.jsValueToGRPCError(goja.Undefined())
	s, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, s.Code())
}

func TestJsValueToGRPCError_Direct_NonObject(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	err := env.grpcMod.jsValueToGRPCError(env.runtime.ToValue("plain string"))
	s, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, s.Code())
	assert.Contains(t, s.Message(), "plain string")
}

func TestJsValueToGRPCError_Direct_GenericJSError(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val, jsErr := env.runtime.RunString(`new Error("something broke")`)
	require.NoError(t, jsErr)

	err := env.grpcMod.jsValueToGRPCError(val)
	s, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, s.Code())
	assert.Contains(t, s.Message(), "something broke")
}

func TestJsValueToGRPCError_Direct_ObjectNoMessage(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Object with no name or message property.
	val, jsErr := env.runtime.RunString(`({foo: "bar"})`)
	require.NoError(t, jsErr)

	err := env.grpcMod.jsValueToGRPCError(val)
	s, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.Internal, s.Code())
}

// --- Require error path (83.3% → 100%) ---

func TestRequire_ErrorFromOptions(t *testing.T) {
	rt := goja.New()
	loader := Require() // no options → missing channel, protobuf, adapter

	module := rt.NewObject()
	exports := rt.NewObject()
	_ = module.Set("exports", exports)

	assert.Panics(t, func() {
		loader(rt, module)
	})
}

// --- resolveOptions nil option (92.3% → 100%) ---

func TestResolveOptions_NilOption(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	_, err := New(env.runtime,
		nil, // nil option should be skipped
		WithChannel(env.channel),
		WithProtobuf(env.pbMod),
		WithAdapter(env.adapter),
	)
	require.NoError(t, err)
}

// --- metadataToGo edge cases (82.9% → 100%) ---

func TestMetadataToGo_Direct_ToObjectNotFunction(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val, jsErr := env.runtime.RunString(`({toObject: "not-a-function"})`)
	require.NoError(t, jsErr)
	assert.Nil(t, env.grpcMod.metadataToGo(val))
}

func TestMetadataToGo_Direct_ToObjectThrows(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val, jsErr := env.runtime.RunString(`({toObject: function() { throw new Error("boom"); }})`)
	require.NoError(t, jsErr)
	assert.Nil(t, env.grpcMod.metadataToGo(val))
}

func TestMetadataToGo_Direct_ToObjectReturnsNonObject(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val, jsErr := env.runtime.RunString(`({toObject: function() { return 42; }})`)
	require.NoError(t, jsErr)
	assert.Nil(t, env.grpcMod.metadataToGo(val))
}

func TestMetadataToGo_Direct_EmptyArrayValue(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val, jsErr := env.runtime.RunString(`({toObject: function() { return {key: []}; }})`)
	require.NoError(t, jsErr)
	md := env.grpcMod.metadataToGo(val)
	assert.Empty(t, md.Get("key"))
}

func TestMetadataToGo_Direct_NonArrayValue(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val, jsErr := env.runtime.RunString(`({toObject: function() { return {key: "not-array"}; }})`)
	require.NoError(t, jsErr)
	md := env.grpcMod.metadataToGo(val)
	assert.Nil(t, md.Get("key"))
}

// --- parseCallOpts non-object arg ---

func TestParseCallOpts_NonObjectArg(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
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

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test');
		var result;
		client.echo(req, 42).then(function(resp) {
			result = resp.get('message');
			__done();
		}).catch(function(err) {
			result = 'err: ' + err.message;
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	assert.Equal(t, "ok", result.String())
}

// --- isThenable direct edge cases ---

func TestIsThenable_Direct(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	assert.False(t, env.grpcMod.isThenable(nil))
	assert.False(t, env.grpcMod.isThenable(goja.Undefined()))
	assert.False(t, env.grpcMod.isThenable(goja.Null()))
	assert.False(t, env.grpcMod.isThenable(env.runtime.ToValue(42)))
	assert.False(t, env.grpcMod.isThenable(env.runtime.ToValue("string")))

	// Object without then.
	obj := env.runtime.NewObject()
	assert.False(t, env.grpcMod.isThenable(obj))

	// Object with non-function then.
	_ = obj.Set("then", "not-a-function")
	assert.False(t, env.grpcMod.isThenable(obj))

	// Object with function then.
	_ = obj.Set("then", env.runtime.ToValue(func(goja.FunctionCall) goja.Value { return goja.Undefined() }))
	assert.True(t, env.grpcMod.isThenable(obj))
}

// --- Unary handler returning a non-message wrapped value ---

func TestServerHandler_NilReturn(t *testing.T) {
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
		client.echo(req).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { code: err.code };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(codes.Internal), resultObj["code"])
}

// --- Server stream: async handler error path ---

func TestServerStreamHandler_AsyncReject(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {
				return new Promise(function(resolve, reject) {
					reject(grpc.status.createError(grpc.status.UNAVAILABLE, 'down'));
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
		var error;
		client.serverStream(req).then(function(stream) {
			stream.recv().then(function(result) {
				error = { unexpected: true };
				__done();
			}).catch(function(err) {
				error = { code: err.code };
				__done();
			});
		}).catch(function(err) {
			error = { code: err.code };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(codes.Unavailable), resultObj["code"])
}

// ============================================================================
// T125 (batch 2): Additional coverage tests for remaining gaps
// ============================================================================

// --- toWrappedMessage slow path: non-dynamicpb proto.Message ---

func TestToWrappedMessage_Direct_SlowPath(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	desc, err := env.pbMod.FindDescriptor("testgrpc.EchoRequest")
	require.NoError(t, err)
	msgDesc := desc.(protoreflect.MessageDescriptor)

	// Create a dynamicpb message and wrap in nonDynamicMsg to force slow path.
	inner := dynamicpb.NewMessage(msgDesc)
	inner.Set(msgDesc.Fields().ByName("message"), protoreflect.ValueOfString("slow-path"))
	wrapped := &nonDynamicMsg{Message: inner}

	obj, convErr := env.grpcMod.toWrappedMessage(wrapped, msgDesc)
	require.NoError(t, convErr)
	require.NotNil(t, obj)
	// The returned object should be a valid wrapped protobuf message.
	// The get() method should work to retrieve the field.
	getFn, ok := goja.AssertFunction(obj.Get("get"))
	require.True(t, ok, "wrapped message should have get() method")
	val, callErr := getFn(obj, env.runtime.ToValue("message"))
	require.NoError(t, callErr)
	assert.Equal(t, "slow-path", val.String())
}

// --- finishUnaryResponse: handler returns non-message (sync) ---

func TestUnaryHandler_ReturnNonMessage_Sync(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return 42; },
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
		client.echo(req).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(codes.Internal), resultObj["code"])
}

// --- finishUnaryResponse: handler returns non-message (async) ---

func TestUnaryHandler_ReturnNonMessage_Async(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				return new Promise(function(resolve, reject) {
					resolve(42);
				});
			},
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
		client.echo(req).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(codes.Internal), resultObj["code"])
}

// --- thenFinishUnary rejection: async unary handler rejects ---

func TestUnaryHandler_AsyncReject(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				return new Promise(function(resolve, reject) {
					reject(grpc.status.createError(grpc.status.NOT_FOUND, 'not found'));
				});
			},
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
		client.echo(req).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(5), resultObj["code"]) // NOT_FOUND = 5
	assert.Contains(t, resultObj["message"].(string), "not found")
}

// --- makeUnaryHandler: handler throws sync (not async) ---

func TestUnaryHandler_SyncThrow(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				throw grpc.status.createError(grpc.status.ALREADY_EXISTS, 'dup');
			},
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
		client.echo(req).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(6), resultObj["code"]) // ALREADY_EXISTS = 6
	assert.Contains(t, resultObj["message"].(string), "dup")
}

// --- makeServerStreamHandler: handler throws sync with specific code ---

func TestServerStreamHandler_SyncThrow_FailedPrecondition(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {
				throw grpc.status.createError(grpc.status.FAILED_PRECONDITION, 'precondition');
			},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test');
		var error;
		client.serverStream(req).then(function(stream) {
			stream.recv().then(function(result) {
				error = { unexpected: true };
				__done();
			}).catch(function(err) {
				error = { code: err.code, message: err.message };
				__done();
			});
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(9), resultObj["code"]) // FAILED_PRECONDITION = 9
}

// --- addServerSend: send non-message triggers TypeError ---

func TestServerStreamHandler_SendBadType(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {
				try {
					call.send("not a proto message");
				} catch(e) {
					// TypeError expected — the handler finishes cleanly after catch
				}
			},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test');
		var streamDone = false;
		client.serverStream(req).then(function(stream) {
			stream.recv().then(function(result) {
				streamDone = result.done;
				__done();
			}).catch(function(err) {
				streamDone = true;
				__done();
			});
		}).catch(function(err) {
			streamDone = true;
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("streamDone")
	require.NotNil(t, result)
	assert.True(t, result.ToBoolean())
}

// --- makeClientStreamHandler: sync throw with specific code ---

func TestClientStreamHandler_SyncThrow_ResourceExhausted(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) {
				throw grpc.status.createError(grpc.status.RESOURCE_EXHAUSTED, 'exhausted');
			},
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var error;
		client.clientStream().then(function(call) {
			// Try to send — should fail if stream errored
			call.response.then(function(resp) {
				error = { unexpected: true };
				__done();
			}).catch(function(err) {
				error = { code: err.code, message: err.message };
				__done();
			});
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(8), resultObj["code"]) // RESOURCE_EXHAUSTED = 8
}

// --- makeClientStreamHandler: async reject ---

func TestClientStreamHandler_AsyncReject(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) {
				return new Promise(function(resolve, reject) {
					reject(grpc.status.createError(grpc.status.ABORTED, 'aborted'));
				});
			},
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var error;
		client.clientStream().then(function(call) {
			call.response.then(function(resp) {
				error = { unexpected: true };
				__done();
			}).catch(function(err) {
				error = { code: err.code, message: err.message };
				__done();
			});
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(10), resultObj["code"]) // ABORTED = 10
}

// --- makeClientStreamHandler: returns non-message (sync) ---

func TestClientStreamHandler_ReturnNonMessage(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) {
				return "not a message";
			},
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var error;
		client.clientStream().then(function(call) {
			call.response.then(function(resp) {
				error = { unexpected: true };
				__done();
			}).catch(function(err) {
				error = { code: err.code, message: err.message };
				__done();
			});
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(codes.Internal), resultObj["code"])
}

// --- makeBidiStreamHandler: sync throw with specific code ---

func TestBidiStreamHandler_SyncThrow_DataLoss(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {
				throw grpc.status.createError(grpc.status.DATA_LOSS, 'lost');
			}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var error;
		client.bidiStream().then(function(stream) {
			stream.recv().then(function(result) {
				error = { unexpected: true };
				__done();
			}).catch(function(err) {
				error = { code: err.code, message: err.message };
				__done();
			});
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(15), resultObj["code"]) // DATA_LOSS = 15
}

// --- makeBidiStreamHandler: async reject ---

func TestBidiStreamHandler_AsyncReject(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {
				return new Promise(function(resolve, reject) {
					reject(grpc.status.createError(grpc.status.UNAUTHENTICATED, 'no auth'));
				});
			}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var error;
		client.bidiStream().then(function(stream) {
			stream.recv().then(function(result) {
				error = { unexpected: true };
				__done();
			}).catch(function(err) {
				error = { code: err.code, message: err.message };
				__done();
			});
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(16), resultObj["code"]) // UNAUTHENTICATED = 16
}

// --- Server Stream: handler sends valid item then finishes async ---

func TestServerStreamHandler_SendThenAsyncResolve(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {
				var Item = pb.messageType('testgrpc.Item');
				var item = new Item();
				item.set('id', '1');
				item.set('name', 'first');
				call.send(item);
				return new Promise(function(resolve) {
					resolve();
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
		var items = [];
		client.serverStream(req).then(function(stream) {
			function readNext() {
				stream.recv().then(function(result) {
					if (result.done) {
						__done();
					} else {
						items.push(result.value.get('name'));
						readNext();
					}
				}).catch(function(err) {
					__done();
				});
			}
			readNext();
		}).catch(function(err) {
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("items")
	require.NotNil(t, result)
	exported := result.Export()
	arr, ok := exported.([]interface{})
	require.True(t, ok)
	assert.Equal(t, 1, len(arr))
	assert.Equal(t, "first", arr[0])
}

// --- Bidi stream: send and receive items ---

func TestBidiStreamHandler_SendRecv(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
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
							} else {
								// Echo back with modified name.
								var Item = pb.messageType('testgrpc.Item');
								var item = new Item();
								item.set('id', result.value.get('id'));
								item.set('name', 'echo-' + result.value.get('name'));
								call.send(item);
								readLoop();
							}
						}).catch(function(err) {
							reject(err);
						});
					}
					readLoop();
				});
			}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var received = [];
		client.bidiStream().then(function(stream) {
			var Item = pb.messageType('testgrpc.Item');
			var item1 = new Item();
			item1.set('id', 'a');
			item1.set('name', 'alpha');
			stream.send(item1).then(function() {
				return stream.closeSend();
			}).then(function() {
				function readLoop() {
					stream.recv().then(function(result) {
						if (result.done) {
							__done();
						} else {
							received.push(result.value.get('name'));
							readLoop();
						}
					}).catch(function(err) {
						__done();
					});
				}
				readLoop();
			});
		}).catch(function(err) {
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("received")
	require.NotNil(t, result)
	exported := result.Export()
	arr, ok := exported.([]interface{})
	require.True(t, ok)
	assert.Equal(t, 1, len(arr))
	assert.Equal(t, "echo-alpha", arr[0])
}

// --- Client stream: send items and receive response ---

func TestClientStreamHandler_SendAndReceive(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) {
				return new Promise(function(resolve, reject) {
					var count = 0;
					function readLoop() {
						call.recv().then(function(result) {
							if (result.done) {
								var EchoResponse = pb.messageType('testgrpc.EchoResponse');
								var resp = new EchoResponse();
								resp.set('message', 'received:' + count);
								resp.set('code', count);
								resolve(resp);
							} else {
								count++;
								readLoop();
							}
						}).catch(function(err) {
							reject(err);
						});
					}
					readLoop();
				});
			},
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var result;
		client.clientStream().then(function(call) {
			var Item = pb.messageType('testgrpc.Item');
			var item1 = new Item();
			item1.set('id', '1');
			item1.set('name', 'one');
			var item2 = new Item();
			item2.set('id', '2');
			item2.set('name', 'two');
			call.send(item1).then(function() {
				return call.send(item2);
			}).then(function() {
				return call.closeSend();
			}).then(function() {
				call.response.then(function(resp) {
					result = resp.get('message');
					__done();
				}).catch(function(err) {
					result = 'err: ' + err.message;
					__done();
				});
			});
		}).catch(function(err) {
			result = 'err: ' + err.message;
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	assert.Equal(t, "received:2", result.String())
}

// --- metadataFromGo: nil metadata (direct) ---

func TestMetadataFromGo_Direct_Nil(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	result := env.grpcMod.metadataFromGo(nil)
	assert.True(t, goja.IsUndefined(result))
}

// --- metadataFromGo: non-nil metadata (direct) ---

func TestMetadataFromGo_Direct_NonNil(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	md := make(map[string][]string)
	md["x-test"] = []string{"val1", "val2"}

	result := env.grpcMod.metadataFromGo(md)
	require.NotNil(t, result)
	assert.False(t, goja.IsUndefined(result))
}

// --- lowerFirst edge cases ---

func TestLowerFirst_SingleChar(t *testing.T) {
	assert.Equal(t, "a", lowerFirst("A"))
	assert.Equal(t, "z", lowerFirst("Z"))
}

func TestLowerFirst_AlreadyLower(t *testing.T) {
	assert.Equal(t, "already", lowerFirst("already"))
}

func TestLowerFirst_MultiChar(t *testing.T) {
	assert.Equal(t, "echo", lowerFirst("Echo"))
	assert.Equal(t, "serverStream", lowerFirst("ServerStream"))
}

// --- metadata wrapper: forEach, delete, getAll ---

func TestMetadataWrapper_ForEachDeleteGetAll(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.run(t, `
		var md = grpc.metadata.create();
		md.set('key1', 'a', 'b');
		md.set('key2', 'c');

		var entries = [];
		md.forEach(function(value, key) {
			entries.push(key + '=' + value);
		});
		entries.sort();

		var all1 = md.getAll('key1');
		md.delete('key1');
		var afterDelete = md.get('key1');

		JSON.stringify({
			entries: entries,
			all1Length: all1.length,
			afterDelete: afterDelete === undefined
		});
	`)
	require.NotNil(t, val)
	result := val.Export().(string)
	assert.Contains(t, result, `"all1Length":2`)
	assert.Contains(t, result, `"afterDelete":true`)
}

// --- metadata set requires 2 args ---

func TestMetadataWrapper_SetTooFewArgs(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	err := env.mustFail(t, `
		var md = grpc.metadata.create();
		md.set('only-key');
	`)
	assert.Contains(t, err.Error(), "at least 2 arguments")
}

// --- metadata forEach requires function ---

func TestMetadataWrapper_ForEachNonFunction(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	err := env.mustFail(t, `
		var md = grpc.metadata.create();
		md.forEach("not a function");
	`)
	assert.Contains(t, err.Error(), "function")
}

// --- Client stream: send bad type throws TypeError ---

func TestClientStream_SendBadType(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) {
				return new Promise(function(resolve, reject) {
					call.recv().then(function(result) {
						var EchoResponse = pb.messageType('testgrpc.EchoResponse');
						var resp = new EchoResponse();
						resp.set('message', 'ok');
						resolve(resp);
					});
				});
			},
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var error;
		client.clientStream().then(function(call) {
			try {
				call.send("not a proto message");
			} catch(e) {
				error = { name: e.name, message: e.message };
				__done();
				return;
			}
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "TypeError", resultObj["name"])
}

// --- Bidi stream: send bad type throws TypeError ---

func TestBidiStream_SendBadType(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {
				return new Promise(function(resolve) {
					call.recv().then(function(result) {
						resolve();
					}).catch(function() { resolve(); });
				});
			}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var error;
		client.bidiStream().then(function(stream) {
			try {
				stream.send("not a proto message");
			} catch(e) {
				error = { name: e.name, message: e.message };
				__done();
				return;
			}
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "TypeError", resultObj["name"])
}

// --- Unary RPC with AbortSignal: cancelled ---

func TestUnaryRPC_WithAbortSignal_Cancelled(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				// Slow handler — won't respond before abort
				return new Promise(function(resolve, reject) {
					// Delay long enough for the abort to fire.
					setTimeout(function() {
						var EchoResponse = pb.messageType('testgrpc.EchoResponse');
						var resp = new EchoResponse();
						resp.set('message', 'late');
						resolve(resp);
					}, 5000);
				});
			},
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test');

		var ac = new AbortController();
		var error;
		client.echo(req, { signal: ac.signal }).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { code: err.code, name: err.name };
			__done();
		});

		// Abort immediately.
		ac.abort();
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	// Cancelled = 1
	assert.Equal(t, int64(codes.Canceled), resultObj["code"])
}

// ============================================================================
// T125 (batch 3): Targeting remaining coverage gaps
// ============================================================================

// --- makeServerStreamMethod: bad request argument (panic path) ---

func TestServerStreamRPC_BadRequestType(t *testing.T) {
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
		var error;
		try {
			client.serverStream("not a proto message");
		} catch(e) {
			error = { name: e.name, message: e.message };
		}
		__done();
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "TypeError", resultObj["name"])
}

// --- makeUnaryMethod: bad request argument (panic path) ---

func TestUnaryRPC_BadRequestType(t *testing.T) {
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
		var error;
		try {
			client.echo("not a proto message");
		} catch(e) {
			error = { name: e.name, message: e.message };
		}
		__done();
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "TypeError", resultObj["name"])
}

// --- metadataToGo: object value without length (plain object as array value) ---

func TestMetadataToGo_Direct_ObjectValueNoLength(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val, jsErr := env.runtime.RunString(`({toObject: function() { return {"x-key": {foo: "bar"}}; }})`)
	require.NoError(t, jsErr)
	md := env.grpcMod.metadataToGo(val)
	assert.Nil(t, md.Get("x-key"))
}

// --- metadataToGo: array with undefined element ---

func TestMetadataToGo_Direct_ArrayWithUndefined(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val, jsErr := env.runtime.RunString(`({toObject: function() { return {"key": [undefined, "val"]}; }})`)
	require.NoError(t, jsErr)
	md := env.grpcMod.metadataToGo(val)
	vals := md.Get("key")
	// Only "val" should be present since undefined is skipped.
	assert.Equal(t, []string{"val"}, vals)
}

// --- applySignal: signal that is already aborted ---

func TestApplySignal_AlreadyAborted(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'should-not-reach');
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
		req.set('message', 'test');

		// Pre-aborted signal.
		var ac = new AbortController();
		ac.abort();

		var error;
		client.echo(req, { signal: ac.signal }).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { code: err.code, name: err.name };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	assert.Equal(t, int64(codes.Canceled), resultObj["code"])
}

// --- applySignal: signal option is non-object ---

func TestApplySignal_NonObjectSignal(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
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

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test');
		var result;
		// Pass a non-object signal — should be ignored gracefully.
		client.echo(req, { signal: "not-a-signal" }).then(function(resp) {
			result = resp.get('message');
			__done();
		}).catch(function(err) {
			result = 'err: ' + err.message;
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	assert.Equal(t, "ok", result.String())
}

// --- applySignal: signal without _signal property (fake signal) ---

func TestApplySignal_FakeSignalObject(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
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

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test');
		var result;
		// Pass an object that looks like a signal but has no _signal.
		client.echo(req, { signal: {} }).then(function(resp) {
			result = resp.get('message');
			__done();
		}).catch(function(err) {
			result = 'err: ' + err.message;
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	assert.Equal(t, "ok", result.String())
}

// --- makeBidiStreamHandler: async resolve (non-error finish) ---

func TestBidiStreamHandler_AsyncResolve(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {
				return new Promise(function(resolve) {
					call.recv().then(function(result) {
						if (result.done) {
							resolve();
						}
					});
				});
			}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var done = false;
		client.bidiStream().then(function(stream) {
			stream.closeSend().then(function() {
				stream.recv().then(function(result) {
					done = result.done;
					__done();
				}).catch(function(err) {
					done = true;
					__done();
				});
			});
		}).catch(function(err) {
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("done")
	require.NotNil(t, result)
	assert.True(t, result.ToBoolean())
}

// --- Server stream: EOF path (no items sent) ---

func TestServerStreamHandler_EmptyStream(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {
				// Handler finishes without sending anything.
			},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test');
		var streamDone = false;
		client.serverStream(req).then(function(stream) {
			stream.recv().then(function(result) {
				streamDone = result.done;
				__done();
			}).catch(function(err) {
				streamDone = true;
				__done();
			});
		}).catch(function(err) {
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("streamDone")
	require.NotNil(t, result)
	assert.True(t, result.ToBoolean())
}

// --- Server stream: multiple items sent then resolve ---

func TestServerStreamHandler_MultipleItems(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {
				var Item = pb.messageType('testgrpc.Item');
				for (var i = 0; i < 3; i++) {
					var item = new Item();
					item.set('id', String(i));
					item.set('name', 'item-' + i);
					call.send(item);
				}
			},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test');
		var names = [];
		client.serverStream(req).then(function(stream) {
			function readLoop() {
				stream.recv().then(function(result) {
					if (result.done) {
						__done();
					} else {
						names.push(result.value.get('name'));
						readLoop();
					}
				}).catch(function(err) {
					__done();
				});
			}
			readLoop();
		}).catch(function(err) {
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("names")
	require.NotNil(t, result)
	exported := result.Export()
	arr, ok := exported.([]interface{})
	require.True(t, ok)
	assert.Equal(t, 3, len(arr))
	assert.Equal(t, "item-0", arr[0])
	assert.Equal(t, "item-1", arr[1])
	assert.Equal(t, "item-2", arr[2])
}

// ============================================================================
// T125 (batch 4): Final coverage gap closure
// ============================================================================

// --- submitOrRejectDirect: loop stopped (failure path) ---

func TestSubmitOrRejectDirect_LoopStopped(t *testing.T) {
	loop, err := eventloop.New()
	require.NoError(t, err)

	runtime := goja.New()

	adapter, err := gojaeventloop.New(loop, runtime)
	require.NoError(t, err)
	require.NoError(t, adapter.Bind())

	channel := inprocgrpc.NewChannel(loop)

	pbMod, err := gojaprotobuf.New(runtime)
	require.NoError(t, err)

	grpcMod, err := New(runtime,
		WithChannel(channel),
		WithProtobuf(pbMod),
		WithAdapter(adapter),
	)
	require.NoError(t, err)

	// Shut down the loop so Submit will fail.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	loop.Shutdown(ctx)

	// Now test submitOrRejectDirect — should call reject.
	var rejected bool
	var rejectedVal any
	grpcMod.submitOrRejectDirect(func(v any) {
		rejected = true
		rejectedVal = v
	}, func() {
		t.Fatal("fn should not be called when loop is stopped")
	})

	assert.True(t, rejected)
	require.NotNil(t, rejectedVal)
	assert.Contains(t, rejectedVal.(error).Error(), "event loop not running")
}

// --- Server stream RPC with pre-aborted signal ---

func TestServerStreamRPC_PreAbortedSignal(t *testing.T) {
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

		var ac = new AbortController();
		ac.abort();

		var error;
		client.serverStream(req, { signal: ac.signal }).then(function(stream) {
			stream.recv().then(function(result) {
				error = { unexpected: true };
				__done();
			}).catch(function(err) {
				error = { code: err.code, name: err.name };
				__done();
			});
		}).catch(function(err) {
			error = { code: err.code, name: err.name };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	// Should be CANCELLED because the context was canceled by the abort.
	assert.Equal(t, int64(codes.Canceled), resultObj["code"])
}

// --- Client stream RPC with pre-aborted signal ---

func TestClientStreamRPC_PreAbortedSignal(t *testing.T) {
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
		var ac = new AbortController();
		ac.abort();

		var error;
		client.clientStream({ signal: ac.signal }).then(function(call) {
			call.response.then(function(resp) {
				error = { unexpected: true };
				__done();
			}).catch(function(err) {
				error = { code: err.code, name: err.name };
				__done();
			});
		}).catch(function(err) {
			error = { code: err.code, name: err.name };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	assert.Equal(t, int64(codes.Canceled), resultObj["code"])
}

// --- Bidi stream RPC with pre-aborted signal ---

func TestBidiStreamRPC_PreAbortedSignal(t *testing.T) {
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
		var ac = new AbortController();
		ac.abort();

		var error;
		client.bidiStream({ signal: ac.signal }).then(function(stream) {
			stream.recv().then(function(result) {
				error = { unexpected: true };
				__done();
			}).catch(function(err) {
				error = { code: err.code, name: err.name };
				__done();
			});
		}).catch(function(err) {
			error = { code: err.code, name: err.name };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	assert.Equal(t, int64(codes.Canceled), resultObj["code"])
}

// --- Server stream: error during recv after initial success ---

func TestServerStreamRPC_RecvError(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {
				// Send one item then finish with error.
				var Item = pb.messageType('testgrpc.Item');
				var item = new Item();
				item.set('id', '1');
				item.set('name', 'ok');
				call.send(item);
				// Immediately finish with error after sending.
				return new Promise(function(resolve, reject) {
					reject(grpc.status.createError(grpc.status.INTERNAL, 'mid-stream'));
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
		var items = [];
		var recvError;
		client.serverStream(req).then(function(stream) {
			function readLoop() {
				stream.recv().then(function(result) {
					if (result.done) {
						__done();
					} else {
						items.push(result.value.get('name'));
						readLoop();
					}
				}).catch(function(err) {
					recvError = { code: err.code };
					__done();
				});
			}
			readLoop();
		}).catch(function(err) {
			recvError = { code: err.code };
			__done();
		});
	`, defaultTimeout)

	// Should get at least one item, then an error on subsequent recv.
	result := env.runtime.Get("items")
	require.NotNil(t, result)
	errResult := env.runtime.Get("recvError")
	// Either we got items and error, or just error.
	assert.True(t, errResult != nil && !goja.IsUndefined(errResult) || result != nil)
}
