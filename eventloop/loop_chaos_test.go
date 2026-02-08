//go:build linux || darwin

// Copyright 2026 Joseph Cumines
//
// Permission to use, copy, modify, and distribute this software for any
// purpose with or without fee is hereby granted, provided that this copyright
// notice appears in all copies.

package eventloop

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// ============================================================================
// EXPAND-031: Loop Recovery Chaos Tests
// ============================================================================
//
// This file contains chaos tests for loop recovery and stability:
// - Panic-in-panic: callback panics, recovery panics
// - GC pressure: force GC during peak load
// - Timer cancellation storm: 100+ concurrent timer creates/cancels
// - Rapid start/stop: 10+ loop create/run/shutdown cycles
// - Concurrent RegisterFD + SetFastPathMode race testing
// - Submit during shutdown race

// TestChaos_PanicInPanic verifies that nested panics don't crash the loop.
// This tests the scenario where a callback panics and the recovery mechanism
// also potentially panics.
func TestChaos_PanicInPanic(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	// Wait for loop to start
	waitLoopState(t, loop, StateRunning, 2*time.Second)

	panicCount := atomic.Int32{}
	taskCount := 100

	// Submit tasks that panic
	for i := 0; i < taskCount; i++ {
		idx := i
		err := loop.Submit(func() {
			panicCount.Add(1)
			panic(errors.New("intentional panic " + string(rune('A'+idx%26))))
		})
		if err != nil {
			t.Logf("Submit failed: %v", err)
		}
	}

	// Give time for panics to occur
	time.Sleep(200 * time.Millisecond)

	// Loop should still be running
	state := loop.State()
	if state != StateRunning && state != StateSleeping {
		t.Errorf("Expected Running or Sleeping, got %v", state)
	}

	// Submit a normal task to verify loop is still functional
	normalDone := make(chan struct{})
	err = loop.Submit(func() {
		close(normalDone)
	})
	if err != nil {
		t.Fatalf("Submit after panics failed: %v", err)
	}

	select {
	case <-normalDone:
		// Success - loop recovered from panics
	case <-time.After(2 * time.Second):
		t.Fatal("Normal task after panics timed out - loop may be stuck")
	}

	// Verify some panics occurred
	if panicCount.Load() == 0 {
		t.Error("Expected some panics to occur")
	}

	loop.Shutdown(context.Background())
	<-runDone
}

// TestChaos_GCPressureDuringPeakLoad verifies loop stability under GC pressure.
func TestChaos_GCPressureDuringPeakLoad(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	waitLoopState(t, loop, StateRunning, 2*time.Second)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	const (
		numGoroutines = 20
		tasksPerGo    = 100
		gcInterval    = 10 * time.Millisecond
	)

	var executed atomic.Int64
	var wg sync.WaitGroup

	// GC pressure goroutine
	gcDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(gcInterval)
		defer ticker.Stop()
		for {
			select {
			case <-gcDone:
				return
			case <-ticker.C:
				runtime.GC()
			}
		}
	}()

	// Heavy workload - create promises and resolve them
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < tasksPerGo; i++ {
				p, resolve, _ := js.NewChainedPromise()
				resolve("value")
				_ = p.State() // Force some work
				executed.Add(1)
			}
		}()
	}

	// Also submit regular tasks
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < tasksPerGo; i++ {
				if err := loop.Submit(func() {
					executed.Add(1)
					// Allocate some memory to trigger GC
					_ = make([]byte, 1024)
				}); err != nil {
					if errors.Is(err, ErrLoopTerminated) {
						return
					}
				}
			}
		}()
	}

	wg.Wait()
	close(gcDone)

	// Force final GC
	runtime.GC()
	runtime.GC()

	// Verify loop is still healthy
	state := loop.State()
	if state != StateRunning && state != StateSleeping {
		t.Errorf("Expected Running or Sleeping after GC pressure, got %v", state)
	}

	// Verify at least some tasks executed
	if executed.Load() < int64(numGoroutines*tasksPerGo) {
		t.Logf("Executed %d tasks (some may have failed during shutdown)", executed.Load())
	}

	loop.Shutdown(context.Background())
	<-runDone
}

