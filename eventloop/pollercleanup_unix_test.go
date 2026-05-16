//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package eventloop

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/joeycumines/logiface"
	"golang.org/x/sys/unix"
)

func TestLoopLifecycleClosesWakeDescriptors(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	if err := loop.ensurePoller(); err != nil {
		t.Fatalf("ensurePoller: %v", err)
	}
	readFD, writeFD := loop.wakePipe, loop.wakePipeWrite
	if readFD < 0 || writeFD < 0 {
		t.Fatalf("published wake descriptors = (%d, %d), want nonnegative", readFD, writeFD)
	}
	if err := loop.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	requireDescriptorClosed(t, readFD)
	if writeFD != readFD {
		requireDescriptorClosed(t, writeFD)
	}
}

func TestLoopUnregisterReportsOwnedDescriptorCloseFailureAfterReleasingLiveness(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	if err := loop.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	loop.poller.fdMu.RLock()
	info, active := loop.poller.fdInfoLocked(pipeFDs[0])
	loop.poller.fdMu.RUnlock()
	if !active || !info.ownsPollFD {
		t.Fatal("owned registration descriptor is unavailable")
	}
	sentinel := errors.New("injected owned descriptor close failure")
	ownedCloseCalls := 0
	loop.poller.descriptorClose = func(fd int) error {
		if fd == info.pollFD {
			ownedCloseCalls++
		}
		if err := unix.Close(fd); err != nil {
			return err
		}
		if fd == info.pollFD {
			return sentinel
		}
		return nil
	}
	err = loop.UnregisterFD(pipeFDs[0])
	if !errors.Is(err, sentinel) {
		t.Fatalf("UnregisterFD error = %v, want injected close failure", err)
	}
	var unregisterErr *FDUnregisterError
	if !errors.As(err, &unregisterErr) || !unregisterErr.Released() {
		t.Fatalf("UnregisterFD error = %v, want released ownership", err)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount = %d, want 0 after retired ownership", got)
	}
	if loop.poller.userFDRegistered(pipeFDs[0]) {
		t.Fatal("registration remains active after owned descriptor cleanup failure")
	}
	if ownedCloseCalls != 1 {
		t.Fatalf("owned descriptor close calls = %d, want 1", ownedCloseCalls)
	}
	requireDescriptorClosed(t, info.pollFD)
}

func TestUnregisterFDUnderflowLoggerRunsAfterResourceLocks(t *testing.T) {
	closeResult := make(chan error, 1)
	var loop *Loop
	logger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](&testEventFactory{}),
		logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(*testEvent) error {
			closeResult <- loop.Close()
			return nil
		})),
	)
	var err error
	loop, err = New(WithLogger(logger.Logger()))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	if err := loop.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}

	// Force the defensive invariant branch. Its logger may re-enter lifecycle;
	// both resource locks must already be released or Close cannot finish.
	loop.userIOFDCount.Store(0)
	unregisterDone := make(chan error, 1)
	go func() { unregisterDone <- loop.UnregisterFD(pipeFDs[0]) }()
	if err := waitContractValue(t, closeResult, "underflow logger Close"); err != nil {
		t.Fatalf("logger Close: %v", err)
	}
	if err := waitContractValue(t, unregisterDone, "UnregisterFD after underflow logger"); err != nil {
		t.Fatalf("UnregisterFD: %v", err)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount = %d, want 0", got)
	}
}

