package eventloop

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestTickTimeDataRace proves the unsafe access to tickTime.
// RUN WITH: go test -race -v -run TestTickTimeDataRace
func TestTickTimeDataRace(t *testing.T) {
	l, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan struct{})
	go func() {
		if err := l.Run(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrLoopTerminated) {
			t.Errorf("Run() unexpected error: %v", err)
		}
		close(runDone)
	}()

	// Submit tasks to force high-frequency tick updates
	go func() {
		for range 1000 {
			l.Submit(func() {})
			time.Sleep(10 * time.Microsecond)
		}
		cancel()
	}()

	// Concurrent Reader (simulates user querying from HTTP handler)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ctx.Done():
				close(done)
				return
			default:
				_ = l.CurrentTickTime()
			}
		}
	}()

	<-done
	l.Shutdown(context.Background())
	<-runDone
}
