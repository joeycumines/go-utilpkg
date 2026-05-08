package gojaprotobuf

import (
	"math"
	"strings"
	"testing"

	"github.com/joeycumines/goja"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Additional setMapFromGoja: JS Map key error with maps iterated via
// the entries() protocol
// ---------------------------------------------------------------------------

func TestSetMapFromGoja_JSMapKeyErrorViaEntries(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	md, err := env.m.findMessageDescriptor("mapkeys.MultiKeyMap")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("uint32_map")

	mapVal, err := env.rt.RunString(`
		var m = new Map();
		m.set(-1, 'neg');
		m
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = env.m.setMapFromGoja(msg, fd, mapVal)
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), "map field") {
		t.Errorf("error %q should contain %q", err.Error(), "map field")
	}
}

// ---------------------------------------------------------------------------
// Additional setMapFromGoja: JS Map value error through entries()
// ---------------------------------------------------------------------------

func TestSetMapFromGoja_JSMapValueErrorViaEntries(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	md, err := env.m.findMessageDescriptor("test.MapMessage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("counts")

	mapVal, err := env.rt.RunString(`
		var m = new Map();
		m.set('k', 9999999999);
		m
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = env.m.setMapFromGoja(msg, fd, mapVal)
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), "map field") {
		t.Errorf("error %q should contain %q", err.Error(), "map field")
	}
}

// ---------------------------------------------------------------------------
// Descriptor loading: file with dependencies resolved from local
// ---------------------------------------------------------------------------

func TestLoadDescriptorSet_WithDependencyResolution(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = m.loadDescriptorSetBytes(testDescriptorSetBytes())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	depFdp := containerMessageProto()
	data, err := proto.Marshal(depFdp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	names, err := m.loadFileDescriptorProtoBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sliceContains(names, "container.Container") {
		t.Errorf("expected names to contain %q, got %v", "container.Container", names)
	}
}

// ---------------------------------------------------------------------------
// BUG-1 fix: repeated field add/set with null/undefined throws TypeError
// ---------------------------------------------------------------------------

func TestRepeatedField_AddNull_Throws(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var RM = pb.messageType('test.RepeatedMessage');
		var msg = new RM();
	`)
	env.mustFail(t, `msg.get('items').add(null)`)
	env.mustFail(t, `msg.get('items').add(undefined)`)
}

func TestRepeatedField_SetNull_Throws(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var RM = pb.messageType('test.RepeatedMessage');
		var msg = new RM();
		msg.set('items', ['a']);
	`)
	env.mustFail(t, `msg.get('items').set(0, null)`)
	env.mustFail(t, `msg.get('items').set(0, undefined)`)
}

// ---------------------------------------------------------------------------
// BUG-2 fix: map key int32/uint32 overflow check
// ---------------------------------------------------------------------------

func TestMapKeyInt32Overflow(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	md, err := env.m.findMessageDescriptor("mapkeys.MultiKeyMap")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fd := md.Fields().ByName("int32_map")

	// Int32 overflow.
	bigVal := env.rt.ToValue(int64(math.MaxInt32) + 1)
	_, err = env.m.gojaToProtoMapKey(bigVal, fd.MapKey())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "overflows int32") {
		t.Fatalf("error %q should contain %q", err.Error(), "overflows int32")
	}

	// Int32 underflow.
	smallVal := env.rt.ToValue(int64(math.MinInt32) - 1)
	_, err = env.m.gojaToProtoMapKey(smallVal, fd.MapKey())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "overflows int32") {
		t.Fatalf("error %q should contain %q", err.Error(), "overflows int32")
	}
}

func TestMapKeyUint32Overflow(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	md, err := env.m.findMessageDescriptor("mapkeys.MultiKeyMap")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msg := dynamicpb.NewMessage(md)
	_ = msg // suppress unused
	fd := md.Fields().ByName("uint32_map")

	// Uint32 overflow.
	bigVal := env.rt.ToValue(int64(math.MaxUint32) + 1)
	_, err = env.m.gojaToProtoMapKey(bigVal, fd.MapKey())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "overflows uint32") {
		t.Fatalf("error %q should contain %q", err.Error(), "overflows uint32")
	}
}

// ---------------------------------------------------------------------------
// CONCERN-2 fix: map set(key, null) deletes the entry
// ---------------------------------------------------------------------------

func TestMapField_SetNullDeletesEntry(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(t, `
		var MM = pb.messageType('test.MapMessage');
		var msg = new MM();
		msg.get('tags').set('keep', 'yes');
		msg.get('tags').set('del', 'no');
		msg.get('tags').set('del', null);
		msg.get('tags').size
	`)
	if got := v.ToInteger(); got != int64(1) {
		t.Errorf("got %d, want %d", got, 1)
	}

	v = env.run(t, `msg.get('tags').get('del')`)
	if !goja.IsUndefined(v) {
		t.Errorf("expected undefined, got %v", v)
	}

	v = env.run(t, `msg.get('tags').get('keep')`)
	if got := v.String(); got != "yes" {
		t.Errorf("got %q, want %q", got, "yes")
	}
}

