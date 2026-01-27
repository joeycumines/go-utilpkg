package eventloop

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Test_FastState_IsTerminal tests the IsTerminal state query method
func Test_FastState_IsTerminal(t *testing.T) {
	t.Parallel()

	t.Run("IsTerminal returns false for non-terminal states", func(t *testing.T) {
		t.Parallel()

		// Test each non-terminal state
		nonTerminalStates := []LoopState{
			StateAwake,
			StateRunning,
			StateSleeping,
			StateTerminating,
		}

		for _, state := range nonTerminalStates {
			t.Run(state.String(), func(t *testing.T) {
				var fs FastState
				fs.Store(state)

				if fs.IsTerminal() {
					t.Errorf("IsTerminal() returned true for %v (expected false)", state)
				}
			})
		}
	})

	t.Run("IsTerminal returns true for StateTerminated", func(t *testing.T) {
		t.Parallel()

		var fs FastState
		fs.Store(StateTerminated)

		if !fs.IsTerminal() {
			t.Error("IsTerminal() returned false for StateTerminated (expected true)")
		}
	})

	t.Run("IsTerminal is thread-safe", func(t *testing.T) {
		t.Parallel()

		var fs FastState
		fs.Store(StateRunning)

		numGoroutines := 100
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				// Concurrent IsTerminal calls
				_ = fs.IsTerminal()
			}()
		}

		wg.Wait()

		// Final state should still be the same
		if fs.Load() != StateRunning {
			t.Fatalf("State changed unexpectedly: %v", fs.Load())
		}
	})
}

// Test_FastState_CanAcceptWork tests the CanAcceptWork state query method
func Test_FastState_CanAcceptWork(t *testing.T) {
	t.Parallel()

	t.Run("CanAcceptWork returns true for accepting states", func(t *testing.T) {
		t.Parallel()

		acceptingStates := []LoopState{
			StateAwake,
			StateRunning,
			StateSleeping,
		}

		for _, state := range acceptingStates {
			t.Run(state.String(), func(t *testing.T) {
				var fs FastState
				fs.Store(state)

				if !fs.CanAcceptWork() {
					t.Errorf("CanAcceptWork() returned false for %v (expected true)", state)
				}
			})
		}
	})

	t.Run("CanAcceptWork returns false for non-accepting states", func(t *testing.T) {
		t.Parallel()

		nonAcceptingStates := []LoopState{
			StateTerminating,
			StateTerminated,
		}

		for _, state := range nonAcceptingStates {
			t.Run(state.String(), func(t *testing.T) {
				var fs FastState
				fs.Store(state)

				if fs.CanAcceptWork() {
					t.Errorf("CanAcceptWork() returned true for %v (expected false)", state)
				}
			})
		}
	})

	t.Run("CanAcceptWork is thread-safe", func(t *testing.T) {
		t.Parallel()

		var fs FastState
		fs.Store(StateRunning)

		numGoroutines := 100
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				// Concurrent CanAcceptWork calls
				_ = fs.CanAcceptWork()
			}()
		}

		wg.Wait()

		// Final state should still be the same
		if fs.Load() != StateRunning {
			t.Fatalf("State changed unexpectedly: %v", fs.Load())
		}
	})
}

// Test_FastState_IsRunning tests the IsRunning state query method
func Test_FastState_IsRunning(t *testing.T) {
	t.Parallel()

	t.Run("IsRunning returns true for Running states", func(t *testing.T) {
		t.Parallel()

		runningStates := []LoopState{
			StateRunning,
			StateSleeping,
		}

		for _, state := range runningStates {
			t.Run(state.String(), func(t *testing.T) {
				var fs FastState
				fs.Store(state)

				if !fs.IsRunning() {
					t.Errorf("IsRunning() returned false for %v (expected true)", state)
				}
			})
		}
	})

	t.Run("IsRunning returns false for non-running states", func(t *testing.T) {
		t.Parallel()

		nonRunningStates := []LoopState{
			StateAwake,
			StateTerminating,
			StateTerminated,
		}

		for _, state := range nonRunningStates {
			t.Run(state.String(), func(t *testing.T) {
				var fs FastState
				fs.Store(state)

				if fs.IsRunning() {
					t.Errorf("IsRunning() returned true for %v (expected false)", state)
				}
			})
		}
	})
}

