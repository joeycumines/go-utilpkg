//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestPollBackendControlInitFailureRetryAndCleanup(t *testing.T) {
	control := newPollBackendControl()
	createErr := errors.New("forced control creation failure")
	createCalls := 0
	control.controlCreate = func() (int, int, error) {
		createCalls++
		if createCalls == 1 {
			return -1, -1, createErr
		}
		return 10, 11, nil
	}
	resetCalls := 0
	if err := control.init(nil, func() { resetCalls++ }); !errors.Is(err, createErr) {
		t.Fatalf("first init error = %v, want %v", err, createErr)
	}
	if control.initialized.Load() || control.controlReadFD != -1 || control.controlWriteFD != -1 || resetCalls != 0 {
		t.Fatalf("failed init state = (initialized %v, control %d/%d, resets %d)", control.initialized.Load(), control.controlReadFD, control.controlWriteFD, resetCalls)
	}
	if err := control.init(nil, func() { resetCalls++ }); err != nil {
		t.Fatalf("init retry: %v", err)
	}
	if resetCalls != 1 || control.controlReadFD != 10 || control.controlWriteFD != 11 {
		t.Fatalf("successful init state = (control %d/%d, resets %d)", control.controlReadFD, control.controlWriteFD, resetCalls)
	}
	committed := false
	if err := control.commit(func() error { committed = true; return nil }); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if !committed {
		t.Fatal("commit callback was not called")
	}
	readFD := -1
	control.controlRead = func(fd int, buffer []byte) (int, error) {
		readFD = fd
		return 0, unix.EAGAIN
	}
	if err := control.drain(func(int, []byte) (int, error) { return 0, errors.New("unexpected default control read") }); err != nil {
		t.Fatalf("drain: %v", err)
	}
	if readFD != 10 {
		t.Fatalf("control read descriptor = %d, want 10", readFD)
	}

	closed := make(map[int]int)
	waits := 0
	inactiveSignal := false
	err := control.close(
		func(int, []byte) (int, error) { inactiveSignal = true; return 0, nil },
		func(fd int) error { closed[fd]++; return nil },
		func() ([]int, []func()) {
			return []int{101, 102}, []func(){func() { waits++ }}
		},
	)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if inactiveSignal {
		t.Fatal("inactive close submitted a control signal")
	}
	for _, fd := range []int{10, 11, 101, 102} {
		if closed[fd] != 1 {
			t.Errorf("descriptor %d close calls = %d, want 1", fd, closed[fd])
		}
	}
	if waits != 1 || control.controlReadFD != -1 || control.controlWriteFD != -1 || control.initialized.Load() {
		t.Fatalf("closed state = (waits %d, control %d/%d, initialized %v)", waits, control.controlReadFD, control.controlWriteFD, control.initialized.Load())
	}
	repeatedRelease := false
	if err := control.close(nil, func(int) error { repeatedRelease = true; return nil }, func() ([]int, []func()) {
		repeatedRelease = true
		return nil, nil
	}); err != nil {
		t.Fatalf("repeated close: %v", err)
	}
	if repeatedRelease {
		t.Fatal("repeated close released or collected resources")
	}
	if err := control.init(func() (int, int, error) { return 12, 13, nil }, func() {}); !errors.Is(err, errPollerClosed) {
		t.Fatalf("init after close = %v, want %v", err, errPollerClosed)
	}
}

