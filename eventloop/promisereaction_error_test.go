package eventloop

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func waitTerminalUnhandledRejectionTrackingDrained(t *testing.T, js *JS) {
	t.Helper()
	if state := js.loop.state.Load(); state != StateTerminated {
		t.Fatalf("unhandled-rejection terminal drain requires a terminated loop, got %v", state)
	}
	waitUnhandledRejectionCheckOwnershipReleased(t, js)
	assertUnhandledRejectionTrackingDrained(t, js)
}

func waitUnhandledRejectionCheckOwnershipReleased(t *testing.T, js *JS) {
	t.Helper()
	for {
		js.checkRejectionRunMu.Lock()
		done := js.checkRejectionRunDone
		js.checkRejectionRunMu.Unlock()
		if done == nil {
			break
		}
		waitContractSignal(t, done, "active unhandled-rejection check completion")
	}
}

func assertUnhandledRejectionTrackingDrained(t *testing.T, js *JS) {
	t.Helper()
	js.checkRejectionRunMu.Lock()
	defer js.checkRejectionRunMu.Unlock()
	js.rejectionsMu.RLock()
	remaining := len(js.unhandledRejections)
	js.rejectionsMu.RUnlock()
	js.handlerReadyMu.Lock()
	readyRemaining := len(js.handlerReadyChans)
	js.handlerReadyMu.Unlock()
	js.loop.rejectionCheckMu.Lock()
	_, retainedOverflow := js.loop.rejectionCheckAdapters[js]
	retained := js.loop.rejectionCheckAdapter == js || retainedOverflow
	js.loop.rejectionCheckMu.Unlock()
	if js.checkRejectionScheduled.Load() ||
		js.checkRejectionRunning.Load() ||
		js.checkRejectionRerun.Load() ||
		js.checkRejectionFallbackRerun.Load() ||
		remaining != 0 ||
		readyRemaining != 0 ||
		retained {
		t.Fatalf("unhandled rejection tracking did not drain after its completion barrier: scheduled=%v running=%v rerun=%v fallbackRerun=%v unhandled=%d handlerReady=%d retained=%v",
			js.checkRejectionScheduled.Load(),
			js.checkRejectionRunning.Load(),
			js.checkRejectionRerun.Load(),
			js.checkRejectionFallbackRerun.Load(),
			remaining,
			readyRemaining,
			retained,
		)
	}
}

func TestChainedPromise_HandlerScheduleErrorRejectsChild(t *testing.T) {
	loop := New()

	js := NewJS(loop)

	parent := js.Resolve("ready")
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	child := parent.Then(func(any) any {
		t.Fatal("handler should not run after ScheduleMicrotask fails")
		return nil
	}, nil)

	if child.State() != Rejected {
		t.Fatalf("child state = %v, want Rejected", child.State())
	}
	reason, ok := child.Reason().(error)
	if !ok {
		t.Fatalf("child reason type = %T, want error", child.Reason())
	}
	if !errors.Is(reason, ErrLoopTerminated) {
		t.Fatalf("child reason = %v, want ErrLoopTerminated", reason)
	}
}

func TestChainedPromise_PassThroughScheduleErrorPreservesFulfillment(t *testing.T) {
	loop := New()

	js := NewJS(loop)

	parent := js.Resolve("ready")
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	child := parent.Then(nil, nil)
	if child.State() != Fulfilled {
		t.Fatalf("child state = %v, want Fulfilled", child.State())
	}
	if child.Value() != "ready" {
		t.Fatalf("child value = %v, want ready", child.Value())
	}
}

func TestChainedPromise_PassThroughScheduleErrorPreservesRejection(t *testing.T) {
	loop := New()

	js := NewJS(loop, WithUnhandledRejection(func(any) {}))

	parent := js.Reject("reason")
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	child := parent.Then(nil, nil)
	if child.State() != Rejected {
		t.Fatalf("child state = %v, want Rejected", child.State())
	}
	if child.Reason() != "reason" {
		t.Fatalf("child reason = %v, want reason", child.Reason())
	}
}

func TestChainedPromise_AdoptionScheduleErrorPreservesFulfillment(t *testing.T) {
	loop := New()

	js := NewJS(loop)

	source := js.Resolve("adopted value")
	adopter, resolveAdopter, _ := js.NewChainedPromise()
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	resolveAdopter(source)
	if adopter.State() != Fulfilled {
		t.Fatalf("adopter state = %v, want Fulfilled", adopter.State())
	}
	if adopter.Value() != "adopted value" {
		t.Fatalf("adopter value = %v, want adopted value", adopter.Value())
	}
}

