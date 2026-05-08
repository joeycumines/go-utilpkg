package timerrefclosure0bc

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestSourceAutoExitIngressInvalidatesDecision(t *testing.T) {
	for _, phase := range []string{"decision", "snapshot"} {
		t.Run(phase, func(t *testing.T) {
			value := newLoop(true)
			phaseReached := make(chan struct{})
			resumePhase, releasePhase := newSourceRelease(t)
			var once sync.Once
			pause := func() {
				once.Do(func() {
					close(phaseReached)
					<-resumePhase
				})
			}
			observer := lifecycleObserver{}
			if phase == "decision" {
				observer.autoExitDecision = pause
			} else {
				observer.autoExitSnapshotValidated = pause
			}
			runResult := make(chan bool, 1)
			go func() { runResult <- value.runObserved(observer) }()
			waitSignal(t, phaseReached, "auto-exit "+phase)
			if state(value.state.Load()) != stateRunning || !value.quiescing.Load() ||
				value.terminalDraining.Load() || value.terminalDrainDone != nil {
				t.Fatal("auto-exit phase did not expose valid quiescence")
			}

			epoch := value.submissionEpoch.Load()
			if finish, ok := value.beginPromisifyWorker(); ok || finish != nil ||
				value.submissionEpoch.Load() != epoch {
				t.Fatal("quiescence admitted Promisify liveness")
			}
			wakePublished := make(chan struct{})
			unrefResult := make(chan error, 1)
			go func() {
				unrefResult <- value.unrefTimerObserved(1, referenceObserver{
					wakePublished: func() { close(wakePublished) },
				})
			}()
			waitSignal(t, wakePublished, "auto-exit invalidating Unref wake")
			if value.submissionEpoch.Load() != epoch+1 || len(value.queue) != 1 ||
				!value.quiescing.Load() || len(value.fastWakeupCh) != 1 {
				t.Fatal("Unref did not occupy the auto-exit invalidation window")
			}
			assertErrorBlocked(t, unrefResult)

			releasePhase()
			if err := receiveError(t, unrefResult); err != nil {
				t.Fatal(err)
			}
			if !receiveBool(t, runResult, "invalidated auto-exit Run") {
				t.Fatal("Run did not retry auto-exit after admitted Unref")
			}
			assertSourceCleanup(t, value, nil)
		})
	}
}

func TestSourceWorkerLivenessPreventsAutoExit(t *testing.T) {
	value := newLoop(true)
	finishWorker, ok := value.beginPromisifyWorker()
	if !ok {
		t.Fatal("Awake worker was not admitted")
	}
	t.Cleanup(finishWorker)
	waiting := make(chan struct{})
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{
			runWait: func() { close(waiting) },
		})
	}()
	waitSignal(t, waiting, "Run wait after worker liveness check")
	assertBoolBlocked(t, runResult)
	if value.promisifyCount.Load() != 1 {
		t.Fatal("worker liveness was not published")
	}
	finishWorker()
	if !receiveBool(t, runResult, "0bc worker-liveness Run") {
		t.Fatal("Run did not auto-exit after worker completion")
	}
	assertSourceCleanup(t, value, nil)
}

func TestSourceAutoExitCompletesGenerationBeforeRunDone(t *testing.T) {
	value := newLoop(true)
	published := make(chan (<-chan struct{}), 1)
	beforeReturn := make(chan struct{})
	resumeReturn, releaseReturn := newSourceRelease(t)
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{
			autoExitPublished: func(done <-chan struct{}) { published <- done },
			beforeRunReturn: func() {
				close(beforeReturn)
				<-resumeReturn
			},
		})
	}()
	var generationDone <-chan struct{}
	select {
	case generationDone = <-published:
	case <-time.After(time.Second):
		t.Fatal("auto-exit generation was not published")
	}
	waitSignal(t, beforeReturn, "auto-exit pre-return boundary")
	assertChannelClosed(t, generationDone, "auto-exit generation before Run return")
	assertChannelClosed(t, value.terminalDone, "terminalDone before auto-exit Run return")
	assertChannelOpen(t, value.loopDone, "loopDone before auto-exit Run return")
	if state(value.state.Load()) != stateTerminated || value.ownerID.Load() == 0 || value.terminalDraining.Load() || value.terminalDrainDone != nil || value.quiescing.Load() {
		t.Fatal("auto-exit did not finish generation and cleanup before Run return")
	}
	releaseReturn()
	if !receiveBool(t, runResult, "0bc auto-exit Run") {
		t.Fatal("auto-exit Run did not complete")
	}
	assertSourceCleanup(t, value, nil)
}

