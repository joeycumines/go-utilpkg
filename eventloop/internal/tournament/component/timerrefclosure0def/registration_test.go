package timerrefclosure0def

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type registrationResult struct {
	id  timerID
	err error
}

func TestSourcePreRunReferenceTimesOutWithoutAdmission(t *testing.T) {
	for _, refed := range []bool{false, true} {
		name := "Unref"
		if refed {
			name = "Ref"
		}
		t.Run(name, func(t *testing.T) {
			value := newLoop(false)
			id, err := value.prepareTimerRegistration()
			if err != nil || id != 1 {
				t.Fatalf("pre-Run registration = (%d, %v)", id, err)
			}
			waitEntered := make(chan struct{})
			timeout := make(chan time.Time)
			var timeoutOnce sync.Once
			releaseTimeout := func() { timeoutOnce.Do(func() { close(timeout) }) }
			t.Cleanup(releaseTimeout)
			result := make(chan error, 1)
			go func() {
				observer := referenceObserver{
					runWaitEntered: func() { close(waitEntered) },
					runWaitTimeout: timeout,
				}
				if refed {
					result <- value.refTimerObserved(id, observer)
					return
				}
				result <- value.unrefTimerObserved(id, observer)
			}()
			waitSignal(t, waitEntered, name+" pre-Run wait")
			assertErrorBlocked(t, result)
			if state(value.state.Load()) != stateAwake || value.ownerID.Load() != 0 ||
				len(value.timerMap) != 0 || len(value.queue) != 1 || value.submissionEpoch.Load() != 1 ||
				len(value.fastWakeupCh) != 1 || value.wakeAttempts.Load() != 0 ||
				value.wakeSuccesses.Load() != 0 || value.wakeRejections.Load() != 0 {
				t.Fatal("pre-Run reference mutated registration admission state")
			}
			releaseTimeout()
			if err := receiveError(t, result); !errors.Is(err, errNotRunning) {
				t.Fatalf("pre-Run %s = %v", name, err)
			}
			if err := value.closeLoop(); err != nil {
				t.Fatal(err)
			}
			assertSourceCleanupWake(t, value, nil, 1)
		})
	}
}

func TestSourcePreRunReferenceTerminalOvertake(t *testing.T) {
	for _, refed := range []bool{false, true} {
		for _, immediate := range []bool{false, true} {
			operation := "Unref"
			if refed {
				operation = "Ref"
			}
			terminal := "Shutdown"
			if immediate {
				terminal = "Close"
			}
			t.Run(operation+"/"+terminal, func(t *testing.T) {
				value := newLoop(false)
				id, err := value.prepareTimerRegistration()
				if err != nil || id != 1 {
					t.Fatalf("pre-Run registration = (%d, %v)", id, err)
				}
				waitEntered := make(chan struct{})
				result := make(chan error, 1)
				go func() {
					observer := referenceObserver{
						runWaitEntered: func() {
							close(waitEntered)
							<-value.loopDone
						},
					}
					if refed {
						result <- value.refTimerObserved(id, observer)
						return
					}
					result <- value.unrefTimerObserved(id, observer)
				}()
				waitSignal(t, waitEntered, operation+" pre-Run wait")
				assertErrorBlocked(t, result)
				if immediate {
					err = value.closeLoop()
				} else {
					err = value.shutdown()
				}
				if err != nil {
					t.Fatal(err)
				}
				if err := receiveError(t, result); !errors.Is(err, errTerminated) {
					t.Fatalf("pre-Run %s after %s = %v", operation, terminal, err)
				}
				wantEpoch := uint64(2)
				if immediate {
					wantEpoch = 1
				}
				if value.nextTimerID.Load() != 1 || value.submissionEpoch.Load() != wantEpoch {
					t.Fatalf("%s epoch/ID = (%d, %d), want (1, %d)", terminal,
						value.nextTimerID.Load(), value.submissionEpoch.Load(), wantEpoch)
				}
				if value.wakeAttempts.Load() != 0 || value.wakeSuccesses.Load() != 0 ||
					value.wakeRejections.Load() != 0 {
					t.Fatal("Awake terminal overtake published a physical wake")
				}
				assertSourceCleanupWake(t, value, nil, 1)
			})
		}
	}
}

