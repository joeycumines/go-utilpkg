package gojaprotojson_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dop251/goja"
	gojaprotobuf "github.com/joeycumines/goja-protobuf"
	gojaprotojson "github.com/joeycumines/goja-protojson"
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
	var parsed map[string]any
	if err := json.Unmarshal([]byte(v.String()), &parsed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed["name"] != "hello" {
		t.Errorf("got %v, want %v", parsed["name"], "hello")
	}
	if parsed["value"] != float64(42) {
		t.Errorf("got %v, want %v", parsed["value"], float64(42))
	}
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
	var parsed map[string]any
	if err := json.Unmarshal([]byte(v.String()), &parsed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed["int32Val"] != float64(-42) {
		t.Errorf("got %v, want %v", parsed["int32Val"], float64(-42))
	}
	if parsed["boolVal"] != true {
		t.Errorf("got %v, want %v", parsed["boolVal"], true)
	}
	if parsed["stringVal"] != "test string" {
		t.Errorf("got %v, want %v", parsed["stringVal"], "test string")
	}
}

func TestMarshal_EnumValue(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var MT = pb.messageType("test.AllTypes");
		var msg = new MT();
		msg.set("enum_val", 1);
		protojson.marshal(msg);
	`)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(v.String()), &parsed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed["enumVal"] != "FIRST" {
		t.Errorf("got %v, want %v", parsed["enumVal"], "FIRST")
	}
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
	var parsed map[string]any
	if err := json.Unmarshal([]byte(v.String()), &parsed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed["name"] != "outer" {
		t.Errorf("got %v, want %v", parsed["name"], "outer")
	}
	inner := parsed["nestedInner"].(map[string]any)
	if inner["value"] != float64(77) {
		t.Errorf("got %v, want %v", inner["value"], float64(77))
	}
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
	var parsed map[string]any
	if err := json.Unmarshal([]byte(v.String()), &parsed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	items := parsed["items"].([]any)
	if len(items) != 3 {
		t.Errorf("expected length %d, got %d", 3, len(items))
	} else if items[0] != "a" || items[1] != "b" || items[2] != "c" {
		t.Errorf("got %v, want %v", items, []any{"a", "b", "c"})
	}
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
	var parsed map[string]any
	if err := json.Unmarshal([]byte(v.String()), &parsed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tags := parsed["tags"].(map[string]any)
	if tags["foo"] != "bar" {
		t.Errorf("got %v, want %v", tags["foo"], "bar")
	}
	counts := parsed["counts"].(map[string]any)
	if counts["x"] != float64(10) {
		t.Errorf("got %v, want %v", counts["x"], float64(10))
	}
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
		var parsed map[string]any
		if err := json.Unmarshal([]byte(v.String()), &parsed); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if parsed["strChoice"] != "pick me" {
			t.Errorf("got %v, want %v", parsed["strChoice"], "pick me")
		}
		_, hasInt := parsed["intChoice"]
		if hasInt {
			t.Error("expected false")
		}
	})
	t.Run("int choice", func(t *testing.T) {
		v := env.run(`
			var MT = pb.messageType("test.OneofMessage");
			var msg = new MT();
			msg.set("int_choice", 99);
			protojson.marshal(msg);
		`)
		var parsed map[string]any
		if err := json.Unmarshal([]byte(v.String()), &parsed); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if parsed["intChoice"] != float64(99) {
			t.Errorf("got %v, want %v", parsed["intChoice"], float64(99))
		}
		_, hasStr := parsed["strChoice"]
		if hasStr {
			t.Error("expected false")
		}
	})
}

func TestMarshal_EmptyMessage(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var MT = pb.messageType("test.SimpleMessage");
		protojson.marshal(new MT());
	`)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(v.String()), &parsed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, hasName := parsed["name"]
	if hasName {
		t.Error("expected false")
	}
}

func TestUnmarshal_SimpleMessage(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var msg = protojson.unmarshal("test.SimpleMessage", '{"name":"world","value":99}');
		msg.get("name") + ":" + msg.get("value");
	`)
	if v.String() != "world:99" {
		t.Errorf("got %v, want %v", v.String(), "world:99")
	}
}

func TestUnmarshal_AllScalarTypes(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var msg = protojson.unmarshal("test.AllTypes", JSON.stringify({
			int32Val: -42, boolVal: true, stringVal: "hello", uint32Val: 100,
		}));
		[msg.get("int32_val"), msg.get("bool_val"), msg.get("string_val"), msg.get("uint32_val")].join(",");
	`)
	if v.String() != "-42,true,hello,100" {
		t.Errorf("got %v, want %v", v.String(), "-42,true,hello,100")
	}
}

