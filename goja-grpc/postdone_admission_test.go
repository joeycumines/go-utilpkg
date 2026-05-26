package gojagrpc

import (
	"context"
	"errors"
	"testing"

	"github.com/joeycumines/goja"
	"google.golang.org/grpc/codes"
	grpcmetadata "google.golang.org/grpc/metadata"
)

// TestLateAfterDoneAdmissionRejectsEveryOwnerEntryPoint reproduces the
// post-Done admission window from review finding 1: Adapter.Done is closed
// while the ownership transfer has NOT run (transferred == false) and a live
// root still exists in the owner maps. In that window every owner-obligation
// entry point must refuse admission — exact rejection (never a pending
// promise), never a map insertion — because the terminal transfer would drop
// any newly created obligation without settling it.
//
// The state is reached deterministically: the module is fully closed first
// (joining the closeAfterAdapter watcher), then a root + fence are planted
// under postDoneMu with transferred reset to false — the exact bridge state
// the window exposes to a late JS entry point.
func TestLateAfterDoneAdmissionRejectsEveryOwnerEntryPoint(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	if err := env.loop.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	<-env.grpcMod.adapter.Done()
	// Join the closeAfterAdapter watcher's transfer: after Close returns,
	// every post-Done writer has finished, so the replanted fixture below
	// cannot race clearPostDoneOwnerIndexes.
	if err := env.grpcMod.Close(); err != nil {
		t.Fatal(err)
	}

	// Replant the live-root fixture with transferred == false (the window).
	rootID := supervisorChildID(1)
	env.grpcMod.owner.postDoneMu.Lock()
	root := &ownerRootEntry{
		promises:  make(map[uint64]ownerPromiseEntry),
		callbacks: make(map[uint64]func(grpcmetadata.MD) error),
	}
	env.grpcMod.owner.roots[rootID] = root
	env.grpcMod.owner.fences.Store(rootID, &ownerRootFence{done: make(chan struct{})})
	env.grpcMod.owner.transferred.Store(false)
	// Restore the supervisor phase to open: the module was fully closed above,
	// and checkOpen must exercise its Adapter.Done branch (a closed control
	// alone would make the assertion vacuous). The supervisor serve loop has
	// exited with complete(), so no command processing races this reset.
	env.grpcMod.control.phase.Store(uint32(supervisorOpen))
	env.grpcMod.owner.postDoneMu.Unlock()
	if env.grpcMod.owner.transferred.Load() {
		t.Fatal("fixture: transfer flag could not be reset")
	}
	if env.grpcMod.owner.roots[rootID] == nil {
		t.Fatal("fixture: live root missing")
	}

	rootsBefore := len(env.grpcMod.owner.roots)
	fencesBefore := syncMapSize(&env.grpcMod.owner.fences)
	plansBefore := len(env.grpcMod.owner.serverPlans)
	dialBefore := len(env.grpcMod.dialObjects)

	// ensureOwnerRoot on a fresh id: refused, no root or fence inserted.
	lateID := supervisorChildID(2)
	if err := env.grpcMod.ensureOwnerRoot(lateID); !errors.Is(err, errModuleClosed) {
		t.Fatalf("ensureOwnerRoot after Adapter.Done = %v, want %v", err, errModuleClosed)
	}
	if _, ok := env.grpcMod.owner.roots[lateID]; ok {
		t.Fatal("ensureOwnerRoot after Adapter.Done inserted a root")
	}
	if _, ok := env.grpcMod.owner.fences.Load(lateID); ok {
		t.Fatal("ensureOwnerRoot after Adapter.Done inserted a fence")
	}

	// newOwnerPromise: unadmitted handle whose native promise is rejected
	// synchronously with Unavailable — never pending, no entry inserted.
	var projectionCalled bool
	handle := env.grpcMod.newOwnerPromise(rootID, func(ownerResult) any {
		projectionCalled = true
		return nil
	}, nil)
	if handle.admitted() {
		t.Fatal("newOwnerPromise after Adapter.Done admitted an obligation")
	}
	if handle.value == nil || goja.IsUndefined(handle.value) || goja.IsNull(handle.value) {
		t.Fatal("newOwnerPromise after Adapter.Done returned no promise value")
	}
	latePromise, ok := handle.value.Export().(*goja.Promise)
	if !ok {
		t.Fatalf("newOwnerPromise value exports as %T, want *goja.Promise", handle.value.Export())
	}
	if state := latePromise.State(); state != goja.PromiseStateRejected {
		t.Fatalf("late promise state = %v, want %v (must be rejected synchronously, never pending)", state, goja.PromiseStateRejected)
	}
	rejection, ok := latePromise.Result().(*goja.Object)
	if !ok {
		t.Fatalf("late promise rejection = %T, want *goja.Object", latePromise.Result())
	}
	if code := codes.Code(rejection.Get("code").ToInteger()); code != codes.Unavailable {
		t.Fatalf("late promise rejection code = %v, want %v", code, codes.Unavailable)
	}
	if projectionCalled {
		t.Fatal("late promise projection ran despite an unadmitted handle")
	}
	if got := len(root.promises); got != 0 {
		t.Fatalf("newOwnerPromise after Adapter.Done inserted %d promise entries", got)
	}

	// addOwnerRootDisposer: refused, no disposer appended or run.
	var disposerRan bool
	if err := env.grpcMod.addOwnerRootDisposer(rootID, func(error) {
		disposerRan = true
	}); !errors.Is(err, errModuleClosed) {
		t.Fatalf("addOwnerRootDisposer after Adapter.Done = %v, want %v", err, errModuleClosed)
	}
	if disposerRan {
		t.Fatal("late disposer ran despite refused admission")
	}
	if got := len(root.disposers); got != 0 {
		t.Fatalf("addOwnerRootDisposer after Adapter.Done appended %d disposers", got)
	}

	// rememberOwnerCallback: empty id, no callback registered.
	if id := env.grpcMod.rememberOwnerCallback(rootID, goja.Callable(func(goja.Value, ...goja.Value) (goja.Value, error) {
		return goja.Undefined(), nil
	})); id != (ownerCallbackID{}) {
		t.Fatalf("rememberOwnerCallback after Adapter.Done = %v, want empty id", id)
	}
	if got := len(root.callbacks); got != 0 {
		t.Fatalf("rememberOwnerCallback after Adapter.Done registered %d callbacks", got)
	}

	// allocateServerMethodPlan: error, no plan inserted.
	if id, err := env.grpcMod.allocateServerMethodPlan(&serverMethodPlan{}); !errors.Is(err, errModuleClosed) || id != 0 {
		t.Fatalf("allocateServerMethodPlan after Adapter.Done = (%d, %v), want (0, %v)", id, err, errModuleClosed)
	}
	if got := len(env.grpcMod.owner.serverPlans); got != plansBefore {
		t.Fatalf("allocateServerMethodPlan after Adapter.Done changed plan count: %d -> %d", plansBefore, got)
	}

	// checkOpen: the module reads as closed once Adapter.Done is closed.
	if err := env.grpcMod.checkOpen(); !errors.Is(err, errModuleClosed) {
		t.Fatalf("checkOpen after Adapter.Done = %v, want %v", err, errModuleClosed)
	}

	// Global invariants: no roots, fences, or dial entries were added.
	if got := len(env.grpcMod.owner.roots); got != rootsBefore {
		t.Fatalf("owner roots after late admissions: %d -> %d", rootsBefore, got)
	}
	if got := syncMapSize(&env.grpcMod.owner.fences); got != fencesBefore {
		t.Fatalf("owner fences after late admissions: %d -> %d", fencesBefore, got)
	}
	if got := len(env.grpcMod.dialObjects); got != dialBefore {
		t.Fatalf("dial objects after late admissions: %d -> %d", dialBefore, got)
	}
}
