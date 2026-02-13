package gojaprotojson_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dop251/goja"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
	gojaprotojson "github.com/joeycumines/goja-protojson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestMarshal_SimpleMessage(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var MT = pb.messageType("test.SimpleMessage");
		var msg = new MT();
		msg.set("name", "hello");
		msg.set("value", 42);
		protojson.marshal(msg);
	`)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(v.String()), &parsed))
	assert.Equal(t, "hello", parsed["name"])
	assert.Equal(t, float64(42), parsed["value"])
}

func TestMarshal_AllScalarTypes(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var MT = pb.messageType("test.AllTypes");
		var msg = new MT();
		msg.set("int32_val", -42);
		msg.set("int64_val", "9007199254740993");
		msg.set("uint32_val", 100);
		msg.set("uint64_val", "9007199254740991");
		msg.set("float_val", 3.14);
		msg.set("double_val", 2.718281828);
		msg.set("bool_val", true);
		msg.set("string_val", "test string");
		msg.set("bytes_val", "AQID");
		protojson.marshal(msg);
	`)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(v.String()), &parsed))
	assert.Equal(t, float64(-42), parsed["int32Val"])
	assert.Equal(t, true, parsed["boolVal"])
	assert.Equal(t, "test string", parsed["stringVal"])
}

func TestMarshal_EnumValue(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var MT = pb.messageType("test.AllTypes");
		var msg = new MT();
		msg.set("enum_val", 1);
		protojson.marshal(msg);
	`)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(v.String()), &parsed))
	assert.Equal(t, "FIRST", parsed["enumVal"])
}

func TestMarshal_NestedMessage(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var Outer = pb.messageType("test.NestedOuter");
		var Inner = pb.messageType("test.NestedInner");
		var inner = new Inner();
		inner.set("value", 77);
		var msg = new Outer();
		msg.set("nested_inner", inner);
		msg.set("name", "outer");
		protojson.marshal(msg);
	`)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(v.String()), &parsed))
	assert.Equal(t, "outer", parsed["name"])
	inner := parsed["nestedInner"].(map[string]interface{})
	assert.Equal(t, float64(77), inner["value"])
}

func TestMarshal_RepeatedFields(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var MT = pb.messageType("test.RepeatedMessage");
		var msg = new MT();
		msg.set("items", ["a", "b", "c"]);
		msg.set("numbers", [1, 2, 3]);
		protojson.marshal(msg);
	`)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(v.String()), &parsed))
	items := parsed["items"].([]interface{})
	assert.Equal(t, []interface{}{"a", "b", "c"}, items)
}

func TestMarshal_MapFields(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var MT = pb.messageType("test.MapMessage");
		var msg = new MT();
		msg.set("tags", {foo: "bar"});
		msg.set("counts", {x: 10});
		protojson.marshal(msg);
	`)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(v.String()), &parsed))
	tags := parsed["tags"].(map[string]interface{})
	assert.Equal(t, "bar", tags["foo"])
	counts := parsed["counts"].(map[string]interface{})
	assert.Equal(t, float64(10), counts["x"])
}

func TestMarshal_OneofField(t *testing.T) {
	env := newTestEnv(t)
	t.Run("string choice", func(t *testing.T) {
		v := env.run(`
			var MT = pb.messageType("test.OneofMessage");
			var msg = new MT();
			msg.set("str_choice", "pick me");
			protojson.marshal(msg);
		`)
		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(v.String()), &parsed))
		assert.Equal(t, "pick me", parsed["strChoice"])
		_, hasInt := parsed["intChoice"]
		assert.False(t, hasInt)
	})
	t.Run("int choice", func(t *testing.T) {
		v := env.run(`
			var MT = pb.messageType("test.OneofMessage");
			var msg = new MT();
			msg.set("int_choice", 99);
			protojson.marshal(msg);
		`)
		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(v.String()), &parsed))
		assert.Equal(t, float64(99), parsed["intChoice"])
		_, hasStr := parsed["strChoice"]
		assert.False(t, hasStr)
	})
}

func TestMarshal_EmptyMessage(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var MT = pb.messageType("test.SimpleMessage");
		protojson.marshal(new MT());
	`)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(v.String()), &parsed))
	_, hasName := parsed["name"]
	assert.False(t, hasName)
}

func TestUnmarshal_SimpleMessage(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var msg = protojson.unmarshal("test.SimpleMessage", '{"name":"world","value":99}');
		msg.get("name") + ":" + msg.get("value");
	`)
	assert.Equal(t, "world:99", v.String())
}