func TestSourceAutoExitTransitionTerminalLosers(t *testing.T) {
	value := newLoop(true)
	published := make(chan (<-chan struct{}), 1)
	transitioned := make(chan struct{})
	resumeTransition, releaseTransition := newSourceRelease(t)
	beforeReturn := make(chan struct{})
	resumeReturn, releaseReturn := newSourceRelease(t)
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{
			autoExitPublished: func(done <-chan struct{}) { published <- done },
			autoExitTransitioned: func() {
				close(transitioned)
				<-resumeTransition
			},
			beforeRunReturn: func() {
				close(beforeReturn)
				<-resumeReturn
			},
		})
	}()
	var generationDone <-chan struct{}
	select {
	case generationDone = <-published:
	case <-time.After(time.Second):
		t.Fatal("auto-exit generation was not published")
	}
	waitSignal(t, transitioned, "auto-exit terminal transition")
	assertChannelOpen(t, generationDone, "generation before auto-exit drain")
	assertChannelOpen(t, value.terminalDone, "terminalDone before auto-exit drain")
	assertChannelOpen(t, value.loopDone, "loopDone before auto-exit drain")
	shutdownResult := make(chan error, 1)
	go func() { shutdownResult <- value.shutdown() }()
	if err := receiveError(t, shutdownResult); !errors.Is(err, errTerminated) {
		t.Fatalf("post-auto-exit-transition Shutdown = %v", err)
	}
	stages := make(chan closeWaitStage, 2)
	closeResult := make(chan error, 1)
	go func() {
		closeResult <- value.closeLoopObserved(lifecycleObserver{
			closeWait: func(stage closeWaitStage) { stages <- stage },
		})
	}()
	select {
	case stage := <-stages:
		if stage != closeWaitLosingTerminal {
			t.Fatalf("Close first wait stage = %d", stage)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not join terminalDone")
	}
	assertErrorBlocked(t, closeResult)
	releaseTransition()
	waitSignal(t, beforeReturn, "auto-exit pre-return boundary")
	select {
	case stage := <-stages:
		if stage != closeWaitLosingLoop {
			t.Fatalf("Close second wait stage = %d", stage)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not join loopDone")
	}
	assertChannelOpen(t, value.loopDone, "loopDone at Close second join")
	releaseReturn()
	if !receiveBool(t, runResult, "0bc auto-exit terminal-loser Run") {
		t.Fatal("Run did not complete")
	}
	if err := receiveError(t, closeResult); !errors.Is(err, errTerminated) {
		t.Fatalf("post-auto-exit-transition Close = %v", err)
	}
	assertChannelClosed(t, generationDone, "auto-exit generation")
	assertSourceCleanup(t, value, nil)
}

func TestSourceAutoExitDecisionShutdownLeavesHistoricalGeneration(t *testing.T) {
	value := newLoop(true)
	decision := make(chan struct{})
	resumeDecision, releaseDecision := newSourceRelease(t)
	autoExitPublished := make(chan (<-chan struct{}), 1)
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{
			autoExitDecision: func() {
				close(decision)
				<-resumeDecision
			},
			autoExitPublished: func(done <-chan struct{}) { autoExitPublished <- done },
		})
	}()
	waitSignal(t, decision, "auto-exit decision")
	shutdownPublished := make(chan (<-chan struct{}), 1)
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- value.shutdownObserved(lifecycleObserver{
			shutdownPublished: func(done <-chan struct{}) { shutdownPublished <- done },
		})
	}()
	var shutdownDone <-chan struct{}
	select {
	case shutdownDone = <-shutdownPublished:
	case <-time.After(time.Second):
		t.Fatal("Shutdown generation was not published")
	}
	shutdownOwner := value.terminalOwnerID.Load()
	if shutdownOwner == 0 {
		t.Fatal("Shutdown generation did not publish an owner")
	}
	if state(value.state.Load()) != stateTerminating || !value.terminalDraining.Load() {
		t.Fatal("Shutdown did not win the stale auto-exit decision window")
	}
	releaseDecision()
	var staleDone <-chan struct{}
	select {
	case staleDone = <-autoExitPublished:
	case <-time.After(time.Second):
		t.Fatal("stale auto-exit did not publish its no-op continuation")
	}
	if staleDone != shutdownDone {
		t.Fatal("stale auto-exit did not observe the active Shutdown generation")
	}
	if !receiveBool(t, runResult, "0bc stale-auto-exit Shutdown Run") {
		t.Fatal("Run did not complete")
	}
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	assertChannelOpen(t, shutdownDone, "historically leaked Shutdown generation")
	if state(value.state.Load()) != stateTerminated || value.ownerID.Load() != 0 || !value.terminalDraining.Load() ||
		value.terminalDrainDone == nil || (<-chan struct{})(value.terminalDrainDone) != shutdownDone || value.terminalOwnerID.Load() != shutdownOwner || value.quiescing.Load() ||
		value.queue != nil || value.spare != nil || value.externalQueue != nil || value.externalSpare != nil || len(value.fastWakeupCh) != 1 || value.wakePending.Load() != 0 {
		t.Fatal("historical stale auto-exit/Shutdown leak differs")
	}
	assertChannelClosed(t, value.terminalDone, "terminalDone after stale auto-exit")
	assertChannelClosed(t, value.loopDone, "loopDone after stale auto-exit")
}