func TestChainedPromise_AdoptionScheduleErrorPreservesRejection(t *testing.T) {
	loop := New()

	js := NewJS(loop, WithUnhandledRejection(func(any) {}))

	source := js.Reject("adopted reason")
	adopter, resolveAdopter, _ := js.NewChainedPromise()
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	resolveAdopter(source)
	if adopter.State() != Rejected {
		t.Fatalf("adopter state = %v, want Rejected", adopter.State())
	}
	if adopter.Reason() != "adopted reason" {
		t.Fatalf("adopter reason = %v, want adopted reason", adopter.Reason())
	}
}

func TestChainedPromise_PendingAdoptionScheduleErrorPreservesSourceSettlement(t *testing.T) {
	t.Run("fulfillment", func(t *testing.T) {
		loop := New()

		js := NewJS(loop)

		source, resolveSource, _ := js.NewChainedPromise()
		adopter, resolveAdopter, _ := js.NewChainedPromise()
		resolveAdopter(source)
		if adopter.State() != Pending {
			t.Fatalf("adopter state before source settlement = %v, want Pending", adopter.State())
		}
		if err := loop.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}

		resolveSource("late value")
		if adopter.State() != Fulfilled {
			t.Fatalf("adopter state = %v, want Fulfilled", adopter.State())
		}
		if adopter.Value() != "late value" {
			t.Fatalf("adopter value = %v, want late value", adopter.Value())
		}
	})

	t.Run("rejection", func(t *testing.T) {
		loop := New()

		js := NewJS(loop, WithUnhandledRejection(func(any) {}))

		source, _, rejectSource := js.NewChainedPromise()
		adopter, resolveAdopter, _ := js.NewChainedPromise()
		resolveAdopter(source)
		if adopter.State() != Pending {
			t.Fatalf("adopter state before source settlement = %v, want Pending", adopter.State())
		}
		if err := loop.Shutdown(context.Background()); err != nil {
			t.Fatal(err)
		}

		rejectSource("late reason")
		if adopter.State() != Rejected {
			t.Fatalf("adopter state = %v, want Rejected", adopter.State())
		}
		if adopter.Reason() != "late reason" {
			t.Fatalf("adopter reason = %v, want late reason", adopter.Reason())
		}
	})
}

func TestChainedPromise_HandlerScheduleErrorRejectsChildAfterParentUnlock(t *testing.T) {
	loop := New()

	var parent *ChainedPromise
	var reentered atomic.Bool
	reenteredParent := make(chan struct{}, 1)
	js := NewJS(loop,
		WithUnhandledRejection(func(any) {
			if reentered.CompareAndSwap(false, true) {
				parent.Then(func(any) any { return nil }, nil)
				reenteredParent <- struct{}{}
			}
		}),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
	)

	var resolve ResolveFunc
	parent, resolve, _ = js.NewChainedPromise()
	child := parent.Then(func(any) any {
		t.Fatal("handler should not run after ScheduleMicrotask fails")
		return nil
	}, nil)

	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	resolved := make(chan struct{})
	go func() {
		resolve("ready")
		close(resolved)
	}()

	select {
	case <-resolved:
	case <-time.After(time.Second):
		t.Fatal("resolve deadlocked while reporting child handler scheduling failure")
	}
	select {
	case <-reenteredParent:
	case <-time.After(time.Second):
		t.Fatal("unhandled rejection callback did not re-enter parent promise")
	}
	if parent.State() != Fulfilled {
		t.Fatalf("parent state = %v, want Fulfilled", parent.State())
	}
	if child.State() != Rejected {
		t.Fatalf("child state = %v, want Rejected", child.State())
	}
}

