package eventloop

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCloseWakesPollSleepingLoopWithRegisteredFD(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	fd, cleanupFD := testCreateIOFD(t)
	defer cleanupFD()
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}
	pollReached := make(chan struct{})
	var pollOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforePollIO: func() { pollOnce.Do(func() { close(pollReached) }) },
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, pollReached, "initial native poll")

	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		cleanupFD()
		t.Fatal("Close did not wake a poll-sleeping loop with a registered FD")
	}

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error after Close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after Close")
	}
}

func TestHandlePollErrorDoesNotWaitPromisifyOnLoopGoroutine(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)

	releasePromisify := make(chan struct{})
	releaseWorker := releaseSignalT(t, releasePromisify)
	promisifyStarted := make(chan struct{})
	promisifyReturned := make(chan struct{})
	promise := loop.Promisify(context.Background(), func(context.Context) (any, error) {
		close(promisifyStarted)
		<-releasePromisify
		close(promisifyReturned)
		return "released", nil
	})
	select {
	case <-promisifyStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("Promisify function did not start")
	}

	loop.pushOwnerExternal(releaseWorker)
	loop.state.Store(StateSleeping)

	handleDone := make(chan struct{})
	go func() {
		loop.handlePollError(errors.New("injected poll failure"))
		close(handleDone)
	}()
	select {
	case <-handleDone:
	case <-time.After(5 * time.Second):
		releaseWorker()
		t.Fatal("poll-error shutdown waited on Promisify before draining the release callback")
	}
	select {
	case <-promisifyReturned:
	case <-time.After(5 * time.Second):
		t.Fatal("poll-error terminal drain did not release the Promisify function")
	}

	result := waitContractValue(t, promise.ToChannel(), "poll-error Promisify settlement")
	if state := promise.State(); state != Fulfilled {
		t.Fatalf("Promisify promise state = %v, want Fulfilled; result=%v", state, result)
	}
	if result != "released" {
		t.Fatalf("Promisify promise result = %v, want released", result)
	}
}
