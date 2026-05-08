package gojaprotobuf

import (
	"fmt"
	"strings"
	"testing"

	"github.com/joeycumines/goja"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func TestResolveOptions_Defaults(t *testing.T) {
	cfg, err := resolveOptions(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.resolver != protoregistry.GlobalTypes {
		t.Errorf("resolver = %v, want GlobalTypes", cfg.resolver)
	}
	if cfg.files != protoregistry.GlobalFiles {
		t.Errorf("files = %v, want GlobalFiles", cfg.files)
	}
}

func TestResolveOptions_NilOption(t *testing.T) {
	if _, err := resolveOptions([]ModuleOption{nil}); err == nil {
		t.Fatal("nil module option was accepted")
	}
}

func TestWithResolver(t *testing.T) {
	r := new(protoregistry.Types)
	cfg, err := resolveOptions([]ModuleOption{WithResolver(r)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.resolver != r {
		t.Errorf("got %v, want %v", cfg.resolver, r)
	}
	if cfg.files != protoregistry.GlobalFiles {
		t.Errorf("files = %v, want GlobalFiles", cfg.files)
	}
}

func TestWithFiles(t *testing.T) {
	f := new(protoregistry.Files)
	cfg, err := resolveOptions([]ModuleOption{WithFiles(f)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.resolver != protoregistry.GlobalTypes {
		t.Errorf("resolver = %v, want GlobalTypes", cfg.resolver)
	}
	if cfg.files != f {
		t.Errorf("got %v, want %v", cfg.files, f)
	}
}

func TestResolveOptionsRejectsTypedNilAndNilValues(t *testing.T) {
	tests := []struct {
		name   string
		option ModuleOption
	}{
		{name: "typed nil resolver", option: (*ResolverOption)(nil)},
		{name: "typed nil files", option: (*FilesOption)(nil)},
		{name: "nil resolver", option: WithResolver(nil)},
		{name: "nil files", option: WithFiles(nil)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := resolveOptions([]ModuleOption{test.option}); err == nil {
				t.Fatal("invalid module option was accepted")
			}
		})
	}
}

func TestNewPanicsOnInvalidOptions(t *testing.T) {
	tests := []struct {
		name   string
		option ModuleOption
		want   string
	}{
		{
			name: "nil option",
			want: "module option 0 is nil",
		},
		{
			name:   "typed nil resolver",
			option: (*ResolverOption)(nil),
			want:   "resolver option is nil",
		},
		{
			name:   "typed nil files",
			option: (*FilesOption)(nil),
			want:   "files option is nil",
		},
		{
			name:   "nil resolver",
			option: WithResolver(nil),
			want:   "resolver must not be nil",
		},
		{
			name:   "nil files",
			option: WithFiles(nil),
			want:   "files registry must not be nil",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recovered := capturePanic(t, func() {
				_, _ = New(goja.New(), test.option)
			})
			if got := fmt.Sprint(recovered); !strings.Contains(got, test.want) {
				t.Fatalf("panic = %q, want substring %q", got, test.want)
			}
		})
	}
}
