package gojaprotobuf

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/joeycumines/goja"
	gojarequire "github.com/joeycumines/goja_nodejs/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// setMapFromGoja: JS Map with bool/int keys (non-string)
// ---------------------------------------------------------------------------

func TestSetMapFromGoja_JSMapBoolKeys(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	v := env.run(t, `
		var MKM = pb.messageType('mapkeys.MultiKeyMap');
		var msg = new MKM();
		var m = new Map();
		m.set(true, 'yes');
		m.set(false, 'no');
		msg.set('bool_map', m);
		msg.get('bool_map').get(true) === 'yes' && msg.get('bool_map').get(false) === 'no'
	`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

func TestSetMapFromGoja_JSMapInt32Keys(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	v := env.run(t, `
		var MKM = pb.messageType('mapkeys.MultiKeyMap');
		var msg = new MKM();
		var m = new Map();
		m.set(42, 'answer');
		m.set(-1, 'neg');
		msg.set('int32_map', m);
		msg.get('int32_map').get(42) === 'answer' && msg.get('int32_map').get(-1) === 'neg'
	`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

// ---------------------------------------------------------------------------
// Additional descriptor loading edge cases
// ---------------------------------------------------------------------------

func TestLoadDescriptorSet_DuplicateFileInSameFDS(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fdp := testFileDescriptorProto()
	fds := &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{fdp, fdp},
	}
	data, err := proto.Marshal(fds)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	names, err := m.loadDescriptorSetBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) == 0 {
		t.Error("expected non-empty names")
	}
}

func TestLoadDescriptorSet_UnmarshalError(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = m.loadDescriptorSetBytes([]byte{0x0A, 0xFF})
	if err == nil {
		t.Error("expected error")
	}
}

func TestLoadFileDescriptorProtoBytes_UnmarshalError(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = m.loadFileDescriptorProtoBytes([]byte{0x0A, 0xFF})
	if err == nil {
		t.Error("expected error")
	}
}

// ---------------------------------------------------------------------------
// gojaToProtoMessage: null/undefined returns zero Value
// ---------------------------------------------------------------------------

func TestGojaToProtoMessage_Nil(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.NestedOuter")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fd := md.Fields().ByName("nested_inner")

	_, err = env.m.gojaToProtoMessage(nil, fd)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "null value for message field") {
		t.Errorf("error %q should contain %q", err.Error(), "null value for message field")
	}
}

func TestGojaToProtoMessage_Undefined(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.NestedOuter")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fd := md.Fields().ByName("nested_inner")

	_, err = env.m.gojaToProtoMessage(goja.Undefined(), fd)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "null value for message field") {
		t.Errorf("error %q should contain %q", err.Error(), "null value for message field")
	}
}

// ---------------------------------------------------------------------------
// gojaToProtoValue: nil/undefined/null returns default
// ---------------------------------------------------------------------------