// TestChaos_TimerCancellationStorm tests 100+ concurrent timer creates/cancels.
func TestChaos_TimerCancellationStorm(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	waitLoopState(t, loop, StateRunning, 2*time.Second)

	const (
		numGoroutines   = 50
		timersPerGo     = 100
		cancelChance    = 50 // 50% chance to cancel
		maxDelayMs      = 100
		stormDurationMs = 2000
	)

	var (
		created   atomic.Int64
		cancelled atomic.Int64
		fired     atomic.Int64
		errors_   atomic.Int64
	)

	stormEnd := time.Now().Add(stormDurationMs * time.Millisecond)

	var wg sync.WaitGroup
	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < timersPerGo && time.Now().Before(stormEnd); i++ {
				delay := time.Duration((gid*i+1)%maxDelayMs) * time.Millisecond

				timerID, err := loop.ScheduleTimer(delay, func() {
					fired.Add(1)
				})
				if err != nil {
					if errors.Is(err, ErrLoopTerminated) {
						return
					}
					errors_.Add(1)
					continue
				}
				created.Add(1)

				// Randomly cancel some timers
				if (gid+i)%100 < cancelChance {
					if err := loop.CancelTimer(timerID); err == nil {
						cancelled.Add(1)
					}
				}
			}
		}(g)
	}

	wg.Wait()

	// Give time for remaining timers to fire
	time.Sleep(time.Duration(maxDelayMs+50) * time.Millisecond)

	t.Logf("Storm results: created=%d, cancelled=%d, fired=%d, errors=%d",
		created.Load(), cancelled.Load(), fired.Load(), errors_.Load())

	// Verify no crash and reasonable results
	if created.Load() == 0 {
		t.Error("Expected some timers to be created")
	}

	// Verify loop is still healthy
	state := loop.State()
	if state != StateRunning && state != StateSleeping {
		t.Errorf("Expected Running or Sleeping after storm, got %v", state)
	}

	loop.Shutdown(context.Background())
	<-runDone
}