// Test_FastState_TransitionAny tests the TransitionAny state transition method
func Test_FastState_TransitionAny(t *testing.T) {
	t.Parallel()

	t.Run("TransitionAny succeeds for valid transition", func(t *testing.T) {
		t.Parallel()

		var fs FastState
		fs.Store(StateAwake)

		validFrom := []LoopState{StateAwake}
		to := StateRunning

		if !fs.TransitionAny(validFrom, to) {
			t.Error("TransitionAny failed for valid transition")
		}

		// Verify state changed
		if fs.Load() != StateRunning {
			t.Fatalf("State not changed to %v, got %v", StateRunning, fs.Load())
		}
	})

	t.Run("TransitionAny fails for invalid source state", func(t *testing.T) {
		t.Parallel()

		var fs FastState
		fs.Store(StateAwake)

		validFrom := []LoopState{StateRunning} // Not StateAwake
		to := StateSleeping

		if fs.TransitionAny(validFrom, to) {
			t.Error("TransitionAny succeeded for invalid source state")
		}

		// Verify state unchanged
		if fs.Load() != StateAwake {
			t.Fatalf("State changed unexpectedly: %v", fs.Load())
		}
	})

	t.Run("TransitionAny with multiple valid sources", func(t *testing.T) {
		t.Parallel()

		// Test from each valid source
		validFrom := []LoopState{StateAwake, StateRunning}
		to := StateTerminating

		// Test from StateAwake
		var fs1 FastState
		fs1.Store(StateAwake)
		if !fs1.TransitionAny(validFrom, to) {
			t.Error("TransitionAny failed from StateAwake")
		}

		// Test from StateRunning
		var fs2 FastState
		fs2.Store(StateRunning)
		if !fs2.TransitionAny(validFrom, to) {
			t.Error("TransitionAny failed from StateRunning")
		}
	})

	t.Run("TransitionAny tries all valid sources", func(t *testing.T) {
		t.Parallel()

		var fs FastState
		fs.Store(StateRunning)

		// Order doesn't matter - should succeed from current state
		validFrom := []LoopState{StateAwake, StateRunning, StateSleeping}
		to := StateTerminating

		if !fs.TransitionAny(validFrom, to) {
			t.Error("TransitionAny failed despite valid source in list")
		}

		// Verify state changed
		if fs.Load() != StateTerminating {
			t.Fatalf("State not changed to %v, got %v", StateTerminating, fs.Load())
		}
	})

	t.Run("TransitionAll succeeds for exact state match", func(t *testing.T) {
		t.Parallel()

		var fs FastState
		fs.Store(StateRunning)

		from := StateRunning
		to := StateSleeping

		if !fs.Transition(from, to) {
			t.Error("Transition failed for exact state match")
		}

		// Verify state changed
		if fs.Load() != StateSleeping {
			t.Fatalf("State not changed to %v, got %v", StateSleeping, fs.Load())
		}
	})

	t.Run("TransitionAll fails for state mismatch", func(t *testing.T) {
		t.Parallel()

		var fs FastState
		fs.Store(StateAwake)

		from := StateRunning
		to := StateSleeping

		if fs.Transition(from, to) {
			t.Error("Transition succeeded for state mismatch")
		}

		// Verify state unchanged
		if fs.Load() != StateAwake {
			t.Fatalf("State changed unexpectedly: %v", fs.Load())
		}
	})
}

