package gojaprotobuf

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
)

func TestMessage_GetSet_Scalars(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	// int32
	v := env.run(t, `msg.set('int32_val', 42); msg.get('int32_val')`)
	assert.Equal(t, int64(42), v.ToInteger())

	// int64
	v = env.run(t, `msg.set('int64_val', 100); msg.get('int64_val')`)
	assert.Equal(t, int64(100), v.ToInteger())

	// uint32
	v = env.run(t, `msg.set('uint32_val', 200); msg.get('uint32_val')`)
	assert.Equal(t, int64(200), v.ToInteger())

	// uint64
	v = env.run(t, `msg.set('uint64_val', 300); msg.get('uint64_val')`)
	assert.Equal(t, int64(300), v.ToInteger())

	// float (uses exact float32 value)
	v = env.run(t, `msg.set('float_val', 1.5); msg.get('float_val')`)
	assert.InDelta(t, 1.5, v.ToFloat(), 0.001)

	// double
	v = env.run(t, `msg.set('double_val', 3.14159); msg.get('double_val')`)
	assert.InDelta(t, 3.14159, v.ToFloat(), 0.00001)

	// bool
	v = env.run(t, `msg.set('bool_val', true); msg.get('bool_val')`)
	assert.True(t, v.ToBoolean())

	// sint32
	v = env.run(t, `msg.set('sint32_val', -42); msg.get('sint32_val')`)
	assert.Equal(t, int64(-42), v.ToInteger())

	// sint64
	v = env.run(t, `msg.set('sint64_val', -100); msg.get('sint64_val')`)
	assert.Equal(t, int64(-100), v.ToInteger())

	// fixed32
	v = env.run(t, `msg.set('fixed32_val', 500); msg.get('fixed32_val')`)
	assert.Equal(t, int64(500), v.ToInteger())

	// fixed64
	v = env.run(t, `msg.set('fixed64_val', 600); msg.get('fixed64_val')`)
	assert.Equal(t, int64(600), v.ToInteger())

	// sfixed32
	v = env.run(t, `msg.set('sfixed32_val', -500); msg.get('sfixed32_val')`)
	assert.Equal(t, int64(-500), v.ToInteger())

	// sfixed64
	v = env.run(t, `msg.set('sfixed64_val', -600); msg.get('sfixed64_val')`)
	assert.Equal(t, int64(-600), v.ToInteger())
}

func TestMessage_GetSet_String(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.SimpleMessage'))()`)

	v := env.run(t, `msg.set('name', 'hello world'); msg.get('name')`)
	assert.Equal(t, "hello world", v.String())

	// Empty string.
	v = env.run(t, `msg.set('name', ''); msg.get('name')`)
	assert.Equal(t, "", v.String())

	// Unicode.
	v = env.run(t, `msg.set('name', '日本語テスト'); msg.get('name')`)
	assert.Equal(t, "日本語テスト", v.String())
}

func TestMessage_GetSet_Bytes(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	v := env.run(t, `
		msg.set('bytes_val', new Uint8Array([1, 2, 3, 255]));
		var b = msg.get('bytes_val');
		b[0] === 1 && b[1] === 2 && b[2] === 3 && b[3] === 255 && b.length === 4
	`)
	assert.True(t, v.ToBoolean())

	// Empty bytes.
	v = env.run(t, `
		msg.set('bytes_val', new Uint8Array([]));
		msg.get('bytes_val').length === 0
	`)
	assert.True(t, v.ToBoolean())
}

func TestMessage_GetSet_Enum(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	// Set by number.
	v := env.run(t, `msg.set('enum_val', 1); msg.get('enum_val')`)
	assert.Equal(t, int64(1), v.ToInteger())

	// Set by name.
	v = env.run(t, `msg.set('enum_val', 'SECOND'); msg.get('enum_val')`)
	assert.Equal(t, int64(2), v.ToInteger())

	// Default value.
	v = env.run(t, `msg.set('enum_val', 0); msg.get('enum_val')`)
	assert.Equal(t, int64(0), v.ToInteger())
}

func TestMessage_GetSet_NestedMessage(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.NestedOuter'))()`)

	// Unset message field returns null.
	v := env.run(t, `msg.get('nested_inner') === null`)
	assert.True(t, v.ToBoolean())

	// Set nested message via a plain object.
	v = env.run(t, `
		msg.set('nested_inner', {value: 42});
		var inner = msg.get('nested_inner');
		inner.get('value')
	`)
	assert.Equal(t, int64(42), v.ToInteger())

	// Set nested message via a wrapped message.
	v = env.run(t, `
		var NI = pb.messageType('test.NestedInner');
		var ni = new NI();
		ni.set('value', 99);
		msg.set('nested_inner', ni);
		msg.get('nested_inner').get('value')
	`)
	assert.Equal(t, int64(99), v.ToInteger())

	// Clear nested message with null.
	v = env.run(t, `
		msg.set('nested_inner', null);
		msg.get('nested_inner') === null
	`)
	assert.True(t, v.ToBoolean())
}

