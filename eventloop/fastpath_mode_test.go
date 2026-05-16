//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestRegisterFDForcedModeRejectsWithoutPollerResources(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathForced))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	if loop.pollerReady.Load() || loop.wakePipe != -1 || loop.wakePipeWrite != -1 {
		t.Fatalf("forced task-only resources = (ready %v, wake %d/%d), want uninitialized", loop.pollerReady.Load(), loop.wakePipe, loop.wakePipeWrite)
	}

	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])

	if err := loop.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); err != ErrFastPathIncompatible {
		t.Fatalf("RegisterFD = %v, want %v", err, ErrFastPathIncompatible)
	}
	if loop.pollerReady.Load() || loop.wakePipe != -1 || loop.wakePipeWrite != -1 {
		t.Fatalf("rejected registration resources = (ready %v, wake %d/%d), want uninitialized", loop.pollerReady.Load(), loop.wakePipe, loop.wakePipeWrite)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount = %d, want 0", got)
	}
}

func TestFastPathAutoTracksFDRegistration(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	if !loop.canUseFastPath() || loop.pollerReady.Load() {
		t.Fatalf("initial Auto state = (fast %v, poller %v), want (true, false)", loop.canUseFastPath(), loop.pollerReady.Load())
	}

	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	if err := loop.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}
	if loop.canUseFastPath() || !loop.pollerReady.Load() || loop.userIOFDCount.Load() != 1 {
		t.Fatalf("registered Auto state = (fast %v, poller %v, count %d), want (false, true, 1)", loop.canUseFastPath(), loop.pollerReady.Load(), loop.userIOFDCount.Load())
	}
	if err := loop.UnregisterFD(pipeFDs[0]); err != nil {
		t.Fatalf("UnregisterFD: %v", err)
	}
	if !loop.canUseFastPath() || loop.userIOFDCount.Load() != 0 {
		t.Fatalf("unregistered Auto state = (fast %v, count %d), want (true, 0)", loop.canUseFastPath(), loop.userIOFDCount.Load())
	}
}
