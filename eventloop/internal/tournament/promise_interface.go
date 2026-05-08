package tournament

import (
	"github.com/joeycumines/go-eventloop"
)

// Promise defines the common interface for tournament promises.
// It reflects the subset of methods we want to benchmark and verify.
type Promise interface {
	Then(onFulfilled, onRejected func(any) any) Promise
	Result() any
	Settlement() PromiseSettlement
}

// PromiseSettlementState distinguishes pending promises from promises settled
// with nil and distinguishes fulfillment values from rejection reasons.
type PromiseSettlementState uint8

const (
	PromiseSettlementPending PromiseSettlementState = iota
	PromiseSettlementFulfilled
	PromiseSettlementRejected
)

// PromiseSettlement is the exact observable state of a tournament promise.
type PromiseSettlement struct {
	State PromiseSettlementState
	Value any
}

// PromiseFactory creates a new promise and its resolver/rejector.
// It takes a *eventloop.JS because all our promises depend on it.
type PromiseFactory func(*eventloop.JS) (Promise, eventloop.ResolveFunc, eventloop.RejectFunc)

// PromiseRaceSettlement settles every race input and reports when all resulting
// input reactions have drained from the loop's microtask queue.
type PromiseRaceSettlement func() (<-chan struct{}, error)

// PromiseRace creates a race combinator over count pending promises and returns
// a settlement operation that prevents benchmark iterations retaining losers.
type PromiseRace func(*eventloop.JS, int) (Promise, PromiseRaceSettlement)

// PromiseCombinatorCase retains every native input and exposes its resolvers so
// successor workloads can prove exact settlement order and values.
type PromiseCombinatorCase struct {
	Promise   Promise
	Resolvers []eventloop.ResolveFunc
	Retention any
}

// PromiseCombinatorFactory creates a pending-input All or Race case.
type PromiseCombinatorFactory func(*eventloop.JS, int) PromiseCombinatorCase

// PromiseAssessmentStatus classifies one implementation behavior without
// projecting that assessment onto unrelated workloads.
type PromiseAssessmentStatus uint8

const (
	PromiseAssessmentUnassessed PromiseAssessmentStatus = iota
	PromiseAssessmentVerified
	PromiseAssessmentInvalid
	PromiseAssessmentNotApplicable
)

// PromiseAssessment records whether one behavior is suitable for comparative
// execution and why. Invalid behaviors remain executable diagnostics.
type PromiseAssessment struct {
	Status PromiseAssessmentStatus
	Reason string
}

// Verified reports whether the assessed behavior can produce comparative
// performance evidence.
func (a PromiseAssessment) Verified() bool {
	return a.Status == PromiseAssessmentVerified
}

// PromiseImplementation represents a named promise implementation.
type PromiseImplementation struct { // betteralign:ignore
	Name                            string
	VariantID                       string
	SourcePackage                   string
	OriginCommit                    string
	OriginTree                      string
	Factory                         PromiseFactory
	Race                            PromiseRace
	AllCase                         PromiseCombinatorFactory
	RaceCase                        PromiseCombinatorFactory
	SequentialFulfillment           PromiseAssessment
	PendingRejectionPropagation     PromiseAssessment
	SettledRejectionPropagation     PromiseAssessment
	ConcurrentSettlementObservation PromiseAssessment
	ConcurrentHandlerRegistration   PromiseAssessment
	AllAssessment                   PromiseAssessment
	RaceAssessment                  PromiseAssessment
}