func TestUnmarshal_EnumByName(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var msg = protojson.unmarshal("test.AllTypes", '{"enumVal":"SECOND"}');
		msg.get("enum_val");
	`)
	if v.ToInteger() != int64(2) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(2))
	}
}

func TestUnmarshal_EnumByNumber(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var msg = protojson.unmarshal("test.AllTypes", '{"enumVal":3}');
		msg.get("enum_val");
	`)
	if v.ToInteger() != int64(3) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(3))
	}
}

func TestUnmarshal_NestedMessage(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var msg = protojson.unmarshal("test.NestedOuter", '{"nestedInner":{"value":55},"name":"test"}');
		msg.get("nested_inner").get("value") + ":" + msg.get("name");
	`)
	if v.String() != "55:test" {
		t.Errorf("got %v, want %v", v.String(), "55:test")
	}
}

func TestUnmarshal_RepeatedFields(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var msg = protojson.unmarshal("test.RepeatedMessage", '{"items":["x","y"],"numbers":[10,20]}');
		msg.get("items").length;
	`)
	if v.ToInteger() != int64(2) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(2))
	}
}

func TestUnmarshal_MapFields(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var msg = protojson.unmarshal("test.MapMessage", '{"tags":{"a":"b"},"counts":{"k":5}}');
		msg.get("tags").get("a");
	`)
	if v.String() != "b" {
		t.Errorf("got %v, want %v", v.String(), "b")
	}
}

func TestUnmarshal_Oneof(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var msg = protojson.unmarshal("test.OneofMessage", '{"strChoice":"chosen"}');
		msg.get("str_choice") + ":" + msg.whichOneof("choice");
	`)
	if v.String() != "chosen:str_choice" {
		t.Errorf("got %v, want %v", v.String(), "chosen:str_choice")
	}
}

func TestUnmarshal_DiscardUnknown(t *testing.T) {
	env := newTestEnv(t)
	env.mustFail(`protojson.unmarshal("test.SimpleMessage", '{"name":"ok","bogusField":true}');`)
	v := env.run(`
		protojson.unmarshal("test.SimpleMessage", '{"name":"ok","bogusField":true}', {discardUnknown: true});
		"passed";
	`)
	if v.String() != "passed" {
		t.Errorf("got %v, want %v", v.String(), "passed")
	}
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
	if v.String() != "" {
		t.Errorf("got %v, want %v", v.String(), "")
	}
}

