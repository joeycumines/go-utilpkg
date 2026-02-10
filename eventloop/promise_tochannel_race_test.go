package eventloop

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestPromiseToChannel_DoubleCheckPath exercises the rarely-hit TOCTOU race
// path in ChainedPromise.ToChannel(), where the promise settles between the
// optimistic state check and the lock acquisition (double-check under lock).
// With 2000 iterations of concurrent resolve+ToChannel, the race window is
// virtually guaranteed to be hit at least once on most architectures.
func TestPromiseToChannel_DoubleCheckPath(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	const iterations = 2000
	for i := 0; i < iterations; i++ {
		p, resolve, _ := js.NewChainedPromise()

		var wg sync.WaitGroup
		wg.Add(1)

		// Goroutine resolves the promise — racing with ToChannel
		go func() {
			defer wg.Done()
			resolve("value")
		}()

		// Call ToChannel concurrently with resolve.
		// Depending on scheduling, this may hit:
		//   (a) the optimistic fast path (already settled)
		//   (b) the pending path (stores channel, notified later)
		//   (c) the double-check path (settles between check and lock)
		ch := p.ToChannel()

		// Wait for resolve to complete
		wg.Wait()

		// Regardless of which path was taken, the result MUST be correct
		select {
		case v := <-ch:
			if v != "value" {
				t.Fatalf("Iteration %d: Expected 'value', got: %v", i, v)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("Iteration %d: Timed out waiting for ToChannel result", i)
		}
	}
}

// TestPromiseToChannel_DoubleCheckReject is the reject counterpart of the
// TOCTOU race path test, ensuring the double-check path works for rejected
// promises too.
func TestPromiseToChannel_DoubleCheckReject(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer loop.Shutdown(context.Background())

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	const iterations = 2000
	for i := 0; i < iterations; i++ {
		p, _, reject := js.NewChainedPromise()

		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			defer wg.Done()
			reject(errors.New("err"))
		}()

		ch := p.ToChannel()
		wg.Wait()

		select {
		case v := <-ch:
			e, ok := v.(error)
			if !ok || e.Error() != "err" {
				t.Fatalf("Iteration %d: Expected error 'err', got: %v", i, v)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("Iteration %d: Timed out waiting for ToChannel result", i)
		}
	}
}

// TestPromiseToChannel_MultipleChannelsRace tests that multiple ToChannel()
// calls racing with settlement all receive the correct value.
func TestPromiseToChannel_MultipleChannelsRace(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	defer loop.Shutdown(ctx)

	js, err := NewJS(loop)
	if err != nil {
		t.Fatal(err)
	}

	const iterations = 500
	const channelsPerPromise = 5

	for i := 0; i < iterations; i++ {
		p, resolve, _ := js.NewChainedPromise()

		var wg sync.WaitGroup
		channels := make([]<-chan Result, channelsPerPromise)

		// Launch multiple goroutines calling ToChannel concurrently
		for j := 0; j < channelsPerPromise; j++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				channels[idx] = p.ToChannel()
			}(j)
		}

		// Also resolve concurrently
		wg.Add(1)
		go func() {
			defer wg.Done()
			resolve("multi")
		}()

		wg.Wait()

		// All channels must receive the value
		for j, ch := range channels {
			select {
			case v := <-ch:
				if v != "multi" {
					t.Fatalf("Iteration %d, channel %d: Expected 'multi', got: %v", i, j, v)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("Iteration %d, channel %d: Timed out", i, j)
			}
		}
	}
}
