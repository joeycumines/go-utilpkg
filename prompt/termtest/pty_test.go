//go:build unix

package termtest

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

func TestConsole_Write_And_ExpectErrorMessage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use 'cat' to echo back input.
	// Set a very short default timeout to test the timeout error path quickly.
	c, err := NewConsole(ctx,
		WithCommand("cat"),
		WithDefaultTimeout(50*time.Millisecond),
	)
	if err != nil {
		t.Fatalf("NewConsole: %v", err)
	}
	defer c.Close()

	snap := c.Snapshot()
	// Test Write (bytes variant)
	n, err := c.Write([]byte("hello-bytes\n"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n <= 0 {
		t.Errorf("expected n > 0, got %d", n)
	}

	// Expect the output
	err = c.Expect(ctx, snap, Contains("hello-bytes"), "waiting for cat output")
	if err != nil {
		t.Fatalf("Expect: %v", err)
	}

	// Test Expect error path
	snap = c.Snapshot()
	_, err = c.WriteString("something\n")
	if err != nil {
		t.Fatalf("WriteString: %v", err)
	}

	// Expect something that won't happen.
	// context.Background() means we rely on the DefaultTimeout (50ms).
	// We expect this to fail with our custom description.
	err = c.Expect(context.Background(), snap, Contains("this will not appear"), "custom description")

	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	// The error message should contain the description we provided
	if !strings.Contains(err.Error(), "custom description") {
		t.Errorf("error %q should contain %q", err.Error(), "custom description")
	}
	// It should also indicate a timeout happened
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected error to wrap DeadlineExceeded, got %v", err)
	}
}

func TestPtyWriter_ExtraControlSequences(t *testing.T) {
	h, err := NewHarness(context.Background())
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	writer := &ptyWriter{file: h.pts}

	testCases := []struct {
		name     string
		action   func()
		expected string
	}{
		{"EraseUp", writer.EraseUp, "\x1b[1J"},
		{"EraseStartOfLine", writer.EraseStartOfLine, "\x1b[1K"},
		{"EraseEndOfLine", writer.EraseEndOfLine, "\x1b[0K"},
		{"AskForCPR", writer.AskForCPR, "\x1b[6n"},
		{"SetDisplayAttributes", func() { writer.SetDisplayAttributes(1, 2) }, "\x1b[31;42m"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			snap := h.Console().Snapshot()
			tc.action()
			if err := h.Console().Await(context.Background(), snap, ContainsRaw(tc.expected)); err != nil {
				t.Fatalf("Await: %v", err)
			}
		})
	}
}

