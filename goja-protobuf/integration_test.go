package gojaprotobuf

import (
	"testing"

	"github.com/dop251/goja"
	gojarequire "github.com/dop251/goja_nodejs/require"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFullJSWorkflow is a comprehensive end-to-end integration test that
// exercises the full require('protobuf') → load → create → populate →
// encode → decode → JSON roundtrip workflow entirely from JavaScript.
func TestFullJSWorkflow(t *testing.T) {
	rt := goja.New()
	registry := gojarequire.NewRegistry()
	registry.RegisterNativeModule("protobuf", Require())
	registry.Enable(rt)

	// Make serialised test descriptors available to JS.
	_ = rt.Set("descriptorBytes", rt.NewArrayBuffer(testDescriptorSetBytes()))

	v, err := rt.RunString(`
		var pb = require('protobuf');

		// ---- Load descriptors ----
		var types = pb.loadDescriptorSet(new Uint8Array(descriptorBytes));
		if (types.length === 0) throw new Error('no types loaded');

		// ---- SimpleMessage roundtrip ----
		var SimpleMessage = pb.messageType('test.SimpleMessage');
		var msg = new SimpleMessage();
		msg.set('name', 'integration test');
		msg.set('value', 42);
		if (msg.$type !== 'test.SimpleMessage') throw new Error('$type mismatch');

		// Binary encode/decode
		var encoded = pb.encode(msg);
		if (encoded.length === 0) throw new Error('encoded should not be empty');
		var decoded = pb.decode(SimpleMessage, encoded);
		if (decoded.get('name') !== 'integration test') throw new Error('binary name mismatch');
		if (decoded.get('value') !== 42) throw new Error('binary value mismatch');

		// JSON roundtrip
		var json = pb.toJSON(msg);
		if (json.name !== 'integration test') throw new Error('json name mismatch: ' + json.name);
		if (json.value !== 42) throw new Error('json value mismatch');
		var fromJson = pb.fromJSON(SimpleMessage, json);
		if (fromJson.get('name') !== 'integration test') throw new Error('fromJSON name mismatch');
		if (fromJson.get('value') !== 42) throw new Error('fromJSON value mismatch');

		// ---- Enum ----
		var TestEnum = pb.enumType('test.TestEnum');
		if (TestEnum.UNKNOWN !== 0) throw new Error('UNKNOWN should be 0');
		if (TestEnum.FIRST !== 1) throw new Error('FIRST should be 1');
		if (TestEnum.SECOND !== 2) throw new Error('SECOND should be 2');
		if (TestEnum.THIRD !== 3) throw new Error('THIRD should be 3');
		if (TestEnum[0] !== 'UNKNOWN') throw new Error('0 should be UNKNOWN');
		if (TestEnum[1] !== 'FIRST') throw new Error('1 should be FIRST');

		// ---- AllTypes ----
		var AllTypes = pb.messageType('test.AllTypes');
		var at = new AllTypes();
		at.set('int32_val', 100);
		at.set('string_val', 'alltest');
		at.set('bool_val', true);
		at.set('double_val', 2.718);
		at.set('enum_val', 'SECOND');
		at.set('bytes_val', new Uint8Array([0xDE, 0xAD]));

		var atEnc = pb.encode(at);
		var atDec = pb.decode(AllTypes, atEnc);
		if (atDec.get('int32_val') !== 100) throw new Error('int32 mismatch');
		if (atDec.get('string_val') !== 'alltest') throw new Error('string mismatch');
		if (atDec.get('bool_val') !== true) throw new Error('bool mismatch');
		if (atDec.get('enum_val') !== 2) throw new Error('enum mismatch');

		// ---- Repeated fields ----
		var RepMsg = pb.messageType('test.RepeatedMessage');
		var rep = new RepMsg();
		rep.get('items').add('a');
		rep.get('items').add('b');
		rep.get('items').add('c');
		rep.get('numbers').add(10);
		rep.get('numbers').add(20);

		var repEnc = pb.encode(rep);
		var repDec = pb.decode(RepMsg, repEnc);
		if (repDec.get('items').length !== 3) throw new Error('repeated items length');
		if (repDec.get('items').get(0) !== 'a') throw new Error('repeated items[0]');
		if (repDec.get('numbers').get(1) !== 20) throw new Error('repeated numbers[1]');

		// ---- Map fields ----
		var MapMsg = pb.messageType('test.MapMessage');
		var mapMsg = new MapMsg();
		mapMsg.get('tags').set('key1', 'val1');
		mapMsg.get('tags').set('key2', 'val2');
		mapMsg.get('counts').set('count_a', 5);

		var mapEnc = pb.encode(mapMsg);
		var mapDec = pb.decode(MapMsg, mapEnc);
		if (mapDec.get('tags').get('key1') !== 'val1') throw new Error('map tag mismatch');
		if (mapDec.get('tags').size !== 2) throw new Error('map tags size');
		if (mapDec.get('counts').get('count_a') !== 5) throw new Error('map counts mismatch');

		// ---- Oneof ----
		var OneofMsg = pb.messageType('test.OneofMessage');
		var oo = new OneofMsg();
		oo.set('str_choice', 'hello');
		if (oo.whichOneof('choice') !== 'str_choice') throw new Error('oneof should be str_choice');
		oo.set('int_choice', 42);
		if (oo.whichOneof('choice') !== 'int_choice') throw new Error('oneof should be int_choice');
		if (oo.get('str_choice') !== '') throw new Error('str_choice should be cleared');

		var ooEnc = pb.encode(oo);
		var ooDec = pb.decode(OneofMsg, ooEnc);
		if (ooDec.whichOneof('choice') !== 'int_choice') throw new Error('decoded oneof');
		if (ooDec.get('int_choice') !== 42) throw new Error('decoded int_choice');

		// ---- Nested message ----
		var NestedOuter = pb.messageType('test.NestedOuter');
		var outer = new NestedOuter();
		outer.set('name', 'parentName');
		outer.set('nested_inner', {value: 777});

		var noEnc = pb.encode(outer);
		var noDec = pb.decode(NestedOuter, noEnc);
		if (noDec.get('name') !== 'parentName') throw new Error('nested name');
		if (noDec.get('nested_inner').get('value') !== 777) throw new Error('nested inner value');

		// ---- has/clear ----
		var sm2 = new SimpleMessage();
		if (sm2.has('name') !== false) throw new Error('has should be false initially');
		sm2.set('name', 'test');
		if (sm2.has('name') !== true) throw new Error('has should be true after set');
		sm2.clear('name');
		if (sm2.has('name') !== false) throw new Error('has should be false after clear');

		// ---- toJSON with all types ----
		var jsonAll = pb.toJSON(at);
		if (jsonAll.int32Val !== 100) throw new Error('json int32Val');
		if (jsonAll.stringVal !== 'alltest') throw new Error('json stringVal');
		if (jsonAll.boolVal !== true) throw new Error('json boolVal');
		if (jsonAll.enumVal !== 'SECOND') throw new Error('json enumVal');

		// Success
		true
	`)
	require.NoError(t, err)
	assert.True(t, v.ToBoolean())
}

// TestFullJSWorkflow_mapFromObject tests setting map field from a plain
// JS object through the full require() path.
func TestFullJSWorkflow_MapFromObject(t *testing.T) {
	rt := goja.New()
	registry := gojarequire.NewRegistry()
	registry.RegisterNativeModule("protobuf", Require())
	registry.Enable(rt)

	_ = rt.Set("descriptorBytes", rt.NewArrayBuffer(testDescriptorSetBytes()))

	v, err := rt.RunString(`
		var pb = require('protobuf');
		pb.loadDescriptorSet(new Uint8Array(descriptorBytes));

		var MapMsg = pb.messageType('test.MapMessage');
		var msg = new MapMsg();
		msg.set('tags', {hello: 'world', foo: 'bar'});

		var tags = msg.get('tags');
		tags.get('hello') === 'world' && tags.get('foo') === 'bar' && tags.size === 2
	`)
	require.NoError(t, err)
	assert.True(t, v.ToBoolean())
}

// TestFullJSWorkflow_RepeatedFromArray tests setting repeated field from
// a JS array through the full require() path.
func TestFullJSWorkflow_RepeatedFromArray(t *testing.T) {
	rt := goja.New()
	registry := gojarequire.NewRegistry()
	registry.RegisterNativeModule("protobuf", Require())
	registry.Enable(rt)

	_ = rt.Set("descriptorBytes", rt.NewArrayBuffer(testDescriptorSetBytes()))

	v, err := rt.RunString(`
		var pb = require('protobuf');
		pb.loadDescriptorSet(new Uint8Array(descriptorBytes));

		var RepMsg = pb.messageType('test.RepeatedMessage');
		var msg = new RepMsg();
		msg.set('items', ['x', 'y', 'z']);

		var items = msg.get('items');
		items.length === 3 && items.get(0) === 'x' && items.get(2) === 'z'
	`)
	require.NoError(t, err)
	assert.True(t, v.ToBoolean())
}

// TestFullJSWorkflow_EqualsCloneIsMessage exercises the equals, clone,
// and isMessage utility functions through the full require() path.
func TestFullJSWorkflow_EqualsCloneIsMessage(t *testing.T) {
	rt := goja.New()
	registry := gojarequire.NewRegistry()
	registry.RegisterNativeModule("protobuf", Require())
	registry.Enable(rt)

	_ = rt.Set("descriptorBytes", rt.NewArrayBuffer(testDescriptorSetBytes()))

	v, err := rt.RunString(`
		var pb = require('protobuf');
		pb.loadDescriptorSet(new Uint8Array(descriptorBytes));

		var SM = pb.messageType('test.SimpleMessage');

		// ---- equals ----
		var a = new SM();
		a.set('name', 'hello');
		a.set('value', 42);

		var b = new SM();
		b.set('name', 'hello');
		b.set('value', 42);

		if (!pb.equals(a, b)) throw new Error('equals: identical messages should be equal');

		b.set('value', 99);
		if (pb.equals(a, b)) throw new Error('equals: different messages should not be equal');

		// ---- clone ----
		var original = new SM();
		original.set('name', 'original');
		original.set('value', 100);

		var cloned = pb.clone(original);
		if (!pb.equals(original, cloned)) throw new Error('clone: should equal original');

		cloned.set('name', 'modified');
		if (pb.equals(original, cloned)) throw new Error('clone: modifying clone should not affect original');
		if (original.get('name') !== 'original') throw new Error('clone: original should be unchanged');

		// ---- isMessage ----
		if (!pb.isMessage(a)) throw new Error('isMessage(msg) should be true');
		if (pb.isMessage('not a message')) throw new Error('isMessage(string) should be false');
		if (pb.isMessage(42)) throw new Error('isMessage(number) should be false');
		if (pb.isMessage(null)) throw new Error('isMessage(null) should be false');
		if (pb.isMessage(undefined)) throw new Error('isMessage(undefined) should be false');

		// isMessage with type name
		if (!pb.isMessage(a, 'test.SimpleMessage')) throw new Error('isMessage with matching type should be true');
		if (pb.isMessage(a, 'test.AllTypes')) throw new Error('isMessage with non-matching type should be false');

		// ---- isFieldSet ----
		var c = new SM();
		if (pb.isFieldSet(c, 'name')) throw new Error('isFieldSet: unset field should be false');
		c.set('name', 'test');
		if (!pb.isFieldSet(c, 'name')) throw new Error('isFieldSet: set field should be true');

		// ---- clearField ----
		pb.clearField(c, 'name');
		if (pb.isFieldSet(c, 'name')) throw new Error('clearField: field should be unset after clear');
		if (c.get('name') !== '') throw new Error('clearField: field should return default after clear');

		// Success
		true
	`)
	require.NoError(t, err)
	assert.True(t, v.ToBoolean())
}
