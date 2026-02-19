// Package tournament provides a test suite for comparing
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
	// Run begins the event loop and BLOCKS until the loop is FULLY stopped.
	// Returns an error if the loop is already running or if context is cancelled.
	Run(ctx context.Context) error

	// Shutdown gracefully shuts down the event loop and BLOCKS until complete.
	// Must have graceful shutdown semantics exactly like Go's http.Server.Shutdown(ctx).
	// Returns an error if context expires before shutdown completes or if already terminated.
	Shutdown(ctx context.Context) error

	// Submit submits a task to the external queue for execution on the loop.
	// Returns an error if the loop is terminated.
	Submit(fn func()) error

	// SubmitInternal submits a task to the internal priority queue.
	// Internal tasks bypass the tick budget and are processed before external tasks.
	// Returns an error if the loop is terminated.
	SubmitInternal(fn func()) error

	// Close immediately terminates the event loop without waiting for graceful shutdown.
	// Implements io.Closer semantics for immediate termination.
	Close() error
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
