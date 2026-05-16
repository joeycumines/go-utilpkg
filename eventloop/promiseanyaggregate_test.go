package eventloop

import (
	"reflect"
	"testing"
)

func TestPromiseAny_NonErrorRejections(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop, WithUnhandledRejection(func(any) {}))
	if err != nil {
		t.Fatal(err)
	}

	first, _, rejectFirst := js.NewChainedPromise()
	second, _, rejectSecond := js.NewChainedPromise()
	result := js.Any([]*ChainedPromise{first, second})
	rejectFirst("string-error")
	rejectSecond(42)
	loop.tick()

	if result.State() != Rejected {
		t.Fatalf("state = %v, want Rejected", result.State())
	}
	aggregate, ok := result.Reason().(*AggregateError)
	if !ok {
		t.Fatalf("reason type = %T, want *AggregateError", result.Reason())
	}
	want := []any{"string-error", 42}
	if !reflect.DeepEqual(aggregate.Errors, want) {
		t.Fatalf("errors = %#v, want %#v", aggregate.Errors, want)
	}
}
