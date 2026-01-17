package alternateone

import (
	"fmt"
	"slices"
	"sync/atomic"
	"time"
)

// LoopState represents the current state of the event loop.
//
// State Machine (Strict Validation):
//
//	StateAwake (0) → StateRunning (4)       [Start()]
//	StateRunning (4) → StateSleeping (2)    [poll() via CAS]
//	StateRunning (4) → StateTerminating (3) [Stop()]
//	StateSleeping (2) → StateRunning (4)    [poll() wake via CAS]
//	StateSleeping (2) → StateTerminating (3) [Stop()]
//	StateTerminating (3) → StateTerminated (1) [shutdown]
//	StateTerminated (1) → (terminal)
//
// SAFETY: All transitions are validated against the transition table.
// Invalid transitions cause a panic in this safety-first implementation.
type LoopState int32

const (
	// StateAwake indicates the loop has been created but not started.
	StateAwake LoopState = 0
	// StateTerminated indicates the loop has been stopped and is fully shut down.
	StateTerminated LoopState = 1
	// StateSleeping indicates the loop is blocked in poll waiting for events.
	StateSleeping LoopState = 2
	// StateTerminating indicates shutdown has been requested but not completed.
	StateTerminating LoopState = 3
	// StateRunning indicates the loop is actively processing tasks.
	StateRunning LoopState = 4
)

// String returns a human-readable representation of the state.
func (s LoopState) String() string {
	switch s {
	case StateAwake:
		return "Awake"
	case StateTerminated:
		return "Terminated"
	case StateSleeping:
		return "Sleeping"
	case StateTerminating:
		return "Terminating"
	case StateRunning:
		return "Running"
	default:
		return fmt.Sprintf("Unknown(%d)", s)
	}
}

// validTransitions defines all valid state transitions.
// SAFETY: Invalid transitions will cause a panic.
var validTransitions = map[LoopState][]LoopState{
	StateAwake:       {StateRunning, StateTerminating},
	StateRunning:     {StateSleeping, StateTerminating},
	StateSleeping:    {StateRunning, StateTerminating},
	StateTerminating: {StateTerminated},
	StateTerminated:  {}, // terminal state - no transitions allowed
}

// StateObserver is notified of state transitions.
// Useful for debugging and monitoring.
type StateObserver interface {
	OnTransition(from, to LoopState, timestamp time.Time)
}

// SafeStateMachine provides strict state transition validation.
// All transitions are validated against the transition table.
// Invalid transitions cause a panic.
type SafeStateMachine struct {
	observer StateObserver
	val      atomic.Int32
}

// NewSafeStateMachine creates a new state machine in the Awake state.
func NewSafeStateMachine(observer StateObserver) *SafeStateMachine {
	sm := &SafeStateMachine{
		observer: observer,
	}
	sm.val.Store(int32(StateAwake))
	return sm
}

// Load returns the current state atomically.
func (sm *SafeStateMachine) Load() LoopState {
	return LoopState(sm.val.Load())
}

// Transition attempts to transition from the expected state to the new state.
// Returns true if the transition was successful (CAS succeeded).
// PANICS if the transition is invalid according to the transition table.
func (sm *SafeStateMachine) Transition(from, to LoopState) bool {
	// Validate the transition is allowed
	if !sm.isValidTransition(from, to) {
		panic(&TransitionError{From: from, To: to})
	}

	// Attempt the atomic transition
	if !sm.val.CompareAndSwap(int32(from), int32(to)) {
		return false
	}

	// Notify observer if present
	if sm.observer != nil {
		sm.observer.OnTransition(from, to, time.Now())
	}
	return true
}

// ForceTerminated forces the state to Terminated.
// This is used during shutdown when we need to set the final state.
// SAFETY: Only valid from StateTerminating state, or idempotent if already Terminated.
func (sm *SafeStateMachine) ForceTerminated() {
	from := sm.Load()
	if from == StateTerminated {
		// Already terminated - idempotent
		return
	}
	if from != StateTerminating {
		panic(&TransitionError{From: from, To: StateTerminated})
	}
	sm.val.Store(int32(StateTerminated))
	if sm.observer != nil {
		sm.observer.OnTransition(from, StateTerminated, time.Now())
	}
}

// isValidTransition checks if a transition is allowed.
func (sm *SafeStateMachine) isValidTransition(from, to LoopState) bool {
	validTargets, ok := validTransitions[from]
	if !ok {
		return false
	}
	return slices.Contains(validTargets, to)
}

// IsTerminal returns true if the current state is terminal (Terminated).
func (sm *SafeStateMachine) IsTerminal() bool {
	return sm.Load() == StateTerminated
}

// IsRunning returns true if the loop is currently running or sleeping.
func (sm *SafeStateMachine) IsRunning() bool {
	state := sm.Load()
	return state == StateRunning || state == StateSleeping
}

// CanAcceptWork returns true if the loop can accept new work.
// Returns false during terminating or terminated states.
func (sm *SafeStateMachine) CanAcceptWork() bool {
	state := sm.Load()
	return state == StateAwake || state == StateRunning || state == StateSleeping
}
