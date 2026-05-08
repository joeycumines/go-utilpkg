package eventloop

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestAlive_ShutdownClearsPendingTimer verifies that Shutdown clears pending
// timer liveness so Alive() reports false after termination even when a timer
// would otherwise still be pending far in the future.
func TestAlive_ShutdownClearsPendingTimer(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitLoopOwnerTurnT(t, loop)

	_, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	waitRefedTimerCount(t, loop, 1)

	if !loop.Alive() {
		t.Fatal("Alive() should return true before shutdown with a pending ref'd timer")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := loop.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if runErr := waitContractValue(t, runDone, "Shutdown Run completion"); runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}

	if loop.Alive() {
		t.Fatal("Alive() should return false after shutdown with pending timer state cleared")
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refedTimerCount should be 0 after shutdown, got %d", got)
	}
	if got := len(loop.timers); got != 0 {
		t.Fatalf("timers heap should be empty after shutdown, got %d entries", got)
	}
	if got := len(loop.timerMap); got != 0 {
		t.Fatalf("timerMap should be empty after shutdown, got %d entries", got)
	}
}

// TestAlive_CloseClearsPendingTimerAndFD verifies that Close clears all
// remaining liveness signals for the running-loop path, including pending
// timers and registered user I/O FDs.
func TestAlive_CloseClearsPendingTimerAndFD(t *testing.T) {
	loop := New()

	fd, fdCleanup := testCreateIOFD(t)
	t.Cleanup(fdCleanup)
	registerLoopCleanupT(t, loop)
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitLoopOwnerTurnT(t, loop)

	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	_, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	waitRefedTimerCount(t, loop, 1)

	if !loop.Alive() {
		t.Fatal("Alive() should return true before Close with pending timer and registered FD")
	}

	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if runErr := waitContractValue(t, runDone, "Close Run completion"); runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}

	if loop.Alive() {
		t.Fatal("Alive() should return false after Close with pending timer and FD state cleared")
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refedTimerCount should be 0 after Close, got %d", got)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount should be 0 after Close, got %d", got)
	}
	if got := len(loop.timers); got != 0 {
		t.Fatalf("timers heap should be empty after Close, got %d entries", got)
	}
	if got := len(loop.timerMap); got != 0 {
		t.Fatalf("timerMap should be empty after Close, got %d entries", got)
	}
}

func TestAliveContextCancellationClearsRegisteredFD(t *testing.T) {
	loop := New()
	fd, fdCleanup := testCreateIOFD(t)
	t.Cleanup(fdCleanup)
	registerLoopCleanupT(t, loop)
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}
	if got := loop.userIOFDCount.Load(); got != 1 {
		t.Fatalf("userIOFDCount before Run = %d, want 1", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitLoopOwnerTurnT(t, loop)
	cancel()
	if runErr := waitContractValue(t, runDone, "context-canceled Run completion"); !errors.Is(runErr, context.Canceled) {
		t.Fatalf("Run = %v, want context.Canceled", runErr)
	}

	if state := loop.State(); state != StateTerminated {
		t.Fatalf("state after context cancellation = %v, want StateTerminated", state)
	}
	if loop.Alive() {
		t.Fatal("Alive returned true after context cancellation")
	}
	if got := loop.refedTimerCount.Load(); got != 0 {
		t.Fatalf("refedTimerCount after context cancellation = %d, want 0", got)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount after context cancellation = %d, want 0", got)
	}
}

// TestAlive_CloseBeforeRunClearsQueuedWork verifies that Close on a loop that
// never started still clears queued work so Alive() becomes false immediately.
func TestAlive_CloseBeforeRunClearsQueuedWork(t *testing.T) {
	loop := New()

	if _, err := loop.ScheduleTimer(time.Hour, func() {}); err != nil {
		t.Fatalf("ScheduleTimer before Run: %v", err)
	}
	if err := loop.ScheduleMicrotask(func() {}); err != nil {
		t.Fatalf("ScheduleMicrotask before Run: %v", err)
	}
	if err := loop.ScheduleNextTick(func() {}); err != nil {
		t.Fatalf("ScheduleNextTick before Run: %v", err)
	}
	if err := loop.Submit(func() {}); err != nil {
		t.Fatalf("Submit before Run: %v", err)
	}

	if !loop.Alive() {
		t.Fatal("Alive() should return true with queued pre-Run work")
	}

	if err := loop.Close(); err != nil {
		t.Fatalf("Close before Run: %v", err)
	}

	if loop.Alive() {
		t.Fatal("Alive() should return false after Close clears pre-Run queued work")
	}
	loop.externalMu.Lock()
	commandLen := loop.commands.Len()
	loop.externalMu.Unlock()
	if commandLen != 0 {
		t.Fatalf("command ingress should be empty after Close, got %d entries", commandLen)
	}
	if internal := loop.ownerInternalCount.Load(); internal != 0 {
		t.Fatalf("owner internal queue should be empty after Close, got %d entries", internal)
	}
	if external := loop.ownerExternalCount.Load(); external != 0 {
		t.Fatalf("owner external queue should be empty after Close, got %d entries", external)
	}
	if loop.ownerMicroCount.Load() != 0 || loop.ingressMicroCount.Load() != 0 {
		t.Fatalf("microtask queues should be empty after Close, owner=%d ingress=%d", loop.ownerMicroCount.Load(), loop.ingressMicroCount.Load())
	}
}
