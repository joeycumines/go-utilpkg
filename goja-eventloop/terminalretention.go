package gojaeventloop

import (
	"runtime"
	"time"
	"weak"

	"github.com/joeycumines/goja"
)

type adapterTimerRegistry struct {
	states map[weak.Pointer[adapterTimer]]struct{}
}

type adapterTimerCleanup struct {
	adapter weak.Pointer[Adapter]
	state   weak.Pointer[adapterTimer]
}

func newAdapterTimerRegistry() *adapterTimerRegistry {
	return &adapterTimerRegistry{states: make(map[weak.Pointer[adapterTimer]]struct{})}
}

func cleanupAdapterTimerRegistration(token adapterTimerCleanup) {
	adapter := token.adapter.Value()
	if adapter == nil {
		return
	}
	adapter.timersMu.Lock()
	delete(adapter.timerRegistry.states, token.state)
	adapter.timersMu.Unlock()
}

func (a *Adapter) registerTimerState(state *adapterTimer) bool {
	if a == nil || state == nil {
		return false
	}
	pointer := weak.Make(state)
	a.timersMu.Lock()
	if a.exiting.Load() {
		retireTimerState(state)
		a.timersMu.Unlock()
		return false
	}
	a.timerRegistry.states[pointer] = struct{}{}
	a.timersMu.Unlock()
	if !state.cleanupSet.CompareAndSwap(false, true) {
		return true
	}
	runtime.AddCleanup(state, cleanupAdapterTimerRegistration, adapterTimerCleanup{
		adapter: weak.Make(a),
		state:   pointer,
	})
	if hooks := a.timerBackendHooks; hooks != nil && hooks.afterCleanupRegistration != nil {
		hooks.afterCleanupRegistration()
	}
	return true
}

func retireTimerState(state *adapterTimer) *adapterTimerPayload {
	if state == nil {
		return nil
	}
	state.canceled.Store(true)
	state.active.Store(false)
	state.executing.Store(false)
	state.refed.Store(false)
	return state.payload.Swap(nil)
}

// terminateCleanup runs as the terminal runtime owner. JavaScript first severs
// every TimersList and callback root; Go then retires the carrier and mirrors.
func (a *Adapter) terminateCleanup() {
	if a == nil {
		return
	}
	a.exiting.Store(true)
	for _, state := range a.takeDelayStates() {
		a.settleClaimedDelay(state, false, goja.Undefined())
	}
	a.timersMu.Lock()
	timerHandles := make([]any, 0, len(a.timerRegistry.states))
	for wp := range a.timerRegistry.states {
		state := wp.Value()
		if state == nil {
			continue
		}
		if payload := state.payload.Load(); payload != nil && payload.object != nil {
			timerHandles = append(timerHandles, payload.object)
		}
	}
	a.timersMu.Unlock()
	a.immediatesMu.Lock()
	immediateHandles := make([]any, 0, len(a.immediates))
	for _, immediate := range a.immediates {
		if immediate != nil && immediate.object != nil {
			immediateHandles = append(immediateHandles, immediate.object)
		}
	}
	a.immediatesMu.Unlock()
	if a.timerTerminator != nil {
		if _, err := a.timerTerminator(
			goja.Undefined(),
			a.runtime.NewArray(timerHandles...),
			a.runtime.NewArray(immediateHandles...),
		); err != nil {
			a.handleHostCallbackResult("timer.terminate", err)
		}
	}
	a.retireTimerBackendCarrier()

	a.timersMu.Lock()
	for wp := range a.timerRegistry.states {
		state := wp.Value()
		if payload := retireTimerState(state); payload != nil {
			payload.object = nil
		}
		delete(a.timerRegistry.states, wp)
	}
	a.timers = make(map[uint64]*adapterTimer)
	a.timeoutBackendRefed = false
	a.genericImmediateRefs = 0
	a.genericImmediateRefID = 0
	a.timersMu.Unlock()

	a.immediatesMu.Lock()
	for _, immediate := range a.immediates {
		retireImmediateState(immediate)
	}
	a.immediates = make(map[uint64]*adapterImmediate)
	a.immediatesMu.Unlock()

	a.processEmitterCore = nil
	a.processMu.Lock()
	clear(a.pendingRejectionOrder)
	a.pendingRejectionOrder = nil
	clear(a.pendingRejections)
	a.pendingRejections = make(map[*goja.Promise]goja.Value)
	a.rejectionCheckScheduled = false
	a.processMu.Unlock()

	a.consoleTimersMu.Lock()
	a.consoleTimers = make(map[string]time.Time)
	a.consoleTimersMu.Unlock()
	a.consoleCountersMu.Lock()
	a.consoleCounters = make(map[string]int)
	a.consoleCountersMu.Unlock()
	a.consoleIndentMu.Lock()
	a.consoleIndent = 0
	a.consoleIndentMu.Unlock()

	// These helpers exist only to construct or roll back the atomic Bind
	// transaction. Installed functions do not depend on them afterward.
	a.processClone = nil
	a.propertyRestore = nil
	a.webConstructorFactory = nil
	a.ordinaryFunctionFactory = nil

	a.dispatchJSEvents.Range(func(key, _ any) bool {
		a.dispatchJSEvents.Delete(key)
		return true
	})
}
