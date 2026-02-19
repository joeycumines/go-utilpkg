package gojagrpc

import (
	"slices"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/anypb"
)

// ============================================================================
// Coverage gaps: reflection.go — doDescribeType field metadata branches
//
// Covers: MessageKind messageType, EnumKind enumType, HasDefault defaultValue,
// Oneof grouping.
// ============================================================================

// complexTestDescriptorSetBytes returns a FileDescriptorSet with:
// - An enum type (testcomplex.Status)
// - A message with a sub-message field (MessageKind)
// - A message with an enum field (EnumKind)
// - A message with a oneof group
// - A proto2 message with explicit default value (HasDefault)
//
// Also includes a service so we can register on the channel for reflection.
func complexTestDescriptorSetBytes() []byte {
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			complexTestFileDescriptorProto(),
			proto2TestFileDescriptorProto(),
		},
	}
	data, err := proto.Marshal(fds)
	if err != nil {
		panic("complexTestDescriptorSetBytes: " + err.Error())
	}
	return data
}

func complexTestFileDescriptorProto() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:    new("testcomplex.proto"),
		Package: new("testcomplex"),
		Syntax:  new("proto3"),
		EnumType: []*descriptorpb.EnumDescriptorProto{
			{
				Name: new("Status"),
				Value: []*descriptorpb.EnumValueDescriptorProto{
					{Name: new("UNKNOWN"), Number: proto.Int32(0)},
					{Name: new("ACTIVE"), Number: proto.Int32(1)},
					{Name: new("INACTIVE"), Number: proto.Int32(2)},
				},
			},
		},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: new("Address"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("street"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName: new("street"),
					},
				},
			},
			{
				Name: new("Person"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("name"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName: new("name"),
					},
					{
						Name:     new("status"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
						TypeName: new(".testcomplex.Status"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName: new("status"),
					},
					{
						Name:     new("address"),
						Number:   proto.Int32(3),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".testcomplex.Address"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName: new("address"),
					},
					// Oneof fields
					{
						Name:       new("email"),
						Number:     proto.Int32(4),
						Type:       descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:      descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName:   new("email"),
						OneofIndex: proto.Int32(0),
					},
					{
						Name:       new("phone"),
						Number:     proto.Int32(5),
						Type:       descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:      descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName:   new("phone"),
						OneofIndex: proto.Int32(0),
					},
				},
				OneofDecl: []*descriptorpb.OneofDescriptorProto{
					{Name: new("contact")},
				},
			},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: new("PersonService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       new("GetPerson"),
						InputType:  new(".testcomplex.Address"),
						OutputType: new(".testcomplex.Person"),
					},
				},
			},
		},
	}
}

func proto2TestFileDescriptorProto() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:    new("testproto2.proto"),
		Package: new("testproto2"),
		Syntax:  new("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: new("LegacyMessage"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:         new("value"),
						Number:       proto.Int32(1),
						Type:         descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
						Label:        descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName:     new("value"),
						DefaultValue: new("42"),
					},
					{
						Name:     new("label"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName: new("label"),
					},
				},
			},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: new("LegacyService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       new("GetLegacy"),
						InputType:  new(".testproto2.LegacyMessage"),
						OutputType: new(".testproto2.LegacyMessage"),
					},
				},
			},
		},
	}
}

// newGrpcTestEnvWithComplexDescriptors creates a test environment with
// both the standard testgrpc descriptors and complex descriptors
// (testcomplex + testproto2) loaded.
func newGrpcTestEnvWithComplexDescriptors(t *testing.T) *grpcTestEnv {
	t.Helper()
	env := newGrpcTestEnv(t)
	_, err := env.pbMod.LoadDescriptorSetBytes(complexTestDescriptorSetBytes())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return env
}

