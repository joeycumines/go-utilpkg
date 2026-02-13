package gojaprotobuf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncode_Decode_RoundTrip(t *testing.T) {
	env := newTestEnv(t)

	v := env.run(t, `
		var SM = pb.messageType('test.SimpleMessage');
		var msg = new SM();
		msg.set('name', 'roundtrip');
		msg.set('value', 42);

		var encoded = pb.encode(msg);
		var decoded = pb.decode(SM, encoded);

		decoded.get('name') === 'roundtrip' && decoded.get('value') === 42
	`)
	assert.True(t, v.ToBoolean())
}

func TestEncode_Decode_AllScalars(t *testing.T) {
	env := newTestEnv(t)

	v := env.run(t, `
		var AT = pb.messageType('test.AllTypes');
		var msg = new AT();
		msg.set('int32_val', -42);
		msg.set('int64_val', 123456);
		msg.set('uint32_val', 300);
		msg.set('uint64_val', 400);
		msg.set('float_val', 1.5);
		msg.set('double_val', 3.14);
		msg.set('bool_val', true);
		msg.set('string_val', 'test');
		msg.set('enum_val', 2);
		msg.set('sint32_val', -10);
		msg.set('sint64_val', -20);
		msg.set('fixed32_val', 1000);
		msg.set('fixed64_val', 2000);
		msg.set('sfixed32_val', -1000);
		msg.set('sfixed64_val', -2000);

		var encoded = pb.encode(msg);
		var d = pb.decode(AT, encoded);

		d.get('int32_val') === -42 &&
		d.get('int64_val') === 123456 &&
		d.get('uint32_val') === 300 &&
		d.get('uint64_val') === 400 &&
		Math.abs(d.get('float_val') - 1.5) < 0.01 &&
		Math.abs(d.get('double_val') - 3.14) < 0.001 &&
		d.get('bool_val') === true &&
		d.get('string_val') === 'test' &&
		d.get('enum_val') === 2 &&
		d.get('sint32_val') === -10 &&
		d.get('sint64_val') === -20 &&
		d.get('fixed32_val') === 1000 &&
		d.get('fixed64_val') === 2000 &&
		d.get('sfixed32_val') === -1000 &&
		d.get('sfixed64_val') === -2000
	`)
	assert.True(t, v.ToBoolean())
}

func TestEncode_EmptyMessage(t *testing.T) {
	env := newTestEnv(t)

	v := env.run(t, `
		var SM = pb.messageType('test.SimpleMessage');
		var msg = new SM();
		var encoded = pb.encode(msg);
		encoded.length
	`)
	assert.Equal(t, int64(0), v.ToInteger())
}

func TestDecode_InvalidBytes(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var SM = pb.messageType('test.SimpleMessage')`)

	// Truncated length-delimited field: tag=0x0A (field 1, wire type 2),
	// length=100, but only 2 data bytes.
	env.rt.Set("badData", env.rt.NewArrayBuffer([]byte{0x0A, 0x64, 0x01, 0x02}))

	env.mustFail(t, `pb.decode(SM, new Uint8Array(badData))`)
}

func TestDecode_NonBytesInput(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var SM = pb.messageType('test.SimpleMessage')`)

	// Passing null should throw.
	env.mustFail(t, `pb.decode(SM, null)`)

	// Passing undefined should throw.
	env.mustFail(t, `pb.decode(SM, undefined)`)
}

func TestEncode_NonMessage(t *testing.T) {
	env := newTestEnv(t)

	// Encoding a non-message value should throw.
	env.mustFail(t, `pb.encode({})`)
	env.mustFail(t, `pb.encode(null)`)
	env.mustFail(t, `pb.encode(42)`)
}

func TestDecode_InvalidConstructor(t *testing.T) {
	env := newTestEnv(t)

	// decode without a valid constructor should throw.
	env.mustFail(t, `pb.decode({}, new Uint8Array([]))`)
	env.mustFail(t, `pb.decode(null, new Uint8Array([]))`)
}

func TestToJSON_FromJSON_RoundTrip(t *testing.T) {
	env := newTestEnv(t)

	v := env.run(t, `
		var SM = pb.messageType('test.SimpleMessage');
		var msg = new SM();
		msg.set('name', 'json test');
		msg.set('value', 99);

		var json = pb.toJSON(msg);
		var msg2 = pb.fromJSON(SM, json);

		msg2.get('name') === 'json test' && msg2.get('value') === 99
	`)
	assert.True(t, v.ToBoolean())
}

