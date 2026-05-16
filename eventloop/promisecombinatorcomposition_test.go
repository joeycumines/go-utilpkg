package eventloop

import (
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"testing"
)

func TestPromiseCombinatorsPreserveNilSettlements(t *testing.T) {
	tests := []struct {
		name  string
		build func(*JS) *ChainedPromise
		want  any
	}{
		{
			name: "All fulfillment",
			build: func(js *JS) *ChainedPromise {
				return js.All([]*ChainedPromise{js.Resolve("first"), js.Resolve(nil), js.Resolve("third")})
			},
			want: []any{"first", nil, "third"},
		},
		{
			name: "Race fulfillment",
			build: func(js *JS) *ChainedPromise {
				winner, resolve, _ := js.NewChainedPromise()
				pending, _, _ := js.NewChainedPromise()
				result := js.Race([]*ChainedPromise{winner, pending})
				resolve(nil)
				return result
			},
		},
		{
			name:  "AllSettled fulfillment",
			build: func(js *JS) *ChainedPromise { return js.AllSettled([]*ChainedPromise{js.Resolve(nil)}) },
			want:  []any{map[string]any{"status": "fulfilled", "value": nil}},
		},
		{
			name:  "AllSettled rejection",
			build: func(js *JS) *ChainedPromise { return js.AllSettled([]*ChainedPromise{js.Reject(nil)}) },
			want:  []any{map[string]any{"status": "rejected", "reason": nil}},
		},
		{
			name: "Any fulfillment",
			build: func(js *JS) *ChainedPromise {
				winner, resolve, _ := js.NewChainedPromise()
				pending, _, _ := js.NewChainedPromise()
				result := js.Any([]*ChainedPromise{winner, pending})
				resolve(nil)
				return result
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop, js := newCombinatorContractJS(t)
			result := test.build(js)
			loop.tick()
			if result.State() != Fulfilled || !reflect.DeepEqual(result.Value(), test.want) {
				t.Fatalf("result = (%v, %T %#v), want Fulfilled %#v", result.State(), result.Value(), result.Value(), test.want)
			}
		})
	}
}

func TestPromiseCombinatorsRejectNilWithoutPartialAttachment(t *testing.T) {
	loop, js := newCombinatorContractJS(t)
	source, _, _ := js.NewChainedPromise()

	combinators := map[string]func([]*ChainedPromise) *ChainedPromise{
		"all":        js.All,
		"race":       js.Race,
		"allSettled": js.AllSettled,
		"any":        js.Any,
	}
	for name, combinator := range combinators {
		t.Run(name, func(t *testing.T) {
			result := combinator([]*ChainedPromise{source, nil})
			if state := result.State(); state != Rejected {
				t.Fatalf("state = %v, want Rejected", state)
			}
			reason, ok := result.Reason().(error)
			var nilInput *NilPromiseError
			if !ok || !errors.As(reason, &nilInput) || nilInput.Index != 1 {
				t.Fatalf("reason = %T %#v, want NilPromiseError at index 1", result.Reason(), result.Reason())
			}
			if source.rejectionHandled.Load() {
				t.Fatal("valid source was marked handled before nil input validation")
			}
		})
	}

	loop.tick()
}

