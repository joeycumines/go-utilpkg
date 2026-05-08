//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package eventloop

import (
	"errors"
	"os"
	"os/exec"
	"testing"

	"golang.org/x/sys/unix"
)

const pollerZeroValueCloseChild = "GO_EVENTLOOP_POLLER_ZERO_VALUE_CLOSE_CHILD"

func TestFastPollerZeroValueClosePreservesDescriptorZero(t *testing.T) {
	if os.Getenv(pollerZeroValueCloseChild) == "1" {
		closeAmbientFDT(t, 0)
		var pipeFDs [2]int
		if err := unix.Pipe(pipeFDs[:]); err != nil {
			t.Fatal(err)
		}
		registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
		if pipeFDs[0] != 0 {
			if err := unix.Dup2(pipeFDs[0], 0); err != nil {
				t.Fatal(err)
			}
			standardInputFD := 0
			registerTestFDCleanupT(t, &standardInputFD)
		}
		var poller fastPoller
		if err := poller.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := unix.FcntlInt(0, unix.F_GETFD, 0); err != nil {
			t.Fatalf("zero-value Close closed ambient descriptor zero: %v", err)
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestFastPollerZeroValueClosePreservesDescriptorZero$")
	cmd.Env = append(os.Environ(), pollerZeroValueCloseChild+"=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("zero-value Close subprocess: %v\n%s", err, output)
	}
}

func TestFastPollerCloseInvalidatesOwnedState(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}

	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	if err := poller.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	if !poller.appendReadyEvent(pipeFDs[0], EventRead) {
		t.Fatal("failed to create retained ready event")
	}

	if err := poller.Close(); err != nil {
		t.Fatal(err)
	}
	if poller.initialized.Load() {
		t.Fatal("poller remains initialized after Close")
	}
	if fd := pollerNativeFD(&poller); fd != -1 {
		t.Fatalf("stored poller descriptor after Close = %d, want -1", fd)
	}
	if poller.fds != nil || poller.sparseFDs != nil {
		t.Fatal("registration tables remain owned after Close")
	}
	if len(poller.readyEventsSnapshot()) != 0 {
		t.Fatal("ready callbacks remain owned after Close")
	}
	if err := poller.Close(); err != nil {
		t.Fatalf("repeated Close: %v", err)
	}
}

func TestFastPollerInitCloseForcedLinearization(t *testing.T) {
	t.Run("init owns lifecycle first", func(t *testing.T) {
		var poller fastPoller
		initEntered := make(chan struct{})
		releaseInit := make(chan struct{})
		release := contractRelease(t, releaseInit)
		poller.pollerCreate = func() (int, error) {
			close(initEntered)
			<-releaseInit
			return pollerCreateNative()
		}
		initResult := make(chan error, 1)
		go func() { initResult <- poller.Init() }()
		waitContractSignal(t, initEntered, "Init native creation")

		closeStarted := make(chan struct{})
		closeResult := make(chan error, 1)
		go func() {
			close(closeStarted)
			closeResult <- poller.Close()
		}()
		waitContractSignal(t, closeStarted, "concurrent Close start")
		release()
		if err := waitContractValue(t, initResult, "Init completion"); err != nil {
			t.Fatalf("Init: %v", err)
		}
		if err := waitContractValue(t, closeResult, "Close completion"); err != nil {
			t.Fatalf("Close: %v", err)
		}
		assertPollerClosedOwnership(t, &poller)
	})

	t.Run("close owns lifecycle first", func(t *testing.T) {
		var poller fastPoller
		closeEntered := make(chan struct{})
		releaseClose := make(chan struct{})
		release := contractRelease(t, releaseClose)
		poller.beforeResourceClose = func() {
			close(closeEntered)
			<-releaseClose
		}
		closeResult := make(chan error, 1)
		go func() { closeResult <- poller.Close() }()
		waitContractSignal(t, closeEntered, "Close resource ownership")

		initStarted := make(chan struct{})
		initResult := make(chan error, 1)
		go func() {
			close(initStarted)
			initResult <- poller.Init()
		}()
		waitContractSignal(t, initStarted, "concurrent Init start")
		release()
		if err := waitContractValue(t, closeResult, "Close completion"); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if err := waitContractValue(t, initResult, "Init completion"); !errors.Is(err, errPollerClosed) {
			t.Fatalf("Init after Close ownership = %v, want errPollerClosed", err)
		}
		assertPollerClosedOwnership(t, &poller)
	})
}

func TestFastPollerInitFailureLeavesRetryableInvalidState(t *testing.T) {
	var poller fastPoller
	initErr := errors.New("injected native poller creation failure")
	poller.pollerCreate = func() (int, error) { return -1, initErr }
	if err := poller.Init(); !errors.Is(err, initErr) {
		t.Fatalf("Init failure = %v, want injected error", err)
	}
	if poller.initialized.Load() || pollerNativeFD(&poller) != -1 {
		t.Fatalf("failed Init state initialized=%v fd=%d, want false/-1", poller.initialized.Load(), pollerNativeFD(&poller))
	}
	if poller.fds != nil || poller.sparseFDs != nil || poller.tokenFDs != nil {
		t.Fatal("failed Init allocated registration ownership")
	}
	poller.pollerCreate = nil
	if err := poller.Init(); err != nil {
		t.Fatalf("retry Init: %v", err)
	}
	if err := poller.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestFastPollerSecondInitPreservesNativeResource(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}
	nativeFD := pollerNativeFD(&poller)
	if err := poller.Init(); !errors.Is(err, errPollerAlreadyInitialized) {
		t.Fatalf("second Init = %v, want errPollerAlreadyInitialized", err)
	}
	if got := pollerNativeFD(&poller); got != nativeFD {
		t.Fatalf("native descriptor after second Init = %d, want %d", got, nativeFD)
	}
	if _, err := unix.FcntlInt(uintptr(nativeFD), unix.F_GETFD, 0); err != nil {
		t.Fatalf("native descriptor after second Init is invalid: %v", err)
	}
	if err := poller.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestFastPollerInitAfterCloseReturnsClosed(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}
	if err := poller.Close(); err != nil {
		t.Fatal(err)
	}
	if err := poller.Init(); !errors.Is(err, errPollerClosed) {
		t.Fatalf("Init after Close = %v, want errPollerClosed", err)
	}
}

func TestFastPollerOperationsAfterCloseReturnClosed(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}
	if err := poller.Close(); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		call func() error
	}{
		{name: "poll", call: func() error { _, err := poller.PollIO(0); return err }},
		{name: "register", call: func() error { return poller.RegisterFD(0, EventRead, func(IOEvents) {}) }},
		{name: "modify", call: func() error { return poller.ModifyFD(0, EventRead) }},
		{name: "unregister", call: func() error { return poller.UnregisterFD(0) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, errPollerClosed) {
				t.Fatalf("operation after Close = %v, want errPollerClosed", err)
			}
		})
	}
}

