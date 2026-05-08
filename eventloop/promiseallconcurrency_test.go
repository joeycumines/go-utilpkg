package eventloop

import (
	"errors"
	"reflect"
	"testing"
)

func TestPromiseAll_ConcurrentResolutions(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	js := NewJS(loop)

	const count = 10
	promises := make([]*ChainedPromise, count)
	resolvers := make([]ResolveFunc, count)
	for i := range count {
		promises[i], resolvers[i], _ = js.NewChainedPromise()
	}
	result := js.All(promises)

	start := make(chan struct{})
	done := make(chan struct{}, count)
	want := make([]any, count)
	for i := range count {
		want[i] = i
		go func() {
			<-start
			resolvers[i](i)
			done <- struct{}{}
		}()
	}
	close(start)
	for range count {
		waitContractSignal(t, done, "concurrent Promise.All source settlement")
	}
	loop.tick()

	if result.State() != Fulfilled {
		t.Fatalf("state = %v, want Fulfilled", result.State())
	}
	if got := result.Value(); !reflect.DeepEqual(got, want) {
		t.Fatalf("value = %#v, want %#v", got, want)
	}
}

func TestPromiseAll_ConcurrentResolveAndReject(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	js := NewJS(loop, WithUnhandledRejection(func(any) {}))

	rejection := errors.New("rejection")
	rejected, _, reject := js.NewChainedPromise()
	fulfilled, resolve, _ := js.NewChainedPromise()
	result := js.All([]*ChainedPromise{rejected, fulfilled})

	start := make(chan struct{})
	done := make(chan struct{}, 2)
	go func() {
		<-start
		reject(rejection)
		done <- struct{}{}
	}()
	go func() {
		<-start
		resolve("resolution")
		done <- struct{}{}
	}()
	close(start)
	waitContractSignal(t, done, "concurrent Promise.All rejection")
	waitContractSignal(t, done, "concurrent Promise.All fulfillment")
	loop.tick()

	if result.State() != Rejected {
		t.Fatalf("state = %v, want Rejected", result.State())
	}
	if !errors.Is(result.Reason().(error), rejection) {
		t.Fatalf("reason = %v, want %v", result.Reason(), rejection)
	}
}

func TestPromiseAll_FirstRejectionRemainsStable(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	js := NewJS(loop, WithUnhandledRejection(func(any) {}))

	first, _, rejectFirst := js.NewChainedPromise()
	second, resolveSecond, _ := js.NewChainedPromise()
	third, _, rejectThird := js.NewChainedPromise()
	result := js.All([]*ChainedPromise{first, second, third})
	want := errors.New("first rejection")

	rejectFirst(want)
	loop.tick()
	if result.State() != Rejected || result.Reason() != want {
		t.Fatalf("result after first rejection = (%v, %#v), want Rejected exact sentinel", result.State(), result.Reason())
	}

	resolveSecond("second fulfillment")
	rejectThird(errors.New("later rejection"))
	loop.tick()
	if result.State() != Rejected || result.Reason() != want {
		t.Fatalf("result after peer settlements = (%v, %#v), want unchanged Rejected sentinel", result.State(), result.Reason())
	}
}

func TestPromiseAll_MixedImmediateAndPending(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	js := NewJS(loop)

	immediate := js.Resolve("immediate")
	pending, resolvePending, _ := js.NewChainedPromise()
	result := js.All([]*ChainedPromise{immediate, pending})
	loop.tick()
	if result.State() != Pending {
		t.Fatalf("state before final input = %v, want Pending", result.State())
	}

	resolvePending("pending")
	loop.tick()
	if result.State() != Fulfilled {
		t.Fatalf("state after final input = %v, want Fulfilled", result.State())
	}
	want := []any{"immediate", "pending"}
	if got := result.Value(); !reflect.DeepEqual(got, want) {
		t.Fatalf("value = %#v, want %#v", got, want)
	}
}
