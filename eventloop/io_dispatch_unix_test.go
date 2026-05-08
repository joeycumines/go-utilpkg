//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/logiface"
)

func TestIOCallbackObservesRunningState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop := New()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	registerFileCleanupT(t, r, w)
	registerLoopCleanupT(t, loop)
	fd := int(r.Fd())

	stateSeen := make(chan LoopState, 1)
	unregisterResult := make(chan error, 1)
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {
		stateSeen <- loop.State()
		unregisterResult <- loop.UnregisterFD(fd)
	}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	if _, err := w.Write([]byte{1}); err != nil {
		t.Fatalf("write pipe: %v", err)
	}

	select {
	case state := <-stateSeen:
		if state != StateRunning {
			t.Fatalf("I/O callback observed loop state %v, want %v", state, StateRunning)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for I/O callback")
	}
	if err := waitContractValue(t, unregisterResult, "running-state callback self-unregistration"); err != nil {
		t.Fatalf("UnregisterFD from I/O callback: %v", err)
	}

	if err := loop.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "running-state Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestIOCallbackSkippedWhenShutdownWinsPollWake(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop := New()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	registerFileCleanupT(t, r, w)
	registerLoopCleanupT(t, loop)

	var callbackRan atomic.Bool
	shutdownDone := make(chan error, 1)
	terminalPublished := make(chan struct{})
	var hookOnce sync.Once
	loop.testHooks = &loopTestHooks{
		AfterShutdownStateTerminating: func() { close(terminalPublished) },
		PrePollAwake: func() {
			hookOnce.Do(func() {
				go func() { shutdownDone <- loop.Shutdown(context.Background()) }()
				waitContractSignal(t, terminalPublished, "Shutdown terminal-state publication at poll wake")
			})
		},
	}

	fd := int(r.Fd())
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {
		callbackRan.Store(true)
	}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	if _, err := w.Write([]byte{1}); err != nil {
		t.Fatalf("write pipe: %v", err)
	}

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for Shutdown")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}
	if callbackRan.Load() {
		t.Fatal("I/O callback ran after shutdown won the poll wake race")
	}
}

func TestModifyFDZeroSuppressesConvertedReadiness(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop := New()
	claimed := make(chan struct{})
	release := make(chan struct{})
	var hookOnce sync.Once
	var fd int
	loop.testHooks = &loopTestHooks{
		AfterReadyEventDispatchClaim: func(claimedFD int) {
			if claimedFD != fd {
				return
			}
			hookOnce.Do(func() {
				close(claimed)
				<-release
			})
		},
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	registerFileCleanupT(t, r, w)
	registerLoopCleanupT(t, loop)
	releaseDispatch := releaseSignalT(t, release)
	fd = int(r.Fd())
	var callbackCount atomic.Int32
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {
		callbackCount.Add(1)
	}); err != nil {
		t.Fatal(err)
	}
	// Remove the registration wake token before Run so the hook can only observe
	// the readiness written below.
	loop.drainWakeUpPipe()

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()
	if _, err := w.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-claimed:
	case <-ctx.Done():
		t.Fatal("converted readiness did not reach callback-start claim")
	}
	if err := loop.ModifyFD(fd, 0); err != nil {
		t.Fatalf("ModifyFD(fd, 0): %v", err)
	}
	afterDispatch := make(chan struct{})
	if err := loop.Submit(func() { close(afterDispatch) }); err != nil {
		t.Fatal(err)
	}
	releaseDispatch()
	select {
	case <-afterDispatch:
	case <-ctx.Done():
		t.Fatal("loop did not progress after converted readiness was released")
	}
	if got := callbackCount.Load(); got != 0 {
		t.Fatalf("callbacks after committed zero interest = %d, want 0", got)
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("Run did not return")
	}
}

