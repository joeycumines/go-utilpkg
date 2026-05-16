package eventloop

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestJSTimeoutLifecycleOperationDoesNotWaitForCallbackClaim(t *testing.T) {
	operations := []struct {
		name            string
		apply           func(*JS, uint64) error
		callbackAllowed bool
	}{
		{name: "clear", apply: (*JS).ClearTimeout},
		{name: "ref", apply: (*JS).RefTimeout, callbackAllowed: true},
		{name: "unref", apply: (*JS).UnrefTimeout, callbackAllowed: true},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			registerLoopCleanupT(t, loop)
			js, err := NewJS(loop)
			if err != nil {
				t.Fatal(err)
			}

			callbackClaimed := make(chan struct{})
			releaseCallback := make(chan struct{})
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(releaseCallback) })
			loop.testHooks = &loopTestHooks{
				BeforeJSTimeoutCallbackClaim: func() {
					close(callbackClaimed)
					<-releaseCallback
				},
			}

			callbackRan := make(chan struct{}, 1)
			id, err := js.SetTimeout(func() { callbackRan <- struct{}{} }, 0)
			if err != nil {
				t.Fatal(err)
			}
			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(context.Background()) }()

			waitContractSignal(t, callbackClaimed, "timeout adapter-claim boundary")

			operationResult := make(chan error, 1)
			go func() { operationResult <- operation.apply(js, id) }()
			if err := waitContractValue(t, operationResult, operation.name+" timeout operation"); err != nil {
				t.Fatalf("%s timeout = %v", operation.name, err)
			}

			if operation.callbackAllowed {
				releaseOnce.Do(func() { close(releaseCallback) })
				waitContractSignal(t, callbackRan, "timeout callback after "+operation.name)
			} else {
				drained := make(chan struct{})
				if err := loop.SubmitInternal(func() { close(drained) }); err != nil {
					t.Fatalf("post-clear owner barrier admission: %v", err)
				}
				releaseOnce.Do(func() { close(releaseCallback) })
				waitContractSignal(t, drained, "post-clear timeout owner barrier")
				select {
				case <-callbackRan:
					t.Fatal("timeout callback ran after successful clear")
				default:
				}
			}

			if err := loop.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := waitContractValue(t, runDone, "timeout lifecycle Run completion"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestJSTimeoutLifecycleCommandsPreserveOwnerFIFO(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	callbackRan := make(chan struct{}, 1)
	id, err := js.SetTimeout(func() { callbackRan <- struct{}{} }, int(time.Hour/time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if err := js.UnrefTimeout(id); err != nil {
		t.Fatal(err)
	}
	if err := js.RefTimeout(id); err != nil {
		t.Fatal(err)
	}
	if err := js.UnrefTimeout(id); err != nil {
		t.Fatal(err)
	}

	firstBarrier := make(chan struct{})
	if err := loop.Submit(func() { close(firstBarrier) }); err != nil {
		t.Fatal(err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case <-firstBarrier:
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not apply pre-Run timeout lifecycle commands")
	}
	if count := loop.refedTimerCount.Load(); count != 0 {
		t.Fatalf("refed timer count after unref/ref/unref = %d, want 0", count)
	}

	if err := js.RefTimeout(id); err != nil {
		t.Fatal(err)
	}
	secondBarrier := make(chan struct{})
	if err := loop.Submit(func() { close(secondBarrier) }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-secondBarrier:
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not apply running RefTimeout command")
	}
	if count := loop.refedTimerCount.Load(); count != 1 {
		t.Fatalf("refed timer count after RefTimeout = %d, want 1", count)
	}

	if err := js.ClearTimeout(id); err != nil {
		t.Fatal(err)
	}
	thirdBarrier := make(chan struct{})
	if err := loop.Submit(func() { close(thirdBarrier) }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-thirdBarrier:
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not apply running ClearTimeout command")
	}
	if count := loop.refedTimerCount.Load(); count != 0 {
		t.Fatalf("refed timer count after ClearTimeout = %d, want 0", count)
	}
	select {
	case <-callbackRan:
		t.Fatal("timeout callback ran after successful clear")
	default:
	}
	if err := js.ClearTimeout(id); err != ErrTimerNotFound {
		t.Fatalf("second ClearTimeout = %v, want ErrTimerNotFound", err)
	}
	if err := js.RefTimeout(id); err != ErrTimerNotFound {
		t.Fatalf("RefTimeout after clear = %v, want ErrTimerNotFound", err)
	}
	if err := js.UnrefTimeout(id); err != ErrTimerNotFound {
		t.Fatalf("UnrefTimeout after clear = %v, want ErrTimerNotFound", err)
	}

	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Shutdown")
	}
}

func TestJSClearTimeoutIntervalHandleDoesNotWaitForCallback(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	callbackEntered := make(chan struct{})
	clearReturned := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(clearReturned) })
	id, err := js.SetInterval(func() {
		close(callbackEntered)
		<-clearReturned
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case <-callbackEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("interval callback did not start")
	}

	clearResult := make(chan error, 1)
	go func() {
		clearResult <- js.ClearTimeout(id)
		releaseOnce.Do(func() { close(clearReturned) })
	}()
	if err := waitContractValue(t, clearResult, "ClearTimeout interval-handle operation"); err != nil {
		t.Fatalf("ClearTimeout(interval ID) = %v", err)
	}

	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Shutdown")
	}
}

func TestJSTimeoutLifecycleOperationDoesNotWaitDuringPreRunTermination(t *testing.T) {
	operations := []struct {
		name     string
		schedule func(*JS) (uint64, error)
		apply    func(*JS, uint64) error
	}{
		{
			name:     "clear-timeout",
			schedule: func(js *JS) (uint64, error) { return js.SetTimeout(func() {}, 60_000) },
			apply:    (*JS).ClearTimeout,
		},
		{
			name:     "clear-interval",
			schedule: func(js *JS) (uint64, error) { return js.SetInterval(func() {}, 60_000) },
			apply:    (*JS).ClearInterval,
		},
		{
			name:     "ref-timeout",
			schedule: func(js *JS) (uint64, error) { return js.SetTimeout(func() {}, 60_000) },
			apply:    (*JS).RefTimeout,
		},
		{
			name:     "unref-timeout",
			schedule: func(js *JS) (uint64, error) { return js.SetTimeout(func() {}, 60_000) },
			apply:    (*JS).UnrefTimeout,
		},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			loop, err := New()
			if err != nil {
				t.Fatal(err)
			}
			js, err := NewJS(loop)
			if err != nil {
				t.Fatal(err)
			}
			id, err := operation.schedule(js)
			if err != nil {
				t.Fatal(err)
			}

			terminating := make(chan struct{})
			releaseShutdown := make(chan struct{})
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(releaseShutdown) })
			loop.testHooks = &loopTestHooks{
				AfterShutdownStateTerminating: func() {
					close(terminating)
					<-releaseShutdown
				},
			}
			shutdownDone := make(chan error, 1)
			go func() { shutdownDone <- loop.Shutdown(context.Background()) }()
			waitContractSignal(t, terminating, "pre-Run timeout StateTerminating publication")

			operationDone := make(chan error, 1)
			go func() { operationDone <- operation.apply(js, id) }()
			if err := waitContractValue(t, operationDone, operation.name+" pre-Run terminal operation"); err != ErrLoopTerminated {
				t.Fatalf("%s during StateTerminating = %v, want ErrLoopTerminated", operation.name, err)
			}

			releaseOnce.Do(func() { close(releaseShutdown) })
			if err := waitContractValue(t, shutdownDone, "pre-Run timeout Shutdown completion"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestJSTimeoutRefOperationsPreserveExternalOwnerFIFO(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}
	id, err := js.SetTimeout(func() {}, int(time.Hour/time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}

	ownerEntered := make(chan struct{})
	releaseOwner := make(chan struct{})
	ownerRefDone := make(chan error, 1)
	if err := loop.Submit(func() {
		close(ownerEntered)
		<-releaseOwner
		ownerRefDone <- js.RefTimeout(id)
	}); err != nil {
		t.Fatal(err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case <-ownerEntered:
	case <-time.After(5 * time.Second):
		close(releaseOwner)
		t.Fatal("owner callback did not start")
	}

	if err := js.UnrefTimeout(id); err != nil {
		close(releaseOwner)
		t.Fatal(err)
	}
	close(releaseOwner)
	select {
	case err := <-ownerRefDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("owner RefTimeout did not return")
	}

	barrier := make(chan struct{})
	if err := loop.enqueueCommand(loopCommand{
		kind: loopCommandExternal,
		fn:   func() { close(barrier) },
	}, loop.terminalQueueAllowed); err != nil {
		t.Fatal(err)
	}
	select {
	case <-barrier:
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not drain ordered timeout ref commands")
	}
	if count := loop.refedTimerCount.Load(); count != 1 {
		t.Fatalf("refed timer count after external unref then owner ref = %d, want 1", count)
	}

	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after Shutdown")
	}
}