func TestUnmarshal_AllScalarTypes(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var msg = protojson.unmarshal("test.AllTypes", JSON.stringify({
			int32Val: -42, boolVal: true, stringVal: "hello", uint32Val: 100,
		}));
		[msg.get("int32_val"), msg.get("bool_val"), msg.get("string_val"), msg.get("uint32_val")].join(",");
	`)
	assert.Equal(t, "-42,true,hello,100", v.String())
}

func TestUnmarshal_EnumByName(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var msg = protojson.unmarshal("test.AllTypes", '{"enumVal":"SECOND"}');
		msg.get("enum_val");
	`)
	assert.Equal(t, int64(2), v.ToInteger())
}

func TestUnmarshal_EnumByNumber(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var msg = protojson.unmarshal("test.AllTypes", '{"enumVal":3}');
		msg.get("enum_val");
	`)
	assert.Equal(t, int64(3), v.ToInteger())
}

func TestUnmarshal_NestedMessage(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var msg = protojson.unmarshal("test.NestedOuter", '{"nestedInner":{"value":55},"name":"test"}');
		msg.get("nested_inner").get("value") + ":" + msg.get("name");
	`)
	assert.Equal(t, "55:test", v.String())
}

func TestUnmarshal_RepeatedFields(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var msg = protojson.unmarshal("test.RepeatedMessage", '{"items":["x","y"],"numbers":[10,20]}');
		msg.get("items").length;
	`)
	assert.Equal(t, int64(2), v.ToInteger())
}

func TestUnmarshal_MapFields(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var msg = protojson.unmarshal("test.MapMessage", '{"tags":{"a":"b"},"counts":{"k":5}}');
		msg.get("tags").get("a");
	`)
	assert.Equal(t, "b", v.String())
}

func TestUnmarshal_Oneof(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var msg = protojson.unmarshal("test.OneofMessage", '{"strChoice":"chosen"}');
		msg.get("str_choice") + ":" + msg.whichOneof("choice");
	`)
	assert.Equal(t, "chosen:str_choice", v.String())
}

func TestUnmarshal_DiscardUnknown(t *testing.T) {
	env := newTestEnv(t)
	env.mustFail(`protojson.unmarshal("test.SimpleMessage", '{"name":"ok","bogusField":true}');`)
	v := env.run(`
		protojson.unmarshal("test.SimpleMessage", '{"name":"ok","bogusField":true}', {discardUnknown: true});
		"passed";
	`)
	assert.Equal(t, "passed", v.String())
}

func TestUnmarshal_MalformedJSON(t *testing.T) {
	env := newTestEnv(t)
	env.mustFail(`protojson.unmarshal("test.SimpleMessage", "not json at all");`)
}

func TestUnmarshal_EmptyJSON(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var msg = protojson.unmarshal("test.SimpleMessage", '{}');
		msg.get("name");
	`)
	assert.Equal(t, "", v.String())
}

func TestMarshalOption_EmitDefaults(t *testing.T) {
	env := newTestEnv(t)
	t.Run("without", func(t *testing.T) {
		v := env.run(`
			var MT = pb.messageType("test.SimpleMessage");
			protojson.marshal(new MT());
		`)
		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(v.String()), &parsed))
		_, hasName := parsed["name"]
		assert.False(t, hasName)
	})
	t.Run("with", func(t *testing.T) {
		v := env.run(`
			var MT = pb.messageType("test.SimpleMessage");
			protojson.marshal(new MT(), {emitDefaults: true});
		`)
		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(v.String()), &parsed))
		assert.Equal(t, "", parsed["name"])
		assert.Equal(t, float64(0), parsed["value"])
	})
}