func TestRetiredTimerCannotBeReseeded(t *testing.T) {
	loop := newLoop(false)
	if !loop.bindOwner() || !loop.seed(1, true) || !loop.remove(1) {
		t.Fatal("retirement setup failed")
	}
	if loop.seed(1, false) {
		t.Fatal("retired monotonic ID was reused")
	}
	before := loop.snapshot(1).submissionEpoch
	if err := loop.refTimer(1); err != nil {
		t.Fatal(err)
	}
	if got := loop.snapshot(1); got.present || got.submissionEpoch != before || got.refedCount != 0 {
		t.Fatalf("retired running refTimer snapshot = %+v", got)
	}
}

func TestSourceAwakeShutdownDrainsRegistrationFIFO(t *testing.T) {
	loop := newLoop(false)
	id, err := loop.prepareTimerRegistration()
	if err != nil || id != 1 {
		t.Fatalf("prepareTimerRegistration = %d, %v", id, err)
	}
	observed := make(chan qualificationSnapshot, 1)
	if err := loop.submitToQueue(func() { observed <- loop.snapshot(id) }); err != nil {
		t.Fatal(err)
	}
	if err := loop.shutdown(); err != nil {
		t.Fatal(err)
	}
	var got qualificationSnapshot
	select {
	case got = <-observed:
	case <-time.After(time.Second):
		t.Fatal("Awake graceful sentinel did not execute")
	}
	if !got.present || !got.refed || got.refedCount != 1 || got.submissionEpoch != 3 || got.state != stateTerminated {
		t.Fatalf("Awake graceful drain snapshot = %+v", got)
	}
	if loop.wakeAttempts.Load() != 0 {
		t.Fatal("Awake graceful Shutdown published a started wake")
	}
	assertSourceCleanupWake(t, loop, nil, 1)
}

func TestRegistrationExhaustionConsumesRejectedIDs(t *testing.T) {
	loop := newLoop(false)
	loop.nextTimerID.Store(uint64(maxTimerID))
	beforeEpoch := loop.submissionEpoch.Load()
	for attempt := range 2 {
		id, err := loop.prepareTimerRegistration()
		if id != 0 || !errors.Is(err, errIDExhausted) {
			t.Fatalf("attempt %d = (%d, %v)", attempt, id, err)
		}
	}
	if got := loop.nextTimerID.Load(); got != uint64(maxTimerID)+2 {
		t.Fatalf("exhausted counter = %d", got)
	}
	if len(loop.queue) != 0 || len(loop.timerMap) != 0 || loop.submissionEpoch.Load() != beforeEpoch || len(loop.fastWakeupCh) != 0 {
		t.Fatal("exhausted registration mutated qualification state")
	}

	wrapped := newLoop(false)
	wrapped.nextTimerID.Store(^uint64(0))
	if id, err := wrapped.prepareTimerRegistration(); id != 0 || err != nil || wrapped.nextTimerID.Load() != 0 {
		t.Fatalf("wrapped allocation = (%d, %v, %d)", id, err, wrapped.nextTimerID.Load())
	}
	expired := make(chan time.Time)
	close(expired)
	observer := referenceObserver{runWaitTimeout: expired}
	if err := wrapped.refTimerObserved(0, observer); !errors.Is(err, errNotRunning) {
		t.Fatalf("Awake wrapped RefTimer = %v", err)
	}
	if err := wrapped.unrefTimerObserved(0, observer); !errors.Is(err, errNotRunning) {
		t.Fatalf("Awake wrapped UnrefTimer = %v", err)
	}
	if !wrapped.bindOwner() || wrapped.drain() != 1 {
		t.Fatal("wrapped ID zero registration did not drain")
	}
	if err := wrapped.unrefTimer(0); err != nil {
		t.Fatalf("running wrapped UnrefTimer = %v", err)
	}
	if got := wrapped.snapshot(0); !got.present || got.refed || got.refedCount != 0 {
		t.Fatalf("wrapped timer snapshot = %+v", got)
	}
}