func TestFastPollerCloseIsIdempotentUnderContention(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	release := contractRelease(t, releaseFirst)
	poller.beforeResourceClose = func() {
		close(firstEntered)
		<-releaseFirst
	}
	firstResult := make(chan error, 1)
	go func() { firstResult <- poller.Close() }()
	waitContractSignal(t, firstEntered, "first Close resource ownership")

	const contenders = 31
	started := make(chan struct{}, contenders)
	results := make(chan error, contenders)
	for range contenders {
		go func() {
			started <- struct{}{}
			results <- poller.Close()
		}()
	}
	for range contenders {
		waitContractSignal(t, started, "contending Close start")
	}
	release()
	if err := waitContractValue(t, firstResult, "first Close completion"); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	for range contenders {
		if err := waitContractValue(t, results, "contending Close completion"); err != nil {
			t.Fatalf("concurrent Close: %v", err)
		}
	}
	assertPollerClosedOwnership(t, &poller)
}

func TestFastPollerCloseWaitsNativePollOwnership(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}
	nativeFD := pollerNativeFD(&poller)
	pollEntered := make(chan struct{})
	releasePoll := make(chan struct{})
	closeEntered := make(chan struct{})
	poller.beforeNativePoll = func() {
		close(pollEntered)
		<-releasePoll
	}
	poller.beforeResourceClose = func() { close(closeEntered) }
	pollDone := make(chan error, 1)
	go func() {
		_, err := poller.PollIO(0)
		pollDone <- err
	}()
	waitContractSignal(t, pollEntered, "native poll ownership")
	closeDone := make(chan error, 1)
	go func() { closeDone <- poller.Close() }()
	waitContractSignal(t, closeEntered, "Close resource ownership")
	if _, err := unix.FcntlInt(uintptr(nativeFD), unix.F_GETFD, 0); err != nil {
		t.Fatalf("native descriptor closed while poll owned it: %v", err)
	}
	close(releasePoll)
	if err := waitContractValue(t, pollDone, "PollIO completion"); !errors.Is(err, errPollerClosed) {
		t.Fatalf("PollIO after concurrent Close = %v, want errPollerClosed", err)
	}
	if err := waitContractValue(t, closeDone, "Close completion"); err != nil {
		t.Fatalf("Close after poll release: %v", err)
	}
	requireDescriptorClosed(t, nativeFD)
}

func TestFastPollerCloseWaitsNativeEventResolution(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}
	nativeFD := pollerNativeFD(&poller)
	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])
	if err := poller.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := unix.Write(pipeFDs[1], []byte{1}); err != nil {
		t.Fatal(err)
	}
	nativeReturned := make(chan struct{})
	releaseResolution := make(chan struct{})
	closeEntered := make(chan struct{})
	poller.afterNativePoll = func() {
		close(nativeReturned)
		<-releaseResolution
	}
	poller.beforeResourceClose = func() { close(closeEntered) }
	pollDone := make(chan error, 1)
	go func() {
		_, err := poller.PollIO(0)
		pollDone <- err
	}()
	waitContractSignal(t, nativeReturned, "native event return")
	closeDone := make(chan error, 1)
	go func() { closeDone <- poller.Close() }()
	waitContractSignal(t, closeEntered, "Close resource ownership")
	if _, err := unix.FcntlInt(uintptr(nativeFD), unix.F_GETFD, 0); err != nil {
		t.Fatalf("native descriptor closed before event resolution: %v", err)
	}
	close(releaseResolution)
	if err := waitContractValue(t, pollDone, "PollIO completion"); !errors.Is(err, errPollerClosed) {
		t.Fatalf("PollIO after concurrent Close = %v, want errPollerClosed", err)
	}
	if err := waitContractValue(t, closeDone, "Close completion"); err != nil {
		t.Fatalf("Close after event resolution: %v", err)
	}
	requireDescriptorClosed(t, nativeFD)
}

func assertPollerClosedOwnership(t *testing.T, poller *fastPoller) {
	t.Helper()
	if poller.initialized.Load() {
		t.Fatal("poller remains initialized after Close")
	}
	if fd := pollerNativeFD(poller); fd != -1 {
		t.Fatalf("stored poller descriptor after Close = %d, want -1", fd)
	}
	if poller.fds != nil || poller.sparseFDs != nil || poller.tokenFDs != nil {
		t.Fatal("poller retains registration ownership after Close")
	}
}
