package gojaprotobuf

import (
	"testing"

	"github.com/joeycumines/goja"
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
	if !m.OwnsRuntime(rt) {
		t.Error("module should own its construction runtime")
	}
	if m.state.typesIdentity != protoregistry.GlobalTypes {
		t.Errorf("type identity = %v, want global registry", m.state.typesIdentity)
	}
	if m.state.filesIdentity != protoregistry.GlobalFiles {
		t.Errorf("file identity = %v, want global registry", m.state.filesIdentity)
	}
	if m.state.baseTypes == protoregistry.GlobalTypes {
		t.Error("base type registry was not snapshotted")
	}
	if m.state.baseFiles == protoregistry.GlobalFiles {
		t.Error("base file registry was not snapshotted")
	}
	if m.state.localTypes == nil {
		t.Error("expected non-nil localTypes")
	}
	if m.state.localFiles == nil {
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
	if m.state.typesIdentity != r {
		t.Errorf("type identity = %v, want %v", m.state.typesIdentity, r)
	}
	if m.state.filesIdentity != f {
		t.Errorf("file identity = %v, want %v", m.state.filesIdentity, f)
	}
	if m.state.baseTypes == r || m.state.baseFiles == f {
		t.Error("configured registries were retained as live resolver state")
	}
}

func TestOwnsRuntime(t *testing.T) {
	rt := goja.New()
	m, err := New(rt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !m.OwnsRuntime(rt) {
		t.Error("OwnsRuntime did not recognize the construction runtime")
	}
	if m.OwnsRuntime(goja.New()) || m.OwnsRuntime(nil) {
		t.Error("OwnsRuntime accepted a foreign or nil runtime")
	}
}
