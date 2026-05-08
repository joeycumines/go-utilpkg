package timerrefclosureb77

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

type registrationSnapshot struct {
	present    bool
	refed      bool
	refedCount int32
	heapLen    int
	heapIndex  int
	epoch      uint64
	nextID     uint64
}

func TestSourcePreRunReferenceOutcomes(t *testing.T) {
	tests := []struct {
		name           string
		call           func(*loop, timerID, referenceObserver) error
		wantRefed      bool
		wantRefedCount int32
		wantEpoch      uint64
	}{
		{
			name:           "Ref",
			call:           (*loop).refTimerObserved,
			wantRefed:      true,
			wantRefedCount: 1,
			wantEpoch:      3,
		},
		{
			name:           "Unref",
			call:           (*loop).unrefTimerObserved,
			wantRefed:      false,
			wantRefedCount: 0,
			wantEpoch:      4,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Run("RunStarts", func(t *testing.T) {
				value := newLoop(false)
				resumeReference := make(chan struct{})
				resumeRun := make(chan struct{})
				t.Cleanup(func() {
					releaseSignal(resumeReference)
					releaseSignal(resumeRun)
					if state(value.state.Load()) != stateTerminated {
						_ = value.closeLoop()
					}
				})
				id, err := value.scheduleTimer(time.Hour, func() {})
				if err != nil || id != 1 {
					t.Fatalf("pre-Run ScheduleTimer = (%d, %v)", id, err)
				}
				if state(value.state.Load()) != stateAwake || value.ownerID.Load() != 0 ||
					value.activeRun != nil || value.activeTerminal != nil ||
					value.nextTimerID.Load() != 1 || len(value.queue) != 1 ||
					value.submissionEpoch.Load() != 1 || len(value.fastWakeupCh) != 1 ||
					len(value.timerMap) != 0 || len(value.timers) != 0 ||
					value.refedTimerCount.Load() != 0 {
					t.Fatal("pending timer registration baseline differs")
				}

				referenceWaiting := make(chan struct{})
				referenceWoke := make(chan struct{})
				referenceResult := make(chan error, 1)
				go func() {
					referenceResult <- test.call(value, id, referenceObserver{
						runWaitPending: func() {
							close(referenceWaiting)
							<-resumeReference
						},
						wakePublished: func() { close(referenceWoke) },
					})
				}()
				waitSignal(t, referenceWaiting, "pre-Run reference wait")

				runStarted := make(chan struct{})
				runResult := make(chan bool, 1)
				go func() {
					runResult <- value.runObserved(lifecycleObserver{runStarted: func() {
						close(runStarted)
						<-resumeRun
					}})
				}()
				waitSignal(t, runStarted, "pre-Run Run publication")
				releaseSignal(resumeReference)
				waitSignal(t, referenceWoke, "pre-Run reference wake")
				if state(value.state.Load()) != stateAwake || value.ownerID.Load() != 0 ||
					value.activeRun != nil || len(value.queue) != 2 ||
					value.submissionEpoch.Load() != 2 || len(value.fastWakeupCh) != 1 {
					t.Fatal("pre-Run reference admission boundary differs")
				}
				assertErrorBlocked(t, referenceResult)

				releaseSignal(resumeRun)
				if err := receiveError(t, referenceResult); err != nil {
					t.Fatal(err)
				}
				timerValue := value.timerMap[id]
				if timerValue == nil || timerValue.refed.Load() != test.wantRefed ||
					value.refedTimerCount.Load() != test.wantRefedCount ||
					value.submissionEpoch.Load() != test.wantEpoch ||
					len(value.timerMap) != 1 || len(value.timers) != 1 ||
					value.timers[0] != timerValue || timerValue.heapIndex != 0 ||
					value.nextTimerID.Load() != 1 || state(value.state.Load()) != stateRunning ||
					value.ownerID.Load() == 0 || value.activeRun == nil ||
					len(value.fastWakeupCh) != 0 || value.wakeAttempts.Load() != 0 ||
					value.wakeSuccesses.Load() != 0 || value.wakeRejections.Load() != 0 {
					t.Fatal("pre-Run reference execution differs")
				}
				if err := value.closeLoop(); err != nil {
					t.Fatal(err)
				}
				if !receiveBool(t, runResult, "pre-Run reference Run") {
					t.Fatal("Run did not exit for Close")
				}
				assertSourceCleanup(t, value, timerValue)
			})

			t.Run("TerminalCompletes", func(t *testing.T) {
				value := newLoop(false)
				resumeReference := make(chan struct{})
				t.Cleanup(func() { releaseSignal(resumeReference) })
				id, err := value.scheduleTimer(time.Hour, func() {})
				if err != nil || id != 1 {
					t.Fatalf("pre-Run ScheduleTimer = (%d, %v)", id, err)
				}
				referenceWaiting := make(chan struct{})
				referenceResult := make(chan error, 1)
				go func() {
					referenceResult <- test.call(value, id, referenceObserver{
						runWaitPending: func() {
							close(referenceWaiting)
							<-resumeReference
						},
					})
				}()
				waitSignal(t, referenceWaiting, "terminal pre-Run reference wait")
				if err := value.closeLoop(); err != nil {
					t.Fatal(err)
				}
				releaseSignal(resumeReference)
				if err := receiveError(t, referenceResult); !errors.Is(err, errTerminated) {
					t.Fatalf("reference after terminal completion = %v", err)
				}
				if value.nextTimerID.Load() != 1 || value.submissionEpoch.Load() != 1 ||
					len(value.timerMap) != 0 || len(value.timers) != 0 ||
					value.refedTimerCount.Load() != 0 || len(value.fastWakeupCh) != 1 ||
					value.wakeAttempts.Load() != 0 || value.wakeSuccesses.Load() != 0 ||
					value.wakeRejections.Load() != 0 {
					t.Fatal("terminal pre-Run reference cleanup differs")
				}
				assertChannelOpen(t, value.runCh, "terminal pre-Run runCh")
				assertSourceTerminalCleanup(
					t,
					value,
					sourceTerminalCleanupExpectation{fastWake: 1, submissionEpoch: 1},
					"terminal pre-Run reference",
				)
			})

			t.Run("Deadline", func(t *testing.T) {
				value := newLoop(false)
				id, err := value.scheduleTimer(time.Hour, func() {})
				if err != nil || id != 1 {
					t.Fatalf("pre-Run ScheduleTimer = (%d, %v)", id, err)
				}
				waitStarted := make(chan time.Time, 1)
				referenceResult := make(chan error, 1)
				go func() {
					referenceResult <- test.call(value, id, referenceObserver{
						runWaitPending: func() { waitStarted <- time.Now() },
					})
				}()
				var started time.Time
				select {
				case started = <-waitStarted:
				case <-time.After(time.Second):
					t.Fatal("pre-Run reference deadline did not start")
				}
				select {
				case err := <-referenceResult:
					if !errors.Is(err, errNotRunning) {
						t.Fatalf("pre-Run reference deadline = %v", err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("pre-Run reference deadline did not return")
				}
				if time.Since(started) < time.Second {
					t.Fatal("pre-Run reference returned before its historical deadline")
				}
				if state(value.state.Load()) != stateAwake || value.ownerID.Load() != 0 ||
					value.activeRun != nil || value.activeTerminal != nil ||
					value.nextTimerID.Load() != 1 || len(value.queue) != 1 ||
					value.submissionEpoch.Load() != 1 || len(value.fastWakeupCh) != 1 ||
					len(value.timerMap) != 0 || len(value.timers) != 0 ||
					value.refedTimerCount.Load() != 0 || value.wakeUpSignalPending.Load() != 0 ||
					value.wakeAttempts.Load() != 0 || value.wakeSuccesses.Load() != 0 ||
					value.wakeRejections.Load() != 0 || value.quiescing.Load() {
					t.Fatal("pre-Run reference deadline changed the pending timer baseline")
				}
				assertChannelOpen(t, value.runCh, "deadline pre-Run runCh")
				assertChannelOpen(t, value.loopDone, "deadline pre-Run loopDone")
				if err := value.closeLoop(); err != nil {
					t.Fatal(err)
				}
				if value.nextTimerID.Load() != 1 {
					t.Fatal("pre-Run reference cleanup changed the claimed timer ID")
				}
				assertChannelOpen(t, value.runCh, "post-deadline pre-Run runCh")
				assertSourceTerminalCleanup(
					t,
					value,
					sourceTerminalCleanupExpectation{fastWake: 1, submissionEpoch: 1},
					"pre-Run reference deadline",
				)
			})
		})
	}
}

func TestSourceOwnerScheduleTimerRegistersSynchronously(t *testing.T) {
	value := newLoop(false)
	result := make(chan registrationSnapshot, 1)
	if err := value.submitToQueue(func() {
		id, err := value.scheduleTimer(time.Hour, func() {})
		if err != nil || id != 1 {
			t.Errorf("owner ScheduleTimer = (%d, %v)", id, err)
			result <- registrationSnapshot{}
			return
		}
		if err := value.unrefTimer(id); err != nil {
			t.Error(err)
		}
		if err := value.refTimer(id); err != nil {
			t.Error(err)
		}
		timerValue := value.timerMap[id]
		result <- registrationSnapshot{
			present: true, refed: timerValue.refed.Load(), refedCount: value.refedTimerCount.Load(),
			heapLen: len(value.timers), heapIndex: timerValue.heapIndex,
			epoch: value.submissionEpoch.Load(), nextID: value.nextTimerID.Load(),
		}
	}); err != nil {
		t.Fatal(err)
	}
	runResult := startSourceRun(t, value)
	got := receiveRegistrationSnapshot(t, result)
	if !got.present || !got.refed || got.refedCount != 1 || got.heapLen != 1 || got.heapIndex != 0 ||
		got.epoch != 4 || got.nextID != 1 {
		t.Fatalf("owner registration snapshot = %+v", got)
	}
	if err := value.closeLoop(); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "owner registration Run") {
		t.Fatal("Run did not exit for Close")
	}
}

