//go:build darwin || linux

package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// COVERAGE-009: drainWakeUpPipe Full Coverage
// Tests: Windows path (wakePipe < 0), Unix path with multiple reads until EAGAIN,
// wakeUpSignalPending reset logic

// TestDrainWakeUpPipe_WindowsPath tests the early return path when wakePipe < 0
// This simulates the Windows case where no wake pipe exists.
func TestDrainWakeUpPipe_WindowsPath(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.closeFDs()

	// Save original values
	origWakePipe := loop.wakePipe
	origWakePipeWrite := loop.wakePipeWrite

	// Simulate Windows: wakePipe < 0
	loop.wakePipe = -1
	loop.wakePipeWrite = -1

	// Set wakeUpSignalPending to verify it gets reset
	loop.wakeUpSignalPending.Store(1)

	// Call drainWakeUpPipe - should take Windows path
	loop.drainWakeUpPipe()

	// Verify wakeUpSignalPending was reset
	if loop.wakeUpSignalPending.Load() != 0 {
		t.Error("wakeUpSignalPending should be reset to 0 on Windows path")
	}

	// Restore original values for proper cleanup
	loop.wakePipe = origWakePipe
	loop.wakePipeWrite = origWakePipeWrite
}

// TestDrainWakeUpPipe_UnixSingleRead tests draining when there's one byte in the pipe
func TestDrainWakeUpPipe_UnixSingleRead(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.closeFDs()

	// Write one byte to the wake pipe
	var one uint64 = 1
	buf := make([]byte, 8)
	for i := 0; i < 8; i++ {
		buf[i] = byte(one >> (i * 8))
	}
	_, err = writeFD(loop.wakePipeWrite, buf)
	if err != nil {
		t.Fatalf("Write to wake pipe failed: %v", err)
	}

	// Set wakeUpSignalPending
	loop.wakeUpSignalPending.Store(1)

	// Drain the pipe
	loop.drainWakeUpPipe()

	// Verify wakeUpSignalPending was reset
	if loop.wakeUpSignalPending.Load() != 0 {
		t.Error("wakeUpSignalPending should be reset to 0")
	}
}

// TestDrainWakeUpPipe_UnixMultipleReads tests draining when multiple writes were made
// This tests the loop that reads until EAGAIN
func TestDrainWakeUpPipe_UnixMultipleReads(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.closeFDs()

	// Write multiple times to the wake pipe
	var one uint64 = 1
	buf := make([]byte, 8)
	for i := 0; i < 8; i++ {
		buf[i] = byte(one >> (i * 8))
	}

	// Write 3 times to ensure multiple reads are needed
	for i := 0; i < 3; i++ {
		_, err = writeFD(loop.wakePipeWrite, buf)
		if err != nil {
			t.Fatalf("Write %d to wake pipe failed: %v", i, err)
		}
	}

	// Set wakeUpSignalPending
	loop.wakeUpSignalPending.Store(1)

	// Drain the pipe - should read all bytes until EAGAIN
	loop.drainWakeUpPipe()

	// Verify wakeUpSignalPending was reset
	if loop.wakeUpSignalPending.Load() != 0 {
		t.Error("wakeUpSignalPending should be reset to 0")
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
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.closeFDs()

	// Don't write anything to the pipe

	// Set wakeUpSignalPending
	loop.wakeUpSignalPending.Store(1)

	// Drain the pipe - should immediately get EAGAIN and return
	loop.drainWakeUpPipe()

	// Verify wakeUpSignalPending was reset even with empty pipe
	if loop.wakeUpSignalPending.Load() != 0 {
		t.Error("wakeUpSignalPending should be reset to 0 even on empty pipe")
	}
}

// TestDrainWakeUpPipe_ResetPendingFlag tests that wakeUpSignalPending is properly
// reset, allowing subsequent wakeups to work
func TestDrainWakeUpPipe_ResetPendingFlag(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.closeFDs()

	// Set wakeUpSignalPending
	loop.wakeUpSignalPending.Store(1)

	// Drain
	loop.drainWakeUpPipe()

	// Verify reset
	if loop.wakeUpSignalPending.Load() != 0 {
		t.Error("wakeUpSignalPending should be 0 after drain")
	}

	// Now a new CAS should succeed (simulating new wakeup)
	if !loop.wakeUpSignalPending.CompareAndSwap(0, 1) {
		t.Error("CAS should succeed after drain reset the flag")
	}
}

// TestDrainWakeUpPipe_ConcurrentWrites tests draining while concurrent writes occur
func TestDrainWakeUpPipe_ConcurrentWrites(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.closeFDs()

	var one uint64 = 1
	buf := make([]byte, 8)
	for i := 0; i < 8; i++ {
		buf[i] = byte(one >> (i * 8))
	}

	var wg sync.WaitGroup
	var writeCount atomic.Int32
	const numWriters = 10
	const writesPerWriter = 100

	// Start writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < writesPerWriter; j++ {
				_, err := writeFD(loop.wakePipeWrite, buf)
				if err == nil {
					writeCount.Add(1)
				}
				// Small delay to allow interleaving with drains
				time.Sleep(100 * time.Microsecond)
			}
		}()
	}

	// Start drainer
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				loop.wakeUpSignalPending.Store(1)
				loop.drainWakeUpPipe()
				time.Sleep(50 * time.Microsecond)
			}
		}
	}()

	wg.Wait()
	close(done)

	// Final drain
	loop.drainWakeUpPipe()

	t.Logf("Concurrent test completed with %d writes", writeCount.Load())
}

