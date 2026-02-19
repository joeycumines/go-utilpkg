package gojagrpc

import (
	"context"
	"testing"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// T102: Edge case - concurrent handler registration during RPC dispatch.
// Registration must happen before RPCs. Verify addService then start works
// correctly and that double start panics.
func TestEdge_DoubleStart(t *testing.T) {
	env := newGrpcTestEnv(t)
	err := env.mustFail(t, `
		const server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo(r) { return r; },
			serverStream() {},
			clientStream() {},
			bidiStream() {},
		});
		server.start();
		server.start(); // should throw
	`)
	if !strings.Contains(err.Error(), "already started") {
		t.Errorf("expected %q to contain %q", err.Error(), "already started")
	}
}

// T103: Edge case - JS handler that panics (throws exception).
// The exception should be caught and converted to INTERNAL error.
func TestEdge_HandlerPanic(t *testing.T) {
	env := newGrpcTestEnv(t)
	env.runOnLoop(t, `
		const server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo(request) {
				throw new Error('handler panic');
			},
			serverStream() {},
			clientStream() {},
			bidiStream() {},
		});
		server.start();

		const client = grpc.createClient('testgrpc.TestService');
		const Req = pb.messageType('testgrpc.EchoRequest');
		const req = new Req();
		req.set('message', 'test');

		client.echo(req).then(
			() => { result = 'unexpected success'; __done(); },
			(err) => { result = err.code; __done(); }
		);
	`, 5*time.Second)

	v := env.runtime.Get("result")
	if v == nil {
		t.Fatalf("expected non-nil")
	}
	// Should get INTERNAL error (code 13)
	if got := v.ToInteger(); got != int64(13) {
		t.Errorf("expected %v, got %v", int64(13), got)
	}
}

// T104: Edge case - handler throws a GrpcError (not generic Error).
func TestEdge_HandlerThrowsGrpcError(t *testing.T) {
	env := newGrpcTestEnv(t)
	env.runOnLoop(t, `
		const server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo(request) {
				throw grpc.status.createError(grpc.status.PERMISSION_DENIED, 'forbidden');
			},
			serverStream() {},
			clientStream() {},
			bidiStream() {},
		});
		server.start();

		const client = grpc.createClient('testgrpc.TestService');
		const Req = pb.messageType('testgrpc.EchoRequest');
		const req = new Req();
		req.set('message', 'test');

		client.echo(req).then(
			() => { result = 'unexpected'; __done(); },
			(err) => { result = err.code + ':' + err.message; __done(); }
		);
	`, 5*time.Second)

	v := env.runtime.Get("result")
	if got := v.String(); got != "7:forbidden" {
		t.Errorf("expected %v, got %v", "7:forbidden", got)
	}
}

// T105: Edge case - very large message (1MB+ byte-like field via string).
func TestEdge_LargeMessage(t *testing.T) {
	env := newGrpcTestEnv(t)

	// Create a 100KB string in JS to test message handling.
	env.runOnLoop(t, `
		const server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo(request) {
				const Resp = pb.messageType('testgrpc.EchoResponse');
				const resp = new Resp();
				resp.set('message', request.get('message'));
				return resp;
			},
			serverStream() {},
			clientStream() {},
			bidiStream() {},
		});
		server.start();

		const client = grpc.createClient('testgrpc.TestService');
		const Req = pb.messageType('testgrpc.EchoRequest');
		const req = new Req();
		// 100KB string
		var big = '';
		for (var i = 0; i < 1000; i++) {
			big += 'abcdefghij' + 'klmnopqrst' + 'uvwxyz0123' + '4567890abc' + 'defghijklm' +
			       'nopqrstuvw' + 'xyz01234567' + '890abcdefg' + 'hijklmnopq' + 'rstuvwxyz0';
		}
		req.set('message', big);

		client.echo(req).then(
			(resp) => { result = resp.get('message').length; __done(); },
			(err) => { result = 'error: ' + err.message; __done(); }
		);
	`, 10*time.Second)

	v := env.runtime.Get("result")
	if got := v.ToInteger(); got != int64(101000) {
		t.Errorf("expected %v, got %v", int64(101000), got)
	}
}

// T106: Edge case - deeply nested message not applicable to goja-grpc
// (message structure is flat EchoRequest/EchoResponse). Test multiple
// sequential RPCs instead.
func TestEdge_SequentialRPCs(t *testing.T) {
	env := newGrpcTestEnv(t)
	env.runOnLoop(t, `
		const server = grpc.createServer();
		var callCount = 0;
		server.addService('testgrpc.TestService', {
			echo(request) {
				callCount++;
				const Resp = pb.messageType('testgrpc.EchoResponse');
				const resp = new Resp();
				resp.set('message', request.get('message') + ':' + callCount);
				return resp;
			},
			serverStream() {},
			clientStream() {},
			bidiStream() {},
		});
		server.start();

		const client = grpc.createClient('testgrpc.TestService');
		const Req = pb.messageType('testgrpc.EchoRequest');

		var results = [];
		function doRPC(n) {
			if (n > 10) {
				result = results.join(',');
				__done();
				return;
			}
			const req = new Req();
			req.set('message', 'rpc' + n);
			client.echo(req).then((resp) => {
				results.push(resp.get('message'));
				doRPC(n + 1);
			});
		}
		doRPC(1);
	`, 5*time.Second)

	v := env.runtime.Get("result")
	if got := v.String(); got != "rpc1:1,rpc2:2,rpc3:3,rpc4:4,rpc5:5,rpc6:6,rpc7:7,rpc8:8,rpc9:9,rpc10:10" {
		t.Errorf("expected %v, got %v", "rpc1:1,rpc2:2,rpc3:3,rpc4:4,rpc5:5,rpc6:6,rpc7:7,rpc8:8,rpc9:9,rpc10:10", got)
	}
}

// T107: Edge case - null/undefined/wrong-type inputs to all APIs.
func TestEdge_NullInputs(t *testing.T) {
	env := newGrpcTestEnv(t)

	// createClient with null should throw.
	env.mustFail(t, `grpc.createClient(null)`)
	env.mustFail(t, `grpc.createClient(undefined)`)
	env.mustFail(t, `grpc.createClient(123)`)

	// createServer().addService with bad args.
	env.mustFail(t, `grpc.createServer().addService(null, {})`)
	env.mustFail(t, `grpc.createServer().addService('testgrpc.TestService', null)`)

	// metadata.create() then operations with wrong types should not panic.
	v := env.run(t, `
		const md = grpc.metadata.create();
		md.set('key', 'value');
		md.get('key');
	`)
	if got := v.String(); got != "value" {
		t.Errorf("expected %v, got %v", "value", got)
	}
}

// T108: Edge case - event loop shutdown during RPC.
// Cancel context while an RPC is in flight.
func TestEdge_ShutdownDuringRPC(t *testing.T) {
	env := newGrpcTestEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var jsErr error
	_ = env.runtime.Set("__done", env.runtime.ToValue(func(call goja.FunctionCall) goja.Value {
		// Don't actually signal done - we'll cancel the context instead.
		return goja.Undefined()
	}))

	_ = env.loop.Submit(func() {
		_, jsErr = env.runtime.RunString(`
			const server = grpc.createServer();
			server.addService('testgrpc.TestService', {
				echo(request) {
					// Slow handler - returns a promise that never resolves.
					return new Promise(function(resolve) {
						// intentionally never resolve
					});
				},
				serverStream() {},
				clientStream() {},
				bidiStream() {},
			});
			server.start();

			const client = grpc.createClient('testgrpc.TestService');
			const Req = pb.messageType('testgrpc.EchoRequest');
			const req = new Req();
			req.set('message', 'test');

			client.echo(req).then(
				function() { result = 'unexpected'; },
				function(err) { result = 'error:' + err.code; }
			);
		`)
	})

	loopDone := make(chan error, 1)
	go func() {
		loopDone <- env.loop.Run(ctx)
	}()

	// Give the RPC time to be in flight, then cancel.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-loopDone:
		// Loop exited due to context cancellation. This is expected.
	case <-time.After(5 * time.Second):
		t.Fatal("loop didn't exit after context cancel")
	}

	// No panics or deadlocks occurred - that's the test.
	if jsErr != nil {
		t.Errorf("unexpected error: %v", jsErr)
	}
}
