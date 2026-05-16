package eventloop

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestChainedPromisePassThroughPublishesPropagationBeforeRejection(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	reported := make(chan any, 2)
	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) { reported <- reason }))
	if err != nil {
		t.Fatal(err)
	}
	parent, _, rejectParent := js.NewChainedPromise()
	child := parent.Then(nil, nil)
	rejectionRecorded := make(chan struct{})
	releasePublication := make(chan struct{})
	releaseRecord := releaseSignalT(t, releasePublication)
	checkerAtParent := make(chan *ChainedPromise, 1)
	releaseChecker := make(chan struct{})
	releaseCheck := releaseSignalT(t, releaseChecker)
	var recordOnce sync.Once
	loop.testHooks = &loopTestHooks{
		AfterPromiseRejectionRecorded: func() {
			recordOnce.Do(func() {
				close(rejectionRecorded)
				<-releasePublication
			})
		},
		BeforeUnhandledRejectionRecordCheck: func(promise *ChainedPromise) {
			select {
			case checkerAtParent <- promise:
			default:
			}
			if promise == parent {
				<-releaseChecker
			}
		},
	}
	rejectDone := make(chan struct{})
	go func() {
		rejectParent("propagated")
		close(rejectDone)
	}()
	select {
	case <-rejectionRecorded:
	case <-time.After(5 * time.Second):
		t.Fatal("parent rejection was not recorded")
	}
	js.rejectionsMu.RLock()
	parentInfo := js.unhandledRejections[parent]
	js.rejectionsMu.RUnlock()
	if parentInfo == nil {
		t.Fatal("parent rejection record disappeared before the checker snapshot")
	}
	checkerDone := make(chan struct{})
	go func() {
		js.checkUnhandledRejections()
		close(checkerDone)
	}()
	select {
	case checked := <-checkerAtParent:
		if checked != parent {
			t.Fatalf("checker evaluated %p, want parent %p", checked, parent)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("active checker did not snapshot the parent")
	}
	releaseRecord()
	select {
	case <-rejectDone:
	case <-time.After(5 * time.Second):
		t.Fatal("parent rejection did not publish")
	}
	releaseCheck()
	select {
	case <-checkerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("active rejection checker did not return")
	}
	select {
	case reason := <-reported:
		t.Fatalf("pass-through parent was reported before propagation: %#v", reason)
	default:
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case reason := <-reported:
		if reason != "propagated" {
			t.Fatalf("reported reason = %#v, want propagated", reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("propagated child rejection was not reported")
	}
	if child.State() != Rejected {
		t.Fatalf("child state = %v, want Rejected", child.State())
	}
	select {
	case reason := <-reported:
		t.Fatalf("duplicate unhandled rejection report: %#v", reason)
	default:
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "published propagation Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestChainedPromiseLatePassThroughLinearizesWithActiveChecker(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	reported := make(chan any, 2)
	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) { reported <- reason }))
	if err != nil {
		t.Fatal(err)
	}
	parent, _, rejectParent := js.NewChainedPromise()
	rejectParent("late propagation")
	checkerAtParent := make(chan struct{})
	releaseChecker := make(chan struct{})
	release := releaseSignalT(t, releaseChecker)
	loop.testHooks = &loopTestHooks{
		BeforeUnhandledRejectionRecordCheck: func(promise *ChainedPromise) {
			if promise != parent {
				return
			}
			select {
			case <-checkerAtParent:
			default:
				close(checkerAtParent)
			}
			<-releaseChecker
		},
	}
	checkerDone := make(chan struct{})
	go func() {
		js.checkUnhandledRejections()
		close(checkerDone)
	}()
	select {
	case <-checkerAtParent:
	case <-time.After(5 * time.Second):
		t.Fatal("active checker did not reach the rejected parent")
	}
	child := parent.Then(nil, nil)
	release()
	select {
	case <-checkerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("active checker did not finish")
	}
	select {
	case reason := <-reported:
		t.Fatalf("parent was reported despite linearized pass-through: %#v", reason)
	default:
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case reason := <-reported:
		if reason != "late propagation" {
			t.Fatalf("reported reason = %#v, want late propagation", reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("late pass-through child was not reported")
	}
	if child.State() != Rejected {
		t.Fatalf("child state = %v, want Rejected", child.State())
	}
	select {
	case reason := <-reported:
		t.Fatalf("duplicate parent/child report: %#v", reason)
	default:
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "active-checker propagation Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestChainedPromiseLatePassThroughCheckerWinSuppressesChildReport(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	reported := make(chan any, 2)
	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) { reported <- reason }))
	if err != nil {
		t.Fatal(err)
	}
	parent, _, rejectParent := js.NewChainedPromise()
	rejectParent("checker owns")
	checkerClaimed := make(chan struct{})
	releaseCallback := make(chan struct{})
	release := releaseSignalT(t, releaseCallback)
	loop.testHooks = &loopTestHooks{
		BeforeUnhandledRejectionCallback: func() {
			close(checkerClaimed)
			<-releaseCallback
		},
	}
	checkerDone := make(chan struct{})
	go func() {
		js.checkUnhandledRejections()
		close(checkerDone)
	}()
	select {
	case <-checkerClaimed:
	case <-time.After(5 * time.Second):
		t.Fatal("checker did not claim the parent report")
	}
	child := parent.Then(nil, nil)
	grandchild := child.Then(nil, nil)
	release()
	select {
	case <-checkerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("checker did not finish parent reporting")
	}
	select {
	case reason := <-reported:
		if reason != "checker owns" {
			t.Fatalf("parent reason = %#v, want checker owns", reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("checker-owned parent was not reported")
	}

	grandchildDone := grandchild.ToChannel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	if value, open := waitContractReceive(t, grandchildDone, "checker-owned pass-through chain settlement"); !open || value != "checker owns" {
		t.Fatalf("grandchild settlement = (%#v, open=%t), want (checker owns, true)", value, open)
	}
	if child.State() != Rejected {
		t.Fatalf("child state = %v, want Rejected", child.State())
	}
	if grandchild.State() != Rejected {
		t.Fatalf("grandchild state = %v, want Rejected", grandchild.State())
	}
	select {
	case reason := <-reported:
		t.Fatalf("checker-owned propagation reported child too: %#v", reason)
	default:
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "checker-owned propagation Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestChainedPromiseLatePassThroughAfterHandledCleanupReportsChild(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	reported := make(chan any, 1)
	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) { reported <- reason }))
	if err != nil {
		t.Fatal(err)
	}
	parent, _, rejectParent := js.NewChainedPromise()
	handled := make(chan struct{}, 1)
	parent.Catch(func(any) any {
		handled <- struct{}{}
		return nil
	})
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	rejectParent("handled parent")
	select {
	case <-handled:
	case <-time.After(5 * time.Second):
		t.Fatal("parent rejection handler did not run")
	}
	waitLoopOwnerTurnT(t, loop)
	waitUnhandledRejectionCheckOwnershipReleased(t, js)
	js.rejectionsMu.RLock()
	_, parentTracked := js.unhandledRejections[parent]
	js.rejectionsMu.RUnlock()
	if parentTracked {
		t.Fatal("handled parent rejection record remained after checker ownership released")
	}

	child := parent.Then(nil, nil)
	select {
	case reason := <-reported:
		if reason != "handled parent" {
			t.Fatalf("child reason = %#v, want handled parent", reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("late pass-through child was incorrectly suppressed")
	}
	if child.State() != Rejected {
		t.Fatalf("child state = %v, want Rejected", child.State())
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "handled-parent propagation Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestChainedPromiseTerminalPassThroughSettlesChildOnce(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	reported := make(chan any, 2)
	js, err := NewJS(loop,
		WithUnhandledRejection(func(reason any) { reported <- reason }),
		WithUnhandledRejectionFallback(UnhandledRejectionFallbackIsolated),
	)
	if err != nil {
		t.Fatal(err)
	}
	parent, _, rejectParent := js.NewChainedPromise()
	child := parent.Then(nil, nil)
	childResult := child.ToChannel()
	rejectParent("discarded propagation")
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case reason := <-reported:
		if reason != "discarded propagation" {
			t.Fatalf("reported reason = %#v, want discarded propagation", reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("discarded propagation suppressed the parent diagnostic")
	}
	if child.State() != Rejected || child.Reason() != "discarded propagation" {
		t.Fatalf("terminal pass-through child = (%v, %#v), want (Rejected, discarded propagation)", child.State(), child.Reason())
	}
	assertSinglePromiseChannelValue(t, childResult, "discarded propagation")
	select {
	case reason := <-reported:
		t.Fatalf("discarded propagation produced duplicate report: %#v", reason)
	default:
	}
}