// ---------------------------------------------------------------------------
// Enum int32 overflow check
// ---------------------------------------------------------------------------

func TestGojaToProtoEnum_Int32Overflow(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.AllTypes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fd := md.Fields().ByName("enum_val")

	// Overflow positive.
	bigVal := env.rt.ToValue(int64(math.MaxInt32) + 1)
	_, err = env.m.gojaToProtoEnum(bigVal, fd)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "overflows int32") {
		t.Fatalf("error %q should contain %q", err.Error(), "overflows int32")
	}

	// Overflow negative.
	smallVal := env.rt.ToValue(int64(math.MinInt32) - 1)
	_, err = env.m.gojaToProtoEnum(smallVal, fd)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "overflows int32") {
		t.Fatalf("error %q should contain %q", err.Error(), "overflows int32")
	}
}

// ---------------------------------------------------------------------------
// Bool map key from plain object: "false" must not be truthy
// ---------------------------------------------------------------------------

func TestBoolMapKey_PlainObject_FalseString(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	v := env.run(t, `
		var MK = pb.messageType('mapkeys.MultiKeyMap');
		var msg = new MK();
		msg.set('bool_map', {"true": "yes", "false": "no"});
		var m = msg.get('bool_map');
		m.size
	`)
	if got := v.ToInteger(); got != int64(2) {
		t.Errorf("got %d, want %d", got, 2)
	}

	v = env.run(t, `m.get(true)`)
	if got := v.String(); got != "yes" {
		t.Errorf("got %q, want %q", got, "yes")
	}

	v = env.run(t, `m.get(false)`)
	if got := v.String(); got != "no" {
		t.Errorf("got %q, want %q", got, "no")
	}
}

func TestBoolMapKey_JSMap(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	v := env.run(t, `
		var MK = pb.messageType('mapkeys.MultiKeyMap');
		var msg = new MK();
		var bm = new Map();
		bm.set(true, "yes");
		bm.set(false, "no");
		msg.set('bool_map', bm);
		msg.get('bool_map').size
	`)
	if got := v.ToInteger(); got != int64(2) {
		t.Errorf("got %d, want %d", got, 2)
	}

	v = env.run(t, `msg.get('bool_map').get(true)`)
	if got := v.String(); got != "yes" {
		t.Errorf("got %q, want %q", got, "yes")
	}

	v = env.run(t, `msg.get('bool_map').get(false)`)
	if got := v.String(); got != "no" {
		t.Errorf("got %q, want %q", got, "no")
	}
}

// ---------------------------------------------------------------------------
// Message type mismatch: setting field with wrong message type
// ---------------------------------------------------------------------------

func TestGojaToProtoMessage_TypeMismatch(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var Outer = pb.messageType('test.NestedOuter');
		var Simple = pb.messageType('test.SimpleMessage');
		var outer = new Outer();
		var simple = new Simple();
		simple.set('name', 'oops');
	`)
	env.mustFail(t, `outer.set('nested_inner', simple)`)
}

func TestGojaToProtoMessage_TypeMatch(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var Outer = pb.messageType('test.NestedOuter');
		var Inner = pb.messageType('test.NestedInner');
		var outer = new Outer();
		var inner = new Inner();
		inner.set('value', 42);
		outer.set('nested_inner', inner);
	`)
	v := env.run(t, `outer.get('nested_inner').get('value')`)
	if got := v.ToInteger(); got != int64(42) {
		t.Errorf("got %d, want %d", got, 42)
	}
}

// ---------------------------------------------------------------------------
// Batch 5: Targeted coverage for remaining gaps
// ---------------------------------------------------------------------------

func TestGojaToProtoValue_NullForMessageField(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.NestedOuter")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fd := md.Fields().ByName("nested_inner")

	// nil → error
	_, err = env.m.gojaToProtoValue(nil, fd)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "null value for message field") {
		t.Fatalf("error %q should contain %q", err.Error(), "null value for message field")
	}

	// goja.Null() → error
	_, err = env.m.gojaToProtoValue(goja.Null(), fd)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "null value for message field") {
		t.Fatalf("error %q should contain %q", err.Error(), "null value for message field")
	}

	// goja.Undefined() → error
	_, err = env.m.gojaToProtoValue(goja.Undefined(), fd)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "null value for message field") {
		t.Fatalf("error %q should contain %q", err.Error(), "null value for message field")
	}
}

func TestGojaToProtoValue_Int32BigIntOverflow(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.AllTypes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fd := md.Fields().ByName("int32_val")

	bigVal, err := env.rt.RunString(`BigInt('9223372036854775808')`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = env.m.gojaToProtoValue(bigVal, fd)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "overflows int64") {
		t.Fatalf("error %q should contain %q", err.Error(), "overflows int64")
	}
}

