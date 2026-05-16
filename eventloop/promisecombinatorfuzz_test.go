package eventloop

import (
	"fmt"
	"math/rand"
	"testing"
)

func newCombinatorContractJS(t *testing.T) (*Loop, *JS) {
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

func combinatorSettlementOrder(count int, seed uint64, mode int64, salt int) []int {
	order := make([]int, count)
	for i := range order {
		order[i] = i
	}
	if count < 2 {
		return order
	}

	modeCase := mode % 3
	if modeCase < 0 {
		modeCase += 3
	}
	switch modeCase {
	case 1:
		for left, right := 0, count-1; left < right; left, right = left+1, right-1 {
			order[left], order[right] = order[right], order[left]
		}
	case 2:
		rng := rand.New(rand.NewSource(int64(seed ^ uint64(mode) ^ uint64(salt))))
		rng.Shuffle(count, func(i, j int) { order[i], order[j] = order[j], order[i] })
	}

	rotation := int((seed ^ uint64(salt)) % uint64(count))
	return append(order[rotation:], order[:rotation]...)
}

func FuzzPromiseAll(f *testing.F) {
	f.Add(uint64(12345), int64(0), 5, 50)
	f.Add(uint64(67890), int64(1), 10, 100)
	f.Add(uint64(11111), int64(2), 20, 200)
	f.Add(uint64(22222), int64(3), 3, 10)
	f.Add(uint64(33333), int64(4), 100, 50)

	f.Fuzz(func(t *testing.T, seed uint64, mode int64, count, orderSalt int) {
		if count < 1 || count > 200 {
			t.Skip()
		}
		loop, js := newCombinatorContractJS(t)
		promises := make([]*ChainedPromise, count)
		resolves := make([]ResolveFunc, count)
		want := make([]any, count)
		for i := range promises {
			promises[i], resolves[i], _ = js.NewChainedPromise()
			want[i] = fmt.Sprintf("value-%d", i)
		}

		result := js.All(promises)
		resultChannel := result.ToChannel()
		for _, index := range combinatorSettlementOrder(count, seed, mode, orderSalt) {
			resolves[index](want[index])
		}
		loop.tick()
		waitContractValue(t, resultChannel, "Promise.All settlement")

		if result.State() != Fulfilled {
			t.Fatalf("All state = %v, want Fulfilled", result.State())
		}
		got, ok := result.Value().([]any)
		if !ok || len(got) != len(want) {
			t.Fatalf("All value = %T %#v, want []any length %d", result.Value(), result.Value(), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("All value[%d] = %#v, want %#v", i, got[i], want[i])
			}
		}
	})
}

func FuzzPromiseRace(f *testing.F) {
	f.Add(uint64(12345), 5, 100, true)
	f.Add(uint64(67890), 10, 50, false)
	f.Add(uint64(11111), 20, 200, true)
	f.Add(uint64(22222), 3, 10, false)
	f.Add(uint64(33333), 50, 30, true)

	f.Fuzz(func(t *testing.T, seed uint64, count, orderSalt int, fulfillWinner bool) {
		if count < 2 || count > 100 {
			t.Skip()
		}
		loop, js := newCombinatorContractJS(t)
		promises := make([]*ChainedPromise, count)
		resolves := make([]ResolveFunc, count)
		rejects := make([]RejectFunc, count)
		for i := range promises {
			promises[i], resolves[i], rejects[i] = js.NewChainedPromise()
		}
		order := combinatorSettlementOrder(count, seed, int64(orderSalt), orderSalt)
		winner := order[0]
		winnerText := fmt.Sprintf("winner-%d", winner)
		result := js.Race(promises)
		resultChannel := result.ToChannel()

		if fulfillWinner {
			resolves[winner](winnerText)
		} else {
			rejects[winner](fmt.Errorf("%s", winnerText))
		}
		loop.tick()
		waitContractValue(t, resultChannel, "Promise.Race winner settlement")
		if fulfillWinner {
			if result.State() != Fulfilled || result.Value() != winnerText {
				t.Fatalf("Race winner = (%v, %#v), want Fulfilled %#v", result.State(), result.Value(), winnerText)
			}
		} else {
			reason, ok := result.Reason().(error)
			if result.State() != Rejected || !ok || reason.Error() != winnerText {
				t.Fatalf("Race winner = (%v, %T %#v), want Rejected error %q", result.State(), result.Reason(), result.Reason(), winnerText)
			}
		}

		for position, index := range order[1:] {
			if (position+int(seed&1))%2 == 0 {
				resolves[index](fmt.Sprintf("loser-%d", index))
			} else {
				rejects[index](fmt.Errorf("loser-%d", index))
			}
		}
		loop.tick()
		if fulfillWinner {
			if result.State() != Fulfilled || result.Value() != winnerText {
				t.Fatalf("Race changed after loser settlements: (%v, %#v)", result.State(), result.Value())
			}
		} else {
			reason, ok := result.Reason().(error)
			if result.State() != Rejected || !ok || reason.Error() != winnerText {
				t.Fatalf("Race changed after loser settlements: (%v, %T %#v)", result.State(), result.Reason(), result.Reason())
			}
		}
	})
}

func FuzzPromiseAllSettled(f *testing.F) {
	f.Add(uint64(12345), 10, 50, uint64(0xAAAA))
	f.Add(uint64(67890), 5, 100, uint64(0x0000))
	f.Add(uint64(11111), 5, 100, uint64(0xFFFF))
	f.Add(uint64(22222), 20, 30, uint64(0x5555))
	f.Add(uint64(33333), 100, 10, uint64(0x1234))

	f.Fuzz(func(t *testing.T, seed uint64, count, orderSalt int, rejectMask uint64) {
		if count < 1 || count > 150 {
			t.Skip()
		}
		loop, js := newCombinatorContractJS(t)
		promises := make([]*ChainedPromise, count)
		resolves := make([]ResolveFunc, count)
		rejects := make([]RejectFunc, count)
		fulfilled := make([]bool, count)
		for i := range promises {
			promises[i], resolves[i], rejects[i] = js.NewChainedPromise()
			fulfilled[i] = (rejectMask>>uint(i%64))&1 == 0
		}

		result := js.AllSettled(promises)
		resultChannel := result.ToChannel()
		for _, index := range combinatorSettlementOrder(count, seed, int64(orderSalt), orderSalt) {
			if fulfilled[index] {
				resolves[index](fmt.Sprintf("value-%d", index))
			} else {
				rejects[index](fmt.Errorf("error-%d", index))
			}
		}
		loop.tick()
		waitContractValue(t, resultChannel, "Promise.AllSettled settlement")

		if result.State() != Fulfilled {
			t.Fatalf("AllSettled state = %v, want Fulfilled", result.State())
		}
		got, ok := result.Value().([]any)
		if !ok || len(got) != count {
			t.Fatalf("AllSettled value = %T %#v, want []any length %d", result.Value(), result.Value(), count)
		}
		for i, raw := range got {
			entry, ok := raw.(map[string]any)
			if !ok {
				t.Fatalf("AllSettled[%d] = %T %#v, want map[string]any", i, raw, raw)
			}
			if fulfilled[i] {
				want := fmt.Sprintf("value-%d", i)
				if entry["status"] != "fulfilled" || entry["value"] != want {
					t.Fatalf("AllSettled[%d] = %#v, want fulfilled value %q", i, entry, want)
				}
				continue
			}
			reason, reasonOK := entry["reason"].(error)
			want := fmt.Sprintf("error-%d", i)
			if entry["status"] != "rejected" || !reasonOK || reason.Error() != want {
				t.Fatalf("AllSettled[%d] = %#v, want rejected error %q", i, entry, want)
			}
		}
	})
}

func FuzzPromiseAny(f *testing.F) {
	f.Add(uint64(12345), 10, 50, 1)
	f.Add(uint64(67890), 5, 100, 0)
	f.Add(uint64(11111), 20, 30, 5)
	f.Add(uint64(22222), 3, 10, 3)
	f.Add(uint64(33333), 50, 20, 10)

	f.Fuzz(func(t *testing.T, seed uint64, count, orderSalt, resolveCount int) {
		if count < 1 || count > 100 || resolveCount < 0 || resolveCount > count {
			t.Skip()
		}
		loop, js := newCombinatorContractJS(t)
		promises := make([]*ChainedPromise, count)
		resolves := make([]ResolveFunc, count)
		rejects := make([]RejectFunc, count)
		for i := range promises {
			promises[i], resolves[i], rejects[i] = js.NewChainedPromise()
		}
		order := combinatorSettlementOrder(count, seed, int64(resolveCount), orderSalt)
		willResolve := make([]bool, count)
		for _, index := range order[:resolveCount] {
			willResolve[index] = true
		}
		result := js.Any(promises)
		resultChannel := result.ToChannel()

		if resolveCount > 0 {
			winner := order[0]
			want := fmt.Sprintf("success-%d", winner)
			resolves[winner](want)
			loop.tick()
			waitContractValue(t, resultChannel, "Promise.Any winning fulfillment")
			if result.State() != Fulfilled || result.Value() != want {
				t.Fatalf("Any winner = (%v, %#v), want Fulfilled %#v", result.State(), result.Value(), want)
			}
			for _, index := range order[1:] {
				if willResolve[index] {
					resolves[index](fmt.Sprintf("success-%d", index))
				} else {
					rejects[index](fmt.Errorf("error-%d", index))
				}
			}
			loop.tick()
			if result.State() != Fulfilled || result.Value() != want {
				t.Fatalf("Any changed after remaining settlements: (%v, %#v)", result.State(), result.Value())
			}
			return
		}

		for _, index := range order {
			rejects[index](fmt.Errorf("error-%d", index))
		}
		loop.tick()
		waitContractValue(t, resultChannel, "Promise.Any aggregate rejection")
		aggregate, ok := result.Reason().(*AggregateError)
		if result.State() != Rejected || !ok {
			t.Fatalf("Any all-rejected result = (%v, %T %#v), want Rejected *AggregateError", result.State(), result.Reason(), result.Reason())
		}
		if len(aggregate.Errors) != count {
			t.Fatalf("AggregateError length = %d, want %d", len(aggregate.Errors), count)
		}
		for i, raw := range aggregate.Errors {
			reason, reasonOK := raw.(error)
			want := fmt.Sprintf("error-%d", i)
			if !reasonOK || reason.Error() != want {
				t.Fatalf("AggregateError[%d] = %T %#v, want error %q", i, raw, raw, want)
			}
		}
	})
}
