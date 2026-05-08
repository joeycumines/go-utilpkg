package eventloop

import (
	"sync/atomic"
	"testing"
)

type livenessObservation struct {
	alive         bool
	macrotaskWork bool
}

type livenessBatchCase struct {
	name     string
	schedule func(*Loop, func(), func()) error
}

func detachedPhaseBatchCases() []livenessBatchCase {
	return []livenessBatchCase{
		{
			name: "check",
			schedule: func(loop *Loop, first, second func()) error {
				if err := loop.ScheduleImmediate(first); err != nil {
					return err
				}
				return loop.ScheduleImmediate(second)
			},
		},
		{
			name: "close",
			schedule: func(loop *Loop, first, second func()) error {
				if err := loop.ScheduleCloseCallback(first); err != nil {
					return err
				}
				return loop.ScheduleCloseCallback(second)
			},
		},
	}
}

var externalRemainderLivenessCase = livenessBatchCase{
	name: "external",
	schedule: func(loop *Loop, first, second func()) error {
		if err := loop.Submit(first); err != nil {
			return err
		}
		return loop.Submit(second)
	},
}

func observeLoopLiveness(loop *Loop) livenessObservation {
	return livenessObservation{
		alive:         loop.Alive(),
		macrotaskWork: loop.HasMacrotaskWork(),
	}
}

func requireLoopLiveness(t *testing.T, scope string, got livenessObservation) {
	t.Helper()
	if !got.alive {
		t.Errorf("%s Alive = false while accepted macrotask work remains", scope)
	}
	if !got.macrotaskWork {
		t.Errorf("%s HasMacrotaskWork = false while accepted macrotask work remains", scope)
	}
}

func exerciseBatchLiveness(t *testing.T, test livenessBatchCase, terminal bool) {
	t.Helper()
	loop := New()
	registerLoopCleanupT(t, loop)

	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	release := releaseSignalT(t, releaseFirst)
	ownerObservation := make(chan livenessObservation, 1)
	secondRan := make(chan struct{})
	if err := test.schedule(loop, func() {
		ownerObservation <- observeLoopLiveness(loop)
		close(firstEntered)
		<-releaseFirst
	}, func() {
		close(secondRan)
	}); err != nil {
		t.Fatal(err)
	}

	if terminal {
		shutdownDone := make(chan error, 1)
		go func() { shutdownDone <- loop.Shutdown(t.Context()) }()
		waitContractSignal(t, firstEntered, "first terminal "+test.name+" callback")
		requireLoopLiveness(t, "terminal drain owner", waitContractValue(t, ownerObservation, "terminal owner liveness"))
		requireLoopLiveness(t, "terminal external observer", observeLoopLiveness(loop))
		release()
		waitContractSignal(t, secondRan, "second terminal "+test.name+" callback")
		if err := waitContractValue(t, shutdownDone, "pre-Run Shutdown completion"); err != nil {
			t.Fatal(err)
		}
		if got := observeLoopLiveness(loop); got.alive || got.macrotaskWork {
			t.Fatalf("liveness after terminal batch completion = %+v, want both false", got)
		}
		return
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(t.Context()) }()
	waitContractSignal(t, firstEntered, "first "+test.name+" callback")
	requireLoopLiveness(t, "owner", waitContractValue(t, ownerObservation, "owner liveness"))
	requireLoopLiveness(t, "external observer", observeLoopLiveness(loop))
	release()
	waitContractSignal(t, secondRan, "second "+test.name+" callback")
	if err := loop.Close(); err != nil {
		t.Fatal(err)
	}
	if err := waitContractValue(t, runDone, "Run completion after Close"); err != nil {
		t.Fatal(err)
	}
}

func TestDetachedPhaseBatchLivenessRemainsVisibleUntilCompletion(t *testing.T) {
	for _, test := range detachedPhaseBatchCases() {
		t.Run(test.name, func(t *testing.T) {
			exerciseBatchLiveness(t, test, false)
		})
	}
}

func TestExternalRemainderLivenessRemainsVisibleUntilCompletion(t *testing.T) {
	exerciseBatchLiveness(t, externalRemainderLivenessCase, false)
}

func TestTerminalDetachedPhaseBatchLivenessRemainsVisibleUntilCompletion(t *testing.T) {
	for _, test := range detachedPhaseBatchCases() {
		t.Run(test.name, func(t *testing.T) {
			exerciseBatchLiveness(t, test, true)
		})
	}
}

func TestTerminalExternalRemainderLivenessRemainsVisibleUntilCompletion(t *testing.T) {
	exerciseBatchLiveness(t, externalRemainderLivenessCase, true)
}

