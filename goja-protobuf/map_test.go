package gojaprotobuf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapField_GetSetHas(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.MapMessage'))();
		var tags = msg.get('tags');
	`)

	// Initially empty.
	v := env.run(t, `tags.size`)
	assert.Equal(t, int64(0), v.ToInteger())

	// Set a key.
	env.run(t, `tags.set('key1', 'val1')`)
	v = env.run(t, `tags.get('key1')`)
	assert.Equal(t, "val1", v.String())

	v = env.run(t, `tags.has('key1')`)
	assert.True(t, v.ToBoolean())

	v = env.run(t, `tags.has('nonexistent')`)
	assert.False(t, v.ToBoolean())

	// Get non-existent key returns undefined.
	v = env.run(t, `tags.get('nonexistent') === undefined`)
	assert.True(t, v.ToBoolean())

	// Multiple entries.
	env.run(t, `tags.set('key2', 'val2'); tags.set('key3', 'val3')`)
	v = env.run(t, `tags.size`)
	assert.Equal(t, int64(3), v.ToInteger())
}

func TestMapField_IntValues(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.MapMessage'))();
		var counts = msg.get('counts');
		counts.set('a', 10);
		counts.set('b', 20);
	`)

	v := env.run(t, `counts.get('a')`)
	assert.Equal(t, int64(10), v.ToInteger())

	v = env.run(t, `counts.get('b')`)
	assert.Equal(t, int64(20), v.ToInteger())
}

func TestMapField_Delete(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.MapMessage'))();
		var tags = msg.get('tags');
		tags.set('a', '1');
		tags.set('b', '2');
		tags.set('c', '3');
	`)

	v := env.run(t, `tags.size`)
	assert.Equal(t, int64(3), v.ToInteger())

	env.run(t, `tags.delete('b')`)
	v = env.run(t, `tags.size`)
	assert.Equal(t, int64(2), v.ToInteger())

	v = env.run(t, `tags.has('b')`)
	assert.False(t, v.ToBoolean())

	// Other entries still present.
	v = env.run(t, `tags.has('a')`)
	assert.True(t, v.ToBoolean())

	v = env.run(t, `tags.has('c')`)
	assert.True(t, v.ToBoolean())
}

func TestMapField_Size(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.MapMessage'))();
		var tags = msg.get('tags');
	`)

	v := env.run(t, `tags.size`)
	assert.Equal(t, int64(0), v.ToInteger())

	env.run(t, `tags.set('x', '1')`)
	v = env.run(t, `tags.size`)
	assert.Equal(t, int64(1), v.ToInteger())

	env.run(t, `tags.set('y', '2')`)
	v = env.run(t, `tags.size`)
	assert.Equal(t, int64(2), v.ToInteger())

	// Overwriting doesn't increment.
	env.run(t, `tags.set('x', 'updated')`)
	v = env.run(t, `tags.size`)
	assert.Equal(t, int64(2), v.ToInteger())
}

func TestMapField_ForEach(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.MapMessage'))();
		var tags = msg.get('tags');
		tags.set('alpha', 'A');
		tags.set('beta', 'B');
	`)

	// forEach passes (value, key) matching ES6 Map convention.
	v := env.run(t, `
		var result = {};
		tags.forEach(function(value, key) {
			result[key] = value;
		});
		result['alpha'] === 'A' && result['beta'] === 'B'
	`)
	assert.True(t, v.ToBoolean())
}

func TestMapField_ForEach_NonFunction(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.MapMessage'))();
		var tags = msg.get('tags');
	`)
	env.mustFail(t, `tags.forEach(42)`)
}

func TestMapField_Entries(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `
		var msg = new (pb.messageType('test.MapMessage'))();
		var tags = msg.get('tags');
		tags.set('one', '1');
		tags.set('two', '2');
	`)

	v := env.run(t, `
		var iter = tags.entries();
		var collected = {};
		var r = iter.next();
		while (!r.done) {
			collected[r.value[0]] = r.value[1];
			r = iter.next();
		}
		collected['one'] === '1' && collected['two'] === '2' && Object.keys(collected).length === 2
	`)
	assert.True(t, v.ToBoolean())
}

func TestMapField_SetViaObject(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.MapMessage'))()`)

	// Set map field from a plain JavaScript object via message set.
	v := env.run(t, `
		msg.set('tags', {hello: 'world', foo: 'bar'});
		var tags = msg.get('tags');
		tags.get('hello') === 'world' && tags.get('foo') === 'bar' && tags.size === 2
	`)
	assert.True(t, v.ToBoolean())
}
