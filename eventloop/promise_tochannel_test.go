package eventloop

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestPromiseToChannel_FulfilledPromise tests ToChannel with fulfilled promise
func TestPromiseToChannel_FulfilledPromise(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()

	// Resolve before getting channel
	resolve("value")

	result := p.ToChannel()

	// Channel should have value immediately
	select {
	case v := <-result:
		if v != "value" {
			t.Errorf("Expected 'value', got: %v", v)
		}
	default:
		t.Error("Channel should have value (non-blocking)")
	}
}

// TestPromiseToChannel_RejectedPromise tests ToChannel with rejected promise
func TestPromiseToChannel_RejectedPromise(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, _, reject := js.NewChainedPromise()

	// Reject before getting channel
	reject(errors.New("error"))

	result := p.ToChannel()

	// Channel should have error immediately
	select {
	case v := <-result:
		err, ok := v.(error)
		if !ok {
			t.Errorf("Expected error, got: %T", v)
		} else if err.Error() != "error" {
			t.Errorf("Expected 'error', got: %s", err.Error())
		}
	default:
		t.Error("Channel should have error (non-blocking)")
	}
}

// TestPromiseToChannel_PendingPromise tests ToChannel with pending promise
func TestPromiseToChannel_PendingPromise(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()

	// Get channel before resolution
	result := p.ToChannel()

	// Channel should block (pending promise)
	select {
	case <-result:
		t.Error("Channel should block for pending promise")
	case <-time.After(100 * time.Millisecond):
		// Expected: timeout means channel is blocking
	}

	// Now resolve
	resolve("value")

	// Channel should now have value
	select {
	case v := <-result:
		if v != "value" {
			t.Errorf("Expected 'value', got: %v", v)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Channel should have value after resolution")
	}
}

// TestPromiseToChannel_BlockingBehavior tests blocking behavior of ToChannel
func TestPromiseToChannel_BlockingBehavior(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()

	// Get channel before resolution (pending promise)
	result := p.ToChannel()

	// First read should block since promise is pending
	select {
	case <-result:
		t.Error("Channel should block for pending promise")
	case <-time.After(100 * time.Millisecond):
		// Expected: timeout means channel is blocking
	}

	// Resolve the promise
	resolve("blocking test")
	loop.tick()

	// First read should succeed
	v1 := <-result
	if v1 != "blocking test" {
		t.Errorf("Expected 'blocking test', got: %v", v1)
	}

	// Second read should return zero value (channel closed after settlement)
	v2 := <-result
	if v2 != nil {
		t.Errorf("Expected nil (channel closed), got: %v", v2)
	}

	// Verify channel is closed by checking ok value
	select {
	case _, ok := <-result:
		if ok {
			t.Error("Channel should be closed after settlement")
		}
	default:
		t.Error("Channel should be closed (check with ok)")
	}
}

// TestPromiseToChannel_ConcurrentReads tests concurrent reads from ToChannel
func TestPromiseToChannel_ConcurrentReads(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()

	result := p.ToChannel()

	// Resolve after a delay
	go func() {
		time.Sleep(50 * time.Millisecond)
		resolve("concurrent")
	}()

	// Multiple concurrent reads
	readCount := 0
	for range 5 {
		go func() {
			select {
			case <-result:
				readCount++
			default:
				// Channel might be empty or closed
			}
		}()
	}

	time.Sleep(100 * time.Millisecond)

	// Just verify it doesn't hang
	if p.State() != Fulfilled {
		t.Error("Promise should have been fulfilled")
	}
}

// TestPromiseToChannel_ChannelClosed tests that channel is closed after settlement
func TestPromiseToChannel_ChannelClosed(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()

	result := p.ToChannel()

	resolve("value")
	loop.tick()

	// First read gets the value
	v, ok := <-result
	if !ok {
		t.Error("First read should get the value")
	}
	if v != "value" {
		t.Errorf("Expected 'value', got: %v", v)
	}

	// Second read should indicate channel is closed
	_, ok = <-result
	if ok {
		t.Error("Channel should be closed after first read")
	}
}

// TestPromiseToChannel_AlreadySettledPromise tests ToChannel with already-settled promise
func TestPromiseToChannel_AlreadySettledPromise(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()

	// Settle first
	resolve("already settled")

	// Get channel after settlement
	result := p.ToChannel()

	// Should have value immediately
	select {
	case v := <-result:
		if v != "already settled" {
			t.Errorf("Expected 'already settled', got: %v", v)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Channel should have value (already settled)")
	}
}

// TestPromiseToChannel_NilResult tests ToChannel with nil result
func TestPromiseToChannel_NilResult(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()

	resolve(nil)

	result := p.ToChannel()

	// Should have nil value
	select {
	case v := <-result:
		if v != nil {
			t.Errorf("Expected nil, got: %v", v)
		}
	default:
		t.Error("Channel should have nil value (non-blocking)")
	}
}

// TestPromiseToChannel_ErrorResult tests ToChannel with error result
func TestPromiseToChannel_ErrorResult(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, _, reject := js.NewChainedPromise()

	reject(errors.New("test error"))

	result := p.ToChannel()

	// Should have error
	select {
	case v := <-result:
		err, ok := v.(error)
		if !ok {
			t.Errorf("Expected error, got: %T", v)
		} else if err.Error() != "test error" {
			t.Errorf("Expected 'test error', got: %s", err.Error())
		}
	default:
		t.Error("Channel should have error (non-blocking)")
	}
}

// TestPromiseToChannel_TerminatedLoop tests ToChannel on terminated loop
func TestPromiseToChannel_TerminatedLoop(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()
	<-done

	js, _ := NewJS(loop)
	p, _, _ := js.NewChainedPromise()

	result := p.ToChannel()

	// Should handle gracefully
	if result == nil {
		t.Error("Channel should not be nil")
	}

	loop.Shutdown(context.Background())
}

// TestPromiseToChannel_ChainedPromise tests ToChannel with chained promise
func TestPromiseToChannel_ChainedPromise(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()

	// Create chain
	chained := p.Then(func(v any) any {
		return v.(string) + "-chained"
	}, nil)

	result := chained.ToChannel()

	resolve("original")
	loop.tick()

	// Should have chained value
	select {
	case v := <-result:
		if v != "original-chained" {
			t.Errorf("Expected 'original-chained', got: %v", v)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Channel should have chained value")
	}
}

// TestPromiseToChannel_MultipleToChannelCalls tests multiple ToChannel calls on same promise
func TestPromiseToChannel_MultipleToChannelCalls(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	p, resolve, _ := js.NewChainedPromise()

	// Multiple ToChannel calls
	ch1 := p.ToChannel()
	ch2 := p.ToChannel()
	ch3 := p.ToChannel()

	resolve("multiple channels")
	loop.tick()

	// All channels should have the same value
	select {
	case v1 := <-ch1:
		if v1 != "multiple channels" {
			t.Errorf("ch1: Expected 'multiple channels', got: %v", v1)
		}
	default:
		t.Error("ch1 should have value")
	}

	select {
	case v2 := <-ch2:
		if v2 != "multiple channels" {
			t.Errorf("ch2: Expected 'multiple channels', got: %v", v2)
		}
	default:
		t.Error("ch2 should have value")
	}

	select {
	case v3 := <-ch3:
		if v3 != "multiple channels" {
			t.Errorf("ch3: Expected 'multiple channels', got: %v", v3)
		}
	default:
		t.Error("ch3 should have value")
	}
}
