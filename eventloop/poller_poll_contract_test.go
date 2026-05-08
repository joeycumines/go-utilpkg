//go:build (aix && ppc64) || (solaris && amd64)

package eventloop

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"golang.org/x/sys/unix"
)

func testPoller(t *testing.T) *fastPoller {
	t.Helper()
	poller := newFastPoller()
	poller.controlCreate = func() (int, int, error) { return 10, 11, nil }
	poller.controlRead = func(int, []byte) (int, error) { return 0, unix.EAGAIN }
	poller.controlWrite = func(_ int, buffer []byte) (int, error) { return len(buffer), nil }
	poller.descriptorClose = func(int) error { return nil }
	nextFD := 100
	poller.descriptorDup = func(int) (int, error) {
		nextFD++
		return nextFD, nil
	}
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		poller.beforeNativePoll = nil
		poller.afterNativePoll = nil
		poller.beforeResourceClose = nil
		poller.beforeDispatchWait = nil
		poller.controlWrite = func(_ int, buffer []byte) (int, error) { return len(buffer), nil }
		if err := poller.Close(); err != nil {
			t.Errorf("close poller: %v", err)
		}
	})
	return &poller
}

func TestPollInitFailureRetryAndControlPairCleanup(t *testing.T) {
	poller := newFastPoller()
	createErr := errors.New("forced control creation failure")
	createCalls := 0
	poller.controlCreate = func() (int, int, error) {
		createCalls++
		if createCalls == 1 {
			return -1, -1, createErr
		}
		return 10, 11, nil
	}
	if err := poller.Init(); !errors.Is(err, createErr) {
		t.Fatalf("first Init error = %v, want %v", err, createErr)
	}
	if poller.initialized.Load() || poller.controlReadFD != -1 || poller.controlWriteFD != -1 {
		t.Fatalf("failed Init state = (initialized %v, control %d/%d), want (false, -1/-1)", poller.initialized.Load(), poller.controlReadFD, poller.controlWriteFD)
	}
	if err := poller.Init(); err != nil {
		t.Fatalf("Init retry: %v", err)
	}
	if err := poller.Init(); !errors.Is(err, errPollerAlreadyInitialized) {
		t.Fatalf("second successful Init error = %v, want %v", err, errPollerAlreadyInitialized)
	}
	readCloseErr := errors.New("forced control read close failure")
	writeCloseErr := errors.New("forced control write close failure")
	closed := make(map[int]int)
	poller.descriptorClose = func(fd int) error {
		closed[fd]++
		switch fd {
		case 10:
			return readCloseErr
		case 11:
			return writeCloseErr
		default:
			return nil
		}
	}
	if err := poller.Close(); !errors.Is(err, readCloseErr) || !errors.Is(err, writeCloseErr) {
		t.Fatalf("Close error = %v, want both control close failures", err)
	}
	if closed[10] != 1 || closed[11] != 1 || poller.controlReadFD != -1 || poller.controlWriteFD != -1 || poller.initialized.Load() {
		t.Fatalf("closed control state = (calls %v, control %d/%d, initialized %v)", closed, poller.controlReadFD, poller.controlWriteFD, poller.initialized.Load())
	}
	if err := poller.Close(); err != nil {
		t.Fatalf("repeated Close: %v", err)
	}
	if err := poller.Init(); !errors.Is(err, errPollerClosed) {
		t.Fatalf("Init after Close error = %v, want %v", err, errPollerClosed)
	}
	if err := poller.RegisterFD(1, EventRead, func(IOEvents) {}); !errors.Is(err, errPollerClosed) {
		t.Fatalf("RegisterFD after Close error = %v, want %v", err, errPollerClosed)
	}
}

