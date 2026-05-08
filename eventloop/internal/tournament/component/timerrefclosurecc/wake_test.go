package timerrefclosurecc

import (
	"errors"
	"testing"
	"time"
)

func TestSourceAutoExitRetainsReferenceWakeResidue(t *testing.T) {
	value := newLoop(true)
	seeded := make(chan *timer, 1)
	var id timerID
	var err error
	id, err = value.scheduleTimerObserved(time.Hour, func() {}, registrationObserver{
		registrationCommitted: func() { seeded <- value.timerMap[id] },
	})
	if err != nil || id != 1 {
		t.Fatalf("ScheduleTimer = (%d, %v)", id, err)
	}
	runResult := make(chan bool, 1)
	go func() { runResult <- value.run() }()
	var timerValue *timer
	select {
	case timerValue = <-seeded:
		if timerValue == nil {
			t.Fatal("Run did not seed the source timer")
		}
	case <-value.loopDone:
		t.Fatal("Run exited before seeding the source timer")
	}
	unrefResult := make(chan error, 1)
	go func() { unrefResult <- value.unrefTimer(id) }()
	if err := receiveError(t, unrefResult); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "reference auto-exit Run") {
		t.Fatal("Run did not complete after the last reference was removed")
	}
	if value.submissionEpoch.Load() != 4 || len(value.fastWakeupCh) != 1 ||
		value.wakeUpSignalPending.Load() != 0 || value.wakeAttempts.Load() != 1 ||
		value.wakeSuccesses.Load() != 1 || value.wakeRejections.Load() != 0 {
		t.Fatal("auto-exit cleanup consumed the historical reference wake residue")
	}
	assertSourceCleanup(t, value, timerValue)
}

func TestSourceLateIngressWakeSurvivesCloseCleanup(t *testing.T) {
	value := newLoop(false)
	seeded := make(chan *timer, 1)
	var id timerID
	var err error
	id, err = value.scheduleTimerObserved(time.Hour, func() {}, registrationObserver{
		registrationCommitted: func() { seeded <- value.timerMap[id] },
	})
	if err != nil || id != 1 {
		t.Fatalf("ScheduleTimer = (%d, %v)", id, err)
	}
	secondWait := make(chan struct{})
	resumeRun := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-resumeRun:
		default:
			close(resumeRun)
		}
	})
	waitCount := 0
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{runWait: func() {
			waitCount++
			if waitCount == 2 {
				close(secondWait)
				<-resumeRun
			}
		}})
	}()
	var timerValue *timer
	select {
	case timerValue = <-seeded:
		if timerValue == nil {
			t.Fatal("Run did not seed the source timer")
		}
	case <-value.loopDone:
		t.Fatal("Run exited before seeding the source timer")
	}
	waitSignal(t, secondWait, "post-seed Run wait")

	admitted := make(chan struct{})
	resumeIngress := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-resumeIngress:
		default:
			close(resumeIngress)
		}
	})
	unrefResult := make(chan error, 1)
	go func() {
		unrefResult <- value.unrefTimerObserved(id, referenceObserver{queueAdmitted: func(uint64) {
			close(admitted)
			<-resumeIngress
		}})
	}()
	waitSignal(t, admitted, "Unref admission before wake")
	closeWoke := make(chan struct{})
	closeResult := make(chan error, 1)
	go func() {
		closeResult <- value.closeLoopObserved(lifecycleObserver{closeWake: func() { close(closeWoke) }})
	}()
	waitSignal(t, closeWoke, "Close wake")
	close(resumeRun)
	if !receiveBool(t, runResult, "late-ingress Close Run") {
		t.Fatal("Run did not exit for Close")
	}
	if err := receiveError(t, closeResult); err != nil {
		t.Fatal(err)
	}
	assertErrorBlocked(t, unrefResult)
	close(resumeIngress)
	if err := receiveError(t, unrefResult); !errors.Is(err, errTerminated) {
		t.Fatalf("late-ingress Unref = %v", err)
	}
	if value.submissionEpoch.Load() != 3 || len(value.fastWakeupCh) != 1 ||
		value.wakeAttempts.Load() != 1 || value.wakeSuccesses.Load() != 0 || value.wakeRejections.Load() != 1 {
		t.Fatal("post-Close ingress did not retain the source late-wake residue")
	}
	assertSourceCleanup(t, value, timerValue)
}

