package gojagrpc

import (
	"reflect"
	"strings"
	"testing"

	"github.com/dop251/goja"
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
	if got := val.String(); got != "object" {
		t.Errorf("expected %v, got %v", "object", got)
	}
}

func TestMetadata_SetAndGet(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.run(t, `
		var md = grpc.metadata.create();
		md.set('my-key', 'my-value');
	`)

	val := env.run(t, `md.get('my-key')`)
	if got := val.String(); got != "my-value" {
		t.Errorf("expected %v, got %v", "my-value", got)
	}
}

func TestMetadata_GetMissing(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.run(t, `
		var md = grpc.metadata.create();
		md.get('nonexistent');
	`)
	if !(goja.IsUndefined(val)) {
		t.Errorf("expected true")
	}
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
	if got := val.String(); got != "v1" {
		t.Errorf("expected %v, got %v", "v1", got)
	}

	// getAll returns all values
	all := env.run(t, `
		var arr = md.getAll('multi');
		arr.length;
	`)
	if got := all.ToInteger(); got != int64(3) {
		t.Errorf("expected %v, got %v", int64(3), got)
	}

	v0 := env.run(t, `arr[0]`)
	v1 := env.run(t, `arr[1]`)
	v2 := env.run(t, `arr[2]`)
	if got := v0.String(); got != "v1" {
		t.Errorf("expected %v, got %v", "v1", got)
	}
	if got := v1.String(); got != "v2" {
		t.Errorf("expected %v, got %v", "v2", got)
	}
	if got := v2.String(); got != "v3" {
		t.Errorf("expected %v, got %v", "v3", got)
	}
}

func TestMetadata_GetAllMissing(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.run(t, `
		var md = grpc.metadata.create();
		md.getAll('nonexistent').length;
	`)
	if got := val.ToInteger(); got != int64(0) {
		t.Errorf("expected %v, got %v", int64(0), got)
	}
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
	if !(goja.IsUndefined(val)) {
		t.Errorf("expected true")
	}
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
	if got := val.String(); got != "new" {
		t.Errorf("expected %v, got %v", "new", got)
	}
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
	if got := val.String(); got != "a=v1,b=v2" {
		t.Errorf("expected %v, got %v", "a=v1,b=v2", got)
	}
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
	if got := val.String(); got != "key=v1,key=v2" {
		t.Errorf("expected %v, got %v", "key=v1,key=v2", got)
	}
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
	if got := val.String(); got != "abc123" {
		t.Errorf("expected %v, got %v", "abc123", got)
	}

	// auth should have array with two elements
	len := env.run(t, `obj['auth'].length`)
	if got := len.ToInteger(); got != int64(2) {
		t.Errorf("expected %v, got %v", int64(2), got)
	}
}

func TestMetadata_ToObjectEmpty(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.run(t, `
		var md = grpc.metadata.create();
		Object.keys(md.toObject()).length;
	`)
	if got := val.ToInteger(); got != int64(0) {
		t.Errorf("expected %v, got %v", int64(0), got)
	}
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
	if got := val.String(); got != "hello" {
		t.Errorf("expected %v, got %v", "hello", got)
	}
}

func TestMetadata_CaseInsensitiveGetAll(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.run(t, `
		var md = grpc.metadata.create();
		md.set('X-Custom', 'a', 'b');
		md.getAll('x-custom').length;
	`)
	if got := val.ToInteger(); got != int64(2) {
		t.Errorf("expected %v, got %v", int64(2), got)
	}
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
	if !(goja.IsUndefined(val)) {
		t.Errorf("expected true")
	}
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
	if goMD == nil {
		t.Fatalf("expected non-nil")
	}

	if !reflect.DeepEqual(goMD.Get("key1"), []string{"val1"}) {
		t.Errorf("expected %v, got %v", []string{"val1"}, goMD.Get("key1"))
	}
	if !reflect.DeepEqual(goMD.Get("key2"), []string{"val2a", "val2b"}) {
		t.Errorf("expected %v, got %v", []string{"val2a", "val2b"}, goMD.Get("key2"))
	}
}

