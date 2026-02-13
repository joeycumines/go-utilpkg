package eventloop

import (
	"context"
	"testing"
	"time"
)

func TestToChannel_Simple(t *testing.T) {
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

	// Get channel before resolving
	ch := p.ToChannel()

	// Resolve asynchronously
	go func() {
		time.Sleep(10 * time.Millisecond)
		resolve("value")
	}()

	// Should receive value
	select {
	case v := <-ch:
		if v != "value" {
			t.Errorf("Expected 'value', got: %v", v)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timed out waiting for value")
	}
}

func TestToChannel_AlreadyResolved(t *testing.T) {
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

	// Resolve first
	resolve("value")

	// Get channel after resolving
	ch := p.ToChannel()

	// Should receive value immediately
	select {
	case v := <-ch:
		if v != "value" {
			t.Errorf("Expected 'value', got: %v", v)
		}
	default:
		t.Error("Channel should have value (non-blocking)")
	}
}

func TestToChannel_MultipleCalls(t *testing.T) {
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

	// Get multiple channels
	ch1 := p.ToChannel()
	ch2 := p.ToChannel()
	ch3 := p.ToChannel()

	// Resolve
	resolve("value")

	// All channels should receive the value
	received := 0
	for i, ch := range []<-chan any{ch1, ch2, ch3} {
		select {
		case v := <-ch:
			if v != "value" {
				t.Errorf("Channel %d: Expected 'value', got: %v", i, v)
			} else {
				received++
			}
		case <-time.After(2 * time.Second):
			t.Errorf("Channel %d: Timed out waiting for value", i)
		}
	}

	if received != 3 {
		t.Errorf("Expected 3 values received, got: %d", received)
	}
}

// TestToChannel_Standalone tests ToChannel on a standalone promise (no JS adapter).
// This exercises the toChannelStandalonePromise path.
func TestToChannel_Standalone(t *testing.T) {
	// Create a standalone ChainedPromise without a JS adapter
	p := &ChainedPromise{}
	p.state.Store(int32(Pending))

	ch := p.ToChannel()

	// Resolve the promise
	go func() {
		time.Sleep(10 * time.Millisecond)
		p.resolve("standalone-value")
	}()

	select {
	case v := <-ch:
		if v != "standalone-value" {
			t.Errorf("Expected 'standalone-value', got: %v", v)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timed out waiting for standalone value")
	}
}

// TestToChannel_StandaloneAlreadyResolved tests ToChannel on a standalone
// promise that is already resolved before ToChannel is called.
func TestToChannel_StandaloneAlreadyResolved(t *testing.T) {
	p := &ChainedPromise{}
	p.state.Store(int32(Pending))
	p.resolve("already-done")

	ch := p.ToChannel()

	select {
	case v := <-ch:
		if v != "already-done" {
			t.Errorf("Expected 'already-done', got: %v", v)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timed out waiting for already-resolved value")
	}
}

// TestToChannel_StandaloneRejected tests ToChannel on a standalone promise
// that is rejected.
func TestToChannel_StandaloneRejected(t *testing.T) {
	p := &ChainedPromise{}
	p.state.Store(int32(Pending))

	ch := p.ToChannel()

	// Reject the promise
	go func() {
		time.Sleep(10 * time.Millisecond)
		p.reject("rejection-reason")
	}()

	select {
	case v := <-ch:
		if v != "rejection-reason" {
			t.Errorf("Expected 'rejection-reason', got: %v", v)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timed out waiting for rejection value")
	}
}
