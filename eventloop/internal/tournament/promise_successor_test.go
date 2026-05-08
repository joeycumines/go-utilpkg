package tournament

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop"
)

func TestPromiseSuccessorImplementationCapabilities(t *testing.T) {
	type behaviorStatuses struct {
		sequential             PromiseAssessmentStatus
		pendingRejection       PromiseAssessmentStatus
		settledRejection       PromiseAssessmentStatus
		concurrentObservation  PromiseAssessmentStatus
		concurrentRegistration PromiseAssessmentStatus
		all                    PromiseAssessmentStatus
		race                   PromiseAssessmentStatus
	}
	wants := map[string]behaviorStatuses{
		"promise.main.chained": {
			PromiseAssessmentVerified, PromiseAssessmentVerified, PromiseAssessmentVerified,
			PromiseAssessmentVerified, PromiseAssessmentVerified, PromiseAssessmentVerified, PromiseAssessmentVerified,
		},
		"promise.alt-one.embedded-first-handler": {
			PromiseAssessmentVerified, PromiseAssessmentVerified, PromiseAssessmentVerified,
			PromiseAssessmentVerified, PromiseAssessmentVerified, PromiseAssessmentInvalid, PromiseAssessmentVerified,
		},
		"promise.alt-two.treiber": {
			PromiseAssessmentVerified, PromiseAssessmentVerified, PromiseAssessmentVerified,
			PromiseAssessmentInvalid, PromiseAssessmentVerified, PromiseAssessmentNotApplicable, PromiseAssessmentNotApplicable,
		},
		"promise.alt-three.pooled-treiber": {
			PromiseAssessmentVerified, PromiseAssessmentVerified, PromiseAssessmentVerified,
			PromiseAssessmentInvalid, PromiseAssessmentVerified, PromiseAssessmentNotApplicable, PromiseAssessmentNotApplicable,
		},
		"promise.alt-four.main-snapshot": {
			PromiseAssessmentVerified, PromiseAssessmentVerified, PromiseAssessmentInvalid,
			PromiseAssessmentInvalid, PromiseAssessmentInvalid, PromiseAssessmentVerified, PromiseAssessmentVerified,
		},
		"promise.alt-five.original-chained": {
			PromiseAssessmentVerified, PromiseAssessmentVerified, PromiseAssessmentInvalid,
			PromiseAssessmentVerified, PromiseAssessmentVerified, PromiseAssessmentNotApplicable, PromiseAssessmentNotApplicable,
		},
	}
	implementations := PromiseImplementations()
	if len(implementations) != len(wants) {
		t.Fatalf("implementation count = %d, want %d", len(implementations), len(wants))
	}
	seen := make(map[string]struct{}, len(implementations))
	for _, implementation := range implementations {
		if implementation.Name == "" || implementation.VariantID == "" || implementation.Factory == nil {
			t.Errorf("incomplete implementation: %+v", implementation)
		}
		if _, duplicate := seen[implementation.VariantID]; duplicate {
			t.Errorf("duplicate implementation ID %q", implementation.VariantID)
		}
		seen[implementation.VariantID] = struct{}{}
		want, ok := wants[implementation.VariantID]
		if !ok {
			t.Errorf("unexpected implementation ID %q", implementation.VariantID)
			continue
		}
		got := behaviorStatuses{
			sequential:             implementation.SequentialFulfillment.Status,
			pendingRejection:       implementation.PendingRejectionPropagation.Status,
			settledRejection:       implementation.SettledRejectionPropagation.Status,
			concurrentObservation:  implementation.ConcurrentSettlementObservation.Status,
			concurrentRegistration: implementation.ConcurrentHandlerRegistration.Status,
			all:                    implementation.AllAssessment.Status,
			race:                   implementation.RaceAssessment.Status,
		}
		if got != want {
			t.Errorf("%s behavior statuses = %+v, want %+v", implementation.VariantID, got, want)
		}
		assessments := []PromiseAssessment{
			implementation.SequentialFulfillment,
			implementation.PendingRejectionPropagation,
			implementation.SettledRejectionPropagation,
			implementation.ConcurrentSettlementObservation,
			implementation.ConcurrentHandlerRegistration,
			implementation.AllAssessment,
			implementation.RaceAssessment,
		}
		for index, assessment := range assessments {
			if assessment.Status == PromiseAssessmentUnassessed || assessment.Reason == "" {
				t.Errorf("%s assessment %d = %+v, want classified with reason", implementation.VariantID, index, assessment)
			}
		}
		if gotCase := implementation.AllCase != nil; gotCase != (want.all != PromiseAssessmentNotApplicable) {
			t.Errorf("%s All case presence = %t, assessment = %d", implementation.VariantID, gotCase, want.all)
		}
		if gotCase := implementation.RaceCase != nil; gotCase != (want.race != PromiseAssessmentNotApplicable) {
			t.Errorf("%s Race case presence = %t, assessment = %d", implementation.VariantID, gotCase, want.race)
		}
	}
}

