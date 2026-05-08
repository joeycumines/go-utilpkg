package eventloop

import (
	"errors"
	"testing"
)

func TestAplus_2_2_1_ThenCallbacksOptional(t *testing.T) {
	loop, js := newPromiseProfileJS(t)
	parent, resolve, _ := js.NewChainedPromise()
	nilCallbacks := parent.Then(nil, nil)
	fulfillmentCallback := parent.Then(func(value any) any { return value }, nil)
	rejectionCallback := parent.Then(nil, func(reason any) any { return reason })

	resolve("value")
	loop.tick()
	for name, promise := range map[string]*ChainedPromise{
		"nil callbacks":        nilCallbacks,
		"fulfillment callback": fulfillmentCallback,
		"rejection callback":   rejectionCallback,
	} {
		if promise.State() != Fulfilled || promise.Value() != "value" {
			t.Fatalf("%s result = (%v, %#v), want Fulfilled %q", name, promise.State(), promise.Value(), "value")
		}
	}
}

func TestAplus_2_2_2_OnFulfilledCalledAfterFulfilled(t *testing.T) {
	loop, js := newPromiseProfileJS(t)
	parent, resolve, _ := js.NewChainedPromise()
	callCount := 0
	var received any
	parent.Then(func(value any) any {
		callCount++
		received = value
		return value
	}, nil)
	if callCount != 0 {
		t.Fatalf("callback count before fulfillment = %d, want 0", callCount)
	}

	resolve("the value")
	loop.tick()
	if callCount != 1 || received != "the value" {
		t.Fatalf("callback observation = (%d, %#v), want (1, %q)", callCount, received, "the value")
	}
}

func TestAplus_2_2_3_OnRejectedCalledAfterRejected(t *testing.T) {
	loop, js := newPromiseProfileJS(t)
	parent, _, reject := js.NewChainedPromise()
	callCount := 0
	var received any
	parent.Then(nil, func(reason any) any {
		callCount++
		received = reason
		return nil
	})
	if callCount != 0 {
		t.Fatalf("callback count before rejection = %d, want 0", callCount)
	}

	want := errors.New("the reason")
	reject(want)
	loop.tick()
	if callCount != 1 || received != want {
		t.Fatalf("callback observation = (%d, %#v), want (1, exact reason)", callCount, received)
	}
}

func TestAplus_2_2_4_Asynchronous(t *testing.T) {
	loop, js := newPromiseProfileJS(t)
	parent, resolve, _ := js.NewChainedPromise()
	order := make([]int, 0, 3)
	parent.Then(func(value any) any {
		order = append(order, 2)
		return value
	}, nil)

	order = append(order, 1)
	resolve("value")
	order = append(order, 3)
	if len(order) != 2 || order[0] != 1 || order[1] != 3 {
		t.Fatalf("order before checkpoint = %v, want [1 3]", order)
	}
	loop.tick()
	if len(order) != 3 || order[2] != 2 {
		t.Fatalf("order after checkpoint = %v, want [1 3 2]", order)
	}
}

func TestAplus_2_2_6_MultipleHandlersOrder(t *testing.T) {
	loop, js := newPromiseProfileJS(t)
	parent, resolve, _ := js.NewChainedPromise()
	order := make([]int, 0, 3)
	for index := 1; index <= 3; index++ {
		value := index
		parent.Then(func(any) any {
			order = append(order, value)
			return nil
		}, nil)
	}

	resolve("value")
	loop.tick()
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Fatalf("handler order = %v, want [1 2 3]", order)
	}
}

func TestAplus_2_2_7_ThenReturnsNewPromise(t *testing.T) {
	loop, js := newPromiseProfileJS(t)
	parent, resolve, _ := js.NewChainedPromise()
	child := parent.Then(func(value any) any { return value }, nil)
	if child == nil || child == parent {
		t.Fatalf("Then child = %p, parent = %p; want distinct non-nil promise", child, parent)
	}

	resolve("value")
	loop.tick()
	if child.State() != Fulfilled || child.Value() != "value" {
		t.Fatalf("child settlement = (%v, %#v), want Fulfilled %q", child.State(), child.Value(), "value")
	}
}

func TestAplus_2_2_7_1_ReturnValueResolvesChild(t *testing.T) {
	for _, test := range []struct {
		name string
		want any
	}{
		{name: "value", want: "transformed"},
		{name: "nil", want: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			loop, js := newPromiseProfileJS(t)
			parent, resolve, _ := js.NewChainedPromise()
			child := parent.Then(func(any) any { return test.want }, nil)

			resolve("original")
			loop.tick()
			if child.State() != Fulfilled || child.Value() != test.want {
				t.Fatalf("child settlement = (%v, %#v), want Fulfilled %#v", child.State(), child.Value(), test.want)
			}
		})
	}
}

func TestAplus_2_2_7_2_ThrowExceptionRejectsChild(t *testing.T) {
	for _, test := range []struct {
		name         string
		rejectParent bool
	}{
		{name: "on fulfillment"},
		{name: "on rejection", rejectParent: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			loop, js := newPromiseProfileJS(t)
			parent, resolve, reject := js.NewChainedPromise()
			panicWitness := &struct{ name string }{name: test.name}
			panicHandler := func(any) any { panic(panicWitness) }
			var child *ChainedPromise
			if test.rejectParent {
				child = parent.Then(nil, panicHandler)
				reject(errors.New("parent rejection"))
			} else {
				child = parent.Then(panicHandler, nil)
				resolve("parent fulfillment")
			}

			loop.tick()
			panicError, ok := child.Reason().(PanicError)
			if child.State() != Rejected || !ok || panicError.Value != panicWitness {
				t.Fatalf("child settlement = (%v, %T %#v), want Rejected PanicError with exact witness", child.State(), child.Reason(), child.Reason())
			}
		})
	}
}

func TestAplus_2_2_7_3_NilOnFulfilledPassThrough(t *testing.T) {
	loop, js := newPromiseProfileJS(t)
	parent, resolve, _ := js.NewChainedPromise()
	child := parent.Then(nil, nil)

	resolve("passthrough")
	loop.tick()
	if child.State() != Fulfilled || child.Value() != "passthrough" {
		t.Fatalf("child settlement = (%v, %#v), want Fulfilled %q", child.State(), child.Value(), "passthrough")
	}
}

func TestAplus_2_2_7_4_NilOnRejectedPassThrough(t *testing.T) {
	loop, js := newPromiseProfileJS(t)
	parent, _, reject := js.NewChainedPromise()
	child := parent.Then(nil, nil)
	want := errors.New("passthrough reason")

	reject(want)
	loop.tick()
	if child.State() != Rejected || child.Reason() != want {
		t.Fatalf("child settlement = (%v, %#v), want Rejected exact reason", child.State(), child.Reason())
	}
}
