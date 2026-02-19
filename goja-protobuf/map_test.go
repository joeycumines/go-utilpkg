package gojaprotobuf

import "testing"

func TestMapField_GetSetHas(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.MapMessage'))(); var tags = msg.get('tags');`)

	v := env.run(t, `tags.size`)
	if v.ToInteger() != int64(0) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(0))
	}

	env.run(t, `tags.set('key1', 'val1')`)
	v = env.run(t, `tags.get('key1')`)
	if v.String() != "val1" {
		t.Errorf("got %v, want %v", v.String(), "val1")
	}

	v = env.run(t, `tags.has('key1')`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}

	v = env.run(t, `tags.has('nonexistent')`)
	if v.ToBoolean() {
		t.Error("expected false")
	}

	v = env.run(t, `tags.get('nonexistent') === undefined`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}

	env.run(t, `tags.set('key2', 'val2'); tags.set('key3', 'val3')`)
	v = env.run(t, `tags.size`)
	if v.ToInteger() != int64(3) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(3))
	}
}

func TestMapField_IntValues(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.MapMessage'))(); var counts = msg.get('counts'); counts.set('a', 10); counts.set('b', 20);`)

	v := env.run(t, `counts.get('a')`)
	if v.ToInteger() != int64(10) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(10))
	}

	v = env.run(t, `counts.get('b')`)
	if v.ToInteger() != int64(20) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(20))
	}
}

func TestMapField_Delete(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.MapMessage'))(); var tags = msg.get('tags'); tags.set('a', '1'); tags.set('b', '2'); tags.set('c', '3');`)

	v := env.run(t, `tags.size`)
	if v.ToInteger() != int64(3) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(3))
	}

	env.run(t, `tags.delete('b')`)
	v = env.run(t, `tags.size`)
	if v.ToInteger() != int64(2) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(2))
	}

	v = env.run(t, `tags.has('b')`)
	if v.ToBoolean() {
		t.Error("expected false")
	}

	v = env.run(t, `tags.has('a')`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}

	v = env.run(t, `tags.has('c')`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

func TestMapField_Size(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.MapMessage'))(); var tags = msg.get('tags');`)

	v := env.run(t, `tags.size`)
	if v.ToInteger() != int64(0) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(0))
	}

	env.run(t, `tags.set('x', '1')`)
	v = env.run(t, `tags.size`)
	if v.ToInteger() != int64(1) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(1))
	}

	env.run(t, `tags.set('y', '2')`)
	v = env.run(t, `tags.size`)
	if v.ToInteger() != int64(2) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(2))
	}

	env.run(t, `tags.set('x', 'updated')`)
	v = env.run(t, `tags.size`)
	if v.ToInteger() != int64(2) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(2))
	}
}

func TestMapField_ForEach(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.MapMessage'))(); var tags = msg.get('tags'); tags.set('alpha', 'A'); tags.set('beta', 'B');`)
	v := env.run(t, `var result = {}; tags.forEach(function(value, key) { result[key] = value; }); result['alpha'] === 'A' && result['beta'] === 'B'`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

func TestMapField_ForEach_NonFunction(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.MapMessage'))(); var tags = msg.get('tags');`)
	env.mustFail(t, `tags.forEach(42)`)
}

func TestMapField_Entries(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.MapMessage'))(); var tags = msg.get('tags'); tags.set('one', '1'); tags.set('two', '2');`)
	v := env.run(t, `var iter = tags.entries(); var collected = {}; var r = iter.next(); while (!r.done) { collected[r.value[0]] = r.value[1]; r = iter.next(); } collected['one'] === '1' && collected['two'] === '2' && Object.keys(collected).length === 2`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

func TestMapField_SetViaObject(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.MapMessage'))()`)
	v := env.run(t, `msg.set('tags', {hello: 'world', foo: 'bar'}); var tags = msg.get('tags'); tags.get('hello') === 'world' && tags.get('foo') === 'bar' && tags.size === 2`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}
