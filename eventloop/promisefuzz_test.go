package eventloop

import (
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

func FuzzPromiseCombinators(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6})
	f.Add([]byte("all-race-any-allsettled"))
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 0})

	f.Fuzz(func(t *testing.T, data []byte) {
		r := newFuzzReader(data)
		loop, err := New(WithAutoExit(true))
		if err != nil {
			panic(err)
		}
		var unhandled atomic.Int32
		js, err := NewJS(loop, WithUnhandledRejection(func(reason any) { unhandled.Add(1) }))
		if err != nil {
			panic(err)
		}

		n := r.intn(7)
		promises := make([]*ChainedPromise, n)
		resolves := make([]ResolveFunc, n)
		rejects := make([]RejectFunc, n)
		fulfilled := make([]bool, n)
		values := make([]any, n)
		for i := range n {
			promises[i], resolves[i], rejects[i] = js.NewChainedPromise()
			fulfilled[i] = r.bool()
			if fulfilled[i] {
				values[i] = fmt.Sprintf("value:%d", i)
			} else {
				values[i] = fmt.Sprintf("reason:%d", i)
			}
		}

		order := make([]int, n)
		for i := range order {
			order[i] = i
		}
		for i := len(order) - 1; i > 0; i-- {
			j := r.intn(i + 1)
			order[i], order[j] = order[j], order[i]
		}

		kind := r.intn(4)
		var result *ChainedPromise
		switch kind {
		case 0:
			result = js.All(promises)
		case 1:
			result = js.Race(promises)
		case 2:
			result = js.AllSettled(promises)
		case 3:
			result = js.Any(promises)
		}
		result.Catch(func(any) any { return nil })

		for _, idx := range order {
			if fulfilled[idx] {
				resolves[idx](values[idx])
			} else {
				rejects[idx](values[idx])
			}
		}

		if err := runAutoExitLoop(t, loop); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got := unhandled.Load(); got != 0 {
			t.Fatalf("unexpected unhandled rejection count: %d", got)
		}
		assertPromiseCombinatorResult(t, kind, result, fulfilled, values, order)
	})
}

// FuzzPromiseAlternatingThenCatch preserves the historical alternating-chain
// corpus against the final Promise implementation and checks its settlement.
func FuzzPromiseAlternatingThenCatch(f *testing.F) {
	f.Add(uint8(1), uint8(1))
	f.Add(uint8(2), uint8(10))

	f.Fuzz(func(t *testing.T, op uint8, depth uint8) {
		loop, err := New(WithAutoExit(true))
		if err != nil {
			panic(err)
		}
		var unhandled atomic.Int32
		js, err := NewJS(loop, WithUnhandledRejection(func(any) {
			if err != nil {
				panic(err)
			}
			unhandled.Add(1)
		}))

		root, resolve, reject := js.NewChainedPromise()
		current := root
		if depth > 50 {
			depth = 50
		}
		for i := 0; i < int(depth); i++ {
			if i%2 == 0 {
				current = current.Then(func(value any) any { return value }, nil)
			} else {
				current = current.Catch(func(reason any) any { return reason })
			}
		}
		current.Catch(func(reason any) any { return reason })

		wantState := Fulfilled
		wantValue := any(1)
		if op%2 == 0 {
			resolve(wantValue)
		} else {
			wantValue = "error"
			if depth < 2 {
				wantState = Rejected
			}
			reject(wantValue)
		}

		if err := runAutoExitLoop(t, loop); err != nil {
			t.Fatalf("Run: %v", err)
		}
		if got := unhandled.Load(); got != 0 {
			t.Fatalf("unexpected unhandled rejection count: %d", got)
		}
		if got := current.State(); got != wantState {
			t.Fatalf("chain state = %v, want %v", got, wantState)
		}
		if wantState == Fulfilled {
			if got := current.Value(); got != wantValue {
				t.Fatalf("chain value = %#v, want %#v", got, wantValue)
			}
		} else if got := current.Reason(); got != wantValue {
			t.Fatalf("chain reason = %#v, want %#v", got, wantValue)
		}
	})
}

