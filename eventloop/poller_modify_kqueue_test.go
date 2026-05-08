//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package eventloop

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"golang.org/x/sys/unix"
)

func TestKqueueModifyInterestsRollsBackPartialNativeChange(t *testing.T) {
	changeErr := errors.New("modify delete failed")
	var calls int
	actual, err := modifyKeventInterestsCall(1, 2, testKeventTag(t), EventRead, EventWrite, func(_ int, changes []unix.Kevent_t) error {
		if len(changes) != 1 {
			t.Fatalf("changes = %d, want 1", len(changes))
		}
		calls++
		if calls == 2 {
			return changeErr
		}
		return nil
	})
	if !errors.Is(err, changeErr) {
		t.Fatalf("Modify interests error = %v, want change error", err)
	}
	if actual != EventRead {
		t.Fatalf("interests after rollback = %v, want %v", actual, EventRead)
	}
	if calls != 3 {
		t.Fatalf("native changes = %d, want add, failed delete, rollback delete", calls)
	}
}

func TestKqueueModifyInterestsReportsRollbackFailureAndActualState(t *testing.T) {
	changeErr := errors.New("modify delete failed")
	rollbackErr := errors.New("rollback delete failed")
	var calls int
	actual, err := modifyKeventInterestsCall(1, 2, testKeventTag(t), EventRead, EventWrite, func(_ int, _ []unix.Kevent_t) error {
		calls++
		switch calls {
		case 2:
			return changeErr
		case 3:
			return rollbackErr
		default:
			return nil
		}
	})
	if !errors.Is(err, changeErr) {
		t.Fatalf("Modify interests error = %v, want change error", err)
	}
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("Modify interests error = %v, want rollback error", err)
	}
	if actual != EventRead|EventWrite {
		t.Fatalf("interests after failed rollback = %v, want %v", actual, EventRead|EventWrite)
	}
	if calls != 3 {
		t.Fatalf("native changes = %d, want add, failed delete, failed rollback delete", calls)
	}
}

func TestKqueueModifyRollbackFailurePreservesIntegratedState(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}
	registerPollerCleanupT(t, &poller)
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &fds[0], &fds[1])
	if err := poller.RegisterFD(fds[0], EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	poller.fdMu.RLock()
	before, _ := poller.fdInfoLocked(fds[0])
	poller.fdMu.RUnlock()
	changeErr := errors.New("injected delete-read failure")
	rollbackErr := errors.New("injected rollback-delete-write failure")
	var calls int
	poller.keventChange = func(kq int, changes []unix.Kevent_t) error {
		calls++
		switch calls {
		case 2:
			return changeErr
		case 3:
			return rollbackErr
		default:
			_, err := unix.Kevent(kq, changes, nil, nil)
			return err
		}
	}
	err = poller.ModifyFD(fds[0], EventWrite)
	if !errors.Is(err, changeErr) || !errors.Is(err, rollbackErr) {
		t.Fatalf("ModifyFD error = %v, want change and rollback errors", err)
	}
	poller.fdMu.RLock()
	after, active := poller.fdInfoLocked(fds[0])
	mappedFD := poller.tokenFDs[before.generation]
	mappedGeneration := poller.kernelTags[before.kernelTag]
	poller.fdMu.RUnlock()
	if !active || after.events != EventRead|EventWrite {
		t.Fatalf("integrated state active=%v events=%v, want true/read|write", active, after.events)
	}
	if after.generation != before.generation || after.kernelTag != before.kernelTag {
		t.Fatal("ModifyFD rollback failure changed registration identity")
	}
	if mappedFD != fds[0] || mappedGeneration != before.generation {
		t.Fatal("ModifyFD rollback failure corrupted token indexes")
	}
	poller.keventChange = nil
	if err := poller.ModifyFD(fds[0], EventRead); err != nil {
		t.Fatalf("repair ModifyFD: %v", err)
	}
	if err := poller.UnregisterFD(fds[0]); err != nil {
		t.Fatalf("cleanup UnregisterFD: %v", err)
	}
}

