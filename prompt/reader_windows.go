//go:build windows

package prompt

import (
	"errors"
	"sync"
	"syscall"
	"unicode/utf8"
	"unsafe"

	tty "github.com/mattn/go-tty"
)

var kernel32 = syscall.NewLazyDLL("kernel32.dll")

var procGetNumberOfConsoleInputEvents = kernel32.NewProc("GetNumberOfConsoleInputEvents")

// WindowsReader is a Reader implementation for Win32 console.
type WindowsReader struct {
	mu   sync.RWMutex
	tty  *tty.TTY
	open bool
	err  error // close error
}

// Open should be called before starting input
func (p *WindowsReader) Open() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	t, err := tty.Open()
	if err != nil {
		return err
	}
	p.tty = t
	p.open = true
	return nil
}

// Close should be called after stopping input
func (p *WindowsReader) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.open {
		// return any previous close error
		return p.err
	}
	// N.B. This error will be visible ONLY if tty.Close() panics (for subsequent calls to Close).
	p.err = errors.New("panic during reader close")
	p.open = false
	p.err = tty.Close()
	return p.err
}

func (p *WindowsReader) getTTY() *tty.TTY {
	p.mu.RLock()
	defer p.mu.RUnlock()
	// WARNING: deliberately doesn't guard on open state
	return p.tty
}

// Read returns byte array.
func (p *WindowsReader) Read(buff []byte) (int, error) {
	tty := p.getTTY()

	var ev uint32
	r0, _, err := procGetNumberOfConsoleInputEvents.Call(tty.Input().Fd(), uintptr(unsafe.Pointer(&ev)))
	if r0 == 0 {
		return 0, err
	}
	if ev == 0 {
		return 0, errors.New("EAGAIN")
	}

	r, err := tty.ReadRune()
	if err != nil {
		return 0, err
	}

	n := utf8.EncodeRune(buff[:], r)
	for tty.Buffered() && n < len(buff) {
		r, err := tty.ReadRune()
		if err != nil {
			break
		}
		n += utf8.EncodeRune(buff[n:], r)
	}
	return n, nil
}

// GetWinSize returns WinSize object to represent width and height of terminal.
func (p *WindowsReader) GetWinSize() *WinSize {
	w, h, err := p.getTTY().Size()
	if err != nil {
		panic(err)
	}
	return &WinSize{
		Row: uint16(h),
		Col: uint16(w),
	}
}

var _ Reader = &WindowsReader{}

// NewStdinReader returns Reader object to read from stdin.
func NewStdinReader() *WindowsReader {
	return &WindowsReader{}
}
