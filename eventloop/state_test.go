package eventloop

import (
	"sync"
	"sync/atomic"
	"testing"
)

// fastState Full Coverage
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

// Test_fastState_TryTransition_SameStateTransition tests that identity
// transitions (from == to) are rejected to prevent re-entrancy bugs.
func Test_fastState_TryTransition_SameStateTransition(t *testing.T) {
	states := []LoopState{StateAwake, StateRunning, StateSleeping, StateTerminating, StateTerminated}

	for _, state := range states {
		t.Run(state.String()+"→"+state.String(), func(t *testing.T) {
			s := newFastState()
			s.Store(state)

			result := s.TryTransition(state, state)
			if result {
				t.Errorf("TryTransition from %v to %v should be rejected (identity)", state, state)
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

// Test_fastState_TransitionAny_AllStatesInList verifies that TransitionAny
// succeeds for distinct-state transitions and rejects identity transitions.
func Test_fastState_TransitionAny_AllStatesInList(t *testing.T) {
	allStates := []LoopState{StateAwake, StateRunning, StateSleeping, StateTerminating, StateTerminated}

	for _, currentState := range allStates {
		t.Run(currentState.String(), func(t *testing.T) {
			s := newFastState()
			s.Store(currentState)

			result := s.TransitionAny(allStates, StateTerminated)

			if currentState == StateTerminated {
				// Identity transition: Terminated→Terminated must be rejected
				if result {
					t.Error("TransitionAny should reject identity transition Terminated→Terminated")
				}
				if s.Load() != StateTerminated {
					t.Errorf("State should remain Terminated, got %v", s.Load())
				}
			} else {
				if !result {
					t.Errorf("TransitionAny with all states should succeed from %v", currentState)
				}
				if s.Load() != StateTerminated {
					t.Errorf("State should be Terminated, got %v", s.Load())
				}
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

// Test_fastState_ConcurrentTryTransition verifies concurrent TryTransition (only one wins).
func Test_fastState_ConcurrentTryTransition(t *testing.T) {
	const numGoroutines = 100

	s := newFastState()
	s.Store(StateRunning)

	start := make(chan struct{})
	startNow := contractRelease(t, start)
	ready := make(chan struct{}, numGoroutines)
	var (
		workers      sync.WaitGroup
		successCount atomic.Int32
	)

	for range numGoroutines {
		workers.Go(func() {
			ready <- struct{}{}
			<-start
			if s.TryTransition(StateRunning, StateTerminating) {
				successCount.Add(1)
			}
		})
	}
	for range numGoroutines {
		waitContractSignal(t, ready, "concurrent TryTransition worker readiness")
	}
	startNow()
	workersDone := make(chan struct{})
	go func() {
		workers.Wait()
		close(workersDone)
	}()
	waitContractSignal(t, workersDone, "concurrent TryTransition operations")

	// Exactly one goroutine should succeed
	if got := successCount.Load(); got != 1 {
		t.Fatalf("successful TryTransition calls = %d, want 1", got)
	}

	// State should be Terminating
	if got := s.Load(); got != StateTerminating {
		t.Fatalf("state after concurrent TryTransition = %v, want %v", got, StateTerminating)
	}
}

// Test_fastState_ConcurrentTransitionAny verifies concurrent TransitionAny (only one wins).
func Test_fastState_ConcurrentTransitionAny(t *testing.T) {
	const numGoroutines = 100

	s := newFastState()
	s.Store(StateRunning)

	start := make(chan struct{})
	startNow := contractRelease(t, start)
	ready := make(chan struct{}, numGoroutines)
	var (
		workers      sync.WaitGroup
		successCount atomic.Int32
	)

	for range numGoroutines {
		workers.Go(func() {
			ready <- struct{}{}
			<-start
			if s.TransitionAny([]LoopState{StateRunning}, StateTerminating) {
				successCount.Add(1)
			}
		})
	}
	for range numGoroutines {
		waitContractSignal(t, ready, "concurrent TransitionAny worker readiness")
	}
	startNow()
	workersDone := make(chan struct{})
	go func() {
		workers.Wait()
		close(workersDone)
	}()
	waitContractSignal(t, workersDone, "concurrent TransitionAny operations")

	// Exactly one goroutine should succeed
	if got := successCount.Load(); got != 1 {
		t.Fatalf("successful TransitionAny calls = %d, want 1", got)
	}
	if got := s.Load(); got != StateTerminating {
		t.Fatalf("state after concurrent TransitionAny = %v, want %v", got, StateTerminating)
	}
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