func TestSourceExternalScheduleReturnsBeforeRegistrationAndOrdersUnref(t *testing.T) {
	value := newLoop(false)
	entered := make(chan struct{})
	resumeRegistration := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeRegistration) })
	id, err := value.scheduleTimerObserved(time.Hour, func() {}, registrationObserver{
		registrationEntered: func() {
			close(entered)
			<-resumeRegistration
		},
	})
	if err != nil || id != 1 {
		t.Fatalf("Awake ScheduleTimer = (%d, %v)", id, err)
	}
	if len(value.timerMap) != 0 || len(value.timers) != 0 || len(value.queue) != 1 ||
		value.submissionEpoch.Load() != 1 || len(value.fastWakeupCh) != 1 {
		t.Fatal("ScheduleTimer did not return at the source admission boundary")
	}
	runResult := startSourceRun(t, value)
	waitSignal(t, entered, "registration entry")
	admitted := make(chan struct{})
	unrefResult := make(chan error, 1)
	go func() {
		unrefResult <- value.unrefTimerObserved(id, referenceObserver{
			queueAdmitted: func(uint64) { close(admitted) },
		})
	}()
	waitSignal(t, admitted, "post-registration Unref admission")
	close(resumeRegistration)
	if err := receiveError(t, unrefResult); err != nil {
		t.Fatal(err)
	}
	snapshotResult := make(chan registrationSnapshot, 1)
	if err := value.submitToQueue(func() {
		timerValue := value.timerMap[id]
		snapshot := registrationSnapshot{
			present: timerValue != nil, refed: timerValue != nil && timerValue.refed.Load(),
			refedCount: value.refedTimerCount.Load(), heapLen: len(value.timers),
			heapIndex: -1, epoch: value.submissionEpoch.Load(), nextID: value.nextTimerID.Load(),
		}
		if timerValue != nil {
			snapshot.heapIndex = timerValue.heapIndex
		}
		snapshotResult <- snapshot
	}); err != nil {
		t.Fatal(err)
	}
	got := receiveRegistrationSnapshot(t, snapshotResult)
	if !got.present || got.refed || got.refedCount != 0 || got.heapLen != 1 || got.heapIndex != 0 ||
		got.epoch != 5 || got.nextID != 1 {
		t.Fatalf("external registration snapshot = %+v", got)
	}
	if err := value.closeLoop(); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "external registration Run") {
		t.Fatal("Run did not exit for Close")
	}
}

