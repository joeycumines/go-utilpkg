package eventloop

import (
	"reflect"
	"sync"
	"testing"
)

func TestChainedPromiseReactionObservesPublishedFulfillment(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}
	promise, resolve, _ := js.NewChainedPromise()
	observed := make(chan struct {
		state PromiseState
		value any
	}, 1)
	promise.Then(func(value any) any {
		observed <- struct {
			state PromiseState
			value any
		}{state: promise.State(), value: promise.Value()}
		return value
	}, nil)

	handlerQueued := make(chan struct{})
	releaseResolver := make(chan struct{})
	release := releaseSignalT(t, releaseResolver)
	var once sync.Once
	loop.testHooks = &loopTestHooks{
		AfterPromiseHandlerScheduled: func() {
			once.Do(func() {
				close(handlerQueued)
				<-releaseResolver
			})
		},
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(t.Context()) }()
	resolveDone := make(chan struct{})
	go func() {
		resolve("published")
		close(resolveDone)
	}()

	waitContractSignal(t, handlerQueued, "fulfillment reaction queue publication")
	got := waitContractValue(t, observed, "fulfillment reaction while resolver paused")
	if got.state != Fulfilled || got.value != "published" {
		t.Fatalf("reaction observed state=%v value=%#v", got.state, got.value)
	}
	release()
	waitContractSignal(t, resolveDone, "fulfillment resolver completion")
	if err := loop.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "fulfillment publication Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestChainedPromisePublishingReactionReentersThenAfterUnlock(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}
	promise, resolve, _ := js.NewChainedPromise()
	observed := make(chan string, 2)
	nestedChild := make(chan *ChainedPromise, 1)
	outerChild := promise.Then(func(value any) any {
		observed <- "outer"
		nestedChild <- promise.Then(func(nestedValue any) any {
			observed <- "nested"
			return "nested-child"
		}, nil)
		return "outer-child"
	}, nil)

	handlerQueued := make(chan struct{})
	releaseResolver := make(chan struct{})
	releaseResolverFn := releaseSignalT(t, releaseResolver)
	nestedPendingChecked := make(chan struct{})
	releaseNested := make(chan struct{})
	releaseNestedFn := releaseSignalT(t, releaseNested)
	var queuedOnce sync.Once
	var pendingOnce sync.Once
	loop.testHooks = &loopTestHooks{
		AfterPromiseHandlerScheduled: func() {
			queuedOnce.Do(func() {
				close(handlerQueued)
				<-releaseResolver
			})
		},
		AfterPromiseHandlerPendingCheck: func() {
			pendingOnce.Do(func() {
				close(nestedPendingChecked)
				<-releaseNested
			})
		},
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(t.Context()) }()
	resolveDone := make(chan struct{})
	go func() {
		resolve("parent")
		close(resolveDone)
	}()

	waitContractSignal(t, handlerQueued, "publishing parent reaction")
	waitContractSignal(t, nestedPendingChecked, "nested publishing-state pending check")
	releaseResolverFn()
	waitContractSignal(t, resolveDone, "parent publication completion")
	if promise.State() != Fulfilled || promise.Value() != "parent" {
		t.Fatalf("parent = (%v, %#v), want Fulfilled parent", promise.State(), promise.Value())
	}
	releaseNestedFn()
	nested := waitContractValue(t, nestedChild, "nested child publication")
	order := []string{
		waitContractValue(t, observed, "outer publishing reaction"),
		waitContractValue(t, observed, "nested publishing reaction"),
	}
	if want := []string{"outer", "nested"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("publishing reaction order = %v, want %v", order, want)
	}
	if got := waitContractValue(t, outerChild.ToChannel(), "outer publishing child"); got != "outer-child" {
		t.Fatalf("outer child = %#v, want outer-child", got)
	}
	if got := waitContractValue(t, nested.ToChannel(), "nested publishing child"); got != "nested-child" {
		t.Fatalf("nested child = %#v, want nested-child", got)
	}

	if err := loop.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "publishing reaction Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestChainedPromiseReactionObservesPublishedRejection(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}
	promise, _, reject := js.NewChainedPromise()
	observed := make(chan struct {
		state  PromiseState
		reason any
	}, 1)
	promise.Catch(func(reason any) any {
		observed <- struct {
			state  PromiseState
			reason any
		}{state: promise.State(), reason: promise.Reason()}
		return reason
	})

	handlerQueued := make(chan struct{})
	releaseRejecter := make(chan struct{})
	release := releaseSignalT(t, releaseRejecter)
	var once sync.Once
	loop.testHooks = &loopTestHooks{
		AfterPromiseHandlerScheduled: func() {
			once.Do(func() {
				close(handlerQueued)
				<-releaseRejecter
			})
		},
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(t.Context()) }()
	rejectDone := make(chan struct{})
	go func() {
		reject("published rejection")
		close(rejectDone)
	}()

	waitContractSignal(t, handlerQueued, "rejection reaction queue publication")
	got := waitContractValue(t, observed, "rejection reaction while rejecter paused")
	if got.state != Rejected || got.reason != "published rejection" {
		t.Fatalf("reaction observed state=%v reason=%#v", got.state, got.reason)
	}
	release()
	waitContractSignal(t, rejectDone, "rejection publisher completion")
	if err := loop.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "rejection publication Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}
