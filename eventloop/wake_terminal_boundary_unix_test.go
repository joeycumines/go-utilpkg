//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestTerminalWinnerAtFinalPollBoundarySkipsPollIO(t *testing.T) {
	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	if err := loop.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	loop.drainWakeUpPipe()
	select {
	case <-loop.fastWakeupCh:
	default:
	}

	boundaryEntered := make(chan struct{})
	releaseBoundary := make(chan struct{})
	terminalVisible := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseBoundary) }) })
	var pollCalls atomic.Int32
	loop.testHooks = &loopTestHooks{
		BeforePollIO: func() {
			close(boundaryEntered)
			<-releaseBoundary
		},
		BeforeClosePromiseRejection: func() { close(terminalVisible) },
		PollIO: func(int) (int, error) {
			pollCalls.Add(1)
			return 0, nil
		},
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case <-boundaryEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not reach final native-poll boundary")
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	select {
	case <-terminalVisible:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not publish StateTerminated at the final poll boundary")
	}
	releaseOnce.Do(func() { close(releaseBoundary) })
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not complete after terminal boundary release")
	}
	if got := pollCalls.Load(); got != 0 {
		t.Fatalf("PollIO calls after terminal winner = %d, want 0", got)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after terminal winner")
	}
}

func TestTerminatingWinnerAtFinalPollBoundarySkipsPollIO(t *testing.T) {
	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	if err := loop.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	loop.drainWakeUpPipe()
	select {
	case <-loop.fastWakeupCh:
	default:
	}

	boundaryEntered := make(chan struct{})
	releaseBoundary := make(chan struct{})
	terminatingVisible := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseBoundary) }) })
	var pollCalls atomic.Int32
	loop.testHooks = &loopTestHooks{
		BeforePollIO: func() {
			close(boundaryEntered)
			<-releaseBoundary
		},
		AfterShutdownStateTerminating: func() { close(terminatingVisible) },
		PollIO: func(int) (int, error) {
			pollCalls.Add(1)
			return 0, nil
		},
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case <-boundaryEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not reach final native-poll boundary")
	}
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(context.Background()) }()
	select {
	case <-terminatingVisible:
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not publish StateTerminating at the final poll boundary")
	}
	releaseOnce.Do(func() { close(releaseBoundary) })
	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown did not complete after terminal boundary release")
	}
	if got := pollCalls.Load(); got != 0 {
		t.Fatalf("PollIO calls after StateTerminating winner = %d, want 0", got)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after StateTerminating winner")
	}
}
