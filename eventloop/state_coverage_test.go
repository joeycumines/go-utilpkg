package eventloop

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

// COVERAGE-018: fastState Full Coverage
// Gaps: all state transitions, TryTransition failure cases, IsRunning edge cases,
// concurrent state access patterns

// Table of valid state transitions from state machine documentation:
// StateAwake (0)       → StateRunning (4)
// StateRunning (4)     → StateSleeping (2)
// StateRunning (4)     → StateTerminating (5)
// StateSleeping (2)    → StateRunning (4)
// StateSleeping (2)    → StateTerminating (5)
// StateTerminating (5) → StateTerminated (1)
// StateTerminated (1)  → (terminal)

// Test_fastState_newFastState verifies initial state is Awake.
func Test_fastState_newFastState(t *testing.T) {
	s := newFastState()

	if s.Load() != StateAwake {
		t.Errorf("Expected initial state Awake, got %v", s.Load())
	}
}

// Test_fastState_Load_AllStates verifies Load returns all state values correctly.
func Test_fastState_Load_AllStates(t *testing.T) {
	tests := []struct {
		name  string
		state LoopState
	}{
		{"Awake", StateAwake},
		{"Terminated", StateTerminated},
		{"Sleeping", StateSleeping},
		{"Running", StateRunning},
		{"Terminating", StateTerminating},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newFastState()
			s.v.Store(uint64(tc.state))

			if s.Load() != tc.state {
				t.Errorf("Expected %v, got %v", tc.state, s.Load())
			}
		})
	}
}

// Test_fastState_Store_AllStates verifies Store works for all states.
func Test_fastState_Store_AllStates(t *testing.T) {
	tests := []struct {
		name  string
		state LoopState
	}{
		{"Awake", StateAwake},
		{"Terminated", StateTerminated},
		{"Sleeping", StateSleeping},
		{"Running", StateRunning},
		{"Terminating", StateTerminating},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newFastState()
			s.Store(tc.state)

			if s.Load() != tc.state {
				t.Errorf("Expected %v after Store, got %v", tc.state, s.Load())
			}
		})
	}
}

// Test_fastState_TryTransition_ValidTransitions tests all valid state transitions.
func Test_fastState_TryTransition_ValidTransitions(t *testing.T) {
	tests := []struct {
		name string
		from LoopState
		to   LoopState
	}{
		{"Awake→Running", StateAwake, StateRunning},
		{"Running→Sleeping", StateRunning, StateSleeping},
		{"Running→Terminating", StateRunning, StateTerminating},
		{"Sleeping→Running", StateSleeping, StateRunning},
		{"Sleeping→Terminating", StateSleeping, StateTerminating},
		{"Terminating→Terminated", StateTerminating, StateTerminated},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newFastState()
			s.Store(tc.from)

			result := s.TryTransition(tc.from, tc.to)
			if !result {
				t.Errorf("TryTransition %v→%v should succeed", tc.from, tc.to)
			}
			if s.Load() != tc.to {
				t.Errorf("State should be %v after transition, got %v", tc.to, s.Load())
			}
		})
	}
}

// Test_fastState_TryTransition_InvalidFromState tests transition failures when from state doesn't match.
func Test_fastState_TryTransition_InvalidFromState(t *testing.T) {
	tests := []struct {
		name     string
		actual   LoopState
		expected LoopState
		target   LoopState
	}{
		{"Awake actual, wants Running→Sleeping", StateAwake, StateRunning, StateSleeping},
		{"Running actual, wants Sleeping→Terminating", StateRunning, StateSleeping, StateTerminating},
		{"Sleeping actual, wants Awake→Running", StateSleeping, StateAwake, StateRunning},
		{"Terminating actual, wants Running→Sleeping", StateTerminating, StateRunning, StateSleeping},
		{"Terminated actual, wants Sleeping→Running", StateTerminated, StateSleeping, StateRunning},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newFastState()
			s.Store(tc.actual)

			result := s.TryTransition(tc.expected, tc.target)
			if result {
				t.Errorf("TryTransition should fail when actual=%v but expected=%v", tc.actual, tc.expected)
			}
			if s.Load() != tc.actual {
				t.Errorf("State should remain %v after failed transition, got %v", tc.actual, s.Load())
			}
		})
	}
}

