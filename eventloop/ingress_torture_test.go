package eventloop

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// REGRESSION TESTS FOR MICROTASKRING DEFECTS
//
// These tests are designed to FAIL on buggy code to prove the existence of
// the defects documented in scratch.md.
// =============================================================================

// TestMicrotaskRing_WriteAfterFree_Race is a torture test proving the Pop ordering bug.
//
// DEFECT #3 (CRITICAL): MicrotaskRing.Pop() Write-After-Free Race
//
// The bug: In Pop(), the code increments head (making the slot available to producers)
// BEFORE clearing the sequence guard for that slot:
//
//   r.head.Add(1)                   // [A] Slot released to producer
//   r.seq[(head)%4096].Store(0)     // [B] Guard cleared (using OLD head)
//
// Race Scenario:
//   1. Consumer reads data at index i
//   2. Consumer executes r.head.Add(1) - slot i is now logically free
//   3. *Context Switch*
//   4. Producer claims slot i, writes new task, stores new valid sequence S > 0
//   5. Consumer resumes, executes r.seq[i].Store(0)
//   6. Result: Slot i has valid data, but sequence guard is 0
//   7. Impact: Next Pop sees seq == 0, enters infinite spin-loop
//
// FIX: Clear sequence guard BEFORE advancing head:
//   r.seq[(head)%4096].Store(0)  // Clear guard FIRST
//   r.head.Add(1)                // Release slot SECOND
//
// RUN: go test -v -timeout 30s -run TestMicrotaskRing_WriteAfterFree_Race
func TestMicrotaskRing_WriteAfterFree_Race(t *testing.T) {
	ring := NewMicrotaskRing()
	const iterations = 1_000_000
	var wg sync.WaitGroup
	wg.Add(2)

	// Producer goroutine - locked to OS thread for consistent scheduling
	go func() {
		defer wg.Done()
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		for i := 0; i < iterations; i++ {
			for !ring.Push(func() {}) {
				runtime.Gosched()
			}
		}
	}()

	// Consumer goroutine - locked to OS thread for consistent scheduling
	var consumed atomic.Int64
	go func() {
		defer wg.Done()
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		count := 0
		// Use a local watchdog that we reset periodically
		lastProgress := time.Now()

		for count < iterations {
			task := ring.Pop()
			if task == nil {
				// Check for stall - if no progress for 5 seconds, we're deadlocked
				if time.Since(lastProgress) > 5*time.Second {
					consumed.Store(int64(count))
					return // Exit with partial count to signal deadlock
				}
				runtime.Gosched()
				continue
			}
			count++
			consumed.Store(int64(count))
			lastProgress = time.Now()
		}
	}()

	// Main goroutine timeout watchdog
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if consumed.Load() == int64(iterations) {
			t.Logf("Success: %d items processed without deadlock", iterations)
		} else {
			t.Fatalf("DEADLOCK DETECTED: Consumer stalled after processing %d/%d items. Write-After-Free race proven.",
				consumed.Load(), iterations)
		}
	case <-time.After(15 * time.Second):
		t.Fatalf("DEADLOCK DETECTED: Test timed out after 15s. Consumer processed %d/%d items. Write-After-Free race proven.",
			consumed.Load(), iterations)
	}
}

