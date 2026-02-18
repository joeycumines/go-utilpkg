package tournament

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestGojaImmediateBurst tests handling of setImmediate-style bursts.
func TestGojaImmediateBurst(t *testing.T) {
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

			const burstSize = 10000
			var executed atomic.Int64

			// Submit massive burst
			for range burstSize {
				if err := loop.Submit(func() {
					executed.Add(1)
				}); err != nil {
					break
				}
			}

			// Wait for drain
			deadline := time.Now().Add(5 * time.Second)
			for executed.Load() < burstSize && time.Now().Before(deadline) {
				time.Sleep(50 * time.Millisecond)
			}

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer shutdownCancel()
			_ = loop.Shutdown(shutdownCtx)

			<-runDone

			// Allow some tolerance due to timing
			if executed.Load() < burstSize*95/100 {
				t.Errorf("Expected ~%d tasks, got %d", burstSize, executed.Load())
			}
		})
	}
}
