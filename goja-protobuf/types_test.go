package gojaprotobuf

import (
	"testing"
)

func TestMessageType_Found(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(t, `typeof pb.messageType('test.SimpleMessage')`)
	if v.String() != "function" {
		t.Errorf("got %v, want %v", v.String(), "function")
	}
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
	if v.String() != "test.SimpleMessage" {
		t.Errorf("got %v, want %v", v.String(), "test.SimpleMessage")
	}
}

func TestMessageType_TypeName(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(t, `
		var SM = pb.messageType('test.SimpleMessage');
		SM.typeName
	`)
	if v.String() != "test.SimpleMessage" {
		t.Errorf("got %v, want %v", v.String(), "test.SimpleMessage")
	}
}

func TestMessageType_HasDescriptor(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(t, `
		var SM = pb.messageType('test.SimpleMessage');
		SM._pbMsgDesc !== undefined && SM._pbMsgDesc !== null
	`)
	if !v.ToBoolean() {
		t.Error("expected true")
	}
}

func TestEnumType_Found(t *testing.T) {
	env := newTestEnv(t)
	v := env.run(t, `typeof pb.enumType('test.TestEnum')`)
	if v.String() != "object" {
		t.Errorf("got %v, want %v", v.String(), "object")
	}
}

func TestEnumType_NotFound(t *testing.T) {
	env := newTestEnv(t)
	env.mustFail(t, `pb.enumType('nonexistent.Enum')`)
}

func TestEnumType_Mapping(t *testing.T) {
	env := newTestEnv(t)

	v := env.run(t, `
		var te = pb.enumType('test.TestEnum');
		te.UNKNOWN
	`)
	if v.ToInteger() != int64(0) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(0))
	}

	v = env.run(t, `te.FIRST`)
	if v.ToInteger() != int64(1) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(1))
	}

	v = env.run(t, `te.SECOND`)
	if v.ToInteger() != int64(2) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(2))
	}

	v = env.run(t, `te.THIRD`)
	if v.ToInteger() != int64(3) {
		t.Errorf("got %v, want %v", v.ToInteger(), int64(3))
	}

	v = env.run(t, `te[0]`)
	if v.String() != "UNKNOWN" {
		t.Errorf("got %v, want %v", v.String(), "UNKNOWN")
	}

	v = env.run(t, `te[1]`)
	if v.String() != "FIRST" {
		t.Errorf("got %v, want %v", v.String(), "FIRST")
	}

	v = env.run(t, `te[2]`)
	if v.String() != "SECOND" {
		t.Errorf("got %v, want %v", v.String(), "SECOND")
	}

	v = env.run(t, `te[3]`)
	if v.String() != "THIRD" {
		t.Errorf("got %v, want %v", v.String(), "THIRD")
	}
}