func TestPollBackendControlRegisterSignalFailureRollsBackOwnership(t *testing.T) {
	control := initializedPollBackendControl(t)
	waitEntered := make(chan struct{})
	releaseWait := make(chan struct{})
	pollDone := make(chan error, 1)
	go func() {
		_, _, err := control.pollAttempt(0, func() {}, func(time.Duration) (int, error) {
			close(waitEntered)
			<-releaseWait
			return 0, nil
		}, func(int) (int, error) { return 0, nil })
		pollDone <- err
	}()
	waitPollCoreSignal(t, waitEntered, "native poll entry")

	signalErr := errors.New("forced control signal failure")
	active := false
	rolledBack := false
	closed := false
	signalOwnershipOK := false
	control.controlWrite = func(_ int, buffer []byte) (int, error) {
		signalOwnershipOK = active && !rolledBack && !closed
		return 0, signalErr
	}
	closeOwnershipOK := false
	err := control.register(
		func(int, []byte) (int, error) { return 0, errors.New("unexpected default control write") },
		func() (int, error) { active = true; return 101, nil },
		func() { active = false; rolledBack = true },
		func(fd int) error {
			closeOwnershipOK = fd == 101 && !active && rolledBack
			closed = true
			return nil
		},
	)
	if !errors.Is(err, signalErr) {
		t.Fatalf("register error = %v, want %v", err, signalErr)
	}
	if !signalOwnershipOK || !closeOwnershipOK {
		t.Fatalf("ownership ordering = (signal %v, close %v)", signalOwnershipOK, closeOwnershipOK)
	}
	if active || !rolledBack || !closed {
		t.Fatalf("failed register ownership = (active %v, rolledBack %v, closed %v)", active, rolledBack, closed)
	}
	close(releaseWait)
	if err := waitPollCoreValue(t, pollDone, "native poll completion"); err != nil {
		t.Fatal(err)
	}
	closePollBackendControl(t, control)
}

func TestPollBackendControlModifySignalsBeforePublication(t *testing.T) {
	control := initializedPollBackendControl(t)
	waitEntered := make(chan struct{})
	releaseWait := make(chan struct{})
	pollDone := make(chan error, 1)
	go func() {
		_, _, err := control.pollAttempt(0, func() {}, func(time.Duration) (int, error) {
			close(waitEntered)
			<-releaseWait
			return 0, nil
		}, func(int) (int, error) { return 0, nil })
		pollDone <- err
	}()
	waitPollCoreSignal(t, waitEntered, "native poll entry")

	oldMask := EventRead
	targetMask := EventWrite
	signalErr := errors.New("forced control signal failure")
	failedSignalBeforePublication := false
	control.controlWrite = func(int, []byte) (int, error) {
		failedSignalBeforePublication = oldMask == EventRead
		return 0, signalErr
	}
	modify := func() error {
		return control.modify(
			func(int, []byte) (int, error) { return 0, errors.New("unexpected default control write") },
			func() error { return nil },
			func() { oldMask = targetMask },
		)
	}
	if err := modify(); !errors.Is(err, signalErr) {
		t.Fatalf("failed modify error = %v, want %v", err, signalErr)
	}
	if !failedSignalBeforePublication || oldMask != EventRead {
		t.Fatalf("failed modify ordering = (signal-before-publication %v, mask %v)", failedSignalBeforePublication, oldMask)
	}

	var releaseOnce sync.Once
	successfulSignalBeforePublication := false
	control.controlWrite = func(_ int, buffer []byte) (int, error) {
		successfulSignalBeforePublication = oldMask == EventRead
		releaseOnce.Do(func() { close(releaseWait) })
		return len(buffer), nil
	}
	if err := modify(); err != nil {
		t.Fatalf("successful modify: %v", err)
	}
	if !successfulSignalBeforePublication || oldMask != targetMask {
		t.Fatalf("successful modify ordering = (signal-before-publication %v, mask %v)", successfulSignalBeforePublication, oldMask)
	}
	if err := waitPollCoreValue(t, pollDone, "native poll completion"); err != nil {
		t.Fatal(err)
	}
	closePollBackendControl(t, control)
}

