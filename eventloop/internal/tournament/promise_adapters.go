package tournament

import (
	"github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/go-eventloop/internal/promisealtfive"
	"github.com/joeycumines/go-eventloop/internal/promisealtfour"
	"github.com/joeycumines/go-eventloop/internal/promisealtone"
	"github.com/joeycumines/go-eventloop/internal/promisealtthree"
	"github.com/joeycumines/go-eventloop/internal/promisealttwo"
)

// ChainedPromiseAdapter adapts eventloop.ChainedPromise
type ChainedPromiseAdapter struct {
	p *eventloop.ChainedPromise
}

func (a *ChainedPromiseAdapter) Then(onFulfilled, onRejected func(any) any) Promise {
	return &ChainedPromiseAdapter{p: a.p.Then(onFulfilled, onRejected)}
}

func (a *ChainedPromiseAdapter) Result() any {
	switch a.p.State() {
	case eventloop.Fulfilled:
		return a.p.Value()
	case eventloop.Rejected:
		return a.p.Reason()
	default:
		return nil
	}
}

func (a *ChainedPromiseAdapter) Settlement() PromiseSettlement {
	return promiseSettlement(a.p.State(), a.p.Value(), a.p.Reason())
}

// PromiseAltOneAdapter adapts promisealtone.Promise
type PromiseAltOneAdapter struct {
	p *promisealtone.Promise
}

func (a *PromiseAltOneAdapter) Then(onFulfilled, onRejected func(any) any) Promise {
	return &PromiseAltOneAdapter{p: a.p.Then(onFulfilled, onRejected)}
}

func (a *PromiseAltOneAdapter) Result() any {
	return a.p.Result()
}

func (a *PromiseAltOneAdapter) Settlement() PromiseSettlement {
	return promiseSettlement(a.p.State(), a.p.Value(), a.p.Reason())
}

// PromiseAltTwoAdapter adapts promisealttwo.Promise
type PromiseAltTwoAdapter struct {
	p *promisealttwo.Promise
}

func (a *PromiseAltTwoAdapter) Then(onFulfilled, onRejected func(any) any) Promise {
	return &PromiseAltTwoAdapter{p: a.p.Then(onFulfilled, onRejected)}
}

func (a *PromiseAltTwoAdapter) Result() any {
	return a.p.Result()
}

func (a *PromiseAltTwoAdapter) Settlement() PromiseSettlement {
	return promiseSettlementResult(a.p.State(), a.p.Result())
}

// PromiseAltThreeAdapter adapts promisealtthree.Promise
type PromiseAltThreeAdapter struct {
	p *promisealtthree.Promise
}

func (a *PromiseAltThreeAdapter) Then(onFulfilled, onRejected func(any) any) Promise {
	return &PromiseAltThreeAdapter{p: a.p.Then(onFulfilled, onRejected)}
}

func (a *PromiseAltThreeAdapter) Result() any {
	return a.p.Result()
}

func (a *PromiseAltThreeAdapter) Settlement() PromiseSettlement {
	return promiseSettlementResult(a.p.State(), a.p.Result())
}

// PromiseAltFourAdapter adapts promisealtfour.Promise
type PromiseAltFourAdapter struct {
	p *promisealtfour.Promise
}

// PromiseAltFiveAdapter adapts promisealtfive.Promise.
type PromiseAltFiveAdapter struct {
	p *promisealtfive.Promise
}

func (a *PromiseAltFiveAdapter) Then(onFulfilled, onRejected func(any) any) Promise {
	return &PromiseAltFiveAdapter{p: a.p.Then(onFulfilled, onRejected)}
}

func (a *PromiseAltFiveAdapter) Result() any {
	switch a.p.State() {
	case promisealtfive.Fulfilled:
		return a.p.Value()
	case promisealtfive.Rejected:
		return a.p.Reason()
	default:
		return nil
	}
}

func (a *PromiseAltFiveAdapter) Settlement() PromiseSettlement {
	return promiseSettlement(a.p.State(), a.p.Value(), a.p.Reason())
}

