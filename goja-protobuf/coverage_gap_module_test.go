package gojaprotobuf

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/joeycumines/goja"
	gojarequire "github.com/joeycumines/goja_nodejs/require"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// module.go: New — option panic path
// ---------------------------------------------------------------------------

// errorOption is a test ModuleOption that always returns an error.
type errorOption struct{}

func (o *errorOption) applyModuleOption(*moduleConfig) error {
	return fmt.Errorf("test option error")
}

func TestNew_ErrorOption(t *testing.T) {
	rt := goja.New()
	recovered := capturePanic(t, func() {
		_, _ = New(rt, &errorOption{})
	})
	if !strings.Contains(fmt.Sprint(recovered), "test option error") {
		t.Errorf(
			"panic %q should contain %q",
			fmt.Sprint(recovered),
			"test option error",
		)
	}
}

// ---------------------------------------------------------------------------
// options.go: resolveOptions — error path
// ---------------------------------------------------------------------------

func TestResolveOptions_Error(t *testing.T) {
	_, err := resolveOptions([]ModuleOption{&errorOption{}})
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), "test option error") {
		t.Errorf("error %q should contain %q", err.Error(), "test option error")
	}
}

// ---------------------------------------------------------------------------
// register.go: Enable — error during require
// ---------------------------------------------------------------------------

func TestRequire_ErrorOption(t *testing.T) {
	recovered := capturePanic(t, func() {
		_ = Require(&errorOption{})
	})
	if !strings.Contains(fmt.Sprint(recovered), "test option error") {
		t.Fatalf("Require panic = %v", recovered)
	}
}

func TestRequireReturnsDynamicConstructionErrorAsGojaException(t *testing.T) {
	rt := goja.New()
	registry := gojarequire.NewRegistry()
	registry.RegisterNativeModule("protobuf", Require())
	registry.Enable(rt)
	if err := rt.Set("WeakMap", goja.Undefined()); err != nil {
		t.Fatal(err)
	}

	_, err := rt.RunString(`require('protobuf')`)
	if err == nil {
		t.Error("expected error")
	}
	if _, ok := errors.AsType[*goja.Exception](err); !ok {
		t.Fatalf("require error = %T %v, want *goja.Exception", err, err)
	}
	if !strings.Contains(err.Error(), "WeakMap constructor") {
		t.Fatalf("require error = %v, want WeakMap failure", err)
	}
}

func capturePanic(t *testing.T, run func()) (recovered any) {
	t.Helper()
	defer func() {
		recovered = recover()
		if recovered == nil {
			t.Fatal("expected panic")
		}
	}()
	run()
	return nil
}

// ---------------------------------------------------------------------------
// types.go: findMessageDescriptor / findEnumDescriptor — global fallback
// ---------------------------------------------------------------------------

func TestFindMessageDescriptor_GlobalFallback(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.SimpleMessage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mt := dynamicpb.NewMessageType(md)

	customResolver := new(protoregistry.Types)
	if err := customResolver.RegisterMessage(mt); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rt := goja.New()
	m, err := New(rt, WithResolver(customResolver))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := m.findMessageDescriptor("test.SimpleMessage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := result.FullName(); got != protoreflect.FullName("test.SimpleMessage") {
		t.Errorf("got %q, want %q", got, "test.SimpleMessage")
	}
}

func TestFindEnumDescriptor_GlobalFallback(t *testing.T) {
	env := newTestEnv(t)
	ed, err := env.m.findEnumDescriptor("test.TestEnum")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	et := dynamicpb.NewEnumType(ed)

	customResolver := new(protoregistry.Types)
	if err := customResolver.RegisterEnum(et); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rt := goja.New()
	m, err := New(rt, WithResolver(customResolver))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := m.findEnumDescriptor("test.TestEnum")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := result.FullName(); got != protoreflect.FullName("test.TestEnum") {
		t.Errorf("got %q, want %q", got, "test.TestEnum")
	}
}

