//go:build js && wasm

package alternatethree

import (
	"errors"
	"testing"
)

func TestWASMIOPollerSemantics(t *testing.T) {
	if EventRead == 0 || EventWrite == 0 || EventError == 0 || EventHangup == 0 {
		t.Fatalf("WASM IO event bits must be non-zero: read=%d write=%d error=%d hangup=%d", EventRead, EventWrite, EventError, EventHangup)
	}
	if EventRead&EventWrite != 0 || EventRead&EventError != 0 || EventRead&EventHangup != 0 || EventWrite&EventError != 0 || EventWrite&EventHangup != 0 || EventError&EventHangup != 0 {
		t.Fatalf("WASM IO event bits overlap: read=%d write=%d error=%d hangup=%d", EventRead, EventWrite, EventError, EventHangup)
	}

	loop := &Loop{}
	if n, err := loop.pollIO(0, 1); n != 0 || err != nil {
		t.Fatalf("pollIO before init = (%d, %v), want (0, nil)", n, err)
	}
	if err := loop.UnregisterFD(1); !errors.Is(err, errPollerNotInitialized) {
		t.Fatalf("UnregisterFD before init = %v, want errPollerNotInitialized", err)
	}
	if err := loop.ModifyFD(1, EventRead); !errors.Is(err, errPollerNotInitialized) {
		t.Fatalf("ModifyFD before init = %v, want errPollerNotInitialized", err)
	}
	if err := loop.ioPoller.closePoller(); err != nil {
		t.Fatalf("closePoller before init: %v", err)
	}
	if err := loop.Wake(); err != nil {
		t.Fatalf("Wakeup after close-before-init = %v, want nil", err)
	}
	if err := loop.RegisterFD(1, EventRead, func(IOEvents) {}); !errors.Is(err, errFDUnsupported) {
		t.Fatalf("RegisterFD = %v, want errFDUnsupported", err)
	}
	if !loop.ioPoller.initialized.Load() {
		t.Fatal("RegisterFD should initialize the WASM ioPoller before reporting unsupported FDs")
	}
	if n, err := loop.pollIO(0, 1); n != 0 || err != nil {
		t.Fatalf("pollIO after unsupported RegisterFD = (%d, %v), want (0, nil)", n, err)
	}
	if err := loop.ioPoller.closePoller(); err != nil {
		t.Fatalf("closePoller: %v", err)
	}
	if n, err := loop.pollIO(0, 1); n != 0 || !errors.Is(err, errEventLoopClosed) {
		t.Fatalf("pollIO after close = (%d, %v), want (0, errEventLoopClosed)", n, err)
	}
}
