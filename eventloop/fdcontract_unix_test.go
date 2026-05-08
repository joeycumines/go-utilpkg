//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestAutoExitTerminalAdmissionRejectsRegisterFD(t *testing.T) {
	loop := New(WithAutoExit(true))

	admissionClosed := make(chan struct{})
	releaseTermination := make(chan struct{})
	release := contractRelease(t, releaseTermination)
	var hookOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeTerminateState: func() {
			hookOnce.Do(func() {
				close(admissionClosed)
				<-releaseTermination
			})
		},
	}
	timerID, err := loop.ScheduleTimer(time.Hour, func() {})
	if err != nil {
		t.Fatalf("ScheduleTimer: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	waitRefedTimerCount(t, loop, 1)
	if err := loop.UnrefTimer(timerID); err != nil {
		t.Fatalf("UnrefTimer: %v", err)
	}
	waitContractSignal(t, admissionClosed, "auto-exit terminal admission closure")

	fd, cleanupFD := testCreateIOFD(t)
	t.Cleanup(cleanupFD)
	registerDone := make(chan error, 1)
	go func() { registerDone <- loop.RegisterFD(fd, EventRead, func(IOEvents) {}) }()
	if err := waitContractValue(t, registerDone, "RegisterFD rejection"); !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("RegisterFD after auto-exit terminal admission closed = %v, want ErrLoopTerminated", err)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount after rejected RegisterFD = %d, want 0", got)
	}

	release()
	if err := waitContractValue(t, runDone, "auto-exit Run completion"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if state := loop.State(); state != StateTerminated {
		t.Fatalf("State after auto-exit = %v, want StateTerminated", state)
	}
	if loop.Alive() {
		t.Fatal("Alive returned true after auto-exit")
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount after auto-exit = %d, want 0", got)
	}
}

func TestFastPollerUnregisterClosedFDReleasesLocalOwnership(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}
	registerPollerCleanupT(t, &poller)

	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	registeredFD := pipeFDs[0]
	if err := poller.RegisterFD(registeredFD, EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	if err := unix.Close(registeredFD); err != nil {
		t.Fatal(err)
	}
	pipeFDs[0] = -1
	if err := poller.UnregisterFD(registeredFD); err != nil {
		t.Fatalf("UnregisterFD after user close: %v", err)
	}
	poller.fdMu.RLock()
	_, active := poller.fdInfoLocked(registeredFD)
	poller.fdMu.RUnlock()
	if active {
		t.Fatal("closed descriptor retains local registration ownership")
	}
}

func TestFastPollerClosedFDRegistrationDoesNotRetainOwnership(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}
	registerPollerCleanupT(t, &poller)
	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	closedFD := pipeFDs[0]
	if err := unix.Close(closedFD); err != nil {
		t.Fatal(err)
	}
	pipeFDs[0] = -1
	if err := poller.RegisterFD(closedFD, EventRead, func(IOEvents) {}); !errors.Is(err, unix.EBADF) {
		t.Fatalf("RegisterFD closed descriptor = %v, want EBADF", err)
	}
	poller.fdMu.RLock()
	_, active := poller.fdInfoLocked(closedFD)
	poller.fdMu.RUnlock()
	if active {
		t.Fatal("failed closed-descriptor registration retained local ownership")
	}
}

func TestFastPollerOwnsNativeRegistrationDescriptor(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}
	registerPollerCleanupT(t, &poller)
	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	registeredFD := pipeFDs[0]
	if err := poller.RegisterFD(registeredFD, EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	poller.fdMu.RLock()
	info, active := poller.fdInfoLocked(registeredFD)
	poller.fdMu.RUnlock()
	if !active || !info.ownsPollFD || info.pollFD < 0 || info.pollFD == registeredFD {
		t.Fatalf("owned registration active=%v owns=%v pollFD=%d original=%d", active, info.ownsPollFD, info.pollFD, registeredFD)
	}
	if err := unix.Close(registeredFD); err != nil {
		t.Fatal(err)
	}
	pipeFDs[0] = -1
	flags, err := unix.FcntlInt(uintptr(info.pollFD), unix.F_GETFD, 0)
	if err != nil {
		t.Fatalf("owned registration descriptor after caller close: %v", err)
	}
	if flags&unix.FD_CLOEXEC == 0 {
		t.Fatalf("owned registration descriptor flags = %#x, want FD_CLOEXEC", flags)
	}
	if _, err := unix.Write(pipeFDs[1], []byte{1}); err != nil {
		t.Fatal(err)
	}
	if got, err := poller.PollIO(0); err != nil {
		t.Fatal(err)
	} else if got != 1 {
		t.Fatalf("owned registration callbacks = %d, want 1", got)
	}
	ready := poller.readyEventsSnapshot()
	if len(ready) != 1 || ready[0].fd != registeredFD || ready[0].events != EventRead {
		t.Fatalf("ready events = %+v, want one EventRead for fd %d", ready, registeredFD)
	}
	if err := poller.UnregisterFD(registeredFD); err != nil {
		t.Fatal(err)
	}
	requireDescriptorClosed(t, info.pollFD)
}

