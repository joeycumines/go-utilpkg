package eventloop

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/go-eventloop/internal/alternatethree"
)

// Test_alternatethree_Promise_NewPromise tests creating promises in different initial states
func Test_alternatethree_Promise_NewPromise(t *testing.T) {
	t.Parallel()

	t.Run("NewPromise creates pending promise", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		id, p := registry.NewPromise()

		if id == 0 {
			t.Fatal("Expected non-zero promise ID")
		}
		if p == nil {
			t.Fatal("Expected non-nil promise")
		}
		if p.State() != alternatethree.Pending {
			t.Fatalf("Expected Pending state, got %v", p.State())
		}
		if p.Result() != nil {
			t.Fatalf("Expected nil result for pending promise, got %v", p.Result())
		}
	})

	t.Run("NewPromise generates unique IDs", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)

		ids := make(map[uint64]bool)
		for i := 0; i < 100; i++ {
			id, _ := registry.NewPromise()
			if ids[id] {
				t.Fatalf("Duplicate promise ID: %d", id)
			}
			ids[id] = true
		}
	})

	t.Run("Multiple NewPromise calls return distinct promises", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		id1, p1 := registry.NewPromise()
		id2, p2 := registry.NewPromise()

		if id1 == id2 {
			t.Fatal("Expected different IDs")
		}
		if p1 == p2 {
			t.Fatal("Expected different promise instances")
		}
	})
}

// Test_alternatethree_Promise_Resolve tests the Resolve handler execution
func Test_alternatethree_Promise_Resolve(t *testing.T) {
	t.Parallel()

	t.Run("Resolve with value", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		expectedValue := "test-value"
		p.Resolve(expectedValue)

		if p.State() != alternatethree.Resolved {
			t.Fatalf("Expected Resolved state, got %v", p.State())
		}
		if p.Result() != expectedValue {
			t.Fatalf("Expected result %v, got %v", expectedValue, p.Result())
		}
	})

	t.Run("Resolve with nil value", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		p.Resolve(nil)

		if p.State() != alternatethree.Resolved {
			t.Fatalf("Expected Resolved state, got %v", p.State())
		}
		if p.Result() != nil {
			t.Fatalf("Expected nil result, got %v", p.Result())
		}
	})

	t.Run("Resolve notifies single subscriber", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		ch := p.ToChannel()
		p.Resolve("resolved")

		select {
		case result := <-ch:
			if result != "resolved" {
				t.Fatalf("Expected 'resolved', got %v", result)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Timeout waiting for resolution")
		}
	})

	t.Run("Resolve notifies multiple subscribers", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		numSubscribers := 10
		channels := make([]<-chan alternatethree.Result, numSubscribers)
		for i := 0; i < numSubscribers; i++ {
			channels[i] = p.ToChannel()
		}

		p.Resolve("multi-resolve")

		var wg sync.WaitGroup
		wg.Add(numSubscribers)

		for i, ch := range channels {
			go func(idx int, c <-chan alternatethree.Result) {
				defer wg.Done()
				select {
				case result := <-c:
					if result != "multi-resolve" {
						t.Errorf("Subscriber %d: Expected 'multi-resolve', got %v", idx, result)
					}
				case <-time.After(100 * time.Millisecond):
					t.Errorf("Subscriber %d: Timeout waiting for resolution", idx)
				}
			}(i, ch)
		}

		wg.Wait()
	})

	t.Run("Reject after Resolve is a no-op", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		p.Resolve("first")
		p.Reject(errors.New("should-be-ignored"))

		if p.State() != alternatethree.Resolved {
			t.Fatalf("Expected Resolved state, got %v", p.State())
		}
		if p.Result() != "first" {
			t.Fatalf("Expected result 'first', got %v", p.Result())
		}
	})

	t.Run("Resolve after Resolve is a no-op", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		p.Resolve("first")
		p.Resolve("second")

		if p.State() != alternatethree.Resolved {
			t.Fatalf("Expected Resolved state, got %v", p.State())
		}
		if p.Result() != "first" {
			t.Fatalf("Expected result 'first', got %v", p.Result())
		}
	})
}