func TestSourceAutoExitDecisionCloseCompletesStaleContinuation(t *testing.T) {
	value := newLoop(true)
	decision := make(chan struct{})
	resumeDecision, releaseDecision := newSourceRelease(t)
	autoExitPublished := make(chan (<-chan struct{}), 1)
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{
			autoExitDecision: func() {
				close(decision)
				<-resumeDecision
			},
			autoExitPublished: func(done <-chan struct{}) { autoExitPublished <- done },
		})
	}()
	waitSignal(t, decision, "auto-exit decision")
	closePublished := make(chan struct{})
	closeResult := make(chan error, 1)
	go func() {
		closeResult <- value.closeLoopObserved(lifecycleObserver{
			closeWait: func(stage closeWaitStage) {
				if stage == closeWaitWinningLoop {
					close(closePublished)
				}
			},
		})
	}()
	waitSignal(t, closePublished, "Close terminal publication")
	releaseDecision()
	var generationDone <-chan struct{}
	select {
	case generationDone = <-autoExitPublished:
	case <-time.After(time.Second):
		t.Fatal("stale auto-exit generation was not published")
	}
	if !receiveBool(t, runResult, "0bc stale-auto-exit Close Run") {
		t.Fatal("Run did not complete")
	}
	if err := receiveError(t, closeResult); err != nil {
		t.Fatal(err)
	}
	assertChannelClosed(t, generationDone, "stale auto-exit generation after Close")
	assertSourceCleanupWake(t, value, nil, 1)
}

func TestSourceAutoExitCloseCompletesCapturedGeneration(t *testing.T) {
	value := newLoop(true)
	published := make(chan (<-chan struct{}), 1)
	resumeAutoExit, releaseAutoExit := newSourceRelease(t)
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{
			autoExitPublished: func(done <-chan struct{}) {
				published <- done
				<-resumeAutoExit
			},
		})
	}()
	var generationDone <-chan struct{}
	select {
	case generationDone = <-published:
	case <-time.After(time.Second):
		t.Fatal("auto-exit generation was not published")
	}
	waitStage := make(chan closeWaitStage, 1)
	closeResult := make(chan error, 1)
	go func() {
		closeResult <- value.closeLoopObserved(lifecycleObserver{
			closeWait: func(stage closeWaitStage) { waitStage <- stage },
		})
	}()
	select {
	case stage := <-waitStage:
		if stage != closeWaitWinningLoop {
			t.Fatalf("Close wait stage = %d", stage)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not win and reach its loop wait")
	}
	assertChannelOpen(t, generationDone, "captured auto-exit generation before continuation")
	if !value.quiescing.Load() {
		t.Fatal("Close cleared quiescing before the captured auto-exit continuation")
	}
	assertErrorBlocked(t, closeResult)
	releaseAutoExit()
	if !receiveBool(t, runResult, "0bc auto-exit/Close Run") {
		t.Fatal("auto-exit continuation did not retain Run authority")
	}
	if err := receiveError(t, closeResult); err != nil {
		t.Fatalf("winning Close = %v", err)
	}
	assertChannelClosed(t, generationDone, "captured auto-exit generation")
	assertSourceCleanupWake(t, value, nil, 1)
}

func TestSourceAutoExitShutdownReusesGeneration(t *testing.T) {
	value := newLoop(true)
	published := make(chan (<-chan struct{}), 1)
	resumeAutoExit, releaseAutoExit := newSourceRelease(t)
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{
			autoExitPublished: func(done <-chan struct{}) {
				published <- done
				<-resumeAutoExit
			},
		})
	}()
	var capturedDone <-chan struct{}
	select {
	case capturedDone = <-published:
	case <-time.After(time.Second):
		t.Fatal("auto-exit generation was not published")
	}
	owner := value.terminalOwnerID.Load()
	if owner == 0 {
		t.Fatal("auto-exit did not publish its loop-thread owner")
	}
	shutdownGeneration := make(chan (<-chan struct{}), 1)
	shutdownWake := make(chan struct{})
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- value.shutdownObserved(lifecycleObserver{
			shutdownPublished: func(done <-chan struct{}) { shutdownGeneration <- done },
			shutdownWake:      func() { close(shutdownWake) },
		})
	}()
	var reusedDone <-chan struct{}
	select {
	case reusedDone = <-shutdownGeneration:
	case <-time.After(time.Second):
		t.Fatal("Shutdown did not publish its transition")
	}
	if reusedDone != capturedDone || value.terminalOwnerID.Load() != owner {
		t.Fatal("Shutdown replaced the 0bc auto-exit generation or owner")
	}
	waitSignal(t, shutdownWake, "Shutdown wake publication")
	if !value.quiescing.Load() {
		t.Fatal("Shutdown cleared quiescing before the captured auto-exit continuation")
	}
	releaseAutoExit()
	if !receiveBool(t, runResult, "0bc auto-exit/Shutdown Run") {
		t.Fatal("auto-exit continuation did not complete Run")
	}
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatalf("winning Shutdown = %v", err)
	}
	assertChannelClosed(t, capturedDone, "reused auto-exit generation")
	assertSourceCleanupWake(t, value, nil, 1)
}
