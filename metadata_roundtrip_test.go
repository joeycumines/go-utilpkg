package gojagrpc

import (
	"strings"
	"testing"
)

// ============================================================================
// T166/T169: Server metadata sending + Client header/trailer access
// Tests cover all 4 RPC types with full metadata round-trip.
// ============================================================================

// -------------- Unary RPC: header/trailer round-trip ----------------

func TestUnaryRPC_HeaderTrailerCallbacks(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		// Server: set response headers and trailers, read request header.
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				// Read incoming request header.
				var auth = call.requestHeader.get('x-auth');

				// Set response header and trailer.
				var hd = grpc.metadata.create();
				hd.set('x-response-id', '42');
				hd.set('x-echo-auth', auth || 'none');
				call.setHeader(hd);

				var tr = grpc.metadata.create();
				tr.set('x-checksum', 'abc123');
				call.setTrailer(tr);

				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'echo: ' + request.get('message'));
				return resp;
			},
			serverStream: function(request, call) { call.send(request); },
			clientStream: function(call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				return new EchoResponse();
			},
			bidiStream: function(call) {}
		});
		server.start();

		// Client: call with onHeader/onTrailer callbacks and metadata.
		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'hi');

		var receivedHeaders;
		var receivedTrailers;
		var result;
		var error;

		var outMd = grpc.metadata.create();
		outMd.set('x-auth', 'bearer-xyz');

		client.echo(req, {
			metadata: outMd,
			onHeader: function(headers) {
				receivedHeaders = headers.toObject();
			},
			onTrailer: function(trailers) {
				receivedTrailers = trailers.toObject();
			}
		}).then(function(resp) {
			result = resp.get('message');
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

	// Verify response message.
	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	if got := result.String(); got != "echo: hi" {
		t.Errorf("expected %v, got %v", "echo: hi", got)
	}

	// Verify headers received.
	hdrs := env.runtime.Get("receivedHeaders")
	if hdrs == nil {
		t.Fatalf("expected non-nil")
	}
	hdrsObj := hdrs.Export().(map[string]any)
	assertMetadataContains(t, hdrsObj, "x-response-id", "42")
	assertMetadataContains(t, hdrsObj, "x-echo-auth", "bearer-xyz")

	// Verify trailers received.
	trls := env.runtime.Get("receivedTrailers")
	if trls == nil {
		t.Fatalf("expected non-nil")
	}
	trlsObj := trls.Export().(map[string]any)
	assertMetadataContains(t, trlsObj, "x-checksum", "abc123")
}

func TestUnaryRPC_HeaderTrailerOnError(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				// Set trailer even on error.
				var tr = grpc.metadata.create();
				tr.set('x-error-trace', 'trace-001');
				call.setTrailer(tr);

				throw grpc.status.createError(grpc.status.INVALID_ARGUMENT, 'bad input');
			},
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'x');

		var receivedHeaders;
		var receivedTrailers;
		var error;

		client.echo(req, {
			onHeader: function(headers) {
				receivedHeaders = headers.toObject();
			},
			onTrailer: function(trailers) {
				receivedTrailers = trailers.toObject();
			}
		}).then(function(resp) {
			__done();
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	errObj := env.runtime.Get("error")
	if errObj == nil {
		t.Fatalf("expected non-nil")
	}
	exported := errObj.Export().(map[string]any)
	if got := exported["code"]; got != int64(3) {
		t.Errorf("expected %v, got %v", int64(3), got)
	}

	// Trailers should still be received even on error.
	trls := env.runtime.Get("receivedTrailers")
	if trls == nil {
		t.Fatalf("expected non-nil")
	}
	trlsObj := trls.Export().(map[string]any)
	assertMetadataContains(t, trlsObj, "x-error-trace", "trace-001")
}

func TestUnaryRPC_NoCallbacks(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Verify that omitting callbacks still works (backward compatibility).
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
		client.echo(req).then(function(resp) {
			result = resp.get('message');
			__done();
		}).catch(function(err) { __done(); });
	`, defaultTimeout)

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	if got := result.String(); got != "ok" {
		t.Errorf("expected %v, got %v", "ok", got)
	}
}

// -------------- Server-Streaming: header/trailer round-trip ----------------

func TestServerStream_HeaderTrailerCallbacks(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {
				var hd = grpc.metadata.create();
				hd.set('x-stream-type', 'server');
				call.setHeader(hd);
				call.sendHeader();

				var tr = grpc.metadata.create();
				tr.set('x-item-count', '2');
				call.setTrailer(tr);

				var Item = pb.messageType('testgrpc.Item');
				var item1 = new Item();
				item1.set('id', '1');
				item1.set('name', 'first');
				call.send(item1);

				var item2 = new Item();
				item2.set('id', '2');
				item2.set('name', 'second');
				call.send(item2);
			},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'list');

		var receivedHeaders;
		var receivedTrailers;
		var items = [];

		client.serverStream(req, {
			onHeader: function(headers) {
				receivedHeaders = headers.toObject();
			},
			onTrailer: function(trailers) {
				receivedTrailers = trailers.toObject();
			}
		}).then(function(stream) {
			function readNext() {
				stream.recv().then(function(r) {
					if (r.done) {
						__done();
						return;
					}
					items.push({ id: r.value.get('id'), name: r.value.get('name') });
					readNext();
				}).catch(function(err) { __done(); });
			}
			readNext();
		}).catch(function(err) { __done(); });
	`, defaultTimeout)

	// Verify items received.
	itemsVal := env.runtime.Get("items")
	if itemsVal == nil {
		t.Fatalf("expected non-nil")
	}
	itemsExport := itemsVal.Export().([]any)
	if got := len(itemsExport); got != 2 {
		t.Errorf("expected len %d, got %d", 2, got)
	}

	// Verify headers.
	hdrs := env.runtime.Get("receivedHeaders")
	if hdrs == nil {
		t.Fatalf("expected non-nil")
	}
	hdrsObj := hdrs.Export().(map[string]any)
	assertMetadataContains(t, hdrsObj, "x-stream-type", "server")

	// Verify trailers.
	trls := env.runtime.Get("receivedTrailers")
	if trls == nil {
		t.Fatalf("expected non-nil")
	}
	trlsObj := trls.Export().(map[string]any)
	assertMetadataContains(t, trlsObj, "x-item-count", "2")
}