func TestPromiseCombinator_NestedPromises(t *testing.T) {
	t.Run("AllAdoptsNestedPromise", func(t *testing.T) {
		loop, js := newCombinatorContractJS(t)
		outer, resolveOuter, _ := js.NewChainedPromise()
		inner, resolveInner, _ := js.NewChainedPromise()
		result := js.All([]*ChainedPromise{outer})

		resolveOuter(inner)
		loop.tick()
		if result.State() != Pending {
			t.Fatalf("All state before nested settlement = %v, want Pending", result.State())
		}
		resolveInner("nested-value")
		loop.tick()

		got, ok := result.Value().([]any)
		if result.State() != Fulfilled || !ok || len(got) != 1 || got[0] != "nested-value" {
			t.Fatalf("All nested result = (%v, %T %#v), want Fulfilled []any{\"nested-value\"}", result.State(), result.Value(), result.Value())
		}
	})

	t.Run("AllContainsRace", func(t *testing.T) {
		loop, js := newCombinatorContractJS(t)
		p1a, resolve1a, _ := js.NewChainedPromise()
		p1b, _, _ := js.NewChainedPromise()
		p2a, _, _ := js.NewChainedPromise()
		p2b, resolve2b, _ := js.NewChainedPromise()
		result := js.All([]*ChainedPromise{
			js.Race([]*ChainedPromise{p1a, p1b}),
			js.Race([]*ChainedPromise{p2a, p2b}),
		})

		resolve2b("second-race")
		resolve1a("first-race")
		loop.tick()

		got, ok := result.Value().([]any)
		if result.State() != Fulfilled || !ok || len(got) != 2 || got[0] != "first-race" || got[1] != "second-race" {
			t.Fatalf("All of Race result = (%v, %T %#v), want ordered race winners", result.State(), result.Value(), result.Value())
		}
	})

	t.Run("RaceContainsAll", func(t *testing.T) {
		loop, js := newCombinatorContractJS(t)
		p1a, resolve1a, _ := js.NewChainedPromise()
		p1b, resolve1b, _ := js.NewChainedPromise()
		p2a, _, _ := js.NewChainedPromise()
		p2b, _, _ := js.NewChainedPromise()
		result := js.Race([]*ChainedPromise{
			js.All([]*ChainedPromise{p1a, p1b}),
			js.All([]*ChainedPromise{p2a, p2b}),
		})

		resolve1b("1b")
		resolve1a("1a")
		loop.tick()

		got, ok := result.Value().([]any)
		if result.State() != Fulfilled || !ok || len(got) != 2 || got[0] != "1a" || got[1] != "1b" {
			t.Fatalf("Race of All result = (%v, %T %#v), want []any{\"1a\", \"1b\"}", result.State(), result.Value(), result.Value())
		}
	})

	t.Run("DeepAdoption", func(t *testing.T) {
		loop, js := newCombinatorContractJS(t)
		const depth = 5
		chain := make([]*ChainedPromise, depth)
		resolves := make([]ResolveFunc, depth)
		for i := range chain {
			chain[i], resolves[i], _ = js.NewChainedPromise()
		}
		result := js.All([]*ChainedPromise{chain[0]})
		for i := range depth - 1 {
			resolves[i](chain[i+1])
		}
		resolves[depth-1]("deep-value")
		loop.tick()

		got, ok := result.Value().([]any)
		if result.State() != Fulfilled || !ok || len(got) != 1 || got[0] != "deep-value" {
			t.Fatalf("deep adoption result = (%v, %T %#v), want Fulfilled []any{\"deep-value\"}", result.State(), result.Value(), result.Value())
		}
	})
}

func TestPromiseAny_PartialRejectionThenFulfillment(t *testing.T) {
	loop, js := newCombinatorContractJS(t)
	first, _, rejectFirst := js.NewChainedPromise()
	winner, resolveWinner, _ := js.NewChainedPromise()
	last, _, rejectLast := js.NewChainedPromise()
	result := js.Any([]*ChainedPromise{first, winner, last})

	rejectFirst("first rejection")
	loop.tick()
	if result.State() != Pending {
		t.Fatalf("state after partial rejection = %v, want Pending", result.State())
	}

	resolveWinner("winner")
	loop.tick()
	if result.State() != Fulfilled || result.Value() != "winner" {
		t.Fatalf("result after fulfillment = (%v, %#v), want (Fulfilled, winner)", result.State(), result.Value())
	}

	rejectLast("late rejection")
	loop.tick()
	if result.State() != Fulfilled || result.Value() != "winner" {
		t.Fatalf("result after late rejection = (%v, %#v), want unchanged winner", result.State(), result.Value())
	}
}

