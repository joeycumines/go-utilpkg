package tournament

import "fmt"

const timerReferenceWorkloadCount = 8

type timerReferenceWorkloadID string

const (
	timerReferenceMissing        timerReferenceWorkloadID = "timer.reference.missing.v1"
	timerReferenceAlreadyRefed   timerReferenceWorkloadID = "timer.reference.already-refed.v1"
	timerReferenceAlreadyUnrefed timerReferenceWorkloadID = "timer.reference.already-unrefed.v1"
	timerReferenceUnref          timerReferenceWorkloadID = "timer.reference.ref-unref-cycle.v2"
	timerReferenceRef            timerReferenceWorkloadID = "timer.reference.unref-ref-cycle.v2"
	timerReferenceFired          timerReferenceWorkloadID = "timer.reference.fired.v1"
	timerReferenceCanceled       timerReferenceWorkloadID = "timer.reference.canceled.v1"
	timerReferenceAggregate      timerReferenceWorkloadID = "timer.reference.aggregate-correctness.v1"
)

var allTimerReferenceWorkloads = [...]timerReferenceWorkloadID{
	timerReferenceMissing,
	timerReferenceAlreadyRefed,
	timerReferenceAlreadyUnrefed,
	timerReferenceUnref,
	timerReferenceRef,
	timerReferenceFired,
	timerReferenceCanceled,
	timerReferenceAggregate,
}

type timerReferenceSeedState string

const (
	timerReferenceSeedMissing  timerReferenceSeedState = "missing"
	timerReferenceSeedRefed    timerReferenceSeedState = "refed"
	timerReferenceSeedUnrefed  timerReferenceSeedState = "unrefed"
	timerReferenceSeedFired    timerReferenceSeedState = "fired"
	timerReferenceSeedCanceled timerReferenceSeedState = "canceled"
	timerReferenceSeedMixed    timerReferenceSeedState = "mixed"
)

type timerReferenceOperation struct {
	ID    uint64
	Refed bool
}

type timerReferenceExpected struct {
	ID      uint64
	Present bool
	Refed   bool
}

type timerReferenceParameters struct {
	SeedState          timerReferenceSeedState
	Operations         [4]timerReferenceOperation
	OperationCount     uint8
	Expected           [3]timerReferenceExpected
	ExpectedCount      uint8
	ExpectedRefedCount int64
	Stationary         bool
}

type timerReferenceSemanticHarness struct {
	ID       string
	Setup    string
	Timed    string
	Teardown string
}

type timerReferenceDefinition struct {
	ID               timerReferenceWorkloadID
	Parameters       timerReferenceParameters
	ParameterSHA256  string
	SemanticHarness  timerReferenceSemanticHarness
	DefinitionSHA256 string
}

