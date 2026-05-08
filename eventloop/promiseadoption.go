package eventloop

import (
	"errors"
	"runtime"
	"weak"
)

var (
	// ErrPromiseSelfResolution is the stable rejection reason used when a
	// promise is resolved with itself.
	ErrPromiseSelfResolution = errors.New("eventloop: promise cannot resolve itself")
	// ErrPromiseNilAdoption is the stable rejection reason used when a promise
	// is resolved with a typed-nil *ChainedPromise.
	ErrPromiseNilAdoption = errors.New("eventloop: promise cannot adopt nil promise")
)

type adoptionSettlement struct {
	result any
	source *ChainedPromise
	target *ChainedPromise
	state  PromiseState
}

type adoptionSettlements []adoptionSettlement

type adoptionCleanup struct {
	js     weak.Pointer[JS]
	target weak.Pointer[ChainedPromise]
}

func (p *ChainedPromise) resolve(value any) {
	if !p.state.CompareAndSwap(int32(Pending), promiseSettlementClaimed) {
		return
	}
	p.resolveClaimed(value)
}

func (p *ChainedPromise) resolveClaimed(value any) {
	// Spec 2.3.1: If promise and x refer to the same object, reject promise with a TypeError.
	if pr, ok := value.(*ChainedPromise); ok && pr == p {
		p.rejectClaimed(ErrPromiseSelfResolution)
		return
	}

	// Spec 2.3.2: If x is a promise, adopt its state.
	// Use addHandler for zero-closure adoption (PromiseAltOne optimization).
	if pr, ok := value.(*ChainedPromise); ok {
		if pr == nil {
			p.rejectClaimed(ErrPromiseNilAdoption)
			return
		}
		// The adoption reaction observes the source rejection, so the source promise
		// is handled even though the adopting target must still carry any rejected
		// state and be reported if nobody handles the adopter.
		if pr.js != nil {
			pr.js.registerAdoption(pr, p)
		}
		pr.handleHandlerScheduleFailure(pr.addHandler(handler{target: p}, true))
		return
	}

	p.mu.Lock()
	if p.state.Load() != promiseSettlementClaimed {
		p.mu.Unlock()
		return
	}

	h0 := p.h0
	useH0 := p.h0.target != nil
	var handlers []handler
	var scheduleFailures []handlerScheduleFailure

	// Extract handlers before they get overwritten with the actual result
	if useH0 && p.result != nil {
		handlers = p.result.([]handler)
	}
	p.h0 = handler{} // Clears h0
	p.result = value
	p.state.Store(promiseFulfilledPublishing)

	if p.js != nil {
		// Publish result and state before any queued reaction can execute. addHandler
		// takes p.mu, so concurrent late handlers cannot enqueue ahead of this
		// pre-settlement snapshot while the lock is held.
		if useH0 {
			if failure := p.scheduleHandler(h0, int32(Fulfilled), value); failure.err != nil {
				scheduleFailures = append(scheduleFailures, failure)
			}
		}
		for _, h := range handlers {
			if failure := p.scheduleHandler(h, int32(Fulfilled), value); failure.err != nil {
				scheduleFailures = append(scheduleFailures, failure)
			}
		}
	} else {
		// Standalone handlers execute synchronously below after p.mu is released.
		// This permits reentrant Then calls without sacrificing state publication.
	}

	if p.js != nil {
		p.js.notifyToChannels(p, value)
	}
	p.state.Store(int32(Fulfilled))
	p.mu.Unlock()

	if p.js == nil {
		if useH0 {
			p.handleHandlerScheduleFailure(p.scheduleHandler(h0, int32(Fulfilled), value))
		}
		for _, h := range handlers {
			p.handleHandlerScheduleFailure(p.scheduleHandler(h, int32(Fulfilled), value))
		}
	}

	for _, failure := range scheduleFailures {
		p.handleHandlerScheduleFailure(failure)
	}
}

func (js *JS) registerAdoption(source, target *ChainedPromise) {
	// The adapter must not root an otherwise unreachable pending source/target
	// cycle. The target cleanup removes the weak metadata without capturing the
	// source adapter or either promise; settled transfers remove it synchronously.
	sourceRef := weak.Make(source)
	targetRef := weak.Make(target)
	js.adoptionsMu.Lock()
	if js.adoptions == nil {
		js.adoptions = make(map[weak.Pointer[ChainedPromise]]weak.Pointer[ChainedPromise])
	}
	js.adoptions[targetRef] = sourceRef
	js.adoptionsMu.Unlock()
	runtime.AddCleanup(target, cleanupAdoption, adoptionCleanup{
		js:     weak.Make(js),
		target: targetRef,
	})
}

func cleanupAdoption(cleanup adoptionCleanup) {
	js := cleanup.js.Value()
	if js == nil {
		return
	}
	js.adoptionsMu.Lock()
	delete(js.adoptions, cleanup.target)
	js.adoptionsMu.Unlock()
}

func (p *ChainedPromise) claimAdoption(target *ChainedPromise) bool {
	js := p.js
	if js == nil {
		return false
	}
	sourceRef := weak.Make(p)
	targetRef := weak.Make(target)
	js.adoptionsMu.Lock()
	registeredSource, exists := js.adoptions[targetRef]
	if exists && registeredSource == sourceRef {
		delete(js.adoptions, targetRef)
	}
	js.adoptionsMu.Unlock()
	return exists && registeredSource == sourceRef
}

func (js *JS) recoverSettledAdoptions() {
	js.takeSettledAdoptions().settle()
}

func (js *JS) takeSettledAdoptions() adoptionSettlements {
	js.adoptionsMu.Lock()
	settlements := make(adoptionSettlements, 0, len(js.adoptions))
	for targetRef, sourceRef := range js.adoptions {
		target := targetRef.Value()
		source := sourceRef.Value()
		if target == nil || source == nil {
			delete(js.adoptions, targetRef)
			continue
		}
		state := promiseState(source.state.Load())
		if state == Pending {
			continue
		}
		delete(js.adoptions, targetRef)
		settlements = append(settlements, adoptionSettlement{
			source: source,
			target: target,
			state:  state,
			result: source.result,
		})
	}
	js.adoptionsMu.Unlock()
	return settlements
}

func (settlements adoptionSettlements) settle() {
	for _, settlement := range settlements {
		settlement.source.settleAdoption(settlement.target, settlement.state, settlement.result, rejectionReportUnowned)
	}
}

func (p *ChainedPromise) settleAdoption(target *ChainedPromise, state PromiseState, result any, reportOwner rejectionReportOwner) {
	switch state {
	case Fulfilled:
		target.resolveClaimed(result)
	case Rejected:
		if reportOwner == rejectionReportUnowned {
			p.propagateRejection(target, result)
			return
		}
		p.propagateRejectionOwned(target, result, reportOwner)
	}
}
