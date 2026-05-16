package eventloop

import (
	"context"
	"errors"
	"testing"
)

func newPromiseResolversT(t *testing.T) (*Loop, *PromiseWithResolvers) {
	t.Helper()
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := loop.Shutdown(context.Background()); err != nil {
			t.Errorf("Shutdown: %v", err)
		}
	})
	js, err := NewJS(loop, WithUnhandledRejection(func(any) {}))
	if err != nil {
		t.Fatal(err)
	}
	return loop, js.WithResolvers()
}

func TestWithResolvers_Basic(t *testing.T) {
	_, resolvers := newPromiseResolversT(t)
	if resolvers.Promise == nil || resolvers.Resolve == nil || resolvers.Reject == nil {
		t.Fatalf("resolvers = %#v, want non-nil Promise, Resolve, and Reject", resolvers)
	}
	if resolvers.Promise.State() != Pending {
		t.Fatalf("initial state = %v, want Pending", resolvers.Promise.State())
	}
}

func TestWithResolvers_ResolveOnlyOnce(t *testing.T) {
	_, resolvers := newPromiseResolversT(t)
	resolvers.Resolve("first")
	resolvers.Resolve("second")
	if resolvers.Promise.State() != Fulfilled || resolvers.Promise.Value() != "first" {
		t.Fatalf("result = (%v, %#v), want (Fulfilled, first)", resolvers.Promise.State(), resolvers.Promise.Value())
	}
}

func TestWithResolvers_RejectOnlyOnce(t *testing.T) {
	_, resolvers := newPromiseResolversT(t)
	first := errors.New("first")
	resolvers.Reject(first)
	resolvers.Reject(errors.New("second"))
	if resolvers.Promise.State() != Rejected || resolvers.Promise.Reason() != first {
		t.Fatalf("result = (%v, %v), want (Rejected, %v)", resolvers.Promise.State(), resolvers.Promise.Reason(), first)
	}
}

func TestWithResolvers_ResolveAfterReject(t *testing.T) {
	_, resolvers := newPromiseResolversT(t)
	reason := errors.New("error")
	resolvers.Reject(reason)
	resolvers.Resolve("value")
	if resolvers.Promise.State() != Rejected || resolvers.Promise.Reason() != reason {
		t.Fatalf("result = (%v, %v), want (Rejected, %v)", resolvers.Promise.State(), resolvers.Promise.Reason(), reason)
	}
}

func TestWithResolvers_RejectAfterResolve(t *testing.T) {
	_, resolvers := newPromiseResolversT(t)
	resolvers.Resolve("value")
	resolvers.Reject(errors.New("error"))
	if resolvers.Promise.State() != Fulfilled || resolvers.Promise.Value() != "value" {
		t.Fatalf("result = (%v, %#v), want (Fulfilled, value)", resolvers.Promise.State(), resolvers.Promise.Value())
	}
}

func TestWithResolvers_ConcurrentResolve(t *testing.T) {
	_, resolvers := newPromiseResolversT(t)
	resolvers.Resolve("winner")

	const losers = 32
	start := make(chan struct{})
	done := make(chan struct{}, losers)
	for value := range losers {
		go func() {
			<-start
			resolvers.Resolve(value)
			done <- struct{}{}
		}()
	}
	close(start)
	for range losers {
		waitContractSignal(t, done, "losing concurrent resolve")
	}
	if resolvers.Promise.State() != Fulfilled || resolvers.Promise.Value() != "winner" {
		t.Fatalf("result = (%v, %#v), want (Fulfilled, winner)", resolvers.Promise.State(), resolvers.Promise.Value())
	}
}

func TestWithResolvers_ConcurrentReject(t *testing.T) {
	_, resolvers := newPromiseResolversT(t)
	winner := errors.New("winner")
	resolvers.Reject(winner)

	const losers = 32
	start := make(chan struct{})
	done := make(chan struct{}, losers)
	for range losers {
		go func() {
			<-start
			resolvers.Reject(errors.New("loser"))
			done <- struct{}{}
		}()
	}
	close(start)
	for range losers {
		waitContractSignal(t, done, "losing concurrent reject")
	}
	if resolvers.Promise.State() != Rejected || resolvers.Promise.Reason() != winner {
		t.Fatalf("result = (%v, %v), want (Rejected, %v)", resolvers.Promise.State(), resolvers.Promise.Reason(), winner)
	}
}

func TestWithResolvers_NilValue(t *testing.T) {
	_, resolvers := newPromiseResolversT(t)
	resolvers.Resolve(nil)
	if resolvers.Promise.State() != Fulfilled || resolvers.Promise.Value() != nil {
		t.Fatalf("result = (%v, %#v), want (Fulfilled, nil)", resolvers.Promise.State(), resolvers.Promise.Value())
	}
}

func TestWithResolvers_NilReason(t *testing.T) {
	_, resolvers := newPromiseResolversT(t)
	resolvers.Reject(nil)
	if resolvers.Promise.State() != Rejected || resolvers.Promise.Reason() != nil {
		t.Fatalf("result = (%v, %#v), want (Rejected, nil)", resolvers.Promise.State(), resolvers.Promise.Reason())
	}
}

func TestWithResolvers_Chaining(t *testing.T) {
	loop, resolvers := newPromiseResolversT(t)
	child := resolvers.Promise.Then(func(value any) any { return value.(int) * 2 }, nil)
	resolvers.Resolve(21)
	loop.tick()
	if child.State() != Fulfilled || child.Value() != 42 {
		t.Fatalf("child = (%v, %#v), want (Fulfilled, 42)", child.State(), child.Value())
	}
}

func TestWithResolvers_AsyncFromGoroutine(t *testing.T) {
	_, resolvers := newPromiseResolversT(t)
	result := resolvers.Promise.ToChannel()
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	go func() {
		close(started)
		<-release
		resolvers.Resolve("async value")
		close(done)
	}()
	waitContractSignal(t, started, "resolver goroutine entry")
	select {
	case value := <-result:
		t.Fatalf("result arrived before resolver release: %#v", value)
	default:
	}
	close(release)
	waitContractSignal(t, done, "resolver goroutine completion")
	if value := waitContractValue(t, result, "off-goroutine resolution"); value != "async value" {
		t.Fatalf("value = %#v, want async value", value)
	}
}

func TestWithResolvers_MultiplePromises(t *testing.T) {
	loop, first := newPromiseResolversT(t)
	js, err := NewJS(loop, WithUnhandledRejection(func(any) {}))
	if err != nil {
		t.Fatal(err)
	}
	second := js.WithResolvers()
	third := js.WithResolvers()
	if first.Promise == second.Promise || second.Promise == third.Promise || first.Promise == third.Promise {
		t.Fatal("WithResolvers reused a Promise instance")
	}
	first.Resolve("one")
	second.Reject("two")
	if first.Promise.State() != Fulfilled || first.Promise.Value() != "one" {
		t.Fatalf("first = (%v, %#v), want (Fulfilled, one)", first.Promise.State(), first.Promise.Value())
	}
	if second.Promise.State() != Rejected || second.Promise.Reason() != "two" {
		t.Fatalf("second = (%v, %#v), want (Rejected, two)", second.Promise.State(), second.Promise.Reason())
	}
	if third.Promise.State() != Pending {
		t.Fatalf("third state = %v, want Pending", third.Promise.State())
	}
}
