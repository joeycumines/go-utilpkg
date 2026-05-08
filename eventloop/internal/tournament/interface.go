// Package tournament provides a longitudinal test suite for comparing current,
// historical, and external event-loop implementations.
//
// The tournament tests correctness, performance, robustness, and memory behavior
// across all implementations to identify trade-offs and ensure API compatibility.
package tournament

import (
	"context"
	"slices"
	"time"
)

// EventLoop is the common interface that all event loop implementations must satisfy.
// This defines the minimal API surface required for tournament testing.
type EventLoop interface {
	// Run begins the event loop and BLOCKS until the loop is FULLY stopped.
	// Returns an error if the loop is already running or if context is cancelled.
	Run(ctx context.Context) error

	// Shutdown requests graceful shutdown and blocks according to the historical
	// implementation's contract. CapabilityGracefulDrain identifies variants
	// that guarantee completion of every preaccepted task.
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

// MicrotaskEventLoop extends EventLoop with optional capabilities that some
// implementations may provide.
type MicrotaskEventLoop interface {
	EventLoop

	// ScheduleMicrotask schedules a microtask (if supported).
	// Not all implementations may support this.
	ScheduleMicrotask(fn func()) error
}

// ReadinessEventLoop exposes the common readable-descriptor surface shared by
// the readiness-capable historical schedulers.
type ReadinessEventLoop interface {
	EventLoop

	RegisterReadable(fd int, fn func()) error
	UnregisterReadiness(fd int) error
}

// TimerScheduleEventLoop exposes the schedule-only surface shared by the
// historical alternates and the Goja baseline. It intentionally returns no
// timer identity: those implementations never exposed public cancellation.
type TimerScheduleEventLoop interface {
	EventLoop

	ScheduleTimer(delay time.Duration, fn func()) error
}

// LoopFactory creates a new event loop instance.
type LoopFactory func() (EventLoop, error)

// Implementation represents a named event loop implementation.
type Implementation struct { // betteralign:ignore
	Name          string
	VariantID     string
	SourcePackage string
	OriginCommit  string
	OriginTree    string
	Capabilities  []string
	Factory       LoopFactory
}

func (i Implementation) HasCapability(capability string) bool {
	return slices.Contains(i.Capabilities, capability)
}

const (
	CapabilityMicrotask      = "microtask"
	CapabilityReadiness      = "readiness"
	CapabilityInternalSubmit = "internal-submit"
	CapabilityGracefulDrain  = "graceful-drain"
)
