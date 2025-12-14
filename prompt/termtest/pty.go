//go:build unix

package termtest

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"

	"github.com/joeycumines/go-prompt"
	promptterm "github.com/joeycumines/go-prompt/term"
	"github.com/pkg/term/termios"
	"golang.org/x/sys/unix"
)

// ptyOps collects platform and system call operations used by ptyReader.
// Using an ops struct allows tests to inject mocks per-instance instead of
// mutating package globals, removing global state races between tests.
type ptyOps struct {
	setNonblock func(int, bool) error
	setRaw      func(int) error
	tcgetattr   func(uintptr) (*unix.Termios, error)
	tcsetattr   func(uintptr, uintptr, *unix.Termios) error
	read        func(int, []byte) (int, error)
	initPoller  func(*ptyReader) error
	waitForRead func(*ptyReader) error
	pipe        func([]int) error
	closeFD     func(int) error

	// platform-specific primitives used by the poller implementations

	//lint:ignore U1000 Unused depending on env.
	kqueue func() (int, error)
	//lint:ignore U1000 Unused depending on env.
	kevent func(int, []ptyOpsUnixKevent_t, []ptyOpsUnixKevent_t, *unix.Timespec) (int, error)

	//lint:ignore U1000 Unused depending on env.
	epollCreate1 func(int) (int, error)
	//lint:ignore U1000 Unused depending on env.
	epollCtl func(int, int, int, *ptyOpsEpollEvent_t) error
	//lint:ignore U1000 Unused depending on env.
	epollWait func(int, []ptyOpsEpollEvent_t, int) (int, error)
}

func newPTYOps() *ptyOps {
	x := ptyOps{
		setNonblock: syscall.SetNonblock,
		setRaw:      promptterm.SetRaw,
		tcgetattr:   termios.Tcgetattr,
		tcsetattr:   termios.Tcsetattr,
		read:        syscall.Read,
		initPoller:  func(r *ptyReader) error { return r.initPoller() },
		waitForRead: func(r *ptyReader) error { return r.waitForRead() },
		// Default platform fn to unix equivalents
		pipe:    unix.Pipe,
		closeFD: unix.Close,
	}
	x.init()
	return &x
}

func newPTYReader(file *os.File) *ptyReader {
	return &ptyReader{
		file:   file,
		fd:     -1,
		pollFD: -1,
		wakeR:  -1,
		wakeW:  -1,
		ops:    newPTYOps(),
	}
}

type ptyReader struct {
	file      *os.File
	fd        int
	pollFD    int
	wakeR     int
	wakeW     int
	closed    bool
	mu        sync.Mutex
	closeOnce sync.Once
	ops       *ptyOps
}

func (r *ptyReader) Open() error {
	if r.file == nil {
		return fmt.Errorf("ptyReader has no file")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.fd = int(r.file.Fd())

	if err := r.ops.setNonblock(r.fd, true); err != nil {
		return fmt.Errorf("failed to set non-blocking mode: %w", err)
	}
	if err := r.ops.setRaw(r.fd); err != nil {
		return fmt.Errorf("failed to set terminal to raw mode: %w", err)
	}

	term, err := r.ops.tcgetattr(uintptr(r.fd))
	if err != nil {
		return fmt.Errorf("failed to get terminal attributes: %w", err)
	}
	term.Cc[unix.VMIN] = 0
	term.Cc[unix.VTIME] = 0
	if err := r.ops.tcsetattr(uintptr(r.fd), termios.TCSANOW, term); err != nil {
		return fmt.Errorf("failed to set VMIN=0: %w", err)
	}
	if err := r.ops.initPoller(r); err != nil {
		return fmt.Errorf("failed to init poller: %w", err)
	}
	return nil
}

func (r *ptyReader) Close() error {
	r.closeOnce.Do(func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.closed = true
		if r.wakeW >= 0 {
			_, _ = unix.Write(r.wakeW, []byte("x"))
		}
		if r.file != nil {
			_ = promptterm.RestoreFD(r.fd)
			_ = r.file.Close()
			_ = r.closePoller()
			r.file = nil
			r.fd = -1
		}
	})
	return nil
}

