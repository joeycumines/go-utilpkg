package gojaprotobuf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMessageType_Found(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(t, `typeof pb.messageType('test.SimpleMessage')`)
	assert.Equal(t, "function", v.String())
}

func TestMessageType_NotFound(t *testing.T) {
	env := newTestEnv(t)
	env.mustFail(t, `pb.messageType('nonexistent.Type')`)
}

func TestMessageType_Constructor(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(t, `
		var SM = pb.messageType('test.SimpleMessage');
		var msg = new SM();
		msg.$type
	`)
	assert.Equal(t, "test.SimpleMessage", v.String())
}

func TestMessageType_TypeName(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(t, `
		var SM = pb.messageType('test.SimpleMessage');
		SM.typeName
	`)
	assert.Equal(t, "test.SimpleMessage", v.String())
}

func TestMessageType_HasDescriptor(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(t, `
		var SM = pb.messageType('test.SimpleMessage');
		SM._pbMsgDesc !== undefined && SM._pbMsgDesc !== null
	`)
	assert.True(t, v.ToBoolean())
}

func TestEnumType_Found(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(t, `typeof pb.enumType('test.TestEnum')`)
	assert.Equal(t, "object", v.String())
}

func TestEnumType_NotFound(t *testing.T) {
	env := newTestEnv(t)
	env.mustFail(t, `pb.enumType('nonexistent.Enum')`)
}

func TestEnumType_Mapping(t *testing.T) {
	env := newTestEnv(t)

	// name → number
	v := env.run(t, `
		var te = pb.enumType('test.TestEnum');
		te.UNKNOWN
	`)
	assert.Equal(t, int64(0), v.ToInteger())

	v = env.run(t, `te.FIRST`)
	assert.Equal(t, int64(1), v.ToInteger())

	v = env.run(t, `te.SECOND`)
	assert.Equal(t, int64(2), v.ToInteger())

	v = env.run(t, `te.THIRD`)
	assert.Equal(t, int64(3), v.ToInteger())

	// number → name (reverse mapping)
	v = env.run(t, `te[0]`)
	assert.Equal(t, "UNKNOWN", v.String())

	v = env.run(t, `te[1]`)
	assert.Equal(t, "FIRST", v.String())

	v = env.run(t, `te[2]`)
	assert.Equal(t, "SECOND", v.String())

	v = env.run(t, `te[3]`)
	assert.Equal(t, "THIRD", v.String())
}
