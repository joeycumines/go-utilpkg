package eventloop

import (
	"context"
	"sync"
	"testing"
)

func TestAliveEpochValidationObservesConcurrentPromisifyCommit(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)

	validationReached := make(chan struct{})
	releaseValidation := make(chan struct{})
	releaseValidationFn := releaseSignalT(t, releaseValidation)
	var pauseOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeAliveEpochValidation: func() {
			pauseOnce.Do(func() {
				close(validationReached)
				<-releaseValidation
			})
		},
	}
	aliveResult := make(chan bool, 1)
	go func() { aliveResult <- loop.Alive() }()
	waitContractSignal(t, validationReached, "Alive epoch-validation boundary")

	releaseWorker := make(chan struct{})
	releaseWorkerFn := releaseSignalT(t, releaseWorker)
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		<-releaseWorker
		return "committed", nil
	})
	if got := loop.promisifyCount.Load(); got != 1 {
		t.Fatalf("promisifyCount after concurrent commit = %d, want 1", got)
	}
	releaseValidationFn()
	if alive := waitContractValue(t, aliveResult, "Alive epoch retry"); !alive {
		t.Fatal("Alive returned false after Promisify committed during final epoch validation")
	}

	releaseWorkerFn()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	if err := waitContractValue(t, runDone, "epoch-test auto-exit completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if state := promise.State(); state != Fulfilled || promise.Result() != "committed" {
		t.Fatalf("Promisify settlement = (%v, %#v), want Fulfilled committed", state, promise.Result())
	}
}