// -------------- Client-Streaming: header/trailer round-trip ----------------

func TestClientStream_HeaderTrailerCallbacks(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) {
				var hd = grpc.metadata.create();
				hd.set('x-stream-type', 'client');
				call.setHeader(hd);

				var tr = grpc.metadata.create();
				tr.set('x-received', 'true');
				call.setTrailer(tr);

				var count = 0;
				return new Promise(function(resolve) {
					function readNext() {
						call.recv().then(function(r) {
							if (r.done) {
								var EchoResponse = pb.messageType('testgrpc.EchoResponse');
								var resp = new EchoResponse();
								resp.set('message', 'received ' + count);
								resolve(resp);
								return;
							}
							count++;
							readNext();
						});
					}
					readNext();
				});
			},
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');

		var receivedHeaders;
		var receivedTrailers;
		var result;

		client.clientStream({
			onHeader: function(headers) {
				receivedHeaders = headers.toObject();
			},
			onTrailer: function(trailers) {
				receivedTrailers = trailers.toObject();
			}
		}).then(function(callObj) {
			var Item = pb.messageType('testgrpc.Item');
			var item1 = new Item();
			item1.set('id', '1');
			item1.set('name', 'a');
			callObj.send(item1).then(function() {
				var item2 = new Item();
				item2.set('id', '2');
				item2.set('name', 'b');
				return callObj.send(item2);
			}).then(function() {
				return callObj.closeSend();
			}).then(function() {
				return callObj.response;
			}).then(function(resp) {
				result = resp.get('message');
				// Allow one more event-loop cycle so the async header
				// goroutine Submit is processed before __done().
				return new Promise(function(resolve) { setTimeout(resolve, 0); });
			}).then(function() {
				__done();
			}).catch(function(err) { __done(); });
		}).catch(function(err) { __done(); });
	`, defaultTimeout)

	resultVal := env.runtime.Get("result")
	if resultVal == nil {
		t.Fatalf("expected non-nil")
	}
	if got := resultVal.String(); got != "received 2" {
		t.Errorf("expected %v, got %v", "received 2", got)
	}

	// Verify headers.
	hdrs := env.runtime.Get("receivedHeaders")
	if hdrs == nil {
		t.Fatalf("expected non-nil")
	}
	hdrsObj := hdrs.Export().(map[string]any)
	assertMetadataContains(t, hdrsObj, "x-stream-type", "client")

	// Verify trailers.
	trls := env.runtime.Get("receivedTrailers")
	if trls == nil {
		t.Fatalf("expected non-nil")
	}
	trlsObj := trls.Export().(map[string]any)
	assertMetadataContains(t, trlsObj, "x-received", "true")
}

// -------------- Bidi-Streaming: header/trailer round-trip ----------------

func TestBidiStream_HeaderTrailerCallbacks(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {
				var hd = grpc.metadata.create();
				hd.set('x-stream-type', 'bidi');
				call.setHeader(hd);
				call.sendHeader();

				var tr = grpc.metadata.create();
				tr.set('x-bidi-done', 'yes');
				call.setTrailer(tr);

				// Return a Promise so the handler doesn't finish prematurely.
				return new Promise(function(resolve) {
					function readAndEcho() {
						call.recv().then(function(r) {
							if (r.done) { resolve(); return; }
							call.send(r.value);
							readAndEcho();
						});
					}
					readAndEcho();
				});
			}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var receivedHeaders;
		var receivedTrailers;
		var items = [];

		client.bidiStream({
			onHeader: function(headers) {
				receivedHeaders = headers.toObject();
			},
			onTrailer: function(trailers) {
				receivedTrailers = trailers.toObject();
			}
		}).then(function(stream) {
			var Item = pb.messageType('testgrpc.Item');
			var item = new Item();
			item.set('id', '1');
			item.set('name', 'hello');
			stream.send(item).then(function() {
				return stream.closeSend();
			}).then(function() {
				function readAll() {
					stream.recv().then(function(r) {
						if (r.done) {
							__done();
							return;
						}
						items.push({ id: r.value.get('id'), name: r.value.get('name') });
						readAll();
					}).catch(function(err) { __done(); });
				}
				readAll();
			});
		}).catch(function(err) { __done(); });
	`, defaultTimeout)

	// Verify items.
	itemsVal := env.runtime.Get("items")
	if itemsVal == nil {
		t.Fatalf("expected non-nil")
	}
	itemsExport := itemsVal.Export().([]any)
	if got := len(itemsExport); got != 1 {
		t.Errorf("expected len %d, got %d", 1, got)
	}

	// Verify headers.
	hdrs := env.runtime.Get("receivedHeaders")
	if hdrs == nil {
		t.Fatalf("expected non-nil")
	}
	hdrsObj := hdrs.Export().(map[string]any)
	assertMetadataContains(t, hdrsObj, "x-stream-type", "bidi")

	// Verify trailers.
	trls := env.runtime.Get("receivedTrailers")
	if trls == nil {
		t.Fatalf("expected non-nil")
	}
	trlsObj := trls.Export().(map[string]any)
	assertMetadataContains(t, trlsObj, "x-bidi-done", "yes")
}

