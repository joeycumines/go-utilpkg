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
	case <-time.After(100 * time.Millisecond):
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
	for i, ch := range []<-chan Result{ch1, ch2, ch3} {
		select {
		case v := <-ch:
			if v != "value" {
				t.Errorf("Channel %d: Expected 'value', got: %v", i, v)
			} else {
				received++
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("Channel %d: Timed out waiting for value", i)
		}
	}

	if received != 3 {
		t.Errorf("Expected 3 values received, got: %d", received)
	}
}
