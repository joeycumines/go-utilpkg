package gojagrpc

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/goja"
)

// TestClientStreamResponseEagerCompletion verifies the eager client-stream
// response contract (review finding 4): the response promise is created at
// call construction (submitting the terminal RecvMsg command), so the
// transport drains, the stream worker terminates, and the supervisor root
// retires even when the caller never reads .response — without producing an
// unhandled rejection. Later access to .response returns the same settled
// promise.
func TestClientStreamResponseEagerCompletion(t *testing.T) {
	for _, tc := range []struct {
		name        string
		serverBody  string
		wantMessage string
		wantError   string
	}{
		{
			name: "success",
			serverBody: `
				clientStream: function(call) {
					return new Promise(function(resolve, reject) {
						function readLoop() {
							call.recv().then(function(result) {
								if (result.done) {
									var EchoResponse = pb.messageType('testgrpc.EchoResponse');
									var resp = new EchoResponse();
									resp.set('message', 'ok');
									resolve(resp);
								} else {
									readLoop();
								}
							}).catch(reject);
						}
						readLoop();
					});
				},`,
			wantMessage: "ok",
		},
		{
			name: "server error",
			serverBody: `
				clientStream: function(call) {
					return new Promise(function(resolve, reject) {
						function readLoop() {
							call.recv().then(function(result) {
								if (result.done) {
									reject(new Error('client stream failed'));
								} else {
									readLoop();
								}
							}).catch(reject);
						}
						readLoop();
					});
				},`,
			wantError: "client stream failed",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newGrpcTestEnv(t)
			defer env.shutdown()

			// Observe unhandled rejections: goja reports a rejection with no
			// handlers synchronously at rejection time. The eagerly armed
			// internal no-op handler must keep the response promise handled,
			// so no PromiseRejectionReject may fire.
			var unhandled atomic.Int32
			env.runtime.SetPromiseRejectionTracker(func(p *goja.Promise, operation goja.PromiseRejectionOperation) {
				if operation == goja.PromiseRejectionReject {
					unhandled.Add(1)
				}
			})

			// Run the loop manually so it stays alive across both script
			// phases (the runOnLoop helper terminates it after __done), and
			// read every runtime global ON the loop: the goja runtime is not
			// goroutine-safe.
			done := make(chan struct{}, 1)
			if err := env.runtime.Set("__done", func(goja.FunctionCall) goja.Value {
				select {
				case done <- struct{}{}:
				default:
				}
				return goja.Undefined()
			}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			runDone := make(chan error, 1)
			go func() { runDone <- env.loop.Run(ctx) }()
			submitClientStreamPhase := func(t *testing.T, code string) {
				t.Helper()
				if err := env.loop.Submit(func() {
					if _, err := env.runtime.RunString(code); err != nil {
						t.Errorf("phase script: %v", err)
					}
				}); err != nil {
					t.Fatal(err)
				}
				select {
				case <-done:
				case <-time.After(defaultTimeout):
					t.Fatal("client stream phase did not complete")
				}
			}
			readGlobalString := func(t *testing.T, name string) (string, bool) {
				t.Helper()
				type result struct {
					value string
					ok    bool
				}
				ch := make(chan result, 1)
				if err := env.loop.Submit(func() {
					value := env.runtime.Get(name)
					if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
						ch <- result{}
						return
					}
					ch <- result{value: value.String(), ok: true}
				}); err != nil {
					t.Fatal(err)
				}
				select {
				case result := <-ch:
					return result.value, result.ok
				case <-time.After(defaultTimeout):
					t.Fatalf("read global %s did not complete", name)
					return "", false
				}
			}

			// Phase 1: run the RPC to completion without reading .response.
			submitClientStreamPhase(t, `
				var server = grpc.createServer();
				server.addService('testgrpc.TestService', {
					echo: function(request, call) { return null; },
					serverStream: function(request, call) {},
					`+tc.serverBody+`
					bidiStream: function(call) {}
				});
				server.start();
				var client = grpc.createClient('testgrpc.TestService');
				client.clientStream().then(function(call) {
					globalThis.__call = call;
					var Item = pb.messageType('testgrpc.Item');
					var item = new Item();
					item.set('id', '1');
					item.set('name', 'one');
					call.send(item).then(function() {
						return call.closeSend();
					}).then(function() {
						// Deliberately do NOT read call.response: the eager
						// construction must already have submitted the
						// terminal recv.
						__done();
					}).catch(function(err) {
						globalThis.__callError = err.message;
						__done();
					});
				}).catch(function(err) {
					globalThis.__callError = err.message;
					__done();
				});
			`)

			if message, ok := readGlobalString(t, "__callError"); ok {
				t.Fatalf("RPC failed without reading .response: %v", message)
			}
			// The stream worker and supervisor root must retire even though
			// .response was never read.
			deadline := time.Now().Add(defaultTimeout)
			for supervisorKindCount(env.grpcMod, supervisorOperation) != 0 {
				if time.Now().After(deadline) {
					t.Fatalf(
						"supervisor operations retained without reading .response = %d",
						supervisorKindCount(env.grpcMod, supervisorOperation),
					)
				}
				runtime.Gosched()
			}
			if got := unhandled.Load(); got != 0 {
				t.Fatalf("unhandled rejections = %d, want 0", got)
			}

			// Phase 2: later access returns the same settled promise.
			submitClientStreamPhase(t, `
				globalThis.__responseMessage = null;
				globalThis.__responseError = null;
				globalThis.__call.response.then(function(resp) {
					globalThis.__responseMessage = resp.get('message');
					__done();
				}, function(err) {
					globalThis.__responseError = err.message;
					__done();
				});
			`)

			if tc.wantError != "" {
				message, ok := readGlobalString(t, "__responseError")
				if !ok {
					t.Fatalf("later .response access did not reject, want %q", tc.wantError)
				}
				if message != tc.wantError {
					t.Fatalf("later .response rejection = %q, want %q", message, tc.wantError)
				}
			} else {
				message, ok := readGlobalString(t, "__responseMessage")
				if !ok {
					if errorMessage, hasError := readGlobalString(t, "__responseError"); hasError {
						t.Fatalf("later .response access rejected: %v", errorMessage)
					}
					t.Fatal("later .response access did not resolve")
				}
				if message != tc.wantMessage {
					t.Fatalf("later .response message = %q, want %q", message, tc.wantMessage)
				}
			}
			if got := unhandled.Load(); got != 0 {
				t.Fatalf("unhandled rejections after later access = %d, want 0", got)
			}

			cancel()
			select {
			case err := <-runDone:
				if err != nil && err != context.Canceled {
					t.Fatal(err)
				}
			case <-time.After(defaultTimeout):
				t.Fatal("event loop did not exit")
			}
		})
	}
}