func TestLoopUnregisterClosedFDReleasesLiveness(t *testing.T) {
	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	loop := New()
	registerLoopCleanupT(t, loop)
	registeredFD := pipeFDs[0]
	if err := loop.RegisterFD(registeredFD, EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	if err := unix.Close(registeredFD); err != nil {
		t.Fatal(err)
	}
	pipeFDs[0] = -1
	if err := loop.UnregisterFD(registeredFD); err != nil {
		t.Fatalf("UnregisterFD after user close: %v", err)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount = %d, want 0", got)
	}
}

func TestLoopDuplicateRegisterPreservesSentinelIdentity(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	if err := loop.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	if err := loop.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); err != ErrFDAlreadyRegistered {
		t.Fatalf("duplicate RegisterFD = %#v, want exact ErrFDAlreadyRegistered", err)
	}
}

func TestRegisterFDRejectsUnusableCallbackAndInterests(t *testing.T) {
	tests := []struct {
		name     string
		events   IOEvents
		callback ioCallback
		want     error
	}{
		{name: "nil callback", events: EventRead, want: errFDNilCallback},
		{name: "zero interests", callback: func(IOEvents) {}, want: errFDInvalidEvents},
		{name: "output-only error", events: EventError, callback: func(IOEvents) {}, want: errFDInvalidEvents},
		{name: "output-only hangup", events: EventHangup, callback: func(IOEvents) {}, want: errFDInvalidEvents},
		{name: "mixed input and result", events: EventRead | EventError, callback: func(IOEvents) {}, want: errFDInvalidEvents},
		{name: "unknown bit", events: IOEvents(1 << 31), callback: func(IOEvents) {}, want: errFDInvalidEvents},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var pipeFDs [2]int
			if err := unix.Pipe(pipeFDs[:]); err != nil {
				t.Fatal(err)
			}
			registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
			var poller fastPoller
			if err := poller.Init(); err != nil {
				t.Fatal(err)
			}
			registerPollerCleanupT(t, &poller)
			if err := poller.RegisterFD(pipeFDs[0], test.events, test.callback); !errors.Is(err, test.want) {
				t.Fatalf("RegisterFD error = %v, want %v", err, test.want)
			}
			poller.fdMu.RLock()
			_, active := poller.fdInfoLocked(pipeFDs[0])
			poller.fdMu.RUnlock()
			if active {
				t.Fatal("rejected registration retained local ownership")
			}
		})
	}
}

func TestLoopRegisterFDRejectsInvalidContractWithoutResources(t *testing.T) {
	tests := []struct {
		name     string
		events   IOEvents
		callback func(IOEvents)
		want     error
	}{
		{name: "nil callback", events: EventRead, want: errFDNilCallback},
		{name: "zero interests", callback: func(IOEvents) {}, want: errFDInvalidEvents},
		{name: "mixed input and result", events: EventWrite | EventHangup, callback: func(IOEvents) {}, want: errFDInvalidEvents},
		{name: "unknown bit", events: IOEvents(1 << 31), callback: func(IOEvents) {}, want: errFDInvalidEvents},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			loop := New()
			registerLoopCleanupT(t, loop)
			panicValue := captureErrorContractPanic(func() {
				_ = loop.RegisterFD(0, test.events, test.callback)
			})
			panicErr, ok := panicValue.(error)
			if !ok || !errors.Is(panicErr, test.want) {
				t.Fatalf("RegisterFD panic = %#v, want error matching %v", panicValue, test.want)
			}
			if loop.pollerReady.Load() {
				t.Fatal("invalid registration initialized poller resources")
			}
			if loop.wakePipe != -1 || loop.wakePipeWrite != -1 {
				t.Fatalf("invalid registration wake descriptors = (%d, %d), want (-1, -1)", loop.wakePipe, loop.wakePipeWrite)
			}
			if got := loop.userIOFDCount.Load(); got != 0 {
				t.Fatalf("userIOFDCount = %d, want 0", got)
			}
		})
	}
}

