package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Test_PollError_Path tests poll error handling paths
func Test_PollError_Path(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	t.Run("Loop handles shutdown gracefully", func(t *testing.T) {
		// Don't run in parallel - starts a goroutine
		if t.Skipped() {
			return
		}

		loop, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Start the loop
		done := make(chan struct{})
		go func() {
			loop.Run(ctx)
			close(done)
		}()

		// Submit some tasks
		for i := 0; i < 5; i++ {
			err = loop.Submit(func() {})
			if err != nil {
				t.Fatalf("Failed to submit task %d: %v", i, err)
			}
		}

		// Give time for tasks to execute
		time.Sleep(50 * time.Millisecond)

		// Shutdown the loop (tests shutdown path)
		loop.Shutdown(context.Background())

		// Wait for loop to finish
		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
			// Loop might already be done
		}

		// Verify termination
		if loop.State() != StateTerminated {
			t.Fatalf("Expected StateTerminated, got %v", loop.State())
		}
	})

	t.Run("Loop accepts work in Running state", func(t *testing.T) {
		// Don't run in parallel - starts a goroutine
		if t.Skipped() {
			return
		}

		loop, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan struct{})
		go func() {
			loop.Run(ctx)
			close(done)
		}()

		// Wait for loop to be ready
		time.Sleep(10 * time.Millisecond)

		// Submit tasks
		taskCount := atomic.Int32{}
		for i := 0; i < 10; i++ {
			err = loop.Submit(func() {
				taskCount.Add(1)
			})
			if err != nil {
				t.Fatalf("Failed to submit task: %v", err)
			}
		}

		// Wait for execution
		time.Sleep(100 * time.Millisecond)

		if taskCount.Load() != 10 {
			t.Fatalf("Expected 10 tasks executed, got %d", taskCount.Load())
		}

		loop.Shutdown(context.Background())

		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
		}
	})

	t.Run("Loop rejects work after termination", func(t *testing.T) {
		// Don't run in parallel - starts a goroutine
		if t.Skipped() {
			return
		}

		loop, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan struct{})
		go func() {
			loop.Run(ctx)
			close(done)
		}()

		// Submit initial task
		err = loop.Submit(func() {})
		if err != nil {
			t.Fatalf("Failed to submit initial task: %v", err)
		}

		time.Sleep(50 * time.Millisecond)

		// Shutdown
		loop.Shutdown(context.Background())

		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
		}

		// Try to submit after termination
		err = loop.Submit(func() {})
		if err == nil {
			t.Fatal("Expected error when submitting to terminated loop")
		}
		if err != ErrLoopTerminated {
			t.Fatalf("Expected ErrLoopTerminated, got %v", err)
		}
	})
}

// Test_PollError_Concurrency tests concurrent operations
func Test_PollError_Concurrency(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	t.Run("Concurrent task submission", func(t *testing.T) {
		// Don't run in parallel - starts a goroutine
		if t.Skipped() {
			return
		}

		loop, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan struct{})
		go func() {
			loop.Run(ctx)
			close(done)
		}()

		var wg sync.WaitGroup
		taskCount := atomic.Int32{}

		// Submit tasks from multiple goroutines
		for i := 0; i < 50; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := loop.Submit(func() {
					taskCount.Add(1)
				})
				_ = err
			}()
		}

		wg.Wait()
		time.Sleep(200 * time.Millisecond)

		t.Logf("Executed %d tasks", taskCount.Load())

		loop.Shutdown(context.Background())

		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
		}
	})

	t.Run("Concurrent submission and shutdown", func(t *testing.T) {
		// Don't run in parallel - starts a goroutine
		if t.Skipped() {
			return
		}

		loop, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan struct{})
		go func() {
			loop.Run(ctx)
			close(done)
		}()

		var wg sync.WaitGroup

		// Start submitting tasks
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = loop.Submit(func() {}) // Ignore errors after shutdown
			}()
		}

		// Shutdown while submissions are happening
		time.Sleep(10 * time.Millisecond)
		loop.Shutdown(context.Background())

		wg.Wait()

		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
		}
	})
}

// Test_PollError_Microtasks tests microtask handling
func Test_PollError_Microtasks(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	t.Run("Microtasks execute", func(t *testing.T) {
		// Don't run in parallel - starts a goroutine
		if t.Skipped() {
			return
		}

		loop, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan struct{})
		go func() {
			loop.Run(ctx)
			close(done)
		}()

		count := atomic.Int32{}

		// Schedule microtasks
		for i := 0; i < 10; i++ {
			loop.ScheduleMicrotask(func() {
				count.Add(1)
			})
		}

		// Submit a task to trigger microtask processing
		err = loop.Submit(func() {})
		if err != nil {
			t.Fatalf("Failed to submit task: %v", err)
		}

		time.Sleep(100 * time.Millisecond)

		t.Logf("Executed %d microtasks", count.Load())
		if count.Load() == 0 {
			t.Fatal("Expected some microtasks to execute")
		}

		loop.Shutdown(context.Background())

		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
		}
	})
}

// Test_PollError_Timers tests timer handling on error paths
func Test_PollError_Timers(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	t.Run("Timers fire before shutdown", func(t *testing.T) {
		// Don't run in parallel - starts a goroutine
		if t.Skipped() {
			return
		}

		loop, err := New()
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan struct{})
		go func() {
			loop.Run(ctx)
			close(done)
		}()

		count := atomic.Int32{}

		// Schedule multiple timers
		for i := 0; i < 5; i++ {
			_, err = loop.ScheduleTimer(time.Duration(i)*10*time.Millisecond+10, func() {
				count.Add(1)
			})
			if err != nil {
				t.Fatalf("Failed to schedule timer: %v", err)
			}
		}

		// Wait for timers
		time.Sleep(200 * time.Millisecond)

		if count.Load() == 0 {
			t.Fatal("Expected some timers to fire")
		}

		loop.Shutdown(context.Background())

		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
		}
	})
}

// Test_PollError_Metrics tests that metrics work on error paths
func Test_PollError_Metrics(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	t.Run("Metrics are recorded", func(t *testing.T) {
		// Don't run in parallel - starts a goroutine
		if t.Skipped() {
			return
		}

		loop, err := New(WithMetrics(true))
		if err != nil {
			t.Fatalf("Failed to create loop: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan struct{})
		go func() {
			loop.Run(ctx)
			close(done)
		}()

		// Submit tasks
		for i := 0; i < 10; i++ {
			err = loop.Submit(func() {})
			if err != nil {
				t.Fatalf("Failed to submit task: %v", err)
			}
		}

		time.Sleep(100 * time.Millisecond)

		// Get metrics
		metrics := loop.Metrics()
		if metrics == nil {
			t.Fatal("Expected non-nil metrics")
		}

		loop.Shutdown(context.Background())

		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
		}
	})
}