func TestRegistrationExhaustionConcurrentBoundary(t *testing.T) {
	loop := newLoop(false)
	loop.nextTimerID.Store(uint64(maxTimerID) - 1)
	const workers = 32
	results := make(chan registrationResult, workers)
	var start sync.WaitGroup
	start.Add(1)
	for range workers {
		go func() {
			start.Wait()
			id, err := loop.prepareTimerRegistration()
			results <- registrationResult{id: id, err: err}
		}()
	}
	start.Done()
	successes := 0
	exhausted := 0
	for range workers {
		result := receiveRegistration(t, results)
		switch {
		case result.err == nil && result.id == maxTimerID:
			successes++
		case result.id == 0 && errors.Is(result.err, errIDExhausted):
			exhausted++
		default:
			t.Fatalf("boundary result = (%d, %v)", result.id, result.err)
		}
	}
	wantNext := uint64(maxTimerID) - 1 + workers
	if successes != 1 || exhausted != workers-1 || loop.nextTimerID.Load() != wantNext || len(loop.queue) != 1 {
		t.Fatalf("boundary totals success=%d exhausted=%d next=%d queued=%d", successes, exhausted, loop.nextTimerID.Load(), len(loop.queue))
	}
	if id, err := loop.prepareTimerRegistration(); id != 0 || !errors.Is(err, errIDExhausted) || loop.nextTimerID.Load() != wantNext+1 {
		t.Fatalf("post-boundary exhaustion = (%d, %v, %d)", id, err, loop.nextTimerID.Load())
	}
}

func TestRegistrationClaimSurvivesAdmissionLoss(t *testing.T) {
	loop := newLoop(false)
	claimed := make(chan timerID, 1)
	resume, release := newSourceRelease(t)
	result := make(chan registrationResult, 1)
	go func() {
		id, err := loop.prepareTimerRegistrationObserved(registrationObserver{
			claimed: func(id timerID) {
				claimed <- id
				<-resume
			},
		})
		result <- registrationResult{id: id, err: err}
	}()
	select {
	case id := <-claimed:
		if id != 1 {
			t.Fatalf("claimed timer ID = %d", id)
		}
	case <-time.After(time.Second):
		t.Fatal("registration did not reach its claimed-ID phase")
	}
	if err := loop.closeLoop(); err != nil {
		t.Fatal(err)
	}
	release()
	registration := receiveRegistration(t, result)
	if registration.id != 0 || !errors.Is(registration.err, errTerminated) {
		t.Fatalf("post-claim terminal admission = (%d, %v)", registration.id, registration.err)
	}
	if loop.nextTimerID.Load() != 1 || len(loop.queue) != 0 || len(loop.timerMap) != 0 {
		t.Fatal("admission loss rolled back or materialized the claimed ID")
	}
	assertSourceCleanup(t, loop, nil)
}

func TestSourceRegistrationFirstGateTerminalOvertake(t *testing.T) {
	for _, immediate := range []bool{false, true} {
		name := "Shutdown"
		if immediate {
			name = "Close"
		}
		t.Run(name, func(t *testing.T) {
			loop := newLoop(false)
			firstGatePassed := make(chan struct{})
			resume, release := newSourceRelease(t)
			result := make(chan registrationResult, 1)
			go func() {
				id, err := loop.prepareTimerRegistrationObserved(registrationObserver{
					firstGatePassed: func() {
						close(firstGatePassed)
						<-resume
					},
				})
				result <- registrationResult{id: id, err: err}
			}()
			waitSignal(t, firstGatePassed, name+" registration first gate")
			var err error
			if immediate {
				err = loop.closeLoop()
			} else {
				err = loop.shutdown()
			}
			if err != nil {
				t.Fatal(err)
			}
			release()
			registration := receiveRegistration(t, result)
			if registration.id != 0 || !errors.Is(registration.err, errTerminated) {
				t.Fatalf("%s post-first-gate registration = (%d, %v)", name, registration.id, registration.err)
			}
			if loop.nextTimerID.Load() != 1 || len(loop.timerMap) != 0 || len(loop.queue) != 0 ||
				loop.submissionEpoch.Load() != 0 || len(loop.fastWakeupCh) != 0 {
				t.Fatal("terminal overtake did not preserve consumed-ID-only state")
			}
			assertSourceCleanup(t, loop, nil)
		})
	}
}