func TestGracefulShutdownKeepsDetachedPhaseLivenessVisible(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	firstEntered := make(chan struct{})
	observeDuringShutdown := make(chan struct{})
	releaseFirst := make(chan struct{})
	releaseObservation := releaseSignalT(t, observeDuringShutdown)
	releaseCallback := releaseSignalT(t, releaseFirst)
	ownerObservation := make(chan livenessObservation, 1)
	secondRan := make(chan struct{})
	if err := loop.ScheduleImmediate(func() {
		close(firstEntered)
		<-observeDuringShutdown
		ownerObservation <- observeLoopLiveness(loop)
		<-releaseFirst
	}); err != nil {
		t.Fatal(err)
	}
	if err := loop.ScheduleImmediate(func() { close(secondRan) }); err != nil {
		t.Fatal(err)
	}

	terminating := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() { close(terminating) },
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(t.Context()) }()
	waitContractSignal(t, firstEntered, "first check callback")

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(t.Context()) }()
	waitContractSignal(t, terminating, "Shutdown StateTerminating publication")
	releaseObservation()
	requireLoopLiveness(t, "graceful drain owner", waitContractValue(t, ownerObservation, "graceful-drain owner liveness"))
	requireLoopLiveness(t, "graceful external observer", observeLoopLiveness(loop))

	releaseCallback()
	waitContractSignal(t, secondRan, "second detached check callback during graceful Shutdown")
	if err := waitContractValue(t, shutdownDone, "Shutdown completion"); err != nil {
		t.Fatal(err)
	}
	if err := waitContractValue(t, runDone, "Run completion after Shutdown"); err != nil {
		t.Fatal(err)
	}
	if got := observeLoopLiveness(loop); got.alive || got.macrotaskWork {
		t.Fatalf("liveness after graceful completion = %+v, want both false", got)
	}
}

func TestImmediateCloseInvalidatesDetachedPhaseLiveness(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	release := releaseSignalT(t, releaseFirst)
	secondRan := make(chan struct{}, 1)
	if err := loop.ScheduleImmediate(func() {
		close(firstEntered)
		<-releaseFirst
	}); err != nil {
		t.Fatal(err)
	}
	if err := loop.ScheduleImmediate(func() { secondRan <- struct{}{} }); err != nil {
		t.Fatal(err)
	}

	terminating := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterCloseStateTerminating: func() { close(terminating) },
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(t.Context()) }()
	waitContractSignal(t, firstEntered, "first check callback")

	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	waitContractSignal(t, terminating, "Close StateTerminating publication")
	if got := observeLoopLiveness(loop); got.alive || got.macrotaskWork {
		t.Fatalf("liveness after immediate Close won = %+v, want both false", got)
	}

	release()
	if err := waitContractValue(t, closeDone, "Close completion"); err != nil {
		t.Fatal(err)
	}
	if err := waitContractValue(t, runDone, "Run completion after Close"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-secondRan:
		t.Fatal("immediate Close admitted a later detached check callback")
	default:
	}
}

func TestImmediatePredicateFinalFalseLinearizesAutoExit(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)

	var (
		refed          atomic.Bool
		predicateCalls atomic.Int32
		callbackRan    atomic.Bool
		scheduled      atomic.Bool
		committing     atomic.Bool
	)
	loop.testHooks = &loopTestHooks{
		AfterAutoExitFinalAliveCheck: func() {
			if !scheduled.CompareAndSwap(false, true) {
				return
			}
			if err := loop.ScheduleImmediateRef(func() { callbackRan.Store(true) }, func() bool {
				predicateCalls.Add(1)
				return refed.Load()
			}); err != nil {
				t.Errorf("ScheduleImmediateRef from final Alive hook: %v", err)
			}
		},
		BeforeAutoExitTerminalDrainCommit: func() {
			committing.Store(true)
			// The last owner evaluation already returned false. Changing only
			// captured predicate state cannot revive work across that cut.
			refed.Store(true)
		},
	}

	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatal(err)
	}
	if !scheduled.Load() {
		t.Fatal("final Alive hook did not schedule the dynamic immediate")
	}
	if !committing.Load() {
		t.Fatal("auto-exit did not reach terminal-drain commit")
	}
	if got := predicateCalls.Load(); got < 2 {
		t.Fatalf("dynamic predicate calls = %d, want at least two owner evaluations", got)
	}
	if callbackRan.Load() {
		t.Fatal("dynamic immediate ran after its final owner evaluation returned false")
	}
}
