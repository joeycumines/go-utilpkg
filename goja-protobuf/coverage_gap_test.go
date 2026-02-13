package gojaprotobuf

import (
	"fmt"
	"math"
	"testing"

	"github.com/dop251/goja"
	gojarequire "github.com/dop251/goja_nodejs/require"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// ---------------------------------------------------------------------------
// Test helpers for coverage test fixtures
// ---------------------------------------------------------------------------

// buildExtensionType creates a proto2 file with an extendable message and
// a top-level string extension. Returns the ExtensionType ready for
// registration.
func buildExtensionType(t *testing.T) (protoreflect.ExtensionType, protoreflect.FullName, protoreflect.FullName, protoreflect.FieldNumber) {
	t.Helper()
	fdp := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("exttest.proto"),
		Package: proto.String("exttest"),
		Syntax:  proto.String("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("ExtMsg"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:   proto.String("id"),
				Number: proto.Int32(1),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			}},
			ExtensionRange: []*descriptorpb.DescriptorProto_ExtensionRange{{
				Start: proto.Int32(100),
				End:   proto.Int32(200),
			}},
		}},
		Extension: []*descriptorpb.FieldDescriptorProto{{
			Name:     proto.String("my_ext_field"),
			Number:   proto.Int32(100),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			Extendee: proto.String(".exttest.ExtMsg"),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
		}},
	}
	fd, err := protodesc.NewFile(fdp, nil)
	require.NoError(t, err)
	extDesc := fd.Extensions().Get(0)
	xt := dynamicpb.NewExtensionType(extDesc)
	return xt, extDesc.FullName(), "exttest.ExtMsg", 100
}

// multiKeyMapFileDescriptorProto returns a proto for a message with map
// fields keyed by bool, int32, int64, uint32, uint64.
func multiKeyMapFileDescriptorProto() *descriptorpb.FileDescriptorProto {
	mkEntry := func(name string, keyType descriptorpb.FieldDescriptorProto_Type) *descriptorpb.DescriptorProto {
		return &descriptorpb.DescriptorProto{
			Name: proto.String(name),
			Field: []*descriptorpb.FieldDescriptorProto{
				{Name: proto.String("key"), Number: proto.Int32(1), Type: keyType.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("key")},
				{Name: proto.String("value"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("value")},
			},
			Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)},
		}
	}
	mkField := func(name string, num int32, entry string) *descriptorpb.FieldDescriptorProto {
		return &descriptorpb.FieldDescriptorProto{
			Name: proto.String(name), Number: proto.Int32(num),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
			TypeName: proto.String(".mapkeys.MultiKeyMap." + entry),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
			JsonName: proto.String(name),
		}
	}
	return &descriptorpb.FileDescriptorProto{
		Name:    proto.String("mapkeys.proto"),
		Package: proto.String("mapkeys"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("MultiKeyMap"),
			Field: []*descriptorpb.FieldDescriptorProto{
				mkField("bool_map", 1, "BoolMapEntry"),
				mkField("int32_map", 2, "Int32MapEntry"),
				mkField("int64_map", 3, "Int64MapEntry"),
				mkField("uint32_map", 4, "Uint32MapEntry"),
				mkField("uint64_map", 5, "Uint64MapEntry"),
			},
			NestedType: []*descriptorpb.DescriptorProto{
				mkEntry("BoolMapEntry", descriptorpb.FieldDescriptorProto_TYPE_BOOL),
				mkEntry("Int32MapEntry", descriptorpb.FieldDescriptorProto_TYPE_INT32),
				mkEntry("Int64MapEntry", descriptorpb.FieldDescriptorProto_TYPE_INT64),
				mkEntry("Uint32MapEntry", descriptorpb.FieldDescriptorProto_TYPE_UINT32),
				mkEntry("Uint64MapEntry", descriptorpb.FieldDescriptorProto_TYPE_UINT64),
			},
		}},
	}
}

// containerMessageProto returns a file with a Container message that has
// a repeated nested message field, to test jsObjectToMessage with lists
// of messages.
func containerMessageProto() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:       proto.String("container.proto"),
		Package:    proto.String("container"),
		Syntax:     proto.String("proto3"),
		Dependency: []string{"test.proto"},
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Container"),
			Field: []*descriptorpb.FieldDescriptorProto{
				{
					Name: proto.String("inner"), Number: proto.Int32(1),
					Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
					TypeName: proto.String(".test.NestedInner"),
					Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					JsonName: proto.String("inner"),
				},
				{
					Name: proto.String("inners"), Number: proto.Int32(2),
					Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
					TypeName: proto.String(".test.NestedInner"),
					Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					JsonName: proto.String("inners"),
				},
			},
		}},
	}
}

// newTestEnvWithMapKeys creates a test environment with both the base
// descriptors and the multi-key-map descriptors loaded.
func newTestEnvWithMapKeys(t *testing.T) *testEnv {
	t.Helper()
	env := newTestEnv(t)
	data, err := proto.Marshal(multiKeyMapFileDescriptorProto())
	require.NoError(t, err)
	_, err = env.m.loadFileDescriptorProtoBytes(data)
	require.NoError(t, err)
	return env
}

// ---------------------------------------------------------------------------
// helpers.go: combinedTypeResolver
// ---------------------------------------------------------------------------

func TestCombinedTypeResolver_FindMessageByName(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.SimpleMessage")
	require.NoError(t, err)
	mt := dynamicpb.NewMessageType(md)

	t.Run("local_hit", func(t *testing.T) {
		local := new(protoregistry.Types)
		global := new(protoregistry.Types)
		require.NoError(t, local.RegisterMessage(mt))
		r := &combinedTypeResolver{local: local, global: global}
		result, err := r.FindMessageByName("test.SimpleMessage")
		require.NoError(t, err)
		assert.Equal(t, "test.SimpleMessage", string(result.Descriptor().FullName()))
	})

	t.Run("global_fallback", func(t *testing.T) {
		local := new(protoregistry.Types)
		global := new(protoregistry.Types)
		require.NoError(t, global.RegisterMessage(mt))
		r := &combinedTypeResolver{local: local, global: global}
		result, err := r.FindMessageByName("test.SimpleMessage")
		require.NoError(t, err)
		assert.Equal(t, "test.SimpleMessage", string(result.Descriptor().FullName()))
	})

	t.Run("both_miss", func(t *testing.T) {
		r := &combinedTypeResolver{local: new(protoregistry.Types), global: new(protoregistry.Types)}
		_, err := r.FindMessageByName("nonexistent.Foo")
		assert.Error(t, err)
	})
}

func TestCombinedTypeResolver_FindMessageByURL(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.SimpleMessage")
	require.NoError(t, err)
	mt := dynamicpb.NewMessageType(md)

	t.Run("local_hit", func(t *testing.T) {
		local := new(protoregistry.Types)
		global := new(protoregistry.Types)
		require.NoError(t, local.RegisterMessage(mt))
		r := &combinedTypeResolver{local: local, global: global}
		result, err := r.FindMessageByURL("test.SimpleMessage")
		require.NoError(t, err)
		assert.Equal(t, "test.SimpleMessage", string(result.Descriptor().FullName()))
	})

	t.Run("global_fallback", func(t *testing.T) {
		local := new(protoregistry.Types)
		global := new(protoregistry.Types)
		require.NoError(t, global.RegisterMessage(mt))
		r := &combinedTypeResolver{local: local, global: global}
		result, err := r.FindMessageByURL("test.SimpleMessage")
		require.NoError(t, err)
		assert.Equal(t, "test.SimpleMessage", string(result.Descriptor().FullName()))
	})

	t.Run("both_miss", func(t *testing.T) {
		r := &combinedTypeResolver{local: new(protoregistry.Types), global: new(protoregistry.Types)}
		_, err := r.FindMessageByURL("nonexistent.Foo")
		assert.Error(t, err)
	})
}