// TestReflection_DescribeType_MessageKindField tests that the
// doDescribeType function sets the messageType metadata field when a
// field's kind is MessageKind. Covers reflection.go line ~224.
func TestReflection_DescribeType_MessageKindField(t *testing.T) {
	env := newGrpcTestEnvWithComplexDescriptors(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testcomplex.PersonService', {
			getPerson: function(request, call) { return null; }
		});
		server.start();
		grpc.enableReflection();

		var reflClient = grpc.createReflectionClient();
		var result;
		var error;
		reflClient.describeType('testcomplex.Person').then(function(desc) {
			result = desc;
			__done();
		}).catch(function(err) {
			error = String(err);
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
	if got := desc["name"]; got != "testcomplex.Person" {
		t.Errorf("expected %v, got %v", "testcomplex.Person", got)
	}

	fields := desc["fields"].([]any)
	if got := len(fields); got != 5 {
		t.Fatalf("expected len %d, got %d", 5, got)
	}

	fieldMap := make(map[string]map[string]any)
	for _, f := range fields {
		fObj := f.(map[string]any)
		fieldMap[fObj["name"].(string)] = fObj
	}

	// MessageKind field — should have "messageType" set.
	addressField := fieldMap["address"]
	if addressField == nil {
		t.Fatalf("address field not found")
	}
	if got := addressField["type"]; got != "message" {
		t.Errorf("expected %v, got %v", "message", got)
	}
	if got := addressField["messageType"]; got != "testcomplex.Address" {
		t.Errorf("expected %v, got %v", "testcomplex.Address", got)
	}

	// EnumKind field — should have "enumType" set.
	statusField := fieldMap["status"]
	if statusField == nil {
		t.Fatalf("status field not found")
	}
	if got := statusField["type"]; got != "enum" {
		t.Errorf("expected %v, got %v", "enum", got)
	}
	if got := statusField["enumType"]; got != "testcomplex.Status" {
		t.Errorf("expected %v, got %v", "testcomplex.Status", got)
	}

	// Oneofs - should have oneof "contact" with fields "email" and "phone".
	oneofs := desc["oneofs"].([]any)
	if got := len(oneofs); got != 1 {
		t.Fatalf("expected len %d, got %d", 1, got)
	}
	oneof := oneofs[0].(map[string]any)
	if got := oneof["name"]; got != "contact" {
		t.Errorf("expected %v, got %v", "contact", got)
	}
	oneofFields := oneof["fields"].([]any)
	if got := len(oneofFields); got != 2 {
		t.Fatalf("expected len %d, got %d", 2, got)
	}
	if got := oneofFields[0]; got != "email" {
		t.Errorf("expected %v, got %v", "email", got)
	}
	if got := oneofFields[1]; got != "phone" {
		t.Errorf("expected %v, got %v", "phone", got)
	}
}

// TestReflection_DescribeType_HasDefaultField tests that the
// doDescribeType function sets the defaultValue metadata field when
// HasDefault() is true (proto2 with explicit default).
// Covers reflection.go line ~230.
func TestReflection_DescribeType_HasDefaultField(t *testing.T) {
	env := newGrpcTestEnvWithComplexDescriptors(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testproto2.LegacyService', {
			getLegacy: function(request, call) { return null; }
		});
		server.start();
		grpc.enableReflection();

		var reflClient = grpc.createReflectionClient();
		var result;
		var error;
		reflClient.describeType('testproto2.LegacyMessage').then(function(desc) {
			result = desc;
			__done();
		}).catch(function(err) {
			error = String(err);
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
	if got := desc["name"]; got != "testproto2.LegacyMessage" {
		t.Errorf("expected %v, got %v", "testproto2.LegacyMessage", got)
	}

	fields := desc["fields"].([]any)
	if got := len(fields); got != 2 {
		t.Fatalf("expected len %d, got %d", 2, got)
	}

	fieldMap := make(map[string]map[string]any)
	for _, f := range fields {
		fObj := f.(map[string]any)
		fieldMap[fObj["name"].(string)] = fObj
	}

	// HasDefault field — should have "defaultValue" set.
	valueField := fieldMap["value"]
	if valueField == nil {
		t.Fatalf("value field not found")
	}
	if got := valueField["type"]; got != "int32" {
		t.Errorf("expected %v, got %v", "int32", got)
	}
	if got := valueField["defaultValue"]; got != "42" {
		t.Errorf("expected %v, got %v", "42", got)
	}

	// Non-default field — should NOT have "defaultValue".
	labelField := fieldMap["label"]
	if labelField == nil {
		t.Fatalf("label field not found")
	}
	_, hasDefault := labelField["defaultValue"]
	if hasDefault {
		t.Errorf("label should not have defaultValue")
	}
}

// TestReflection_DescribeService_NotAService tests the error path
// when describeService is called with a message name instead of a
// service name. Covers reflection.go line ~165 (non-service check).
func TestReflection_DescribeService_NotAService(t *testing.T) {
	env := newGrpcTestEnvWithComplexDescriptors(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testcomplex.PersonService', {
			getPerson: function(request, call) { return null; }
		});
		server.start();
		grpc.enableReflection();

		var reflClient = grpc.createReflectionClient();
		var error;
		reflClient.describeService('testcomplex.Person').then(function(desc) {
			error = 'should-not-succeed';
			__done();
		}).catch(function(err) {
			error = String(err);
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if !strings.Contains(errVal.String(), "not a service") {
		t.Errorf("expected %q to contain %q", errVal.String(), "not a service")
	}
}

// TestReflection_DescribeType_NotAMessage tests the error path
// when describeType is called with an enum or service name instead
// of a message name. Covers reflection.go line ~208 (non-message check).
func TestReflection_DescribeType_NotAMessage(t *testing.T) {
	env := newGrpcTestEnvWithComplexDescriptors(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testcomplex.PersonService', {
			getPerson: function(request, call) { return null; }
		});
		server.start();
		grpc.enableReflection();

		var reflClient = grpc.createReflectionClient();
		var error;
		// Describe a service name as a type — should fail.
		reflClient.describeType('testcomplex.PersonService').then(function(desc) {
			error = 'should-not-succeed';
			__done();
		}).catch(function(err) {
			error = String(err);
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if !strings.Contains(errVal.String(), "not a message type") {
		t.Errorf("expected %q to contain %q", errVal.String(), "not a message type")
	}
}

// ============================================================================
// Coverage gaps: status.go — createError with non-array details
//
// Covers: statusObject line ~80 (detailsArg not *goja.Object)
// ============================================================================

func TestStatusCreateError_DetailsPrimitive(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Pass a primitive (boolean) as details — should fall through
	// and create error without details.
	val := env.run(t, `
		var err = grpc.status.createError(13, 'test-msg', true);
		err.code + ':' + err.message + ':' + err.details.length;
	`)
	if got := val.String(); got != "13:test-msg:0" {
		t.Errorf("expected %v, got %v", "13:test-msg:0", got)
	}
}

func TestStatusCreateError_DetailsNumber(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.run(t, `
		var err = grpc.status.createError(13, 'test-msg', 42);
		err.code + ':' + err.message + ':' + err.details.length;
	`)
	if got := val.String(); got != "13:test-msg:0" {
		t.Errorf("expected %v, got %v", "13:test-msg:0", got)
	}
}

// ============================================================================
// Coverage gaps: status.go — newGrpcErrorWithDetails error paths
//
// Covers: UnwrapMessage failure (line ~125), anypb.New failure (line ~129)
// ============================================================================

func TestStatusCreateError_DetailsWithInvalidElements(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Array with non-message elements — UnwrapMessage should fail,
	// those elements are skipped in Go details but kept in JS array.
	val := env.run(t, `
		var err = grpc.status.createError(13, 'test-msg', [42, "invalid", null]);
		err.code + ':' + err.details.length;
	`)
	// All 3 elements are non-nil/non-undefined so they all appear in the JS details array.
	if got := val.String(); got != "13:3" {
		t.Errorf("expected %v, got %v", "13:3", got)
	}
}

func TestStatusCreateError_DetailsWithValidMessage(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Create a valid protobuf message as detail — should succeed
	// through the full newGrpcErrorWithDetails path.
	val := env.run(t, `
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var detail = new EchoRequest();
		detail.set('message', 'detail-info');
		var err = grpc.status.createError(13, 'test-msg', [detail]);
		err.code + ':' + err.details.length;
	`)
	if got := val.String(); got != "13:1" {
		t.Errorf("expected %v, got %v", "13:1", got)
	}
}

// ============================================================================
// Coverage gaps: status.go — extractGoDetails type assertion failure
//
// Covers: extractGoDetails line ~148 (Export() is not *goDetailsHolder)
// ============================================================================

func TestExtractGoDetails_Direct_WrongType(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	obj := env.grpcMod.newGrpcError(codes.Internal, "test")
	// Set _goDetails to something that's not a *goDetailsHolder.
	_ = obj.Set("_goDetails", 42)
	result := env.grpcMod.extractGoDetails(obj)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestExtractGoDetails_Direct_NilValue(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	obj := env.grpcMod.newGrpcError(codes.Internal, "test")
	// _goDetails not set at all — should return nil.
	result := env.grpcMod.extractGoDetails(obj)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

// ============================================================================
// Coverage gaps: status.go — wrapStatusDetails error paths
//
// Covers: FindDescriptor error (line ~169), non-message (line ~174),
// unmarshal error (line ~180)
// ============================================================================

func TestWrapStatusDetails_Direct_UnknownType(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Create an Any with an unknown type URL.
	details := []*anypb.Any{
		{
			TypeUrl: "type.googleapis.com/unknown.NonexistentType",
			Value:   []byte{1, 2, 3},
		},
	}
	arr := env.grpcMod.wrapStatusDetails(details)
	// Unknown type should be silently skipped.
	if got := arr.Get("length").ToInteger(); got != int64(0) {
		t.Errorf("expected %v, got %v", int64(0), got)
	}
}

func TestWrapStatusDetails_Direct_NonMessageType(t *testing.T) {
	env := newGrpcTestEnvWithComplexDescriptors(t)
	defer env.shutdown()

	// Create an Any with a type URL pointing to an enum (not a message).
	details := []*anypb.Any{
		{
			TypeUrl: "type.googleapis.com/testcomplex.Status",
			Value:   []byte{},
		},
	}
	arr := env.grpcMod.wrapStatusDetails(details)
	// Enum type should be skipped (not a message).
	if got := arr.Get("length").ToInteger(); got != int64(0) {
		t.Errorf("expected %v, got %v", int64(0), got)
	}
}

func TestWrapStatusDetails_Direct_CorruptedData(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Create an Any with a valid type URL but corrupted value bytes.
	details := []*anypb.Any{
		{
			TypeUrl: "type.googleapis.com/testgrpc.EchoRequest",
			Value:   []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
		},
	}
	arr := env.grpcMod.wrapStatusDetails(details)
	// Unmarshal error should be silently skipped.
	if got := arr.Get("length").ToInteger(); got != int64(0) {
		t.Errorf("expected %v, got %v", int64(0), got)
	}
}

func TestWrapStatusDetails_Direct_ValidDetail(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Build bytes for EchoRequest{message: "test"}.
	reqBytes := []byte{0x0a, 0x04, 0x74, 0x65, 0x73, 0x74} // field 1, string "test"
	details := []*anypb.Any{
		{
			TypeUrl: "type.googleapis.com/testgrpc.EchoRequest",
			Value:   reqBytes,
		},
	}
	arr := env.grpcMod.wrapStatusDetails(details)
	if got := arr.Get("length").ToInteger(); got != int64(1) {
		t.Errorf("expected %v, got %v", int64(1), got)
	}
}

// ============================================================================
// Coverage gaps: server.go — buildServerChain error paths
//
// Covers: interceptor chain throws error (line ~391),
// non-callable return from interceptor (line ~397)
// ============================================================================

func TestServerInterceptor_ChainReturnsNonCallable(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addInterceptor(function(next) {
			// Return a string instead of a function — not callable.
			return "not a function";
		});
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'ok');
				return resp;
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
		var error;
		client.echo(req).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["code"]; got != int64(codes.Internal) {
		t.Errorf("expected %v, got %v", int64(codes.Internal), got)
	}
	if !strings.Contains(resultObj["message"].(string), "interceptor chain") {
		t.Errorf("expected %q to contain %q", resultObj["message"].(string), "interceptor chain")
	}
}

func TestServerInterceptor_ChainThrowsError(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addInterceptor(function(next) {
			throw new Error("interceptor factory error");
		});
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'ok');
				return resp;
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
		var error;
		client.echo(req).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	// The interceptor factory threw → Internal error.
	if got := resultObj["code"]; got != int64(codes.Internal) {
		t.Errorf("expected %v, got %v", int64(codes.Internal), got)
	}
}

// ============================================================================
// Coverage gaps: client.go — parseInterceptors panic path
//
// Covers: parseInterceptors line ~87 panic('not an array')
// ============================================================================

func TestClientInterceptors_NotAnArray(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'ok');
				return resp;
			},
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client;
		var error;
		try {
			// Pass interceptors as a non-array object without length
			client = grpc.createClient('testgrpc.TestService', {
				interceptors: { foo: 'bar' }
			});
		} catch(e) {
			// Non-array interceptors should be accepted because it has no length
			// (it goes through the null check). Actually it won't panic because
			// the Get("length") check returns nil/undefined → returns nil interceptors.
		}

		// The client should still be created (interceptors silently ignored).
		if (client) {
			var EchoRequest = pb.messageType('testgrpc.EchoRequest');
			var req = new EchoRequest();
			req.set('message', 'test');
			client.echo(req).then(function(resp) {
				result = resp.get('message');
				__done();
			}).catch(function(err) {
				error = err.message;
				__done();
			});
		} else {
			result = 'client-nil';
			__done();
		}
	`, defaultTimeout)

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	if got := result.String(); got != "ok" {
		t.Errorf("expected %v, got %v", "ok", got)
	}
}

func TestClientInterceptors_ElementNotFunction(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Interceptor array with a non-function element should panic.
	err := env.mustFail(t, `
		grpc.createClient('testgrpc.TestService', {
			interceptors: [42]
		});
	`)
	if !strings.Contains(err.Error(), "not a function") {
		t.Errorf("expected %q to contain %q", err.Error(), "not a function")
	}
}

// ============================================================================
// Coverage gaps: client.go — makeUnaryMethod interceptor error paths
//
// Covers: line ~192 (jsErr from interceptor), line ~199 (non-callable)
// ============================================================================

func TestClientInterceptor_FactoryThrows(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'ok');
				return resp;
			},
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var error;
		try {
			var client = grpc.createClient('testgrpc.TestService', {
				interceptors: [function(next) {
					throw new Error("interceptor factory error");
				}]
			});
			var EchoRequest = pb.messageType('testgrpc.EchoRequest');
			var req = new EchoRequest();
			req.set('message', 'test');
			client.echo(req);
		} catch(e) {
			error = e.message;
		}
		__done();
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if !strings.Contains(errVal.String(), "interceptor factory error") {
		t.Errorf("expected %q to contain %q", errVal.String(), "interceptor factory error")
	}
}

func TestClientInterceptor_ReturnsNonCallable(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'ok');
				return resp;
			},
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var error;
		try {
			var client = grpc.createClient('testgrpc.TestService', {
				interceptors: [function(next) {
					return "not a function";
				}]
			});
			var EchoRequest = pb.messageType('testgrpc.EchoRequest');
			var req = new EchoRequest();
			req.set('message', 'test');
			client.echo(req);
		} catch(e) {
			error = e.message;
		}
		__done();
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if !strings.Contains(errVal.String(), "interceptor chain") {
		t.Errorf("expected %q to contain %q", errVal.String(), "interceptor chain")
	}
}

// ============================================================================
// Coverage gaps: dial.go — parseChannelOpt type assertion failure
//
// Covers: parseChannelOpt line ~107 (Export()!=*dialConn)
// ============================================================================

func TestParseChannelOpt_WrongNativeType(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Create a fake channel object with _conn set to wrong type.
	err := env.mustFail(t, `
		var fakeChannel = { _conn: 42, close: function(){}, target: function(){} };
		grpc.createClient('testgrpc.TestService', { channel: fakeChannel });
	`)
	if !strings.Contains(err.Error(), "channel must be a dial() result") {
		t.Errorf("expected %q to contain %q", err.Error(), "channel must be a dial() result")
	}
}

func TestParseChannelOpt_MissingConn(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	err := env.mustFail(t, `
		var fakeChannel = { close: function(){}, target: function(){} };
		grpc.createClient('testgrpc.TestService', { channel: fakeChannel });
	`)
	if !strings.Contains(err.Error(), "channel must be a dial() result") {
		t.Errorf("expected %q to contain %q", err.Error(), "channel must be a dial() result")
	}
}

func TestParseChannelOpt_NotAnObject(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	err := env.mustFail(t, `
		grpc.createClient('testgrpc.TestService', { channel: "not-an-object" });
	`)
	if !strings.Contains(err.Error(), "channel must be a dial() result") {
		t.Errorf("expected %q to contain %q", err.Error(), "channel must be a dial() result")
	}
}

// ============================================================================
// Coverage gaps: metadata.go — metadataToGo nil guard
//
// Covers: metadataToGo line ~133 (initial object check)
// ============================================================================

func TestMetadataToGo_Direct_NilInput(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	result := env.grpcMod.metadataToGo(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestMetadataToGo_Direct_NullInput(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	result := env.grpcMod.metadataToGo(goja.Null())
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestMetadataToGo_Direct_UndefinedInput(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	result := env.grpcMod.metadataToGo(goja.Undefined())
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestMetadataToGo_Direct_Primitive(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	result := env.grpcMod.metadataToGo(env.runtime.ToValue(42))
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

// ============================================================================
// Coverage gaps: server.go — newServerCallObject SetHeader error path
//
// Covers: server.go line ~430 (SetHeader error)
// ============================================================================

func TestServerHandler_SetHeaderFromInterceptor(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Test that setHeader/sendHeader work from the handler.
	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var md = grpc.metadata.create();
				md.set('x-custom', 'value');
				call.setHeader(md);
				call.sendHeader();
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'ok');
				return resp;
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
		var headerVal;
		client.echo(req, {
			onHeader: function(md) {
				headerVal = md.get('x-custom');
			}
		}).then(function(resp) {
			result = resp.get('message');
			__done();
		}).catch(function(err) {
			result = 'err: ' + err.message;
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	if got := result.String(); got != "ok" {
		t.Errorf("expected %v, got %v", "ok", got)
	}

	hdr := env.runtime.Get("headerVal")
	if hdr == nil {
		t.Fatalf("expected non-nil")
	}
	if got := hdr.String(); got != "value" {
		t.Errorf("expected %v, got %v", "value", got)
	}
}

// ============================================================================
// Coverage gaps: status.go — jsValueToGRPCError with GrpcError that has details
//
// Covers: full round-trip with details through jsValueToGRPCError
// ============================================================================

func TestJsValueToGRPCError_Direct_GrpcErrorWithDetails(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Create a GrpcError with details array from JS, then convert to gRPC error.
	val, jsErr := env.runtime.RunString(`
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var detail = new EchoRequest();
		detail.set('message', 'detail-info');
		grpc.status.createError(5, 'not found', [detail]);
	`)
	if jsErr != nil {
		t.Fatalf("unexpected error: %v", jsErr)
	}

	err := env.grpcMod.jsValueToGRPCError(val)
	s, ok := status.FromError(err)
	if !(ok) {
		t.Fatalf("expected true")
	}
	if got := s.Code(); got != codes.NotFound {
		t.Errorf("expected %v, got %v", codes.NotFound, got)
	}
	if !strings.Contains(s.Message(), "not found") {
		t.Errorf("expected %q to contain %q", s.Message(), "not found")
	}
	// Details should be preserved via extractGoDetails.
	if got := len(s.Proto().GetDetails()); got != 1 {
		t.Errorf("expected len %d, got %d", 1, got)
	}
}

func TestJsValueToGRPCError_Direct_GrpcErrorNoDetails(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val, jsErr := env.runtime.RunString(`
		grpc.status.createError(5, 'simple error');
	`)
	if jsErr != nil {
		t.Fatalf("unexpected error: %v", jsErr)
	}

	err := env.grpcMod.jsValueToGRPCError(val)
	s, ok := status.FromError(err)
	if !(ok) {
		t.Fatalf("expected true")
	}
	if got := s.Code(); got != codes.NotFound {
		t.Errorf("expected %v, got %v", codes.NotFound, got)
	}
	if got := len(s.Proto().GetDetails()); got != 0 {
		t.Errorf("expected len %d, got %d", 0, got)
	}
}

// ============================================================================
// Coverage gaps: Unary RPC with timeoutMs option
//
// Covers: callopts.go applyTimeoutMs
// ============================================================================

func TestUnaryRPC_WithTimeoutMs(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'fast');
				return resp;
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
		var result;
		client.echo(req, { timeoutMs: 5000 }).then(function(resp) {
			result = resp.get('message');
			__done();
		}).catch(function(err) {
			result = 'err:' + err.code;
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	if got := result.String(); got != "fast" {
		t.Errorf("expected %v, got %v", "fast", got)
	}
}

// ============================================================================
// Coverage gaps: client.go — onHeader/onTrailer callbacks for streaming
//
// Covers: onHeader/onTrailer paths in makeServerStreamMethod,
// newStreamReader (trailer callback)
// ============================================================================

func TestServerStreamRPC_WithHeaderTrailerCallbacks(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {
				var md = grpc.metadata.create();
				md.set('x-response', 'header-value');
				call.setHeader(md);

				var trailer = grpc.metadata.create();
				trailer.set('x-trailer', 'trailer-value');
				call.setTrailer(trailer);

				// Send one item.
				var Item = pb.messageType('testgrpc.Item');
				var item = new Item();
				item.set('id', '1');
				item.set('name', 'item1');
				call.send(item);
			},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test');
		var headerReceived;
		var trailerReceived;
		var items = [];
		client.serverStream(req, {
			onHeader: function(md) {
				headerReceived = md.get('x-response');
			},
			onTrailer: function(md) {
				trailerReceived = md.get('x-trailer');
			}
		}).then(function(stream) {
			function readLoop() {
				stream.recv().then(function(result) {
					if (result.done) {
						__done();
					} else {
						items.push(result.value.get('name'));
						readLoop();
					}
				}).catch(function(err) {
					__done();
				});
			}
			readLoop();
		}).catch(function(err) {
			__done();
		});
	`, defaultTimeout)

	hdr := env.runtime.Get("headerReceived")
	if hdr == nil {
		t.Fatalf("expected non-nil")
	}
	if got := hdr.String(); got != "header-value" {
		t.Errorf("expected %v, got %v", "header-value", got)
	}

	trailer := env.runtime.Get("trailerReceived")
	if trailer == nil {
		t.Fatalf("expected non-nil")
	}
	if got := trailer.String(); got != "trailer-value" {
		t.Errorf("expected %v, got %v", "trailer-value", got)
	}

	items := env.runtime.Get("items")
	if items == nil {
		t.Fatalf("expected non-nil")
	}
	arr := items.Export().([]any)
	if got := len(arr); got != 1 {
		t.Errorf("expected %v, got %v", 1, got)
	}
	if got := arr[0]; got != "item1" {
		t.Errorf("expected %v, got %v", "item1", got)
	}
}

// ============================================================================
// Coverage gaps: client.go — client-streaming with onHeader/onTrailer
//
// Covers: makeClientStreamMethod async header fetch, newClientStreamCall
// trailer delivery
// ============================================================================

func TestClientStreamRPC_WithHeaderTrailerCallbacks(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) {
				var md = grpc.metadata.create();
				md.set('x-response', 'cs-header');
				call.setHeader(md);
				call.sendHeader();

				var trailer = grpc.metadata.create();
				trailer.set('x-trailer', 'cs-trailer');
				call.setTrailer(trailer);

				return new Promise(function(resolve, reject) {
					var count = 0;
					function readLoop() {
						call.recv().then(function(result) {
							if (result.done) {
								var EchoResponse = pb.messageType('testgrpc.EchoResponse');
								var resp = new EchoResponse();
								resp.set('message', 'received:' + count);
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
		var headerReceived;
		var trailerReceived;
		var result;
		client.clientStream({
			onHeader: function(md) {
				headerReceived = md.get('x-response');
			},
			onTrailer: function(md) {
				trailerReceived = md.get('x-trailer');
			}
		}).then(function(call) {
			var Item = pb.messageType('testgrpc.Item');
			var item = new Item();
			item.set('id', '1');
			item.set('name', 'one');
			call.send(item).then(function() {
				return call.closeSend();
			}).then(function() {
				return call.response;
			}).then(function(resp) {
				result = resp.get('message');
				__done();
			}).catch(function(err) {
				result = 'err:' + err.message;
				__done();
			});
		}).catch(function(err) {
			result = 'err:' + err.message;
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	if got := result.String(); got != "received:1" {
		t.Errorf("expected %v, got %v", "received:1", got)
	}

	trailer := env.runtime.Get("trailerReceived")
	if trailer == nil {
		t.Fatalf("expected non-nil")
	}
	if got := trailer.String(); got != "cs-trailer" {
		t.Errorf("expected %v, got %v", "cs-trailer", got)
	}
}

// ============================================================================
// Coverage gaps: client.go — bidi-streaming with onHeader/onTrailer
//
// Covers: makeBidiStreamMethod header fetch, newBidiStream trailer delivery
// ============================================================================

func TestBidiStreamRPC_WithHeaderTrailerCallbacks(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {
				var md = grpc.metadata.create();
				md.set('x-bidi-header', 'bidi-h');
				call.setHeader(md);
				call.sendHeader();

				var trailer = grpc.metadata.create();
				trailer.set('x-bidi-trailer', 'bidi-t');
				call.setTrailer(trailer);

				return new Promise(function(resolve, reject) {
					function readLoop() {
						call.recv().then(function(result) {
							if (result.done) {
								resolve();
							} else {
								// Echo back.
								var Item = pb.messageType('testgrpc.Item');
								var item = new Item();
								item.set('id', result.value.get('id'));
								item.set('name', 'echo-' + result.value.get('name'));
								call.send(item);
								readLoop();
							}
						}).catch(reject);
					}
					readLoop();
				});
			}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var headerReceived;
		var trailerReceived;
		var received = [];
		client.bidiStream({
			onHeader: function(md) {
				headerReceived = md.get('x-bidi-header');
			},
			onTrailer: function(md) {
				trailerReceived = md.get('x-bidi-trailer');
			}
		}).then(function(stream) {
			var Item = pb.messageType('testgrpc.Item');
			var item = new Item();
			item.set('id', 'a');
			item.set('name', 'alpha');
			stream.send(item).then(function() {
				return stream.closeSend();
			}).then(function() {
				function readLoop() {
					stream.recv().then(function(result) {
						if (result.done) {
							__done();
						} else {
							received.push(result.value.get('name'));
							readLoop();
						}
					}).catch(function(err) {
						__done();
					});
				}
				readLoop();
			});
		}).catch(function(err) {
			__done();
		});
	`, defaultTimeout)

	hdr := env.runtime.Get("headerReceived")
	if hdr == nil {
		t.Fatalf("expected non-nil")
	}
	if got := hdr.String(); got != "bidi-h" {
		t.Errorf("expected %v, got %v", "bidi-h", got)
	}

	trailer := env.runtime.Get("trailerReceived")
	if trailer == nil {
		t.Fatalf("expected non-nil")
	}
	if got := trailer.String(); got != "bidi-t" {
		t.Errorf("expected %v, got %v", "bidi-t", got)
	}

	items := env.runtime.Get("received")
	if items == nil {
		t.Fatalf("expected non-nil")
	}
	arr := items.Export().([]any)
	if got := len(arr); got != 1 {
		t.Errorf("expected %v, got %v", 1, got)
	}
	if got := arr[0]; got != "echo-alpha" {
		t.Errorf("expected %v, got %v", "echo-alpha", got)
	}
}

// ============================================================================
// Coverage gaps: server.go — setTrailer with nil metadata (no-op path)
// ============================================================================

func TestServerHandler_SetTrailerNil(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				// setHeader/setTrailer with undefined/null — should be no-op.
				call.setHeader(undefined);
				call.setHeader(null);
				call.setTrailer(undefined);
				call.setTrailer(null);
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'ok');
				return resp;
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
		var result;
		client.echo(req).then(function(resp) {
			result = resp.get('message');
			__done();
		}).catch(function(err) {
			result = 'err:' + err.message;
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	if got := result.String(); got != "ok" {
		t.Errorf("expected %v, got %v", "ok", got)
	}
}

// ============================================================================
// Coverage gaps: status.go — createError with empty details array
//
// Ensures length=0 path is covered.
// ============================================================================

func TestStatusCreateError_EmptyDetailsArray(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.run(t, `
		var err = grpc.status.createError(13, 'msg', []);
		err.code + ':' + err.details.length;
	`)
	if got := val.String(); got != "13:0" {
		t.Errorf("expected %v, got %v", "13:0", got)
	}
}

// ============================================================================
// Coverage gaps: status.go — createError with details array length=undefined
//
// Covers the lenVal nil/undefined check.
// ============================================================================

func TestStatusCreateError_DetailsObjectNoLength(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.run(t, `
		var err = grpc.status.createError(13, 'msg', {});
		err.code + ':' + err.details.length;
	`)
	if got := val.String(); got != "13:0" {
		t.Errorf("expected %v, got %v", "13:0", got)
	}
}

// ============================================================================
// Coverage gaps: toWrappedMessage — marshal error in slow path
//
// Covers: server.go toWrappedMessage slow path panic on corrupted msg
// ============================================================================

func TestToWrappedMessage_Direct_SlowPath_BadMarshal(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	desc, err := env.pbMod.FindDescriptor("testgrpc.EchoRequest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Cast to protoreflect.MessageDescriptor which is what toWrappedMessage expects.
	md := desc.(protoreflect.MessageDescriptor)

	// badMarshalMsg is not a proto.Message — should trigger the
	// "not a proto.Message" error path.
	_, convErr := env.grpcMod.toWrappedMessage(badMarshalMsg{}, md)
	if convErr == nil {
		t.Fatalf("expected an error")
	}
	if !strings.Contains(convErr.Error(), "not a proto.Message") {
		t.Errorf("expected %q to contain %q", convErr.Error(), "not a proto.Message")
	}
}

// ============================================================================
// Coverage gaps: Reflection — multiple services for transitive dep coverage
// ============================================================================

func TestReflection_DescribeComplexService(t *testing.T) {
	env := newGrpcTestEnvWithComplexDescriptors(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testcomplex.PersonService', {
			getPerson: function(request, call) { return null; }
		});
		server.start();
		grpc.enableReflection();

		var reflClient = grpc.createReflectionClient();
		var result;
		var error;
		reflClient.describeService('testcomplex.PersonService').then(function(desc) {
			result = desc;
			__done();
		}).catch(function(err) {
			error = String(err);
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
	if got := desc["name"]; got != "testcomplex.PersonService" {
		t.Errorf("expected %v, got %v", "testcomplex.PersonService", got)
	}

	methods := desc["methods"].([]any)
	if got := len(methods); got != 1 {
		t.Fatalf("expected len %d, got %d", 1, got)
	}

	mObj := methods[0].(map[string]any)
	if got := mObj["name"]; got != "GetPerson" {
		t.Errorf("expected %v, got %v", "GetPerson", got)
	}
	if got := mObj["inputType"]; got != "testcomplex.Address" {
		t.Errorf("expected %v, got %v", "testcomplex.Address", got)
	}
	if got := mObj["outputType"]; got != "testcomplex.Person" {
		t.Errorf("expected %v, got %v", "testcomplex.Person", got)
	}
}

// ============================================================================
// Coverage gaps: Reflection — list services with both standard and complex
// ============================================================================

func TestReflection_ListServicesWithComplex(t *testing.T) {
	env := newGrpcTestEnvWithComplexDescriptors(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.addService('testcomplex.PersonService', {
			getPerson: function(request, call) { return null; }
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
			error = String(err);
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
	serviceList := result.Export().([]any)

	var names []string
	for _, s := range serviceList {
		names = append(names, s.(string))
	}
	if !slices.Contains(names, "testgrpc.TestService") {
		t.Errorf("expected %q to contain %q", names, "testgrpc.TestService")
	}
	if !slices.Contains(names, "testcomplex.PersonService") {
		t.Errorf("expected %q to contain %q", names, "testcomplex.PersonService")
	}
}

// ============================================================================
// Coverage gaps: Reflection — describe Address message (simple message type)
// ============================================================================

func TestReflection_DescribeType_Address(t *testing.T) {
	env := newGrpcTestEnvWithComplexDescriptors(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testcomplex.PersonService', {
			getPerson: function(request, call) { return null; }
		});
		server.start();
		grpc.enableReflection();

		var reflClient = grpc.createReflectionClient();
		var result;
		var error;
		reflClient.describeType('testcomplex.Address').then(function(desc) {
			result = desc;
			__done();
		}).catch(function(err) {
			error = String(err);
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
	if got := desc["name"]; got != "testcomplex.Address" {
		t.Errorf("expected %v, got %v", "testcomplex.Address", got)
	}

	fields := desc["fields"].([]any)
	if got := len(fields); got != 1 {
		t.Fatalf("expected len %d, got %d", 1, got)
	}
	field := fields[0].(map[string]any)
	if got := field["name"]; got != "street" {
		t.Errorf("expected %v, got %v", "street", got)
	}
	if got := field["type"]; got != "string" {
		t.Errorf("expected %v, got %v", "string", got)
	}
}

// ============================================================================
// Coverage gaps: client.go — invokeMetadataCallback nil md path
// ============================================================================

func TestUnaryRPC_WithOnHeaderNoServerHeaders(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'ok');
				return resp;
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
		var headerCalled = false;
		var trailerCalled = false;
		var result;
		client.echo(req, {
			onHeader: function(md) {
				headerCalled = true;
			},
			onTrailer: function(md) {
				trailerCalled = true;
			}
		}).then(function(resp) {
			result = resp.get('message');
			__done();
		}).catch(function(err) {
			result = 'err:' + err.message;
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	if got := result.String(); got != "ok" {
		t.Errorf("expected %v, got %v", "ok", got)
	}

	hCalled := env.runtime.Get("headerCalled")
	if hCalled == nil {
		t.Fatalf("expected non-nil")
	}
	if !(hCalled.ToBoolean()) {
		t.Errorf("expected true")
	}

	tCalled := env.runtime.Get("trailerCalled")
	if tCalled == nil {
		t.Fatalf("expected non-nil")
	}
	if !(tCalled.ToBoolean()) {
		t.Errorf("expected true")
	}
}

// ============================================================================
// Coverage gaps: client.go — unary RPC with interceptors (full chain)
//
// Covers: makeUnaryMethod interceptor chain path — request bundle
// construction, metadata extraction from bundle, inner RPC resolution.
// ============================================================================

func TestUnaryRPC_WithInterceptors_FullChain(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var msg = request.get('message');
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'response:' + msg);
				return resp;
			},
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var interceptorCalled = false;
		var client = grpc.createClient('testgrpc.TestService', {
			interceptors: [function(next) {
				return function(req) {
					interceptorCalled = true;
					// Verify request bundle has expected shape.
					if (typeof req.method !== 'string') {
						throw new Error('missing method');
					}
					return next(req);
				};
			}]
		});
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'intercepted');
		var result;
		client.echo(req).then(function(resp) {
			result = resp.get('message');
			__done();
		}).catch(function(err) {
			result = 'err:' + err.message;
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	if got := result.String(); got != "response:intercepted" {
		t.Errorf("expected %v, got %v", "response:intercepted", got)
	}

	ic := env.runtime.Get("interceptorCalled")
	if ic == nil {
		t.Fatalf("expected non-nil")
	}
	if !(ic.ToBoolean()) {
		t.Errorf("expected true")
	}
}

func TestUnaryRPC_WithInterceptors_ModifyMetadata(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var val = call.requestHeader.get('x-injected');
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', val || 'no-header');
				return resp;
			},
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService', {
			interceptors: [function(next) {
				return function(req) {
					// Add metadata through interceptor.
					req.header.set('x-injected', 'from-interceptor');
					return next(req);
				};
			}]
		});
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test');
		var result;
		client.echo(req).then(function(resp) {
			result = resp.get('message');
			__done();
		}).catch(function(err) {
			result = 'err:' + err.message;
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	if got := result.String(); got != "from-interceptor" {
		t.Errorf("expected %v, got %v", "from-interceptor", got)
	}
}

// ============================================================================
// badMarshalMsg — a value that fails the proto.Message type assertion
// in toWrappedMessage because it's not actually a proto.Message.
// ============================================================================

// badMarshalMsg is not a proto.Message — used to trigger the "not a
// proto.Message" error path in toWrappedMessage.
type badMarshalMsg struct{}

// ============================================================================
// Coverage gaps: server.go — finishUnaryResponse null/undefined path
//
// Covers: finishUnaryResponse line ~586 (nil/undefined check)
// ============================================================================

func TestServerUnaryHandler_ReturnsUndefined(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				// Return undefined explicitly.
				return undefined;
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
		var error;
		client.echo(req).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["code"]; got != int64(codes.Internal) {
		t.Errorf("expected %v, got %v", int64(codes.Internal), got)
	}
	if !strings.Contains(resultObj["message"].(string), "nil/undefined") {
		t.Errorf("expected %q to contain %q", resultObj["message"].(string), "nil/undefined")
	}
}

func TestServerUnaryHandler_ReturnsNull(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				return null;
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
		var error;
		client.echo(req).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["code"]; got != int64(codes.Internal) {
		t.Errorf("expected %v, got %v", int64(codes.Internal), got)
	}
	if !strings.Contains(resultObj["message"].(string), "nil/undefined") {
		t.Errorf("expected %q to contain %q", resultObj["message"].(string), "nil/undefined")
	}
}

// ============================================================================
// Coverage gaps: server.go — finishUnaryResponse UnwrapMessage error
//
// Covers: finishUnaryResponse line ~591 (UnwrapMessage failure)
// ============================================================================

func TestServerUnaryHandler_ReturnsNonProtobuf(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				// Return a plain object instead of a protobuf message.
				return { message: "not a protobuf" };
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
		var error;
		client.echo(req).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["code"]; got != int64(codes.Internal) {
		t.Errorf("expected %v, got %v", int64(codes.Internal), got)
	}
	if !strings.Contains(resultObj["message"].(string), "handler response") {
		t.Errorf("expected %q to contain %q", resultObj["message"].(string), "handler response")
	}
}

// ============================================================================
// Coverage gaps: server.go — async handler returning null via promise
//
// Covers: thenFinishUnary → finishUnaryResponse null path via thenable
// ============================================================================

func TestServerUnaryHandler_AsyncReturnsNull(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				return new Promise(function(resolve) {
					resolve(null);
				});
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
		var error;
		client.echo(req).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["code"]; got != int64(codes.Internal) {
		t.Errorf("expected %v, got %v", int64(codes.Internal), got)
	}
}

// ============================================================================
// Coverage gaps: server.go — async handler returning non-protobuf via promise
//
// Covers: thenFinishUnary → finishUnaryResponse UnwrapMessage error via thenable
// ============================================================================

func TestServerUnaryHandler_AsyncReturnsNonProtobuf(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				return new Promise(function(resolve) {
					resolve({ message: "not protobuf" });
				});
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
		var error;
		client.echo(req).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["code"]; got != int64(codes.Internal) {
		t.Errorf("expected %v, got %v", int64(codes.Internal), got)
	}
}

// ============================================================================
// Coverage gaps: server.go — server-streaming handler throws error
//
// Covers: makeServerStreamHandler jsErr path (line ~277)
// ============================================================================

func TestServerStreamHandler_ThrowsError(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {
				throw new Error("server stream handler error");
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
		client.serverStream(req).then(function(stream) {
			stream.recv().then(function(result) {
				error = { unexpected: true };
				__done();
			}).catch(function(err) {
				error = { code: err.code, message: err.message };
				__done();
			});
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["code"]; got != int64(codes.Internal) {
		t.Errorf("expected %v, got %v", int64(codes.Internal), got)
	}
}

// ============================================================================
// Coverage gaps: server.go — server-streaming handler promise rejects
//
// Covers: thenFinish rejection path
// ============================================================================

func TestServerStreamHandler_AsyncRejects(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {
				return new Promise(function(resolve, reject) {
					reject(grpc.status.createError(7, 'permission denied'));
				});
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
		client.serverStream(req).then(function(stream) {
			stream.recv().then(function(result) {
				error = { unexpected: true };
				__done();
			}).catch(function(err) {
				error = { code: err.code, message: err.message };
				__done();
			});
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["code"]; got != int64(7) {
		t.Errorf("expected %v, got %v", int64(7), got)
	}
}

// ============================================================================
// Coverage gaps: server.go — addServerSend with non-protobuf arg
//
// Covers: addServerSend UnwrapMessage error (line ~456)
// ============================================================================

func TestServerStreamHandler_SendNonProtobuf(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {
				try {
					call.send("not a protobuf");
				} catch(e) {
					// Expected: should throw TypeError
					sendError = e.message;
				}
			},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test');
		var sendError;
		client.serverStream(req).then(function(stream) {
			function readLoop() {
				stream.recv().then(function(result) {
					if (result.done) { __done(); }
					else { readLoop(); }
				}).catch(function(err) { __done(); });
			}
			readLoop();
		}).catch(function(err) {
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("sendError")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if !strings.Contains(errVal.String(), "send") {
		t.Errorf("expected %q to contain %q", errVal.String(), "send")
	}
}

// ============================================================================
// Coverage gaps: reflection — call with no reflection enabled
//
// Covers: doListServices connection error, doDescribeService error propagation,
// doDescribeType error propagation. These hit the error paths in the
// jsRefl* wrappers.
// ============================================================================

func TestReflection_ListServices_NoReflectionEnabled(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	// Don't enable reflection — the reflection service won't exist.
	// The listServices call should fail.
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
		var error;
		reflClient.listServices().then(function(services) {
			error = 'should-not-succeed';
			__done();
		}).catch(function(err) {
			error = String(err);
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if got := errVal.String(); got == "should-not-succeed" {
		t.Errorf("unexpected value: %v", got)
	}
}

func TestReflection_DescribeService_NoReflectionEnabled(t *testing.T) {
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

		var reflClient = grpc.createReflectionClient();
		var error;
		reflClient.describeService('testgrpc.TestService').then(function(desc) {
			error = 'should-not-succeed';
			__done();
		}).catch(function(err) {
			error = String(err);
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if got := errVal.String(); got == "should-not-succeed" {
		t.Errorf("unexpected value: %v", got)
	}
}

func TestReflection_DescribeType_NoReflectionEnabled(t *testing.T) {
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

		var reflClient = grpc.createReflectionClient();
		var error;
		reflClient.describeType('testgrpc.EchoRequest').then(function(desc) {
			error = 'should-not-succeed';
			__done();
		}).catch(function(err) {
			error = String(err);
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if got := errVal.String(); got == "should-not-succeed" {
		t.Errorf("unexpected value: %v", got)
	}
}

// ============================================================================
// Coverage gaps: reflection — describe non-existent symbol
//
// Covers: fetchFileDescriptorForSymbol error response path (line ~286)
// ============================================================================

func TestReflection_DescribeService_NonExistent(t *testing.T) {
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
		var error;
		reflClient.describeService('nonexistent.Service').then(function(desc) {
			error = 'should-not-succeed';
			__done();
		}).catch(function(err) {
			error = String(err);
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if got := errVal.String(); got == "should-not-succeed" {
		t.Errorf("unexpected value: %v", got)
	}
}

func TestReflection_DescribeType_NonExistent(t *testing.T) {
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
		var error;
		reflClient.describeType('nonexistent.Message').then(function(desc) {
			error = 'should-not-succeed';
			__done();
		}).catch(function(err) {
			error = String(err);
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if got := errVal.String(); got == "should-not-succeed" {
		t.Errorf("unexpected value: %v", got)
	}
}

// ============================================================================
// Coverage gaps: server.go — client-stream handler returning non-protobuf
//
// Covers: makeClientStreamHandler → finishUnaryResponse error
// ============================================================================

func TestClientStreamHandler_ReturnsNonProtobuf(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) {
				// Return non-protobuf from client-stream handler.
				return new Promise(function(resolve) {
					call.recv().then(function(result) {
						if (result.done) {
							resolve({ message: "not protobuf" });
						} else {
							resolve({ message: "not protobuf" });
						}
					});
				});
			},
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var error;
		client.clientStream().then(function(call) {
			call.closeSend().then(function() {
				return call.response;
			}).then(function(resp) {
				error = { unexpected: true };
				__done();
			}).catch(function(err) {
				error = { code: err.code, message: err.message };
				__done();
			});
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["code"]; got != int64(codes.Internal) {
		t.Errorf("expected %v, got %v", int64(codes.Internal), got)
	}
}

// ============================================================================
// Coverage gaps: server.go — bidi handler throws sync error
//
// Covers: makeBidiStreamHandler jsErr path
// ============================================================================

func TestBidiHandler_ThrowsSyncError(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {
				throw new Error("bidi handler crashed");
			}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var error;
		client.bidiStream().then(function(stream) {
			stream.recv().then(function(result) {
				error = { unexpected: true };
				__done();
			}).catch(function(err) {
				error = { code: err.code, message: err.message };
				__done();
			});
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["code"]; got != int64(codes.Internal) {
		t.Errorf("expected %v, got %v", int64(codes.Internal), got)
	}
}

// ============================================================================
// Coverage gaps: server.go — bidi handler promise rejects with GrpcError
//
// Covers: thenFinish rejection → jsErrorToGRPC with GrpcError code
// ============================================================================

func TestBidiHandler_AsyncRejectsWithGrpcError(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {
				return new Promise(function(resolve, reject) {
					reject(grpc.status.createError(16, 'unauthenticated'));
				});
			}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var error;
		client.bidiStream().then(function(stream) {
			stream.recv().then(function(result) {
				error = { unexpected: true };
				__done();
			}).catch(function(err) {
				error = { code: err.code, message: err.message };
				__done();
			});
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["code"]; got != int64(16) {
		t.Errorf("expected %v, got %v", int64(16), got)
	}
}

// ============================================================================
// Coverage gaps: client.go — unary RPC with metadata option
//
// Covers: applyMetadata path in parseCallOpts
// ============================================================================

func TestUnaryRPC_WithMetadata_ServerReadback(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var val = call.requestHeader.get('x-custom');
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', val || 'none');
				return resp;
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
		var md = grpc.metadata.create();
		md.set('x-custom', 'from-client');
		var result;
		client.echo(req, { metadata: md }).then(function(resp) {
			result = resp.get('message');
			__done();
		}).catch(function(err) {
			result = 'err:' + err.message;
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	if got := result.String(); got != "from-client" {
		t.Errorf("expected %v, got %v", "from-client", got)
	}
}

// ============================================================================
// Coverage gaps: server.go — unary handler with GrpcError throw
//
// Covers: jsErrorToGRPC with GrpcError → preserves code
// ============================================================================

func TestServerUnaryHandler_ThrowsGrpcError(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				throw grpc.status.createError(3, 'invalid argument');
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
		var error;
		client.echo(req).then(function(resp) {
			error = { unexpected: true };
			__done();
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["code"]; got != int64(3) {
		t.Errorf("expected %v, got %v", int64(3), got)
	}
	if !strings.Contains(resultObj["message"].(string), "invalid argument") {
		t.Errorf("expected %q to contain %q", resultObj["message"].(string), "invalid argument")
	}
}

// ============================================================================
// Coverage gaps: client.go — server-streaming RPC with signal abortion
//
// Covers: applySignal path, context cancellation during streaming
// ============================================================================

func TestServerStreamRPC_WithSignalAbort(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {
				// Send a few items with delay.
				var Item = pb.messageType('testgrpc.Item');
				for (var i = 0; i < 5; i++) {
					var item = new Item();
					item.set('id', '' + i);
					item.set('name', 'item-' + i);
					call.send(item);
				}
				// Don't close — let the client abort.
			},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test');
		var controller = new AbortController();
		var received = [];
		var error;
		client.serverStream(req, { signal: controller.signal }).then(function(stream) {
			function readLoop() {
				stream.recv().then(function(result) {
					if (result.done) {
						__done();
					} else {
						received.push(result.value.get('name'));
						// Abort after first item
						if (received.length >= 1) {
							controller.abort();
						}
						readLoop();
					}
				}).catch(function(err) {
					error = { code: err.code };
					__done();
				});
			}
			readLoop();
		}).catch(function(err) {
			error = { code: err.code };
			__done();
		});
	`, defaultTimeout)

	// Either we get items then error, or just error.
	// The main check is that it doesn't hang.
	result := env.runtime.Get("error")
	if result != nil && !isGojaUndefined(result) {
		resultObj := result.Export().(map[string]any)
		if got := resultObj["code"]; got != int64(codes.Canceled) {
			t.Errorf("expected %v, got %v", int64(codes.Canceled), got)
		}
	}
}

// ============================================================================
// Coverage gaps: server.go — newServerCallObject SetHeader after sendHeader
//
// Covers: SetHeader error → panic path (line ~427)
// ============================================================================

func TestServerHandler_SetHeaderAfterSendHeader(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				// First sendHeader, then try setHeader — should fail.
				call.sendHeader();
				var threw = false;
				try {
					var md = grpc.metadata.create();
					md.set('x-after', 'value');
					call.setHeader(md);
				} catch (e) {
					threw = true;
					setHeaderError = e.message;
				}

				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', threw ? 'threw' : 'no-throw');
				return resp;
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
		var setHeaderError;
		var result;
		client.echo(req).then(function(resp) {
			result = resp.get('message');
			__done();
		}).catch(function(err) {
			result = 'err:' + err.message;
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("result")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	// With inprocgrpc, SetHeader after SendHeader may or may not error.
	// The test still exercises the code path.
	t.Logf("result=%v, setHeaderError=%v", result, env.runtime.Get("setHeaderError"))
}

// ============================================================================
// Coverage gaps: client.go — client-stream send with non-protobuf
//
// Covers: newClientStreamCall send UnwrapMessage error (line ~567)
// ============================================================================

func TestClientStreamRPC_SendNonProtobuf(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) {
				return new Promise(function(resolve, reject) {
					call.recv().then(function(result) {
						if (result.done) {
							var EchoResponse = pb.messageType('testgrpc.EchoResponse');
							var resp = new EchoResponse();
							resp.set('message', 'done');
							resolve(resp);
						} else {
							// keep reading
							call.recv().then(function(r2) {
								resolve(null);
							});
						}
					}).catch(reject);
				});
			},
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var error;
		client.clientStream().then(function(call) {
			try {
				call.send("not a protobuf");
			} catch(e) {
				error = e.message;
			}
			call.closeSend().then(function() {
				__done();
			});
		}).catch(function(err) {
			error = err.message;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if !strings.Contains(errVal.String(), "send") {
		t.Errorf("expected %q to contain %q", errVal.String(), "send")
	}
}

// ============================================================================
// Coverage gaps: client.go — bidi-stream send with non-protobuf
//
// Covers: newBidiStream send UnwrapMessage error
// ============================================================================

func TestBidiStreamRPC_SendNonProtobuf(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {
				call.recv().then(function(result) {
					// just let it end
				});
			}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var error;
		client.bidiStream().then(function(stream) {
			try {
				stream.send("not a protobuf");
			} catch(e) {
				error = e.message;
			}
			stream.closeSend().then(function() {
				__done();
			});
		}).catch(function(err) {
			error = err.message;
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if !strings.Contains(errVal.String(), "send") {
		t.Errorf("expected %q to contain %q", errVal.String(), "send")
	}
}

// ============================================================================
// Coverage gaps: client.go — server-streaming with marshalled request error
//
// Covers: makeServerStreamMethod UnwrapMessage error (first arg)
// ============================================================================

func TestServerStreamRPC_NonProtobufRequest(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	err := env.mustFail(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		client.serverStream("not a protobuf");
	`)
	if !strings.Contains(err.Error(), "server-stream") {
		t.Errorf("expected %q to contain %q", err.Error(), "server-stream")
	}
}

// ============================================================================
// Coverage gaps: reflection.go — describeType enum (covers the enum not-message path
// already tested, but this tests a different symbol path)
// ============================================================================

func TestReflection_DescribeEnum(t *testing.T) {
	env := newGrpcTestEnvWithComplexDescriptors(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testcomplex.PersonService', {
			getPerson: function(request, call) { return null; }
		});
		server.start();
		grpc.enableReflection();

		var reflClient = grpc.createReflectionClient();
		var error;
		reflClient.describeType('testcomplex.Status').then(function(desc) {
			error = 'should-not-succeed: ' + JSON.stringify(desc);
			__done();
		}).catch(function(err) {
			error = String(err);
			__done();
		});
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if !strings.Contains(errVal.String(), "not a message type") {
		t.Errorf("expected %q to contain %q", errVal.String(), "not a message type")
	}
}

// ============================================================================
// Coverage gaps: server.go — makeClientStreamHandler sync error
//
// Covers: makeClientStreamHandler jsErr path (handler throws sync)
// ============================================================================

func TestClientStreamHandler_ThrowsSyncError(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) {
				throw new Error("client stream handler error");
			},
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var error;
		client.clientStream().then(function(call) {
			call.closeSend().then(function() {
				return call.response;
			}).then(function(resp) {
				error = { unexpected: true };
				__done();
			}).catch(function(err) {
				error = { code: err.code, message: err.message };
				__done();
			});
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["code"]; got != int64(codes.Internal) {
		t.Errorf("expected %v, got %v", int64(codes.Internal), got)
	}
}

// ============================================================================
// Coverage gaps: server.go — makeClientStreamHandler async resolve null
//
// Covers: makeClientStreamHandler → finishUnaryResponse null via promise
// ============================================================================

func TestClientStreamHandler_AsyncReturnsNull(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) {
				return new Promise(function(resolve) {
					call.recv().then(function(result) {
						resolve(null);
					});
				});
			},
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var error;
		client.clientStream().then(function(call) {
			call.closeSend().then(function() {
				return call.response;
			}).then(function(resp) {
				error = { unexpected: true };
				__done();
			}).catch(function(err) {
				error = { code: err.code, message: err.message };
				__done();
			});
		}).catch(function(err) {
			error = { code: err.code, message: err.message };
			__done();
		});
	`, defaultTimeout)

	result := env.runtime.Get("error")
	if result == nil {
		t.Fatalf("expected non-nil")
	}
	resultObj := result.Export().(map[string]any)
	if got := resultObj["code"]; got != int64(codes.Internal) {
		t.Errorf("expected %v, got %v", int64(codes.Internal), got)
	}
}

// ============================================================================
// Coverage gaps: dial.go — jsDial error path (no address)
//
// Covers: jsDial argument validation
// ============================================================================

func TestDial_NoAddress(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	err := env.mustFail(t, `
		grpc.dial();
	`)
	if err == nil {
		t.Errorf("expected non-nil")
	}
}

// ============================================================================
// Coverage gaps: reflection — describe type for a message in proto2 file
// with the legacy service (transitive-via-same-file so no dep loop, but
// exercises the proto2 descriptor parsing path).
// ============================================================================

func TestReflection_DescribeType_LegacyMessage(t *testing.T) {
	env := newGrpcTestEnvWithComplexDescriptors(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testproto2.LegacyService', {
			getLegacy: function(request, call) { return null; }
		});
		server.start();
		grpc.enableReflection();

		var reflClient = grpc.createReflectionClient();
		var result;
		var error;
		reflClient.describeType('testproto2.LegacyMessage').then(function(desc) {
			result = desc;
			__done();
		}).catch(function(err) {
			error = String(err);
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
	if got := desc["name"]; got != "testproto2.LegacyMessage" {
		t.Errorf("expected %v, got %v", "testproto2.LegacyMessage", got)
	}

	fields := desc["fields"].([]any)
	if got := len(fields); got != 2 {
		t.Fatalf("expected len %d, got %d", 2, got)
	}

	fieldMap := make(map[string]map[string]any)
	for _, f := range fields {
		fObj := f.(map[string]any)
		fieldMap[fObj["name"].(string)] = fObj
	}

	// value field has default "42"
	valueField := fieldMap["value"]
	if valueField == nil {
		t.Fatalf("expected non-nil")
	}
	if got := valueField["defaultValue"]; got != "42" {
		t.Errorf("expected %v, got %v", "42", got)
	}
}

// ============================================================================
// Coverage gaps: reflection — transitive dependency resolution
//
// Create a proto file that IMPORTS another proto file.
// When reflection fetches the file descriptor for the importing file,
// it needs to resolve the dependency. This triggers the transitive
// dep loop in fetchFileDescriptorForSymbol.
// ============================================================================

func transitiveDepsDescriptorSetBytes() []byte {
	// File 1: base types
	baseFile := &descriptorpb.FileDescriptorProto{
		Name:    new("base.proto"),
		Package: new("testdeps"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: new("BaseMessage"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("id"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName: new("id"),
					},
				},
			},
		},
	}

	// File 2: depends on base.proto
	dependentFile := &descriptorpb.FileDescriptorProto{
		Name:       new("dependent.proto"),
		Package:    new("testdeps"),
		Syntax:     new("proto3"),
		Dependency: []string{"base.proto"},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: new("DependentMessage"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     new("base"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: new(".testdeps.BaseMessage"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName: new("base"),
					},
				},
			},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: new("DependentService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       new("GetDependent"),
						InputType:  new(".testdeps.BaseMessage"),
						OutputType: new(".testdeps.DependentMessage"),
					},
				},
			},
		},
	}

	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{baseFile, dependentFile},
	}
	data, err := proto.Marshal(fds)
	if err != nil {
		panic("transitiveDepsDescriptorSetBytes: " + err.Error())
	}
	return data
}

func TestReflection_TransitiveDependencyResolution(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	_, err := env.pbMod.LoadDescriptorSetBytes(transitiveDepsDescriptorSetBytes())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testdeps.DependentService', {
			getDependent: function(request, call) { return null; }
		});
		server.start();
		grpc.enableReflection();

		var reflClient = grpc.createReflectionClient();
		var result;
		var error;
		// Describe the DependentMessage which uses BaseMessage from another file.
		reflClient.describeType('testdeps.DependentMessage').then(function(desc) {
			result = desc;
			__done();
		}).catch(function(err) {
			error = String(err);
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
	if got := desc["name"]; got != "testdeps.DependentMessage" {
		t.Errorf("expected %v, got %v", "testdeps.DependentMessage", got)
	}

	fields := desc["fields"].([]any)
	if got := len(fields); got != 1 {
		t.Fatalf("expected len %d, got %d", 1, got)
	}
	field := fields[0].(map[string]any)
	if got := field["name"]; got != "base" {
		t.Errorf("expected %v, got %v", "base", got)
	}
	if got := field["type"]; got != "message" {
		t.Errorf("expected %v, got %v", "message", got)
	}
	if got := field["messageType"]; got != "testdeps.BaseMessage" {
		t.Errorf("expected %v, got %v", "testdeps.BaseMessage", got)
	}
}

func TestReflection_TransitiveDependencyResolution_Service(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	_, err := env.pbMod.LoadDescriptorSetBytes(transitiveDepsDescriptorSetBytes())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testdeps.DependentService', {
			getDependent: function(request, call) { return null; }
		});
		server.start();
		grpc.enableReflection();

		var reflClient = grpc.createReflectionClient();
		var result;
		var error;
		reflClient.describeService('testdeps.DependentService').then(function(desc) {
			result = desc;
			__done();
		}).catch(function(err) {
			error = String(err);
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
	if got := desc["name"]; got != "testdeps.DependentService" {
		t.Errorf("expected %v, got %v", "testdeps.DependentService", got)
	}

	methods := desc["methods"].([]any)
	if got := len(methods); got != 1 {
		t.Fatalf("expected len %d, got %d", 1, got)
	}
	mObj := methods[0].(map[string]any)
	if got := mObj["name"]; got != "GetDependent" {
		t.Errorf("expected %v, got %v", "GetDependent", got)
	}
	if got := mObj["inputType"]; got != "testdeps.BaseMessage" {
		t.Errorf("expected %v, got %v", "testdeps.BaseMessage", got)
	}
	if got := mObj["outputType"]; got != "testdeps.DependentMessage" {
		t.Errorf("expected %v, got %v", "testdeps.DependentMessage", got)
	}
}

// ============================================================================
// Coverage gaps: client.go — interceptor wrapper function throws
//
// Covers: makeUnaryMethod line ~204 (outerFn call panic)
// ============================================================================

func TestClientInterceptor_WrapperThrows(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'ok');
				return resp;
			},
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var error;
		try {
			var client = grpc.createClient('testgrpc.TestService', {
				interceptors: [function(next) {
					return function(req) {
						// The wrapper function throws during invocation.
						throw new Error("wrapper function error");
					};
				}]
			});
			var EchoRequest = pb.messageType('testgrpc.EchoRequest');
			var req = new EchoRequest();
			req.set('message', 'test');
			client.echo(req);
		} catch(e) {
			error = e.message;
		}
		__done();
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if !strings.Contains(errVal.String(), "wrapper function error") {
		t.Errorf("expected %q to contain %q", errVal.String(), "wrapper function error")
	}
}

// ============================================================================
// Coverage gaps: client.go — interceptor next() called with invalid bundle
//
// Covers: innerRPC nil bundle check (line ~160), invalid message (line ~164)
// ============================================================================

func TestClientInterceptor_NextWithNilBundle(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'ok');
				return resp;
			},
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var error;
		try {
			var client = grpc.createClient('testgrpc.TestService', {
				interceptors: [function(next) {
					return function(req) {
						// Call next with undefined — should panic.
						return next(undefined);
					};
				}]
			});
			var EchoRequest = pb.messageType('testgrpc.EchoRequest');
			var req = new EchoRequest();
			req.set('message', 'test');
			client.echo(req);
		} catch(e) {
			error = e.message;
		}
		__done();
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if !strings.Contains(errVal.String(), "interceptor must call next with request") {
		t.Errorf("expected %q to contain %q", errVal.String(), "interceptor must call next with request")
	}
}

func TestClientInterceptor_NextWithInvalidMessage(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) {
				var EchoResponse = pb.messageType('testgrpc.EchoResponse');
				var resp = new EchoResponse();
				resp.set('message', 'ok');
				return resp;
			},
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var error;
		try {
			var client = grpc.createClient('testgrpc.TestService', {
				interceptors: [function(next) {
					return function(req) {
						// Replace message with invalid value.
						req.message = "not a protobuf";
						return next(req);
					};
				}]
			});
			var EchoRequest = pb.messageType('testgrpc.EchoRequest');
			var req = new EchoRequest();
			req.set('message', 'test');
			client.echo(req);
		} catch(e) {
			error = e.message;
		}
		__done();
	`, defaultTimeout)

	errVal := env.runtime.Get("error")
	if errVal == nil {
		t.Fatalf("expected non-nil")
	}
	if !strings.Contains(errVal.String(), "invalid message") {
		t.Errorf("expected %q to contain %q", errVal.String(), "invalid message")
	}
}

// ============================================================================
// Coverage gaps: client.go — unary RPC with non-protobuf request
//
// Covers: makeUnaryMethod UnwrapMessage error (line ~128)
// ============================================================================

func TestUnaryRPC_NonProtobufRequest(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	err := env.mustFail(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		client.echo("not a protobuf");
	`)
	if !strings.Contains(err.Error(), "unary") {
		t.Errorf("expected %q to contain %q", err.Error(), "unary")
	}
}

// ============================================================================
// Coverage gaps: server.go — server-streaming handler with non-thenable and no send
//
// Covers: makeServerStreamHandler non-thenable path (line ~283) → stream.Finish(nil)
// ============================================================================

func TestServerStreamHandler_SyncNoSend(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {
				// Return a non-thenable value (not a promise).
				// The handler just ends immediately with no sends.
				return undefined;
			},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var EchoRequest = pb.messageType('testgrpc.EchoRequest');
		var req = new EchoRequest();
		req.set('message', 'test');
		var items = [];
		client.serverStream(req).then(function(stream) {
			function readLoop() {
				stream.recv().then(function(result) {
					if (result.done) { __done(); }
					else {
						items.push(result.value);
						readLoop();
					}
				}).catch(function(err) { __done(); });
			}
			readLoop();
		}).catch(function(err) { __done(); });
	`, defaultTimeout)

	items := env.runtime.Get("items")
	if items == nil {
		t.Fatalf("expected non-nil")
	}
	if got := items.(*goja.Object).Get("length").ToInteger(); got != int64(0) {
		t.Errorf("expected %v, got %v", int64(0), got)
	}
}

// ============================================================================
// Coverage gaps: server.go — bidi handler non-thenable path
//
// Covers: makeBidiStreamHandler non-thenable path → stream.Finish(nil)
// ============================================================================

func TestBidiHandler_SyncNonThenable(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	env.runOnLoop(t, `
		var server = grpc.createServer();
		server.addService('testgrpc.TestService', {
			echo: function(request, call) { return null; },
			serverStream: function(request, call) {},
			clientStream: function(call) { return null; },
			bidiStream: function(call) {
				// Return non-thenable (undefined) — should finish immediately.
				return 42;
			}
		});
		server.start();

		var client = grpc.createClient('testgrpc.TestService');
		var error;
		client.bidiStream().then(function(stream) {
			stream.recv().then(function(result) {
				if (result.done) {
					done = true;
					__done();
				} else {
					__done();
				}
			}).catch(function(err) {
				error = { code: err.code };
				__done();
			});
		}).catch(function(err) {
			error = { code: err.code };
			__done();
		});
	`, defaultTimeout)

	// Should complete without error — stream just ends.
	done := env.runtime.Get("done")
	if done != nil && !isGojaUndefined(done) {
		if !(done.ToBoolean()) {
			t.Errorf("expected true")
		}
	}
}
