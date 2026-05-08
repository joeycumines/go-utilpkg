package eventloop

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/goroutineid"
)

func holdFirstFastPathEntryT(t *testing.T, hooks *loopTestHooks) (<-chan struct{}, func()) {
	t.Helper()
	entered := make(chan struct{})
	release := make(chan struct{})
	releaseFn := releaseSignalT(t, release)
	var once sync.Once
	hooks.OnFastPathEntry = func() {
		once.Do(func() {
			close(entered)
			<-release
		})
	}
	return entered, releaseFn
}

func TestFastPathInitialTurnDrainsMicrotasksBeforeSubmittedWork(t *testing.T) {
	loop := New(WithAutoExit(true))
	registerLoopCleanupT(t, loop)

	var mu sync.Mutex
	var order []string
	record := func(label string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, label)
	}

	if err := loop.ScheduleNextTick(func() { record("nextTick") }); err != nil {
		t.Fatalf("ScheduleNextTick: %v", err)
	}
	if err := loop.ScheduleMicrotask(func() { record("microtask") }); err != nil {
		t.Fatalf("ScheduleMicrotask: %v", err)
	}
	if err := loop.Submit(func() { record("submit") }); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	if err := runAutoExitLoop(t, loop); err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	got := append([]string(nil), order...)
	mu.Unlock()
	want := []string{"nextTick", "microtask", "submit"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("execution order = %v, want %v", got, want)
	}
}

func TestFastPathInitialMicrotaskTimerDoesNotSleepForever(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop := New(WithAutoExit(true))

	timerDone := make(chan struct{})
	if err := loop.ScheduleMicrotask(func() {
		if _, err := loop.ScheduleTimer(time.Millisecond, func() { close(timerDone) }); err != nil {
			t.Errorf("ScheduleTimer from initial microtask: %v", err)
		}
	}); err != nil {
		t.Fatalf("ScheduleMicrotask: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	select {
	case <-timerDone:
	case <-ctx.Done():
		t.Fatal("timer scheduled by initial fast-path microtask did not fire")
	}

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Run did not return after timer scheduled by initial fast-path microtask")
	}
}

func TestFastPathDrainsInternalBeforeExternalWithoutPollTick(t *testing.T) {
	loop := New(WithFastPathMode(FastPathForced))
	registerLoopCleanupT(t, loop)

	// Hold the owner before its first ready-work check. An internal callback is
	// not a turn boundary: it runs inside the internal phase snapshot, where a
	// later batch may correctly miss that phase while joining the external one.
	var pollEntered atomic.Bool
	hooks := &loopTestHooks{PrePollSleep: func() { pollEntered.Store(true) }}
	fastPathEntered, releaseFastPath := holdFirstFastPathEntryT(t, hooks)
	loop.testHooks = hooks

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitContractSignal(t, fastPathEntered, "fast-path entry")

	var mu sync.Mutex
	var order []string
	done := make(chan struct{})
	record := func(label string) {
		mu.Lock()
		order = append(order, label)
		if len(order) == 2 {
			close(done)
		}
		mu.Unlock()
	}

	// Commit both commands under one ingress lock so the test covers the owner
	// tick phase order rather than racing two separate producer wakeups.
	loop.externalMu.Lock()
	loop.enqueueCommandLocked(loopCommand{kind: loopCommandExternal, fn: func() { record("external") }})
	loop.enqueueCommandLocked(loopCommand{kind: loopCommandInternal, fn: func() { record("internal") }})
	loop.externalMu.Unlock()
	loop.wakeAfterIngress()
	releaseFastPath()

	waitContractSignal(t, done, "fast-path internal and external callbacks")

	mu.Lock()
	got := append([]string(nil), order...)
	mu.Unlock()
	if want := []string{"internal", "external"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fast-path order = %v, want %v", got, want)
	}
	if pollEntered.Load() {
		t.Fatal("fast path entered poll tick for task-only internal/external work")
	}

	cancel()
	if err := waitContractValue(t, runDone, "fast-path internal/external Run completion"); err != context.Canceled {
		t.Fatalf("Run after cancel = %v, want %v", err, context.Canceled)
	}
}

