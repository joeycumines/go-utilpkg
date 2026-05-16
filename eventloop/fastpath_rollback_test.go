//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"errors"
	"sync"
	"testing"
)

func TestSetFastPathModeSerializesWithRegisterFDCommit(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	fd, cleanup := testCreateIOFD(t)
	defer cleanup()

	atCommit := make(chan struct{})
	releaseRegister := make(chan struct{})
	releaseRegisterFn := releaseSignalT(t, releaseRegister)
	modeAtLock := make(chan struct{})
	var hookOnce sync.Once
	var modeOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeRegisterFDCommit: func() {
			hookOnce.Do(func() { close(atCommit) })
			<-releaseRegister
		},
		BeforeSetFastPathModeLock: func() {
			modeOnce.Do(func() { close(modeAtLock) })
		},
	}

	registerDone := make(chan error, 1)
	go func() {
		registerDone <- loop.RegisterFD(fd, EventRead, func(IOEvents) {})
	}()

	waitContractSignal(t, atCommit, "RegisterFD commit hook")

	modeDone := make(chan error, 1)
	go func() { modeDone <- loop.SetFastPathMode(FastPathForced) }()
	waitContractSignal(t, modeAtLock, "SetFastPathMode liveness-lock boundary")
	if loop.livenessMu.TryLock() {
		loop.livenessMu.Unlock()
		t.Fatal("RegisterFD commit hook did not retain liveness ownership")
	}

	releaseRegisterFn()
	if err := waitContractValue(t, registerDone, "RegisterFD completion"); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}
	if err := waitContractValue(t, modeDone, "SetFastPathMode completion"); !errors.Is(err, ErrFastPathIncompatible) {
		t.Fatalf("SetFastPathMode while FD registered = %v, want ErrFastPathIncompatible", err)
	}
	if got := FastPathMode(loop.fastPathMode.Load()); got == FastPathForced {
		t.Fatalf("fastPathMode = %v with registered FD", got)
	}
	if got := loop.userIOFDCount.Load(); got != 1 {
		t.Fatalf("userIOFDCount = %d, want 1", got)
	}
	if loop.canUseFastPath() {
		t.Fatal("canUseFastPath true while FD remains registered")
	}
}
