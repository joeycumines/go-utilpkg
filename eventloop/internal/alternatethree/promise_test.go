package alternatethree

import (
	"errors"
	"runtime"
	"testing"
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
	t.Run("Concurrent resolve attempts", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()
		_, p := registry.NewPromise()

		const numGoroutines = 10
		done := make(chan struct{}, numGoroutines)

		// Try to resolve from multiple goroutines
		for i := range numGoroutines {
			go func(index int) {
				p.Resolve(index)
				done <- struct{}{}
			}(i)
		}
		for range numGoroutines {
			waitAlternateThreeSignal(t, done, "concurrent resolve")
		}

		// Exactly one resolution should have succeeded
		if p.State() != Resolved {
			t.Fatalf("Expected Resolved state, got %v", p.State())
		}
		result, ok := p.Result().(int)
		if !ok || result < 0 || result >= numGoroutines {
			t.Errorf("winning resolve result = %v, want submitted integer", p.Result())
		}
	})

	t.Run("Concurrent reject attempts", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()
		_, p := registry.NewPromise()

		const numGoroutines = 10
		done := make(chan struct{}, numGoroutines)

		// Try to reject from multiple goroutines
		for range numGoroutines {
			go func() {
				p.Reject(errors.New("error"))
				done <- struct{}{}
			}()
		}
		for range numGoroutines {
			waitAlternateThreeSignal(t, done, "concurrent reject")
		}

		// Exactly one rejection should have succeeded
		if p.State() != Rejected {
			t.Fatalf("Expected Rejected state, got %v", p.State())
		}
		result, ok := p.Result().(error)
		if !ok || result.Error() != "error" {
			t.Errorf("winning reject result = %v, want error", p.Result())
		}
	})

	t.Run("Concurrent resolve and reject attempts", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()
		_, p := registry.NewPromise()

		const numGoroutines = 20
		done := make(chan struct{}, numGoroutines)

		for i := range numGoroutines {
			go func(index int) {
				if index%2 == 0 {
					p.Resolve(index)
				} else {
					p.Reject(errors.New("error"))
				}
				done <- struct{}{}
			}(i)
		}
		for range numGoroutines {
			waitAlternateThreeSignal(t, done, "concurrent mixed settlement")
		}

		// Exactly one settlement should have succeeded
		state := p.State()
		if state != Resolved && state != Rejected {
			t.Fatalf("Expected settled state, got %v", state)
		}
		if state == Resolved {
			value, ok := p.Result().(int)
			if !ok || value < 0 || value >= numGoroutines || value%2 != 0 {
				t.Errorf("winning resolved value = %v, want submitted even integer", p.Result())
			}
		} else if reason, ok := p.Result().(error); !ok || reason.Error() != "error" {
			t.Errorf("winning rejected reason = %v, want error", p.Result())
		}
	})
}

// Test_Promise_ToChannel tests promise result retrieval via channel
func Test_Promise_ToChannel(t *testing.T) {
	t.Parallel()

	t.Run("ToChannel returns result when resolved after subscription", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()
		_, p := registry.NewPromise()

		resultCh := p.ToChannel()

		p.Resolve("result")
		select {
		case result := <-resultCh:
			if result != "result" {
				t.Fatalf("Expected 'result', got %v", result)
			}
		default:
			t.Fatal("resolved result was not published synchronously")
		}
		if _, ok := <-resultCh; ok {
			t.Fatal("result channel remained open after resolution")
		}
	})

	t.Run("ToChannel returns error when rejected", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()
		_, p := registry.NewPromise()

		resultCh := p.ToChannel()

		p.Reject(errors.New("rejection"))
		select {
		case result := <-resultCh:
			reason, ok := result.(error)
			if !ok || reason.Error() != "rejection" {
				t.Fatalf("Expected error 'rejection', got %v", result)
			}
		default:
			t.Fatal("rejected result was not published synchronously")
		}
		if _, ok := <-resultCh; ok {
			t.Fatal("result channel remained open after rejection")
		}
	})
}

// Test_Promise_WeakPointerBehavior tests interaction with weak pointers and GC
func Test_Promise_WeakPointerBehavior(t *testing.T) {
	t.Run("reachable pending promise survives scavenge", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()
		id, promise := registry.NewPromise()
		runtime.GC()
		registry.Scavenge(1)
		registry.mu.RLock()
		_, exists := registry.data[id]
		registry.mu.RUnlock()
		if !exists {
			t.Error("reachable pending promise was removed")
		}
		runtime.KeepAlive(promise)
	})

	t.Run("Scavenger cleans up resolved promises", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()

		// Create and resolve a promise
		id, p := registry.NewPromise()
		p.Resolve("result")

		// Run scavenger
		registry.Scavenge(100)

		registry.mu.RLock()
		_, exists := registry.data[id]
		registry.mu.RUnlock()
		if exists {
			t.Error("settled promise remained in registry after Scavenge")
		}
		runtime.KeepAlive(p)
	})
}

// Test_Promise_CallbackMemoryLeak tests that callbacks don't cause memory leaks
func Test_Promise_CallbackMemoryLeak(t *testing.T) {
	t.Run("Channels are cleaned up after settlement", func(t *testing.T) {
		t.Parallel()

		registry := newRegistry()
		id, p := registry.NewPromise()

		// Register many channels
		const numChannels = 1000
		channels := make([]<-chan Result, numChannels)

		for i := range numChannels {
			channels[i] = p.ToChannel()
		}

		// Resolve promise (triggers all channels)
		p.Resolve("result")

		for index, ch := range channels {
			select {
			case result := <-ch:
				if result != "result" {
					t.Errorf("channel %d result = %v, want result", index, result)
				}
			default:
				t.Errorf("channel %d had no result", index)
				continue
			}
			if _, ok := <-ch; ok {
				t.Errorf("channel %d remained open", index)
			}
		}
		p.mu.Lock()
		subscriberCount := len(p.subscribers)
		p.mu.Unlock()
		if subscriberCount != 0 {
			t.Errorf("subscriber count after settlement = %d, want 0", subscriberCount)
		}
		registry.Scavenge(200)
		registry.mu.RLock()
		_, exists := registry.data[id]
		registry.mu.RUnlock()
		if exists {
			t.Error("settled promise remained registered")
		}
	})
}