func TestPollBackendControlUnregisterJoinsResultConversion(t *testing.T) {
	control := initializedPollBackendControl(t)
	waitEntered := make(chan struct{})
	releaseWait := make(chan struct{})
	conversionEntered := make(chan struct{})
	releaseConversion := make(chan struct{})
	pollDone := make(chan error, 1)
	go func() {
		_, _, err := control.pollAttempt(0, func() {}, func(time.Duration) (int, error) {
			close(waitEntered)
			<-releaseWait
			return 1, nil
		}, func(int) (int, error) {
			close(conversionEntered)
			<-releaseConversion
			return 1, nil
		})
		pollDone <- err
	}()
	waitPollCoreSignal(t, waitEntered, "native poll entry")

	active := true
	signalAfterRetirement := atomic.Bool{}
	control.controlWrite = func(_ int, buffer []byte) (int, error) {
		signalAfterRetirement.Store(!active)
		close(releaseWait)
		return len(buffer), nil
	}
	var descriptorClosed atomic.Bool
	var closedDescriptor atomic.Int64
	var dispatchWaited atomic.Bool
	unregisterDone := make(chan error, 1)
	go func() {
		unregisterDone <- control.unregister(
			func(int, []byte) (int, error) { return 0, errors.New("unexpected default control write") },
			false,
			func() (pollBackendRetirement, error) {
				active = false
				return pollBackendRetirement{
					descriptor:     101,
					ownsDescriptor: true,
					wait:           func() { dispatchWaited.Store(true) },
				}, nil
			},
			func(fd int) error {
				closedDescriptor.Store(int64(fd))
				descriptorClosed.Store(true)
				return nil
			},
		)
	}()
	waitPollCoreSignal(t, conversionEntered, "native result conversion")
	if descriptorClosed.Load() {
		t.Fatal("owned descriptor closed during native result conversion")
	}
	select {
	case err := <-unregisterDone:
		t.Fatalf("unregister completed during result conversion: %v", err)
	default:
	}
	close(releaseConversion)
	if err := waitPollCoreValue(t, unregisterDone, "unregister completion"); err != nil {
		t.Fatal(err)
	}
	if !signalAfterRetirement.Load() || !descriptorClosed.Load() || closedDescriptor.Load() != 101 || !dispatchWaited.Load() {
		t.Fatalf("retirement completion = (signal after retirement %v, descriptor closed %v/%d, dispatch waited %v)", signalAfterRetirement.Load(), descriptorClosed.Load(), closedDescriptor.Load(), dispatchWaited.Load())
	}
	if err := waitPollCoreValue(t, pollDone, "native poll completion"); err != nil {
		t.Fatal(err)
	}
	closePollBackendControl(t, control)
}

func TestPollBackendControlLatchedWakeSkipsPrivateSignal(t *testing.T) {
	control := initializedPollBackendControl(t)
	waitEntered := make(chan struct{})
	releaseWait := make(chan struct{})
	pollDone := make(chan error, 1)
	go func() {
		_, _, err := control.pollAttempt(0, func() {}, func(time.Duration) (int, error) {
			close(waitEntered)
			<-releaseWait
			return 0, nil
		}, func(int) (int, error) { return 0, nil })
		pollDone <- err
	}()
	waitPollCoreSignal(t, waitEntered, "native poll entry")

	retired := make(chan struct{})
	writes := atomic.Int32{}
	control.controlWrite = func(_ int, buffer []byte) (int, error) {
		writes.Add(1)
		return len(buffer), nil
	}
	unownedDescriptorClosed := atomic.Bool{}
	unregisterDone := make(chan error, 1)
	go func() {
		unregisterDone <- control.unregister(
			func(int, []byte) (int, error) { return 0, errors.New("unexpected default control write") },
			true,
			func() (pollBackendRetirement, error) {
				close(retired)
				return pollBackendRetirement{}, nil
			},
			func(int) error { unownedDescriptorClosed.Store(true); return nil },
		)
	}()
	waitPollCoreSignal(t, retired, "local retirement")
	if writes.Load() != 0 {
		t.Fatalf("private control writes = %d, want 0", writes.Load())
	}
	close(releaseWait)
	if err := waitPollCoreValue(t, unregisterDone, "latched unregister completion"); err != nil {
		t.Fatal(err)
	}
	if err := waitPollCoreValue(t, pollDone, "native poll completion"); err != nil {
		t.Fatal(err)
	}
	if unownedDescriptorClosed.Load() {
		t.Fatal("unowned descriptor was closed")
	}
	closePollBackendControl(t, control)
}

