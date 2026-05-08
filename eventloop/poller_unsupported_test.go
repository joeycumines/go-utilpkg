//go:build plan9 || windows || ((js || wasip1) && wasm)

package eventloop

import (
	"errors"
	"testing"
)

func TestTaskOnlyPollerLifecycle(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := poller.Init(); !errors.Is(err, errPollerAlreadyInitialized) {
		t.Fatalf("second Init = %v, want errPollerAlreadyInitialized", err)
	}
	if err := poller.RegisterFD(1, EventRead, func(IOEvents) {}); !errors.Is(err, ErrReadinessUnsupported) {
		t.Fatalf("RegisterFD = %v, want ErrReadinessUnsupported", err)
	}
	if err := poller.RegisterFD(-1, EventRead, func(IOEvents) {}); !errors.Is(err, errFDNegative) {
		t.Fatalf("RegisterFD negative = %v, want errFDNegative", err)
	}
	if err := poller.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := poller.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := poller.PollIO(0); !errors.Is(err, errPollerClosed) {
		t.Fatalf("PollIO after Close = %v, want errPollerClosed", err)
	}
}

func TestTaskOnlyLoopReadinessUnsupported(t *testing.T) {
	tests := []struct {
		name string
		new  func() *Loop
	}{
		{name: "default", new: func() *Loop { return New() }},
		{name: "disabled", new: func() *Loop { return New(WithFastPathMode(FastPathDisabled)) }},
		{name: "forced", new: func() *Loop { return New(WithFastPathMode(FastPathForced)) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop := test.new()
			registerLoopCleanupT(t, loop)
			if err := loop.RegisterFD(42, EventRead, func(IOEvents) {}); !errors.Is(err, ErrReadinessUnsupported) {
				t.Fatalf("RegisterFD = %v, want ErrReadinessUnsupported", err)
			}
			if err := loop.UnregisterFD(42); !errors.Is(err, ErrReadinessUnsupported) {
				t.Fatalf("UnregisterFD = %v, want ErrReadinessUnsupported", err)
			}
			if err := loop.ModifyFD(42, EventWrite); !errors.Is(err, ErrReadinessUnsupported) {
				t.Fatalf("ModifyFD = %v, want ErrReadinessUnsupported", err)
			}
			if loop.pollerReady.Load() {
				t.Fatal("task-only loop initialized readiness resources")
			}
			if err := loop.SetFastPathMode(FastPathDisabled); err != nil {
				t.Fatalf("SetFastPathMode(FastPathDisabled): %v", err)
			}
			if loop.pollerReady.Load() {
				t.Fatal("dynamic fast-path disable initialized readiness resources")
			}
		})
	}
}

func TestTaskOnlyReadinessStaticContractsPanic(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	for name, call := range map[string]func(){
		"register": func() { _ = loop.RegisterFD(1, 0, nil) },
		"modify":   func() { _ = loop.ModifyFD(1, EventError) },
	} {
		t.Run(name, func(t *testing.T) {
			if got := captureErrorContractPanic(call); got == nil {
				t.Fatal("static readiness contract did not panic")
			}
		})
	}
}
