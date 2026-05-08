//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestRegisterFDPublicationPrecedesCallback(t *testing.T) {
	loop := New()
	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	if _, err := unix.Write(pipeFDs[1], []byte{1}); err != nil {
		t.Fatal(err)
	}

	returnHookEntered := make(chan struct{}, 1)
	callbackWaiting := make(chan struct{}, 1)
	releaseReturnHook := make(chan struct{})
	var callbackWaitingOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeRegisterFDReturn: func(fd int) {
			if fd == pipeFDs[0] {
				returnHookEntered <- struct{}{}
				<-releaseReturnHook
			}
		},
		BeforeFDPublicationCheck: func(fd int) {
			if fd == pipeFDs[0] {
				callbackWaitingOnce.Do(func() { callbackWaiting <- struct{}{} })
			}
		},
	}

	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	registerActiveLoopCleanupT(t, loop, runDone)
	releaseReturn := releaseSignalT(t, releaseReturnHook)
	waitLoopOwnerTurnT(t, loop)

	callbackRan := make(chan struct{}, 1)
	registerResult := make(chan error, 1)
	go func() {
		registerResult <- loop.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {
			select {
			case callbackRan <- struct{}{}:
			default:
			}
		})
	}()

	select {
	case <-returnHookEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("RegisterFD did not reach its pre-return publication hook")
	}
	select {
	case <-callbackWaiting:
	case <-time.After(5 * time.Second):
		t.Fatal("ready callback did not reach its publication wait")
	}
	select {
	case err := <-registerResult:
		t.Fatalf("RegisterFD returned before publication release: %v", err)
	default:
	}
	select {
	case <-callbackRan:
		t.Fatal("callback entered before RegisterFD released publication")
	default:
	}

	releaseReturn()
	select {
	case err := <-registerResult:
		if err != nil {
			t.Fatalf("RegisterFD = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RegisterFD did not return after publication release")
	}
	select {
	case <-callbackRan:
	case <-time.After(5 * time.Second):
		t.Fatal("callback did not run after publication release")
	}
	if err := loop.UnregisterFD(pipeFDs[0]); err != nil {
		t.Fatalf("UnregisterFD: %v", err)
	}
}

func TestUnregisterFDWhileRegistrationUnpublishedSuppressesCallback(t *testing.T) {
	loop := New()
	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	if _, err := unix.Write(pipeFDs[1], []byte{1}); err != nil {
		t.Fatal(err)
	}

	returnHookEntered := make(chan struct{}, 1)
	callbackChecked := make(chan struct{}, 1)
	releaseReturnHook := make(chan struct{})
	var callbackCheckOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeRegisterFDReturn: func(fd int) {
			if fd == pipeFDs[0] {
				returnHookEntered <- struct{}{}
				<-releaseReturnHook
			}
		},
		BeforeFDPublicationCheck: func(fd int) {
			if fd == pipeFDs[0] {
				callbackCheckOnce.Do(func() { callbackChecked <- struct{}{} })
			}
		},
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	registerActiveLoopCleanupT(t, loop, runDone)
	releaseReturn := releaseSignalT(t, releaseReturnHook)
	waitLoopOwnerTurnT(t, loop)

	callbackRan := make(chan struct{}, 1)
	registerResult := make(chan error, 1)
	go func() {
		registerResult <- loop.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {
			select {
			case callbackRan <- struct{}{}:
			default:
			}
		})
	}()
	select {
	case <-returnHookEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("RegisterFD did not reach its pre-return hook")
	}
	select {
	case <-callbackChecked:
	case <-time.After(5 * time.Second):
		t.Fatal("ready event was not checked while registration was unpublished")
	}
	unregisterResult := make(chan error, 1)
	go func() { unregisterResult <- loop.UnregisterFD(pipeFDs[0]) }()
	select {
	case err := <-unregisterResult:
		if err != nil {
			t.Fatalf("UnregisterFD = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("UnregisterFD did not linearize while RegisterFD return hook was held")
	}
	releaseReturn()
	select {
	case err := <-registerResult:
		if err != nil {
			t.Fatalf("RegisterFD = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RegisterFD did not return")
	}

	barrier := make(chan struct{})
	if err := loop.Submit(func() { close(barrier) }); err != nil {
		t.Fatal(err)
	}
	select {
	case <-barrier:
	case <-time.After(5 * time.Second):
		t.Fatal("loop barrier did not execute")
	}
	select {
	case <-callbackRan:
		t.Fatal("callback entered after UnregisterFD removed unpublished registration")
	default:
	}
}

func TestRetainedRegisterFDErrorPublishesBeforeCallback(t *testing.T) {
	loop := New()
	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	if _, err := unix.Write(pipeFDs[1], []byte{1}); err != nil {
		t.Fatal(err)
	}

	rollbackErr := errors.New("injected rollback failure")
	returnHookEntered := make(chan struct{}, 1)
	callbackChecked := make(chan struct{}, 1)
	releaseReturnHook := make(chan struct{})
	var callbackCheckOnce sync.Once
	loop.testHooks = &loopTestHooks{
		BeforeRegisterFDRollbackCheck: func() { loop.beginQuiescing() },
		RegisterFDRollback:            func(int) error { return rollbackErr },
		BeforeRegisterFDReturn: func(fd int) {
			if fd == pipeFDs[0] {
				returnHookEntered <- struct{}{}
				<-releaseReturnHook
			}
		},
		BeforeFDPublicationCheck: func(fd int) {
			if fd == pipeFDs[0] {
				callbackCheckOnce.Do(func() { callbackChecked <- struct{}{} })
			}
		},
	}
	runDone := make(chan error, 1)
	go func() { runDone <- loop.Run(context.Background()) }()
	registerActiveLoopCleanupT(t, loop, runDone)
	releaseReturn := releaseSignalT(t, releaseReturnHook)
	waitLoopOwnerTurnT(t, loop)

	callbackRan := make(chan struct{}, 1)
	registerResult := make(chan error, 1)
	go func() {
		registerResult <- loop.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {
			select {
			case callbackRan <- struct{}{}:
			default:
			}
		})
	}()
	select {
	case <-returnHookEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("retained RegisterFD did not reach its pre-return hook")
	}
	select {
	case <-callbackChecked:
	case <-time.After(5 * time.Second):
		t.Fatal("retained ready event was not checked before error publication")
	}
	select {
	case <-callbackRan:
		t.Fatal("retained callback entered before RegisterFD error publication")
	default:
	}
	releaseReturn()
	select {
	case err := <-registerResult:
		var partial *FDRegistrationRollbackError
		if !errors.As(err, &partial) || !partial.Registered() || !errors.Is(err, rollbackErr) {
			t.Fatalf("RegisterFD error = %#v, want retained rollback failure", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("retained RegisterFD did not return")
	}
	select {
	case <-callbackRan:
	case <-time.After(5 * time.Second):
		t.Fatal("retained callback did not run after error publication")
	}
	if err := loop.UnregisterFD(pipeFDs[0]); err != nil {
		t.Fatal(err)
	}
}