func TestMarshalOption_EnumAsNumber(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var MT = pb.messageType("test.AllTypes");
		var msg = new MT();
		msg.set("enum_val", 2);
		protojson.marshal(msg, {enumAsNumber: true});
	`)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(v.String()), &parsed))
	assert.Equal(t, float64(2), parsed["enumVal"])
}

func TestMarshalOption_UseProtoNames(t *testing.T) {
	env := newTestEnv(t)
	t.Run("camelCase", func(t *testing.T) {
		v := env.run(`
			var MT = pb.messageType("test.AllTypes");
			var msg = new MT();
			msg.set("int32_val", 1);
			protojson.marshal(msg);
		`)
		assert.Contains(t, v.String(), "int32Val")
		assert.NotContains(t, v.String(), "int32_val")
	})
	t.Run("proto names", func(t *testing.T) {
		v := env.run(`
			var MT = pb.messageType("test.AllTypes");
			var msg = new MT();
			msg.set("int32_val", 1);
			protojson.marshal(msg, {useProtoNames: true});
		`)
		assert.Contains(t, v.String(), "int32_val")
	})
}

func TestMarshalOption_Indent(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var MT = pb.messageType("test.SimpleMessage");
		var msg = new MT();
		msg.set("name", "x");
		protojson.marshal(msg, {indent: "    "});
	`)
	s := v.String()
	assert.Contains(t, s, "\n")
	assert.Contains(t, s, "    ")
}

func TestMarshalOption_Combined(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var MT = pb.messageType("test.AllTypes");
		var msg = new MT();
		msg.set("enum_val", 1);
		protojson.marshal(msg, {emitDefaults: true, enumAsNumber: true, useProtoNames: true});
	`)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(v.String()), &parsed))
	assert.Equal(t, float64(1), parsed["enum_val"])
	assert.Equal(t, float64(0), parsed["int32_val"])
}

func TestFormat_ProducesMultiline(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var MT = pb.messageType("test.SimpleMessage");
		var msg = new MT();
		msg.set("name", "formatted");
		msg.set("value", 1);
		protojson.format(msg);
	`)
	s := v.String()
	lines := strings.Split(s, "\n")
	assert.Greater(t, len(lines), 1)
	assert.Contains(t, s, "  ")
}

func TestMarshal_InvalidArgument(t *testing.T) {
	env := newTestEnv(t)
	env.mustFail(`protojson.marshal("not a message");`)
	env.mustFail(`protojson.marshal(undefined);`)
	env.mustFail(`protojson.marshal(null);`)
	env.mustFail(`protojson.marshal(42);`)
}

func TestFormat_InvalidArgument(t *testing.T) {
	env := newTestEnv(t)
	env.mustFail(`protojson.format("not a message");`)
	env.mustFail(`protojson.format(undefined);`)
	env.mustFail(`protojson.format(null);`)
}

func TestUnmarshal_MissingTypeName(t *testing.T) {
	env := newTestEnv(t)
	env.mustFail(`protojson.unmarshal(undefined, '{}');`)
	env.mustFail(`protojson.unmarshal(null, '{}');`)
}

func TestUnmarshal_MissingJSONString(t *testing.T) {
	env := newTestEnv(t)
	env.mustFail(`protojson.unmarshal("test.SimpleMessage", undefined);`)
	env.mustFail(`protojson.unmarshal("test.SimpleMessage", null);`)
}

func TestUnmarshal_UnknownType(t *testing.T) {
	env := newTestEnv(t)
	err := env.mustFail(`protojson.unmarshal("test.NoSuchType", '{}');`)
	assert.Contains(t, err.Error(), "unknown type")
}

func TestUnmarshal_NotAMessageType(t *testing.T) {
	env := newTestEnv(t)
	err := env.mustFail(`protojson.unmarshal("test.TestEnum", '{}');`)
	assert.Contains(t, err.Error(), "not a message type")
}

func TestNew_NilRuntime(t *testing.T) {
	assert.Panics(t, func() { _, _ = gojaprotojson.New(nil) })
}

func TestNew_MissingProtobuf(t *testing.T) {
	rt := goja.New()
	_, err := gojaprotojson.New(rt)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "protobuf module is required")
}

func TestWithProtobuf_Nil(t *testing.T) {
	rt := goja.New()
	_, err := gojaprotojson.New(rt, gojaprotojson.WithProtobuf(nil))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not be nil")
}

