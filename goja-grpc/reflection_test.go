package gojagrpc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// T204: Reflection - listServices
// ============================================================================

func TestReflection_ListServices(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		// Enable reflection from JS after server start.
		grpc.enableReflection();

		var reflClient = grpc.createReflectionClient();
		var result;
		var error;
		reflClient.listServices().then(function(services) {
			result = [];
			for (var i = 0; i < services.length; i++) {
				result.push(services[i]);
			}
			__done();
		}).catch(function(err) {
			error = err;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	services := result.Export()
	serviceList, ok := services.([]any)
	require.True(t, ok, "result should be a slice, got %T", services)

	// Should contain our TestService and the reflection service itself.
	var names []string
	for _, s := range serviceList {
		names = append(names, s.(string))
	}
	assert.Contains(t, names, "testgrpc.TestService")
	assert.Contains(t, names, "grpc.reflection.v1.ServerReflection")
}

// ============================================================================
// T205: Reflection - describeService
// ============================================================================

func TestReflection_DescribeService(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();
		grpc.enableReflection();

		var reflClient = grpc.createReflectionClient();
		var result;
		var error;
		reflClient.describeService('testgrpc.TestService').then(function(desc) {
			result = desc;
			__done();
		}).catch(function(err) {
			error = err;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	desc := result.Export().(map[string]any)

	assert.Equal(t, "testgrpc.TestService", desc["name"])

	methods, ok := desc["methods"].([]any)
	require.True(t, ok, "methods should be array, got %T", desc["methods"])
	assert.Len(t, methods, 4)

	// Verify each method.
	methodMap := make(map[string]map[string]any)
	for _, m := range methods {
		mObj := m.(map[string]any)
		methodMap[mObj["name"].(string)] = mObj
	}

	// Echo - unary
	echo := methodMap["Echo"]
	require.NotNil(t, echo, "Echo method not found")
	assert.Equal(t, "testgrpc.EchoRequest", echo["inputType"])
	assert.Equal(t, "testgrpc.EchoResponse", echo["outputType"])
	assert.Equal(t, false, echo["clientStreaming"])
	assert.Equal(t, false, echo["serverStreaming"])

	// ServerStream - server streaming
	ss := methodMap["ServerStream"]
	require.NotNil(t, ss, "ServerStream method not found")
	assert.Equal(t, false, ss["clientStreaming"])
	assert.Equal(t, true, ss["serverStreaming"])

	// ClientStream - client streaming
	cs := methodMap["ClientStream"]
	require.NotNil(t, cs, "ClientStream method not found")
	assert.Equal(t, true, cs["clientStreaming"])
	assert.Equal(t, false, cs["serverStreaming"])

	// BidiStream - bidi streaming
	bidi := methodMap["BidiStream"]
	require.NotNil(t, bidi, "BidiStream method not found")
	assert.Equal(t, true, bidi["clientStreaming"])
	assert.Equal(t, true, bidi["serverStreaming"])
}

// ============================================================================
// T206: Reflection - describeType
// ============================================================================

func TestReflection_DescribeType(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();
		grpc.enableReflection();

		var reflClient = grpc.createReflectionClient();
		var result;
		var error;
		reflClient.describeType('testgrpc.EchoResponse').then(function(desc) {
			result = desc;
			__done();
		}).catch(function(err) {
			error = err;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	desc := result.Export().(map[string]any)

	assert.Equal(t, "testgrpc.EchoResponse", desc["name"])

	fields, ok := desc["fields"].([]any)
	require.True(t, ok, "fields should be array, got %T", desc["fields"])
	assert.Len(t, fields, 2)

	fieldMap := make(map[string]map[string]any)
	for _, f := range fields {
		fObj := f.(map[string]any)
		fieldMap[fObj["name"].(string)] = fObj
	}

	msgField := fieldMap["message"]
	require.NotNil(t, msgField, "message field not found")
	assert.Equal(t, "string", msgField["type"])
	assert.Equal(t, int64(1), msgField["number"])
	assert.Equal(t, false, msgField["repeated"])
	assert.Equal(t, false, msgField["map"])

	codeField := fieldMap["code"]
	require.NotNil(t, codeField, "code field not found")
	assert.Equal(t, "int32", codeField["type"])
	assert.Equal(t, int64(2), codeField["number"])
}

// ============================================================================
// T207: Reflection - error handling
// ============================================================================

func TestReflection_DescribeService_NotFound(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();
		grpc.enableReflection();

		var reflClient = grpc.createReflectionClient();
		var result;
		var error;
		reflClient.describeService('nonexistent.Service').then(function(desc) {
			result = desc;
			__done();
		}).catch(function(err) {
			error = String(err);
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	require.False(t, errVal == nil || isGojaUndefined(errVal), "expected error but got none")
	assert.Contains(t, errVal.String(), "not found")
}

func TestReflection_DescribeType_NotFound(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();
		grpc.enableReflection();

		var reflClient = grpc.createReflectionClient();
		var result;
		var error;
		reflClient.describeType('nonexistent.Type').then(function(desc) {
			result = desc;
			__done();
		}).catch(function(err) {
			error = String(err);
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	require.False(t, errVal == nil || isGojaUndefined(errVal), "expected error but got none")
	assert.Contains(t, errVal.String(), "not found")
}

// ============================================================================
// T208: Reflection - enableReflection from JS
// ============================================================================

func TestReflection_EnableFromJS(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();
		grpc.enableReflection();

		var reflClient = grpc.createReflectionClient();
		var result;
		var error;
		reflClient.listServices().then(function(services) {
			result = [];
			for (var i = 0; i < services.length; i++) {
				result.push(services[i]);
			}
			__done();
		}).catch(function(err) {
			error = err;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	services := result.Export()
	serviceList, ok := services.([]any)
	require.True(t, ok, "result should be a slice, got %T", services)

	var names []string
	for _, s := range serviceList {
		names = append(names, s.(string))
	}
	assert.Contains(t, names, "testgrpc.TestService")
}

// ============================================================================
// Reflection - Item message type with multiple fields
// ============================================================================

func TestReflection_DescribeType_Item(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();
		grpc.enableReflection();

		var reflClient = grpc.createReflectionClient();
		var result;
		var error;
		reflClient.describeType('testgrpc.Item').then(function(desc) {
			result = desc;
			__done();
		}).catch(function(err) {
			error = err;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	desc := result.Export().(map[string]any)

	assert.Equal(t, "testgrpc.Item", desc["name"])
	fields := desc["fields"].([]any)
	assert.Len(t, fields, 2)

	fieldMap := make(map[string]map[string]any)
	for _, f := range fields {
		fObj := f.(map[string]any)
		fieldMap[fObj["name"].(string)] = fObj
	}
	assert.Contains(t, fieldMap, "id")
	assert.Contains(t, fieldMap, "name")
}

// ============================================================================
// Reflection - chained operations (list then describe)
// ============================================================================

func TestReflection_ChainedListAndDescribe(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();
		grpc.enableReflection();

		var reflClient = grpc.createReflectionClient();
		var result;
		var error;
		reflClient.listServices().then(function(services) {
			for (var i = 0; i < services.length; i++) {
				if (services[i] === 'testgrpc.TestService') {
					return reflClient.describeService(services[i]);
				}
			}
			throw new Error('TestService not found in list');
		}).then(function(desc) {
			result = desc;
			__done();
		}).catch(function(err) {
			error = err;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	desc := result.Export().(map[string]any)
	assert.Equal(t, "testgrpc.TestService", desc["name"])
	methods := desc["methods"].([]any)
	assert.Len(t, methods, 4)
}

// ============================================================================
// Reflection - full discovery: service -> method -> input type
// ============================================================================

func TestReflection_FullDiscoveryWorkflow(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();
		grpc.enableReflection();

		var reflClient = grpc.createReflectionClient();
		var result;
		var error;
		reflClient.describeService('testgrpc.TestService').then(function(svc) {
			var echoMethod;
			for (var i = 0; i < svc.methods.length; i++) {
				if (svc.methods[i].name === 'Echo') {
					echoMethod = svc.methods[i];
					break;
				}
			}
			if (!echoMethod) throw new Error('Echo not found');
			return reflClient.describeType(echoMethod.inputType);
		}).then(function(typeDesc) {
			result = typeDesc;
			__done();
		}).catch(function(err) {
			error = err;
			__done();
		});
	`, 10*time.Second)

	errVal := env.runtime.Get("error")
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	desc := result.Export().(map[string]any)
	assert.Equal(t, "testgrpc.EchoRequest", desc["name"])
	fields := desc["fields"].([]any)
	assert.Len(t, fields, 1)
	field := fields[0].(map[string]any)
	assert.Equal(t, "message", field["name"])
	assert.Equal(t, "string", field["type"])
}

// ============================================================================
// Reflection - EnableReflection from Go (called before loop starts)
// ============================================================================

func TestReflection_EnableFromGo(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Enable reflection from Go before the JS runs.
	env.grpcMod.EnableReflection()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var reflClient = grpc.createReflectionClient();
		var result;
		var error;
		reflClient.listServices().then(function(services) {
			result = [];
			for (var i = 0; i < services.length; i++) {
				result.push(services[i]);
			}
			__done();
		}).catch(function(err) {
			error = err;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	services := result.Export()
	serviceList, ok := services.([]any)
	require.True(t, ok, "result should be a slice, got %T", services)

	var names []string
	for _, s := range serviceList {
		names = append(names, s.(string))
	}
	assert.Contains(t, names, "testgrpc.TestService")
	assert.Contains(t, names, "grpc.reflection.v1.ServerReflection")
}

// ============================================================================
// Reflection - describe the reflection service itself
// ============================================================================

func TestReflection_DescribeReflectionService(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();
		grpc.enableReflection();

		var reflClient = grpc.createReflectionClient();
		var result;
		var error;
		reflClient.describeService('grpc.reflection.v1.ServerReflection').then(function(desc) {
			result = desc;
			__done();
		}).catch(function(err) {
			error = err;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	desc := result.Export().(map[string]any)
	assert.Equal(t, "grpc.reflection.v1.ServerReflection", desc["name"])
	methods := desc["methods"].([]any)
	assert.Greater(t, len(methods), 0)
}
