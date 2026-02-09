package eventloop

import (
	"testing"
)

// Test_FastsState_IsTerminal tests IsTerminal state query method
func Test_fastState_IsTerminal(t *testing.T) {
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
				var fs fastState
				fs.Store(state)

				if fs.IsTerminal() {
					t.Errorf("IsTerminal() returned true for %v (expected false)", state)
				}
			})
		}
	})

	t.Run("IsTerminal returns true for StateTerminated", func(t *testing.T) {
		t.Parallel()

		var fs fastState
		fs.Store(StateTerminated)

		if !fs.IsTerminal() {
			t.Error("IsTerminal() returned false for StateTerminated (expected true)")
		}
	})
}

// Test_fastState_CanAcceptWork tests CanAcceptWork state query method
func Test_fastState_CanAcceptWork(t *testing.T) {
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
				var fs fastState
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
				var fs fastState
				fs.Store(state)

				if fs.CanAcceptWork() {
					t.Errorf("CanAcceptWork() returned true for %v (expected false)", state)
				}
			})
		}
	})
}

// Test_fastState_IsRunning tests IsRunning state query method
func Test_fastState_IsRunning(t *testing.T) {
	t.Parallel()

	t.Run("IsRunning returns true for Running states", func(t *testing.T) {
		t.Parallel()

		runningStates := []LoopState{
			StateRunning,
			StateSleeping,
		}

		for _, state := range runningStates {
			t.Run(state.String(), func(t *testing.T) {
				var fs fastState
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
				var fs fastState
				fs.Store(state)

				if fs.IsRunning() {
					t.Errorf("IsRunning() returned true for %v (expected false)", state)
				}
			})
		}
	})
}

// Test_fastState_TransitionAny tests TransitionAny state transition method
func Test_fastState_TransitionAny(t *testing.T) {
	t.Parallel()

	t.Run("TransitionAny succeeds for valid transition", func(t *testing.T) {
		t.Parallel()

		var fs fastState
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

		var fs fastState
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
		var fs1 fastState
		fs1.Store(StateAwake)
		if !fs1.TransitionAny(validFrom, to) {
			t.Error("TransitionAny failed from StateAwake")
		}

		// Test from StateRunning
		var fs2 fastState
		fs2.Store(StateRunning)
		if !fs2.TransitionAny(validFrom, to) {
			t.Error("TransitionAny failed from StateRunning")
		}
	})

	t.Run("TransitionAny tries all valid sources", func(t *testing.T) {
		t.Parallel()

		var fs fastState
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
}

// Test_fastState_TryTransition_Exact tests exact state transitions
func Test_fastState_TryTransition_Exact(t *testing.T) {
	t.Parallel()

	t.Run("Transition succeeds for exact state match", func(t *testing.T) {
		t.Parallel()

		var fs fastState
		fs.Store(StateRunning)

		from := StateRunning
		to := StateSleeping

		if !fs.TryTransition(from, to) {
			t.Error("TryTransition failed for exact state match")
		}

		// Verify state changed
		if fs.Load() != StateSleeping {
			t.Fatalf("State not changed to %v, got %v", StateSleeping, fs.Load())
		}
	})

	t.Run("Transition fails for state mismatch", func(t *testing.T) {
		t.Parallel()

		var fs fastState
		fs.Store(StateAwake)

		from := StateRunning
		to := StateSleeping

		if fs.TryTransition(from, to) {
			t.Error("TryTransition succeeded for state mismatch")
		}

		// Verify state unchanged
		if fs.Load() != StateAwake {
			t.Fatalf("State changed unexpectedly: %v", fs.Load())
		}
	})
}

// Test_LoopState_String tests that String() returns non-empty values
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
}