func TestIOCallbackMicrotaskCheckpointPrecedesNextReadyFD(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop := New()
	r1, w1, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe 1: %v", err)
	}
	registerFileCleanupT(t, r1, w1)
	r2, w2, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe 2: %v", err)
	}
	registerFileCleanupT(t, r2, w2)
	registerLoopCleanupT(t, loop)

	var (
		mu                sync.Mutex
		once              sync.Once
		order             []string
		done              = make(chan []string, 1)
		unregisterResults = make(chan error, 2)
	)
	record := func(value string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, value)
		if len(order) == 4 {
			copyOrder := append([]string(nil), order...)
			once.Do(func() { done <- copyOrder })
		}
	}

	fd1 := int(r1.Fd())
	fd2 := int(r2.Fd())
	if err := loop.RegisterFD(fd1, EventRead, func(IOEvents) {
		record("io1")
		if err := loop.ScheduleMicrotask(func() { record("micro1") }); err != nil {
			record("microerr1")
		}
		unregisterResults <- loop.UnregisterFD(fd1)
	}); err != nil {
		t.Fatalf("RegisterFD 1: %v", err)
	}
	if err := loop.RegisterFD(fd2, EventRead, func(IOEvents) {
		record("io2")
		if err := loop.ScheduleMicrotask(func() { record("micro2") }); err != nil {
			record("microerr2")
		}
		unregisterResults <- loop.UnregisterFD(fd2)
	}); err != nil {
		t.Fatalf("RegisterFD 2: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	if _, err := w1.Write([]byte{1}); err != nil {
		t.Fatalf("write pipe 1: %v", err)
	}
	if _, err := w2.Write([]byte{1}); err != nil {
		t.Fatalf("write pipe 2: %v", err)
	}

	var got []string
	select {
	case got = <-done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for I/O microtask order")
	}
	for range 2 {
		if err := waitContractValue(t, unregisterResults, "I/O callback self-unregistration"); err != nil {
			t.Fatalf("UnregisterFD from I/O callback: %v", err)
		}
	}

	if err := loop.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "I/O microtask-checkpoint Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(got) != 4 {
		t.Fatalf("order length = %d, want 4: %v", len(got), got)
	}
	for i := 0; i < len(got); i += 2 {
		switch got[i] {
		case "io1":
			if got[i+1] != "micro1" {
				t.Fatalf("order = %v, want micro1 immediately after io1", got)
			}
		case "io2":
			if got[i+1] != "micro2" {
				t.Fatalf("order = %v, want micro2 immediately after io2", got)
			}
		default:
			t.Fatalf("order = %v, want I/O callback at index %d", got, i)
		}
	}
}

func TestStaleReadyEventSkippedAfterFDUnregisterAndReregister(t *testing.T) {
	loop := New(WithFastPathMode(FastPathDisabled))
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	registerFileCleanupT(t, r, w)
	registerFDResourceCleanupT(t, loop)

	fd := int(r.Fd())
	var staleCalled atomic.Bool
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) { staleCalled.Store(true) }); err != nil {
		t.Fatalf("RegisterFD stale callback: %v", err)
	}
	if _, err := w.Write([]byte{1}); err != nil {
		t.Fatalf("write pipe: %v", err)
	}
	if n, err := loop.poller.PollIO(0); err != nil {
		t.Fatalf("PollIO: %v", err)
	} else if n == 0 {
		t.Fatal("PollIO returned no ready events")
	}
	ready := append([]pollEvent(nil), loop.poller.readyEventsSnapshot()...)
	if err := loop.UnregisterFD(fd); err != nil {
		t.Fatalf("UnregisterFD: %v", err)
	}
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) { t.Fatal("new callback should not receive stale event") }); err != nil {
		t.Fatalf("RegisterFD new callback: %v", err)
	}

	loop.dispatchPollEvents(ready)
	if staleCalled.Load() {
		t.Fatal("stale ready event dispatched after fd was unregistered and re-registered")
	}
}

