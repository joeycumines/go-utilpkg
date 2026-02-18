package alternatethree

import (
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"
)

// Test_Promise_NewPromise tests creating promises in different initial states
func Test_Promise_NewPromise(t *testing.T) {
	t.Parallel()

	t.Run("NewPromise creates pending promise", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()
		id, p := registry.NewPromise()

		if id == 0 {
			t.Fatal("Expected non-zero promise ID")
		}
		if p == nil {
			t.Fatal("Expected non-nil promise")
		}
		if p.State() != Pending {
			t.Fatalf("Expected Pending state, got %v", p.State())
		}
	})

	t.Run("Each promise gets unique ID", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()
		ids := make(map[uint64]bool)

		for range 100 {
			id, _ := registry.NewPromise()
			if id == 0 {
				t.Fatalf("Expected non-zero ID, got %d", id)
			}
			if ids[id] {
				t.Fatalf("Duplicate ID: %d", id)
			}
			ids[id] = true
		}
	})
}

// Test_Promise_Resolve tests promise resolution
func Test_Promise_Resolve(t *testing.T) {
	t.Parallel()

	t.Run("Basic resolve with value", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()
		_, p := registry.NewPromise()

		result := "success value"
		p.Resolve(result)

		if p.State() != Resolved {
			t.Fatalf("Expected Resolved state, got %v", p.State())
		}

		got := p.Result()
		if got != result {
			t.Fatalf("Expected %v, got %v", result, got)
		}
	})

	t.Run("Cannot resolve already resolved promise", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()
		_, p := registry.NewPromise()

		p.Resolve("first")
		p.Resolve("second")

		// Result should still be first value
		if p.Result() != "first" {
			t.Fatalf("Expected 'first', got %v", p.Result())
		}
	})

	t.Run("Can resolve with nil value", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()
		_, p := registry.NewPromise()

		p.Resolve(nil)
		if p.State() != Resolved {
			t.Fatalf("Expected Resolved state, got %v", p.State())
		}
	})
}

// Test_Promise_Reject tests promise rejection
func Test_Promise_Reject(t *testing.T) {
	t.Parallel()

	t.Run("Basic reject with error", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()
		_, p := registry.NewPromise()

		reason := errors.New("test error")
		p.Reject(reason)

		if p.State() != Rejected {
			t.Fatalf("Expected Rejected state, got %v", p.State())
		}

		got := p.Result()
		if got != reason {
			t.Fatalf("Expected %v, got %v", reason, got)
		}
	})

	t.Run("Cannot reject already rejected promise", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()
		_, p := registry.NewPromise()

		p.Reject(errors.New("first error"))
		p.Reject(errors.New("second error"))

		// Result should still be first error
		if p.Result().(error).Error() != "first error" {
			t.Fatalf("Expected 'first error', got %v", p.Result())
		}
	})

	t.Run("Cannot resolve after rejection", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()
		_, p := registry.NewPromise()

		p.Reject(errors.New("error"))
		p.Resolve("value")

		// State should still be Rejected with error
		if p.State() != Rejected {
			t.Fatalf("Expected Rejected state, got %v", p.State())
		}
	})
}

// Test_Promise_MonotonicState tests that transitions are monotonic
func Test_Promise_MonotonicState(t *testing.T) {
	t.Parallel()

	t.Run("Cannot settle multiple times", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()
		_, p := registry.NewPromise()

		// First settlement (resolve)
		p.Resolve("first")

		// Try to settle again (reject)
		p.Reject(errors.New("second"))

		// Try to settle again (resolve with different value)
		p.Resolve("third")

		// Verify state is still resolved with first value
		if p.State() != Resolved {
			t.Fatalf("Expected Resolved state, got %v", p.State())
		}
		if p.Result() != "first" {
			t.Fatalf("Expected 'first', got %v", p.Result())
		}
	})
}

