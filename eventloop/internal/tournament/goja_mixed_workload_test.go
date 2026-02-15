package tournament

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGojaMixedWorkload(t *testing.T) {
	for _, impl := range Implementations() {
		t.Run(impl.Name, func(t *testing.T) {
			loop, err := impl.Factory()
			if err != nil {
				t.Fatalf("Failed to create loop: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			runDone := make(chan error, 1)
			go func() {
				runDone <- loop.Run(ctx)
			}()

			var wg sync.WaitGroup
			var externalCount, internalCount atomic.Int64

			for range 10 {
				wg.Go(func() {
					for range 100 {
						_ = loop.Submit(func() {
							externalCount.Add(1)
							_ = loop.SubmitInternal(func() {
								internalCount.Add(1)
							})
						})
						time.Sleep(time.Millisecond)
					}
				})
			}

			wg.Wait()

			deadline := time.Now().Add(5 * time.Second)
			for (externalCount.Load() < 1000 || internalCount.Load() < 1000) && time.Now().Before(deadline) {
				time.Sleep(50 * time.Millisecond)
			}

			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer shutdownCancel()
			_ = loop.Shutdown(shutdownCtx)

			<-runDone

			if externalCount.Load() < 1000 {
				t.Errorf("Expected 1000 external tasks, got %d", externalCount.Load())
			}
			if internalCount.Load() < 1000 {
				t.Errorf("Expected 1000 internal tasks, got %d", internalCount.Load())
			}
		})
	}
}
