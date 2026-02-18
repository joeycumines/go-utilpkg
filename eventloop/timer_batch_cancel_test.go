// EXPAND-034: Tests for batch timer cancellation.

package eventloop

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestCancelTimers_Basic verifies basic batch cancellation works.
func TestCancelTimers_Basic(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- loop.Run(ctx)
	}()

	// Wait for loop to start
	time.Sleep(10 * time.Millisecond)

	// Schedule some timers
	ids := make([]TimerID, 5)
	var counter atomic.Int64
	for i := range 5 {
		id, err := loop.ScheduleTimer(1*time.Hour, func() {
			counter.Add(1)
		})
		if err != nil {
			t.Fatalf("ScheduleTimer failed: %v", err)
		}
		ids[i] = id
	}

	// Cancel all timers in batch
	errors := loop.CancelTimers(ids)
	if len(errors) != 5 {
		t.Fatalf("expected 5 errors, got %d", len(errors))
	}

	for i, err := range errors {
		if err != nil {
			t.Errorf("expected nil error for timer %d, got %v", i, err)
		}
	}

	// Verify none fired
	time.Sleep(50 * time.Millisecond)
	if counter.Load() != 0 {
		t.Errorf("expected 0 timers fired, got %d", counter.Load())
	}

	cancel()
	<-errCh
}

// TestCancelTimers_MixedResults verifies mixed success/failure in batch.
func TestCancelTimers_MixedResults(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- loop.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)

	// Schedule 3 timers
	id1, _ := loop.ScheduleTimer(1*time.Hour, func() {})
	id2, _ := loop.ScheduleTimer(1*time.Hour, func() {})
	id3, _ := loop.ScheduleTimer(1*time.Hour, func() {})

	// Try to cancel mix of valid and invalid IDs
	ids := []TimerID{id1, 99999, id2, 88888, id3}
	errors := loop.CancelTimers(ids)

	if len(errors) != 5 {
		t.Fatalf("expected 5 errors, got %d", len(errors))
	}

	// id1 should succeed
	if errors[0] != nil {
		t.Errorf("expected nil for id1, got %v", errors[0])
	}
	// 99999 should be not found
	if errors[1] != ErrTimerNotFound {
		t.Errorf("expected ErrTimerNotFound for 99999, got %v", errors[1])
	}
	// id2 should succeed
	if errors[2] != nil {
		t.Errorf("expected nil for id2, got %v", errors[2])
	}
	// 88888 should be not found
	if errors[3] != ErrTimerNotFound {
		t.Errorf("expected ErrTimerNotFound for 88888, got %v", errors[3])
	}
	// id3 should succeed
	if errors[4] != nil {
		t.Errorf("expected nil for id3, got %v", errors[4])
	}

	cancel()
	<-errCh
}

// TestCancelTimers_Empty verifies empty slice returns nil.
func TestCancelTimers_Empty(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- loop.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)

	errors := loop.CancelTimers([]TimerID{})
	if errors != nil {
		t.Errorf("expected nil for empty input, got %v", errors)
	}

	errors = loop.CancelTimers(nil)
	if errors != nil {
		t.Errorf("expected nil for nil input, got %v", errors)
	}

	cancel()
	<-errCh
}

// TestCancelTimers_AllNotFound verifies all-not-found case.
func TestCancelTimers_AllNotFound(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- loop.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)

	ids := []TimerID{99999, 88888, 77777}
	errors := loop.CancelTimers(ids)

	if len(errors) != 3 {
		t.Fatalf("expected 3 errors, got %d", len(errors))
	}

	for i, err := range errors {
		if err != ErrTimerNotFound {
			t.Errorf("expected ErrTimerNotFound for index %d, got %v", i, err)
		}
	}

	cancel()
	<-errCh
}

// TestCancelTimers_DuplicateIDs verifies duplicate IDs are handled.
func TestCancelTimers_DuplicateIDs(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- loop.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)

	id, _ := loop.ScheduleTimer(1*time.Hour, func() {})

	// Cancel same ID twice
	ids := []TimerID{id, id}
	errors := loop.CancelTimers(ids)

	if len(errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(errors))
	}

	// First should succeed
	if errors[0] != nil {
		t.Errorf("expected nil for first cancel, got %v", errors[0])
	}
	// Second should be not found (already cancelled)
	if errors[1] != ErrTimerNotFound {
		t.Errorf("expected ErrTimerNotFound for second cancel, got %v", errors[1])
	}

	cancel()
	<-errCh
}

// TestCancelTimers_LoopNotRunning verifies error when loop not running.
func TestCancelTimers_LoopNotRunning(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Close()

	// Loop not started yet
	ids := []TimerID{1, 2, 3}
	errors := loop.CancelTimers(ids)

	if len(errors) != 3 {
		t.Fatalf("expected 3 errors, got %d", len(errors))
	}

	for i, err := range errors {
		if err != ErrLoopNotRunning {
			t.Errorf("expected ErrLoopNotRunning for index %d, got %v", i, err)
		}
	}
}

