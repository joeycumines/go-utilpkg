package gojaprotobuf

import (
	"testing"

	"github.com/dop251/goja"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestLoadDescriptorSet_Success(t *testing.T) {
	env := newTestEnv(t)

	// Test descriptors were loaded by newTestEnv; verify that
	// messageType resolves without error.
	v := env.run(t, `typeof pb.messageType('test.SimpleMessage')`)
	if v.String() != "function" {
		t.Errorf("got %v, want %v", v.String(), "function")
	}
}

func TestLoadDescriptorSet_InvalidBytes(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// A valid FDS containing a message that references a non-existent
	// type should fail during descriptor resolution.
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:    new("bad.proto"),
				Package: new("bad"),
				Syntax:  new("proto3"),
				MessageType: []*descriptorpb.DescriptorProto{
					{
						Name: new("BadMsg"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     new("ref"),
								Number:   proto.Int32(1),
								Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
								TypeName: new(".nonexist.NoSuchMessage"),
								Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
							},
						},
					},
				},
			},
		},
	}
	data, err := proto.Marshal(fds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = m.loadDescriptorSetBytes(data)
	if err == nil {
		t.Error("expected error")
	}
}

func TestLoadDescriptorSet_EmptySet(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	emptyFDS := &descriptorpb.FileDescriptorSet{}
	data, err := proto.Marshal(emptyFDS)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	names, err := m.loadDescriptorSetBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected empty, got %v", names)
	}
}

func TestLoadDescriptorSet_ReturnsTypeNames(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	names, err := m.loadDescriptorSetBytes(testDescriptorSetBytes())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify all expected top-level types are registered.
	for _, want := range []string{
		"test.SimpleMessage",
		"test.AllTypes",
		"test.NestedInner",
		"test.NestedOuter",
		"test.OneofMessage",
		"test.RepeatedMessage",
		"test.MapMessage",
		"test.TestEnum",
	} {
		if !sliceContains(names, want) {
			t.Errorf("expected names to contain %q, got %v", want, names)
		}
	}
}

func TestLoadDescriptorSet_DuplicateLoad(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := testDescriptorSetBytes()
	names1, err := m.loadDescriptorSetBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names1) == 0 {
		t.Error("expected non-empty names")
	}

	// Loading the same set again should silently skip already-registered files.
	names2, err := m.loadDescriptorSetBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names2) != 0 {
		t.Errorf("expected empty, got %v", names2)
	}
}

func TestLoadFileDescriptorProto_Success(t *testing.T) {
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

	names, err := m.loadFileDescriptorProtoBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sliceContains(names, "test.SimpleMessage") {
		t.Errorf("expected names to contain test.SimpleMessage, got %v", names)
	}
	if !sliceContains(names, "test.TestEnum") {
		t.Errorf("expected names to contain test.TestEnum, got %v", names)
	}
}

func TestLoadFileDescriptorProto_InvalidBytes(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// File with missing dependency should fail.
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

	_, err = m.loadFileDescriptorProtoBytes(data)
	if err == nil {
		t.Error("expected error")
	}
}

func TestLoadDescriptorSet_ViaJS(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pb := rt.NewObject()
	m.setupExports(pb)
	_ = rt.Set("pb", pb)
	_ = rt.Set("descriptorBytes", rt.NewArrayBuffer(testDescriptorSetBytes()))

	v, err := rt.RunString(`
		var types = pb.loadDescriptorSet(new Uint8Array(descriptorBytes));
		types.length > 0
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

func TestLoadDescriptorSet_ViaJS_InvalidInput(t *testing.T) {
	env := newTestEnv(t)
	// Passing a non-byte value should throw.
	env.mustFail(t, `pb.loadDescriptorSet("not bytes")`)
}
