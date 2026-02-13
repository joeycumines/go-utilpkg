package gojagrpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

	assert.Equal(t, int64(3), env.runtime.Get("errorCode").ToInteger())
	assert.Equal(t, "bad request", env.runtime.Get("errorMessage").String())
	assert.Equal(t, int64(1), env.runtime.Get("detailCount").ToInteger())
	assert.Equal(t, "extra info", env.runtime.Get("detailMsg").String())
	assert.Equal(t, int64(42), env.runtime.Get("detailCode").ToInteger())
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

	assert.Equal(t, int64(2), env.runtime.Get("detailCount").ToInteger())
	assert.Equal(t, "id-1", env.runtime.Get("d1Id").String())
	assert.Equal(t, "first", env.runtime.Get("d1Name").String())
	assert.Equal(t, "id-2", env.runtime.Get("d2Id").String())
	assert.Equal(t, "second", env.runtime.Get("d2Name").String())
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

	assert.Equal(t, int64(5), env.runtime.Get("errorCode").ToInteger())
	assert.Equal(t, int64(0), env.runtime.Get("detailCount").ToInteger())
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

	assert.Equal(t, int64(0), env.runtime.Get("detailCount").ToInteger())
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

	assert.True(t, env.runtime.Get("hasDetails1").ToBoolean())
	assert.True(t, env.runtime.Get("hasDetails2").ToBoolean())
	assert.Equal(t, int64(0), env.runtime.Get("len1").ToInteger())
	assert.Equal(t, int64(0), env.runtime.Get("len2").ToInteger())
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

	assert.Equal(t, int64(10), env.runtime.Get("errorCode").ToInteger())
	assert.Equal(t, int64(1), env.runtime.Get("detailCount").ToInteger())
	assert.Equal(t, "err-id", env.runtime.Get("detailId").String())
}