func assertPromiseCombinatorResult(t *testing.T, kind int, result *ChainedPromise, fulfilled []bool, values []any, order []int) {
	t.Helper()
	switch kind {
	case 0: // All
		firstRejection := -1
		for _, idx := range order {
			if !fulfilled[idx] {
				firstRejection = idx
				break
			}
		}
		if firstRejection >= 0 {
			if result.State() != Rejected || result.Reason() != values[firstRejection] {
				t.Fatalf("All rejection = (%v, %#v), want Rejected %#v", result.State(), result.Reason(), values[firstRejection])
			}
			return
		}
		if result.State() != Fulfilled {
			t.Fatalf("All state = %v, want Fulfilled", result.State())
		}
		got, ok := result.Value().([]any)
		if !ok {
			t.Fatalf("All value type = %T, want []any", result.Value())
		}
		if !reflect.DeepEqual(got, values) {
			t.Fatalf("All values = %#v, want %#v", got, values)
		}

	case 1: // Race
		if len(order) == 0 {
			if result.State() != Pending {
				t.Fatalf("Race(empty) state = %v, want Pending", result.State())
			}
			return
		}
		winner := order[0]
		if fulfilled[winner] {
			if result.State() != Fulfilled || result.Value() != values[winner] {
				t.Fatalf("Race fulfillment = (%v, %#v), want Fulfilled %#v", result.State(), result.Value(), values[winner])
			}
		} else if result.State() != Rejected || result.Reason() != values[winner] {
			t.Fatalf("Race rejection = (%v, %#v), want Rejected %#v", result.State(), result.Reason(), values[winner])
		}

	case 2: // AllSettled
		if result.State() != Fulfilled {
			t.Fatalf("AllSettled state = %v, want Fulfilled", result.State())
		}
		got, ok := result.Value().([]any)
		if !ok {
			t.Fatalf("AllSettled value type = %T, want []any", result.Value())
		}
		if len(got) != len(values) {
			t.Fatalf("AllSettled length = %d, want %d", len(got), len(values))
		}
		for i := range got {
			entry, ok := got[i].(map[string]any)
			if !ok {
				t.Fatalf("AllSettled[%d] type = %T, want map[string]any", i, got[i])
			}
			if fulfilled[i] {
				if entry["status"] != "fulfilled" || entry["value"] != values[i] {
					t.Fatalf("AllSettled[%d] = %#v, want fulfilled value %#v", i, entry, values[i])
				}
			} else if entry["status"] != "rejected" || entry["reason"] != values[i] {
				t.Fatalf("AllSettled[%d] = %#v, want rejected reason %#v", i, entry, values[i])
			}
		}

	case 3: // Any
		firstFulfilled := -1
		for _, idx := range order {
			if fulfilled[idx] {
				firstFulfilled = idx
				break
			}
		}
		if firstFulfilled >= 0 {
			if result.State() != Fulfilled || result.Value() != values[firstFulfilled] {
				t.Fatalf("Any fulfillment = (%v, %#v), want Fulfilled %#v", result.State(), result.Value(), values[firstFulfilled])
			}
			return
		}
		if result.State() != Rejected {
			t.Fatalf("Any all-rejected state = %v, want Rejected", result.State())
		}
		var agg *AggregateError
		reason, ok := result.Reason().(error)
		if !ok || !errors.As(reason, &agg) {
			t.Fatalf("Any all-rejected reason = %T %#v, want AggregateError", result.Reason(), result.Reason())
		}
		if len(agg.Errors) != len(values) {
			t.Fatalf("AggregateError length = %d, want %d", len(agg.Errors), len(values))
		}
		if len(values) > 0 {
			for i, reason := range agg.Errors {
				if !reflect.DeepEqual(reason, values[i]) {
					t.Fatalf("AggregateError[%d] = %#v, want %#v", i, reason, values[i])
				}
			}
		}
	}
}

func FuzzPromiseChainAdoptionPanicAndChannels(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0, 1, 2, 3, 4, 5, 6, 7})
	f.Add([]byte("then-catch-finally-adoption-panic"))

	f.Fuzz(func(t *testing.T, data []byte) {
		r := newFuzzReader(data)
		loop, err := New(WithAutoExit(true))
		if err != nil {
			panic(err)
		}
		var callbackErrs fuzzErrs
		js, err := NewJS(loop, WithUnhandledRejection(func(reason any) {
			if err != nil {
				panic(err)
			}
			callbackErrs.add("unexpected unhandled rejection: %#v", reason)
		}))

		root, resolveRoot, rejectRoot := js.NewChainedPromise()
		current := root
		steps := 1 + r.intn(16)
		for i := range steps {
			op := r.byte() % 6
			switch op {
			case 0:
				value := fmt.Sprintf("then:%d", i)
				current = current.Then(func(any) any { return value }, nil)
			case 1:
				current = current.Then(func(v any) any { return v }, func(r any) any { return fmt.Sprintf("recovered:%v", r) })
			case 2:
				current = current.Then(func(any) any { panic(fmt.Sprintf("panic:%d", i)) }, nil)
			case 3:
				adopted := js.Resolve(fmt.Sprintf("adopted:%d", i))
				current = current.Then(func(any) any { return adopted }, nil)
			case 4:
				current = current.Catch(func(r any) any { return fmt.Sprintf("caught:%v", r) })
			case 5:
				current = current.Finally(func() {
					if i%3 == 0 {
						panic("finally panic should preserve original settlement")
					}
				})
			}
		}
		current.Catch(func(any) any { return nil })
		ch := current.ToChannel()
		if r.bool() {
			resolveRoot("root-value")
		} else {
			rejectRoot("root-reason")
		}

		if err := runAutoExitLoop(t, loop); err != nil {
			t.Fatalf("Run: %v", err)
		}
		callbackErrs.failNow(t)
		select {
		case got, ok := <-ch:
			if !ok {
				t.Fatalf("ToChannel closed without value")
			}
			switch current.State() {
			case Fulfilled:
				if got != current.Value() {
					t.Fatalf("ToChannel got %#v, want fulfilled value %#v", got, current.Value())
				}
			case Rejected:
				if !reflect.DeepEqual(got, current.Reason()) {
					t.Fatalf("ToChannel got %#v, want rejection reason %#v", got, current.Reason())
				}
			default:
				t.Fatalf("chain remained pending after root settlement")
			}
		case <-time.After(time.Second):
			t.Fatalf("ToChannel did not receive settled value")
		}
	})
}