func TestMarshalOption_EmitDefaults(t *testing.T) {
	env := newTestEnv(t)
	t.Run("without", func(t *testing.T) {
		v := env.run(`
			var MT = pb.messageType("test.SimpleMessage");
			protojson.marshal(new MT());
		`)
		var parsed map[string]any
		if err := json.Unmarshal([]byte(v.String()), &parsed); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		_, hasName := parsed["name"]
		if hasName {
			t.Error("expected false")
		}
	})
	t.Run("with", func(t *testing.T) {
		v := env.run(`
			var MT = pb.messageType("test.SimpleMessage");
			protojson.marshal(new MT(), {emitDefaults: true});
		`)
		var parsed map[string]any
		if err := json.Unmarshal([]byte(v.String()), &parsed); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if parsed["name"] != "" {
			t.Errorf("got %v, want %v", parsed["name"], "")
		}
		if parsed["value"] != float64(0) {
			t.Errorf("got %v, want %v", parsed["value"], float64(0))
		}
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
	var parsed map[string]any
	if err := json.Unmarshal([]byte(v.String()), &parsed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed["enumVal"] != float64(2) {
		t.Errorf("got %v, want %v", parsed["enumVal"], float64(2))
	}
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
		if !strings.Contains(v.String(), "int32Val") {
			t.Errorf("expected %q to contain %q", v.String(), "int32Val")
		}
		if strings.Contains(v.String(), "int32_val") {
			t.Errorf("expected %q to not contain %q", v.String(), "int32_val")
		}
	})
	t.Run("proto names", func(t *testing.T) {
		v := env.run(`
			var MT = pb.messageType("test.AllTypes");
			var msg = new MT();
			msg.set("int32_val", 1);
			protojson.marshal(msg, {useProtoNames: true});
		`)
		if !strings.Contains(v.String(), "int32_val") {
			t.Errorf("expected %q to contain %q", v.String(), "int32_val")
		}
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
	if !strings.Contains(s, "\n") {
		t.Errorf("expected %q to contain %q", s, "\n")
	}
	if !strings.Contains(s, "    ") {
		t.Errorf("expected %q to contain %q", s, "    ")
	}
}

func TestMarshalOption_Combined(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var MT = pb.messageType("test.AllTypes");
		var msg = new MT();
		msg.set("enum_val", 1);
		protojson.marshal(msg, {emitDefaults: true, enumAsNumber: true, useProtoNames: true});
	`)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(v.String()), &parsed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed["enum_val"] != float64(1) {
		t.Errorf("got %v, want %v", parsed["enum_val"], float64(1))
	}
	if parsed["int32_val"] != float64(0) {
		t.Errorf("got %v, want %v", parsed["int32_val"], float64(0))
	}
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
	if len(lines) <= 1 {
		t.Errorf("expected length > 1, got %d", len(lines))
	}
	if !strings.Contains(s, "  ") {
		t.Errorf("expected %q to contain %q", s, "  ")
	}
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
	if !strings.Contains(err.Error(), "unknown type") {
		t.Errorf("expected %q to contain %q", err.Error(), "unknown type")
	}
}

func TestUnmarshal_NotAMessageType(t *testing.T) {
	env := newTestEnv(t)
	err := env.mustFail(`protojson.unmarshal("test.TestEnum", '{}');`)
	if !strings.Contains(err.Error(), "not a message type") {
		t.Errorf("expected %q to contain %q", err.Error(), "not a message type")
	}
}

func TestNew_NilRuntime(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic")
		}
	}()
	_, _ = gojaprotojson.New(nil)
}

func TestNew_MissingProtobuf(t *testing.T) {
	rt := goja.New()
	_, err := gojaprotojson.New(rt)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "protobuf module is required") {
		t.Errorf("expected %q to contain %q", err.Error(), "protobuf module is required")
	}
}

func TestWithProtobuf_Nil(t *testing.T) {
	rt := goja.New()
	_, err := gojaprotojson.New(rt, gojaprotojson.WithProtobuf(nil))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "must not be nil") {
		t.Errorf("expected %q to contain %q", err.Error(), "must not be nil")
	}
}

func TestRequire(t *testing.T) {
	rt := goja.New()
	pb, err := gojaprotobuf.New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = pb.LoadDescriptorSetBytes(testDescriptorSetBytes())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	module := rt.NewObject()
	exports := rt.NewObject()
	_ = module.Set("exports", exports)
	loader := gojaprotojson.Require(gojaprotojson.WithProtobuf(pb))
	loader(rt, module)
	if err := rt.Set("protojson", module.Get("exports")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pbObj := rt.NewObject()
	pb.SetupExports(pbObj)
	if err := rt.Set("pb", pbObj); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	v, err := rt.RunString(`
		var MT = pb.messageType("test.SimpleMessage");
		var msg = new MT();
		msg.set("name", "require test");
		protojson.marshal(msg);
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(v.String(), "require test") {
		t.Errorf("expected %q to contain %q", v.String(), "require test")
	}
}

func TestRequire_PanicsOnBadOptions(t *testing.T) {
	rt := goja.New()
	module := rt.NewObject()
	exports := rt.NewObject()
	_ = module.Set("exports", exports)
	loader := gojaprotojson.Require()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic")
		}
	}()
	loader(rt, module)
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
	if v.String() != "42,hello,true,2" {
		t.Errorf("got %v, want %v", v.String(), "42,hello,true,2")
	}
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
	if v.String() != "v1:2" {
		t.Errorf("got %v, want %v", v.String(), "v1:2")
	}
}

func TestMarshal_NilOptions(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(`
		var MT = pb.messageType("test.SimpleMessage");
		var msg = new MT();
		msg.set("name", "test");
		protojson.marshal(msg, undefined);
	`)
	if !strings.Contains(v.String(), "test") {
		t.Errorf("expected %q to contain %q", v.String(), "test")
	}
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
	if err := env.rt.Set("badTimestamp", wrapped); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Attempting to marshal should trigger protojson.Marshal error.
	err := env.mustFail(`protojson.marshal(badTimestamp);`)
	if !strings.Contains(err.Error(), "marshal:") {
		t.Errorf("expected %q to contain %q", err.Error(), "marshal:")
	}
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
	if err := env.rt.Set("badTimestamp", wrapped); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err := env.mustFail(`protojson.format(badTimestamp);`)
	if !strings.Contains(err.Error(), "format:") {
		t.Errorf("expected %q to contain %q", err.Error(), "format:")
	}
}