// -------------- Server-side requestHeader access ----------------

func TestServerHandler_RequestHeaderAccess(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				// Read multiple request headers.
				var auth = call.requestHeader.get('x-auth');
				var trace = call.requestHeader.get('x-trace-id');

				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'auth=' + (auth || 'none') + ' trace=' + (trace || 'none'));
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
		var md = grpc.metadata.create();
		md.set('x-auth', 'token-123');
		md.set('x-trace-id', 'trace-456');

		client.echo(req, { metadata: md }).then(function(resp) {
			result = resp.get('message');
			__done();
		}).catch(function(err) { __done(); });
	`, defaultTimeout)

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	if got := result.String(); got != "auth=token-123 trace=trace-456" {
		t.Errorf("expected %v, got %v", "auth=token-123 trace=trace-456", got)
	}
}

func TestServerHandler_RequestHeaderEmpty(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				// requestHeader should exist even with no metadata.
				var keys = [];
				call.requestHeader.forEach(function(val, key) {
					keys.push(key);
				});

				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'keys=' + keys.length);
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
	if got := result.String(); got != "keys=0" {
		t.Errorf("expected %v, got %v", "keys=0", got)
	}
}

// -------------- Server-side sendHeader (explicit early headers) ----------------

func TestServerStream_SendHeader_ExplicitEarly(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Server stream handler sends headers before first message.
	// This verifies sendHeader() works independently.
	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {
				var hd = grpc.metadata.create();
				hd.set('x-early', 'true');
				call.setHeader(hd);
				call.sendHeader();

				// Small delay before sending items.
				var Item = pb.messageType('testgrpc.Item');
				var item = new Item();
				item.set('id', '1');
				item.set('name', 'delayed');
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

		var receivedHeaders;

		client.serverStream(req, {
			onHeader: function(headers) {
				receivedHeaders = headers.toObject();
			}
		}).then(function(stream) {
			// Read all items to completion.
			function drain() {
				stream.recv().then(function(r) {
					if (r.done) { __done(); return; }
					drain();
				});
			}
			drain();
		}).catch(function(err) { __done(); });
	`, defaultTimeout)

	hdrs := env.runtime.Get("receivedHeaders")
	if hdrs == nil {
		t.Fatalf("expected non-nil")
	}
	hdrsObj := hdrs.Export().(map[string]any)
	assertMetadataContains(t, hdrsObj, "x-early", "true")
}

