package gojaeventloop

import (
	"context"
	"errors"
	"fmt"
	"runtime"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

// trackedBridge registers one pending [Adapter.TrackPromise] settlement so
// terminateCleanup can dispose it on the drain owner if the worker's own
// settlement could not be admitted (terminal drain rejects foreign Submit).
// Exactly one side disposes: removal from pendingBridges is the admission
// ticket, so worker and sweep cannot both act. The claimed flag provides a
// synchronous ticket so duplicate Settle calls fail fast with
// ErrLoopTerminated without enqueuing a second owner callback, while the
// entry remains sweep-visible if Submit is rejected.
type trackedBridge struct {
	settle  func()
	claimed bool
}

// TrackedSettlement settles the promise returned by [Adapter.TrackPromise].
//
// This method is safe to call from the tracked worker goroutine. The result
// and reason callbacks run on the runtime owner; a panic inside such a
// callback rejects the promise with that panic. Settlement is exactly once:
// after a terminal-disposition sweep has claimed the bridge, this returns
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
// TrackedSettlement. It is the knowledge-store for all Go→JS promise bridges.
//
// Lifecycle guarantees (intent explicit):
//   - Liveness: in-flight work contributes to Loop.Alive, so auto-exit cannot commit while run executes.
//   - Graceful Shutdown (Loop.Shutdown) waits for run via promisifyWg; if run finishes before the terminal gate closes its settlement is admitted and the promise settles normally.
//   - If run finishes after the loop stopped admitting submissions, the adapter's terminal cleanup disposes the promise on the drain owner with an ErrLoopTerminated GoError — awaiting JS observes rejection, never an eternally pending promise, for graceful paths.
//   - Immediate Close is hard-kill and does NOT wait for claimed workers (Loop.Close claim boundary). If Submit is rejected (terminal), the bridge remains in the map and the sweep disposes it on the drain owner; no reinsert or off-owner settlement is needed. Hard Close may still strand if the worker never calls Settle — stranded is defined behavior for hard Close; callers requiring observation must use Shutdown or Promise.race with AbortSignal.
//   - Context fast-path: if ctx is already canceled before admission, TrackPromise returns an immediately rejected promise without allocating a bridge, goroutine, or detached context — intent is immediate rejection, not wasted liveness.
//
// Context propagation: ctx (nil selects context.Background) is checked before admission for fast-path, otherwise a detached view is passed to Loop.Promisify to avoid stranded on pre-canceled ctx while still delegating the original ctx to run for cancellation observation.
//
// Ownership: TrackPromise must be called by the runtime owner while the adapter is bound — same contract as [Adapter.NewPromise]; violations panic.
// A nil run panics per ADR-003 (static contract violation). Runtime failures of fn surface as rejections; they are not errors, because the caller's contract is the promise itself.
//
// Contract on run: it must dispose the settlement exactly once — via Settle, or by returning without settling only when ctx cancellation makes abandonment intentional (the promise then disposes at terminal cleanup). A panic inside run is recovered and rejected promptly with a GoError carrying the recovered value and the worker's stack; it never strands pending. A run that unwinds via runtime.Goexit without settling intentionally follows the same abandonment path (distinguishable neither from a plain return nor from a test helper like testing.T.FailNow), so Goexit-only abandonment still disposes only at terminal cleanup — use [Adapter.Promisify] when the callback must never be allowed to strand.
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
	if err := ctx.Err(); err != nil {
		nativePromise, _, rawReject := a.runtime.NewPromise()
		if err := rawReject(a.runtime.NewGoError(err)); err != nil {
			a.reportPromiseJobError(wrapRuntimeError("reject tracked promise", err))
		}
		return a.runtime.ToValue(nativePromise)
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
		// Terminal disposal on the drain owner: pass a real GoError so the
		// JS-visible rejection carries message/name semantics — a raw Go error
		// would convert to an opaque host object.
		if err := rawReject(a.runtime.NewGoError(goeventloop.ErrLoopTerminated)); err != nil {
			a.reportPromiseJobError(wrapRuntimeError("reject tracked promise", err))
		}
	}}
	a.addTrackedBridge(entry)

	future := a.loop.Promisify(detached, func(_ context.Context) (any, error) {
		defer func() {
			// Runtime failures of run surface as prompt rejections: capture the
			// value (and the worker's stack, while still on this goroutine) and
			// settle through the shared claim path. If the terminal sweep
			// already claimed the bridge, submission is a no-op — exactly once
			// is preserved. recovery-driven, never stranding pending.
			if recovered := recover(); recovered != nil {
				stack := captureWorkerStack()
				if err := a.submitTrackedSettlement(entry, true, func(rt *goja.Runtime) any {
					return rt.NewGoError(fmt.Errorf("goja-eventloop: TrackPromise callback panicked: %v\n%s", recovered, stack))
				}, rawResolve, rawReject); err != nil {
					// Submit rejected (terminal) — bridge remains in pendingBridges for sweep to dispose.
				}
			}
		}()
		run(ctx, trackedSettlement{
			adapter: a,
			settle: func(rejected bool, produce func(*goja.Runtime) any) error {
				return a.submitTrackedSettlement(entry, rejected, produce, rawResolve, rawReject)
			},
		})
		return nil, nil
	})
	if future.State() == goeventloop.Rejected {
		// Admission refused: the loop is terminal and the sweep will not be
		// wired for a future lifecycle — dispose here, on the owner. Use the
		// future's actual refusal reason, wrapping it as a GoError so it is a
		// real JS Error rather than an opaque host object.
		if a.removeTrackedBridge(entry) {
			reason := goeventloop.ErrLoopTerminated
			if r, ok := future.Result().(error); ok && r != nil {
				reason = r
			}
			if err := rawReject(a.runtime.NewGoError(reason)); err != nil {
				a.reportPromiseJobError(wrapRuntimeError("reject tracked promise", err))
			}
		}
	}
	return promise
}

