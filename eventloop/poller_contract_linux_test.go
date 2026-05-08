//go:build linux

package eventloop

import (
	"errors"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"

	"golang.org/x/sys/unix"
)

const pollerFDZeroChild = "GO_EVENTLOOP_POLLER_FD_ZERO_CHILD"

func pollerNativeFD(p *fastPoller) int { return int(p.epfd) }

func pollerCreateNative() (int, error) { return unix.EpollCreate1(unix.EPOLL_CLOEXEC) }

func TestLinuxUnregisterFailureRetainsOwnership(t *testing.T) {
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
	sentinel := errors.New("injected retained epoll delete failure")
	loop.poller.epollCtl = func(epfd, operation, fd int, event *unix.EpollEvent) error {
		if operation == unix.EPOLL_CTL_DEL {
			return sentinel
		}
		return unix.EpollCtl(epfd, operation, fd, event)
	}
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
	loop.poller.epollCtl = nil
	if err := loop.UnregisterFD(pipeFDs[0]); err != nil {
		t.Fatalf("cleanup UnregisterFD: %v", err)
	}
}

func TestFastPollerLinuxClosesDescriptorZero(t *testing.T) {
	if os.Getenv(pollerFDZeroChild) == "1" {
		closeAmbientFDT(t, 0)
		var poller fastPoller
		if err := poller.Init(); err != nil {
			t.Fatal(err)
		}
		if got := pollerNativeFD(&poller); got != 0 {
			t.Fatalf("epoll descriptor = %d, want 0", got)
		}
		if err := poller.Close(); err != nil {
			t.Fatal(err)
		}
		requireDescriptorClosed(t, 0)
		if got := pollerNativeFD(&poller); got != -1 {
			t.Fatalf("stored epoll descriptor = %d, want -1", got)
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
	cmd := exec.Command(os.Args[0], "-test.run=^TestFastPollerLinuxClosesDescriptorZero$")
	cmd.Env = append(os.Environ(), pollerFDZeroChild+"=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fd-zero subprocess: %v\n%s", err, output)
	}
}

func TestLinuxNativeEventDoesNotInheritReusedFDRegistration(t *testing.T) {
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
	if _, err := unix.Write(oldPipe[1], []byte{1}); err != nil {
		t.Fatal(err)
	}
	var captured [1]unix.EpollEvent
	n, err := unix.EpollWait(int(poller.epfd), captured[:], 1000)
	if err != nil || n != 1 {
		t.Fatalf("capture old epoll event = (%d, %v), want (1, nil)", n, err)
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
	if err := poller.UnregisterFD(oldFD); err != nil {
		t.Fatalf("retire closed registration after numeric reuse: %v", err)
	}
	if err := poller.RegisterFD(oldFD, EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	poller.eventBuf[0] = captured[0]
	if got := poller.dispatchEvents(1); got != 0 {
		t.Fatalf("stale native event produced %d ready callbacks, want 0", got)
	}
}

func TestLinuxUnregisterClosedDescriptorPurgesDuplicatedOpenDescription(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}
	registerPollerCleanupT(t, &poller)

	var retiredPipe [2]int
	if err := unix.Pipe(retiredPipe[:]); err != nil {
		t.Fatal(err)
	}
	duplicate, err := unix.Dup(retiredPipe[0])
	if err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &retiredPipe[0], &retiredPipe[1], &duplicate)
	registeredFD := retiredPipe[0]
	if err := poller.RegisterFD(registeredFD, EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	if err := unix.Close(registeredFD); err != nil {
		t.Fatal(err)
	}
	retiredPipe[0] = -1
	if err := poller.UnregisterFD(registeredFD); err != nil {
		t.Fatal(err)
	}
	if _, err := unix.Write(retiredPipe[1], []byte{1}); err != nil {
		t.Fatal(err)
	}
	if got, err := poller.PollIO(0); err != nil {
		t.Fatal(err)
	} else if got != 0 {
		t.Fatalf("retired open description produced %d callbacks, want 0", got)
	}
	var raw [1]unix.EpollEvent
	if got, err := unix.EpollWait(int(poller.epfd), raw[:], 0); err != nil {
		t.Fatal(err)
	} else if got != 0 {
		t.Fatalf("retired duplicated open description remains in rebuilt epoll set: %d events", got)
	}

	var livePipe [2]int
	if err := unix.Pipe(livePipe[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &livePipe[0], &livePipe[1])
	if err := poller.RegisterFD(livePipe[0], EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := unix.Write(livePipe[1], []byte{1}); err != nil {
		t.Fatal(err)
	}
	if got, err := poller.PollIO(1000); err != nil {
		t.Fatal(err)
	} else if got != 1 {
		t.Fatalf("live registration callbacks = %d, want 1", got)
	}
}

func TestLinuxUnregisterDescriptorReusedAsRegularFileUsesOwnedWatch(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}
	registerPollerCleanupT(t, &poller)

	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	registeredFD := pipeFDs[0]
	duplicate, err := unix.Dup(registeredFD)
	if err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1], &duplicate)
	if err := poller.RegisterFD(registeredFD, EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	if err := unix.Close(registeredFD); err != nil {
		t.Fatal(err)
	}
	pipeFDs[0] = -1
	file, err := os.CreateTemp(t.TempDir(), "epoll-replacement")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Errorf("close replacement file: %v", err)
		}
	})
	if int(file.Fd()) != registeredFD {
		if err := unix.Dup2(int(file.Fd()), registeredFD); err != nil {
			t.Fatal(err)
		}
		replacementAlias := registeredFD
		registerTestFDCleanupT(t, &replacementAlias)
	}
	if err := poller.UnregisterFD(registeredFD); err != nil {
		t.Fatalf("UnregisterFD after regular-file reuse: %v", err)
	}
	if _, err := unix.Write(pipeFDs[1], []byte{1}); err != nil {
		t.Fatal(err)
	}
	if got, err := poller.PollIO(0); err != nil {
		t.Fatal(err)
	} else if got != 0 {
		t.Fatalf("retired open description produced %d callbacks, want 0", got)
	}
	var raw [1]unix.EpollEvent
	if got, err := unix.EpollWait(int(poller.epfd), raw[:], 0); err != nil {
		t.Fatal(err)
	} else if got != 0 {
		t.Fatalf("retired owned watch remains after unregister: %d events", got)
	}
}

