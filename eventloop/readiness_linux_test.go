//go:build linux

package eventloop

import (
	"context"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestLinuxPeerHalfCloseReportsHangup(t *testing.T) {
	sockets, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &sockets[0], &sockets[1])
	loop := New()
	events := make(chan IOEvents, 1)
	if err := loop.RegisterFD(sockets[0], EventRead, func(ready IOEvents) {
		select {
		case events <- ready:
		default:
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := unix.Shutdown(sockets[1], unix.SHUT_WR); err != nil {
		t.Fatal(err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	registerActiveLoopCleanupT(t, loop, runDone)

	select {
	case ready := <-events:
		if ready&EventHangup == 0 {
			t.Fatalf("ready events = %v, want EventHangup", ready)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("peer half-close did not produce readiness callback")
	}
	if err := loop.UnregisterFD(sockets[0]); err != nil {
		t.Fatal(err)
	}
}
