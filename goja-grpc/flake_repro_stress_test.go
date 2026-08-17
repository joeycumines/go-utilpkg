package gojagrpc

import (
	"testing"
	"time"

	"github.com/joeycumines/goja"
)

// TestServerStreamSyncThrowRecvRejectsServerErrorStress is a regression guard
// for the pre-existing "expected 13, got 1" race: a server-stream handler
// that throws synchronously must reject the client recv promise with the
// server's error (Internal, 13) — never with context.Canceled (1).
//
// The old failure mode: the worker's header loop saw the server error and
// called failLocal, which stopped (canceled) the operation context BEFORE the
// worker terminal was published. The ctx-done watcher then treated that
// internal release cancel as an unexplained local cancellation, published
// context.Canceled as the worker terminal (racing the real error), while the
// transport's receive path simultaneously claimed the terminal as Canceled
// via callerCancel. The race reproduced within ~20 iterations under -race
// before the failLocal ordering fix; 100 iterations leave a comfortable
// margin while keeping the test fast.
func TestServerStreamSyncThrowRecvRejectsServerErrorStress(t *testing.T) {
	for iteration := range 100 {
		env := newGrpcTestEnv(t)
		env.runOnLoop(t, `
			var server = grpc.createServer();
			server.addService('testgrpc.TestService', {
				echo: function(request, call) { return null; },
				serverStream: function(request, call) {
					throw new Error("sync server stream error");
				},
				clientStream: function(call) { return null; },
				bidiStream: function(call) {}
			});
			server.start();
			var client = grpc.createClient('testgrpc.TestService');
			var EchoRequest = pb.messageType('testgrpc.EchoRequest');
			var req = new EchoRequest();
			req.set('message', 'test');
			var error;
			var which = "none";
			client.serverStream(req).then(function(stream) {
				which = "reader-ok";
				stream.recv().then(function() {
					__done();
				}).catch(function(err) {
					which = "recv";
					error = { code: err.code, message: err.message };
					__done();
				});
			}).catch(function(err) {
				which = "reader";
				error = { code: err.code, message: err.message };
				__done();
			});
		`, 5*time.Second)
		result := env.runtime.Get("error")
		which := env.runtime.Get("which")
		env.shutdown()
		if result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
			t.Fatalf("iteration %d: expected non-nil error (which=%v)", iteration, which)
		}
		resultObj := result.Export().(map[string]any)
		if got := resultObj["code"]; got != int64(13) {
			t.Fatalf("iteration %d: expected 13, got %v (%v) via %v", iteration, got, resultObj["message"], which)
		}
	}
}
