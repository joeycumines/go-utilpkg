package tournament

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop"
)

const (
	promiseSuccessorFanOut = 100
	promiseSuccessorWinner = 37
	promiseSuccessorBatch  = 64
)

type promiseBenchmarkValue uint64

type promiseConstructionCase struct {
	root    Promise
	tail    Promise
	resolve eventloop.ResolveFunc
}

func BenchmarkPromiseChainEndToEndV3(b *testing.B) {
	for _, implementation := range PromiseImplementations() {
		b.Run(implementation.Name, func(b *testing.B) {
			for _, depth := range []int{1, 10, 100} {
				b.Run("Depth="+promiseBenchmarkInteger(depth), func(b *testing.B) {
					benchmarkPromiseChainSuccessor(b, implementation, depth)
				})
			}
		})
	}
}

func BenchmarkPromiseResolvedReactionEndToEndV3(b *testing.B) {
	for _, implementation := range PromiseImplementations() {
		b.Run(implementation.Name, func(b *testing.B) {
			loop, js, cleanup := startPromiseBenchmarkLoop(b)
			defer cleanup()
			root, resolve, _ := implementation.Factory(js)
			resolve(promiseBenchmarkValue(41))
			promiseSuccessorRequireSettlement(b, root, promiseBenchmarkValue(41))
			deadline := time.NewTimer(30 * time.Minute)
			defer deadline.Stop()
			b.ReportAllocs()
			b.ResetTimer()
			b.StopTimer()
			for range b.N {
				b.StartTimer()
				child := root.Then(promiseSuccessorIncrement, promiseSuccessorRejectMarker)
				promiseSuccessorCheckpoint(b, loop, deadline.C)
				b.StopTimer()
				promiseSuccessorRequireSettlement(b, child, promiseBenchmarkValue(42))
				runtime.KeepAlive(child)
			}
		})
	}
}

func BenchmarkPromiseFanOut100EndToEndV3(b *testing.B) {
	for _, implementation := range PromiseImplementations() {
		b.Run(implementation.Name, func(b *testing.B) {
			loop, js, cleanup := startPromiseBenchmarkLoop(b)
			defer cleanup()
			deadline := time.NewTimer(30 * time.Minute)
			defer deadline.Stop()
			b.ReportAllocs()
			b.ResetTimer()
			b.StopTimer()
			for iteration := range b.N {
				b.StartTimer()
				root, resolve, _ := implementation.Factory(js)
				seed := promiseBenchmarkValue(iteration + 1)
				children := make([]Promise, promiseSuccessorFanOut)
				for index := range children {
					offset := promiseBenchmarkValue(index)
					children[index] = root.Then(func(value any) any {
						typed, ok := value.(promiseBenchmarkValue)
						if !ok {
							return promiseBenchmarkTypeFailure{value: value}
						}
						return typed + offset
					}, promiseSuccessorRejectMarker)
				}
				resolve(seed)
				promiseSuccessorCheckpoint(b, loop, deadline.C)
				b.StopTimer()
				for index, child := range children {
					promiseSuccessorRequireSettlement(b, child, seed+promiseBenchmarkValue(index))
				}
				runtime.KeepAlive(root)
				runtime.KeepAlive(children)
			}
		})
	}
}

func BenchmarkPromiseRace100UniqueWinnerEndToEndV3(b *testing.B) {
	for _, implementation := range PromiseImplementations() {
		b.Run(implementation.Name, func(b *testing.B) {
			if !implementation.RaceAssessment.Verified() {
				b.Skipf("%s Race assessment %d: %s", implementation.VariantID, implementation.RaceAssessment.Status, implementation.RaceAssessment.Reason)
			}
			loop, js, cleanup := startPromiseBenchmarkLoop(b)
			defer cleanup()
			deadline := time.NewTimer(30 * time.Minute)
			defer deadline.Stop()
			b.ReportAllocs()
			b.ResetTimer()
			b.StopTimer()
			for iteration := range b.N {
				b.StartTimer()
				combinator := implementation.RaceCase(js, promiseSuccessorFanOut)
				seed := promiseBenchmarkValue(iteration*promiseSuccessorFanOut + 1)
				promiseSuccessorSettleRaceMeasured(combinator.Resolvers, promiseSuccessorWinner, seed)
				promiseSuccessorCheckpoint(b, loop, deadline.C)
				b.StopTimer()
				promiseSuccessorRequireSettlement(b, combinator.Promise, seed+promiseSuccessorWinner)
				runtime.KeepAlive(combinator)
			}
		})
	}
}

