package eventloop

import (
	"context"
	"runtime"
	"testing"
	"weak"
)

type combinatorRetentionPayload struct {
	value int
	_     [32]byte
}

type combinatorRetentionProof struct {
	promises []weak.Pointer[ChainedPromise]
	payload  weak.Pointer[combinatorRetentionPayload]
}

func TestSettledPromiseCombinatorsReleasePromiseGraph(t *testing.T) {
	for _, name := range []string{"all", "race", "allSettled", "any"} {
		t.Run(name, func(t *testing.T) {
			loop := New()
			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(context.Background()) }()
			ready := make(chan struct{})
			if err := loop.Submit(func() { close(ready) }); err != nil {
				t.Fatalf("warmup Submit: %v", err)
			}
			waitContractSignal(t, ready, "combinator-retention loop warmup")
			t.Cleanup(func() {
				if err := loop.Close(); err != nil {
					t.Errorf("Close: %v", err)
				}
				if err := waitContractValue(t, runDone, "combinator-retention loop completion"); err != nil {
					t.Errorf("Run: %v", err)
				}
			})
			reported := make(chan any, 1)
			js := NewJS(loop, WithUnhandledRejection(func(reason any) { reported <- reason }))

			proof := settledCombinatorRetentionProof(t, loop, js, name)
			for _, pointer := range proof.promises {
				waitContractCollected(t, pointer, js)
			}
			waitContractCollected(t, proof.payload, js)
			select {
			case reason := <-reported:
				t.Fatalf("handled combinator input was reported with reason %#v", reason)
			default:
			}
		})
	}
}

func settledCombinatorRetentionProof(t *testing.T, loop *Loop, js *JS, name string) combinatorRetentionProof {
	t.Helper()
	payload := &combinatorRetentionPayload{value: 7}
	proof := combinatorRetentionProof{payload: weak.Make(payload)}
	inputs := make([]*ChainedPromise, 3)
	resolvers := make([]ResolveFunc, len(inputs))
	rejecters := make([]RejectFunc, len(inputs))
	for index := range inputs {
		inputs[index], resolvers[index], rejecters[index] = js.NewChainedPromise()
		proof.promises = append(proof.promises, weak.Make(inputs[index]))
	}

	var result *ChainedPromise
	switch name {
	case "all":
		result = js.All(inputs)
		for index, resolve := range resolvers {
			resolve(struct {
				index   int
				payload *combinatorRetentionPayload
			}{index: index, payload: payload})
		}
	case "race":
		result = js.Race(inputs)
		resolvers[1](payload)
		resolvers[0]("late zero")
		resolvers[2]("late two")
	case "allSettled":
		result = js.AllSettled(inputs)
		resolvers[0](payload)
		rejecters[1]("handled rejection")
		resolvers[2]("fulfilled")
	case "any":
		result = js.Any(inputs)
		rejecters[0]("handled zero")
		resolvers[2](payload)
		rejecters[1]("handled one")
	default:
		t.Fatalf("unknown combinator %q", name)
	}
	proof.promises = append(proof.promises, weak.Make(result))
	if got := waitContractValue(t, result.ToChannel(), name+" settlement"); got == nil {
		t.Fatalf("%s result = nil, want fulfillment", name)
	}
	if state := result.State(); state != Fulfilled {
		t.Fatalf("%s state = %v, want Fulfilled", name, state)
	}
	barrier := make(chan struct{})
	if err := loop.scheduleMicrotaskCheckpoint(func() { close(barrier) }); err != nil {
		t.Fatalf("schedule %s cleanup barrier: %v", name, err)
	}
	waitContractSignal(t, barrier, name+" cleanup barrier")
	runtime.KeepAlive(inputs)
	runtime.KeepAlive(result)
	runtime.KeepAlive(payload)
	return proof
}
