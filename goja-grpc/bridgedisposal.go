package gojagrpc

import (
	"slices"
	"sync"

	goeventloop "github.com/joeycumines/go-eventloop"
)

type ownerDisposalID uint64

type ownerDisposalActionKind uint8

const (
	ownerDisposalPromise ownerDisposalActionKind = iota + 1
	ownerDisposalDisposer
	ownerDisposalRoot
)

type ownerDisposalAction struct {
	entry    ownerPromiseEntry
	disposer func(error)
	root     supervisorChildID
	kind     ownerDisposalActionKind
}

// ownerDisposerCall pairs a disposer with the error it must receive. Post-Done
// disposers are collected while postDoneMu is held but invoked only after the
// lock is released, so a disposer that re-enters the bridge cannot deadlock.
type ownerDisposerCall struct {
	disposer func(error)
	err      error
}

// runPostDoneDisposers invokes every collected post-Done disposer outside the
// postDone lock. Each disposer runs under recover() so that one panicking
// disposer cannot skip the rest (matching the per-step panic isolation of the
// live-owner path in runOwnerDisposalStep). Note: runtime.Goexit inside a
// disposer still terminates the collecting goroutine, exactly as it would a
// live-owner step goroutine.
func runPostDoneDisposers(disposers []ownerDisposerCall) {
	for _, call := range disposers {
		if call.disposer == nil {
			continue
		}
		func() {
			defer func() { _ = recover() }()
			call.disposer(call.err)
		}()
	}
}

type ownerDisposalRun struct {
	done     chan struct{}
	actions  []ownerDisposalAction
	roots    []supervisorChildID
	err      error
	next     int
	doneOnce sync.Once
}

func (r *ownerDisposalRun) finish() {
	r.doneOnce.Do(func() { close(r.done) })
}

type ownerRootDisposal struct {
	id   supervisorChildID
	root *ownerRootEntry
}

func (d *ownerDispatcher) prepareOwnerRootDisposal(
	id supervisorChildID,
	preparing bool,
) ownerRootDisposal {
	d.beginRootClose(id)
	root := d.bridge.roots[id]
	if root == nil && preparing {
		// Tombstones exist only for close racing a preparing constructor.
		// The late constructor consumes this bounded marker.
		d.bridge.tombstones[id] = struct{}{}
	} else {
		delete(d.bridge.roots, id)
	}
	d.bridge.effects.Range(func(key, value any) bool {
		effect := value.(*ownerEffect)
		if effect.promise.root != id {
			return true
		}
		if _, loaded := d.bridge.effects.LoadAndDelete(key); loaded {
			d.releaseRootEffect(id)
			effect.finish(newOwnerEffectAck(goeventloop.ErrLoopTerminated))
		}
		return true
	})
	d.bridge.callbackEffects.Range(func(key, value any) bool {
		effect := value.(*ownerCallbackEffect)
		if effect.callback.root != id {
			return true
		}
		if _, loaded := d.bridge.callbackEffects.LoadAndDelete(key); loaded {
			d.releaseRootEffect(id)
			effect.finish(newOwnerEffectAck(goeventloop.ErrLoopTerminated))
		}
		return true
	})
	return ownerRootDisposal{id: id, root: root}
}

func ownerDisposalActions(
	disposals []ownerRootDisposal,
) []ownerDisposalAction {
	var actions []ownerDisposalAction
	for _, disposal := range disposals {
		root := disposal.root
		if root != nil {
			for index, disposer := range root.disposers {
				root.disposers[index] = nil
				actions = append(actions, ownerDisposalAction{
					disposer: disposer,
					root:     disposal.id,
					kind:     ownerDisposalDisposer,
				})
			}
			root.disposers = nil
			children := make([]uint64, 0, len(root.promises))
			for child := range root.promises {
				children = append(children, child)
			}
			slices.Sort(children)
			for _, child := range children {
				entry := root.promises[child]
				delete(root.promises, child)
				actions = append(actions, ownerDisposalAction{
					entry: entry,
					root:  disposal.id,
					kind:  ownerDisposalPromise,
				})
			}
			clear(root.callbacks)
		}
		actions = append(actions, ownerDisposalAction{
			root: disposal.id,
			kind: ownerDisposalRoot,
		})
	}
	return actions
}