func TestSourceRegistrationErrorPrecedenceAfterFirstGate(t *testing.T) {
	loop := newLoop(false)
	loop.nextTimerID.Store(uint64(maxTimerID))
	firstGatePassed := make(chan struct{})
	resume, release := newSourceRelease(t)
	result := make(chan registrationResult, 1)
	go func() {
		id, err := loop.prepareTimerRegistrationObserved(registrationObserver{
			firstGatePassed: func() {
				close(firstGatePassed)
				<-resume
			},
		})
		result <- registrationResult{id: id, err: err}
	}()
	waitSignal(t, firstGatePassed, "exhaustion registration first gate")
	if err := loop.closeLoop(); err != nil {
		t.Fatal(err)
	}
	release()
	registration := receiveRegistration(t, result)
	if registration.id != 0 || !errors.Is(registration.err, errIDExhausted) {
		t.Fatalf("post-first-gate exhaustion = (%d, %v)", registration.id, registration.err)
	}
	if loop.nextTimerID.Load() != uint64(maxTimerID)+1 || loop.submissionEpoch.Load() != 0 || len(loop.fastWakeupCh) != 0 {
		t.Fatal("range error did not precede the second terminal gate")
	}
	assertSourceCleanup(t, loop, nil)
}

func TestSourceOwnerRegistrationIsSynchronous(t *testing.T) {
	loop := newLoop(false)
	t.Cleanup(func() { _ = loop.closeLoop() })
	initialWait := make(chan struct{})
	postCallback := make(chan struct{})
	resumeTerminal, releaseTerminal := newSourceRelease(t)
	waitCount := 0
	runResult := make(chan bool, 1)
	go func() {
		runResult <- loop.runObserved(lifecycleObserver{
			runWait: func() {
				waitCount++
				if waitCount == 1 {
					close(initialWait)
				} else if waitCount == 2 {
					close(postCallback)
					<-resumeTerminal
				}
			},
		})
	}()
	waitSignal(t, initialWait, "owner registration initial wait")
	type ownerRegistrationObservation struct {
		id       timerID
		err      error
		timer    *timer
		snapshot qualificationSnapshot
	}
	observed := make(chan ownerRegistrationObservation, 1)
	if err := loop.submitToQueue(func() {
		id, err := loop.prepareTimerRegistration()
		observed <- ownerRegistrationObservation{
			id: id, err: err, timer: loop.timerMap[id], snapshot: loop.snapshot(id),
		}
	}); err != nil {
		t.Fatal(err)
	}
	var observation ownerRegistrationObservation
	select {
	case observation = <-observed:
	case <-time.After(time.Second):
		t.Fatal("owner registration callback did not execute")
	}
	if observation.err != nil || observation.id != 1 || observation.timer == nil {
		t.Fatalf("owner registration = (%d, %v, %p)", observation.id, observation.err, observation.timer)
	}
	want := qualificationSnapshot{
		present: true, refed: true, refedCount: 1, submissionEpoch: 2, state: stateRunning,
	}
	if observation.snapshot != want || loop.nextTimerID.Load() != 1 || len(loop.timerMap) != 1 {
		t.Fatalf("owner synchronous registration snapshot = %+v, want %+v", observation.snapshot, want)
	}
	waitSignal(t, postCallback, "owner registration post-callback wait")
	shutdownPublished := make(chan struct{})
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- loop.shutdownObserved(lifecycleObserver{
			shutdownPublished: func(<-chan struct{}) { close(shutdownPublished) },
		})
	}()
	waitSignal(t, shutdownPublished, "owner registration Shutdown")
	releaseTerminal()
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "owner registration Run") {
		t.Fatal("Run did not complete")
	}
	assertSourceCleanupWake(t, loop, observation.timer, 0)
}

