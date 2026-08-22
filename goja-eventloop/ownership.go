package gojaeventloop

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"weak"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
)

var (
	// ErrAdapterInvalid reports use of an adapter that was not created by New.
	ErrAdapterInvalid = errors.New("goja-eventloop: invalid adapter")
	// ErrAdapterBound reports a repeated Bind call after successful installation.
	ErrAdapterBound = errors.New("goja-eventloop: adapter already bound")
	// ErrAdapterBinding reports a concurrent Bind call while installation is active.
	ErrAdapterBinding = errors.New("goja-eventloop: adapter bind in progress")
	// ErrAdapterUnbound reports an operation that requires a completed Bind.
	ErrAdapterUnbound = errors.New("goja-eventloop: adapter is not bound")
	// ErrAdapterFailed reports use of an adapter whose construction or Bind failed.
	ErrAdapterFailed = errors.New("goja-eventloop: adapter failed")
	// ErrOwnershipConflict reports an already claimed runtime or event loop.
	ErrOwnershipConflict = errors.New("goja-eventloop: ownership conflict")
	// ErrLoopState reports construction or installation outside StateAwake.
	ErrLoopState = errors.New("goja-eventloop: loop must be awake")
	// ErrPromiseSettled reports a second attempt to settle an adapter Promise.
	ErrPromiseSettled = errors.New("goja-eventloop: promise already settled")
)

type adapterState uint32

const (
	adapterStateInvalid adapterState = iota
	adapterStateReady
	adapterStateBinding
	adapterStateBound
	adapterStateFailed
)

type adapterOwnership struct {
	original  weak.Pointer[Adapter]
	claim     adapterClaimToken
	jsOptions []goeventloop.JSOption
	bindMu    sync.Mutex
	state     atomic.Uint32
}

type adapterClaimToken struct {
	loop    weak.Pointer[goeventloop.Loop]
	runtime weak.Pointer[goja.Runtime]
	owner   weak.Pointer[Adapter]
}

type adapterClaim struct {
	owner weak.Pointer[Adapter]
}

var adapterClaimRegistry = struct {
	sync.Mutex
	loops    map[weak.Pointer[goeventloop.Loop]]adapterClaim
	runtimes map[weak.Pointer[goja.Runtime]]adapterClaim
}{
	loops:    make(map[weak.Pointer[goeventloop.Loop]]adapterClaim),
	runtimes: make(map[weak.Pointer[goja.Runtime]]adapterClaim),
}

func claimAdapter(adapter *Adapter, jsOptions []goeventloop.JSOption) error {
	if adapter == nil || adapter.loop == nil || adapter.runtime == nil {
		return ErrAdapterInvalid
	}
	if adapter.loop.State() != goeventloop.StateAwake {
		return fmt.Errorf("%w: %s", ErrLoopState, adapter.loop.State())
	}

	loopKey := weak.Make(adapter.loop)
	runtimeKey := weak.Make(adapter.runtime)
	owner := weak.Make(adapter)

	adapterClaimRegistry.Lock()
	sweepAdapterClaimsLocked()
	if claim, ok := adapterClaimRegistry.loops[loopKey]; ok && claim.owner.Value() != nil {
		adapterClaimRegistry.Unlock()
		return fmt.Errorf("%w: event loop already claimed", ErrOwnershipConflict)
	}
	if claim, ok := adapterClaimRegistry.runtimes[runtimeKey]; ok && claim.owner.Value() != nil {
		adapterClaimRegistry.Unlock()
		return fmt.Errorf("%w: runtime already claimed", ErrOwnershipConflict)
	}
	token := adapterClaimToken{loop: loopKey, runtime: runtimeKey, owner: owner}
	claim := adapterClaim{owner: owner}
	adapterClaimRegistry.loops[loopKey] = claim
	adapterClaimRegistry.runtimes[runtimeKey] = claim
	adapterClaimRegistry.Unlock()

	ownership := &adapterOwnership{
		claim:     token,
		original:  owner,
		jsOptions: append([]goeventloop.JSOption(nil), jsOptions...),
	}
	ownership.state.Store(uint32(adapterStateReady))
	adapter.ownership = ownership
	runtime.AddCleanup(adapter, cleanupAdapterClaim, token)
	runtime.KeepAlive(adapter)
	return nil
}

func cleanupAdapterClaim(token adapterClaimToken) {
	adapterClaimRegistry.Lock()
	deleteAdapterClaimLocked(adapterClaimRegistry.loops, token.loop, token.owner)
	deleteAdapterClaimLocked(adapterClaimRegistry.runtimes, token.runtime, token.owner)
	adapterClaimRegistry.Unlock()
}

func deleteAdapterClaimLocked[T any](claims map[weak.Pointer[T]]adapterClaim, key weak.Pointer[T], owner weak.Pointer[Adapter]) {
	if claim, ok := claims[key]; ok && claim.owner == owner {
		delete(claims, key)
	}
}

func sweepAdapterClaimsLocked() {
	for key, claim := range adapterClaimRegistry.loops {
		if key.Value() == nil || claim.owner.Value() == nil {
			delete(adapterClaimRegistry.loops, key)
		}
	}
	for key, claim := range adapterClaimRegistry.runtimes {
		if key.Value() == nil || claim.owner.Value() == nil {
			delete(adapterClaimRegistry.runtimes, key)
		}
	}
}

func (a *Adapter) releaseClaim() {
	if !a.originalReceiver() {
		return
	}
	cleanupAdapterClaim(a.ownership.claim)
	runtime.KeepAlive(a)
}

