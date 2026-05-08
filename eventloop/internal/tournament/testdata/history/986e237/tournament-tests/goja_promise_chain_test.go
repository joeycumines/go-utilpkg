package tournament

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestGojaPromiseChain tests deep promise chain resolution.
// This simulates chains like: Promise.resolve().then().then()...
func TestGojaPromiseChain(t *testing.T) {
	for _, impl := range Implementations() {
		t.Run(impl.Name, func(t *testing.T) {
			loop, err := impl.Factory()
			if err != nil {
				t.Fatalf("Failed to create loop: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			runDone := make(chan error, 1)
			go func() {
				runDone <- loop.Run(ctx)
			}()

			const chainDepth = 100
			var step atomic.Int64
			done := make(chan struct{})

			// Build chain by submitting tasks that submit the next
			var submitNext func(n int)
			submitNext = func(n int) {
				if n >= chainDepth {
					close(done)
					return
				}
				_ = loop.SubmitInternal(func() {
					step.Add(1)
					submitNext(n + 1)
				})
			}

			_ = loop.Submit(func() {
				submitNext(0)
			})

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatalf("Promise chain timed out at step %d", step.Load())
			}

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer shutdownCancel()
			_ = loop.Shutdown(shutdownCtx)

			<-runDone

			if step.Load() != chainDepth {
				t.Errorf("Expected chain depth %d, got %d", chainDepth, step.Load())
			}
		})
	}
}