func TestCombinedTypeResolver_FindExtensionByName(t *testing.T) {
	xt, extName, _, _ := buildExtensionType(t)

	t.Run("local_hit", func(t *testing.T) {
		local := new(protoregistry.Types)
		global := new(protoregistry.Types)
		require.NoError(t, local.RegisterExtension(xt))
		r := &combinedTypeResolver{local: local, global: global}
		result, err := r.FindExtensionByName(extName)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("global_fallback", func(t *testing.T) {
		local := new(protoregistry.Types)
		global := new(protoregistry.Types)
		require.NoError(t, global.RegisterExtension(xt))
		r := &combinedTypeResolver{local: local, global: global}
		result, err := r.FindExtensionByName(extName)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("both_miss", func(t *testing.T) {
		r := &combinedTypeResolver{local: new(protoregistry.Types), global: new(protoregistry.Types)}
		_, err := r.FindExtensionByName("nonexistent.ext")
		assert.Error(t, err)
	})
}

func TestCombinedTypeResolver_FindExtensionByNumber(t *testing.T) {
	xt, _, msgName, fieldNum := buildExtensionType(t)

	t.Run("local_hit", func(t *testing.T) {
		local := new(protoregistry.Types)
		global := new(protoregistry.Types)
		require.NoError(t, local.RegisterExtension(xt))
		r := &combinedTypeResolver{local: local, global: global}
		result, err := r.FindExtensionByNumber(msgName, protoreflect.FieldNumber(fieldNum))
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("global_fallback", func(t *testing.T) {
		local := new(protoregistry.Types)
		global := new(protoregistry.Types)
		require.NoError(t, global.RegisterExtension(xt))
		r := &combinedTypeResolver{local: local, global: global}
		result, err := r.FindExtensionByNumber(msgName, protoreflect.FieldNumber(fieldNum))
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("both_miss", func(t *testing.T) {
		r := &combinedTypeResolver{local: new(protoregistry.Types), global: new(protoregistry.Types)}
		_, err := r.FindExtensionByNumber("nonexistent.Msg", 999)
		assert.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// helpers.go: combinedFileResolver fallback paths
// ---------------------------------------------------------------------------

func TestCombinedFileResolver_FindFileByPath_GlobalFallback(t *testing.T) {
	// Create a file descriptor registered only in global.
	fdp := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("globalonly.proto"),
		Package: proto.String("globalonly"),
		Syntax:  proto.String("proto3"),
	}
	fd, err := protodesc.NewFile(fdp, nil)
	require.NoError(t, err)

	global := new(protoregistry.Files)
	require.NoError(t, global.RegisterFile(fd))

	r := &combinedFileResolver{local: new(protoregistry.Files), global: global}
	result, err := r.FindFileByPath("globalonly.proto")
	require.NoError(t, err)
	assert.Equal(t, "globalonly.proto", result.Path())
}

func TestCombinedFileResolver_FindFileByPath_LocalHit(t *testing.T) {
	fdp := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("localonly.proto"),
		Package: proto.String("localonly"),
		Syntax:  proto.String("proto3"),
	}
	fd, err := protodesc.NewFile(fdp, nil)
	require.NoError(t, err)

	local := new(protoregistry.Files)
	require.NoError(t, local.RegisterFile(fd))

	r := &combinedFileResolver{local: local, global: new(protoregistry.Files)}
	result, err := r.FindFileByPath("localonly.proto")
	require.NoError(t, err)
	assert.Equal(t, "localonly.proto", result.Path())
}

func TestCombinedFileResolver_FindDescriptorByName_GlobalFallback(t *testing.T) {
	fdp := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("globalonly2.proto"),
		Package: proto.String("globalonly2"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Msg"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name: proto.String("x"), Number: proto.Int32(1),
				Type:  descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
				Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			}},
		}},
	}
	fd, err := protodesc.NewFile(fdp, nil)
	require.NoError(t, err)

	global := new(protoregistry.Files)
	require.NoError(t, global.RegisterFile(fd))

	r := &combinedFileResolver{local: new(protoregistry.Files), global: global}
	desc, err := r.FindDescriptorByName("globalonly2.Msg")
	require.NoError(t, err)
	assert.Equal(t, protoreflect.FullName("globalonly2.Msg"), desc.FullName())
}

func TestCombinedFileResolver_FindDescriptorByName_LocalHit(t *testing.T) {
	fdp := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("localonly3.proto"),
		Package: proto.String("localonly3"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Msg"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name: proto.String("x"), Number: proto.Int32(1),
				Type:  descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
				Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			}},
		}},
	}
	fd, err := protodesc.NewFile(fdp, nil)
	require.NoError(t, err)

	local := new(protoregistry.Files)
	require.NoError(t, local.RegisterFile(fd))

	r := &combinedFileResolver{local: local, global: new(protoregistry.Files)}
	desc, err := r.FindDescriptorByName("localonly3.Msg")
	require.NoError(t, err)
	assert.Equal(t, protoreflect.FullName("localonly3.Msg"), desc.FullName())
}

// ---------------------------------------------------------------------------
// helpers.go: extractBytes — non-bytes error
// ---------------------------------------------------------------------------

func TestExtractBytes_NonBytesValue(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	// A number should fail all extraction attempts.
	_, err = m.extractBytes(rt.ToValue(42))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected Uint8Array or ArrayBuffer")
}

// ---------------------------------------------------------------------------
// helpers.go: newUint8Array — fallback paths
// ---------------------------------------------------------------------------

func TestNewUint8Array_NoGlobal(t *testing.T) {
	rt := goja.New()
	// Remove the Uint8Array global.
	require.NoError(t, rt.Set("Uint8Array", goja.Undefined()))
	m, err := New(rt)
	require.NoError(t, err)

	result := m.newUint8Array([]byte{1, 2, 3})
	assert.NotNil(t, result)
	// Should return an ArrayBuffer directly.
	exported := result.Export()
	_, isAB := exported.(goja.ArrayBuffer)
	assert.True(t, isAB, "expected ArrayBuffer, got %T", exported)
}

func TestNewUint8Array_ConstructorError(t *testing.T) {
	rt := goja.New()
	// Set Uint8Array to something that's not a constructor.
	require.NoError(t, rt.Set("Uint8Array", "not_a_constructor"))
	m, err := New(rt)
	require.NoError(t, err)

	result := m.newUint8Array([]byte{1, 2, 3})
	assert.NotNil(t, result)
	// Should fall back to ArrayBuffer.
	exported := result.Export()
	_, isAB := exported.(goja.ArrayBuffer)
	assert.True(t, isAB, "expected ArrayBuffer fallback, got %T", exported)
}

// ---------------------------------------------------------------------------
// helpers.go: extractMessageDesc — invalid holder / non-object path
// ---------------------------------------------------------------------------

func TestExtractMessageDesc_InvalidHolder(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	// Object with _pbMsgDesc set to wrong type.
	obj := rt.NewObject()
	require.NoError(t, obj.Set("_pbMsgDesc", "not a holder"))
	_, err = m.extractMessageDesc(obj)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a protobuf message type constructor")
}

func TestExtractMessageDesc_NoDescProperty(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	// Plain object without _pbMsgDesc.
	obj := rt.NewObject()
	_, err = m.extractMessageDesc(obj)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a protobuf message type constructor")
}

func TestExtractMessageDesc_NilHolder(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	// Object with _pbMsgDesc set to nil *messageDescHolder.
	obj := rt.NewObject()
	require.NoError(t, obj.Set("_pbMsgDesc", (*messageDescHolder)(nil)))
	_, err = m.extractMessageDesc(obj)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a protobuf message type constructor")
}

// ---------------------------------------------------------------------------
// descriptors.go: loadFileDescriptorProtoBytes — already-registered path
// ---------------------------------------------------------------------------

func TestLoadFileDescriptorProtoBytes_AlreadyRegistered(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	fdp := testFileDescriptorProto()
	data, err := proto.Marshal(fdp)
	require.NoError(t, err)

	// Load once.
	names1, err := m.loadFileDescriptorProtoBytes(data)
	require.NoError(t, err)
	assert.NotEmpty(t, names1)

	// Load again — should return nil, nil since already registered.
	names2, err := m.loadFileDescriptorProtoBytes(data)
	require.NoError(t, err)
	assert.Nil(t, names2)
}

// ---------------------------------------------------------------------------
// descriptors.go: jsLoadFileDescriptorProto via JS (was 0%)
// ---------------------------------------------------------------------------

func TestJsLoadFileDescriptorProto_ViaJS(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	pb := rt.NewObject()
	m.setupExports(pb)
	require.NoError(t, rt.Set("pb", pb))

	fdp := testFileDescriptorProto()
	data, err := proto.Marshal(fdp)
	require.NoError(t, err)
	require.NoError(t, rt.Set("protoBytes", rt.NewArrayBuffer(data)))

	v, err := rt.RunString(`
		var names = pb.loadFileDescriptorProto(new Uint8Array(protoBytes));
		names.length > 0
	`)
	require.NoError(t, err)
	assert.True(t, v.ToBoolean())
}

func TestJsLoadFileDescriptorProto_InvalidInput(t *testing.T) {
	env := newTestEnv(t)
	// Pass a string — should fail extractBytes.
	env.mustFail(t, `pb.loadFileDescriptorProto("not bytes")`)
}

func TestJsLoadFileDescriptorProto_BadProtoData(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	pb := rt.NewObject()
	m.setupExports(pb)
	require.NoError(t, rt.Set("pb", pb))

	// File that depends on nonexistent dependency → protodesc.NewFile error.
	fdp := &descriptorpb.FileDescriptorProto{
		Name:       proto.String("dep.proto"),
		Package:    proto.String("dep"),
		Syntax:     proto.String("proto3"),
		Dependency: []string{"nonexistent.proto"},
	}
	data, err := proto.Marshal(fdp)
	require.NoError(t, err)
	require.NoError(t, rt.Set("badBytes", rt.NewArrayBuffer(data)))

	_, err = rt.RunString(`pb.loadFileDescriptorProto(new Uint8Array(badBytes))`)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// descriptors.go: jsLoadDescriptorSet — loadDescriptorSetBytes error via JS
// ---------------------------------------------------------------------------

func TestJsLoadDescriptorSet_BadProtoData(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	pb := rt.NewObject()
	m.setupExports(pb)
	require.NoError(t, rt.Set("pb", pb))

	// An FDS with a message referencing a non-existent type.
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{{
			Name:    proto.String("bad2.proto"),
			Package: proto.String("bad2"),
			Syntax:  proto.String("proto3"),
			MessageType: []*descriptorpb.DescriptorProto{{
				Name: proto.String("Bad"),
				Field: []*descriptorpb.FieldDescriptorProto{{
					Name: proto.String("ref"), Number: proto.Int32(1),
					Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
					TypeName: proto.String(".nonexist.Missing"),
					Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				}},
			}},
		}},
	}
	data, err := proto.Marshal(fds)
	require.NoError(t, err)
	require.NoError(t, rt.Set("badBytes", rt.NewArrayBuffer(data)))

	_, err = rt.RunString(`pb.loadDescriptorSet(new Uint8Array(badBytes))`)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// message.go: unwrapMessage — non-Object value
// ---------------------------------------------------------------------------

func TestUnwrapMessage_NonObject(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	// Pass a primitive value (number) — val.(*goja.Object) should fail.
	_, err = m.unwrapMessage(rt.ToValue(42))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected protobuf message object")
}

func TestUnwrapMessage_ObjectWithoutPbMsg(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	// Plain object without _pbMsg.
	obj := rt.NewObject()
	_, err = m.unwrapMessage(obj)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a protobuf message wrapper")
}

func TestUnwrapMessage_InvalidPbMsg(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	// Object with _pbMsg set to wrong type.
	obj := rt.NewObject()
	require.NoError(t, obj.Set("_pbMsg", "wrong"))
	_, err = m.unwrapMessage(obj)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a protobuf message wrapper")
}

// ---------------------------------------------------------------------------
// message.go: wrapMessage — set error paths (repeated/map)
// ---------------------------------------------------------------------------

func TestWrapMessage_SetRepeatedError(t *testing.T) {
	env := newTestEnv(t)
	// Trigger setRepeatedFromGoja error: int32 overflow in repeated field.
	env.mustFail(t, `
		var msg = new (pb.messageType('test.RepeatedMessage'))();
		msg.set('numbers', [9999999999]);
	`)
}

func TestWrapMessage_SetMapError(t *testing.T) {
	env := newTestEnv(t)
	// Trigger setMapFromGoja error: int32 overflow in map<string,int32> value.
	env.run(t, `var MM = pb.messageType('test.MapMessage')`)
	env.mustFail(t, `
		var msg = new MM();
		msg.set('counts', {k: 9999999999});
	`)
}

// ---------------------------------------------------------------------------
// message.go: wrapRepeatedField — set error, no-length, sparse array
// ---------------------------------------------------------------------------

func TestRepeatedField_SetError(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.RepeatedMessage'))();
		msg.get('numbers').add(10);
	`)
	// set() on repeated element: overflow int32.
	env.mustFail(t, `msg.get('numbers').set(0, 9999999999)`)
}

func TestRepeatedField_AddError(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.RepeatedMessage'))();
	`)
	// add() with overflow int32.
	env.mustFail(t, `msg.get('numbers').add(9999999999)`)
}

// ---------------------------------------------------------------------------
// message.go: wrapMapField — get/set/has/delete error paths
// ---------------------------------------------------------------------------

func TestMapField_GetKeyError(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	env.run(t, `var MKM = pb.messageType('mapkeys.MultiKeyMap')`)
	env.run(t, `var msg = new MKM()`)
	// int32 map key error: BigInt that overflows int64 → gojaToInt64 error → gojaToProtoMapKey error.
	env.mustFail(t, `msg.get('int32_map').get(BigInt('9223372036854775808'))`)
}

func TestMapField_SetKeyError(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	env.run(t, `var MKM = pb.messageType('mapkeys.MultiKeyMap')`)
	env.run(t, `var msg = new MKM()`)
	env.mustFail(t, `msg.get('int32_map').set(BigInt('9223372036854775808'), 'v')`)
}

func TestMapField_HasKeyError(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	env.run(t, `var MKM = pb.messageType('mapkeys.MultiKeyMap')`)
	env.run(t, `var msg = new MKM()`)
	env.mustFail(t, `msg.get('int32_map').has(BigInt('9223372036854775808'))`)
}

func TestMapField_DeleteKeyError(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	env.run(t, `var MKM = pb.messageType('mapkeys.MultiKeyMap')`)
	env.run(t, `var msg = new MKM()`)
	env.mustFail(t, `msg.get('int32_map').delete(BigInt('9223372036854775808'))`)
}

func TestMapField_SetValueError(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	env.run(t, `var MKM = pb.messageType('mapkeys.MultiKeyMap')`)
	env.run(t, `var msg = new MKM()`)
	// Value is string → gojaToProtoValue for StringKind always succeeds.
	// We need a map with a different value type. The existing MapMessage has
	// map<string, int32> for counts. Let's trigger int32 overflow in the value.
	env.run(t, `var MM = pb.messageType('test.MapMessage')`)
	env.run(t, `var mm = new MM()`)
	env.mustFail(t, `mm.get('counts').set('key', 9999999999)`)
}

// ---------------------------------------------------------------------------
// module.go: New — error option path
// ---------------------------------------------------------------------------

// errorOption is a test Option that always returns an error.
type errorOption struct{}

func (o *errorOption) applyOption(*moduleOptions) error {
	return fmt.Errorf("test option error")
}

func TestNew_ErrorOption(t *testing.T) {
	rt := goja.New()
	_, err := New(rt, &errorOption{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "test option error")
}

// ---------------------------------------------------------------------------
// options.go: resolveOptions — error path
// ---------------------------------------------------------------------------

func TestResolveOptions_Error(t *testing.T) {
	_, err := resolveOptions([]Option{&errorOption{}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "test option error")
}

// ---------------------------------------------------------------------------
// register.go: Enable — error during require
// ---------------------------------------------------------------------------

func TestRequire_ErrorOption(t *testing.T) {
	rt := goja.New()
	registry := gojarequire.NewRegistry()
	registry.RegisterNativeModule("protobuf", Require(&errorOption{}))
	registry.Enable(rt)

	// require('protobuf') should panic → goja throws.
	_, err := rt.RunString(`require('protobuf')`)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// types.go: findMessageDescriptor / findEnumDescriptor — global fallback
// ---------------------------------------------------------------------------

func TestFindMessageDescriptor_GlobalFallback(t *testing.T) {
	// Create a Module with a custom resolver that has a type registered.
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.SimpleMessage")
	require.NoError(t, err)
	mt := dynamicpb.NewMessageType(md)

	// Create new module with custom resolver containing the type.
	customResolver := new(protoregistry.Types)
	require.NoError(t, customResolver.RegisterMessage(mt))

	rt := goja.New()
	m, err := New(rt, WithResolver(customResolver))
	require.NoError(t, err)

	// Type is not in localTypes (empty) but IS in resolver → global fallback.
	result, err := m.findMessageDescriptor("test.SimpleMessage")
	require.NoError(t, err)
	assert.Equal(t, protoreflect.FullName("test.SimpleMessage"), result.FullName())
}

func TestFindEnumDescriptor_GlobalFallback(t *testing.T) {
	env := newTestEnv(t)
	// Find a known enum in localTypes.
	ed, err := env.m.findEnumDescriptor("test.TestEnum")
	require.NoError(t, err)
	et := dynamicpb.NewEnumType(ed)

	// Create new module with custom resolver containing the enum.
	customResolver := new(protoregistry.Types)
	require.NoError(t, customResolver.RegisterEnum(et))

	rt := goja.New()
	m, err := New(rt, WithResolver(customResolver))
	require.NoError(t, err)

	// Enum is not in localTypes but IS in resolver → global fallback.
	result, err := m.findEnumDescriptor("test.TestEnum")
	require.NoError(t, err)
	assert.Equal(t, protoreflect.FullName("test.TestEnum"), result.FullName())
}

// ---------------------------------------------------------------------------
// conversion.go: protoMessageToGoja — non-dynamic message
// ---------------------------------------------------------------------------

func TestProtoMessageToGoja_NonDynamic(t *testing.T) {
	env := newTestEnv(t)

	// Use a generated protobuf message (FileDescriptorProto).
	fdp := &descriptorpb.FileDescriptorProto{
		Name:   proto.String("nondynamic.proto"),
		Syntax: proto.String("proto3"),
	}
	result := env.m.protoMessageToGoja(fdp.ProtoReflect())
	assert.NotNil(t, result)

	// The wrapper should expose the $type accessor.
	obj := result.ToObject(env.rt)
	typeVal := obj.Get("$type")
	assert.Contains(t, typeVal.String(), "FileDescriptorProto")
}

// ---------------------------------------------------------------------------
// conversion.go: gojaToInt64 — BigInt overflow + default case
// ---------------------------------------------------------------------------

func TestGojaToInt64_BigIntOverflow(t *testing.T) {
	env := newTestEnv(t)
	// BigInt that exceeds int64 range.
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)
	env.mustFail(t, `msg.set('int64_val', BigInt('9223372036854775808'))`)
}

func TestGojaToInt64_FloatDefault(t *testing.T) {
	env := newTestEnv(t)
	// A float value triggers the default case in gojaToInt64.
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)
	env.run(t, `msg.set('int64_val', 3.7)`)
	v := env.run(t, `msg.get('int64_val')`)
	assert.Equal(t, int64(3), v.ToInteger())
}

// ---------------------------------------------------------------------------
// conversion.go: gojaToUint64 — BigInt overflow + default cases
// ---------------------------------------------------------------------------

func TestGojaToUint64_BigIntOverflow(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)
	// 2^64 = 18446744073709551616 → overflows uint64.
	env.mustFail(t, `msg.set('uint64_val', BigInt('18446744073709551616'))`)
}

func TestGojaToUint64_FloatPositive(t *testing.T) {
	env := newTestEnv(t)
	// Float triggers default case, positive → uint conversion.
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)
	env.run(t, `msg.set('uint64_val', 3.7)`)
	v := env.run(t, `msg.get('uint64_val')`)
	assert.Equal(t, int64(3), v.ToInteger())
}

