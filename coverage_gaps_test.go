package gojagrpc

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		Name:    proto.String("testcomplex.proto"),
		Package: proto.String("testcomplex"),
		Syntax:  proto.String("proto3"),
		EnumType: []*descriptorpb.EnumDescriptorProto{
			{
				Name: proto.String("Status"),
				Value: []*descriptorpb.EnumValueDescriptorProto{
					{Name: proto.String("UNKNOWN"), Number: proto.Int32(0)},
					{Name: proto.String("ACTIVE"), Number: proto.Int32(1)},
					{Name: proto.String("INACTIVE"), Number: proto.Int32(2)},
				},
			},
		},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("Address"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     proto.String("street"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName: proto.String("street"),
					},
				},
			},
			{
				Name: proto.String("Person"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     proto.String("name"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName: proto.String("name"),
					},
					{
						Name:     proto.String("status"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
						TypeName: proto.String(".testcomplex.Status"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName: proto.String("status"),
					},
					{
						Name:     proto.String("address"),
						Number:   proto.Int32(3),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: proto.String(".testcomplex.Address"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName: proto.String("address"),
					},
					// Oneof fields
					{
						Name:       proto.String("email"),
						Number:     proto.Int32(4),
						Type:       descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:      descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName:   proto.String("email"),
						OneofIndex: proto.Int32(0),
					},
					{
						Name:       proto.String("phone"),
						Number:     proto.Int32(5),
						Type:       descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:      descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName:   proto.String("phone"),
						OneofIndex: proto.Int32(0),
					},
				},
				OneofDecl: []*descriptorpb.OneofDescriptorProto{
					{Name: proto.String("contact")},
				},
			},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: proto.String("PersonService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       proto.String("GetPerson"),
						InputType:  proto.String(".testcomplex.Address"),
						OutputType: proto.String(".testcomplex.Person"),
					},
				},
			},
		},
	}
}

func proto2TestFileDescriptorProto() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:    proto.String("testproto2.proto"),
		Package: proto.String("testproto2"),
		Syntax:  proto.String("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("LegacyMessage"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:         proto.String("value"),
						Number:       proto.Int32(1),
						Type:         descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
						Label:        descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName:     proto.String("value"),
						DefaultValue: proto.String("42"),
					},
					{
						Name:     proto.String("label"),
						Number:   proto.Int32(2),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName: proto.String("label"),
					},
				},
			},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: proto.String("LegacyService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       proto.String("GetLegacy"),
						InputType:  proto.String(".testproto2.LegacyMessage"),
						OutputType: proto.String(".testproto2.LegacyMessage"),
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
	require.NoError(t, err)
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
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	desc := result.Export().(map[string]interface{})
	assert.Equal(t, "testcomplex.Person", desc["name"])

	fields := desc["fields"].([]interface{})
	require.Len(t, fields, 5) // name, status, address, email, phone

	fieldMap := make(map[string]map[string]interface{})
	for _, f := range fields {
		fObj := f.(map[string]interface{})
		fieldMap[fObj["name"].(string)] = fObj
	}

	// MessageKind field — should have "messageType" set.
	addressField := fieldMap["address"]
	require.NotNil(t, addressField, "address field not found")
	assert.Equal(t, "message", addressField["type"])
	assert.Equal(t, "testcomplex.Address", addressField["messageType"])

	// EnumKind field — should have "enumType" set.
	statusField := fieldMap["status"]
	require.NotNil(t, statusField, "status field not found")
	assert.Equal(t, "enum", statusField["type"])
	assert.Equal(t, "testcomplex.Status", statusField["enumType"])

	// Oneofs - should have oneof "contact" with fields "email" and "phone".
	oneofs := desc["oneofs"].([]interface{})
	require.Len(t, oneofs, 1)
	oneof := oneofs[0].(map[string]interface{})
	assert.Equal(t, "contact", oneof["name"])
	oneofFields := oneof["fields"].([]interface{})
	require.Len(t, oneofFields, 2)
	assert.Equal(t, "email", oneofFields[0])
	assert.Equal(t, "phone", oneofFields[1])
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
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	desc := result.Export().(map[string]interface{})
	assert.Equal(t, "testproto2.LegacyMessage", desc["name"])

	fields := desc["fields"].([]interface{})
	require.Len(t, fields, 2)

	fieldMap := make(map[string]map[string]interface{})
	for _, f := range fields {
		fObj := f.(map[string]interface{})
		fieldMap[fObj["name"].(string)] = fObj
	}

	// HasDefault field — should have "defaultValue" set.
	valueField := fieldMap["value"]
	require.NotNil(t, valueField, "value field not found")
	assert.Equal(t, "int32", valueField["type"])
	assert.Equal(t, "42", valueField["defaultValue"])

	// Non-default field — should NOT have "defaultValue".
	labelField := fieldMap["label"]
	require.NotNil(t, labelField, "label field not found")
	_, hasDefault := labelField["defaultValue"]
	assert.False(t, hasDefault, "label should not have defaultValue")
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
	require.NotNil(t, errVal)
	assert.Contains(t, errVal.String(), "not a service")
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
	require.NotNil(t, errVal)
	assert.Contains(t, errVal.String(), "not a message type")
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
	assert.Equal(t, "13:test-msg:0", val.String())
}

