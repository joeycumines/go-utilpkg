package gojaprotobuf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOneof_WhichOneof(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.OneofMessage'))()`)

	// Initially no oneof is set.
	v := env.run(t, `msg.whichOneof('choice') === undefined`)
	assert.True(t, v.ToBoolean())

	// Set str_choice.
	env.run(t, `msg.set('str_choice', 'hello')`)
	v = env.run(t, `msg.whichOneof('choice')`)
	assert.Equal(t, "str_choice", v.String())

	// Set int_choice.
	env.run(t, `msg.set('int_choice', 42)`)
	v = env.run(t, `msg.whichOneof('choice')`)
	assert.Equal(t, "int_choice", v.String())
}

func TestOneof_AutoClear(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.OneofMessage'))()`)

	// Set str_choice.
	env.run(t, `msg.set('str_choice', 'hello')`)
	v := env.run(t, `msg.get('str_choice')`)
	assert.Equal(t, "hello", v.String())

	// Setting int_choice should auto-clear str_choice.
	env.run(t, `msg.set('int_choice', 42)`)
	v = env.run(t, `msg.whichOneof('choice')`)
	assert.Equal(t, "int_choice", v.String())

	// str_choice should now return default value.
	v = env.run(t, `msg.get('str_choice')`)
	assert.Equal(t, "", v.String())

	v = env.run(t, `msg.get('int_choice')`)
	assert.Equal(t, int64(42), v.ToInteger())
}

func TestOneof_ClearOneof(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.OneofMessage'))()`)

	env.run(t, `msg.set('str_choice', 'hello')`)
	v := env.run(t, `msg.whichOneof('choice')`)
	assert.Equal(t, "str_choice", v.String())

	// clearOneof clears whichever field is set.
	env.run(t, `msg.clearOneof('choice')`)
	v = env.run(t, `msg.whichOneof('choice') === undefined`)
	assert.True(t, v.ToBoolean())

	// Both fields should be at default.
	v = env.run(t, `msg.get('str_choice')`)
	assert.Equal(t, "", v.String())

	v = env.run(t, `msg.get('int_choice')`)
	assert.Equal(t, int64(0), v.ToInteger())
}

func TestOneof_ClearOneof_NoneSet(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.OneofMessage'))()`)

	// clearOneof when nothing is set should not panic.
	env.run(t, `msg.clearOneof('choice')`)
	v := env.run(t, `msg.whichOneof('choice') === undefined`)
	assert.True(t, v.ToBoolean())
}

func TestOneof_NotFound(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.OneofMessage'))()`)

	// whichOneof with non-existent oneof name should throw.
	env.mustFail(t, `msg.whichOneof('nonexistent')`)

	// clearOneof with non-existent oneof name should throw.
	env.mustFail(t, `msg.clearOneof('nonexistent')`)
}