func TestGojaToUint64_FloatNegative(t *testing.T) {
	env := newTestEnv(t)
	// Float triggers default case, negative → error.
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)
	env.mustFail(t, `msg.set('uint64_val', -1.5)`)
}

// ---------------------------------------------------------------------------
// conversion.go: gojaToProtoValue — bytes error path
// ---------------------------------------------------------------------------

func TestGojaToProtoValue_BytesError(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)
	// Set bytes_val to a number → extractBytes error.
	env.mustFail(t, `msg.set('bytes_val', 42)`)
}

// ---------------------------------------------------------------------------
// conversion.go: jsObjectToMessage — repeated, map, error branches
// ---------------------------------------------------------------------------

func TestJsObjectToMessage_WithRepeatedField(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.AllTypes")
	require.NoError(t, err)

	arrVal, err := env.rt.RunString(`([10, 20, 30])`)
	require.NoError(t, err)

	obj := env.rt.NewObject()
	require.NoError(t, obj.Set("repeated_int32", arrVal))
	msg, err := env.m.jsObjectToMessage(obj, md)
	require.NoError(t, err)

	fd := md.Fields().ByName("repeated_int32")
	assert.Equal(t, 3, msg.Get(fd).List().Len())
}

func TestJsObjectToMessage_WithMapField(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.AllTypes")
	require.NoError(t, err)

	mapObj := env.rt.NewObject()
	require.NoError(t, mapObj.Set("k1", "v1"))
	require.NoError(t, mapObj.Set("k2", "v2"))

	obj := env.rt.NewObject()
	require.NoError(t, obj.Set("tags", mapObj))
	msg, err := env.m.jsObjectToMessage(obj, md)
	require.NoError(t, err)

	fd := md.Fields().ByName("tags")
	assert.Equal(t, 2, msg.Get(fd).Map().Len())
}

func TestJsObjectToMessage_ScalarError(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.AllTypes")
	require.NoError(t, err)

	// bytes_val with a number triggers extractBytes error.
	obj := env.rt.NewObject()
	require.NoError(t, obj.Set("bytes_val", 42))
	_, err = env.m.jsObjectToMessage(obj, md)
	assert.Error(t, err)
}

func TestJsObjectToMessage_RepeatedError(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.AllTypes")
	require.NoError(t, err)

	// Pass a repeated int32 with overflow values.
	arrVal, err := env.rt.RunString(`([9999999999])`)
	require.NoError(t, err)
	obj := env.rt.NewObject()
	require.NoError(t, obj.Set("repeated_int32", arrVal))
	_, err = env.m.jsObjectToMessage(obj, md)
	assert.Error(t, err)
}