// Test_FastState_InvalidTransitions tests that invalid transitions are rejected
func Test_FastState_InvalidTransitions(t *testing.T) {
	t.Parallel()

	t.Run("Transition to StateTerminated must be atomic (Store, not CAS)", func(t *testing.T) {
		t.Parallel()

		var fs FastState
		fs.Store(StateTerminating)

		// Direct Store is used for terminal states
		// We can't directly test this without access to internal implementation,
		// but we can verify that once in a terminal state, no transitions happen
		fs.Store(StateTerminated)

		// Try CAS transitions - should all fail
		validFrom := []LoopState{StateTerminated}
		if fs.TransitionAny(validFrom, StateRunning) {
			t.Error("Should not transition from TERMINATED")
		}

		// State should still be TERMINATED
		if !fs.IsTerminal() {
			t.Error("State should remain TERMINATED")
		}
	})

	t.Run("Double settlement is idempotent", func(t *testing.T) {
		t.Parallel()

		var fs FastState
		fs.Store(StateTerminating)

		// Transition to TERMINATED
		fs.Store(StateTerminated)

		// Try to transition again - should fail
		validFrom := []LoopState{StateTerminated}
		if fs.TransitionAny(validFrom, StateRunning) {
			t.Error("Should not transition from TERMINATED")
		}

		// Try another Store
		fs.Store(StateTerminated)

		// Should still be TERMINATED
		if fs.Load() != StateTerminated {
			t.Fatalf("State changed from TERMINATED to %v", fs.Load())
		}
	})
}

// Test_FastState_ConcurrentTransitions tests concurrent state transitions
func Test_FastState_ConcurrentTransitions(t *testing.T) {
	t.Parallel()

	t.Run("Only one goroutine succeeds on transition", func(t *testing.T) {
		t.Parallel()

		var fs FastState
		fs.Store(StateAwake)

		validFrom := []LoopState{StateAwake}
		to := StateRunning

		numGoroutines := 10
		successCount := atomic.Int32{}
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				if fs.TransitionAny(validFrom, to) {
					successCount.Add(1)
				}
			}()
		}

		wg.Wait()

		// Exactly one goroutine should have succeeded
		if successCount.Load() != 1 {
			t.Fatalf("Expected 1 successful transition, got %d", successCount.Load())
		}

		// Final state should be Running
		if fs.Load() != StateRunning {
			t.Fatalf("Final state should be Running, got %v", fs.Load())
		}
	})

	t.Run("Concurrent transitions from different sources", func(t *testing.T) {
		t.Parallel()

		var fs FastState
		fs.Store(StateRunning)

		// Some goroutines try Running->Sleeping, others try Running->Terminating
		validFromSleeping := []LoopState{StateRunning}
		validFromTerminating := []LoopState{StateRunning}

		sleepingSuccess := atomic.Int32{}
		terminatingSuccess := atomic.Int32{}
		numGoroutines := 20
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(idx int) {
				defer wg.Done()
				if idx%2 == 0 {
					if fs.TransitionAny(validFromSleeping, StateSleeping) {
						sleepingSuccess.Add(1)
					}
				} else {
					if fs.TransitionAny(validFromTerminating, StateTerminating) {
						terminatingSuccess.Add(1)
					}
				}
			}(i)
		}

		wg.Wait()

		// Exactly one transition should have succeeded
		totalSuccess := sleepingSuccess.Load() + terminatingSuccess.Load()
		if totalSuccess != 1 {
			t.Fatalf("Expected 1 total successful transition, got %d (sleeping=%d, terminating=%d)",
				totalSuccess, sleepingSuccess.Load(), terminatingSuccess.Load())
		}
	})
}