// Promisify runs fn off the event loop via [goeventloop.Loop.Promisify] and
// returns a native Goja promise: fn's non-nil error rejects the promise with
// a GoError (JS sees error.message), otherwise the promise fulfills with fn's
// result (nil maps to undefined).
//
// Cancellation: unlike raw TrackPromise — where run owns all cancellation
// observation — this sugar rejects promptly with ctx.Err() when the context
// is already done by the time the worker starts, mirroring Go carrier
// conventions (net/http, database/sql). Disposition still flows exclusively
// through the tracked bridge, so no timing can strand the promise. If fn
// panics, the promise rejects promptly with a GoError carrying the recovered
// value and the worker's stack; if fn unwinds via runtime.Goexit, it
// rejects promptly with a GoError reporting the abandoned exit. An untyped-nil
// result fulfills undefined.
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
		if err := ctx.Err(); err != nil {
			_ = settle.Settle(true, func(rt *goja.Runtime) any {
				return rt.NewGoError(err)
			})
			return
		}
		completed := false
		defer func() {
			if completed {
				return
			}
			recovered := recover()
			// runtime.Goexit unwinds the goroutine: recover() returns nil (a
			// real panic(nil) is canonicalized to *runtime.PanicNilError on
			// Go 1.21+, so a nil recover with !completed means Goexit).
			if recovered != nil {
				stack := captureWorkerStack()
				_ = settle.Settle(true, func(rt *goja.Runtime) any {
					return rt.NewGoError(fmt.Errorf("goja-eventloop: promisify callback panicked: %v\n%s", recovered, stack))
				})
				return
			}
			_ = settle.Settle(true, func(rt *goja.Runtime) any {
				return rt.NewGoError(errors.New("goja-eventloop: promisify callback exited via runtime.Goexit"))
			})
		}()
		result, err := fn(ctx)
		completed = true
		_ = settle.Settle(err != nil, func(rt *goja.Runtime) any {
			if err != nil {
				return rt.NewGoError(err)
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

// claimTrackedBridge attempts to claim sole settlement ownership synchronously.
// It returns true if the caller now owns disposition; false if the bridge was
// already claimed or already removed by the terminal sweep. The entry remains
// in pendingBridges so a Submit rejection still leaves it sweep-visible.
func (a *Adapter) claimTrackedBridge(entry *trackedBridge) bool {
	a.bridgesMu.Lock()
	defer a.bridgesMu.Unlock()
	if _, ok := a.pendingBridges[entry]; !ok {
		return false
	}
	if entry.claimed {
		return false
	}
	entry.claimed = true
	return true
}

// submitTrackedSettlement owns the claim-and-submit boundary for every
// TrackPromise disposition. A synchronous claim (claimTrackedBridge) ensures
// duplicate Settle calls fail fast with ErrLoopTerminated. The value-conversion
// callback then runs on the runtime owner under the final removal ticket
// (removeTrackedBridge), so the worker side and the terminal sweep contend
// exactly once. When rejecting==false, an untyped-nil product fulfills
// undefined per the TrackedSettlement contract — a raw nil would otherwise
// convert to JS null.
func (a *Adapter) submitTrackedSettlement(entry *trackedBridge, rejected bool, produce func(*goja.Runtime) any, rawResolve, rawReject func(any) error) error {
	if !a.claimTrackedBridge(entry) {
		return goeventloop.ErrLoopTerminated
	}
	err := a.Submit(func(_ *goja.Runtime) {
		if !a.removeTrackedBridge(entry) {
			return
		}
		if !rejected {
			base := produce
			produce = func(rt *goja.Runtime) any {
				value := base(rt)
				if value == nil {
					return goja.Undefined()
				}
				return value
			}
		}
		a.runPromiseSettlement(a.runtime, produce, rejected, rawResolve, rawReject)
	})
	if err != nil {
		return err
	}
	return nil
}

// captureWorkerStack returns a formatted stack of the current goroutine, growing
// the buffer so large stacks are not truncated mid-capture.
func captureWorkerStack() string {
	buffer := make([]byte, 4096)
	for {
		n := runtime.Stack(buffer, false)
		if n < len(buffer) {
			return string(buffer[:n])
		}
		buffer = make([]byte, 2*len(buffer))
	}
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
	clear(a.pendingBridges)
	a.bridgesMu.Unlock()
	for _, entry := range entries {
		entry.settle()
	}
}
