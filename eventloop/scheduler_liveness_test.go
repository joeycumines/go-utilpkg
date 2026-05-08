package eventloop

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestFastModeFiniteTimerOverOneSecondFiresWithoutWakeup(t *testing.T) {
	loop := New(WithAutoExit(false))
	registerLoopCleanupT(t, loop)
	fired := make(chan struct{})
	if _, err := loop.ScheduleTimer(1100*time.Millisecond, func() { close(fired) }); err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()

	select {
	case <-fired:
	case <-time.After(2500 * time.Millisecond):
		t.Fatal("finite fast-mode timer >=1s did not fire without an unrelated wakeup")
	}
	if err := loop.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := waitContractValue(t, runDone, "finite-timer Run completion"); err != nil {
		t.Fatalf("Run after Close: %v", err)
	}
}

func TestIneligibleTimerDoesNotBlockEligibleDueTimer(t *testing.T) {
	loop := New()
	loop.state.Store(StateRunning)
	loop.tickCount = 10
	now := time.Now().Add(-time.Millisecond)
	var order []string
	ineligible := &timer{id: 1, when: now, task: func() { order = append(order, "ineligible") }, scheduledTick: 10, deferTick: true, heapIndex: -1}
	ineligible.refed.Store(true)
	eligible := &timer{id: 2, when: now.Add(time.Nanosecond), task: func() { order = append(order, "eligible") }, heapIndex: -1}
	eligible.refed.Store(true)
	loop.timerMap[ineligible.id] = ineligible
	loop.timerMap[eligible.id] = eligible
	loop.refedTimerCount.Store(2)
	pushTestTimer(loop, ineligible)
	pushTestTimer(loop, eligible)

	loop.runTimers()
	if !reflect.DeepEqual(order, []string{"eligible"}) {
		t.Fatalf("timer order = %v, want only eligible timer", order)
	}
	if _, ok := loop.timerMap[ineligible.id]; !ok {
		t.Fatal("ineligible timer was not preserved for a later tick")
	}
	if loop.refedTimerCount.Load() != 1 {
		t.Fatalf("refedTimerCount = %d, want 1", loop.refedTimerCount.Load())
	}
}

func TestSubmitFIFOAcrossFDModeTransition(t *testing.T) {
	loop := New()
	registerFDResourceCleanupT(t, loop)
	loop.state.Store(StateRunning)
	var order []string
	if err := loop.Submit(func() { order = append(order, "A") }); err != nil {
		t.Fatalf("Submit A: %v", err)
	}
	fd, cleanupFD := testCreateIOFD(t)
	defer cleanupFD()
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}
	if err := loop.Submit(func() { order = append(order, "B") }); err != nil {
		t.Fatalf("Submit B: %v", err)
	}
	loop.tick()
	if !reflect.DeepEqual(order, []string{"A", "B"}) {
		t.Fatalf("Submit order = %v, want [A B]", order)
	}
}

func TestRegisterFDReleasesLivenessLockBeforePostRegistrationRollbackCheck(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	fd, cleanupFD := testCreateIOFD(t)
	t.Cleanup(cleanupFD)

	hookRan := false
	terminalPublished := make(chan struct{})
	shutdownDone := make(chan error, 1)
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() { close(terminalPublished) },
		BeforeRegisterFDRollbackCheck: func() {
			hookRan = true
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				shutdownDone <- loop.Shutdown(ctx)
			}()
			waitContractSignal(t, terminalPublished, "Shutdown terminal-state publication")
		},
	}

	err := loop.RegisterFD(fd, EventRead, func(IOEvents) {})
	if !hookRan {
		t.Fatal("BeforeRegisterFDRollbackCheck hook did not run")
	}
	if !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("RegisterFD error = %v, want ErrLoopTerminated after hook shutdown", err)
	}
	select {
	case err := <-shutdownDone:
		if err != nil && !errors.Is(err, ErrLoopTerminated) {
			t.Fatalf("Shutdown during RegisterFD post-registration window: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Shutdown did not complete after RegisterFD rolled back")
	}
}

func TestCheckRefedPredicateCanCallLivenessAPIWhileQuiescing(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)

	var sawQuiescing bool
	var acceptedTimer bool
	var unexpectedErr error
	if err := loop.ScheduleImmediateRef(func() {}, func() bool {
		if loop.quiescing.Load() {
			sawQuiescing = true
			_, err := loop.ScheduleTimer(time.Millisecond, func() {})
			if err == nil {
				acceptedTimer = true
			} else if !errors.Is(err, ErrLoopTerminated) {
				unexpectedErr = err
			}
		}
		return false
	}); err != nil {
		t.Fatalf("ScheduleImmediateRef: %v", err)
	}

	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !sawQuiescing {
		t.Fatal("liveness predicate was not evaluated during auto-exit quiescing")
	}
	if acceptedTimer {
		t.Fatal("ScheduleTimer unexpectedly succeeded during auto-exit quiescing")
	}
	if unexpectedErr != nil {
		t.Fatalf("ScheduleTimer during quiescing returned %v, want ErrLoopTerminated", unexpectedErr)
	}
}

func TestCommandImmediateRefedPredicateCanCallLivenessAPIWhileCommittingAutoExit(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)

	var enqueued bool
	var predicateCalls int
	var scheduleErr error
	var hookErr error
	loop.testHooks = &loopTestHooks{AfterAutoExitFinalAliveCheck: func() {
		if enqueued {
			return
		}
		enqueued = true
		hookErr = loop.ScheduleImmediateRef(func() {}, func() bool {
			predicateCalls++
			_, err := loop.ScheduleTimer(time.Millisecond, func() {})
			if err != nil && scheduleErr == nil {
				scheduleErr = err
			}
			return false
		})
	}}

	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if hookErr != nil {
		t.Fatalf("ScheduleImmediateRef from auto-exit hook: %v", hookErr)
	}
	if predicateCalls == 0 {
		t.Fatal("pending immediate command liveness predicate was not evaluated")
	}
	if scheduleErr != nil {
		t.Fatalf("ScheduleTimer from pending command predicate returned %v, want nil", scheduleErr)
	}
}

func TestPreRunUnrefTimerDoesNotKeepAutoExitAlive(t *testing.T) {
	loop := New(WithAutoExit(true))
	fired := make(chan struct{}, 1)
	id, err := loop.ScheduleTimer(time.Hour, func() { fired <- struct{}{} })
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	if err := loop.UnrefTimer(id); err != nil {
		t.Fatalf("UnrefTimer before Run: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("unref'd pre-Run timer kept auto-exit loop alive")
	}
	select {
	case <-fired:
		t.Fatal("unref'd long timer fired during auto-exit")
	default:
	}
}
