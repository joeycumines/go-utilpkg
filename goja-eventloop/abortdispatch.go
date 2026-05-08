package gojaeventloop

import (
	"runtime"
	"sync"
	"sync/atomic"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

type abortAlgorithm struct {
	callback func()
	cleanup  *abortAlgorithmCleanup
	removed  atomic.Bool
}

type abortAlgorithmCleanup struct {
	state     *abortSignalState
	algorithm *abortAlgorithm
	mu        sync.Mutex
}

type abortSignalDispatch struct {
	state      *abortSignalState
	algorithms []*abortAlgorithm
	completed  bool
}

type abortSignalPanicState struct {
	value    any
	panicked bool
}

// TrackAbortSignal registers callback for signal abort and returns a cleanup
// function that may be called from any goroutine. The signal value must come
// from this adapter's AbortSignal implementation; non-signals return ok=false.
// Callback must not be nil. The returned cleanup is idempotent and may be
// called from any goroutine.
// TrackAbortSignal itself must be called on the bound original Adapter by the
// runtime owner, either under the current logical adapter callback-owner role or
// during exclusive setup before the loop starts. Violating that ownership
// precondition panics.
func (a *Adapter) TrackAbortSignal(signal goja.Value, callback func()) (cleanup func(), aborted bool, ok bool) {
	a.mustOriginalReceiver("TrackAbortSignal")
	if a.state() != adapterStateBound || !a.claimed() {
		panic("goja-eventloop: TrackAbortSignal requires a bound adapter on its runtime owner")
	}
	if callback == nil {
		panic("goja-eventloop: TrackAbortSignal callback must not be nil")
	}
	state, ok := a.abortSignalStateValue(signal)
	if !ok {
		return nil, false, false
	}
	cleanup, aborted = a.addAbortAlgorithm(state, callback)
	return cleanup, aborted, true
}

func (a *Adapter) addAbortAlgorithm(state *abortSignalState, callback func()) (cleanup func(), aborted bool) {
	if state == nil {
		return nil, false
	}
	algorithm := &abortAlgorithm{callback: callback}
	cleanupState := &abortAlgorithmCleanup{state: state, algorithm: algorithm}
	algorithm.cleanup = cleanupState
	state.mu.Lock()
	if state.aborted {
		state.mu.Unlock()
		return nil, true
	}
	wasObserved := state.observers != 0
	state.algorithms = append(state.algorithms, algorithm)
	state.observers++
	state.mu.Unlock()
	if !wasObserved {
		updateAbortSignalRetention(state)
	}
	return cleanupState.run, false
}

func (cleanup *abortAlgorithmCleanup) run() {
	if cleanup == nil {
		return
	}
	cleanup.mu.Lock()
	state := cleanup.state
	algorithm := cleanup.algorithm
	cleanup.state = nil
	cleanup.algorithm = nil
	cleanup.mu.Unlock()
	removeAbortAlgorithm(state, algorithm)
}

func (algorithm *abortAlgorithm) detachCleanup() {
	if algorithm == nil {
		return
	}
	cleanup := algorithm.cleanup
	algorithm.cleanup = nil
	if cleanup == nil {
		return
	}
	cleanup.mu.Lock()
	cleanup.state = nil
	cleanup.algorithm = nil
	cleanup.mu.Unlock()
}

func removeAbortAlgorithm(state *abortSignalState, target *abortAlgorithm) bool {
	if state == nil || target == nil || target.removed.Swap(true) {
		return false
	}
	target.callback = nil
	target.detachCleanup()
	state.mu.Lock()
	removed := false
	for i, algorithm := range state.algorithms {
		if algorithm == target {
			copy(state.algorithms[i:], state.algorithms[i+1:])
			state.algorithms[len(state.algorithms)-1] = nil
			state.algorithms = state.algorithms[:len(state.algorithms)-1]
			if len(state.algorithms) == 0 {
				state.algorithms = nil
			}
			state.observers--
			removed = true
			break
		}
	}
	unobserved := removed && state.observers == 0 && !state.aborted
	state.mu.Unlock()
	if unobserved {
		updateAbortSignalRetention(state)
	}
	return removed
}

func (a *Adapter) abortSignalState(state *abortSignalState, reason goja.Value) {
	if state == nil {
		return
	}
	if reason == nil {
		reason = a.abortDefaultReason()
	}
	root, dependents, ok := beginAbortSignal(state, reason)
	if !ok {
		return
	}
	dispatches := []*abortSignalDispatch{root}
	seen := map[*abortSignalState]struct{}{state: {}}
	for len(dependents) != 0 {
		dependent := dependents[0]
		dependents = dependents[1:]
		if dependent == nil {
			continue
		}
		if _, exists := seen[dependent]; exists {
			continue
		}
		seen[dependent] = struct{}{}
		dispatch, nested, marked := beginAbortSignal(dependent, reason)
		if !marked {
			continue
		}
		dispatches = append(dispatches, dispatch)
		dependents = append(dependents, nested...)
	}
	a.runAbortSignalDispatches(dispatches)
}

func beginAbortSignal(state *abortSignalState, reason goja.Value) (*abortSignalDispatch, []*abortSignalState, bool) {
	state.mu.Lock()
	if state.aborted {
		state.mu.Unlock()
		return nil, nil, false
	}
	state.aborted = true
	state.reason = reason
	algorithms := state.algorithms
	state.algorithms = nil
	state.observers -= len(algorithms)
	sourceLinks := state.sourceLinks
	state.sourceLinks = nil
	dependentLinks := state.dependentLinks
	state.dependentLinks = nil
	timeout := state.timeout
	state.timeout = nil
	state.mu.Unlock()

	for index, link := range sourceLinks {
		sourceLinks[index] = nil
		unlinkAbortSignal(link)
	}
	dependents := make([]*abortSignalState, 0, len(dependentLinks))
	for index, link := range dependentLinks {
		dependentLinks[index] = nil
		if dependent := activeAbortSignalLinkDependent(link); dependent != nil {
			dependents = append(dependents, dependent)
		}
		unlinkAbortSignal(link)
	}
	if timeout != nil {
		timeout.retained.Store(nil)
		stopAbortTimeoutCleanup(timeout, state)
	}
	return &abortSignalDispatch{state: state, algorithms: algorithms}, dependents, true
}

func (a *Adapter) runAbortSignalDispatches(dispatches []*abortSignalDispatch) {
	panicState := abortSignalPanicState{}
	completed := false
	defer func() {
		for _, dispatch := range dispatches {
			dispatch.abandon()
		}
		if !completed && panicState.panicked {
			panic(panicState.value)
		}
	}()

	for _, dispatch := range dispatches {
		a.runAbortSignalDispatch(dispatch, &panicState)
	}
	completed = true
	if panicState.panicked {
		panic(panicState.value)
	}
}

func (a *Adapter) runAbortSignalDispatch(dispatch *abortSignalDispatch, panicState *abortSignalPanicState) {
	if dispatch == nil || dispatch.state == nil || dispatch.completed {
		return
	}
	for index, algorithm := range dispatch.algorithms {
		dispatch.algorithms[index] = nil
		if algorithm == nil || algorithm.removed.Swap(true) {
			continue
		}
		callback := algorithm.callback
		algorithm.callback = nil
		algorithm.detachCleanup()
		if callback != nil {
			panicState.capture(invokeAbortSignalStep(callback))
		}
	}
	dispatch.algorithms = nil
	panicState.capture(invokeAbortSignalStep(func() {
		event := goeventloop.NewEvent("abort")
		eventObj := a.wrapEvent(event).(*goja.Object)
		_, eventState := a.eventStateArgument(eventObj)
		a.dispatchJSEvent(dispatch.state.target, eventObj, eventState)
	}))
	dispatch.completed = true
}

func (dispatch *abortSignalDispatch) abandon() {
	if dispatch == nil || dispatch.completed {
		return
	}
	for index, algorithm := range dispatch.algorithms {
		if algorithm != nil && !algorithm.removed.Swap(true) {
			algorithm.callback = nil
			algorithm.detachCleanup()
		}
		dispatch.algorithms[index] = nil
	}
	dispatch.algorithms = nil
	dispatch.completed = true
	dispatch.state = nil
}

func (state *abortSignalPanicState) capture(value any, panicked bool) {
	if state != nil && panicked && !state.panicked {
		state.value = value
		state.panicked = true
	}
}

func invokeAbortSignalStep(callback func()) (value any, panicked bool) {
	completed := false
	defer func() {
		if completed {
			return
		}
		value = recover()
		if value == nil {
			value = new(runtime.PanicNilError)
		}
		panicked = true
	}()
	callback()
	completed = true
	return nil, false
}