// ---------------------------------------------------------------------------
// conversion.go: protoMessageToGoja — non-dynamic message
// ---------------------------------------------------------------------------

func TestProtoMessageToGoja_NonDynamic(t *testing.T) {
	env := newTestEnv(t)

	fdp := &descriptorpb.FileDescriptorProto{
		Name:   new("nondynamic.proto"),
		Syntax: new("proto3"),
	}
	result := env.m.protoMessageToGoja(fdp.ProtoReflect())
	if result == nil {
		t.Error("expected non-nil result")
	}

	obj := result.ToObject(env.rt)
	typeVal := obj.Get("$type")
	if !strings.Contains(typeVal.String(), "FileDescriptorProto") {
		t.Errorf("$type %q should contain %q", typeVal.String(), "FileDescriptorProto")
	}
}

// ---------------------------------------------------------------------------
// conversion.go: gojaToInt64 — BigInt overflow + default case
// ---------------------------------------------------------------------------

func TestGojaToInt64_BigIntOverflow(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)
	env.mustFail(t, `msg.set('int64_val', BigInt('9223372036854775808'))`)
}

func TestGojaToInt64_FloatDefault(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)
	env.mustFail(t, `msg.set('int64_val', 3.7)`)
}

// ---------------------------------------------------------------------------
// conversion.go: gojaToUint64 — BigInt overflow + default cases
// ---------------------------------------------------------------------------

func TestGojaToUint64_BigIntOverflow(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)
	env.mustFail(t, `msg.set('uint64_val', BigInt('18446744073709551616'))`)
}

func TestGojaToUint64_FloatPositive(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)
	env.mustFail(t, `msg.set('uint64_val', 3.7)`)
}

func TestGojaToUint64_FloatNegative(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)
	env.mustFail(t, `msg.set('uint64_val', -1.5)`)
}

// ---------------------------------------------------------------------------
// conversion.go: gojaToProtoValue — bytes error path
// ---------------------------------------------------------------------------

func TestGojaToProtoValue_BytesError(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)
	env.mustFail(t, `msg.set('bytes_val', 42)`)
}

// ---------------------------------------------------------------------------
// conversion.go: jsObjectToMessage — repeated, map, error branches
// ---------------------------------------------------------------------------