func timerReferenceDefinitions() []timerReferenceDefinition {
	definitions := []timerReferenceDefinition{
		newTimerReferenceDefinition(timerReferenceMissing,
			timerReferenceParameters{SeedState: timerReferenceSeedMissing, Operations: [4]timerReferenceOperation{{ID: 1, Refed: true}}, OperationCount: 1, Expected: [3]timerReferenceExpected{{ID: 1}}, ExpectedCount: 1, Stationary: true},
			timerReferenceSemanticHarness{ID: "timer-reference-missing-v1", Setup: "construct an empty concrete reference core", Timed: "apply reference=true to timer ID 1", Teardown: "prove timer ID 1 is absent and aggregate is zero"}),
		newTimerReferenceDefinition(timerReferenceAlreadyRefed,
			timerReferenceParameters{SeedState: timerReferenceSeedRefed, Operations: [4]timerReferenceOperation{{ID: 1, Refed: true}}, OperationCount: 1, Expected: [3]timerReferenceExpected{{ID: 1, Present: true, Refed: true}}, ExpectedCount: 1, ExpectedRefedCount: 1, Stationary: true},
			timerReferenceSemanticHarness{ID: "timer-reference-already-refed-v1", Setup: "seed timer ID 1 referenced outside timing", Timed: "apply reference=true to timer ID 1", Teardown: "prove timer ID 1 remains referenced and aggregate is one"}),
		newTimerReferenceDefinition(timerReferenceAlreadyUnrefed,
			timerReferenceParameters{SeedState: timerReferenceSeedUnrefed, Operations: [4]timerReferenceOperation{{ID: 1}}, OperationCount: 1, Expected: [3]timerReferenceExpected{{ID: 1, Present: true}}, ExpectedCount: 1, Stationary: true},
			timerReferenceSemanticHarness{ID: "timer-reference-already-unrefed-v1", Setup: "seed timer ID 1 unreferenced outside timing", Timed: "apply reference=false to timer ID 1", Teardown: "prove timer ID 1 remains unreferenced and aggregate is zero"}),
		newTimerReferenceDefinition(timerReferenceUnref,
			timerReferenceParameters{SeedState: timerReferenceSeedRefed, Operations: [4]timerReferenceOperation{{ID: 1}, {ID: 1, Refed: true}}, OperationCount: 2, Expected: [3]timerReferenceExpected{{ID: 1, Present: true, Refed: true}}, ExpectedCount: 1, ExpectedRefedCount: 1, Stationary: true},
			timerReferenceSemanticHarness{ID: "timer-reference-ref-unref-cycle-v2", Setup: "seed timer ID 1 referenced outside timing", Timed: "apply reference=false then reference=true to timer ID 1 as one stationary pair", Teardown: "prove timer ID 1 is referenced and aggregate returned to one"}),
		newTimerReferenceDefinition(timerReferenceRef,
			timerReferenceParameters{SeedState: timerReferenceSeedUnrefed, Operations: [4]timerReferenceOperation{{ID: 1, Refed: true}, {ID: 1}}, OperationCount: 2, Expected: [3]timerReferenceExpected{{ID: 1, Present: true}}, ExpectedCount: 1, Stationary: true},
			timerReferenceSemanticHarness{ID: "timer-reference-unref-ref-cycle-v2", Setup: "seed timer ID 1 unreferenced outside timing", Timed: "apply reference=true then reference=false to timer ID 1 as one stationary pair", Teardown: "prove timer ID 1 is unreferenced and aggregate returned to zero"}),
		newTimerReferenceDefinition(timerReferenceFired,
			timerReferenceParameters{SeedState: timerReferenceSeedFired, Operations: [4]timerReferenceOperation{{ID: 1, Refed: true}}, OperationCount: 1, Expected: [3]timerReferenceExpected{{ID: 1}}, ExpectedCount: 1, Stationary: true},
			timerReferenceSemanticHarness{ID: "timer-reference-fired-v1", Setup: "seed referenced timer ID 1 then remove it as fired outside timing", Timed: "apply reference=true to timer ID 1", Teardown: "prove timer ID 1 is absent and aggregate is zero"}),
		newTimerReferenceDefinition(timerReferenceCanceled,
			timerReferenceParameters{SeedState: timerReferenceSeedCanceled, Operations: [4]timerReferenceOperation{{ID: 1}}, OperationCount: 1, Expected: [3]timerReferenceExpected{{ID: 1}}, ExpectedCount: 1, Stationary: true},
			timerReferenceSemanticHarness{ID: "timer-reference-canceled-v1", Setup: "seed referenced timer ID 1 then remove it as canceled outside timing", Timed: "apply reference=false to timer ID 1", Teardown: "prove timer ID 1 is absent and aggregate is zero"}),
		newTimerReferenceDefinition(timerReferenceAggregate,
			timerReferenceParameters{SeedState: timerReferenceSeedMixed, Operations: [4]timerReferenceOperation{{ID: 1}, {ID: 2, Refed: true}, {ID: 3, Refed: true}, {ID: 2}}, OperationCount: 4, Expected: [3]timerReferenceExpected{{ID: 1, Present: true}, {ID: 2, Present: true}, {ID: 3, Present: true, Refed: true}}, ExpectedCount: 3, ExpectedRefedCount: 1},
			timerReferenceSemanticHarness{ID: "timer-reference-aggregate-correctness-v1", Setup: "seed timer IDs 1 and 3 referenced and timer ID 2 unreferenced outside timing", Timed: "apply the fixed four-transition sequence to timer IDs 1, 2, 3, and 2", Teardown: "prove every reference bit and aggregate equal live referenced membership"}),
	}
	return definitions
}

func newTimerReferenceDefinition(id timerReferenceWorkloadID, parameters timerReferenceParameters, harness timerReferenceSemanticHarness) timerReferenceDefinition {
	parameterSHA := timerParametersSHA256("reference", id, parameters)
	definitionSHA := fmt.Sprintf("%x", framedTimerSeal(
		"go-utilpkg/eventloop/tournament/timer-reference-definition/v1",
		string(id), parameterSHA, harness.ID, harness.Setup, harness.Timed, harness.Teardown,
	))
	return timerReferenceDefinition{
		ID: id, Parameters: parameters, ParameterSHA256: parameterSHA,
		SemanticHarness: harness, DefinitionSHA256: definitionSHA,
	}
}

func canonicalTimerReferenceDefinition(id timerReferenceWorkloadID) (timerReferenceDefinition, bool) {
	for _, definition := range timerReferenceDefinitions() {
		if definition.ID == id {
			return definition, true
		}
	}
	return timerReferenceDefinition{}, false
}

func validTimerReferenceDefinition(definition timerReferenceDefinition) bool {
	canonical, ok := canonicalTimerReferenceDefinition(definition.ID)
	return ok && definition == canonical
}