func TestSourceRunningRegistrationAdmission(t *testing.T) {
	loop := newLoop(false)
	t.Cleanup(func() { _ = loop.closeLoop() })
	runWaiting := make(chan struct{})
	postDrain := make(chan struct{})
	resumeRun, releaseRun := newSourceRelease(t)
	resumeTerminal, releaseTerminal := newSourceRelease(t)
	waitCount := 0
	runResult := make(chan bool, 1)
	go func() {
		runResult <- loop.runObserved(lifecycleObserver{
			runWait: func() {
				waitCount++
				switch waitCount {
				case 1:
					close(runWaiting)
					<-resumeRun
				case 2:
					close(postDrain)
					<-resumeTerminal
				}
			},
		})
	}()
	waitSignal(t, runWaiting, "Running registration wait")
	id, err := loop.prepareTimerRegistration()
	if err != nil || id != 1 {
		t.Fatalf("Running registration = (%d, %v)", id, err)
	}
	observed := make(chan qualificationSnapshot, 1)
	if err := loop.submitToQueue(func() { observed <- loop.snapshot(id) }); err != nil {
		t.Fatal(err)
	}
	if loop.nextTimerID.Load() != 1 || len(loop.timerMap) != 0 || len(loop.queue) != 2 ||
		loop.submissionEpoch.Load() != 2 || len(loop.fastWakeupCh) != 1 {
		t.Fatal("Running registration did not remain queued before owner execution")
	}
	releaseRun()
	var snapshot qualificationSnapshot
	select {
	case snapshot = <-observed:
	case <-time.After(time.Second):
		t.Fatal("Running registration sentinel did not execute")
	}
	if !snapshot.present || !snapshot.refed || snapshot.refedCount != 1 || snapshot.submissionEpoch != 3 ||
		snapshot.queued != 0 || snapshot.fastWakePending != 0 || snapshot.wakeAttempts != 0 || snapshot.state != stateRunning {
		t.Fatalf("Running registration snapshot = %+v", snapshot)
	}
	waitSignal(t, postDrain, "Running registration post-drain wait")
	timerValue := loop.timerMap[id]
	shutdownPublished := make(chan struct{})
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- loop.shutdownObserved(lifecycleObserver{
			shutdownPublished: func(<-chan struct{}) { close(shutdownPublished) },
		})
	}()
	waitSignal(t, shutdownPublished, "Running registration Shutdown")
	releaseTerminal()
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "Running registration Run") {
		t.Fatal("Run did not complete")
	}
	assertSourceCleanupWake(t, loop, timerValue, 0)
}