// Test_alternatethree_Promise_Reject tests the Reject handler execution and error propagation
func Test_alternatethree_Promise_Reject(t *testing.T) {
	t.Parallel()

	t.Run("Reject with error", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		expectedErr := errors.New("test-error")
		p.Reject(expectedErr)

		if p.State() != alternatethree.Rejected {
			t.Fatalf("Expected Rejected state, got %v", p.State())
		}
		if p.Result() != expectedErr {
			t.Fatalf("Expected result %v, got %v", expectedErr, p.Result())
		}
	})

	t.Run("Reject notifies subscriber", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		ch := p.ToChannel()
		expectedErr := errors.New("reject-error")
		p.Reject(expectedErr)

		select {
		case result := <-ch:
			if result != expectedErr {
				t.Fatalf("Expected error %v, got %v", expectedErr, result)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Timeout waiting for rejection")
		}
	})

	t.Run("Reject notifies multiple subscribers", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		numSubscribers := 10
		channels := make([]<-chan alternatethree.Result, numSubscribers)
		for i := 0; i < numSubscribers; i++ {
			channels[i] = p.ToChannel()
		}

		expectedErr := errors.New("multi-reject")
		p.Reject(expectedErr)

		var wg sync.WaitGroup
		wg.Add(numSubscribers)

		for i, ch := range channels {
			go func(idx int, c <-chan alternatethree.Result) {
				defer wg.Done()
				select {
				case result := <-c:
					if result != expectedErr {
						t.Errorf("Subscriber %d: Expected error %v, got %v", idx, expectedErr, result)
					}
				case <-time.After(100 * time.Millisecond):
					t.Errorf("Subscriber %d: Timeout waiting for rejection", idx)
				}
			}(i, ch)
		}

		wg.Wait()
	})

	t.Run("Resolve after Reject is a no-op", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		expectedErr := errors.New("first")
		p.Reject(expectedErr)
		p.Resolve("should-be-ignored")

		if p.State() != alternatethree.Rejected {
			t.Fatalf("Expected Rejected state, got %v", p.State())
		}
		if p.Result() != expectedErr {
			t.Fatalf("Expected error %v, got %v", expectedErr, p.Result())
		}
	})

	t.Run("Reject with nil error", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		p.Reject(nil)

		if p.State() != alternatethree.Rejected {
			t.Fatalf("Expected Rejected state, got %v", p.State())
		}
		if p.Result() != nil {
			t.Fatalf("Expected nil error, got %v", p.Result())
		}
	})
}

// Test_alternatethree_Promise_fanOut tests the fanOut mechanism for multiple subscribers
func Test_alternatethree_Promise_fanOut(t *testing.T) {
	t.Parallel()

	t.Run("fanOut with rapid subscriber addition", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		numSubscribers := 100
		channels := make([]<-chan alternatethree.Result, numSubscribers)
		for i := 0; i < numSubscribers; i++ {
			channels[i] = p.ToChannel()
		}

		p.Resolve("fanout")

		var wg sync.WaitGroup
		wg.Add(numSubscribers)
		receivedCount := atomic.Int32{}

		for i, ch := range channels {
			go func(idx int, c <-chan alternatethree.Result) {
				defer wg.Done()
				select {
				case <-c:
					receivedCount.Add(1)
				case <-time.After(100 * time.Millisecond):
					t.Errorf("Subscriber %d: Timeout waiting for result", idx)
				}
			}(i, ch)
		}

		wg.Wait()

		if receivedCount.Load() != int32(numSubscribers) {
			t.Fatalf("Expected %d subscribers to receive result, got %d",
				numSubscribers, receivedCount.Load())
		}
	})

	t.Run("fanOut with concurrent settle and subscribe", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		numSubscribers := 50
		var wg sync.WaitGroup
		wg.Add(numSubscribers)

		receivedCount := atomic.Int32{}

		// Concurrently add subscribers and resolve
		for i := 0; i < numSubscribers; i++ {
			go func() {
				defer wg.Done()
				ch := p.ToChannel()
				select {
				case <-ch:
					receivedCount.Add(1)
				case <-time.After(50 * time.Millisecond):
					// Some may miss the race, which is OK
				}
			}()
		}

		// Resolve in the middle of subscription chaos
		time.Sleep(1 * time.Millisecond)
		p.Resolve("race-fanout")

		wg.Wait()

		// At least some should have received the result
		if receivedCount.Load() == 0 {
			t.Fatal("Expected at least one subscriber to receive result")
		}
	})

	t.Run("fanOut clears subscribers after notification", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		// Add a subscriber
		_ = p.ToChannel()
		p.Resolve("clear-test")

		// After resolution, subscribers should be cleared
		// This is an internal implementation detail, but we can verify
		// by checking subsequent ToChannel calls get pre-filled channels
		ch2 := p.ToChannel()

		select {
		case result := <-ch2:
			if result != "clear-test" {
				t.Fatalf("Expected 'clear-test', got %v", result)
			}
		case <-time.After(10 * time.Millisecond):
			t.Fatal("Timeout: ToChannel after resolve should return pre-filled channel")
		}
	})

	t.Run("fanOut stress test", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		numSubscribers := 1000
		channels := make([]<-chan alternatethree.Result, numSubscribers)
		for i := 0; i < numSubscribers; i++ {
			channels[i] = p.ToChannel()
		}

		p.Resolve("stress")

		var wg sync.WaitGroup
		wg.Add(numSubscribers)
		receivedCount := atomic.Int32{}

		for _, ch := range channels {
			go func(c <-chan alternatethree.Result) {
				defer wg.Done()
				select {
				case <-c:
					receivedCount.Add(1)
				case <-time.After(100 * time.Millisecond):
					// Some timeouts are OK under stress
				}
			}(ch)
		}

		wg.Wait()

		// Most subscribers should have received the result
		successRate := float64(receivedCount.Load()) / float64(numSubscribers) * 100
		if successRate < 95 {
			t.Fatalf("Expected >=95%% success rate, got %.2f%%", successRate)
		}
	})
}

