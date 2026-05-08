package eventloop

import (
	"testing"
)

func TestPromiseRace_LateHandlerObservesWinner(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	js := NewJS(loop)

	source, resolve, _ := js.NewChainedPromise()
	result := js.Race([]*ChainedPromise{source})
	resolve("quick")
	loop.tick()
	if result.State() != Fulfilled || result.Value() != "quick" {
		t.Fatalf("race result = (%v, %#v), want (Fulfilled, quick)", result.State(), result.Value())
	}

	child := result.Then(func(value any) any { return value }, nil)
	loop.tick()
	if child.State() != Fulfilled || child.Value() != "quick" {
		t.Fatalf("late handler child = (%v, %#v), want (Fulfilled, quick)", child.State(), child.Value())
	}
}

func TestPromiseRace_AlreadySettledUsesInputSchedulingOrder(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	js := NewJS(loop, WithUnhandledRejection(func(any) {}))

	first := js.Resolve("first")
	second := js.Reject("second")
	result := js.Race([]*ChainedPromise{first, second})
	loop.tick()

	if result.State() != Fulfilled || result.Value() != "first" {
		t.Fatalf("race result = (%v, %#v), want (Fulfilled, first)", result.State(), result.Value())
	}
}
