package eventloop

import (
	"context"
	"fmt"
	"testing"
)

type promiseReactionObservation struct {
	handler string
	branch  string
	value   any
}

func TestChainedPromiseConcurrentRegistrationOrder(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		for _, registrationFirst := range []bool{false, true} {
			name := fmt.Sprintf("rejected=%t/registration-first=%t", rejected, registrationFirst)
			t.Run(name, func(t *testing.T) {
				testChainedPromiseConcurrentRegistrationOrder(t, rejected, registrationFirst)
			})
		}
	}
}

func testChainedPromiseConcurrentRegistrationOrder(t *testing.T, rejected, registrationFirst bool) {
	loop := New()
	registerLoopCleanupT(t, loop)
	js := NewJS(loop)

	promise, resolve, reject := js.NewChainedPromise()
	observed := make(chan promiseReactionObservation, 2)
	handler := func(label string, fulfilled bool) func(any) any {
		return func(value any) any {
			branch := "rejected"
			if fulfilled {
				branch = "fulfilled"
			}
			observed <- promiseReactionObservation{handler: label, branch: branch, value: value}
			return label + "-child"
		}
	}
	preexisting := promise.Then(handler("preexisting", true), handler("preexisting", false))

	contenderDone := make(chan *ChainedPromise, 1)
	settlementDone := make(chan struct{})
	settle := func() {
		if rejected {
			reject("parent-reason")
		} else {
			resolve("parent-value")
		}
	}
	attachContender := func() {
		contenderDone <- promise.Then(handler("contender", true), handler("contender", false))
	}

	if registrationFirst {
		registered := make(chan struct{})
		releaseRegistration := make(chan struct{})
		release := releaseSignalT(t, releaseRegistration)
		loop.testHooks = &loopTestHooks{
			BeforePromiseHandlerRegister: func() {
				close(registered)
				<-releaseRegistration
			},
		}
		go attachContender()
		waitContractSignal(t, registered, "stored contender reaction")
		settlementStarted := make(chan struct{})
		go func() {
			close(settlementStarted)
			settle()
			close(settlementDone)
		}()
		waitContractSignal(t, settlementStarted, "concurrent promise settlement start")
		release()
	} else {
		pendingChecked := make(chan struct{})
		releasePendingCheck := make(chan struct{})
		release := releaseSignalT(t, releasePendingCheck)
		loop.testHooks = &loopTestHooks{
			AfterPromiseHandlerPendingCheck: func() {
				close(pendingChecked)
				<-releasePendingCheck
			},
		}
		go attachContender()
		waitContractSignal(t, pendingChecked, "contender optimistic pending check")
		settle()
		close(settlementDone)
		release()
	}

	contender := waitContractValue(t, contenderDone, "contender reaction registration")
	waitContractSignal(t, settlementDone, "promise settlement")
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	first := waitContractValue(t, observed, "preexisting reaction")
	second := waitContractValue(t, observed, "contender reaction")
	wantBranch := "fulfilled"
	wantParent := any("parent-value")
	if rejected {
		wantBranch = "rejected"
		wantParent = "parent-reason"
	}
	if first != (promiseReactionObservation{handler: "preexisting", branch: wantBranch, value: wantParent}) ||
		second != (promiseReactionObservation{handler: "contender", branch: wantBranch, value: wantParent}) {
		t.Fatalf("reaction order = [%+v %+v], want preexisting then contender on %s", first, second, wantBranch)
	}
	if rejected {
		if promise.State() != Rejected || promise.Reason() != wantParent {
			t.Fatalf("parent = (%v, %#v), want Rejected %#v", promise.State(), promise.Reason(), wantParent)
		}
	} else if promise.State() != Fulfilled || promise.Value() != wantParent {
		t.Fatalf("parent = (%v, %#v), want Fulfilled %#v", promise.State(), promise.Value(), wantParent)
	}
	if got := waitContractValue(t, preexisting.ToChannel(), "preexisting child settlement"); got != "preexisting-child" {
		t.Fatalf("preexisting child = %#v, want preexisting-child", got)
	}
	if got := waitContractValue(t, contender.ToChannel(), "contender child settlement"); got != "contender-child" {
		t.Fatalf("contender child = %#v, want contender-child", got)
	}
	if preexisting.State() != Fulfilled || contender.State() != Fulfilled {
		t.Fatalf("child states = (%v, %v), want Fulfilled", preexisting.State(), contender.State())
	}

	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "promise ordering Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestStandalonePromiseReentrantHandlerAfterSettlementUnlock(t *testing.T) {
	promise := &ChainedPromise{}
	promise.state.Store(int32(Pending))
	nestedChild := make(chan *ChainedPromise, 1)
	outerChild := promise.Then(func(value any) any {
		nestedChild <- promise.Then(func(nestedValue any) any {
			return fmt.Sprintf("nested:%v", nestedValue)
		}, nil)
		return fmt.Sprintf("outer:%v", value)
	}, nil)

	resolved := make(chan struct{})
	go func() {
		promise.resolve(42)
		close(resolved)
	}()
	waitContractSignal(t, resolved, "standalone reentrant settlement")
	nested := waitContractValue(t, nestedChild, "standalone nested child publication")
	if promise.State() != Fulfilled || promise.Value() != 42 {
		t.Fatalf("parent = (%v, %#v), want Fulfilled 42", promise.State(), promise.Value())
	}
	if outerChild.State() != Fulfilled || outerChild.Value() != "outer:42" {
		t.Fatalf("outer child = (%v, %#v), want Fulfilled outer:42", outerChild.State(), outerChild.Value())
	}
	if nested.State() != Fulfilled || nested.Value() != "nested:42" {
		t.Fatalf("nested child = (%v, %#v), want Fulfilled nested:42", nested.State(), nested.Value())
	}
}