// TestMicrotaskRing_FIFO_Violation is a deterministic test proving overflow priority inversion.
//
// DEFECT #4 (CRITICAL): MicrotaskRing FIFO Violation
//
// The bug: The hybrid design of Ring (lock-free) + Overflow (mutex) creates
// "Priority Inversion" where newer tasks can be processed before older tasks.
//
// The Pop method iterates strictly over the Ring. It only checks the Overflow
// buffer if the Ring is empty.
//
// Failure Scenario:
//   1. Saturation: Ring fills up (4096 items)
//   2. Overflow: Producer pushes Task A (Seq 4097). Goes into overflow.
//   3. Drain: Consumer pops one item from Ring. Ring now has 4095 items.
//   4. Race: Producer pushes Task B (Seq 4098). Ring has space, Task B enters Ring.
//   5. Ordering Failure: Consumer processes Task B before Task A.
//
// FIX: In Push, if overflow buffer is non-empty, append to overflow even if ring has space.
//
// RUN: go test -v -run TestMicrotaskRing_FIFO_Violation
func TestMicrotaskRing_FIFO_Violation(t *testing.T) {
	r := NewMicrotaskRing()

	// 1. Saturate the Ring Buffer (4096 items)
	for i := 0; i < 4096; i++ {
		val := i
		if !r.Push(func() { _ = val }) {
			t.Fatalf("Failed to push item %d to ring", i)
		}
	}

	// Verify ring is full
	if r.Length() != 4096 {
		t.Fatalf("Expected ring length 4096, got %d", r.Length())
	}

	// 2. Force Overflow - This is "Task A" (older task)
	var taskA_Order int
	var taskB_Order int
	var orderCounter atomic.Int32

	r.Push(func() {
		taskA_Order = int(orderCounter.Add(1))
	})

	// Verify Task A went to overflow (ring is full)
	if r.Length() != 4097 {
		t.Fatalf("Expected length 4097 after overflow push, got %d", r.Length())
	}

	// 3. Create Space in the Ring by popping one item
	if fn := r.Pop(); fn == nil {
		t.Fatal("Expected item from ring")
	}

	// Ring now has 4095 items + 1 in overflow = 4096 total

	// 4. Push "Task B" (newer task) - this is where the bug manifests
	// If buggy: Task B goes to Ring (has space) instead of Overflow
	// If fixed: Task B goes to Overflow (after Task A)
	r.Push(func() {
		taskB_Order = int(orderCounter.Add(1))
	})

	// 5. Drain the Ring (4095 items remaining from saturation)
	for i := 0; i < 4095; i++ {
		if fn := r.Pop(); fn == nil {
			t.Fatalf("Expected item from ring at iteration %d", i)
		}
	}

	// 6. The Moment of Truth - next item MUST be Task A (the older overflow task)
	nextFn := r.Pop()
	if nextFn == nil {
		t.Fatal("Queue should not be empty - expected Task A or Task B")
	}
	nextFn() // Execute first task after ring drain

	// Pop and execute the second task
	secondFn := r.Pop()
	if secondFn == nil {
		t.Fatal("Queue should not be empty - expected second task")
	}
	secondFn()

	// Verify ordering
	if taskA_Order == 0 {
		t.Fatal("Task A was never executed")
	}
	if taskB_Order == 0 {
		t.Fatal("Task B was never executed")
	}

	// CRITICAL CHECK: Task A (older) MUST execute before Task B (newer)
	if taskB_Order < taskA_Order {
		t.Fatalf("CRITICAL FIFO VIOLATION: Task B (newer, order=%d) executed before Task A (older, order=%d). "+
			"Priority inversion detected!", taskB_Order, taskA_Order)
	}

	t.Logf("Success: Task A (order=%d) executed before Task B (order=%d)", taskA_Order, taskB_Order)
}

// TestMicrotaskRing_NilInput_Liveness proves nil input causes infinite loop.
//
// DEFECT #6 (HIGH): MicrotaskRing.Pop() Infinite Loop on nil Input
//
// The bug: Push does not prevent nil functions. If Push(nil) is called,
// Pop enters an infinite loop:
//
//   1. Pop reads a valid sequence number for the slot containing nil
//   2. It reads fn, which is nil
//   3. It hits the defensive check: if fn == nil
//   4. It re-reads head and tail and continues WITHOUT advancing head
//   5. Next iteration pops the exact same nil task, repeating indefinitely
//
// FIX Option A: In Pop, when nil is encountered, still consume it by advancing
//               head and clearing sequence, then continue.
// FIX Option B: In Push, silently drop or return error for nil functions.
//
// RUN: go test -v -timeout 10s -run TestMicrotaskRing_NilInput_Liveness
func TestMicrotaskRing_NilInput_Liveness(t *testing.T) {
	r := NewMicrotaskRing()

	// Push a nil function - this should NOT be allowed, but the current
	// implementation doesn't validate input
	r.Push(nil)

	// Push a valid function after the nil
	executed := false
	r.Push(func() {
		executed = true
	})

	// Try to pop - if buggy, this will hang forever on the nil slot
	done := make(chan struct{})
	go func() {
		defer close(done)

		// First pop should either:
		// - Skip the nil and return the valid function (if fixed with Option A)
		// - Return nil indicating empty (if fixed with Option B - nil was dropped)
		// - Hang forever (buggy)
		for i := 0; i < 10; i++ {
			fn := r.Pop()
			if fn != nil {
				fn()
				return
			}
			// If we got nil, maybe the queue is being drained properly
			// Try a few more times
			runtime.Gosched()
		}
	}()

	select {
	case <-done:
		if executed {
			t.Log("Success: Pop correctly handled nil input and retrieved subsequent task")
		} else {
			// This is also acceptable if the implementation drops nil inputs
			t.Log("Note: Pop returned nil repeatedly - verify nil was dropped by Push")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("LIVENESS FAILURE: Pop() is stuck in infinite loop due to nil input. " +
			"The nil task is never consumed because head is not advanced.")
	}
}