func TestSourceRegistrationFirstGateTerminalOvertakeConsumesID(t *testing.T) {
	for _, test := range []struct {
		name      string
		terminate func(*loop) error
	}{
		{name: "Shutdown", terminate: (*loop).shutdown},
		{name: "Close", terminate: (*loop).closeLoop},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := newLoop(false)
			passed := make(chan struct{})
			resume := make(chan struct{})
			t.Cleanup(func() { releaseSignal(resume) })
			result := make(chan registrationResult, 1)
			go func() {
				id, err := value.scheduleTimerObserved(time.Hour, func() {}, registrationObserver{
					firstGatePassed: func() {
						close(passed)
						<-resume
					},
				})
				result <- registrationResult{id: id, err: err}
			}()
			waitSignal(t, passed, "ScheduleTimer first gate")
			if err := test.terminate(value); err != nil {
				t.Fatal(err)
			}
			close(resume)
			got := receiveRegistration(t, result)
			if got.id != 0 || !errors.Is(got.err, errTerminated) || value.nextTimerID.Load() != 1 ||
				value.submissionEpoch.Load() != 0 || len(value.queue) != 0 || len(value.timerMap) != 0 || len(value.timers) != 0 {
				t.Fatalf("terminal-overrun registration = (%+v next=%d epoch=%d)", got, value.nextTimerID.Load(), value.submissionEpoch.Load())
			}
		})
	}
}

