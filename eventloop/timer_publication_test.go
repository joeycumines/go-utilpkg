package eventloop

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestScheduleTimerPublicationPrecedesCallback(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	returnHookEntered := make(chan TimerID, 1)
	callbackWaiting := make(chan TimerID, 1)
	releaseReturnHook := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releaseReturnHook) })
	loop.testHooks = &loopTestHooks{
		BeforeScheduleTimerReturn: func(id TimerID) {
			returnHookEntered <- id
			<-releaseReturnHook
		},
		BeforeTimerPublicationWait: func(id TimerID) { callbackWaiting <- id },
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	registerActiveLoopCleanupT(t, loop, runDone)
	waitLoopOwnerTurnT(t, loop)

	callbackRan := make(chan struct{})
	type scheduleResult struct {
		id  TimerID
		err error
	}
	result := make(chan scheduleResult, 1)
	go func() {
		id, err := loop.ScheduleTimer(0, func() { close(callbackRan) })
		result <- scheduleResult{id: id, err: err}
	}()

	var admittedID TimerID
	select {
	case admittedID = <-returnHookEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("ScheduleTimer did not reach its pre-return publication hook")
	}
	select {
	case waitingID := <-callbackWaiting:
		if waitingID != admittedID {
			t.Fatalf("callback timer ID = %d, want admitted ID %d", waitingID, admittedID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timer callback did not reach its publication wait")
	}
	select {
	case got := <-result:
		t.Fatalf("ScheduleTimer returned before publication release: %+v", got)
	default:
	}
	select {
	case <-callbackRan:
		t.Fatal("timer callback entered before ScheduleTimer released publication")
	default:
	}

	releaseOnce.Do(func() { close(releaseReturnHook) })
	select {
	case got := <-result:
		if got.err != nil {
			t.Fatalf("ScheduleTimer = %v", got.err)
		}
		if got.id != admittedID {
			t.Fatalf("ScheduleTimer ID = %d, want admitted ID %d", got.id, admittedID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ScheduleTimer did not return after publication release")
	}
	select {
	case <-callbackRan:
	case <-time.After(5 * time.Second):
		t.Fatal("timer callback did not run after publication release")
	}
}