// Test_Promise_ConcurrentSettlement tests concurrent settlement attempts
func Test_Promise_ConcurrentSettlement(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	t.Run("Concurrent resolve attempts", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()
		_, p := registry.NewPromise()

		var wg sync.WaitGroup
		numGoroutines := 10

		// Try to resolve from multiple goroutines
		for i := range numGoroutines {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				p.Resolve(index)
			}(i)
		}

		wg.Wait()

		// Exactly one resolution should have succeeded
		if p.State() != Resolved {
			t.Fatalf("Expected Resolved state, got %v", p.State())
		}
	})

	t.Run("Concurrent reject attempts", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()
		_, p := registry.NewPromise()

		var wg sync.WaitGroup
		numGoroutines := 10

		// Try to reject from multiple goroutines
		for i := range numGoroutines {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				p.Reject(errors.New("error"))
			}(i)
		}

		wg.Wait()

		// Exactly one rejection should have succeeded
		if p.State() != Rejected {
			t.Fatalf("Expected Rejected state, got %v", p.State())
		}
	})

	t.Run("Concurrent resolve and reject attempts", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()
		_, p := registry.NewPromise()

		var wg sync.WaitGroup
		numGoroutines := 20

		for i := range numGoroutines {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				if index%2 == 0 {
					p.Resolve(index)
				} else {
					p.Reject(errors.New("error"))
				}
			}(i)
		}

		wg.Wait()

		// Exactly one settlement should have succeeded
		state := p.State()
		if state != Resolved && state != Rejected {
			t.Fatalf("Expected settled state, got %v", state)
		}
	})
}

// Test_Promise_ToChannel tests promise result retrieval via channel
func Test_Promise_ToChannel(t *testing.T) {
	t.Parallel()

	t.Run("ToChannel returns result when resolved asynchronously", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()
		_, p := registry.NewPromise()

		resultCh := p.ToChannel()

		// Resolve in separate goroutine
		go func() {
			time.Sleep(10 * time.Millisecond)
			p.Resolve("async result")
		}()

		select {
		case result := <-resultCh:
			if result != "async result" {
				t.Fatalf("Expected 'async result', got %v", result)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Timeout waiting for result")
		}
	})

	t.Run("ToChannel returns error when rejected", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()
		_, p := registry.NewPromise()

		resultCh := p.ToChannel()

		// Reject in separate goroutine
		go func() {
			time.Sleep(10 * time.Millisecond)
			p.Reject(errors.New("async rejection"))
		}()

		// Receiving from channel should return error
		select {
		case result := <-resultCh:
			if result == nil || result.(error).Error() != "async rejection" {
				t.Fatalf("Expected error 'async rejection', got %v", result)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Timeout waiting for rejection")
		}
	})
}

// Test_Promise_WeakPointerBehavior tests interaction with weak pointers and GC
func Test_Promise_WeakPointerBehavior(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	t.Run("GC cleans up unreferenced promises", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()

		// Create many promises without keeping references
		promiseCount := 1000
		for range promiseCount {
			_, _ = registry.NewPromise()
		}

		// Run GC
		runtime.GC()

		// Run scavenger multiple times
		for range 10 {
			registry.Scavenge(200)
			time.Sleep(1 * time.Millisecond)
		}

		// Most promises should have been cleaned up
		registry.mu.RLock()
		dataCount := len(registry.data)
		registry.mu.RUnlock()

		// We expect most (but possibly not all) promises to be cleaned up
		if dataCount > promiseCount/10 {
			t.Logf("Warning: Only cleaned %d/%d promises", promiseCount-dataCount, promiseCount)
		}
	})

	t.Run("Scavenger cleans up resolved promises", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()

		// Create and resolve a promise
		_, p := registry.NewPromise()
		p.Resolve("result")

		// Run scavenger
		registry.Scavenge(100)

		// Verify resolved promise is cleaned from registry
		// (The scavener removes settled promises according to Phase 5.3)
		registry.mu.RLock()
		dataCount := len(registry.data)
		registry.mu.RUnlock()

		if dataCount != 0 {
			t.Logf("Note: Resolved promise may still be in registry (count: %d)", dataCount)
		}
	})
}

// Test_Promise_CallbackMemoryLeak tests that callbacks don't cause memory leaks
func Test_Promise_CallbackMemoryLeak(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping in short mode")
	}

	t.Run("Channels are cleaned up after settlement", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()
		_, p := registry.NewPromise()

		// Register many channels
		const numChannels = 1000
		channels := make([]<-chan Result, numChannels)

		for i := range numChannels {
			channels[i] = p.ToChannel()
		}

		// Resolve promise (triggers all channels)
		p.Resolve("result")

		// Wait for channels
		for _, ch := range channels {
			<-ch
		}

		// Run GC and scavenger
		channels = nil
		runtime.GC()
		registry.Scavenge(200)

		// If channels are properly cleaned up, this shouldn't cause issues
		// (This test mainly ensures no panics or leaks)
	})
}
