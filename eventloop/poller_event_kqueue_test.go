//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package eventloop

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"

	"golang.org/x/sys/unix"
)

const pollerFDZeroChild = "GO_EVENTLOOP_POLLER_FD_ZERO_CHILD"

func TestKqueueUnregisterFailureRetainsOwnership(t *testing.T) {
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
	sentinel := errors.New("injected retained kqueue delete failure")
	loop.poller.keventChange = func(int, []unix.Kevent_t) error { return sentinel }
	err := loop.UnregisterFD(pipeFDs[0])
	var unregisterErr *FDUnregisterError
	if !errors.As(err, &unregisterErr) || unregisterErr.Released() || !errors.Is(err, sentinel) {
		t.Fatalf("UnregisterFD error = %v, want retained ownership failure", err)
	}
	if got := loop.userIOFDCount.Load(); got != 1 {
		t.Fatalf("userIOFDCount = %d, want 1 after retained ownership", got)
	}
	if !loop.poller.userFDRegistered(pipeFDs[0]) {
		t.Fatal("registration was retired after native delete failure")
	}
	loop.poller.keventChange = nil
	if err := loop.UnregisterFD(pipeFDs[0]); err != nil {
		t.Fatalf("cleanup UnregisterFD: %v", err)
	}
}

func TestFastPollerKqueueClosesDescriptorZero(t *testing.T) {
	if os.Getenv(pollerFDZeroChild) == "1" {
		closeAmbientFDT(t, 0)
		var poller fastPoller
		if err := poller.Init(); err != nil {
			t.Fatal(err)
		}
		if got := pollerNativeFD(&poller); got != 0 {
			t.Fatalf("kqueue descriptor = %d, want 0", got)
		}
		if err := poller.Close(); err != nil {
			t.Fatal(err)
		}
		requireDescriptorClosed(t, 0)
		if got := pollerNativeFD(&poller); got != -1 {
			t.Fatalf("stored kqueue descriptor = %d, want -1", got)
		}
		var replacement [2]int
		if err := unix.Pipe(replacement[:]); err != nil {
			t.Fatal(err)
		}
		registerTestFDCleanupT(t, &replacement[0], &replacement[1])
		if replacement[0] != 0 {
			if err := unix.Dup2(replacement[0], 0); err != nil {
				t.Fatal(err)
			}
			replacementAlias := 0
			registerTestFDCleanupT(t, &replacementAlias)
		}
		if err := poller.Close(); err != nil {
			t.Fatalf("repeated Close: %v", err)
		}
		if _, err := unix.FcntlInt(0, unix.F_GETFD, 0); err != nil {
			t.Fatalf("repeated Close closed descriptor-zero replacement: %v", err)
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestFastPollerKqueueClosesDescriptorZero$")
	cmd.Env = append(os.Environ(), pollerFDZeroChild+"=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fd-zero subprocess: %v\n%s", err, output)
	}
}

func TestKqueueNativeEventDoesNotInheritReusedFDRegistration(t *testing.T) {
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
	if err := poller.RegisterFD(oldFD, EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	nativeReturned := make(chan struct{})
	releaseResolution := make(chan struct{})
	releaseResolutionNow := contractRelease(t, releaseResolution)
	recycleEntered := make(chan struct{})
	var returnOnce sync.Once
	poller.afterNativePoll = func() {
		returnOnce.Do(func() { close(nativeReturned) })
		<-releaseResolution
	}
	var recycleOnce sync.Once
	poller.beforeTagRecycle = func() { recycleOnce.Do(func() { close(recycleEntered) }) }
	if _, err := unix.Write(oldPipe[1], []byte{1}); err != nil {
		t.Fatal(err)
	}
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
	waitContractSignal(t, nativeReturned, "native event return")
	unregisterDone := make(chan error, 1)
	go func() { unregisterDone <- poller.UnregisterFD(oldFD) }()
	waitContractSignal(t, recycleEntered, "retired tag recycle")
	releaseResolutionNow()
	result := waitContractValue(t, pollDone, "stale native event resolution")
	if result.err != nil {
		t.Fatal(result.err)
	}
	if result.count != 0 {
		t.Fatalf("retired native event produced %d ready callbacks, want 0", result.count)
	}
	if err := waitContractValue(t, unregisterDone, "stale registration retirement"); err != nil {
		t.Fatalf("retire old registration: %v", err)
	}
	poller.afterNativePoll = nil
	poller.beforeTagRecycle = nil
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
	if err := poller.RegisterFD(oldFD, EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
}

func TestKqueueReadWriteReadinessCoalescesOneCallback(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &fds[0], &fds[1])
	loop := New()
	registerLoopCleanupT(t, loop)
	var callbackCalls atomic.Int32
	var callbackEvents atomic.Uint32
	if err := loop.RegisterFD(fds[0], EventRead|EventWrite, func(events IOEvents) {
		callbackCalls.Add(1)
		callbackEvents.Store(uint32(events))
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := unix.Write(fds[1], []byte{1}); err != nil {
		t.Fatal(err)
	}
	if got, err := loop.poller.PollIO(1000); err != nil {
		t.Fatal(err)
	} else if got < 1 {
		t.Fatalf("ready callback count = %d, want at least one user event", got)
	}
	ready := append([]pollEvent(nil), loop.poller.readyEventsSnapshot()...)
	var userReady []pollEvent
	for _, event := range ready {
		if event.fd == fds[0] {
			userReady = append(userReady, event)
		}
	}
	if len(userReady) != 1 {
		t.Fatalf("user ready events = %d, want 1", len(userReady))
	}
	if got := userReady[0].events; got&EventRead == 0 || got&EventWrite == 0 {
		t.Fatalf("coalesced events = %v, want read|write", got)
	}
	loop.dispatchPollEvents(userReady)
	if got := callbackCalls.Load(); got != 1 {
		t.Fatalf("callback calls = %d, want 1", got)
	}
	if got := IOEvents(callbackEvents.Load()); got&EventRead == 0 || got&EventWrite == 0 {
		t.Fatalf("callback events = %v, want read|write", got)
	}
}

func TestKqueueSameTokenCoalescesEOFAndErrorFlags(t *testing.T) {
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
	if err := poller.RegisterFD(fds[0], EventRead|EventWrite, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	poller.fdMu.RLock()
	info, _ := poller.fdInfoLocked(fds[0])
	poller.fdMu.RUnlock()
	poller.eventBuf[0] = newKevent(fds[0], unix.EVFILT_READ, unix.EV_EOF, 0, 0, info.kernelTag)
	poller.eventBuf[1] = newKevent(fds[0], unix.EVFILT_WRITE, unix.EV_ERROR, 0, 0, info.kernelTag)
	if got := poller.dispatchEvents(2); got != 1 {
		t.Fatalf("dispatchEvents = %d, want one coalesced callback", got)
	}
	ready := poller.readyEventsSnapshot()
	if len(ready) != 1 {
		t.Fatalf("ready events = %d, want 1", len(ready))
	}
	want := EventRead | EventWrite | EventError | EventHangup
	if ready[0].events != want {
		t.Fatalf("coalesced events = %v, want %v", ready[0].events, want)
	}
}
