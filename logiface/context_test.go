package logiface

import (
	"testing"
)

func TestBuilder_Call_nilReceiver(t *testing.T) {
	var called int
	if ((*Builder[Event])(nil)).Call(func(b *Builder[Event]) {
		if b != nil {
			t.Error()
		}
		called++
	}) != nil {
		t.Error()
	}
	if called != 1 {
		t.Error(called)
	}
}

func TestBuilder_Call(t *testing.T) {
	builder := &Builder[Event]{}
	var called int
	if b := builder.Call(func(b *Builder[Event]) {
		if b != builder {
			t.Error(b)
		}
		called++
	}); b != builder {
		t.Error(b)
	}
	if called != 1 {
		t.Error(called)
	}
}
