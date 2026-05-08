//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import (
	"errors"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func duplicateSparseTestFD(t *testing.T, fd int) int {
	t.Helper()
	dup, err := unix.FcntlInt(uintptr(fd), unix.F_DUPFD, maxFDs+32)
	if err != nil {
		if errors.Is(err, unix.EMFILE) || errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EBADF) {
			t.Skipf("cannot allocate sparse test fd above dense table: %v", err)
		}
		t.Fatalf("F_DUPFD sparse fd: %v", err)
	}
	if dup < maxFDs {
		invalidFD := dup
		closeTestFDT(t, &dup)
		t.Fatalf("F_DUPFD returned fd %d below dense threshold %d", invalidFD, maxFDs)
	}
	return dup
}

func TestFastPollerDenseFDTableGrowsForNearbyRegistration(t *testing.T) {
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
	initialLength := len(poller.fds)
	if err := poller.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); err != nil {
		t.Fatal(err)
	}
	if got := len(poller.fds); got <= initialLength || pipeFDs[0] >= got || got > maxFDs {
		t.Fatalf("dense table after fd %d grew from %d to %d, want bounded covering growth", pipeFDs[0], initialLength, got)
	}
}

func TestFastPollerSparseFDRegisterModifyUnregister(t *testing.T) {
	poller := &fastPoller{}
	if err := poller.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	registerPollerCleanupT(t, poller)

	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])

	highFD := duplicateSparseTestFD(t, pipeFDs[1])
	registerTestFDCleanupT(t, &highFD)

	initialDenseLen := len(poller.fds)
	if err := poller.RegisterFD(highFD, EventWrite, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD sparse fd %d: %v", highFD, err)
	}
	if len(poller.fds) != initialDenseLen {
		t.Fatalf("dense FD table grew from %d to %d for sparse fd %d", initialDenseLen, len(poller.fds), highFD)
	}
	poller.fdMu.RLock()
	_, active := poller.fdInfoLocked(highFD)
	_, inSparse := poller.sparseFDs[highFD]
	poller.fdMu.RUnlock()
	if !active || !inSparse {
		t.Fatalf("sparse fd %d active=%v inSparse=%v", highFD, active, inSparse)
	}

	if err := poller.ModifyFD(highFD, EventWrite); err != nil {
		t.Fatalf("ModifyFD sparse fd %d: %v", highFD, err)
	}
	if err := poller.UnregisterFD(highFD); err != nil {
		t.Fatalf("UnregisterFD sparse fd %d: %v", highFD, err)
	}
	poller.fdMu.RLock()
	_, active = poller.fdInfoLocked(highFD)
	_, inSparse = poller.sparseFDs[highFD]
	poller.fdMu.RUnlock()
	if active || inSparse {
		t.Fatalf("sparse fd %d retained after unregister: active=%v inSparse=%v", highFD, active, inSparse)
	}
}

func TestFastPollerReadyEventCallbackCanMutatePoller(t *testing.T) {
	poller := &fastPoller{}
	if err := poller.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	registerPollerCleanupT(t, poller)

	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])

	fd := pipeFDs[0]
	callbackDone := make(chan error, 1)
	if err := poller.RegisterFD(fd, EventRead, func(IOEvents) {
		if err := poller.ModifyFD(fd, EventRead); err != nil {
			callbackDone <- err
			return
		}
		if err := poller.UnregisterFD(fd); err != nil {
			callbackDone <- err
			return
		}
		callbackDone <- nil
	}); err != nil {
		t.Fatalf("RegisterFD: %v", err)
	}

	if _, err := unix.Write(pipeFDs[1], []byte{1}); err != nil {
		t.Fatalf("write pipe: %v", err)
	}
	n, err := poller.PollIO(100)
	if err != nil {
		t.Fatalf("PollIO: %v", err)
	}
	if n != 1 {
		t.Fatalf("PollIO ready count = %d, want 1", n)
	}
	ready := append([]pollEvent(nil), poller.readyEventsSnapshot()...)
	if len(ready) != 1 {
		t.Fatalf("ready event snapshot length = %d, want 1", len(ready))
	}
	callback, _, dispatch, ok := poller.beginReadyEventDispatch(ready[0])
	if !ok {
		t.Fatal("beginReadyEventDispatch rejected current event")
	}
	events, ok := poller.startReadyEventDispatch(ready[0], dispatch)
	if !ok {
		t.Fatal("startReadyEventDispatch rejected current event")
	}
	callback(events)
	poller.clearReadyEvents()

	select {
	case err := <-callbackDone:
		if err != nil {
			t.Fatalf("ready callback poller mutation failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ready callback did not finish")
	}
}

func TestLoopSparseFDRegisterModifyUnregister(t *testing.T) {
	loop := New()
	defer closeFDResourcesT(t, loop)

	var pipeFDs [2]int
	if err := unix.Pipe(pipeFDs[:]); err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	registerTestFDCleanupT(t, &pipeFDs[0], &pipeFDs[1])

	highFD := duplicateSparseTestFD(t, pipeFDs[1])
	registerTestFDCleanupT(t, &highFD)

	if err := loop.RegisterFD(highFD, EventWrite, func(IOEvents) {}); err != nil {
		t.Fatalf("RegisterFD sparse fd %d: %v", highFD, err)
	}
	if len(loop.poller.fds) >= maxFDs || highFD < len(loop.poller.fds) {
		t.Fatalf("dense FD table length = %d after sparse fd %d, want bounded prefix excluding sparse fd", len(loop.poller.fds), highFD)
	}
	loop.poller.fdMu.RLock()
	_, active := loop.poller.fdInfoLocked(highFD)
	_, inSparse := loop.poller.sparseFDs[highFD]
	loop.poller.fdMu.RUnlock()
	if !active || !inSparse {
		t.Fatalf("loop sparse fd %d active=%v inSparse=%v", highFD, active, inSparse)
	}
	if got := loop.userIOFDCount.Load(); got != 1 {
		t.Fatalf("userIOFDCount after sparse register = %d, want 1", got)
	}
	if err := loop.ModifyFD(highFD, EventWrite); err != nil {
		t.Fatalf("ModifyFD sparse fd %d: %v", highFD, err)
	}
	if err := loop.UnregisterFD(highFD); err != nil {
		t.Fatalf("UnregisterFD sparse fd %d: %v", highFD, err)
	}
	if got := loop.userIOFDCount.Load(); got != 0 {
		t.Fatalf("userIOFDCount after sparse unregister = %d, want 0", got)
	}
}