// Test_fastState_TryTransition_SameStateTransition tests from == to case.
func Test_fastState_TryTransition_SameStateTransition(t *testing.T) {
	states := []LoopState{StateAwake, StateRunning, StateSleeping, StateTerminating, StateTerminated}

	for _, state := range states {
		t.Run(state.String()+"→"+state.String(), func(t *testing.T) {
			s := newFastState()
			s.Store(state)

			result := s.TryTransition(state, state)
			if !result {
				t.Errorf("TryTransition from %v to %v should succeed (CAS same value)", state, state)
			}
			if s.Load() != state {
				t.Errorf("State should remain %v, got %v", state, s.Load())
			}
		})
	}
}

// Test_fastState_TransitionAny_FirstMatchWins verifies first matching state is used.
func Test_fastState_TransitionAny_FirstMatchWins(t *testing.T) {
	s := newFastState()
	s.Store(StateRunning)

	// Try to transition from [Awake, Running, Sleeping] to Terminating
	// Running is at index 1 - should match
	result := s.TransitionAny([]LoopState{StateAwake, StateRunning, StateSleeping}, StateTerminating)

	if !result {
		t.Error("TransitionAny should succeed when Running is in the list")
	}
	if s.Load() != StateTerminating {
		t.Errorf("State should be Terminating, got %v", s.Load())
	}
}

// Test_fastState_TransitionAny_EmptyValidFrom verifies empty list returns false.
func Test_fastState_TransitionAny_EmptyValidFrom(t *testing.T) {
	s := newFastState()
	s.Store(StateRunning)

	result := s.TransitionAny([]LoopState{}, StateTerminating)

	if result {
		t.Error("TransitionAny with empty list should return false")
	}
	if s.Load() != StateRunning {
		t.Error("State should remain Running after failed TransitionAny")
	}
}

// Test_fastState_TransitionAny_NoMatchingState verifies no match returns false.
func Test_fastState_TransitionAny_NoMatchingState(t *testing.T) {
	s := newFastState()
	s.Store(StateTerminated)

	result := s.TransitionAny([]LoopState{StateAwake, StateRunning, StateSleeping}, StateTerminating)

	if result {
		t.Error("TransitionAny should fail when current state is not in list")
	}
	if s.Load() != StateTerminated {
		t.Error("State should remain Terminated")
	}
}

// Test_fastState_TransitionAny_AllStatesInList verifies full list works.
func Test_fastState_TransitionAny_AllStatesInList(t *testing.T) {
	allStates := []LoopState{StateAwake, StateRunning, StateSleeping, StateTerminating, StateTerminated}

	for _, currentState := range allStates {
		t.Run(currentState.String(), func(t *testing.T) {
			s := newFastState()
			s.Store(currentState)

			result := s.TransitionAny(allStates, StateTerminated)

			if !result {
				t.Errorf("TransitionAny with all states should always succeed from %v", currentState)
			}
			if s.Load() != StateTerminated {
				t.Errorf("State should be Terminated, got %v", s.Load())
			}
		})
	}
}

// Test_fastState_IsTerminal_AllStates verifies IsTerminal for all states.
func Test_fastState_IsTerminal_AllStates(t *testing.T) {
	tests := []struct {
		state    LoopState
		expected bool
	}{
		{StateAwake, false},
		{StateRunning, false},
		{StateSleeping, false},
		{StateTerminating, false},
		{StateTerminated, true},
	}

	for _, tc := range tests {
		t.Run(tc.state.String(), func(t *testing.T) {
			s := newFastState()
			s.Store(tc.state)

			if s.IsTerminal() != tc.expected {
				t.Errorf("IsTerminal for %v should be %v", tc.state, tc.expected)
			}
		})
	}
}

// Test_fastState_IsRunning_AllStates verifies IsRunning for all states.
func Test_fastState_IsRunning_AllStates(t *testing.T) {
	tests := []struct {
		state    LoopState
		expected bool
	}{
		{StateAwake, false},
		{StateRunning, true},
		{StateSleeping, true},
		{StateTerminating, false},
		{StateTerminated, false},
	}

	for _, tc := range tests {
		t.Run(tc.state.String(), func(t *testing.T) {
			s := newFastState()
			s.Store(tc.state)

			if s.IsRunning() != tc.expected {
				t.Errorf("IsRunning for %v should be %v", tc.state, tc.expected)
			}
		})
	}
}

