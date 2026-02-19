package gojaprotobuf

import (
	"math"
	"testing"

	"github.com/dop251/goja"
)

func TestMessage_GetSet_Scalars(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	// int32
	v := env.run(t, `msg.set('int32_val', 42); msg.get('int32_val')`)
	if v.ToInteger() != int64(42) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(42))
	}

	// int64
	v = env.run(t, `msg.set('int64_val', 100); msg.get('int64_val')`)
	if v.ToInteger() != int64(100) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(100))
	}

	// uint32
	v = env.run(t, `msg.set('uint32_val', 200); msg.get('uint32_val')`)
	if v.ToInteger() != int64(200) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(200))
	}

	// uint64
	v = env.run(t, `msg.set('uint64_val', 300); msg.get('uint64_val')`)
	if v.ToInteger() != int64(300) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(300))
	}

	// float (uses exact float32 value)
	v = env.run(t, `msg.set('float_val', 1.5); msg.get('float_val')`)
	if math.Abs(v.ToFloat()-1.5) > 0.001 {
		t.Errorf("got %v, want %v (delta 0.001)", v.ToFloat(), 1.5)
	}

	// double
	v = env.run(t, `msg.set('double_val', 3.14159); msg.get('double_val')`)
	if math.Abs(v.ToFloat()-3.14159) > 0.00001 {
		t.Errorf("got %v, want %v (delta 0.00001)", v.ToFloat(), 3.14159)
	}

	// bool
	v = env.run(t, `msg.set('bool_val', true); msg.get('bool_val')`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}

	// sint32
	v = env.run(t, `msg.set('sint32_val', -42); msg.get('sint32_val')`)
	if v.ToInteger() != int64(-42) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(-42))
	}

	// sint64
	v = env.run(t, `msg.set('sint64_val', -100); msg.get('sint64_val')`)
	if v.ToInteger() != int64(-100) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(-100))
	}

	// fixed32
	v = env.run(t, `msg.set('fixed32_val', 500); msg.get('fixed32_val')`)
	if v.ToInteger() != int64(500) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(500))
	}

	// fixed64
	v = env.run(t, `msg.set('fixed64_val', 600); msg.get('fixed64_val')`)
	if v.ToInteger() != int64(600) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(600))
	}

	// sfixed32
	v = env.run(t, `msg.set('sfixed32_val', -500); msg.get('sfixed32_val')`)
	if v.ToInteger() != int64(-500) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(-500))
	}

	// sfixed64
	v = env.run(t, `msg.set('sfixed64_val', -600); msg.get('sfixed64_val')`)
	if v.ToInteger() != int64(-600) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(-600))
	}
}

func TestMessage_GetSet_String(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.SimpleMessage'))()`)

	v := env.run(t, `msg.set('name', 'hello world'); msg.get('name')`)
	if v.String() != "hello world" {
		t.Errorf("got %v, want %v", v.String(), "hello world")
	}

	// Empty string.
	v = env.run(t, `msg.set('name', ''); msg.get('name')`)
	if v.String() != "" {
		t.Errorf("got %v, want %v", v.String(), "")
	}

	// Unicode.
	v = env.run(t, `msg.set('name', '日本語テスト'); msg.get('name')`)
	if v.String() != "日本語テスト" {
		t.Errorf("got %v, want %v", v.String(), "日本語テスト")
	}
}

func TestMessage_GetSet_Bytes(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	v := env.run(t, `
		msg.set('bytes_val', new Uint8Array([1, 2, 3, 255]));
		var b = msg.get('bytes_val');
		b[0] === 1 && b[1] === 2 && b[2] === 3 && b[3] === 255 && b.length === 4
	`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}

	// Empty bytes.
	v = env.run(t, `
		msg.set('bytes_val', new Uint8Array([]));
		msg.get('bytes_val').length === 0
	`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

func TestMessage_GetSet_Enum(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	// Set by number.
	v := env.run(t, `msg.set('enum_val', 1); msg.get('enum_val')`)
	if v.ToInteger() != int64(1) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(1))
	}

	// Set by name.
	v = env.run(t, `msg.set('enum_val', 'SECOND'); msg.get('enum_val')`)
	if v.ToInteger() != int64(2) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(2))
	}

	// Default value.
	v = env.run(t, `msg.set('enum_val', 0); msg.get('enum_val')`)
	if v.ToInteger() != int64(0) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(0))
	}
}