func TestPromiseSuccessorSettlementAdapters(t *testing.T) {
	for _, implementation := range PromiseImplementations() {
		t.Run(implementation.Name, func(t *testing.T) {
			loop, js := startPromiseAdapterTestLoop(t)
			deadline := time.NewTimer(5 * time.Second)
			defer deadline.Stop()

			pending, _, _ := implementation.Factory(js)
			if got := pending.Settlement(); got.State != PromiseSettlementPending || got.Value != nil {
				t.Fatalf("pending settlement = %+v, want pending nil", got)
			}

			fulfilled, resolve, _ := implementation.Factory(js)
			resolve(promiseBenchmarkValue(42))
			promiseSuccessorCheckpoint(t, loop, deadline.C)
			promiseSuccessorRequireSettlement(t, fulfilled, promiseBenchmarkValue(42))

			fulfilledNil, resolveNil, _ := implementation.Factory(js)
			resolveNil(nil)
			promiseSuccessorCheckpoint(t, loop, deadline.C)
			if got := fulfilledNil.Settlement(); got.State != PromiseSettlementFulfilled || got.Value != nil {
				t.Fatalf("fulfilled-nil settlement = %+v, want fulfilled nil", got)
			}

			rejected, _, reject := implementation.Factory(js)
			reason := errors.New("promise successor rejection")
			reject(reason)
			promiseSuccessorCheckpoint(t, loop, deadline.C)
			got := rejected.Settlement()
			if got.State != PromiseSettlementRejected || got.Value != reason {
				t.Fatalf("rejected settlement = %+v, want rejected %v", got, reason)
			}

			rejectedNil, _, rejectNil := implementation.Factory(js)
			rejectNil(nil)
			promiseSuccessorCheckpoint(t, loop, deadline.C)
			if got := rejectedNil.Settlement(); got.State != PromiseSettlementRejected || got.Value != nil {
				t.Fatalf("rejected-nil settlement = %+v, want rejected nil", got)
			}
		})
	}
}

func TestPromiseSuccessorTailWaitsCallbackReturn(t *testing.T) {
	for _, implementation := range PromiseImplementations() {
		t.Run(implementation.Name, func(t *testing.T) {
			loop, js := startPromiseAdapterTestLoop(t)
			root, resolve, _ := implementation.Factory(js)
			started := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			releaseCallback := func() { releaseOnce.Do(func() { close(release) }) }
			t.Cleanup(releaseCallback)
			child := root.Then(func(value any) any {
				close(started)
				<-release
				return promiseSuccessorIncrement(value)
			}, promiseSuccessorRejectMarker)
			resolve(promiseBenchmarkValue(41))
			waitPromiseAdapterTestSignal(t, started, "blocked successor reaction")
			checkpoint := make(chan PromiseSettlement, 1)
			if err := loop.ScheduleMicrotaskCheckpoint(func() { checkpoint <- child.Settlement() }); err != nil {
				t.Fatalf("schedule checkpoint: %v", err)
			}
			releaseCallback()
			select {
			case got := <-checkpoint:
				if got.State != PromiseSettlementFulfilled || got.Value != promiseBenchmarkValue(42) {
					t.Fatalf("checkpoint child settlement = %+v, want fulfilled 42", got)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("successor tail checkpoint timed out")
			}
		})
	}
}

func TestPromiseSuccessorRejectionPropagation(t *testing.T) {
	for _, implementation := range PromiseImplementations() {
		t.Run(implementation.Name, func(t *testing.T) {
			loop, js := startPromiseAdapterTestLoop(t)
			for _, settled := range []bool{false, true} {
				phase := "Pending"
				if settled {
					phase = "Settled"
				}
				t.Run(phase, func(t *testing.T) {
					reason := errors.New("promise successor propagation")

					explicitRoot, _, explicitReject := implementation.Factory(js)
					var explicitChild Promise
					if !settled {
						explicitChild = explicitRoot.Then(nil, func(got any) any {
							if got != reason {
								return promiseBenchmarkRejection{reason: got}
							}
							return promiseBenchmarkValue(42)
						})
					}
					explicitReject(reason)
					promiseSuccessorTestCheckpoint(t, loop)
					if settled {
						explicitChild = explicitRoot.Then(nil, func(got any) any {
							if got != reason {
								return promiseBenchmarkRejection{reason: got}
							}
							return promiseBenchmarkValue(42)
						})
					}
					promiseSuccessorTestCheckpoint(t, loop)
					promiseSuccessorRequireSettlement(t, explicitChild, promiseBenchmarkValue(42))

					passthroughRoot, _, passthroughReject := implementation.Factory(js)
					var passthroughChild Promise
					if !settled {
						passthroughChild = passthroughRoot.Then(nil, nil)
					}
					passthroughReject(reason)
					promiseSuccessorTestCheckpoint(t, loop)
					if settled {
						passthroughChild = passthroughRoot.Then(nil, nil)
					}
					promiseSuccessorTestCheckpoint(t, loop)
					got := passthroughChild.Settlement()
					assessment := implementation.PendingRejectionPropagation
					if settled {
						assessment = implementation.SettledRejectionPropagation
					}
					if assessment.Verified() {
						if got.State != PromiseSettlementRejected || got.Value != reason {
							t.Fatalf("nil-handler settlement = %+v, want rejected identical reason", got)
						}
						return
					}
					if assessment.Status != PromiseAssessmentInvalid || !settled {
						t.Fatalf("unexpected rejection assessment %+v with settlement %+v", assessment, got)
					}
					switch implementation.VariantID {
					case "promise.alt-four.main-snapshot":
						if got.State != PromiseSettlementFulfilled || got.Value != nil {
							t.Fatalf("historical AltFour nil-handler settlement = %+v, want fulfilled nil", got)
						}
					case "promise.alt-five.original-chained":
						if got.State != PromiseSettlementFulfilled || got.Value != reason {
							t.Fatalf("historical AltFive nil-handler settlement = %+v, want fulfilled reason", got)
						}
					default:
						t.Fatalf("unowned invalid rejection assessment for %s", implementation.VariantID)
					}
				})
			}
		})
	}
}

func promiseSuccessorTestCheckpoint(t *testing.T, loop *eventloop.Loop) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	promiseSuccessorCheckpoint(t, loop, deadline.C)
}

