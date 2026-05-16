package eventloop

import (
	"errors"
	"testing"
)

// These tests exercise the Promise/A+-inspired state rules implemented by this
// Go-native Promise profile. They are not a Promise/A+ compliance suite: this
// package adopts ChainedPromise values but deliberately does not assimilate
// arbitrary objects with a method named Then.

func newPromiseProfileJS(t *testing.T) (*Loop, *JS) {
	t.Helper()
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}
	return loop, js
}

func TestAplus_2_1_1_PendingToFulfilled(t *testing.T) {
	loop, js := newPromiseProfileJS(t)
	promise, resolve, _ := js.NewChainedPromise()
	if promise.State() != Pending {
		t.Fatalf("initial state = %v, want Pending", promise.State())
	}

	resolve("success")
	loop.tick()
	if promise.State() != Fulfilled || promise.Value() != "success" {
		t.Fatalf("settlement = (%v, %#v), want Fulfilled %q", promise.State(), promise.Value(), "success")
	}
}

func TestAplus_2_1_1_PendingToRejected(t *testing.T) {
	loop, js := newPromiseProfileJS(t)
	promise, _, reject := js.NewChainedPromise()
	if promise.State() != Pending {
		t.Fatalf("initial state = %v, want Pending", promise.State())
	}

	want := errors.New("failure")
	reject(want)
	loop.tick()
	if promise.State() != Rejected || promise.Reason() != want {
		t.Fatalf("settlement = (%v, %#v), want Rejected exact reason", promise.State(), promise.Reason())
	}
}

func TestAplus_2_1_2_FulfilledImmutable(t *testing.T) {
	loop, js := newPromiseProfileJS(t)
	promise, resolve, reject := js.NewChainedPromise()
	resolve("first")
	loop.tick()

	resolve("second")
	reject(errors.New("error"))
	loop.tick()
	if promise.State() != Fulfilled || promise.Value() != "first" || promise.Reason() != nil {
		t.Fatalf("settlement after later attempts = (%v, %#v, %#v), want Fulfilled first and nil reason", promise.State(), promise.Value(), promise.Reason())
	}
}

func TestAplus_2_1_3_RejectedImmutable(t *testing.T) {
	loop, js := newPromiseProfileJS(t)
	promise, resolve, reject := js.NewChainedPromise()
	want := errors.New("first error")
	reject(want)
	loop.tick()

	reject(errors.New("second error"))
	resolve("value")
	loop.tick()
	if promise.State() != Rejected || promise.Reason() != want || promise.Value() != nil {
		t.Fatalf("settlement after later attempts = (%v, %#v, %#v), want Rejected exact reason and nil value", promise.State(), promise.Reason(), promise.Value())
	}
}
