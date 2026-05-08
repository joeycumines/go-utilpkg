package timerrefclosureawake

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

func TestAwakeScheduleReferenceFIFO(t *testing.T) {
	value := newLoop(false)
	id, err := value.prepareTimerRegistration()
	if err != nil || id != 1 {
		t.Fatalf("prepareTimerRegistration = %d, %v", id, err)
	}
	if value.ownerID.Load() != 0 || state(value.state.Load()) != stateAwake ||
		len(value.timerMap) != 0 || len(value.queue) != 1 {
		t.Fatal("pre-Run registration did not remain ownerless and queued")
	}
	if err := value.unrefTimer(id); err != nil {
		t.Fatalf("Awake unrefTimer = %v", err)
	}
	if err := value.refTimer(id); err != nil {
		t.Fatalf("Awake refTimer = %v", err)
	}
	for _, invalid := range []timerID{0, id + 1} {
		if err := value.refTimer(invalid); !errors.Is(err, errNotRunning) {
			t.Fatalf("Awake refTimer(%d) = %v", invalid, err)
		}
		if err := value.unrefTimer(invalid); !errors.Is(err, errNotRunning) {
			t.Fatalf("Awake unrefTimer(%d) = %v", invalid, err)
		}
	}
	drained := make(chan struct{})
	waits := 0
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{runWait: func() {
			waits++
			if waits == 2 {
				close(drained)
			}
		}})
	}()
	waitSignal(t, drained, "Awake FIFO drain")
	timerValue := value.timerMap[id]
	if timerValue == nil || timerValue.heapIndex != -1 || !timerValue.refed.Load() ||
		value.refedTimerCount.Load() != 1 || value.submissionEpoch.Load() != 6 ||
		len(value.queue) != 0 || len(value.fastWakeupCh) != 0 {
		t.Fatal("post-Run registration and reference FIFO mismatch")
	}
	closeResult := make(chan error, 1)
	go func() { closeResult <- value.closeLoop() }()
	if !receiveBool(t, runResult, "Awake FIFO Run") {
		t.Fatal("Run did not exit for Close")
	}
	if err := receiveError(t, closeResult); err != nil {
		t.Fatal(err)
	}
}
func TestAwakeRangeValidMissingIDQueuesNoOp(t *testing.T) {
	value := newLoop(false)
	claimed := make(chan timerID, 1)
	resumeRegistration := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeRegistration) })
	registration := make(chan registrationResult, 1)
	go func() {
		id, err := value.prepareTimerRegistrationObserved(registrationObserver{timerIDClaimed: func(id timerID) {
			claimed <- id
			<-resumeRegistration
		}})
		registration <- registrationResult{id: id, err: err}
	}()
	var id timerID
	select {
	case id = <-claimed:
		if id != 1 {
			t.Fatalf("claimed ID = %d", id)
		}
	case <-time.After(time.Second):
		t.Fatal("registration did not claim an ID")
	}
	if err := value.unrefTimer(id); err != nil {
		t.Fatalf("Awake claimed-but-missing Unref = %v", err)
	}
	if err := value.refTimer(id); err != nil {
		t.Fatalf("Awake claimed-but-missing Ref = %v", err)
	}
	close(resumeRegistration)
	result := receiveRegistration(t, registration)
	if result.err != nil || result.id != id {
		t.Fatalf("registration = (%d, %v)", result.id, result.err)
	}
	drained := make(chan struct{})
	waits := 0
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{runWait: func() {
			waits++
			if waits == 2 {
				close(drained)
			}
		}})
	}()
	waitSignal(t, drained, "claimed-but-missing FIFO drain")
	timerValue := value.timerMap[id]
	if timerValue == nil || !timerValue.refed.Load() || value.refedTimerCount.Load() != 1 ||
		value.submissionEpoch.Load() != 4 || timerValue.heapIndex != -1 {
		t.Fatal("claimed-but-missing operations fabricated an early timer or changed registration")
	}
	closeResult := make(chan error, 1)
	go func() { closeResult <- value.closeLoop() }()
	if !receiveBool(t, runResult, "claimed-but-missing Run") {
		t.Fatal("Run did not exit for Close")
	}
	if err := receiveError(t, closeResult); err != nil {
		t.Fatal(err)
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

func TestAwakeFinishDiscardsRegistration(t *testing.T) {
	value := newLoop(false)
	id, err := value.prepareTimerRegistration()
	if err != nil || id != 1 {
		t.Fatalf("prepareTimerRegistration = %d, %v", id, err)
	}
	if err := value.closeLoop(); err != nil {
		t.Fatal(err)
	}
	if value.run() || len(value.timerMap) != 0 || len(value.queue) != 0 ||
		len(value.fastWakeupCh) != 0 || value.wakePending.Load() != 0 || value.quiescing.Load() {
		t.Fatal("Awake Close did not permanently discard registration state")
	}
}
func TestAwakeGracefulShutdownDrainsAcceptedFIFO(t *testing.T) {
	value := newLoop(false)
	id, err := value.prepareTimerRegistration()
	if err != nil || id != 1 {
		t.Fatalf("prepareTimerRegistration = %d, %v", id, err)
	}
	if err := value.unrefTimer(id); err != nil {
		t.Fatal(err)
	}
	if err := value.refTimer(id); err != nil {
		t.Fatal(err)
	}
	if err := value.shutdown(); err != nil {
		t.Fatal(err)
	}
	timerValue := value.timerMap[id]
	if value.wakeAttempts.Load() != 0 || timerValue == nil || timerValue.heapIndex != -1 ||
		!timerValue.refed.Load() || value.refedTimerCount.Load() != 1 ||
		value.submissionEpoch.Load() != 6 || state(value.state.Load()) != stateTerminated {
		t.Fatal("Awake graceful Shutdown did not drain accepted FIFO")
	}
	assertChannelClosed(t, value.loopDone, "Awake loopDone")
}
func TestRegistrationExhaustionIsSticky(t *testing.T) {
	value := newLoopWithTimerIDLimit(false, 2)
	for want := timerID(1); want <= 2; want++ {
		id, err := value.prepareTimerRegistration()
		if err != nil || id != want {
			t.Fatalf("registration %d = (%d, %v)", want, id, err)
		}
	}
	beforeEpoch := value.submissionEpoch.Load()
	for attempt := range 2 {
		id, err := value.prepareTimerRegistration()
		if id != 0 || !errors.Is(err, errIDExhausted) {
			t.Fatalf("attempt %d = (%d, %v)", attempt, id, err)
		}
	}
	if value.nextTimerID.Load() != 2 || len(value.queue) != 2 ||
		len(value.timerMap) != 0 || value.submissionEpoch.Load() != beforeEpoch ||
		len(value.fastWakeupCh) != 1 {
		t.Fatal("sticky configured exhaustion mutated registration state")
	}
}
func TestRegistrationExhaustionConcurrentBoundary(t *testing.T) {
	value := newLoopWithTimerIDLimit(false, 1)
	const workers = 32
	results := make(chan registrationResult, workers)
	var start sync.WaitGroup
	start.Add(1)
	for range workers {
		go func() {
			start.Wait()
			id, err := value.prepareTimerRegistration()
			results <- registrationResult{id: id, err: err}
		}()
	}
	start.Done()
	successes := 0
	exhausted := 0
	for range workers {
		result := receiveRegistration(t, results)
		switch {
		case result.err == nil && result.id == 1:
			successes++
		case result.id == 0 && errors.Is(result.err, errIDExhausted):
			exhausted++
		default:
			t.Fatalf("boundary result = (%d, %v)", result.id, result.err)
		}
	}
	if successes != 1 || exhausted != workers-1 || value.nextTimerID.Load() != 1 || len(value.queue) != 1 {
		t.Fatalf("boundary totals success=%d exhausted=%d next=%d queued=%d",
			successes, exhausted, value.nextTimerID.Load(), len(value.queue))
	}
	if id, err := value.prepareTimerRegistration(); id != 0 || !errors.Is(err, errIDExhausted) {
		t.Fatalf("post-boundary exhaustion = (%d, %v)", id, err)
	}
}
func TestRegistrationClaimSurvivesAdmissionLoss(t *testing.T) {
	value := newLoop(false)
	claimed := make(chan timerID, 1)
	resumeRegistration := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeRegistration) })
	result := make(chan registrationResult, 1)
	go func() {
		id, err := value.prepareTimerRegistrationObserved(registrationObserver{timerIDClaimed: func(id timerID) {
			claimed <- id
			<-resumeRegistration
		}})
		result <- registrationResult{id: id, err: err}
	}()
	select {
	case id := <-claimed:
		if id != 1 {
			t.Fatalf("claimed ID = %d", id)
		}
	case <-time.After(time.Second):
		t.Fatal("registration did not claim an ID")
	}
	if err := value.closeLoop(); err != nil {
		t.Fatal(err)
	}
	close(resumeRegistration)
	registration := receiveRegistration(t, result)
	if registration.id != 0 || !errors.Is(registration.err, errTerminated) {
		t.Fatalf("post-claim registration = (%d, %v)", registration.id, registration.err)
	}
	if value.nextTimerID.Load() != 1 || len(value.queue) != 0 || len(value.timerMap) != 0 {
		t.Fatal("admission loss rolled back or materialized the claimed ID")
	}
}
func TestAwakeRegistrationStateBoundary(t *testing.T) {
	t.Run("CloseAfterStateCheck", func(t *testing.T) {
		value := newLoop(false)
		stateChecked := make(chan struct{})
		resume := make(chan struct{})
		t.Cleanup(func() { releaseSignal(resume) })
		result := make(chan registrationResult, 1)
		go func() {
			id, err := value.prepareTimerRegistrationObserved(registrationObserver{
				stateChecked: func() {
					close(stateChecked)
					<-resume
				},
			})
			result <- registrationResult{id: id, err: err}
		}()
		waitSignal(t, stateChecked, "Awake registration state check")
		if err := value.closeLoop(); err != nil {
			t.Fatal(err)
		}
		close(resume)
		registration := receiveRegistration(t, result)
		if registration.id != 0 || !errors.Is(registration.err, errTerminated) {
			t.Fatalf("registration after Close = (%d, %v)", registration.id, registration.err)
		}
		if value.nextTimerID.Load() != 0 || len(value.queue) != 0 || len(value.timerMap) != 0 {
			t.Fatal("state-boundary rejection consumed or materialized an ID")
		}
	})

	t.Run("RunningOwner", func(t *testing.T) {
		value := newLoop(false)
		result := make(chan registrationResult, 1)
		if err := value.submitToQueue(func() {
			id, err := value.prepareTimerRegistration()
			result <- registrationResult{id: id, err: err}
		}); err != nil {
			t.Fatal(err)
		}
		runResult := make(chan bool, 1)
		go func() { runResult <- value.run() }()
		registration := receiveRegistration(t, result)
		if registration.id != 0 || !errors.Is(registration.err, errNotRunning) ||
			value.nextTimerID.Load() != 0 {
			t.Fatalf("Running owner registration = (%d, %v)", registration.id, registration.err)
		}
		if err := value.closeLoop(); err != nil {
			t.Fatal(err)
		}
		if !receiveBool(t, runResult, "Running registration Close Run") {
			t.Fatal("Run did not complete Close")
		}
	})
}

func TestAwakeRemovalLifecycleOutcomes(t *testing.T) {
	value := newLoop(false)
	if !value.bindOwner() || !value.seed(1, true) || !value.seed(2, false) {
		t.Fatal("owner timer setup failed")
	}
	if !value.remove(1) || value.remove(1) ||
		value.refedTimerCount.Load() != 0 || len(value.timerMap) != 1 {
		t.Fatal("Running removal outcomes differ")
	}
	if !value.transition(stateSleeping) {
		t.Fatal("owner did not enter Sleeping")
	}
	if value.remove(2) || len(value.timerMap) != 1 {
		t.Fatal("Sleeping removal changed the timer set")
	}
	if !value.transition(stateRunning) || !value.remove(2) ||
		len(value.timerMap) != 0 || value.refedTimerCount.Load() != 0 {
		t.Fatal("resumed removal did not settle")
	}
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
