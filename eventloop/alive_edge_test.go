package eventloop

import (
	"context"
	"testing"
)

// TestAlive_AfterAllTimersFire exercises the refedTimerCount check (line 1455)
// transitioning from >0 to 0 after timers fire.
func TestAlive_AfterAllTimersFire(t *testing.T) {
	loop := New()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(t.Context()) }()
	waitLoopOwnerTurnT(t, loop)

	fired := make(chan struct{})
	_, err := loop.ScheduleTimer(0, func() { close(fired) })
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	waitContractSignal(t, fired, "timer callback")

	alive := make(chan bool, 1)
	if err := loop.SubmitInternal(func() { alive <- loop.Alive() }); err != nil {
		t.Fatalf("SubmitInternal post-timer observation: %v", err)
	}
	if waitContractValue(t, alive, "post-timer liveness") {
		t.Error("Alive() should return false after all timers have fired")
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "post-timer Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// TestAlive_PromisifyDuringExecution exercises Alive() from within a Promisify
// callback (line 1473: promisifyCount > 0). Verifies no deadlock and returns true.
func TestAlive_PromisifyDuringExecution(t *testing.T) {
	loop := New()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitLoopOwnerTurnT(t, loop)

	aliveResult := make(chan bool, 1)
	const wantResult = "alive callback result"
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		aliveResult <- loop.Alive()
		return wantResult, nil
	})
	if !waitContractValue(t, aliveResult, "Promisify Alive observation") {
		t.Error("Alive() should return true from within a Promisify callback (promisifyCount > 0)")
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "Promisify Alive Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if state := promise.State(); state != Fulfilled {
		t.Fatalf("Promisify state = %v, want Fulfilled", state)
	}
	if result := promise.Result(); result != wantResult {
		t.Fatalf("Promisify result = %#v, want %q", result, wantResult)
	}
}