func TestSourceRegistrationExhaustionAndConcurrentIDs(t *testing.T) {
	exhausted := newLoopWithTimerLimit(false, 2)
	for want := timerID(1); want <= 2; want++ {
		id, err := exhausted.scheduleTimer(time.Hour, func() {})
		if err != nil || id != want {
			t.Fatalf("ScheduleTimer %d = (%d, %v)", want, id, err)
		}
	}
	if id, err := exhausted.scheduleTimer(time.Hour, func() {}); id != 0 || !errors.Is(err, errTimerIDExhausted) {
		t.Fatalf("exhausted ScheduleTimer = (%d, %v)", id, err)
	}
	if exhausted.nextTimerID.Load() != 3 || exhausted.submissionEpoch.Load() != 2 || len(exhausted.queue) != 2 {
		t.Fatal("exhaustion changed admitted registration state")
	}
	if err := exhausted.closeLoop(); err != nil {
		t.Fatal(err)
	}

	const workers = 32
	value := newLoopWithTimerLimit(false, workers)
	results := make(chan registrationResult, workers)
	var start sync.WaitGroup
	start.Add(1)
	for range workers {
		go func() {
			start.Wait()
			id, err := value.scheduleTimer(time.Hour, func() {})
			results <- registrationResult{id: id, err: err}
		}()
	}
	start.Done()
	seen := make(map[timerID]struct{}, workers)
	for range workers {
		result := receiveRegistration(t, results)
		if result.err != nil || result.id == 0 || result.id > workers {
			t.Fatalf("concurrent registration = %+v", result)
		}
		if _, duplicate := seen[result.id]; duplicate {
			t.Fatalf("duplicate timer ID %d", result.id)
		}
		seen[result.id] = struct{}{}
	}
	if len(seen) != workers || value.nextTimerID.Load() != workers || value.submissionEpoch.Load() != workers || len(value.queue) != workers {
		t.Fatal("concurrent registration admission totals differ")
	}
	if err := value.closeLoop(); err != nil {
		t.Fatal(err)
	}
}

func TestSourceStartedTerminalRegistrationDisposition(t *testing.T) {
	for _, immediate := range []bool{false, true} {
		name := "ShutdownDrains"
		if immediate {
			name = "CloseDiscards"
		}
		t.Run(name, func(t *testing.T) {
			value := newLoop(false)
			waiting := make(chan struct{})
			resumeRun := make(chan struct{})
			t.Cleanup(func() { releaseSignal(resumeRun) })
			runResult := make(chan bool, 1)
			go func() {
				runResult <- value.runObserved(lifecycleObserver{runWait: func() {
					close(waiting)
					<-resumeRun
				}})
			}()
			waitSignal(t, waiting, "started terminal Run wait")
			committed := make(chan struct{})
			id, err := value.scheduleTimerObserved(time.Hour, func() {}, registrationObserver{
				registrationCommitted: func() { close(committed) },
			})
			if err != nil || id != 1 {
				t.Fatalf("started terminal ScheduleTimer = (%d, %v)", id, err)
			}
			woke := make(chan struct{})
			terminalResult := make(chan error, 1)
			if immediate {
				go func() {
					terminalResult <- value.closeLoopObserved(lifecycleObserver{closeWake: func() { close(woke) }})
				}()
			} else {
				go func() {
					terminalResult <- value.shutdownObserved(lifecycleObserver{shutdownWake: func() { close(woke) }})
				}()
			}
			waitSignal(t, woke, "started terminal wake")
			close(resumeRun)
			if !receiveBool(t, runResult, "started terminal Run") {
				t.Fatal("Run did not exit for terminal operation")
			}
			if err := receiveError(t, terminalResult); err != nil {
				t.Fatal(err)
			}
			if immediate {
				assertChannelOpen(t, committed, "Close-discarded registration")
				if value.submissionEpoch.Load() != 1 {
					t.Fatal("Close executed admitted registration")
				}
			} else {
				waitSignal(t, committed, "Shutdown-drained registration")
				if value.submissionEpoch.Load() != 2 {
					t.Fatal("Shutdown did not execute admitted registration")
				}
			}
			if len(value.timerMap) != 0 || len(value.timers) != 0 || len(value.queue) != 0 {
				t.Fatal("terminal registration cleanup differs")
			}
		})
	}
}

