//go:build plan9 || windows || ((js || wasip1) && wasm)

package eventloop

import (
	"errors"
	"testing"
)

func TestTaskOnlyEventBitsAreDistinct(t *testing.T) {
	events := []IOEvents{EventRead, EventWrite, EventError, EventHangup}
	for i, event := range events {
		if event == 0 {
			t.Fatalf("event bit %d is zero", i)
		}
		for j := i + 1; j < len(events); j++ {
			if event&events[j] != 0 {
				t.Fatalf("event bits %d and %d overlap: %#x %#x", i, j, event, events[j])
			}
		}
	}
}

func TestTaskOnlyUnsupportedFDErrorsAreStable(t *testing.T) {
	if _, err := readFD(1, make([]byte, 1)); !errors.Is(err, ErrReadinessUnsupported) {
		t.Fatalf("readFD error = %v, want ErrReadinessUnsupported", err)
	}
	if _, err := writeFD(1, []byte{1}); !errors.Is(err, ErrReadinessUnsupported) {
		t.Fatalf("writeFD error = %v, want ErrReadinessUnsupported", err)
	}

	poller := &fastPoller{}
	if err := poller.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	if err := poller.Init(); !errors.Is(err, errPollerAlreadyInitialized) {
		t.Fatalf("second Init error = %v, want errPollerAlreadyInitialized", err)
	}
	if err := poller.RegisterFD(1, EventRead, func(IOEvents) {}); !errors.Is(err, ErrReadinessUnsupported) {
		t.Fatalf("RegisterFD error = %v, want ErrReadinessUnsupported", err)
	}
	if err := poller.UnregisterFD(1); !errors.Is(err, ErrReadinessUnsupported) {
		t.Fatalf("UnregisterFD error = %v, want ErrReadinessUnsupported", err)
	}
	if err := poller.ModifyFD(1, EventWrite); !errors.Is(err, ErrReadinessUnsupported) {
		t.Fatalf("ModifyFD error = %v, want ErrReadinessUnsupported", err)
	}
	if err := poller.RegisterFD(-1, EventRead, func(IOEvents) {}); !errors.Is(err, errFDNegative) {
		t.Fatalf("RegisterFD negative error = %v, want errFDNegative", err)
	}
	if err := poller.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if err := poller.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
	if _, err := poller.PollIO(0); !errors.Is(err, errPollerClosed) {
		t.Fatalf("PollIO after Close error = %v, want errPollerClosed", err)
	}
	if err := poller.Init(); !errors.Is(err, errPollerClosed) {
		t.Fatalf("Init after Close error = %v, want errPollerClosed", err)
	}
}

func TestTaskOnlyFDUnsupportedDoesNotAdjustLiveness(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	if err := loop.RegisterFD(1, EventRead, func(IOEvents) {}); !errors.Is(err, ErrReadinessUnsupported) {
		t.Fatalf("RegisterFD err = %v, want ErrReadinessUnsupported", err)
	}
	if got := captureErrorContractPanic(func() { _ = loop.RegisterFD(1, 0, nil) }); got == nil {
		t.Fatal("RegisterFD invalid contract did not panic")
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount after failed RegisterFD = %d, want 0", got)
	}

	if err := loop.UnregisterFD(1); !errors.Is(err, ErrReadinessUnsupported) {
		t.Fatalf("UnregisterFD err = %v, want ErrReadinessUnsupported", err)
	}
	if err := loop.ModifyFD(1, EventWrite); !errors.Is(err, ErrReadinessUnsupported) {
		t.Fatalf("ModifyFD err = %v, want ErrReadinessUnsupported", err)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount after unsupported FD mutations = %d, want 0", got)
	}
	if loop.pollerReady.Load() {
		t.Fatal("unsupported FD operations initialized task-only poller resources")
	}
	if err := loop.Close(); err != nil {
		t.Fatal(err)
	}
	if got := captureErrorContractPanic(func() { _ = loop.RegisterFD(-1, 0, nil) }); got == nil {
		t.Fatal("terminated invalid RegisterFD did not panic")
	}
	if got := captureErrorContractPanic(func() { _ = loop.ModifyFD(-1, EventError) }); got == nil {
		t.Fatal("terminated invalid ModifyFD did not panic")
	}
}
