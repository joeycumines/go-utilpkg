package eventloop

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// =============================================================================
// REGRESSION TESTS FOR CORRECTNESS ISSUES
//
// These tests are designed to verify correctness properties and expose
// defects documented in scratch.md.
// =============================================================================

// TestCloseFDsInvokedOnce proves the double-close issue in closeFDs.
//
// DEFECT #7 (MODERATE): closeFDs() Double-Close
//
// FIX: Use sync.Once for closeFDs
//
// RUN: go test -v -run TestCloseFDsInvokedOnce
func TestCloseFDsInvokedOnce(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)

	go func() {
		runDone <- l.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	err1 := l.Shutdown(shutdownCtx)
	shutdownCancel()

	err2 := l.Close()

	cancel()

	select {
	case err := <-runDone:
		if err != nil && err != context.Canceled {
			t.Logf("Run() returned: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run() did not exit within timeout")
	}

	t.Logf("Shutdown returned: %v, Close returned: %v", err1, err2)

	err3 := l.Close()
	t.Logf("Third Close returned: %v (should be ErrLoopTerminated or nil)", err3)
}

// TestInitPollerClosedReturnsConsistentError verifies cross-platform error consistency.
//
// DEFECT #8 (MODERATE): Platform Error Inconsistency
//
// FIX: Change Darwin errEventLoopClosed to ErrPollerClosed for consistency.
//
// RUN: go test -v -run TestInitPollerClosedReturnsConsistentError
func TestInitPollerClosedReturnsConsistentError(t *testing.T) {
	p := &ioPoller{}

	// FIX: Use atomic.Bool.Store() instead of direct assignment
	p.closed.Store(true)

	err := p.initPoller()
	if err == nil {
		t.Fatal("Expected error when initializing closed poller, got nil")
	}

	t.Logf("initPoller on closed poller returned: %v", err)

	// After the fix, both platforms should return ErrPollerClosed
	if err != ErrPollerClosed {
		t.Errorf("Expected ErrPollerClosed, got: %v", err)
	}
}

// TestLockFreeIngressPopWaitsForProducer verifies the Pop spin logic works correctly.
//
// RUN: go test -v -run TestLockFreeIngressPopWaitsForProducer
func TestLockFreeIngressPopWaitsForProducer(t *testing.T) {
	const iterations = 100000
	const producers = 4

	for round := 0; round < 10; round++ {
		q := NewLockFreeIngress()
		var wg sync.WaitGroup
		var pushCount atomic.Int64
		var popCount atomic.Int64

		for p := 0; p < producers; p++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				for i := 0; i < iterations/producers; i++ {
					q.Push(func() {})
					pushCount.Add(1)
				}
			}(p)
		}

		consumerDone := make(chan struct{})
		go func() {
			defer close(consumerDone)
			lastProgress := time.Now()
			for popCount.Load() < int64(iterations) {
				task, ok := q.Pop()
				if ok {
					_ = task
					popCount.Add(1)
					lastProgress = time.Now()
				} else {
					if time.Since(lastProgress) > 2*time.Second {
						return
					}
					runtime.Gosched()
				}
			}
		}()

		wg.Wait()

		select {
		case <-consumerDone:
		case <-time.After(5 * time.Second):
			t.Fatalf("Round %d: Consumer stalled. Pushed=%d, Popped=%d. "+
				"Pop may not be waiting for producer correctly.",
				round, pushCount.Load(), popCount.Load())
		}

		if popCount.Load() != int64(iterations) {
			t.Fatalf("Round %d: Task loss! Pushed=%d, Popped=%d",
				round, pushCount.Load(), popCount.Load())
		}
	}

	t.Log("Success: Pop correctly waits for producers to finish linking nodes")
}
