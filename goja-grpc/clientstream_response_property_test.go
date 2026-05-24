package gojagrpc

import "testing"

// TestClientStreamResponsePropertyBecomesDataProperty verifies that the lazy
// `.response` promise on client stream call objects replaces its accessor
// property with a plain data property on first access.
//
// Regression: the replacement previously used Object.Set, which invokes
// [[Set]] and fails with a TypeError on an accessor property that has no
// setter; the error was discarded and the accessor was never replaced, so
// every `.response` read re-executed the getter.
func TestClientStreamResponsePropertyBecomesDataProperty(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) {
				return new Promise(function(resolve, reject) {
					var count = 0;
					function readLoop() {
						call.recv().then(function(result) {
							if (result.done) {
								var EchoResponse = pb.messageType('testgrpc.EchoResponse');
								var resp = new EchoResponse();
								resp.set('message', 'ok');
								resolve(resp);
							} else {
								count++;
								readLoop();
							}
						}).catch(reject);
					}
					readLoop();
				});
			},
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		client.clientStream().then(function(call) {
			var descBefore = Object.getOwnPropertyDescriptor(call, 'response');
			beforeIsAccessor = typeof descBefore.get === 'function';
			var Item = pb.messageType('testgrpc.Item');
			var item = new Item();
			item.set('id', '1');
			item.set('name', 'one');
			call.send(item).then(function() {
				return call.closeSend();
			}).then(function() {
				var p = call.response;
				var descAfter = Object.getOwnPropertyDescriptor(call, 'response');
				afterIsAccessor = typeof descAfter.get === 'function';
				afterIsData = 'value' in descAfter;
				// The data property must mirror the accessor it replaced:
				// enumerable and configurable true (as the accessor was
				// defined), writable false (a getter-only accessor cannot be
				// assigned through, and the data property must behave the
				// same so the object's shape does not depend on whether
				// .response was read).
				afterWritable = descAfter.writable;
				afterEnumerable = descAfter.enumerable;
				afterConfigurable = descAfter.configurable;
				// Assignment must have no effect (sloppy mode), exactly as
				// with the setter-less accessor.
				call.response = 'not a promise';
				afterAssignmentStable = call.response === p;
				return p;
			}).then(function(resp) {
				respMessage = resp.get('message');
				__done();
			}).catch(function(err) {
				respMessage = 'err:' + err.message;
				__done();
			});
		}).catch(function(err) {
			respMessage = 'err:' + err.message;
			__done();
		});
	`, defaultTimeout)

	if value := env.runtime.Get("beforeIsAccessor"); value == nil || !value.ToBoolean() {
		t.Fatalf("expected response to start as an accessor property, got %v", value)
	}
	if value := env.runtime.Get("afterIsAccessor"); value == nil || value.ToBoolean() {
		t.Fatalf("expected response accessor to be replaced after first read, got %v", value)
	}
	if value := env.runtime.Get("afterIsData"); value == nil || !value.ToBoolean() {
		t.Fatalf("expected response to be a data property after first read, got %v", value)
	}
	if value := env.runtime.Get("afterWritable"); value == nil || value.ToBoolean() {
		t.Fatalf("expected response data property to be non-writable (mirroring the getter-only accessor), got %v", value)
	}
	if value := env.runtime.Get("afterEnumerable"); value == nil || !value.ToBoolean() {
		t.Fatalf("expected response data property to be enumerable, got %v", value)
	}
	if value := env.runtime.Get("afterConfigurable"); value == nil || !value.ToBoolean() {
		t.Fatalf("expected response data property to be configurable, got %v", value)
	}
	if value := env.runtime.Get("afterAssignmentStable"); value == nil || !value.ToBoolean() {
		t.Fatalf("expected assignment to .response to have no effect, got %v", value)
	}
	if value := env.runtime.Get("respMessage"); value == nil || value.String() != "ok" {
		t.Fatalf("expected response promise to resolve, got %v", value)
	}
}