func TestPtyWriter_WriteBytes(t *testing.T) {
	h, err := NewHarness(context.Background())
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	writer := &ptyWriter{file: h.pts}
	snap := h.Console().Snapshot()
	_, err = writer.Write([]byte("raw-write-test\n"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := h.Console().Await(context.Background(), snap, Contains("raw-write-test")); err != nil {
		t.Fatalf("Await: %v", err)
	}
}

func TestPtyWriter_Write_ClosedFileIsIgnored(t *testing.T) {
	_, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	writer := &ptyWriter{file: w}
	_, err = writer.Write([]byte("x"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
}

func TestPtyWriter_WriteString_ClosedFileIsIgnored(t *testing.T) {
	_, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	writer := &ptyWriter{file: w}
	_, err = writer.WriteString("x")
	if err != nil {
		t.Fatalf("WriteString: %v", err)
	}
}

func TestPtyReader_Open_NoFile(t *testing.T) {
	r := &ptyReader{file: nil, fd: -1, pollFD: -1, wakeR: -1, wakeW: -1}
	err := r.Open()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "ptyReader has no file") {
		t.Errorf("error %q should contain %q", err.Error(), "ptyReader has no file")
	}
}

func TestConsole_Write_ClosedReturnsErrClosedPipe(t *testing.T) {
	ctx := context.Background()
	cp, err := NewConsole(ctx, WithCommand("echo", "test"))
	if err != nil {
		t.Fatalf("NewConsole: %v", err)
	}
	if err := cp.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	_, err = cp.Write([]byte("x"))
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Errorf("expected io.ErrClosedPipe, got %v", err)
	}
}

// FuzzPtyReader tests the robustness of the ptyReader implementation
// (including platform-specific polling in reader_linux.go/reader_darwin.go)
// against arbitrary byte streams and closure events.
func FuzzPtyReader(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		ptm, pts, err := pty.Open()
		if err != nil {
			t.Skip(err)
		}
		// Ensure ptm is closed. pts is closed by r.Close() (or manually if Open fails).
		defer func() {
			_ = ptm.Close()
		}()

		r := newPTYReader(pts)

		if err := r.Open(); err != nil {
			_ = pts.Close()
			t.Skip(err)
		}

		done := make(chan struct{})
		go func() {
			defer close(done)
			buf := make([]byte, 1024)
			for {
				_, err := r.Read(buf)
				if err != nil {
					// Expected errors during teardown/EOF
					if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) || errors.Is(err, syscall.EIO) {
						return
					}
					// Handle platform-specific or wrapper error messages
					if strings.Contains(err.Error(), "file already closed") {
						return
					}
					return
				}
			}
		}()

		// Write data in a separate goroutine to avoid blocking the main thread
		// if the PTY buffer fills up.
		go func() {
			_, _ = ptm.Write(data)
		}()

		// Explicitly Close the reader to signal the read loop to exit.
		// This writes to the wake pipe, unblocking the poller (waitForRead).
		// We MUST do this before waiting on `done`.
		_ = r.Close()

		select {
		case <-done:
			// Success: Reader loop exited cleanly
		case <-time.After(1 * time.Second):
			t.Fatal("timeout waiting for ptyReader to finish reading")
		}
	})
}

func TestPtyWriter_ControlSequences(t *testing.T) {
	h, err := NewHarness(context.Background())
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	writer := &ptyWriter{file: h.pts}

	testCases := []struct {
		name     string
		action   func()
		expected string
	}{
		{"EraseScreen", writer.EraseScreen, "\x1b[2J"},
		{"HideCursor", writer.HideCursor, "\x1b[?25l"},
		{"ShowCursor", writer.ShowCursor, "\x1b[?25h"},
		{"CursorGoTo", func() { writer.CursorGoTo(5, 10) }, "\x1b[5;10H"},
		{"CursorUp", func() { writer.CursorUp(3) }, "\x1b[3A"},
		{"SetTitle", func() { writer.SetTitle("My Title") }, "\x1b]0;My Title\x07"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Snapshot current console output, write control sequence and await expected output
			snap := h.Console().Snapshot()
			tc.action()
			err := h.Console().Await(context.Background(), snap, ContainsRaw(tc.expected))
			if err != nil {
				t.Fatalf("Await: %v", err)
			}
			newOut := h.Console().String()[snap.offset:]
			if newOut != tc.expected {
				t.Errorf("got %q, want %q", newOut, tc.expected)
			}
		})
	}
}

