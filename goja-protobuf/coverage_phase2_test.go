package gojaprotobuf

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NotNil(t, resolver, "TypeResolver must return non-nil")

	// FindMessageByName — type in local registries.
	mt, err := resolver.FindMessageByName("test.SimpleMessage")
	require.NoError(t, err)
	assert.Equal(t, protoreflect.FullName("test.SimpleMessage"), mt.Descriptor().FullName())

	// FindMessageByURL — same type via URL.
	mt2, err := resolver.FindMessageByURL("test.SimpleMessage")
	require.NoError(t, err)
	assert.Equal(t, protoreflect.FullName("test.SimpleMessage"), mt2.Descriptor().FullName())

	// FindExtensionByName — not found is OK; we just prove the method works.
	_, err = resolver.FindExtensionByName("nonexistent.ext")
	assert.Error(t, err)

	// FindExtensionByNumber — not found is OK.
	_, err = resolver.FindExtensionByNumber("nonexistent.Msg", 999)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// 2. loadDescriptorSetBytes RegisterFile name conflict (descriptors.go:80)
//
// Two files in the same FDS with different paths but the same fully-
// qualified message name. The first file registers fine; the second
// file's RegisterFile call fails due to the name conflict, triggering
// the defensive `continue` on line 85.
// ---------------------------------------------------------------------------

func TestLoadDescriptorSetBytes_RegisterFileNameConflict(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:    proto.String("phase2_conflict_a.proto"),
				Package: proto.String("phase2conflict"),
				Syntax:  proto.String("proto3"),
				MessageType: []*descriptorpb.DescriptorProto{{
					Name: proto.String("DupMsg"),
					Field: []*descriptorpb.FieldDescriptorProto{{
						Name:     proto.String("x"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName: proto.String("x"),
					}},
				}},
			},
			{
				Name:    proto.String("phase2_conflict_b.proto"),
				Package: proto.String("phase2conflict"),
				Syntax:  proto.String("proto3"),
				MessageType: []*descriptorpb.DescriptorProto{{
					Name: proto.String("DupMsg"), // same full name: phase2conflict.DupMsg
					Field: []*descriptorpb.FieldDescriptorProto{{
						Name:     proto.String("y"),
						Number:   proto.Int32(1),
						Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
						Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
						JsonName: proto.String("y"),
					}},
				}},
			},
		},
	}
	data, err := proto.Marshal(fds)
	require.NoError(t, err)

	// The second file's RegisterFile fails (name conflict), but the
	// function treats it as non-fatal and continues. Overall call succeeds.
	names, err := m.loadDescriptorSetBytes(data)
	require.NoError(t, err)
	// Only type from the first file should be registered.
	assert.Contains(t, names, "phase2conflict.DupMsg")
}

// ---------------------------------------------------------------------------
// 3. loadFileDescriptorProtoBytes RegisterFile name conflict
//    (descriptors.go:110)
//
// Load two files sequentially with different paths but the same fully-
// qualified message name. The second load's RegisterFile fails, and the
// function returns nil, nil.
// ---------------------------------------------------------------------------

func TestLoadFileDescriptorProtoBytes_RegisterFileNameConflict(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	// First file: registers phase2conflict2.DupMsg successfully.
	fdp1 := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("phase2_conflict_c.proto"),
		Package: proto.String("phase2conflict2"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("DupMsg"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:     proto.String("a"),
				Number:   proto.Int32(1),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
				Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				JsonName: proto.String("a"),
			}},
		}},
	}
	data1, err := proto.Marshal(fdp1)
	require.NoError(t, err)
	names1, err := m.loadFileDescriptorProtoBytes(data1)
	require.NoError(t, err)
	require.Contains(t, names1, "phase2conflict2.DupMsg")

	// Second file: different path, same type name → RegisterFile fails.
	fdp2 := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("phase2_conflict_d.proto"),
		Package: proto.String("phase2conflict2"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("DupMsg"), // same full name: phase2conflict2.DupMsg
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name:     proto.String("b"),
				Number:   proto.Int32(1),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				JsonName: proto.String("b"),
			}},
		}},
	}
	data2, err := proto.Marshal(fdp2)
	require.NoError(t, err)
	names2, err := m.loadFileDescriptorProtoBytes(data2)
	require.NoError(t, err)
	// RegisterFile fails → returns nil, nil (no names registered).
	assert.Nil(t, names2)
}

// ---------------------------------------------------------------------------
// Additional TypeResolver test: verify FindMessageByName falls back to
// global resolver (not just local).
// ---------------------------------------------------------------------------

func TestTypeResolver_GlobalFallback(t *testing.T) {
	rt := goja.New()
	// Create a module WITHOUT loading any descriptors into localTypes.
	m, err := New(rt)
	require.NoError(t, err)

	resolver := m.TypeResolver()
	require.NotNil(t, resolver)

	// Try to find a message that's only in global registry (not local).
	// google.protobuf.Timestamp is in protoregistry.GlobalTypes.
	mt, err := resolver.FindMessageByName("google.protobuf.Timestamp")
	require.NoError(t, err)
	assert.Equal(t, protoreflect.FullName("google.protobuf.Timestamp"), mt.Descriptor().FullName())

	// Also via URL.
	mt2, err := resolver.FindMessageByURL("type.googleapis.com/google.protobuf.Timestamp")
	require.NoError(t, err)
	assert.Equal(t, protoreflect.FullName("google.protobuf.Timestamp"), mt2.Descriptor().FullName())
}

// ---------------------------------------------------------------------------
// Additional FileResolver test: basic verification via public API.
// ---------------------------------------------------------------------------

func TestFileResolver_ReturnsWorkingResolver(t *testing.T) {
	env := newTestEnv(t)
	resolver := env.m.FileResolver()
	require.NotNil(t, resolver)

	// FindDescriptorByName for a locally-loaded type.
	desc, err := resolver.FindDescriptorByName("test.SimpleMessage")
	require.NoError(t, err)
	assert.Equal(t, protoreflect.FullName("test.SimpleMessage"), desc.FullName())

	// FindFileByPath for a locally-loaded file.
	fd, err := resolver.FindFileByPath("test.proto")
	require.NoError(t, err)
	assert.Equal(t, "test.proto", fd.Path())
}
