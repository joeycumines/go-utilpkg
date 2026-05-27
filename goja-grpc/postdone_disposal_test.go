package gojagrpc

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// postDoneFence loads the planted fence for a root, or nil when absent.
func postDoneFence(env *grpcTestEnv, id supervisorChildID) *ownerRootFence {
	value, ok := env.grpcMod.owner.fences.Load(id)
	if !ok {
		return nil
	}
	return value.(*ownerRootFence)
}

// postDoneDisposalFixture plants a pending disposal run (and optionally a
// fence) under postDoneMu after the adapter is dead, mirroring the state the
// post-Done sweeps consume. The caller must shut down the loop first.
func postDoneDisposalFixture(
	t *testing.T,
	env *grpcTestEnv,
	actions []ownerDisposalAction,
	roots []supervisorChildID,
) (*ownerDisposalRun, ownerDisposalID) {
	t.Helper()
	id := ownerDisposalID(1)
	run := &ownerDisposalRun{
		done:    make(chan struct{}),
		actions: actions,
		roots:   roots,
		err:     errModuleUnavailable,
	}
	env.grpcMod.owner.postDoneMu.Lock()
	env.grpcMod.owner.disposals[id] = run
	for _, root := range roots {
		if _, ok := env.grpcMod.owner.fences.Load(root); !ok {
			env.grpcMod.owner.fences.Store(root, &ownerRootFence{done: make(chan struct{})})
		}
	}
	env.grpcMod.owner.postDoneMu.Unlock()
	return run, id
}

func awaitPostDoneDisposal(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(defaultTimeout):
		t.Fatal("post-Done disposal did not complete")
	}
}

func assertNotClosed(t *testing.T, name string, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
		t.Fatalf("%s closed while a disposer was still running", name)
	default:
	}
}

// TestPostDoneDisposalWaitsForBlockingDisposerAndAcksAfter proves the review
// finding 2 completion ordering for the post-Done path: a waiter observes
// disposal completion (the run's done channel) and the root fence/supervisor
// acknowledgement only after every disposer for that snapshot has returned.
// A blocking disposer therefore keeps both the run completion and the fence
// open until released.
func TestPostDoneDisposalWaitsForBlockingDisposerAndAcksAfter(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	if err := env.loop.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	<-env.grpcMod.adapter.Done()

	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	run, id := postDoneDisposalFixture(t, env, []ownerDisposalAction{{
		disposer: func(error) {
			calls.Add(1)
			close(entered)
			<-release
		},
		kind: ownerDisposalDisposer,
	}}, []supervisorChildID{1})

	done := make(chan struct{})
	go func() {
		env.grpcMod.dispatcher.finishOwnerDisposalPostDone(id)
		close(done)
	}()
	select {
	case <-entered:
	case <-time.After(defaultTimeout):
		t.Fatal("post-Done disposer did not enter")
	}
	// The disposer is still running: neither the run completion nor the root
	// fence acknowledgement may be observable.
	assertNotClosed(t, "run.done", run.done)
	if fence := postDoneFence(env, 1); fence != nil {
		assertNotClosed(t, "fence.done", fence.done)
	} else {
		t.Fatal("fixture fence missing")
	}
	select {
	case <-done:
		t.Fatal("finishOwnerDisposalPostDone returned while the disposer was blocked")
	default:
	}

	close(release)
	awaitPostDoneDisposal(t, done)
	awaitPostDoneDisposal(t, run.done)
	if got := calls.Load(); got != 1 {
		t.Fatalf("blocking disposer calls = %d, want 1", got)
	}
	// The fence is force-closed and deleted by the acknowledgement: absent
	// from the map is the acked state, and a still-present fence must have
	// its done channel closed.
	if fence := postDoneFence(env, 1); fence != nil {
		awaitPostDoneDisposal(t, fence.done)
	}
}

// TestPostDoneDisposalGoexitIsolatesDisposers proves that a first disposer
// calling runtime.Goexit abandons only its own disposable helper goroutine:
// the collector survives, later disposers still run, and the run completion
// still closes.
func TestPostDoneDisposalGoexitIsolatesDisposers(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	if err := env.loop.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	<-env.grpcMod.adapter.Done()

	var first, second atomic.Int32
	run, id := postDoneDisposalFixture(t, env, []ownerDisposalAction{
		{
			disposer: func(error) {
				first.Add(1)
				runtime.Goexit()
			},
			kind: ownerDisposalDisposer,
		},
		{
			disposer: func(error) {
				second.Add(1)
			},
			kind: ownerDisposalDisposer,
		},
	}, []supervisorChildID{1})

	done := make(chan struct{})
	go func() {
		env.grpcMod.dispatcher.finishOwnerDisposalPostDone(id)
		close(done)
	}()
	awaitPostDoneDisposal(t, done)
	awaitPostDoneDisposal(t, run.done)
	if got := first.Load(); got != 1 {
		t.Fatalf("Goexit disposer calls = %d, want 1", got)
	}
	if got := second.Load(); got != 1 {
		t.Fatalf("later disposer calls = %d, want 1 (Goexit must abandon at most one disposer)", got)
	}
}