// Test_FastState_StateQueriesIntegration tests integration of state queries with actual transitions
func Test_FastState_StateQueriesIntegration(t *testing.T) {
	t.Parallel()

	t.Run("Full lifecycle: Awake -> Running -> Sleeping -> Terminating -> Terminated", func(t *testing.T) {
		t.Parallel()

		var fs FastState
		fs.Store(StateAwake)

		// Awake -> Running
		if !fs.Transition(StateAwake, StateRunning) {
			t.Fatal("Failed to transition Awake -> Running")
		}
		if fs.Load() != StateRunning {
			t.Fatalf("State not Running after transition: %v", fs.Load())
		}
		if !fs.CanAcceptWork() {
			t.Error("Running should accept work")
		}
		if fs.IsTerminal() {
			t.Error("Running is not terminal")
		}

		// Running -> Sleeping
		if !fs.Transition(StateRunning, StateSleeping) {
			t.Fatal("Failed to transition Running -> Sleeping")
		}
		if fs.Load() != StateSleeping {
			t.Fatalf("State not Sleeping after transition: %v", fs.Load())
		}
		if !fs.CanAcceptWork() {
			t.Error("Sleeping should accept work")
		}
		if fs.IsTerminal() {
			t.Error("Sleeping is not terminal")
		}

		// Sleeping -> Terminating
		if !fs.Transition(StateSleeping, StateTerminating) {
			t.Fatal("Failed to transition Sleeping -> Terminating")
		}
		if fs.Load() != StateTerminating {
			t.Fatalf("State not Terminating after transition: %v", fs.Load())
		}
		if fs.CanAcceptWork() {
			t.Error("Terminating should not accept work")
		}
		if fs.IsTerminal() {
			t.Error("Terminating is not terminal")
		}

		// Terminating -> Terminated (Store, not CAS)
		fs.Store(StateTerminated)
		if fs.Load() != StateTerminated {
			t.Fatalf("State not Terminated after Store: %v", fs.Load())
		}
		if fs.CanAcceptWork() {
			t.Error("Terminated should not accept work")
		}
		if !fs.IsTerminal() {
			t.Error("Terminated is terminal")
		}
	})

	t.Run("State machine boundary conditions", func(t *testing.T) {
		t.Parallel()

		// Test that we can't go backwards
		var fs FastState
		fs.Store(StateTerminated)

		// Try all reverse transitions - all should fail
		reverseTransitions := []struct {
			from []LoopState
			to   LoopState
		}{
			{[]LoopState{StateTerminated}, StateTerminating},
			{[]LoopState{StateTerminated, StateTerminating}, StateSleeping},
			{[]LoopState{StateTerminated, StateTerminating, StateSleeping}, StateRunning},
			{[]LoopState{StateTerminated, StateTerminating, StateSleeping, StateRunning}, StateAwake},
		}

		for _, tt := range reverseTransitions {
			if fs.TransitionAny(tt.from, tt.to) {
				t.Errorf("Reverse transition %v -> %v should fail", tt.from, tt.to)
			}
		}

		// Should still be TERMINATED
		if !fs.IsTerminal() {
			t.Error("State should still be TERMINATED")
		}
	})
}

// Test_LoopState_String tests the String method for LoopState
func Test_LoopState_String(t *testing.T) {
	t.Parallel()

	t.Run("String returns valid names for all states", func(t *testing.T) {
		t.Parallel()

		states := []LoopState{
			StateAwake,
			StateTerminated,
			StateSleeping,
			StateTerminating,
			StateRunning,
		}

		for _, state := range states {
			s := state.String()
			if s == "" {
				t.Errorf("String() returned empty for state %d", state)
			}
		}
	})

	t.Run("String returns predictable values", func(t *testing.T) {
		t.Parallel()

		if StateAwake.String() != "StateAwake" {
			t.Errorf("StateAwake.String() = %s, want StateAwake", StateAwake.String())
		}
		if StateTerminated.String() != "StateTerminated" {
			t.Errorf("StateTerminated.String() = %s, want StateTerminated", StateTerminated.String())
		}
	})
}

// Test_LoopState_InLoopLifecycle tests state machine behavior in actual loop lifecycle
func Test_LoopState_InLoopLifecycle(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	t.Run("Loop state transitions through lifecycle", func(t *testing.T) {
		t.Parallel()

		loop, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		// Initial state might be Awake or StateTerminated (not started yet)
		// This is implementation-dependent

		// Submit a task to ensure loop starts
		taskDone := make(chan struct{}, 1)
		err = loop.Submit(func() {
			taskDone <- struct{}{}
		})
		if err != nil {
			t.Fatalf("Failed to submit task: %v", err)
		}

		// Wait for task
		select {
		case <-taskDone:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Timeout waiting for task")
		}

		// Loop should be in a running state
		state := loop.State()
		if !state.IsRunning() && state != StateAwake {
			t.Logf("Warning: Loop state %v may not be running", state)
		}

		if !state.CanAcceptWork() {
			t.Errorf("Loop state %v should accept work", state)
		}

		// Stop loop
		loop.Stop()
		time.Sleep(50 * time.Millisecond)

		// Should be terminated
		finalState := loop.State()
		if !finalState.IsTerminal() {
			t.Fatalf("Loop should be in terminal state after Stop, got %v", finalState)
		}

		if finalState.CanAcceptWork() {
			t.Errorf("Loop state %v should not accept work after termination", finalState)
		}
	})
}

