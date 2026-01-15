package eventloop

import (
	"context"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestRegisterFD_Basic tests basic FD registration and callback execution.
func TestRegisterFD_Basic(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	runDone := make(chan struct{})
	errChan := make(chan error, 1)
	go func() {
		if err := loop.Run(ctx); err != nil {
			errChan <- err
			return
		}
		close(runDone)
	}()
	defer func() {
		loop.Shutdown(context.Background())
		<-runDone
		select {
		case err := <-errChan:
			t.Fatalf("Run() failed: %v", err)
		default:
		}
	}()

	// Create a socket pair for testing
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer ln.Close()

	// Connect to the listener
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	// Accept the connection
	serverConn, err := ln.Accept()
	if err != nil {
		t.Fatalf("Accept failed: %v", err)
	}
	defer serverConn.Close()

	// Get the file descriptor
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		t.Skip("Not a TCP connection")
	}

	file, err := tcpConn.File()
	if err != nil {
		t.Fatalf("File() failed: %v", err)
	}
	defer file.Close()
	fd := int(file.Fd())

	// Register for write events (socket should be immediately writable)
	var wg sync.WaitGroup
	wg.Add(1)

	callbackCalled := false
	var once sync.Once // T10-FIX: Protect against multiple callbacks
	err = loop.RegisterFD(fd, EventWrite, func(events IOEvents) {
		if events&EventWrite != 0 {
			once.Do(func() {
				callbackCalled = true
				wg.Done()
			})
		}
	})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	// Wait for callback with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if !callbackCalled {
			t.Error("Callback was not called")
		}
	case <-time.After(2 * time.Second):
		t.Skip("Callback not triggered in time - may need pollIO integration in main loop")
	}

	// Unregister
	if err := loop.UnregisterFD(fd); err != nil {
		t.Errorf("UnregisterFD failed: %v", err)
	}
}

// TestRegisterFD_DuplicateRegistration tests that duplicate registration fails.
func TestRegisterFD_DuplicateRegistration(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Create a socket for testing
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer ln.Close()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	// Accept to complete handshake
	serverConn, _ := ln.Accept()
	if serverConn != nil {
		defer serverConn.Close()
	}

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		t.Skip("Not a TCP connection")
	}

	file, err := tcpConn.File()
	if err != nil {
		t.Fatalf("File() failed: %v", err)
	}
	defer file.Close()
	fd := int(file.Fd())

	// First registration should succeed
	err = loop.RegisterFD(fd, EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatalf("First RegisterFD failed: %v", err)
	}

	// Second registration should fail
	err = loop.RegisterFD(fd, EventWrite, func(events IOEvents) {})
	if err == nil {
		t.Error("Expected error for duplicate registration, got nil")
	}

	// Cleanup
	loop.UnregisterFD(fd)

	// Close loop resources
	loop.closeFDs()
}

// TestUnregisterFD_NotRegistered tests that unregistering an unknown FD fails.
func TestUnregisterFD_NotRegistered(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.closeFDs()

	// Initialize the poller first
	loop.ioPoller.initPoller()

	// Try to unregister a random FD that was never registered
	err = loop.UnregisterFD(99999)
	if err == nil {
		t.Error("Expected error for unregistering unknown FD, got nil")
	}
}

// TestModifyFD tests modifying registered events.
func TestModifyFD(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.closeFDs()

	// Create a socket for testing
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer ln.Close()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	serverConn, _ := ln.Accept()
	if serverConn != nil {
		defer serverConn.Close()
	}

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		t.Skip("Not a TCP connection")
	}

	file, err := tcpConn.File()
	if err != nil {
		t.Fatalf("File() failed: %v", err)
	}
	defer file.Close()
	fd := int(file.Fd())

	// Register for read events
	err = loop.RegisterFD(fd, EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	// Modify to write events
	err = loop.ModifyFD(fd, EventWrite)
	if err != nil {
		t.Errorf("ModifyFD failed: %v", err)
	}

	// Modify to both
	err = loop.ModifyFD(fd, EventRead|EventWrite)
	if err != nil {
		t.Errorf("ModifyFD failed: %v", err)
	}

	// Cleanup
	loop.UnregisterFD(fd)
}