func TestPollBackendControlCloseSignalsAndJoinsNativeOwnership(t *testing.T) {
	control := initializedPollBackendControl(t)
	waitEntered := make(chan struct{})
	releaseWait := make(chan struct{})
	afterPollEntered := make(chan struct{})
	releaseAfterPoll := make(chan struct{})
	control.afterNativePoll = func() {
		close(afterPollEntered)
		<-releaseAfterPoll
	}
	pollDone := make(chan error, 1)
	converted := atomic.Bool{}
	go func() {
		_, _, err := control.pollAttempt(-1, func() {}, func(time.Duration) (int, error) {
			close(waitEntered)
			<-releaseWait
			return 1, nil
		}, func(int) (int, error) {
			converted.Store(true)
			return 0, nil
		})
		pollDone <- err
	}()
	waitPollCoreSignal(t, waitEntered, "native poll entry")

	var signalOnce sync.Once
	control.controlWrite = func(_ int, buffer []byte) (int, error) {
		signalOnce.Do(func() { close(releaseWait) })
		return len(buffer), nil
	}
	closed := make(map[int]int)
	waited := atomic.Bool{}
	closeDone := make(chan error, 1)
	go func() {
		closeDone <- control.close(
			func(int, []byte) (int, error) { return 0, errors.New("unexpected default control write") },
			func(fd int) error { closed[fd]++; return nil },
			func() ([]int, []func()) {
				return []int{101}, []func(){func() { waited.Store(true) }}
			},
		)
	}()
	waitPollCoreSignal(t, afterPollEntered, "post-poll ownership boundary")
	select {
	case err := <-closeDone:
		t.Fatalf("close completed before native ownership release: %v", err)
	default:
	}
	if len(closed) != 0 {
		t.Fatalf("descriptors closed before native ownership release: %v", closed)
	}
	close(releaseAfterPoll)
	if err := waitPollCoreValue(t, closeDone, "close completion"); err != nil {
		t.Fatal(err)
	}
	for _, fd := range []int{10, 11, 101} {
		if closed[fd] != 1 {
			t.Errorf("descriptor %d close calls = %d, want 1", fd, closed[fd])
		}
	}
	if !waited.Load() {
		t.Fatal("close did not join pending dispatch starts")
	}
	if err := waitPollCoreValue(t, pollDone, "closed native poll completion"); !errors.Is(err, errPollerClosed) {
		t.Fatalf("poll completion = %v, want %v", err, errPollerClosed)
	}
	if converted.Load() {
		t.Fatal("closed poll converted native results")
	}
}

func TestPollBackendControlZeroValueClosePreservesDescriptorZero(t *testing.T) {
	var control pollBackendControl
	closed := false
	if err := control.close(nil, func(int) error { closed = true; return nil }, func() ([]int, []func()) {
		return []int{0}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if closed {
		t.Fatal("zero-value close released descriptor zero")
	}
}

func initializedPollBackendControl(t *testing.T) *pollBackendControl {
	t.Helper()
	control := newPollBackendControl()
	if err := control.init(func() (int, int, error) { return 10, 11, nil }, func() {}); err != nil {
		t.Fatal(err)
	}
	return &control
}

func closePollBackendControl(t *testing.T, control *pollBackendControl) {
	t.Helper()
	if err := control.close(
		func(_ int, buffer []byte) (int, error) { return len(buffer), nil },
		func(int) error { return nil },
		func() ([]int, []func()) { return nil, nil },
	); err != nil {
		t.Fatal(err)
	}
}

func waitPollCoreSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func waitPollCoreValue[T any](t *testing.T, values <-chan T, name string) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
		var zero T
		return zero
	}
}