func TestJsObjectToMessage_MapError(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.MapMessage")
	require.NoError(t, err)

	// counts is map<string, int32>. Set value to overflow int32.
	countsObj := env.rt.NewObject()
	require.NoError(t, countsObj.Set("k", 9999999999))
	obj := env.rt.NewObject()
	require.NoError(t, obj.Set("counts", countsObj))
	_, err = env.m.jsObjectToMessage(obj, md)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// conversion.go: setRepeatedFromGoja — no-length, sparse array
// ---------------------------------------------------------------------------

func TestSetRepeatedFromGoja_NoLength(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.RepeatedMessage")
	require.NoError(t, err)
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("items")

	// Object without a 'length' property.
	noLength := env.rt.NewObject()
	err = env.m.setRepeatedFromGoja(msg, fd, noLength)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected array for repeated field")
}

func TestSetRepeatedFromGoja_SparseArray(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.RepeatedMessage")
	require.NoError(t, err)
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("items")

	// Simulate a sparse array: length=3, only indices 0 and 2 set.
	sparse := env.rt.NewObject()
	require.NoError(t, sparse.Set("length", 3))
	require.NoError(t, sparse.Set("0", "first"))
	// index 1 missing
	require.NoError(t, sparse.Set("2", "third"))

	err = env.m.setRepeatedFromGoja(msg, fd, sparse)
	require.NoError(t, err)
	// Only 2 elements should be appended (sparse index 1 skipped).
	list := msg.Get(fd).List()
	assert.Equal(t, 2, list.Len())
}

func TestSetRepeatedFromGoja_ConversionError(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.RepeatedMessage")
	require.NoError(t, err)
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("numbers") // repeated int32

	// Array with overflow int32 value.
	arrVal, err := env.rt.RunString(`([9999999999])`)
	require.NoError(t, err)
	err = env.m.setRepeatedFromGoja(msg, fd, arrVal)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "repeated field")
}

// ---------------------------------------------------------------------------
// conversion.go: setMapFromGoja — JS Map with entries()
// ---------------------------------------------------------------------------

func TestSetMapFromGoja_JSMap(t *testing.T) {
	env := newTestEnv(t)

	v := env.run(t, `
		var MM = pb.messageType('test.MapMessage');
		var msg = new MM();
		var m = new Map();
		m.set('key1', 'val1');
		m.set('key2', 'val2');
		msg.set('tags', m);
		msg.get('tags').get('key1') === 'val1' && msg.get('tags').get('key2') === 'val2' && msg.get('tags').size === 2
	`)
	assert.True(t, v.ToBoolean())
}

func TestSetMapFromGoja_JSMapKeyError(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	md, err := env.m.findMessageDescriptor("mapkeys.MultiKeyMap")
	require.NoError(t, err)
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("int32_map")

	// Create a JS Map with a key that causes gojaToProtoMapKey error.
	mapVal, err := env.rt.RunString(`
		var m = new Map();
		m.set(BigInt('9223372036854775808'), 'v');
		m
	`)
	require.NoError(t, err)
	err = env.m.setMapFromGoja(msg, fd, mapVal)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "map field")
}

func TestSetMapFromGoja_JSMapValueError(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.MapMessage")
	require.NoError(t, err)
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("counts") // map<string, int32>

	// JS Map with int32 overflow value.
	mapVal, err := env.rt.RunString(`
		var m = new Map();
		m.set('k', 9999999999);
		m
	`)
	require.NoError(t, err)
	err = env.m.setMapFromGoja(msg, fd, mapVal)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "map field")
}

// ---------------------------------------------------------------------------
// conversion.go: gojaToProtoMapKey — all key types
// ---------------------------------------------------------------------------

func TestGojaToProtoMapKey_AllTypes(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	md, err := env.m.findMessageDescriptor("mapkeys.MultiKeyMap")
	require.NoError(t, err)

	tests := []struct {
		field string
		jsKey string
	}{
		{"bool_map", "true"},
		{"int32_map", "42"},
		{"int64_map", "100"},
		{"uint32_map", "200"},
		{"uint64_map", "300"},
	}
	for _, tc := range tests {
		t.Run(tc.field, func(t *testing.T) {
			fd := md.Fields().ByName(protoreflect.Name(tc.field))
			require.NotNil(t, fd, "field %s not found", tc.field)
			keyDesc := fd.MapKey()

			val, err := env.rt.RunString(tc.jsKey)
			require.NoError(t, err)
			mk, err := env.m.gojaToProtoMapKey(val, keyDesc)
			require.NoError(t, err)
			assert.True(t, mk.IsValid())
		})
	}
}

func TestGojaToProtoMapKey_Int32Error(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	md, err := env.m.findMessageDescriptor("mapkeys.MultiKeyMap")
	require.NoError(t, err)
	fd := md.Fields().ByName("int32_map")
	keyDesc := fd.MapKey()

	// BigInt that overflows int64.
	val, err := env.rt.RunString(`BigInt('9223372036854775808')`)
	require.NoError(t, err)
	_, err = env.m.gojaToProtoMapKey(val, keyDesc)
	assert.Error(t, err)
}

func TestGojaToProtoMapKey_Int64Error(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	md, err := env.m.findMessageDescriptor("mapkeys.MultiKeyMap")
	require.NoError(t, err)
	fd := md.Fields().ByName("int64_map")
	keyDesc := fd.MapKey()

	// BigInt that overflows int64.
	val, err := env.rt.RunString(`BigInt('9223372036854775808')`)
	require.NoError(t, err)
	_, err = env.m.gojaToProtoMapKey(val, keyDesc)
	assert.Error(t, err)
}

func TestGojaToProtoMapKey_Uint32Error(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	md, err := env.m.findMessageDescriptor("mapkeys.MultiKeyMap")
	require.NoError(t, err)
	fd := md.Fields().ByName("uint32_map")
	keyDesc := fd.MapKey()

	// Negative value.
	val := env.rt.ToValue(-1)
	_, err = env.m.gojaToProtoMapKey(val, keyDesc)
	assert.Error(t, err)
}

func TestGojaToProtoMapKey_Uint64Error(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	md, err := env.m.findMessageDescriptor("mapkeys.MultiKeyMap")
	require.NoError(t, err)
	fd := md.Fields().ByName("uint64_map")
	keyDesc := fd.MapKey()

	// Negative value.
	val := env.rt.ToValue(-1)
	_, err = env.m.gojaToProtoMapKey(val, keyDesc)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// conversion.go: mapKeyToGoja — all key types
// ---------------------------------------------------------------------------

func TestMapKeyToGoja_AllTypes(t *testing.T) {
	env := newTestEnvWithMapKeys(t)

	// Populate each map type via JS, then read back to exercise mapKeyToGoja.
	env.run(t, `var MKM = pb.messageType('mapkeys.MultiKeyMap')`)
	env.run(t, `var msg = new MKM()`)

	// bool key
	env.run(t, `msg.get('bool_map').set(true, 'yes')`)
	v := env.run(t, `msg.get('bool_map').get(true)`)
	assert.Equal(t, "yes", v.String())

	// int32 key
	env.run(t, `msg.get('int32_map').set(42, 'answer')`)
	v = env.run(t, `msg.get('int32_map').get(42)`)
	assert.Equal(t, "answer", v.String())

	// int64 key
	env.run(t, `msg.get('int64_map').set(100, 'hundred')`)
	v = env.run(t, `msg.get('int64_map').get(100)`)
	assert.Equal(t, "hundred", v.String())

	// uint32 key
	env.run(t, `msg.get('uint32_map').set(200, 'twohundred')`)
	v = env.run(t, `msg.get('uint32_map').get(200)`)
	assert.Equal(t, "twohundred", v.String())

	// uint64 key
	env.run(t, `msg.get('uint64_map').set(300, 'threehundred')`)
	v = env.run(t, `msg.get('uint64_map').get(300)`)
	assert.Equal(t, "threehundred", v.String())

	// Exercise forEach on each type → triggers mapKeyToGoja.
	env.run(t, `
		var keys = [];
		msg.get('bool_map').forEach(function(v, k) { keys.push(typeof k); });
		msg.get('int32_map').forEach(function(v, k) { keys.push(typeof k); });
		msg.get('int64_map').forEach(function(v, k) { keys.push(typeof k); });
		msg.get('uint32_map').forEach(function(v, k) { keys.push(typeof k); });
		msg.get('uint64_map').forEach(function(v, k) { keys.push(typeof k); });
	`)
}

func TestMapKeyToGoja_Entries(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	env.run(t, `var MKM = pb.messageType('mapkeys.MultiKeyMap')`)
	env.run(t, `var msg = new MKM()`)
	env.run(t, `msg.get('int32_map').set(1, 'one')`)
	env.run(t, `msg.get('int32_map').set(2, 'two')`)

	v := env.run(t, `
		var iter = msg.get('int32_map').entries();
		var result = {};
		var r = iter.next();
		while (!r.done) {
			result[r.value[0]] = r.value[1];
			r = iter.next();
		}
		result[1] === 'one' && result[2] === 'two'
	`)
	assert.True(t, v.ToBoolean())
}

// ---------------------------------------------------------------------------
// conversion.go: gojaToProtoMessage — error from jsObjectToMessage
// ---------------------------------------------------------------------------

func TestGojaToProtoMessage_ObjectConversionError(t *testing.T) {
	env := newTestEnv(t)
	// NestedInner.value is int32; passing a value > MaxInt32 via a plain
	// object triggers int32 overflow within jsObjectToMessage → error
	// propagates through gojaToProtoMessage → gojaToProtoValue.
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)
	env.mustFail(t, `msg.set('nested_val', {value: 9999999999})`)
}

// ---------------------------------------------------------------------------
// conversion.go: setMapFromGoja — plain object key/value errors
// ---------------------------------------------------------------------------

func TestSetMapFromGoja_PlainObjectValueError(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.MapMessage")
	require.NoError(t, err)
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("counts") // map<string, int32>

	// Plain object with int32 overflow value.
	objVal, err := env.rt.RunString(`({k: 9999999999})`)
	require.NoError(t, err)
	err = env.m.setMapFromGoja(msg, fd, objVal)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "map field")
}