// TestCancelTimers_LargeBatch verifies performance with large batch.
func TestCancelTimers_LargeBatch(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- loop.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)

	// Schedule 100 timers
	numTimers := 100
	ids := make([]TimerID, numTimers)
	for i := range numTimers {
		id, err := loop.ScheduleTimer(1*time.Hour, func() {})
		if err != nil {
			t.Fatalf("ScheduleTimer failed: %v", err)
		}
		ids[i] = id
	}

	// Cancel all in one batch
	start := time.Now()
	errors := loop.CancelTimers(ids)
	elapsed := time.Since(start)

	if len(errors) != numTimers {
		t.Fatalf("expected %d errors, got %d", numTimers, len(errors))
	}

	for i, err := range errors {
		if err != nil {
			t.Errorf("expected nil error for timer %d, got %v", i, err)
		}
	}

	// Should complete quickly (< 100ms for 100 timers)
	if elapsed > 100*time.Millisecond {
		t.Logf("warning: batch cancellation took %v for %d timers", elapsed, numTimers)
	}

	cancel()
	<-errCh
}

// TestCancelTimers_PreventsFiring verifies cancelled timers don't fire.
func TestCancelTimers_PreventsFiring(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- loop.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)

	var fired atomic.Int64
	ids := make([]TimerID, 5)
	for i := range 5 {
		id, _ := loop.ScheduleTimer(50*time.Millisecond, func() {
			fired.Add(1)
		})
		ids[i] = id
	}

	// Cancel before they fire
	errors := loop.CancelTimers(ids)
	for i, err := range errors {
		if err != nil {
			t.Errorf("cancel failed for timer %d: %v", i, err)
		}
	}

	// Wait past their scheduled time
	time.Sleep(100 * time.Millisecond)

	if fired.Load() != 0 {
		t.Errorf("expected 0 timers to fire, got %d", fired.Load())
	}

	cancel()
	<-errCh
}

// BenchmarkCancelTimer_Individual benchmarks cancelling timers one at a time.
func BenchmarkCancelTimer_Individual(b *testing.B) {
	for _, numTimers := range []int{10, 50, 100} {
		b.Run("timers_"+string(rune('0'+numTimers/10)), func(b *testing.B) {
			loop, err := New()
			if err != nil {
				b.Fatalf("New() failed: %v", err)
			}
			defer loop.Close()

			ctx := b.Context()

			go func() { _ = loop.Run(ctx) }()
			time.Sleep(10 * time.Millisecond)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Schedule timers
				ids := make([]TimerID, numTimers)
				for j := range numTimers {
					id, _ := loop.ScheduleTimer(1*time.Hour, func() {})
					ids[j] = id
				}

				// Cancel one by one
				for _, id := range ids {
					_ = loop.CancelTimer(id)
				}
			}
			b.StopTimer()
		})
	}
}

// BenchmarkCancelTimers_Batch benchmarks batch timer cancellation.
func BenchmarkCancelTimers_Batch(b *testing.B) {
	for _, numTimers := range []int{10, 50, 100} {
		b.Run("timers_"+string(rune('0'+numTimers/10)), func(b *testing.B) {
			loop, err := New()
			if err != nil {
				b.Fatalf("New() failed: %v", err)
			}
			defer loop.Close()

			ctx := b.Context()

			go func() { _ = loop.Run(ctx) }()
			time.Sleep(10 * time.Millisecond)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				// Schedule timers
				ids := make([]TimerID, numTimers)
				for j := range numTimers {
					id, _ := loop.ScheduleTimer(1*time.Hour, func() {})
					ids[j] = id
				}

				// Cancel all at once
				_ = loop.CancelTimers(ids)
			}
			b.StopTimer()
		})
	}
}

// BenchmarkCancelTimers_Comparison directly compares individual vs batch.
func BenchmarkCancelTimers_Comparison(b *testing.B) {
	numTimers := 50

	b.Run("Individual", func(b *testing.B) {
		loop, err := New()
		if err != nil {
			b.Fatalf("New() failed: %v", err)
		}
		defer loop.Close()

		ctx := b.Context()

		go func() { _ = loop.Run(ctx) }()
		time.Sleep(10 * time.Millisecond)

		ids := make([]TimerID, numTimers)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			for j := range numTimers {
				id, _ := loop.ScheduleTimer(1*time.Hour, func() {})
				ids[j] = id
			}
			for _, id := range ids {
				_ = loop.CancelTimer(id)
			}
		}
	})

	b.Run("Batch", func(b *testing.B) {
		loop, err := New()
		if err != nil {
			b.Fatalf("New() failed: %v", err)
		}
		defer loop.Close()

		ctx := b.Context()

		go func() { _ = loop.Run(ctx) }()
		time.Sleep(10 * time.Millisecond)

		ids := make([]TimerID, numTimers)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			for j := range numTimers {
				id, _ := loop.ScheduleTimer(1*time.Hour, func() {})
				ids[j] = id
			}
			_ = loop.CancelTimers(ids)
		}
	})
}