func TestLoopRegisterFDRejectedOwnershipCannotDispatchCallback(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	if err := loop.ensurePoller(); err != nil {
		t.Fatal(err)
	}
	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	if _, err := unix.Write(pipeFDs[1], []byte{1}); err != nil {
		t.Fatal(err)
	}
	registrationEntered := make(chan struct{})
	releaseRegistration := make(chan struct{})
	release := contractRelease(t, releaseRegistration)
	loop.testHooks = &loopTestHooks{BeforeRegisterFDRollbackCheck: func() {
		close(registrationEntered)
		<-releaseRegistration
	}}
	var callbackCalls atomic.Int32
	registerDone := make(chan error, 1)
	go func() {
		registerDone <- loop.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {
			callbackCalls.Add(1)
		})
	}()
	waitContractSignal(t, registrationEntered, "provisional FD registration")
	if got, err := loop.poller.PollIO(0); err != nil {
		t.Fatal(err)
	} else if got != 1 {
		t.Fatalf("ready events before registration commit = %d, want 1", got)
	}
	ready := append([]pollEvent(nil), loop.poller.readyEventsSnapshot()...)
	loop.dispatchPollEvents(ready)
	loop.poller.clearReadyEvents()
	if got := callbackCalls.Load(); got != 0 {
		t.Fatalf("callback calls before registration ownership commit = %d, want 0", got)
	}
	if err := loop.SetFastPathMode(FastPathForced); err != nil {
		t.Fatalf("force fast path while registration is provisional: %v", err)
	}
	release()
	if err := waitContractValue(t, registerDone, "rejected RegisterFD completion"); !errors.Is(err, ErrFastPathIncompatible) {
		t.Fatalf("RegisterFD after mode rejection = %v, want ErrFastPathIncompatible", err)
	}
	if got := callbackCalls.Load(); got != 0 {
		t.Fatalf("callback calls for rejected registration = %d, want 0", got)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount after rejected registration = %d, want 0", got)
	}
	if loop.poller.userFDRegistered(pipeFDs[0]) {
		t.Fatal("rejected registration retains poller ownership")
	}
}

func TestModifyFDValidationContract(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}
	registerPollerCleanupT(t, &poller)
	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	if err := poller.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	if err := poller.ModifyFD(pipeFDs[0], 0); err != nil {
		t.Fatalf("ModifyFD(0): %v", err)
	}
	for _, events := range []IOEvents{EventError, EventHangup, EventRead | EventError, IOEvents(1 << 31)} {
		if err := poller.ModifyFD(pipeFDs[0], events); !errors.Is(err, errFDInvalidEvents) {
			t.Fatalf("ModifyFD(%v) = %v, want errFDInvalidEvents", events, err)
		}
	}
}

func TestLoopPublicFDMutationCannotTargetInternalWake(t *testing.T) {
	loop := New(WithFastPathMode(FastPathDisabled))
	registerLoopCleanupT(t, loop)

	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	if err := loop.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}

	wakeFD := loop.wakePipe
	if err := loop.ModifyFD(wakeFD, 0); !errors.Is(err, ErrFDNotRegistered) {
		t.Fatalf("ModifyFD(internal wake) = %v, want ErrFDNotRegistered", err)
	}
	if err := loop.UnregisterFD(wakeFD); !errors.Is(err, ErrFDNotRegistered) {
		t.Fatalf("UnregisterFD(internal wake) = %v, want ErrFDNotRegistered", err)
	}
	if got := loop.userIOFDCount.Load(); got != 1 {
		t.Fatalf("userIOFDCount after internal mutation attempts = %d, want 1", got)
	}
	loop.poller.fdMu.RLock()
	_, active := loop.poller.fdInfoLocked(wakeFD)
	loop.poller.fdMu.RUnlock()
	if !active {
		t.Fatal("internal wake registration was removed by public FD mutation")
	}
	if err := loop.UnregisterFD(pipeFDs[0]); err != nil {
		t.Fatalf("UnregisterFD(user FD): %v", err)
	}
}

func TestRegisterFDRollbackFailureRetainsLifecycleOwnership(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	fd, cleanup := testCreateIOFD(t)
	t.Cleanup(cleanup)
	rollbackErr := errors.New("injected unregister failure")
	loop.testHooks = &loopTestHooks{
		BeforeRegisterFDRollbackCheck: func() { loop.beginQuiescing() },
		RegisterFDRollback:            func(int) error { return rollbackErr },
	}

	err := loop.RegisterFD(fd, EventRead, func(IOEvents) {})
	var partial *FDRegistrationRollbackError
	if !errors.As(err, &partial) || !partial.Registered() {
		t.Fatalf("RegisterFD error = %#v, want retained FDRegistrationRollbackError", err)
	}
	if !errors.Is(err, ErrLoopTerminated) || !errors.Is(err, rollbackErr) {
		t.Fatalf("RegisterFD error = %v, want lifecycle and rollback errors", err)
	}
	if got := loop.userIOFDCount.Load(); got != 1 {
		t.Fatalf("userIOFDCount after retained rollback = %d, want 1", got)
	}
	if !loop.pollerReady.Load() || !loop.poller.userFDRegistered(fd) {
		t.Fatal("retained rollback did not publish owned user registration")
	}
	loop.testHooks = nil
	if err := loop.UnregisterFD(fd); err != nil {
		t.Fatalf("cleanup UnregisterFD: %v", err)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount after cleanup = %d, want 0", got)
	}
}

