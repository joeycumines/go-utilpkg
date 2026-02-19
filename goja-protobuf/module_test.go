package gojaprotobuf

import (
	"testing"

	"github.com/dop251/goja"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func TestNew_NilRuntime_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		if r != "gojaprotobuf: runtime must not be nil" {
			t.Fatalf("unexpected panic value: %v", r)
		}
	}()
	_, _ = New(nil)
}

func TestNew_Default(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil module")
	}
	if m.Runtime() != rt {
		t.Errorf("got %v, want %v", m.Runtime(), rt)
	}
	if m.resolver != protoregistry.GlobalTypes {
		t.Errorf("got %v, want %v", m.resolver, protoregistry.GlobalTypes)
	}
	if m.files != protoregistry.GlobalFiles {
		t.Errorf("got %v, want %v", m.files, protoregistry.GlobalFiles)
	}
	if m.localTypes == nil {
		t.Error("expected non-nil localTypes")
	}
	if m.localFiles == nil {
		t.Error("expected non-nil localFiles")
	}
}

func TestNew_WithOptions(t *testing.T) {
	rt := goja.New()
	r := new(protoregistry.Types)
	f := new(protoregistry.Files)

	m, err := New(rt, WithResolver(r), WithFiles(f))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.resolver != r {
		t.Errorf("got %v, want %v", m.resolver, r)
	}
	if m.files != f {
		t.Errorf("got %v, want %v", m.files, f)
	}
}

func TestRuntime_Accessor(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Runtime() != rt {
		t.Error("Runtime() did not return the same runtime pointer")
	}
}