func TestUnregisterFDWaitsForClaimedReadyEventDispatchStart(t *testing.T) {
	loop := New(WithFastPathMode(FastPathDisabled))
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	registerFileCleanupT(t, r, w)
	registerFDResourceCleanupT(t, loop)

	fd := int(r.Fd())
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}
	if !loop.poller.appendReadyEvent(fd, EventRead) {
		t.Fatal("appendReadyEvent returned false for registered fd")
	}
	ready := append([]pollEvent(nil), loop.poller.readyEventsSnapshot()...)
	if len(ready) != 1 {
		t.Fatalf("ready event count = %d, want 1", len(ready))
	}
	callback, events, dispatch, ok := loop.poller.beginReadyEventDispatch(ready[0])
	if !ok || callback == nil || dispatch == nil {
		t.Fatal("beginReadyEventDispatch failed for registered ready event")
	}
	if events != EventRead {
		t.Fatalf("claimed events = %v, want %v", events, EventRead)
	}

	unregisterAtWait := make(chan struct{})
	loop.poller.beforeDispatchWait = func() { close(unregisterAtWait) }
	unregisterDone := make(chan error, 1)
	go func() { unregisterDone <- loop.UnregisterFD(fd) }()
	waitContractSignal(t, unregisterAtWait, "UnregisterFD pending-dispatch join")

	dispatch.dispatchStarted()
	if err := waitContractValue(t, unregisterDone, "UnregisterFD after dispatch start"); err != nil {
		t.Fatalf("UnregisterFD after dispatch start: %v", err)
	}
}

func TestCloseUnregisterPendingDispatchDoesNotDeadlock(t *testing.T) {
	loop := New(WithFastPathMode(FastPathDisabled))
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	registerFileCleanupT(t, r, w)
	registerLoopCleanupT(t, loop)

	fd := int(r.Fd())
	dispatchClaimed := make(chan struct{})
	releaseDispatch := make(chan struct{})
	releaseDispatchFn := releaseSignalT(t, releaseDispatch)
	closeReachedLifecycleLock := make(chan struct{})
	var dispatchOnce sync.Once
	var closeOnce sync.Once
	var callbackCount atomic.Int32
	loop.testHooks = &loopTestHooks{
		AfterReadyEventDispatchClaim: func(claimedFD int) {
			if claimedFD != fd {
				return
			}
			dispatchOnce.Do(func() { close(dispatchClaimed) })
			<-releaseDispatch
		},
		BeforeCloseLifecycleLock: func() {
			closeOnce.Do(func() { close(closeReachedLifecycleLock) })
		},
	}

	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) { callbackCount.Add(1) }); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitLoopOwnerTurnT(t, loop)
	if _, err := w.Write([]byte{1}); err != nil {
		t.Fatalf("write pipe: %v", err)
	}
	select {
	case <-dispatchClaimed:
	case <-time.After(5 * time.Second):
		t.Fatal("I/O dispatch did not claim its pending callback start")
	}

	unregisterAtWait := make(chan struct{})
	var unregisterWaitOnce sync.Once
	loop.poller.beforeDispatchWait = func() {
		unregisterWaitOnce.Do(func() { close(unregisterAtWait) })
	}
	unregisterDone := make(chan error, 1)
	go func() { unregisterDone <- loop.UnregisterFD(fd) }()
	waitContractSignal(t, unregisterAtWait, "UnregisterFD pending-dispatch join")
	select {
	case err := <-unregisterDone:
		t.Fatalf("UnregisterFD returned before callback admission decided: %v", err)
	default:
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	select {
	case <-closeReachedLifecycleLock:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not reach lifecycle ownership")
	}
	loop.callbackGateMu.Lock()
	gateMode := loop.callbackGateMode
	loop.callbackGateMu.Unlock()
	if gateMode != callbackGateOpen {
		t.Fatalf("callback admission mode before Close owns liveness = %v, want open", gateMode)
	}
	select {
	case err := <-closeDone:
		t.Fatalf("Close returned before pending callback-start claim resolved: %v", err)
	default:
	}

	releaseDispatchFn()
	select {
	case err := <-unregisterDone:
		if err != nil {
			t.Fatalf("UnregisterFD: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("UnregisterFD did not return after pending callback start was suppressed")
	}
	if got := callbackCount.Load(); got != 0 {
		t.Fatalf("callbacks after UnregisterFD retired pending start = %d, want 0", got)
	}

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return after pending callback start was suppressed")
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not exit after Close")
	}
}

func TestIOCallbackPanicRecoveryKeepsLoopAlive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	loop := New()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	registerFileCleanupT(t, r, w)
	registerLoopCleanupT(t, loop)

	panicked := make(chan struct{}, 1)
	unregisterResult := make(chan error, 1)
	fd := int(r.Fd())
	if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {
		unregisterResult <- loop.UnregisterFD(fd)
		panicked <- struct{}{}
		panic("io callback panic")
	}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(ctx) }()

	if _, err := w.Write([]byte{1}); err != nil {
		t.Fatalf("write pipe: %v", err)
	}

	select {
	case <-panicked:
	case <-ctx.Done():
		t.Fatal("timed out waiting for panicking I/O callback")
	}
	if err := waitContractValue(t, unregisterResult, "panicking I/O callback self-unregistration"); err != nil {
		t.Fatalf("UnregisterFD from panicking callback: %v", err)
	}

	survived := make(chan struct{}, 1)
	if err := loop.Submit(func() { survived <- struct{}{} }); err != nil {
		t.Fatalf("Submit after I/O panic: %v", err)
	}
	select {
	case <-survived:
	case <-ctx.Done():
		t.Fatal("loop did not execute work after I/O callback panic")
	}

	if err := loop.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := waitContractValue(t, runDone, "I/O panic-recovery Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
}
func TestInternalPollPanicLoggerCannotEscape(t *testing.T) {
	var writes atomic.Int32
	panicLogger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](&testEventFactory{}),
		logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(*testEvent) error {
			writes.Add(1)
			panic("injected log writer panic")
		})),
	)
	loop := New(WithLogger(panicLogger.Logger()))
	registerLoopCleanupT(t, loop)
	loop.executePollInternal(func() { panic("injected internal callback panic") })
	if got := writes.Load(); got != 1 {
		t.Fatalf("log writes = %d, want 1", got)
	}
}