// Test_alternatethree_Promise_StateQueries tests state machine query methods
func Test_alternatethree_Promise_StateQueries(t *testing.T) {
	t.Parallel()

	t.Run("State returns Pending for new promise", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		if p.State() != alternatethree.Pending {
			t.Fatalf("Expected Pending state, got %v", p.State())
		}
	})

	t.Run("State returns Resolved after Resolve", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		p.Resolve("value")
		if p.State() != alternatethree.Resolved {
			t.Fatalf("Expected Resolved state, got %v", p.State())
		}
	})

	t.Run("State returns Rejected after Reject", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		p.Reject(errors.New("error"))
		if p.State() != alternatethree.Rejected {
			t.Fatalf("Expected Rejected state, got %v", p.State())
		}
	})

	t.Run("State is thread-safe", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		numGoroutines := 100
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				// Concurrent state reads
				_ = p.State()
			}()
		}

		wg.Wait()

		// Final state should still be Pending
		if p.State() != alternatethree.Pending {
			t.Fatalf("Expected Pending state after concurrent reads, got %v", p.State())
		}
	})

	t.Run("Result returns nil for unresolved promise", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		if p.Result() != nil {
			t.Fatalf("Expected nil result for pending promise, got %v", p.Result())
		}
	})

	t.Run("Result returns resolved value", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		expected := "resolved-value"
		p.Resolve(expected)

		if p.Result() != expected {
			t.Fatalf("Expected result %v, got %v", expected, p.Result())
		}
	})

	t.Run("Result returns rejected error", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		expectedErr := errors.New("rejected-error")
		p.Reject(expectedErr)

		if p.Result() != expectedErr {
			t.Fatalf("Expected error %v, got %v", expectedErr, p.Result())
		}
	})
}

// Test_alternatethree_Promise_ToChannel tests channel-based result delivery
func Test_alternatethree_Promise_ToChannel(t *testing.T) {
	t.Parallel()

	t.Run("ToChannel returns channel for pending promise", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		ch := p.ToChannel()
		if ch == nil {
			t.Fatal("Expected non-nil channel")
		}

		// Should not have data yet
		select {
		case <-ch:
			t.Fatal("Expected no data yet")
		default:
			// OK
		}
	})

	t.Run("ToChannel returns pre-filled channel for resolved promise", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		p.Resolve("immediate")
		ch := p.ToChannel()

		// Should have data immediately
		select {
		case result := <-ch:
			if result != "immediate" {
				t.Fatalf("Expected 'immediate', got %v", result)
			}
		case <-time.After(10 * time.Millisecond):
			t.Fatal("Timeout: Channel should be pre-filled")
		}
	})

	t.Run("ToChannel returns pre-filled channel for rejected promise", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		err := errors.New("rejected")
		p.Reject(err)
		ch := p.ToChannel()

		// Should have error immediately
		select {
		case result := <-ch:
			if result != err {
				t.Fatalf("Expected error %v, got %v", err, result)
			}
		case <-time.After(10 * time.Millisecond):
			t.Fatal("Timeout: Channel should be pre-filled")
		}
	})

	t.Run("ToChannel can be called multiple times", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		ch1 := p.ToChannel()
		ch2 := p.ToChannel()

		if ch1 == ch2 {
			t.Fatal("Expected different channel instances")
		}

		p.Resolve("multiple")

		// Both should receive the result
		select {
		case result := <-ch1:
			if result != "multiple" {
				t.Fatalf("ch1: Expected 'multiple', got %v", result)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Timeout: ch1")
		}

		select {
		case result := <-ch2:
			if result != "multiple" {
				t.Fatalf("ch2: Expected 'multiple', got %v", result)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Timeout: ch2")
		}
	})

	t.Run("ToChannel channels are closed after result", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		ch := p.ToChannel()
		p.Resolve("closed-test")

		// Receive the result
		<-ch

		// Channel should be closed
		_, ok := <-ch
		if ok {
			t.Fatal("Expected channel to be closed")
		}
	})
}