func TestSourceRegistrationRunOvertakesFirstGate(t *testing.T) {
	loop := newLoop(false)
	t.Cleanup(func() { _ = loop.closeLoop() })
	firstGatePassed := make(chan struct{})
	resumeRegistration, releaseRegistration := newSourceRelease(t)
	registrationDone := make(chan registrationResult, 1)
	go func() {
		id, err := loop.prepareTimerRegistrationObserved(registrationObserver{
			firstGatePassed: func() {
				close(firstGatePassed)
				<-resumeRegistration
			},
		})
		registrationDone <- registrationResult{id: id, err: err}
	}()
	waitSignal(t, firstGatePassed, "registration first gate before Run")

	runWaiting := make(chan struct{})
	postDrain := make(chan struct{})
	resumeRun, releaseRun := newSourceRelease(t)
	resumeTerminal, releaseTerminal := newSourceRelease(t)
	waitCount := 0
	runResult := make(chan bool, 1)
	go func() {
		runResult <- loop.runObserved(lifecycleObserver{
			runWait: func() {
				waitCount++
				switch waitCount {
				case 1:
					close(runWaiting)
					<-resumeRun
				case 2:
					close(postDrain)
					<-resumeTerminal
				}
			},
		})
	}()
	waitSignal(t, runWaiting, "Run overtake registration wait")
	releaseRegistration()
	registration := receiveRegistration(t, registrationDone)
	if registration.err != nil || registration.id != 1 {
		t.Fatalf("Run-overtaken registration = (%d, %v)", registration.id, registration.err)
	}
	unrefPublished := make(chan struct{})
	unrefResult := make(chan error, 1)
	go func() {
		unrefResult <- loop.unrefTimerObserved(registration.id, referenceObserver{
			wakePublished: func() { close(unrefPublished) },
		})
	}()
	waitSignal(t, unrefPublished, "Run-overtaken registration Unref")
	assertErrorBlocked(t, unrefResult)
	if loop.nextTimerID.Load() != 1 || len(loop.timerMap) != 0 || len(loop.queue) != 2 ||
		loop.submissionEpoch.Load() != 2 || len(loop.fastWakeupCh) != 1 {
		t.Fatal("Run overtake did not preserve queued registration-Unref FIFO")
	}
	releaseRun()
	if err := receiveError(t, unrefResult); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, postDrain, "Run-overtaken registration post-drain wait")
	timerValue := loop.timerMap[registration.id]
	if timerValue == nil || timerValue.refed.Load() || loop.refedTimerCount.Load() != 0 ||
		loop.submissionEpoch.Load() != 4 || len(loop.fastWakeupCh) != 0 || loop.wakeAttempts.Load() != 0 {
		t.Fatal("Run-overtaken registration FIFO effect differs")
	}
	shutdownPublished := make(chan struct{})
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- loop.shutdownObserved(lifecycleObserver{
			shutdownPublished: func(<-chan struct{}) { close(shutdownPublished) },
		})
	}()
	waitSignal(t, shutdownPublished, "Run-overtaken registration Shutdown")
	releaseTerminal()
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "Run-overtaken registration Run") {
		t.Fatal("Run did not complete")
	}
	assertSourceCleanupWake(t, loop, timerValue, 0)
}

