package eventloop

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestFastPath_FuzzModeTransitions exercises mode transitions with concurrent
// task submissions to verify no tasks are lost under any interleaving.
//
// This test specifically targets the TOCTOU race conditions in:
// - Fast path entry condition (line 407)
// - Submit() queue selection (line 968-995)
// - RegisterFD/UnregisterFD mode transitions
func TestFastPath_FuzzModeTransitions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping fuzz test in short mode")
	}

	const (
		iterations        = 100
		goroutines        = 20
		tasksPerGoroutine = 50
		modeSwitches      = 30
	)

	for iter := range iterations {
		loop, err := New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		runCh := make(chan error, 1)
		go func() {
			runCh <- loop.Run(ctx)
		}()

		// Wait for running
		deadline := time.Now().Add(time.Second)
		for loop.State() != StateRunning && time.Now().Before(deadline) {
			time.Sleep(10 * time.Microsecond)
		}
		if loop.State() != StateRunning {
			cancel()
			t.Fatalf("Iteration %d: Loop not running", iter)
		}

		var accepted, executed atomic.Int64
		var wg sync.WaitGroup

		// Concurrent submitters - mix of Submit and SubmitInternal
		for g := range goroutines {
			wg.Add(1)
			go func(gid int) {
				defer wg.Done()
				rng := rand.New(rand.NewSource(int64(iter*1000 + gid)))
				for range tasksPerGoroutine {
					var err error
					if rng.Intn(2) == 0 {
						err = loop.Submit(func() {
							executed.Add(1)
						})
					} else {
						err = loop.SubmitInternal(func() {
							executed.Add(1)
						})
					}
					if err == nil {
						accepted.Add(1)
					} else if err == ErrLoopTerminated {
						return
					}
					// Random yield to increase interleaving
					if rng.Intn(10) == 0 {
						time.Sleep(time.Microsecond * time.Duration(rng.Intn(100)))
					}
				}
			}(g)
		}

		// Concurrent mode switcher
		wg.Go(func() {
			rng := rand.New(rand.NewSource(int64(iter)))
			modes := []FastPathMode{FastPathAuto, FastPathDisabled, FastPathForced}
			for range modeSwitches {
				mode := modes[rng.Intn(len(modes))]
				_ = loop.SetFastPathMode(mode)
				time.Sleep(time.Microsecond * time.Duration(rng.Intn(200)))
			}
		})

		wg.Wait()

		// Give time for in-flight execution
		time.Sleep(100 * time.Millisecond)

		// Shutdown with fresh context
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := loop.Shutdown(shutdownCtx); err != nil && !errors.Is(err, ErrLoopTerminated) {
			t.Logf("Iteration %d: Shutdown: %v", iter, err)
		}
		shutdownCancel()
		<-runCh

		acc := accepted.Load()
		exec := executed.Load()
		if acc != exec {
			t.Fatalf("Iteration %d: DATA LOSS! Accepted %d, Executed %d (lost %d)",
				iter, acc, exec, acc-exec)
		}

		cancel()
	}
}

// TestFastPath_FuzzExternalQueueTransition specifically targets the race
// between poll->fast transition and tasks in l.external.
func TestFastPath_FuzzExternalQueueTransition(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping fuzz test in short mode")
	}

	const iterations = 200

	for iter := range iterations {
		loop, err := New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		// Start in poll mode
		_ = loop.SetFastPathMode(FastPathDisabled)

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

		runCh := make(chan error, 1)
		go func() {
			runCh <- loop.Run(ctx)
		}()

		// Wait for sleeping (poll mode)
		deadline := time.Now().Add(time.Second)
		for loop.State() != StateSleeping && time.Now().Before(deadline) {
			time.Sleep(100 * time.Microsecond)
		}

		// Submit tasks - they go to l.external
		var executed atomic.Int64
		const taskCount = 100
		for range taskCount {
			if err := loop.Submit(func() {
				executed.Add(1)
			}); err != nil {
				t.Fatalf("Iteration %d: Submit failed: %v", iter, err)
			}
		}

		// Immediately switch to fast path
		_ = loop.SetFastPathMode(FastPathAuto)

		// Wait for execution
		deadline = time.Now().Add(500 * time.Millisecond)
		for executed.Load() < taskCount && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}

		if executed.Load() != taskCount {
			t.Fatalf("Iteration %d: STARVATION! Only %d/%d tasks executed",
				iter, executed.Load(), taskCount)
		}

		if err := loop.Shutdown(context.Background()); err != nil && !errors.Is(err, ErrLoopTerminated) {
			t.Logf("Shutdown: %v", err)
		}
		<-runCh
		cancel()
	}
}

