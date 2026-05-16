package eventloop

import (
	"errors"
	"reflect"
	"testing"
)

func TestPromiseAllSettled_ChainedPromises(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	fulfilled, resolve, _ := js.NewChainedPromise()
	rejected, _, reject := js.NewChainedPromise()
	fulfilledChild := fulfilled.Then(func(value any) any {
		return value.(string) + "-chained"
	}, nil)
	recoveredChild := rejected.Catch(func(reason any) any {
		return reason.(error).Error() + "-recovered"
	})
	result := js.AllSettled([]*ChainedPromise{fulfilledChild, recoveredChild})

	resolve("value")
	reject(errors.New("error"))
	loop.tick()

	want := []any{
		map[string]any{"status": "fulfilled", "value": "value-chained"},
		map[string]any{"status": "fulfilled", "value": "error-recovered"},
	}
	if result.State() != Fulfilled {
		t.Fatalf("state = %v, want Fulfilled", result.State())
	}
	if got := result.Value(); !reflect.DeepEqual(got, want) {
		t.Fatalf("value = %#v, want %#v", got, want)
	}
}

func TestPromiseAllSettled_RejectionAfterFulfillment(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	fulfilled, resolve, _ := js.NewChainedPromise()
	rejected, _, reject := js.NewChainedPromise()
	result := js.AllSettled([]*ChainedPromise{fulfilled, rejected})

	resolve("fulfilled")
	loop.tick()
	if result.State() != Pending {
		t.Fatalf("state before rejection = %v, want Pending", result.State())
	}

	reason := errors.New("error")
	reject(reason)
	loop.tick()
	want := []any{
		map[string]any{"status": "fulfilled", "value": "fulfilled"},
		map[string]any{"status": "rejected", "reason": reason},
	}
	if result.State() != Fulfilled {
		t.Fatalf("state after rejection = %v, want Fulfilled", result.State())
	}
	if got := result.Value(); !reflect.DeepEqual(got, want) {
		t.Fatalf("value = %#v, want %#v", got, want)
	}
}

func TestPromiseAllSettled_PreservesInputOrder(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	first, resolveFirst, _ := js.NewChainedPromise()
	second, _, rejectSecond := js.NewChainedPromise()
	third, resolveThird, _ := js.NewChainedPromise()
	result := js.AllSettled([]*ChainedPromise{first, second, third})

	resolveThird("third")
	rejectSecond("second")
	resolveFirst("first")
	loop.tick()

	want := []any{
		map[string]any{"status": "fulfilled", "value": "first"},
		map[string]any{"status": "rejected", "reason": "second"},
		map[string]any{"status": "fulfilled", "value": "third"},
	}
	if result.State() != Fulfilled {
		t.Fatalf("state = %v, want Fulfilled", result.State())
	}
	if got := result.Value(); !reflect.DeepEqual(got, want) {
		t.Fatalf("value = %#v, want input-ordered %#v", got, want)
	}
}