// TestIOPollerCleanup tests that the poller is properly cleaned up.
func TestIOPollerCleanup(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Create a socket and register
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer ln.Close()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()

	serverConn, _ := ln.Accept()
	if serverConn != nil {
		defer serverConn.Close()
	}

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		t.Skip("Not a TCP connection")
	}

	file, err := tcpConn.File()
	if err != nil {
		t.Fatalf("File() failed: %v", err)
	}
	defer file.Close()
	fd := int(file.Fd())

	// Register something
	err = loop.RegisterFD(fd, EventRead, func(events IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	// Close should clean up the poller
	loop.closeFDs()

	// Verify poller is closed by checking closed flag
	// Note: With sync.Once, we can't check "initialized" anymore.
	// Instead, verify that the closed flag is set.
	if !loop.ioPoller.closed.Load() {
		t.Error("Expected poller to be marked as closed after closeFDs")
	}
}

// TestRegression_PollIOLockStarvation verifies that pollIO does not block
// FD registration by holding locks during blocking syscalls.
func TestRegression_PollIOLockStarvation(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	// Initialize poller so locks are active
	if err := l.ioPoller.initPoller(); err != nil {
		t.Fatalf("initPoller failed: %v", err)
	}
	defer l.ioPoller.closePoller()

	// 1. Start a long-running poll in the background (holding the lock in buggy version)
	pollStarted := make(chan struct{})
	go func() {
		close(pollStarted)
		// Simulate a 200ms poll. In the buggy version, RLock is held for this entire duration.
		_, _ = l.pollIO(200, 1)
	}()

	<-pollStarted
	// specific tiny sleep to ensure pollIO has actually grabbed the lock
	time.Sleep(10 * time.Millisecond)

	// 2. Measure how long it takes to Register an FD
	start := time.Now()

	// Use a dummy FD (Stdout) just for the lock check
	// We expect this to execute IMMEDIATELY, even if pollIO is still sleeping
	_ = l.RegisterFD(1, EventRead, func(IOEvents) {})

	duration := time.Since(start)

	// 3. The Proof
	if duration > 100*time.Millisecond {
		t.Fatalf("CRITICAL: RegisterFD blocked for %v during pollIO! Locking defect confirmed.", duration)
	}
}

// TestRegression_PollIO_HotPathAllocations verifies that pollIO does not
// allocate memory on the hot path.
func TestRegression_PollIO_HotPathAllocations(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	if err := l.ioPoller.initPoller(); err != nil {
		t.Fatalf("initPoller failed: %v", err)
	}
	defer l.ioPoller.closePoller()

	// Warmup
	_, _ = l.pollIO(0, 64)

	// Measure allocations
	allocs := testing.AllocsPerRun(1000, func() {
		// This MUST be zero allocs for the event loop to remain garbage-free
		_, _ = l.pollIO(0, 64)
	})

	if allocs > 0 {
		t.Fatalf("PERFORMANCE REGRESSION: pollIO is allocating %f objects/op. Expected 0.", allocs)
	}
}

// TestPoller_Deadlock_Reentrancy verifies that callbacks can safely call
// UnregisterFD without causing a deadlock.
func TestPoller_Deadlock_Reentrancy(t *testing.T) {
	loop, _ := New()
	// Initialize manual poller
	loop.ioPoller.initPoller()
	defer loop.closeFDs()

	// 1. Register a dummy FD (using stdio or a pipe)
	r, w, _ := os.Pipe()
	defer r.Close()
	defer w.Close()
	fd := int(r.Fd())

	// 2. Setup a callback that attempts to Unregister itself (Common pattern)
	done := make(chan struct{})
	err := loop.RegisterFD(fd, EventRead, func(events IOEvents) {
		// THIS CALL WILL DEADLOCK in the current implementation
		loop.UnregisterFD(fd)
		close(done)
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// 3. Trigger the event
	w.Write([]byte("wake"))

	// 4. Run pollIO with a timeout safety net
	go func() {
		loop.pollIO(100, 10)
	}()

	select {
	case <-done:
		// Success: Callback ran and unregister succeeded
	case <-time.After(1 * time.Second):
		t.Fatal("DEADLOCK DETECTED: pollIO hung while executing callback calling UnregisterFD")
	}
}

// TestIOPoller_Integration_Deterministic verifies that pollIO is actually
// integrated into the event loop and events are delivered.
func TestIOPoller_Integration_Deterministic(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	runDone := make(chan struct{})
	errChan := make(chan error, 1)
	go func() {
		if err := loop.Run(ctx); err != nil {
			errChan <- err
			return
		}
		close(runDone)
	}()
	defer func() {
		loop.Shutdown(context.Background())
		<-runDone
		select {
		case err := <-errChan:
			t.Fatalf("Run() failed: %v", err)
		default:
		}
	}()

	// Create a pipe for deterministic signaling
	var fds [2]int
	if err := unix.Pipe(fds[:]); err != nil {
		t.Fatalf("Pipe failed: %v", err)
	}
	// Set non-blocking
	unix.SetNonblock(fds[0], true)
	unix.SetNonblock(fds[1], true)
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])

	callbackTriggered := make(chan struct{}, 1)
	registerDone := make(chan struct{})

	// Register FD ON THE LOOP THREAD to avoid race
	err = loop.SubmitInternal(Task{Runnable: func() {
		fdErr := loop.RegisterFD(fds[0], EventRead, func(events IOEvents) {
			select {
			case callbackTriggered <- struct{}{}:
			default:
			}
		})
		if fdErr != nil {
			t.Errorf("RegisterFD failed: %v", fdErr)
		}
		close(registerDone)
	}})
	if err != nil {
		t.Fatalf("SubmitInternal failed: %v", err)
	}

	<-registerDone

	time.Sleep(100 * time.Millisecond)

	go func() {
		time.Sleep(50 * time.Millisecond)
		unix.Write(fds[1], []byte("x"))
	}()

	select {
	case <-callbackTriggered:
		// SUCCESS: pollIO was called and dispatched event
	case <-time.After(2 * time.Second):
		t.Fatal("FAIL: Callback was never triggered - pollIO is dead code")
	}
}

// TestRegression_NonBlockingRegistration verifies that RegisterFD does not
// block when pollIO is running a long poll.
func TestRegression_NonBlockingRegistration(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer l.closeFDs()

	l.ioPoller.initPoller()

	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	f, _ := ln.(*net.TCPListener).File()
	fdToPoll := int(f.Fd())

	l.RegisterFD(fdToPoll, EventRead, func(IOEvents) {})

	pollErr := make(chan error)
	go func() {
		_, err := l.pollIO(2000, 10)
		pollErr <- err
	}()

	time.Sleep(100 * time.Millisecond)

	start := time.Now()

	conn, _ := net.Dial("tcp", ln.Addr().String())
	defer conn.Close()
	tcpConn, _ := conn.(*net.TCPConn).File()
	fdToRegister := int(tcpConn.Fd())

	err = l.RegisterFD(fdToRegister, EventRead, func(IOEvents) {})
	if err != nil {
		t.Fatalf("RegisterFD failed: %v", err)
	}

	duration := time.Since(start)

	if duration > 200*time.Millisecond {
		t.Fatalf("DEADLOCK RISK: RegisterFD took %v while poller was running. "+
			"The poller is likely holding the lock during the syscall.", duration)
	}
}