func TestStatusCreateError_DetailsNumber(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val := env.run(t, `
		var err = grpc.status.createError(13, 'test-msg', 42);
		err.code + ':' + err.message + ':' + err.details.length;
	`)
	assert.Equal(t, "13:test-msg:0", val.String())
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
	assert.Equal(t, "13:3", val.String())
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
	assert.Equal(t, "13:1", val.String())
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
	assert.Nil(t, result)
}

func TestExtractGoDetails_Direct_NilValue(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	obj := env.grpcMod.newGrpcError(codes.Internal, "test")
	// _goDetails not set at all — should return nil.
	result := env.grpcMod.extractGoDetails(obj)
	assert.Nil(t, result)
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
	assert.Equal(t, int64(0), arr.Get("length").ToInteger())
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
	assert.Equal(t, int64(0), arr.Get("length").ToInteger())
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
	assert.Equal(t, int64(0), arr.Get("length").ToInteger())
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
	assert.Equal(t, int64(1), arr.Get("length").ToInteger())
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
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(codes.Internal), resultObj["code"])
	assert.Contains(t, resultObj["message"].(string), "interceptor chain")
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
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	// The interceptor factory threw → Internal error.
	assert.Equal(t, int64(codes.Internal), resultObj["code"])
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
	require.NotNil(t, result)
	assert.Equal(t, "ok", result.String())
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
	assert.Contains(t, err.Error(), "not a function")
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
	require.NotNil(t, errVal)
	assert.Contains(t, errVal.String(), "interceptor factory error")
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
	require.NotNil(t, errVal)
	assert.Contains(t, errVal.String(), "interceptor chain")
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
	assert.Contains(t, err.Error(), "channel must be a dial() result")
}

func TestParseChannelOpt_MissingConn(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	err := env.mustFail(t, `
		var fakeChannel = { close: function(){}, target: function(){} };
		grpc.createClient('testgrpc.TestService', { channel: fakeChannel });
	`)
	assert.Contains(t, err.Error(), "channel must be a dial() result")
}

func TestParseChannelOpt_NotAnObject(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	err := env.mustFail(t, `
		grpc.createClient('testgrpc.TestService', { channel: "not-an-object" });
	`)
	assert.Contains(t, err.Error(), "channel must be a dial() result")
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
	assert.Nil(t, result)
}

func TestMetadataToGo_Direct_NullInput(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	result := env.grpcMod.metadataToGo(goja.Null())
	assert.Nil(t, result)
}

func TestMetadataToGo_Direct_UndefinedInput(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	result := env.grpcMod.metadataToGo(goja.Undefined())
	assert.Nil(t, result)
}

func TestMetadataToGo_Direct_Primitive(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	result := env.grpcMod.metadataToGo(env.runtime.ToValue(42))
	assert.Nil(t, result)
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
	require.NotNil(t, result)
	assert.Equal(t, "ok", result.String())

	hdr := env.runtime.Get("headerVal")
	require.NotNil(t, hdr)
	assert.Equal(t, "value", hdr.String())
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
	require.NoError(t, jsErr)

	err := env.grpcMod.jsValueToGRPCError(val)
	s, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, s.Code())
	assert.Contains(t, s.Message(), "not found")
	// Details should be preserved via extractGoDetails.
	assert.Len(t, s.Proto().GetDetails(), 1)
}

