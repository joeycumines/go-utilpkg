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
		Name:    proto.String("test.proto"),
		Package: proto.String("test"),
		Syntax:  proto.String("proto3"),
		EnumType: []*descriptorpb.EnumDescriptorProto{{
			Name: proto.String("TestEnum"),
			Value: []*descriptorpb.EnumValueDescriptorProto{
				{Name: proto.String("UNKNOWN"), Number: proto.Int32(0)},
				{Name: proto.String("FIRST"), Number: proto.Int32(1)},
				{Name: proto.String("SECOND"), Number: proto.Int32(2)},
				{Name: proto.String("THIRD"), Number: proto.Int32(3)},
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
		Name: proto.String("NestedInner"),
		Field: []*descriptorpb.FieldDescriptorProto{{
			Name: proto.String("value"), Number: proto.Int32(1),
			Type:     descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			JsonName: proto.String("value"),
		}},
	}
}

func simpleMessageDesc() *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name: proto.String("SimpleMessage"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{Name: proto.String("name"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("name")},
			{Name: proto.String("value"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("value")},
		},
	}
}

func allTypesDesc() *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name: proto.String("AllTypes"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{Name: proto.String("int32_val"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("int32Val")},
			{Name: proto.String("int64_val"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("int64Val")},
			{Name: proto.String("uint32_val"), Number: proto.Int32(3), Type: descriptorpb.FieldDescriptorProto_TYPE_UINT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("uint32Val")},
			{Name: proto.String("uint64_val"), Number: proto.Int32(4), Type: descriptorpb.FieldDescriptorProto_TYPE_UINT64.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("uint64Val")},
			{Name: proto.String("float_val"), Number: proto.Int32(5), Type: descriptorpb.FieldDescriptorProto_TYPE_FLOAT.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("floatVal")},
			{Name: proto.String("double_val"), Number: proto.Int32(6), Type: descriptorpb.FieldDescriptorProto_TYPE_DOUBLE.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("doubleVal")},
			{Name: proto.String("bool_val"), Number: proto.Int32(7), Type: descriptorpb.FieldDescriptorProto_TYPE_BOOL.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("boolVal")},
			{Name: proto.String("string_val"), Number: proto.Int32(8), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("stringVal")},
			{Name: proto.String("bytes_val"), Number: proto.Int32(9), Type: descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("bytesVal")},
			{Name: proto.String("enum_val"), Number: proto.Int32(10), Type: descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(), TypeName: proto.String(".test.TestEnum"), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("enumVal")},
			{Name: proto.String("nested_val"), Number: proto.Int32(11), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".test.NestedInner"), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("nestedVal")},
			{Name: proto.String("repeated_int32"), Number: proto.Int32(12), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(), JsonName: proto.String("repeatedInt32")},
			{Name: proto.String("repeated_string"), Number: proto.Int32(13), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(), JsonName: proto.String("repeatedString")},
			{Name: proto.String("tags"), Number: proto.Int32(14), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".test.AllTypes.TagsEntry"), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(), JsonName: proto.String("tags")},
			{Name: proto.String("sint32_val"), Number: proto.Int32(15), Type: descriptorpb.FieldDescriptorProto_TYPE_SINT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("sint32Val")},
			{Name: proto.String("sint64_val"), Number: proto.Int32(16), Type: descriptorpb.FieldDescriptorProto_TYPE_SINT64.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("sint64Val")},
			{Name: proto.String("fixed32_val"), Number: proto.Int32(17), Type: descriptorpb.FieldDescriptorProto_TYPE_FIXED32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("fixed32Val")},
			{Name: proto.String("fixed64_val"), Number: proto.Int32(18), Type: descriptorpb.FieldDescriptorProto_TYPE_FIXED64.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("fixed64Val")},
			{Name: proto.String("sfixed32_val"), Number: proto.Int32(19), Type: descriptorpb.FieldDescriptorProto_TYPE_SFIXED32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("sfixed32Val")},
			{Name: proto.String("sfixed64_val"), Number: proto.Int32(20), Type: descriptorpb.FieldDescriptorProto_TYPE_SFIXED64.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("sfixed64Val")},
			{Name: proto.String("optional_string"), Number: proto.Int32(21), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("optionalString"), Proto3Optional: proto.Bool(true), OneofIndex: proto.Int32(0)},
		},
		NestedType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("TagsEntry"),
			Field: []*descriptorpb.FieldDescriptorProto{
				{Name: proto.String("key"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("key")},
				{Name: proto.String("value"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("value")},
			},
			Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)},
		}},
		OneofDecl: []*descriptorpb.OneofDescriptorProto{
			{Name: proto.String("_optional_string")},
		},
	}
}

func nestedOuterDesc() *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name: proto.String("NestedOuter"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{Name: proto.String("nested_inner"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".test.NestedInner"), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("nestedInner")},
			{Name: proto.String("name"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("name")},
		},
	}
}

func oneofMessageDesc() *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name: proto.String("OneofMessage"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{Name: proto.String("str_choice"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), OneofIndex: proto.Int32(0), JsonName: proto.String("strChoice")},
			{Name: proto.String("int_choice"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), OneofIndex: proto.Int32(0), JsonName: proto.String("intChoice")},
		},
		OneofDecl: []*descriptorpb.OneofDescriptorProto{{Name: proto.String("choice")}},
	}
}

func repeatedMessageDesc() *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name: proto.String("RepeatedMessage"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{Name: proto.String("items"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(), JsonName: proto.String("items")},
			{Name: proto.String("numbers"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(), JsonName: proto.String("numbers")},
		},
	}
}

func mapMessageDesc() *descriptorpb.DescriptorProto {
	return &descriptorpb.DescriptorProto{
		Name: proto.String("MapMessage"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{Name: proto.String("tags"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".test.MapMessage.TagsEntry"), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(), JsonName: proto.String("tags")},
			{Name: proto.String("counts"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(), TypeName: proto.String(".test.MapMessage.CountsEntry"), Label: descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(), JsonName: proto.String("counts")},
		},
		NestedType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("TagsEntry"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("key"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("key")},
					{Name: proto.String("value"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("value")},
				},
				Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)},
			},
			{
				Name: proto.String("CountsEntry"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: proto.String("key"), Number: proto.Int32(1), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("key")},
					{Name: proto.String("value"), Number: proto.Int32(2), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), JsonName: proto.String("value")},
				},
				Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)},
			},
		},
	}
}
