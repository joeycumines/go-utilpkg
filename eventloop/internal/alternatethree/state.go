package alternatethree

// LoopState represents the current state of the event loop.
//
// State Machine:
//
//	StateAwake (0) → StateRunning (4)      [Start()]
//	StateRunning (4) → StateSleeping (2)      [poll() via CAS]
//	StateRunning (4) → StateTerminating (3)   [Stop()]
//	StateSleeping (2) → StateRunning (4)      [poll() wake via CAS]
//	StateSleeping (2) → StateTerminating (3)   [Stop()]
//	StateTerminating (3) → StateTerminated (1)   [shutdown]
//	StateTerminated (1) → (terminal)
//
// State Transition Rules:
//   - Use `state.CompareAndSwap()` for temporary states (Running, Sleeping)
//   - Use `state.Store()` ONLY for irreversible states (Terminated)
//   - Using `Store(Running)` or `Store(Sleeping)` is a BUG (breaks CAS logic)
//
// This must be accessed atomically to prevent race conditions.
type LoopState int32

const (
	// StateAwake indicates the loop is actively processing tasks.
	// Producers should elide wake-up syscalls when seeing this state.
	StateAwake LoopState = 0

	// StateTerminated indicates the loop has been stopped and is shutting down.
	// Defect 6 Fix: Spec requires StateTerminated = 1
	StateTerminated LoopState = 1

	// StateSleeping indicates the loop is preparing to or currently blocked
	// in epoll_wait/kqueue. Producers must perform wake-up syscall when seeing this state.
	// Defect 6 Fix: Spec requires StateSleeping = 2
	StateSleeping LoopState = 2

	// StateTerminating indicates the loop shutdown has been requested but not completed.
	StateTerminating LoopState = 3

	// StateRunning indicates run() is actively executing.
	// Used to prevent multiple goroutines from executing run() concurrently.
	StateRunning LoopState = 4
)
