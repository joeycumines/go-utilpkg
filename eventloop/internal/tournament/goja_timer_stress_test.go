package tournament

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestGojaTimerStress simulates heavy JavaScript timer usage.
// This tests rapid setTimeout/clearTimeout cycles.
func TestGojaTimerStress(t *testing.T) {
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

			var executed atomic.Int64
			const taskCount = 1000

			// Submit burst of tasks
			for i := 0; i < taskCount; i++ {
				if err := loop.Submit(func() {
					executed.Add(1)
				}); err != nil {
					t.Errorf("Submit failed: %v", err)
				}
			}

			// Wait for all tasks to execute
			deadline := time.Now().Add(5 * time.Second)
			for executed.Load() < taskCount && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer shutdownCancel()
			if err := loop.Shutdown(shutdownCtx); err != nil {
				t.Errorf("Shutdown failed: %v", err)
			}

			<-runDone

			if executed.Load() != taskCount {
				t.Errorf("Expected %d tasks, got %d", taskCount, executed.Load())
			}
		})
	}
}
