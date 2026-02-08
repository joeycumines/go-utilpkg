//go:build linux || darwin

package tournament

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestGojaNestedTimeouts tests setTimeout scheduling setTimeout.
func TestGojaNestedTimeouts(t *testing.T) {
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

			const depth = 50
			var step atomic.Int64
			done := make(chan struct{})

			// Each task schedules the next via SubmitInternal (simulating timer)
			var scheduleNext func(n int)
			scheduleNext = func(n int) {
				if n >= depth {
					close(done)
					return
				}
				_ = loop.SubmitInternal(func() {
					step.Add(1)
					scheduleNext(n + 1)
				})
			}

			_ = loop.Submit(func() {
				scheduleNext(0)
			})

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatalf("Nested timeouts timed out at step %d", step.Load())
			}

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer shutdownCancel()
			_ = loop.Shutdown(shutdownCtx)

			<-runDone

			if step.Load() != depth {
				t.Errorf("Expected depth %d, got %d", depth, step.Load())
			}
		})
	}
}