func (a *Adapter) claimed() bool {
	if !a.originalReceiver() || a.loop == nil || a.runtime == nil {
		return false
	}
	token := a.ownership.claim
	adapterClaimRegistry.Lock()
	loopClaim, loopOK := adapterClaimRegistry.loops[token.loop]
	runtimeClaim, runtimeOK := adapterClaimRegistry.runtimes[token.runtime]
	claimed := loopOK && runtimeOK && loopClaim.owner == token.owner && runtimeClaim.owner == token.owner &&
		loopClaim.owner.Value() == a && runtimeClaim.owner.Value() == a
	adapterClaimRegistry.Unlock()
	runtime.KeepAlive(a)
	return claimed
}

func (a *Adapter) state() adapterState {
	if a == nil || a.ownership == nil {
		return adapterStateInvalid
	}
	return adapterState(a.ownership.state.Load())
}

func (a *Adapter) originalReceiver() bool {
	if a == nil || a.ownership == nil {
		return false
	}
	original := a.ownership.original.Value()
	valid := original != nil && original == a
	runtime.KeepAlive(a)
	return valid
}

func (a *Adapter) mustOriginalReceiver(operation string) {
	if !a.originalReceiver() {
		panic("goja-eventloop: " + operation + " requires the adapter returned by New")
	}
}

func (a *Adapter) fail() {
	if !a.originalReceiver() {
		return
	}
	clear(a.ownership.jsOptions)
	a.ownership.jsOptions = nil
	a.ownership.state.Store(uint32(adapterStateFailed))
	a.releaseClaim()
}

func (a *Adapter) stateError() error {
	switch a.state() {
	case adapterStateReady:
		return nil
	case adapterStateBinding:
		return ErrAdapterBinding
	case adapterStateBound:
		return ErrAdapterBound
	case adapterStateFailed:
		return ErrAdapterFailed
	default:
		return ErrAdapterInvalid
	}
}

// OwnsRuntime reports whether the receiver is the original Adapter returned by
// New, Bind completed successfully, and candidate is its exact claimed
// runtime. It returns false for a nil receiver or candidate and for zero,
// copied, pre-Bind, failed, released, or foreign adapters and runtimes. Loop
// termination alone does not release a successful claim.
//
// OwnsRuntime is an identity predicate, not evidence that the loop accepts work
// or permission to access Goja directly. Use Submit for owner-serialized Goja
// work. OwnsRuntime does not access Goja state and may be called from any
// goroutine.
func (a *Adapter) OwnsRuntime(candidate *goja.Runtime) bool {
	owned := candidate != nil && a != nil && a.runtime == candidate && a.state() == adapterStateBound && a.claimed()
	runtime.KeepAlive(a)
	return owned
}

// OwnsLoop reports whether the receiver is the original Adapter returned by
// New, Bind completed successfully, and candidate is its exact claimed loop.
// It returns false for a nil receiver or candidate and for zero, copied,
// pre-Bind, failed, released, or foreign adapters and loops. Loop termination
// alone does not release a successful claim.
//
// OwnsLoop is an identity predicate, not a liveness predicate; scheduling
// operations report terminal errors independently. OwnsLoop does not access
// Goja state and may be called from any goroutine.
func (a *Adapter) OwnsLoop(candidate *goeventloop.Loop) bool {
	owned := candidate != nil && a != nil && a.loop == candidate && a.state() == adapterStateBound && a.claimed()
	runtime.KeepAlive(a)
	return owned
}

// Done returns the exact event loop terminal-completion signal. The signal
// closes only after terminal cleanup, when no callback accepted by the
// adapter's loop can still execute.
//
// Done does not access Goja state and may be called from any goroutine. It
// panics when the receiver is nil, zero, or a copied Adapter.
func (a *Adapter) Done() <-chan struct{} {
	a.mustOriginalReceiver("Done")
	done := a.loop.Done()
	runtime.KeepAlive(a)
	return done
}

// Submit schedules fn under the adapter's serialized logical callback-owner
// role. The event loop may temporarily transfer that role to an isolated
// callback worker; physical goroutine identity is not part of the contract. The
// runtime passed to fn is the exact runtime claimed by the adapter. Callers must
// use Submit for external Goja work after callbacks may execute and must never
// access the runtime or its values concurrently. Submit panics if fn is nil or
// the receiver is nil, zero, or a copied Adapter. Mutable lifecycle failures are
// returned as errors. A JavaScript exception raised by a direct Goja operation
// in fn is reported through the adapter's uncaught-exception boundary; native
// Go panics retain the event loop callback-panic contract.
func (a *Adapter) Submit(fn func(*goja.Runtime)) error {
	if fn == nil {
		panic("goja-eventloop: submit callback must not be nil")
	}
	a.mustOriginalReceiver("Submit")
	if a.state() == adapterStateFailed {
		return ErrAdapterFailed
	}
	if a.state() == adapterStateReady {
		return ErrAdapterUnbound
	}
	if a.state() == adapterStateBinding {
		return ErrAdapterBinding
	}
	if a.state() != adapterStateBound || !a.claimed() {
		return ErrAdapterInvalid
	}
	err := a.loop.Submit(func() {
		if exception := a.runtime.Try(func() { fn(a.runtime) }); exception != nil {
			a.handleHostCallbackError("Adapter.Submit", exception, "uncaughtException")
		}
	})
	runtime.KeepAlive(a)
	return err
}