func TestMessage_Has_Clear(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	// Proto3 scalar: has returns false for default value (0 for int32).
	v := env.run(t, `msg.has('int32_val')`)
	assert.False(t, v.ToBoolean())

	// After setting to non-default, has returns true.
	v = env.run(t, `msg.set('int32_val', 42); msg.has('int32_val')`)
	assert.True(t, v.ToBoolean())

	// Clear resets to default; has returns false.
	v = env.run(t, `msg.clear('int32_val'); msg.has('int32_val')`)
	assert.False(t, v.ToBoolean())

	// Proto3 string: has returns false for default ("").
	v = env.run(t, `msg.has('string_val')`)
	assert.False(t, v.ToBoolean())

	v = env.run(t, `msg.set('string_val', 'hello'); msg.has('string_val')`)
	assert.True(t, v.ToBoolean())

	v = env.run(t, `msg.clear('string_val'); msg.has('string_val')`)
	assert.False(t, v.ToBoolean())

	// Proto3 optional (explicit presence): has returns true even for default.
	v = env.run(t, `msg.has('optional_string')`)
	assert.False(t, v.ToBoolean())

	v = env.run(t, `msg.set('optional_string', ''); msg.has('optional_string')`)
	assert.True(t, v.ToBoolean())

	v = env.run(t, `msg.clear('optional_string'); msg.has('optional_string')`)
	assert.False(t, v.ToBoolean())

	// Message field: has returns false when not set.
	v = env.run(t, `msg.has('nested_val')`)
	assert.False(t, v.ToBoolean())

	v = env.run(t, `msg.set('nested_val', {value: 1}); msg.has('nested_val')`)
	assert.True(t, v.ToBoolean())

	v = env.run(t, `msg.clear('nested_val'); msg.has('nested_val')`)
	assert.False(t, v.ToBoolean())
}

func TestMessage_TypeAccessor(t *testing.T) {
	env := newTestEnv(t)

	v := env.run(t, `
		var msg = new (pb.messageType('test.SimpleMessage'))();
		msg.$type
	`)
	assert.Equal(t, "test.SimpleMessage", v.String())

	v = env.run(t, `
		var at = new (pb.messageType('test.AllTypes'))();
		at.$type
	`)
	assert.Equal(t, "test.AllTypes", v.String())
}

func TestMessage_NullClearsField(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.SimpleMessage'))();
		msg.set('name', 'hello');
		msg.set('value', 42);
	`)

	// Setting to null clears the field.
	v := env.run(t, `msg.set('name', null); msg.get('name')`)
	assert.Equal(t, "", v.String())

	// Setting to undefined also clears.
	v = env.run(t, `msg.set('value', undefined); msg.get('value')`)
	assert.Equal(t, int64(0), v.ToInteger())
}

func TestMessage_FieldNotFound(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.SimpleMessage'))()`)

	// get with unknown field should throw.
	env.mustFail(t, `msg.get('nonexistent')`)

	// set with unknown field should throw.
	env.mustFail(t, `msg.set('nonexistent', 42)`)

	// has with unknown field should throw.
	env.mustFail(t, `msg.has('nonexistent')`)

	// clear with unknown field should throw.
	env.mustFail(t, `msg.clear('nonexistent')`)
}

func TestMessage_DefaultValues(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	// Proto3 default values.
	assert.Equal(t, int64(0), env.run(t, `msg.get('int32_val')`).ToInteger())
	assert.Equal(t, int64(0), env.run(t, `msg.get('int64_val')`).ToInteger())
	assert.Equal(t, int64(0), env.run(t, `msg.get('uint32_val')`).ToInteger())
	assert.Equal(t, int64(0), env.run(t, `msg.get('uint64_val')`).ToInteger())
	assert.InDelta(t, 0.0, env.run(t, `msg.get('float_val')`).ToFloat(), 0.001)
	assert.InDelta(t, 0.0, env.run(t, `msg.get('double_val')`).ToFloat(), 0.001)
	assert.False(t, env.run(t, `msg.get('bool_val')`).ToBoolean())
	assert.Equal(t, "", env.run(t, `msg.get('string_val')`).String())
	assert.Equal(t, int64(0), env.run(t, `msg.get('enum_val')`).ToInteger())

	// Nested message field defaults to null.
	assert.True(t, goja.IsNull(env.run(t, `msg.get('nested_val')`)))
}
