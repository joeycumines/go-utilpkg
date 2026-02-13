package gojaprotobuf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRepeatedField_AddGetLength(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.RepeatedMessage'))();
		var items = msg.get('items');
	`)

	// Initially empty.
	v := env.run(t, `items.length`)
	assert.Equal(t, int64(0), v.ToInteger())

	// Add elements.
	env.run(t, `items.add('hello'); items.add('world'); items.add('foo')`)

	v = env.run(t, `items.length`)
	assert.Equal(t, int64(3), v.ToInteger())

	v = env.run(t, `items.get(0)`)
	assert.Equal(t, "hello", v.String())

	v = env.run(t, `items.get(1)`)
	assert.Equal(t, "world", v.String())

	v = env.run(t, `items.get(2)`)
	assert.Equal(t, "foo", v.String())
}

func TestRepeatedField_Numbers(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.RepeatedMessage'))();
		var nums = msg.get('numbers');
		nums.add(10); nums.add(20); nums.add(30);
	`)

	v := env.run(t, `nums.length`)
	assert.Equal(t, int64(3), v.ToInteger())

	v = env.run(t, `nums.get(0)`)
	assert.Equal(t, int64(10), v.ToInteger())

	v = env.run(t, `nums.get(2)`)
	assert.Equal(t, int64(30), v.ToInteger())
}

func TestRepeatedField_Set(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.RepeatedMessage'))();
		var items = msg.get('items');
		items.add('a'); items.add('b'); items.add('c');
	`)

	// Replace element at index 1.
	v := env.run(t, `items.set(1, 'B'); items.get(1)`)
	assert.Equal(t, "B", v.String())

	// Other elements unaffected.
	v = env.run(t, `items.get(0)`)
	assert.Equal(t, "a", v.String())

	v = env.run(t, `items.get(2)`)
	assert.Equal(t, "c", v.String())
}

func TestRepeatedField_Clear(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.RepeatedMessage'))();
		var items = msg.get('items');
		items.add('x'); items.add('y');
	`)

	v := env.run(t, `items.length`)
	assert.Equal(t, int64(2), v.ToInteger())

	env.run(t, `items.clear()`)
	v = env.run(t, `items.length`)
	assert.Equal(t, int64(0), v.ToInteger())
}

func TestRepeatedField_ForEach(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.RepeatedMessage'))();
		var items = msg.get('items');
		items.add('a'); items.add('b'); items.add('c');
	`)

	v := env.run(t, `
		var collected = [];
		var indices = [];
		items.forEach(function(val, idx) {
			collected.push(val);
			indices.push(idx);
		});
		collected.join(',') + '|' + indices.join(',')
	`)
	assert.Equal(t, "a,b,c|0,1,2", v.String())
}

func TestRepeatedField_OutOfBounds(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.RepeatedMessage'))();
		var items = msg.get('items');
		items.add('only');
	`)

	// get with out-of-bounds index returns undefined.
	v := env.run(t, `items.get(5) === undefined`)
	assert.True(t, v.ToBoolean())

	v = env.run(t, `items.get(-1) === undefined`)
	assert.True(t, v.ToBoolean())

	// set with out-of-bounds index throws.
	env.mustFail(t, `items.set(5, 'bad')`)
	env.mustFail(t, `items.set(-1, 'bad')`)
}

func TestRepeatedField_SetViaArray(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.RepeatedMessage'))()`)

	// Set repeated field via a JS array using the message set method.
	v := env.run(t, `
		msg.set('items', ['alpha', 'beta', 'gamma']);
		var items = msg.get('items');
		items.length
	`)
	assert.Equal(t, int64(3), v.ToInteger())

	v = env.run(t, `items.get(0)`)
	assert.Equal(t, "alpha", v.String())

	v = env.run(t, `items.get(2)`)
	assert.Equal(t, "gamma", v.String())
}

func TestRepeatedField_ForEachNonFunction(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.RepeatedMessage'))();
		var items = msg.get('items');
	`)
	env.mustFail(t, `items.forEach('not a function')`)
}
