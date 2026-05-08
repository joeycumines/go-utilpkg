//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package eventloop

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

func TestKqueueRegisterInterestsStopsAfterFirstFailure(t *testing.T) {
	changeErr := errors.New("add read failed")
	var calls int
	actual, err := applyKeventInterestsCall(1, 2, testKeventTag(t), 0, EventRead|EventWrite, false, func(_ int, _ []unix.Kevent_t) error {
		calls++
		return changeErr
	})
	if !errors.Is(err, changeErr) {
		t.Fatalf("Register interests error = %v, want change error", err)
	}
	if actual != 0 {
		t.Fatalf("interests after first registration failure = %v, want 0", actual)
	}
	if calls != 1 {
		t.Fatalf("native changes = %d, want only failed read add", calls)
	}
}

func TestKqueuePartialRegistrationHonorsTerminalWinner(t *testing.T) {
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
	changeErr := errors.New("injected second-filter failure")
	rollbackErr := errors.New("injected registration rollback failure")
	var calls int
	loop.poller.keventChange = func(_ int, _ []unix.Kevent_t) error {
		calls++
		switch calls {
		case 2:
			loop.beginQuiescing()
			return changeErr
		case 3:
			return rollbackErr
		default:
			return nil
		}
	}

	err := loop.RegisterFD(pipeFDs[0], EventRead|EventWrite, func(IOEvents) {})
	if !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("RegisterFD error = %v, want ErrLoopTerminated", err)
	}
	if !errors.Is(err, changeErr) || !errors.Is(err, rollbackErr) {
		t.Fatalf("RegisterFD error = %v, want native change and rollback errors", err)
	}
	var partial *FDRegistrationRollbackError
	if !errors.As(err, &partial) || partial.Registered() {
		t.Fatalf("RegisterFD error = %#v, want final Registered=false", err)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount after terminal cleanup = %d, want 0", got)
	}
	if loop.poller.userFDRegistered(pipeFDs[0]) {
		t.Fatal("terminal winner left partial Kqueue registration owned")
	}
	if calls != 4 {
		t.Fatalf("native changes = %d, want two forward, failed rollback, terminal cleanup", calls)
	}
}

func TestKqueuePartialRegistrationRetainsOwnedNativeFilter(t *testing.T) {
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
	changeErr := errors.New("injected second-filter failure")
	rollbackErr := errors.New("injected rollback failure")
	var calls int
	var nativeIdents []uint64
	loop.poller.keventChange = func(kq int, changes []unix.Kevent_t) error {
		calls++
		if len(changes) != 1 {
			t.Fatalf("native changes = %d, want 1", len(changes))
		}
		nativeIdents = append(nativeIdents, uint64(keventIdent(&changes[0])))
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
	err := loop.RegisterFD(pipeFDs[0], EventRead|EventWrite, func(IOEvents) {})
	var partial *FDRegistrationRollbackError
	if !errors.As(err, &partial) || !partial.Registered() {
		t.Fatalf("RegisterFD error = %#v, want retained partial ownership", err)
	}
	if !errors.Is(err, changeErr) || !errors.Is(err, rollbackErr) {
		t.Fatalf("RegisterFD error = %v, want change and rollback errors", err)
	}
	if got := loop.userIOFDCount.Load(); got != 1 {
		t.Fatalf("userIOFDCount = %d, want 1", got)
	}
	loop.poller.fdMu.RLock()
	info, active := loop.poller.fdInfoLocked(pipeFDs[0])
	loop.poller.fdMu.RUnlock()
	if !active || info.events != EventRead {
		t.Fatalf("partial registration active=%v events=%v, want true/read", active, info.events)
	}
	if info.pollFD == pipeFDs[0] {
		t.Fatalf("owned native descriptor = caller descriptor %d", info.pollFD)
	}
	for call, ident := range nativeIdents {
		if ident != uint64(info.pollFD) {
			t.Fatalf("native change %d ident = %d, want owned descriptor %d", call+1, ident, info.pollFD)
		}
	}
	loop.poller.keventChange = nil
	if err := loop.UnregisterFD(pipeFDs[0]); err != nil {
		t.Fatalf("cleanup UnregisterFD: %v", err)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount after cleanup = %d, want 0", got)
	}
}
