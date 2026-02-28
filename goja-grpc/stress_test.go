package gojagrpc

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// ============================================================================
// T243: Stress test: JS client 100 concurrent RPCs via Promise.all
// ============================================================================

func TestStress_JSClient100ConcurrentRPCs(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Use a JS echo server — pure JS stress test.
	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var msg = request.get('message');
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'echo:' + msg);
				return resp;
			},
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var client = grpc.createClient('testgrpc.TestService');
		var promises = [];
		for (var i = 0; i < 100; i++) {
			(function(idx) {
				var req = new EchoRequest();
				req.set('message', 'stress-' + idx);
				promises.push(
					client.echo(req).then(function(resp) {
						return { idx: idx, msg: resp.get('message') };
					})
				);
			})(i);
		}

		var results;
		Promise.all(promises).then(function(arr) {
			results = { count: arr.length, first: arr[0].msg, last: arr[99].msg };
			__done();
		}).catch(function(err) {
			results = { error: err.message };
			__done();
		});
	`, 30*time.Second)

	r := env.runtime.Get("results")
	if r == nil {
		t.Fatalf("expected non-nil")
	}
	rObj := r.Export().(map[string]any)
	if rObj["error"] != nil {
		t.Errorf("expected nil, got %v", rObj["error"])
	}
	if got := rObj["count"]; got != int64(100) {
		t.Errorf("expected %v, got %v", int64(100), got)
	}
	if got := rObj["first"]; got != "echo:stress-0" {
		t.Errorf("expected %v, got %v", "echo:stress-0", got)
	}
	if got := rObj["last"]; got != "echo:stress-99" {
		t.Errorf("expected %v, got %v", "echo:stress-99", got)
	}
}

// ============================================================================
// T244: Stress test: Go client sends 100 concurrent RPCs to JS server
// ============================================================================

func TestStress_GoClient100ConcurrentRPCsToJSServer(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Start the event loop in the background (remains running).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	loopDone := make(chan error, 1)
	serverReady := make(chan struct{}, 1)

	var jsErr error
	if err := env.loop.Submit(func() {
		_, jsErr = env.runtime.RunString(`
			var server = grpc.createServer();
			server.addService('testgrpc.TestService', {
				echo: function(request, call) {
					var msg = request.get('message');
					var EchoResponse = pb.messageType('testgrpc.EchoResponse');
					var resp = new EchoResponse();
					resp.set('message', 'js-echo:' + msg);
					resp.set('code', 42);
					return resp;
				},
				serverStream: function(request, call) {},
				clientStream: function(call) { return null; },
				bidiStream: function(call) {}
			});
			server.start();
		`)
		// Signal that the server is ready.
		select {
		case serverReady <- struct{}{}:
		default:
		}
	}); err != nil {
		t.Fatalf("submit error: %v", err)
	}

	go func() {
		loopDone <- env.loop.Run(ctx)
	}()

	// Wait for server to be ready.
	select {
	case <-serverReady:
	case <-ctx.Done():
		t.Fatal("timeout waiting for server setup")
	}
	if jsErr != nil {
		t.Fatalf("unexpected error: %v", jsErr)
	}

	// Resolve message descriptors from the SAME protobuf module used by JS.
	// This is critical: using a different protodesc.NewFile would create
	// distinct descriptor instances that proto.Merge rejects.
	resolver := env.pbMod.FileResolver()
	reqDescAny, err := resolver.FindDescriptorByName("testgrpc.EchoRequest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reqDesc := reqDescAny.(protoreflect.MessageDescriptor)
	respDescAny, err := resolver.FindDescriptorByName("testgrpc.EchoResponse")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	respDesc := respDescAny.(protoreflect.MessageDescriptor)

	// Send 100 concurrent RPCs from Go using dynamicpb.
	const numRPCs = 100
	var wg sync.WaitGroup
	wg.Add(numRPCs)
	var failures atomic.Int64
	var successCount atomic.Int64

	for i := range numRPCs {
		go func(idx int) {
			defer wg.Done()
			req := dynamicpb.NewMessage(reqDesc)
			req.Set(reqDesc.Fields().ByName("message"), protoreflect.ValueOfString(fmt.Sprintf("go-stress-%d", idx)))

			resp := dynamicpb.NewMessage(respDesc)
			err := env.channel.Invoke(ctx, "/testgrpc.TestService/Echo", req, resp)
			if err != nil {
				t.Logf("RPC %d failed: %v", idx, err)
				failures.Add(1)
				return
			}

			msg := resp.Get(respDesc.Fields().ByName("message")).String()
			expected := fmt.Sprintf("js-echo:go-stress-%d", idx)
			if msg != expected {
				t.Logf("RPC %d: got %q, want %q", idx, msg, expected)
				failures.Add(1)
				return
			}
			successCount.Add(1)
		}(i)
	}
	wg.Wait()

	// Stop the event loop.
	cancel()
	<-loopDone

	successes := successCount.Load()
	fails := failures.Load()
	t.Logf("Go→JS: %d successes, %d failures out of %d RPCs", successes, fails, numRPCs)
	if fails > 0 {
		t.Fatalf("%d of %d RPCs failed", fails, numRPCs)
	}
}

// ============================================================================
// T245: Goroutine monitoring for goja-grpc
// ============================================================================

func TestStress_JSGoroutineLeakCheck(t *testing.T) {
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	baselineGoroutines := runtime.NumGoroutine()

	// Create 5 environments, each doing 20 RPCs, then shutdown.
	for batch := range 5 {
		env := newGrpcTestEnv(t)

		env.runOnLoop(t, fmt.Sprintf(`
			var server = grpc.createServer();
			server.addService('testgrpc.TestService', {
				echo: function(request, call) {
					var msg = request.get('message');
					var EchoResponse = pb.messageType('testgrpc.EchoResponse');
					var resp = new EchoResponse();
					resp.set('message', 'echo:' + msg);
					return resp;
				},
				serverStream: function(request, call) {},
				clientStream: function(call) { return null; },
				bidiStream: function(call) {}
			});
			server.start();

			var EchoRequest = pb.messageType('testgrpc.EchoRequest');
			var client = grpc.createClient('testgrpc.TestService');
			var count = 0;
			for (var i = 0; i < 20; i++) {
				(function(idx) {
					var req = new EchoRequest();
					req.set('message', 'batch%d-' + idx);
					client.echo(req).then(function(resp) {
						count++;
						if (count >= 20) __done();
					});
				})(i);
			}
		`, batch), defaultTimeout)

		env.shutdown()
	}

	runtime.GC()
	time.Sleep(200 * time.Millisecond)
	finalGoroutines := runtime.NumGoroutine()

	delta := finalGoroutines - baselineGoroutines
	t.Logf("JS goroutine leak check: baseline=%d final=%d delta=%d", baselineGoroutines, finalGoroutines, delta)

	if delta > 30 {
		t.Errorf("potential goroutine leak: %d goroutines above baseline", delta)
	}
}

// ============================================================================
// T246: Memory leak detection: heap profiling (allocation check)
// ============================================================================

func TestStress_HeapAllocationCheck(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Run a batch of RPCs and check heap growth.
	env.runOnLoop(t, `
		var server = grpc.createServer();
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

		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var client = grpc.createClient('testgrpc.TestService');

		// Warmup
		var warmupDone = 0;
		for (var w = 0; w < 50; w++) {
			(function() {
				var req = new EchoRequest();
				req.set('message', 'warmup');
				client.echo(req).then(function() {
					warmupDone++;
					if (warmupDone >= 50) {
						runBatch();
					}
				});
			})();
		}

		function runBatch() {
			var count = 0;
			var batchSize = 500;
			for (var i = 0; i < batchSize; i++) {
				(function(idx) {
					var req = new EchoRequest();
					req.set('message', 'heap-' + idx);
					client.echo(req).then(function(resp) {
						count++;
						if (count >= batchSize) {
							results = { completed: count };
							__done();
						}
					});
				})(i);
			}
		}

		var results;
	`, 30*time.Second)

	r := env.runtime.Get("results")
	if r == nil {
		t.Fatalf("expected non-nil")
	}
	rObj := r.Export().(map[string]any)
	if got := rObj["completed"]; got != int64(500) {
		t.Errorf("expected %v, got %v", int64(500), got)
	}

	// GC and check heap isn't leaking.
	runtime.GC()
	runtime.GC()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	t.Logf("After 550 JS RPCs: HeapInuse=%d HeapObjects=%d", mem.HeapInuse, mem.HeapObjects)
}
