//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
)

func TestPhysicalPendingWakeStillExecutesScheduledMicrotask(t *testing.T) {
	loop := New()
	pipeReader, pipeWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	registerFileCleanupT(t, pipeReader, pipeWriter)
	registerLoopCleanupT(t, loop)
	if err := loop.RegisterFD(int(pipeReader.Fd()), EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	loop.drainWakeUpPipe()
	select {
	case <-loop.fastWakeupCh:
	default:
	}

	pollEntered := make(chan struct{})
	releasePoll := make(chan struct{})
	releasePollFn := releaseSignalT(t, releasePoll)
	var writes atomic.Int32
	loop.testHooks = &loopTestHooks{
		PollIO: func(int) (int, error) {
			if writes.Load() == 0 {
				close(pollEntered)
				<-releasePoll
			}
			return 0, nil
		},
		WriteWakeFD: func(fd int, payload []byte) (int, error) {
			writes.Add(1)
			return writeFD(fd, payload)
		},
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, pollEntered, "blocked native poll entry")

	if err := loop.submitPendingWakeup(); err != nil {
		t.Fatalf("submitPendingWakeup: %v", err)
	}
	if got := loop.wakeUpSignalPending.Load(); got != wakeSignalPending {
		t.Fatalf("physical pending state = %d, want %d", got, wakeSignalPending)
	}
	executed := make(chan struct{})
	if err := loop.ScheduleMicrotask(func() { close(executed) }); err != nil {
		t.Fatalf("ScheduleMicrotask: %v", err)
	}
	if got := writes.Load(); got != 1 {
		t.Fatalf("physical writes after represented microtask wake = %d, want 1", got)
	}
	select {
	case <-executed:
		t.Fatal("microtask executed before blocked native poll returned")
	default:
	}

	releasePollFn()
	waitContractSignal(t, executed, "microtask behind represented physical wake")
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := waitContractValue(t, runDone, "Run completion after Close"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}