// -------------- Multiple setHeader calls accumulate ----------------

func TestUnaryRPC_MultipleSetHeader(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var hd1 = grpc.metadata.create();
				hd1.set('x-first', '1');
				call.setHeader(hd1);

				var hd2 = grpc.metadata.create();
				hd2.set('x-second', '2');
				call.setHeader(hd2);

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

		var receivedHeaders;
		client.echo(req, {
			onHeader: function(headers) {
				receivedHeaders = headers.toObject();
			}
		}).then(function(resp) { __done(); }).catch(function(err) { __done(); });
	`, defaultTimeout)

	hdrs := env.runtime.Get("receivedHeaders")
	if hdrs == nil {
		t.Fatalf("expected non-nil")
	}
	hdrsObj := hdrs.Export().(map[string]any)
	assertMetadataContains(t, hdrsObj, "x-first", "1")
	assertMetadataContains(t, hdrsObj, "x-second", "2")
}

// -------------- onHeader/onTrailer callback type validation ----------------

func TestCallOpts_OnHeaderNotAFunction(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Register server to avoid unimplemented error.
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
		try {
			client.echo(req, { onHeader: "not a function" });
		} catch(e) {
			error = e.message;
		}
		__done();
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if !strings.Contains(errVal.String(), "onHeader must be a function") {
		t.Errorf("expected %q to contain %q", errVal.String(), "onHeader must be a function")
	}
}

func TestCallOpts_OnTrailerNotAFunction(t *testing.T) {
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
		try {
			client.echo(req, { onTrailer: 42 });
		} catch(e) {
			error = e.message;
		}
		__done();
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if !strings.Contains(errVal.String(), "onTrailer must be a function") {
		t.Errorf("expected %q to contain %q", errVal.String(), "onTrailer must be a function")
	}
}

// -------------- T170: timeoutMs client call option ----------------

func TestUnaryRPC_TimeoutMs_DeadlineExceeded(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				// Server handler that never returns (simulates slow handler).
				// The RPC should time out before this completes.
				return new Promise(function(resolve) {
					// Never resolve.
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
		req.set('message', 'slow');

		var error;
		client.echo(req, { timeoutMs: 50 }).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { code: err.code, name: err.name };
			__done();
		});
	`, defaultTimeout)

	errObj := env.runtime.Get("error")
	if errObj == nil {
		t.Fatalf("expected non-nil")
	}
	exported := errObj.Export().(map[string]any)
	if got := exported["code"]; got != int64(4) {
		t.Errorf("expected %v, got %v", int64(4), got)
	}
}

func TestUnaryRPC_TimeoutMs_SuccessBeforeTimeout(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'fast');
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
		client.echo(req, { timeoutMs: 5000 }).then(function(resp) {
			result = resp.get('message');
			__done();
		}).catch(function(err) { __done(); });
	`, defaultTimeout)

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	if got := result.String(); got != "fast" {
		t.Errorf("expected %v, got %v", "fast", got)
	}
}

func TestUnaryRPC_TimeoutMs_ZeroIgnored(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// timeoutMs of 0 should be ignored (no timeout set).
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
		client.echo(req, { timeoutMs: 0 }).then(function(resp) {
			result = resp.get('message');
			__done();
		}).catch(function(err) { __done(); });
	`, defaultTimeout)

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	if got := result.String(); got != "ok" {
		t.Errorf("expected %v, got %v", "ok", got)
	}
}

// ======================== Test Helpers ==========================

// assertMetadataContains checks that the exported metadata object
// contains the expected key with the expected first value.
func assertMetadataContains(t *testing.T, obj map[string]any, key, expectedValue string) {
	t.Helper()
	vals, ok := obj[key]
	if !(ok) {
		t.Fatalf("metadata key %q not found in %v", key, obj)
	}
	arr, ok := vals.([]any)
	if !(ok) {
		t.Fatalf("metadata key %q value is not an array: %T", key, vals)
	}
	if len(arr) == 0 {
		t.Fatalf("metadata key %q has empty array", key)
	}
	if got := arr[0]; got != expectedValue {
		t.Errorf("metadata key %q first value: expected %v, got %v", key, expectedValue, got)
	}
}
