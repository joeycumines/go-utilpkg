package gojagrpc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// defaultTimeout is the maximum time for event loop operations in tests.
const defaultTimeout = 5 * time.Second

// ============================================================================
// T073: Client unary RPC
// ============================================================================

func TestUnaryRPC_HappyPath(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		// Server: echo handler
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'echo: ' + request.get('message'));
				resp.set('code', 42);
				return resp;
			},
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		// Client: call echo
		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'hello');
		var result;
		var error;
		client.echo(req).then(function(resp) {
			result = { message: resp.get('message'), code: resp.get('code') };
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
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "echo: hello", resultObj["message"])
	assert.Equal(t, int64(42), resultObj["code"])
}

func TestUnaryRPC_ServerError(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				throw grpc.status.createError(grpc.status.NOT_FOUND, 'item not found');
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
	assert.Equal(t, int64(5), resultObj["code"]) // NOT_FOUND = 5
	assert.Contains(t, resultObj["message"], "item not found")
}

func TestUnaryRPC_AsyncHandler(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Server handler returns a Promise (via .then chaining on a resolved value)
	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				return new Promise(function(resolve) {
					var EchoResponse = pb.messageType('testgrpc.EchoResponse');
					var resp = new EchoResponse();
					resp.set('message', 'async: ' + request.get('message'));
					resolve(resp);
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
		req.set('message', 'world');
		var result;
		client.echo(req).then(function(resp) {
			result = resp.get('message');
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	assert.Equal(t, "async: world", result.String())
}

// ============================================================================
// T074: Client server-streaming RPC
// ============================================================================

func TestServerStreamRPC_HappyPath(t *testing.T) {
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
		req.set('message', 'list');
		var items = [];
		var error;
		client.serverStream(req).then(function(stream) {
			function readNext() {
				stream.recv().then(function(result) {
					if (result.done) {
						__done();
						return;
					}
					items.push({ id: result.value.get('id'), name: result.value.get('name') });
					readNext();
				}).catch(function(err) {
					error = err;
					__done();
				});
			}
			readNext();
		}).catch(function(err) {
			error = err;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	items := env.runtime.Get("items")
	require.NotNil(t, items)
	arr := items.Export().([]interface{})
	assert.Equal(t, 3, len(arr))
	assert.Equal(t, "item-0", arr[0].(map[string]interface{})["name"])
	assert.Equal(t, "item-2", arr[2].(map[string]interface{})["name"])
}

// ============================================================================
// T075: Client client-streaming RPC
// ============================================================================

func TestClientStreamRPC_HappyPath(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
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
								resp.set('message', 'received: ' + names.join(','));
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

		var client = grpc.createClient('testgrpc.TestService');
		var Item = pb.messageType('testgrpc.Item');
		var result;
		var error;
		client.clientStream().then(function(call) {
			var item1 = new Item();
			item1.set('id', '1');
			item1.set('name', 'alpha');
			var item2 = new Item();
			item2.set('id', '2');
			item2.set('name', 'beta');
			call.send(item1).then(function() {
				return call.send(item2);
			}).then(function() {
				return call.closeSend();
			}).then(function() {
				return call.response;
			}).then(function(resp) {
				result = { message: resp.get('message'), code: resp.get('code') };
				__done();
			}).catch(function(err) {
				error = err;
				__done();
			});
		}).catch(function(err) {
			error = err;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "received: alpha,beta", resultObj["message"])
	assert.Equal(t, int64(2), resultObj["code"])
}

// ============================================================================
// T076: Client bidi-streaming RPC
// ============================================================================

func TestBidiStreamRPC_HappyPath(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {
				// Echo server: receive items and send them back with modified name
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
							echo.set('name', 'echo-' + result.value.get('name'));
							call.send(echo);
							readLoop();
						}).catch(reject);
					}
					readLoop();
				});
			}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var Item = pb.messageType('testgrpc.Item');
		var received = [];
		var error;
		client.bidiStream().then(function(stream) {
			// Send two items
			var item1 = new Item();
			item1.set('id', '1');
			item1.set('name', 'foo');
			var item2 = new Item();
			item2.set('id', '2');
			item2.set('name', 'bar');
			
			stream.send(item1).then(function() {
				return stream.send(item2);
			}).then(function() {
				return stream.closeSend();
			}).then(function() {
				// Read responses
				function readLoop() {
					stream.recv().then(function(result) {
						if (result.done) {
							__done();
							return;
						}
						received.push({ id: result.value.get('id'), name: result.value.get('name') });
						readLoop();
					}).catch(function(err) {
						error = err;
						__done();
					});
				}
				readLoop();
			});
		}).catch(function(err) {
			error = err;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	received := env.runtime.Get("received")
	require.NotNil(t, received)
	arr := received.Export().([]interface{})
	assert.Equal(t, 2, len(arr))
	assert.Equal(t, "echo-foo", arr[0].(map[string]interface{})["name"])
	assert.Equal(t, "echo-bar", arr[1].(map[string]interface{})["name"])
}

// ============================================================================
// T073 continued: AbortSignal cancellation
// ============================================================================

func TestUnaryRPC_AbortSignal(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				// Return a promise that will never resolve (simulates slow handler).
				// The AbortSignal should cancel the context before this resolves.
				return new Promise(function(resolve) {
					// deliberate: never resolve
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
		req.set('message', 'abort-test');

		var controller = new AbortController();
		var error;
		client.echo(req, { signal: controller.signal }).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { name: err.name, code: err.code, message: err.message };
			__done();
		});

		// Abort immediately
		controller.abort();
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	// Cancelled code = 1
	assert.Equal(t, int64(1), resultObj["code"])
}

// ============================================================================
// T074 continued: Server-stream error mid-stream
// ============================================================================

func TestServerStreamRPC_ServerError(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {
				var Item = pb.messageType('testgrpc.Item');
				var item = new Item();
				item.set('id', '0');
				item.set('name', 'before-error');
				call.send(item);
				throw grpc.status.createError(grpc.status.RESOURCE_EXHAUSTED, 'too many');
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
		var error;
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
					error = { name: err.name, code: err.code, message: err.message };
					__done();
				});
			}
			readNext();
		}).catch(function(err) {
			error = { name: err.name, code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	assert.Equal(t, int64(8), resultObj["code"]) // RESOURCE_EXHAUSTED = 8
	assert.Contains(t, resultObj["message"], "too many")
}

// ============================================================================
// T075 continued: Client-stream server error
// ============================================================================

func TestClientStreamRPC_ServerError(t *testing.T) {
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
						throw grpc.status.createError(grpc.status.INVALID_ARGUMENT, 'bad input');
					}).catch(reject);
				});
			},
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var Item = pb.messageType('testgrpc.Item');
		var error;
		client.clientStream().then(function(call) {
			var item = new Item();
			item.set('id', '1');
			item.set('name', 'test');
			call.send(item).then(function() {
				return call.closeSend();
			}).then(function() {
				return call.response;
			}).then(function(resp) {
				error = { unexpected: true };
				__done();
			}).catch(function(err) {
				error = { name: err.name, code: err.code, message: err.message };
				__done();
			});
		}).catch(function(err) {
			error = { name: err.name, code: err.code };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	assert.Equal(t, int64(3), resultObj["code"]) // INVALID_ARGUMENT = 3
	assert.Contains(t, resultObj["message"], "bad input")
}

// ============================================================================
// T076 continued: Bidi-stream server error
// ============================================================================

func TestBidiStreamRPC_ServerError(t *testing.T) {
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
					call.recv().then(function(result) {
						reject(grpc.status.createError(grpc.status.ALREADY_EXISTS, 'dup'));
					});
				});
			}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var Item = pb.messageType('testgrpc.Item');
		var error;
		client.bidiStream().then(function(stream) {
			var item = new Item();
			item.set('id', '1');
			item.set('name', 'test');
			stream.send(item).then(function() {
				return stream.closeSend();
			}).then(function() {
				return stream.recv();
			}).then(function(result) {
				error = { unexpected: true };
				__done();
			}).catch(function(err) {
				error = { name: err.name, code: err.code, message: err.message };
				__done();
			});
		}).catch(function(err) {
			error = { name: err.name, code: err.code };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	assert.Equal(t, int64(6), resultObj["code"]) // ALREADY_EXISTS = 6
	assert.Contains(t, resultObj["message"], "dup")
}

// ============================================================================
// T080: Full JS integration test: JS client + JS server
// ============================================================================

func TestIntegration_JSClientJSServer_AllRPCTypes(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		// ===================== Server Setup ===========================
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			// Unary: echo with prefix
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'reply: ' + request.get('message'));
				resp.set('code', 200);
				return resp;
			},
			// Server-streaming: send N items
			serverStream: function(request, call) {
				var Item = pb.messageType('testgrpc.Item');
				var count = parseInt(request.get('message'), 10) || 2;
				for (var i = 0; i < count; i++) {
					var item = new Item();
					item.set('id', String(i));
					item.set('name', 'stream-' + i);
					call.send(item);
				}
			},
			// Client-streaming: collect items, respond with count
			clientStream: function(call) {
				var count = 0;
				return new Promise(function(resolve, reject) {
					function doRead() {
						call.recv().then(function(result) {
							if (result.done) {
								var EchoResponse = pb.messageType('testgrpc.EchoResponse');
								var resp = new EchoResponse();
								resp.set('message', 'count=' + count);
								resp.set('code', count);
								resolve(resp);
								return;
							}
							count++;
							doRead();
						}).catch(reject);
					}
					doRead();
				});
			},
			// Bidi: echo items with modified names
			bidiStream: function(call) {
				return new Promise(function(resolve, reject) {
					function doRead() {
						call.recv().then(function(result) {
							if (result.done) {
								resolve();
								return;
							}
							var Item = pb.messageType('testgrpc.Item');
							var out = new Item();
							out.set('id', result.value.get('id'));
							out.set('name', 'bidi-' + result.value.get('name'));
							call.send(out);
							doRead();
						}).catch(reject);
					}
					doRead();
				});
			}
		});
		server.start();

		// ===================== Client Tests ===========================
		var client = grpc.createClient('testgrpc.TestService');
		var results = {};
		var errors = [];
		var pending = 4;
		function checkAllDone() {
			pending--;
			if (pending === 0) __done();
		}

		// 1) Unary
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req1 = new EchoRequest();
		req1.set('message', 'integration');
		client.echo(req1).then(function(resp) {
			results.unary = { message: resp.get('message'), code: resp.get('code') };
			checkAllDone();
		}).catch(function(err) { errors.push('unary: ' + err); checkAllDone(); });

		// 2) Server-streaming
		var req2 = new EchoRequest();
		req2.set('message', '3');
		client.serverStream(req2).then(function(stream) {
			results.serverStream = [];
			function readServerStream() {
				stream.recv().then(function(r) {
					if (r.done) {
						checkAllDone();
						return;
					}
					results.serverStream.push(r.value.get('name'));
					readServerStream();
				}).catch(function(err) { errors.push('ss: ' + err); checkAllDone(); });
			}
			readServerStream();
		}).catch(function(err) { errors.push('ss-open: ' + err); checkAllDone(); });

		// 3) Client-streaming
		var Item = pb.messageType('testgrpc.Item');
		client.clientStream().then(function(call) {
			var a = new Item();
			a.set('id', '1'); a.set('name', 'a');
			var b = new Item();
			b.set('id', '2'); b.set('name', 'b');
			var c = new Item();
			c.set('id', '3'); c.set('name', 'c');
			call.send(a).then(function() {
				return call.send(b);
			}).then(function() {
				return call.send(c);
			}).then(function() {
				return call.closeSend();
			}).then(function() {
				return call.response;
			}).then(function(resp) {
				results.clientStream = { message: resp.get('message'), code: resp.get('code') };
				checkAllDone();
			}).catch(function(err) { errors.push('cs: ' + err); checkAllDone(); });
		}).catch(function(err) { errors.push('cs-open: ' + err); checkAllDone(); });

		// 4) Bidi-streaming
		client.bidiStream().then(function(stream) {
			results.bidi = [];
			var x = new Item();
			x.set('id', '1'); x.set('name', 'x');
			var y = new Item();
			y.set('id', '2'); y.set('name', 'y');
			stream.send(x).then(function() {
				return stream.send(y);
			}).then(function() {
				return stream.closeSend();
			}).then(function() {
				function readBidi() {
					stream.recv().then(function(r) {
						if (r.done) {
							checkAllDone();
							return;
						}
						results.bidi.push(r.value.get('name'));
						readBidi();
					}).catch(function(err) { errors.push('bidi: ' + err); checkAllDone(); });
				}
				readBidi();
			});
		}).catch(function(err) { errors.push('bidi-open: ' + err); checkAllDone(); });
	`, defaultTimeout)

	// Check for errors
	errorsVal := env.runtime.Get("errors")
	if errorsVal != nil && !isGojaUndefined(errorsVal) {
		errArr := errorsVal.Export().([]interface{})
		require.Empty(t, errArr, "JS errors: %v", errArr)
	}

	results := env.runtime.Get("results")
	require.NotNil(t, results)
	r := results.Export().(map[string]interface{})

	// 1) Unary
	unary := r["unary"].(map[string]interface{})
	assert.Equal(t, "reply: integration", unary["message"])
	assert.Equal(t, int64(200), unary["code"])

	// 2) Server-streaming
	ss := r["serverStream"].([]interface{})
	assert.Equal(t, 3, len(ss))
	assert.Equal(t, "stream-0", ss[0])
	assert.Equal(t, "stream-2", ss[2])

	// 3) Client-streaming
	cs := r["clientStream"].(map[string]interface{})
	assert.Equal(t, "count=3", cs["message"])
	assert.Equal(t, int64(3), cs["code"])

	// 4) Bidi-streaming
	bidi := r["bidi"].([]interface{})
	assert.Equal(t, 2, len(bidi))
	assert.Equal(t, "bidi-x", bidi[0])
	assert.Equal(t, "bidi-y", bidi[1])
}

// ============================================================================
// Helpers
// ============================================================================

func isGojaUndefined(v interface{}) bool {
	if v == nil {
		return true
	}
	s, ok := v.(interface{ String() string })
	if ok && s.String() == "undefined" {
		return true
	}
	return false
}
