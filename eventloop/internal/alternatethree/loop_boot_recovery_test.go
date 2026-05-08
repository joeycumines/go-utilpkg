package alternatethree

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

// TestLoop_New_RepeatedUnstartedShutdown covers repeated construction and
// checked unstarted teardown. Poller initialization failure has no injection
// seam in this historical implementation and is not claimed here.
func TestLoop_New_RepeatedUnstartedShutdown(t *testing.T) {
	for i := range 3 {
		loop, err := New()
		if err != nil {
			t.Fatalf("New() iteration %d failed: %v", i, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), alternateThreeTestTimeout)
		err = loop.Shutdown(ctx)
		cancel()
		if err != nil {
			t.Fatalf("Shutdown() iteration %d failed: %v", i, err)
		}
	}
}

// TestLoop_Shutdown_Unstarted tests shutdown of a loop that was never started.
// This exercises the StateAwake -> Terminating -> Terminated path.
func TestLoop_Shutdown_Unstarted(t *testing.T) {
	// t.Parallel() // Cannot parallel: tests loop lifecycle

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), alternateThreeTestTimeout)
	defer cancel()

	// Shutdown should succeed for unstarted loop
	err = loop.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown() of unstarted loop failed: %v", err)
	}

	// Verify state is Terminated
	if LoopState(loop.state.Load()) != StateTerminated {
		t.Errorf("Expected StateTerminated after shutdown, got %v", LoopState(loop.state.Load()))
	}

	// Second shutdown should also succeed (idempotent)
	err = loop.Shutdown(ctx)
	if err != nil {
		t.Errorf("Second Shutdown() should be idempotent, got error: %v", err)
	}
}

// TestLoop_Shutdown_Idempotent tests that Shutdown can be called multiple times.
func TestLoop_Shutdown_Idempotent(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	harness := startAlternateThreeTestLoop(t, loop)
	harness.shutdown(t)

	// Second Shutdown should succeed (idempotent)
	ctx, cancel := context.WithTimeout(context.Background(), alternateThreeTestTimeout)
	defer cancel()
	err = loop.Shutdown(ctx)
	if err != nil {
		t.Errorf("Second Shutdown() should succeed, got error: %v", err)
	}

	// Verify state
	if LoopState(loop.state.Load()) != StateTerminated {
		t.Errorf("Expected StateTerminated, got %v", LoopState(loop.state.Load()))
	}
}

// TestLoop_Shutdown_ContextCanceled tests shutdown with context cancellation.
func TestLoop_Shutdown_ContextCanceled(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	harness := startAlternateThreeTestLoop(t, loop)
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseCallback := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseCallback)
	if err := loop.Submit(func() {
		close(entered)
		<-release
	}); err != nil {
		t.Fatalf("Submit(blocking callback) failed: %v", err)
	}
	waitAlternateThreeSignal(t, entered, "blocking callback entry")

	baseCtx, cancel := context.WithCancel(context.Background())
	shutdownCtx := newObservedContext(baseCtx)
	shutdownResult := make(chan error, 1)
	go func() { shutdownResult <- loop.Shutdown(shutdownCtx) }()
	waitAlternateThreeSignal(t, shutdownCtx.observed, "Shutdown context observation")
	cancel()
	select {
	case err := <-shutdownResult:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Shutdown() returned %v, want context.Canceled", err)
		}
	case <-time.After(alternateThreeTestTimeout):
		t.Fatal("canceled Shutdown() did not return")
	}
	releaseCallback()
	harness.wait(t)

	finalCtx, finalCancel := context.WithTimeout(context.Background(), alternateThreeTestTimeout)
	defer finalCancel()
	err = loop.Shutdown(finalCtx)
	if err != nil {
		t.Errorf("Final Shutdown() failed: %v", err)
	}
}

// TestLoop_Shutdown_LogicalSleepingState tests termination while the loop is
// held at its deterministic post-poll StateSleeping boundary. It does not
// claim to prove a wake from a kernel-blocked poll.
func TestLoop_Shutdown_LogicalSleepingState(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	loop.forceNonBlockingPoll = true
	pollReturned := make(chan struct{})
	releasePoll := make(chan struct{})
	var pollOnce sync.Once
	loop.testHooks = &loopTestHooks{PrePollAwake: func() {
		pollOnce.Do(func() { close(pollReturned) })
		<-releasePoll
	}}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releasePoll) }) }
	t.Cleanup(release)
	harness := startAlternateThreeTestLoop(t, loop)
	waitAlternateThreeSignal(t, pollReturned, "post-poll sleeping boundary")
	if state := LoopState(loop.state.Load()); state != StateSleeping {
		t.Fatalf("state at post-poll hook = %v, want StateSleeping", state)
	}

	baseCtx := t.Context()
	shutdownCtx := newObservedContext(baseCtx)
	shutdownResult := make(chan error, 1)
	go func() { shutdownResult <- loop.Shutdown(shutdownCtx) }()
	waitAlternateThreeSignal(t, shutdownCtx.observed, "Shutdown context observation")
	release()
	select {
	case err := <-shutdownResult:
		if err != nil {
			t.Fatalf("Shutdown() failed: %v", err)
		}
	case <-time.After(alternateThreeTestTimeout):
		t.Fatal("Shutdown() did not complete")
	}
	harness.wait(t)

	// Verify terminated state
	if LoopState(loop.state.Load()) != StateTerminated {
		t.Errorf("Expected StateTerminated, got %v", LoopState(loop.state.Load()))
	}
}

