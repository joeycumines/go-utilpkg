//go:build unix

package prompt

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

func TestPosixReaderOpenUsesInjectedFunctions(t *testing.T) {
	var calls []string
	pr := &PosixReader{}
	pr.open = func(path string, flag int, perm uint32) (int, error) {
		calls = append(calls, "open:"+path)
		return 99, nil
	}
	pr.setNonblock = func(fd int, on bool) error {
		if fd != 99 {
			t.Fatalf("expected fd 99, got %d", fd)
		}
		calls = append(calls, "nonblock")
		return nil
	}
	pr.setRaw = func(fd int) error {
		calls = append(calls, "raw")
		return nil
	}

	if err := pr.Open(); err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	if pr.fd != 99 {
		t.Fatalf("expected fd to be 99, got %d", pr.fd)
	}
	expected := []string{"open:/dev/tty", "nonblock", "raw"}
	if len(calls) != len(expected) {
		t.Fatalf("unexpected call count %d: %v", len(calls), calls)
	}
	for i := range calls {
		if calls[i] != expected[i] {
			t.Fatalf("call %d = %q, expected %q", i, calls[i], expected[i])
		}
	}
}

func TestPosixReaderOpenReturnsSetNonblockError(t *testing.T) {
	pr := &PosixReader{}
	pr.open = func(path string, flag int, perm uint32) (int, error) {
		return 123, nil
	}
	pr.setNonblock = func(fd int, on bool) error {
		return errors.New("nb fail")
	}
	pr.setRaw = func(fd int) error {
		t.Fatalf("termSetRaw should not be called when nonblock fails")
		return nil
	}

	if err := pr.Open(); err == nil || err.Error() != "nb fail" {
		t.Fatalf("expected nonblock error, got %v", err)
	}
}

func TestPosixReaderCloseRestoresThenCloses(t *testing.T) {
	pr := &PosixReader{fd: 7}
	pr.restoreFD = func(fd int) error {
		if fd != 7 {
			t.Fatalf("expected fd 7, got %d", fd)
		}
		return nil
	}
	closed := false
	pr.close = func(fd int) error {
		if fd != 7 {
			t.Fatalf("expected close fd 7, got %d", fd)
		}
		closed = true
		return nil
	}

	if err := pr.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if !closed {
		t.Fatalf("expected close to be called")
	}
}

func TestPosixReaderCloseReturnsRestoreError(t *testing.T) {
	pr := &PosixReader{fd: 11}
	pr.restoreFD = func(fd int) error {
		return errors.New("restore fail")
	}
	closed := false
	pr.close = func(fd int) error {
		closed = true
		return nil
	}

	if err := pr.Close(); err == nil || err.Error() != "restore fail" {
		t.Fatalf("expected restore error, got %v", err)
	}
	if !closed {
		t.Fatalf("expected close to be called even on restore error")
	}
}

func TestPosixReaderReadDelegates(t *testing.T) {
	pr := &PosixReader{fd: 5}
	pr.read = func(fd int, b []byte) (int, error) {
		if fd != 5 {
			t.Fatalf("expected fd 5, got %d", fd)
		}
		copy(b, []byte("hi"))
		return 2, nil
	}

	buf := make([]byte, 4)
	n, err := pr.Read(buf)
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	if n != 2 || string(buf[:n]) != "hi" {
		t.Fatalf("unexpected read result n=%d buf=%q", n, buf[:n])
	}
}

func TestPosixReaderGetWinSizeUsesIoctl(t *testing.T) {
	pr := &PosixReader{fd: 3}
	pr.ioctlWinsize = func(fd int, req uint) (*unix.Winsize, error) {
		if fd != 3 || req != unix.TIOCGWINSZ {
			t.Fatalf("unexpected ioctl args fd=%d req=%d", fd, req)
		}
		return &unix.Winsize{Row: 10, Col: 20}, nil
	}

	ws := pr.GetWinSize()
	if ws.Row != 10 || ws.Col != 20 {
		t.Fatalf("unexpected winsize: %#v", ws)
	}
}

func TestPosixReaderGetWinSizeDefaultsOnError(t *testing.T) {
	pr := &PosixReader{fd: 4}
	pr.ioctlWinsize = func(fd int, req uint) (*unix.Winsize, error) {
		return nil, errors.New("ioctl fail")
	}

	ws := pr.GetWinSize()
	if ws.Row != DefRowCount || ws.Col != DefColCount {
		t.Fatalf("expected default size, got %#v", ws)
	}
}
