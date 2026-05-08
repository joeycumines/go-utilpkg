package eventloop

import (
	"errors"
	"sync/atomic"
	"testing"
)

type promiseThenableProbe struct {
	calls atomic.Int32
}

func (p *promiseThenableProbe) Then(func(any) any, func(any) any) *ChainedPromise {
	p.calls.Add(1)
	return nil
}

func TestAplus_2_3_2_PromiseAdoption(t *testing.T) {
	loop, js := newPromiseProfileJS(t)
	inner, resolveInner, _ := js.NewChainedPromise()
	outer, resolveOuter, _ := js.NewChainedPromise()
	resolveOuter(inner)
	loop.tick()
	if outer.State() != Pending {
		t.Fatalf("outer state before inner settlement = %v, want Pending", outer.State())
	}

	resolveInner("inner value")
	loop.tick()
	if outer.State() != Fulfilled || outer.Value() != "inner value" {
		t.Fatalf("outer settlement = (%v, %#v), want Fulfilled %q", outer.State(), outer.Value(), "inner value")
	}
}

func TestAplus_2_3_2_HandlerReturnedPromiseAdoption(t *testing.T) {
	for _, test := range []struct {
		name   string
		reject bool
	}{
		{name: "fulfillment"},
		{name: "rejection", reject: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			loop, js := newPromiseProfileJS(t)
			inner, resolveInner, rejectInner := js.NewChainedPromise()
			parent, resolveParent, _ := js.NewChainedPromise()
			child := parent.Then(func(any) any { return inner }, nil)

			resolveParent("parent value")
			loop.tick()
			if child.State() != Pending {
				t.Fatalf("child state before inner settlement = %v, want Pending", child.State())
			}

			witness := &struct{ name string }{name: test.name}
			if test.reject {
				rejectInner(witness)
			} else {
				resolveInner(witness)
			}
			loop.tick()
			if test.reject {
				if child.State() != Rejected || child.Reason() != witness {
					t.Fatalf("child settlement = (%v, %#v), want Rejected exact witness", child.State(), child.Reason())
				}
				return
			}
			if child.State() != Fulfilled || child.Value() != witness {
				t.Fatalf("child settlement = (%v, %#v), want Fulfilled exact witness", child.State(), child.Value())
			}
		})
	}
}

func TestChainedPromiseTypedNilAdoptionRejects(t *testing.T) {
	target := &ChainedPromise{}
	target.state.Store(int32(Pending))
	var source *ChainedPromise
	target.resolve(source)

	if state := target.State(); state != Rejected {
		t.Fatalf("target state = %v, want Rejected", state)
	}
	reason, ok := target.Reason().(error)
	if !ok || reason == nil {
		t.Fatalf("typed-nil adoption reason = %T %#v, want nonnil error", target.Reason(), target.Reason())
	}
	var nilInput *NilPromiseError
	if errors.As(reason, &nilInput) {
		t.Fatal("typed-nil adoption used combinator input error")
	}
}

func TestResolvePreservesNonPromiseThenableObject(t *testing.T) {
	loop, js := newPromiseProfileJS(t)
	probe := &promiseThenableProbe{}
	result := js.Resolve(probe)
	loop.tick()

	if result.State() != Fulfilled || result.Value() != probe {
		t.Fatalf("Resolve result = (%v, %T %#v), want Fulfilled exact probe", result.State(), result.Value(), result.Value())
	}
	if calls := probe.calls.Load(); calls != 0 {
		t.Fatalf("arbitrary Then method calls = %d, want 0", calls)
	}
}

func TestErrorPropagation_ThroughChain(t *testing.T) {
	loop, js := newPromiseProfileJS(t)
	parent, _, reject := js.NewChainedPromise()
	want := errors.New("original error")
	var caught any
	parent.Then(func(value any) any {
		t.Error("first fulfillment callback ran for rejected chain")
		return value
	}, nil).Then(func(value any) any {
		t.Error("second fulfillment callback ran for rejected chain")
		return value
	}, nil).Then(nil, func(reason any) any {
		caught = reason
		return nil
	})

	reject(want)
	loop.tick()
	if caught != want {
		t.Fatalf("caught reason = %#v, want exact original error", caught)
	}
}

func TestErrorPropagation_Recovery(t *testing.T) {
	loop, js := newPromiseProfileJS(t)
	parent, _, reject := js.NewChainedPromise()
	var result any
	parent.Then(nil, func(any) any {
		return "recovered"
	}).Then(func(value any) any {
		result = value
		return value
	}, nil)

	reject(errors.New("error"))
	loop.tick()
	if result != "recovered" {
		t.Fatalf("recovered chain value = %#v, want %q", result, "recovered")
	}
}

func TestAlreadySettled_ThenOnFulfilled(t *testing.T) {
	loop, js := newPromiseProfileJS(t)
	parent, resolve, _ := js.NewChainedPromise()
	resolve("pre-fulfilled")
	loop.tick()

	var received any
	parent.Then(func(value any) any {
		received = value
		return value
	}, nil)
	if received != nil {
		t.Fatalf("late callback ran before checkpoint with value %#v", received)
	}
	loop.tick()
	if received != "pre-fulfilled" {
		t.Fatalf("late callback value = %#v, want %q", received, "pre-fulfilled")
	}
}

func TestAlreadySettled_MultipleHandlers(t *testing.T) {
	loop, js := newPromiseProfileJS(t)
	parent, resolve, _ := js.NewChainedPromise()
	resolve("value")
	loop.tick()

	var count atomic.Int32
	for range 5 {
		parent.Then(func(value any) any {
			count.Add(1)
			return value
		}, nil)
	}
	if count.Load() != 0 {
		t.Fatalf("late callbacks ran before checkpoint: %d", count.Load())
	}
	loop.tick()
	if got := count.Load(); got != 5 {
		t.Fatalf("late callback count = %d, want 5", got)
	}
}