func TestJsObjectToMessage_WithRepeatedField(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.AllTypes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	arrVal, err := env.rt.RunString(`([10, 20, 30])`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	obj := env.rt.NewObject()
	if err := obj.Set("repeated_int32", arrVal); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msg, err := env.m.jsObjectToMessage(obj, md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fd := md.Fields().ByName("repeated_int32")
	if got := msg.Get(fd).List().Len(); got != 3 {
		t.Errorf("got %d, want %d", got, 3)
	}
}

func TestJsObjectToMessage_WithMapField(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.AllTypes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mapObj := env.rt.NewObject()
	if err := mapObj.Set("k1", "v1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := mapObj.Set("k2", "v2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	obj := env.rt.NewObject()
	if err := obj.Set("tags", mapObj); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msg, err := env.m.jsObjectToMessage(obj, md)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fd := md.Fields().ByName("tags")
	if got := msg.Get(fd).Map().Len(); got != 2 {
		t.Errorf("got %d, want %d", got, 2)
	}
}

func TestJsObjectToMessage_ScalarError(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.AllTypes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	obj := env.rt.NewObject()
	if err := obj.Set("bytes_val", 42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = env.m.jsObjectToMessage(obj, md)
	if err == nil {
		t.Error("expected error")
	}
}

func TestJsObjectToMessage_RepeatedError(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.AllTypes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	arrVal, err := env.rt.RunString(`([9999999999])`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	obj := env.rt.NewObject()
	if err := obj.Set("repeated_int32", arrVal); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = env.m.jsObjectToMessage(obj, md)
	if err == nil {
		t.Error("expected error")
	}
}

func TestJsObjectToMessage_MapError(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.MapMessage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	countsObj := env.rt.NewObject()
	if err := countsObj.Set("k", 9999999999); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	obj := env.rt.NewObject()
	if err := obj.Set("counts", countsObj); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = env.m.jsObjectToMessage(obj, md)
	if err == nil {
		t.Error("expected error")
	}
}

// ---------------------------------------------------------------------------
// conversion.go: setRepeatedFromGoja — no-length, sparse array
// ---------------------------------------------------------------------------

func TestSetRepeatedFromGoja_NoLength(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.RepeatedMessage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("items")

	noLength := env.rt.NewObject()
	err = env.m.setRepeatedFromGoja(msg, fd, noLength)
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), "expected array for repeated field") {
		t.Errorf("error %q should contain %q", err.Error(), "expected array for repeated field")
	}
}

func TestSetRepeatedFromGoja_InvalidElementRejectsAtomically(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.RepeatedMessage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fd := md.Fields().ByName("items")
	tests := []struct {
		name  string
		input string
	}{
		{name: "null", input: `["staged", null]`},
		{name: "undefined", input: `["staged", undefined]`},
		{name: "sparse", input: `["staged", , "later"]`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			msg := dynamicpb.NewMessage(md)
			msg.Mutable(fd).List().Append(
				protoreflect.ValueOfString("preserved"),
			)
			input, err := env.rt.RunString(test.input)
			if err != nil {
				t.Fatal(err)
			}
			err = env.m.setRepeatedFromGoja(msg, fd, input)
			if err == nil {
				t.Fatal("invalid repeated replacement was accepted")
			}
			if !strings.Contains(err.Error(), "repeated field items[1]") {
				t.Fatalf("error = %q, want element context", err)
			}
			list := msg.Get(fd).List()
			if got := list.Len(); got != 1 {
				t.Fatalf("failed replacement length = %d, want 1", got)
			}
			if got := list.Get(0).String(); got != "preserved" {
				t.Fatalf(
					"failed replacement value = %q, want preserved",
					got,
				)
			}
		})
	}
}

func TestSetRepeatedFromGoja_ConversionError(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.RepeatedMessage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("numbers")
	msg.Mutable(fd).List().Append(protoreflect.ValueOfInt32(7))

	arrVal, err := env.rt.RunString(`([8, 9999999999])`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = env.m.setRepeatedFromGoja(msg, fd, arrVal)
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), "repeated field") {
		t.Errorf("error %q should contain %q", err.Error(), "repeated field")
	}
	list := msg.Get(fd).List()
	if got := list.Len(); got != 1 {
		t.Fatalf("failed replacement length = %d, want 1", got)
	}
	if got := list.Get(0).Int(); got != 7 {
		t.Fatalf("failed replacement value = %d, want 7", got)
	}
}

// ---------------------------------------------------------------------------
// conversion.go: setMapFromGoja — JS Map with entries()
// ---------------------------------------------------------------------------

func TestSetMapFromGoja_JSMap(t *testing.T) {
	env := newTestEnv(t)

	v := env.run(t, `
		var MM = pb.messageType('test.MapMessage');
		var msg = new MM();
		var m = new Map();
		m.set('key1', 'val1');
		m.set('key2', 'val2');
		msg.set('tags', m);
		msg.get('tags').get('key1') === 'val1' && msg.get('tags').get('key2') === 'val2' && msg.get('tags').size === 2
	`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

func TestSetMapFromGoja_JSMapKeyError(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	md, err := env.m.findMessageDescriptor("mapkeys.MultiKeyMap")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("int32_map")

	mapVal, err := env.rt.RunString(`
		var m = new Map();
		m.set(BigInt('9223372036854775808'), 'v');
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

func TestSetMapFromGoja_JSMapValueError(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.MapMessage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("counts")
	msg.Mutable(fd).Map().Set(
		protoreflect.ValueOfString("preserved").MapKey(),
		protoreflect.ValueOfInt32(7),
	)

	mapVal, err := env.rt.RunString(`
		var m = new Map();
		m.set('accepted', 8);
		m.set('rejected', 9999999999);
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
	protoMap := msg.Get(fd).Map()
	if got := protoMap.Len(); got != 1 {
		t.Fatalf("failed replacement length = %d, want 1", got)
	}
	if got := protoMap.Get(
		protoreflect.ValueOfString("preserved").MapKey(),
	).Int(); got != 7 {
		t.Fatalf("failed replacement value = %d, want 7", got)
	}
}

// ---------------------------------------------------------------------------
// conversion.go: gojaToProtoMapKey — all key types
// ---------------------------------------------------------------------------

func TestGojaToProtoMapKey_AllTypes(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	md, err := env.m.findMessageDescriptor("mapkeys.MultiKeyMap")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := []struct {
		field string
		jsKey string
	}{
		{"bool_map", "true"},
		{"int32_map", "42"},
		{"int64_map", "100"},
		{"uint32_map", "200"},
		{"uint64_map", "300"},
	}
	for _, tc := range tests {
		t.Run(tc.field, func(t *testing.T) {
			fd := md.Fields().ByName(protoreflect.Name(tc.field))
			if fd == nil {
				t.Fatalf("field %s not found", tc.field)
			}
			keyDesc := fd.MapKey()

			val, err := env.rt.RunString(tc.jsKey)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			mk, err := env.m.gojaToProtoMapKey(val, keyDesc)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !mk.IsValid() {
				t.Error("expected valid map key")
			}
		})
	}
}

func TestGojaToProtoMapKey_Int32Error(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	md, err := env.m.findMessageDescriptor("mapkeys.MultiKeyMap")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fd := md.Fields().ByName("int32_map")
	keyDesc := fd.MapKey()

	val, err := env.rt.RunString(`BigInt('9223372036854775808')`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = env.m.gojaToProtoMapKey(val, keyDesc)
	if err == nil {
		t.Error("expected error")
	}
}

func TestGojaToProtoMapKey_Int64Error(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	md, err := env.m.findMessageDescriptor("mapkeys.MultiKeyMap")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fd := md.Fields().ByName("int64_map")
	keyDesc := fd.MapKey()

	val, err := env.rt.RunString(`BigInt('9223372036854775808')`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = env.m.gojaToProtoMapKey(val, keyDesc)
	if err == nil {
		t.Error("expected error")
	}
}

func TestGojaToProtoMapKey_Uint32Error(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	md, err := env.m.findMessageDescriptor("mapkeys.MultiKeyMap")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fd := md.Fields().ByName("uint32_map")
	keyDesc := fd.MapKey()

	val := env.rt.ToValue(-1)
	_, err = env.m.gojaToProtoMapKey(val, keyDesc)
	if err == nil {
		t.Error("expected error")
	}
}

func TestGojaToProtoMapKey_Uint64Error(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	md, err := env.m.findMessageDescriptor("mapkeys.MultiKeyMap")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fd := md.Fields().ByName("uint64_map")
	keyDesc := fd.MapKey()

	val := env.rt.ToValue(-1)
	_, err = env.m.gojaToProtoMapKey(val, keyDesc)
	if err == nil {
		t.Error("expected error")
	}
}

// ---------------------------------------------------------------------------
// conversion.go: mapKeyToGoja — all key types
// ---------------------------------------------------------------------------

func TestMapKeyToGoja_AllTypes(t *testing.T) {
	env := newTestEnvWithMapKeys(t)

	env.run(t, `var MKM = pb.messageType('mapkeys.MultiKeyMap')`)
	env.run(t, `var msg = new MKM()`)

	// bool key
	env.run(t, `msg.get('bool_map').set(true, 'yes')`)
	v := env.run(t, `msg.get('bool_map').get(true)`)
	if got := v.String(); got != "yes" {
		t.Errorf("got %q, want %q", got, "yes")
	}

	// int32 key
	env.run(t, `msg.get('int32_map').set(42, 'answer')`)
	v = env.run(t, `msg.get('int32_map').get(42)`)
	if got := v.String(); got != "answer" {
		t.Errorf("got %q, want %q", got, "answer")
	}

	// int64 key
	env.run(t, `msg.get('int64_map').set(100, 'hundred')`)
	v = env.run(t, `msg.get('int64_map').get(100)`)
	if got := v.String(); got != "hundred" {
		t.Errorf("got %q, want %q", got, "hundred")
	}

	// uint32 key
	env.run(t, `msg.get('uint32_map').set(200, 'twohundred')`)
	v = env.run(t, `msg.get('uint32_map').get(200)`)
	if got := v.String(); got != "twohundred" {
		t.Errorf("got %q, want %q", got, "twohundred")
	}

	// uint64 key
	env.run(t, `msg.get('uint64_map').set(300, 'threehundred')`)
	v = env.run(t, `msg.get('uint64_map').get(300)`)
	if got := v.String(); got != "threehundred" {
		t.Errorf("got %q, want %q", got, "threehundred")
	}

	// Exercise forEach on each type → triggers mapKeyToGoja.
	env.run(t, `
		var keys = [];
		msg.get('bool_map').forEach(function(v, k) { keys.push(typeof k); });
		msg.get('int32_map').forEach(function(v, k) { keys.push(typeof k); });
		msg.get('int64_map').forEach(function(v, k) { keys.push(typeof k); });
		msg.get('uint32_map').forEach(function(v, k) { keys.push(typeof k); });
		msg.get('uint64_map').forEach(function(v, k) { keys.push(typeof k); });
	`)
}

func TestMapKeyToGoja_Entries(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	env.run(t, `var MKM = pb.messageType('mapkeys.MultiKeyMap')`)
	env.run(t, `var msg = new MKM()`)
	env.run(t, `msg.get('int32_map').set(1, 'one')`)
	env.run(t, `msg.get('int32_map').set(2, 'two')`)

	v := env.run(t, `
		var iter = msg.get('int32_map').entries();
		var result = {};
		var r = iter.next();
		while (!r.done) {
			result[r.value[0]] = r.value[1];
			r = iter.next();
		}
		result[1] === 'one' && result[2] === 'two'
	`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

// ---------------------------------------------------------------------------
// conversion.go: gojaToProtoMessage — error from jsObjectToMessage
// ---------------------------------------------------------------------------

func TestGojaToProtoMessage_ObjectConversionError(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)
	env.mustFail(t, `msg.set('nested_val', {value: 9999999999})`)
}

// ---------------------------------------------------------------------------
// conversion.go: setMapFromGoja — plain object key/value errors
// ---------------------------------------------------------------------------

func TestSetMapFromGoja_PlainObjectValueError(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.MapMessage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("counts")
	msg.Mutable(fd).Map().Set(
		protoreflect.ValueOfString("preserved").MapKey(),
		protoreflect.ValueOfInt32(7),
	)

	objVal, err := env.rt.RunString(`({accepted: 8, rejected: 9999999999})`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = env.m.setMapFromGoja(msg, fd, objVal)
	if err == nil {
		t.Error("expected error")
	}
	if !strings.Contains(err.Error(), "map field") {
		t.Errorf("error %q should contain %q", err.Error(), "map field")
	}
	protoMap := msg.Get(fd).Map()
	if got := protoMap.Len(); got != 1 {
		t.Fatalf("failed replacement length = %d, want 1", got)
	}
	if got := protoMap.Get(
		protoreflect.ValueOfString("preserved").MapKey(),
	).Int(); got != 7 {
		t.Fatalf("failed replacement value = %d, want 7", got)
	}
}

// ---------------------------------------------------------------------------
// serialize.go: jsEncode — proto.Marshal error
// ---------------------------------------------------------------------------

func TestJsEncode_EncodesEmptyCorrectly(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(t, `
		var SM = pb.messageType('test.SimpleMessage');
		var msg = new SM();
		var encoded = pb.encode(msg);
		encoded.length === 0
	`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

// ---------------------------------------------------------------------------
// serialize.go: jsToJSON — protojson.Marshal error
// ---------------------------------------------------------------------------

func TestJsToJSON_EmptyMessage(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(t, `
		var SM = pb.messageType('test.SimpleMessage');
		var msg = new SM();
		var json = pb.toJSON(msg);
		typeof json === 'object' && json !== null
	`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

// ---------------------------------------------------------------------------
// serialize.go: jsFromJSON — protojson.Unmarshal error
// ---------------------------------------------------------------------------

func TestJsFromJSON_InvalidFieldValue(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var SM = pb.messageType('test.SimpleMessage')`)
	env.mustFail(t, `pb.fromJSON(SM, 42)`)
}

func TestJsFromJSON_EmptyObject(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var SM = pb.messageType('test.SimpleMessage')`)
	v := env.run(t, `
		var msg = pb.fromJSON(SM, {});
		msg.get('name') === '' && msg.get('value') === 0
	`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

// ---------------------------------------------------------------------------
// serialize.go: jsDecode — various error paths
// ---------------------------------------------------------------------------

func TestJsDecode_NonConstructorFirstArg(t *testing.T) {
	env := newTestEnv(t)
	env.mustFail(t, `pb.decode(42, new Uint8Array([]))`)
}

// ---------------------------------------------------------------------------
// Comprehensive multi-key-map integration test
// ---------------------------------------------------------------------------

func TestMultiKeyMap_RoundTrip(t *testing.T) {
	env := newTestEnvWithMapKeys(t)

	v := env.run(t, `
		var MKM = pb.messageType('mapkeys.MultiKeyMap');
		var msg = new MKM();

		msg.get('bool_map').set(true, 'yes');
		msg.get('bool_map').set(false, 'no');

		msg.get('int32_map').set(-5, 'neg');
		msg.get('int32_map').set(0, 'zero');
		msg.get('int32_map').set(42, 'pos');

		msg.get('int64_map').set(100, 'hundred');

		msg.get('uint32_map').set(200, 'twohundred');

		msg.get('uint64_map').set(300, 'threehundred');

		var encoded = pb.encode(msg);
		var decoded = pb.decode(MKM, encoded);

		decoded.get('bool_map').get(true) === 'yes' &&
		decoded.get('bool_map').get(false) === 'no' &&
		decoded.get('int32_map').get(-5) === 'neg' &&
		decoded.get('int32_map').get(0) === 'zero' &&
		decoded.get('int32_map').get(42) === 'pos' &&
		decoded.get('int64_map').get(100) === 'hundred' &&
		decoded.get('uint32_map').get(200) === 'twohundred' &&
		decoded.get('uint64_map').get(300) === 'threehundred'
	`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

func TestMultiKeyMap_Delete(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	env.run(t, `var MKM = pb.messageType('mapkeys.MultiKeyMap')`)
	env.run(t, `var msg = new MKM()`)
	env.run(t, `msg.get('bool_map').set(true, 'yes')`)
	env.run(t, `msg.get('bool_map').delete(true)`)
	v := env.run(t, `msg.get('bool_map').has(true)`)
	if v.ToBoolean() {
		t.Error("expected false")
	}
}

func TestMultiKeyMap_Has(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	env.run(t, `var MKM = pb.messageType('mapkeys.MultiKeyMap')`)
	env.run(t, `var msg = new MKM()`)
	env.run(t, `msg.get('int32_map').set(42, 'answer')`)
	v := env.run(t, `msg.get('int32_map').has(42)`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}
	v = env.run(t, `msg.get('int32_map').has(99)`)
	if v.ToBoolean() {
		t.Error("expected false")
	}
}

// ---------------------------------------------------------------------------
