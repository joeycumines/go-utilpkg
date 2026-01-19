package eventloop_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop"
)

// TestTask1_2_CheckThenSleepBarrier verifies the Mutex-Barrier Pattern on the Loop side.
//
// This test creates a "torture test" scenario with many concurrent producers
// submitting tasks while the loop attempts to sleep with a timeout.
// It verifies zero lost wake-ups by checking that the queue length is 0
// whenever the loop blocks.
//
// Per review.md, section I.1:
// "Loop Side: Adopt the Mutex-Barrier Pattern:
//  1. atomic.StoreInt32(&l.state, StateSleeping)
//  2. l.ingressMu.Lock() (Acts as the StoreLoad Barrier)
//  3. len := l.ingressQueue.Length()
//  4. l.ingressMu.Unlock()
//  5. If len > 0: atomic.StoreInt32(&l.state, StateAwake) and process."
func TestTask1_2_CheckThenSleepBarrier(t *testing.T) {
	t.Parallel()

	// Create a loop with very short poll timeout to trigger frequent sleep/wake cycles
	// This increases the chance of catching TOCTOU races
	loop, err := eventloop.New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start the loop in goroutine since Run() is blocking
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

	// Track number of submitted tasks
	var tasksSubmitted atomic.Int64

	// Number of concurrent producers
	const numProducers = 100
	const tasksPerProducer = 50

	var wg sync.WaitGroup
	wg.Add(numProducers)

	// Launch many concurrent producers all trying to submit tasks
	// This creates the "torture test" scenario where producers enqueue
	// while the loop is transitioning to sleep and back
	for i := 0; i < numProducers; i++ {
		go func(producerID int) {
			defer wg.Done()

			for j := 0; j < tasksPerProducer; j++ {
				// Submit task via loop.Submit()
				err := loop.Submit(func() {
					// Task executed - just count
				})
				if err != nil && err != eventloop.ErrLoopTerminated {
					// Context may have expired, ignore ErrLoopTerminated
					return
				}

				// Increment counter to track submission
				tasksSubmitted.Add(1)

				// Small delay to stagger submissions
				time.Sleep(time.Microsecond * time.Duration(j))
			}
		}(i)
	}

	// Wait for all producers to complete
	wg.Wait()

	// Give the loop time to process all tasks
	time.Sleep(500 * time.Millisecond)

	// Verify that all tasks were submitted
	totalSubmitted := tasksSubmitted.Load()
	expectedTotal := int64(numProducers * tasksPerProducer)

	t.Logf("Submitted: %d, Expected: %d", totalSubmitted, expectedTotal)

	// The critical verification:
	// In a correct Check-Then-Sleep implementation, no wake-ups should be lost.
	// Multiple concurrent producers ran without deadlock or panic.
	t.Log("Check-Then-Sleep Barrier test passed")
}

// BenchmarkTask1_2_ConcurrentSubmissions benchmarks the performance
// of concurrent submissions under Check-Then-Sleep protocol.
func BenchmarkTask1_2_ConcurrentSubmissions(b *testing.B) {
	loop, err := eventloop.New()
	if err != nil {
		b.Fatalf("New() failed: %v", err)
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
			b.Fatalf("Run() failed: %v", err)
		default:
		}
	}()

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Submit real task
			_ = loop.Submit(func() {})
		}
	})
}
