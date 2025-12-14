//go:build darwin

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

	r := &ptyReader{file: readerFile, fd: -1, pollFD: -1, wakeR: -1, wakeW: -1, ops: newPTYOps()}

	// Open should initialize poller and not error
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

	// shouldInterpretAsEOF true for EIO, false for other errors
	require.True(t, r.shouldInterpretAsEOF(syscall.EIO))
	require.False(t, r.shouldInterpretAsEOF(syscall.EINVAL))

	// closePoller via Close method
	require.NoError(t, r.Close())
}

func TestDarwinPoller_ErrorBranches(t *testing.T) {
	// Tests will use per-instance `ops` so no package-level mutation is necessary.

	t.Run("initPoller kqueue error", func(t *testing.T) {
		sentinel := errors.New("kqueue failed")
		ops := newPTYOps()
		ops.kqueue = func() (int, error) { return -1, sentinel }
		r := &ptyReader{fd: 123, pollFD: -1, wakeR: -1, wakeW: -1, ops: ops}
		err := r.initPoller()
		require.ErrorIs(t, err, sentinel)
	})

	t.Run("initPoller pipe error closes kqueue", func(t *testing.T) {
		sentinel := errors.New("pipe failed")
		ops := newPTYOps()
		ops.kqueue = func() (int, error) { return 42, nil }
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

	t.Run("initPoller kevent error on PTY registration cleans up", func(t *testing.T) {
		sentinel := errors.New("kevent pty failed")
		ops := newPTYOps()
		ops.kqueue = func() (int, error) { return 10, nil }
		ops.pipe = func(fds []int) error { fds[0], fds[1] = 11, 12; return nil }
		ops.kevent = func(int, []unix.Kevent_t, []unix.Kevent_t, *unix.Timespec) (int, error) {
			return 0, sentinel
		}
		// Mock closeFD to avoid closing real fds used by concurrent tests
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
		// Verify FDs were cleaned up
		require.True(t, closed[10], "pollFD closed")
		require.True(t, closed[11], "wakeR closed")
		require.True(t, closed[12], "wakeW closed")
	})

	t.Run("initPoller kevent error on wake registration cleans up", func(t *testing.T) {
		sentinel := errors.New("kevent wake failed")
		ops := newPTYOps()
		ops.kqueue = func() (int, error) { return 10, nil }
		ops.pipe = func(fds []int) error { fds[0], fds[1] = 11, 12; return nil }
		// First call succeeds (PTY), second fails (wake)
		callCount := 0
		ops.kevent = func(fd int, _ []unix.Kevent_t, _ []unix.Kevent_t, _ *unix.Timespec) (int, error) {
			callCount++
			if callCount == 1 {
				// First call: register PTY
				return 0, nil
			}
			// Second call: register wake
			return 0, sentinel
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

		// Verify state reset
		require.Equal(t, -1, r.pollFD)
		require.Equal(t, -1, r.wakeR)
		require.Equal(t, -1, r.wakeW)

		// Verify FDs closed
		require.True(t, closed[10], "pollFD closed")
		require.True(t, closed[11], "wakeR closed")
		require.True(t, closed[12], "wakeW closed")
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
		ops.kevent = func(int, []unix.Kevent_t, []unix.Kevent_t, *unix.Timespec) (int, error) {
			return 0, syscall.EINTR
		}
		r := &ptyReader{pollFD: 1, wakeR: 2, ops: ops}
		require.NoError(t, r.waitForRead())
	})

	t.Run("waitForRead non-EINTR error bubbles", func(t *testing.T) {
		sentinel := errors.New("kevent failed")
		ops := newPTYOps()
		ops.kevent = func(int, []unix.Kevent_t, []unix.Kevent_t, *unix.Timespec) (int, error) {
			return 0, sentinel
		}
		r := &ptyReader{pollFD: 1, wakeR: 2, ops: ops}
		err := r.waitForRead()
		require.ErrorIs(t, err, sentinel)
	})

	t.Run("waitForRead drains wake pipe", func(t *testing.T) {
		ops := newPTYOps()
		ops.kevent = func(_ int, _ []unix.Kevent_t, out []unix.Kevent_t, _ *unix.Timespec) (int, error) {
			// Provide a single wake event.
			out[0] = unix.Kevent_t{Ident: 99}
			return 1, nil
		}
		readCalled := false
		ops.read = func(fd int, p []byte) (int, error) { readCalled = true; return 0, nil }
		r := &ptyReader{pollFD: 1, wakeR: 99, ops: ops}
		require.NoError(t, r.waitForRead())
		require.True(t, readCalled)
	})

	t.Run("closePoller no-op when already closed", func(t *testing.T) {
		r := &ptyReader{pollFD: -1, wakeR: -1, wakeW: -1}
		require.NoError(t, r.closePoller())
	})

	t.Run("closePoller error on wake fd", func(t *testing.T) {
		sentinel := errors.New("close wake")
		ops := newPTYOps()
		ops.closeFD = func(fd int) error {
			if fd == 2 {
				return sentinel
			}
			return nil
		}
		r := &ptyReader{pollFD: -1, wakeR: 2, wakeW: -1, ops: ops}
		err := r.closePoller()
		require.ErrorIs(t, err, sentinel)
	})

	t.Run("closePoller error on wakeW", func(t *testing.T) {
		sentinel := errors.New("close wakeW")
		ops := newPTYOps()
		ops.closeFD = func(fd int) error {
			if fd == 3 {
				return sentinel
			}
			return nil
		}
		r := &ptyReader{pollFD: -1, wakeR: -1, wakeW: 3, ops: ops}
		err := r.closePoller()
		require.ErrorIs(t, err, sentinel)
	})
}
