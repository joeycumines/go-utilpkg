//go:build (aix && ppc64) || darwin || dragonfly || freebsd || linux || netbsd || openbsd || (solaris && amd64)

package eventloop

import "testing"

func ensureWakePipeForTest(t *testing.T, loop *Loop) {
	t.Helper()
	if err := loop.ensurePoller(); err != nil {
		t.Fatalf("ensurePoller failed: %v", err)
	}
	if loop.wakePipe < 0 || loop.wakePipeWrite < 0 {
		t.Fatalf("wake pipe not initialized: read=%d write=%d", loop.wakePipe, loop.wakePipeWrite)
	}
}

// drainWakeUpPipe coverage includes the uninitialized-descriptor path, multiple
// reads through EAGAIN, and wakeUpSignalPending reset behavior.

// TestDrainWakeUpPipeUninitialized tests the early return when no wake
// descriptor has been initialized.
func TestDrainWakeUpPipeUninitialized(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer closeFDResourcesT(t, loop)

	// Save original values
	origWakePipe := loop.wakePipe
	origWakePipeWrite := loop.wakePipeWrite

	// Simulate a loop without initialized wake descriptors.
	loop.wakePipe = -1
	loop.wakePipeWrite = -1

	// Set wakeUpSignalPending to verify it gets reset
	loop.wakeUpSignalPending.Store(wakeSignalPending)

	// Call drainWakeUpPipe - should take the uninitialized path.
	loop.drainWakeUpPipe()

	// Verify wakeUpSignalPending was reset
	if loop.wakeUpSignalPending.Load() != wakeSignalIdle {
		t.Error("wakeUpSignalPending should be reset to idle on uninitialized path")
	}

	// Restore original values for proper cleanup
	loop.wakePipe = origWakePipe
	loop.wakePipeWrite = origWakePipeWrite
}

// TestDrainWakeUpPipe_UnixSingleRead tests draining when there's one byte in the pipe
func TestDrainWakeUpPipe_UnixSingleRead(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer closeFDResourcesT(t, loop)
	ensureWakePipeForTest(t, loop)

	// Write one byte to the wake pipe
	var one uint64 = 1
	buf := make([]byte, 8)
	for i := range 8 {
		buf[i] = byte(one >> (i * 8))
	}
	_, err = writeFD(loop.wakePipeWrite, buf)
	if err != nil {
		t.Fatalf("Write to wake pipe failed: %v", err)
	}

	// Set wakeUpSignalPending
	loop.wakeUpSignalPending.Store(wakeSignalPending)

	// Drain the pipe
	loop.drainWakeUpPipe()

	// Verify wakeUpSignalPending was reset
	if loop.wakeUpSignalPending.Load() != wakeSignalIdle {
		t.Error("wakeUpSignalPending should be reset to idle")
	}
}

// TestDrainWakeUpPipe_UnixMultipleReads tests draining when multiple writes were made
// This tests the loop that reads until EAGAIN
func TestDrainWakeUpPipe_UnixMultipleReads(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer closeFDResourcesT(t, loop)
	ensureWakePipeForTest(t, loop)

	// Write multiple times to the wake pipe
	var one uint64 = 1
	buf := make([]byte, 8)
	for i := range 8 {
		buf[i] = byte(one >> (i * 8))
	}

	// Write 3 times to ensure multiple reads are needed
	for i := range 3 {
		_, err = writeFD(loop.wakePipeWrite, buf)
		if err != nil {
			t.Fatalf("Write %d to wake pipe failed: %v", i, err)
		}
	}

	// Set wakeUpSignalPending
	loop.wakeUpSignalPending.Store(wakeSignalPending)

	// Drain the pipe - should read all bytes until EAGAIN
	loop.drainWakeUpPipe()

	// Verify wakeUpSignalPending was reset
	if loop.wakeUpSignalPending.Load() != wakeSignalIdle {
		t.Error("wakeUpSignalPending should be reset to idle")
	}

	// Verify pipe is now empty by trying to read (should fail with EAGAIN)
	var readBuf [8]byte
	_, err = readFD(loop.wakePipe, readBuf[:])
	if err == nil {
		t.Error("Expected EAGAIN after drain, but read succeeded")
	}
}

// TestDrainWakeUpPipe_EmptyPipe tests draining when the pipe is already empty
func TestDrainWakeUpPipe_EmptyPipe(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer closeFDResourcesT(t, loop)
	ensureWakePipeForTest(t, loop)

	// Don't write anything to the pipe

	// Set wakeUpSignalPending
	loop.wakeUpSignalPending.Store(wakeSignalPending)

	// Drain the pipe - should immediately get EAGAIN and return
	loop.drainWakeUpPipe()

	// Verify wakeUpSignalPending was reset even with empty pipe
	if loop.wakeUpSignalPending.Load() != wakeSignalIdle {
		t.Error("wakeUpSignalPending should be reset to idle even on empty pipe")
	}
}

// TestDrainWakeUpPipe_ResetPendingFlag tests that wakeUpSignalPending is properly
// reset, allowing subsequent wakeups to work
func TestDrainWakeUpPipe_ResetPendingFlag(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer closeFDResourcesT(t, loop)
	ensureWakePipeForTest(t, loop)

	// Set wakeUpSignalPending
	loop.wakeUpSignalPending.Store(wakeSignalPending)

	// Drain
	loop.drainWakeUpPipe()

	// Verify reset
	if loop.wakeUpSignalPending.Load() != wakeSignalIdle {
		t.Error("wakeUpSignalPending should be idle after drain")
	}

	// Now a new CAS should succeed (simulating new wakeup)
	if !loop.wakeUpSignalPending.CompareAndSwap(wakeSignalIdle, wakeSignalPending) {
		t.Error("CAS should succeed after drain reset the flag")
	}
}