func TestToJSON_FieldTypes(t *testing.T) {
	env := newTestEnv(t)

	// Proto3 JSON uses camelCase names and specific type representations.
	v := env.run(t, `
		var AT = pb.messageType('test.AllTypes');
		var msg = new AT();
		msg.set('int32_val', 42);
		msg.set('string_val', 'hello');
		msg.set('bool_val', true);
		msg.set('double_val', 3.14);
		msg.set('enum_val', 'FIRST');

		var json = pb.toJSON(msg);
		json.int32Val === 42 &&
		json.stringVal === 'hello' &&
		json.boolVal === true &&
		typeof json.doubleVal === 'number' &&
		json.enumVal === 'FIRST'
	`)
	assert.True(t, v.ToBoolean())
}

func TestToJSON_Int64AsString(t *testing.T) {
	env := newTestEnv(t)

	// Proto3 JSON serializes int64/uint64 as strings.
	v := env.run(t, `
		var AT = pb.messageType('test.AllTypes');
		var msg = new AT();
		msg.set('int64_val', 9007199254740993);

		var json = pb.toJSON(msg);
		typeof json.int64Val === 'string'
	`)
	assert.True(t, v.ToBoolean())
}

func TestToJSON_BytesAsBase64(t *testing.T) {
	env := newTestEnv(t)

	// Proto3 JSON serializes bytes as base64.
	v := env.run(t, `
		var AT = pb.messageType('test.AllTypes');
		var msg = new AT();
		msg.set('bytes_val', new Uint8Array([1, 2, 3]));

		var json = pb.toJSON(msg);
		typeof json.bytesVal === 'string' && json.bytesVal === 'AQID'
	`)
	assert.True(t, v.ToBoolean())
}

func TestFromJSON_InvalidJSON(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var SM = pb.messageType('test.SimpleMessage')`)

	// fromJSON with null/undefined should throw.
	env.mustFail(t, `pb.fromJSON(SM, null)`)
}

func TestFromJSON_InvalidConstructor(t *testing.T) {
	env := newTestEnv(t)

	env.mustFail(t, `pb.fromJSON({}, {name: 'test'})`)
	env.mustFail(t, `pb.fromJSON(null, {name: 'test'})`)
}

func TestToJSON_NonMessage(t *testing.T) {
	env := newTestEnv(t)
	env.mustFail(t, `pb.toJSON({})`)
	env.mustFail(t, `pb.toJSON(null)`)
}

func TestEncode_Decode_WithRepeated(t *testing.T) {
	env := newTestEnv(t)

	v := env.run(t, `
		var RM = pb.messageType('test.RepeatedMessage');
		var msg = new RM();
		msg.get('items').add('a');
		msg.get('items').add('b');
		msg.get('items').add('c');
		msg.get('numbers').add(10);
		msg.get('numbers').add(20);

		var encoded = pb.encode(msg);
		var decoded = pb.decode(RM, encoded);
		var items = decoded.get('items');
		var nums = decoded.get('numbers');

		items.length === 3 &&
		items.get(0) === 'a' && items.get(1) === 'b' && items.get(2) === 'c' &&
		nums.length === 2 &&
		nums.get(0) === 10 && nums.get(1) === 20
	`)
	assert.True(t, v.ToBoolean())
}

func TestEncode_Decode_WithMap(t *testing.T) {
	env := newTestEnv(t)

	v := env.run(t, `
		var MM = pb.messageType('test.MapMessage');
		var msg = new MM();
		msg.get('tags').set('k1', 'v1');
		msg.get('tags').set('k2', 'v2');
		msg.get('counts').set('a', 10);

		var encoded = pb.encode(msg);
		var decoded = pb.decode(MM, encoded);
		var tags = decoded.get('tags');
		var counts = decoded.get('counts');

		tags.get('k1') === 'v1' && tags.get('k2') === 'v2' && tags.size === 2 &&
		counts.get('a') === 10 && counts.size === 1
	`)
	assert.True(t, v.ToBoolean())
}

func TestEncode_Decode_WithOneof(t *testing.T) {
	env := newTestEnv(t)

	v := env.run(t, `
		var OM = pb.messageType('test.OneofMessage');
		var msg = new OM();
		msg.set('str_choice', 'picked');

		var encoded = pb.encode(msg);
		var decoded = pb.decode(OM, encoded);
		decoded.whichOneof('choice') === 'str_choice' &&
		decoded.get('str_choice') === 'picked'
	`)
	assert.True(t, v.ToBoolean())
}

func TestEncode_Decode_WithNested(t *testing.T) {
	env := newTestEnv(t)

	v := env.run(t, `
		var NO = pb.messageType('test.NestedOuter');
		var msg = new NO();
		msg.set('name', 'outer');
		msg.set('nested_inner', {value: 77});

		var encoded = pb.encode(msg);
		var decoded = pb.decode(NO, encoded);
		decoded.get('name') === 'outer' &&
		decoded.get('nested_inner').get('value') === 77
	`)
	assert.True(t, v.ToBoolean())
}
