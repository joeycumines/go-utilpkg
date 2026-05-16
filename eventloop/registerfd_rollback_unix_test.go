//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"errors"
	"testing"
)

func TestRegisterFDPreCountRollbackPollerCloseReleasesOwnership(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	fd, cleanupFD := testCreateIOFD(t)
	defer cleanupFD()

	var pollerCloseCalled bool
	var pollerCloseErr error
	loop.testHooks = &loopTestHooks{
		BeforeRegisterFDRollbackCheck: func() {
			loop.beginQuiescing()
			pollerCloseCalled = true
			pollerCloseErr = loop.poller.Close()
		},
	}

	err = loop.RegisterFD(fd, EventRead, func(IOEvents) {})
	if !pollerCloseCalled || pollerCloseErr != nil {
		t.Fatalf("rollback hook poller Close = (called=%t, err=%v), want (true, nil)", pollerCloseCalled, pollerCloseErr)
	}
	if !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("RegisterFD error = %v, want ErrLoopTerminated", err)
	}
	var rollbackErr *FDRegistrationRollbackError
	if errors.As(err, &rollbackErr) {
		t.Fatalf("RegisterFD returned retained-ownership error after poller Close: %#v", rollbackErr)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount after poller Close rollback = %d, want 0", got)
	}

	loop.poller.fdMu.RLock()
	active := fd < len(loop.poller.fds) && loop.poller.fds[fd].active
	loop.poller.fdMu.RUnlock()
	if active {
		t.Fatal("poller FD entry remained active after poller Close")
	}
	if loop.pollerReady.Load() {
		t.Fatal("pollerReady remained published after unpublished poller Close rollback")
	}
}

func TestRegisterFDPreCountRollbackNotRegisteredIsNotOwned(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	fd, cleanupFD := testCreateIOFD(t)
	defer cleanupFD()

	loop.testHooks = &loopTestHooks{
		BeforeRegisterFDRollbackCheck: func() {
			if err := loop.poller.UnregisterFD(fd); err != nil {
				t.Errorf("test rollback unregister setup: %v", err)
			}
			loop.beginQuiescing()
		},
	}

	err = loop.RegisterFD(fd, EventRead, func(IOEvents) {})
	if !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("RegisterFD error = %v, want ErrLoopTerminated", err)
	}
	var rollbackErr *FDRegistrationRollbackError
	if errors.As(err, &rollbackErr) {
		t.Fatalf("RegisterFD returned rollback ownership error = %#v, want plain lifecycle rejection", rollbackErr)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount after ErrFDNotRegistered rollback = %d, want 0", got)
	}
}

func TestRegisterFDRollbackPollerClosePreservesForcedMode(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	fd, cleanupFD := testCreateIOFD(t)
	defer cleanupFD()

	var pollerCloseCalled bool
	var pollerCloseErr error
	loop.testHooks = &loopTestHooks{
		BeforeRegisterFDCommit: func() {
			loop.fastPathMode.Store(int32(FastPathForced))
			pollerCloseCalled = true
			pollerCloseErr = loop.poller.Close()
		},
	}

	err = loop.RegisterFD(fd, EventRead, func(IOEvents) {})
	if !pollerCloseCalled || pollerCloseErr != nil {
		t.Fatalf("commit hook poller Close = (called=%t, err=%v), want (true, nil)", pollerCloseCalled, pollerCloseErr)
	}
	if !errors.Is(err, ErrFastPathIncompatible) {
		t.Fatalf("RegisterFD error = %v, want ErrFastPathIncompatible", err)
	}
	var rollbackErr *FDRegistrationRollbackError
	if errors.As(err, &rollbackErr) {
		t.Fatalf("RegisterFD returned retained-ownership error after poller Close: %#v", rollbackErr)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount after poller Close rollback = %d, want 0", got)
	}
	if got := FastPathMode(loop.fastPathMode.Load()); got != FastPathForced {
		t.Fatalf("fastPathMode after poller Close rollback = %v, want %v", got, FastPathForced)
	}
	if !loop.canUseFastPath() {
		t.Fatal("canUseFastPath returned false with forced mode and no retained FD")
	}

	loop.poller.fdMu.RLock()
	active := fd < len(loop.poller.fds) && loop.poller.fds[fd].active
	loop.poller.fdMu.RUnlock()
	if active {
		t.Fatal("poller FD entry remained active after poller Close")
	}
}

func TestRegisterFDSuccessfulForcedModeRollbackPreservesMode(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	fd, cleanupFD := testCreateIOFD(t)
	defer cleanupFD()

	loop.testHooks = &loopTestHooks{
		BeforeRegisterFDCommit: func() {
			loop.fastPathMode.Store(int32(FastPathForced))
		},
	}

	err = loop.RegisterFD(fd, EventRead, func(IOEvents) {})
	if !errors.Is(err, ErrFastPathIncompatible) {
		t.Fatalf("RegisterFD error = %v, want ErrFastPathIncompatible", err)
	}
	if got := FastPathMode(loop.fastPathMode.Load()); got != FastPathForced {
		t.Fatalf("fastPathMode after successful rollback = %v, want FastPathForced", got)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount after successful rollback = %d, want 0", got)
	}
}
