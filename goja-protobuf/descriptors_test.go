package gojaprotobuf

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestLoadDescriptorSet_Success(t *testing.T) {
	env := newTestEnv(t)

	// Test descriptors were loaded by newTestEnv; verify that
	// messageType resolves without error.
	v := env.run(t, `typeof pb.messageType('test.SimpleMessage')`)
	assert.Equal(t, "function", v.String())
}

func TestLoadDescriptorSet_InvalidBytes(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

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
	require.NoError(t, err)

	_, err = m.loadDescriptorSetBytes(data)
	assert.Error(t, err)
}

func TestLoadDescriptorSet_EmptySet(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	emptyFDS := &descriptorpb.FileDescriptorSet{}
	data, err := proto.Marshal(emptyFDS)
	require.NoError(t, err)

	names, err := m.loadDescriptorSetBytes(data)
	require.NoError(t, err)
	assert.Empty(t, names)
}

func TestLoadDescriptorSet_ReturnsTypeNames(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	names, err := m.loadDescriptorSetBytes(testDescriptorSetBytes())
	require.NoError(t, err)

	// Verify all expected top-level types are registered.
	assert.Contains(t, names, "test.SimpleMessage")
	assert.Contains(t, names, "test.AllTypes")
	assert.Contains(t, names, "test.NestedInner")
	assert.Contains(t, names, "test.NestedOuter")
	assert.Contains(t, names, "test.OneofMessage")
	assert.Contains(t, names, "test.RepeatedMessage")
	assert.Contains(t, names, "test.MapMessage")
	assert.Contains(t, names, "test.TestEnum")
}

func TestLoadDescriptorSet_DuplicateLoad(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	data := testDescriptorSetBytes()
	names1, err := m.loadDescriptorSetBytes(data)
	require.NoError(t, err)
	assert.NotEmpty(t, names1)

	// Loading the same set again should silently skip already-registered files.
	names2, err := m.loadDescriptorSetBytes(data)
	require.NoError(t, err)
	assert.Empty(t, names2)
}

func TestLoadFileDescriptorProto_Success(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	fdp := testFileDescriptorProto()
	data, err := proto.Marshal(fdp)
	require.NoError(t, err)

	names, err := m.loadFileDescriptorProtoBytes(data)
	require.NoError(t, err)
	assert.Contains(t, names, "test.SimpleMessage")
	assert.Contains(t, names, "test.TestEnum")
}

func TestLoadFileDescriptorProto_InvalidBytes(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	// File with missing dependency should fail.
	fdp := &descriptorpb.FileDescriptorProto{
		Name:       new("dep.proto"),
		Package:    new("dep"),
		Syntax:     new("proto3"),
		Dependency: []string{"nonexistent.proto"},
	}
	data, err := proto.Marshal(fdp)
	require.NoError(t, err)

	_, err = m.loadFileDescriptorProtoBytes(data)
	assert.Error(t, err)
}

func TestLoadDescriptorSet_ViaJS(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)

	pb := rt.NewObject()
	m.setupExports(pb)
	_ = rt.Set("pb", pb)
	_ = rt.Set("descriptorBytes", rt.NewArrayBuffer(testDescriptorSetBytes()))

	v, err := rt.RunString(`
		var types = pb.loadDescriptorSet(new Uint8Array(descriptorBytes));
		types.length > 0
	`)
	require.NoError(t, err)
	assert.True(t, v.ToBoolean())
}

func TestLoadDescriptorSet_ViaJS_InvalidInput(t *testing.T) {
	env := newTestEnv(t)
	// Passing a non-byte value should throw.
	env.mustFail(t, `pb.loadDescriptorSet("not bytes")`)
}
