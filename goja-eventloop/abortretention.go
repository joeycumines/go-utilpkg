package gojaeventloop

import (
	"runtime"
	"sync"
	"sync/atomic"
	"weak"
)

type abortSignalLink struct {
	source     weak.Pointer[abortSignalState]
	dependent  weak.Pointer[abortSignalState]
	retained   atomic.Pointer[abortSignalState]
	cleanup    runtime.Cleanup
	mu         sync.Mutex
	id         uint64
	active     atomic.Bool
	cleanupSet bool
}

type abortTimeoutRef struct {
	adapter    weak.Pointer[Adapter]
	signal     weak.Pointer[abortSignalState]
	retained   atomic.Pointer[abortSignalState]
	cleanup    runtime.Cleanup
	timerID    uint64
	mu         sync.Mutex
	cleanupSet bool
}

type abortTimeoutCleanup struct {
	adapter weak.Pointer[Adapter]
	timerID uint64
}

type abortSignalLinkCleanup struct {
	source weak.Pointer[abortSignalState]
	id     uint64
}

var nextAbortSignalLinkID atomic.Uint64

func cleanupAbortTimeout(cleanup abortTimeoutCleanup) {
	adapter := cleanup.adapter.Value()
	if adapter == nil || adapter.loop == nil {
		return
	}
	_ = adapter.loop.SubmitInternal(func() {
		adapter.clearTimer(cleanup.timerID)
		if hooks := adapter.timerBackendHooks; hooks != nil && hooks.afterAbortRemoval != nil {
			hooks.afterAbortRemoval()
		}
	})
}

func (a *Adapter) abortOriginalSources(state *abortSignalState) []*abortSignalState {
	if state == nil {
		return nil
	}
	state.mu.Lock()
	if !state.dependent {
		state.mu.Unlock()
		return []*abortSignalState{state}
	}
	links := append([]*abortSignalLink(nil), state.sourceLinks...)
	state.mu.Unlock()
	sources := make([]*abortSignalState, 0, len(links))
	for _, link := range links {
		if source := activeAbortSignalLinkSource(link); source != nil {
			sources = append(sources, source)
		}
	}
	return sources
}

func (a *Adapter) linkAbortSignal(source, dependent *abortSignalState) {
	if source == nil || dependent == nil || source == dependent {
		return
	}
	link := &abortSignalLink{
		source:    weak.Make(source),
		dependent: weak.Make(dependent),
		id:        nextAbortSignalLinkID.Add(1),
	}
	link.active.Store(true)
	source.mu.Lock()
	source.dependentLinks = append(source.dependentLinks, link)
	source.mu.Unlock()
	dependent.mu.Lock()
	dependent.sourceLinks = append(dependent.sourceLinks, link)
	dependent.mu.Unlock()
	updateAbortSignalRetention(dependent)
	cleanup := runtime.AddCleanup(dependent, cleanupAbortSignalLink, abortSignalLinkCleanup{source: weak.Make(source), id: link.id})
	link.mu.Lock()
	if link.active.Load() {
		link.cleanup = cleanup
		link.cleanupSet = true
	} else {
		cleanup.Stop()
	}
	link.mu.Unlock()
	runtime.KeepAlive(dependent)
}

func cleanupAbortSignalLink(cleanup abortSignalLinkCleanup) {
	source := cleanup.source.Value()
	if source == nil {
		return
	}
	source.mu.Lock()
	var target *abortSignalLink
	for _, link := range source.dependentLinks {
		if link != nil && link.id == cleanup.id {
			target = link
			break
		}
	}
	source.mu.Unlock()
	unlinkAbortSignal(target)
}

func unlinkAbortSignal(link *abortSignalLink) {
	if link == nil {
		return
	}
	link.mu.Lock()
	if !link.active.Swap(false) {
		link.mu.Unlock()
		return
	}
	source := link.source.Value()
	dependent := link.dependent.Value()
	if link.cleanupSet {
		link.cleanup.Stop()
		link.cleanupSet = false
		runtime.KeepAlive(dependent)
	}
	retained := link.retained.Swap(nil) != nil
	if retained && source != nil {
		adjustAbortDependentObservers(source, -1)
	}
	link.mu.Unlock()
	if source != nil {
		source.mu.Lock()
		source.dependentLinks = removeAbortSignalLinkSlot(source.dependentLinks, link)
		source.mu.Unlock()
	}
	if dependent != nil {
		dependent.mu.Lock()
		dependent.sourceLinks = removeAbortSignalLinkSlot(dependent.sourceLinks, link)
		dependent.mu.Unlock()
	}
}

