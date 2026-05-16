//go:build darwin || dragonfly || freebsd || openbsd

package eventloop

import (
	"context"
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestLoopPollerInitRetriesRetainedKernelTagCleanup(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = loop.Close()
		_ = loop.Shutdown(context.Background())
	})
	sentinel := errors.New("forced retained tag cleanup")
	unmapCalls := 0
	loop.poller.kernelTagStore.unmap = func(page []byte) error {
		unmapCalls++
		if unmapCalls <= 2 {
			return sentinel
		}
		return unix.Munmap(page)
	}
	createCalls := 0
	loop.poller.pollerCreate = func() (int, error) {
		createCalls++
		return unix.Kqueue()
	}
	changeErr := errors.New("forced wake registration failure")
	loop.poller.keventChange = func(int, []unix.Kevent_t) error { return changeErr }

	if err := loop.ensurePoller(); !errors.Is(err, sentinel) || !errors.Is(err, changeErr) {
		t.Fatalf("first ensurePoller error = %v, want %v and %v", err, sentinel, changeErr)
	}
	if !loop.pollerCleanupPending || len(loop.poller.kernelTagStore.pages) != 1 {
		t.Fatalf("first retained cleanup state = (pending %v, pages %d), want (true, 1)", loop.pollerCleanupPending, len(loop.poller.kernelTagStore.pages))
	}
	if loop.pollerReady.Load() || loop.wakePipe != -1 || loop.wakePipeWrite != -1 {
		t.Fatalf("failed initialization published resources = (ready %v, wake %d/%d)", loop.pollerReady.Load(), loop.wakePipe, loop.wakePipeWrite)
	}
	if err := loop.ensurePoller(); !errors.Is(err, sentinel) {
		t.Fatalf("cleanup-only retry error = %v, want %v", err, sentinel)
	}
	if createCalls != 1 || unmapCalls != 2 {
		t.Fatalf("cleanup-only retry calls = (create %d, unmap %d), want (1, 2)", createCalls, unmapCalls)
	}
	if err := loop.ensurePoller(); err != nil {
		t.Fatalf("ensurePoller after cleanup retry: %v", err)
	}
	if loop.pollerCleanupPending || !loop.pollerReady.Load() {
		t.Fatalf("successful retry state = (pending %v, ready %v), want (false, true)", loop.pollerCleanupPending, loop.pollerReady.Load())
	}
	if err := loop.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestLoopTerminalCallRetriesRetainedKernelTagCleanup(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = loop.Close()
		_ = loop.Shutdown(context.Background())
	})
	if err := loop.ensurePoller(); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("forced terminal tag cleanup")
	unmapCalls := 0
	loop.poller.kernelTagStore.unmap = func(page []byte) error {
		unmapCalls++
		if unmapCalls == 1 {
			return sentinel
		}
		return unix.Munmap(page)
	}
	descriptorCloseCalls := 0
	loop.poller.descriptorClose = func(fd int) error {
		descriptorCloseCalls++
		return unix.Close(fd)
	}

	if err := loop.Close(); !errors.Is(err, sentinel) {
		t.Fatalf("Close error = %v, want %v", err, sentinel)
	}
	if !loop.pollerCleanupPending || unmapCalls != 1 || len(loop.poller.kernelTagStore.pages) != 1 {
		t.Fatalf("first terminal cleanup = (pending %v, calls %d, pages %d), want (true, 1, 1)", loop.pollerCleanupPending, unmapCalls, len(loop.poller.kernelTagStore.pages))
	}
	firstDescriptorCloseCalls := descriptorCloseCalls
	if err := loop.Shutdown(context.Background()); !errors.Is(err, ErrLoopTerminated) {
		t.Fatalf("Shutdown after terminal completion = %v, want %v", err, ErrLoopTerminated)
	}
	if loop.pollerCleanupPending || unmapCalls != 2 {
		t.Fatalf("terminal retry cleanup = (pending %v, calls %d), want (false, 2)", loop.pollerCleanupPending, unmapCalls)
	}
	if descriptorCloseCalls != firstDescriptorCloseCalls {
		t.Fatalf("terminal retry descriptor closes = %d, want unchanged %d", descriptorCloseCalls, firstDescriptorCloseCalls)
	}
	if err := loop.fdResourceCloseError(); !errors.Is(err, sentinel) {
		t.Fatalf("retained terminal cleanup error = %v, want %v", err, sentinel)
	}
}

func TestFastPollerCloseRetriesFailedKernelTagCleanup(t *testing.T) {
	sentinel := errors.New("unmap failed")
	page := []byte{1}
	calls := 0
	poller := newFastPoller()
	poller.kernelTagStore.pages = [][]byte{page}
	poller.kernelTagStore.free = []keventTag{&page[0]}
	poller.kernelTagStore.offset = 1
	poller.kernelTagStore.unmap = func(got []byte) error {
		calls++
		if &got[0] != &page[0] {
			t.Fatalf("unmap page = %p, want %p", &got[0], &page[0])
		}
		if calls == 1 {
			return sentinel
		}
		return nil
	}

	if err := poller.Close(); !errors.Is(err, sentinel) {
		t.Fatalf("first Close error = %v, want sentinel", err)
	}
	if len(poller.kernelTagStore.pages) != 1 || poller.kernelTagStore.free != nil || poller.kernelTagStore.offset != 0 {
		t.Fatalf("retained tag store = %+v, want one page without reusable pointers", poller.kernelTagStore)
	}
	if err := poller.Close(); err != nil {
		t.Fatalf("retry Close: %v", err)
	}
	if calls != 2 || poller.kernelTagStore.pages != nil {
		t.Fatalf("cleanup retry = (calls %d, pages %d), want (2, 0)", calls, len(poller.kernelTagStore.pages))
	}
}

func TestKeventPointerTagStorageRemainsStableUntilPollerClose(t *testing.T) {
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
	poller.fdMu.RLock()
	info, active := poller.fdInfoLocked(pipeFDs[0])
	storagePages := len(poller.kernelTagStore.pages)
	poller.fdMu.RUnlock()
	if !active || storagePages == 0 {
		t.Fatal("active kqueue registration does not retain stable Udata storage")
	}
	if err := poller.UnregisterFD(pipeFDs[0]); err != nil {
		t.Fatal(err)
	}
	poller.fdMu.RLock()
	_, mapped := poller.kernelTags[info.kernelTag]
	storagePages = len(poller.kernelTagStore.pages)
	poller.fdMu.RUnlock()
	if mapped {
		t.Fatal("unregistered kqueue tag still resolves to a live generation")
	}
	if storagePages == 0 {
		t.Fatal("unregistered kqueue tag storage was released before in-flight polls joined")
	}
	if err := poller.Close(); err != nil {
		t.Fatal(err)
	}
	if poller.kernelTagStore.pages != nil {
		t.Fatal("poller Close retained kqueue tag storage")
	}
}

func TestKeventPointerTagStorageTracksPeakRegistrations(t *testing.T) {
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
	for range os.Getpagesize() + 1 {
		if err := poller.RegisterFD(pipeFDs[0], EventRead, func(IOEvents) {}); err != nil {
			t.Fatal(err)
		}
		if err := poller.UnregisterFD(pipeFDs[0]); err != nil {
			t.Fatal(err)
		}
	}
	poller.fdMu.RLock()
	pageCount := len(poller.kernelTagStore.pages)
	poller.fdMu.RUnlock()
	if pageCount != 1 {
		t.Fatalf("kernel Udata pages after sequential reuse = %d, want 1", pageCount)
	}
}
