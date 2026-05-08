//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"sync/atomic"
	"testing"
)

func TestSubmitCommittedBeforeRegisterFDDrainsWhenPollPathEntered(t *testing.T) {
	loop := New()
	registerFDResourceCleanupT(t, loop)
	loop.state.Store(StateRunning)

	var ran atomic.Bool
	if err := loop.Submit(func() { ran.Store(true) }); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if ran.Load() {
		t.Fatal("submitted job ran before the loop tick drained ingress")
	}

	fd, cleanupFD := testCreateIOFD(t)
	defer cleanupFD()
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	loop.tick()

	if !ran.Load() {
		t.Fatal("submitted job committed before RegisterFD did not drain after poll path wake")
	}
	if err := loop.UnregisterFD(fd); err != nil {
		t.Fatalf("UnregisterFD: %v", err)
	}
}
