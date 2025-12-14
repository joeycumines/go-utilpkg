//go:build unix

package prompt

import (
	"os"
	"syscall"

	"github.com/joeycumines/go-prompt/term"
	"golang.org/x/sys/unix"
)

// PosixReader is a Reader implementation for the POSIX environment.
type PosixReader struct {
	fd           int
	open         func(string, int, uint32) (int, error)
	close        func(int) error
	read         func(int, []byte) (int, error)
	setNonblock  func(int, bool) error
	setRaw       func(int) error
	restoreFD    func(int) error
	ioctlWinsize func(int, uint) (*unix.Winsize, error)
}

func (t *PosixReader) initFuncs() {
	if t.open == nil {
		t.open = syscall.Open
	}
	if t.close == nil {
		t.close = syscall.Close
	}
	if t.read == nil {
		t.read = syscall.Read
	}
	if t.setNonblock == nil {
		t.setNonblock = syscall.SetNonblock
	}
	if t.setRaw == nil {
		t.setRaw = term.SetRaw
	}
	if t.restoreFD == nil {
		t.restoreFD = term.RestoreFD
	}
	if t.ioctlWinsize == nil {
		t.ioctlWinsize = unix.IoctlGetWinsize
	}
}

// Open should be called before starting input
func (t *PosixReader) Open() error {
	t.initFuncs()
	in, err := t.open("/dev/tty", syscall.O_RDONLY, 0)
	if os.IsNotExist(err) {
		in = syscall.Stdin
	} else if err != nil {
		panic(err)
	}
	t.fd = in
	// Set NonBlocking mode because if syscall.Read block this goroutine, it cannot receive data from stopCh.
	if err := t.setNonblock(t.fd, true); err != nil {
		return err
	}
	if err := t.setRaw(t.fd); err != nil {
		return err
	}
	return nil
}

// Close should be called after stopping input
func (t *PosixReader) Close() error {
	if err := t.restoreFD(t.fd); err != nil {
		_ = t.close(t.fd)
		return err
	}
	return t.close(t.fd)
}

// Read returns byte array.
func (t *PosixReader) Read(buff []byte) (int, error) {
	return t.read(t.fd, buff)
}

// GetWinSize returns WinSize object to represent width and height of terminal.
func (t *PosixReader) GetWinSize() *WinSize {
	ws, err := t.ioctlWinsize(t.fd, unix.TIOCGWINSZ)
	if err != nil {
		// If this errors, we simply return the default window size as
		// it's our best guess.
		return &WinSize{
			Row: DefRowCount,
			Col: DefColCount,
		}
	}
	return &WinSize{
		Row: ws.Row,
		Col: ws.Col,
	}
}

var _ Reader = &PosixReader{}

// NewStdinReader returns Reader object to read from stdin.
func NewStdinReader() *PosixReader {
	pr := &PosixReader{
		open:         syscall.Open,
		close:        syscall.Close,
		read:         syscall.Read,
		setNonblock:  syscall.SetNonblock,
		setRaw:       term.SetRaw,
		restoreFD:    term.RestoreFD,
		ioctlWinsize: unix.IoctlGetWinsize,
	}
	return pr
}