// TestDrainWakeUpPipe_IntegrationWithLoop tests drainWakeUpPipe in the context
// of the full event loop
func TestDrainWakeUpPipe_IntegrationWithLoop(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var taskCount atomic.Int32

	go func() {
		_ = loop.Run(ctx)
	}()

	// Wait for loop to start
	time.Sleep(50 * time.Millisecond)

	// Submit tasks rapidly - each should trigger wakeup and drain
	for i := 0; i < 100; i++ {
		err := loop.Submit(func() {
			taskCount.Add(1)
		})
		if err != nil {
			t.Logf("Submit %d failed: %v", i, err)
		}
	}

	// Wait for tasks to complete
	time.Sleep(200 * time.Millisecond)

	cancel()
	_ = loop.Shutdown(context.Background())

	if taskCount.Load() != 100 {
		t.Errorf("Expected 100 tasks, got %d", taskCount.Load())
	}
}

// TestDrainWakeUpPipe_WithRegisteredFD tests drainWakeUpPipe when FD is registered
// with the poller (covers the callback path)
func TestDrainWakeUpPipe_WithRegisteredFD(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Create a pipe for user FD
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	unix.SetNonblock(fds[0], true)
	unix.SetNonblock(fds[1], true)
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])

	var readCount atomic.Int32

	go func() {
		_ = loop.Run(ctx)
	}()

	// Wait for loop to start
	time.Sleep(50 * time.Millisecond)

	// Register user FD - this puts loop in I/O mode
	err = loop.RegisterFD(fds[0], EventRead, func(events IOEvents) {
		readCount.Add(1)
		// Drain the user pipe
		var buf [64]byte
		unix.Read(fds[0], buf[:])
	})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	// Write to user pipe to trigger event
	unix.Write(fds[1], []byte("test"))

	// Wait for event
	time.Sleep(100 * time.Millisecond)

	// Submit a task - this should trigger wake pipe write and drain
	var taskDone atomic.Bool
	err = loop.Submit(func() {
		taskDone.Store(true)
	})
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}

	// Wait for task
	time.Sleep(100 * time.Millisecond)

	// Unregister before shutdown
	_ = loop.UnregisterFD(fds[0])

	cancel()
	_ = loop.Shutdown(context.Background())

	if !taskDone.Load() {
		t.Error("Task should have completed")
	}
	if readCount.Load() < 1 {
		t.Error("User FD callback should have been called")
	}
}

// TestDrainWakeUpPipe_LargePipeBuffer tests draining when many bytes are buffered
func TestDrainWakeUpPipe_LargePipeBuffer(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.closeFDs()

	var one uint64 = 1
	buf := make([]byte, 8)
	for i := 0; i < 8; i++ {
		buf[i] = byte(one >> (i * 8))
	}

	// Write many times to fill the pipe buffer
	// Pipe buffer is typically 16-64KB, so write until we get EAGAIN
	writes := 0
	for {
		_, err := writeFD(loop.wakePipeWrite, buf)
		if err != nil {
			break // EAGAIN - pipe is full
		}
		writes++
		if writes >= 10000 {
			break // Safety limit
		}
	}

	t.Logf("Wrote %d times before pipe was full", writes)

	// Set wakeUpSignalPending
	loop.wakeUpSignalPending.Store(1)

	// Drain the pipe - should read all buffered bytes
	loop.drainWakeUpPipe()

	// Verify wakeUpSignalPending was reset
	if loop.wakeUpSignalPending.Load() != 0 {
		t.Error("wakeUpSignalPending should be reset to 0")
	}

	// Verify pipe is empty
	var readBuf [8]byte
	_, err = readFD(loop.wakePipe, readBuf[:])
	if err == nil {
		t.Error("Pipe should be empty after drain")
	}
}