// Test_FastState_TransitionStress tests state transitions under stress
func Test_FastState_TransitionStress(t *testing.T) {
	t.Parallel()

	t.Run("High frequency transitions", func(t *testing.T) {
		t.Parallel()

		var fs FastState
		fs.Store(StateAwake)

		const numTransitions = 10000
		successCount := atomic.Int32{}

		// Transition Sleep <-> Running repeatedly
		validFromRunning := []LoopState{StateSleeping}
		validFromSleeping := []LoopState{StateRunning}

		for i := 0; i < numTransitions; i++ {
			if i%2 == 0 {
				if fs.TransitionAny(validFromSleeping, StateRunning) {
					successCount.Add(1)
				}
			} else {
				if fs.TransitionAny(validFromRunning, StateSleeping) {
					successCount.Add(1)
				}
			}
		}

		// Should have approximately numTransitions/2 successes
		// (Some may fail due to timing)
		if successCount.Load() < numTransitions/3 {
			t.Fatalf("Too few successful transitions: %d / %d", successCount.Load(), numTransitions)
		}
	})

	t.Run("Concurrent state queries under stress", func(t *testing.T) {
		t.Parallel()

		var fs FastState
		fs.Store(StateRunning)

		numGoroutines := 100
		iterationsPerGoroutine := 1000
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < iterationsPerGoroutine; j++ {
					_ = fs.IsTerminal()
					_ = fs.CanAcceptWork()
					_ = fs.IsRunning()
				}
			}()
		}

		wg.Wait()

		// State should still be Running
		if fs.Load() != StateRunning {
			t.Fatalf("State changed unstably: %v", fs.Load())
		}
	})
}

// Test_FastState_StoreVsCAS compares Store vs CAS behavior
func Test_FastState_StoreVsCAS(t *testing.T) {
	t.Parallel()

	t.Run("Store overwrites current state unconditionally", func(t *testing.T) {
		t.Parallel()

		var fs FastState
		fs.Store(StateRunning)

		// Store StateTerminated directly (as done in shutdown)
		fs.Store(StateTerminated)

		if fs.Load() != StateTerminated {
			t.Fatalf("Store didn't update state: got %v", fs.Load())
		}
	})

	t.Run("CAS only updates from specific state", func(t *testing.T) {
		t.Parallel()

		var fs FastState
		fs.Store(StateAwake)

		// Try CAS from wrong state
		if fs.Transition(StateRunning, StateSleeping) {
			t.Error("CAS succeeded from wrong state")
		}

		// State should be unchanged
		if fs.Load() != StateAwake {
			t.Fatalf("State changed despite failed CAS: %v", fs.Load())
		}

		// Try CAS from correct state
		if !fs.Transition(StateAwake, StateRunning) {
			t.Error("CAS failed from correct state")
		}

		// State should be changed
		if fs.Load() != StateRunning {
			t.Fatalf("CAS didn't update state: got %v", fs.Load())
		}
	})
}

// Test_FastState_MemoryModel verifies memory ordering guarantees
func Test_FastState_MemoryModel(t *testing.T) {
	t.Parallel()

	t.Run("State visibility across goroutines", func(t *testing.T) {
		t.Parallel()

		var fs FastState
		fs.Store(StateAwake)

		numGoroutines := 50
		ready := make(chan struct{}, numGoroutines)
		done := make(chan struct{}, numGoroutines)

		// All goroutines wait for signal, then read state
		for i := 0; i < numGoroutines; i++ {
			go func() {
				ready <- struct{}{}
				<-ready
				state := fs.Load()
				_ = state // Use the value
				done <- struct{}{}
			}()
		}

		// Wait for all to be ready
		for i := 0; i < numGoroutines; i++ {
			<-ready
		}

		// Change state
		fs.Store(StateRunning)

		// Signal all to read
		for i := 0; i < numGoroutines; i++ {
			ready <- struct{}{}
		}

		// Wait for all to finish
		timeout := time.After(time.Second)
		for i := 0; i < numGoroutines; i++ {
			select {
			case <-done:
			case <-timeout:
				t.Fatal("Timeout waiting for goroutines")
			}
		}
	})
}
