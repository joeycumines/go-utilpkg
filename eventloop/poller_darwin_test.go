//go:build darwin

package eventloop

import (
	"testing"

	"golang.org/x/sys/unix"
)

// TestModifyFD_Darwin_ErrorPropagation verifies that ModifyFD correctly
// propagates errors for closed file descriptors on Darwin.
func TestModifyFD_Darwin_ErrorPropagation(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.closeFDs()

	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	// Set non-blocking
	unix.SetNonblock(fds[0], true)
	unix.SetNonblock(fds[1], true)
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])

	err = loop.RegisterFD(fds[0], EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	unix.Close(fds[0])

	err = loop.ModifyFD(fds[0], EventWrite)

	if err == nil {
		t.Error("FAIL: ModifyFD should return error for closed FD on Darwin")
	}
}
