package eventloop

import (
	"sync/atomic"
)

// LoopState represents the current state of the event loop.
//
// State Machine (Performance-First Design):
//
//	StateAwake (0) → StateRunning (3)      [Run()]
//	StateRunning (3) → StateSleeping (2)   [poll() via CAS]
//	StateRunning (3) → StateTerminating (4) [Shutdown()]
//	StateSleeping (2) → StateRunning (3)   [poll() wake via CAS]
//	StateSleeping (2) → StateTerminating (4) [Shutdown()]
//	StateTerminating (4) → StateTerminated (1) [shutdown complete]
//	StateTerminated (1) → (terminal)
//
// State Transition Rules:
//   - Use TryTransition() (CAS) for temporary states (Running, Sleeping)
//   - Use Store() for irreversible states (Terminated)
//   - Using Store(Running) or Store(Sleeping) is a BUG (breaks CAS logic)
//
// NOTE: State values are intentionally ordered for backward compatibility
// with the original implementation spec (StateTerminated=1, StateSleeping=2).
type LoopState uint64

const (
	// StateAwake indicates the loop has been created but not started.
	StateAwake LoopState = 0
	// StateTerminated indicates the loop has been stopped and is fully shut down.
	// NOTE: This is value 1 for backward compatibility with original spec.
	StateTerminated LoopState = 1
	// StateSleeping indicates the loop is blocked in poll waiting for events.
	// NOTE: This is value 2 for backward compatibility with original spec.
	StateSleeping LoopState = 2
	// StateRunning indicates the loop is actively processing tasks.
	StateRunning LoopState = 3
	// StateTerminating indicates shutdown has been requested but not completed.
	StateTerminating LoopState = 4
)

// String returns a human-readable representation of the state.
func (s LoopState) String() string {
	switch s {
	case StateAwake:
		return "Awake"
	case StateRunning:
		return "Running"
	case StateSleeping:
		return "Sleeping"
	case StateTerminating:
		return "Terminating"
	case StateTerminated:
		return "Terminated"
	default:
		return "Unknown"
	}
}

// FastState is a lock-free state machine with cache-line padding.
//
// PERFORMANCE: Uses pure atomic CAS operations with no mutex.
// Cache-line padding prevents false sharing between cores.
type FastState struct { // betteralign:ignore
	_ [64]byte      // Cache line padding (before value) //nolint:unused
	v atomic.Uint64 // State value
	_ [56]byte      // Pad to complete cache line (64 - 8 = 56) //nolint:unused
}

// NewFastState creates a new state machine in the Awake state.
func NewFastState() *FastState {
	s := &FastState{}
	s.v.Store(uint64(StateAwake))
	return s
}

// Load returns the current state atomically.
// PERFORMANCE: No validation, trusts the stored value.
func (s *FastState) Load() LoopState {
	return LoopState(s.v.Load())
}

// Store atomically stores a new state.
// PERFORMANCE: No transition validation.
func (s *FastState) Store(state LoopState) {
	s.v.Store(uint64(state))
}

// TryTransition attempts to atomically transition from one state to another.
// Returns true if the transition was successful.
// PERFORMANCE: Pure CAS, no validation of transition validity.
func (s *FastState) TryTransition(from, to LoopState) bool {
	return s.v.CompareAndSwap(uint64(from), uint64(to))
}

// TransitionAny attempts to transition from any valid source state to the target.
// Returns true if the transition was successful.
// PERFORMANCE: Uses CAS loop for any-to-target transitions.
func (s *FastState) TransitionAny(validFrom []LoopState, to LoopState) bool {
	for _, from := range validFrom {
		if s.v.CompareAndSwap(uint64(from), uint64(to)) {
			return true
		}
	}
	return false
}

// IsTerminal returns true if the current state is terminal (Terminated).
func (s *FastState) IsTerminal() bool {
	return s.Load() == StateTerminated
}

// IsRunning returns true if the loop is currently running or sleeping.
func (s *FastState) IsRunning() bool {
	state := s.Load()
	return state == StateRunning || state == StateSleeping
}

// CanAcceptWork returns true if the loop can accept new work.
func (s *FastState) CanAcceptWork() bool {
	state := s.Load()
	return state == StateAwake || state == StateRunning || state == StateSleeping
}
