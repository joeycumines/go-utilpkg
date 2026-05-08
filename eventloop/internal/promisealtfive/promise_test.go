package promisealtfive

import (
	"errors"
	"slices"
	"testing"
)

func TestSettlement(t *testing.T) {
	t.Run("fulfilled", func(t *testing.T) {
		promise, resolve, reject := New(nil)
		if resolve == nil || reject == nil {
			t.Fatal("New() returned a nil settlement function")
		}
		resolve(42)
		if promise.State() != Fulfilled || promise.Value() != 42 || promise.Reason() != nil {
			t.Errorf("settlement = (%v, %v, %v), want (Fulfilled, 42, nil)", promise.State(), promise.Value(), promise.Reason())
		}
	})

	t.Run("rejected", func(t *testing.T) {
		promise, _, reject := New(nil)
		reason := errors.New("rejected")
		reject(reason)
		gotReason, ok := promise.Reason().(error)
		if promise.State() != Rejected || !ok || !errors.Is(gotReason, reason) || promise.Value() != nil {
			t.Errorf("settlement = (%v, %v, %v), want (Rejected, nil, %v)", promise.State(), promise.Value(), promise.Reason(), reason)
		}
	})
}

func TestPropagation(t *testing.T) {
	t.Run("fulfilled", func(t *testing.T) {
		promise, resolve, _ := New(nil)
		child := promise.Then(func(value any) any { return value.(int) + 1 }, nil)
		grandchild := child.Then(nil, nil)
		resolve(41)
		if grandchild.State() != Fulfilled || grandchild.Value() != 42 {
			t.Errorf("fulfilled grandchild = (%v, %v), want (Fulfilled, 42)", grandchild.State(), grandchild.Value())
		}
	})

	t.Run("rejected", func(t *testing.T) {
		promise, _, reject := New(nil)
		reason := errors.New("rejected")
		child := promise.Then(nil, nil).Then(nil, nil)
		reject(reason)
		gotReason, ok := child.Reason().(error)
		if child.State() != Rejected || !ok || !errors.Is(gotReason, reason) {
			t.Errorf("rejected child = (%v, %v), want (Rejected, %v)", child.State(), child.Reason(), reason)
		}
	})
}

func TestPendingHandlerOrder(t *testing.T) {
	promise, resolve, _ := New(nil)
	order := make([]int, 0, 3)
	for index := 1; index <= 3; index++ {
		index := index
		promise.Then(func(value any) any {
			order = append(order, index)
			return value
		}, nil)
	}
	resolve("value")
	if !slices.Equal(order, []int{1, 2, 3}) {
		t.Errorf("handler order = %v, want [1 2 3]", order)
	}
}

func TestSelfResolution(t *testing.T) {
	promise, resolve, _ := New(nil)
	resolve(promise)
	if promise.State() != Rejected {
		t.Fatalf("state = %v, want Rejected", promise.State())
	}
	reason, ok := promise.Reason().(error)
	if !ok || reason.Error() != "TypeError: Chaining cycle detected" {
		t.Errorf("reason = %v, want chaining-cycle TypeError", promise.Reason())
	}
}