func TestPtyReader_Open_ErrorBranches(t *testing.T) {
	h, err := NewHarness(context.Background())
	if err != nil {
		t.Fatalf("NewHarness: %v", err)
	}
	defer h.Close()

	_, readerFile := h.dupPTS()
	if readerFile == nil {
		t.Fatalf("expected non-nil readerFile")
	}

	// Prepare default ops; tests will mutate per-instance ops to avoid global state.
	t.Cleanup(func() {})

	t.Run("set nonblock error", func(t *testing.T) {
		sentinel := errors.New("setnonblock")
		ops := newPTYOps()
		ops.setNonblock = func(int, bool) error { return sentinel }
		r := &ptyReader{file: readerFile, fd: -1, pollFD: -1, wakeR: -1, wakeW: -1, ops: ops}
		err := r.Open()
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected error wrapping sentinel, got %v", err)
		}
		if !strings.Contains(err.Error(), "failed to set non-blocking mode") {
			t.Fatalf("error %q should contain %q", err.Error(), "failed to set non-blocking mode")
		}
	})

	t.Run("set raw error", func(t *testing.T) {
		sentinel := errors.New("setraw")
		ops := newPTYOps()
		ops.setNonblock = func(int, bool) error { return nil }
		ops.setRaw = func(int) error { return sentinel }
		r := &ptyReader{file: readerFile, fd: -1, pollFD: -1, wakeR: -1, wakeW: -1, ops: ops}
		err := r.Open()
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected error wrapping sentinel, got %v", err)
		}
		if !strings.Contains(err.Error(), "failed to set terminal to raw mode") {
			t.Fatalf("error %q should contain %q", err.Error(), "failed to set terminal to raw mode")
		}
	})

	t.Run("tcgetattr error", func(t *testing.T) {
		sentinel := errors.New("tcgetattr")
		ops := newPTYOps()
		ops.setNonblock = func(int, bool) error { return nil }
		ops.setRaw = func(int) error { return nil }
		ops.tcgetattr = func(uintptr) (*unix.Termios, error) { return nil, sentinel }
		r := &ptyReader{file: readerFile, fd: -1, pollFD: -1, wakeR: -1, wakeW: -1, ops: ops}
		err := r.Open()
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected error wrapping sentinel, got %v", err)
		}
		if !strings.Contains(err.Error(), "failed to get terminal attributes") {
			t.Fatalf("error %q should contain %q", err.Error(), "failed to get terminal attributes")
		}
	})

	t.Run("tcsetattr error", func(t *testing.T) {
		sentinel := errors.New("tcsetattr")
		ops := newPTYOps()
		ops.setNonblock = func(int, bool) error { return nil }
		ops.setRaw = func(int) error { return nil }
		ops.tcgetattr = func(uintptr) (*unix.Termios, error) { return &unix.Termios{}, nil }
		ops.tcsetattr = func(uintptr, uintptr, *unix.Termios) error { return sentinel }
		r := &ptyReader{file: readerFile, fd: -1, pollFD: -1, wakeR: -1, wakeW: -1, ops: ops}
		err := r.Open()
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected error wrapping sentinel, got %v", err)
		}
		if !strings.Contains(err.Error(), "failed to set VMIN=0") {
			t.Fatalf("error %q should contain %q", err.Error(), "failed to set VMIN=0")
		}
	})

	t.Run("initPoller error", func(t *testing.T) {
		sentinel := errors.New("initpoller")
		ops := newPTYOps()
		ops.setNonblock = func(int, bool) error { return nil }
		ops.setRaw = func(int) error { return nil }
		ops.tcgetattr = func(uintptr) (*unix.Termios, error) { return &unix.Termios{}, nil }
		ops.tcsetattr = func(uintptr, uintptr, *unix.Termios) error { return nil }
		ops.initPoller = func(*ptyReader) error { return sentinel }
		r := &ptyReader{file: readerFile, fd: -1, pollFD: -1, wakeR: -1, wakeW: -1, ops: ops}
		err := r.Open()
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected error wrapping sentinel, got %v", err)
		}
		if !strings.Contains(err.Error(), "failed to init poller") {
			t.Fatalf("error %q should contain %q", err.Error(), "failed to init poller")
		}
	})
}

