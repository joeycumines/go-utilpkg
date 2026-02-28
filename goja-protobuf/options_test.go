package gojaprotobuf

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoregistry"
)

func TestResolveOptions_Empty(t *testing.T) {
	cfg, err := resolveOptions(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.resolver != nil {
		t.Errorf("expected nil resolver, got %v", cfg.resolver)
	}
	if cfg.files != nil {
		t.Errorf("expected nil files, got %v", cfg.files)
	}
}

func TestResolveOptions_NilOption(t *testing.T) {
	cfg, err := resolveOptions([]Option{nil, nil})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.resolver != nil {
		t.Errorf("expected nil resolver, got %v", cfg.resolver)
	}
	if cfg.files != nil {
		t.Errorf("expected nil files, got %v", cfg.files)
	}
}

func TestWithResolver(t *testing.T) {
	r := new(protoregistry.Types)
	cfg, err := resolveOptions([]Option{WithResolver(r)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.resolver != r {
		t.Errorf("got %v, want %v", cfg.resolver, r)
	}
	if cfg.files != nil {
		t.Errorf("expected nil files, got %v", cfg.files)
	}
}

func TestWithFiles(t *testing.T) {
	f := new(protoregistry.Files)
	cfg, err := resolveOptions([]Option{WithFiles(f)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.resolver != nil {
		t.Errorf("expected nil resolver, got %v", cfg.resolver)
	}
	if cfg.files != f {
		t.Errorf("got %v, want %v", cfg.files, f)
	}
}