func TestLoopLifecycleReportsPollerDescriptorCloseFailure(t *testing.T) {
	operations := []struct {
		name string
		run  func(*Loop) error
	}{
		{name: "Close", run: func(loop *Loop) error { return loop.Close() }},
		{name: "Shutdown", run: func(loop *Loop) error { return loop.Shutdown(context.Background()) }},
	}
	loggerFaults := []struct {
		name string
		run  func()
	}{
		{name: "panic", run: func() { panic("injected log writer panic") }},
		{name: "runtime_Goexit", run: runtime.Goexit},
	}
	for _, operation := range operations {
		for _, loggerFault := range loggerFaults {
			t.Run(operation.name+"/"+loggerFault.name, func(t *testing.T) {
				panicLogger := logiface.New[*testEvent](
					logiface.WithEventFactory[*testEvent](&testEventFactory{}),
					logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(*testEvent) error {
						loggerFault.run()
						return nil
					})),
				)
				loop, err := New(WithLogger(panicLogger.Logger()))
				if err != nil {
					t.Fatal(err)
				}
				var pipeFDs [2]int
				if err := unix.Pipe(pipeFDs[:]); err != nil {
					t.Fatal(err)
				}
				registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
				if err := loop.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); err != nil {
					t.Fatal(err)
				}
				loop.poller.fdMu.RLock()
				info, active := loop.poller.fdInfoLocked(pipeFDs[0])
				loop.poller.fdMu.RUnlock()
				if !active || !info.ownsPollFD {
					t.Fatal("owned registration descriptor is unavailable")
				}
				sentinel := errors.New("injected terminal descriptor close failure")
				ownedCloseCalls := 0
				loop.poller.descriptorClose = func(fd int) error {
					if fd == info.pollFD {
						ownedCloseCalls++
					}
					if err := unix.Close(fd); err != nil {
						return err
					}
					if fd == info.pollFD {
						return sentinel
					}
					return nil
				}
				operationDone := make(chan error, 1)
				go func() { operationDone <- operation.run(loop) }()
				err = waitContractValue(t, operationDone, "lifecycle operation after logger failure")
				if !errors.Is(err, sentinel) {
					t.Fatalf("lifecycle error = %v, want injected close failure", err)
				}
				if got := loop.userIOFDCount.Load(); got != 0 {
					t.Fatalf("userIOFDCount = %d, want 0 after terminal cleanup", got)
				}
				if ownedCloseCalls != 1 {
					t.Fatalf("owned descriptor close calls = %d, want 1", ownedCloseCalls)
				}
				requireDescriptorClosed(t, info.pollFD)
			})
		}
	}
}

func TestTerminalCompletionLoggerLifecycleReentry(t *testing.T) {
	tests := []struct {
		name        string
		winner      func(*Loop) error
		reenter     func(*Loop) error
		wantReentry error
	}{
		{
			name:        "graceful_Shutdown",
			winner:      func(loop *Loop) error { return loop.Shutdown(context.Background()) },
			reenter:     func(loop *Loop) error { return loop.Shutdown(context.Background()) },
			wantReentry: nil,
		},
		{
			name:        "graceful_Close",
			winner:      func(loop *Loop) error { return loop.Shutdown(context.Background()) },
			reenter:     func(loop *Loop) error { return loop.Close() },
			wantReentry: ErrLoopTerminated,
		},
		{
			name:        "immediate_Shutdown",
			winner:      func(loop *Loop) error { return loop.Close() },
			reenter:     func(loop *Loop) error { return loop.Shutdown(context.Background()) },
			wantReentry: ErrLoopTerminated,
		},
		{
			name:        "immediate_Close",
			winner:      func(loop *Loop) error { return loop.Close() },
			reenter:     func(loop *Loop) error { return loop.Close() },
			wantReentry: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ownerObserved := make(chan bool, 1)
			reentryResult := make(chan error, 1)
			var writerCalls atomic.Int32
			var loop *Loop
			logger := logiface.New[*testEvent](
				logiface.WithEventFactory[*testEvent](&testEventFactory{}),
				logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(*testEvent) error {
					if writerCalls.Add(1) == 1 {
						ownerObserved <- loop.isTerminalCompletionOwner()
						reentryResult <- test.reenter(loop)
					}
					return nil
				})),
			)
			var err error
			loop, err = New(WithLogger(logger.Logger()))
			if err != nil {
				t.Fatal(err)
			}

			var pipeFDs [2]int
			if err := unix.Pipe(pipeFDs[:]); err != nil {
				t.Fatal(err)
			}
			registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
			if err := loop.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); err != nil {
				t.Fatal(err)
			}
			loop.poller.fdMu.RLock()
			info, active := loop.poller.fdInfoLocked(pipeFDs[0])
			loop.poller.fdMu.RUnlock()
			if !active || !info.ownsPollFD {
				t.Fatal("owned registration descriptor is unavailable")
			}

			sentinel := errors.New("injected terminal descriptor close failure")
			loop.poller.descriptorClose = func(fd int) error {
				if err := unix.Close(fd); err != nil {
					return err
				}
				if fd == info.pollFD {
					return sentinel
				}
				return nil
			}
			var terminalJoins atomic.Int32
			loop.testHooks = &loopTestHooks{
				BeforeTerminalJoin: func() { terminalJoins.Add(1) },
			}

			winnerDone := make(chan error, 1)
			go func() { winnerDone <- test.winner(loop) }()
			if !waitContractValue(t, ownerObserved, "logger completion ownership") {
				t.Fatal("descriptor cleanup logger did not observe completion ownership")
			}
			got := waitContractValue(t, reentryResult, "logger lifecycle reentry")
			if test.wantReentry == nil {
				if got != nil {
					t.Fatalf("logger lifecycle reentry = %v, want nil", got)
				}
			} else if !errors.Is(got, test.wantReentry) {
				t.Fatalf("logger lifecycle reentry = %v, want %v", got, test.wantReentry)
			}
			if err := waitContractValue(t, winnerDone, "winning terminal operation"); !errors.Is(err, sentinel) {
				t.Fatalf("winning terminal operation = %v, want injected close failure", err)
			}
			if got := writerCalls.Load(); got != 1 {
				t.Fatalf("descriptor cleanup logger calls = %d, want 1", got)
			}
			if got := terminalJoins.Load(); got != 0 {
				t.Fatalf("logger lifecycle reentry terminal joins = %d, want 0", got)
			}
			if owner := loop.terminalCompletionOwner.Load(); owner != 0 {
				t.Fatalf("terminal completion owner after logger return = %d, want 0", owner)
			}
			requireDescriptorClosed(t, info.pollFD)
		})
	}
}

