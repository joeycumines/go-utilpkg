package eventloop

import (
	"errors"
	"testing"
)

func TestTrySynchronousSettlement(t *testing.T) {
	returnedError := errors.New("returned error")
	panicError := errors.New("panic error")
	tests := []struct {
		name      string
		callback  func() any
		wantState PromiseState
		wantValue any
	}{
		{name: "string fulfillment", callback: func() any { return "success" }, wantState: Fulfilled, wantValue: "success"},
		{name: "nil fulfillment", callback: func() any { return nil }, wantState: Fulfilled},
		{name: "returned error fulfillment", callback: func() any { return returnedError }, wantState: Fulfilled, wantValue: returnedError},
		{name: "string panic", callback: func() any { panic("test panic") }, wantState: Rejected, wantValue: "test panic"},
		{name: "error panic", callback: func() any { panic(panicError) }, wantState: Rejected, wantValue: panicError},
	}

	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			promise := js.Try(test.callback)
			if state := promise.State(); state != test.wantState {
				t.Fatalf("Try state = %v, want %v", state, test.wantState)
			}
			if test.wantState == Fulfilled {
				if value := promise.Value(); value != test.wantValue {
					t.Fatalf("Try value = %#v, want %#v", value, test.wantValue)
				}
				return
			}
			panicResult, ok := promise.Reason().(PanicError)
			if !ok || panicResult.Value != test.wantValue {
				t.Fatalf("Try reason = %T %#v, want PanicError containing %#v", promise.Reason(), promise.Reason(), test.wantValue)
			}
		})
	}
}