func TestFastPathContinuesReentrantExternalSnapshotsWithoutPollTick(t *testing.T) {
	var pressure atomic.Bool
	loop := New(
		WithFastPathMode(FastPathForced),
		WithQueuePressureHandler(func() { pressure.Store(true) }),
	)
	registerLoopCleanupT(t, loop)

	var pollEntered atomic.Bool
	loop.testHooks = &loopTestHooks{PrePollSleep: func() { pollEntered.Store(true) }}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitLoopOwnerTurnT(t, loop)

	var mu sync.Mutex
	var order []string
	done := make(chan struct{})
	record := func(label string) {
		mu.Lock()
		order = append(order, label)
		if len(order) == 2 {
			close(done)
		}
		mu.Unlock()
	}

	if err := loop.Submit(func() {
		record("first")
		if err := loop.Submit(func() { record("second") }); err != nil {
			t.Errorf("reentrant Submit: %v", err)
		}
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	waitContractSignal(t, done, "reentrant fast-path submission")

	mu.Lock()
	got := append([]string(nil), order...)
	mu.Unlock()
	if want := []string{"first", "second"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fast-path reentrant order = %v, want %v", got, want)
	}
	if !pressure.Load() {
		t.Fatal("reentrant submit beyond the external snapshot should signal queue pressure")
	}
	if pollEntered.Load() {
		t.Fatal("fast path entered poll tick for reentrant task-only external work")
	}

	cancel()
	if err := waitContractValue(t, runDone, "reentrant fast-path Run completion"); err != context.Canceled {
		t.Fatalf("Run after cancel = %v, want %v", err, context.Canceled)
	}
}

func TestFastPathContinuesInternalSnapshotsWithoutPollTick(t *testing.T) {
	loop := New(WithFastPathMode(FastPathForced))
	registerLoopCleanupT(t, loop)

	fastPathEntered := make(chan struct{})
	var fastPathOnce sync.Once
	var pollEntered atomic.Bool
	loop.testHooks = &loopTestHooks{
		OnFastPathEntry: func() { fastPathOnce.Do(func() { close(fastPathEntered) }) },
		PrePollSleep:    func() { pollEntered.Store(true) },
	}

	sentinelEntered := make(chan struct{})
	releaseSentinel := make(chan struct{})
	releaseSentinelFn := releaseSignalT(t, releaseSentinel)
	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(runCtx) }()
	waitContractSignal(t, fastPathEntered, "fast-path entry")
	if err := loop.SubmitInternal(func() {
		close(sentinelEntered)
		<-releaseSentinel
	}); err != nil {
		t.Fatalf("sentinel SubmitInternal: %v", err)
	}
	waitContractSignal(t, sentinelEntered, "internal snapshot sentinel")

	markerRan := make(chan struct{})
	if err := loop.SubmitInternal(func() { close(markerRan) }); err != nil {
		t.Fatalf("late SubmitInternal: %v", err)
	}
	releaseSentinelFn()
	waitContractSignal(t, markerRan, "late internal snapshot continuation")
	if pollEntered.Load() {
		t.Fatal("fast-path internal continuation entered a poll tick")
	}

	cancelRun()
	if err := waitContractValue(t, runDone, "fast-path Run completion"); err != context.Canceled {
		t.Fatalf("Run = %v, want %v", err, context.Canceled)
	}
}

func TestFastPathRunsCheckAndCloseBeforeExternalWithoutPollTick(t *testing.T) {
	loop := New(WithFastPathMode(FastPathForced))
	registerLoopCleanupT(t, loop)

	// Hold the owner before check, close, or external work can enter a phase
	// snapshot; an internal owner-turn callback would already be too late.
	var pollEntered atomic.Bool
	hooks := &loopTestHooks{PrePollSleep: func() { pollEntered.Store(true) }}
	fastPathEntered, releaseFastPath := holdFirstFastPathEntryT(t, hooks)
	loop.testHooks = hooks

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	waitContractSignal(t, fastPathEntered, "fast-path entry")

	var mu sync.Mutex
	var order []string
	done := make(chan struct{})
	record := func(label string) {
		mu.Lock()
		order = append(order, label)
		if len(order) == 3 {
			close(done)
		}
		mu.Unlock()
	}

	// Commit all three callbacks under one ingress lock. Separate public calls
	// against a running loop may execute in separate turns, so they cannot prove
	// phase ordering within one fast-path turn.
	loop.externalMu.Lock()
	loop.enqueueCommandLocked(loopCommand{kind: loopCommandExternal, fn: func() { record("external") }})
	loop.enqueueCommandLocked(loopCommand{kind: loopCommandImmediate, fn: func() { record("check") }})
	loop.enqueueCommandLocked(loopCommand{kind: loopCommandClose, fn: func() { record("close") }})
	loop.externalMu.Unlock()
	loop.wakeAfterIngress()
	releaseFastPath()

	waitContractSignal(t, done, "fast-path check, close, and external callbacks")

	mu.Lock()
	got := append([]string(nil), order...)
	mu.Unlock()
	if want := []string{"check", "close", "external"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fast-path phase order = %v, want %v", got, want)
	}
	if pollEntered.Load() {
		t.Fatal("fast path entered poll tick for task-only check/close work")
	}

	cancel()
	if err := waitContractValue(t, runDone, "fast-path phase Run completion"); err != context.Canceled {
		t.Fatalf("Run after cancel = %v, want %v", err, context.Canceled)
	}
}

func TestFastPathTimerCancelCommandPrecedesDueTimer(t *testing.T) {
	loop := New(WithFastPathMode(FastPathForced))
	registerLoopCleanupT(t, loop)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	release := releaseSignalT(t, releaseCallback)
	if err := loop.Submit(func() {
		close(callbackEntered)
		<-releaseCallback
	}); err != nil {
		t.Fatalf("Submit blocker: %v", err)
	}

	waitContractSignal(t, callbackEntered, "blocking fast-path callback entry")

	var fired atomic.Bool
	id, err := loop.ScheduleTimer(0, func() { fired.Store(true) })
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	if err := loop.queueTimerCancel(id); err != nil {
		t.Fatalf("queueTimerCancel: %v", err)
	}

	release()
	if fired.Load() {
		t.Fatal("timer fired before an already-accepted cancellation command applied")
	}

	cancel()
	if err := waitContractValue(t, runDone, "timer-cancel fast-path Run completion"); err != context.Canceled {
		t.Fatalf("Run after cancel = %v, want %v", err, context.Canceled)
	}
}

func TestFastPathTimerUnrefCommandPrecedesDueTimerAutoExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop := New(WithFastPathMode(FastPathForced), WithAutoExit(true))

	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	if err := loop.Submit(func() {
		close(callbackEntered)
		<-releaseCallback
	}); err != nil {
		t.Fatalf("Submit blocker: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	select {
	case <-callbackEntered:
	case <-ctx.Done():
		t.Fatal("timed out waiting for blocking callback to enter")
	}

	var fired atomic.Bool
	id, err := loop.ScheduleTimer(0, func() { fired.Store(true) })
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	if err := loop.queueTimerRefChange(id, false); err != nil {
		t.Fatalf("queueTimerRefChange: %v", err)
	}

	close(releaseCallback)

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Run did not auto-exit after due timer was unrefed before the timer phase")
	}
	if fired.Load() {
		t.Fatal("unrefed due timer fired after it became the loop's only remaining work")
	}
}