func TestLoopOwnerCleanupLoggerShutdownAcknowledgesGracefulMode(t *testing.T) {
	type cleanupObservation struct {
		message         string
		loopOwner       bool
		completionOwner bool
		state           LoopState
		draining        bool
		immediate       bool
		completed       bool
		shutdownErr     error
	}
	observed := make(chan cleanupObservation, 1)
	var writerCalls atomic.Int32
	var loop *Loop
	logger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](&testEventFactory{}),
		logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(event *testEvent) error {
			if writerCalls.Add(1) == 1 {
				observation := cleanupObservation{
					message:         event.message,
					loopOwner:       loop.isLoopThread(),
					completionOwner: loop.isTerminalCompletionOwner(),
					state:           loop.state.Load(),
					draining:        loop.terminalDraining.Load(),
					immediate:       loop.immediateCloseWon(),
					completed:       loop.terminalCompletionPublished(),
				}
				observation.shutdownErr = loop.Shutdown(context.Background())
				observed <- observation
			}
			return nil
		})),
	)
	var err error
	loop, err = New(WithLogger(logger.Logger()))
	if err != nil {
		t.Fatal(err)
	}

	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	if err := loop.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	loop.poller.fdMu.RLock()
	info, active := loop.poller.fdInfoLocked(pipeFDs[0])
	loop.poller.fdMu.RUnlock()
	if !active || !info.ownsPollFD {
		t.Fatal("owned registration descriptor is unavailable")
	}

	sentinel := errors.New("injected running graceful descriptor close failure")
	loop.poller.descriptorClose = func(fd int) error {
		if err := unix.Close(fd); err != nil {
			return err
		}
		if fd == info.pollFD {
			return sentinel
		}
		return nil
	}
	var terminalJoins atomic.Int32
	loop.testHooks = &loopTestHooks{
		BeforeTerminalJoin: func() { terminalJoins.Add(1) },
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitLoopOwnerTurnT(t, loop)
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(context.Background()) }()

	observation := waitContractValue(t, observed, "running graceful cleanup logger observation")
	if observation.message != "eventloop: descriptor cleanup failed" {
		t.Fatalf("cleanup logger message = %q, want descriptor cleanup diagnostic", observation.message)
	}
	if !observation.loopOwner {
		t.Fatal("running graceful cleanup logger did not observe loop ownership")
	}
	if observation.completionOwner {
		t.Fatal("running graceful cleanup logger unexpectedly observed finisher ownership")
	}
	if observation.state != StateTerminated || observation.draining || observation.immediate || observation.completed {
		t.Fatalf("cleanup logger lifecycle = (state %v, draining %v, immediate %v, completed %v), want (StateTerminated, false, false, false)", observation.state, observation.draining, observation.immediate, observation.completed)
	}
	if observation.shutdownErr != nil {
		t.Fatalf("cleanup logger Shutdown = %v, want nil", observation.shutdownErr)
	}
	if err := waitContractValue(t, runDone, "Run after graceful cleanup failure"); !errors.Is(err, sentinel) {
		t.Fatalf("Run = %v, want injected close failure", err)
	}
	if err := waitContractValue(t, shutdownDone, "external Shutdown after cleanup failure"); !errors.Is(err, sentinel) {
		t.Fatalf("Shutdown = %v, want injected close failure", err)
	}
	if got := writerCalls.Load(); got != 1 {
		t.Fatalf("descriptor cleanup logger calls = %d, want 1", got)
	}
	if got := terminalJoins.Load(); got != 0 {
		t.Fatalf("cleanup logger Shutdown terminal joins = %d, want 0", got)
	}
	requireDescriptorClosed(t, info.pollFD)
}