func TestKqueueModifyInterestsStopsAfterFirstForwardFailure(t *testing.T) {
	changeErr := errors.New("add write failed")
	var calls int
	actual, err := modifyKeventInterestsCall(1, 2, testKeventTag(t), EventRead, EventWrite, func(_ int, _ []unix.Kevent_t) error {
		calls++
		return changeErr
	})
	if !errors.Is(err, changeErr) {
		t.Fatalf("Modify interests error = %v, want change error", err)
	}
	if actual != EventRead {
		t.Fatalf("interests after first forward failure = %v, want %v", actual, EventRead)
	}
	if calls != 1 {
		t.Fatalf("native changes = %d, want only failed add", calls)
	}
}

func TestKqueueModifyInterestsDoesNotRestoreMissingFilter(t *testing.T) {
	var calls int
	actual, err := modifyKeventInterestsCall(1, 2, testKeventTag(t), EventRead, 0, func(_ int, changes []unix.Kevent_t) error {
		calls++
		if calls == 1 {
			if changes[0].Flags&unix.EV_DELETE == 0 {
				t.Fatalf("first flags = %x, want EV_DELETE", changes[0].Flags)
			}
			return unix.ENOENT
		}
		t.Fatal("missing filter must not be restored onto a possibly reused descriptor")
		return nil
	})
	if !errors.Is(err, unix.ENOENT) {
		t.Fatalf("Modify interests error = %v, want ENOENT", err)
	}
	if actual != 0 {
		t.Fatalf("interests after missing filter = %v, want 0", actual)
	}
	if calls != 1 {
		t.Fatalf("native changes = %d, want only failed delete", calls)
	}
}

func TestKqueueModifyInterestsMissingFilterDoesNotRestoreEarlierDeletion(t *testing.T) {
	var calls int
	actual, err := modifyKeventInterestsCall(1, 2, testKeventTag(t), EventRead|EventWrite, 0, func(_ int, changes []unix.Kevent_t) error {
		calls++
		if changes[0].Flags&unix.EV_ADD != 0 {
			t.Fatal("missing filter rollback must not restore an earlier deletion")
		}
		if calls == 2 {
			return unix.ENOENT
		}
		return nil
	})
	if !errors.Is(err, unix.ENOENT) {
		t.Fatalf("Modify interests error = %v, want ENOENT", err)
	}
	if actual != 0 {
		t.Fatalf("interests after mixed delete/missing result = %v, want 0", actual)
	}
	if calls != 2 {
		t.Fatalf("native changes = %d, want two deletes and no restore", calls)
	}
}

func TestKqueueModifyMissingFilterDoesNotAttachReplacement(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}
	registerPollerCleanupT(t, &poller)
	var oldPipe [2]int
	if err := unix.Pipe(oldPipe[:]); err != nil {
		t.Fatal(err)
	}
	oldFD := oldPipe[0]
	registerTestFDCleanupT(t, &oldPipe[0], &oldPipe[1])
	var callbackCalls atomic.Int32
	if err := poller.RegisterFD(oldFD, EventRead, func(IOEvents) { callbackCalls.Add(1) }); err != nil {
		t.Fatal(err)
	}
	if err := unix.Close(oldFD); err != nil {
		t.Fatal(err)
	}
	oldPipe[0] = -1
	var replacement [2]int
	if err := unix.Pipe(replacement[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &replacement[0], &replacement[1])
	if replacement[0] != oldFD {
		if err := unix.Dup2(replacement[0], oldFD); err != nil {
			t.Fatal(err)
		}
		replacementAlias := oldFD
		registerTestFDCleanupT(t, &replacementAlias)
	}
	if err := poller.ModifyFD(oldFD, 0); err != nil {
		t.Fatalf("ModifyFD after numeric reuse: %v", err)
	}
	poller.fdMu.RLock()
	info, active := poller.fdInfoLocked(oldFD)
	poller.fdMu.RUnlock()
	if !active || info.events != 0 {
		t.Fatalf("registration after missing filter active=%v events=%v, want true/0", active, info.events)
	}
	if _, err := unix.Write(replacement[1], []byte{1}); err != nil {
		t.Fatal(err)
	}
	if got, err := poller.PollIO(0); err != nil {
		t.Fatal(err)
	} else if got != 0 {
		t.Fatalf("replacement received %d stale callbacks, want 0", got)
	}
	if got := callbackCalls.Load(); got != 0 {
		t.Fatalf("stale callback calls = %d, want 0", got)
	}
	if err := poller.UnregisterFD(oldFD); err != nil {
		t.Fatalf("cleanup UnregisterFD: %v", err)
	}
}

