// Package tournament provides a comprehensive test suite for comparing
// three event loop implementations: Main (balanced), AlternateOne (maximum safety),
// and AlternateTwo (maximum performance).
//
// The tournament tests correctness, performance, robustness, and memory behavior
// across all implementations to identify trade-offs and ensure API compatibility.
package tournament

import (
	"context"
)

// EventLoop is the common interface that all event loop implementations must satisfy.
// This defines the minimal API surface required for tournament testing.
type EventLoop interface {
	// Start begins the event loop in a new goroutine.
	// Returns an error if the loop is already running or terminated.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the event loop.
	// Should wait for the loop to fully terminate before returning.
	Stop(ctx context.Context) error

	// Submit submits a task to the external queue for execution on the loop.
	// Returns an error if the loop is not running or terminated.
	Submit(fn func()) error

	// SubmitInternal submits a task to the internal priority queue.
	// Internal tasks bypass the tick budget and are processed before external tasks.
	// Returns an error if the loop is terminated.
	SubmitInternal(fn func()) error

	// Done returns a channel that is closed when the loop terminates.
	// Used to wait for loop completion after Stop().
	Done() <-chan struct{}
}

// FullEventLoop extends EventLoop with optional capabilities that some
// implementations may provide.
type FullEventLoop interface {
	EventLoop

	// ScheduleMicrotask schedules a microtask (if supported).
	// Not all implementations may support this.
	ScheduleMicrotask(fn func()) error
}

// LoopFactory creates a new event loop instance.
type LoopFactory func() (EventLoop, error)

// Implementation represents a named event loop implementation.
type Implementation struct { // betteralign:ignore
	Name    string
	Factory LoopFactory
}
