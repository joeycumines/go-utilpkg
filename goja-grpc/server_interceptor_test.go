package gojagrpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// T174: Server interceptor tests
// ============================================================================

func TestServerInterceptor_AuthCheck(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addInterceptor(function(next) {
			return function(call) {
				var auth = call.requestHeader.get('x-auth');
				if (auth !== 'valid-token') {
					throw grpc.status.createError(16, 'unauthenticated');
				}
				return next(call);
			};
		});
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'ok:' + request.get('message'));
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
		req.set('message', 'hello');

		// First call without auth — should fail.
		var error1;
		var result;
		client.echo(req).then(function(resp) {
			error1 = 'should not succeed';
		}).catch(function(err) {
			error1 = err.code + ':' + err.message;

			// Second call with auth — should succeed.
			var md = grpc.metadata.create();
			md.set('x-auth', 'valid-token');
			return client.echo(req, { metadata: md });
		}).then(function(resp) {
			result = resp.get('message');
			__done();
		}).catch(function(err) {
			result = 'error:' + err.message;
			__done();
		});
	`, defaultTimeout)

	err1 := env.runtime.Get("error1")
	require.NotNil(t, err1)
	assert.Contains(t, err1.String(), "16:")
	assert.Contains(t, err1.String(), "unauthenticated")

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	assert.Equal(t, "ok:hello", result.String())
}

func TestServerInterceptor_Logging(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var logs = [];
		var server = grpc.createServer();
		server.addInterceptor(function(next) {
			return function(call) {
				logs.push('before:' + call.method);
				var result = next(call);
				logs.push('after:' + call.method);
				return result;
			};
		});
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'logged');
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
		client.echo(req).then(function(resp) {
			result = resp.get('message');
			__done();
		}).catch(function(err) { __done(); });
	`, defaultTimeout)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	assert.Equal(t, "logged", result.String())

	logs := env.runtime.Get("logs")
	require.NotNil(t, logs)
	exported := logs.Export().([]interface{})
	require.Len(t, exported, 2)
	assert.Equal(t, "before:/testgrpc.TestService/Echo", exported[0])
	assert.Equal(t, "after:/testgrpc.TestService/Echo", exported[1])
}

func TestServerInterceptor_Chain(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var order = [];
		var server = grpc.createServer();
		server.addInterceptor(function(next) {
			return function(call) {
				order.push('first-before');
				var result = next(call);
				order.push('first-after');
				return result;
			};
		});
		server.addInterceptor(function(next) {
			return function(call) {
				order.push('second-before');
				var result = next(call);
				order.push('second-after');
				return result;
			};
		});
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				order.push('handler');
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

		client.echo(req).then(function() {
			__done();
		}).catch(function(err) { __done(); });
	`, defaultTimeout)

	order := env.runtime.Get("order")
	require.NotNil(t, order)
	exported := order.Export().([]interface{})
	// Onion pattern: first-before → second-before → handler → second-after → first-after
	require.Len(t, exported, 5)
	assert.Equal(t, "first-before", exported[0])
	assert.Equal(t, "second-before", exported[1])
	assert.Equal(t, "handler", exported[2])
	assert.Equal(t, "second-after", exported[3])
	assert.Equal(t, "first-after", exported[4])
}

func TestServerInterceptor_AddResponseMetadata(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addInterceptor(function(next) {
			return function(call) {
				// Add response header from interceptor.
				var md = grpc.metadata.create();
				md.set('x-interceptor', 'was-here');
				call.setHeader(md);
				return next(call);
			};
		});
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

		var headerVal;
		client.echo(req, {
			onHeader: function(md) {
				headerVal = md.get('x-interceptor');
			}
		}).then(function(resp) {
			__done();
		}).catch(function(err) { __done(); });
	`, defaultTimeout)

	hdr := env.runtime.Get("headerVal")
	require.NotNil(t, hdr)
	assert.Equal(t, "was-here", hdr.String())
}

