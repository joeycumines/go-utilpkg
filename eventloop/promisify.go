package eventloop

import (
	"context"
	"errors"
	"fmt"

	"github.com/joeycumines/goroutineid"
)

var (
	// ErrGoexit rejects a promise when its Go callback exits via runtime.Goexit.
	ErrGoexit = errors.New("eventloop: promise callback exited via runtime.Goexit")
)

// PanicError wraps a panic value recovered from a Go promise callback.
type PanicError struct {
	// Value is the recovered panic value (may be any type, including error).
	Value any
}

// Error implements the error interface.
func (e PanicError) Error() string {
	return fmt.Sprintf("eventloop: promise callback panicked: %v", e.Value)
}

// Promisify executes the given function in a new goroutine and returns a Promise
// representing its result.
//
// This is the context-aware version that accepts a context and passes it to the
// function. The function can use ctx.Done() to detect cancellation. A nil ctx
// is treated as [context.Background].
//
// It ensures:
//   - Goexit Handler: Even if runtime.Goexit() is called, the promise is rejected rather than hanging indefinitely.
//   - Context Propagation: The context is passed to the user function.
//   - Single-Owner: Resolution goes through SubmitInternal so promise settlement happens on the logical callback owner.
//   - Fallback: Direct resolution if SubmitInternal fails (e.g., during shutdown) to ensure promises always settle.
//   - Shutdown tracking: Uses promisifyWg so graceful Shutdown can wait for in-flight functions.
//     Immediate Close rejects the promise and returns without waiting for a user
//     function that already claimed execution. A committed worker that has not
//     claimed execution when Close wins skips the function entirely.
//   - Atomic check: Checks state before adding to promisifyWg to prevent race with shutdown.
func (l *Loop) Promisify(ctx context.Context, fn func(ctx context.Context) (any, error)) Promise {
	if fn == nil {
		panic("eventloop: nil Promisify callback")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Lock livenessMu and promisifyMu to atomically check terminal admission and
	// add to promisifyWg. Graceful Shutdown uses both locks; immediate Close
	// publishes terminal state under livenessMu and never waits for this worker.
	l.livenessMu.Lock()
	l.promisifyMu.Lock()
	currentState := l.state.Load()
	if currentState == StateTerminating || currentState == StateTerminated {
		l.promisifyMu.Unlock()
		l.livenessMu.Unlock()
		return newRejectedPromise(ErrLoopTerminated)
	}

	// Reject during active terminal drain or auto-exit quiescing window (GAP-AE-05).
	// Checked under promisifyMu for atomicity with the state check above.
	if err := l.rejectLivenessAddLocked(); err != nil {
		l.promisifyMu.Unlock()
		l.livenessMu.Unlock()
		return newRejectedPromise(err)
	}
	if l.testHooks != nil && l.testHooks.BeforePromisifyCommit != nil {
		l.testHooks.BeforePromisifyCommit()
	}

	p := l.registry.NewPromise()

	l.promisifyWg.Add(1)
	l.promisifyCount.Add(1)
	l.submissionEpoch.Add(1)
	l.promisifyMu.Unlock()
	l.livenessMu.Unlock()

	go func() {
		defer l.promisifyWg.Done()
		workerID := goroutineid.Get()
		l.promisifyWorkerIDs.Store(workerID, struct{}{})
		defer l.promisifyWorkerIDs.Delete(workerID)
		defer func() {
			l.promisifyCount.Add(-1)
			// Wake the loop so it re-checks Alive() after the count changes.
			// Only needed when auto-exit is enabled: the loop may be blocked
			// in PollIO/fast-path select and needs to re-evaluate liveness.
			// When auto-exit is disabled, this is pure overhead (syscall).
			if l.autoExit {
				if l.testHooks != nil && l.testHooks.BeforePromisifyWorkerWake != nil {
					l.testHooks.BeforePromisifyWorkerWake()
				}
				// Close owns livenessMu from its terminal-state publication through
				// release of lifecycle admission. Serialize the final wake with that
				// publication so a worker cannot observe a live loop, lose the race to
				// Close, and then write through a released poller wake descriptor.
				l.livenessMu.Lock()
				if l.state.Load() != StateTerminated {
					l.doWakeup()
				}
				l.livenessMu.Unlock()
			}
		}()

		rejectReason := func(reason error) {
			if err := l.SubmitInternal(func() {
				l.rejectPromisify(p, reason)
			}); err != nil {
				l.rejectPromisify(p, reason)
			}
		}

		// invokeCallback canonicalizes panic(nil), while this outer completion
		// guard remains armed only when runtime.Goexit prevents it from returning.
		completed := false
		defer func() {
			if !completed {
				rejectReason(ErrGoexit)
			}
		}()

		panicValue, panicked := invokeCallback(func() {
			// Respect context cancellation
			select {
			case <-ctx.Done():
				rejectReason(ctx.Err())
				return
			default:
			}

			if l.testHooks != nil && l.testHooks.BeforePromisifyWorkerStart != nil {
				l.testHooks.BeforePromisifyWorkerStart()
			}
			l.terminalDrainMu.Lock()
			if l.immediateClose.Load() {
				l.terminalDrainMu.Unlock()
				l.rejectPromisify(p, ErrLoopTerminated)
				return
			}
			l.terminalDrainMu.Unlock()
			if l.testHooks != nil && l.testHooks.AfterPromisifyWorkerEntryClaim != nil {
				l.testHooks.AfterPromisifyWorkerEntryClaim()
			}

			res, err := fn(ctx)

			// Resolution goes through SubmitInternal to ensure single-owner
			if err != nil {
				rejectReason(err)
			} else if submitErr := l.SubmitInternal(func() {
				l.resolvePromisify(p, res)
			}); submitErr != nil {
				l.resolvePromisify(p, res)
			}
		})
		completed = true
		if panicked {
			rejectReason(PanicError{Value: panicValue})
		}
	}()

	return Promise{promise: p}
}

func (l *Loop) isPromisifyWorker() bool {
	_, ok := l.promisifyWorkerIDs.Load(goroutineid.Get())
	return ok
}

func newRejectedPromise(err error) Promise {
	p := &promise{}
	p.reject(err)
	return Promise{promise: p}
}

func (l *Loop) rejectPromisify(p *promise, err error) {
	l.terminalDrainMu.Lock()
	defer l.terminalDrainMu.Unlock()
	if l.immediateClose.Load() {
		p.reject(ErrLoopTerminated)
		return
	}
	p.reject(err)
}

func (l *Loop) resolvePromisify(p *promise, result any) {
	l.terminalDrainMu.Lock()
	defer l.terminalDrainMu.Unlock()
	if l.immediateClose.Load() {
		p.reject(ErrLoopTerminated)
		return
	}
	p.resolve(result)
}