func TestSourceTerminatingWindowAcceptsRegistration(t *testing.T) {
	value := newLoop(false)
	waiting := make(chan struct{})
	resumeRun := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeRun) })
	runResult := make(chan bool, 1)
	go func() {
		runResult <- value.runObserved(lifecycleObserver{runWait: func() {
			close(waiting)
			<-resumeRun
		}})
	}()
	waitSignal(t, waiting, "terminating registration Run wait")
	beforeCommit := make(chan struct{})
	resumeCommit := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeCommit) })
	shutdownResult := make(chan error, 1)
	go func() {
		shutdownResult <- value.shutdownObserved(lifecycleObserver{
			beforeShutdownCommit: func() {
				close(beforeCommit)
				<-resumeCommit
			},
		})
	}()
	close(resumeRun)
	if !receiveBool(t, runResult, "terminating registration Run") {
		t.Fatal("Run did not exit for Shutdown")
	}
	waitSignal(t, beforeCommit, "terminating registration commit boundary")
	committed := make(chan struct{})
	id, err := value.scheduleTimerObserved(time.Hour, func() {}, registrationObserver{
		registrationCommitted: func() { close(committed) },
	})
	if err != nil || id != 1 || len(value.queue) != 1 || value.submissionEpoch.Load() != 1 {
		t.Fatalf("Terminating ScheduleTimer = (%d, %v, queued=%d, epoch=%d)", id, err, len(value.queue), value.submissionEpoch.Load())
	}
	close(resumeCommit)
	waitSignal(t, committed, "Terminating registration drain")
	if err := receiveError(t, shutdownResult); err != nil {
		t.Fatal(err)
	}
	if value.submissionEpoch.Load() != 2 || len(value.timerMap) != 0 || len(value.timers) != 0 {
		t.Fatal("Terminating registration cleanup differs")
	}
}

func TestSourcePostTerminalReferencesAreRejected(t *testing.T) {
	for _, test := range []struct {
		name      string
		terminate func(*loop) error
	}{
		{name: "Shutdown", terminate: (*loop).shutdown},
		{name: "Close", terminate: (*loop).closeLoop},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := newLoop(false)
			if err := test.terminate(value); err != nil {
				t.Fatal(err)
			}
			if err := value.refTimer(1); !errors.Is(err, errTerminated) {
				t.Fatalf("post-%s RefTimer = %v", test.name, err)
			}
			if err := value.unrefTimer(1); !errors.Is(err, errTerminated) {
				t.Fatalf("post-%s UnrefTimer = %v", test.name, err)
			}
			if value.nextTimerID.Load() != 0 ||
				value.submissionEpoch.Load() != 0 ||
				len(value.queue) != 0 ||
				len(value.fastWakeupCh) != 0 {
				t.Fatal("post-terminal reference call mutated state")
			}
		})
	}
}

func TestSourceTimerHeapOrdersEarlierDeadline(t *testing.T) {
	value := newLoop(false)
	ordered := make(chan bool, 1)
	if err := value.submitToQueue(func() {
		lateID, lateErr := value.scheduleTimer(2*time.Hour, func() {})
		earlyID, earlyErr := value.scheduleTimer(time.Hour, func() {})
		if lateErr != nil || earlyErr != nil {
			t.Errorf("ScheduleTimer errors = (%v, %v)", lateErr, earlyErr)
			ordered <- false
			return
		}
		late := value.timerMap[lateID]
		early := value.timerMap[earlyID]
		ordered <- lateID == 1 &&
			earlyID == 2 &&
			len(value.timers) == 2 &&
			value.timers[0] == early &&
			value.timers[1] == late &&
			early.heapIndex == 0 &&
			late.heapIndex == 1 &&
			value.timers.Less(0, 1)
	}); err != nil {
		t.Fatal(err)
	}

	runResult := startSourceRun(t, value)
	if !receiveBool(t, ordered, "timer heap ordering") {
		t.Fatal("earlier timer did not repair heap order and indices")
	}
	if err := value.closeLoop(); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "timer heap ordering Run") {
		t.Fatal("Run did not exit for Close")
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

func receiveRegistrationSnapshot(t *testing.T, result <-chan registrationSnapshot) registrationSnapshot {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(time.Second):
		t.Fatal("registration snapshot did not return")
		return registrationSnapshot{}
	}
}

func assertChannelOpen(t *testing.T, channel <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-channel:
		t.Fatalf("%s closed", name)
	default:
	}
}
