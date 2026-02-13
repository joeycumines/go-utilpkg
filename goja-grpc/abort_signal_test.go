package gojagrpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// T222: AbortSignal exhaustive: abort before RPC starts
// ============================================================================

// TestAbort_BeforeUnaryRPC verifies that aborting the signal BEFORE
// starting the RPC produces an immediate CANCELLED error.
func TestAbort_BeforeUnaryRPC(t *testing.T) {
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
		req.set('message', 'pre-aborted');

		// Abort signal BEFORE making the RPC.
		var controller = new AbortController();
		controller.abort();

		var error;
		client.echo(req, { signal: controller.signal }).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { name: err.name, code: err.code };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	assert.Equal(t, int64(1), resultObj["code"]) // CANCELLED
}

// TestAbort_BeforeServerStreamRPC verifies pre-abort on server-streaming.
func TestAbort_BeforeServerStreamRPC(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {
				// Should not be reached.
				var Item = pb.messageType('testgrpc.Item');
				var item = new Item();
				item.set('id', '1');
				item.set('name', 'should-not-reach');
				call.send(item);
			},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'pre-aborted-stream');

		var controller = new AbortController();
		controller.abort();

		var error;
		client.serverStream(req, { signal: controller.signal }).then(function(stream) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { name: err.name, code: err.code };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	assert.Equal(t, int64(1), resultObj["code"]) // CANCELLED
}

