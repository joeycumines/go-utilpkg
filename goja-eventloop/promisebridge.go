package gojaeventloop

import (
	"errors"
	"sync/atomic"

	"github.com/joeycumines/goja"
)

var errPromiseResultExit = errors.New("goja-eventloop: promise result callback exited")

// PromiseSettler admits exactly one Promise result. Its value-producing
// callbacks always run under the logical adapter owner and receive the exact
// claimed runtime. If settlement conversion encounters a Goja value owned by
// another runtime, the Promise is rejected with the resulting TypeError. The
// fields are intentionally private so callers cannot split the shared
// exactly-once admission state from the native Promise settlement. Resolve or
// Reject on a nil pointer or zero value returns ErrAdapterInvalid; calls after
// the first admitted attempt return ErrPromiseSettled.
type PromiseSettler struct {
	state *promiseSettlementState
}

type promiseSettlementState struct {
	payload atomic.Pointer[promiseSettlementPayload]
}

type promiseSettlementPayload struct {
	adapter *Adapter
	resolve func(any) error
	reject  func(any) error
}

// NewPromise creates a native Goja Promise on the runtime owner and returns an
// exactly-once settler safe to call from another goroutine. NewPromise must be
// called by the runtime owner, either under the logical callback-owner role or
// during exclusive setup before the loop starts, and must never run
// concurrently with other Goja access. It panics for an invalid or unbound
// adapter.
func (a *Adapter) NewPromise() (goja.Value, *PromiseSettler) {
	a.mustOriginalReceiver("NewPromise")
	if a.state() != adapterStateBound || !a.claimed() {
		panic("goja-eventloop: NewPromise requires a bound adapter on its runtime owner")
	}
	promise, resolve, reject := a.runtime.NewPromise()
	state := new(promiseSettlementState)
	state.payload.Store(&promiseSettlementPayload{
		adapter: a,
		resolve: resolve,
		reject:  reject,
	})
	settler := &PromiseSettler{state: state}
	return a.runtime.ToValue(promise), settler
}

// Resolve admits a fulfillment callback. It panics if result is nil, including
// for a nil or zero receiver. A nil or zero receiver with a non-nil result
// returns ErrAdapterInvalid. The sole settlement attempt is consumed even when
// owner submission fails.
func (s *PromiseSettler) Resolve(result func(*goja.Runtime) any) error {
	return s.settle(result, false)
}

// Reject admits a rejection callback. It has the same nil receiver and nil
// result behavior as Resolve. Resolve and Reject share one exactly-once
// admission state.
func (s *PromiseSettler) Reject(result func(*goja.Runtime) any) error {
	return s.settle(result, true)
}

func (s *PromiseSettler) settle(result func(*goja.Runtime) any, rejected bool) error {
	if result == nil {
		panic("goja-eventloop: promise result callback must not be nil")
	}
	if s == nil || s.state == nil {
		return ErrAdapterInvalid
	}
	payload := s.state.payload.Swap(nil)
	if payload == nil {
		return ErrPromiseSettled
	}
	if payload.adapter == nil || payload.resolve == nil || payload.reject == nil {
		return ErrAdapterInvalid
	}
	return payload.adapter.Submit(func(runtime *goja.Runtime) {
		payload.adapter.runPromiseSettlement(runtime, result, rejected, payload.resolve, payload.reject)
	})
}

func (a *Adapter) runPromiseSettlement(
	runtime *goja.Runtime,
	result func(*goja.Runtime) any,
	rejected bool,
	resolve func(any) error,
	reject func(any) error,
) {
	returned := false
	defer func() {
		if returned {
			return
		}
		reason := recover()
		if reason == nil {
			reason = errPromiseResultExit
		}
		if err := reject(reason); err != nil {
			a.reportPromiseJobError(wrapRuntimeError("reject adapter promise result", err))
		}
	}()
	value := invokePromiseResult(runtime, result)
	var err error
	if rejected {
		err = reject(value)
	} else {
		err = resolve(value)
	}
	returned = true
	if err != nil {
		a.reportPromiseJobError(wrapRuntimeError("settle adapter promise", err))
	}
}

func invokePromiseResult(runtime *goja.Runtime, result func(*goja.Runtime) any) (value any) {
	returned := false
	defer func() {
		panicValue := recover()
		if panicValue != nil {
			panic(panicValue)
		}
		if !returned {
			panic(errPromiseResultExit)
		}
	}()
	value = result(runtime)
	returned = true
	return value
}