func TestPromiseSuccessorChainValues(t *testing.T) {
	for _, implementation := range PromiseImplementations() {
		t.Run(implementation.Name, func(t *testing.T) {
			loop, js := startPromiseAdapterTestLoop(t)
			for _, depth := range []int{1, 10, 100} {
				t.Run(promiseBenchmarkInteger(depth), func(t *testing.T) {
					root, resolve, _ := implementation.Factory(js)
					tail := root
					for range depth {
						tail = tail.Then(promiseSuccessorIncrement, promiseSuccessorRejectMarker)
					}
					resolve(promiseBenchmarkValue(7))
					deadline := time.NewTimer(5 * time.Second)
					defer deadline.Stop()
					promiseSuccessorCheckpoint(t, loop, deadline.C)
					promiseSuccessorRequireSettlement(t, tail, promiseBenchmarkValue(7+depth))
				})
			}
		})
	}
}

func TestPromiseSuccessorFanOutValues(t *testing.T) {
	for _, implementation := range PromiseImplementations() {
		t.Run(implementation.Name, func(t *testing.T) {
			loop, js := startPromiseAdapterTestLoop(t)
			root, resolve, _ := implementation.Factory(js)
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
			resolve(promiseBenchmarkValue(1000))
			deadline := time.NewTimer(5 * time.Second)
			defer deadline.Stop()
			promiseSuccessorCheckpoint(t, loop, deadline.C)
			for index, child := range children {
				promiseSuccessorRequireSettlement(t, child, promiseBenchmarkValue(1000+index))
			}
		})
	}
}

func TestPromiseSuccessorRaceUniqueWinner(t *testing.T) {
	for _, implementation := range PromiseImplementations() {
		if !implementation.RaceAssessment.Verified() {
			continue
		}
		t.Run(implementation.Name, func(t *testing.T) {
			loop, js := startPromiseAdapterTestLoop(t)
			combinator := implementation.RaceCase(js, promiseSuccessorFanOut)
			promiseSuccessorRequireResolvers(t, combinator, promiseSuccessorFanOut)
			if err := promiseSuccessorSettleRace(combinator.Resolvers, promiseSuccessorWinner, 1000); err != nil {
				t.Fatal(err)
			}
			deadline := time.NewTimer(5 * time.Second)
			defer deadline.Stop()
			promiseSuccessorCheckpoint(t, loop, deadline.C)
			promiseSuccessorRequireSettlement(t, combinator.Promise, 1000+promiseSuccessorWinner)
		})
	}
}

