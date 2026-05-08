package timerrefclosureb77

import (
	"errors"
	"testing"
	"time"
)

type sourceTimerSetup struct {
	value      *timer
	configured bool
}

func TestSourceStartedShutdownWrapperCleansAfterRunExit(t *testing.T) {
	value := newLoop(false)
	runResult := startSourceRun(t, value)
	timerValue := seedSourceTimer(t, value, true)
	if err := value.shutdown(); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "source Run exit") {
		t.Fatal("source Run did not own its terminal exit")
	}
	if value.wakeAttempts.Load() != 1 || value.wakeSuccesses.Load() != 1 || value.wakeRejections.Load() != 0 {
		t.Fatal("started Shutdown physical wake classification differs")
	}
	assertSourceCleanup(t, value, timerValue)
}

func TestSourceStartedCloseWrapperCleansAfterRunExit(t *testing.T) {
	value := newLoop(false)
	runResult := startSourceRun(t, value)
	timerValue := seedSourceTimer(t, value, true)
	if err := value.closeLoop(); err != nil {
		t.Fatal(err)
	}
	if !receiveBool(t, runResult, "source Run exit") {
		t.Fatal("source Run did not own its Close exit")
	}
	if value.wakeAttempts.Load() != 1 || value.wakeSuccesses.Load() != 0 || value.wakeRejections.Load() != 1 {
		t.Fatal("started Close did not separate fast wake from rejected physical wake")
	}
	assertSourceCleanup(t, value, timerValue)
}

func TestSourceAwakeShutdownAndCloseDiscardAcceptedWork(t *testing.T) {
	for _, test := range []struct {
		name      string
		terminate func(*loop) error
	}{
		{name: "Shutdown", terminate: (*loop).shutdown},
		{name: "Close", terminate: (*loop).closeLoop},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := newLoop(false)
			started := make(chan struct{})
			resumeRun := make(chan struct{})
			t.Cleanup(func() { releaseSignal(resumeRun) })
			runResult := make(chan bool, 1)
			go func() {
				runResult <- value.runObserved(lifecycleObserver{runStarted: func() {
					close(started)
					<-resumeRun
				}})
			}()
			waitSignal(t, started, "Awake Run publication")
			executed := false
			if err := value.submitToQueue(func() { executed = true }); err != nil {
				t.Fatal(err)
			}
			if err := test.terminate(value); err != nil {
				t.Fatal(err)
			}
			close(resumeRun)
			if receiveBool(t, runResult, "Awake terminal Run") {
				t.Fatal("Awake terminal winner permitted a Run generation")
			}
			if executed || len(value.queue) != 0 || len(value.spare) != 0 {
				t.Fatal("Awake terminal cleanup executed or retained accepted work")
			}
			if state(value.state.Load()) != stateTerminated || value.ownerID.Load() != 0 {
				t.Fatal("Awake terminal state did not settle")
			}
			assertChannelClosed(t, value.loopDone, "Awake loopDone")
		})
	}
}

func TestSourceRepeatedShutdownDuringCloseTransition(t *testing.T) {
	value := newLoop(false)
	transitioned := make(chan struct{})
	resumeClose := make(chan struct{})
	t.Cleanup(func() { releaseSignal(resumeClose) })
	closeResult := make(chan error, 1)
	go func() {
		closeResult <- value.closeLoopObserved(lifecycleObserver{
			closeTransitioned: func() {
				close(transitioned)
				<-resumeClose
			},
		})
	}()
	waitSignal(t, transitioned, "paused Close transition")

	operation := value.activeTerminal
	if state(value.state.Load()) != stateTerminating || operation == nil ||
		operation.kind != terminalClose || operation.started || operation.run != nil {
		t.Fatal("Close transition boundary differs")
	}
	assertChannelOpen(t, value.runCh, "paused Close runCh")
	assertChannelOpen(t, value.loopDone, "paused Close loopDone")
	assertErrorBlocked(t, closeResult)
	if err := value.shutdown(); !errors.Is(err, errTerminated) {
		t.Fatalf("first losing Shutdown = %v", err)
	}
	if err := value.shutdown(); !errors.Is(err, errTerminated) {
		t.Fatalf("repeated losing Shutdown = %v", err)
	}
	if state(value.state.Load()) != stateTerminating || value.activeTerminal != operation {
		t.Fatal("losing Shutdown changed paused Close ownership")
	}

	releaseSignal(resumeClose)
	if err := receiveError(t, closeResult); err != nil {
		t.Fatal(err)
	}
	// Historical sync.Once residue: after Close commits, the consumed
	// Shutdown once returns nil.
	if err := value.shutdown(); err != nil {
		t.Fatalf("post-Close Shutdown = %v", err)
	}
	if value.wakeAttempts.Load() != 0 || value.wakeSuccesses.Load() != 0 ||
		value.wakeRejections.Load() != 0 {
		t.Fatal("Awake Close unexpectedly published a wake")
	}
	assertChannelOpen(t, value.runCh, "post-Close runCh")
	assertSourceTerminalCleanup(
		t,
		value,
		sourceTerminalCleanupExpectation{},
		"repeated Shutdown during Close transition",
	)
}

