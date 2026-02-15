package gojaprotojson_test

import (
	"testing"

	"github.com/dop251/goja"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
	gojaprotojson "github.com/joeycumines/goja-protojson"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

type testEnv struct {
	rt *goja.Runtime
	pb *gojaprotobuf.Module
	pj *gojaprotojson.Module
	t  *testing.T
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	rt := goja.New()
	pb, err := gojaprotobuf.New(rt)
	require.NoError(t, err)
	_, err = pb.LoadDescriptorSetBytes(testDescriptorSetBytes())
	require.NoError(t, err)
	pbObj := rt.NewObject()
	pb.SetupExports(pbObj)
	require.NoError(t, rt.Set("pb", pbObj))
	pj, err := gojaprotojson.New(rt, gojaprotojson.WithProtobuf(pb))
	require.NoError(t, err)
	pjObj := rt.NewObject()
	pj.SetupExports(pjObj)
	require.NoError(t, rt.Set("protojson", pjObj))
	return &testEnv{rt: rt, pb: pb, pj: pj, t: t}
}

func (e *testEnv) run(code string) goja.Value {
	e.t.Helper()
	v, err := e.rt.RunString(code)
	require.NoError(e.t, err)
	return v
}

func (e *testEnv) mustFail(code string) error {
	e.t.Helper()
	_, err := e.rt.RunString(code)
	require.Error(e.t, err)
	return err
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
	str := proto.String
	i32 := proto.Int32
	tSTRING := descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()
	tINT32 := descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum()
	tINT64 := descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum()
	tUINT32 := descriptorpb.FieldDescriptorProto_TYPE_UINT32.Enum()
	tUINT64 := descriptorpb.FieldDescriptorProto_TYPE_UINT64.Enum()
	tFLOAT := descriptorpb.FieldDescriptorProto_TYPE_FLOAT.Enum()
	tDOUBLE := descriptorpb.FieldDescriptorProto_TYPE_DOUBLE.Enum()
	tBOOL := descriptorpb.FieldDescriptorProto_TYPE_BOOL.Enum()
	tBYTES := descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum()
	tENUM := descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum()
	tMESSAGE := descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum()
	lOPT := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum()
	lREP := descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum()

	f := func(name string, num int32, typ *descriptorpb.FieldDescriptorProto_Type, label *descriptorpb.FieldDescriptorProto_Label, jsonName string) *descriptorpb.FieldDescriptorProto {
		return &descriptorpb.FieldDescriptorProto{
			Name: str(name), Number: i32(num), Type: typ, Label: label, JsonName: str(jsonName),
		}
	}

	return &descriptorpb.FileDescriptorProto{
		Name:    str("test.proto"),
		Package: str("test"),
		Syntax:  str("proto3"),
		EnumType: []*descriptorpb.EnumDescriptorProto{{
			Name: str("TestEnum"),
			Value: []*descriptorpb.EnumValueDescriptorProto{
				{Name: str("UNKNOWN"), Number: i32(0)},
				{Name: str("FIRST"), Number: i32(1)},
				{Name: str("SECOND"), Number: i32(2)},
				{Name: str("THIRD"), Number: i32(3)},
			},
		}},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name:  str("NestedInner"),
				Field: []*descriptorpb.FieldDescriptorProto{f("value", 1, tINT32, lOPT, "value")},
			},
			{
				Name: str("SimpleMessage"),
				Field: []*descriptorpb.FieldDescriptorProto{
					f("name", 1, tSTRING, lOPT, "name"),
					f("value", 2, tINT32, lOPT, "value"),
				},
			},
			{
				Name: str("AllTypes"),
				Field: []*descriptorpb.FieldDescriptorProto{
					f("int32_val", 1, tINT32, lOPT, "int32Val"),
					f("int64_val", 2, tINT64, lOPT, "int64Val"),
					f("uint32_val", 3, tUINT32, lOPT, "uint32Val"),
					f("uint64_val", 4, tUINT64, lOPT, "uint64Val"),
					f("float_val", 5, tFLOAT, lOPT, "floatVal"),
					f("double_val", 6, tDOUBLE, lOPT, "doubleVal"),
					f("bool_val", 7, tBOOL, lOPT, "boolVal"),
					f("string_val", 8, tSTRING, lOPT, "stringVal"),
					f("bytes_val", 9, tBYTES, lOPT, "bytesVal"),
					{Name: str("enum_val"), Number: i32(10), Type: tENUM, TypeName: str(".test.TestEnum"), Label: lOPT, JsonName: str("enumVal")},
					{Name: str("nested_val"), Number: i32(11), Type: tMESSAGE, TypeName: str(".test.NestedInner"), Label: lOPT, JsonName: str("nestedVal")},
					f("repeated_int32", 12, tINT32, lREP, "repeatedInt32"),
					f("repeated_string", 13, tSTRING, lREP, "repeatedString"),
				},
			},
			{
				Name: str("NestedOuter"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: str("nested_inner"), Number: i32(1), Type: tMESSAGE, TypeName: str(".test.NestedInner"), Label: lOPT, JsonName: str("nestedInner")},
					f("name", 2, tSTRING, lOPT, "name"),
				},
			},
			{
				Name: str("OneofMessage"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: str("str_choice"), Number: i32(1), Type: tSTRING, Label: lOPT, OneofIndex: i32(0), JsonName: str("strChoice")},
					{Name: str("int_choice"), Number: i32(2), Type: tINT32, Label: lOPT, OneofIndex: i32(0), JsonName: str("intChoice")},
				},
				OneofDecl: []*descriptorpb.OneofDescriptorProto{{Name: str("choice")}},
			},
			{
				Name: str("RepeatedMessage"),
				Field: []*descriptorpb.FieldDescriptorProto{
					f("items", 1, tSTRING, lREP, "items"),
					f("numbers", 2, tINT32, lREP, "numbers"),
				},
			},
			{
				Name: str("MapMessage"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{Name: str("tags"), Number: i32(1), Type: tMESSAGE, TypeName: str(".test.MapMessage.TagsEntry"), Label: lREP, JsonName: str("tags")},
					{Name: str("counts"), Number: i32(2), Type: tMESSAGE, TypeName: str(".test.MapMessage.CountsEntry"), Label: lREP, JsonName: str("counts")},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: str("TagsEntry"),
						Field: []*descriptorpb.FieldDescriptorProto{
							f("key", 1, tSTRING, lOPT, "key"),
							f("value", 2, tSTRING, lOPT, "value"),
						},
						Options: &descriptorpb.MessageOptions{MapEntry: new(true)},
					},
					{
						Name: str("CountsEntry"),
						Field: []*descriptorpb.FieldDescriptorProto{
							f("key", 1, tSTRING, lOPT, "key"),
							f("value", 2, tINT32, lOPT, "value"),
						},
						Options: &descriptorpb.MessageOptions{MapEntry: new(true)},
					},
				},
			},
		},
	}
}