func TestPromiseSuccessorRaceSettlesEveryInputOnce(t *testing.T) {
	const count = 100
	type call struct {
		index int
		value any
	}
	calls := make([]call, 0, count)
	resolvers := make([]eventloop.ResolveFunc, count)
	for index := range resolvers {
		resolvers[index] = func(value any) {
			calls = append(calls, call{index: index, value: value})
		}
	}
	if err := promiseSuccessorSettleRace(resolvers, promiseSuccessorWinner, 500); err != nil {
		t.Fatal(err)
	}
	if len(calls) != count {
		t.Fatalf("resolver calls = %d, want %d", len(calls), count)
	}
	if calls[0] != (call{index: promiseSuccessorWinner, value: promiseBenchmarkValue(500 + promiseSuccessorWinner)}) {
		t.Fatalf("first resolver call = %+v, want winner %d", calls[0], promiseSuccessorWinner)
	}
	seen := make([]bool, count)
	for _, got := range calls {
		if got.index < 0 || got.index >= count || seen[got.index] {
			t.Fatalf("invalid or repeated resolver call %+v", got)
		}
		seen[got.index] = true
		want := promiseBenchmarkValue(500 + got.index)
		if got.value != want {
			t.Fatalf("resolver %d value = %#v, want %#v", got.index, got.value, want)
		}
	}
}

func TestPromiseSuccessorAllOrdering(t *testing.T) {
	for _, implementation := range PromiseImplementations() {
		if !implementation.AllAssessment.Verified() {
			continue
		}
		t.Run(implementation.Name, func(t *testing.T) {
			loop, js := startPromiseAdapterTestLoop(t)
			combinator := implementation.AllCase(js, promiseSuccessorFanOut)
			promiseSuccessorRequireResolvers(t, combinator, promiseSuccessorFanOut)
			for index := len(combinator.Resolvers) - 1; index >= 0; index-- {
				combinator.Resolvers[index](promiseBenchmarkValue(1000 + index))
			}
			deadline := time.NewTimer(5 * time.Second)
			defer deadline.Stop()
			promiseSuccessorCheckpoint(t, loop, deadline.C)
			promiseSuccessorRequireOrderedAll(t, combinator.Promise, 1000, promiseSuccessorFanOut)
		})
	}
}

func TestPromiseAltOneAllHistoricalContractInvalid(t *testing.T) {
	implementation := PromiseImplementations()[1]
	if implementation.VariantID != "promise.alt-one.embedded-first-handler" || implementation.AllCase == nil || implementation.AllAssessment.Status != PromiseAssessmentInvalid {
		t.Fatalf("AltOne All metadata = %+v, want implemented correctness-invalid", implementation)
	}
	loop, js := startPromiseAdapterTestLoop(t)
	combinator := implementation.AllCase(js, 2)
	promiseSuccessorRequireResolvers(t, combinator, 2)
	combinator.Resolvers[1](promiseBenchmarkValue(2))
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	promiseSuccessorCheckpoint(t, loop, deadline.C)
	got := combinator.Promise.Settlement()
	if got.State != PromiseSettlementFulfilled || got.Value != nil {
		t.Fatalf("historical AltOne All first-input settlement = %+v, want fulfilled nil defect", got)
	}
}

func TestPromiseSuccessorFreshConstructionStationary(t *testing.T) {
	for _, implementation := range PromiseImplementations() {
		t.Run(implementation.Name, func(t *testing.T) {
			loop, js := startPromiseAdapterTestLoop(t)
			roots := make([]Promise, 3)
			tails := make([]Promise, 3)
			resolvers := make([]eventloop.ResolveFunc, 3)
			for index := range roots {
				roots[index], resolvers[index], _ = implementation.Factory(js)
				tails[index] = roots[index]
				for range 100 {
					tails[index] = tails[index].Then(promiseSuccessorIdentity, promiseSuccessorRejectMarker)
				}
			}
			if reflect.ValueOf(roots[0]).Pointer() == reflect.ValueOf(roots[1]).Pointer() {
				t.Fatal("successive construction cases reused one root adapter")
			}
			resolvers[0](promiseBenchmarkValue(1))
			deadline := time.NewTimer(5 * time.Second)
			defer deadline.Stop()
			promiseSuccessorCheckpoint(t, loop, deadline.C)
			promiseSuccessorRequireSettlement(t, tails[0], promiseBenchmarkValue(1))
			for index := 1; index < len(tails); index++ {
				if got := tails[index].Settlement(); got.State != PromiseSettlementPending {
					t.Fatalf("unsettled case %d state = %d, want pending", index, got.State)
				}
				resolvers[index](promiseBenchmarkValue(index + 1))
			}
			promiseSuccessorCheckpoint(t, loop, deadline.C)
			for index := 1; index < len(tails); index++ {
				promiseSuccessorRequireSettlement(t, tails[index], promiseBenchmarkValue(index+1))
			}
		})
	}
}

func TestPromiseSuccessorSettleRaceRejectsInvalidCases(t *testing.T) {
	if err := promiseSuccessorSettleRace(nil, 0, 1); err == nil {
		t.Fatal("empty resolver set succeeded")
	}
	if err := promiseSuccessorSettleRace([]eventloop.ResolveFunc{nil}, 0, 1); err == nil {
		t.Fatal("nil resolver succeeded")
	}
}
