package eventloop

import (
	"errors"
	"testing"
)

type promiseCombinatorTestCase struct {
	name    string
	combine func(*JS, []*ChainedPromise) *ChainedPromise
}

func promiseCombinatorTestCases() []promiseCombinatorTestCase {
	return []promiseCombinatorTestCase{
		{name: "All", combine: (*JS).All},
		{name: "Race", combine: (*JS).Race},
		{name: "AllSettled", combine: (*JS).AllSettled},
		{name: "Any", combine: (*JS).Any},
	}
}

func TestPromiseAllEmptyInputSettlesSynchronously(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}
	result := js.All(nil)
	if err != nil {
		t.Fatal(err)
	}
	if state := result.State(); state != Fulfilled {
		t.Fatalf("empty All state = %v, want Fulfilled", state)
	}
	values, ok := result.Value().([]any)
	if !ok || len(values) != 0 {
		t.Fatalf("empty All value = %T %#v, want empty []any", result.Value(), result.Value())
	}
}

func TestPromiseAllSettledEmptyInputSettlesSynchronously(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}
	result := js.AllSettled(nil)
	if err != nil {
		t.Fatal(err)
	}
	if state := result.State(); state != Fulfilled {
		t.Fatalf("empty AllSettled state = %v, want Fulfilled", state)
	}
	values, ok := result.Value().([]any)
	if !ok || len(values) != 0 {
		t.Fatalf("empty AllSettled value = %T %#v, want empty []any", result.Value(), result.Value())
	}
}

func TestPromiseAnyEmptyInputRejectsSynchronously(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}
	result := js.Any(nil)
	if err != nil {
		t.Fatal(err)
	}
	if state := result.State(); state != Rejected {
		t.Fatalf("empty Any state = %v, want Rejected", state)
	}
	aggregate, ok := result.Reason().(*AggregateError)
	if !ok {
		t.Fatalf("empty Any reason = %T %#v, want *AggregateError", result.Reason(), result.Reason())
	}
	if len(aggregate.Errors) != 0 {
		t.Fatalf("empty Any errors = %#v, want empty", aggregate.Errors)
	}
	if aggregate.Message != "All promises were rejected" {
		t.Fatalf("empty Any message = %q, want %q", aggregate.Message, "All promises were rejected")
	}
}

func TestPromiseRaceEmptyInputRemainsPending(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}
	result := js.Race(nil)
	if err != nil {
		t.Fatal(err)
	}
	settled := result.ToChannel()
	if state := result.State(); state != Pending {
		t.Fatalf("empty Race initial state = %v, want Pending", state)
	}

	loop.tick()
	if state := result.State(); state != Pending {
		t.Fatalf("empty Race state after owner drain = %v, want Pending", state)
	}
	select {
	case value, open := <-settled:
		t.Fatalf("empty Race settlement = (%#v, open=%t), want no settlement", value, open)
	default:
	}
}

func newCombinatorTestAdapter(t *testing.T, reported chan<- any) (*Loop, *JS) {
	t.Helper()
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop,
		WithUnhandledRejection(func(reason any) { reported <- reason }),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
	)
	if err != nil {
		t.Fatal(err)
	}
	return loop, js
}

func assertTerminalCombinatorRejection(t *testing.T, result *ChainedPromise, resultChannel <-chan any) {
	t.Helper()
	if state := result.State(); state != Rejected {
		t.Fatalf("aggregate state = %v, want Rejected", state)
	}
	reason, ok := result.Reason().(error)
	if !ok || !errors.Is(reason, ErrLoopTerminated) {
		t.Fatalf("aggregate reason = %T %v, want ErrLoopTerminated", result.Reason(), result.Reason())
	}
	channelValue := waitContractValue(t, resultChannel, "terminal combinator settlement")
	channelError, ok := channelValue.(error)
	if !ok || !errors.Is(channelError, ErrLoopTerminated) {
		t.Fatalf("aggregate channel result = %T %v, want ErrLoopTerminated", channelValue, channelValue)
	}
	assertPromiseResultChannelClosed(t, resultChannel)
}

func assertSinglePromiseChannelValue(t *testing.T, resultChannel <-chan any, want any) {
	t.Helper()
	if got := waitContractValue(t, resultChannel, "promise settlement"); got != want {
		t.Fatalf("promise channel result = %v, want %v", got, want)
	}
	assertPromiseResultChannelClosed(t, resultChannel)
}

func assertPromiseResultChannelClosed(t *testing.T, resultChannel <-chan any) {
	t.Helper()
	select {
	case value, open := <-resultChannel:
		if open {
			t.Fatalf("promise result channel delivered a second value: %v", value)
		}
	default:
		t.Fatal("promise result channel remained open after settlement")
	}
}