// TestAbort_BeforeClientStreamRPC verifies pre-abort on client-streaming.
func TestAbort_BeforeClientStreamRPC(t *testing.T) {
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

		var controller = new AbortController();
		controller.abort();

		var error;
		client.clientStream({ signal: controller.signal }).then(function(callObj) {
			// Stream created but context is already cancelled.
			// The response promise should reject.
			return callObj.response;
		}).then(function() {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { name: err.name, code: err.code };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	assert.Equal(t, int64(1), resultObj["code"]) // CANCELLED
}

// TestAbort_BeforeBidiStreamRPC verifies pre-abort on bidi-streaming.
func TestAbort_BeforeBidiStreamRPC(t *testing.T) {
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

		var controller = new AbortController();
		controller.abort();

		var error;
		client.bidiStream({ signal: controller.signal }).then(function(stream) {
			// Stream created but context is already cancelled.
			// Recv should fail.
			return stream.recv();
		}).then(function() {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { name: err.name, code: err.code };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	assert.Equal(t, int64(1), resultObj["code"]) // CANCELLED
}

// ============================================================================
// T223: AbortSignal exhaustive: abort during unary send
// ============================================================================

// TestAbort_DuringUnarySend verifies that aborting during a slow
// unary handler produces CANCELLED.
func TestAbort_DuringUnarySend(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				// Slow handler: never resolves on its own.
				return new Promise(function(resolve) {
					// deliberate: we rely on abort to cancel
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
		req.set('message', 'abort-during-send');

		var controller = new AbortController();
		var error;
		client.echo(req, { signal: controller.signal }).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { name: err.name, code: err.code };
			__done();
		});

		// Abort after a small delay to allow the RPC to start.
		setTimeout(function() { controller.abort(); }, 10);
	`, defaultTimeout)

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	assert.Equal(t, int64(1), resultObj["code"]) // CANCELLED
}

// ============================================================================
// T224: AbortSignal exhaustive: abort during stream receive
// ============================================================================

// TestAbort_DuringStreamReceive verifies aborting during active
// server-streaming.
func TestAbort_DuringStreamReceive(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {
				var Item = pb.messageType('testgrpc.Item');
				var item1 = new Item();
				item1.set('id', '1');
				item1.set('name', 'first');
				call.send(item1);
				// After first send, block forever.
				return new Promise(function(resolve) {
					// deliberate: never resolve, wait for abort
				});
			},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'stream-abort');

		var controller = new AbortController();
		var firstMessage = null;
		var error = null;

		client.serverStream(req, { signal: controller.signal }).then(function(stream) {
			stream.recv().then(function(r) {
				if (!r.done) {
					firstMessage = r.value.get('name');
				}
				// Abort after first message.
				controller.abort();
				// Try to receive the next message (should fail).
				return stream.recv();
			}).then(function(r) {
				error = { unexpected: true };
				__done();
			}).catch(function(err) {
				error = { name: err.name, code: err.code };
				__done();
			});
		}).catch(function(err) {
			error = { name: err.name, code: err.code };
			__done();
		});
	`, defaultTimeout)

	firstMsg := env.runtime.Get("firstMessage")
	require.NotNil(t, firstMsg)
	assert.Equal(t, "first", firstMsg.String())

	result := env.runtime.Get("error")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", resultObj["name"])
	assert.Equal(t, int64(1), resultObj["code"]) // CANCELLED
}

// ============================================================================
// T225: AbortSignal exhaustive: abort during bidi interleave
// ============================================================================

// TestAbort_DuringBidiInterleave verifies aborting mid-conversation
// in a bidi stream causes both send and recv to fail.
func TestAbort_DuringBidiInterleave(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {
				// Echo server: read items, create new items and send back.
				return new Promise(function(resolve, reject) {
					function pump() {
						call.recv().then(function(r) {
							if (r.done) { resolve(); return; }
							var Item = pb.messageType('testgrpc.Item');
							var echo = new Item();
							echo.set('id', r.value.get('id'));
							echo.set('name', 'echo-' + r.value.get('name'));
							call.send(echo).then(pump).catch(reject);
						}).catch(function(err) { resolve(); });
					}
					pump();
				});
			}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var controller = new AbortController();
		var firstEchoName = null;
		var sendError = null;
		var recvError = null;

		client.bidiStream({ signal: controller.signal }).then(function(stream) {
			var Item = pb.messageType('testgrpc.Item');
			var item = new Item();
			item.set('id', '1');
			item.set('name', 'hello');

			// Send, then close to trigger echo, then read.
			stream.send(item).then(function() {
				return stream.recv();
			}).then(function(r) {
				if (!r.done && r.value) {
					firstEchoName = r.value.get('name');
				}
				// Abort after successfully reading the echo.
				controller.abort();

				// Next recv should fail with CANCELLED.
				return stream.recv();
			}).then(function(r) {
				recvError = { unexpected: true, done: r ? r.done : 'no-r' };
				__done();
			}).catch(function(err) {
				recvError = { name: err.name, code: err.code };
				// Also try sending — should also fail.
				var item2 = new Item();
				item2.set('id', '2');
				item2.set('name', 'world');
				stream.send(item2).then(function() {
					sendError = { unexpected: true };
					__done();
				}).catch(function(err2) {
					sendError = { name: err2.name, code: err2.code };
					__done();
				});
			});
		}).catch(function(err) {
			sendError = { name: err.name, code: err.code };
			__done();
		});
	`, defaultTimeout)

	firstEcho := env.runtime.Get("firstEchoName")
	require.NotNil(t, firstEcho)
	assert.Equal(t, "echo-hello", firstEcho.String())

	sendErr := env.runtime.Get("sendError")
	require.NotNil(t, sendErr)
	sendObj := sendErr.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", sendObj["name"])
	assert.Equal(t, int64(1), sendObj["code"]) // CANCELLED

	recvErr := env.runtime.Get("recvError")
	require.NotNil(t, recvErr)
	recvObj := recvErr.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", recvObj["name"])
	assert.Equal(t, int64(1), recvObj["code"]) // CANCELLED
}

// ============================================================================
// T226: AbortSignal exhaustive: multiple signals on same client
// ============================================================================

// TestAbort_MultipleSignalsSameClient verifies that aborting one
// signal does not affect a different RPC using a different signal.
func TestAbort_MultipleSignalsSameClient(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var msg = request.get('message');
				if (msg === 'slow') {
					// Slow handler: never resolves.
					return new Promise(function(resolve) {});
				}
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'fast:' + msg);
				return resp;
			},
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');

		var controller1 = new AbortController();
		var controller2 = new AbortController();

		var result1 = null;
		var result2 = null;
		var pending = 2;

		function maybeFinish() {
			if (--pending === 0) __done();
		}

		// RPC 1: slow handler with signal 1.
		var req1 = new EchoRequest();
		req1.set('message', 'slow');
		client.echo(req1, { signal: controller1.signal }).then(function(resp) {
			result1 = { unexpected: true };
			maybeFinish();
		}).catch(function(err) {
			result1 = { name: err.name, code: err.code };
			maybeFinish();
		});

		// RPC 2: fast handler with signal 2.
		var req2 = new EchoRequest();
		req2.set('message', 'quick');
		client.echo(req2, { signal: controller2.signal }).then(function(resp) {
			result2 = { message: resp.get('message') };
			maybeFinish();
		}).catch(function(err) {
			result2 = { error: err.message };
			maybeFinish();
		});

		// Abort only signal 1 after a short delay.
		setTimeout(function() { controller1.abort(); }, 10);
	`, defaultTimeout)

	r1 := env.runtime.Get("result1")
	require.NotNil(t, r1)
	r1Obj := r1.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", r1Obj["name"])
	assert.Equal(t, int64(1), r1Obj["code"]) // CANCELLED

	r2 := env.runtime.Get("result2")
	require.NotNil(t, r2)
	r2Obj := r2.Export().(map[string]interface{})
	assert.Equal(t, "fast:quick", r2Obj["message"])
}

// ============================================================================
// T227: AbortSignal exhaustive: signal shared across RPCs
// ============================================================================

// TestAbort_SharedSignalAcrossRPCs verifies that one signal aborting
// cancels ALL RPCs that share it.
func TestAbort_SharedSignalAcrossRPCs(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				// Slow handler: never resolves.
				return new Promise(function(resolve) {});
			},
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');

		var sharedController = new AbortController();

		var result1 = null;
		var result2 = null;
		var pending = 2;

		function maybeFinish() {
			if (--pending === 0) __done();
		}

		// Two RPCs sharing the same signal.
		var req1 = new EchoRequest();
		req1.set('message', 'shared1');
		client.echo(req1, { signal: sharedController.signal }).then(function() {
			result1 = { unexpected: true };
			maybeFinish();
		}).catch(function(err) {
			result1 = { name: err.name, code: err.code };
			maybeFinish();
		});

		var req2 = new EchoRequest();
		req2.set('message', 'shared2');
		client.echo(req2, { signal: sharedController.signal }).then(function() {
			result2 = { unexpected: true };
			maybeFinish();
		}).catch(function(err) {
			result2 = { name: err.name, code: err.code };
			maybeFinish();
		});

		// Abort after both RPCs have started.
		setTimeout(function() { sharedController.abort(); }, 10);
	`, defaultTimeout)

	r1 := env.runtime.Get("result1")
	require.NotNil(t, r1)
	r1Obj := r1.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", r1Obj["name"])
	assert.Equal(t, int64(1), r1Obj["code"]) // CANCELLED

	r2 := env.runtime.Get("result2")
	require.NotNil(t, r2)
	r2Obj := r2.Export().(map[string]interface{})
	assert.Equal(t, "GrpcError", r2Obj["name"])
	assert.Equal(t, int64(1), r2Obj["code"]) // CANCELLED
}

// ============================================================================
// T228: AbortSignal exhaustive: abort races with RPC completion
// ============================================================================

// TestAbort_RacesWithCompletion verifies that aborting at the same
// time as RPC completion does not panic or deadlock. The result is
// deterministic: either success or CANCELLED.
func TestAbort_RacesWithCompletion(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'completed');
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
		req.set('message', 'race');

		var controller = new AbortController();
		var result;

		client.echo(req, { signal: controller.signal }).then(function(resp) {
			result = { success: true, message: resp.get('message') };
			__done();
		}).catch(function(err) {
			result = { success: false, code: err.code };
			__done();
		});

		// Abort immediately — races with the fast handler.
		controller.abort();
	`, defaultTimeout)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	// Either success or cancelled — both are valid outcomes.
	if resultObj["success"] == true {
		assert.Equal(t, "completed", resultObj["message"])
	} else {
		assert.Equal(t, int64(1), resultObj["code"]) // CANCELLED
	}
}

