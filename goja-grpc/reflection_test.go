package gojagrpc

import (
	"slices"
	"strings"
	"testing"
	"time"
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
	if !(errVal == nil || isGojaUndefined(errVal)) {
		t.Fatalf("unexpected error: %v", errVal)
	}

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	services := result.Export()
	serviceList, ok := services.([]any)
	if !(ok) {
		t.Fatalf("result should be a slice, got %T", services)
	}

	// Should contain our TestService and the reflection service itself.
	var names []string
	for _, s := range serviceList {
		names = append(names, s.(string))
	}
	if !slices.Contains(names, "testgrpc.TestService") {
		t.Errorf("expected %q to contain %q", names, "testgrpc.TestService")
	}
	if !slices.Contains(names, "grpc.reflection.v1.ServerReflection") {
		t.Errorf("expected %q to contain %q", names, "grpc.reflection.v1.ServerReflection")
	}
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
	if !(errVal == nil || isGojaUndefined(errVal)) {
		t.Fatalf("unexpected error: %v", errVal)
	}

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	desc := result.Export().(map[string]any)

	if got := desc["name"]; got != "testgrpc.TestService" {
		t.Errorf("expected %v, got %v", "testgrpc.TestService", got)
	}

	methods, ok := desc["methods"].([]any)
	if !(ok) {
		t.Fatalf("methods should be array, got %T", desc["methods"])
	}
	if got := len(methods); got != 4 {
		t.Errorf("expected len %d, got %d", 4, got)
	}

	// Verify each method.
	methodMap := make(map[string]map[string]any)
	for _, m := range methods {
		mObj := m.(map[string]any)
		methodMap[mObj["name"].(string)] = mObj
	}

	// Echo - unary
	echo := methodMap["Echo"]
	if echo == nil {
		t.Fatalf("Echo method not found")
	}
	if got := echo["inputType"]; got != "testgrpc.EchoRequest" {
		t.Errorf("expected %v, got %v", "testgrpc.EchoRequest", got)
	}
	if got := echo["outputType"]; got != "testgrpc.EchoResponse" {
		t.Errorf("expected %v, got %v", "testgrpc.EchoResponse", got)
	}
	if got := echo["clientStreaming"]; got != false {
		t.Errorf("expected %v, got %v", false, got)
	}
	if got := echo["serverStreaming"]; got != false {
		t.Errorf("expected %v, got %v", false, got)
	}

	// ServerStream - server streaming
	ss := methodMap["ServerStream"]
	if ss == nil {
		t.Fatalf("ServerStream method not found")
	}
	if got := ss["clientStreaming"]; got != false {
		t.Errorf("expected %v, got %v", false, got)
	}
	if got := ss["serverStreaming"]; got != true {
		t.Errorf("expected %v, got %v", true, got)
	}

	// ClientStream - client streaming
	cs := methodMap["ClientStream"]
	if cs == nil {
		t.Fatalf("ClientStream method not found")
	}
	if got := cs["clientStreaming"]; got != true {
		t.Errorf("expected %v, got %v", true, got)
	}
	if got := cs["serverStreaming"]; got != false {
		t.Errorf("expected %v, got %v", false, got)
	}

	// BidiStream - bidi streaming
	bidi := methodMap["BidiStream"]
	if bidi == nil {
		t.Fatalf("BidiStream method not found")
	}
	if got := bidi["clientStreaming"]; got != true {
		t.Errorf("expected %v, got %v", true, got)
	}
	if got := bidi["serverStreaming"]; got != true {
		t.Errorf("expected %v, got %v", true, got)
	}
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
	if !(errVal == nil || isGojaUndefined(errVal)) {
		t.Fatalf("unexpected error: %v", errVal)
	}

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	desc := result.Export().(map[string]any)

	if got := desc["name"]; got != "testgrpc.EchoResponse" {
		t.Errorf("expected %v, got %v", "testgrpc.EchoResponse", got)
	}

	fields, ok := desc["fields"].([]any)
	if !(ok) {
		t.Fatalf("fields should be array, got %T", desc["fields"])
	}
	if got := len(fields); got != 2 {
		t.Errorf("expected len %d, got %d", 2, got)
	}

	fieldMap := make(map[string]map[string]any)
	for _, f := range fields {
		fObj := f.(map[string]any)
		fieldMap[fObj["name"].(string)] = fObj
	}

	msgField := fieldMap["message"]
	if msgField == nil {
		t.Fatalf("message field not found")
	}
	if got := msgField["type"]; got != "string" {
		t.Errorf("expected %v, got %v", "string", got)
	}
	if got := msgField["number"]; got != int64(1) {
		t.Errorf("expected %v, got %v", int64(1), got)
	}
	if got := msgField["repeated"]; got != false {
		t.Errorf("expected %v, got %v", false, got)
	}
	if got := msgField["map"]; got != false {
		t.Errorf("expected %v, got %v", false, got)
	}

	codeField := fieldMap["code"]
	if codeField == nil {
		t.Fatalf("code field not found")
	}
	if got := codeField["type"]; got != "int32" {
		t.Errorf("expected %v, got %v", "int32", got)
	}
	if got := codeField["number"]; got != int64(2) {
		t.Errorf("expected %v, got %v", int64(2), got)
	}
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
	if errVal == nil || isGojaUndefined(errVal) {
		t.Fatalf("expected error but got none")
	}
	if !strings.Contains(errVal.String(), "not found") {
		t.Errorf("expected %q to contain %q", errVal.String(), "not found")
	}
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
	if errVal == nil || isGojaUndefined(errVal) {
		t.Fatalf("expected error but got none")
	}
	if !strings.Contains(errVal.String(), "not found") {
		t.Errorf("expected %q to contain %q", errVal.String(), "not found")
	}
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
	if !(errVal == nil || isGojaUndefined(errVal)) {
		t.Fatalf("unexpected error: %v", errVal)
	}

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	services := result.Export()
	serviceList, ok := services.([]any)
	if !(ok) {
		t.Fatalf("result should be a slice, got %T", services)
	}

	var names []string
	for _, s := range serviceList {
		names = append(names, s.(string))
	}
	if !slices.Contains(names, "testgrpc.TestService") {
		t.Errorf("expected %q to contain %q", names, "testgrpc.TestService")
	}
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
	if !(errVal == nil || isGojaUndefined(errVal)) {
		t.Fatalf("unexpected error: %v", errVal)
	}

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	desc := result.Export().(map[string]any)

	if got := desc["name"]; got != "testgrpc.Item" {
		t.Errorf("expected %v, got %v", "testgrpc.Item", got)
	}
	fields := desc["fields"].([]any)
	if got := len(fields); got != 2 {
		t.Errorf("expected len %d, got %d", 2, got)
	}

	fieldMap := make(map[string]map[string]any)
	for _, f := range fields {
		fObj := f.(map[string]any)
		fieldMap[fObj["name"].(string)] = fObj
	}
	if _, ok := fieldMap["id"]; !ok {
		t.Errorf("expected fieldMap to contain key %q", "id")
	}
	if _, ok := fieldMap["name"]; !ok {
		t.Errorf("expected fieldMap to contain key %q", "name")
	}
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
	if !(errVal == nil || isGojaUndefined(errVal)) {
		t.Fatalf("unexpected error: %v", errVal)
	}

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	desc := result.Export().(map[string]any)
	if got := desc["name"]; got != "testgrpc.TestService" {
		t.Errorf("expected %v, got %v", "testgrpc.TestService", got)
	}
	methods := desc["methods"].([]any)
	if got := len(methods); got != 4 {
		t.Errorf("expected len %d, got %d", 4, got)
	}
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
	if !(errVal == nil || isGojaUndefined(errVal)) {
		t.Fatalf("unexpected error: %v", errVal)
	}

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	desc := result.Export().(map[string]any)
	if got := desc["name"]; got != "testgrpc.EchoRequest" {
		t.Errorf("expected %v, got %v", "testgrpc.EchoRequest", got)
	}
	fields := desc["fields"].([]any)
	if got := len(fields); got != 1 {
		t.Errorf("expected len %d, got %d", 1, got)
	}
	field := fields[0].(map[string]any)
	if got := field["name"]; got != "message" {
		t.Errorf("expected %v, got %v", "message", got)
	}
	if got := field["type"]; got != "string" {
		t.Errorf("expected %v, got %v", "string", got)
	}
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
	if !(errVal == nil || isGojaUndefined(errVal)) {
		t.Fatalf("unexpected error: %v", errVal)
	}

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	services := result.Export()
	serviceList, ok := services.([]any)
	if !(ok) {
		t.Fatalf("result should be a slice, got %T", services)
	}

	var names []string
	for _, s := range serviceList {
		names = append(names, s.(string))
	}
	if !slices.Contains(names, "testgrpc.TestService") {
		t.Errorf("expected %q to contain %q", names, "testgrpc.TestService")
	}
	if !slices.Contains(names, "grpc.reflection.v1.ServerReflection") {
		t.Errorf("expected %q to contain %q", names, "grpc.reflection.v1.ServerReflection")
	}
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
	if !(errVal == nil || isGojaUndefined(errVal)) {
		t.Fatalf("unexpected error: %v", errVal)
	}

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	desc := result.Export().(map[string]any)
	if got := desc["name"]; got != "grpc.reflection.v1.ServerReflection" {
		t.Errorf("expected %v, got %v", "grpc.reflection.v1.ServerReflection", got)
	}
	methods := desc["methods"].([]any)
	if len(methods) <= 0 {
		t.Errorf("expected %v > %v", len(methods), 0)
	}
}