// TestLoop_Shutdown_TerminatingStateDoubleCall tests calling Shutdown when already terminating.
func TestLoop_Shutdown_TerminatingStateDoubleCall(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	harness := startAlternateThreeTestLoop(t, loop)
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseCallback := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseCallback)
	if err := loop.Submit(func() {
		close(entered)
		<-release
	}); err != nil {
		t.Fatalf("Submit(blocking callback) failed: %v", err)
	}
	waitAlternateThreeSignal(t, entered, "blocking callback entry")

	baseCtx, cancel := context.WithCancel(context.Background())
	ownerCtx := newObservedContext(baseCtx)
	ownerResult := make(chan error, 1)
	go func() { ownerResult <- loop.Shutdown(ownerCtx) }()
	waitAlternateThreeSignal(t, ownerCtx.observed, "owner Shutdown context observation")
	cancel()
	select {
	case err := <-ownerResult:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("owner Shutdown() returned %v, want context.Canceled", err)
		}
	case <-time.After(alternateThreeTestTimeout):
		t.Fatal("owner Shutdown() did not return after cancellation")
	}

	secondCtx, secondCancel := context.WithTimeout(context.Background(), alternateThreeTestTimeout)
	defer secondCancel()
	if err := loop.Shutdown(secondCtx); !errors.Is(err, ErrLoopTerminated) {
		t.Errorf("nonwinning Shutdown() returned %v, want ErrLoopTerminated", err)
	}
	releaseCallback()
	harness.wait(t)
	finalCtx, finalCancel := context.WithTimeout(context.Background(), alternateThreeTestTimeout)
	defer finalCancel()
	if err := loop.Shutdown(finalCtx); err != nil {
		t.Errorf("post-termination Shutdown() returned %v, want nil", err)
	}
}

// TestLoop_New_MultipleLiveInstances verifies independent descriptor ownership
// for a small deterministic group. It does not claim to exhaust the host limit.
func TestLoop_New_MultipleLiveInstances(t *testing.T) {
	const loopCount = 10
	loops := make([]*Loop, loopCount)
	for index := range loops {
		loop, err := New()
		if err != nil {
			t.Fatalf("New() instance %d failed: %v", index, err)
		}
		loops[index] = loop
	}
	for index, loop := range loops {
		ctx, cancel := context.WithTimeout(context.Background(), alternateThreeTestTimeout)
		err := loop.Shutdown(ctx)
		cancel()
		if err != nil {
			t.Errorf("Shutdown() instance %d failed: %v", index, err)
		}
	}
}

// TestLoop_New_EventFD_Linux tests eventfd creation on Linux.
// This is a platform-specific test.
func TestLoop_New_EventFD_Linux(t *testing.T) {
	// t.Parallel() // Cannot parallel: platform-specific

	if runtime.GOOS != "linux" {
		t.Skip("eventfd test only applicable on Linux")
	}

	// Just verify that New() works and creates proper eventfd
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), alternateThreeTestTimeout)
		defer cancel()
		if err := loop.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown() failed: %v", err)
		}
	})

	// On Linux, wakeFd and wakePipeWrite should be the same (eventfd)
	if loop.wakePipe != loop.wakePipeWrite {
		t.Error("On Linux, wakeFd and wakePipeWrite should be the same (eventfd)")
	}

	// Verify they're valid file descriptors
	if loop.wakePipe < 0 || loop.wakePipeWrite < 0 {
		t.Error("File descriptors should be non-negative")
	}
}

// TestLoop_New_SelfPipe_Darwin tests self-pipe creation on Darwin/Unix.
func TestLoop_New_SelfPipe_Darwin(t *testing.T) {
	// t.Parallel() // Cannot parallel: platform-specific

	if runtime.GOOS != "darwin" {
		t.Skip("self-pipe identity is Darwin-specific")
	}

	// On Darwin (and on Linux as fallback), should create a self-pipe
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), alternateThreeTestTimeout)
		defer cancel()
		if err := loop.Shutdown(ctx); err != nil {
			t.Errorf("Shutdown() failed: %v", err)
		}
	})

	// Verify file descriptors are valid
	if loop.wakePipe < 0 || loop.wakePipeWrite < 0 {
		t.Error("File descriptors should be non-negative")
	}

	if loop.wakePipe == loop.wakePipeWrite {
		t.Error("Darwin self-pipe read and write descriptors must differ")
	}
}

// TestLoop_CloseFDs_ErrorHandling tests that closeFDs handles errors gracefully.
func TestLoop_CloseFDs_ErrorHandling(t *testing.T) {
	// t.Parallel() // Cannot parallel: modifies system state

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Close the file descriptors manually to simulate an invalid FD
	if err := closeFD(loop.wakePipe); err != nil {
		t.Fatalf("closeFD(read) failed: %v", err)
	}
	if loop.wakePipe != loop.wakePipeWrite {
		if err := closeFD(loop.wakePipeWrite); err != nil {
			t.Fatalf("closeFD(write) failed: %v", err)
		}
	}

	// closeFDs should not panic even with invalid FDs
	loop.closeFDs()
}

// TestLoop_Run_ErrAlreadyRunning tests calling Run() multiple times.
func TestLoop_Run_ErrAlreadyRunning(t *testing.T) {
	// t.Parallel() // Cannot parallel: tests loop lifecycle

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	harness := startAlternateThreeTestLoop(t, loop)

	// Try to start second Run - should fail
	err = loop.Run(context.Background())
	if err != ErrLoopAlreadyRunning {
		t.Errorf("Expected ErrLoopAlreadyRunning, got: %v", err)
	}

	harness.shutdown(t)
}
