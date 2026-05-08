//go:build js && wasm

package alternatetwo

import (
	"errors"
	"testing"
)

func TestWASMFastPollerSemantics(t *testing.T) {
	if EventRead == 0 || EventWrite == 0 || EventError == 0 || EventHangup == 0 {
		t.Fatalf("WASM IO event bits must be non-zero: read=%d write=%d error=%d hangup=%d", EventRead, EventWrite, EventError, EventHangup)
	}
	if EventRead&EventWrite != 0 || EventRead&EventError != 0 || EventRead&EventHangup != 0 || EventWrite&EventError != 0 || EventWrite&EventHangup != 0 || EventError&EventHangup != 0 {
		t.Fatalf("WASM IO event bits overlap: read=%d write=%d error=%d hangup=%d", EventRead, EventWrite, EventError, EventHangup)
	}

	var poller FastPoller
	if err := poller.Wakeup(); err != nil {
		t.Fatalf("Wakeup before init = %v, want nil", err)
	}
	if err := poller.RegisterFD(1, EventRead, func(IOEvents) {}); !errors.Is(err, ErrPollerClosed) {
		t.Fatalf("RegisterFD before init = %v, want ErrPollerClosed", err)
	}
	if n, err := poller.PollIO(0); n != 0 || !errors.Is(err, ErrPollerClosed) {
		t.Fatalf("PollIO before init = (%d, %v), want (0, ErrPollerClosed)", n, err)
	}
	if err := poller.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := poller.RegisterFD(-1, EventRead, func(IOEvents) {}); !errors.Is(err, ErrFDOutOfRange) {
		t.Fatalf("RegisterFD negative = %v, want ErrFDOutOfRange", err)
	}
	if err := poller.RegisterFD(1, EventRead, func(IOEvents) {}); !errors.Is(err, ErrFDUnsupported) {
		t.Fatalf("RegisterFD initialized = %v, want ErrFDUnsupported", err)
	}
	if err := poller.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := poller.Wakeup(); !errors.Is(err, ErrPollerClosed) {
		t.Fatalf("Wakeup after close = %v, want ErrPollerClosed", err)
	}
}