func TestSourceFailedPhysicalWakeResidueSurvivesShutdownCleanup(t *testing.T) {
	value := newLoopWithWakeBackend(false, func() bool { return false })
	registered := make(chan bool, 1)
	if err := value.submitToQueue(func() {
		registered <- value.registerFD(1) == nil
	}); err != nil {
		t.Fatal(err)
	}
	sleeping := make(chan bool, 1)
	waits := 0
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{runWait: func() {
			waits++
			if waits == 3 {
				sleeping <- value.transition(stateSleeping)
			}
		}})
	}()
	if !receiveBool(t, registered, "FD registration") || !receiveBool(t, sleeping, "Sleeping publication") {
		t.Fatal("Run did not enter Sleeping with injected wake failure")
	}
	callbackStarted := make(chan struct{})
	callbackRelease := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-callbackRelease:
		default:
			close(callbackRelease)
		}
	})
	if err := value.submitToQueue(func() {
		close(callbackStarted)
		<-callbackRelease
	}); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, callbackStarted, "callback after failed physical wake")
	shutdownWoke := make(chan struct{})
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- value.shutdownObserved(lifecycleObserver{shutdownWake: func() { close(shutdownWoke) }})
	}()
	waitSignal(t, shutdownWoke, "Shutdown failed physical wake")
	if value.wakeAttempts.Load() != 2 || value.wakeSuccesses.Load() != 0 ||
		value.wakeRejections.Load() != 0 || value.wakeUpSignalPending.Load() != 1 || len(value.fastWakeupCh) != 1 {
		t.Fatal("failed physical wake residue changed before terminal cleanup")
	}
	close(callbackRelease)
	if !receiveBool(t, runResult, "failed-wake Shutdown Run") {
		t.Fatal("Run did not exit after the blocking callback")
	}
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if state(value.state.Load()) != stateTerminated || value.ownerID.Load() != 0 ||
		value.userIOFDCount.Load() != 0 || value.wakeUpSignalPending.Load() != 1 ||
		len(value.fastWakeupCh) != 1 || value.wakeAttempts.Load() != 2 ||
		value.wakeSuccesses.Load() != 0 || value.wakeRejections.Load() != 0 {
		t.Fatal("Shutdown cleanup repaired historical failed-wake residue")
	}
	assertChannelClosed(t, value.loopDone, "failed-wake loopDone")
}