func TestSourceSleepingRegistrationWake(t *testing.T) {
	loop := newLoop(false)
	t.Cleanup(func() { _ = loop.closeLoop() })
	initialWait := make(chan struct{})
	sleepingWait := make(chan struct{})
	postDrain := make(chan struct{})
	resumeSleeping, releaseSleeping := newSourceRelease(t)
	resumeTerminal, releaseTerminal := newSourceRelease(t)
	waitCount := 0
	runResult := make(chan bool, 1)
	go func() {
		runResult <- loop.runObserved(lifecycleObserver{
			runWait: func() {
				waitCount++
				switch waitCount {
				case 1:
					close(initialWait)
				case 2:
					close(sleepingWait)
					<-resumeSleeping
				case 3:
					close(postDrain)
					<-resumeTerminal
				}
			},
		})
	}()
	waitSignal(t, initialWait, "Sleeping registration initial wait")
	sleepConfigured := make(chan bool, 1)
	if err := loop.submitToQueue(func() {
		sleepConfigured <- loop.configureUserFDCount(1) && loop.transition(stateSleeping)
	}); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, sleepConfigured, "Sleeping registration setup") {
		t.Fatal("Run did not enter Sleeping with one user FD")
	}
	waitSignal(t, sleepingWait, "Sleeping registration pre-receive wait")
	baselineEpoch := loop.submissionEpoch.Load()
	baselineAttempts := loop.wakeAttempts.Load()
	baselineSuccesses := loop.wakeSuccesses.Load()
	baselineRejections := loop.wakeRejections.Load()
	id, err := loop.prepareTimerRegistration()
	if err != nil || id != 1 {
		t.Fatalf("Sleeping registration = (%d, %v)", id, err)
	}
	observed := make(chan qualificationSnapshot, 1)
	if err := loop.submitToQueue(func() { observed <- loop.snapshot(id) }); err != nil {
		t.Fatal(err)
	}
	if state(loop.state.Load()) != stateSleeping || loop.nextTimerID.Load() != 1 || len(loop.timerMap) != 0 ||
		len(loop.queue) != 2 || loop.submissionEpoch.Load() != baselineEpoch+2 || len(loop.fastWakeupCh) != 1 ||
		loop.wakePending.Load() != 1 || loop.wakeAttempts.Load() != baselineAttempts+1 ||
		loop.wakeSuccesses.Load() != baselineSuccesses+1 || loop.wakeRejections.Load() != baselineRejections {
		t.Fatal("Sleeping registration wake publication differs")
	}
	releaseSleeping()
	var snapshot qualificationSnapshot
	select {
	case snapshot = <-observed:
	case <-time.After(time.Second):
		t.Fatal("Sleeping registration sentinel did not execute")
	}
	if !snapshot.present || !snapshot.refed || snapshot.refedCount != 1 || snapshot.submissionEpoch != baselineEpoch+3 ||
		snapshot.queued != 0 || snapshot.fastWakePending != 0 || snapshot.wakePending || snapshot.state != stateRunning ||
		snapshot.wakeAttempts != baselineAttempts+1 || snapshot.wakeSuccesses != baselineSuccesses+1 ||
		snapshot.wakeRejections != baselineRejections {
		t.Fatalf("Sleeping registration materialization snapshot = %+v", snapshot)
	}
	waitSignal(t, postDrain, "Sleeping registration post-drain wait")
	timerValue := loop.timerMap[id]
	shutdownPublished := make(chan struct{})
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- loop.shutdownObserved(lifecycleObserver{
			shutdownPublished: func(<-chan struct{}) { close(shutdownPublished) },
		})
	}()
	waitSignal(t, shutdownPublished, "Sleeping registration Shutdown")
	releaseTerminal()
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "Sleeping registration Run") {
		t.Fatal("Run did not complete")
	}
	assertSourceCleanupWake(t, loop, timerValue, 0)
}

func TestSourceRegistrationTerminalErrors(t *testing.T) {
	t.Run("Terminating", func(t *testing.T) {
		loop := newLoop(false)
		runResult := startSourceRun(t, loop)
		callbackStarted := make(chan struct{})
		callbackRelease, releaseCallback := newSourceRelease(t)
		if err := loop.submitToQueue(func() {
			close(callbackStarted)
			<-callbackRelease
		}); err != nil {
			t.Fatal(err)
		}
		waitSignal(t, callbackStarted, "blocking registration callback")
		published := make(chan struct{})
		shutdownResult := make(chan error, 1)
		go func() {
			shutdownResult <- loop.shutdownObserved(lifecycleObserver{
				shutdownPublished: func(<-chan struct{}) { close(published) },
			})
		}()
		waitSignal(t, published, "Terminating registration boundary")
		if id, err := loop.prepareTimerRegistration(); id != 0 || !errors.Is(err, errTerminated) {
			t.Fatalf("Terminating registration = (%d, %v)", id, err)
		}
		if loop.nextTimerID.Load() != 0 {
			t.Fatal("terminal rejection consumed a timer ID")
		}
		releaseCallback()
		if err := receiveError(t, shutdownResult); err != nil {
			t.Fatal(err)
		}
		if !receiveBool(t, runResult, "Terminating registration Run") {
			t.Fatal("Run did not complete")
		}
		assertSourceCleanupWake(t, loop, nil, 1)
	})

	t.Run("Terminated", func(t *testing.T) {
		loop := newLoop(false)
		if err := loop.closeLoop(); err != nil {
			t.Fatal(err)
		}
		if id, err := loop.prepareTimerRegistration(); id != 0 || !errors.Is(err, errTerminated) {
			t.Fatalf("Terminated registration = (%d, %v)", id, err)
		}
		if loop.nextTimerID.Load() != 0 {
			t.Fatal("terminal rejection consumed a timer ID")
		}
		assertSourceCleanup(t, loop, nil)
	})
}

