package gojaeventloop

import (
	"sync"

	"github.com/joeycumines/goja"
)

type abortSignalState struct {
	reason             goja.Value
	target             *eventTargetWrapper
	onabort            *eventTargetListenerInfo
	timeout            *abortTimeoutRef
	algorithms         []*abortAlgorithm
	sourceLinks        []*abortSignalLink
	dependentLinks     []*abortSignalLink
	observers          int
	dependentObservers int
	retentionMu        sync.Mutex
	mu                 sync.Mutex
	aborted            bool
	dependent          bool

	// retainedObject strongly pins the JS signal object exactly while the
	// Go-side retention machinery pins this state (see
	// refreshAbortTimeoutRetention): a non-aborted signal with abort
	// listeners, abort algorithms, or dependent observers must not be
	// garbage-collected (DOM: retained signals are not collected), and
	// dispatch must be able to observe the exact object identity for
	// this/target/currentTarget. The pin is released when the retention
	// condition ends or after the abort dispatch completes. It is scoped to
	// retained states only — it is NOT an unconditional strong pointer on
	// every EventTarget (unrelated EventTarget objects remain weakly
	// referenced and collectible).
	retainedObject *goja.Object
}

// abortControllerState carries exactly one reference — the strong JS signal
// object. The abortSignalState is always derived from that object through the
// hidden-state store (see abortControllerSignalState), so the two can never
// drift. The signal object must be strongly held: the previous design stored
// only a weak pointer, so after a Go GC cycle controller.signal returned nil
// and any JS use of it crashed the VM.
type abortControllerState struct {
	signalObject *goja.Object
}

func markAbortSignal(state *abortSignalState, reason goja.Value) {
	if state == nil {
		return
	}
	state.mu.Lock()
	state.aborted = true
	state.reason = reason
	state.mu.Unlock()
}

func (a *Adapter) abortSignalSequence(iterable goja.Value) []*abortSignalState {
	if iterable == nil || goja.IsUndefined(iterable) || goja.IsNull(iterable) || a.getIterator == nil {
		panic(a.runtime.NewTypeError("AbortSignal.any requires an iterable"))
	}
	iteratorMethod, err := a.getIterator(goja.Undefined(), iterable)
	if err != nil {
		a.panicJSException(err)
	}
	iteratorFactory, ok := goja.AssertFunction(iteratorMethod)
	if !ok {
		panic(a.runtime.NewTypeError("AbortSignal.any requires an iterable"))
	}
	iteratorValue, err := iteratorFactory(iterable)
	if err != nil {
		a.panicJSException(err)
	}
	iterator, ok := iteratorValue.(*goja.Object)
	if !ok || iterator == nil {
		panic(a.runtime.NewTypeError("AbortSignal.any iterator must be an object"))
	}
	next, ok := goja.AssertFunction(iterator.Get("next"))
	if !ok {
		panic(a.runtime.NewTypeError("AbortSignal.any iterator.next must be a function"))
	}

	var signals []*abortSignalState
	for {
		nextValue, err := next(iterator)
		if err != nil {
			a.panicJSException(err)
		}
		nextResult, ok := nextValue.(*goja.Object)
		if !ok || nextResult == nil {
			panic(a.runtime.NewTypeError("AbortSignal.any iterator result must be an object"))
		}

		var (
			done   bool
			signal *abortSignalState
		)
		if exception := a.runtime.Try(func() {
			done = nextResult.Get("done").ToBoolean()
			if !done {
				signal = a.abortSignalValue(nextResult.Get("value"))
			}
		}); exception != nil {
			panic(exception.Value())
		}
		if done {
			return signals
		}
		signals = append(signals, signal)
	}
}

func (a *Adapter) abortReason(value goja.Value) goja.Value {
	if value == nil || goja.IsUndefined(value) {
		return a.abortDefaultReason()
	}
	return value
}

func (a *Adapter) abortDefaultReason() goja.Value {
	return a.throwDOMException("AbortError", "This operation was aborted")
}

func (a *Adapter) timeoutReason() goja.Value {
	return a.throwDOMException("TimeoutError", "The operation was aborted due to timeout")
}

func (a *Adapter) abortSignalValue(value goja.Value) *abortSignalState {
	return a.abortSignalValueMessage(value, "AbortSignal.any requires AbortSignal instances")
}

func (a *Adapter) abortSignalValueMessage(value goja.Value, message string) *abortSignalState {
	state, ok := a.abortSignalStateValue(value)
	if !ok {
		panic(a.runtime.NewTypeError(message))
	}
	return state
}

func (a *Adapter) abortSignalStateValue(value goja.Value) (*abortSignalState, bool) {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil, false
	}
	obj, ok := value.(*goja.Object)
	if !ok || obj == nil {
		return nil, false
	}
	stateValue := a.hiddenState(a.abortSignalStateStore, obj)
	if stateValue == nil || goja.IsUndefined(stateValue) || goja.IsNull(stateValue) {
		return nil, false
	}
	state, ok := stateValue.Export().(*abortSignalState)
	if !ok || state == nil {
		return nil, false
	}
	return state, true
}