func TestRegisterFDRollbackFailureForcesPollingMode(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)
	fd, cleanup := testCreateIOFD(t)
	t.Cleanup(cleanup)
	rollbackErr := errors.New("injected unregister failure")
	loop.testHooks = &loopTestHooks{
		BeforeRegisterFDCommit: func() { loop.fastPathMode.Store(int32(FastPathForced)) },
		RegisterFDRollback:     func(int) error { return rollbackErr },
	}

	err := loop.RegisterFD(fd, EventRead, func(IOEvents) {})
	var partial *FDRegistrationRollbackError
	if !errors.As(err, &partial) || !partial.Registered() {
		t.Fatalf("RegisterFD error = %#v, want retained FDRegistrationRollbackError", err)
	}
	if !errors.Is(err, ErrFastPathIncompatible) || !errors.Is(err, rollbackErr) {
		t.Fatalf("RegisterFD error = %v, want mode and rollback errors", err)
	}
	if got := loop.userIOFDCount.Load(); got != 1 {
		t.Fatalf("userIOFDCount after retained rollback = %d, want 1", got)
	}
	if got := FastPathMode(loop.fastPathMode.Load()); got == FastPathForced {
		t.Fatalf("fastPathMode after retained rollback = %v, want polling-capable mode", got)
	}
	if loop.canUseFastPath() {
		t.Fatal("canUseFastPath returned true while retained user FD is owned")
	}
	loop.testHooks = nil
	if err := loop.UnregisterFD(fd); err != nil {
		t.Fatalf("cleanup UnregisterFD: %v", err)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount after cleanup = %d, want 0", got)
	}
}

func TestLoopModifyUnregisterForcedLinearization(t *testing.T) {
	t.Run("modify first", func(t *testing.T) {
		loop := New()
		registerLoopCleanupT(t, loop)
		fd, cleanup := testCreateIOFD(t)
		t.Cleanup(cleanup)
		if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
			t.Fatal(err)
		}
		entered := make(chan struct{})
		release := make(chan struct{})
		releaseOperation := contractRelease(t, release)
		loop.testHooks = &loopTestHooks{BeforeFDModify: func() { close(entered); <-release }}
		modifyDone := make(chan error, 1)
		go func() { modifyDone <- loop.ModifyFD(fd, EventWrite) }()
		waitContractSignal(t, entered, "ModifyFD ownership")
		unregisterDone := make(chan error, 1)
		go func() { unregisterDone <- loop.UnregisterFD(fd) }()
		releaseOperation()
		if err := waitContractValue(t, modifyDone, "ModifyFD completion"); err != nil {
			t.Fatalf("ModifyFD: %v", err)
		}
		if err := waitContractValue(t, unregisterDone, "UnregisterFD completion"); err != nil {
			t.Fatalf("UnregisterFD: %v", err)
		}
	})

	t.Run("unregister first", func(t *testing.T) {
		loop := New()
		registerLoopCleanupT(t, loop)
		fd, cleanup := testCreateIOFD(t)
		t.Cleanup(cleanup)
		if err := loop.RegisterFD(fd, EventRead, func(IOEvents) {}); err != nil {
			t.Fatal(err)
		}
		entered := make(chan struct{})
		release := make(chan struct{})
		releaseOperation := contractRelease(t, release)
		loop.testHooks = &loopTestHooks{BeforeFDUnregister: func() { close(entered); <-release }}
		unregisterDone := make(chan error, 1)
		go func() { unregisterDone <- loop.UnregisterFD(fd) }()
		waitContractSignal(t, entered, "UnregisterFD ownership")
		modifyDone := make(chan error, 1)
		go func() { modifyDone <- loop.ModifyFD(fd, EventWrite) }()
		releaseOperation()
		if err := waitContractValue(t, unregisterDone, "UnregisterFD completion"); err != nil {
			t.Fatalf("UnregisterFD: %v", err)
		}
		if err := waitContractValue(t, modifyDone, "ModifyFD completion"); !errors.Is(err, ErrFDNotRegistered) {
			t.Fatalf("ModifyFD after UnregisterFD = %v, want ErrFDNotRegistered", err)
		}
	})
}