func (r *ptyReader) Read(p []byte) (int, error) {
	for {
		r.mu.Lock()
		if r.closed || r.fd < 0 {
			r.mu.Unlock()
			return 0, io.EOF
		}

		n, err := r.ops.read(r.fd, p)
		if err != nil {
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
				r.mu.Unlock()
				if waitErr := r.ops.waitForRead(r); waitErr != nil {
					r.mu.Lock()
					isClosed := r.closed
					r.mu.Unlock()
					if isClosed {
						return 0, io.EOF
					}
					return 0, waitErr
				}
				continue
			}

			if n > 0 {
				r.mu.Unlock()
				if r.shouldInterpretAsEOF(err) {
					return n, io.EOF
				}
				return n, err
			}

			if r.shouldInterpretAsEOF(err) {
				r.mu.Unlock()
				return 0, io.EOF
			}
		}

		// VMIN=0 means read returns 0 when no data is available.
		// We must NOT treat n=0, err=nil as EOF in this case, or go-prompt
		// will exit immediately on startup. We must treat it as "no data" and
		// continue polling.
		// Real EOF is handled via the p.Close() -> reader.Close() -> r.closed flow.
		if n == 0 && err == nil {
			r.mu.Unlock()
			// Continue loop to wait for data (equivalent to EAGAIN handling)
			if waitErr := r.ops.waitForRead(r); waitErr != nil {
				r.mu.Lock()
				isClosed := r.closed
				r.mu.Unlock()
				if isClosed {
					return 0, io.EOF
				}
				return 0, waitErr
			}
			continue
		}

		r.mu.Unlock()
		return n, err
	}
}

func (r *ptyReader) GetWinSize() *prompt.WinSize {
	return &prompt.WinSize{Row: 24, Col: 80}
}

type ptyWriter struct {
	file *os.File
}

func (w *ptyWriter) Write(p []byte) (int, error) {
	n, err := w.file.Write(p)
	if err != nil && strings.Contains(err.Error(), "file already closed") {
		return n, nil
	}
	return n, err
}

func (w *ptyWriter) WriteString(s string) (int, error) {
	n, err := w.file.WriteString(s)
	if err != nil && strings.Contains(err.Error(), "file already closed") {
		return n, nil
	}
	return n, err
}

func (w *ptyWriter) WriteRaw(data []byte)       { _, _ = w.file.Write(data) }
func (w *ptyWriter) WriteRawString(data string) { _, _ = w.file.WriteString(data) }
func (w *ptyWriter) Flush() error               { return w.file.Sync() }
func (w *ptyWriter) EraseScreen()               { w.WriteRawString("\x1b[2J") }
func (w *ptyWriter) EraseUp()                   { w.WriteRawString("\x1b[1J") }
func (w *ptyWriter) EraseDown()                 { w.WriteRawString("\x1b[0J") }
func (w *ptyWriter) EraseStartOfLine()          { w.WriteRawString("\x1b[1K") }
func (w *ptyWriter) EraseEndOfLine()            { w.WriteRawString("\x1b[0K") }
func (w *ptyWriter) EraseLine()                 { w.WriteRawString("\x1b[2K") }
func (w *ptyWriter) ShowCursor()                { w.WriteRawString("\x1b[?25h") }
func (w *ptyWriter) HideCursor()                { w.WriteRawString("\x1b[?25l") }
func (w *ptyWriter) CursorGoTo(row, col int)    { w.WriteRawString(fmt.Sprintf("\x1b[%d;%dH", row, col)) }
func (w *ptyWriter) CursorUp(n int)             { w.WriteRawString(fmt.Sprintf("\x1b[%dA", n)) }
func (w *ptyWriter) CursorDown(n int)           { w.WriteRawString(fmt.Sprintf("\x1b[%dB", n)) }
func (w *ptyWriter) CursorForward(n int)        { w.WriteRawString(fmt.Sprintf("\x1b[%dC", n)) }
func (w *ptyWriter) CursorBackward(n int)       { w.WriteRawString(fmt.Sprintf("\x1b[%dD", n)) }
func (w *ptyWriter) AskForCPR()                 { w.WriteRawString("\x1b[6n") }
func (w *ptyWriter) SaveCursor()                { w.WriteRawString("\x1b[s") }
func (w *ptyWriter) UnSaveCursor()              { w.WriteRawString("\x1b[u") }
func (w *ptyWriter) ScrollDown()                { w.WriteRawString("\x1bD") }
func (w *ptyWriter) ScrollUp()                  { w.WriteRawString("\x1bM") }
func (w *ptyWriter) SetTitle(title string)      { w.WriteRawString(fmt.Sprintf("\x1b]0;%s\x07", title)) }
func (w *ptyWriter) ClearTitle()                { w.WriteRawString("\x1b]0;\x07") }
func (w *ptyWriter) SetColor(fg, bg prompt.Color, bold bool) {
	var code string
	if bold {
		code = fmt.Sprintf("\x1b[1;%d;%dm", int(fg)+30, int(bg)+40)
	} else {
		code = fmt.Sprintf("\x1b[%d;%dm", int(fg)+30, int(bg)+40)
	}
	w.WriteRawString(code)
}
func (w *ptyWriter) SetDisplayAttributes(fg, bg prompt.Color, attrs ...prompt.DisplayAttribute) {
	w.SetColor(fg, bg, false)
}
