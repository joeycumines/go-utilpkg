// EXPAND-033: Tests for configurable chunk size in chunkedIngress.

package eventloop

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestWithIngressChunkSize_Default verifies default chunk size is 64.
func TestWithIngressChunkSize_Default(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Close()

	// The default should be 64 (defaultIngressChunkSize)
	if loop.external.chunkSize != defaultIngressChunkSize {
		t.Errorf("expected external chunk size %d, got %d", defaultIngressChunkSize, loop.external.chunkSize)
	}
	if loop.internal.chunkSize != defaultIngressChunkSize {
		t.Errorf("expected internal chunk size %d, got %d", defaultIngressChunkSize, loop.internal.chunkSize)
	}
}

// TestWithIngressChunkSize_Custom verifies custom chunk sizes are applied.
func TestWithIngressChunkSize_Custom(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{"exact power of 2 - 16", 16, 16},
		{"exact power of 2 - 32", 32, 32},
		{"exact power of 2 - 64", 64, 64},
		{"exact power of 2 - 128", 128, 128},
		{"exact power of 2 - 256", 256, 256},
		{"exact power of 2 - 512", 512, 512},
		{"exact power of 2 - 1024", 1024, 1024},
		{"exact power of 2 - 2048", 2048, 2048},
		{"exact power of 2 - 4096", 4096, 4096},
		{"not power of 2 - 17", 17, 16},
		{"not power of 2 - 50", 50, 32},
		{"not power of 2 - 100", 100, 64},
		{"not power of 2 - 200", 200, 128},
		{"not power of 2 - 1000", 1000, 512},
		{"not power of 2 - 3000", 3000, 2048},
		{"below min - 1", 1, 16},
		{"below min - 8", 8, 16},
		{"below min - 15", 15, 16},
		{"above max - 5000", 5000, 4096},
		{"above max - 10000", 10000, 4096},
		{"zero", 0, 16},
		{"negative", -1, 16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loop, err := New(WithIngressChunkSize(tt.input))
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}
			defer loop.Close()

			if loop.external.chunkSize != tt.expected {
				t.Errorf("expected chunk size %d, got %d", tt.expected, loop.external.chunkSize)
			}
		})
	}
}

// TestWithIngressChunkSize_FunctionalCorrectness verifies that tasks are properly
// processed with custom chunk sizes.
func TestWithIngressChunkSize_FunctionalCorrectness(t *testing.T) {
	sizes := []int{16, 32, 64, 128, 256}

	for _, size := range sizes {
		t.Run("chunkSize_"+string(rune('0'+size/16)), func(t *testing.T) {
			loop, err := New(WithIngressChunkSize(size))
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}
			defer loop.Close()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Run the loop in background
			errCh := make(chan error, 1)
			go func() {
				errCh <- loop.Run(ctx)
			}()

			// Wait for loop to start
			time.Sleep(10 * time.Millisecond)

			// Submit more tasks than chunk size to test chunking
			numTasks := size * 3
			var counter atomic.Int64
			var wg sync.WaitGroup
			wg.Add(numTasks)

			for i := 0; i < numTasks; i++ {
				if err := loop.Submit(func() {
					counter.Add(1)
					wg.Done()
				}); err != nil {
					t.Errorf("Submit failed: %v", err)
				}
			}

			// Wait for all tasks
			done := make(chan struct{})
			go func() {
				wg.Wait()
				close(done)
			}()

			select {
			case <-done:
				if counter.Load() != int64(numTasks) {
					t.Errorf("expected %d tasks executed, got %d", numTasks, counter.Load())
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("timeout waiting for tasks, executed %d/%d", counter.Load(), numTasks)
			}

			// Stop the loop
			cancel()
			<-errCh
		})
	}
}

