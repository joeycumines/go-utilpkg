//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"errors"
	"math"
	"testing"

	"golang.org/x/sys/unix"
)

func TestFDGenerationExhaustionPreventsIdentityReuse(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}
	registerPollerCleanupT(t, &poller)
	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])

	poller.fdGeneration.Store(math.MaxUint64)
	if err := poller.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); !errors.Is(err, ErrFDRegistrationExhausted) {
		t.Fatalf("RegisterFD = %v, want ErrFDRegistrationExhausted", err)
	}
	if poller.userFDRegistered(pipeFDs[0]) {
		t.Fatal("registration remained active after identity exhaustion")
	}
}

func TestLoopFDGenerationExhaustionRollsBackLazyPoller(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	loop.poller.fdGeneration.Store(math.MaxUint64)
	if err := loop.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); !errors.Is(err, ErrFDRegistrationExhausted) {
		t.Fatalf("RegisterFD = %v, want ErrFDRegistrationExhausted", err)
	}
	if loop.pollerReady.Load() || loop.userIOFDCount.Load() != 0 {
		t.Fatalf("failed registration published pollerReady=%v userIOFDCount=%d", loop.pollerReady.Load(), loop.userIOFDCount.Load())
	}
	if loop.wakePipe != -1 || loop.wakePipeWrite != -1 {
		t.Fatalf("failed registration retained wake descriptors (%d, %d)", loop.wakePipe, loop.wakePipeWrite)
	}
}