func TestChainedPromise_HandlerScheduleErrorRegistersCatchBeforeChildFallback(t *testing.T) {
	reported := make(chan any, 3)
	loop := New()

	js := NewJS(loop, WithUnhandledRejection(func(reason any) {
		reported <- reason
	}))

	parent := &ChainedPromise{js: js, result: "parent"}
	parent.state.Store(int32(Rejected))
	js.recordRejection(parent, "parent")
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	catchRan := make(chan struct{}, 1)
	child := parent.Catch(func(any) any {
		select {
		case catchRan <- struct{}{}:
		default:
		}
		return nil
	})

	if parent.State() != Rejected {
		t.Fatalf("parent state = %v, want Rejected", parent.State())
	}
	if child.State() != Rejected {
		t.Fatalf("child state = %v, want Rejected", child.State())
	}
	select {
	case <-catchRan:
		t.Fatal("catch handler ran after ScheduleMicrotask failed")
	default:
	}

	for {
		select {
		case reason := <-reported:
			if reason == "parent" {
				t.Fatal("parent rejection was reported even though Catch was attached before child fallback")
			}
		default:
			return
		}
	}
}

func TestChainedPromise_ThenWithoutRejectedHandlesParentReportsChild(t *testing.T) {
	reported := make(chan any, 3)
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop, WithUnhandledRejection(func(reason any) {
		reported <- reason
	}))

	parent := js.Reject("pass-through")
	var fulfilledCalled atomic.Bool
	child := parent.Then(func(any) any {
		fulfilledCalled.Store(true)
		return nil
	}, nil)

	loop.tick()

	if fulfilledCalled.Load() {
		t.Fatal("fulfillment handler ran for rejected parent")
	}
	if parent.State() != Rejected {
		t.Fatalf("parent state = %v, want Rejected", parent.State())
	}
	if child.State() != Rejected {
		t.Fatalf("child state = %v, want Rejected", child.State())
	}
	if child.Reason() != "pass-through" {
		t.Fatalf("child reason = %v, want pass-through", child.Reason())
	}

	var reports []any
	for {
		select {
		case reason := <-reported:
			reports = append(reports, reason)
		default:
			if len(reports) != 1 || reports[0] != "pass-through" {
				t.Fatalf("unhandled reports = %#v, want exactly the pass-through child rejection", reports)
			}
			return
		}
	}
}

func TestChainedPromise_CatchHandlesParentAndChildStaysQuiet(t *testing.T) {
	reported := make(chan any, 3)
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop, WithUnhandledRejection(func(reason any) {
		reported <- reason
	}))

	parent := js.Reject("handled")
	child := parent.Catch(func(reason any) any {
		if reason != "handled" {
			t.Fatalf("catch reason = %v, want handled", reason)
		}
		return "recovered"
	})

	loop.tick()

	if parent.State() != Rejected {
		t.Fatalf("parent state = %v, want Rejected", parent.State())
	}
	if child.State() != Fulfilled {
		t.Fatalf("child state = %v, want Fulfilled", child.State())
	}
	if child.result != "recovered" {
		t.Fatalf("child result = %v, want recovered", child.result)
	}
	select {
	case reason := <-reported:
		t.Fatalf("handled rejection was unexpectedly reported: %v", reason)
	default:
	}
}

func TestChainedPromise_AdoptionHandlesSourceReportsAdopter(t *testing.T) {
	reported := make(chan any, 3)
	loop := New()
	registerLoopCleanupT(t, loop)

	js := NewJS(loop, WithUnhandledRejection(func(reason any) {
		reported <- reason
	}))

	source := js.Reject("adopted")
	adopter, resolveAdopter, _ := js.NewChainedPromise()
	resolveAdopter(source)

	loop.tick()

	if source.State() != Rejected {
		t.Fatalf("source state = %v, want Rejected", source.State())
	}
	if adopter.State() != Rejected {
		t.Fatalf("adopter state = %v, want Rejected", adopter.State())
	}
	if adopter.Reason() != "adopted" {
		t.Fatalf("adopter reason = %v, want adopted", adopter.Reason())
	}

	var reports []any
	for {
		select {
		case reason := <-reported:
			reports = append(reports, reason)
		default:
			if len(reports) != 1 || reports[0] != "adopted" {
				t.Fatalf("unhandled reports = %#v, want exactly the adopter rejection", reports)
			}
			return
		}
	}
}

