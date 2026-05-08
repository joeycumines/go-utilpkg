package eventloop

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestJSIntervalRefOperationDoesNotWaitForCallback(t *testing.T) {
	operations := []struct {
		name  string
		apply func(*JS, uint64) error
	}{
		{name: "ref", apply: (*JS).RefInterval},
		{name: "unref", apply: (*JS).UnrefInterval},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			loop := New()
			registerLoopCleanupT(t, loop)
			js := NewJS(loop)

			callbackEntered := make(chan struct{})
			releaseCallback := make(chan struct{})
			var callbackOnce sync.Once
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(releaseCallback) })
			id, err := js.SetInterval(func() {
				callbackOnce.Do(func() {
					close(callbackEntered)
					<-releaseCallback
				})
			}, 0)
			if err != nil {
				t.Fatal(err)
			}

			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(context.Background()) }()
			waitContractSignal(t, callbackEntered, "interval callback entry")

			operationDone := make(chan error, 1)
			go func() { operationDone <- operation.apply(js, id) }()
			if err := waitContractValue(t, operationDone, operation.name+" interval operation"); err != nil {
				t.Fatalf("%s interval = %v", operation.name, err)
			}

			releaseOnce.Do(func() { close(releaseCallback) })
			if err := js.ClearInterval(id); err != nil {
				t.Fatal(err)
			}
			if err := loop.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := waitContractValue(t, runDone, "interval ref-operation Run completion"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestJSIntervalRefOperationsPreservePreRunFIFO(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	js := NewJS(loop)
	id, err := js.SetInterval(func() {}, int(time.Hour/time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}

	if err := js.UnrefInterval(id); err != nil {
		t.Fatal(err)
	}
	if err := js.RefInterval(id); err != nil {
		t.Fatal(err)
	}
	if err := js.UnrefInterval(id); err != nil {
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
		t.Fatal("loop did not apply pre-Run interval ref commands")
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refed timer count after unref/ref/unref = %d, want 0", got)
	}

	if err := js.RefInterval(id); err != nil {
		t.Fatal(err)
	}
	secondBarrier := make(chan struct{})
	if err := loop.Submit(func() { close(secondBarrier) }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-secondBarrier:
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not apply running RefInterval command")
	}
	if got := loop.refedTimerCount.Load(); got != 1 {
		t.Fatalf("refed timer count after RefInterval = %d, want 1", got)
	}

	if err := js.ClearInterval(id); err != nil {
		t.Fatal(err)
	}
	thirdBarrier := make(chan struct{})
	if err := loop.Submit(func() { close(thirdBarrier) }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-thirdBarrier:
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not apply running ClearInterval command")
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refed timer count after ClearInterval = %d, want 0", got)
	}
	if err := js.RefInterval(id); !errors.Is(err, ErrTimerNotFound) {
		t.Fatalf("RefInterval after clear = %v, want ErrTimerNotFound", err)
	}
	if err := js.UnrefInterval(id); !errors.Is(err, ErrTimerNotFound) {
		t.Fatalf("UnrefInterval after clear = %v, want ErrTimerNotFound", err)
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

func TestJSIntervalRefOperationsPreserveExternalOwnerFIFO(t *testing.T) {
	tests := []struct {
		name          string
		initialUnref  bool
		externalApply func(*JS, uint64) error
		ownerApply    func(*JS, uint64) error
		wantRefCount  int64
	}{
		{
			name:          "external-unref-owner-ref",
			externalApply: (*JS).UnrefInterval,
			ownerApply:    (*JS).RefInterval,
			wantRefCount:  1,
		},
		{
			name:          "external-ref-owner-unref",
			initialUnref:  true,
			externalApply: (*JS).RefInterval,
			ownerApply:    (*JS).UnrefInterval,
			wantRefCount:  0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop := New()
			registerLoopCleanupT(t, loop)
			js := NewJS(loop)

			callbackEntered := make(chan struct{})
			releaseCallback := make(chan struct{})
			var callbackOnce sync.Once
			var releaseOnce sync.Once
			defer releaseOnce.Do(func() { close(releaseCallback) })
			ownerDone := make(chan error, 1)
			var intervalID uint64
			intervalID, err := js.SetInterval(func() {
				callbackOnce.Do(func() {
					close(callbackEntered)
					<-releaseCallback
					ownerDone <- test.ownerApply(js, intervalID)
				})
			}, 0)
			if err != nil {
				t.Fatal(err)
			}
			if test.initialUnref {
				if err := js.UnrefInterval(intervalID); err != nil {
					t.Fatal(err)
				}
			}

			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(context.Background()) }()
			waitContractSignal(t, callbackEntered, "ordered interval callback entry")

			externalDone := make(chan error, 1)
			go func() { externalDone <- test.externalApply(js, intervalID) }()
			if err := waitContractValue(t, externalDone, "external interval ref operation"); err != nil {
				t.Fatal(err)
			}

			releaseOnce.Do(func() { close(releaseCallback) })
			if err := waitContractValue(t, ownerDone, "owner interval ref operation"); err != nil {
				t.Fatal(err)
			}

			barrier := make(chan struct{})
			if err := loop.enqueueCommand(loopCommand{
				kind: loopCommandExternal,
				fn:   func() { close(barrier) },
			}, loop.terminalQueueAllowed); err != nil {
				t.Fatal(err)
			}
			waitContractSignal(t, barrier, "ordered interval ref-command drain")
			if got := loop.refedTimerCount.Load(); got != test.wantRefCount {
				t.Fatalf("refed timer count = %d, want %d", got, test.wantRefCount)
			}

			if err := js.ClearInterval(intervalID); err != nil {
				t.Fatal(err)
			}
			if err := loop.Shutdown(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := waitContractValue(t, runDone, "ordered interval Run completion"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestJSIntervalRefOperationDoesNotWaitDuringPreRunTermination(t *testing.T) {
	operations := []struct {
		name  string
		apply func(*JS, uint64) error
	}{
		{name: "ref", apply: (*JS).RefInterval},
		{name: "unref", apply: (*JS).UnrefInterval},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			loop := New()
			registerLoopCleanupT(t, loop)
			js := NewJS(loop)
			id, err := js.SetInterval(func() {}, int(time.Hour/time.Millisecond))
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
			waitContractSignal(t, terminating, "pre-Run interval StateTerminating publication")

			operationDone := make(chan error, 1)
			go func() { operationDone <- operation.apply(js, id) }()
			if err := waitContractValue(t, operationDone, operation.name+" pre-Run terminal interval operation"); !errors.Is(err, ErrLoopTerminated) {
				t.Fatalf("%s interval during StateTerminating = %v, want ErrLoopTerminated", operation.name, err)
			}

			releaseOnce.Do(func() { close(releaseShutdown) })
			if err := waitContractValue(t, shutdownDone, "pre-Run interval Shutdown completion"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