func TestLoopOwnerDescriptorCleanupLoggerRetiresFallbackWorker(t *testing.T) {
	const rejectionReason = "descriptor cleanup logger rejection"
	const rejectionDiagnostic = "eventloop: unhandled rejection after loop termination (fallback callback disabled)"

	messages := make(chan string, 2)
	var writerCalls atomic.Int32
	var reject RejectFunc
	logger := logiface.New[*testEvent](
		logiface.WithEventFactory[*testEvent](&testEventFactory{}),
		logiface.WithWriter[*testEvent](logiface.NewWriterFunc(func(event *testEvent) error {
			messages <- event.message
			if writerCalls.Add(1) == 1 {
				reject(rejectionReason)
			}
			return nil
		})),
	)
	loop, err := New(WithLogger(logger.Logger()))
	if err != nil {
		t.Fatal(err)
	}
	registerLoopCleanupT(t, loop)
	js, err := NewJS(loop, WithUnhandledRejectionFallback(UnhandledRejectionFallbackDisabled))
	if err != nil {
		t.Fatal(err)
	}
	_, _, reject = js.NewChainedPromise()

	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	if err := loop.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	loop.poller.fdMu.RLock()
	info, active := loop.poller.fdInfoLocked(pipeFDs[0])
	loop.poller.fdMu.RUnlock()
	if !active || !info.ownsPollFD {
		t.Fatal("owned registration descriptor is unavailable")
	}

	sentinel := errors.New("injected descriptor cleanup worker-retirement failure")
	loop.poller.descriptorClose = func(fd int) error {
		if err := unix.Close(fd); err != nil {
			return err
		}
		if fd == info.pollFD {
			return sentinel
		}
		return nil
	}
	runStarted := make(chan struct{})
	loop.testHooks = &loopTestHooks{
		AfterRunStateRunningBeforeStart: func() { close(runStarted) },
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitContractSignal(t, runStarted, "descriptor cleanup worker-retirement Run publication")
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- loop.Shutdown(context.Background()) }()

	if message := waitContractValue(t, messages, "descriptor cleanup failure diagnostic"); message != "eventloop: descriptor cleanup failed" {
		t.Fatalf("first cleanup diagnostic = %q, want descriptor cleanup failure", message)
	}
	if message := waitContractValue(t, messages, "descriptor cleanup rejection diagnostic"); message != rejectionDiagnostic {
		t.Fatalf("second cleanup diagnostic = %q, want disabled rejection fallback", message)
	}
	if err := waitContractValue(t, runDone, "Run after descriptor cleanup rejection"); !errors.Is(err, sentinel) {
		t.Fatalf("Run = %v, want injected close failure", err)
	}
	if err := waitContractValue(t, shutdownDone, "Shutdown after descriptor cleanup rejection"); !errors.Is(err, sentinel) {
		t.Fatalf("Shutdown = %v, want injected close failure", err)
	}
	if got := writerCalls.Load(); got != 2 {
		t.Fatalf("descriptor cleanup logger calls = %d, want 2", got)
	}
	if loop.callbackWorker != nil {
		t.Fatal("descriptor cleanup logger left its rejection fallback worker parked")
	}
	waitTerminalUnhandledRejectionTrackingDrained(t, js)
	requireDescriptorClosed(t, info.pollFD)
}

func TestRunReportsRunningCloseDescriptorCleanupFailure(t *testing.T) {
	loop, err := New(WithFastPathMode(FastPathDisabled))
	if err != nil {
		t.Fatal(err)
	}
	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	if err := loop.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	loop.poller.fdMu.RLock()
	info, active := loop.poller.fdInfoLocked(pipeFDs[0])
	loop.poller.fdMu.RUnlock()
	if !active || !info.ownsPollFD {
		t.Fatal("owned registration descriptor is unavailable")
	}

	sentinel := errors.New("injected running Close descriptor cleanup failure")
	loop.poller.descriptorClose = func(fd int) error {
		if err := unix.Close(fd); err != nil {
			return err
		}
		if fd == info.pollFD {
			return sentinel
		}
		return nil
	}
	cleanupReached := make(chan struct{})
	releaseCleanup := make(chan struct{})
	releaseCleanupFn := releaseSignalT(t, releaseCleanup)
	terminalJoined := make(chan struct{})
	var joinedOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeCloseFDLock: func() {
			close(cleanupReached)
			<-releaseCleanup
		},
		BeforeTerminalJoin: func() {
			joinedOnce.Do(func() { close(terminalJoined) })
		},
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitLoopOwnerTurnT(t, loop)
	closeDone := make(chan error, 1)
	go func() { closeDone <- loop.Close() }()
	waitContractSignal(t, cleanupReached, "running Close descriptor cleanup")

	joinedCloseDone := make(chan error, 1)
	go func() { joinedCloseDone <- loop.Close() }()
	waitContractSignal(t, terminalJoined, "losing Close descriptor cleanup join")
	select {
	case err := <-joinedCloseDone:
		t.Fatalf("losing Close returned before descriptor cleanup: %v", err)
	default:
	}

	releaseCleanupFn()
	if err := waitContractValue(t, closeDone, "winning Close descriptor cleanup result"); !errors.Is(err, sentinel) {
		t.Fatalf("winning Close error = %v, want injected cleanup failure", err)
	}
	if err := waitContractValue(t, joinedCloseDone, "joined Close descriptor cleanup result"); !errors.Is(err, sentinel) {
		t.Fatalf("joined Close error = %v, want injected cleanup failure", err)
	}
	if err := waitContractValue(t, runDone, "Run completion after Close cleanup"); !errors.Is(err, sentinel) {
		t.Fatalf("Run error = %v, want injected cleanup failure", err)
	}
}

