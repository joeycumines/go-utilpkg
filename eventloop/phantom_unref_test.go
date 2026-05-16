package eventloop

import (
	"testing"
	"time"
)

type ownerTimerUnrefResult struct {
	timerID     TimerID
	scheduleErr error
	unrefErr    error
	refCount    int64
}

type ownerTimerCancelResult struct {
	timerID     TimerID
	scheduleErr error
	cancelErr   error
	present     bool
	refCount    int64
}

func TestIOModeOwnerScheduleThenUnref(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	fd, cleanupFD := testCreateIOFD(t)
	t.Cleanup(cleanupFD)
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}
	stop := startCancelableLoopT(t, loop)

	result := make(chan ownerTimerUnrefResult, 1)
	if err := loop.SubmitInternal(func() {
		value := ownerTimerUnrefResult{}
		value.timerID, value.scheduleErr = loop.ScheduleTimer(time.Hour, func() {})
		if value.scheduleErr == nil {
			value.unrefErr = loop.UnrefTimer(value.timerID)
		}
		value.refCount = loop.refedTimerCount.Load()
		result <- value
	}); err != nil {
		t.Fatalf("SubmitInternal: %v", err)
	}
	value := waitContractValue(t, result, "owner timer schedule and unref")
	if value.timerID == 0 || value.scheduleErr != nil || value.unrefErr != nil || value.refCount != 0 {
		t.Fatalf("owner timer result = (id=%d, schedule=%v, unref=%v, refs=%d), want (nonzero, nil, nil, 0)", value.timerID, value.scheduleErr, value.unrefErr, value.refCount)
	}
	stop()
}

func TestIOModeOwnerScheduleThenCancel(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	fd, cleanupFD := testCreateIOFD(t)
	t.Cleanup(cleanupFD)
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}
	stop := startCancelableLoopT(t, loop)

	result := make(chan ownerTimerCancelResult, 1)
	if err := loop.SubmitInternal(func() {
		value := ownerTimerCancelResult{}
		value.timerID, value.scheduleErr = loop.ScheduleTimer(time.Hour, func() {})
		if value.scheduleErr == nil {
			value.cancelErr = loop.CancelTimer(value.timerID)
		}
		_, value.present = loop.timerMap[value.timerID]
		value.refCount = loop.refedTimerCount.Load()
		result <- value
	}); err != nil {
		t.Fatalf("SubmitInternal: %v", err)
	}
	value := waitContractValue(t, result, "owner timer schedule and cancellation")
	if value.timerID == 0 || value.scheduleErr != nil || value.cancelErr != nil || value.present || value.refCount != 0 {
		t.Fatalf("owner timer cancellation = (id=%d, schedule=%v, cancel=%v, present=%v, refs=%d), want (nonzero, nil, nil, false, 0)", value.timerID, value.scheduleErr, value.cancelErr, value.present, value.refCount)
	}
	stop()
}

func TestCancelOwnerUnrefedTimer(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	fd, cleanupFD := testCreateIOFD(t)
	t.Cleanup(cleanupFD)
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}
	stop := startCancelableLoopT(t, loop)

	setup := make(chan ownerTimerUnrefResult, 1)
	if err := loop.SubmitInternal(func() {
		value := ownerTimerUnrefResult{}
		value.timerID, value.scheduleErr = loop.ScheduleTimer(time.Hour, func() {})
		if value.scheduleErr == nil {
			value.unrefErr = loop.UnrefTimer(value.timerID)
		}
		value.refCount = loop.refedTimerCount.Load()
		setup <- value
	}); err != nil {
		t.Fatalf("SubmitInternal: %v", err)
	}
	value := waitContractValue(t, setup, "owner unrefed timer setup")
	if value.timerID == 0 || value.scheduleErr != nil || value.unrefErr != nil || value.refCount != 0 {
		t.Fatalf("owner timer setup = (id=%d, schedule=%v, unref=%v, refs=%d), want (nonzero, nil, nil, 0)", value.timerID, value.scheduleErr, value.unrefErr, value.refCount)
	}
	if err := loop.CancelTimer(value.timerID); err != nil {
		t.Fatalf("CancelTimer: %v", err)
	}

	type observation struct {
		present  bool
		refCount int64
	}
	observed := make(chan observation, 1)
	if err := loop.SubmitInternal(func() {
		_, present := loop.timerMap[value.timerID]
		observed <- observation{present: present, refCount: loop.refedTimerCount.Load()}
	}); err != nil {
		t.Fatalf("post-cancel observation: %v", err)
	}
	final := waitContractValue(t, observed, "owner timer cancellation state")
	if final.present || final.refCount != 0 {
		t.Fatalf("canceled owner timer = (present=%v, refs=%d), want (false, 0)", final.present, final.refCount)
	}
	stop()
}