func TestMetadataToGo_NilInput(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	if env.grpcMod.metadataToGo(nil) != nil {
		t.Errorf("expected nil, got %v", env.grpcMod.metadataToGo(nil))
	}
	if env.grpcMod.metadataToGo(goja.Null()) != nil {
		t.Errorf("expected nil, got %v", env.grpcMod.metadataToGo(goja.Null()))
	}
	if env.grpcMod.metadataToGo(goja.Undefined()) != nil {
		t.Errorf("expected nil, got %v", env.grpcMod.metadataToGo(goja.Undefined()))
	}
}

func TestMetadataToGo_NonWrapper(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// A plain object without toObject() should return nil.
	plainObj := env.runtime.NewObject()
	if env.grpcMod.metadataToGo(plainObj) != nil {
		t.Errorf("expected nil, got %v", env.grpcMod.metadataToGo(plainObj))
	}
}

func TestMetadataFromGo_Nil(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.grpcMod.metadataFromGo(nil)
	if !(goja.IsUndefined(val)) {
		t.Errorf("expected true")
	}
}

func TestMetadataFromGo_Roundtrip(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	goMD := metadata.Pairs("alpha", "1", "beta", "2", "beta", "3")
	jsVal := env.grpcMod.metadataFromGo(goMD)

	// Store in JS and verify
	_ = env.runtime.Set("goMD", jsVal)

	val := env.run(t, `goMD.get('alpha')`)
	if got := val.String(); got != "1" {
		t.Errorf("expected %v, got %v", "1", got)
	}

	all := env.run(t, `goMD.getAll('beta').length`)
	if got := all.ToInteger(); got != int64(2) {
		t.Errorf("expected %v, got %v", int64(2), got)
	}
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
	if got := val.String(); got != "a,b,c" {
		t.Errorf("expected %v, got %v", "a,b,c", got)
	}
}

// =============== Service Resolution Tests (T072 adjacent) ===============

func TestResolveService_Success(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	sd, err := env.grpcMod.resolveService("testgrpc.TestService")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := string(sd.Name()); got != "TestService" {
		t.Errorf("expected %v, got %v", "TestService", got)
	}
	if got := sd.Methods().Len(); got != 4 {
		t.Errorf("expected %v, got %v", 4, got)
	}
}

func TestResolveService_NotFound(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	_, err := env.grpcMod.resolveService("nonexistent.Service")
	if err == nil {
		t.Fatalf("expected an error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected %q to contain %q", err.Error(), "not found")
	}
}

func TestResolveService_NotAService(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// EchoRequest is a message, not a service.
	_, err := env.grpcMod.resolveService("testgrpc.EchoRequest")
	if err == nil {
		t.Fatalf("expected an error")
	}
	if !strings.Contains(err.Error(), "not a service") {
		t.Errorf("expected %q to contain %q", err.Error(), "not a service")
	}
}

func TestLowerFirst(t *testing.T) {
	if got := lowerFirst("Echo"); got != "echo" {
		t.Errorf("expected %v, got %v", "echo", got)
	}
	if got := lowerFirst("ServerStream"); got != "serverStream" {
		t.Errorf("expected %v, got %v", "serverStream", got)
	}
	if got := lowerFirst("A"); got != "a" {
		t.Errorf("expected %v, got %v", "a", got)
	}
	if got := lowerFirst(""); got != "" {
		t.Errorf("expected %v, got %v", "", got)
	}
}

func TestCreateClient_ServiceMethods(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// createClient should return an object with all service methods.
	env.run(t, `var client = grpc.createClient('testgrpc.TestService')`)

	hasEcho := env.run(t, `typeof client.echo === 'function'`)
	if !(hasEcho.ToBoolean()) {
		t.Errorf("expected true")
	}

	hasServerStream := env.run(t, `typeof client.serverStream === 'function'`)
	if !(hasServerStream.ToBoolean()) {
		t.Errorf("expected true")
	}

	hasClientStream := env.run(t, `typeof client.clientStream === 'function'`)
	if !(hasClientStream.ToBoolean()) {
		t.Errorf("expected true")
	}

	hasBidiStream := env.run(t, `typeof client.bidiStream === 'function'`)
	if !(hasBidiStream.ToBoolean()) {
		t.Errorf("expected true")
	}
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
