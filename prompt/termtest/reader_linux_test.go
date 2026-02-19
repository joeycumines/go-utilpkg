//go:build linux

package termtest

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestPTYReader_InitCloseWaitAndEOFInterpretation(t *testing.T) {
	// Use a harness to get PTS for reading
	h, err := NewHarness(context.Background())
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	_, readerFile := h.dupPTS()
	if readerFile == nil {
		t.Fatalf("expected non-nil readerFile")
	}

	r := newPTYReader(readerFile)

	// Open should initialize poller (epoll) and not error
	if err := r.Open(); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if r.pollFD < 0 {
		t.Fatalf("pollFD: got %d, want >= 0", r.pollFD)
	}
	if r.wakeR < 0 {
		t.Fatalf("wakeR: got %d, want >= 0", r.wakeR)
	}
	if r.wakeW < 0 {
		t.Fatalf("wakeW: got %d, want >= 0", r.wakeW)
	}

	// Calling waitForRead without anything should block until data; test using short timeout by running in goroutine.
	done := make(chan error, 1)
	go func() {
		done <- r.waitForRead()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("waitForRead: %v", err)
		}
	case <-time.After(50 * time.Millisecond):
		// If blocked, attempt to wake it by writing to wakeW
		if r.wakeW >= 0 {
			_, _ = unix.Write(r.wakeW, []byte("x"))
		}
		// Wait briefly for the goroutine to return
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("waitForRead after wake: %v", err)
			}
		case <-time.After(50 * time.Millisecond):
			t.Log("waitForRead still blocking after writing to wakeW; continuing")
		}
	}

	// shouldInterpretAsEOF true for EIO on Linux, false for other errors (e.g. EINVAL)
	if !r.shouldInterpretAsEOF(syscall.EIO) {
		t.Fatalf("expected shouldInterpretAsEOF(EIO) to be true")
	}
	if r.shouldInterpretAsEOF(syscall.EINVAL) {
		t.Fatalf("expected shouldInterpretAsEOF(EINVAL) to be false")
	}

	// closePoller via Close method
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestLinuxPoller_ErrorBranches(t *testing.T) {
	t.Run("initPoller EpollCreate1 error", func(t *testing.T) {
		sentinel := errors.New("epoll create failed")
		ops := newPTYOps()
		ops.epollCreate1 = func(int) (int, error) { return -1, sentinel }
		r := &ptyReader{fd: 123, pollFD: -1, wakeR: -1, wakeW: -1, ops: ops}
		err := r.initPoller()
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected error wrapping sentinel, got %v", err)
		}
	})

	t.Run("initPoller pipe error closes epoll fd", func(t *testing.T) {
		sentinel := errors.New("pipe failed")
		ops := newPTYOps()
		ops.epollCreate1 = func(int) (int, error) { return 42, nil }
		ops.pipe = func([]int) error { return sentinel }
		closeCalls := 0
		ops.closeFD = func(fd int) error {
			if fd == 42 {
				closeCalls++
			}
			return nil
		}
		r := &ptyReader{fd: 7, pollFD: -1, wakeR: -1, wakeW: -1, ops: ops}
		err := r.initPoller()
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected error wrapping sentinel, got %v", err)
		}
		if closeCalls != 1 {
			t.Fatalf("closeCalls: got %d, want 1", closeCalls)
		}
		if r.pollFD != -1 {
			t.Fatalf("pollFD: got %d, want -1", r.pollFD)
		}
	})

	t.Run("initPoller EpollCtl error on PTY registration cleans up", func(t *testing.T) {
		sentinel := errors.New("epoll ctl pty failed")
		ops := newPTYOps()
		ops.epollCreate1 = func(int) (int, error) { return 10, nil }
		ops.pipe = func(fds []int) error { fds[0], fds[1] = 11, 12; return nil }
		ops.epollCtl = func(epfd, op, fd int, event *unix.EpollEvent) error {
			return sentinel
		}
		// Mock closeFD to verify calls
		closed := make(map[int]bool)
		ops.closeFD = func(fd int) error {
			closed[fd] = true
			return nil
		}
		r := &ptyReader{fd: 9, pollFD: -1, wakeR: -1, wakeW: -1, ops: ops}
		err := r.initPoller()
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected error wrapping sentinel, got %v", err)
		}
		if r.pollFD != -1 {
			t.Fatalf("pollFD: got %d, want -1", r.pollFD)
		}
		if r.wakeR != -1 {
			t.Fatalf("wakeR: got %d, want -1", r.wakeR)
		}
		if r.wakeW != -1 {
			t.Fatalf("wakeW: got %d, want -1", r.wakeW)
		}
		if !closed[10] {
			t.Fatalf("pollFD not closed")
		}
		if !closed[11] {
			t.Fatalf("wakeR not closed")
		}
		if !closed[12] {
			t.Fatalf("wakeW not closed")
		}
	})

	t.Run("initPoller EpollCtl error on wake registration cleans up", func(t *testing.T) {
		sentinel := errors.New("epoll ctl wake failed")
		ops := newPTYOps()
		ops.epollCreate1 = func(int) (int, error) { return 10, nil }
		ops.pipe = func(fds []int) error { fds[0], fds[1] = 11, 12; return nil }
		callCount := 0
		ops.epollCtl = func(epfd, op, fd int, event *unix.EpollEvent) error {
			callCount++
			if callCount == 1 {
				// First call: register PTY
				return nil
			}
			// Second call: register wake
			return sentinel
		}
		// Mock closeFD to verify calls
		closed := make(map[int]bool)
		ops.closeFD = func(fd int) error {
			closed[fd] = true
			return nil
		}
		r := &ptyReader{fd: 9, pollFD: -1, wakeR: -1, wakeW: -1, ops: ops}
		err := r.initPoller()
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected error wrapping sentinel, got %v", err)
		}
		if r.pollFD != -1 {
			t.Fatalf("pollFD: got %d, want -1", r.pollFD)
		}
		if r.wakeR != -1 {
			t.Fatalf("wakeR: got %d, want -1", r.wakeR)
		}
		if r.wakeW != -1 {
			t.Fatalf("wakeW: got %d, want -1", r.wakeW)
		}
		if !closed[10] {
			t.Fatalf("pollFD not closed")
		}
		if !closed[11] {
			t.Fatalf("wakeR not closed")
		}
		if !closed[12] {
			t.Fatalf("wakeW not closed")
		}
	})

	t.Run("closePoller returns first error", func(t *testing.T) {
		sentinel := errors.New("close failed")
		ops := newPTYOps()
		ops.closeFD = func(fd int) error {
			if fd == 1 {
				return sentinel
			}
			return nil
		}
		r := &ptyReader{pollFD: 1, wakeR: 2, wakeW: 3, ops: ops}
		err := r.closePoller()
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected error wrapping sentinel, got %v", err)
		}
		if r.pollFD != -1 {
			t.Fatalf("pollFD: got %d, want -1", r.pollFD)
		}
		if r.wakeR != -1 {
			t.Fatalf("wakeR: got %d, want -1", r.wakeR)
		}
		if r.wakeW != -1 {
			t.Fatalf("wakeW: got %d, want -1", r.wakeW)
		}
	})

	t.Run("waitForRead treats EINTR as nil", func(t *testing.T) {
		ops := newPTYOps()
		ops.epollWait = func(int, []unix.EpollEvent, int) (int, error) {
			return 0, syscall.EINTR
		}
		r := &ptyReader{pollFD: 1, wakeR: 2, ops: ops}
		if err := r.waitForRead(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("waitForRead non-EINTR error bubbles", func(t *testing.T) {
		sentinel := errors.New("epoll wait failed")
		ops := newPTYOps()
		ops.epollWait = func(int, []unix.EpollEvent, int) (int, error) {
			return 0, sentinel
		}
		r := &ptyReader{pollFD: 1, wakeR: 2, ops: ops}
		err := r.waitForRead()
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected error wrapping sentinel, got %v", err)
		}
	})

	t.Run("waitForRead drains wake pipe", func(t *testing.T) {
		ops := newPTYOps()
		ops.epollWait = func(_ int, events []unix.EpollEvent, _ int) (int, error) {
			// Provide a single wake event.
			events[0] = unix.EpollEvent{Fd: 99}
			return 1, nil
		}
		readCalled := false
		ops.read = func(fd int, p []byte) (int, error) { readCalled = true; return 0, nil }
		r := &ptyReader{pollFD: 1, wakeR: 99, ops: ops}
		if err := r.waitForRead(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !readCalled {
			t.Fatalf("expected read to be called")
		}
	})
}
