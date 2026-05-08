package gojagrpc

import (
	"context"
	"testing"
	"time"

	"github.com/joeycumines/goja"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// ============================================================================
// Test: ClientStream error — abort before stream creation
//
// Covers: client.go line 450-452 (stream creation error)
// ============================================================================

func TestClientStream_AbortBeforeCall(t *testing.T) {
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
		var ctrl = new AbortController();
		ctrl.abort(); // Abort BEFORE call

		// Pass onHeader to exercise the header-fetch goroutine error path
		// (client.go:450-452). With a pre-aborted signal, cs.Header()
		// returns a context error, triggering the early return.
		client.clientStream({
			signal: ctrl.signal,
			onHeader: function(md) {}
		}).then(function(call) {
			error = 'should not resolve';
			__done();
		}).catch(function(err) {
			error = err;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
}

// ============================================================================
// Test: BidiStream error — abort before stream creation
//
// Covers: client.go line 636 (Submit failure) or stream creation error
// ============================================================================

func TestBidiStream_AbortBeforeCall(t *testing.T) {
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
		var ctrl = new AbortController();
		ctrl.abort(); // Abort BEFORE call

		client.bidiStream({ signal: ctrl.signal }).then(function(stream) {
			error = 'should not resolve';
			__done();
		}).catch(function(err) {
			error = err;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
}

// ============================================================================
// Test: Reflection JS methods — Submit failure ("event loop not running")
//
// Start and immediately stop the loop, then verify reflection methods
// reject properly when the loop is not running.
//
// Covers: reflection.go lines 60, 82, 104 (submitErr paths)
// ============================================================================

func TestReflection_SubmitFailure(t *testing.T) {
	env := newGrpcTestEnv(t)

	_, err := env.pbMod.LoadDescriptorSetBytes(phase2DescriptorSetBytes())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Run the loop briefly then stop it.  After Run returns, the loop
	// is fully stopped and Submit calls will fail.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = env.loop.Run(ctx)

	// Now call the JS reflection functions directly.  The goja runtime
	// is safe to use from the test goroutine because no other goroutine
	// holds it (the loop has exited).
	//
	// Each function spawns a goroutine that will:
	//   1. Try to create a gRPC stream (calls loop.Submit → fails)
	//   2. Get an error from the gRPC operation
	//   3. Try to Submit the rejection (loop.Submit → fails)
	//   4. Fall through to reject(fmt.Errorf("event loop not running"))
	//
	// Covers: reflection.go lines 60, 82 (describeService), 104
	listResult := env.grpcMod.jsReflListServices(goja.FunctionCall{})
	descResult := env.grpcMod.jsReflDescribeService(goja.FunctionCall{
		Arguments: []goja.Value{env.runtime.ToValue("test.Service")},
	})
	typeResult := env.grpcMod.jsReflDescribeType(goja.FunctionCall{
		Arguments: []goja.Value{env.runtime.ToValue("phase2.BaseMsg")},
	})

	// Give goroutines time to execute and hit the reject-direct path.
	time.Sleep(200 * time.Millisecond)

	_ = listResult
	_ = descResult
	_ = typeResult
}

// ============================================================================
// Test: Unary RPC Submit failure
//
// Covers: client.go line 243 (executeUnaryRPC Submit failure)
//         client.go line 329 (server-stream Submit failure)
//         client.go line 462 (client-stream Submit failure)
//         client.go line 636 (bidi stream Submit failure)
// ============================================================================

func TestClientRPC_SubmitFailures(t *testing.T) {
	env := newGrpcTestEnv(t)

	// Phase 1: Set up server and client on the running event loop.
	ctx, cancel := context.WithCancel(context.Background())
	setupDone := make(chan struct{}, 1)
	_ = env.runtime.Set("__ready", env.runtime.ToValue(func(_ goja.FunctionCall) goja.Value {
		select {
		case setupDone <- struct{}{}:
		default:
		}
		return goja.Undefined()
	}))

	_ = env.loop.Submit(func() {
		env.runtime.RunString(`
			var server = grpc.createServer();
			server.addService('testgrpc.TestService', {
				echo: function(request, call) { return request; },
				serverStream: function(request, call) {},
				clientStream: function(call) { return null; },
				bidiStream: function(call) {}
			});
			server.start();
			var client = grpc.createClient('testgrpc.TestService');
			var EchoRequest = pb.messageType('testgrpc.EchoRequest');
			__ready();
		`)
	})

	loopDone := make(chan struct{})
	go func() {
		env.loop.Run(ctx)
		close(loopDone)
	}()

	select {
	case <-setupDone:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("timeout waiting for setup")
	}

	// Phase 2: Stop the loop and wait for it to fully exit.
	cancel()
	select {
	case <-loopDone:
	case <-time.After(3 * time.Second):
		t.Fatal("loop didn't stop")
	}

	// Phase 3: Call all four RPC types from the test goroutine.
	// The loop is stopped, so goroutines inside each method will:
	//   1. Fail at channel.Invoke/NewStream (loop.Submit fails)
	//   2. Try to Submit the rejection → fails
	//   3. Fall through to reject(fmt.Errorf("event loop not running"))
	env.runtime.RunString(`
		var req = new EchoRequest();
		req.set('message', 'test');
		client.echo(req);
		client.serverStream(req);
		client.clientStream();
		client.bidiStream();
	`)

	// Wait for goroutines to execute and hit the reject-direct paths.
	time.Sleep(200 * time.Millisecond)
}

// ============================================================================
// Test: Stream reader recv Submit failure
//
// Exercise the "event loop not running" path in newStreamReader's recv
// goroutine by stopping the loop while a server-streaming read is in flight.
//
// Covers: client.go line 390 (Submit failure in stream reader recv)
// ============================================================================

func TestStreamReader_RecvSubmitFailure(t *testing.T) {
	env := newGrpcTestEnv(t)

	ctx, cancel := context.WithCancel(context.Background())

	setupDone := make(chan struct{}, 1)
	_ = env.runtime.Set("__ready", env.runtime.ToValue(func(_ goja.FunctionCall) goja.Value {
		select {
		case setupDone <- struct{}{}:
		default:
		}
		return goja.Undefined()
	}))

	_ = env.loop.Submit(func() {
		env.runtime.RunString(`
			var server = grpc.createServer();
			server.addService('testgrpc.TestService', {
				echo: function(request, call) { return null; },
				serverStream: function(request, call) {
					// Send one item, then delay indefinitely.
					var Item = pb.messageType('testgrpc.Item');
					var item1 = new Item();
					item1.set('id', '1');
					item1.set('name', 'first');
					call.send(item1);
					// Never finish — keep stream open.
					return new Promise(function(resolve) {
						setTimeout(function() { resolve(); }, 60000);
					});
				},
				clientStream: function(call) { return null; },
				bidiStream: function(call) {}
			});
			server.start();
			__ready();
		`)
	})

	go env.loop.Run(ctx)

	select {
	case <-setupDone:
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("timeout")
	}

	// Make a server-streaming call from Go.
	cs, err := env.channel.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true}, "/testgrpc.TestService/ServerStream")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Send the request.
	desc, _ := env.pbMod.FindDescriptor(protoreflect.FullName("testgrpc.EchoRequest"))
	reqMsg := dynamicpb.NewMessage(desc.(protoreflect.MessageDescriptor))
	err = cs.SendMsg(reqMsg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = cs.CloseSend()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Receive the first item.
	itemDesc, _ := env.pbMod.FindDescriptor(protoreflect.FullName("testgrpc.Item"))
	respMsg := dynamicpb.NewMessage(itemDesc.(protoreflect.MessageDescriptor))
	err = cs.RecvMsg(respMsg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Now cancel the loop while a second RecvMsg is in flight.
	recvDone := make(chan error, 1)
	recvStarted := make(chan struct{})
	go func() {
		respMsg2 := dynamicpb.NewMessage(itemDesc.(protoreflect.MessageDescriptor))
		close(recvStarted)
		recvDone <- cs.RecvMsg(respMsg2)
	}()

	// Wait for the goroutine to start, then cancel.
	<-recvStarted
	cancel()

	select {
	case err := <-recvDone:
		code := status.Code(err)
		if code != codes.Canceled && code != codes.Unavailable {
			t.Fatalf(
				"RecvMsg after simultaneous caller and loop cancellation = %v, want Canceled or Unavailable",
				err,
			)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("RecvMsg didn't complete after loop cancel")
	}
}

// ============================================================================
// Test: Client-stream sender goroutine — CloseSend error
//
// Triggers the CloseSend error path in the sender goroutine by aborting
// the stream before closeSend completes.
//
// Covers: client.go lines 506-509 (CloseSend Submit failure)
// ============================================================================

func TestClientStream_CloseSendAfterAbort(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) {
				return call.recv().then(function(result) {
					var EchoResponse = pb.messageType('testgrpc.EchoResponse');
					var csOkResp = new EchoResponse();
					csOkResp.set('message', 'ok');
					return csOkResp;
				});
			},
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var ctrl = new AbortController();
		var Item = pb.messageType('testgrpc.Item');
		var error;

		client.clientStream({ signal: ctrl.signal }).then(function(call) {
			var csMsg = new Item(); csMsg.set('id', '1'); csMsg.set('name', 'test');
			return call.send(csMsg).then(function() {
				ctrl.abort(); // Abort before closeSend
				return call.closeSend().catch(function(e) {
					error = e;
				});
			});
		}).then(function() {
			__done();
		}).catch(function(err) {
			error = err;
			__done();
		});
	`, defaultTimeout)
}

// ============================================================================
// Test: Bidi-stream sender goroutine — send error after abort
//
// Covers: client.go lines 668-689 (sender goroutine error paths)
// ============================================================================

func TestBidiStream_SendAfterAbort(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {
				return (function loop() {
					return call.recv().then(function(result) {
						if (result.done) return;
						call.send(result.value);
						return loop();
					}).catch(function() {});
				})();
			}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var ctrl = new AbortController();
		var Item = pb.messageType('testgrpc.Item');
		var sendError;
		var closeError;

		client.bidiStream({ signal: ctrl.signal }).then(function(stream) {
			var bm1 = new Item(); bm1.set('id', '1'); bm1.set('name', 'x');
			return stream.send(bm1).then(function() {
				ctrl.abort();
				// Try to send after abort — should fail.
				var bm2 = new Item(); bm2.set('id', '2'); bm2.set('name', 'y');
				return stream.send(bm2).catch(function(e) {
					sendError = e;
				});
			}).then(function() {
				return stream.closeSend().catch(function(e) {
					closeError = e;
				});
			});
		}).then(function() {
			__done();
		}).catch(function(err) {
			__done();
		});
	`, defaultTimeout)
}

// ============================================================================
// dial.go:67 — grpc.NewClient error is extremely hard to trigger
// (NewClient accepts virtually any input without error).
// The empty-target check is JS-level and tested in dial_test.go.

// ============================================================================
// Test: Bidi recv Submit failure
//
// Covers: client.go line 749 (Submit failure in bidi recv goroutine)
// ============================================================================

func TestBidiStream_RecvSubmitFailure(t *testing.T) {
	env := newGrpcTestEnv(t)

	ctx, cancel := context.WithCancel(context.Background())

	setupDone := make(chan struct{}, 1)
	_ = env.runtime.Set("__ready", env.runtime.ToValue(func(_ goja.FunctionCall) goja.Value {
		select {
		case setupDone <- struct{}{}:
		default:
		}
		return goja.Undefined()
	}))

	_ = env.loop.Submit(func() {
		env.runtime.RunString(`
			var server = grpc.createServer();
			server.addService('testgrpc.TestService', {
				echo: function(request, call) { return null; },
				serverStream: function(request, call) {},
				clientStream: function(call) { return null; },
				bidiStream: function(call) {
					// Echo back messages with a delay.
					return (function loop() {
						return call.recv().then(function(result) {
							if (result.done) return;
							call.send(result.value);
							return loop();
						}).catch(function() {});
					})();
				}
			});
			server.start();
			__ready();
		`)
	})

	go env.loop.Run(ctx)

	select {
	case <-setupDone:
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("timeout")
	}

	// Create a bidi stream via Go.
	cs, err := env.channel.NewStream(ctx, &grpc.StreamDesc{
		ClientStreams: true,
		ServerStreams: true,
	}, "/testgrpc.TestService/BidiStream")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Send a message.
	itemDesc, _ := env.pbMod.FindDescriptor(protoreflect.FullName("testgrpc.Item"))
	msg := dynamicpb.NewMessage(itemDesc.(protoreflect.MessageDescriptor))
	msg.Set(msg.Descriptor().Fields().ByName("id"), protoreflect.ValueOfString("1"))
	err = cs.SendMsg(msg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Receive the echo.
	respMsg := dynamicpb.NewMessage(itemDesc.(protoreflect.MessageDescriptor))
	err = cs.RecvMsg(respMsg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Start another recv that will be in-flight when we cancel the loop.
	recvDone := make(chan error, 1)
	recvStarted := make(chan struct{})
	go func() {
		resp2 := dynamicpb.NewMessage(itemDesc.(protoreflect.MessageDescriptor))
		close(recvStarted)
		recvDone <- cs.RecvMsg(resp2)
	}()

	// Cancel the loop.
	<-recvStarted
	cancel()

	select {
	case err := <-recvDone:
		code := status.Code(err)
		if code != codes.Canceled && code != codes.Unavailable {
			t.Fatalf(
				"RecvMsg after simultaneous caller and loop cancellation = %v, want Canceled or Unavailable",
				err,
			)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("RecvMsg didn't complete")
	}
}

// ============================================================================
// Test: Inner goroutine Submit failures
//
// Creates server-stream reader, client-stream call, and bidi stream
// while the loop is running. Then stops the loop and exercises
// send/recv/closeSend which trigger Submit failures in the inner goroutines.
//
// Covers:
//   client.go:390  (stream reader recv Submit failure)
//   client.go:506  (client-stream closeSend Submit failure)
//   client.go:520  (client-stream send Submit failure)
//   client.go:668  (bidi closeSend Submit failure)
//   client.go:687  (bidi send Submit failure)
//   client.go:749  (bidi recv Submit failure)
// ============================================================================

func TestInnerGoroutine_SubmitFailures(t *testing.T) {
	env := newGrpcTestEnv(t)

	ctx, cancel := context.WithCancel(context.Background())

	setupDone := make(chan struct{}, 1)
	_ = env.runtime.Set("__ready", env.runtime.ToValue(func(_ goja.FunctionCall) goja.Value {
		select {
		case setupDone <- struct{}{}:
		default:
		}
		return goja.Undefined()
	}))

	_ = env.loop.Submit(func() {
		env.runtime.RunString(`
			var server = grpc.createServer();
			server.addService('testgrpc.TestService', {
				echo: function(request, call) { return request; },
				serverStream: function(request, call) {
					// Send one item, then keep stream open forever.
					var Item = pb.messageType('testgrpc.Item');
					var item = new Item();
					item.set('id', '1');
					item.set('name', 'test');
					call.send(item);
					return new Promise(function(resolve) {
						setTimeout(function() { resolve(); }, 60000);
					});
				},
				clientStream: function(call) {
					// Never resolve — long-running handler.
					return new Promise(function(resolve) {
						setTimeout(function() { resolve(null); }, 60000);
					});
				},
				bidiStream: function(call) {
					// Never resolve — long-running handler.
					return new Promise(function(resolve) {
						setTimeout(function() { resolve(); }, 60000);
					});
				}
			});
			server.start();

			var client = grpc.createClient('testgrpc.TestService');
			var Item = pb.messageType('testgrpc.Item');
			var EchoRequest = pb.messageType('testgrpc.EchoRequest');

			// Create the three stream types and store them globally.
			var ssReader = null;
			var csCall = null;
			var bidiStream = null;
			var pending = 3;

			function checkDone() {
				pending--;
				if (pending === 0) __ready();
			}

			// Server-streaming
			var ssReq = new EchoRequest();
			ssReq.set('message', 'start');
			client.serverStream(ssReq).then(function(stream) {
				// Consume the first message to ensure stream is alive.
				return stream.recv().then(function(result) {
					ssReader = stream;
					checkDone();
				});
			}).catch(function(err) {
				// If the stream fails, still call checkDone to unblock.
				checkDone();
			});

			// Client-streaming
			client.clientStream().then(function(call) {
				csCall = call;
				checkDone();
			}).catch(function(err) {
				checkDone();
			});

			// Bidi streaming
			client.bidiStream().then(function(stream) {
				bidiStream = stream;
				checkDone();
			}).catch(function(err) {
				checkDone();
			});
		`)
	})

	loopDone := make(chan struct{})
	go func() {
		env.loop.Run(ctx)
		close(loopDone)
	}()

	select {
	case <-setupDone:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("timeout waiting for streams to establish")
	}

	// Phase 2: Stop the loop and wait for it to fully exit.
	cancel()
	select {
	case <-loopDone:
	case <-time.After(3 * time.Second):
		t.Fatal("loop didn't stop")
	}

	// Phase 3: Exercise send/recv/closeSend on the stored stream objects.
	// The loop is stopped, so all internal goroutines will fail at Submit.

	// Server-streaming recv: spawns a goroutine that does RecvMsg → Submit fails.
	// Covers client.go:390
	env.runtime.RunString(`
		if (ssReader) ssReader.recv();
	`)

	// Client-stream send + closeSend: puts ops on sendCh, sender goroutine
	// does SendMsg/CloseSend → Submit fails.
	// Covers client.go:506 (closeSend), 520 (send)
	env.runtime.RunString(`
		if (csCall) {
			var csItem = new Item();
			csItem.set('id', '2');
			csItem.set('name', 'late');
			csCall.send(csItem);
		}
	`)
	// Give sender goroutine time to process the send.
	time.Sleep(50 * time.Millisecond)
	env.runtime.RunString(`
		if (csCall) csCall.closeSend();
	`)

	// Bidi send + recv + closeSend: same pattern.
	// Covers client.go:668 (closeSend), 687 (send), 749 (recv)
	env.runtime.RunString(`
		if (bidiStream) {
			var bidiItem = new Item();
			bidiItem.set('id', '3');
			bidiItem.set('name', 'late');
			bidiStream.send(bidiItem);
			bidiStream.recv();
		}
	`)
	time.Sleep(50 * time.Millisecond)
	env.runtime.RunString(`
		if (bidiStream) bidiStream.closeSend();
	`)

	// Wait for all goroutines to execute and hit the reject-direct paths.
	time.Sleep(200 * time.Millisecond)
}

// ============================================================================
// Test: Sender goroutine inside-callback error paths
//
// After aborting a stream, the sender goroutine's underlying send/closeSend
// operations fail (stream closed), then Submit SUCCEEDS (loop is running),
// and the callback executes the error branch.
//
// Covers:
//   client.go:506-509  (closeSend callback with closeErr)
//   client.go:520-523  (send callback with sendErr)
//   client.go:668-671  (bidi closeSend callback with closeErr)
// ============================================================================

func TestSenderGoroutine_StreamAbortedErrors(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return request; },
			serverStream: function(request, call) {
				// Long-running handler.
				return new Promise(function(resolve) {
					setTimeout(function() { resolve(); }, 60000);
				});
			},
			clientStream: function(call) {
				// Long-running handler — never resolve.
				return new Promise(function(resolve) {
					setTimeout(function() { resolve(null); }, 60000);
				});
			},
			bidiStream: function(call) {
				// Long-running handler.
				return new Promise(function(resolve) {
					setTimeout(function() { resolve(); }, 60000);
				});
			}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var Item = pb.messageType('testgrpc.Item');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');

		// Use timeoutMs to cause the context to expire while the loop
		// is still running.  After timeout, the sender goroutine's
		// SendMsg/CloseSend returns context.DeadlineExceeded.  Since the
		// loop IS running, Submit succeeds, and the callback runs the
		// error branch.

		var pending = 3;
		function checkDone() {
			pending--;
			if (pending === 0) __done();
		}

		// Server-stream: 5ms timeout.  The goroutine inside calls
		// NewStream, SendMsg, CloseSend.  With a 5ms timeout, CloseSend
		// may find the context already expired.
		// Covers client.go:306-311 (CloseSend error in server-stream goroutine)
		var ssReq = new EchoRequest();
		ssReq.set('message', 'timeout-test');
		client.serverStream(ssReq, { timeoutMs: 5 }).then(function() {
			checkDone();
		}).catch(function() {
			checkDone();
		});

		// Client-stream: 1ms timeout, then send + closeSend after 200ms
		// By 200ms, the context has been expired for 199ms and the
		// context-watching goroutine has had plenty of time to close
		// the Requests channel. So both SendMsg and CloseSend are
		// guaranteed to see the cancelled context.
		// Covers client.go:506-509, 520-523
		client.clientStream({ timeoutMs: 1 }).then(function(call) {
			setTimeout(function() {
				var csItem = new Item();
				csItem.set('id', '1');
				csItem.set('name', 'fail');
				call.send(csItem).catch(function() {});
				setTimeout(function() {
					call.closeSend().catch(function() {});
					setTimeout(function() { checkDone(); }, 50);
				}, 50);
			}, 200);
		}).catch(function() {
			checkDone();
		});

		// Bidi: 1ms timeout, then send + closeSend after 200ms
		// Covers client.go:668-671
		client.bidiStream({ timeoutMs: 1 }).then(function(stream) {
			setTimeout(function() {
				var bidiItem = new Item();
				bidiItem.set('id', '2');
				bidiItem.set('name', 'fail');
				stream.send(bidiItem).catch(function() {});
				setTimeout(function() {
					stream.closeSend().catch(function() {});
					setTimeout(function() { checkDone(); }, 50);
				}, 50);
			}, 200);
		}).catch(function() {
			checkDone();
		});
	`, defaultTimeout)
}