func TestPollSnapshotRetainsZeroInterestAndGeneration(t *testing.T) {
	poller := testPoller(t)
	if err := poller.RegisterFD(21, EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	if err := poller.RegisterFD(22, EventWrite, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	if err := poller.ModifyFD(21, 0); err != nil {
		t.Fatal(err)
	}
	poller.lifecycleMu.Lock()
	poller.fdMu.Lock()
	poller.rebuildPollSnapshotLocked()
	poller.fdMu.Unlock()
	poller.lifecycleMu.Unlock()
	if len(poller.pollFDs) != 3 || len(poller.pollTokens) != 3 || poller.pollTokens[0] != 0 {
		t.Fatalf("snapshot sizes/tokens = (%d, %d, %v), want control plus two registrations", len(poller.pollFDs), len(poller.pollTokens), poller.pollTokens)
	}
	seenZero := false
	for index := 1; index < len(poller.pollFDs); index++ {
		if poller.pollFDs[index].Events == 0 {
			seenZero = true
		}
		if poller.pollTokens[index] == 0 {
			t.Fatalf("registration token %d is zero", index)
		}
	}
	if !seenZero {
		t.Fatal("zero-interest registration was omitted from poll snapshot")
	}
}

func TestPollRegisterSignalFailureRollsBackOwnedDescriptor(t *testing.T) {
	poller := testPoller(t)
	signalErr := errors.New("forced register control failure")
	poller.lifecycleMu.Lock()
	poller.nativePolling = true
	poller.lifecycleMu.Unlock()
	poller.controlWrite = func(int, []byte) (int, error) { return 0, signalErr }
	closed := 0
	poller.descriptorClose = func(fd int) error {
		if fd == 101 {
			closed++
		}
		return nil
	}

	if err := poller.RegisterFD(21, EventRead, func(IOEvents) {}); !errors.Is(err, signalErr) {
		t.Fatalf("RegisterFD error = %v, want %v", err, signalErr)
	}
	poller.lifecycleMu.Lock()
	poller.nativePolling = false
	poller.lifecycleMu.Unlock()
	if poller.userFDRegistered(21) {
		t.Fatal("failed RegisterFD retained registration ownership")
	}
	if closed != 1 {
		t.Fatalf("failed RegisterFD duplicate closes = %d, want 1", closed)
	}
}

func TestPollUnregisterSignalFailureReleasesOwnership(t *testing.T) {
	poller := testPoller(t)
	if err := poller.RegisterFD(21, EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	signalErr := errors.New("forced unregister control failure")
	poller.lifecycleMu.Lock()
	poller.nativePolling = true
	poller.lifecycleMu.Unlock()
	poller.controlWrite = func(int, []byte) (int, error) { return 0, signalErr }
	closed := 0
	poller.descriptorClose = func(fd int) error {
		if fd == 101 {
			closed++
		}
		return nil
	}

	err := poller.UnregisterFD(21)
	poller.lifecycleMu.Lock()
	poller.nativePolling = false
	poller.lifecycleMu.Unlock()
	var unregisterErr *FDUnregisterError
	if !errors.As(err, &unregisterErr) || !unregisterErr.Released() || !errors.Is(err, signalErr) {
		t.Fatalf("UnregisterFD error = %v, want released error wrapping %v", err, signalErr)
	}
	if poller.userFDRegistered(21) || closed != 1 {
		t.Fatalf("failed UnregisterFD ownership = (registered %v, closes %d), want (false, 1)", poller.userFDRegistered(21), closed)
	}
}

func TestPollLatchedLoopWakeSkipsPrivateControl(t *testing.T) {
	poller := testPoller(t)
	if err := poller.RegisterFD(21, EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	poller.lifecycleMu.Lock()
	poller.nativePolling = true
	poller.lifecycleMu.Unlock()
	writes := 0
	poller.controlWrite = func(_ int, buffer []byte) (int, error) {
		writes++
		return len(buffer), nil
	}
	if err := poller.unregisterFD(21, true); err != nil {
		t.Fatal(err)
	}
	poller.lifecycleMu.Lock()
	poller.nativePolling = false
	poller.lifecycleMu.Unlock()
	if writes != 0 {
		t.Fatalf("private control writes with latched loop wake = %d, want 0", writes)
	}
}

func TestPollCloseSignalFailureStillReleasesEveryDescriptor(t *testing.T) {
	poller := testPoller(t)
	if err := poller.RegisterFD(21, EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	signalErr := errors.New("forced close control failure")
	poller.lifecycleMu.Lock()
	poller.nativePolling = true
	poller.lifecycleMu.Unlock()
	poller.controlWrite = func(int, []byte) (int, error) { return 0, signalErr }
	closed := make(map[int]int)
	poller.descriptorClose = func(fd int) error {
		closed[fd]++
		return nil
	}

	if err := poller.Close(); !errors.Is(err, signalErr) {
		t.Fatalf("Close error = %v, want %v", err, signalErr)
	}
	for _, fd := range []int{10, 11, 101} {
		if closed[fd] != 1 {
			t.Errorf("descriptor %d close calls = %d, want 1", fd, closed[fd])
		}
	}
	if poller.controlReadFD != -1 || poller.controlWriteFD != -1 || poller.userFDRegistered(21) {
		t.Fatalf("Close retained poll resources = (control %d/%d, registered %v)", poller.controlReadFD, poller.controlWriteFD, poller.userFDRegistered(21))
	}
}

func TestPollControlDescriptorFailuresReachDispatch(t *testing.T) {
	tests := []struct {
		name    string
		revents pollEventMask
		readErr error
	}{
		{name: "error", revents: pollEventMask(unix.POLLERR)},
		{name: "hangup", revents: pollEventMask(unix.POLLHUP)},
		{name: "invalid", revents: pollEventMask(unix.POLLNVAL)},
		{name: "drain failure", revents: pollEventMask(unix.POLLIN), readErr: unix.EBADF},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			poller := testPoller(t)
			poller.fdMu.Lock()
			poller.rebuildPollSnapshotLocked()
			poller.pollFDs[0].Revents = test.revents
			poller.fdMu.Unlock()
			if test.readErr != nil {
				poller.controlRead = func(int, []byte) (int, error) { return 0, test.readErr }
			}
			err := func() error {
				_, err := poller.dispatchPollDescriptors(1)
				return err
			}()
			if !errors.Is(err, errPollControlDescriptor) {
				t.Fatalf("dispatch error = %v, want %v", err, errPollControlDescriptor)
			}
			if test.readErr != nil && !errors.Is(err, test.readErr) {
				t.Fatalf("dispatch error = %v, want %v", err, test.readErr)
			}
		})
	}
}

func TestPollModifySignalFailurePreservesConcurrentReadiness(t *testing.T) {
	poller := testPoller(t)
	if err := poller.RegisterFD(21, EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	afterPoll := make(chan struct{})
	allowConversion := make(chan struct{})
	releaseAfterPoll := contractRelease(t, afterPoll)
	releaseConversion := contractRelease(t, allowConversion)
	descriptorCount := make(chan int, 1)
	descriptorCountErr := errors.New("unexpected poll descriptor count")
	poller.pollWait = func(descriptors []unix.PollFd, _ int) (int, error) {
		descriptorCount <- len(descriptors)
		if len(descriptors) != 2 {
			return 0, descriptorCountErr
		}
		descriptors[1].Revents = pollEventMask(unix.POLLIN)
		return 1, nil
	}
	poller.afterNativePoll = func() {
		releaseAfterPoll()
		<-allowConversion
	}
	signalEntered := make(chan struct{})
	allowSignalFailure := make(chan struct{})
	releaseSignalEntered := contractRelease(t, signalEntered)
	releaseSignalFailure := contractRelease(t, allowSignalFailure)
	signalErr := errors.New("forced control write failure")
	poller.controlWrite = func(int, []byte) (int, error) {
		releaseSignalEntered()
		<-allowSignalFailure
		return 0, signalErr
	}
	pollDone := make(chan error, 1)
	go func() {
		_, err := poller.PollIO(1000)
		pollDone <- err
	}()
	waitContractSignal(t, afterPoll, "poll result before failed ModifyFD")
	if count := waitContractValue(t, descriptorCount, "poll descriptor count"); count != 2 {
		releaseConversion()
		if err := waitContractValue(t, pollDone, "invalid-descriptor PollIO completion"); !errors.Is(err, descriptorCountErr) {
			t.Fatalf("PollIO error = %v, want %v", err, descriptorCountErr)
		}
		t.Fatalf("poll descriptor count = %d, want control plus registration", count)
	}
	modifyDone := make(chan error, 1)
	go func() { modifyDone <- poller.ModifyFD(21, EventWrite) }()
	waitContractSignal(t, signalEntered, "ModifyFD control signal")
	poller.fdMu.RLock()
	info, active := poller.fdInfoLocked(21)
	poller.fdMu.RUnlock()
	if !active || info.events != EventRead {
		t.Fatalf("state during control signal = (%v, %v), want active read", active, info.events)
	}

	releaseConversion()
	poller.resourceMu.Lock()
	poller.resourceMu.Unlock()
	events := poller.readyEventsSnapshot()
	if len(events) != 1 || events[0].fd != 21 || events[0].events != EventRead {
		t.Fatalf("ready events after old-mask conversion = %+v, want fd 21 read", events)
	}

	releaseSignalFailure()
	if err := waitContractValue(t, modifyDone, "failed ModifyFD completion"); !errors.Is(err, signalErr) {
		t.Fatalf("ModifyFD error = %v, want %v", err, signalErr)
	}
	if err := waitContractValue(t, pollDone, "PollIO completion after failed ModifyFD"); err != nil {
		t.Fatal(err)
	}
	poller.fdMu.RLock()
	info, active = poller.fdInfoLocked(21)
	poller.fdMu.RUnlock()
	if !active || info.events != EventRead {
		t.Fatalf("state after failed control signal = (%v, %v), want active read", active, info.events)
	}
}

func TestPollModifySignalsOldSnapshotBeforePublishingNewMask(t *testing.T) {
	poller := testPoller(t)
	if err := poller.RegisterFD(21, EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	pollStarted := make(chan struct{})
	pollRelease := make(chan struct{})
	releasePollStarted := contractRelease(t, pollStarted)
	releasePoll := contractRelease(t, pollRelease)
	poller.beforeNativePoll = releasePollStarted
	poller.pollWait = func(descriptors []unix.PollFd, _ int) (int, error) {
		<-pollRelease
		descriptors[0].Revents = pollEventMask(unix.POLLIN)
		return 1, nil
	}
	poller.controlWrite = func(_ int, buffer []byte) (int, error) {
		poller.fdMu.RLock()
		info, active := poller.fdInfoLocked(21)
		poller.fdMu.RUnlock()
		if !active || info.events != EventRead {
			t.Errorf("state during control signal = (%v, %v), want active read", active, info.events)
		}
		releasePoll()
		poller.resourceMu.Lock()
		poller.resourceMu.Unlock()
		poller.fdMu.RLock()
		info, active = poller.fdInfoLocked(21)
		poller.fdMu.RUnlock()
		if !active || info.events != EventRead {
			t.Errorf("state after old snapshot retirement = (%v, %v), want active read", active, info.events)
		}
		return len(buffer), nil
	}
	pollDone := make(chan error, 1)
	go func() {
		_, err := poller.PollIO(1000)
		pollDone <- err
	}()
	waitContractSignal(t, pollStarted, "poll before successful ModifyFD")
	if err := poller.ModifyFD(21, EventWrite); err != nil {
		t.Fatal(err)
	}
	if err := waitContractValue(t, pollDone, "PollIO completion after successful ModifyFD"); err != nil {
		t.Fatal(err)
	}
	poller.fdMu.RLock()
	info, active := poller.fdInfoLocked(21)
	poller.fdMu.RUnlock()
	if !active || info.events != EventWrite {
		t.Fatalf("state after successful control signal = (%v, %v), want active write", active, info.events)
	}

	poller.beforeNativePoll = nil
	var nextMask pollEventMask
	poller.pollWait = func(descriptors []unix.PollFd, _ int) (int, error) {
		if len(descriptors) == 2 {
			nextMask = descriptors[1].Events
		}
		return 0, nil
	}
	if _, err := poller.PollIO(0); err != nil {
		t.Fatal(err)
	}
	if nextMask != pollEventMask(unix.POLLOUT) {
		t.Fatalf("next poll interest mask = %#x, want POLLOUT", nextMask)
	}
}

func TestPollUnregisterJoinsResultConversionBeforeClose(t *testing.T) {
	poller := testPoller(t)
	if err := poller.RegisterFD(21, EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	afterPoll := make(chan struct{})
	allowConversion := make(chan struct{})
	releaseAfterPoll := contractRelease(t, afterPoll)
	releaseConversion := contractRelease(t, allowConversion)
	poller.pollWait = func(descriptors []unix.PollFd, _ int) (int, error) {
		for index := range descriptors {
			if index != 0 {
				descriptors[index].Revents = pollEventMask(unix.POLLIN)
				return 1, nil
			}
		}
		return 0, nil
	}
	poller.afterNativePoll = func() {
		releaseAfterPoll()
		<-allowConversion
	}
	retired := make(chan struct{})
	releaseRetired := contractRelease(t, retired)
	poller.controlWrite = func(_ int, buffer []byte) (int, error) {
		releaseRetired()
		return len(buffer), nil
	}
	var duplicateClosed atomic.Bool
	poller.descriptorClose = func(fd int) error {
		if fd == 101 {
			duplicateClosed.Store(true)
		}
		return nil
	}
	pollDone := make(chan error, 1)
	go func() {
		_, err := poller.PollIO(1000)
		pollDone <- err
	}()
	waitContractSignal(t, afterPoll, "poll result before unregister")
	unregisterDone := make(chan error, 1)
	go func() { unregisterDone <- poller.UnregisterFD(21) }()
	waitContractSignal(t, retired, "unregister control signal")
	poller.fdMu.RLock()
	_, active := poller.fdInfoLocked(21)
	poller.fdMu.RUnlock()
	if active {
		t.Fatal("control signal preceded local unregister publication")
	}
	if duplicateClosed.Load() {
		t.Fatal("owned duplicate closed while native result conversion retained it")
	}
	releaseConversion()
	if err := waitContractValue(t, unregisterDone, "UnregisterFD completion"); err != nil {
		t.Fatal(err)
	}
	if !duplicateClosed.Load() {
		t.Fatal("owned duplicate was not closed after result conversion")
	}
	if err := waitContractValue(t, pollDone, "PollIO completion after unregister"); err != nil {
		t.Fatal(err)
	}
	if events := poller.readyEventsSnapshot(); len(events) != 0 {
		t.Fatalf("unregistered generation produced ready events: %+v", events)
	}
}

func TestPollCloseSignalsAndJoinsIndefiniteWait(t *testing.T) {
	poller := testPoller(t)
	pollStarted := make(chan struct{})
	pollRelease := make(chan struct{})
	var releaseOnce sync.Once
	releasePollStarted := contractRelease(t, pollStarted)
	releasePoll := contractRelease(t, pollRelease)
	poller.beforeNativePoll = releasePollStarted
	poller.controlWrite = func(_ int, buffer []byte) (int, error) {
		releaseOnce.Do(releasePoll)
		return len(buffer), nil
	}
	poller.pollWait = func(descriptors []unix.PollFd, _ int) (int, error) {
		<-pollRelease
		descriptors[0].Revents = pollEventMask(unix.POLLIN)
		return 1, nil
	}
	pollDone := make(chan error, 1)
	go func() {
		_, err := poller.PollIO(-1)
		pollDone <- err
	}()
	waitContractSignal(t, pollStarted, "indefinite poll before Close")
	if err := poller.Close(); err != nil {
		t.Fatal(err)
	}
	if err := waitContractValue(t, pollDone, "PollIO completion after Close"); !errors.Is(err, errPollerClosed) {
		t.Fatalf("PollIO after Close = %v, want %v", err, errPollerClosed)
	}
}

func TestPollZeroValueCloseDoesNotCloseDescriptorZero(t *testing.T) {
	var poller fastPoller
	called := false
	poller.descriptorClose = func(int) error {
		called = true
		return nil
	}
	if err := poller.Close(); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("zero-value Close attempted descriptor cleanup")
	}
}