func TestChainedPromise_PendingCatchRegistersBeforeConcurrentRejectReport(t *testing.T) {
	reported := make(chan any, 4)
	loop := New()

	var hookSeen atomic.Bool
	var rejectHookSeen atomic.Bool
	handlerStored := make(chan struct{})
	releaseRegister := make(chan struct{})
	rejectReachedLock := make(chan struct{})
	releaseRejectLock := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		BeforePromiseHandlerRegister: func() {
			if hookSeen.CompareAndSwap(false, true) {
				close(handlerStored)
				<-releaseRegister
			}
		},
		BeforePromiseRejectLock: func() {
			if rejectHookSeen.CompareAndSwap(false, true) {
				close(rejectReachedLock)
				<-releaseRejectLock
			}
		},
	}

	js := NewJS(loop, WithUnhandledRejection(func(reason any) {
		reported <- reason
	}))

	parent, _, rejectParent := js.NewChainedPromise()
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	catchRan := make(chan struct{}, 1)
	catchDone := make(chan *ChainedPromise, 1)
	go func() {
		catchDone <- parent.Catch(func(any) any {
			select {
			case catchRan <- struct{}{}:
			default:
			}
			return nil
		})
	}()

	select {
	case <-handlerStored:
	case <-time.After(time.Second):
		t.Fatal("pending catch did not reach handler-registration hook")
	}

	rejectDone := make(chan struct{})
	const parentReason = "pending parent rejection"
	go func() {
		rejectParent(parentReason)
		close(rejectDone)
	}()

	select {
	case <-rejectReachedLock:
	case <-time.After(time.Second):
		t.Fatal("reject did not reach the promise lock while pending Catch registration was blocked")
	}
	close(releaseRejectLock)

	close(releaseRegister)

	var child *ChainedPromise
	select {
	case child = <-catchDone:
	case <-time.After(time.Second):
		t.Fatal("pending catch did not return after handler registration was released")
	}
	select {
	case <-rejectDone:
	case <-time.After(time.Second):
		t.Fatal("reject did not complete after pending catch registered the handler")
	}

	if parent.State() != Rejected {
		t.Fatalf("parent state = %v, want Rejected", parent.State())
	}
	if child.State() != Rejected {
		t.Fatalf("child state = %v, want Rejected", child.State())
	}
	if reason, ok := child.Reason().(error); !ok || !errors.Is(reason, ErrLoopTerminated) {
		t.Fatalf("child reason = %v, want ErrLoopTerminated", child.Reason())
	}
	select {
	case <-catchRan:
		t.Fatal("catch handler ran after ScheduleMicrotask failed")
	default:
	}

	for {
		select {
		case reason := <-reported:
			if reason == parentReason {
				t.Fatal("parent rejection was reported before pending Catch registration completed")
			}
		default:
			return
		}
	}
}

func TestChainedPromise_RejectedStateLateCatchSeesRecordedRejection(t *testing.T) {
	reported := make(chan any, 4)
	loop := New()

	var js *JS
	var hookSeen atomic.Bool
	var pendingCheckRan atomic.Bool
	statePublished := make(chan struct{})
	releaseReject := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterPromiseRejectionRecorded: func() {
			if pendingCheckRan.CompareAndSwap(false, true) {
				js.checkUnhandledRejections()
			}
		},
		AfterPromiseRejectedStateStore: func() {
			if hookSeen.CompareAndSwap(false, true) {
				close(statePublished)
				<-releaseReject
			}
		},
	}

	js = NewJS(loop, WithUnhandledRejection(func(reason any) {
		reported <- reason
	}))

	parent, _, rejectParent := js.NewChainedPromise()
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}

	const parentReason = "recorded before rejected state"
	rejectDone := make(chan struct{})
	go func() {
		rejectParent(parentReason)
		close(rejectDone)
	}()

	select {
	case <-statePublished:
	case <-time.After(time.Second):
		t.Fatal("reject did not publish Rejected state before diagnostic scheduling")
	}
	if !pendingCheckRan.Load() {
		t.Fatal("test did not run unhandled check while rejected promise was still Pending")
	}

	child := parent.Catch(func(any) any {
		t.Fatal("catch handler should not run after ScheduleMicrotask fails")
		return nil
	})
	if child.State() != Rejected {
		t.Fatalf("child state = %v, want Rejected", child.State())
	}
	if reason, ok := child.Reason().(error); !ok || !errors.Is(reason, ErrLoopTerminated) {
		t.Fatalf("child reason = %v, want ErrLoopTerminated", child.Reason())
	}

	close(releaseReject)
	select {
	case <-rejectDone:
	case <-time.After(time.Second):
		t.Fatal("reject did not complete after late Catch registration")
	}

	for {
		select {
		case reason := <-reported:
			if reason == parentReason {
				t.Fatal("parent rejection was reported even though late Catch observed the recorded rejection")
			}
		default:
			return
		}
	}
}