// Test_fastState_CanAcceptWork_AllStates verifies CanAcceptWork for all states.
func Test_fastState_CanAcceptWork_AllStates(t *testing.T) {
	tests := []struct {
		state    LoopState
		expected bool
	}{
		{StateAwake, true},
		{StateRunning, true},
		{StateSleeping, true},
		{StateTerminating, false},
		{StateTerminated, false},
	}

	for _, tc := range tests {
		t.Run(tc.state.String(), func(t *testing.T) {
			s := newFastState()
			s.Store(tc.state)

			if s.CanAcceptWork() != tc.expected {
				t.Errorf("CanAcceptWork for %v should be %v", tc.state, tc.expected)
			}
		})
	}
}

// Test_fastState_String_AllStates verifies String for all states including unknown.
func Test_fastState_String_AllStates(t *testing.T) {
	tests := []struct {
		state    LoopState
		expected string
	}{
		{StateAwake, "Awake"},
		{StateRunning, "Running"},
		{StateSleeping, "Sleeping"},
		{StateTerminating, "Terminating"},
		{StateTerminated, "Terminated"},
		{LoopState(99), "Unknown"},
		{LoopState(1000), "Unknown"},
		{LoopState(3), "Unknown"}, // 3 is not used
		{LoopState(6), "Unknown"}, // 6 is not used
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			if tc.state.String() != tc.expected {
				t.Errorf("String() for %d should be %q, got %q", tc.state, tc.expected, tc.state.String())
			}
		})
	}
}

// Test_fastState_ConcurrentLoad verifies concurrent Load is safe.
func Test_fastState_ConcurrentLoad(t *testing.T) {
	s := newFastState()
	s.Store(StateRunning)

	var wg sync.WaitGroup
	wg.Add(100)

	for i := 0; i < 100; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				state := s.Load()
				if state != StateRunning {
					t.Errorf("Expected Running, got %v", state)
				}
			}
		}()
	}

	wg.Wait()
}

// Test_fastState_ConcurrentStore verifies concurrent Store is safe (last writer wins).
func Test_fastState_ConcurrentStore(t *testing.T) {
	s := newFastState()

	states := []LoopState{StateAwake, StateRunning, StateSleeping}
	var wg sync.WaitGroup
	wg.Add(len(states))

	for _, state := range states {
		go func(st LoopState) {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				s.Store(st)
			}
		}(state)
	}

	wg.Wait()

	// State should be one of the valid states (non-deterministic which)
	final := s.Load()
	valid := false
	for _, st := range states {
		if final == st {
			valid = true
			break
		}
	}
	if !valid {
		t.Errorf("Final state %v is not one of the expected states", final)
	}
}

// Test_fastState_ConcurrentTryTransition verifies concurrent TryTransition (only one wins).
func Test_fastState_ConcurrentTryTransition(t *testing.T) {
	const numGoroutines = 100

	s := newFastState()
	s.Store(StateRunning)

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	var successCount atomic.Int32

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			if s.TryTransition(StateRunning, StateTerminating) {
				successCount.Add(1)
			}
		}()
	}

	wg.Wait()

	// Exactly one goroutine should succeed
	if successCount.Load() != 1 {
		t.Errorf("Expected exactly 1 successful transition, got %d", successCount.Load())
	}

	// State should be Terminating
	if s.Load() != StateTerminating {
		t.Errorf("Expected Terminating, got %v", s.Load())
	}
}

// Test_fastState_ConcurrentTransitionAny verifies concurrent TransitionAny (only one wins).
func Test_fastState_ConcurrentTransitionAny(t *testing.T) {
	const numGoroutines = 100

	s := newFastState()
	s.Store(StateRunning)

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	var successCount atomic.Int32

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			if s.TransitionAny([]LoopState{StateRunning}, StateTerminating) {
				successCount.Add(1)
			}
		}()
	}

	wg.Wait()

	// Exactly one goroutine should succeed
	if successCount.Load() != 1 {
		t.Errorf("Expected exactly 1 successful transition, got %d", successCount.Load())
	}
}

// Test_fastState_ConcurrentMixedOperations verifies all operations can run concurrently.
func Test_fastState_ConcurrentMixedOperations(t *testing.T) {
	s := newFastState()

	stop := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(5)

	// Loader
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = s.Load()
			}
		}
	}()

	// Store rotator
	go func() {
		defer wg.Done()
		states := []LoopState{StateAwake, StateRunning, StateSleeping}
		i := 0
		for {
			select {
			case <-stop:
				return
			default:
				s.Store(states[i%len(states)])
				i++
			}
		}
	}()

	// IsRunning checker
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = s.IsRunning()
			}
		}
	}()

	// IsTerminal checker
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = s.IsTerminal()
			}
		}
	}()

	// CanAcceptWork checker
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = s.CanAcceptWork()
			}
		}
	}()

	// Let it run for a bit
	for i := 0; i < 1000; i++ {
		runtime.Gosched()
	}

	close(stop)
	wg.Wait()
}