func TestSourceConcurrentZeroOriginRegistrations(t *testing.T) {
	loop := newLoop(false)
	const workers = 32
	results := make(chan registrationResult, workers)
	var start sync.WaitGroup
	start.Add(1)
	for range workers {
		go func() {
			start.Wait()
			id, err := loop.prepareTimerRegistration()
			results <- registrationResult{id: id, err: err}
		}()
	}
	start.Done()
	seen := make(map[timerID]struct{}, workers)
	for range workers {
		result := receiveRegistration(t, results)
		if result.err != nil || result.id == 0 || result.id > workers {
			t.Fatalf("zero-origin registration = (%d, %v)", result.id, result.err)
		}
		if _, duplicate := seen[result.id]; duplicate {
			t.Fatalf("duplicate zero-origin timer ID %d", result.id)
		}
		seen[result.id] = struct{}{}
	}
	if len(seen) != workers || loop.nextTimerID.Load() != workers || len(loop.queue) != workers ||
		loop.submissionEpoch.Load() != workers || len(loop.fastWakeupCh) != 1 {
		t.Fatalf("zero-origin admission totals seen=%d next=%d queued=%d epoch=%d wake=%d",
			len(seen), loop.nextTimerID.Load(), len(loop.queue), loop.submissionEpoch.Load(), len(loop.fastWakeupCh))
	}
	beforeReceive := make(chan struct{})
	postDrain := make(chan struct{})
	resumeReceive, releaseReceive := newSourceRelease(t)
	resumeTerminal, releaseTerminal := newSourceRelease(t)
	waitCount := 0
	runResult := make(chan bool, 1)
	go func() {
		runResult <- loop.runObserved(lifecycleObserver{
			runWait: func() {
				waitCount++
				switch waitCount {
				case 1:
					close(beforeReceive)
					<-resumeReceive
				case 2:
					close(postDrain)
					<-resumeTerminal
				}
			},
		})
	}()
	waitSignal(t, beforeReceive, "zero-origin registration ordinary wait")
	if len(loop.timerMap) != 0 || loop.refedTimerCount.Load() != 0 || len(loop.queue) != workers ||
		loop.submissionEpoch.Load() != workers || len(loop.fastWakeupCh) != 1 {
		t.Fatal("Run materialized registrations before acquiring the source wake")
	}
	releaseReceive()
	waitSignal(t, postDrain, "zero-origin registration post-drain wait")
	timers := make([]*timer, 0, workers)
	for id := timerID(1); id <= workers; id++ {
		value, exists := loop.timerMap[id]
		if !exists || value == nil || value.id != id || !value.refed.Load() {
			t.Fatalf("materialized timer %d differs", id)
		}
		timers = append(timers, value)
	}
	if len(loop.timerMap) != workers || loop.refedTimerCount.Load() != workers || loop.submissionEpoch.Load() != workers*2 || len(loop.fastWakeupCh) != 0 {
		t.Fatalf("materialized totals timers=%d refed=%d epoch=%d wake=%d", len(loop.timerMap), loop.refedTimerCount.Load(), loop.submissionEpoch.Load(), len(loop.fastWakeupCh))
	}
	shutdownWoke := make(chan struct{})
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- loop.shutdownObserved(lifecycleObserver{
			shutdownWake: func() { close(shutdownWoke) },
		})
	}()
	waitSignal(t, shutdownWoke, "zero-origin registration Shutdown wake")
	releaseTerminal()
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "zero-origin registration Run") {
		t.Fatal("Run did not complete")
	}
	for id, value := range timers {
		if value.task != nil || value.refed.Load() || !value.canceled.Load() {
			t.Fatalf("timer %d cleanup differs", id+1)
		}
	}
	assertSourceCleanup(t, loop, nil)
}

func receiveRegistration(t *testing.T, result <-chan registrationResult) registrationResult {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(time.Second):
		t.Fatal("registration result did not return")
		return registrationResult{}
	}
}
