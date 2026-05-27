package gojagrpc

import (
	"runtime"
	"testing"
	"time"
)

// TestClientStreamConstructionFailureReleasesWorker guards the worker-entry
// lifecycle introduced with the abort terminal-precedence fix: the stream
// worker stays in the clientStreamExecutor map after completion (so late
// send/recv observe its terminal) and is removed by the root-disposal
// disposer. The projection that registers that disposer is created eagerly at
// construction, so a construction failure that prevents the stream promise
// from resolving must still remove the worker entry — otherwise it would leak
// until the module dies.
//
// The leak is a scheduling race: the worker's disposal could run before the
// stream-promise resolve effect (which created the projection when the
// projection was registered only inside the resolve projection), leaving the
// executor entry behind. It reproduced probabilistically within a few hundred
// iterations per fresh environment (observed at iteration 100), so this test
// drives 200 fresh environments with a throwing server and requires the
// executor map to be empty after every failed RPC.
func TestClientStreamConstructionFailureReleasesWorker(t *testing.T) {
	for iteration := 0; iteration < 200; iteration++ {
		env := newGrpcTestEnv(t)
		env.runOnLoop(t, `
			var server = grpc.createServer();
			server.addService('testgrpc.TestService', {
				echo: function(request, call) { return null; },
				serverStream: function(request, call) { throw new Error("boom"); },
				clientStream: function(call) { throw new Error("boom"); },
				bidiStream: function(call) { throw new Error("boom"); }
			});
			server.start();
			var client = grpc.createClient('testgrpc.TestService');
			client.clientStream().then(function(call) {
				call.response.catch(function() { __done(); });
			}, function() { __done(); });
		`, defaultTimeout)
		deadline := time.Now().Add(defaultTimeout)
		for syncMapSize(&env.grpcMod.streams.workers) != 0 {
			if time.Now().After(deadline) {
				t.Fatalf(
					"iteration %d: %d stream workers retained after a failed construction",
					iteration,
					syncMapSize(&env.grpcMod.streams.workers),
				)
			}
			runtime.Gosched()
		}
		env.shutdown()
	}
}
