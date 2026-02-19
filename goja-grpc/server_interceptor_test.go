package gojagrpc

import (
	"testing"
	"strings"

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
	if err1 == nil {
		t.Fatalf("expected non-nil")
	}
	if !strings.Contains(err1.String(), "16:") {
		t.Errorf("expected %q to contain %q", err1.String(), "16:")
	}
	if !strings.Contains(err1.String(), "unauthenticated") {
		t.Errorf("expected %q to contain %q", err1.String(), "unauthenticated")
	}

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	if got := result.String(); got != "ok:hello" {
		t.Errorf("expected %v, got %v", "ok:hello", got)
	}
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
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	if got := result.String(); got != "logged" {
		t.Errorf("expected %v, got %v", "logged", got)
	}

	logs := env.runtime.Get("logs")
	if logs == nil {
		t.Fatalf("expected non-nil")
	}
	exported := logs.Export().([]any)
	if got := len(exported); got != 2 {
		t.Fatalf("expected len %d, got %d", 2, got)
	}
	if got := exported[0]; got != "before:/testgrpc.TestService/Echo" {
		t.Errorf("expected %v, got %v", "before:/testgrpc.TestService/Echo", got)
	}
	if got := exported[1]; got != "after:/testgrpc.TestService/Echo" {
		t.Errorf("expected %v, got %v", "after:/testgrpc.TestService/Echo", got)
	}
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
	if order == nil {
		t.Fatalf("expected non-nil")
	}
	exported := order.Export().([]any)
	// Onion pattern: first-before → second-before → handler → second-after → first-after
	if got := len(exported); got != 5 {
		t.Fatalf("expected len %d, got %d", 5, got)
	}
	if got := exported[0]; got != "first-before" {
		t.Errorf("expected %v, got %v", "first-before", got)
	}
	if got := exported[1]; got != "second-before" {
		t.Errorf("expected %v, got %v", "second-before", got)
	}
	if got := exported[2]; got != "handler" {
		t.Errorf("expected %v, got %v", "handler", got)
	}
	if got := exported[3]; got != "second-after" {
		t.Errorf("expected %v, got %v", "second-after", got)
	}
	if got := exported[4]; got != "first-after" {
		t.Errorf("expected %v, got %v", "first-after", got)
	}
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
	if hdr == nil {
		t.Fatalf("expected non-nil")
	}
	if got := hdr.String(); got != "was-here" {
		t.Errorf("expected %v, got %v", "was-here", got)
	}
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
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	if !strings.Contains(result.String(), "7:") {
		t.Errorf("expected %q to contain %q", result.String(), "7:")
	}
	if !strings.Contains(result.String(), "access denied") {
		t.Errorf("expected %q to contain %q", result.String(), "access denied")
	}
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
	if method == nil {
		t.Fatalf("expected non-nil")
	}
	if got := method.String(); got != "/testgrpc.TestService/Echo" {
		t.Errorf("expected %v, got %v", "/testgrpc.TestService/Echo", got)
	}
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
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if !strings.Contains(errVal.String(), "addInterceptor: argument must be a function") {
		t.Errorf("expected %q to contain %q", errVal.String(), "addInterceptor: argument must be a function")
	}
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
	if isSame == nil {
		t.Fatalf("expected non-nil")
	}
	if !(isSame.ToBoolean()) {
		t.Errorf("expected true")
	}
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
	if ec == nil {
		t.Fatalf("expected non-nil")
	}
	if got := ec.ToInteger(); got != int64(16) {
		t.Errorf("expected %v, got %v", int64(16), got)
	}

	if !(env.runtime.Get("interceptorCalled").ToBoolean()) {
		t.Errorf("expected true")
	}

	items := env.runtime.Get("items")
	if items == nil {
		t.Fatalf("expected non-nil")
	}
	exported := items.Export().([]any)
	if got := len(exported); got != 1 {
		t.Fatalf("expected len %d, got %d", 1, got)
	}
	if got := exported[0]; got != "test" {
		t.Errorf("expected %v, got %v", "test", got)
	}
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
	if capturedMsg == nil {
		t.Fatalf("expected non-nil")
	}
	if got := capturedMsg.String(); got != "intercepted-value" {
		t.Errorf("expected %v, got %v", "intercepted-value", got)
	}

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	if got := result.String(); got != "echo:intercepted-value" {
		t.Errorf("expected %v, got %v", "echo:intercepted-value", got)
	}
}