func removeAbortSignalLinkSlot(links []*abortSignalLink, target *abortSignalLink) []*abortSignalLink {
	for i, link := range links {
		if link == target {
			copy(links[i:], links[i+1:])
			links[len(links)-1] = nil
			links = links[:len(links)-1]
			if len(links) == 0 {
				return nil
			}
			return links
		}
	}
	return links
}

func changeAbortSignalObservers(state *abortSignalState, delta int) {
	if state == nil || delta == 0 {
		return
	}
	state.mu.Lock()
	before := state.observers != 0 && !state.aborted
	state.observers += delta
	if state.observers < 0 {
		state.mu.Unlock()
		panic("goja-eventloop: negative AbortSignal observer count")
	}
	after := state.observers != 0 && !state.aborted
	state.mu.Unlock()
	if before != after {
		updateAbortSignalRetention(state)
	}
}

func updateAbortSignalRetention(state *abortSignalState) {
	if state == nil {
		return
	}
	state.retentionMu.Lock()
	defer state.retentionMu.Unlock()
	state.mu.Lock()
	retain := state.observers != 0 && !state.aborted
	links := append([]*abortSignalLink(nil), state.sourceLinks...)
	state.mu.Unlock()
	for _, link := range links {
		if link == nil {
			continue
		}
		link.mu.Lock()
		if !link.active.Load() {
			link.mu.Unlock()
			continue
		}
		source := link.source.Value()
		changed := false
		if retain {
			if link.retained.Load() == nil {
				link.retained.Store(state)
				changed = true
			}
		} else if link.retained.Load() == state {
			link.retained.Store(nil)
			changed = true
		}
		if changed && source != nil {
			if retain {
				adjustAbortDependentObservers(source, 1)
			} else {
				adjustAbortDependentObservers(source, -1)
			}
		}
		link.mu.Unlock()
	}
	refreshAbortTimeoutRetention(state)
}

func activeAbortSignalLinkSource(link *abortSignalLink) *abortSignalState {
	if link == nil {
		return nil
	}
	link.mu.Lock()
	defer link.mu.Unlock()
	if !link.active.Load() {
		return nil
	}
	return link.source.Value()
}

func activeAbortSignalLinkDependent(link *abortSignalLink) *abortSignalState {
	if link == nil {
		return nil
	}
	link.mu.Lock()
	defer link.mu.Unlock()
	if !link.active.Load() {
		return nil
	}
	return link.dependent.Value()
}

func adjustAbortDependentObservers(state *abortSignalState, delta int) {
	if state == nil || delta == 0 {
		return
	}
	state.mu.Lock()
	state.dependentObservers += delta
	if state.dependentObservers < 0 {
		state.mu.Unlock()
		panic("goja-eventloop: negative AbortSignal dependent observer count")
	}
	state.mu.Unlock()
	refreshAbortTimeoutRetention(state)
}

func refreshAbortTimeoutRetention(state *abortSignalState) {
	if state == nil {
		return
	}
	state.mu.Lock()
	timeout := state.timeout
	retain := !state.aborted && state.observers+state.dependentObservers != 0
	if timeout != nil {
		if retain {
			timeout.retained.Store(state)
		} else {
			timeout.retained.Store(nil)
		}
	}
	state.mu.Unlock()
}

func stopAbortTimeoutCleanup(timeout *abortTimeoutRef, state *abortSignalState) {
	if timeout == nil {
		return
	}
	timeout.mu.Lock()
	if timeout.cleanupSet {
		timeout.cleanup.Stop()
		timeout.cleanupSet = false
		runtime.KeepAlive(state)
	}
	timeout.mu.Unlock()
}
