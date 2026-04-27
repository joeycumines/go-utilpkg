package eventloop

import (
	"context"
	"errors"
	"fmt"
)

var (
	// ErrGoexit is returned/used to reject a promise when the goroutine exits via runtime.Goexit().
	ErrGoexit = errors.New("promise: goroutine exited via runtime.Goexit")

	// ErrPanic is returned when a promisified function panics.
	ErrPanic = errors.New("promise: goroutine panicked")
)

// PanicError wraps a panic value recovered from a [Loop.Promisify] goroutine.
type PanicError struct {
	// Value is the recovered panic value (may be any type, including error).
	Value any
}

// Error implements the error interface.
func (e PanicError) Error() string {
	return fmt.Sprintf("promise: goroutine panicked: %v", e.Value)
}

// Promisify executes the given function in a new goroutine and returns a Promise
// representing its result.
//
// This is the context-aware version that accepts a context and passes it to the
// function. The function can use ctx.Done() to detect cancellation.
//
// It ensures:
//   - Goexit Handler: Even if runtime.Goexit() is called, the promise is rejected rather than hanging indefinitely.
//   - Context Propagation: The context is passed to the user function.
//   - Single-Owner: Resolution goes through SubmitInternal to ensure promise resolution happens on the loop thread.
//   - Fallback: Direct resolution if SubmitInternal fails (e.g., during shutdown) to ensure promises always settle.
//   - Shutdown tracking: Uses promisifyWg to track in-flight goroutines so shutdown can wait for them.
//   - Atomic check: Checks state before adding to promisifyWg to prevent race with shutdown.
func (l *Loop) Promisify(ctx context.Context, fn func(ctx context.Context) (any, error)) Promise {
	// Lock promisifyMu to atomically check state and add to promisifyWg
	// This prevents race with shutdown which also holds promisifyMu
	l.promisifyMu.Lock()
	currentState := l.state.Load()
	if currentState == StateTerminating || currentState == StateTerminated {
		l.promisifyMu.Unlock()
		_, p := l.registry.NewPromise()
		p.Reject(ErrLoopTerminated)
		return p
	}

	// Reject during auto-exit quiescing window (GAP-AE-05).
	// Checked under promisifyMu for atomicity with the state check above.
	if l.quiescing.Load() {
		l.promisifyMu.Unlock()
		_, p := l.registry.NewPromise()
		p.Reject(ErrLoopTerminated)
		return p
	}

	_, p := l.registry.NewPromise()

	l.promisifyWg.Add(1)
	l.promisifyCount.Add(1)
	l.submissionEpoch.Add(1)
	l.promisifyMu.Unlock()

	go func() {
		defer l.promisifyWg.Done()
		defer func() {
			l.promisifyCount.Add(-1)
			// Wake the loop so it re-checks Alive() after the count changes.
			// Only needed when auto-exit is enabled: the loop may be blocked
			// in PollIO/fast-path select and needs to re-evaluate liveness.
			// When auto-exit is disabled, this is pure overhead (syscall).
			if l.autoExit {
				l.doWakeup()
			}
		}()

		// Completion flag to distinguish normal return from Goexit
		completed := false

		// Respect context cancellation
		select {
		case <-ctx.Done():
			completed = true
			if err := l.SubmitInternal(func() {
				p.Reject(ctx.Err())
			}); err != nil {
				p.Reject(ctx.Err()) // Fallback: direct resolution
			}
			return
		default:
		}

		defer func() {
			r := recover()
			if r != nil {
				// Panic detected
				panicErr := PanicError{Value: r}
				if err := l.SubmitInternal(func() {
					p.Reject(panicErr)
				}); err != nil {
					p.Reject(panicErr) // Fallback: direct resolution
				}
			} else if !completed {
				// Function ended but not via normal return -> Goexit (or panic(nil))
				if err := l.SubmitInternal(func() {
					p.Reject(ErrGoexit)
				}); err != nil {
					p.Reject(ErrGoexit) // Fallback: direct resolution
				}
			}
		}()

		res, err := fn(ctx)

		// Resolution goes through SubmitInternal to ensure single-owner
		if err != nil {
			if submitErr := l.SubmitInternal(func() {
				p.Reject(err)
			}); submitErr != nil {
				p.Reject(err) // Fallback: direct resolution
			}
		} else {
			if submitErr := l.SubmitInternal(func() {
				p.Resolve(res)
			}); submitErr != nil {
				// Loop terminated but operation succeeded
				p.Resolve(res) // Fallback: direct resolution
			}
		}
		completed = true
	}()

	return p
}