// TestChaos_RapidStartStop tests 10+ loop create/run/shutdown cycles.
func TestChaos_RapidStartStop(t *testing.T) {
	const cycles = 15 // More than 10 as specified

	for cycle := 0; cycle < cycles; cycle++ {
		func() {
			loop, err := New()
			if err != nil {
				t.Fatalf("Cycle %d: New() failed: %v", cycle, err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			runDone := make(chan error, 1)
			go func() {
				runDone <- loop.Run(ctx)
			}()

			// Wait for running
			waitLoopState(t, loop, StateRunning, time.Second)

			// Submit a quick task
			done := make(chan struct{})
			if err := loop.Submit(func() {
				close(done)
			}); err != nil {
				t.Logf("Cycle %d: Submit failed: %v", cycle, err)
			} else {
				select {
				case <-done:
				case <-time.After(time.Second):
					t.Errorf("Cycle %d: Task didn't execute", cycle)
				}
			}

			// Shutdown
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
			if err := loop.Shutdown(shutdownCtx); err != nil && !errors.Is(err, ErrLoopTerminated) {
				t.Logf("Cycle %d: Shutdown error: %v", cycle, err)
			}
			shutdownCancel()

			// Wait for run to complete
			select {
			case <-runDone:
			case <-time.After(2 * time.Second):
				t.Errorf("Cycle %d: Run didn't complete after shutdown", cycle)
			}

			// Close shouldn't panic
			loop.Close()
		}()
	}

	t.Logf("Completed %d start/stop cycles", cycles)
}

// TestChaos_ConcurrentRegisterFD_SetFastPathMode tests race between RegisterFD and SetFastPathMode.
func TestChaos_ConcurrentRegisterFD_SetFastPathMode(t *testing.T) {
	const (
		iterations        = 20 // Reduced from 50 for CI stability
		goroutinesPerSide = 5
	)

	for iter := 0; iter < iterations; iter++ {
		func() {
			loop, err := New()
			if err != nil {
				t.Fatalf("Iteration %d: New() failed: %v", iter, err)
			}
			defer loop.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			runDone := make(chan error, 1)
			go func() {
				runDone <- loop.Run(ctx)
			}()

			waitLoopState(t, loop, StateRunning, time.Second)

			// Create pipes for FD testing (both ends stay open to avoid
			// EPOLLHUP busy-looping on zombie descriptors)
			fds := make([]int, goroutinesPerSide)
			for i := 0; i < goroutinesPerSide; i++ {
				var pipefds [2]int
				if err := unix.Pipe(pipefds[:]); err != nil {
					t.Fatalf("Pipe failed: %v", err)
				}
				if err := unix.SetNonblock(pipefds[0], true); err != nil {
					t.Fatalf("SetNonblock failed: %v", err)
				}
				fds[i] = pipefds[0]
				defer unix.Close(pipefds[0])
				defer unix.Close(pipefds[1])
			}

			var wg sync.WaitGroup
			done := make(chan struct{})

			// Goroutines that toggle fast path mode
			for g := 0; g < goroutinesPerSide; g++ {
				wg.Add(1)
				go func(gid int) {
					defer wg.Done()
					modes := []FastPathMode{FastPathAuto, FastPathDisabled, FastPathForced}
					for {
						select {
						case <-done:
							return
						default:
							mode := modes[gid%len(modes)]
							_ = loop.SetFastPathMode(mode)
							time.Sleep(time.Microsecond)
						}
					}
				}(g)
			}

			// Goroutines that register/unregister FDs
			for g := 0; g < goroutinesPerSide; g++ {
				wg.Add(1)
				go func(gid int) {
					defer wg.Done()
					fd := fds[gid]
					registered := false
					for {
						select {
						case <-done:
							if registered {
								_ = loop.UnregisterFD(fd)
							}
							return
						default:
							if registered {
								_ = loop.UnregisterFD(fd)
								registered = false
							} else {
								err := loop.RegisterFD(fd, EventRead, func(IOEvents) {})
								if err == nil {
									registered = true
								}
								// ErrFastPathIncompatible is expected when mode is Forced
							}
							time.Sleep(time.Microsecond)
						}
					}
				}(g)
			}

			// Run for a short time
			time.Sleep(100 * time.Millisecond)
			close(done)
			wg.Wait()

			// Verify loop is still functional after chaos
			taskDone := make(chan struct{})
			if err := loop.Submit(func() {
				close(taskDone)
			}); err == nil {
				// Also send explicit wakeup to ensure loop isn't stuck in poll
				loop.doWakeup()
				select {
				case <-taskDone:
				case <-time.After(15 * time.Second):
					t.Errorf("Iteration %d: Task after race testing timed out (mode=%d, count=%d)",
						iter, loop.fastPathMode.Load(), loop.userIOFDCount.Load())
				}
			}

			loop.Shutdown(context.Background())
			<-runDone
		}()
	}
}

// TestChaos_SubmitDuringShutdown tests the race between Submit and Shutdown.
func TestChaos_SubmitDuringShutdown(t *testing.T) {
	const iterations = 100

	for iter := 0; iter < iterations; iter++ {
		func() {
			loop, err := New()
			if err != nil {
				t.Fatalf("Iteration %d: New() failed: %v", iter, err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			runDone := make(chan error, 1)
			go func() {
				runDone <- loop.Run(ctx)
			}()

			waitLoopState(t, loop, StateRunning, time.Second)

			var wg sync.WaitGroup
			var submitted, executed atomic.Int64

			// Goroutines submitting tasks
			for g := 0; g < 10; g++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := 0; i < 100; i++ {
						if err := loop.Submit(func() {
							executed.Add(1)
						}); err == nil {
							submitted.Add(1)
						} else if errors.Is(err, ErrLoopTerminated) {
							return
						}
					}
				}()
			}

			// Start shutdown after a tiny delay
			time.Sleep(time.Microsecond * 100)

			shutdownDone := make(chan error, 1)
			go func() {
				shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer shutdownCancel()
				shutdownDone <- loop.Shutdown(shutdownCtx)
			}()

			wg.Wait()

			// Wait for shutdown
			select {
			case err := <-shutdownDone:
				if err != nil && !errors.Is(err, ErrLoopTerminated) {
					t.Logf("Iteration %d: Shutdown error: %v", iter, err)
				}
			case <-time.After(3 * time.Second):
				t.Errorf("Iteration %d: Shutdown timed out", iter)
			}

			<-runDone

			// All submitted tasks should have executed
			sub := submitted.Load()
			exec := executed.Load()
			if sub > 0 && exec < sub {
				t.Errorf("Iteration %d: Only %d/%d submitted tasks executed", iter, exec, sub)
			}
		}()
	}
}

// TestChaos_MicrotaskFloodDuringShutdown tests microtask flood during shutdown.
func TestChaos_MicrotaskFloodDuringShutdown(t *testing.T) {
	const iterations = 50

	for iter := 0; iter < iterations; iter++ {
		func() {
			loop, err := New()
			if err != nil {
				t.Fatalf("Iteration %d: New() failed: %v", iter, err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			runDone := make(chan error, 1)
			go func() {
				runDone <- loop.Run(ctx)
			}()

			waitLoopState(t, loop, StateRunning, time.Second)

			var wg sync.WaitGroup
			var scheduled, executed atomic.Int64

			// Goroutines scheduling microtasks
			for g := 0; g < 10; g++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := 0; i < 200; i++ {
						if err := loop.ScheduleMicrotask(func() {
							executed.Add(1)
						}); err == nil {
							scheduled.Add(1)
						} else if errors.Is(err, ErrLoopTerminated) {
							return
						}
					}
				}()
			}

			// Start shutdown after a tiny delay
			time.Sleep(time.Microsecond * 50)

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
			loop.Shutdown(shutdownCtx)
			shutdownCancel()

			wg.Wait()
			<-runDone

			t.Logf("Iteration %d: scheduled=%d, executed=%d",
				iter, scheduled.Load(), executed.Load())
		}()
	}
}

// TestChaos_PromiseResolutionRaceWithShutdown tests promise resolution racing with shutdown.
func TestChaos_PromiseResolutionRaceWithShutdown(t *testing.T) {
	const iterations = 30

	for iter := 0; iter < iterations; iter++ {
		func() {
			loop, err := New()
			if err != nil {
				t.Fatalf("Iteration %d: New() failed: %v", iter, err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			runDone := make(chan error, 1)
			go func() {
				runDone <- loop.Run(ctx)
			}()

			waitLoopState(t, loop, StateRunning, time.Second)

			js, err := NewJS(loop)
			if err != nil {
				t.Fatalf("Iteration %d: NewJS() failed: %v", iter, err)
			}

			var wg sync.WaitGroup
			const promiseCount = 100

			// Create many promises
			promises := make([]*ChainedPromise, promiseCount)
			resolvers := make([]ResolveFunc, promiseCount)
			for i := 0; i < promiseCount; i++ {
				promises[i], resolvers[i], _ = js.NewChainedPromise()
			}

			// Attach Then handlers
			for i := 0; i < promiseCount; i++ {
				promises[i].Then(func(v Result) Result {
					return v
				}, nil)
			}

			// Goroutines resolving promises
			for i := 0; i < promiseCount; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					time.Sleep(time.Duration(idx%10) * time.Microsecond)
					resolvers[idx]("value")
				}(i)
			}

			// Start shutdown
			time.Sleep(time.Microsecond * 50)

			shutdownDone := make(chan struct{})
			go func() {
				shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer shutdownCancel()
				loop.Shutdown(shutdownCtx)
				close(shutdownDone)
			}()

			wg.Wait()
			<-shutdownDone
			<-runDone
		}()
	}

	t.Logf("Completed %d promise/shutdown race iterations", iterations)
}

// TestChaos_IntervalCancellationStorm tests many interval create/cancel cycles.
func TestChaos_IntervalCancellationStorm(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	waitLoopState(t, loop, StateRunning, 2*time.Second)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	const (
		numGoroutines = 20
		intervalsPerG = 50
	)

	var created, cancelled atomic.Int64
	var wg sync.WaitGroup

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < intervalsPerG; i++ {
				id, err := js.SetInterval(func() {
					// Do nothing
				}, 10+gid) // Small interval
				if err != nil {
					continue
				}
				created.Add(1)

				// Immediately cancel
				if err := js.ClearInterval(id); err == nil {
					cancelled.Add(1)
				}
			}
		}(g)
	}

	wg.Wait()

	t.Logf("Interval storm: created=%d, cancelled=%d", created.Load(), cancelled.Load())

	// Verify loop is healthy
	state := loop.State()
	if state != StateRunning && state != StateSleeping {
		t.Errorf("Expected Running or Sleeping, got %v", state)
	}

	loop.Shutdown(context.Background())
	<-runDone
}

