package gojagrpc

import (
	"testing"

	"github.com/joeycumines/goja"
	"google.golang.org/grpc/codes"
)

// TestServerRecv_NonEOFError proves that a canceled bidi receive settles both
// sides exactly once and retires the server and client supervisor roots.
func TestServerRecv_NonEOFError(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var handlerSettlements = 0;
		var handlerCode = null;
		var clientSettlements = 0;
		var clientCode = null;
		var setupError = null;
		var firstReceivedResolve;
		var firstReceived = new Promise(function(resolve) {
			firstReceivedResolve = resolve;
		});
		function completeWhenSettled() {
			if (handlerSettlements === 1 && clientSettlements === 1) {
				setTimeout(function() { __done(); }, 0);
			}
		}
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {
				return call.recv().then(function(result) {
					if (result.done) throw new Error('request ended before message');
					firstReceivedResolve();
					return call.recv();
				}).then(function() {
					throw new Error('pending recv resolved after cancellation');
				}, function(err) {
					handlerSettlements++;
					handlerCode = err.code;
					completeWhenSettled();
				});
			}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var ctrl = new AbortController();
		var Item = pb.messageType('testgrpc.Item');

		client.bidiStream({ signal: ctrl.signal }).then(function(stream) {
			var bidiMsg = new Item(); bidiMsg.set('id', '1'); bidiMsg.set('name', 'test');
			return stream.send(bidiMsg).then(function() {
				return firstReceived.then(function() {
					stream.recv().then(function() {
						clientSettlements++;
						clientCode = -1;
						completeWhenSettled();
					}, function(err) {
						clientSettlements++;
						clientCode = err.code;
						completeWhenSettled();
					});
					ctrl.abort();
				});
			});
		}).catch(function(err) {
			setupError = String(err);
			__done();
		});
	`, defaultTimeout)

	if got := env.runtime.Get("handlerSettlements").ToInteger(); got != 1 {
		t.Fatalf("handler recv settlements = %d, want 1", got)
	}
	if got := env.runtime.Get("handlerCode").ToInteger(); got !=
		int64(codes.Canceled) {
		t.Fatalf("handler recv code = %d, want Canceled", got)
	}
	if got := env.runtime.Get("clientSettlements").ToInteger(); got != 1 {
		t.Fatalf("client recv settlements = %d, want 1", got)
	}
	if got := env.runtime.Get("clientCode").ToInteger(); got !=
		int64(codes.Canceled) {
		t.Fatalf("client recv code = %d, want Canceled", got)
	}
	setupError := env.runtime.Get("setupError")
	if setupError != nil && !goja.IsNull(setupError) {
		t.Fatalf("setup error = %v", setupError)
	}
	// Close the module to synchronously retire supervisor roots before
	// checking retention. The JS __done() signal fires after both recv
	// callbacks settle, but supervisor root retirement happens
	// asynchronously via the closeAfterAdapter goroutine, which races
	// with env.shutdown(). Module.Close blocks on <-run.done, joining
	// executeCloseRun (stopJoin + disposeOwnerRootsWorker) so all roots
	// are retired before the assertions run. This matches the pattern in
	// server_rollback_contract_test.go.
	if err := env.grpcMod.Close(); err != nil {
		t.Fatalf("module close: %v", err)
	}
	if remaining := supervisorKindCount(env.grpcMod, supervisorServerRPC); remaining != 0 {
		t.Fatalf("retained server RPCs = %d, want 0", remaining)
	}
	if operations := supervisorKindCount(env.grpcMod, supervisorOperation); operations != 0 {
		t.Fatalf("retained client operations = %d, want 0", operations)
	}
}
