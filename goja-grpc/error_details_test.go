package gojagrpc

import (
	"testing"
)

// ============================================================================
// T176: Error details round-trip tests
// ============================================================================

func TestErrorDetails_UnaryRoundTrip(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				// Create a detail message.
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var detail = new EchoResponse();
				detail.set('message', 'extra info');
				detail.set('code', 42);

				// Throw error with details.
				throw grpc.status.createError(3, 'bad request', [detail]);
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

		var errorCode, errorMessage, detailCount, detailMsg, detailCode;
		client.echo(req).catch(function(err) {
			errorCode = err.code;
			errorMessage = err.message;
			detailCount = err.details.length;
			if (detailCount > 0) {
				detailMsg = err.details[0].get('message');
				detailCode = err.details[0].get('code');
			}
			__done();
		});
	`, defaultTimeout)

	if got := env.runtime.Get("errorCode").ToInteger(); got != int64(3) {
		t.Errorf("errorCode: expected %v, got %v", int64(3), got)
	}
	if got := env.runtime.Get("errorMessage").String(); got != "bad request" {
		t.Errorf("errorMessage: expected %q, got %q", "bad request", got)
	}
	if got := env.runtime.Get("detailCount").ToInteger(); got != int64(1) {
		t.Errorf("detailCount: expected %v, got %v", int64(1), got)
	}
	if got := env.runtime.Get("detailMsg").String(); got != "extra info" {
		t.Errorf("detailMsg: expected %q, got %q", "extra info", got)
	}
	if got := env.runtime.Get("detailCode").ToInteger(); got != int64(42) {
		t.Errorf("detailCode: expected %v, got %v", int64(42), got)
	}
}

func TestErrorDetails_MultipleDetails(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var Item = pb.messageType('testgrpc.Item');

				var d1 = new Item();
				d1.set('id', 'id-1');
				d1.set('name', 'first');

				var d2 = new Item();
				d2.set('id', 'id-2');
				d2.set('name', 'second');

				throw grpc.status.createError(9, 'precondition', [d1, d2]);
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

		var detailCount, d1Id, d1Name, d2Id, d2Name;
		client.echo(req).catch(function(err) {
			detailCount = err.details.length;
			if (detailCount >= 2) {
				d1Id = err.details[0].get('id');
				d1Name = err.details[0].get('name');
				d2Id = err.details[1].get('id');
				d2Name = err.details[1].get('name');
			}
			__done();
		});
	`, defaultTimeout)

	if got := env.runtime.Get("detailCount").ToInteger(); got != int64(2) {
		t.Errorf("detailCount: expected %v, got %v", int64(2), got)
	}
	if got := env.runtime.Get("d1Id").String(); got != "id-1" {
		t.Errorf("d1Id: expected %q, got %q", "id-1", got)
	}
	if got := env.runtime.Get("d1Name").String(); got != "first" {
		t.Errorf("d1Name: expected %q, got %q", "first", got)
	}
	if got := env.runtime.Get("d2Id").String(); got != "id-2" {
		t.Errorf("d2Id: expected %q, got %q", "id-2", got)
	}
	if got := env.runtime.Get("d2Name").String(); got != "second" {
		t.Errorf("d2Name: expected %q, got %q", "second", got)
	}
}

func TestErrorDetails_NoDetails(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				throw grpc.status.createError(5, 'not found');
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

		var errorCode, detailCount;
		client.echo(req).catch(function(err) {
			errorCode = err.code;
			detailCount = err.details.length;
			__done();
		});
	`, defaultTimeout)

	if got := env.runtime.Get("errorCode").ToInteger(); got != int64(5) {
		t.Errorf("errorCode: expected %v, got %v", int64(5), got)
	}
	if got := env.runtime.Get("detailCount").ToInteger(); got != int64(0) {
		t.Errorf("detailCount: expected %v, got %v", int64(0), got)
	}
}

func TestErrorDetails_EmptyDetailsArray(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				throw grpc.status.createError(3, 'bad request', []);
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

		var detailCount;
		client.echo(req).catch(function(err) {
			detailCount = err.details.length;
			__done();
		});
	`, defaultTimeout)

	if got := env.runtime.Get("detailCount").ToInteger(); got != int64(0) {
		t.Errorf("detailCount: expected %v, got %v", int64(0), got)
	}
}

func TestErrorDetails_DetailsProperty_ExistsOnAllErrors(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Details array should exist even for plain errors.
	env.runOnLoop(t, `
		var err1 = grpc.status.createError(5, 'not found');
		var err2 = grpc.status.createError(3, 'bad', []);
		var hasDetails1 = Array.isArray(err1.details);
		var hasDetails2 = Array.isArray(err2.details);
		var len1 = err1.details.length;
		var len2 = err2.details.length;
		__done();
	`, defaultTimeout)

	if got := env.runtime.Get("hasDetails1").ToBoolean(); !got {
		t.Errorf("hasDetails1: expected true")
	}
	if got := env.runtime.Get("hasDetails2").ToBoolean(); !got {
		t.Errorf("hasDetails2: expected true")
	}
	if got := env.runtime.Get("len1").ToInteger(); got != int64(0) {
		t.Errorf("len1: expected %v, got %v", int64(0), got)
	}
	if got := env.runtime.Get("len2").ToInteger(); got != int64(0) {
		t.Errorf("len2: expected %v, got %v", int64(0), got)
	}
}

func TestErrorDetails_ServerStreamWithDetails(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {
				var Item = pb.messageType('testgrpc.Item');
				var detail = new Item();
				detail.set('id', 'err-id');
				detail.set('name', 'err-name');
				throw grpc.status.createError(10, 'aborted', [detail]);
			},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test');

		var errorCode, detailCount, detailId;
		client.serverStream(req).then(function(stream) {
			return stream.recv();
		}).catch(function(err) {
			errorCode = err.code;
			detailCount = err.details.length;
			if (detailCount > 0) {
				detailId = err.details[0].get('id');
			}
			__done();
		});
	`, defaultTimeout)

	if got := env.runtime.Get("errorCode").ToInteger(); got != int64(10) {
		t.Errorf("errorCode: expected %v, got %v", int64(10), got)
	}
	if got := env.runtime.Get("detailCount").ToInteger(); got != int64(1) {
		t.Errorf("detailCount: expected %v, got %v", int64(1), got)
	}
	if got := env.runtime.Get("detailId").String(); got != "err-id" {
		t.Errorf("detailId: expected %q, got %q", "err-id", got)
	}
}
