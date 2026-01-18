package gojabaseline

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
	gojaloop "github.com/dop251/goja_nodejs/eventloop"
)

var (
	// ErrLoopAlreadyRunning is returned when Run() is called on a running loop.
	ErrLoopAlreadyRunning = errors.New("gojabaseline: loop is already running")
	// ErrLoopTerminated is returned when operations are attempted on a stopped loop.
	ErrLoopTerminated = errors.New("gojabaseline: loop has been terminated")
)

// Loop wraps goja_nodejs eventloop to implement the tournament interface.
//
// IMPORTANT SEMANTIC DIFFERENCE:
// goja's eventloop is designed around Node.js semantics where the loop
// automatically exits when there are no pending jobs. Our tournament interface
// requires Run() to block indefinitely until Shutdown() or Close() is called.
//
// We bridge this gap by using StartInForeground() which blocks until Stop()
// is called, rather than Run() which exits when idle.
type Loop struct {
	inner        *gojaloop.EventLoop
	loopDone     chan struct{}
	shutdownOnce sync.Once
	running      atomic.Bool
	stopped      atomic.Bool
}

// New creates a new goja-based event loop.
func New() (*Loop, error) {
	return &Loop{
		inner:    gojaloop.NewEventLoop(),
		loopDone: make(chan struct{}),
	}, nil
}

// Run runs the event loop and blocks until stopped.
//
// Unlike goja's native Run() which exits when idle, this method blocks
// until Shutdown() or Close() is called, matching our tournament interface.
func (l *Loop) Run(ctx context.Context) error {
	if !l.running.CompareAndSwap(false, true) {
		return ErrLoopAlreadyRunning
	}
	if l.stopped.Load() {
		return ErrLoopTerminated
	}

	// Start context cancellation monitor
	go func() {
		select {
		case <-ctx.Done():
			// Context cancelled - trigger stop via Terminate for immediate exit
			l.inner.Terminate()
		case <-l.loopDone:
			// Loop already done, nothing to do
		}
	}()

	// StartInForeground blocks until Stop() is called, unlike Run() which
	// exits when there are no pending jobs. This gives us the blocking
	// semantics required by the tournament interface.
	l.inner.StartInForeground()

	// Signal completion
	close(l.loopDone)

	// Check if context was the cause
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

// Shutdown gracefully shuts down the event loop.
//
// goja's Stop() is synchronous - it blocks until the loop fully stops.
// We use sync.Once to ensure only one goroutine actually calls Stop(),
// and subsequent callers just wait for completion.
func (l *Loop) Shutdown(ctx context.Context) error {
	// Quick check for already-terminated
	if l.stopped.Load() {
		// Wait for loop to actually finish before returning
		select {
		case <-l.loopDone:
			return ErrLoopTerminated
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// Mark as stopped - first caller wins
	wasFirst := !l.stopped.Swap(true)

	if !l.running.Load() {
		return nil
	}

	// Only the first caller initiates the stop
	if wasFirst {
		l.shutdownOnce.Do(func() {
			// Use StopNoWait so we don't block - the loop will stop asynchronously
			// and StartInForeground will return, closing loopDone
			l.inner.StopNoWait()
		})
	}

	// All callers wait for the loop to finish
	select {
	case <-l.loopDone:
		if !wasFirst {
			return ErrLoopTerminated
		}
		return nil
	case <-ctx.Done():
		// Context expired - force terminate
		l.inner.Terminate()
		return ctx.Err()
	}
}

// Submit submits a task to the event loop.
//
// The task is wrapped with panic recovery since goja's RunOnLoop does not
// recover from panics in callbacks (unlike SetTimeout/SetInterval when using
// StartInForeground). This ensures the loop survives panicking tasks.
func (l *Loop) Submit(fn func()) error {
	if l.stopped.Load() {
		return ErrLoopTerminated
	}

	// Wrap the function with panic recovery since goja doesn't recover
	// from panics in RunOnLoop callbacks
	wrapped := func(*goja.Runtime) {
		defer func() {
			if r := recover(); r != nil {
				// Log the panic but don't propagate it
				// This mirrors how our other implementations handle panics
			}
		}()
		fn()
	}

	// RunOnLoop returns false if the loop is terminated
	if !l.inner.RunOnLoop(wrapped) {
		return ErrLoopTerminated
	}
	return nil
}

// SubmitInternal submits an internal task (same as Submit for goja).
// goja doesn't have an internal queue concept, so this is identical to Submit.
func (l *Loop) SubmitInternal(fn func()) error {
	return l.Submit(fn)
}

// Close immediately terminates the loop without waiting for pending jobs.
// Uses goja's Terminate() which clears all timers and stops immediately.
func (l *Loop) Close() error {
	l.stopped.Store(true)
	l.inner.Terminate()
	return nil
}

// ScheduleTimer schedules a function to run after a delay.
func (l *Loop) ScheduleTimer(delay time.Duration, fn func()) error {
	if l.stopped.Load() {
		return ErrLoopTerminated
	}

	l.inner.SetTimeout(func(*goja.Runtime) {
		fn()
	}, delay)
	return nil
}