// beginOwnerDisposal must run on-owner. It detaches every exact root before
// any user-derived cleanup, then schedules one scalar-ID owner step per
// obligation. A panic or Goexit can abandon only its current obligation.
func (d *ownerDispatcher) beginOwnerDisposal(
	roots []supervisorRoot,
	err error,
) <-chan struct{} {
	if len(roots) == 0 {
		done := make(chan struct{})
		close(done)
		return done
	}
	// This mutex is normally uncontended because owner work is serialized.
	// It closes the one exceptional boundary where Adapter.Done transfers the
	// owner maps to worker cleanup while a late owner-side disposal entry is
	// still being routed.
	d.bridge.postDoneMu.Lock()
	if d.bridge.transferred.Load() {
		disposers, done := d.discardOwnerRootsPostDoneLocked(roots, err)
		d.bridge.postDoneMu.Unlock()
		runPostDoneDisposers(disposers)
		return done
	}
	select {
	case <-d.adapter.Done():
		d.bridge.transferred.Store(true)
		disposers, done := d.discardOwnerRootsPostDoneLocked(roots, err)
		d.bridge.postDoneMu.Unlock()
		runPostDoneDisposers(disposers)
		return done
	default:
	}
	runs := make([]<-chan struct{}, 0, len(roots))
	schedule := make([]ownerDisposalID, 0, len(roots))
	for _, root := range roots {
		id := ownerDisposalID(root.id)
		if existing := d.bridge.disposals[id]; existing != nil {
			runs = append(runs, existing.done)
			continue
		}
		disposal := d.prepareOwnerRootDisposal(root.id, root.preparing)
		run := &ownerDisposalRun{
			done:    make(chan struct{}),
			actions: ownerDisposalActions([]ownerRootDisposal{disposal}),
			roots:   []supervisorChildID{root.id},
			err:     err,
		}
		d.bridge.disposals[id] = run
		runs = append(runs, run.done)
		schedule = append(schedule, id)
		go func() {
			select {
			case <-run.done:
			case <-d.adapter.Done():
				d.finishOwnerDisposalPostDone(id)
			}
		}()
	}
	for _, id := range schedule {
		d.scheduleOwnerDisposal(id)
	}
	d.bridge.postDoneMu.Unlock()
	if len(runs) == 1 {
		return runs[0]
	}
	done := make(chan struct{})
	go func() {
		for _, runDone := range runs {
			<-runDone
		}
		close(done)
	}()
	return done
}

func (d *ownerDispatcher) scheduleOwnerDisposal(id ownerDisposalID) {
	run := d.bridge.disposals[id]
	if run == nil {
		return
	}
	if run.next >= len(run.actions) {
		delete(d.bridge.disposals, id)
		run.actions = nil
		run.roots = nil
		run.finish()
		return
	}
	_ = d.submit(func() { d.runOwnerDisposalStep(id) })
}

func (d *ownerDispatcher) runOwnerDisposalStep(id ownerDisposalID) {
	run := d.bridge.disposals[id]
	if run == nil || run.next >= len(run.actions) {
		d.scheduleOwnerDisposal(id)
		return
	}
	action := run.actions[run.next]
	run.actions[run.next] = ownerDisposalAction{}
	run.next++
	defer func() {
		_ = recover()
		d.scheduleOwnerDisposal(id)
	}()
	switch action.kind {
	case ownerDisposalPromise:
		settleOwnerDisposalPromise(action.entry, run.err)
	case ownerDisposalDisposer:
		action.disposer(run.err)
	case ownerDisposalRoot:
		d.finishRootClose(action.root)
	default:
		panic("gojagrpc: invalid owner disposal action")
	}
}

func settleOwnerDisposalPromise(entry ownerPromiseEntry, err error) {
	settled := false
	defer func() {
		if !settled {
			_ = settleOwnerEntry(
				entry,
				entry.terminalProjection,
				ownerDisposalFallbackResult,
				true,
			)
		}
	}()
	result := ownerStatusResult{status: canonicalOwnerStatus(err)}
	_ = settleOwnerEntry(entry, entry.terminalProjection, result, true)
	settled = true
}

func (d *ownerDispatcher) finishOwnerDisposalPostDone(
	id ownerDisposalID,
) {
	<-d.adapter.Done()
	d.bridge.postDoneMu.Lock()
	d.bridge.transferred.Store(true)
	disposers, _ := d.finishOwnerDisposalRunPostDoneLocked(id)
	d.bridge.postDoneMu.Unlock()
	runPostDoneDisposers(disposers)
}

