package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/logiface"
)

const terminalAdoptionCleanupReason = "terminal adoption cleanup"

type cleanupLoggerTimerResult struct {
	id      TimerID
	err     error
	blocked bool
}

func TestTerminalAdoptionCleanupReleasesLivenessBeforeLoggerReentry(t *testing.T) {
	const diagnostic = "eventloop: unhandled rejection after loop termination (fallback callback disabled)"

	results := make(chan cleanupLoggerTimerResult, 1)
	var writerCalls atomic.Int32
	var loop *Loop
	logger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](&testEventFactory{}),
		logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(event *testEvent) error {
			if writerCalls.Add(1) != 1 {
				return nil
			}
			if event.message != diagnostic {
				results <- cleanupLoggerTimerResult{err: errors.New("unexpected cleanup diagnostic")}
				return nil
			}

			reentered := make(chan cleanupLoggerTimerResult, 1)
			go func() {
				id, err := loop.ScheduleTimer(time.Hour, func() {})
				reentered <- cleanupLoggerTimerResult{id: id, err: err}
			}()

			watchdog := time.NewTimer(2 * time.Second)
			defer watchdog.Stop()
			select {
			case result := <-reentered:
				results <- result
			case <-watchdog.C:
				// Return so a broken cleanup can release livenessMu and the test can
				// report the deadlock without stranding the package test process.
				results <- cleanupLoggerTimerResult{blocked: true}
			}
			return nil
		})),
	)
	var err error
	loop, err = New(WithLogger(logger.Logger()))
	if err != nil {
		t.Fatal(err)
	}

	var result cleanupLoggerTimerResult
	runTerminalAdoptionCleanupRaceT(t, loop, func() {
		result = waitContractValue(t, results, "terminal cleanup logger timer reentry")
	})

	if result.blocked {
		t.Fatal("ScheduleTimer from terminal cleanup logger blocked on liveness ownership")
	}
	if result.id != 0 || !errors.Is(result.err, ErrLoopTerminated) {
		t.Fatalf("ScheduleTimer from terminal cleanup logger = (%d, %v), want (0, ErrLoopTerminated)", result.id, result.err)
	}
	if got := writerCalls.Load(); got != 1 {
		t.Fatalf("terminal cleanup logger calls = %d, want 1", got)
	}
}

func TestTerminalAdoptionCleanupNilLoggerRetiresCallbackWorker(t *testing.T) {
	loop, err := New(WithLogger(nil))
	if err != nil {
		t.Fatal(err)
	}

	runTerminalAdoptionCleanupRaceT(t, loop, func() {})

	if loop.callbackWorker != nil {
		t.Fatal("terminal adoption cleanup left the nil-logger callback worker parked")
	}
}

func runTerminalAdoptionCleanupRaceT(t *testing.T, loop *Loop, observeCleanup func()) {
	t.Helper()

	js, err := NewJS(loop, WithUnhandledRejectionFallback(UnhandledRejectionFallbackDisabled))
	if err != nil {
		t.Fatal(err)
	}
	source, _, rejectSource := js.NewChainedPromise()
	adopter, resolveAdopter, _ := js.NewChainedPromise()
	resolveAdopter(source)
	adopterResult := adopter.ToChannel()

	runStarted := make(chan struct{})
	drainGeneration := make(chan (<-chan struct{}), 1)
	releaseDrain := make(chan struct{})
	releaseDrainNow := releaseSignalT(t, releaseDrain)
	rejectionPublishing := make(chan struct{})
	releaseRejection := make(chan struct{})
	releaseRejectionNow := releaseSignalT(t, releaseRejection)
	var drainHookUsed atomic.Bool
	var rejectionHookUsed atomic.Bool
	loop.testHooks = &loopTestHooks{
		AfterRunStateRunningBeforeStart: func() {
			close(runStarted)
		},
		BeforeTerminalDrainFinish: func() {
			if drainHookUsed.CompareAndSwap(false, true) {
				drainGeneration <- loop.terminalDrainDone
				<-releaseDrain
			}
		},
		BeforeTerminalEphemeralDrainSync: func() {
			if rejectionHookUsed.CompareAndSwap(false, true) {
				close(rejectionPublishing)
				<-releaseRejection
			}
		},
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, runStarted, "Run StateRunning publication")
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(context.Background()) }()

	activeDrain := waitContractValue(t, drainGeneration, "terminal drain finish boundary")
	loop.livenessMu.Lock()
	var unlockLiveness sync.Once
	unlockLivenessNow := func() { unlockLiveness.Do(loop.livenessMu.Unlock) }
	t.Cleanup(unlockLivenessNow)
	releaseDrainNow()
	waitContractSignal(t, activeDrain, "terminal drain generation completion")
	if loop.terminalDraining.Load() {
		t.Fatal("terminal drain remained active after its completion signal")
	}

	rejectDone := make(chan struct{})
	go func() {
		rejectSource(terminalAdoptionCleanupReason)
		close(rejectDone)
	}()
	waitContractSignal(t, rejectionPublishing, "source rejection publication boundary")
	if got := source.state.Load(); got != promiseRejectedPublishing {
		t.Fatalf("source raw state = %d, want rejected publication state %d", got, promiseRejectedPublishing)
	}

	unlockLivenessNow()
	if got := waitContractValue(t, adopterResult, "terminal cleanup adoption settlement"); got != terminalAdoptionCleanupReason {
		t.Fatalf("adopter rejection reason = %#v, want %q", got, terminalAdoptionCleanupReason)
	}
	observeCleanup()
	releaseRejectionNow()
	waitContractSignal(t, rejectDone, "source rejection completion")
	if err := waitContractValue(t, runDone, "terminal adoption cleanup Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := waitContractValue(t, shutdownDone, "terminal adoption cleanup Shutdown completion"); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	if state := adopter.State(); state != Rejected {
		t.Fatalf("adopter state = %v, want Rejected", state)
	}
	if reason := adopter.Reason(); reason != terminalAdoptionCleanupReason {
		t.Fatalf("adopter reason = %#v, want %q", reason, terminalAdoptionCleanupReason)
	}
	if duplicate, open := waitContractReceive(t, adopterResult, "terminal cleanup adoption result closure"); open {
		t.Fatalf("adopter result channel published duplicate value %#v", duplicate)
	}
	waitTerminalUnhandledRejectionTrackingDrained(t, js)
}
