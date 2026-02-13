package gojagrpc

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

// ============================================================================
// T071: Metadata
// ============================================================================

func TestMetadata_Create(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.run(t, `
		var md = grpc.metadata.create();
		typeof md;
	`)
	assert.Equal(t, "object", val.String())
}

func TestMetadata_SetAndGet(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.run(t, `
		var md = grpc.metadata.create();
		md.set('my-key', 'my-value');
	`)

	val := env.run(t, `md.get('my-key')`)
	assert.Equal(t, "my-value", val.String())
}

func TestMetadata_GetMissing(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.run(t, `
		var md = grpc.metadata.create();
		md.get('nonexistent');
	`)
	assert.True(t, goja.IsUndefined(val))
}

func TestMetadata_SetMultipleValues(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.run(t, `
		var md = grpc.metadata.create();
		md.set('multi', 'v1', 'v2', 'v3');
	`)

	// get returns first value
	val := env.run(t, `md.get('multi')`)
	assert.Equal(t, "v1", val.String())

	// getAll returns all values
	all := env.run(t, `
		var arr = md.getAll('multi');
		arr.length;
	`)
	assert.Equal(t, int64(3), all.ToInteger())

	v0 := env.run(t, `arr[0]`)
	v1 := env.run(t, `arr[1]`)
	v2 := env.run(t, `arr[2]`)
	assert.Equal(t, "v1", v0.String())
	assert.Equal(t, "v2", v1.String())
	assert.Equal(t, "v3", v2.String())
}

func TestMetadata_GetAllMissing(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.run(t, `
		var md = grpc.metadata.create();
		md.getAll('nonexistent').length;
	`)
	assert.Equal(t, int64(0), val.ToInteger())
}

func TestMetadata_Delete(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.run(t, `
		var md = grpc.metadata.create();
		md.set('key', 'val');
		md.delete('key');
		md.get('key');
	`)
	assert.True(t, goja.IsUndefined(val))
}

func TestMetadata_DeleteNonexistent(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Should not throw on deleting nonexistent key.
	env.run(t, `
		var md = grpc.metadata.create();
		md.delete('nope');
	`)
}

func TestMetadata_SetOverwrite(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.run(t, `
		var md = grpc.metadata.create();
		md.set('key', 'old');
		md.set('key', 'new');
		md.get('key');
	`)
	assert.Equal(t, "new", val.String())
}

func TestMetadata_ForEach(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.run(t, `
		var md = grpc.metadata.create();
		md.set('a', 'v1');
		md.set('b', 'v2');
		var pairs = [];
		md.forEach(function(value, key) {
			pairs.push(key + '=' + value);
		});
		pairs.sort().join(',');
	`)
	assert.Equal(t, "a=v1,b=v2", val.String())
}

func TestMetadata_ForEachMultipleValues(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.run(t, `
		var md = grpc.metadata.create();
		md.set('key', 'v1', 'v2');
		var pairs = [];
		md.forEach(function(value, key) {
			pairs.push(key + '=' + value);
		});
		pairs.sort().join(',');
	`)
	assert.Equal(t, "key=v1,key=v2", val.String())
}

func TestMetadata_ForEachNotAFunction(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.mustFail(t, `
		var md = grpc.metadata.create();
		md.forEach('not a function');
	`)
}

func TestMetadata_ToObject(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.run(t, `
		var md = grpc.metadata.create();
		md.set('x-request-id', 'abc123');
		md.set('auth', 'token1', 'token2');
		var obj = md.toObject();
	`)

	// x-request-id should have array with one element
	val := env.run(t, `obj['x-request-id'][0]`)
	assert.Equal(t, "abc123", val.String())

	// auth should have array with two elements
	len := env.run(t, `obj['auth'].length`)
	assert.Equal(t, int64(2), len.ToInteger())
}

func TestMetadata_ToObjectEmpty(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.run(t, `
		var md = grpc.metadata.create();
		Object.keys(md.toObject()).length;
	`)
	assert.Equal(t, int64(0), val.ToInteger())
}

func TestMetadata_CaseInsensitive(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// gRPC metadata keys are case-insensitive (stored lowercase).
	val := env.run(t, `
		var md = grpc.metadata.create();
		md.set('My-Key', 'hello');
		md.get('my-key');
	`)
	assert.Equal(t, "hello", val.String())
}

func TestMetadata_CaseInsensitiveGetAll(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.run(t, `
		var md = grpc.metadata.create();
		md.set('X-Custom', 'a', 'b');
		md.getAll('x-custom').length;
	`)
	assert.Equal(t, int64(2), val.ToInteger())
}

func TestMetadata_CaseInsensitiveDelete(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.run(t, `
		var md = grpc.metadata.create();
		md.set('Remove-Me', 'value');
		md.delete('remove-me');
		md.get('remove-me');
	`)
	assert.True(t, goja.IsUndefined(val))
}

func TestMetadata_SetRequiresMinArgs(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.mustFail(t, `
		var md = grpc.metadata.create();
		md.set('only-key');
	`)
}