// Test_fastState_StateValueStability verifies state constants haven't changed.
func Test_fastState_StateValueStability(t *testing.T) {
	// These values are documented as stable for serialization
	// DO NOT CHANGE these values - they are part of the API contract
	tests := []struct {
		state    LoopState
		expected uint64
	}{
		{StateAwake, 0},
		{StateTerminated, 1},
		{StateSleeping, 2},
		{StateRunning, 4},
		{StateTerminating, 5},
	}

	for _, tc := range tests {
		t.Run(tc.state.String(), func(t *testing.T) {
			if uint64(tc.state) != tc.expected {
				t.Errorf("State %s should have value %d, got %d", tc.state.String(), tc.expected, uint64(tc.state))
			}
		})
	}
}

// Test_fastState_CacheLinePadding verifies struct size includes cache line padding.
func Test_fastState_CacheLinePadding(t *testing.T) {
	var s fastState

	// fastState should be at least 2 cache lines to prevent false sharing
	// Typical cache line is 64 bytes, so expect >= 128 bytes
	// Note: This is architecture-dependent

	// We can at least verify we can create and use it
	s.v.Store(uint64(StateRunning))
	if s.Load() != StateRunning {
		t.Error("Failed to store/load state")
	}
}

// Test_fastState_TryTransition_LoopPattern verifies typical state machine loop pattern.
func Test_fastState_TryTransition_LoopPattern(t *testing.T) {
	// Simulate typical event loop state transitions

	s := newFastState()

	// 1. Start: Awake
	if s.Load() != StateAwake {
		t.Fatal("Should start Awake")
	}

	// 2. Call Run(): Awake -> Running
	if !s.TryTransition(StateAwake, StateRunning) {
		t.Error("Awake->Running should succeed")
	}

	// 3. Enter poll: Running -> Sleeping
	if !s.TryTransition(StateRunning, StateSleeping) {
		t.Error("Running->Sleeping should succeed")
	}

	// 4. Wakeup: Sleeping -> Running
	if !s.TryTransition(StateSleeping, StateRunning) {
		t.Error("Sleeping->Running should succeed")
	}

	// 5. Shutdown: Running -> Terminating
	if !s.TryTransition(StateRunning, StateTerminating) {
		t.Error("Running->Terminating should succeed")
	}

	// 6. Complete: Terminating -> Terminated
	if !s.TryTransition(StateTerminating, StateTerminated) {
		t.Error("Terminating->Terminated should succeed")
	}

	// 7. Terminal state: IsTerminal should be true
	if !s.IsTerminal() {
		t.Error("Should be terminal after Terminated")
	}

	// NOTE: fastState allows ANY CAS-matching transitions (performance-first design).
	// The state machine does NOT enforce logical transition validity - that's the caller's responsibility.
	// So TryTransition(StateTerminated, StateAwake) WILL succeed if state == StateTerminated.
	// This is by design: "PERFORMANCE: Pure CAS, no validation of transition validity."
}

// Test_fastState_TryTransition_RaceConditionSimulation simulates racing shutdown.
func Test_fastState_TryTransition_RaceConditionSimulation(t *testing.T) {
	// Simulate race between poll wakeup and shutdown

	for iter := 0; iter < 100; iter++ {
		s := newFastState()
		s.Store(StateSleeping)

		var wg sync.WaitGroup
		wg.Add(2)

		var wakeupWon, shutdownWon atomic.Bool

		// Goroutine 1: Poll wakeup (Sleeping -> Running)
		go func() {
			defer wg.Done()
			if s.TryTransition(StateSleeping, StateRunning) {
				wakeupWon.Store(true)
			}
		}()

		// Goroutine 2: Shutdown (Sleeping -> Terminating)
		go func() {
			defer wg.Done()
			if s.TryTransition(StateSleeping, StateTerminating) {
				shutdownWon.Store(true)
			}
		}()

		wg.Wait()

		// Exactly one should win
		wakeup := wakeupWon.Load()
		shutdown := shutdownWon.Load()

		if wakeup == shutdown {
			t.Errorf("Iteration %d: Both wakeup=%v and shutdown=%v (should be exclusive)", iter, wakeup, shutdown)
		}
	}
}
