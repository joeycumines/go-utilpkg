package eventloop

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestFastPath_EntryDebug is a diagnostic test to verify fast path is being entered.
// It tests both:
// 1. runFastPath() - called when loop is in fast path mode (no I/O, no timers)
// 2. SubmitInternal direct execution - when called from loop thread with fast path enabled
func TestFastPath_EntryDebug(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Track fast path entries via hook
	var hookCalls int64
	var hookMu sync.Mutex
	loop.testHooks = &loopTestHooks{
		OnFastPathEntry: func() {
			hookMu.Lock()
			hookCalls++
			hookMu.Unlock()
		},
	}

	// Start the loop
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		loop.Run(ctx)
	}()

	// Wait for loop to start
	for i := 0; i < 100; i++ {
		if loop.state.Load() == StateRunning || loop.state.Load() == StateSleeping {
			break
		}
		time.Sleep(time.Millisecond)
	}

	// Submit a task and wait for completion
	done := make(chan struct{})
	err = loop.Submit(func() {
		close(done)
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	select {
	case <-done:
		// Task executed
	case <-time.After(5 * time.Second):
		t.Fatal("Task did not execute within timeout")
	}

	// Check counters
	entries := loop.fastPathEntries.Load()
	hookMu.Lock()
	hooks := hookCalls
	hookMu.Unlock()

	t.Logf("=== FAST PATH DEBUG REPORT ===")
	t.Logf("FastPathEntries counter: %d", entries)
	t.Logf("OnFastPathEntry hook calls: %d", hooks)
	t.Logf("fastPathMode: %v (0=Auto, 1=Forced, 2=Disabled)", FastPathMode(loop.fastPathMode.Load()))
	t.Logf("canUseFastPath: %v", loop.canUseFastPath())
	t.Logf("userIOFDCount: %d", loop.userIOFDCount.Load())
	t.Logf("timers pending: %d", len(loop.timers))
	t.Logf("state: %v", loop.state.Load())

	// Report diagnosis
	if entries == 0 {
		t.Logf("⚠️  Fast path NOT entered!")
		t.Logf("Possible reasons:")
		if !loop.canUseFastPath() {
			t.Logf("  - canUseFastPath() returned false")
		}
		if loop.userIOFDCount.Load() != 0 {
			t.Logf("  - userIOFDCount != 0 (I/O FDs registered)")
		}
		if len(loop.timers) > 0 {
			t.Logf("  - timers pending")
		}
		t.Logf("  - Note: runFastPath requires NO I/O FDs, NO timers, NO internal tasks")
		t.Logf("  - Note: SubmitInternal direct exec requires being ON the loop thread")
	} else {
		t.Logf("✓ Fast path WAS entered %d times", entries)
	}

	// Cleanup
	cancel()
	wg.Wait()
}

// TestFastPath_SubmitInternalDirectExec tests if SubmitInternal executes directly
// when called from within a loop task (i.e., on the loop thread).
func TestFastPath_SubmitInternalDirectExec(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		loop.Run(ctx)
	}()

	// Wait for loop to start
	for i := 0; i < 100; i++ {
		if loop.state.Load() == StateRunning || loop.state.Load() == StateSleeping {
			break
		}
		time.Sleep(time.Millisecond)
	}

	// Reset counter
	initialEntries := loop.fastPathEntries.Load()

	// Submit a task that submits another task (from loop thread)
	done := make(chan struct{})
	innerDone := make(chan struct{})

	err = loop.Submit(func() {
		// Now we're on the loop thread - submit internal task
		loop.SubmitInternal(func() {
			close(innerDone)
		})
		close(done)
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	// Wait for both tasks
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Outer task did not execute")
	}
	select {
	case <-innerDone:
	case <-time.After(5 * time.Second):
		t.Fatal("Inner task did not execute")
	}

	// Check if SubmitInternal used fast path
	finalEntries := loop.fastPathEntries.Load()
	directExecEntries := finalEntries - initialEntries

	t.Logf("=== SUBMIT INTERNAL DIRECT EXEC DEBUG ===")
	t.Logf("Initial FastPathEntries: %d", initialEntries)
	t.Logf("Final FastPathEntries: %d", finalEntries)
	t.Logf("New entries during SubmitInternal: %d", directExecEntries)
	t.Logf("state during inner call: %v (expected: StateRunning=%d)", loop.state.Load(), StateRunning)

	if directExecEntries > 0 {
		t.Logf("✓ SubmitInternal used direct execution (fast path)")
	} else {
		t.Logf("⚠️  SubmitInternal did NOT use direct execution")
		t.Logf("Requirements for direct exec:")
		t.Logf("  - canUseFastPath(): %v (need true)", loop.canUseFastPath())
		t.Logf("  - state == StateRunning: %v", loop.state.Load() == StateRunning)
		t.Logf("  - isLoopThread(): called from loop thread")
		t.Logf("  - external queue empty: checked at runtime")
	}

	cancel()
	wg.Wait()
}