func TestLoopWakeRegistrationFailureRollsBackPollerResources(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	var wakeFD, wakeWriteFD int
	loop.testHooks = &loopTestHooks{
		BeforeWakeFDRegister: func(readFD, writeFD int) {
			wakeFD, wakeWriteFD = readFD, writeFD
			if err := unix.Close(readFD); err != nil {
				t.Errorf("close wake read descriptor: %v", err)
			}
		},
	}
	if err := loop.ensurePoller(); err == nil {
		t.Fatal("ensurePoller succeeded after wake descriptor was closed")
	}
	if loop.pollerReady.Load() {
		t.Fatal("failed poller initialization published pollerReady")
	}
	if loop.wakePipe != -1 || loop.wakePipeWrite != -1 {
		t.Fatalf("stored wake descriptors after rollback = (%d, %d), want (-1, -1)", loop.wakePipe, loop.wakePipeWrite)
	}
	if fd := pollerNativeFD(&loop.poller); fd != -1 {
		t.Fatalf("stored poller descriptor after rollback = %d, want -1", fd)
	}
	if loop.poller.initialized.Load() {
		t.Fatal("poller remains initialized after wake registration rollback")
	}
	if loop.poller.fds != nil || loop.poller.sparseFDs != nil || loop.poller.tokenFDs != nil {
		t.Fatal("poller retains registration ownership after wake registration rollback")
	}
	requireDescriptorClosed(t, wakeFD)
	if wakeWriteFD != wakeFD {
		requireDescriptorClosed(t, wakeWriteFD)
	}
	loop.testHooks = nil
	if err := loop.ensurePoller(); err != nil {
		t.Fatalf("ensurePoller retry: %v", err)
	}
	if !loop.pollerReady.Load() || pollerNativeFD(&loop.poller) < 0 {
		t.Fatal("successful retry did not publish valid poller resources")
	}
	if err := loop.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalCleanupClearsRetainedRegistrationCount(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	fd, cleanup := testCreateIOFD(t)
	t.Cleanup(cleanup)
	closeFDLock := make(chan struct{})
	shutdownDone := make(chan error, 1)
	rollbackErr := errors.New("injected terminal unregister failure")
	var startOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeRegisterFDRollbackCheck: func() {
			startOnce.Do(func() {
				go func() { shutdownDone <- loop.Shutdown(context.Background()) }()
			})
			waitContractSignal(t, closeFDLock, "terminal FD cleanup ownership")
		},
		RegisterFDRollback: func(int) error { return rollbackErr },
		BeforeCloseFDLock: func() {
			select {
			case <-closeFDLock:
			default:
				close(closeFDLock)
			}
		},
	}
	err = loop.RegisterFD(fd, EventRead, func(IOEvents) {})
	var partial *FDRegistrationRollbackError
	if !errors.As(err, &partial) || !partial.Registered() {
		t.Fatalf("RegisterFD error at return = %#v, want retained ownership", err)
	}
	if !errors.Is(err, ErrLoopTerminated) || !errors.Is(err, rollbackErr) {
		t.Fatalf("RegisterFD error = %v, want terminal and rollback failures", err)
	}
	if err := waitContractValue(t, shutdownDone, "Shutdown completion"); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount after terminal cleanup = %d, want 0", got)
	}
	if loop.pollerReady.Load() || loop.poller.userFDRegistered(fd) {
		t.Fatal("terminal cleanup retained poller ownership")
	}
}