func TestFastPathRunAuxAutoExitSkipsUnrefImmediateAfterMicrotask(t *testing.T) {
	loop := New(WithFastPathMode(FastPathForced), WithAutoExit(true))
	registerFDResourceCleanupT(t, loop)
	loop.state.Store(StateRunning)
	loop.loopGoroutineID.Store(goroutineid.Get())
	defer loop.loopGoroutineID.Store(0)

	var microRan atomic.Bool
	var immediateRan atomic.Bool
	if err := loop.ScheduleMicrotask(func() { microRan.Store(true) }); err != nil {
		t.Fatalf("ScheduleMicrotask: %v", err)
	}
	if err := loop.ScheduleImmediateRef(func() { immediateRan.Store(true) }, func() bool { return false }); err != nil {
		t.Fatalf("ScheduleImmediateRef: %v", err)
	}

	loop.runAux()
	if !microRan.Load() {
		t.Fatal("microtask did not run")
	}
	if immediateRan.Load() {
		t.Fatal("unref immediate ran after the last refed microtask drained in fast path")
	}
}

func TestRunFastPathReturnsAfterContextCancelWithContinuousReadyWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loop := New(WithFastPathMode(FastPathForced))
	registerLoopCleanupT(t, loop)

	started := make(chan struct{})
	var startedOnce sync.Once
	terminalSubmit := make(chan error, 1)
	var recur func()
	recur = func() {
		startedOnce.Do(func() { close(started) })
		if err := loop.Submit(recur); err != nil {
			terminalSubmit <- err
		}
	}
	if err := loop.Submit(recur); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("self-resubmitting task did not start")
	}
	cancel()
	select {
	case err := <-runDone:
		if err != context.Canceled {
			t.Fatalf("Run = %v, want %v", err, context.Canceled)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancellation while fast-path work stayed ready")
	}
	if err := waitContractValue(t, terminalSubmit, "recursive Submit terminal rejection"); err != ErrLoopTerminated {
		t.Fatalf("recursive Submit after cancellation = %v, want %v", err, ErrLoopTerminated)
	}
}

