package gojaeventloop

import (
	"context"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// trackedBridge registers one pending [Adapter.TrackPromise] settlement so
// terminateCleanup can dispose it on the drain owner if the worker's own
// settlement could not be admitted (terminal drain rejects foreign Submit).
// Exactly one side disposes: removal from pendingBridges is the admission
// ticket, so worker and sweep cannot both act.
type trackedBridge struct {
	settle func()
}

// TrackedSettlement settles the promise returned by [Adapter.TrackPromise].
//
// Both methods are safe to call from the tracked worker goroutine. The result
// and reason callbacks run on the runtime owner; a panic inside such a
// callback rejects the promise with that panic. Settlement is exactly once:
// after a terminal-disposition sweep has claimed the bridge, these return
// [goeventloop.ErrLoopTerminated] without touching the promise.
type TrackedSettlement interface {
	// Settle disposes the promise exactly once: rejected=false fulfills with
	// the value produced by result on the runtime owner (nil maps to
	// undefined); rejected=true rejects with the produced reason. Returns
	// [goeventloop.ErrLoopTerminated] if the terminal sweep already claimed
	// disposal.
	Settle(rejected bool, produce func(rt *goja.Runtime) any) error
}

// TrackPromise runs run off the event loop via [goeventloop.Loop.Promisify]
// and returns a native Goja promise settled by run through the supplied
// TrackedSettlement.
//
// Lifecycle guarantees, all inherited from Promisify plus the terminal sweep:
//   - Liveness: the in-flight work contributes to Loop.Alive, so auto-exit
//     cannot commit while run executes.
//   - Graceful Shutdown waits for run (promisifyWg). If run finishes before
//     the terminal gate closes its settlement submission is admitted and the
//     promise settles normally.
//   - If run finishes after the loop stopped admitting submissions, the
//     adapter's terminal cleanup disposes the promise on the drain owner with
//     an ErrLoopTerminated GoError — awaiting JS observes rejection, never an
//     eternally pending promise.
//   - Immediate Close follows the same sweep semantics as Promisify itself:
//     hard-kill disposition, no waiting for run.
//
// Context propagation: ctx (nil selects context.Background) is passed to both
// Loop.Promisify and run, so cancellation is observable inside run.
//
// Ownership: TrackPromise must be called by the runtime owner while the
// adapter is bound — same contract as [Adapter.NewPromise]; violations panic.
// A nil run panics per ADR-003 (static contract violation). Runtime failures
// of fn surface as rejections; they are not errors, because the caller's
// contract is the promise itself.
//
// Contract on run: it must dispose the settlement exactly once — via Resolve,
// Reject, or by returning without settling only when ctx cancellation makes
// abandonment intentional (the promise then disposes at terminal cleanup).
func (a *Adapter) TrackPromise(ctx context.Context, run func(ctx context.Context, settle TrackedSettlement)) goja.Value {
	a.mustOriginalReceiver("TrackPromise")
	if a.state() != adapterStateBound || !a.claimed() {
		panic("goja-eventloop: TrackPromise requires a bound adapter on its runtime owner")
	}
	if run == nil {
		panic("goja-eventloop: nil TrackPromise callback")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// Admission uses a cancellation-detached view of ctx: Loop.Promisify
	// rejects at worker entry when ctx is already done, which would skip run
	// entirely and leave the native promise stranded. Cancellation remains
	// fully enforced — the worker checks ctx before and delegates it to run —
	// but *our* settlement path owns disposition instead of an internal
	// early-reject that cannot reach the Goja promise.
	detached := context.WithoutCancel(ctx)

	nativePromise, rawResolve, rawReject := a.runtime.NewPromise()
	promise := a.runtime.ToValue(nativePromise)

	entry := &trackedBridge{settle: func() {
		if err := rawReject(goeventloop.ErrLoopTerminated); err != nil {
			a.reportPromiseJobError(wrapRuntimeError("reject tracked promise", err))
		}
	}}
	a.addTrackedBridge(entry)

	future := a.loop.Promisify(detached, func(_ context.Context) (any, error) {
		run(ctx, trackedSettlement{
			adapter: a,
			settle: func(rejected bool, produce func(*goja.Runtime) any) error {
				if !a.removeTrackedBridge(entry) {
					return goeventloop.ErrLoopTerminated
				}
				// Settlement conversion must run on the runtime owner. If the
				// loop has stopped admitting submissions (graceful drain
				// started between our claim and this submit), hand the entry
				// back so terminateCleanup — which runs strictly after
				// waitPromisifyGoroutines — disposes the promise on the
				// drain owner.
				err := a.Submit(func(_ *goja.Runtime) {
					a.runPromiseSettlement(a.runtime, produce, rejected, rawResolve, rawReject)
				})
				if err != nil {
					a.addTrackedBridge(entry)
					return err
				}
				return nil
			},
		})
		return nil, nil
	})
	if future.State() == goeventloop.Rejected {
		// Admission refused: the loop is terminal and the sweep will not be
		// wired for a future lifecycle — dispose here, on the owner.
		if a.removeTrackedBridge(entry) {
			_ = rawReject(goeventloop.ErrLoopTerminated)
		}
	}
	return promise
}

// Promisify runs fn off the event loop via [goeventloop.Loop.Promisify] and
// returns a native Goja promise: fn's non-nil error rejects the promise with
// a GoError (JS sees error.message), otherwise the promise fulfills with fn's
// result (nil maps to undefined).
//
// This is the sugar over [Adapter.TrackPromise] for the common shape; use
// TrackPromise directly when the binding needs custom result conversion or
// split resolution paths — all such conversion callbacks execute on the
// runtime owner. See TrackPromise for the full lifecycle contract.
func (a *Adapter) Promisify(ctx context.Context, fn func(ctx context.Context) (any, error)) goja.Value {
	if fn == nil {
		panic("goja-eventloop: nil Promisify callback")
	}
	return a.TrackPromise(ctx, func(ctx context.Context, settle TrackedSettlement) {
		result, err := fn(ctx)
		_ = settle.Settle(err != nil, func(rt *goja.Runtime) any {
			if err != nil {
				return rt.NewGoError(err)
			}
			if result == nil {
				return goja.Undefined()
			}
			return result
		})
	})
}

// trackedSettlement is the TrackedSettlement handed to TrackPromise callers.
// Its claim funnels through removeTrackedBridge so that exactly one of
// {worker settle, terminal sweep} disposes each promise. Settlement reuses
// runPromiseSettlement for owner-side conversion, panic canonicalization,
// and error reporting — identical semantics to Adapter.NewPromise settlers.
type trackedSettlement struct {
	adapter *Adapter
	settle  func(rejected bool, produce func(*goja.Runtime) any) error
}

func (t trackedSettlement) Settle(rejected bool, produce func(rt *goja.Runtime) any) error {
	return t.settle(rejected, produce)
}

func (a *Adapter) addTrackedBridge(entry *trackedBridge) {
	a.bridgesMu.Lock()
	if a.pendingBridges == nil {
		a.pendingBridges = make(map[*trackedBridge]struct{})
	}
	a.pendingBridges[entry] = struct{}{}
	a.bridgesMu.Unlock()
}

// removeTrackedBridge reports whether the caller claimed sole disposition.
func (a *Adapter) removeTrackedBridge(entry *trackedBridge) bool {
	a.bridgesMu.Lock()
	_, ok := a.pendingBridges[entry]
	delete(a.pendingBridges, entry)
	a.bridgesMu.Unlock()
	return ok
}

// sweepTrackedBridges disposes every still-pending tracked promise. It runs
// on the drain owner inside terminateCleanup, where runtime access is legal.
func (a *Adapter) sweepTrackedBridges() {
	a.bridgesMu.Lock()
	entries := make([]*trackedBridge, 0, len(a.pendingBridges))
	for entry := range a.pendingBridges {
		entries = append(entries, entry)
	}
	a.pendingBridges = make(map[*trackedBridge]struct{})
	a.bridgesMu.Unlock()
	for _, entry := range entries {
		entry.settle()
	}
}
