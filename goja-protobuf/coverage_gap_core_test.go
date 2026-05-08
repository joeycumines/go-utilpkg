package gojaprotobuf

import (
	"strings"
	"testing"

	"github.com/joeycumines/goja"
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
		Name:    new("exttest.proto"),
		Package: new("exttest"),
		Syntax:  new("proto2"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("ExtMsg"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:   new("id"),
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
			Name:     new("my_ext_field"),
			Number:   proto.Int32(100),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			Extendee: new(".exttest.ExtMsg"),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
		}},
	}
	fd, err := protodesc.NewFile(fdp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	extDesc := fd.Extensions().Get(0)
	xt := dynamicpb.NewExtensionType(extDesc)
	return xt, extDesc.FullName(), "exttest.ExtMsg", 100
}

// multiKeyMapFileDescriptorProto returns a proto for a message with map
// fields keyed by bool, int32, int64, uint32, uint64.
func multiKeyMapFileDescriptorProto() *descriptorpb.FileDescriptorProto {
	mkEntry := func(name string, keyType descriptorpb.FieldDescriptorProto_Type) *descriptorpb.DescriptorProto {
		return &descriptorpb.DescriptorProto{
			Name: new(name),
			Field: []*descriptorpb.FieldDescriptorProto{
				{Name: new("key"), Number: proto.Int32(1), Type: keyType.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("key")},
				{Name: new("value"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("value")},
			},
			Options: &descriptorpb.MessageOptions{MapEntry: new(true)},
		}
	}
	mkField := func(name string, num int32, entry string) *descriptorpb.FieldDescriptorProto {
		return &descriptorpb.FieldDescriptorProto{
			Name: new(name), Number: new(num),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
			TypeName: new(".mapkeys.MultiKeyMap." + entry),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
			JsonName: new(name),
		}
	}
	return &descriptorpb.FileDescriptorProto{
		Name:    new("mapkeys.proto"),
		Package: new("mapkeys"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("MultiKeyMap"),
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
		Name:       new("container.proto"),
		Package:    new("container"),
		Syntax:     new("proto3"),
		Dependency: []string{"test.proto"},
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Container"),
			Field: []*descriptorpb.FieldDescriptorProto{
				{
					Name: new("inner"), Number: proto.Int32(1),
					Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
					TypeName: new(".test.NestedInner"),
					Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					JsonName: new("inner"),
				},
				{
					Name: new("inners"), Number: proto.Int32(2),
					Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
					TypeName: new(".test.NestedInner"),
					Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
					JsonName: new("inners"),
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := env.m.loadFileDescriptorProtoBytes(data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return env
}

// ---------------------------------------------------------------------------
// helpers.go: combinedTypeResolver
// ---------------------------------------------------------------------------

func TestCombinedTypeResolver_FindMessageByName(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.SimpleMessage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mt := dynamicpb.NewMessageType(md)

	t.Run("local_hit", func(t *testing.T) {
		local := new(protoregistry.Types)
		global := new(protoregistry.Types)
		if err := local.RegisterMessage(mt); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r := &combinedTypeResolver{local: local, global: global}
		result, err := r.FindMessageByName("test.SimpleMessage")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := string(result.Descriptor().FullName()); got != "test.SimpleMessage" {
			t.Errorf("got %q, want %q", got, "test.SimpleMessage")
		}
	})

	t.Run("global_fallback", func(t *testing.T) {
		local := new(protoregistry.Types)
		global := new(protoregistry.Types)
		if err := global.RegisterMessage(mt); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r := &combinedTypeResolver{local: local, global: global}
		result, err := r.FindMessageByName("test.SimpleMessage")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := string(result.Descriptor().FullName()); got != "test.SimpleMessage" {
			t.Errorf("got %q, want %q", got, "test.SimpleMessage")
		}
	})

	t.Run("both_miss", func(t *testing.T) {
		r := &combinedTypeResolver{local: new(protoregistry.Types), global: new(protoregistry.Types)}
		_, err := r.FindMessageByName("nonexistent.Foo")
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestCombinedTypeResolver_FindMessageByURL(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.SimpleMessage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mt := dynamicpb.NewMessageType(md)

	t.Run("local_hit", func(t *testing.T) {
		local := new(protoregistry.Types)
		global := new(protoregistry.Types)
		if err := local.RegisterMessage(mt); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r := &combinedTypeResolver{local: local, global: global}
		result, err := r.FindMessageByURL("test.SimpleMessage")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := string(result.Descriptor().FullName()); got != "test.SimpleMessage" {
			t.Errorf("got %q, want %q", got, "test.SimpleMessage")
		}
	})

	t.Run("global_fallback", func(t *testing.T) {
		local := new(protoregistry.Types)
		global := new(protoregistry.Types)
		if err := global.RegisterMessage(mt); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r := &combinedTypeResolver{local: local, global: global}
		result, err := r.FindMessageByURL("test.SimpleMessage")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := string(result.Descriptor().FullName()); got != "test.SimpleMessage" {
			t.Errorf("got %q, want %q", got, "test.SimpleMessage")
		}
	})

	t.Run("both_miss", func(t *testing.T) {
		r := &combinedTypeResolver{local: new(protoregistry.Types), global: new(protoregistry.Types)}
		_, err := r.FindMessageByURL("nonexistent.Foo")
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestCombinedTypeResolver_FindExtensionByName(t *testing.T) {
	xt, extName, _, _ := buildExtensionType(t)

	t.Run("local_hit", func(t *testing.T) {
		local := new(protoregistry.Types)
		global := new(protoregistry.Types)
		if err := local.RegisterExtension(xt); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r := &combinedTypeResolver{local: local, global: global}
		result, err := r.FindExtensionByName(extName)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("global_fallback", func(t *testing.T) {
		local := new(protoregistry.Types)
		global := new(protoregistry.Types)
		if err := global.RegisterExtension(xt); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r := &combinedTypeResolver{local: local, global: global}
		result, err := r.FindExtensionByName(extName)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("both_miss", func(t *testing.T) {
		r := &combinedTypeResolver{local: new(protoregistry.Types), global: new(protoregistry.Types)}
		_, err := r.FindExtensionByName("nonexistent.ext")
		if err == nil {
			t.Error("expected error")
		}
	})
}

func TestCombinedTypeResolver_FindExtensionByNumber(t *testing.T) {
	xt, _, msgName, fieldNum := buildExtensionType(t)

	t.Run("local_hit", func(t *testing.T) {
		local := new(protoregistry.Types)
		global := new(protoregistry.Types)
		if err := local.RegisterExtension(xt); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r := &combinedTypeResolver{local: local, global: global}
		result, err := r.FindExtensionByNumber(msgName, protoreflect.FieldNumber(fieldNum))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("global_fallback", func(t *testing.T) {
		local := new(protoregistry.Types)
		global := new(protoregistry.Types)
		if err := global.RegisterExtension(xt); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r := &combinedTypeResolver{local: local, global: global}
		result, err := r.FindExtensionByNumber(msgName, protoreflect.FieldNumber(fieldNum))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Error("expected non-nil result")
		}
	})

	t.Run("both_miss", func(t *testing.T) {
		r := &combinedTypeResolver{local: new(protoregistry.Types), global: new(protoregistry.Types)}
		_, err := r.FindExtensionByNumber("nonexistent.Msg", 999)
		if err == nil {
			t.Error("expected error")
		}
	})
}

// ---------------------------------------------------------------------------
// helpers.go: combinedFileResolver fallback paths
// ---------------------------------------------------------------------------

func TestCombinedFileResolver_FindFileByPath_GlobalFallback(t *testing.T) {
	fdp := &descriptorpb.FileDescriptorProto{
		Name:    new("globalonly.proto"),
		Package: new("globalonly"),
		Syntax:  new("proto3"),
	}
	fd, err := protodesc.NewFile(fdp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	global := new(protoregistry.Files)
	if err := global.RegisterFile(fd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := &combinedFileResolver{local: new(protoregistry.Files), global: global}
	result, err := r.FindFileByPath("globalonly.proto")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := result.Path(); got != "globalonly.proto" {
		t.Errorf("got %q, want %q", got, "globalonly.proto")
	}
}

func TestCombinedFileResolver_FindFileByPath_LocalHit(t *testing.T) {
	fdp := &descriptorpb.FileDescriptorProto{
		Name:    new("localonly.proto"),
		Package: new("localonly"),
		Syntax:  new("proto3"),
	}
	fd, err := protodesc.NewFile(fdp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	local := new(protoregistry.Files)
	if err := local.RegisterFile(fd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := &combinedFileResolver{local: local, global: new(protoregistry.Files)}
	result, err := r.FindFileByPath("localonly.proto")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := result.Path(); got != "localonly.proto" {
		t.Errorf("got %q, want %q", got, "localonly.proto")
	}
}

func TestCombinedFileResolver_FindDescriptorByName_GlobalFallback(t *testing.T) {
	fdp := &descriptorpb.FileDescriptorProto{
		Name:    new("globalonly2.proto"),
		Package: new("globalonly2"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Msg"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name: new("x"), Number: proto.Int32(1),
				Type:  descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
				Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			}},
		}},
	}
	fd, err := protodesc.NewFile(fdp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	global := new(protoregistry.Files)
	if err := global.RegisterFile(fd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := &combinedFileResolver{local: new(protoregistry.Files), global: global}
	desc, err := r.FindDescriptorByName("globalonly2.Msg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := desc.FullName(); got != protoreflect.FullName("globalonly2.Msg") {
		t.Errorf("got %q, want %q", got, "globalonly2.Msg")
	}
}

func TestCombinedFileResolver_FindDescriptorByName_LocalHit(t *testing.T) {
	fdp := &descriptorpb.FileDescriptorProto{
		Name:    new("localonly3.proto"),
		Package: new("localonly3"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("Msg"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name: new("x"), Number: proto.Int32(1),
				Type:  descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
				Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			}},
		}},
	}
	fd, err := protodesc.NewFile(fdp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	local := new(protoregistry.Files)
	if err := local.RegisterFile(fd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := &combinedFileResolver{local: local, global: new(protoregistry.Files)}
	desc, err := r.FindDescriptorByName("localonly3.Msg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := desc.FullName(); got != protoreflect.FullName("localonly3.Msg") {
		t.Errorf("got %q, want %q", got, "localonly3.Msg")
	}
}

// ---------------------------------------------------------------------------
// helpers.go: extractBytes — non-bytes error
// ---------------------------------------------------------------------------

func TestExtractBytes_NonBytesValue(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = m.extractBytes(rt.ToValue(42))
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), "expected Uint8Array or ArrayBuffer") {
		t.Errorf("error %q should contain %q", err.Error(), "expected Uint8Array or ArrayBuffer")
	}
}

// ---------------------------------------------------------------------------
// helpers.go: newUint8Array — fallback paths
// ---------------------------------------------------------------------------

func TestNewUint8Array_NoGlobal(t *testing.T) {
	rt := goja.New()
	if err := rt.Set("Uint8Array", goja.Undefined()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := m.newUint8Array([]byte{1, 2, 3})
	if result == nil {
		t.Error("expected non-nil result")
	}
	exported := result.Export()
	if _, isAB := exported.(goja.ArrayBuffer); !isAB {
		t.Errorf("expected ArrayBuffer, got %T", exported)
	}
}

func TestNewUint8Array_ConstructorError(t *testing.T) {
	rt := goja.New()
	if err := rt.Set("Uint8Array", "not_a_constructor"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := m.newUint8Array([]byte{1, 2, 3})
	if result == nil {
		t.Error("expected non-nil result")
	}
	exported := result.Export()
	if _, isAB := exported.(goja.ArrayBuffer); !isAB {
		t.Errorf("expected ArrayBuffer fallback, got %T", exported)
	}
}

// ---------------------------------------------------------------------------
// helpers.go: extractMessageDesc — invalid holder / non-object path
// ---------------------------------------------------------------------------

func TestExtractMessageDesc_InvalidHolder(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	obj := rt.NewObject()
	if err := obj.Set("_pbMsgDesc", "not a holder"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = m.extractMessageDesc(obj)
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), "not a protobuf message type constructor") {
		t.Errorf("error %q should contain %q", err.Error(), "not a protobuf message type constructor")
	}
}

func TestExtractMessageDesc_NoDescProperty(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	obj := rt.NewObject()
	_, err = m.extractMessageDesc(obj)
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), "not a protobuf message type constructor") {
		t.Errorf("error %q should contain %q", err.Error(), "not a protobuf message type constructor")
	}
}

func TestExtractMessageDesc_NilHolder(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	obj := rt.NewObject()
	if err := obj.Set("_pbMsgDesc", (*messageDescHolder)(nil)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = m.extractMessageDesc(obj)
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), "not a protobuf message type constructor") {
		t.Errorf("error %q should contain %q", err.Error(), "not a protobuf message type constructor")
	}
}

// ---------------------------------------------------------------------------
// descriptors.go: loadFileDescriptorProtoBytes — already-registered path
// ---------------------------------------------------------------------------

func TestLoadFileDescriptorProtoBytes_AlreadyRegistered(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fdp := testFileDescriptorProto()
	data, err := proto.Marshal(fdp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Load once.
	names1, err := m.loadFileDescriptorProtoBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names1) == 0 {
		t.Error("expected non-empty names")
	}

	// Load again — should return nil, nil since already registered.
	names2, err := m.loadFileDescriptorProtoBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if names2 != nil {
		t.Errorf("expected nil, got %v", names2)
	}
}

// ---------------------------------------------------------------------------
// descriptors.go: jsLoadFileDescriptorProto via JS (was 0%)
// ---------------------------------------------------------------------------

func TestJsLoadFileDescriptorProto_ViaJS(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pb := rt.NewObject()
	m.setupExports(pb)
	if err := rt.Set("pb", pb); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fdp := testFileDescriptorProto()
	data, err := proto.Marshal(fdp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := rt.Set("protoBytes", rt.NewArrayBuffer(data)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v, err := rt.RunString(`
		var names = pb.loadFileDescriptorProto(new Uint8Array(protoBytes));
		names.length > 0
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

func TestJsLoadFileDescriptorProto_InvalidInput(t *testing.T) {
	env := newTestEnv(t)
	env.mustFail(t, `pb.loadFileDescriptorProto("not bytes")`)
}

func TestJsLoadFileDescriptorProto_BadProtoData(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pb := rt.NewObject()
	m.setupExports(pb)
	if err := rt.Set("pb", pb); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fdp := &descriptorpb.FileDescriptorProto{
		Name:       new("dep.proto"),
		Package:    new("dep"),
		Syntax:     new("proto3"),
		Dependency: []string{"nonexistent.proto"},
	}
	data, err := proto.Marshal(fdp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := rt.Set("badBytes", rt.NewArrayBuffer(data)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = rt.RunString(`pb.loadFileDescriptorProto(new Uint8Array(badBytes))`)
	if err == nil {
		t.Error("expected error")
	}
}

// ---------------------------------------------------------------------------
// descriptors.go: jsLoadDescriptorSet — loadDescriptorSetBytes error via JS
// ---------------------------------------------------------------------------

func TestJsLoadDescriptorSet_BadProtoData(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pb := rt.NewObject()
	m.setupExports(pb)
	if err := rt.Set("pb", pb); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{{
			Name:    new("bad2.proto"),
			Package: new("bad2"),
			Syntax:  new("proto3"),
			MessageType: []*descriptorpb.DescriptorProto{{
				Name: new("Bad"),
				Field: []*descriptorpb.FieldDescriptorProto{{
					Name: new("ref"), Number: proto.Int32(1),
					Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
					TypeName: new(".nonexist.Missing"),
					Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				}},
			}},
		}},
	}
	data, err := proto.Marshal(fds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := rt.Set("badBytes", rt.NewArrayBuffer(data)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = rt.RunString(`pb.loadDescriptorSet(new Uint8Array(badBytes))`)
	if err == nil {
		t.Error("expected error")
	}
}

// ---------------------------------------------------------------------------
// message.go: unwrapMessage — non-Object value
// ---------------------------------------------------------------------------

func TestUnwrapMessage_NonObject(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = m.unwrapMessage(rt.ToValue(42))
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), "expected protobuf message object") {
		t.Errorf("error %q should contain %q", err.Error(), "expected protobuf message object")
	}
}

func TestUnwrapMessage_ObjectWithoutPbMsg(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	obj := rt.NewObject()
	_, err = m.unwrapMessage(obj)
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), "not a protobuf message wrapper") {
		t.Errorf("error %q should contain %q", err.Error(), "not a protobuf message wrapper")
	}
}

func TestUnwrapMessage_InvalidPbMsg(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	obj := rt.NewObject()
	if err := obj.Set("_pbMsg", "wrong"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = m.unwrapMessage(obj)
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), "not a protobuf message wrapper") {
		t.Errorf("error %q should contain %q", err.Error(), "not a protobuf message wrapper")
	}
}

// ---------------------------------------------------------------------------
// message.go: wrapMessage — set error paths (repeated/map)
// ---------------------------------------------------------------------------

func TestWrapMessage_SetRepeatedError(t *testing.T) {
	env := newTestEnv(t)
	env.mustFail(t, `
		var msg = new (pb.messageType('test.RepeatedMessage'))();
		msg.set('numbers', [9999999999]);
	`)
}

func TestWrapMessage_SetMapError(t *testing.T) {
	env := newTestEnv(t)
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
	env.mustFail(t, `msg.get('numbers').set(0, 9999999999)`)
}

func TestRepeatedField_AddError(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.RepeatedMessage'))();
	`)
	env.mustFail(t, `msg.get('numbers').add(9999999999)`)
}

// ---------------------------------------------------------------------------
// message.go: wrapMapField — get/set/has/delete error paths
// ---------------------------------------------------------------------------

func TestMapField_GetKeyError(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	env.run(t, `var MKM = pb.messageType('mapkeys.MultiKeyMap')`)
	env.run(t, `var msg = new MKM()`)
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
	env.run(t, `var MM = pb.messageType('test.MapMessage')`)
	env.run(t, `var mm = new MM()`)
	env.mustFail(t, `mm.get('counts').set('key', 9999999999)`)
}

// ---------------------------------------------------------------------------