// ---------------------------------------------------------------------------
// serialize.go: jsEncode — proto.Marshal error
// (This is very hard to trigger with dynamicpb; we test what we can.)
// ---------------------------------------------------------------------------

func TestJsEncode_EncodesEmptyCorrectly(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(t, `
		var SM = pb.messageType('test.SimpleMessage');
		var msg = new SM();
		var encoded = pb.encode(msg);
		encoded.length === 0
	`)
	assert.True(t, v.ToBoolean())
}

// ---------------------------------------------------------------------------
// serialize.go: jsToJSON — protojson.Marshal error
// (Hard to trigger without Any types; test what we can.)
// ---------------------------------------------------------------------------

func TestJsToJSON_EmptyMessage(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(t, `
		var SM = pb.messageType('test.SimpleMessage');
		var msg = new SM();
		var json = pb.toJSON(msg);
		typeof json === 'object' && json !== null
	`)
	assert.True(t, v.ToBoolean())
}

// ---------------------------------------------------------------------------
// serialize.go: jsFromJSON — protojson.Unmarshal error
// ---------------------------------------------------------------------------

func TestJsFromJSON_InvalidFieldValue(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var SM = pb.messageType('test.SimpleMessage')`)
	// Pass a value that cause protojson unmarshal to fail.
	// An int where an object is expected.
	env.mustFail(t, `pb.fromJSON(SM, 42)`)
}

func TestJsFromJSON_EmptyObject(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var SM = pb.messageType('test.SimpleMessage')`)
	v := env.run(t, `
		var msg = pb.fromJSON(SM, {});
		msg.get('name') === '' && msg.get('value') === 0
	`)
	assert.True(t, v.ToBoolean())
}

// ---------------------------------------------------------------------------
// serialize.go: jsDecode — various error paths
// ---------------------------------------------------------------------------

func TestJsDecode_NonConstructorFirstArg(t *testing.T) {
	env := newTestEnv(t)
	// Pass a number as the first arg (not a constructor).
	env.mustFail(t, `pb.decode(42, new Uint8Array([]))`)
}

// ---------------------------------------------------------------------------
// Comprehensive multi-key-map integration test (exercises mapKeyToGoja,
// gojaToProtoMapKey, setMapFromGoja for all key types)
// ---------------------------------------------------------------------------

func TestMultiKeyMap_RoundTrip(t *testing.T) {
	env := newTestEnvWithMapKeys(t)

	v := env.run(t, `
		var MKM = pb.messageType('mapkeys.MultiKeyMap');
		var msg = new MKM();

		// bool key
		msg.get('bool_map').set(true, 'yes');
		msg.get('bool_map').set(false, 'no');

		// int32 key
		msg.get('int32_map').set(-5, 'neg');
		msg.get('int32_map').set(0, 'zero');
		msg.get('int32_map').set(42, 'pos');

		// int64 key
		msg.get('int64_map').set(100, 'hundred');

		// uint32 key
		msg.get('uint32_map').set(200, 'twohundred');

		// uint64 key
		msg.get('uint64_map').set(300, 'threehundred');

		// Encode/decode roundtrip
		var encoded = pb.encode(msg);
		var decoded = pb.decode(MKM, encoded);

		decoded.get('bool_map').get(true) === 'yes' &&
		decoded.get('bool_map').get(false) === 'no' &&
		decoded.get('int32_map').get(-5) === 'neg' &&
		decoded.get('int32_map').get(0) === 'zero' &&
		decoded.get('int32_map').get(42) === 'pos' &&
		decoded.get('int64_map').get(100) === 'hundred' &&
		decoded.get('uint32_map').get(200) === 'twohundred' &&
		decoded.get('uint64_map').get(300) === 'threehundred'
	`)
	assert.True(t, v.ToBoolean())
}

func TestMultiKeyMap_Delete(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	env.run(t, `var MKM = pb.messageType('mapkeys.MultiKeyMap')`)
	env.run(t, `var msg = new MKM()`)
	env.run(t, `msg.get('bool_map').set(true, 'yes')`)
	env.run(t, `msg.get('bool_map').delete(true)`)
	v := env.run(t, `msg.get('bool_map').has(true)`)
	assert.False(t, v.ToBoolean())
}

func TestMultiKeyMap_Has(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	env.run(t, `var MKM = pb.messageType('mapkeys.MultiKeyMap')`)
	env.run(t, `var msg = new MKM()`)
	env.run(t, `msg.get('int32_map').set(42, 'answer')`)
	v := env.run(t, `msg.get('int32_map').has(42)`)
	assert.True(t, v.ToBoolean())
	v = env.run(t, `msg.get('int32_map').has(99)`)
	assert.False(t, v.ToBoolean())
}

// ---------------------------------------------------------------------------
// setMapFromGoja: JS Map with bool/int keys (non-string)
// ---------------------------------------------------------------------------

func TestSetMapFromGoja_JSMapBoolKeys(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	v := env.run(t, `
		var MKM = pb.messageType('mapkeys.MultiKeyMap');
		var msg = new MKM();
		var m = new Map();
		m.set(true, 'yes');
		m.set(false, 'no');
		msg.set('bool_map', m);
		msg.get('bool_map').get(true) === 'yes' && msg.get('bool_map').get(false) === 'no'
	`)
	assert.True(t, v.ToBoolean())
}

func TestSetMapFromGoja_JSMapInt32Keys(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	v := env.run(t, `
		var MKM = pb.messageType('mapkeys.MultiKeyMap');
		var msg = new MKM();
		var m = new Map();
		m.set(42, 'answer');
		m.set(-1, 'neg');
		msg.set('int32_map', m);
		msg.get('int32_map').get(42) === 'answer' && msg.get('int32_map').get(-1) === 'neg'
	`)
	assert.True(t, v.ToBoolean())
}

// ---------------------------------------------------------------------------
// Additional descriptor loading edge cases
// ---------------------------------------------------------------------------

func TestLoadDescriptorSet_DuplicateFileInSameFDS(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	fdp := testFileDescriptorProto()
	// FDS with the same file twice → second one should be skipped.
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{fdp, fdp},
	}
	data, err := proto.Marshal(fds)
	require.NoError(t, err)

	names, err := m.loadDescriptorSetBytes(data)
	require.NoError(t, err)
	// Should return names only from the first load.
	assert.NotEmpty(t, names)
}

func TestLoadDescriptorSet_UnmarshalError(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	// Truncated varint → proto.Unmarshal error.
	_, err = m.loadDescriptorSetBytes([]byte{0x0A, 0xFF})
	assert.Error(t, err)
}