func TestSourceFDOrdinaryBehavior(t *testing.T) {
	t.Run("Errors", func(t *testing.T) {
		value := newLoop(false)
		if err := value.unregisterFD(7); !errors.Is(err, errFDNotRegistered) {
			t.Fatalf("missing UnregisterFD = %v", err)
		}
		if err := value.registerFD(7); err != nil {
			t.Fatal(err)
		}
		if err := value.registerFD(7); !errors.Is(err, errFDRegistered) {
			t.Fatalf("duplicate RegisterFD = %v", err)
		}
		if err := value.unregisterFD(8); !errors.Is(err, errFDNotRegistered) {
			t.Fatalf("other missing UnregisterFD = %v", err)
		}
		if err := value.unregisterFD(7); err != nil {
			t.Fatal(err)
		}
		if err := value.unregisterFD(7); !errors.Is(err, errFDNotRegistered) {
			t.Fatalf("repeated UnregisterFD = %v", err)
		}
		if len(value.fds) != 0 ||
			value.userIOFDCount.Load() != 0 ||
			value.submissionEpoch.Load() != 2 ||
			len(value.fastWakeupCh) != 1 ||
			value.wakeAttempts.Load() != 1 ||
			value.wakeSuccesses.Load() != 1 ||
			value.wakeRejections.Load() != 0 {
			t.Fatal("ordinary FD error handling changed liveness state")
		}
		if err := value.closeLoop(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("SleepingWake", func(t *testing.T) {
		physicalWake := make(chan struct{})
		value := newLoopWithWakeBackend(false, func() bool {
			close(physicalWake)
			return true
		})
		sleeping := make(chan bool, 1)
		resumeRun := make(chan struct{})
		resumeRunClose := closeSignalOnce(resumeRun)
		t.Cleanup(resumeRunClose)
		waits := 0
		runResult := make(chan bool, 1)
		go func() {
			runResult <- value.runObserved(lifecycleObserver{
				runWait: func() {
					waits++
					if waits == 1 {
						sleeping <- value.transition(stateSleeping)
						<-resumeRun
					}
				},
			})
		}()
		if !receiveBool(t, sleeping, "Sleeping FD Run") {
			t.Fatal("owner did not enter Sleeping")
		}
		if err := value.registerFD(9); err != nil {
			t.Fatal(err)
		}
		waitSignal(t, physicalWake, "Sleeping FD physical wake")
		if value.userIOFDCount.Load() != 1 ||
			value.submissionEpoch.Load() != 1 ||
			value.wakeUpSignalPending.Load() != 1 ||
			len(value.fastWakeupCh) != 1 ||
			value.wakeAttempts.Load() != 1 ||
			value.wakeSuccesses.Load() != 1 ||
			value.wakeRejections.Load() != 0 {
			t.Fatal("Sleeping RegisterFD wake differs")
		}

		closeWoke := make(chan struct{})
		closeResult := make(chan error, 1)
		go func() {
			closeResult <- value.closeLoopObserved(lifecycleObserver{
				closeWake: func() { close(closeWoke) },
			})
		}()
		waitSignal(t, closeWoke, "Sleeping FD Close wake")
		resumeRunClose()
		if !receiveBool(t, runResult, "Sleeping FD Run") {
			t.Fatal("Run did not exit for Close")
		}
		if err := receiveError(t, closeResult); err != nil {
			t.Fatal(err)
		}
		if len(value.fds) != 0 ||
			value.userIOFDCount.Load() != 0 ||
			value.activeTerminal != nil ||
			len(value.fastWakeupCh) != 0 ||
			value.wakeUpSignalPending.Load() != 1 ||
			value.wakeAttempts.Load() != 2 ||
			value.wakeSuccesses.Load() != 1 ||
			value.wakeRejections.Load() != 1 {
			t.Fatal("Sleeping FD terminal residue differs")
		}
		assertChannelClosed(t, value.loopDone, "Sleeping FD loopDone")
	})
}

func TestSourceFDRegistrationRollsBackAfterQuiescence(t *testing.T) {
	value := newLoop(true)
	inserted := make(chan struct{})
	resumeRegistration := make(chan struct{})
	resumeRegistrationClose := closeSignalOnce(resumeRegistration)
	t.Cleanup(resumeRegistrationClose)
	registrationResult := make(chan error, 1)
	go func() {
		registrationResult <- value.registerFDObserved(11, fdObserver{
			registrationInserted: func() {
				close(inserted)
				<-resumeRegistration
			},
		})
	}()
	waitSignal(t, inserted, "FD insertion")
	if _, exists := value.fds[11]; !exists ||
		value.userIOFDCount.Load() != 0 ||
		value.submissionEpoch.Load() != 0 {
		t.Fatal("paused FD insertion crossed its count boundary")
	}

	quiescing := make(chan struct{})
	resumeAutoExit := make(chan struct{})
	resumeAutoExitClose := closeSignalOnce(resumeAutoExit)
	t.Cleanup(resumeAutoExitClose)
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{
			autoExitQuiescing: func() {
				close(quiescing)
				<-resumeAutoExit
			},
		})
	}()
	waitSignal(t, quiescing, "FD rollback auto-exit quiescing")
	if !value.quiescing.Load() ||
		state(value.state.Load()) != stateRunning ||
		value.ownerID.Load() == 0 {
		t.Fatal("auto-exit did not enter quiescence over the uncounted FD")
	}

	resumeRegistrationClose()
	if err := receiveError(t, registrationResult); !errors.Is(err, errTerminated) {
		t.Fatalf("quiescing RegisterFD = %v", err)
	}
	if len(value.fds) != 0 ||
		value.userIOFDCount.Load() != 0 ||
		value.submissionEpoch.Load() != 0 ||
		len(value.fastWakeupCh) != 0 ||
		value.wakeAttempts.Load() != 0 {
		t.Fatal("quiescing RegisterFD did not roll back atomically")
	}

	resumeAutoExitClose()
	if !receiveBool(t, runResult, "FD rollback auto-exit Run") {
		t.Fatal("Run did not complete after FD rollback")
	}
	if state(value.state.Load()) != stateTerminated ||
		value.ownerID.Load() != 0 ||
		value.activeRun != nil ||
		value.activeTerminal != nil ||
		value.quiescing.Load() ||
		len(value.fds) != 0 ||
		value.userIOFDCount.Load() != 0 {
		t.Fatal("FD rollback auto-exit cleanup differs")
	}
	assertChannelClosed(t, value.loopDone, "FD rollback loopDone")
}