func TestNormalTickTimerUnrefCommandPrecedesDueTimerAutoExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop := New(WithAutoExit(true), WithFastPathMode(FastPathDisabled))

	var fired atomic.Bool
	var timerID TimerID
	secondCheckEntered := make(chan struct{})
	releaseSecondCheck := make(chan struct{})
	if err := loop.ScheduleImmediate(func() {
		id, err := loop.ScheduleTimer(0, func() { fired.Store(true) })
		if err != nil {
			t.Errorf("ScheduleTimer from check callback: %v", err)
			return
		}
		timerID = id
		if err := loop.ScheduleImmediate(func() {
			close(secondCheckEntered)
			<-releaseSecondCheck
		}); err != nil {
			t.Errorf("ScheduleImmediate second check: %v", err)
		}
	}); err != nil {
		t.Fatalf("ScheduleImmediate first check: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	select {
	case <-secondCheckEntered:
	case <-ctx.Done():
		t.Fatal("timed out waiting for second check callback to enter")
	}

	if err := loop.queueTimerRefChange(timerID, false); err != nil {
		t.Fatalf("queueTimerRefChange: %v", err)
	}

	close(releaseSecondCheck)

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Run did not auto-exit after normal tick applied unref before timers")
	}
	if fired.Load() {
		t.Fatal("normal tick fired a due timer after an accepted unref command made it unrefed-only work")
	}
}

func TestNormalTimerPhaseDrainsAcceptedLifecycleCommandAtEntry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	loop := New(WithAutoExit(true), WithFastPathMode(FastPathDisabled))

	var fired atomic.Bool
	var timerID TimerID
	var sawIneligibleTimerPhase bool
	var cancelStarted bool
	cancelDone := make(chan error, 1)
	loop.testHooks = &loopTestHooks{BeforeRunTimers: func() {
		if timerID == 0 || !sawIneligibleTimerPhase {
			if timerID != 0 {
				sawIneligibleTimerPhase = true
			}
			return
		}
		if !cancelStarted {
			cancelStarted = true
			cancelDone <- loop.queueTimerCancel(timerID)
		}
	}}

	if err := loop.ScheduleImmediate(func() {
		id, err := loop.ScheduleTimer(0, func() { fired.Store(true) })
		if err != nil {
			t.Errorf("ScheduleTimer from check callback: %v", err)
			return
		}
		timerID = id
	}); err != nil {
		t.Fatalf("ScheduleImmediate: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	select {
	case err := <-cancelDone:
		if err != nil {
			t.Fatalf("CancelTimer: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for timer phase cancellation result")
	}

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Run did not auto-exit after timer phase cancellation")
	}
	if fired.Load() {
		t.Fatal("timer phase fired a due timer before draining an already-accepted cancellation command")
	}
}