func TestLoadFileDescriptorProtoBytes_UnmarshalError(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	_, err = m.loadFileDescriptorProtoBytes([]byte{0x0A, 0xFF})
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// gojaToProtoMessage: null/undefined returns zero Value
// ---------------------------------------------------------------------------

func TestGojaToProtoMessage_Nil(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.NestedOuter")
	require.NoError(t, err)
	fd := md.Fields().ByName("nested_inner")

	_, err = env.m.gojaToProtoMessage(nil, fd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "null value for message field")
}

func TestGojaToProtoMessage_Undefined(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.NestedOuter")
	require.NoError(t, err)
	fd := md.Fields().ByName("nested_inner")

	_, err = env.m.gojaToProtoMessage(goja.Undefined(), fd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "null value for message field")
}

// ---------------------------------------------------------------------------
// gojaToProtoValue: nil/undefined/null returns default
// ---------------------------------------------------------------------------

func TestGojaToProtoValue_NilReturnsDefault(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.SimpleMessage")
	require.NoError(t, err)
	fd := md.Fields().ByName("name")

	pv, err := env.m.gojaToProtoValue(nil, fd)
	require.NoError(t, err)
	assert.Equal(t, "", pv.String())
}

// ---------------------------------------------------------------------------
// Enable — with valid options
// ---------------------------------------------------------------------------

func TestRequire_WithOptions(t *testing.T) {
	rt := goja.New()
	registry := gojarequire.NewRegistry()
	customResolver := new(protoregistry.Types)
	registry.RegisterNativeModule("protobuf", Require(WithResolver(customResolver)))
	registry.Enable(rt)

	v, err := rt.RunString(`typeof require('protobuf').encode === 'function'`)
	require.NoError(t, err)
	assert.True(t, v.ToBoolean())
}

// ---------------------------------------------------------------------------
// Comprehensive bytes handling
// ---------------------------------------------------------------------------

func TestExtractBytes_ArrayBuffer(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	ab := rt.NewArrayBuffer([]byte{1, 2, 3})
	b, err := m.extractBytes(rt.ToValue(ab))
	require.NoError(t, err)
	assert.Equal(t, []byte{1, 2, 3}, b)
}

func TestExtractBytes_NullUndefined(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	_, err = m.extractBytes(nil)
	assert.Error(t, err)

	_, err = m.extractBytes(goja.Undefined())
	assert.Error(t, err)

	_, err = m.extractBytes(goja.Null())
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// extractMessageDesc: null/undefined
// ---------------------------------------------------------------------------

func TestExtractMessageDesc_NullUndefined(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	_, err = m.extractMessageDesc(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "null/undefined")

	_, err = m.extractMessageDesc(goja.Undefined())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "null/undefined")
}

// ---------------------------------------------------------------------------
// protoValueToGoja: bytes with nil
// ---------------------------------------------------------------------------

func TestProtoValueToGoja_NilBytes(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.AllTypes")
	require.NoError(t, err)
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("bytes_val")

	// Default bytes val is empty → should wrap to Uint8Array of length 0.
	val := env.m.protoValueToGoja(msg.Get(fd), fd)
	assert.NotNil(t, val)
}

// ---------------------------------------------------------------------------
// MapField: forEach callback with non-function (already tested elsewhere
// but needed for wrapMapField coverage completeness)
// ---------------------------------------------------------------------------

func TestMapField_ForEachNonFunctionOnMultiKeyMap(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	env.run(t, `var MKM = pb.messageType('mapkeys.MultiKeyMap')`)
	env.run(t, `var msg = new MKM()`)
	env.mustFail(t, `msg.get('bool_map').forEach(42)`)
}

// ---------------------------------------------------------------------------
// Additional serialization tests
// ---------------------------------------------------------------------------

func TestToJSON_FromJSON_MultiKeyMap(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	v := env.run(t, `
		var MKM = pb.messageType('mapkeys.MultiKeyMap');
		var msg = new MKM();
		msg.get('bool_map').set(true, 'yes');
		msg.get('int32_map').set(42, 'answer');

		var json = pb.toJSON(msg);
		var msg2 = pb.fromJSON(MKM, json);
		msg2.get('bool_map').get(true) === 'yes' && msg2.get('int32_map').get(42) === 'answer'
	`)
	assert.True(t, v.ToBoolean())
}

// ---------------------------------------------------------------------------
// helpers.go: combinedFileResolver — both-miss paths
// ---------------------------------------------------------------------------

func TestCombinedFileResolver_FindFileByPath_BothMiss(t *testing.T) {
	r := &combinedFileResolver{local: new(protoregistry.Files), global: new(protoregistry.Files)}
	_, err := r.FindFileByPath("nonexistent.proto")
	assert.Error(t, err)
}

func TestCombinedFileResolver_FindDescriptorByName_BothMiss(t *testing.T) {
	r := &combinedFileResolver{local: new(protoregistry.Files), global: new(protoregistry.Files)}
	_, err := r.FindDescriptorByName("nonexistent.Msg")
	assert.Error(t, err)
}

func TestModule_FindDescriptor(t *testing.T) {
	env := newTestEnv(t)

	// Finds a message descriptor by full name.
	desc, err := env.m.FindDescriptor("test.SimpleMessage")
	require.NoError(t, err)
	assert.Equal(t, protoreflect.FullName("test.SimpleMessage"), desc.FullName())

	// Not found returns error.
	_, err = env.m.FindDescriptor("nonexistent.Type")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// conversion.go: setMapFromGoja — entries property not a function
// ---------------------------------------------------------------------------

func TestSetMapFromGoja_EntriesNotFunction(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.MapMessage")
	require.NoError(t, err)
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("tags") // map<string, string>

	// Create an object with 'entries' property that is NOT a function.
	// Should fall through to plain object iteration.
	objVal, err := env.rt.RunString(`({entries: 'not_a_function', k1: 'v1', k2: 'v2'})`)
	require.NoError(t, err)
	err = env.m.setMapFromGoja(msg, fd, objVal)
	require.NoError(t, err)
	// entries is an own property key, so it will be iterated as a map entry.
	// k1 and k2 are the actual entries.
	protoMap := msg.Get(fd).Map()
	assert.True(t, protoMap.Len() >= 2)
}

func TestSetMapFromGoja_EntriesCallError(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.MapMessage")
	require.NoError(t, err)
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("tags")

	// Create an object with 'entries' function that throws.
	// Should fall through to plain object iteration.
	objVal, err := env.rt.RunString(`
		var o = {k1: 'v1'};
		o.entries = function() { throw new Error("boom"); };
		o
	`)
	require.NoError(t, err)
	err = env.m.setMapFromGoja(msg, fd, objVal)
	require.NoError(t, err)
	protoMap := msg.Get(fd).Map()
	assert.True(t, protoMap.Len() >= 1)
}

func TestSetMapFromGoja_EntriesNoNext(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.MapMessage")
	require.NoError(t, err)
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("tags")

	// Create an object whose entries() returns an object without next().
	// Should fall through to plain object iteration.
	objVal, err := env.rt.RunString(`
		var o = {k1: 'v1'};
		o.entries = function() { return {}; };
		o
	`)
	require.NoError(t, err)
	err = env.m.setMapFromGoja(msg, fd, objVal)
	require.NoError(t, err)
	protoMap := msg.Get(fd).Map()
	assert.True(t, protoMap.Len() >= 1)
}

// ---------------------------------------------------------------------------
// conversion.go: setRepeatedFromGoja — obj with entries (not an array)
// triggers the object-without-length path through wrapMessage.set()
// ---------------------------------------------------------------------------

func TestSetRepeatedFromGoja_ObjectNoLengthViaJS(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.RepeatedMessage'))()`)
	// Passing an object without length to a repeated field via set() should throw.
	env.mustFail(t, `msg.set('items', {foo: 'bar'})`)
}

// ---------------------------------------------------------------------------
// descriptors.go: jsLoadDescriptorSet — empty FDS via JS (loop body uncovered)
// ---------------------------------------------------------------------------

func TestJsLoadDescriptorSet_EmptyFDS(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	pb := rt.NewObject()
	m.setupExports(pb)
	require.NoError(t, rt.Set("pb", pb))

	emptyFDS := &descriptorpb.FileDescriptorSet{}
	data, err := proto.Marshal(emptyFDS)
	require.NoError(t, err)
	require.NoError(t, rt.Set("emptyBytes", rt.NewArrayBuffer(data)))

	v, err := rt.RunString(`
		var names = pb.loadDescriptorSet(new Uint8Array(emptyBytes));
		names.length === 0
	`)
	require.NoError(t, err)
	assert.True(t, v.ToBoolean())
}

// ---------------------------------------------------------------------------
// message.go: wrapRepeatedField — forEach callback throws
// ---------------------------------------------------------------------------

func TestRepeatedField_ForEachCallbackThrows(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.RepeatedMessage'))();
		msg.get('items').add('a');
	`)
	env.mustFail(t, `
		msg.get('items').forEach(function() { throw new Error('callback error'); })
	`)
}

// ---------------------------------------------------------------------------
// message.go: wrapMapField — forEach callback throws
// ---------------------------------------------------------------------------

func TestMapField_ForEachCallbackThrows(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.MapMessage'))();
		msg.get('tags').set('k', 'v');
	`)
	env.mustFail(t, `
		msg.get('tags').forEach(function() { throw new Error('callback error'); })
	`)
}

// ---------------------------------------------------------------------------
// conversion.go: setMapFromGoja — JS Map iterator next() error
// ---------------------------------------------------------------------------

func TestSetMapFromGoja_IteratorNextError(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.MapMessage")
	require.NoError(t, err)
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("tags")

	// Create an object with entries() that returns iterator whose next() throws.
	objVal, err := env.rt.RunString(`
		var o = {};
		o.entries = function() {
			return {
				next: function() { throw new Error('iter error'); }
			};
		};
		o
	`)
	require.NoError(t, err)
	// The next() error must be propagated — silent swallowing is a bug.
	err = env.m.setMapFromGoja(msg, fd, objVal)
	require.Error(t, err)
	require.Contains(t, err.Error(), "map field tags iterator:")
	require.Contains(t, err.Error(), "iter error")
}

// ---------------------------------------------------------------------------
// descriptors.go: loadFileDescriptorProtoBytes — RegisterFile error
// ---------------------------------------------------------------------------

func TestLoadFileDescriptorProtoBytes_RegisterFileError(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	fdp := testFileDescriptorProto()
	data, err := proto.Marshal(fdp)
	require.NoError(t, err)

	// First load succeeds.
	_, err = m.loadFileDescriptorProtoBytes(data)
	require.NoError(t, err)

	// Create a slightly different file with the SAME path but different
	// content. The localFiles.FindFileByPath check should find it (already
	// registered), so it returns nil, nil (the already-registered path
	// tested earlier). This confirms we can't easily trigger RegisterFile
	// error in a single-threaded test.
	names, err := m.loadFileDescriptorProtoBytes(data)
	require.NoError(t, err)
	assert.Nil(t, names)
}

// ---------------------------------------------------------------------------
// Additional tests for coverage completeness
// ---------------------------------------------------------------------------

func TestJsLoadFileDescriptorProto_AlreadyRegisteredViaJS(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	pb := rt.NewObject()
	m.setupExports(pb)
	require.NoError(t, rt.Set("pb", pb))

	fdp := testFileDescriptorProto()
	data, err := proto.Marshal(fdp)
	require.NoError(t, err)
	require.NoError(t, rt.Set("protoBytes", rt.NewArrayBuffer(data)))

	// First load.
	v, err := rt.RunString(`
		var names1 = pb.loadFileDescriptorProto(new Uint8Array(protoBytes));
		names1.length
	`)
	require.NoError(t, err)
	assert.True(t, v.ToInteger() > 0)

	// Second load — already registered, returns empty array.
	v, err = rt.RunString(`
		var names2 = pb.loadFileDescriptorProto(new Uint8Array(protoBytes));
		names2.length
	`)
	require.NoError(t, err)
	assert.Equal(t, int64(0), v.ToInteger())
}

// Test that the map entries iterator properly terminates (done=true path).
func TestMapField_EntriesIteratorDone(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.MapMessage'))();
		msg.get('tags').set('only', 'one');
	`)
	v := env.run(t, `
		var iter = msg.get('tags').entries();
		var r1 = iter.next();
		var r2 = iter.next();
		!r1.done && r2.done && r1.value[0] === 'only' && r1.value[1] === 'one'
	`)
	assert.True(t, v.ToBoolean())
}

// Test encoding/decoding with bytes field.
func TestEncodeDecodeBytes(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(t, `
		var AT = pb.messageType('test.AllTypes');
		var msg = new AT();
		msg.set('bytes_val', new Uint8Array([0xDE, 0xAD, 0xBE, 0xEF]));
		var encoded = pb.encode(msg);
		var decoded = pb.decode(AT, encoded);
		var b = decoded.get('bytes_val');
		b[0] === 0xDE && b[1] === 0xAD && b[2] === 0xBE && b[3] === 0xEF
	`)
	assert.True(t, v.ToBoolean())
}

// ---------------------------------------------------------------------------
// serialize.go: jsToJSON — JSON.parse overridden to non-function
// ---------------------------------------------------------------------------

func TestJsToJSON_JSONParseOverridden(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var SM = pb.messageType('test.SimpleMessage');
		var msg = new SM();
		msg.set('name', 'test');
	`)
	// Override JSON.parse with a non-function.
	env.run(t, `JSON.parse = 42`)
	env.mustFail(t, `pb.toJSON(msg)`)
}

// ---------------------------------------------------------------------------
// serialize.go: jsFromJSON — JSON.stringify overridden to non-function
// ---------------------------------------------------------------------------

