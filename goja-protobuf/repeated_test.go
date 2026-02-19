package gojaprotobuf

import "testing"

func TestRepeatedField_AddGetLength(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.RepeatedMessage'))(); var items = msg.get('items');`)
	v := env.run(t, `items.length`)
	if v.ToInteger() != int64(0) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(0))
	}
	env.run(t, `items.add('hello'); items.add('world'); items.add('foo')`)
	v = env.run(t, `items.length`)
	if v.ToInteger() != int64(3) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(3))
	}
	v = env.run(t, `items.get(0)`)
	if v.String() != "hello" {
		t.Errorf("got %v, want %v", v.String(), "hello")
	}
	v = env.run(t, `items.get(1)`)
	if v.String() != "world" {
		t.Errorf("got %v, want %v", v.String(), "world")
	}
	v = env.run(t, `items.get(2)`)
	if v.String() != "foo" {
		t.Errorf("got %v, want %v", v.String(), "foo")
	}
}

func TestRepeatedField_Numbers(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.RepeatedMessage'))(); var nums = msg.get('numbers'); nums.add(10); nums.add(20); nums.add(30);`)
	v := env.run(t, `nums.length`)
	if v.ToInteger() != int64(3) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(3))
	}
	v = env.run(t, `nums.get(0)`)
	if v.ToInteger() != int64(10) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(10))
	}
	v = env.run(t, `nums.get(2)`)
	if v.ToInteger() != int64(30) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(30))
	}
}

func TestRepeatedField_Set(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.RepeatedMessage'))(); var items = msg.get('items'); items.add('a'); items.add('b'); items.add('c');`)
	v := env.run(t, `items.set(1, 'B'); items.get(1)`)
	if v.String() != "B" {
		t.Errorf("got %v, want %v", v.String(), "B")
	}
	v = env.run(t, `items.get(0)`)
	if v.String() != "a" {
		t.Errorf("got %v, want %v", v.String(), "a")
	}
	v = env.run(t, `items.get(2)`)
	if v.String() != "c" {
		t.Errorf("got %v, want %v", v.String(), "c")
	}
}

func TestRepeatedField_Clear(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.RepeatedMessage'))(); var items = msg.get('items'); items.add('x'); items.add('y');`)
	v := env.run(t, `items.length`)
	if v.ToInteger() != int64(2) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(2))
	}
	env.run(t, `items.clear()`)
	v = env.run(t, `items.length`)
	if v.ToInteger() != int64(0) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(0))
	}
}

func TestRepeatedField_ForEach(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.RepeatedMessage'))(); var items = msg.get('items'); items.add('a'); items.add('b'); items.add('c');`)
	v := env.run(t, `var collected = []; var indices = []; items.forEach(function(val, idx) { collected.push(val); indices.push(idx); }); collected.join(',') + '|' + indices.join(',')`)
	if v.String() != "a,b,c|0,1,2" {
		t.Errorf("got %v, want %v", v.String(), "a,b,c|0,1,2")
	}
}

func TestRepeatedField_OutOfBounds(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.RepeatedMessage'))(); var items = msg.get('items'); items.add('only');`)
	v := env.run(t, `items.get(5) === undefined`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}
	v = env.run(t, `items.get(-1) === undefined`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}
	env.mustFail(t, `items.set(5, 'bad')`)
	env.mustFail(t, `items.set(-1, 'bad')`)
}

func TestRepeatedField_SetViaArray(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.RepeatedMessage'))()`)
	v := env.run(t, `msg.set('items', ['alpha', 'beta', 'gamma']); var items = msg.get('items'); items.length`)
	if v.ToInteger() != int64(3) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(3))
	}
	v = env.run(t, `items.get(0)`)
	if v.String() != "alpha" {
		t.Errorf("got %v, want %v", v.String(), "alpha")
	}
	v = env.run(t, `items.get(2)`)
	if v.String() != "gamma" {
		t.Errorf("got %v, want %v", v.String(), "gamma")
	}
}

func TestRepeatedField_ForEachNonFunction(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.RepeatedMessage'))(); var items = msg.get('items');`)
	env.mustFail(t, `items.forEach('not a function')`)
}