func TestGojaToProtoEnum_BigIntOverflow(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.AllTypes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fd := md.Fields().ByName("enum_val")

	bigVal, err := env.rt.RunString(`BigInt('9223372036854775808')`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = env.m.gojaToProtoEnum(bigVal, fd)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "overflows int64") {
		t.Fatalf("error %q should contain %q", err.Error(), "overflows int64")
	}
}

func TestSetMapFromGoja_InvalidValueRejectsAtomically(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.MapMessage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fd := md.Fields().ByName("tags")
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "Map null",
			input: `new Map([
				["accepted", "staged"],
				["rejected", null],
			])`,
		},
		{
			name: "Map undefined",
			input: `new Map([
				["accepted", "staged"],
				["rejected", undefined],
			])`,
		},
		{
			name:  "object null",
			input: `({accepted: "staged", rejected: null})`,
		},
		{
			name:  "object undefined",
			input: `({accepted: "staged", rejected: undefined})`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			msg := dynamicpb.NewMessage(md)
			msg.Mutable(fd).Map().Set(
				protoreflect.ValueOfString("preserved").MapKey(),
				protoreflect.ValueOfString("original"),
			)
			input, err := env.rt.RunString(test.input)
			if err != nil {
				t.Fatal(err)
			}
			err = env.m.setMapFromGoja(msg, fd, input)
			if err == nil {
				t.Fatal("invalid map value was accepted")
			}
			if !strings.Contains(
				err.Error(),
				"null/undefined values are not allowed",
			) {
				t.Fatalf("error = %q, want invalid-value rejection", err)
			}
			protoMap := msg.Get(fd).Map()
			if got := protoMap.Len(); got != 1 {
				t.Fatalf("failed replacement length = %d, want 1", got)
			}
			if got := protoMap.Get(
				protoreflect.ValueOfString("preserved").MapKey(),
			).String(); got != "original" {
				t.Fatalf(
					"failed replacement value = %q, want original",
					got,
				)
			}
		})
	}
}

func TestJsLoadDescriptorSet_ExtractBytesError(t *testing.T) {
	env := newTestEnv(t)
	env.mustFail(t, `pb.loadDescriptorSet(42)`)
}

func TestJsLoadFileDescriptorProto_ExtractBytesError(t *testing.T) {
	env := newTestEnv(t)
	env.mustFail(t, `pb.loadFileDescriptorProto(42)`)
}

// ---------------------------------------------------------------------------
// Batch 6: Mock FieldDescriptor to test default branches in type switches.
// ---------------------------------------------------------------------------

// mockFieldDesc is a minimal mock for [protoreflect.FieldDescriptor] that
// returns a custom [protoreflect.Kind]. All other methods delegate to the
// embedded interface and will panic if called — only Kind() is overridden.
type mockFieldDesc struct {
	protoreflect.FieldDescriptor
	kind protoreflect.Kind
}

func (f *mockFieldDesc) Kind() protoreflect.Kind { return f.kind }

func TestProtoValueToGoja_DefaultBranch(t *testing.T) {
	env := newTestEnv(t)
	fd := &mockFieldDesc{kind: protoreflect.Kind(99)}
	result := env.m.protoValueToGoja(protoreflect.ValueOfString("x"), fd)
	if !goja.IsUndefined(result) {
		t.Errorf("expected undefined, got %v", result)
	}
}

func TestGojaToProtoValue_DefaultBranch(t *testing.T) {
	env := newTestEnv(t)
	fd := &mockFieldDesc{kind: protoreflect.Kind(99)}
	_, err := env.m.gojaToProtoValue(env.rt.ToValue("test"), fd)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported field kind") {
		t.Fatalf("error %q should contain %q", err.Error(), "unsupported field kind")
	}
}

func TestGojaToProtoMapKey_DefaultBranch(t *testing.T) {
	env := newTestEnv(t)
	fd := &mockFieldDesc{kind: protoreflect.Kind(99)}
	_, err := env.m.gojaToProtoMapKey(env.rt.ToValue("test"), fd)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported map key kind") {
		t.Fatalf("error %q should contain %q", err.Error(), "unsupported map key kind")
	}
}

func TestMapKeyToGoja_DefaultBranch(t *testing.T) {
	env := newTestEnv(t)
	fd := &mockFieldDesc{kind: protoreflect.Kind(99)}
	mk := protoreflect.ValueOfString("test").MapKey()
	result := env.m.mapKeyToGoja(mk, fd)
	if got := result.String(); got != "test" {
		t.Errorf("got %q, want %q", got, "test")
	}
}

// ---------------------------------------------------------------------------
// Batch 7: Trigger protojson.Marshal error via well-known type validation.
// ---------------------------------------------------------------------------

func TestJsToJSON_ProtojsonMarshalError(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msg := &timestamppb.Timestamp{Seconds: 999999999999}
	wrapped := m.wrapMessage(msg)
	pb := rt.NewObject()
	m.setupExports(pb)
	if err := rt.Set("pb", pb); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := rt.Set("msg", wrapped); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = rt.RunString("pb.toJSON(msg)")
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), "seconds") {
		t.Errorf("error %q should contain %q", err.Error(), "seconds")
	}
}