func TestInternalWakeDispatchExcludedFromUserMetrics(t *testing.T) {
	loop := New(WithMetrics(true), WithFastPathMode(FastPathDisabled))
	registerLoopCleanupT(t, loop)
	if err := loop.ensurePoller(); err != nil {
		t.Fatalf("initialize native poller: %v", err)
	}
	loop.poller.fdMu.RLock()
	info, active := loop.poller.fdInfoLocked(loop.wakePipe)
	loop.poller.fdMu.RUnlock()
	if !active || !info.internal {
		t.Fatal("internal wake registration is unavailable")
	}
	// Consume setup wake state so the event below must originate from this
	// test's production physical submission and native result conversion.
	loop.drainWakeUpPipe()
	var drained atomic.Int32
	loop.testHooks = &loopTestHooks{AfterWakeDrain: func() { drained.Add(1) }}
	latencyBefore := loop.metrics.latency.count.Load()
	var tpsBefore int64
	for i := range loop.metrics.tps.buckets {
		tpsBefore += loop.metrics.tps.buckets[i].Load()
	}
	if err := loop.submitPendingWakeup(); err != nil {
		t.Fatal(err)
	}
	if got, err := loop.poller.PollIO(0); err != nil {
		t.Fatal(err)
	} else if got != 1 {
		t.Fatalf("native ready events = %d, want 1", got)
	}
	ready := append([]pollEvent(nil), loop.poller.readyEventsSnapshot()...)
	loop.poller.clearReadyEvents()
	if len(ready) != 1 {
		t.Fatalf("converted ready events = %d, want 1", len(ready))
	}
	if ready[0].fd != loop.wakePipe || ready[0].generation != info.generation || !ready[0].internal {
		t.Fatalf("converted wake event = %+v, want fd=%d generation=%d internal=true", ready[0], loop.wakePipe, info.generation)
	}
	loop.dispatchPollEvents(ready)
	if got := drained.Load(); got != 1 {
		t.Fatalf("internal wake drain calls = %d, want 1", got)
	}
	if got := loop.metrics.latency.count.Load(); got != latencyBefore {
		t.Fatalf("latency samples after internal wake = %d, want %d", got, latencyBefore)
	}
	var tpsAfter int64
	for i := range loop.metrics.tps.buckets {
		tpsAfter += loop.metrics.tps.buckets[i].Load()
	}
	if tpsAfter != tpsBefore {
		t.Fatalf("TPS events after internal wake = %d, want %d", tpsAfter, tpsBefore)
	}
}
