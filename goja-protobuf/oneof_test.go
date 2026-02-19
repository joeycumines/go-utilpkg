package gojaprotobuf

import (
	"testing"
)

func TestOneof_WhichOneof(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.OneofMessage'))()`)

	v := env.run(t, `msg.whichOneof('choice') === undefined`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}

	env.run(t, `msg.set('str_choice', 'hello')`)
	v = env.run(t, `msg.whichOneof('choice')`)
	if v.String() != "str_choice" {
		t.Errorf("got %v, want %v", v.String(), "str_choice")
	}

	env.run(t, `msg.set('int_choice', 42)`)
	v = env.run(t, `msg.whichOneof('choice')`)
	if v.String() != "int_choice" {
		t.Errorf("got %v, want %v", v.String(), "int_choice")
	}
}

func TestOneof_AutoClear(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.OneofMessage'))()`)

	env.run(t, `msg.set('str_choice', 'hello')`)
	v := env.run(t, `msg.get('str_choice')`)
	if v.String() != "hello" {
		t.Errorf("got %v, want %v", v.String(), "hello")
	}

	env.run(t, `msg.set('int_choice', 42)`)
	v = env.run(t, `msg.whichOneof('choice')`)
	if v.String() != "int_choice" {
		t.Errorf("got %v, want %v", v.String(), "int_choice")
	}

	v = env.run(t, `msg.get('str_choice')`)
	if v.String() != "" {
		t.Errorf("got %v, want %v", v.String(), "")
	}

	v = env.run(t, `msg.get('int_choice')`)
	if v.ToInteger() != int64(42) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(42))
	}
}

func TestOneof_ClearOneof(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.OneofMessage'))()`)

	env.run(t, `msg.set('str_choice', 'hello')`)
	v := env.run(t, `msg.whichOneof('choice')`)
	if v.String() != "str_choice" {
		t.Errorf("got %v, want %v", v.String(), "str_choice")
	}

	env.run(t, `msg.clearOneof('choice')`)
	v = env.run(t, `msg.whichOneof('choice') === undefined`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}

	v = env.run(t, `msg.get('str_choice')`)
	if v.String() != "" {
		t.Errorf("got %v, want %v", v.String(), "")
	}

	v = env.run(t, `msg.get('int_choice')`)
	if v.ToInteger() != int64(0) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(0))
	}
}

func TestOneof_ClearOneof_NoneSet(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.OneofMessage'))()`)

	env.run(t, `msg.clearOneof('choice')`)
	v := env.run(t, `msg.whichOneof('choice') === undefined`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

func TestOneof_NotFound(t *testing.T) {
	env := newTestEnv(t)
	env.run(t, `var msg = new (pb.messageType('test.OneofMessage'))()`)

	env.mustFail(t, `msg.whichOneof('nonexistent')`)
	env.mustFail(t, `msg.clearOneof('nonexistent')`)
}