// TestFastPath_FuzzInternalDuringTransition verifies internal queue isn't
// starved during mode transitions.
func TestFastPath_FuzzInternalDuringTransition(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping fuzz test in short mode")
	}

	const iterations = 100

	for iter := range iterations {
		loop, err := New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

		runCh := make(chan error, 1)
		go func() {
			runCh <- loop.Run(ctx)
		}()

		// Wait for running
		for loop.State() != StateRunning {
			time.Sleep(100 * time.Microsecond)
		}

		var executed atomic.Int64
		const taskCount = 50
		var wg sync.WaitGroup

		// Submit internal tasks while switching modes
		wg.Go(func() {
			for range taskCount {
				if err := loop.SubmitInternal(func() {
					executed.Add(1)
				}); err == ErrLoopTerminated {
					break
				}
			}
		})

		// Rapidly switch modes
		wg.Go(func() {
			for i := range 20 {
				_ = loop.SetFastPathMode(FastPathMode(i % 3))
				time.Sleep(50 * time.Microsecond)
			}
		})

		wg.Wait()

		// Wait for execution
		deadline := time.Now().Add(500 * time.Millisecond)
		for executed.Load() < taskCount && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}

		if executed.Load() != taskCount {
			t.Fatalf("Iteration %d: STARVATION! Only %d/%d internal tasks executed",
				iter, executed.Load(), taskCount)
		}

		if err := loop.Shutdown(context.Background()); err != nil && !errors.Is(err, ErrLoopTerminated) {
			t.Logf("Shutdown: %v", err)
		}
		<-runCh
		cancel()
	}
}

// TestFastPath_FuzzMicrotasksDuringTransition verifies microtasks aren't
// starved during fast path transitions.
func TestFastPath_FuzzMicrotasksDuringTransition(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping fuzz test in short mode")
	}

	const iterations = 50

	for iter := range iterations {
		loop, err := New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

		runCh := make(chan error, 1)
		go func() {
			runCh <- loop.Run(ctx)
		}()

		// Wait for running
		for loop.State() != StateRunning {
			time.Sleep(100 * time.Microsecond)
		}

		var executed atomic.Int64
		const taskCount = 20
		const microtasksPerTask = 10
		expectedTotal := int64(taskCount * microtasksPerTask)

		done := make(chan struct{})

		// Submit tasks that schedule microtasks
		for range taskCount {
			if err := loop.Submit(func() {
				for range microtasksPerTask {
					_ = loop.ScheduleMicrotask(func() {
						if executed.Add(1) == expectedTotal {
							close(done)
						}
					})
				}
			}); err != nil {
				t.Fatalf("Iteration %d: Submit failed: %v", iter, err)
			}
		}

		// Rapid mode switching in parallel
		go func() {
			for i := range 15 {
				_ = loop.SetFastPathMode(FastPathMode(i % 3))
				time.Sleep(100 * time.Microsecond)
			}
		}()

		select {
		case <-done:
			// Success
		case <-time.After(5 * time.Second):
			t.Fatalf("Iteration %d: STARVATION! Only %d/%d microtasks executed",
				iter, executed.Load(), expectedTotal)
		}

		if err := loop.Shutdown(context.Background()); err != nil && !errors.Is(err, ErrLoopTerminated) {
			t.Logf("Shutdown: %v", err)
		}
		<-runCh
		cancel()
	}
}
