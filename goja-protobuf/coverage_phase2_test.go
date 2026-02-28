package gojaprotobuf

import (
	"testing"

	"github.com/dop251/goja"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// ---------------------------------------------------------------------------
// coverage_phase2_test.go — Targeted tests to push goja-protobuf coverage
// from 99.1% toward 100%. Covers 3 of 8 uncovered statement blocks:
//
//   1. TypeResolver          (module.go:127)      — NEW public method, 0%→100%
//   2. loadDescriptorSetBytes RegisterFile error   (descriptors.go:80)
//   3. loadFileDescriptorProtoBytes RegisterFile error (descriptors.go:110)
//
// The remaining 5 uncovered blocks are defensive nil-checks on
// val.ToObject(runtime) (unreachable since goja never returns nil for
// non-nil/undefined/null values) or the proto.Marshal error path in
// jsAnyPack (unreachable since dynamicpb messages always marshal).
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 1. TypeResolver (module.go:127) — 0% → 100%
// ---------------------------------------------------------------------------

func TestTypeResolver_ReturnsWorkingResolver(t *testing.T) {
	env := newTestEnv(t)
	resolver := env.m.TypeResolver()
	if resolver == nil {
		t.Fatal("TypeResolver must return non-nil")
	}

	// FindMessageByName — type in local registries.
	mt, err := resolver.FindMessageByName("test.SimpleMessage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mt.Descriptor().FullName() != protoreflect.FullName("test.SimpleMessage") {
		t.Errorf("got %v, want %v", mt.Descriptor().FullName(), "test.SimpleMessage")
	}

	// FindMessageByURL — same type via URL.
	mt2, err := resolver.FindMessageByURL("test.SimpleMessage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mt2.Descriptor().FullName() != protoreflect.FullName("test.SimpleMessage") {
		t.Errorf("got %v, want %v", mt2.Descriptor().FullName(), "test.SimpleMessage")
	}

	// FindExtensionByName — not found is OK; we just prove the method works.
	_, err = resolver.FindExtensionByName("nonexistent.ext")
	if err == nil {
		t.Error("expected error")
	}

	// FindExtensionByNumber — not found is OK.
	_, err = resolver.FindExtensionByNumber("nonexistent.Msg", 999)
	if err == nil {
		t.Error("expected error")
	}
}

// ---------------------------------------------------------------------------
// 2. loadDescriptorSetBytes RegisterFile name conflict (descriptors.go:80)
// ---------------------------------------------------------------------------

func TestLoadDescriptorSetBytes_RegisterFileNameConflict(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:    new("phase2_conflict_a.proto"),
				Package: new("phase2conflict"),
				Syntax:  new("proto3"),
				MessageType: []*descriptorpb.DescriptorProto{{
					Name: new("DupMsg"),
					Field: []*descriptorpb.FieldDescriptorProto{{
						Name:     new("x"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName: new("x"),
					}},
				}},
			},
			{
				Name:    new("phase2_conflict_b.proto"),
				Package: new("phase2conflict"),
				Syntax:  new("proto3"),
				MessageType: []*descriptorpb.DescriptorProto{{
					Name: new("DupMsg"), // same full name: phase2conflict.DupMsg
					Field: []*descriptorpb.FieldDescriptorProto{{
						Name:     new("y"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName: new("y"),
					}},
				}},
			},
		},
	}
	data, err := proto.Marshal(fds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The second file's RegisterFile fails (name conflict), but the
	// function treats it as non-fatal and continues.
	names, err := m.loadDescriptorSetBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sliceContains(names, "phase2conflict.DupMsg") {
		t.Errorf("expected names to contain phase2conflict.DupMsg, got %v", names)
	}
}

// ---------------------------------------------------------------------------
// 3. loadFileDescriptorProtoBytes RegisterFile name conflict
// ---------------------------------------------------------------------------

func TestLoadFileDescriptorProtoBytes_RegisterFileNameConflict(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First file: registers phase2conflict2.DupMsg successfully.
	fdp1 := &descriptorpb.FileDescriptorProto{
		Name:    new("phase2_conflict_c.proto"),
		Package: new("phase2conflict2"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("DupMsg"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:     new("a"),
				Number:   proto.Int32(1),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
				Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				JsonName: new("a"),
			}},
		}},
	}
	data1, err := proto.Marshal(fdp1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	names1, err := m.loadFileDescriptorProtoBytes(data1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sliceContains(names1, "phase2conflict2.DupMsg") {
		t.Fatalf("expected names to contain phase2conflict2.DupMsg, got %v", names1)
	}

	// Second file: different path, same type name → RegisterFile fails.
	fdp2 := &descriptorpb.FileDescriptorProto{
		Name:    new("phase2_conflict_d.proto"),
		Package: new("phase2conflict2"),
		Syntax:  new("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: new("DupMsg"), // same full name: phase2conflict2.DupMsg
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:     new("b"),
				Number:   proto.Int32(1),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				JsonName: new("b"),
			}},
		}},
	}
	data2, err := proto.Marshal(fdp2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	names2, err := m.loadFileDescriptorProtoBytes(data2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// RegisterFile fails → returns nil, nil (no names registered).
	if names2 != nil {
		t.Errorf("expected nil, got %v", names2)
	}
}

// ---------------------------------------------------------------------------
// Additional TypeResolver test: verify FindMessageByName falls back to
// global resolver (not just local).
// ---------------------------------------------------------------------------

func TestTypeResolver_GlobalFallback(t *testing.T) {
	rt := goja.New()
	// Create a module WITHOUT loading any descriptors into localTypes.
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	resolver := m.TypeResolver()
	if resolver == nil {
		t.Fatal("expected non-nil")
	}

	// Try to find a message that's only in global registry (not local).
	mt, err := resolver.FindMessageByName("google.protobuf.Timestamp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mt.Descriptor().FullName() != protoreflect.FullName("google.protobuf.Timestamp") {
		t.Errorf("got %v, want %v", mt.Descriptor().FullName(), "google.protobuf.Timestamp")
	}

	// Also via URL.
	mt2, err := resolver.FindMessageByURL("type.googleapis.com/google.protobuf.Timestamp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mt2.Descriptor().FullName() != protoreflect.FullName("google.protobuf.Timestamp") {
		t.Errorf("got %v, want %v", mt2.Descriptor().FullName(), "google.protobuf.Timestamp")
	}
}

// ---------------------------------------------------------------------------
// Additional FileResolver test: basic verification via public API.
// ---------------------------------------------------------------------------

func TestFileResolver_ReturnsWorkingResolver(t *testing.T) {
	env := newTestEnv(t)
	resolver := env.m.FileResolver()
	if resolver == nil {
		t.Fatal("expected non-nil")
	}

	// FindDescriptorByName for a locally-loaded type.
	desc, err := resolver.FindDescriptorByName("test.SimpleMessage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if desc.FullName() != protoreflect.FullName("test.SimpleMessage") {
		t.Errorf("got %v, want %v", desc.FullName(), "test.SimpleMessage")
	}

	// FindFileByPath for a locally-loaded file.
	fd, err := resolver.FindFileByPath("test.proto")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fd.Path() != "test.proto" {
		t.Errorf("got %v, want %v", fd.Path(), "test.proto")
	}
}