// TestWithIngressChunkSize_ChunkPoolReuse verifies that chunk pools are working
// correctly with configurable sizes.
func TestWithIngressChunkSize_ChunkPoolReuse(t *testing.T) {
	// Create ingress with custom size
	q := newChunkedIngressWithSize(32)

	// Push more than one chunk's worth
	for i := 0; i < 64; i++ {
		q.Push(func() {})
	}

	// Pop all tasks
	for i := 0; i < 64; i++ {
		task, ok := q.Pop()
		if !ok {
			t.Fatalf("Pop() returned false at index %d", i)
		}
		if task == nil {
			t.Fatalf("Pop() returned nil task at index %d", i)
		}
	}

	// Queue should be empty
	_, ok := q.Pop()
	if ok {
		t.Error("expected empty queue after popping all tasks")
	}

	// Push more tasks - should reuse pooled chunks
	for i := 0; i < 32; i++ {
		q.Push(func() {})
	}

	if q.Length() != 32 {
		t.Errorf("expected length 32, got %d", q.Length())
	}
}

// TestWithIngressChunkSize_CombinedWithOtherOptions tests that chunk size option
// works correctly with other options.
func TestWithIngressChunkSize_CombinedWithOtherOptions(t *testing.T) {
	loop, err := New(
		WithIngressChunkSize(128),
		WithMetrics(true),
		WithFastPathMode(FastPathAuto),
		WithStrictMicrotaskOrdering(true),
	)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer loop.Close()

	if loop.external.chunkSize != 128 {
		t.Errorf("expected chunk size 128, got %d", loop.external.chunkSize)
	}
	if loop.metrics == nil {
		t.Error("expected metrics to be enabled")
	}
	if !loop.StrictMicrotaskOrdering {
		t.Error("expected strict microtask ordering to be enabled")
	}
}

// TestRoundDownToPowerOf2 tests the power of 2 rounding function.
func TestRoundDownToPowerOf2(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		{1, 1},
		{2, 2},
		{3, 2},
		{4, 4},
		{5, 4},
		{7, 4},
		{8, 8},
		{9, 8},
		{15, 8},
		{16, 16},
		{17, 16},
		{31, 16},
		{32, 32},
		{63, 32},
		{64, 64},
		{127, 64},
		{128, 128},
		{255, 128},
		{256, 256},
		{1000, 512},
		{1024, 1024},
		{2000, 1024},
		{2048, 2048},
		{4000, 2048},
		{4096, 4096},
		{8000, 4096},
		{0, 1},
		{-1, 1},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := roundDownToPowerOf2(tt.input)
			if result != tt.expected {
				t.Errorf("roundDownToPowerOf2(%d) = %d, expected %d", tt.input, result, tt.expected)
			}
		})
	}
}

// Test_newChunkedIngressWithSize_EdgeCases tests edge cases for newChunkedIngressWithSize.
func Test_newChunkedIngressWithSize_EdgeCases(t *testing.T) {
	// Zero size should use default
	q := newChunkedIngressWithSize(0)
	if q.chunkSize != defaultChunkSize {
		t.Errorf("expected default chunk size %d for zero input, got %d", defaultChunkSize, q.chunkSize)
	}

	// Negative size should use default
	q = newChunkedIngressWithSize(-10)
	if q.chunkSize != defaultChunkSize {
		t.Errorf("expected default chunk size %d for negative input, got %d", defaultChunkSize, q.chunkSize)
	}

	// Very large size is accepted (caller's responsibility to be reasonable)
	q = newChunkedIngressWithSize(1000000)
	if q.chunkSize != 1000000 {
		t.Errorf("expected chunk size 1000000, got %d", q.chunkSize)
	}
}

// Test_chunkedIngress_ChunkSizeConsistency verifies chunk size is consistent throughout lifecycle.
func Test_chunkedIngress_ChunkSizeConsistency(t *testing.T) {
	sizes := []int{16, 64, 256, 1024}

	for _, size := range sizes {
		t.Run("", func(t *testing.T) {
			q := newChunkedIngressWithSize(size)

			// Initial state
			if q.chunkSize != size {
				t.Errorf("initial chunk size: expected %d, got %d", size, q.chunkSize)
			}

			// After many push/pop cycles
			for cycle := 0; cycle < 5; cycle++ {
				for i := 0; i < size*2; i++ {
					q.Push(func() {})
				}
				for i := 0; i < size*2; i++ {
					q.Pop()
				}
			}

			// Chunk size should still be the same
			if q.chunkSize != size {
				t.Errorf("after cycles chunk size: expected %d, got %d", size, q.chunkSize)
			}
		})
	}
}
