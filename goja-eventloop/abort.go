package gojaeventloop

import (
	"sync"

	"github.com/joeycumines/goja"
)

type abortSignalState struct {
	reason             goja.Value
	target             *eventTargetWrapper
	object             *goja.Object
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
}

type abortControllerState struct {
	signal *abortSignalState
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
