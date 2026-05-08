//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"errors"
	"testing"
)

func TestUnregisterFDLatchesSuccessfulWakeThroughRetirement(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	fd, cleanup := testCreateIOFD(t)
	t.Cleanup(cleanup)
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	writes := 0
	loop.testHooks = &loopTestHooks{WriteWakeFD: func(_ int, value []byte) (int, error) {
		writes++
		return len(value), nil
	}}
	loop.poller.beforeDispatchWait = func() {
		if writes != 1 {
			t.Fatalf("physical writes at retirement barrier = %d, want 1", writes)
		}
		if loop.wakeMu.TryLock() {
			loop.wakeMu.Unlock()
			t.Fatal("wakeMu was not latched through native retirement")
		}
	}

	if err := loop.UnregisterFD(fd); err != nil {
		t.Fatal(err)
	}
	if writes != 2 {
		t.Fatalf("physical writes after last-FD transition = %d, want retirement and transition wakes", writes)
	}
}

func TestUnregisterFDWakeFailureReleasesLatchBeforeRetirement(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	fd, cleanup := testCreateIOFD(t)
	t.Cleanup(cleanup)
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("forced loop wake failure")
	writes := 0
	loop.testHooks = &loopTestHooks{WriteWakeFD: func(_ int, value []byte) (int, error) {
		writes++
		if writes == 1 {
			return 0, sentinel
		}
		return len(value), nil
	}}
	loop.poller.beforeDispatchWait = func() {
		if !loop.wakeMu.TryLock() {
			t.Fatal("failed physical wake retained wakeMu through native retirement")
		}
		loop.wakeMu.Unlock()
	}

	if err := loop.UnregisterFD(fd); err != nil {
		t.Fatal(err)
	}
	if writes != 2 {
		t.Fatalf("physical writes after wake failure and last-FD transition = %d, want 2", writes)
	}
}
