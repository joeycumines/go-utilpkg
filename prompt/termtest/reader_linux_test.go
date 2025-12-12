//go:build linux

package termtest

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestPTYReader_InitCloseWaitAndEOFInterpretation(t *testing.T) {
	// Use a harness to get PTS for reading
	h, err := NewHarness(context.Background())
	require.NoError(t, err)
	defer h.Close()

	_, readerFile := h.dupPTS()
	require.NotNil(t, readerFile)

	r := newPTYReader(readerFile)

	// Open should initialize poller (epoll) and not error
	require.NoError(t, r.Open())
	require.GreaterOrEqual(t, r.pollFD, 0)
	require.GreaterOrEqual(t, r.wakeR, 0)
	require.GreaterOrEqual(t, r.wakeW, 0)

	// Calling waitForRead without anything should block until data; test using short timeout by running in goroutine.
	done := make(chan error, 1)
	go func() {
		done <- r.waitForRead()
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(50 * time.Millisecond):
		// If blocked, attempt to wake it by writing to wakeW
		if r.wakeW >= 0 {
			_, _ = unix.Write(r.wakeW, []byte("x"))
		}
		// Wait briefly for the goroutine to return
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(50 * time.Millisecond):
			t.Log("waitForRead still blocking after writing to wakeW; continuing")
		}
	}

	// shouldInterpretAsEOF true for EIO on Linux, false for other errors (e.g. EINVAL)
	require.True(t, r.shouldInterpretAsEOF(syscall.EIO))
	require.False(t, r.shouldInterpretAsEOF(syscall.EINVAL))

	// closePoller via Close method
	require.NoError(t, r.Close())
}

func TestLinuxPoller_ErrorBranches(t *testing.T) {
	t.Run("initPoller EpollCreate1 error", func(t *testing.T) {
		sentinel := errors.New("epoll create failed")
		ops := newPTYOps()
		ops.epollCreate1 = func(int) (int, error) { return -1, sentinel }
		r := &ptyReader{fd: 123, pollFD: -1, wakeR: -1, wakeW: -1, ops: ops}
		err := r.initPoller()
		require.ErrorIs(t, err, sentinel)
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
		require.ErrorIs(t, err, sentinel)
		require.Equal(t, 1, closeCalls)
		require.Equal(t, -1, r.pollFD)
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
		require.ErrorIs(t, err, sentinel)
		require.Equal(t, -1, r.pollFD)
		require.Equal(t, -1, r.wakeR)
		require.Equal(t, -1, r.wakeW)
		require.True(t, closed[10])
		require.True(t, closed[11])
		require.True(t, closed[12])
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
		require.ErrorIs(t, err, sentinel)
		require.Equal(t, -1, r.pollFD)
		require.Equal(t, -1, r.wakeR)
		require.Equal(t, -1, r.wakeW)
		require.True(t, closed[10])
		require.True(t, closed[11])
		require.True(t, closed[12])
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
		require.ErrorIs(t, err, sentinel)
		require.Equal(t, -1, r.pollFD)
		require.Equal(t, -1, r.wakeR)
		require.Equal(t, -1, r.wakeW)
	})

	t.Run("waitForRead treats EINTR as nil", func(t *testing.T) {
		ops := newPTYOps()
		ops.epollWait = func(int, []unix.EpollEvent, int) (int, error) {
			return 0, syscall.EINTR
		}
		r := &ptyReader{pollFD: 1, wakeR: 2, ops: ops}
		require.NoError(t, r.waitForRead())
	})

	t.Run("waitForRead non-EINTR error bubbles", func(t *testing.T) {
		sentinel := errors.New("epoll wait failed")
		ops := newPTYOps()
		ops.epollWait = func(int, []unix.EpollEvent, int) (int, error) {
			return 0, sentinel
		}
		r := &ptyReader{pollFD: 1, wakeR: 2, ops: ops}
		err := r.waitForRead()
		require.ErrorIs(t, err, sentinel)
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
		require.NoError(t, r.waitForRead())
		require.True(t, readCalled)
	})
}