// ==================== Go <-> JS Conversion ======================

func TestMetadataToGo_Roundtrip(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Create metadata in JS
	env.run(t, `
		var md = grpc.metadata.create();
		md.set('key1', 'val1');
		md.set('key2', 'val2a', 'val2b');
	`)

	// Extract as Go metadata.MD
	mdVal := env.runtime.Get("md")
	goMD := env.grpcMod.metadataToGo(mdVal)
	require.NotNil(t, goMD)

	assert.Equal(t, []string{"val1"}, goMD.Get("key1"))
	assert.Equal(t, []string{"val2a", "val2b"}, goMD.Get("key2"))
}

func TestMetadataToGo_NilInput(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	assert.Nil(t, env.grpcMod.metadataToGo(nil))
	assert.Nil(t, env.grpcMod.metadataToGo(goja.Null()))
	assert.Nil(t, env.grpcMod.metadataToGo(goja.Undefined()))
}

func TestMetadataToGo_NonWrapper(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// A plain object without toObject() should return nil.
	plainObj := env.runtime.NewObject()
	assert.Nil(t, env.grpcMod.metadataToGo(plainObj))
}

func TestMetadataFromGo_Nil(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.grpcMod.metadataFromGo(nil)
	assert.True(t, goja.IsUndefined(val))
}

func TestMetadataFromGo_Roundtrip(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	goMD := metadata.Pairs("alpha", "1", "beta", "2", "beta", "3")
	jsVal := env.grpcMod.metadataFromGo(goMD)

	// Store in JS and verify
	_ = env.runtime.Set("goMD", jsVal)

	val := env.run(t, `goMD.get('alpha')`)
	assert.Equal(t, "1", val.String())

	all := env.run(t, `goMD.getAll('beta').length`)
	assert.Equal(t, int64(2), all.ToInteger())
}

func TestMetadataFromGo_GetAllValues(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	goMD := metadata.Pairs("x", "a", "x", "b", "x", "c")
	jsVal := env.grpcMod.metadataFromGo(goMD)
	_ = env.runtime.Set("goMD", jsVal)

	val := env.run(t, `
		var vals = goMD.getAll('x');
		vals[0] + ',' + vals[1] + ',' + vals[2];
	`)
	assert.Equal(t, "a,b,c", val.String())
}

// =============== Service Resolution Tests (T072 adjacent) ===============

func TestResolveService_Success(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	sd, err := env.grpcMod.resolveService("testgrpc.TestService")
	require.NoError(t, err)
	assert.Equal(t, "TestService", string(sd.Name()))
	assert.Equal(t, 4, sd.Methods().Len())
}

func TestResolveService_NotFound(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	_, err := env.grpcMod.resolveService("nonexistent.Service")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResolveService_NotAService(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// EchoRequest is a message, not a service.
	_, err := env.grpcMod.resolveService("testgrpc.EchoRequest")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a service")
}

func TestLowerFirst(t *testing.T) {
	assert.Equal(t, "echo", lowerFirst("Echo"))
	assert.Equal(t, "serverStream", lowerFirst("ServerStream"))
	assert.Equal(t, "a", lowerFirst("A"))
	assert.Equal(t, "", lowerFirst(""))
}

func TestCreateClient_ServiceMethods(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// createClient should return an object with all service methods.
	env.run(t, `var client = grpc.createClient('testgrpc.TestService')`)

	hasEcho := env.run(t, `typeof client.echo === 'function'`)
	assert.True(t, hasEcho.ToBoolean())

	hasServerStream := env.run(t, `typeof client.serverStream === 'function'`)
	assert.True(t, hasServerStream.ToBoolean())

	hasClientStream := env.run(t, `typeof client.clientStream === 'function'`)
	assert.True(t, hasClientStream.ToBoolean())

	hasBidiStream := env.run(t, `typeof client.bidiStream === 'function'`)
	assert.True(t, hasBidiStream.ToBoolean())
}

func TestCreateClient_InvalidService(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.mustFail(t, `grpc.createClient('nonexistent.Service')`)
}

func TestCreateServer_AddServiceAndStart(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Verify addService returns the server (chaining).
	env.run(t, `
		var server = grpc.createServer();
		var result = server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		result === server;
	`)
}

func TestCreateServer_MissingHandler(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Must provide handlers for ALL methods.
	env.mustFail(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function() {}
		});
	`)
}

func TestCreateServer_HandlerNotAFunction(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.mustFail(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: 'not a function',
			serverStream: function() {},
			clientStream: function() {},
			bidiStream: function() {}
		});
	`)
}

func TestCreateServer_DoubleStart(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.mustFail(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();
		server.start();
	`)
}

func TestCreateServer_InvalidService(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.mustFail(t, `
		var server = grpc.createServer();
		server.addService('nonexistent.Service', {});
	`)
}

func TestCreateServer_HandlersNotObject(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.mustFail(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', 'not an object');
	`)
}
