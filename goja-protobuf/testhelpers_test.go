package gojaprotobuf

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

type testEnv struct {
	rt *goja.Runtime
	m  *Module
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	rt := goja.New()
	m, err := New(rt)
	require.NoError(t, err)
	_, err = m.loadDescriptorSetBytes(testDescriptorSetBytes())
	require.NoError(t, err)
	pb := rt.NewObject()
	m.setupExports(pb)
	if err := rt.Set("pb", pb); err != nil {
		t.Fatal(err)
	}
	return &testEnv{rt: rt, m: m}
}

func (e *testEnv) run(t *testing.T, code string) goja.Value {
	t.Helper()
	v, err := e.rt.RunString(code)
	require.NoError(t, err)
	return v
}

func (e *testEnv) mustFail(t *testing.T, code string) {
	t.Helper()
	_, err := e.rt.RunString(code)
	require.Error(t, err)
}

func testDescriptorSetBytes() []byte {
	fds := testFileDescriptorSet()
	data, err := proto.Marshal(fds)
	if err != nil {
		panic("testDescriptorSetBytes: " + err.Error())
	}
	return data
}

func testFileDescriptorSet() *descriptorpb.FileDescriptorSet {
	return &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{testFileDescriptorProto()},
	}
}

func testFileDescriptorProto() *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:    new("test.proto"),
		Package: new("test"),
		Syntax:  new("proto3"),
		EnumType: []*descriptorpb.EnumDescriptorProto{{
			Name: new("TestEnum"),
			Value: []*descriptorpb.EnumValueDescriptorProto{
				{Name: new("UNKNOWN"), Number: proto.Int32(0)},
				{Name: new("FIRST"), Number: proto.Int32(1)},
				{Name: new("SECOND"), Number: proto.Int32(2)},
				{Name: new("THIRD"), Number: proto.Int32(3)},
			},
		}},
		MessageType: []*descriptorpb.DescriptorProto{
			nestedInnerDesc(),
			simpleMessageDesc(),
			allTypesDesc(),
			nestedOuterDesc(),
			oneofMessageDesc(),
			repeatedMessageDesc(),
			mapMessageDesc(),
		},
	}
}

func nestedInnerDesc() *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name: new("NestedInner"),
		Field: []*descriptorpb.FieldDescriptorProto{{
			Name: new("value"), Number: proto.Int32(1),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			JsonName: new("value"),
		}},
	}
}

func simpleMessageDesc() *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name: new("SimpleMessage"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{Name: new("name"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("name")},
			{Name: new("value"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("value")},
		},
	}
}

func allTypesDesc() *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name: new("AllTypes"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{Name: new("int32_val"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("int32Val")},
			{Name: new("int64_val"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("int64Val")},
			{Name: new("uint32_val"), Number: proto.Int32(3), Type: descriptorpb.FieldDescriptorProto_TYPE_UINT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("uint32Val")},
			{Name: new("uint64_val"), Number: proto.Int32(4), Type: descriptorpb.FieldDescriptorProto_TYPE_UINT64.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("uint64Val")},
			{Name: new("float_val"), Number: proto.Int32(5), Type: descriptorpb.FieldDescriptorProto_TYPE_FLOAT.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("floatVal")},
			{Name: new("double_val"), Number: proto.Int32(6), Type: descriptorpb.FieldDescriptorProto_TYPE_DOUBLE.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("doubleVal")},
			{Name: new("bool_val"), Number: proto.Int32(7), Type: descriptorpb.FieldDescriptorProto_TYPE_BOOL.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("boolVal")},
			{Name: new("string_val"), Number: proto.Int32(8), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("stringVal")},
			{Name: new("bytes_val"), Number: proto.Int32(9), Type: descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("bytesVal")},
			{Name: new("enum_val"), Number: proto.Int32(10), Type: descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(), TypeName: new(".test.TestEnum"), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("enumVal")},
			{Name: new("nested_val"), Number: proto.Int32(11), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: new(".test.NestedInner"), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("nestedVal")},
			{Name: new("repeated_int32"), Number: proto.Int32(12), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(), JsonName: new("repeatedInt32")},
			{Name: new("repeated_string"), Number: proto.Int32(13), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(), JsonName: new("repeatedString")},
			{Name: new("tags"), Number: proto.Int32(14), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: new(".test.AllTypes.TagsEntry"), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(), JsonName: new("tags")},
			{Name: new("sint32_val"), Number: proto.Int32(15), Type: descriptorpb.FieldDescriptorProto_TYPE_SINT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("sint32Val")},
			{Name: new("sint64_val"), Number: proto.Int32(16), Type: descriptorpb.FieldDescriptorProto_TYPE_SINT64.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("sint64Val")},
			{Name: new("fixed32_val"), Number: proto.Int32(17), Type: descriptorpb.FieldDescriptorProto_TYPE_FIXED32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("fixed32Val")},
			{Name: new("fixed64_val"), Number: proto.Int32(18), Type: descriptorpb.FieldDescriptorProto_TYPE_FIXED64.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("fixed64Val")},
			{Name: new("sfixed32_val"), Number: proto.Int32(19), Type: descriptorpb.FieldDescriptorProto_TYPE_SFIXED32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("sfixed32Val")},
			{Name: new("sfixed64_val"), Number: proto.Int32(20), Type: descriptorpb.FieldDescriptorProto_TYPE_SFIXED64.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("sfixed64Val")},
			{Name: new("optional_string"), Number: proto.Int32(21), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("optionalString"), Proto3Optional: new(true), OneofIndex: proto.Int32(0)},
		},
		NestedType: []*descriptorpb.DescriptorProto{{
			Name: new("TagsEntry"),
			Field: []*descriptorpb.FieldDescriptorProto{
				{Name: new("key"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("key")},
				{Name: new("value"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("value")},
			},
			Options: &descriptorpb.MessageOptions{MapEntry: new(true)},
		}},
		OneofDecl: []*descriptorpb.OneofDescriptorProto{
			{Name: new("_optional_string")},
		},
	}
}

func nestedOuterDesc() *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name: new("NestedOuter"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{Name: new("nested_inner"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: new(".test.NestedInner"), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("nestedInner")},
			{Name: new("name"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("name")},
		},
	}
}

func oneofMessageDesc() *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name: new("OneofMessage"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{Name: new("str_choice"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), OneofIndex: proto.Int32(0), JsonName: new("strChoice")},
			{Name: new("int_choice"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), OneofIndex: proto.Int32(0), JsonName: new("intChoice")},
		},
		OneofDecl: []*descriptorpb.OneofDescriptorProto{{Name: new("choice")}},
	}
}

func repeatedMessageDesc() *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name: new("RepeatedMessage"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{Name: new("items"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(), JsonName: new("items")},
			{Name: new("numbers"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(), JsonName: new("numbers")},
		},
	}
}

func mapMessageDesc() *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name: new("MapMessage"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{Name: new("tags"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: new(".test.MapMessage.TagsEntry"), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(), JsonName: new("tags")},
			{Name: new("counts"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: new(".test.MapMessage.CountsEntry"), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(), JsonName: new("counts")},
		},
		NestedType: []*descriptorpb.DescriptorProto{
			{
				Name: new("TagsEntry"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: new("key"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("key")},
					{Name: new("value"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("value")},
				},
				Options: &descriptorpb.MessageOptions{MapEntry: new(true)},
			},
			{
				Name: new("CountsEntry"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: new("key"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("key")},
					{Name: new("value"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: new("value")},
				},
				Options: &descriptorpb.MessageOptions{MapEntry: new(true)},
			},
		},
	}
}