func TestJsFromJSON_JSONStringifyOverridden(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var SM = pb.messageType('test.SimpleMessage')`)
	// Override JSON.stringify with a non-function.
	env.run(t, `JSON.stringify = 42`)
	env.mustFail(t, `pb.fromJSON(SM, {name: 'test'})`)
}

// ---------------------------------------------------------------------------
// serialize.go: jsFromJSON — JSON.stringify call throws (circular ref)
// ---------------------------------------------------------------------------

func TestJsFromJSON_StringifyCallErrors(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var SM = pb.messageType('test.SimpleMessage')`)
	// A circular reference causes JSON.stringify to throw a TypeError.
	env.mustFail(t, `
		var c = {};
		c.self = c;
		pb.fromJSON(SM, c)
	`)
}

// ---------------------------------------------------------------------------
// serialize.go: jsToJSON — JSON.parse call throws
// We override JSON.parse with a function that always throws.
// ---------------------------------------------------------------------------

func TestJsToJSON_JSONParseCallErrors(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var SM = pb.messageType('test.SimpleMessage');
		var msg = new SM();
		msg.set('name', 'test');
	`)
	// Override JSON.parse with a function that throws.
	env.run(t, `JSON.parse = function() { throw new Error("parse error"); }`)
	env.mustFail(t, `pb.toJSON(msg)`)
}

// ---------------------------------------------------------------------------
// descriptors.go: jsLoadDescriptorSet — invalid-but-extractable bytes
// This tests the proto.Unmarshal error path inside loadDescriptorSetBytes
// when called via JS.
// ---------------------------------------------------------------------------

func TestJsLoadDescriptorSet_InvalidProtobufBytes(t *testing.T) {
	env := newTestEnv(t)
	// Create bytes that are extractable but fail proto.Unmarshal.
	// A truncated length-delimited field: tag 0x0A (field 1, wire type 2),
	// length 200, but no data.
	env.rt.Set("badData", env.rt.NewArrayBuffer([]byte{0x0A, 0xC8, 0x01}))
	env.mustFail(t, `pb.loadDescriptorSet(new Uint8Array(badData))`)
}

// ---------------------------------------------------------------------------
// descriptors.go: jsLoadFileDescriptorProto — proto.Unmarshal error via JS
// ---------------------------------------------------------------------------

func TestJsLoadFileDescriptorProto_InvalidProtobufBytes(t *testing.T) {
	env := newTestEnv(t)
	// Same truncated bytes approach.
	env.rt.Set("badData", env.rt.NewArrayBuffer([]byte{0x0A, 0xC8, 0x01}))
	env.mustFail(t, `pb.loadFileDescriptorProto(new Uint8Array(badData))`)
}

// Test gojaToProtoValue with null/undefined for various field kinds.
func TestGojaToProtoValue_NullForAllScalarTypes(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	// Setting each field to null should clear to its default.
	env.run(t, `msg.set('int32_val', 42); msg.set('int32_val', null)`)
	v := env.run(t, `msg.get('int32_val')`)
	assert.Equal(t, int64(0), v.ToInteger())

	env.run(t, `msg.set('uint64_val', 100); msg.set('uint64_val', null)`)
	v = env.run(t, `msg.get('uint64_val')`)
	assert.Equal(t, int64(0), v.ToInteger())

	env.run(t, `msg.set('float_val', 1.5); msg.set('float_val', null)`)
	v = env.run(t, `msg.get('float_val')`)
	assert.InDelta(t, 0.0, v.ToFloat(), 0.001)
}

// Test map field set via plain object key error path.
func TestSetMapFromGoja_PlainObjectKeyError(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	md, err := env.m.findMessageDescriptor("mapkeys.MultiKeyMap")
	require.NoError(t, err)
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("uint32_map")

	// Plain object with string key "-1" → when parsed, gojaToUint64 returns
	// error for negative value.
	objVal, err := env.rt.RunString(`({"-1": "v"})`)
	require.NoError(t, err)
	err = env.m.setMapFromGoja(msg, fd, objVal)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "map field")
}

// ---------------------------------------------------------------------------
// Additional setMapFromGoja: JS Map key error with maps iterated via
// the entries() protocol (exercises the error return inside the iterator)
// ---------------------------------------------------------------------------

func TestSetMapFromGoja_JSMapKeyErrorViaEntries(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	md, err := env.m.findMessageDescriptor("mapkeys.MultiKeyMap")
	require.NoError(t, err)
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("uint32_map")

	// A JS Map with a negative key → gojaToUint64 returns error for neg value.
	mapVal, err := env.rt.RunString(`
		var m = new Map();
		m.set(-1, 'neg');
		m
	`)
	require.NoError(t, err)
	err = env.m.setMapFromGoja(msg, fd, mapVal)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "map field")
}

// ---------------------------------------------------------------------------
// Additional setMapFromGoja: JS Map value error through entries()
// ---------------------------------------------------------------------------

func TestSetMapFromGoja_JSMapValueErrorViaEntries(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	// Use a message with map<string, int32> values (like MapMessage.counts)
	md, err := env.m.findMessageDescriptor("test.MapMessage")
	require.NoError(t, err)
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("counts") // map<string, int32>

	// A JS Map with a value that causes int32 overflow.
	mapVal, err := env.rt.RunString(`
		var m = new Map();
		m.set('k', 9999999999);
		m
	`)
	require.NoError(t, err)
	err = env.m.setMapFromGoja(msg, fd, mapVal)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "map field")
}

// ---------------------------------------------------------------------------
// Descriptor loading: file with dependencies resolved from local
// (exercises combinedFileResolver.FindFileByPath local-hit through protodesc)
// ---------------------------------------------------------------------------

func TestLoadDescriptorSet_WithDependencyResolution(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	// First: load the base test descriptors into localFiles.
	_, err = m.loadDescriptorSetBytes(testDescriptorSetBytes())
	require.NoError(t, err)

	// Now: load a file that depends on test.proto (already in localFiles).
	// This exercises FindFileByPath for the combinedFileResolver local hit
	// path within protodesc.NewFile.
	depFdp := containerMessageProto()
	data, err := proto.Marshal(depFdp)
	require.NoError(t, err)
	names, err := m.loadFileDescriptorProtoBytes(data)
	require.NoError(t, err)
	assert.Contains(t, names, "container.Container")
}

// ---------------------------------------------------------------------------
// BUG-1 fix: repeated field add/set with null/undefined throws TypeError
// ---------------------------------------------------------------------------

func TestRepeatedField_AddNull_Throws(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var RM = pb.messageType('test.RepeatedMessage');
		var msg = new RM();
	`)
	env.mustFail(t, `msg.get('items').add(null)`)
	env.mustFail(t, `msg.get('items').add(undefined)`)
}