func TestPtyReader_Read_Branches(t *testing.T) {
	// We'll create per-instance ops where needed to avoid global test pollution.

	t.Run("closed returns EOF", func(t *testing.T) {
		r := &ptyReader{closed: true, fd: 1}
		_, err := r.Read(make([]byte, 1))
		if !errors.Is(err, io.EOF) {
			t.Fatalf("expected io.EOF, got %v", err)
		}
	})

	t.Run("fd negative returns EOF", func(t *testing.T) {
		r := &ptyReader{closed: false, fd: -1}
		_, err := r.Read(make([]byte, 1))
		if !errors.Is(err, io.EOF) {
			t.Fatalf("expected io.EOF, got %v", err)
		}
	})

	t.Run("EAGAIN then wait error returns wait error", func(t *testing.T) {
		sentinel := errors.New("wait")
		ops := newPTYOps()
		ops.read = func(int, []byte) (int, error) { return 0, syscall.EAGAIN }
		ops.waitForRead = func(*ptyReader) error { return sentinel }
		r := &ptyReader{fd: 1, ops: ops}
		_, err := r.Read(make([]byte, 1))
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected error wrapping sentinel, got %v", err)
		}
	})

	t.Run("EAGAIN then wait error but closed returns EOF", func(t *testing.T) {
		sentinel := errors.New("wait")
		ops := newPTYOps()
		ops.read = func(int, []byte) (int, error) { return 0, syscall.EAGAIN }
		ops.waitForRead = func(r *ptyReader) error {
			r.mu.Lock()
			r.closed = true
			r.mu.Unlock()
			return sentinel
		}
		r := &ptyReader{fd: 1, ops: ops}
		_, err := r.Read(make([]byte, 1))
		if !errors.Is(err, io.EOF) {
			t.Fatalf("expected io.EOF, got %v", err)
		}
	})

	t.Run("n>0 EIO returns EOF", func(t *testing.T) {
		ops := newPTYOps()
		ops.read = func(int, []byte) (int, error) { return 1, syscall.EIO }
		r := &ptyReader{fd: 1, ops: ops}
		n, err := r.Read(make([]byte, 8))
		if n != 1 {
			t.Fatalf("n: got %d, want 1", n)
		}
		if !errors.Is(err, io.EOF) {
			t.Fatalf("expected io.EOF, got %v", err)
		}
	})

	t.Run("n>0 non-EOF error returns error", func(t *testing.T) {
		ops := newPTYOps()
		ops.read = func(int, []byte) (int, error) { return 1, syscall.EINVAL }
		r := &ptyReader{fd: 1, ops: ops}
		n, err := r.Read(make([]byte, 8))
		if n != 1 {
			t.Fatalf("n: got %d, want 1", n)
		}
		if !errors.Is(err, syscall.EINVAL) {
			t.Fatalf("expected syscall.EINVAL, got %v", err)
		}
	})

	t.Run("n==0 EIO returns EOF", func(t *testing.T) {
		ops := newPTYOps()
		ops.read = func(int, []byte) (int, error) { return 0, syscall.EIO }
		r := &ptyReader{fd: 1, ops: ops}
		_, err := r.Read(make([]byte, 8))
		if !errors.Is(err, io.EOF) {
			t.Fatalf("expected io.EOF, got %v", err)
		}
	})

	t.Run("n==0 err==nil then wait error returns wait error", func(t *testing.T) {
		sentinel := errors.New("wait")
		ops := newPTYOps()
		ops.read = func(int, []byte) (int, error) { return 0, nil }
		ops.waitForRead = func(*ptyReader) error { return sentinel }
		r := &ptyReader{fd: 1, ops: ops}
		_, err := r.Read(make([]byte, 8))
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected error wrapping sentinel, got %v", err)
		}
	})

	t.Run("n==0 err==nil then wait error but closed returns EOF", func(t *testing.T) {
		sentinel := errors.New("wait")
		ops := newPTYOps()
		ops.read = func(int, []byte) (int, error) { return 0, nil }
		ops.waitForRead = func(r *ptyReader) error {
			r.mu.Lock()
			r.closed = true
			r.mu.Unlock()
			return sentinel
		}
		r := &ptyReader{fd: 1, ops: ops}
		_, err := r.Read(make([]byte, 8))
		if !errors.Is(err, io.EOF) {
			t.Fatalf("expected io.EOF, got %v", err)
		}
	})

	t.Run("EAGAIN then wait nil continues", func(t *testing.T) {
		calls := 0
		ops := newPTYOps()
		ops.read = func(int, []byte) (int, error) {
			calls++
			if calls == 1 {
				return 0, syscall.EAGAIN
			}
			return 2, nil
		}
		ops.waitForRead = func(*ptyReader) error { return nil }
		r := &ptyReader{fd: 1, ops: ops}
		n, err := r.Read(make([]byte, 8))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 2 {
			t.Fatalf("n: got %d, want 2", n)
		}
	})
}
