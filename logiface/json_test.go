package logiface

import (
	"fmt"
	"testing"
)

type (
	minimalJSONSupportMethods[E Event, O any, A any] interface {
		NewObject() O
		AddObject(evt E, key string, obj O)
		SetField(obj O, key string, val any) O
		NewArray() A
		AddArray(evt E, key string, arr A)
		AppendField(arr A, val any) A
	}
)

var (
	// compile time assertions

	_ iJSONSupport[Event]                       = (*UnimplementedJSONSupport[Event, map[string]any, []any])(nil)
	_ JSONSupport[Event, map[string]any, []any] = (*defaultJSONSupport[Event])(nil)
	_ JSONSupport[Event, map[string]any, []any] = struct {
		minimalJSONSupportMethods[Event, map[string]any, []any]
		UnimplementedJSONSupport[Event, map[string]any, []any]
	}{}
)

func TestUnimplementedJSONSupport_mustEmbedUnimplementedJSONSupport(t *testing.T) {
	(UnimplementedJSONSupport[Event, map[string]any, []any]{}).mustEmbedUnimplementedJSONSupport()
}

func TestSliceJSONSupport_mustEmbedUnimplementedJSONSupport(t *testing.T) {
	defaultJSONSupport[Event]{}.mustEmbedUnimplementedJSONSupport()
}

func TestUnimplementedJSONSupport_CanAppendArray(t *testing.T) {
	if (UnimplementedJSONSupport[Event, map[string]any, []any]{}).CanAppendArray() {
		t.Error("expected false")
	}
}

func TestUnimplementedJSONSupport_AppendArray(t *testing.T) {
	defer func() {
		if v := fmt.Sprint(recover()); v != `unimplemented` {
			t.Errorf("expected panic, got %q", v)
		}
	}()
	(UnimplementedJSONSupport[Event, map[string]any, []any]{}).AppendArray(nil, nil)
	t.Error("expected panic")
}
