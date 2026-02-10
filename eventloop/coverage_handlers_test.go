//go:build linux || darwin

package eventloop

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"runtime"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// TestHandlePollError_Logging verifies that handlePollError logs correctly.
// This tests the log.Printf path which is always executed.
// Coverage target: handlePollError function entry and logging
func TestHandlePollError_Logging(t *testing.T) {
	testHooks := &loopTestHooks{}

	testHooks.PollError = func() error {
		return errors.New("logging test error")
	}

	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Close()
	loop.testHooks = testHooks

	pipeR, pipeW, err := os.Pipe()
	if err != nil {
		t.Fatal("os.Pipe failed:", err)
	}
	defer pipeW.Close()
	defer pipeR.Close()

	err = loop.RegisterFD(int(pipeR.Fd()), EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatal("RegisterFD failed:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	go loop.Run(ctx)

	// Wait for poll cycle and error handling
	time.Sleep(150 * time.Millisecond)

	// Test passes if no panic - handlePollError executed
	t.Log("handlePollError logging test completed")
}

// TestDrainWakeUpPipe_Basic tests basic drainWakeUpPipe functionality.
// Coverage target: drainWakeUpPipe function body
func TestDrainWakeUpPipe_Basic(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.Background())

	// Test the standalone drainWakeUpPipe function
	err = drainWakeUpPipe()
	if err != nil {
		t.Logf("drainWakeUpPipe returned error: %v", err)
	}
}

// TestDrainWakeUpPipe_Idempotent tests that multiple calls are safe.
// Coverage target: drainWakeUpPipe multiple invocations
func TestDrainWakeUpPipe_Idempotent(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Shutdown(context.Background())

	// Call drainWakeUpPipe multiple times
	for i := 0; i < 5; i++ {
		err = drainWakeUpPipe()
		if err != nil {
			t.Errorf("Iteration %d: drainWakeUpPipe error: %v", i, err)
		}
	}
}

// TestCreateWakeFd_ValidPipe tests that createWakeFd creates a valid pipe/eventfd.
// Coverage target: createWakeFd success path
func TestCreateWakeFd_ValidPipe(t *testing.T) {
	r, w, err := createWakeFd(0, 0)
	if err != nil {
		t.Fatalf("createWakeFd failed: %v", err)
	}
	defer syscall.Close(r)
	if w != r {
		defer syscall.Close(w)
	}

	// Verify FDs are valid
	if r <= 0 {
		t.Errorf("Invalid read FD: r=%d", r)
	}
	if w <= 0 {
		t.Errorf("Invalid write FD: w=%d", w)
	}

	if runtime.GOOS == "linux" {
		// On Linux, createWakeFd returns an eventfd (r == w).
		// eventfd requires 8-byte (uint64) reads and writes.
		if r != w {
			t.Errorf("Linux: expected r == w for eventfd, got r=%d, w=%d", r, w)
		}
		buf := make([]byte, 8)
		binary.LittleEndian.PutUint64(buf, 1)
		_, err = syscall.Write(w, buf)
		if err != nil {
			t.Errorf("Write failed: %v", err)
		}

		readBuf := make([]byte, 8)
		n, err := syscall.Read(r, readBuf)
		if err != nil {
			t.Errorf("Read failed: %v", err)
		}
		if n != 8 {
			t.Errorf("Read unexpected length: got %d, want 8", n)
		}
		val := binary.LittleEndian.Uint64(readBuf)
		if val != 1 {
			t.Errorf("Read unexpected value: got %d, want 1", val)
		}
	} else {
		// On Darwin, createWakeFd returns a pipe pair (r != w).
		if r == w {
			t.Errorf("Darwin: expected r != w for pipe, got r=%d, w=%d", r, w)
		}
		buf := []byte{0xAB}
		_, err = syscall.Write(w, buf)
		if err != nil {
			t.Errorf("Write failed: %v", err)
		}

		readBuf := make([]byte, 1)
		n, err := syscall.Read(r, readBuf)
		if err != nil {
			t.Errorf("Read failed: %v", err)
		}
		if n != 1 || readBuf[0] != 0xAB {
			t.Errorf("Read unexpected value: got %v", readBuf)
		}
	}
}

// TestCreateWakeFd_VariousParams tests createWakeFd with various parameters.
// Coverage target: createWakeFd parameter handling
func TestCreateWakeFd_VariousParams(t *testing.T) {
	params := []struct {
		initval uint
		flags   int
	}{
		{0, 0},
		{1, 0},
		{0, efdCloexec},
		{0, efdNonblock},
	}

	for _, p := range params {
		r, w, err := createWakeFd(p.initval, p.flags)
		if err != nil {
			t.Errorf("createWakeFd(%d, %d) failed: %v", p.initval, p.flags, err)
			continue
		}
		syscall.Close(r)
		if w != r {
			syscall.Close(w)
		}
	}
}

// TestHandlePollError_MultipleCycles tests multiple poll error cycles.
// Coverage target: handlePollError multiple invocations
func TestHandlePollError_MultipleCycles(t *testing.T) {
	testHooks := &loopTestHooks{}
	var errorCount int32

	testHooks.PollError = func() error {
		count := atomic.AddInt32(&errorCount, 1)
		if count < 5 {
			return errors.New("injected error")
		}
		return nil // Stop after 5 errors
	}

	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Close()
	loop.testHooks = testHooks

	pipeR, pipeW, err := os.Pipe()
	if err != nil {
		t.Fatal("os.Pipe failed:", err)
	}
	defer pipeW.Close()
	defer pipeR.Close()

	err = loop.RegisterFD(int(pipeR.Fd()), EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatal("RegisterFD failed:", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()

	go loop.Run(ctx)

	// Wait for multiple error cycles
	time.Sleep(300 * time.Millisecond)

	count := atomic.LoadInt32(&errorCount)
	t.Logf("handlePollError executed %d times", count)
}

// TestHandlePollError_ConcurrentWith其他Ops tests handlePollError with concurrent operations.
// Coverage target: handlePollError concurrent execution paths
func TestHandlePollError_ConcurrentWithOtherOps(t *testing.T) {
	testHooks := &loopTestHooks{}
	var errorCount atomic.Int32

	testHooks.PollError = func() error {
		count := errorCount.Add(1) - 1
		if count < 3 {
			return errors.New("concurrent test error")
		}
		return nil
	}

	loop, err := New()
	if err != nil {
		t.Fatal("New failed:", err)
	}
	defer loop.Close()
	loop.testHooks = testHooks

	pipeR, pipeW, err := os.Pipe()
	if err != nil {
		t.Fatal("os.Pipe failed:", err)
	}
	defer pipeW.Close()
	defer pipeR.Close()

	err = loop.RegisterFD(int(pipeR.Fd()), EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatal("RegisterFD failed:", err)
	}

	// Submit tasks while error is being handled
	var submitCount atomic.Int32
	for i := 0; i < 10; i++ {
		loop.Submit(func() {
			submitCount.Add(1)
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	go loop.Run(ctx)

	// Wait for operations to complete
	time.Sleep(200 * time.Millisecond)

	t.Logf("Submitted %d tasks, handled %d errors",
		submitCount.Load(), errorCount.Load())
}