// finishOwnerDisposalRunPostDoneLocked tears down a pending disposal run after
// the adapter is done and returns the disposers that must still fire. It must
// be called with postDoneMu held, and the returned disposers must be invoked
// with runPostDoneDisposers only after the lock is released: disposers are
// host-registered user code and must never run inside the critical section.
//
// The returned ok reports whether a run was found. That guard is load-bearing
// for memory safety: a second entry after run.actions = nil would panic on
// nil[run.next:], and deletion under the lock plus the early return make that
// impossible. run.next is set to len(run.actions) before any collection so
// that exactly-once holds structurally rather than by argument: the live-owner
// path advances run.next before invoking an action (runOwnerDisposalStep), and
// this function cannot re-enter because the run is removed from disposals
// first and every sweep runs under the same lock.
func (d *ownerDispatcher) finishOwnerDisposalRunPostDoneLocked(
	id ownerDisposalID,
) (disposers []ownerDisposerCall, ok bool) {
	run := d.bridge.disposals[id]
	if run == nil {
		return nil, false
	}
	delete(d.bridge.disposals, id)
	// Post-Done contract: disposers must still run. Promise and root actions
	// are intentionally dropped (no Goja projections post-Done; root fences
	// are closed below), but every not-yet-executed disposer fires exactly
	// once with the disposal error.
	for _, action := range run.actions[run.next:] {
		if action.kind == ownerDisposalDisposer && action.disposer != nil {
			disposers = append(disposers, ownerDisposerCall{
				disposer: action.disposer,
				err:      run.err,
			})
		}
	}
	run.next = len(run.actions)
	clear(run.actions)
	run.actions = nil
	for _, root := range run.roots {
		var fenceDone <-chan struct{}
		if value, ok := d.bridge.fences.Load(root); ok {
			fenceDone = value.(*ownerRootFence).done
		}
		d.finishRootClose(root)
		if fenceDone != nil {
			<-fenceDone
		}
	}
	run.roots = nil
	run.finish()
	return disposers, true
}

// discardOwnerRootsPostDoneLocked discards the given roots (and any pending
// disposal runs) after the adapter is done, collecting every disposer that
// must still fire. It must be called with postDoneMu held; the returned
// disposers must be invoked with runPostDoneDisposers only after the lock is
// released (see finishOwnerDisposalRunPostDoneLocked).
func (d *ownerDispatcher) discardOwnerRootsPostDoneLocked(
	roots []supervisorRoot,
	err error,
) ([]ownerDisposerCall, <-chan struct{}) {
	var disposers []ownerDisposerCall
	for _, root := range roots {
		runDisposers, ok := d.finishOwnerDisposalRunPostDoneLocked(
			ownerDisposalID(root.id),
		)
		if ok {
			disposers = append(disposers, runDisposers...)
			continue
		}
		disposal := d.prepareOwnerRootDisposal(root.id, root.preparing)
		if disposal.root != nil {
			clear(disposal.root.promises)
			clear(disposal.root.callbacks)
			for _, disposer := range disposal.root.disposers {
				if disposer != nil {
					disposers = append(disposers, ownerDisposerCall{
						disposer: disposer,
						err:      err,
					})
				}
			}
			clear(disposal.root.disposers)
		}
		var fenceDone <-chan struct{}
		if value, ok := d.bridge.fences.Load(root.id); ok {
			fenceDone = value.(*ownerRootFence).done
		}
		d.finishRootClosePostDone(root.id)
		if fenceDone != nil {
			<-fenceDone
		}
	}
	done := make(chan struct{})
	close(done)
	return disposers, done
}

func (d *ownerDispatcher) disposeOwnerRootOwner(
	id supervisorChildID,
	err error,
) <-chan struct{} {
	if id == 0 {
		done := make(chan struct{})
		close(done)
		return done
	}
	return d.beginOwnerDisposal(
		[]supervisorRoot{{id: id}},
		err,
	)
}

func (d *ownerDispatcher) disposeOwnerRootsOwner(
	roots []supervisorRoot,
	err error,
) <-chan struct{} {
	return d.beginOwnerDisposal(roots, err)
}

// disposeOwnerRootWorker submits preparation to the owner, waits for every
// restartable owner step, or enters the single post-Done transfer path.
func (d *ownerDispatcher) disposeOwnerRootWorker(
	id supervisorChildID,
	err error,
) {
	if id == 0 {
		return
	}
	d.disposeOwnerRootsWorker([]supervisorRoot{{id: id}}, err)
}

func (d *ownerDispatcher) disposeOwnerRootsWorker(
	roots []supervisorRoot,
	err error,
) {
	if len(roots) == 0 {
		return
	}
	runReady := make(chan (<-chan struct{}), 1)
	submitErr := d.submit(func() {
		runReady <- d.beginOwnerDisposal(roots, err)
	})
	if submitErr == nil {
		select {
		case done := <-runReady:
			select {
			case <-done:
			case <-d.adapter.Done():
				for _, root := range roots {
					d.finishOwnerDisposalPostDone(
						ownerDisposalID(root.id),
					)
				}
				<-done
			}
			return
		case <-d.adapter.Done():
			select {
			case done := <-runReady:
				for _, root := range roots {
					d.finishOwnerDisposalPostDone(
						ownerDisposalID(root.id),
					)
				}
				<-done
				return
			default:
			}
		}
	}
	<-d.adapter.Done()
	d.bridge.postDoneMu.Lock()
	d.bridge.transferred.Store(true)
	disposers, done := d.discardOwnerRootsPostDoneLocked(roots, err)
	d.bridge.postDoneMu.Unlock()
	runPostDoneDisposers(disposers)
	<-done
}
