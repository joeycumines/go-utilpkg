package eventloop

import (
	"errors"
	"testing"
)

func TestJSTimerFacadesRejectTerminatedLoopWithoutResidue(t *testing.T) {
	loop := New()
	if err := loop.Close(); err != nil {
		t.Fatal(err)
	}
	js := NewJS(loop)

	if id, err := js.SetTimeout(func() { t.Fatal("terminated SetTimeout callback ran") }, 0); id != 0 || !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("SetTimeout = (%d, %v), want (0, ErrLoopTerminated)", id, err)
	}
	js.timeoutsMu.Lock()
	timeouts := len(js.timeouts)
	js.timeoutsMu.Unlock()
	if timeouts != 0 {
		t.Fatalf("timeout registry entries = %d, want 0", timeouts)
	}

	if id, err := js.SetInterval(func() { t.Fatal("terminated SetInterval callback ran") }, 0); id != 0 || !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("SetInterval = (%d, %v), want (0, ErrLoopTerminated)", id, err)
	}
	js.intervalsMu.Lock()
	intervals := len(js.intervals)
	js.intervalsMu.Unlock()
	if intervals != 0 {
		t.Fatalf("interval registry entries = %d, want 0", intervals)
	}
}

func TestJSQueueMicrotaskRejectsTerminatedLoop(t *testing.T) {
	loop := New()
	if err := loop.Close(); err != nil {
		t.Fatal(err)
	}
	js := NewJS(loop)
	if err := js.QueueMicrotask(func() { t.Fatal("terminated QueueMicrotask callback ran") }); !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("QueueMicrotask error = %v, want ErrLoopTerminated", err)
	}
}
