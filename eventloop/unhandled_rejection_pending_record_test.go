package eventloop

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestUnhandledRejectionPendingRecordDoesNotSpinCheckpoint(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	var firstReportedOnce sync.Once
	firstReported := make(chan struct{})

	js, err := NewJS(loop, WithUnhandledRejection(func(reason any) {
		if fmt.Sprint(reason) == "first" {
			firstReportedOnce.Do(func() { close(firstReported) })
		}
	}))
	if err != nil {
		t.Fatal(err)
	}

	_, _, rejectFirst := js.NewChainedPromise()
	rejectFirst("first")

	recordedSecond := make(chan struct{})
	releaseSecondReject := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseSecondReject) }) }
	t.Cleanup(release)

	loop.testHooks = &loopTestHooks{
		AfterPromiseRejectionRecorded: func() {
			select {
			case <-recordedSecond:
			default:
				close(recordedSecond)
			}
			<-releaseSecondReject
		},
	}

	_, _, rejectSecond := js.NewChainedPromise()
	secondDone := make(chan struct{})
	go func() {
		rejectSecond("second")
		close(secondDone)
	}()

	select {
	case <-recordedSecond:
	case <-time.After(time.Second):
		t.Fatal("second rejection did not reach recorded-before-published hook")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	select {
	case <-firstReported:
	case <-time.After(time.Second):
		release()
		cancel()
		t.Fatal("first unhandled rejection was not reported; checker did not start")
	}

	taskRan := make(chan struct{})
	if err := loop.Submit(func() {
		close(taskRan)
	}); err != nil {
		release()
		cancel()
		t.Fatalf("Submit failed unexpectedly: %v", err)
	}

	select {
	case <-taskRan:
	case <-time.After(100 * time.Millisecond):
		release()
		cancel()
		t.Fatal("loop appears stuck in unhandled-rejection checkpoint spin on a Pending rejection record")
	}

	release()

	select {
	case <-secondDone:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("second rejection did not complete after release")
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(time.Second):
		t.Fatal("loop did not exit after cancellation")
	}
}