func TestRequire(t *testing.T) {
	rt := goja.New()
	pb, err := gojaprotobuf.New(rt)
	require.NoError(t, err)
	_, err = pb.LoadDescriptorSetBytes(testDescriptorSetBytes())
	require.NoError(t, err)
	module := rt.NewObject()
	exports := rt.NewObject()
	_ = module.Set("exports", exports)
	loader := gojaprotojson.Require(gojaprotojson.WithProtobuf(pb))
	loader(rt, module)
	require.NoError(t, rt.Set("protojson", module.Get("exports")))
	pbObj := rt.NewObject()
	pb.SetupExports(pbObj)
	require.NoError(t, rt.Set("pb", pbObj))
	v, err := rt.RunString(`
		var MT = pb.messageType("test.SimpleMessage");
		var msg = new MT();
		msg.set("name", "require test");
		protojson.marshal(msg);
	`)
	require.NoError(t, err)
	assert.Contains(t, v.String(), "require test")
}

func TestRequire_PanicsOnBadOptions(t *testing.T) {
	rt := goja.New()
	module := rt.NewObject()
	exports := rt.NewObject()
	_ = module.Set("exports", exports)
	loader := gojaprotojson.Require()
	assert.Panics(t, func() { loader(rt, module) })
}

func TestRoundTrip_MarshalUnmarshal(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var MT = pb.messageType("test.AllTypes");
		var msg = new MT();
		msg.set("int32_val", 42);
		msg.set("string_val", "hello");
		msg.set("bool_val", true);
		msg.set("enum_val", 2);
		msg.set("repeated_int32", [1, 2, 3]);
		msg.set("repeated_string", ["a", "b"]);
		var jsonStr = protojson.marshal(msg);
		var restored = protojson.unmarshal("test.AllTypes", jsonStr);
		[restored.get("int32_val"), restored.get("string_val"),
		 restored.get("bool_val"), restored.get("enum_val")].join(",");
	`)
	assert.Equal(t, "42,hello,true,2", v.String())
}

func TestRoundTrip_MapMessage(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var MT = pb.messageType("test.MapMessage");
		var msg = new MT();
		msg.set("tags", {k1: "v1", k2: "v2"});
		msg.set("counts", {a: 1, b: 2});
		var jsonStr = protojson.marshal(msg);
		var restored = protojson.unmarshal("test.MapMessage", jsonStr);
		restored.get("tags").get("k1") + ":" + restored.get("counts").get("b");
	`)
	assert.Equal(t, "v1:2", v.String())
}

func TestMarshal_NilOptions(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var MT = pb.messageType("test.SimpleMessage");
		var msg = new MT();
		msg.set("name", "test");
		protojson.marshal(msg, undefined);
	`)
	assert.Contains(t, v.String(), "test")
}

// TestMarshal_MarshalError covers the protojson.Marshal error path (marshal.go:19-20).
// We create a google.protobuf.Timestamp with out-of-range seconds, which causes
// protojson.Marshal to fail its well-known type validation.
func TestMarshal_MarshalError(t *testing.T) {
	env := newTestEnv(t)

	// Create a Timestamp with seconds way out of range (valid range is [-62135596800, 253402300799]).
	tsDesc := timestamppb.File_google_protobuf_timestamp_proto.Messages().ByName("Timestamp")
	msg := dynamicpb.NewMessage(tsDesc)
	msg.Set(tsDesc.Fields().ByName("seconds"), protoreflect.ValueOfInt64(999999999999999))
	msg.Set(tsDesc.Fields().ByName("nanos"), protoreflect.ValueOfInt32(0))

	// Wrap and inject into JS.
	wrapped := env.pb.WrapMessage(msg)
	require.NoError(t, env.rt.Set("badTimestamp", wrapped))

	// Attempting to marshal should trigger protojson.Marshal error.
	err := env.mustFail(`protojson.marshal(badTimestamp);`)
	assert.Contains(t, err.Error(), "marshal:")
}

// TestFormat_MarshalError covers the protojson.Format error path (marshal.go:40-41).
func TestFormat_MarshalError(t *testing.T) {
	env := newTestEnv(t)

	// Same approach: invalid Timestamp.
	tsDesc := timestamppb.File_google_protobuf_timestamp_proto.Messages().ByName("Timestamp")
	msg := dynamicpb.NewMessage(tsDesc)
	msg.Set(tsDesc.Fields().ByName("seconds"), protoreflect.ValueOfInt64(999999999999999))
	msg.Set(tsDesc.Fields().ByName("nanos"), protoreflect.ValueOfInt32(0))

	wrapped := env.pb.WrapMessage(msg)
	require.NoError(t, env.rt.Set("badTimestamp", wrapped))

	err := env.mustFail(`protojson.format(badTimestamp);`)
	assert.Contains(t, err.Error(), "format:")
}