func TestServerInterceptor_ErrorMapping(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addInterceptor(function(next) {
			return function(call) {
				try {
					return next(call);
				} catch(e) {
					// Map all errors to PERMISSION_DENIED.
					throw grpc.status.createError(7, 'access denied: ' + e.message);
				}
			};
		});
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				throw grpc.status.createError(2, 'something broke');
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
		client.echo(req).catch(function(err) {
			result = err.code + ':' + err.message;
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	assert.Contains(t, result.String(), "7:")
	assert.Contains(t, result.String(), "access denied")
}

func TestServerInterceptor_MethodAccess(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var capturedMethod;
		var server = grpc.createServer();
		server.addInterceptor(function(next) {
			return function(call) {
				capturedMethod = call.method;
				return next(call);
			};
		});
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

		client.echo(req).then(function() {
			__done();
		}).catch(function(err) { __done(); });
	`, defaultTimeout)

	method := env.runtime.Get("capturedMethod")
	require.NotNil(t, method)
	assert.Equal(t, "/testgrpc.TestService/Echo", method.String())
}

func TestServerInterceptor_AddInterceptorNotAFunction(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var error;
		try {
			var server = grpc.createServer();
			server.addInterceptor('not a function');
		} catch(e) {
			error = e.message;
		}
		__done();
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	require.NotNil(t, errVal)
	assert.Contains(t, errVal.String(), "addInterceptor: argument must be a function")
}

func TestServerInterceptor_Chaining(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// addInterceptor should return server for chaining.
	env.runOnLoop(t, `
		var server = grpc.createServer();
		var returnVal = server.addInterceptor(function(next) {
			return function(call) { return next(call); };
		});
		var isSame = (returnVal === server);
		__done();
	`, defaultTimeout)

	isSame := env.runtime.Get("isSame")
	require.NotNil(t, isSame)
	assert.True(t, isSame.ToBoolean())
}

func TestServerInterceptor_ServerStream(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var interceptorCalled = false;
		var server = grpc.createServer();
		server.addInterceptor(function(next) {
			return function(call) {
				interceptorCalled = true;
				var auth = call.requestHeader.get('x-auth');
				if (!auth) {
					throw grpc.status.createError(16, 'unauthenticated');
				}
				return next(call);
			};
		});
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

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test');

		// Without auth — should fail.
		var errorCode;
		var items = [];
		client.serverStream(req).then(function(stream) {
			return stream.recv();
		}).catch(function(err) {
			errorCode = err.code;

			// With auth — should succeed.
			var md = grpc.metadata.create();
			md.set('x-auth', 'valid');
			return client.serverStream(req, { metadata: md });
		}).then(function(stream) {
			return stream.recv().then(function(r) {
				if (!r.done) items.push(r.value.get('name'));
				return stream.recv();
			});
		}).then(function() {
			__done();
		}).catch(function(err) {
			__done();
		});
	`, defaultTimeout)

	ec := env.runtime.Get("errorCode")
	require.NotNil(t, ec)
	assert.Equal(t, int64(16), ec.ToInteger())

	assert.True(t, env.runtime.Get("interceptorCalled").ToBoolean())

	items := env.runtime.Get("items")
	require.NotNil(t, items)
	exported := items.Export().([]interface{})
	require.Len(t, exported, 1)
	assert.Equal(t, "test", exported[0])
}

func TestServerInterceptor_RequestFieldAccess(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var capturedMsg;
		var server = grpc.createServer();
		server.addInterceptor(function(next) {
			return function(call) {
				// For unary RPCs, call.request is the request message.
				if (call.request) {
					capturedMsg = call.request.get('message');
				}
				return next(call);
			};
		});
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'echo:' + request.get('message'));
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
		req.set('message', 'intercepted-value');

		var result;
		client.echo(req).then(function(resp) {
			result = resp.get('message');
			__done();
		}).catch(function(err) { __done(); });
	`, defaultTimeout)

	capturedMsg := env.runtime.Get("capturedMsg")
	require.NotNil(t, capturedMsg)
	assert.Equal(t, "intercepted-value", capturedMsg.String())

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	assert.Equal(t, "echo:intercepted-value", result.String())
}
