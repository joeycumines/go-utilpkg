package gojaprotobuf

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBigInt_Int64(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	// Value outside safe integer range: 2^53 + 1 = 9007199254740993
	env.run(t, `msg.set('int64_val', BigInt('9007199254740993'))`)

	v := env.run(t, `msg.get('int64_val')`)
	exported := v.Export()

	bi, ok := exported.(*big.Int)
	require.True(t, ok, "expected *big.Int, got %T", exported)
	assert.Equal(t, "9007199254740993", bi.String())
}

func TestBigInt_Int64_SafeRange(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	// Value within safe integer range should be returned as number, not BigInt.
	env.run(t, `msg.set('int64_val', 42)`)
	v := env.run(t, `msg.get('int64_val')`)
	assert.Equal(t, int64(42), v.ToInteger())

	// Verify it's not a BigInt in JS.
	v = env.run(t, `typeof msg.get('int64_val') === 'number'`)
	assert.True(t, v.ToBoolean())
}

func TestBigInt_Int64_Negative(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	// Negative value outside safe integer range.
	env.run(t, `msg.set('int64_val', BigInt('-9007199254740993'))`)

	v := env.run(t, `msg.get('int64_val')`)
	exported := v.Export()

	bi, ok := exported.(*big.Int)
	require.True(t, ok, "expected *big.Int, got %T", exported)
	assert.Equal(t, "-9007199254740993", bi.String())
}

func TestBigInt_Uint64(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	// Large uint64: 2^53 + 1
	env.run(t, `msg.set('uint64_val', BigInt('9007199254740993'))`)

	v := env.run(t, `msg.get('uint64_val')`)
	exported := v.Export()

	bi, ok := exported.(*big.Int)
	require.True(t, ok, "expected *big.Int, got %T", exported)
	assert.Equal(t, "9007199254740993", bi.String())
}

func TestBigInt_Uint64_SafeRange(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	env.run(t, `msg.set('uint64_val', 42)`)
	v := env.run(t, `typeof msg.get('uint64_val') === 'number'`)
	assert.True(t, v.ToBoolean())
}

func TestOverflow_Int32(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	// MaxInt32 + 1 = 2147483648 should overflow.
	env.mustFail(t, `msg.set('int32_val', 2147483648)`)

	// MinInt32 - 1 = -2147483649 should overflow.
	env.mustFail(t, `msg.set('int32_val', -2147483649)`)

	// MaxInt32 is fine.
	env.run(t, `msg.set('int32_val', 2147483647)`)
	v := env.run(t, `msg.get('int32_val')`)
	assert.Equal(t, int64(2147483647), v.ToInteger())

	// MinInt32 is fine.
	env.run(t, `msg.set('int32_val', -2147483648)`)
	v = env.run(t, `msg.get('int32_val')`)
	assert.Equal(t, int64(-2147483648), v.ToInteger())
}

func TestOverflow_Uint32(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	// MaxUint32 + 1 = 4294967296 should overflow.
	env.mustFail(t, `msg.set('uint32_val', 4294967296)`)

	// Negative should fail.
	env.mustFail(t, `msg.set('uint32_val', -1)`)

	// MaxUint32 is fine.
	env.run(t, `msg.set('uint32_val', 4294967295)`)
	v := env.run(t, `msg.get('uint32_val')`)
	assert.Equal(t, int64(4294967295), v.ToInteger())
}

func TestOverflow_Uint64_Negative(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	// Negative BigInt for unsigned field should fail.
	env.mustFail(t, `msg.set('uint64_val', BigInt('-1'))`)
}

func TestEnumByName(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	// Set by string name.
	env.run(t, `msg.set('enum_val', 'FIRST')`)
	v := env.run(t, `msg.get('enum_val')`)
	assert.Equal(t, int64(1), v.ToInteger())

	env.run(t, `msg.set('enum_val', 'THIRD')`)
	v = env.run(t, `msg.get('enum_val')`)
	assert.Equal(t, int64(3), v.ToInteger())
}

func TestEnumByNumber(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	env.run(t, `msg.set('enum_val', 2)`)
	v := env.run(t, `msg.get('enum_val')`)
	assert.Equal(t, int64(2), v.ToInteger())

	env.run(t, `msg.set('enum_val', 0)`)
	v = env.run(t, `msg.get('enum_val')`)
	assert.Equal(t, int64(0), v.ToInteger())
}

func TestEnumByName_Invalid(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	// Unknown enum name should throw.
	env.mustFail(t, `msg.set('enum_val', 'NONEXISTENT')`)
}

func TestBoolCoercion(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.AllTypes'))()`)

	// Truthy values coerce to true.
	env.run(t, `msg.set('bool_val', 1)`)
	v := env.run(t, `msg.get('bool_val')`)
	assert.True(t, v.ToBoolean())

	// Falsy values coerce to false.
	env.run(t, `msg.set('bool_val', 0)`)
	v = env.run(t, `msg.get('bool_val')`)
	assert.False(t, v.ToBoolean())

	env.run(t, `msg.set('bool_val', '')`)
	v = env.run(t, `msg.get('bool_val')`)
	assert.False(t, v.ToBoolean())
}

func TestBigInt_RoundTrip_Encode_Decode(t *testing.T) {
	env := newTestEnv(t)

	// BigInt value should survive encode/decode.
	v := env.run(t, `
		var AT = pb.messageType('test.AllTypes');
		var msg = new AT();
		msg.set('int64_val', BigInt('9007199254740993'));

		var encoded = pb.encode(msg);
		var decoded = pb.decode(AT, encoded);
		var val = decoded.get('int64_val');
		typeof val === 'bigint' && val === BigInt('9007199254740993')
	`)
	assert.True(t, v.ToBoolean())
}