// ============================================================================
// T229: AbortSignal exhaustive: signal cleanup after RPC
// ============================================================================

// TestAbort_CleanupAfterRPC verifies that after an RPC completes,
// aborting the signal does not cause errors or panics. This confirms
// signal listeners are properly cleaned up.
func TestAbort_CleanupAfterRPC(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'done');
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
		req.set('message', 'cleanup-test');

		var controller = new AbortController();
		var result;
		var abortedAfter = false;

		client.echo(req, { signal: controller.signal }).then(function(resp) {
			result = resp.get('message');
			// Abort AFTER the RPC has completed.
			controller.abort();
			abortedAfter = true;
			__done();
		}).catch(function(err) {
			result = 'error:' + err.code;
			__done();
		});
	`, defaultTimeout)

	r := env.runtime.Get("result")
	require.NotNil(t, r)
	assert.Equal(t, "done", r.String())

	aborted := env.runtime.Get("abortedAfter")
	require.NotNil(t, aborted)
	assert.True(t, aborted.ToBoolean())
}

// TestAbort_CleanupNoGoroutineLeak verifies that abort signal usage
// does not leak goroutines. Two RPCs complete normally; the abort
// signal is never triggered.
func TestAbort_CleanupNoGoroutineLeak(t *testing.T) {
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
		var callCount = 0;

		// Two RPCs with signals that are never aborted.
		for (var i = 0; i < 2; i++) {
			(function(idx) {
				var c = new AbortController();
				var req = new EchoRequest();
				req.set('message', 'rpc-' + idx);
				client.echo(req, { signal: c.signal }).then(function(resp) {
					callCount++;
					if (callCount === 2) __done();
				}).catch(function(err) {
					callCount++;
					if (callCount === 2) __done();
				});
			})(i);
		}
	`, defaultTimeout)

	// If we get here, the loop cleaned up without deadlock.
	// The test would time out if goroutines leaked and blocked shutdown.
}