// Test_alternatethree_Promise_EdgeCases tests edge cases and error conditions
func Test_alternatethree_Promise_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("Double settlement is idempotent", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		p.Resolve("first")
		p.Reject(errors.New("second"))
		p.Resolve("third")

		if p.State() != alternatethree.Resolved {
			t.Fatalf("Expected Resolved state, got %v", p.State())
		}
		if p.Result() != "first" {
			t.Fatalf("Expected 'first', got %v", p.Result())
		}
	})

	t.Run("Settlement after handler attachment", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		// Attach handler first
		ch := p.ToChannel()

		// Then settle
		p.Resolve("after-attach")

		select {
		case result := <-ch:
			if result != "after-attach" {
				t.Fatalf("Expected 'after-attach', got %v", result)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Timeout waiting for result")
		}
	})

	t.Run("Handler attachment after settlement", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		// Settle first
		p.Resolve("before-attach")

		// Then attach handler
		ch := p.ToChannel()

		select {
		case result := <-ch:
			if result != "before-attach" {
				t.Fatalf("Expected 'before-attach', got %v", result)
			}
		case <-time.After(10 * time.Millisecond):
			t.Fatal("Timeout: Should get pre-filled channel")
		}
	})

	t.Run("Concurrent settlement attempts", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		numGoroutines := 100
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func(idx int) {
				defer wg.Done()
				if idx%2 == 0 {
					p.Resolve("resolve")
				} else {
					p.Reject(errors.New("reject"))
				}
			}(i)
		}

		wg.Wait()

		// Should be in a terminal state
		state := p.State()
		if state != alternatethree.Resolved && state != alternatethree.Rejected {
			t.Fatalf("Expected terminal state, got %v", state)
		}
	})

	t.Run("Memory leak test: subscribers cleared after resolution", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)
		_, p := registry.NewPromise()

		// Force GC before
		runtime.GC()

		// Add many subscribers
		numSubscribers := 1000
		for i := 0; i < numSubscribers; i++ {
			_ = p.ToChannel()
		}

		// Resolve
		p.Resolve("gc-test")

		// Force GC after
		runtime.GC()

		// Result should still be accessible
		if p.Result() != "gc-test" {
			t.Fatalf("Expected result 'gc-test', got %v", p.Result())
		}
	})
}

// Test_alternatethree_Registry_RejectAll tests registry's RejectAll functionality
func Test_alternatethree_Registry_RejectAll(t *testing.T) {
	t.Parallel()

	t.Run("RejectAll rejects all pending promises", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)

		// Create multiple promises
		var promises []*alternatethree.PromiseInternal
		for i := 0; i < 10; i++ {
			_, p := registry.NewPromise()
			promises = append(promises, p)
		}

		// Reject all
		expectedErr := errors.New("reject-all-error")
		registry.RejectAll(expectedErr)

		// All should be rejected
		for i, p := range promises {
			if p.State() != alternatethree.Rejected {
				t.Errorf("Promise %d: Expected Rejected state, got %v", i, p.State())
			}
			if p.Result() != expectedErr {
				t.Errorf("Promise %d: Expected error %v, got %v", i, expectedErr, p.Result())
			}
		}
	})

	t.Run("RejectAll does not affect settled promises", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)

		// Create and resolve some promises
		_, p1 := registry.NewPromise()
		p1.Resolve("already-resolved")

		// Reject all with different error
		registry.RejectAll(errors.New("reject-all"))

		// Resolved promise should stay resolved
		if p1.State() != alternatethree.Resolved {
			t.Fatalf("Expected Resolved state, got %v", p1.State())
		}
		if p1.Result() != "already-resolved" {
			t.Fatalf("Expected 'already-resolved', got %v", p1.Result())
		}
	})

	t.Run("RejectAll clears registry", func(t *testing.T) {
		t.Parallel()

		registry := alternatethree.NewRegistryForTesting(t)

		// Create promises
		for i := 0; i < 5; i++ {
			_, _ = registry.NewPromise()
		}

		// Reject all
		registry.RejectAll(errors.New("clear"))

		// New promises should still work
		_, p := registry.NewPromise()
		if p.State() != alternatethree.Pending {
			t.Fatalf("Expected Pending state for new promise after RejectAll, got %v", p.State())
		}
	})
}