func TestLinuxEpollRebuildDoesNotAttachStaleRegistrationToReusedFD(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}
	registerPollerCleanupT(t, &poller)

	var triggerPipe [2]int
	if err := unix.Pipe(triggerPipe[:]); err != nil {
		t.Fatal(err)
	}
	triggerDuplicate, err := unix.Dup(triggerPipe[0])
	if err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &triggerPipe[0], &triggerPipe[1], &triggerDuplicate)
	if err := poller.RegisterFD(triggerPipe[0], EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}

	var stalePipe [2]int
	if err := unix.Pipe(stalePipe[:]); err != nil {
		t.Fatal(err)
	}
	staleFD := stalePipe[0]
	registerTestFDCleanupT(t, &stalePipe[0], &stalePipe[1])
	var staleCalls int
	if err := poller.RegisterFD(staleFD, EventRead, func(IOEvents) { staleCalls++ }); err != nil {
		t.Fatal(err)
	}

	if err := unix.Close(triggerPipe[0]); err != nil {
		t.Fatal(err)
	}
	triggerFD := triggerPipe[0]
	triggerPipe[0] = -1
	if err := poller.UnregisterFD(triggerFD); err != nil {
		t.Fatal(err)
	}
	var replacement [2]int
	if err := unix.Pipe(replacement[:]); err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &replacement[0], &replacement[1])
	if err := unix.Close(staleFD); err != nil {
		t.Fatal(err)
	}
	stalePipe[0] = -1
	if replacement[0] != staleFD {
		if err := unix.Dup2(replacement[0], staleFD); err != nil {
			t.Fatal(err)
		}
		replacementAlias := staleFD
		registerTestFDCleanupT(t, &replacementAlias)
	}
	poller.lifecycleMu.Lock()
	poller.rebuildNeeded = true
	poller.lifecycleMu.Unlock()
	if got, err := poller.PollIO(0); err != nil {
		t.Fatal(err)
	} else if got != 0 {
		t.Fatalf("rebuild produced %d callbacks, want 0", got)
	}
	if _, err := unix.Write(replacement[1], []byte{1}); err != nil {
		t.Fatal(err)
	}
	if got, err := poller.PollIO(0); err != nil {
		t.Fatal(err)
	} else if got != 0 {
		t.Fatalf("replacement inherited %d stale callbacks, want 0", got)
	}
	if staleCalls != 0 {
		t.Fatalf("stale callback calls = %d, want 0", staleCalls)
	}
	if err := poller.UnregisterFD(staleFD); err != nil {
		t.Fatalf("retire detached stale registration: %v", err)
	}
	if err := poller.RegisterFD(staleFD, EventRead, func(IOEvents) {}); err != nil {
		t.Fatalf("register replacement after stale retirement: %v", err)
	}
	if got, err := poller.PollIO(1000); err != nil {
		t.Fatal(err)
	} else if got != 1 {
		t.Fatalf("fresh replacement callbacks = %d, want 1", got)
	}
}