func TestGojaToProtoValue_NilReturnsDefault(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.SimpleMessage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fd := md.Fields().ByName("name")

	pv, err := env.m.gojaToProtoValue(nil, fd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := pv.String(); got != "" {
		t.Errorf("got %q, want %q", got, "")
	}
}

// ---------------------------------------------------------------------------
// Enable — with valid options
// ---------------------------------------------------------------------------

func TestRequire_WithOptions(t *testing.T) {
	rt := goja.New()
	registry := gojarequire.NewRegistry()
	customResolver := new(protoregistry.Types)
	registry.RegisterNativeModule("protobuf", Require(WithResolver(customResolver)))
	registry.Enable(rt)

	v, err := rt.RunString(`typeof require('protobuf').encode === 'function'`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

// ---------------------------------------------------------------------------
// Comprehensive bytes handling
// ---------------------------------------------------------------------------

func TestExtractBytes_ArrayBuffer(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ab := rt.NewArrayBuffer([]byte{1, 2, 3})
	b, err := m.extractBytes(rt.ToValue(ab))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(b, []byte{1, 2, 3}) {
		t.Errorf("got %v, want %v", b, []byte{1, 2, 3})
	}
}

func TestExtractBytes_NullUndefined(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = m.extractBytes(nil)
	if err == nil {
		t.Error("expected error for nil")
	}

	_, err = m.extractBytes(goja.Undefined())
	if err == nil {
		t.Error("expected error for undefined")
	}

	_, err = m.extractBytes(goja.Null())
	if err == nil {
		t.Error("expected error for null")
	}
}

// ---------------------------------------------------------------------------
// extractMessageDesc: null/undefined
// ---------------------------------------------------------------------------

func TestExtractMessageDesc_NullUndefined(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = m.extractMessageDesc(nil)
	if err == nil {
		t.Error("expected error for nil")
	}
	if !strings.Contains(err.Error(), "null/undefined") {
		t.Errorf("error %q should contain %q", err.Error(), "null/undefined")
	}

	_, err = m.extractMessageDesc(goja.Undefined())
	if err == nil {
		t.Error("expected error for undefined")
	}
	if !strings.Contains(err.Error(), "null/undefined") {
		t.Errorf("error %q should contain %q", err.Error(), "null/undefined")
	}
}

// ---------------------------------------------------------------------------
// protoValueToGoja: bytes with nil
// ---------------------------------------------------------------------------

func TestProtoValueToGoja_NilBytes(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.AllTypes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("bytes_val")

	val := env.m.protoValueToGoja(msg.Get(fd), fd)
	if val == nil {
		t.Error("expected non-nil value")
	}
}

// ---------------------------------------------------------------------------
// MapField: forEach callback with non-function
// ---------------------------------------------------------------------------

func TestMapField_ForEachNonFunctionOnMultiKeyMap(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	env.run(t, `var MKM = pb.messageType('mapkeys.MultiKeyMap')`)
	env.run(t, `var msg = new MKM()`)
	env.mustFail(t, `msg.get('bool_map').forEach(42)`)
}

// ---------------------------------------------------------------------------
// Additional serialization tests
// ---------------------------------------------------------------------------

func TestToJSON_FromJSON_MultiKeyMap(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	v := env.run(t, `
		var MKM = pb.messageType('mapkeys.MultiKeyMap');
		var msg = new MKM();
		msg.get('bool_map').set(true, 'yes');
		msg.get('int32_map').set(42, 'answer');

		var json = pb.toJSON(msg);
		var msg2 = pb.fromJSON(MKM, json);
		msg2.get('bool_map').get(true) === 'yes' && msg2.get('int32_map').get(42) === 'answer'
	`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

// ---------------------------------------------------------------------------
// helpers.go: combinedFileResolver — both-miss paths
// ---------------------------------------------------------------------------

func TestCombinedFileResolver_FindFileByPath_BothMiss(t *testing.T) {
	r := &combinedFileResolver{local: new(protoregistry.Files), global: new(protoregistry.Files)}
	_, err := r.FindFileByPath("nonexistent.proto")
	if err == nil {
		t.Error("expected error")
	}
}

func TestCombinedFileResolver_FindDescriptorByName_BothMiss(t *testing.T) {
	r := &combinedFileResolver{local: new(protoregistry.Files), global: new(protoregistry.Files)}
	_, err := r.FindDescriptorByName("nonexistent.Msg")
	if err == nil {
		t.Error("expected error")
	}
}

func TestModule_FindDescriptor(t *testing.T) {
	env := newTestEnv(t)

	desc, err := env.m.FindDescriptor("test.SimpleMessage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := desc.FullName(); got != protoreflect.FullName("test.SimpleMessage") {
		t.Errorf("got %q, want %q", got, "test.SimpleMessage")
	}

	_, err = env.m.FindDescriptor("nonexistent.Type")
	if err == nil {
		t.Error("expected error")
	}
}

// ---------------------------------------------------------------------------
// conversion.go: setMapFromGoja — entries property not a function
// ---------------------------------------------------------------------------

func TestSetMapFromGoja_EntriesStringKey(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.MapMessage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("tags")

	objVal, err := env.rt.RunString(`({entries: 'not_a_function', k1: 'v1', k2: 'v2'})`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = env.m.setMapFromGoja(msg, fd, objVal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	protoMap := msg.Get(fd).Map()
	if got := protoMap.Get(protoreflect.ValueOfString("entries").MapKey()).String(); got != "not_a_function" {
		t.Fatalf("entries map value = %q", got)
	}
}

func TestSetMapFromGoja_EntriesFunctionIsPlainValue(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.MapMessage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("tags")

	objVal, err := env.rt.RunString(`
		var o = {k1: 'v1'};
		o.entries = function() { throw new Error("boom"); };
		o
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = env.m.setMapFromGoja(msg, fd, objVal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := msg.Get(fd).Map().Len(); got != 2 {
		t.Fatalf("map length = %d, want 2", got)
	}
}

func TestSetMapFromGoja_EntriesFunctionDoesNotSelectIterator(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.MapMessage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("tags")

	objVal, err := env.rt.RunString(`
		var o = {k1: 'v1'};
		o.entries = function() { return {}; };
		o
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = env.m.setMapFromGoja(msg, fd, objVal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := msg.Get(fd).Map().Len(); got != 2 {
		t.Fatalf("map length = %d, want 2", got)
	}
}

func TestSetMapFromGoja_SymbolIteratorCallError(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.MapMessage")
	if err != nil {
		t.Fatal(err)
	}
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("tags")
	value, err := env.rt.RunString(`
		var o = {};
		o[Symbol.iterator] = function() { throw new Error("iterator call"); };
		o;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.m.setMapFromGoja(msg, fd, value); err == nil {
		t.Fatal("expected iterator call error")
	}
	if msg.Has(fd) {
		t.Fatal("failed iterable conversion mutated the map")
	}
}

func TestSetMapFromGoja_SymbolIteratorNoNext(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.MapMessage")
	if err != nil {
		t.Fatal(err)
	}
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("tags")
	value, err := env.rt.RunString(`
		var o = {};
		o[Symbol.iterator] = function() { return {}; };
		o;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.m.setMapFromGoja(msg, fd, value); err == nil {
		t.Fatal("expected missing next error")
	}
	if msg.Has(fd) {
		t.Fatal("failed iterable conversion mutated the map")
	}
}

func TestSetMapFromGoja_SymbolIteratorMalformedResult(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.MapMessage")
	if err != nil {
		t.Fatal(err)
	}
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("tags")
	value, err := env.rt.RunString(`
		var o = {};
		o[Symbol.iterator] = function() {
			return { next: function() { return 1; } };
		};
		o;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.m.setMapFromGoja(msg, fd, value); err == nil {
		t.Fatal("expected malformed iterator result error")
	}
	if msg.Has(fd) {
		t.Fatal("failed iterable conversion mutated the map")
	}
}

// ---------------------------------------------------------------------------
// conversion.go: setRepeatedFromGoja — obj with entries (not an array)
// ---------------------------------------------------------------------------

func TestSetRepeatedFromGoja_ObjectNoLengthViaJS(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.RepeatedMessage'))()`)
	env.mustFail(t, `msg.set('items', {foo: 'bar'})`)
}

// ---------------------------------------------------------------------------
// descriptors.go: jsLoadDescriptorSet — empty FDS via JS
// ---------------------------------------------------------------------------

func TestJsLoadDescriptorSet_EmptyFDS(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pb := rt.NewObject()
	m.setupExports(pb)
	if err := rt.Set("pb", pb); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	emptyFDS := &descriptorpb.FileDescriptorSet{}
	data, err := proto.Marshal(emptyFDS)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := rt.Set("emptyBytes", rt.NewArrayBuffer(data)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	v, err := rt.RunString(`
		var names = pb.loadDescriptorSet(new Uint8Array(emptyBytes));
		names.length === 0
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

// ---------------------------------------------------------------------------
// message.go: wrapRepeatedField — forEach callback throws
// ---------------------------------------------------------------------------

func TestRepeatedField_ForEachCallbackThrows(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.RepeatedMessage'))();
		msg.get('items').add('a');
	`)
	env.mustFail(t, `
		msg.get('items').forEach(function() { throw new Error('callback error'); })
	`)
}

// ---------------------------------------------------------------------------
// message.go: wrapMapField — forEach callback throws
// ---------------------------------------------------------------------------

func TestMapField_ForEachCallbackThrows(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.MapMessage'))();
		msg.get('tags').set('k', 'v');
	`)
	env.mustFail(t, `
		msg.get('tags').forEach(function() { throw new Error('callback error'); })
	`)
}

// ---------------------------------------------------------------------------
// conversion.go: setMapFromGoja — JS Map iterator next() error
// ---------------------------------------------------------------------------

func TestSetMapFromGoja_IteratorNextError(t *testing.T) {
	env := newTestEnv(t)
	md, err := env.m.findMessageDescriptor("test.MapMessage")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("tags")

	objVal, err := env.rt.RunString(`
		var o = {};
		o[Symbol.iterator] = function() {
			return {
				next: function() { throw new Error('iter error'); }
			};
		};
		o
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = env.m.setMapFromGoja(msg, fd, objVal)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "map field tags iterator:") {
		t.Fatalf("error %q should contain %q", err.Error(), "map field tags iterator:")
	}
	if !strings.Contains(err.Error(), "iter error") {
		t.Fatalf("error %q should contain %q", err.Error(), "iter error")
	}
}

// ---------------------------------------------------------------------------
// descriptors.go: loadFileDescriptorProtoBytes — RegisterFile error
// ---------------------------------------------------------------------------

func TestLoadFileDescriptorProtoBytes_RegisterFileError(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fdp := testFileDescriptorProto()
	data, err := proto.Marshal(fdp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = m.loadFileDescriptorProtoBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	names, err := m.loadFileDescriptorProtoBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if names != nil {
		t.Errorf("expected nil, got %v", names)
	}
}

// ---------------------------------------------------------------------------
// Additional tests for coverage completeness
// ---------------------------------------------------------------------------

func TestJsLoadFileDescriptorProto_AlreadyRegisteredViaJS(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pb := rt.NewObject()
	m.setupExports(pb)
	if err := rt.Set("pb", pb); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fdp := testFileDescriptorProto()
	data, err := proto.Marshal(fdp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := rt.Set("protoBytes", rt.NewArrayBuffer(data)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First load.
	v, err := rt.RunString(`
		var names1 = pb.loadFileDescriptorProto(new Uint8Array(protoBytes));
		names1.length
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.ToInteger() <= 0 {
		t.Error("expected positive length")
	}

	// Second load — already registered, returns empty array.
	v, err = rt.RunString(`
		var names2 = pb.loadFileDescriptorProto(new Uint8Array(protoBytes));
		names2.length
	`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := v.ToInteger(); got != int64(0) {
		t.Errorf("got %d, want %d", got, 0)
	}
}

func TestMapField_EntriesIteratorDone(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.MapMessage'))();
		msg.get('tags').set('only', 'one');
	`)
	v := env.run(t, `
		var iter = msg.get('tags').entries();
		var r1 = iter.next();
		var r2 = iter.next();
		!r1.done && r2.done && r1.value[0] === 'only' && r1.value[1] === 'one'
	`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

func TestEncodeDecodeBytes(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(t, `
		var AT = pb.messageType('test.AllTypes');
		var msg = new AT();
		msg.set('bytes_val', new Uint8Array([0xDE, 0xAD, 0xBE, 0xEF]));
		var encoded = pb.encode(msg);
		var decoded = pb.decode(AT, encoded);
		var b = decoded.get('bytes_val');
		b[0] === 0xDE && b[1] === 0xAD && b[2] === 0xBE && b[3] === 0xEF
	`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

// ---------------------------------------------------------------------------
// serialize.go: jsToJSON — JSON.parse overridden to non-function
// ---------------------------------------------------------------------------

func TestJsToJSON_JSONParseOverridden(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var SM = pb.messageType('test.SimpleMessage');
		var msg = new SM();
		msg.set('name', 'test');
	`)
	env.run(t, `JSON.parse = 42`)
	env.mustFail(t, `pb.toJSON(msg)`)
}

// ---------------------------------------------------------------------------
// serialize.go: jsFromJSON — JSON.stringify overridden to non-function
// ---------------------------------------------------------------------------

func TestJsFromJSON_JSONStringifyOverridden(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var SM = pb.messageType('test.SimpleMessage')`)
	env.run(t, `JSON.stringify = 42`)
	env.mustFail(t, `pb.fromJSON(SM, {name: 'test'})`)
}

// ---------------------------------------------------------------------------
// serialize.go: jsFromJSON — JSON.stringify call throws (circular ref)
// ---------------------------------------------------------------------------

func TestJsFromJSON_StringifyCallErrors(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var SM = pb.messageType('test.SimpleMessage')`)
	env.mustFail(t, `
		var c = {};
		c.self = c;
		pb.fromJSON(SM, c)
	`)
}

// ---------------------------------------------------------------------------
// serialize.go: jsToJSON — JSON.parse call throws
// ---------------------------------------------------------------------------

func TestJsToJSON_JSONParseCallErrors(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var SM = pb.messageType('test.SimpleMessage');
		var msg = new SM();
		msg.set('name', 'test');
	`)
	env.run(t, `JSON.parse = function() { throw new Error("parse error"); }`)
	env.mustFail(t, `pb.toJSON(msg)`)
}

// ---------------------------------------------------------------------------
// descriptors.go: jsLoadDescriptorSet — invalid-but-extractable bytes
// ---------------------------------------------------------------------------

func TestJsLoadDescriptorSet_InvalidProtobufBytes(t *testing.T) {
	env := newTestEnv(t)
	env.rt.Set("badData", env.rt.NewArrayBuffer([]byte{0x0A, 0xC8, 0x01}))
	env.mustFail(t, `pb.loadDescriptorSet(new Uint8Array(badData))`)
}

// ---------------------------------------------------------------------------
// descriptors.go: jsLoadFileDescriptorProto — proto.Unmarshal error via JS
// ---------------------------------------------------------------------------

func TestJsLoadFileDescriptorProto_InvalidProtobufBytes(t *testing.T) {
	env := newTestEnv(t)
	env.rt.Set("badData", env.rt.NewArrayBuffer([]byte{0x0A, 0xC8, 0x01}))
	env.mustFail(t, `pb.loadFileDescriptorProto(new Uint8Array(badData))`)
}

// Test gojaToProtoValue with null/undefined for various field kinds.
func TestGojaToProtoValue_NullForAllScalarTypes(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	env.run(t, `msg.set('int32_val', 42); msg.set('int32_val', null)`)
	v := env.run(t, `msg.get('int32_val')`)
	if got := v.ToInteger(); got != int64(0) {
		t.Errorf("got %d, want %d", got, 0)
	}

	env.run(t, `msg.set('uint64_val', 100); msg.set('uint64_val', null)`)
	v = env.run(t, `msg.get('uint64_val')`)
	if got := v.ToInteger(); got != int64(0) {
		t.Errorf("got %d, want %d", got, 0)
	}

	env.run(t, `msg.set('float_val', 1.5); msg.set('float_val', null)`)
	v = env.run(t, `msg.get('float_val')`)
	if d := math.Abs(v.ToFloat() - 0.0); d > 0.001 {
		t.Errorf("got %f, want %f (delta %f > 0.001)", v.ToFloat(), 0.0, d)
	}
}

// Test map field set via plain object key error path.
func TestSetMapFromGoja_PlainObjectKeyError(t *testing.T) {
	env := newTestEnvWithMapKeys(t)
	md, err := env.m.findMessageDescriptor("mapkeys.MultiKeyMap")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msg := dynamicpb.NewMessage(md)
	fd := md.Fields().ByName("uint32_map")

	objVal, err := env.rt.RunString(`({"-1": "v"})`)
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
}

// ---------------------------------------------------------------------------
