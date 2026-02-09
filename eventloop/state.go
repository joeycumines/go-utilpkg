package eventloop

import (
	"sync/atomic"
)

// LoopState represents the current state of the event loop.
//
// State Machine (Performance-First Design):
//
//	StateAwake (0) → StateRunning (4)      [Run()]
//	StateRunning (4) → StateSleeping (2)   [poll() via CAS]
//	StateRunning (4) → StateTerminating (5) [Shutdown()]
//	StateSleeping (2) → StateRunning (4)   [poll() wake via CAS]
//	StateSleeping (2) → StateTerminating (5) [Shutdown()]
//	StateTerminating (5) → StateTerminated (1) [shutdown complete]
//	StateTerminated (1) → (terminal)
//
// State Transition Rules:
//   - Use TryTransition() (CAS) for temporary states (Running, Sleeping)
//   - Use Store() for irreversible states (Terminated)
//   - Using Store(Running) or Store(Sleeping) is a BUG (breaks CAS logic)
//
// Note: State values (0, 1, 2, 4, 5) are deliberately non-sequential to preserve
// stable serialization. Do not renumber.
type LoopState uint64

const (
	StateAwake       LoopState = 0
	StateTerminated  LoopState = 1
	StateSleeping    LoopState = 2
	StateRunning     LoopState = 4
	StateTerminating LoopState = 5
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

// fastState is a lock-free state machine with cache-line padding.
// PERFORMANCE: Uses pure atomic CAS operations with no mutex.
// Cache-line padding prevents false sharing between cores.
type fastState struct { // betteralign:ignore
	_ [sizeOfCacheLine]byte                      // Cache line padding (before value) //nolint:unused
	v atomic.Uint64                              // State value
	_ [sizeOfCacheLine - sizeOfAtomicUint64]byte // Pad to complete cache line //nolint:unused
}

func newFastState() *fastState {
	s := &fastState{}
	s.v.Store(uint64(StateAwake))
	return s
}

// Load returns the current state atomically.
// PERFORMANCE: No validation, trusts the stored value.
func (s *fastState) Load() LoopState {
	return LoopState(s.v.Load())
}

// Store atomically stores a new state.
// PERFORMANCE: No transition validation.
func (s *fastState) Store(state LoopState) {
	s.v.Store(uint64(state))
}

// TryTransition attempts to atomically transition from one state to another.
// Returns true if the transition was successful.
// PERFORMANCE: Pure CAS, no validation of transition validity.
func (s *fastState) TryTransition(from, to LoopState) bool {
	return s.v.CompareAndSwap(uint64(from), uint64(to))
}

// TransitionAny attempts to transition from any valid source state to the target.
// Returns true if the transition was successful.
// PERFORMANCE: Uses CAS loop for any-to-target transitions.
func (s *fastState) TransitionAny(validFrom []LoopState, to LoopState) bool {
	for _, from := range validFrom {
		if s.v.CompareAndSwap(uint64(from), uint64(to)) {
			return true
		}
	}
	return false
}

// IsTerminal returns true if the current state is terminal.
func (s *fastState) IsTerminal() bool {
	return s.Load() == StateTerminated
}

// IsRunning returns true if the loop is currently running or sleeping.
func (s *fastState) IsRunning() bool {
	state := s.Load()
	return state == StateRunning || state == StateSleeping
}

// CanAcceptWork returns true if the loop can accept new work.
func (s *fastState) CanAcceptWork() bool {
	state := s.Load()
	return state == StateAwake || state == StateRunning || state == StateSleeping
}