func TestRepeatedField_SetNull_Throws(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var RM = pb.messageType('test.RepeatedMessage');
		var msg = new RM();
		msg.set('items', ['a']);
	`)
	env.mustFail(t, `msg.get('items').set(0, null)`)
	env.mustFail(t, `msg.get('items').set(0, undefined)`)
}

// ---------------------------------------------------------------------------
// BUG-2 fix: map key int32/uint32 overflow check
// ---------------------------------------------------------------------------

func TestMapKeyInt32Overflow(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	md, err := env.m.findMessageDescriptor("mapkeys.MultiKeyMap")
	require.NoError(t, err)
	fd := md.Fields().ByName("int32_map")

	// Int32 overflow.
	bigVal := env.rt.ToValue(int64(math.MaxInt32) + 1)
	_, err = env.m.gojaToProtoMapKey(bigVal, fd.MapKey())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overflows int32")

	// Int32 underflow.
	smallVal := env.rt.ToValue(int64(math.MinInt32) - 1)
	_, err = env.m.gojaToProtoMapKey(smallVal, fd.MapKey())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overflows int32")
}

func TestMapKeyUint32Overflow(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	md, err := env.m.findMessageDescriptor("mapkeys.MultiKeyMap")
	require.NoError(t, err)
	msg := dynamicpb.NewMessage(md)
	_ = msg // suppress unused
	fd := md.Fields().ByName("uint32_map")

	// Uint32 overflow.
	bigVal := env.rt.ToValue(int64(math.MaxUint32) + 1)
	_, err = env.m.gojaToProtoMapKey(bigVal, fd.MapKey())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overflows uint32")
}

// ---------------------------------------------------------------------------
// CONCERN-2 fix: map set(key, null) deletes the entry
// ---------------------------------------------------------------------------

func TestMapField_SetNullDeletesEntry(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(t, `
		var MM = pb.messageType('test.MapMessage');
		var msg = new MM();
		msg.get('tags').set('keep', 'yes');
		msg.get('tags').set('del', 'no');
		msg.get('tags').set('del', null);
		msg.get('tags').size
	`)
	assert.Equal(t, int64(1), v.ToInteger())

	v = env.run(t, `msg.get('tags').get('del')`)
	assert.True(t, goja.IsUndefined(v))

	v = env.run(t, `msg.get('tags').get('keep')`)
	assert.Equal(t, "yes", v.String())
}

// ---------------------------------------------------------------------------
// Enum int32 overflow check
// ---------------------------------------------------------------------------

func TestGojaToProtoEnum_Int32Overflow(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.AllTypes")
	require.NoError(t, err)
	fd := md.Fields().ByName("enum_val")

	// Overflow positive.
	bigVal := env.rt.ToValue(int64(math.MaxInt32) + 1)
	_, err = env.m.gojaToProtoEnum(bigVal, fd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overflows int32")

	// Overflow negative.
	smallVal := env.rt.ToValue(int64(math.MinInt32) - 1)
	_, err = env.m.gojaToProtoEnum(smallVal, fd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overflows int32")
}

// ---------------------------------------------------------------------------
// Bool map key from plain object: "false" must not be truthy
// ---------------------------------------------------------------------------

func TestBoolMapKey_PlainObject_FalseString(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	v := env.run(t, `
		var MK = pb.messageType('mapkeys.MultiKeyMap');
		var msg = new MK();
		msg.set('bool_map', {"true": "yes", "false": "no"});
		var m = msg.get('bool_map');
		m.size
	`)
	assert.Equal(t, int64(2), v.ToInteger())

	v = env.run(t, `m.get(true)`)
	assert.Equal(t, "yes", v.String())

	v = env.run(t, `m.get(false)`)
	assert.Equal(t, "no", v.String())
}

func TestBoolMapKey_JSMap(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	v := env.run(t, `
		var MK = pb.messageType('mapkeys.MultiKeyMap');
		var msg = new MK();
		var bm = new Map();
		bm.set(true, "yes");
		bm.set(false, "no");
		msg.set('bool_map', bm);
		msg.get('bool_map').size
	`)
	assert.Equal(t, int64(2), v.ToInteger())

	v = env.run(t, `msg.get('bool_map').get(true)`)
	assert.Equal(t, "yes", v.String())

	v = env.run(t, `msg.get('bool_map').get(false)`)
	assert.Equal(t, "no", v.String())
}

// ---------------------------------------------------------------------------
// Message type mismatch: setting field with wrong message type
// ---------------------------------------------------------------------------

func TestGojaToProtoMessage_TypeMismatch(t *testing.T) {
	env := newTestEnv(t)
	// Create a NestedOuter with field nested_inner (type NestedInner).
	// Try to set nested_inner with a SimpleMessage — wrong type.
	env.run(t, `
		var Outer = pb.messageType('test.NestedOuter');
		var Simple = pb.messageType('test.SimpleMessage');
		var outer = new Outer();
		var simple = new Simple();
		simple.set('name', 'oops');
	`)
	// Should throw TypeError, not Go panic.
	env.mustFail(t, `outer.set('nested_inner', simple)`)
}

func TestGojaToProtoMessage_TypeMatch(t *testing.T) {
	env := newTestEnv(t)
	// Create matching types — should succeed.
	env.run(t, `
		var Outer = pb.messageType('test.NestedOuter');
		var Inner = pb.messageType('test.NestedInner');
		var outer = new Outer();
		var inner = new Inner();
		inner.set('value', 42);
		outer.set('nested_inner', inner);
	`)
	v := env.run(t, `outer.get('nested_inner').get('value')`)
	assert.Equal(t, int64(42), v.ToInteger())
}

// ---------------------------------------------------------------------------
// Batch 5: Targeted coverage for remaining gaps identified via
// `grep ' 0$' coverage.out`.
// ---------------------------------------------------------------------------

// conversion.go:109 — gojaToProtoValue null/undefined for MessageKind field.
// The wrapMessage set() handler catches null before calling gojaToProtoValue,
// so we must call the function directly to exercise this path.
func TestGojaToProtoValue_NullForMessageField(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.NestedOuter")
	require.NoError(t, err)
	fd := md.Fields().ByName("nested_inner") // MessageKind

	// nil → error
	_, err = env.m.gojaToProtoValue(nil, fd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "null value for message field")

	// goja.Null() → error
	_, err = env.m.gojaToProtoValue(goja.Null(), fd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "null value for message field")

	// goja.Undefined() → error
	_, err = env.m.gojaToProtoValue(goja.Undefined(), fd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "null value for message field")
}

// conversion.go:121 — gojaToProtoValue Int32Kind when gojaToInt64 returns
// error (BigInt that overflows int64).
func TestGojaToProtoValue_Int32BigIntOverflow(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.AllTypes")
	require.NoError(t, err)
	fd := md.Fields().ByName("int32_val") // Int32Kind

	// BigInt that overflows int64 → gojaToInt64 error path.
	bigVal, err := env.rt.RunString(`BigInt('9223372036854775808')`)
	require.NoError(t, err)
	_, err = env.m.gojaToProtoValue(bigVal, fd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overflows int64")
}

// conversion.go:239 — gojaToProtoEnum when gojaToInt64 returns error
// (BigInt that overflows int64, before the int32 overflow check).
func TestGojaToProtoEnum_BigIntOverflow(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.AllTypes")
	require.NoError(t, err)
	fd := md.Fields().ByName("enum_val") // EnumKind

	// BigInt that overflows int64 → gojaToInt64 error path inside gojaToProtoEnum.
	bigVal, err := env.rt.RunString(`BigInt('9223372036854775808')`)
	require.NoError(t, err)
	_, err = env.m.gojaToProtoEnum(bigVal, fd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "overflows int64")
}

// conversion.go:423 — setMapFromGoja entries iterator null value skip.
// A JS Map entry where value is null → continue (skip the entry).
func TestSetMapFromGoja_JSMapNullValueSkip(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.MapMessage")
	require.NoError(t, err)
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("tags") // map<string, string>

	// JS Map with one valid and one null entry.
	mapVal, err := env.rt.RunString(`
		var m = new Map();
		m.set('keep', 'yes');
		m.set('skip', null);
		m
	`)
	require.NoError(t, err)
	err = env.m.setMapFromGoja(msg, fd, mapVal)
	require.NoError(t, err)

	protoMap := msg.Get(fd).Map()
	assert.Equal(t, 1, protoMap.Len())
	assert.Equal(t, "yes", protoMap.Get(protoreflect.ValueOfString("keep").MapKey()).String())
}

// conversion.go:447 — setMapFromGoja plain object null value skip.
// A plain object where one property value is null → skip.
func TestSetMapFromGoja_PlainObjNullValueSkip(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.MapMessage")
	require.NoError(t, err)
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("tags")

	// Plain object with one valid and one null value.
	objVal, err := env.rt.RunString(`({keep: 'yes', skip: null})`)
	require.NoError(t, err)
	err = env.m.setMapFromGoja(msg, fd, objVal)
	require.NoError(t, err)

	protoMap := msg.Get(fd).Map()
	assert.Equal(t, 1, protoMap.Len())
	assert.Equal(t, "yes", protoMap.Get(protoreflect.ValueOfString("keep").MapKey()).String())
}

// descriptors.go:19 — jsLoadDescriptorSet extractBytes error.
// Passing a number (not bytes) triggers extractBytes error.
func TestJsLoadDescriptorSet_ExtractBytesError(t *testing.T) {
	env := newTestEnv(t)
	env.mustFail(t, `pb.loadDescriptorSet(42)`)
}

// descriptors.go:41 — jsLoadFileDescriptorProto extractBytes error.
// Passing a number (not bytes) triggers extractBytes error.
func TestJsLoadFileDescriptorProto_ExtractBytesError(t *testing.T) {
	env := newTestEnv(t)
	env.mustFail(t, `pb.loadFileDescriptorProto(42)`)
}

// ---------------------------------------------------------------------------
// Batch 6: Mock FieldDescriptor to test default branches in type switches.
// These branches handle invalid/unknown protoreflect.Kind values which
// cannot be produced by the normal protobuf library. They are defensive
// safety nets.
// ---------------------------------------------------------------------------

// mockFieldDesc is a minimal mock for [protoreflect.FieldDescriptor] that
// returns a custom [protoreflect.Kind]. All other methods delegate to the
// embedded interface and will panic if called — only Kind() is overridden.
type mockFieldDesc struct {
	protoreflect.FieldDescriptor // nil: panics on any non-overridden call
	kind                         protoreflect.Kind
}

func (f *mockFieldDesc) Kind() protoreflect.Kind { return f.kind }

// conversion.go:60 — protoValueToGoja default case (unknown Kind).
func TestProtoValueToGoja_DefaultBranch(t *testing.T) {
	env := newTestEnv(t)
	fd := &mockFieldDesc{kind: protoreflect.Kind(99)}
	result := env.m.protoValueToGoja(protoreflect.ValueOfString("x"), fd)
	assert.True(t, goja.IsUndefined(result))
}

// conversion.go:175 — gojaToProtoValue default case (unknown Kind).
func TestGojaToProtoValue_DefaultBranch(t *testing.T) {
	env := newTestEnv(t)
	fd := &mockFieldDesc{kind: protoreflect.Kind(99)}
	_, err := env.m.gojaToProtoValue(env.rt.ToValue("test"), fd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported field kind")
}

// conversion.go:356 — gojaToProtoMapKey default case (unknown Kind).
func TestGojaToProtoMapKey_DefaultBranch(t *testing.T) {
	env := newTestEnv(t)
	fd := &mockFieldDesc{kind: protoreflect.Kind(99)}
	_, err := env.m.gojaToProtoMapKey(env.rt.ToValue("test"), fd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported map key kind")
}

// conversion.go:479 — mapKeyToGoja default case (unknown Kind).
func TestMapKeyToGoja_DefaultBranch(t *testing.T) {
	env := newTestEnv(t)
	fd := &mockFieldDesc{kind: protoreflect.Kind(99)}
	mk := protoreflect.ValueOfString("test").MapKey()
	result := env.m.mapKeyToGoja(mk, fd)
	// Default branch returns mk.String().
	assert.Equal(t, "test", result.String())
}

// ---------------------------------------------------------------------------
// Batch 7: Trigger protojson.Marshal error via well-known type validation.
//
// protojson has special validation for google.protobuf.Timestamp: if
// seconds is outside [-62135596800, 253402300800], marshal fails. We
// create a dynamic Timestamp descriptor with the correct full name,
// set seconds to an out-of-range value, and call toJSON.
// ---------------------------------------------------------------------------

// serialize.go:62 — jsToJSON protojson.Marshal error.
func TestJsToJSON_ProtojsonMarshalError(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	// Build a fake google.protobuf.Timestamp descriptor so protojson
	// recognises it and applies range validation.
	fdp := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("fake_timestamp.proto"),
		Package: proto.String("google.protobuf"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Timestamp"),
			Field: []*descriptorpb.FieldDescriptorProto{
				{
					Name:   proto.String("seconds"),
					Number: proto.Int32(1),
					Type:   descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
					Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				},
				{
					Name:   proto.String("nanos"),
					Number: proto.Int32(2),
					Type:   descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
					Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				},
			},
		}},
	}
	data, err := proto.Marshal(fdp)
	require.NoError(t, err)
	names, err := m.loadFileDescriptorProtoBytes(data)
	require.NoError(t, err)
	require.Contains(t, names, "google.protobuf.Timestamp")

	// Create a Timestamp with out-of-range seconds → protojson.Marshal error.
	md, err := m.findMessageDescriptor("google.protobuf.Timestamp")
	require.NoError(t, err)
	msg := dynamicpb.NewMessage(md)
	msg.Set(md.Fields().ByName("seconds"), protoreflect.ValueOfInt64(999999999999))

	// Wrap and attempt toJSON.
	wrapped := m.wrapMessage(msg)
	pb := rt.NewObject()
	m.setupExports(pb)
	require.NoError(t, rt.Set("pb", pb))
	require.NoError(t, rt.Set("msg", wrapped))

	_, err = rt.RunString("pb.toJSON(msg)")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "seconds") // error mentions seconds field
}
