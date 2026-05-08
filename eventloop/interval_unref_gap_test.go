package eventloop

import (
	"testing"
	"time"
)

func TestUnrefIntervalAllowsAutoExit(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)
	js := NewJS(loop)

	fired := make(chan struct{}, 1)
	id, err := js.SetInterval(func() { fired <- struct{}{} }, int(time.Hour/time.Millisecond))
	if err != nil {
		t.Fatalf("SetInterval: %v", err)
	}
	if !loop.Alive() {
		t.Fatal("refed interval did not publish loop liveness")
	}
	if err := js.UnrefInterval(id); err != nil {
		t.Fatalf("UnrefInterval: %v", err)
	}
	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	select {
	case <-fired:
		t.Fatal("long unrefed interval fired before auto-exit")
	default:
	}
	if loop.Alive() || loop.State() != StateTerminated {
		t.Fatalf("loop after unrefed-interval auto-exit = (alive=%v, state=%v), want (false, StateTerminated)", loop.Alive(), loop.State())
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refedTimerCount = %d after auto-exit, want 0", got)
	}
	js.intervalsMu.RLock()
	_, retained := js.intervals[id]
	js.intervalsMu.RUnlock()
	if retained {
		t.Fatal("terminal cleanup retained unrefed interval handle")
	}
}
