package gojaeventloop

import (
	"math"
	"strconv"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

const maxDelayMilliseconds int64 = math.MaxInt64 / int64(time.Millisecond)

// adapterDelayState is confined to the adapter's serialized logical owner.
// Intrusive-list membership is the sole settlement claim.
type adapterDelayState struct {
	resolve func(any) error
	reject  func(any) error
	prev    *adapterDelayState
	next    *adapterDelayState
	linked  bool
}

func (a *Adapter) delayDuration(value goja.Value) time.Duration {
	milliseconds := value.ToFloat()
	if math.IsNaN(milliseconds) || math.IsInf(milliseconds, -1) || milliseconds <= 0 {
		return 0
	}
	milliseconds = math.Trunc(milliseconds)
	if math.IsInf(milliseconds, 1) || milliseconds > float64(maxDelayMilliseconds) {
		message := "delay must not exceed " + strconv.FormatInt(maxDelayMilliseconds, 10) + " milliseconds"
		result, err := a.rangeErrorConstructor(goja.Undefined(), a.runtime.ToValue(message))
		if err != nil {
			a.panicJSException(err)
		}
		panic(result)
	}
	return time.Duration(int64(milliseconds)) * time.Millisecond
}

func (a *Adapter) registerDelay(state *adapterDelayState) {
	if a == nil || state == nil || state.resolve == nil || state.reject == nil {
		panic("goja-eventloop: invalid delay state")
	}
	if state.linked {
		panic("goja-eventloop: delay state registered more than once")
	}
	state.next = a.delayHead
	if a.delayHead != nil {
		a.delayHead.prev = state
	}
	a.delayHead = state
	state.linked = true
}

func (a *Adapter) finishDelay(state *adapterDelayState, rejected bool, result any) bool {
	if a == nil || state == nil || !state.linked {
		return false
	}
	if state.prev != nil {
		state.prev.next = state.next
	} else {
		a.delayHead = state.next
	}
	if state.next != nil {
		state.next.prev = state.prev
	}
	state.prev = nil
	state.next = nil
	state.linked = false
	a.settleClaimedDelay(state, rejected, result)
	return true
}

func (a *Adapter) takeDelayStates() []*adapterDelayState {
	if a == nil || a.delayHead == nil {
		return nil
	}
	var states []*adapterDelayState
	for state := a.delayHead; state != nil; {
		next := state.next
		state.prev = nil
		state.next = nil
		state.linked = false
		states = append(states, state)
		state = next
	}
	a.delayHead = nil
	return states
}

func (a *Adapter) settleClaimedDelay(state *adapterDelayState, rejected bool, result any) {
	if state == nil {
		return
	}
	resolve := state.resolve
	reject := state.reject
	state.resolve = nil
	state.reject = nil
	var err error
	if rejected {
		if reject != nil {
			err = reject(result)
		}
	} else if resolve != nil {
		err = resolve(result)
	}
	if err != nil {
		operation := "resolve delay promise"
		if rejected {
			operation = "reject delay promise"
		}
		a.reportPromiseJobError(wrapRuntimeError(operation, err))
	}
}

// delay returns a native Promise that settles after the extension delay.
func (a *Adapter) delay(call goja.FunctionCall) goja.Value {
	duration := a.delayDuration(call.Argument(0))
	nativePromise, resolve, reject := a.runtime.NewPromise()
	value := a.runtime.ToValue(nativePromise)
	state := &adapterDelayState{resolve: resolve, reject: reject}
	if a.exiting.Load() {
		a.settleClaimedDelay(state, true, goeventloop.ErrLoopTerminated)
		return value
	}

	a.registerDelay(state)
	parent := a.js.Sleep(duration)
	parent.Then(func(any) any {
		a.finishDelay(state, false, goja.Undefined())
		return nil
	}, func(reason any) any {
		a.finishDelay(state, true, reason)
		return nil
	})
	switch parent.State() {
	case goeventloop.Fulfilled:
		a.finishDelay(state, false, goja.Undefined())
	case goeventloop.Rejected:
		a.finishDelay(state, true, parent.Reason())
	}
	return value
}
