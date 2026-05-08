package tournament

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestSequentialExternalRoundTrip verifies that every accepted external task
// completes across repeated idle handoffs. The portable adapter has no
// poll-entry hook, so this test intentionally makes no kernel-wakeup claim.
func TestSequentialExternalRoundTrip(t *testing.T) {
	for _, impl := range Implementations() {
		t.Run(impl.Name, func(t *testing.T) {
			loop, cleanup := startTournamentTestLoop(t, impl)
			for iteration := range 100 {
				done := make(chan struct{})
				if err := loop.Submit(func() { close(done) }); err != nil {
					t.Fatalf("Submit iteration %d: %v", iteration, err)
				}
				waitTournamentSignal(t, done, fmt.Sprintf("external round trip %d", iteration))
			}
			cleanup()
		})
	}
}

// TestConcurrentExternalRoundTrip verifies exact round-trip conservation under
// concurrent producers without guessing when an implementation enters poll.
func TestConcurrentExternalRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping concurrent round-trip stress in short mode")
	}

	const producerCount = 10
	const iterationsPerProducer = 50
	const operationCount = producerCount * iterationsPerProducer
	for _, impl := range Implementations() {
		t.Run(impl.Name, func(t *testing.T) {
			loop, cleanup := startTournamentTestLoop(t, impl)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			results := make(chan error, operationCount)
			for producer := range producerCount {
				go func(producer int) {
					for iteration := range iterationsPerProducer {
						done := make(chan struct{})
						if err := loop.Submit(func() { close(done) }); err != nil {
							results <- fmt.Errorf("producer %d iteration %d Submit: %w", producer, iteration, err)
							continue
						}
						select {
						case <-done:
							results <- nil
						case <-ctx.Done():
							results <- fmt.Errorf("producer %d iteration %d completion: %w", producer, iteration, ctx.Err())
						}
					}
				}(producer)
			}

			for completed := range operationCount {
				select {
				case err := <-results:
					if err != nil {
						t.Error(err)
					}
				case <-ctx.Done():
					t.Fatalf("round trips completed %d of %d: %v", completed, operationCount, ctx.Err())
				}
			}
			cleanup()
		})
	}
}