func TestSourceAutoExitCleansBeforeRunCompletion(t *testing.T) {
	value := newLoop(true)
	runResult := startSourceRun(t, value)
	if !receiveBool(t, runResult, "auto-exit Run") {
		t.Fatal("auto-exit Run did not complete")
	}
	if state(value.state.Load()) != stateTerminated || value.ownerID.Load() != 0 || value.quiescing.Load() ||
		value.activeRun != nil || value.activeTerminal != nil {
		t.Fatal("auto-exit state did not settle")
	}
	assertChannelClosed(t, value.loopDone, "auto-exit loopDone")
	if err := value.shutdown(); !errors.Is(err, errTerminated) {
		t.Fatalf("first post-auto-exit Shutdown = %v", err)
	}
	if err := value.shutdown(); err != nil {
		t.Fatalf("repeated post-auto-exit Shutdown = %v", err)
	}
}

func startSourceRun(t *testing.T, value *loop) <-chan bool {
	t.Helper()
	result := make(chan bool, 1)
	go func() { result <- value.run() }()
	waitSignal(t, value.runCh, "source Run start")
	return result
}

func startSeededRun(
	t *testing.T,
	value *loop,
	refed bool,
	observer lifecycleObserver,
) (*timer, <-chan bool) {
	t.Helper()
	type seedResult struct {
		timerValue *timer
		err        error
	}
	seeded := make(chan seedResult, 1)
	if err := value.submitToQueue(func() {
		id, err := value.scheduleTimer(time.Hour, func() {})
		if err == nil && id != 1 {
			err = errors.New("ScheduleTimer returned an unexpected ID")
		}
		if err == nil && !refed {
			err = value.unrefTimer(id)
		}
		seeded <- seedResult{timerValue: value.timerMap[id], err: err}
	}); err != nil {
		t.Fatal(err)
	}
	runResult := make(chan bool, 1)
	go func() { runResult <- value.runObserved(observer) }()
	select {
	case result := <-seeded:
		if result.err != nil || result.timerValue == nil {
			t.Fatalf("Run owner seed = (%p, %v)", result.timerValue, result.err)
		}
		return result.timerValue, runResult
	case <-value.loopDone:
		t.Fatal("Run exited before seeding the source timer")
		return nil, runResult
	}
}

func releaseSignal(signal chan struct{}) {
	select {
	case <-signal:
	default:
		close(signal)
	}
}

func seedSourceTimer(t *testing.T, value *loop, refed bool) *timer {
	t.Helper()
	id, err := value.scheduleTimer(time.Hour, func() {})
	if err != nil || id == 0 {
		t.Fatalf("ScheduleTimer = (%d, %v)", id, err)
	}
	result := make(chan sourceTimerSetup, 1)
	if err := value.submitToQueue(func() {
		timerValue := value.timerMap[id]
		if timerValue == nil {
			result <- sourceTimerSetup{}
			return
		}
		if !refed {
			if err := value.unrefTimer(id); err != nil {
				result <- sourceTimerSetup{}
				return
			}
		}
		result <- sourceTimerSetup{value: timerValue, configured: value.registerFD(1) == nil}
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case setup := <-result:
		if setup.value == nil || !setup.configured {
			t.Fatal("source timer setup failed")
		}
		return setup.value
	case <-time.After(time.Second):
		t.Fatal("source timer setup did not complete")
		return nil
	}
}

func assertSourceCleanup(t *testing.T, value *loop, timerValue *timer) {
	t.Helper()
	if timerValue == nil || timerValue.task != nil || timerValue.refed.Load() || !timerValue.canceled.Load() ||
		timerValue.heapIndex != -1 || timerValue.nestingLevel != 0 {
		t.Fatalf("timer cleanup = %+v", timerValue)
	}
	if len(value.timerMap) != 0 || len(value.timers) != 0 || len(value.fds) != 0 || value.refedTimerCount.Load() != 0 || value.promisifyCount.Load() != 0 || value.userIOFDCount.Load() != 0 ||
		len(value.queue) != 0 || len(value.spare) != 0 || value.quiescing.Load() {
		t.Fatal("loop cleanup did not restore terminal baseline")
	}
	if state(value.state.Load()) != stateTerminated || value.ownerID.Load() != 0 {
		t.Fatal("terminal owner/state did not settle")
	}
	assertChannelClosed(t, value.loopDone, "terminal loopDone")
}
