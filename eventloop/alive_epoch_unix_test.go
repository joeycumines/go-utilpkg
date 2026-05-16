//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"sync"
	"testing"
)

func TestAliveEpochValidationObservesConcurrentFDRegistration(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	validationReached := make(chan struct{})
	releaseValidation := make(chan struct{})
	release := releaseSignalT(t, releaseValidation)
	var pauseOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeAliveEpochValidation: func() {
			pauseOnce.Do(func() {
				close(validationReached)
				<-releaseValidation
			})
		},
	}
	aliveDone := make(chan bool, 1)
	go func() { aliveDone <- loop.Alive() }()
	waitContractSignal(t, validationReached, "Alive epoch-validation boundary")

	fd, cleanupFD := testCreateIOFD(t)
	t.Cleanup(cleanupFD)
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}
	release()
	if alive := waitContractValue(t, aliveDone, "Alive epoch retry"); !alive {
		t.Fatal("Alive returned false after FD registration committed during final epoch validation")
	}
	if err := loop.UnregisterFD(fd); err != nil {
		t.Fatalf("UnregisterFD: %v", err)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount after UnregisterFD = %d, want 0", got)
	}
	if loop.Alive() {
		t.Fatal("Alive returned true after the only FD was unregistered")
	}
}