func (a *PromiseAltFourAdapter) Then(onFulfilled, onRejected func(any) any) Promise {
	return &PromiseAltFourAdapter{p: a.p.Then(onFulfilled, onRejected)}
}

func (a *PromiseAltFourAdapter) Result() any {
	return a.p.Result()
}

func (a *PromiseAltFourAdapter) Settlement() PromiseSettlement {
	return promiseSettlement(a.p.State(), a.p.Value(), a.p.Reason())
}

// PromiseImplementations returns the list of promise implementations.
func PromiseImplementations() []PromiseImplementation {
	return []PromiseImplementation{
		{
			Name:          "ChainedPromise",
			VariantID:     "promise.main.chained",
			SourcePackage: eventloopPackage,
			OriginCommit:  "current",
			OriginTree:    "current",
			Factory: func(js *eventloop.JS) (Promise, eventloop.ResolveFunc, eventloop.RejectFunc) {
				p, resolve, reject := js.NewChainedPromise()
				return &ChainedPromiseAdapter{p: p}, resolve, reject
			},
			Race: func(js *eventloop.JS, count int) (Promise, PromiseRaceSettlement) {
				promises := make([]*eventloop.ChainedPromise, count)
				resolvers := make([]eventloop.ResolveFunc, count)
				for i := range promises {
					promises[i], resolvers[i], _ = js.NewChainedPromise()
				}
				return &ChainedPromiseAdapter{p: js.Race(promises)}, settlePromiseRaceInputs(js, resolvers)
			},
			AllCase:                         chainedPromiseAllCase,
			RaceCase:                        chainedPromiseRaceCase,
			SequentialFulfillment:           promiseAssessmentVerified("sequential fulfillment is source- and test-verified"),
			PendingRejectionPropagation:     promiseAssessmentVerified("pending rejection propagates the identical reason"),
			SettledRejectionPropagation:     promiseAssessmentVerified("settled rejection propagates the identical reason"),
			ConcurrentSettlementObservation: promiseAssessmentVerified("payload publication precedes terminal-state publication"),
			ConcurrentHandlerRegistration:   promiseAssessmentVerified("handler registration rechecks settlement under ownership"),
			AllAssessment:                   promiseAssessmentVerified("All preserves input order and awaits every input"),
			RaceAssessment:                  promiseAssessmentVerified("Race preserves the first settled input"),
		},
		{
			Name:          "PromiseAltOne",
			VariantID:     "promise.alt-one.embedded-first-handler",
			SourcePackage: eventloopPackage + "/internal/promisealtone",
			OriginCommit:  "986e2378c1484aa917a1bb0fd13aef914bdce50f",
			OriginTree:    "d21aa3b30d6bb98d0c217b12a69916e3bbcbd45b",
			Factory: func(js *eventloop.JS) (Promise, eventloop.ResolveFunc, eventloop.RejectFunc) {
				p, r1, r2 := promisealtone.New(js)
				return &PromiseAltOneAdapter{p: p}, eventloop.ResolveFunc(r1), eventloop.RejectFunc(r2)
			},
			Race: func(js *eventloop.JS, count int) (Promise, PromiseRaceSettlement) {
				promises := make([]*promisealtone.Promise, count)
				resolvers := make([]eventloop.ResolveFunc, count)
				for i := range promises {
					promise, resolve, _ := promisealtone.New(js)
					promises[i] = promise
					resolvers[i] = eventloop.ResolveFunc(resolve)
				}
				return &PromiseAltOneAdapter{p: promisealtone.Race(js, promises)}, settlePromiseRaceInputs(js, resolvers)
			},
			AllCase:                         promiseAltOneAllCase,
			RaceCase:                        promiseAltOneRaceCase,
			SequentialFulfillment:           promiseAssessmentVerified("sequential fulfillment is source- and test-verified"),
			PendingRejectionPropagation:     promiseAssessmentVerified("pending rejection propagates the identical reason"),
			SettledRejectionPropagation:     promiseAssessmentVerified("settled rejection propagates the identical reason"),
			ConcurrentSettlementObservation: promiseAssessmentVerified("payload publication is protected by the implementation mutex"),
			ConcurrentHandlerRegistration:   promiseAssessmentVerified("handler registration is serialized by the implementation mutex"),
			AllAssessment:                   promiseAssessmentInvalid("the first fulfilled input resolves the aggregate with the handler's nil return"),
			RaceAssessment:                  promiseAssessmentVerified("Race preserves the first settled input"),
		},
		{
			Name:          "PromiseAltTwo",
			VariantID:     "promise.alt-two.treiber",
			SourcePackage: eventloopPackage + "/internal/promisealttwo",
			OriginCommit:  "986e2378c1484aa917a1bb0fd13aef914bdce50f",
			OriginTree:    "e4d94e7fe925fcbdef676f60a291736921b678b3",
			Factory: func(js *eventloop.JS) (Promise, eventloop.ResolveFunc, eventloop.RejectFunc) {
				p, r1, r2 := promisealttwo.New(js)
				return &PromiseAltTwoAdapter{p: p}, eventloop.ResolveFunc(r1), eventloop.RejectFunc(r2)
			},
			SequentialFulfillment:           promiseAssessmentVerified("checkpoint synchronization makes sequential fulfillment observable"),
			PendingRejectionPropagation:     promiseAssessmentVerified("pending rejection propagates the identical reason"),
			SettledRejectionPropagation:     promiseAssessmentVerified("settled rejection propagates the identical reason"),
			ConcurrentSettlementObservation: promiseAssessmentInvalid("terminal state is published before the result payload"),
			ConcurrentHandlerRegistration:   promiseAssessmentVerified("the Treiber registration path rechecks terminal state"),
			AllAssessment:                   promiseAssessmentNotApplicable("implementation has no historical All combinator"),
			RaceAssessment:                  promiseAssessmentNotApplicable("implementation has no historical Race combinator"),
		},
		{
			Name:          "PromiseAltThree",
			VariantID:     "promise.alt-three.pooled-treiber",
			SourcePackage: eventloopPackage + "/internal/promisealtthree",
			OriginCommit:  "986e2378c1484aa917a1bb0fd13aef914bdce50f",
			OriginTree:    "c6870653742d65e6b19a4c5d53713577c8c7be53",
			Factory: func(js *eventloop.JS) (Promise, eventloop.ResolveFunc, eventloop.RejectFunc) {
				p, r1, r2 := promisealtthree.New(js)
				return &PromiseAltThreeAdapter{p: p}, eventloop.ResolveFunc(r1), eventloop.RejectFunc(r2)
			},
			SequentialFulfillment:           promiseAssessmentVerified("checkpoint synchronization makes sequential fulfillment observable"),
			PendingRejectionPropagation:     promiseAssessmentVerified("pending rejection propagates the identical reason"),
			SettledRejectionPropagation:     promiseAssessmentVerified("settled rejection propagates the identical reason"),
			ConcurrentSettlementObservation: promiseAssessmentInvalid("terminal state is published before the result payload"),
			ConcurrentHandlerRegistration:   promiseAssessmentVerified("the pooled Treiber registration path rechecks terminal state"),
			AllAssessment:                   promiseAssessmentNotApplicable("implementation has no historical All combinator"),
			RaceAssessment:                  promiseAssessmentNotApplicable("implementation has no historical Race combinator"),
		},
		{
			Name:          "PromiseAltFour",
			VariantID:     "promise.alt-four.main-snapshot",
			SourcePackage: eventloopPackage + "/internal/promisealtfour",
			OriginCommit:  "986e2378c1484aa917a1bb0fd13aef914bdce50f",
			OriginTree:    "fd0ff142948d28aac4c9f5dc9248ba1314fc5ba6",
			Factory: func(js *eventloop.JS) (Promise, eventloop.ResolveFunc, eventloop.RejectFunc) {
				p, r1, r2 := promisealtfour.New(js)
				return &PromiseAltFourAdapter{p: p}, eventloop.ResolveFunc(r1), eventloop.RejectFunc(r2)
			},
			Race: func(js *eventloop.JS, count int) (Promise, PromiseRaceSettlement) {
				promises := make([]*promisealtfour.Promise, count)
				resolvers := make([]eventloop.ResolveFunc, count)
				for i := range promises {
					promise, resolve, _ := promisealtfour.New(js)
					promises[i] = promise
					resolvers[i] = eventloop.ResolveFunc(resolve)
				}
				return &PromiseAltFourAdapter{p: promisealtfour.Race(js, promises)}, settlePromiseRaceInputs(js, resolvers)
			},
			AllCase:                         promiseAltFourAllCase,
			RaceCase:                        promiseAltFourRaceCase,
			SequentialFulfillment:           promiseAssessmentVerified("checkpoint synchronization makes sequential fulfillment observable"),
			PendingRejectionPropagation:     promiseAssessmentVerified("pending rejection propagates the identical reason"),
			SettledRejectionPropagation:     promiseAssessmentInvalid("a nil rejection handler fulfills the child with nil"),
			ConcurrentSettlementObservation: promiseAssessmentInvalid("terminal state is published before the value or reason"),
			ConcurrentHandlerRegistration:   promiseAssessmentInvalid("the pending registration path does not recheck state under the handler lock"),
			AllAssessment:                   promiseAssessmentVerified("All preserves input order and awaits every input"),
			RaceAssessment:                  promiseAssessmentVerified("Race preserves the first settled input"),
		},
		{
			Name:          "PromiseAltFive",
			VariantID:     "promise.alt-five.original-chained",
			SourcePackage: eventloopPackage + "/internal/promisealtfive",
			OriginCommit:  "986e2378c1484aa917a1bb0fd13aef914bdce50f",
			OriginTree:    "16618a665a99a47a678630cb94381a9db47745f4",
			Factory: func(js *eventloop.JS) (Promise, eventloop.ResolveFunc, eventloop.RejectFunc) {
				p, r1, r2 := promisealtfive.New(js)
				return &PromiseAltFiveAdapter{p: p}, eventloop.ResolveFunc(r1), eventloop.RejectFunc(r2)
			},
			SequentialFulfillment:           promiseAssessmentVerified("sequential fulfillment is source- and test-verified"),
			PendingRejectionPropagation:     promiseAssessmentVerified("pending rejection propagates the identical reason"),
			SettledRejectionPropagation:     promiseAssessmentInvalid("a nil rejection handler fulfills the child with the rejection reason"),
			ConcurrentSettlementObservation: promiseAssessmentVerified("payload publication precedes terminal-state publication"),
			ConcurrentHandlerRegistration:   promiseAssessmentVerified("handler registration rechecks state under the implementation mutex"),
			AllAssessment:                   promiseAssessmentNotApplicable("implementation has no historical All combinator"),
			RaceAssessment:                  promiseAssessmentNotApplicable("implementation has no historical Race combinator"),
		},
	}
}