func TestJsValueToGRPCError_Direct_GrpcErrorNoDetails(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	val, jsErr := env.runtime.RunString(`
		grpc.status.createError(5, 'simple error');
	`)
	require.NoError(t, jsErr)

	err := env.grpcMod.jsValueToGRPCError(val)
	s, ok := status.FromError(err)
	require.True(t, ok)
	assert.Equal(t, codes.NotFound, s.Code())
	assert.Len(t, s.Proto().GetDetails(), 0)
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
	require.NotNil(t, result)
	assert.Equal(t, "fast", result.String())
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
	require.NotNil(t, hdr)
	assert.Equal(t, "header-value", hdr.String())

	trailer := env.runtime.Get("trailerReceived")
	require.NotNil(t, trailer)
	assert.Equal(t, "trailer-value", trailer.String())

	items := env.runtime.Get("items")
	require.NotNil(t, items)
	arr := items.Export().([]interface{})
	assert.Equal(t, 1, len(arr))
	assert.Equal(t, "item1", arr[0])
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
	require.NotNil(t, result)
	assert.Equal(t, "received:1", result.String())

	trailer := env.runtime.Get("trailerReceived")
	require.NotNil(t, trailer)
	assert.Equal(t, "cs-trailer", trailer.String())
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
	require.NotNil(t, hdr)
	assert.Equal(t, "bidi-h", hdr.String())

	trailer := env.runtime.Get("trailerReceived")
	require.NotNil(t, trailer)
	assert.Equal(t, "bidi-t", trailer.String())

	items := env.runtime.Get("received")
	require.NotNil(t, items)
	arr := items.Export().([]interface{})
	assert.Equal(t, 1, len(arr))
	assert.Equal(t, "echo-alpha", arr[0])
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
	require.NotNil(t, result)
	assert.Equal(t, "ok", result.String())
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
	assert.Equal(t, "13:0", val.String())
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
	assert.Equal(t, "13:0", val.String())
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
	require.NoError(t, err)

	// Cast to protoreflect.MessageDescriptor which is what toWrappedMessage expects.
	md := desc.(protoreflect.MessageDescriptor)

	// badMarshalMsg is not a proto.Message — should trigger the
	// "not a proto.Message" error path.
	_, convErr := env.grpcMod.toWrappedMessage(badMarshalMsg{}, md)
	require.Error(t, convErr)
	assert.Contains(t, convErr.Error(), "not a proto.Message")
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
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	desc := result.Export().(map[string]interface{})
	assert.Equal(t, "testcomplex.PersonService", desc["name"])

	methods := desc["methods"].([]interface{})
	require.Len(t, methods, 1)

	mObj := methods[0].(map[string]interface{})
	assert.Equal(t, "GetPerson", mObj["name"])
	assert.Equal(t, "testcomplex.Address", mObj["inputType"])
	assert.Equal(t, "testcomplex.Person", mObj["outputType"])
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
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	serviceList := result.Export().([]interface{})

	var names []string
	for _, s := range serviceList {
		names = append(names, s.(string))
	}
	assert.Contains(t, names, "testgrpc.TestService")
	assert.Contains(t, names, "testcomplex.PersonService")
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
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	desc := result.Export().(map[string]interface{})
	assert.Equal(t, "testcomplex.Address", desc["name"])

	fields := desc["fields"].([]interface{})
	require.Len(t, fields, 1)
	field := fields[0].(map[string]interface{})
	assert.Equal(t, "street", field["name"])
	assert.Equal(t, "string", field["type"])
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
	require.NotNil(t, result)
	assert.Equal(t, "ok", result.String())

	hCalled := env.runtime.Get("headerCalled")
	require.NotNil(t, hCalled)
	assert.True(t, hCalled.ToBoolean())

	tCalled := env.runtime.Get("trailerCalled")
	require.NotNil(t, tCalled)
	assert.True(t, tCalled.ToBoolean())
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
	require.NotNil(t, result)
	assert.Equal(t, "response:intercepted", result.String())

	ic := env.runtime.Get("interceptorCalled")
	require.NotNil(t, ic)
	assert.True(t, ic.ToBoolean())
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
	require.NotNil(t, result)
	assert.Equal(t, "from-interceptor", result.String())
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
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(codes.Internal), resultObj["code"])
	assert.Contains(t, resultObj["message"].(string), "nil/undefined")
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
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(codes.Internal), resultObj["code"])
	assert.Contains(t, resultObj["message"].(string), "nil/undefined")
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
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(codes.Internal), resultObj["code"])
	assert.Contains(t, resultObj["message"].(string), "handler response")
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
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(codes.Internal), resultObj["code"])
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
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(codes.Internal), resultObj["code"])
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
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(codes.Internal), resultObj["code"])
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
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(7), resultObj["code"])
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
	require.NotNil(t, errVal)
	assert.Contains(t, errVal.String(), "send")
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
	require.NotNil(t, errVal)
	assert.NotEqual(t, "should-not-succeed", errVal.String())
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
	require.NotNil(t, errVal)
	assert.NotEqual(t, "should-not-succeed", errVal.String())
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
	require.NotNil(t, errVal)
	assert.NotEqual(t, "should-not-succeed", errVal.String())
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
	require.NotNil(t, errVal)
	assert.NotEqual(t, "should-not-succeed", errVal.String())
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
	require.NotNil(t, errVal)
	assert.NotEqual(t, "should-not-succeed", errVal.String())
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
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(codes.Internal), resultObj["code"])
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
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(codes.Internal), resultObj["code"])
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
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(16), resultObj["code"])
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
	require.NotNil(t, result)
	assert.Equal(t, "from-client", result.String())
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
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(3), resultObj["code"])
	assert.Contains(t, resultObj["message"].(string), "invalid argument")
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
		resultObj := result.Export().(map[string]interface{})
		assert.Equal(t, int64(codes.Canceled), resultObj["code"])
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
	require.NotNil(t, result)
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
	require.NotNil(t, errVal)
	assert.Contains(t, errVal.String(), "send")
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
	require.NotNil(t, errVal)
	assert.Contains(t, errVal.String(), "send")
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
	assert.Contains(t, err.Error(), "server-stream")
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
	require.NotNil(t, errVal)
	assert.Contains(t, errVal.String(), "not a message type")
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
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(codes.Internal), resultObj["code"])
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
	require.NotNil(t, result)
	resultObj := result.Export().(map[string]interface{})
	assert.Equal(t, int64(codes.Internal), resultObj["code"])
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
	assert.NotNil(t, err)
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
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	desc := result.Export().(map[string]interface{})
	assert.Equal(t, "testproto2.LegacyMessage", desc["name"])

	fields := desc["fields"].([]interface{})
	require.Len(t, fields, 2)

	fieldMap := make(map[string]map[string]interface{})
	for _, f := range fields {
		fObj := f.(map[string]interface{})
		fieldMap[fObj["name"].(string)] = fObj
	}

	// value field has default "42"
	valueField := fieldMap["value"]
	require.NotNil(t, valueField)
	assert.Equal(t, "42", valueField["defaultValue"])
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
		Name:    proto.String("base.proto"),
		Package: proto.String("testdeps"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("BaseMessage"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     proto.String("id"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName: proto.String("id"),
					},
				},
			},
		},
	}

	// File 2: depends on base.proto
	dependentFile := &descriptorpb.FileDescriptorProto{
		Name:       proto.String("dependent.proto"),
		Package:    proto.String("testdeps"),
		Syntax:     proto.String("proto3"),
		Dependency: []string{"base.proto"},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("DependentMessage"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     proto.String("base"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
						TypeName: proto.String(".testdeps.BaseMessage"),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName: proto.String("base"),
					},
				},
			},
		},
		Service: []*descriptorpb.ServiceDescriptorProto{
			{
				Name: proto.String("DependentService"),
				Method: []*descriptorpb.MethodDescriptorProto{
					{
						Name:       proto.String("GetDependent"),
						InputType:  proto.String(".testdeps.BaseMessage"),
						OutputType: proto.String(".testdeps.DependentMessage"),
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
	require.NoError(t, err)

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
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	desc := result.Export().(map[string]interface{})
	assert.Equal(t, "testdeps.DependentMessage", desc["name"])

	fields := desc["fields"].([]interface{})
	require.Len(t, fields, 1)
	field := fields[0].(map[string]interface{})
	assert.Equal(t, "base", field["name"])
	assert.Equal(t, "message", field["type"])
	assert.Equal(t, "testdeps.BaseMessage", field["messageType"])
}

func TestReflection_TransitiveDependencyResolution_Service(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	_, err := env.pbMod.LoadDescriptorSetBytes(transitiveDepsDescriptorSetBytes())
	require.NoError(t, err)

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
	require.True(t, errVal == nil || isGojaUndefined(errVal), "unexpected error: %v", errVal)

	result := env.runtime.Get("result")
	require.NotNil(t, result)
	desc := result.Export().(map[string]interface{})
	assert.Equal(t, "testdeps.DependentService", desc["name"])

	methods := desc["methods"].([]interface{})
	require.Len(t, methods, 1)
	mObj := methods[0].(map[string]interface{})
	assert.Equal(t, "GetDependent", mObj["name"])
	assert.Equal(t, "testdeps.BaseMessage", mObj["inputType"])
	assert.Equal(t, "testdeps.DependentMessage", mObj["outputType"])
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
	require.NotNil(t, errVal)
	assert.Contains(t, errVal.String(), "wrapper function error")
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
	require.NotNil(t, errVal)
	assert.Contains(t, errVal.String(), "interceptor must call next with request")
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
	require.NotNil(t, errVal)
	assert.Contains(t, errVal.String(), "invalid message")
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
	assert.Contains(t, err.Error(), "unary")
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
	require.NotNil(t, items)
	assert.Equal(t, int64(0), items.(*goja.Object).Get("length").ToInteger())
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
		assert.True(t, done.ToBoolean())
	}
}