func TestLoopThreadImmediateAndCloseUseOwnerLocalQueues(t *testing.T) {
	loop := New(WithFastPathMode(FastPathForced))
	registerLoopCleanupT(t, loop)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	checked := make(chan struct{})
	if err := loop.Submit(func() {
		if err := loop.ScheduleImmediate(func() {}); err != nil {
			t.Errorf("ScheduleImmediate from loop thread: %v", err)
		}
		if err := loop.ScheduleCloseCallback(func() {}); err != nil {
			t.Errorf("ScheduleCloseCallback from loop thread: %v", err)
		}
		loop.externalMu.Lock()
		cmdLen := loop.commands.Len()
		loop.externalMu.Unlock()
		if cmdLen != 0 {
			t.Errorf("loop-thread check/close scheduling enqueued %d external commands, want owner-local append", cmdLen)
		}
		if got := loop.ownerCheckCount.Load(); got != 1 {
			t.Errorf("ownerCheckCount = %d, want 1", got)
		}
		if got := loop.ownerCloseCount.Load(); got != 1 {
			t.Errorf("ownerCloseCount = %d, want 1", got)
		}
		close(checked)
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	waitContractSignal(t, checked, "loop-thread owner-local scheduling check")

	cancel()
	if err := waitContractValue(t, runDone, "owner-local queue Run completion"); err != context.Canceled {
		t.Fatalf("Run after cancel = %v, want %v", err, context.Canceled)
	}
}

func TestMixedExternalAndOwnerImmediateFIFO(t *testing.T) {
	testMixedImmediateFIFO(t, true, []string{"external", "owner"})
	testMixedImmediateFIFO(t, false, []string{"owner", "external"})
}

func TestMixedExternalAndOwnerCloseFIFO(t *testing.T) {
	testMixedCloseFIFO(t, true, []string{"external", "owner"})
	testMixedCloseFIFO(t, false, []string{"owner", "external"})
}

func testMixedImmediateFIFO(t *testing.T, externalFirst bool, want []string) {
	t.Helper()
	loop := New(WithFastPathMode(FastPathForced))
	registerLoopCleanupT(t, loop)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	var mu sync.Mutex
	var order []string
	done := make(chan struct{})
	record := func(label string) {
		mu.Lock()
		order = append(order, label)
		if len(order) == 2 {
			close(done)
		}
		mu.Unlock()
	}

	if err := loop.Submit(func() {
		scheduleExternal := func() {
			errCh := make(chan error, 1)
			go func() { errCh <- loop.ScheduleImmediate(func() { record("external") }) }()
			select {
			case err := <-errCh:
				if err != nil {
					t.Errorf("external ScheduleImmediate: %v", err)
				}
			case <-time.After(time.Second):
				t.Errorf("timed out accepting external ScheduleImmediate")
			}
		}
		scheduleOwner := func() {
			if err := loop.ScheduleImmediate(func() { record("owner") }); err != nil {
				t.Errorf("owner ScheduleImmediate: %v", err)
			}
		}
		if externalFirst {
			scheduleExternal()
			scheduleOwner()
		} else {
			scheduleOwner()
			scheduleExternal()
		}
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	waitContractSignal(t, done, "mixed external/owner immediate callbacks")

	mu.Lock()
	got := append([]string(nil), order...)
	mu.Unlock()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mixed immediate order externalFirst=%v got %v, want %v", externalFirst, got, want)
	}

	cancel()
	if err := waitContractValue(t, runDone, "mixed immediate Run completion"); err != context.Canceled {
		t.Fatalf("Run after cancel = %v, want %v", err, context.Canceled)
	}
}

func testMixedCloseFIFO(t *testing.T, externalFirst bool, want []string) {
	t.Helper()
	loop := New(WithFastPathMode(FastPathForced))
	registerLoopCleanupT(t, loop)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	var mu sync.Mutex
	var order []string
	done := make(chan struct{})
	record := func(label string) {
		mu.Lock()
		order = append(order, label)
		if len(order) == 2 {
			close(done)
		}
		mu.Unlock()
	}

	if err := loop.Submit(func() {
		scheduleExternal := func() {
			errCh := make(chan error, 1)
			go func() { errCh <- loop.ScheduleCloseCallback(func() { record("external") }) }()
			select {
			case err := <-errCh:
				if err != nil {
					t.Errorf("external ScheduleCloseCallback: %v", err)
				}
			case <-time.After(time.Second):
				t.Errorf("timed out accepting external ScheduleCloseCallback")
			}
		}
		scheduleOwner := func() {
			if err := loop.ScheduleCloseCallback(func() { record("owner") }); err != nil {
				t.Errorf("owner ScheduleCloseCallback: %v", err)
			}
		}
		if externalFirst {
			scheduleExternal()
			scheduleOwner()
		} else {
			scheduleOwner()
			scheduleExternal()
		}
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	waitContractSignal(t, done, "mixed external/owner close callbacks")

	mu.Lock()
	got := append([]string(nil), order...)
	mu.Unlock()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mixed close order externalFirst=%v got %v, want %v", externalFirst, got, want)
	}

	cancel()
	if err := waitContractValue(t, runDone, "mixed close Run completion"); err != context.Canceled {
		t.Fatalf("Run after cancel = %v, want %v", err, context.Canceled)
	}
}