func TestLinuxIndefinitePollRebuildsBeforeNextNativeWait(t *testing.T) {
	var poller fastPoller
	if err := poller.Init(); err != nil {
		t.Fatal(err)
	}
	registerPollerCleanupT(t, &poller)
	oldEPFD := poller.epfd
	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatal(err)
	}
	duplicate, err := unix.Dup(pipeFDs[0])
	if err != nil {
		t.Fatal(err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1], &duplicate)
	if err := poller.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	firstWait := make(chan struct{})
	releaseFirst := make(chan struct{})
	releaseFirstNow := contractRelease(t, releaseFirst)
	secondWait := make(chan int32, 1)
	releaseSecond := make(chan struct{})
	releaseSecondNow := contractRelease(t, releaseSecond)
	var calls atomic.Int32
	poller.epollWait = func(epfd int, _ []unix.EpollEvent, _ int) (int, error) {
		switch calls.Add(1) {
		case 1:
			close(firstWait)
			<-releaseFirst
		case 2:
			secondWait <- int32(epfd)
			<-releaseSecond
		}
		return 0, nil
	}
	pollDone := make(chan error, 1)
	go func() {
		_, err := poller.PollIO(-1)
		pollDone <- err
	}()
	waitContractSignal(t, firstWait, "first indefinite native poll")
	if err := unix.Close(pipeFDs[0]); err != nil {
		t.Fatal(err)
	}
	registeredFD := pipeFDs[0]
	pipeFDs[0] = -1
	if err := poller.UnregisterFD(registeredFD); err != nil {
		t.Fatal(err)
	}
	poller.lifecycleMu.Lock()
	poller.rebuildNeeded = true
	poller.lifecycleMu.Unlock()
	releaseFirstNow()
	newEPFD := waitContractValue(t, secondWait, "rebuilt indefinite native poll")
	if newEPFD == oldEPFD {
		t.Fatalf("epoll descriptor before second native wait = %d, want rebuild from %d", newEPFD, oldEPFD)
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- poller.Close() }()
	releaseSecondNow()
	if err := waitContractValue(t, pollDone, "indefinite PollIO completion"); !errors.Is(err, errPollerClosed) {
		t.Fatalf("PollIO after Close = %v, want errPollerClosed", err)
	}
	if err := waitContractValue(t, closeDone, "poller Close completion"); err != nil {
		t.Fatal(err)
	}
}
