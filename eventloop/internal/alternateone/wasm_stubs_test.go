//go:build js && wasm

package alternateone

import (
	"errors"
	"testing"
)

func TestWASMSafePollerSemantics(t *testing.T) {
	if EventRead == 0 || EventWrite == 0 || EventError == 0 || EventHangup == 0 {
		t.Fatalf("WASM IO event bits must be non-zero: read=%d write=%d error=%d hangup=%d", EventRead, EventWrite, EventError, EventHangup)
	}
	if EventRead&EventWrite != 0 || EventRead&EventError != 0 || EventRead&EventHangup != 0 || EventWrite&EventError != 0 || EventWrite&EventHangup != 0 || EventError&EventHangup != 0 {
		t.Fatalf("WASM IO event bits overlap: read=%d write=%d error=%d hangup=%d", EventRead, EventWrite, EventError, EventHangup)
	}

	var poller SafePoller
	if n, err := poller.PollIO(0); n != 0 || err != nil {
		t.Fatalf("PollIO before init = (%d, %v), want (0, nil)", n, err)
	}
	if err := poller.Wakeup(); err != nil {
		t.Fatalf("Wakeup before init = %v, want nil", err)
	}
	if err := poller.UnregisterFD(1); !errors.Is(err, ErrPollerNotInitialized) {
		t.Fatalf("UnregisterFD before init = %v, want ErrPollerNotInitialized", err)
	}
	if err := poller.ModifyFD(1, EventRead); !errors.Is(err, ErrPollerNotInitialized) {
		t.Fatalf("ModifyFD before init = %v, want ErrPollerNotInitialized", err)
	}
	if err := poller.RegisterFD(1, EventRead, func(IOEvents) {}); !errors.Is(err, ErrFDUnsupported) {
		t.Fatalf("RegisterFD = %v, want ErrFDUnsupported", err)
	}
	if !poller.initialized {
		t.Fatal("RegisterFD should initialize the WASM SafePoller before reporting unsupported FDs")
	}
	if err := poller.closePoller(); err != nil {
		t.Fatalf("closePoller: %v", err)
	}
	if err := poller.Wakeup(); !errors.Is(err, ErrPollerClosed) {
		t.Fatalf("Wakeup after close = %v, want ErrPollerClosed", err)
	}
}