// TestPostDoneDisposalPanicIsolatesDisposers proves that a panicking disposer
// does not skip subsequent disposers and does not kill the collector.
func TestPostDoneDisposalPanicIsolatesDisposers(t *testing.T) {
	env := newGrpcTestEnv(t)
	defer env.shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	if err := env.loop.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	<-env.grpcMod.adapter.Done()

	var first, second atomic.Int32
	run, id := postDoneDisposalFixture(t, env, []ownerDisposalAction{
		{
			disposer: func(error) {
				first.Add(1)
				panic("expected post-Done disposal panic")
			},
			kind: ownerDisposalDisposer,
		},
		{
			disposer: func(error) {
				second.Add(1)
			},
			kind: ownerDisposalDisposer,
		},
	}, []supervisorChildID{1})

	done := make(chan struct{})
	go func() {
		env.grpcMod.dispatcher.finishOwnerDisposalPostDone(id)
		close(done)
	}()
	awaitPostDoneDisposal(t, done)
	awaitPostDoneDisposal(t, run.done)
	if got := first.Load(); got != 1 {
		t.Fatalf("panicking disposer calls = %d, want 1", got)
	}
	if got := second.Load(); got != 1 {
		t.Fatalf("later disposer calls = %d, want 1 (a panic must abandon at most one disposer)", got)
	}
}

// TestPostDoneCloseWaitsForBlockingDisposer proves the Module.Close contract
// for the post-Done path: a blocking disposer keeps Module.Close open until
// released, because with the loop dead disposeOwnerRootsWorker runs its
// post-Done sweep inline on the executeCloseRun goroutine (submission to the
// dead loop fails), and executeCloseRun reaches cancel/complete/close(run.done)
// only after the captured disposers complete.
func TestPostDoneCloseWaitsForBlockingDisposer(t *testing.T) {
	env := newGrpcTestEnv(t)

	rootID, err := env.grpcMod.control.reserve(supervisorOperation)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.grpcMod.ensureOwnerRoot(rootID); err != nil {
		t.Fatal(err)
	}
	if err := env.grpcMod.executor.install(rootID, newImmediateRootControl()); err != nil {
		t.Fatal(err)
	}
	if err := env.grpcMod.control.activate(rootID); err != nil {
		t.Fatal(err)
	}
	env.grpcMod.activateOwnerRoot(rootID)
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	if err := env.grpcMod.addOwnerRootDisposer(rootID, func(error) {
		calls.Add(1)
		close(entered)
		<-release
	}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	if err := env.loop.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	<-env.grpcMod.adapter.Done()
	closeDone := make(chan error, 1)
	go func() { closeDone <- env.grpcMod.Close() }()
	select {
	case <-entered:
	case <-time.After(defaultTimeout):
		t.Fatal("post-Done disposer did not enter")
	}
	select {
	case err := <-closeDone:
		t.Fatalf("Module.Close returned while the post-Done disposer was blocked: %v", err)
	default:
	}
	close(release)
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(defaultTimeout):
		t.Fatal("Module.Close did not join the post-Done disposer")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("blocking disposer calls = %d, want 1", got)
	}
}

// TestPostDoneCloseSurvivesGoexitDisposer proves that a disposer calling
// runtime.Goexit during the post-Done transfer cannot strand Module.Close:
// the disposer runs in its own joined goroutine (here via disposeOwnerRootsWorker's
// post-Done sweep, which runs inline on the executeCloseRun goroutine), so
// executeCloseRun survives and completes the close (the pre-fix behavior
// blocked forever because the inline disposer Goexit killed the
// executeCloseRun goroutine before cancel/complete/close(run.done)).
func TestPostDoneCloseSurvivesGoexitDisposer(t *testing.T) {
	env := newGrpcTestEnv(t)

	rootID, err := env.grpcMod.control.reserve(supervisorOperation)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.grpcMod.ensureOwnerRoot(rootID); err != nil {
		t.Fatal(err)
	}
	if err := env.grpcMod.executor.install(rootID, newImmediateRootControl()); err != nil {
		t.Fatal(err)
	}
	if err := env.grpcMod.control.activate(rootID); err != nil {
		t.Fatal(err)
	}
	env.grpcMod.activateOwnerRoot(rootID)
	var goexitCalls, laterCalls atomic.Int32
	if err := env.grpcMod.addOwnerRootDisposer(rootID, func(error) {
		goexitCalls.Add(1)
		runtime.Goexit()
	}); err != nil {
		t.Fatal(err)
	}
	if err := env.grpcMod.addOwnerRootDisposer(rootID, func(error) {
		laterCalls.Add(1)
	}); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()
	if err := env.loop.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	<-env.grpcMod.adapter.Done()
	closeDone := make(chan error, 1)
	go func() { closeDone <- env.grpcMod.Close() }()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(defaultTimeout):
		t.Fatal("Module.Close did not continue after a post-Done disposer Goexit")
	}
	if got := goexitCalls.Load(); got != 1 {
		t.Fatalf("Goexit disposer calls = %d, want 1", got)
	}
	if got := laterCalls.Load(); got != 1 {
		t.Fatalf("later disposer calls = %d, want 1", got)
	}
}
