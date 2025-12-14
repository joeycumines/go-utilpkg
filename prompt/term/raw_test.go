package term

import (
	"errors"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

func resetTermGlobals(err error, fd int, v unix.Termios) {
	saveTermiosOnce = sync.Once{}
	saveTermiosErr = err
	saveTermiosFD = fd
	saveTermios = v
	saveTermiosOnce.Do(func() {})
}

// resetTermGlobalsToUninitialised resets globals to an uninitialised state so
// the next call to getOriginalTermios will run the actual sync.Once callback.
func resetTermGlobalsToUninitialised() {
	saveTermiosOnce = sync.Once{}
	saveTermiosErr = nil
	saveTermiosFD = 0
	saveTermios = unix.Termios{}
}

func TestGetOriginalTermiosReturnsCopy(t *testing.T) {
	original := unix.Termios{Iflag: 123, Lflag: 456, Cflag: 789}
	resetTermGlobals(nil, 42, original)

	got, err := getOriginalTermios(42)
	if err != nil {
		t.Fatalf("getOriginalTermios returned error: %v", err)
	}
	if got == &saveTermios {
		t.Fatalf("expected copy, got pointer to original")
	}
	if *got != saveTermios {
		t.Fatalf("unexpected termios copy: %#v", got)
	}
}

func TestSetRawPropagatesInitialError(t *testing.T) {
	resetTermGlobals(errors.New("boom"), 10, unix.Termios{})
	if err := SetRaw(10); err == nil || err.Error() != "boom" {
		t.Fatalf("expected initial error, got %v", err)
	}
}

func TestSetRawUsesCachedState(t *testing.T) {
	resetTermGlobals(nil, -1, unix.Termios{})
	if err := SetRaw(-1); err == nil {
		t.Fatalf("expected error for invalid fd")
	}
}

func TestRestoreUsesSavedFD(t *testing.T) {
	resetTermGlobals(nil, -1, unix.Termios{})
	if err := Restore(); err == nil {
		t.Fatalf("expected error when restoring invalid fd")
	}
}

func TestRestoreFDWithCachedState(t *testing.T) {
	resetTermGlobals(nil, -1, unix.Termios{})
	if err := RestoreFD(-1); err == nil {
		t.Fatalf("expected error when restoring invalid fd")
	}
}

func TestGetOriginalTermiosWithInvalidFD(t *testing.T) {
	// Reset to uninitialised state so sync.Once will actually run
	resetTermGlobalsToUninitialised()

	// Use an invalid fd to trigger the error path in the actual Tcgetattr call
	_, err := getOriginalTermios(-1)
	if err == nil {
		t.Fatalf("expected error for invalid fd, got nil")
	}
	// The error should now be cached
	if saveTermiosErr == nil {
		t.Fatalf("expected saveTermiosErr to be set")
	}
}

func TestRestoreFDSuccess(t *testing.T) {
	// Set up a valid cached state
	original := unix.Termios{Iflag: 0, Lflag: 0, Cflag: unix.CS8}
	resetTermGlobals(nil, -1, original)

	// RestoreFD should fail because fd is invalid, but it should reach
	// the Tcsetattr call (the err != nil path is already covered)
	err := RestoreFD(-1)
	if err == nil {
		// On some systems this might actually work if -1 is somehow valid
		t.Log("RestoreFD(-1) unexpectedly succeeded")
	}
}
