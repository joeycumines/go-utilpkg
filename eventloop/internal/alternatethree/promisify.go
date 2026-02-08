//go:build linux || darwin

package alternatethree

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

// PanicError wraps a panic value.
type PanicError struct {
	Value any
}

func (e PanicError) Error() string {
	return fmt.Sprintf("promise: goroutine panicked: %v", e.Value)
}

// Promisify executes the given function in a new goroutine and returns a Promise
// representing its result.
//
// This is the context-aware version that accepts a context and passes it to the
// function. The function can use ctx.Done() to detect cancellation (D12).
//
// It implements:
//   - Task 9.2 (Goexit Handler): ensures that even if runtime.Goexit() is called,
//     the promise is rejected rather than hanging indefinitely.
//   - D12 (Context Propagation): the context is passed to the user function.
//   - D05 (Single-Owner): resolution goes through SubmitInternal to ensure
//     promise resolution happens on the loop thread.
//   - PROMISIFY FIX: Fallback to direct resolution if SubmitInternal fails
//     (e.g., during shutdown). This ensures promises always settle.
//   - C4 FIX: Uses promisifyWg to track in-flight goroutines so shutdown
//     can wait for them to complete before calling RejectAll.
func (l *Loop) Promisify(ctx context.Context, fn func(ctx context.Context) (Result, error)) Promise {
	// Task 2.2/2.4 integration: use registry
	_, p := l.registry.NewPromise()

	// C4 FIX: Track this goroutine so shutdown can wait for it
	l.promisifyWg.Add(1)

	go func() {
		// C4 FIX: Signal completion when done (even on panic)
		defer l.promisifyWg.Done()

		// Completion flag to distinguish normal return from Goexit
		completed := false

		// D12: Respect context cancellation
		select {
		case <-ctx.Done():
			// Defect 9 Fix: Set completed to prevent defer from triggering double-rejection
			completed = true
			// Context already cancelled - reject immediately via internal lane
			// PROMISIFY FIX: Fallback to direct reject if SubmitInternal fails
			if err := l.SubmitInternal(Task{Runnable: func() {
				p.Reject(ctx.Err())
			}}); err != nil {
				p.Reject(ctx.Err()) // Fallback: direct resolution
			}
			return
		default:
		}

		defer func() {
			r := recover()
			if r != nil {
				// Panic detected - D05: resolve via SubmitInternal
				// PROMISIFY FIX: Fallback to direct reject if SubmitInternal fails
				panicErr := PanicError{Value: r}
				if err := l.SubmitInternal(Task{Runnable: func() {
					p.Reject(panicErr)
				}}); err != nil {
					p.Reject(panicErr) // Fallback: direct resolution
				}
			} else {
				// recover() is nil
				if !completed {
					// Function ended but not via normal return -> Goexit (or panic(nil))
					// D05: resolve via SubmitInternal
					// PROMISIFY FIX: Fallback to direct reject if SubmitInternal fails
					if err := l.SubmitInternal(Task{Runnable: func() {
						p.Reject(ErrGoexit)
					}}); err != nil {
						p.Reject(ErrGoexit) // Fallback: direct resolution
					}
				}
			}
		}()

		// D12: Pass context to user function
		res, err := fn(ctx)

		// D05: Resolution goes through SubmitInternal to ensure single-owner
		// PROMISIFY FIX: Fallback to direct resolution if SubmitInternal fails
		if err != nil {
			if submitErr := l.SubmitInternal(Task{Runnable: func() {
				p.Reject(err)
			}}); submitErr != nil {
				p.Reject(err) // Fallback: direct resolution
			}
		} else {
			if submitErr := l.SubmitInternal(Task{Runnable: func() {
				p.Resolve(res)
			}}); submitErr != nil {
				// Loop terminated but operation succeeded
				// PROMISIFY FIX: Resolve directly - loop is gone so single-owner is moot
				// This ensures successful operations are not incorrectly reported as failure
				p.Resolve(res) // Fallback: direct resolution with result
			}
		}
		completed = true
	}()

	return p
}