func BenchmarkPromiseAll100OrderedEndToEndV3(b *testing.B) {
	for _, implementation := range PromiseImplementations() {
		b.Run(implementation.Name, func(b *testing.B) {
			if !implementation.AllAssessment.Verified() {
				b.Skipf("%s All assessment %d: %s", implementation.VariantID, implementation.AllAssessment.Status, implementation.AllAssessment.Reason)
			}
			loop, js, cleanup := startPromiseBenchmarkLoop(b)
			defer cleanup()
			deadline := time.NewTimer(30 * time.Minute)
			defer deadline.Stop()
			b.ReportAllocs()
			b.ResetTimer()
			b.StopTimer()
			for iteration := range b.N {
				b.StartTimer()
				combinator := implementation.AllCase(js, promiseSuccessorFanOut)
				seed := promiseBenchmarkValue(iteration*promiseSuccessorFanOut + 1)
				for index := len(combinator.Resolvers) - 1; index >= 0; index-- {
					combinator.Resolvers[index](seed + promiseBenchmarkValue(index))
				}
				promiseSuccessorCheckpoint(b, loop, deadline.C)
				b.StopTimer()
				promiseSuccessorRequireOrderedAll(b, combinator.Promise, seed, promiseSuccessorFanOut)
				runtime.KeepAlive(combinator)
			}
		})
	}
}

func BenchmarkPromiseFreshChain100ConstructionV3(b *testing.B) {
	for _, implementation := range PromiseImplementations() {
		b.Run(implementation.Name, func(b *testing.B) {
			loop, js, cleanup := startPromiseBenchmarkLoop(b)
			defer cleanup()
			deadline := time.NewTimer(30 * time.Minute)
			defer deadline.Stop()
			cases := make([]promiseConstructionCase, promiseSuccessorBatch)
			b.ReportAllocs()
			b.ResetTimer()
			b.StopTimer()
			for completed := 0; completed < b.N; {
				count := min(b.N-completed, len(cases))
				b.StartTimer()
				for index := range count {
					root, resolve, _ := implementation.Factory(js)
					tail := root
					for range 100 {
						tail = tail.Then(promiseSuccessorIdentity, promiseSuccessorRejectMarker)
					}
					cases[index] = promiseConstructionCase{root: root, tail: tail, resolve: resolve}
				}
				b.StopTimer()
				for index := range count {
					cases[index].resolve(promiseBenchmarkValue(1))
				}
				promiseSuccessorCheckpoint(b, loop, deadline.C)
				for index := range count {
					promiseSuccessorRequireSettlement(b, cases[index].tail, promiseBenchmarkValue(1))
					cases[index] = promiseConstructionCase{}
				}
				completed += count
			}
			runtime.KeepAlive(cases)
		})
	}
}

func benchmarkPromiseChainSuccessor(b *testing.B, implementation PromiseImplementation, depth int) {
	b.Helper()
	loop, js, cleanup := startPromiseBenchmarkLoop(b)
	defer cleanup()
	deadline := time.NewTimer(30 * time.Minute)
	defer deadline.Stop()
	b.ReportAllocs()
	b.ResetTimer()
	b.StopTimer()
	for iteration := range b.N {
		b.StartTimer()
		root, resolve, _ := implementation.Factory(js)
		tail := root
		for range depth {
			tail = tail.Then(promiseSuccessorIncrement, promiseSuccessorRejectMarker)
		}
		seed := promiseBenchmarkValue(iteration + 1)
		resolve(seed)
		promiseSuccessorCheckpoint(b, loop, deadline.C)
		b.StopTimer()
		promiseSuccessorRequireSettlement(b, tail, seed+promiseBenchmarkValue(depth))
		runtime.KeepAlive(root)
		runtime.KeepAlive(tail)
	}
}

