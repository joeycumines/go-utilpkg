package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSubmitInternalPreservesOwnerAffinity(t *testing.T) {
	tests := []struct {
		name string
		mode FastPathMode
	}{
		{name: "fast path", mode: FastPathForced},
		{name: "native poll path", mode: FastPathDisabled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop := New(WithFastPathMode(test.mode))
			registerLoopCleanupT(t, loop)

			runDone := make(chan error, 1)
			go func() { runDone <- loop.Run(context.Background()) }()
			waitLoopOwnerTurnT(t, loop)

			admission := make(chan error, 1)
			affinity := make(chan bool, 1)
			go func() {
				admission <- loop.SubmitInternal(func() { affinity <- loop.isLoopThread() })
			}()
			if err := waitContractValue(t, admission, "external SubmitInternal admission"); err != nil {
				t.Fatalf("SubmitInternal: %v", err)
			}
			if !waitContractValue(t, affinity, "SubmitInternal owner-affinity observation") {
				t.Fatal("SubmitInternal callback ran outside the loop owner")
			}

			if err := loop.Shutdown(context.Background()); err != nil {
				t.Fatalf("Shutdown: %v", err)
			}
			if err := waitContractValue(t, runDone, "owner-affinity Run completion"); err != nil {
				t.Fatalf("Run: %v", err)
			}
		})
	}
}

func TestTickAnchorSynchronization(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	start := make(chan struct{})
	startNow := contractRelease(t, start)
	ready := make(chan struct{}, 8)
	var (
		workers      sync.WaitGroup
		zeroTickTime atomic.Bool
	)
	for writerID := range 4 {
		workers.Go(func() {
			ready <- struct{}{}
			<-start
			for offset := range 100 {
				loop.setTickAnchor(time.Now().Add(time.Duration(writerID*1000+offset) * time.Millisecond))
			}
		})
	}
	for range 4 {
		workers.Go(func() {
			ready <- struct{}{}
			<-start
			for range 200 {
				if loop.CurrentTickTime().IsZero() {
					zeroTickTime.Store(true)
				}
			}
		})
	}
	for range 8 {
		waitContractSignal(t, ready, "concurrent tick-anchor worker readiness")
	}
	startNow()
	workersDone := make(chan struct{})
	go func() {
		workers.Wait()
		close(workersDone)
	}()
	waitContractSignal(t, workersDone, "concurrent tick-anchor operations")
	if zeroTickTime.Load() {
		t.Fatal("CurrentTickTime returned the zero time during concurrent anchor updates")
	}
	if loop.tickAnchorTime().IsZero() {
		t.Fatal("concurrent anchor writers left a zero tick anchor")
	}
}