func TestMessage_GetSet_NestedMessage(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.NestedOuter'))()`)

	// Unset message field returns null.
	v := env.run(t, `msg.get('nested_inner') === null`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}

	// Set nested message via a plain object.
	v = env.run(t, `
		msg.set('nested_inner', {value: 42});
		var inner = msg.get('nested_inner');
		inner.get('value')
	`)
	if v.ToInteger() != int64(42) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(42))
	}

	// Set nested message via a wrapped message.
	v = env.run(t, `
		var NI = pb.messageType('test.NestedInner');
		var ni = new NI();
		ni.set('value', 99);
		msg.set('nested_inner', ni);
		msg.get('nested_inner').get('value')
	`)
	if v.ToInteger() != int64(99) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(99))
	}

	// Clear nested message with null.
	v = env.run(t, `
		msg.set('nested_inner', null);
		msg.get('nested_inner') === null
	`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

func TestMessage_Has_Clear(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	// Proto3 scalar: has returns false for default value (0 for int32).
	v := env.run(t, `msg.has('int32_val')`)
	if v.ToBoolean() {
		t.Error("expected false")
	}

	// After setting to non-default, has returns true.
	v = env.run(t, `msg.set('int32_val', 42); msg.has('int32_val')`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}

	// Clear resets to default; has returns false.
	v = env.run(t, `msg.clear('int32_val'); msg.has('int32_val')`)
	if v.ToBoolean() {
		t.Error("expected false")
	}

	// Proto3 string: has returns false for default ("").
	v = env.run(t, `msg.has('string_val')`)
	if v.ToBoolean() {
		t.Error("expected false")
	}

	v = env.run(t, `msg.set('string_val', 'hello'); msg.has('string_val')`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}

	v = env.run(t, `msg.clear('string_val'); msg.has('string_val')`)
	if v.ToBoolean() {
		t.Error("expected false")
	}

	// Proto3 optional (explicit presence): has returns true even for default.
	v = env.run(t, `msg.has('optional_string')`)
	if v.ToBoolean() {
		t.Error("expected false")
	}

	v = env.run(t, `msg.set('optional_string', ''); msg.has('optional_string')`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}

	v = env.run(t, `msg.clear('optional_string'); msg.has('optional_string')`)
	if v.ToBoolean() {
		t.Error("expected false")
	}

	// Message field: has returns false when not set.
	v = env.run(t, `msg.has('nested_val')`)
	if v.ToBoolean() {
		t.Error("expected false")
	}

	v = env.run(t, `msg.set('nested_val', {value: 1}); msg.has('nested_val')`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}

	v = env.run(t, `msg.clear('nested_val'); msg.has('nested_val')`)
	if v.ToBoolean() {
		t.Error("expected false")
	}
}

func TestMessage_TypeAccessor(t *testing.T) {
	env := newTestEnv(t)

	v := env.run(t, `
		var msg = new (pb.messageType('test.SimpleMessage'))();
		msg.$type
	`)
	if v.String() != "test.SimpleMessage" {
		t.Errorf("got %v, want %v", v.String(), "test.SimpleMessage")
	}

	v = env.run(t, `
		var at = new (pb.messageType('test.AllTypes'))();
		at.$type
	`)
	if v.String() != "test.AllTypes" {
		t.Errorf("got %v, want %v", v.String(), "test.AllTypes")
	}
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
	if v.String() != "" {
		t.Errorf("got %v, want %v", v.String(), "")
	}

	// Setting to undefined also clears.
	v = env.run(t, `msg.set('value', undefined); msg.get('value')`)
	if v.ToInteger() != int64(0) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(0))
	}
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
	if env.run(t, `msg.get('int32_val')`).ToInteger() != int64(0) {
		t.Errorf("expected int32 default 0")
	}
	if env.run(t, `msg.get('int64_val')`).ToInteger() != int64(0) {
		t.Errorf("expected int64 default 0")
	}
	if env.run(t, `msg.get('uint32_val')`).ToInteger() != int64(0) {
		t.Errorf("expected uint32 default 0")
	}
	if env.run(t, `msg.get('uint64_val')`).ToInteger() != int64(0) {
		t.Errorf("expected uint64 default 0")
	}
	if math.Abs(env.run(t, `msg.get('float_val')`).ToFloat()-0.0) > 0.001 {
		t.Errorf("expected float default 0.0")
	}
	if math.Abs(env.run(t, `msg.get('double_val')`).ToFloat()-0.0) > 0.001 {
		t.Errorf("expected double default 0.0")
	}
	if env.run(t, `msg.get('bool_val')`).ToBoolean() {
		t.Error("expected false")
	}
	if env.run(t, `msg.get('string_val')`).String() != "" {
		t.Errorf("expected empty string")
	}
	if env.run(t, `msg.get('enum_val')`).ToInteger() != int64(0) {
		t.Errorf("expected enum default 0")
	}

	// Nested message field defaults to null.
	if !goja.IsNull(env.run(t, `msg.get('nested_val')`)) {
		t.Error("expected null for unset message field")
	}
}
