package eventloop

import (
	"sync"
	"testing"
	"time"
)

func TestPromiseFanOut(t *testing.T) {
	// Task 4.2, 4.3: Verify multiple subscribers get result
	r := newRegistry()
	_, p := r.NewPromise()

	const numSubscribers = 10
	var wg sync.WaitGroup
	wg.Add(numSubscribers)

	results := make([]any, numSubscribers)

	for i := 0; i < numSubscribers; i++ {
		go func(idx int) {
			defer wg.Done()
			ch := p.ToChannel()
			res := <-ch
			results[idx] = res
		}(i)
	}

	// Wait a bit to ensure subscribers are subscribed (though valid even if race happens)
	time.Sleep(10 * time.Millisecond)

	// Resolve
	expected := "success"
	p.Resolve(expected)

	wg.Wait()

	for i, res := range results {
		if res != expected {
			t.Errorf("Subscriber %d got %v, expected %v", i, res, expected)
		}
	}

	// Ensure p.subscribers is cleared
	p.mu.Lock()
	if len(p.subscribers) != 0 {
		t.Error("Subscribers list not cleared after fan-out")
	}
	p.mu.Unlock()
}

func TestPromiseLateBinding(t *testing.T) {
	// Task 4.4: Verify late binding works
	r := newRegistry()
	_, p := r.NewPromise()

	expected := "late"
	p.Resolve(expected)

	// ToChannel AFTER resolve
	ch := p.ToChannel()

	select {
	case res := <-ch:
		if res != expected {
			t.Errorf("Got %v, expected %v", res, expected)
		}
		// Verify channel is closed
		_, ok := <-ch
		if ok {
			t.Error("Channel should be closed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for late binding result")
	}
}

func TestToChannelIdentity(t *testing.T) {
	// Task 4.2: Multiple calls create different channels
	r := newRegistry()
	_, p := r.NewPromise()

	ch1 := p.ToChannel()
	ch2 := p.ToChannel()

	if ch1 == ch2 {
		t.Error("ToChannel returned same channel for multiple calls")
	}
}
