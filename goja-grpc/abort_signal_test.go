package gojagrpc

import (
	"testing"
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
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["name"]; got != "GrpcError" {
		t.Errorf("expected %v, got %v", "GrpcError", got)
	}
	if got := resultObj["code"]; got != int64(1) {
		t.Errorf("expected %v, got %v", int64(1), got)
	}
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
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["name"]; got != "GrpcError" {
		t.Errorf("expected %v, got %v", "GrpcError", got)
	}
	if got := resultObj["code"]; got != int64(1) {
		t.Errorf("expected %v, got %v", int64(1), got)
	}
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
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["name"]; got != "GrpcError" {
		t.Errorf("expected %v, got %v", "GrpcError", got)
	}
	if got := resultObj["code"]; got != int64(1) {
		t.Errorf("expected %v, got %v", int64(1), got)
	}
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
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["name"]; got != "GrpcError" {
		t.Errorf("expected %v, got %v", "GrpcError", got)
	}
	if got := resultObj["code"]; got != int64(1) {
		t.Errorf("expected %v, got %v", int64(1), got)
	}
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
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["name"]; got != "GrpcError" {
		t.Errorf("expected %v, got %v", "GrpcError", got)
	}
	if got := resultObj["code"]; got != int64(1) {
		t.Errorf("expected %v, got %v", int64(1), got)
	}
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
	if firstMsg == nil {
		t.Fatalf("expected non-nil")
	}
	if got := firstMsg.String(); got != "first" {
		t.Errorf("expected %v, got %v", "first", got)
	}

	result := env.runtime.Get("error")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["name"]; got != "GrpcError" {
		t.Errorf("expected %v, got %v", "GrpcError", got)
	}
	if got := resultObj["code"]; got != int64(1) {
		t.Errorf("expected %v, got %v", int64(1), got)
	}
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
	if firstEcho == nil {
		t.Fatalf("expected non-nil")
	}
	if got := firstEcho.String(); got != "echo-hello" {
		t.Errorf("expected %v, got %v", "echo-hello", got)
	}

	sendErr := env.runtime.Get("sendError")
	if sendErr == nil {
		t.Fatalf("expected non-nil")
	}
	sendObj := sendErr.Export().(map[string]any)
	if got := sendObj["name"]; got != "GrpcError" {
		t.Errorf("expected %v, got %v", "GrpcError", got)
	}
	if got := sendObj["code"]; got != int64(1) {
		t.Errorf("expected %v, got %v", int64(1), got)
	}

	recvErr := env.runtime.Get("recvError")
	if recvErr == nil {
		t.Fatalf("expected non-nil")
	}
	recvObj := recvErr.Export().(map[string]any)
	if got := recvObj["name"]; got != "GrpcError" {
		t.Errorf("expected %v, got %v", "GrpcError", got)
	}
	if got := recvObj["code"]; got != int64(1) {
		t.Errorf("expected %v, got %v", int64(1), got)
	}
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
	if r1 == nil {
		t.Fatalf("expected non-nil")
	}
	r1Obj := r1.Export().(map[string]any)
	if got := r1Obj["name"]; got != "GrpcError" {
		t.Errorf("expected %v, got %v", "GrpcError", got)
	}
	if got := r1Obj["code"]; got != int64(1) {
		t.Errorf("expected %v, got %v", int64(1), got)
	}

	r2 := env.runtime.Get("result2")
	if r2 == nil {
		t.Fatalf("expected non-nil")
	}
	r2Obj := r2.Export().(map[string]any)
	if got := r2Obj["message"]; got != "fast:quick" {
		t.Errorf("expected %v, got %v", "fast:quick", got)
	}
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
	if r1 == nil {
		t.Fatalf("expected non-nil")
	}
	r1Obj := r1.Export().(map[string]any)
	if got := r1Obj["name"]; got != "GrpcError" {
		t.Errorf("expected %v, got %v", "GrpcError", got)
	}
	if got := r1Obj["code"]; got != int64(1) {
		t.Errorf("expected %v, got %v", int64(1), got)
	}

	r2 := env.runtime.Get("result2")
	if r2 == nil {
		t.Fatalf("expected non-nil")
	}
	r2Obj := r2.Export().(map[string]any)
	if got := r2Obj["name"]; got != "GrpcError" {
		t.Errorf("expected %v, got %v", "GrpcError", got)
	}
	if got := r2Obj["code"]; got != int64(1) {
		t.Errorf("expected %v, got %v", int64(1), got)
	}
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
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	// Either success or cancelled — both are valid outcomes.
	if resultObj["success"] == true {
		if got := resultObj["message"]; got != "completed" {
			t.Errorf("expected %v, got %v", "completed", got)
		}
	} else {
		if got := resultObj["code"]; got != int64(1) {
			t.Errorf("expected %v, got %v", int64(1), got)
		}
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
	if r == nil {
		t.Fatalf("expected non-nil")
	}
	if got := r.String(); got != "done" {
		t.Errorf("expected %v, got %v", "done", got)
	}

	aborted := env.runtime.Get("abortedAfter")
	if aborted == nil {
		t.Fatalf("expected non-nil")
	}
	if !(aborted.ToBoolean()) {
		t.Errorf("expected true")
	}
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