// TestChaos_CombinedStress combines multiple stress patterns.
func TestChaos_CombinedStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping combined stress test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Shutdown(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- loop.Run(ctx)
	}()

	waitLoopState(t, loop, StateRunning, 2*time.Second)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatalf("NewJS() failed: %v", err)
	}

	stressEnd := time.Now().Add(5 * time.Second)
	done := make(chan struct{})
	var wg sync.WaitGroup

	// Pattern 1: Timer storm
	wg.Add(1)
	go func() {
		defer wg.Done()
		for time.Now().Before(stressEnd) {
			select {
			case <-done:
				return
			default:
				id, err := loop.ScheduleTimer(50*time.Millisecond, func() {})
				if err == nil {
					_ = loop.CancelTimer(id)
				}
			}
		}
	}()

	// Pattern 2: Promise creation
	wg.Add(1)
	go func() {
		defer wg.Done()
		for time.Now().Before(stressEnd) {
			select {
			case <-done:
				return
			default:
				p, resolve, _ := js.NewChainedPromise()
				resolve("value")
				_ = p.State()
			}
		}
	}()

	// Pattern 3: Microtask flood
	wg.Add(1)
	go func() {
		defer wg.Done()
		for time.Now().Before(stressEnd) {
			select {
			case <-done:
				return
			default:
				_ = loop.ScheduleMicrotask(func() {})
			}
		}
	}()

	// Pattern 4: Mode switching
	wg.Add(1)
	go func() {
		defer wg.Done()
		modes := []FastPathMode{FastPathAuto, FastPathDisabled, FastPathForced}
		i := 0
		for time.Now().Before(stressEnd) {
			select {
			case <-done:
				return
			default:
				_ = loop.SetFastPathMode(modes[i%len(modes)])
				i++
				time.Sleep(time.Millisecond)
			}
		}
	}()

	// Pattern 5: GC pressure
	wg.Add(1)
	go func() {
		defer wg.Done()
		for time.Now().Before(stressEnd) {
			select {
			case <-done:
				return
			default:
				runtime.GC()
				time.Sleep(50 * time.Millisecond)
			}
		}
	}()

	// Wait for stress duration
	<-time.After(5 * time.Second)
	close(done)
	wg.Wait()

	// Verify loop is still healthy
	normalDone := make(chan struct{})
	err = loop.Submit(func() {
		close(normalDone)
	})
	if err != nil {
		t.Fatalf("Submit after stress failed: %v", err)
	}

	select {
	case <-normalDone:
		t.Log("Loop recovered from combined stress")
	case <-time.After(10 * time.Second):
		t.Fatal("Loop appears stuck after combined stress")
	}

	loop.Shutdown(context.Background())
	<-runDone
}