func TestKqueueModifyAddAfterNumericReuseStaysWithOwnedDescription(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}
	registerPollerCleanupT(t, &poller)
	oldPair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	oldFD := oldPair[0]
	registerTestFDCleanupT(t, &oldPair[0], &oldPair[1])
	if err := poller.RegisterFD(oldFD, EventWrite, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := poller.PollIO(0); err != nil {
		t.Fatal(err)
	}
	poller.clearReadyEvents()
	if err := unix.Close(oldFD); err != nil {
		t.Fatal(err)
	}
	oldPair[0] = -1
	replacement, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &replacement[0], &replacement[1])
	if replacement[0] != oldFD {
		if err := unix.Dup2(replacement[0], oldFD); err != nil {
			t.Fatal(err)
		}
		replacementAlias := oldFD
		registerTestFDCleanupT(t, &replacementAlias)
	}
	if err := poller.ModifyFD(oldFD, EventRead|EventWrite); err != nil {
		t.Fatalf("ModifyFD after numeric reuse: %v", err)
	}
	if _, err := unix.Write(replacement[1], []byte{1}); err != nil {
		t.Fatal(err)
	}
	if _, err := poller.PollIO(0); err != nil {
		t.Fatal(err)
	}
	for _, event := range poller.readyEventsSnapshot() {
		if event.events&EventRead != 0 {
			t.Fatalf("replacement readiness attached to old registration: %v", event.events)
		}
	}
	poller.clearReadyEvents()
	if _, err := unix.Write(oldPair[1], []byte{1}); err != nil {
		t.Fatal(err)
	}
	if got, err := poller.PollIO(1000); err != nil {
		t.Fatal(err)
	} else if got != 1 {
		t.Fatalf("owned description callbacks = %d, want 1", got)
	}
	ready := poller.readyEventsSnapshot()
	if len(ready) != 1 || ready[0].events&EventRead == 0 {
		t.Fatalf("owned description readiness = %#v, want read", ready)
	}
}

func TestKqueueModifyRollbackSuppressesTransientReadiness(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}
	registerPollerCleanupT(t, &poller)
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &fds[0], &fds[1])
	if err := poller.RegisterFD(fds[0], EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	nativeEntered := make(chan struct{})
	nativeReturned := make(chan struct{})
	changeErr := errors.New("injected delete-read failure")
	var calls atomic.Int32
	poller.keventChange = func(kq int, changes []unix.Kevent_t) error {
		switch calls.Add(1) {
		case 1:
			_, err := unix.Kevent(kq, changes, nil, nil)
			<-nativeReturned
			return err
		case 2:
			return changeErr
		default:
			_, err := unix.Kevent(kq, changes, nil, nil)
			return err
		}
	}
	var enterOnce sync.Once
	poller.beforeNativePoll = func() { enterOnce.Do(func() { close(nativeEntered) }) }
	var returnOnce sync.Once
	poller.afterNativePoll = func() { returnOnce.Do(func() { close(nativeReturned) }) }
	pollDone := make(chan struct {
		count int
		err   error
	}, 1)
	go func() {
		count, err := poller.PollIO(1000)
		pollDone <- struct {
			count int
			err   error
		}{count: count, err: err}
	}()
	waitContractSignal(t, nativeEntered, "native poll entry")
	modifyDone := make(chan error, 1)
	go func() { modifyDone <- poller.ModifyFD(fds[0], EventWrite) }()
	if err := waitContractValue(t, modifyDone, "ModifyFD rollback completion"); !errors.Is(err, changeErr) {
		t.Fatalf("ModifyFD = %v, want injected error", err)
	}
	result := waitContractValue(t, pollDone, "transient readiness poll completion")
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.count != 0 {
		t.Fatalf("transient rolled-back readiness produced %d callbacks, want 0", result.count)
	}
}
