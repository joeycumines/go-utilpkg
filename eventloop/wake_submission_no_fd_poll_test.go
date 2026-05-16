//go:build plan9 || windows || ((js || wasip1) && wasm)

package eventloop

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestNoFDPollPlatformWakeUsesOnlyActiveFastWait(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}

	waitEntered := make(chan struct{}, 2)
	var physicalSubmissions atomic.Int32
	loop.testHooks = &loopTestHooks{
		BeforeFastPollWait: func(int) { waitEntered <- struct{}{} },
		BeforePhysicalWake: func() { physicalSubmissions.Add(1) },
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	registerActiveLoopCleanupT(t, loop, runDone)

	select {
	case <-waitEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not enter the fast-channel wait")
	}
	if err := loop.Wake(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-waitEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("public Wake did not release the active fast-channel wait")
	}
	if got := physicalSubmissions.Load(); got != 0 {
		t.Fatalf("physical submissions while no native FD wait is possible = %d, want 0", got)
	}
	if got := loop.wakeUpSignalPending.Load(); got != wakeSignalIdle {
		t.Fatalf("physical pending state = %d, want idle", got)
	}
}