type promiseBenchmarkTypeFailure struct {
	value any
}

type promiseBenchmarkRejection struct {
	reason any
}

func promiseSuccessorIncrement(value any) any {
	typed, ok := value.(promiseBenchmarkValue)
	if !ok {
		return promiseBenchmarkTypeFailure{value: value}
	}
	return typed + 1
}

func promiseSuccessorIdentity(value any) any {
	if _, ok := value.(promiseBenchmarkValue); !ok {
		return promiseBenchmarkTypeFailure{value: value}
	}
	return value
}

func promiseSuccessorRejectMarker(reason any) any {
	return promiseBenchmarkRejection{reason: reason}
}

func promiseSuccessorCheckpoint(t testing.TB, loop *eventloop.Loop, deadline <-chan time.Time) {
	t.Helper()
	done := make(chan struct{})
	if err := loop.ScheduleMicrotaskCheckpoint(func() { close(done) }); err != nil {
		t.Fatalf("schedule Promise successor checkpoint: %v", err)
	}
	waitPromiseBenchmarkDeadline(t, done, deadline)
}

func promiseSuccessorRequireSettlement(t testing.TB, promise Promise, want promiseBenchmarkValue) {
	t.Helper()
	settlement := promise.Settlement()
	if settlement.State != PromiseSettlementFulfilled || settlement.Value != want {
		t.Fatalf("settlement = {state:%d value:%#v}, want fulfilled %#v", settlement.State, settlement.Value, want)
	}
}

func promiseSuccessorRequireResolvers(t testing.TB, combinator PromiseCombinatorCase, want int) {
	t.Helper()
	if combinator.Promise == nil || combinator.Retention == nil || len(combinator.Resolvers) != want {
		t.Fatalf("combinator case = {promise:%t retention:%t resolvers:%d}, want complete case with %d resolvers", combinator.Promise != nil, combinator.Retention != nil, len(combinator.Resolvers), want)
	}
	for index, resolve := range combinator.Resolvers {
		if resolve == nil {
			t.Fatalf("resolver %d is nil", index)
		}
	}
}

func promiseSuccessorSettleRace(resolvers []eventloop.ResolveFunc, winner int, seed promiseBenchmarkValue) error {
	if winner < 0 || winner >= len(resolvers) {
		return fmt.Errorf("promise successor winner index %d outside %d resolvers", winner, len(resolvers))
	}
	for index, resolve := range resolvers {
		if resolve == nil {
			return fmt.Errorf("promise successor resolver %d is nil", index)
		}
	}
	resolvers[winner](seed + promiseBenchmarkValue(winner))
	for index, resolve := range resolvers {
		if index != winner {
			resolve(seed + promiseBenchmarkValue(index))
		}
	}
	return nil
}

func promiseSuccessorSettleRaceMeasured(resolvers []eventloop.ResolveFunc, winner int, seed promiseBenchmarkValue) {
	resolvers[winner](seed + promiseBenchmarkValue(winner))
	for index, resolve := range resolvers {
		if index != winner {
			resolve(seed + promiseBenchmarkValue(index))
		}
	}
}

func promiseSuccessorRequireOrderedAll(t testing.TB, promise Promise, seed promiseBenchmarkValue, count int) {
	t.Helper()
	settlement := promise.Settlement()
	if settlement.State != PromiseSettlementFulfilled {
		t.Fatalf("All settlement state = %d, want fulfilled", settlement.State)
	}
	values, ok := settlement.Value.([]any)
	if !ok || len(values) != count {
		t.Fatalf("All settlement value = %#v, want []any length %d", settlement.Value, count)
	}
	for index, value := range values {
		want := seed + promiseBenchmarkValue(index)
		if value != want {
			t.Fatalf("All value %d = %#v, want %#v", index, value, want)
		}
	}
}

func promiseBenchmarkInteger(value int) string {
	if value == 1 {
		return "1"
	}
	if value == 10 {
		return "10"
	}
	if value == 100 {
		return "100"
	}
	panic("tournament: unsupported Promise benchmark integer")
}
