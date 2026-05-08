package eventloop

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestCloseStateTerminatingSkipsCurrentPhaseRemainder(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	outerStarted := make(chan struct{})
	releaseOuter := make(chan struct{})
	releaseOuterFn := releaseSignalT(t, releaseOuter)
	if err := loop.Submit(func() {
		close(outerStarted)
		<-releaseOuter
	}); err != nil {
		t.Fatalf("Submit blocking callback: %v", err)
	}
	discardedRan := make(chan struct{}, 1)
	if err := loop.Submit(func() { discardedRan <- struct{}{} }); err != nil {
		t.Fatalf("Submit discarded callback: %v", err)
	}

	closeTransitioned := make(chan struct{})
	releaseClose := make(chan struct{})
	releaseCloseFn := releaseSignalT(t, releaseClose)
	loop.testHooks = &loopTestHooks{
		AfterCloseStateTerminating: func() {
			close(closeTransitioned)
			<-releaseClose
		},
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case <-outerStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("blocking callback did not start")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	waitContractSignal(t, closeTransitioned, "Close StateTerminating publication")
	releaseOuterFn()
	releaseCloseFn()

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not complete")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not complete")
	}
	select {
	case <-discardedRan:
		t.Fatal("callback ran after immediate Close published StateTerminating")
	default:
	}
}

func TestCloseSkipsMicrotaskPausedDuringIngressTransfer(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	transferStarted := make(chan struct{})
	releaseTransfer := make(chan struct{})
	releaseTransferFn := releaseSignalT(t, releaseTransfer)
	closeTerminated := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterCommandIngressPopBeforeApply: func(kind loopCommandKind) {
			if kind != loopCommandMicrotask {
				return
			}
			close(transferStarted)
			<-releaseTransfer
		},
		BeforeClosePromiseRejection: func() { close(closeTerminated) },
	}
	microtaskRan := make(chan struct{}, 1)
	if err := loop.ScheduleMicrotask(func() { microtaskRan <- struct{}{} }); err != nil {
		t.Fatalf("ScheduleMicrotask: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case <-transferStarted:
	case <-time.After(5 * time.Second):
		releaseTransferFn()
		t.Fatal("microtask ingress transfer did not start")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	waitContractSignal(t, closeTerminated, "Close terminal-state publication")
	releaseTransferFn()

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not complete after ingress transfer release")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not complete after Close")
	}
	select {
	case <-microtaskRan:
		t.Fatal("microtask ran after immediate Close won during ingress transfer")
	default:
	}
}

func TestCloseSkipsMicrotaskPausedBeforeCallbackAdmission(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	callbackReached := make(chan struct{})
	releaseCallback := make(chan struct{})
	releaseCallbackFn := releaseSignalT(t, releaseCallback)
	closeTransitioned := make(chan struct{})
	var hookOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeCallbackAdmission: func() {
			hookOnce.Do(func() { close(callbackReached) })
			<-releaseCallback
		},
		AfterCloseStateTerminating: func() { close(closeTransitioned) },
	}
	microtaskRan := make(chan struct{}, 1)
	if err := loop.ScheduleMicrotask(func() { microtaskRan <- struct{}{} }); err != nil {
		t.Fatalf("ScheduleMicrotask: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case <-callbackReached:
	case <-time.After(5 * time.Second):
		releaseCallbackFn()
		t.Fatal("microtask did not reach callback admission")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	waitContractSignal(t, closeTransitioned, "Close StateTerminating publication")
	releaseCallbackFn()

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not complete after callback admission release")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not complete after Close")
	}
	select {
	case <-microtaskRan:
		t.Fatal("microtask ran after Close closed callback admission")
	default:
	}
}

func TestCloseClaimedCallbackCannotClaimNestedCallbackAfterAdmissionCloses(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	outerStarted := make(chan struct{})
	releaseNestedClaim := make(chan struct{})
	releaseNestedClaimFn := releaseSignalT(t, releaseNestedClaim)
	closeTransitioned := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterCloseStateTerminating: func() { close(closeTransitioned) },
	}
	nestedRan := make(chan struct{}, 1)
	if err := loop.Submit(func() {
		close(outerStarted)
		<-releaseNestedClaim
		loop.safeExecuteFn(func() { nestedRan <- struct{}{} })
	}); err != nil {
		t.Fatalf("Submit outer callback: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case <-outerStarted:
	case <-time.After(5 * time.Second):
		releaseNestedClaimFn()
		t.Fatal("outer callback did not start")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	waitContractSignal(t, closeTransitioned, "Close StateTerminating publication")
	releaseNestedClaimFn()

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not complete after the claimed callback finished")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not complete after Close")
	}
	select {
	case <-nestedRan:
		t.Fatal("already-claimed callback admitted a new nested callback after Close closed admission")
	default:
	}
}

func TestCloseSkipsCheckPredicatePausedBeforeAdmission(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)

	predicateStarted := make(chan struct{})
	closeReturned := make(chan struct{})
	releasePredicate := make(chan struct{})
	var releasePredicateOnce sync.Once
	releasePredicateFn := func() { releasePredicateOnce.Do(func() { close(releasePredicate) }) }
	t.Cleanup(releasePredicateFn)
	predicateAdmissionReached := make(chan struct{})
	closeTerminated := make(chan struct{})
	releaseAdmission := make(chan struct{})
	var releaseAdmissionOnce sync.Once
	var predicateAdmissionOnce sync.Once
	releaseAdmissionFn := func() { releaseAdmissionOnce.Do(func() { close(releaseAdmission) }) }
	t.Cleanup(releaseAdmissionFn)
	loop.testHooks = &loopTestHooks{
		BeforeCheckPredicateAdmission: func() {
			predicateAdmissionOnce.Do(func() {
				close(predicateAdmissionReached)
				<-releaseAdmission
			})
		},
		BeforeClosePromiseRejection: func() { close(closeTerminated) },
	}
	if err := loop.ScheduleImmediateRef(func() {}, func() bool {
		close(predicateStarted)
		select {
		case <-closeReturned:
		case <-releasePredicate:
		}
		return true
	}); err != nil {
		t.Fatalf("ScheduleImmediateRef: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case <-predicateAdmissionReached:
	case <-time.After(5 * time.Second):
		t.Fatal("dynamic check predicate did not reach callback admission")
	}

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- loop.Close()
		close(closeReturned)
	}()
	waitContractSignal(t, closeTerminated, "Close terminal-state publication")
	releaseAdmissionFn()

	select {
	case <-predicateStarted:
		releasePredicateFn()
		select {
		case <-closeDone:
		case <-time.After(5 * time.Second):
			t.Fatal("Close remained blocked after releasing the incorrectly admitted predicate")
		}
		t.Fatal("dynamic check predicate started after Close closed callback admission")
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not complete after rejecting predicate admission")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not exit after Close")
	}
	select {
	case <-predicateStarted:
		t.Fatal("dynamic check predicate ran after Close returned")
	default:
	}
}