func promiseAssessmentVerified(reason string) PromiseAssessment {
	return PromiseAssessment{Status: PromiseAssessmentVerified, Reason: reason}
}

func promiseAssessmentInvalid(reason string) PromiseAssessment {
	return PromiseAssessment{Status: PromiseAssessmentInvalid, Reason: reason}
}

func promiseAssessmentNotApplicable(reason string) PromiseAssessment {
	return PromiseAssessment{Status: PromiseAssessmentNotApplicable, Reason: reason}
}

func promiseSettlement(state eventloop.PromiseState, value, reason any) PromiseSettlement {
	switch state {
	case eventloop.Pending:
		return PromiseSettlement{State: PromiseSettlementPending}
	case eventloop.Fulfilled:
		return PromiseSettlement{State: PromiseSettlementFulfilled, Value: value}
	case eventloop.Rejected:
		return PromiseSettlement{State: PromiseSettlementRejected, Value: reason}
	default:
		panic("tournament: invalid promise state")
	}
}

func promiseSettlementResult(state eventloop.PromiseState, result any) PromiseSettlement {
	switch state {
	case eventloop.Pending:
		return PromiseSettlement{State: PromiseSettlementPending}
	case eventloop.Fulfilled:
		return PromiseSettlement{State: PromiseSettlementFulfilled, Value: result}
	case eventloop.Rejected:
		return PromiseSettlement{State: PromiseSettlementRejected, Value: result}
	default:
		panic("tournament: invalid promise state")
	}
}