func TestPromiseAny_ChainedPromise(t *testing.T) {
	loop, js := newCombinatorContractJS(t)
	base, resolveBase, _ := js.NewChainedPromise()
	chained := base.Then(func(value any) any {
		return value.(string) + "-transformed"
	}, nil)
	result := js.Any([]*ChainedPromise{chained})

	resolveBase("base")
	loop.tick()
	if result.State() != Fulfilled || result.Value() != "base-transformed" {
		t.Fatalf("Any chained result = (%v, %#v), want (Fulfilled, base-transformed)", result.State(), result.Value())
	}
}

func TestPromiseAll_CrossAdapterComposition(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	producer, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}
	consumer, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}
	promises := []*ChainedPromise{
		producer.Resolve("a").Then(func(value any) any { return value.(string) + "-transformed" }, nil),
		producer.Resolve("b").Then(func(value any) any { return value.(string) + "-transformed" }, nil),
		producer.Resolve("c").Then(func(value any) any { return value.(string) + "-transformed" }, nil),
	}
	result := consumer.All(promises)
	loop.tick()

	want := []any{"a-transformed", "b-transformed", "c-transformed"}
	if result.State() != Fulfilled || !reflect.DeepEqual(result.Value(), want) {
		t.Fatalf("cross-adapter All result = (%v, %#v), want Fulfilled %#v", result.State(), result.Value(), want)
	}
}

func FuzzPromiseCombinator_MixedOperations(f *testing.F) {
	f.Add(uint64(12345), 5, 3)
	f.Add(uint64(67890), 10, 5)
	f.Add(uint64(11111), 3, 2)
	f.Add(uint64(12345), -7, 3)

	f.Fuzz(func(t *testing.T, seed uint64, count, operations int) {
		if count < 1 || count > 20 || operations < 1 || operations > 5 {
			t.Skip()
		}
		loop, js := newCombinatorContractJS(t)
		rng := rand.New(rand.NewSource(int64(seed)))
		promises := make([]*ChainedPromise, count)
		resolvers := make([]ResolveFunc, 0, count+operations)
		for i := range promises {
			var resolve ResolveFunc
			promises[i], resolve, _ = js.NewChainedPromise()
			resolvers = append(resolvers, resolve)
		}

		var current *ChainedPromise
		finalKind := 0
		finalSubset := 0
		for operation := range operations {
			finalKind = rng.Intn(4)
			finalSubset = 1 + rng.Intn(len(promises))
			sources := promises[:finalSubset]
			switch finalKind {
			case 0:
				current = js.All(sources)
			case 1:
				current = js.Race(sources)
			case 2:
				current = js.AllSettled(sources)
			case 3:
				current = js.Any(sources)
			}
			if operation == operations-1 {
				break
			}

			pending, resolvePending, _ := js.NewChainedPromise()
			resolvers = append(resolvers, resolvePending)
			next := []*ChainedPromise{pending, current}
			if len(promises) > 2 {
				next = append(next, promises[2:]...)
			}
			promises = next
		}

		resultChannel := current.ToChannel()
		for sequence, index := range rng.Perm(len(resolvers)) {
			resolvers[index](fmt.Sprintf("value-%d", sequence))
		}
		loop.tick()
		waitContractValue(t, resultChannel, "mixed Promise combinator settlement")
		if current.State() != Fulfilled {
			t.Fatalf("mixed combinator state = %v, want Fulfilled", current.State())
		}

		switch finalKind {
		case 0:
			values, ok := current.Value().([]any)
			if !ok || len(values) != finalSubset {
				t.Fatalf("final All value = %T %#v, want []any length %d", current.Value(), current.Value(), finalSubset)
			}
		case 1, 3:
			if current.Value() == nil {
				t.Fatalf("final winner value = nil, want settled source value")
			}
		case 2:
			values, ok := current.Value().([]any)
			if !ok || len(values) != finalSubset {
				t.Fatalf("final AllSettled value = %T %#v, want []any length %d", current.Value(), current.Value(), finalSubset)
			}
			for i, raw := range values {
				entry, entryOK := raw.(map[string]any)
				if !entryOK || entry["status"] != "fulfilled" {
					t.Fatalf("final AllSettled[%d] = %T %#v, want fulfilled outcome", i, raw, raw)
				}
			}
		}
	})
}