func chainedPromiseAllCase(js *eventloop.JS, count int) PromiseCombinatorCase {
	promises := make([]*eventloop.ChainedPromise, count)
	resolvers := make([]eventloop.ResolveFunc, count)
	for index := range promises {
		promises[index], resolvers[index], _ = js.NewChainedPromise()
	}
	return PromiseCombinatorCase{
		Promise:   &ChainedPromiseAdapter{p: js.All(promises)},
		Resolvers: resolvers,
		Retention: promises,
	}
}

func chainedPromiseRaceCase(js *eventloop.JS, count int) PromiseCombinatorCase {
	promises := make([]*eventloop.ChainedPromise, count)
	resolvers := make([]eventloop.ResolveFunc, count)
	for index := range promises {
		promises[index], resolvers[index], _ = js.NewChainedPromise()
	}
	return PromiseCombinatorCase{
		Promise:   &ChainedPromiseAdapter{p: js.Race(promises)},
		Resolvers: resolvers,
		Retention: promises,
	}
}

func promiseAltOneAllCase(js *eventloop.JS, count int) PromiseCombinatorCase {
	promises := make([]*promisealtone.Promise, count)
	resolvers := make([]eventloop.ResolveFunc, count)
	for index := range promises {
		promise, resolve, _ := promisealtone.New(js)
		promises[index] = promise
		resolvers[index] = eventloop.ResolveFunc(resolve)
	}
	return PromiseCombinatorCase{
		Promise:   &PromiseAltOneAdapter{p: promisealtone.All(js, promises)},
		Resolvers: resolvers,
		Retention: promises,
	}
}

func promiseAltOneRaceCase(js *eventloop.JS, count int) PromiseCombinatorCase {
	promises := make([]*promisealtone.Promise, count)
	resolvers := make([]eventloop.ResolveFunc, count)
	for index := range promises {
		promise, resolve, _ := promisealtone.New(js)
		promises[index] = promise
		resolvers[index] = eventloop.ResolveFunc(resolve)
	}
	return PromiseCombinatorCase{
		Promise:   &PromiseAltOneAdapter{p: promisealtone.Race(js, promises)},
		Resolvers: resolvers,
		Retention: promises,
	}
}

func promiseAltFourAllCase(js *eventloop.JS, count int) PromiseCombinatorCase {
	promises := make([]*promisealtfour.Promise, count)
	resolvers := make([]eventloop.ResolveFunc, count)
	for index := range promises {
		promise, resolve, _ := promisealtfour.New(js)
		promises[index] = promise
		resolvers[index] = eventloop.ResolveFunc(resolve)
	}
	return PromiseCombinatorCase{
		Promise:   &PromiseAltFourAdapter{p: promisealtfour.All(js, promises)},
		Resolvers: resolvers,
		Retention: promises,
	}
}

func promiseAltFourRaceCase(js *eventloop.JS, count int) PromiseCombinatorCase {
	promises := make([]*promisealtfour.Promise, count)
	resolvers := make([]eventloop.ResolveFunc, count)
	for index := range promises {
		promise, resolve, _ := promisealtfour.New(js)
		promises[index] = promise
		resolvers[index] = eventloop.ResolveFunc(resolve)
	}
	return PromiseCombinatorCase{
		Promise:   &PromiseAltFourAdapter{p: promisealtfour.Race(js, promises)},
		Resolvers: resolvers,
		Retention: promises,
	}
}

func settlePromiseRaceInputs(js *eventloop.JS, resolvers []eventloop.ResolveFunc) PromiseRaceSettlement {
	return func() (<-chan struct{}, error) {
		for _, resolve := range resolvers {
			resolve(1)
		}
		drained := make(chan struct{})
		if err := js.QueueMicrotask(func() { close(drained) }); err != nil {
			return nil, err
		}
		return drained, nil
	}
}
