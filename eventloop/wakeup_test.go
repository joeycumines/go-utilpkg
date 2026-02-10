package eventloop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestWakeup_HighContention verifies that under high contention from multiple
// producers, all tasks are eventually executed with no lost wake-ups.
func TestWakeup_HighContention(t *testing.T) {
	loop, err := New()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	runDone := make(chan struct{})
	errChan := make(chan error, 1)
	go func() {
		if err := loop.Run(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrLoopTerminated) {
			errChan <- err
			return
		}
		close(runDone)
	}()

	defer func() {
		loop.Shutdown(context.Background())
		select {
		case <-runDone:
		case err := <-errChan:
			t.Fatal(err)
		}
	}()

	const producers = 100
	const tasksPerProducer = 1000
	var executed atomic.Int64

	var wg sync.WaitGroup
	wg.Add(producers)

	for p := 0; p < producers; p++ {
		go func() {
			defer wg.Done()
			for i := 0; i < tasksPerProducer; i++ {
				loop.Submit(func() {
					executed.Add(1)
				})
				if i%100 == 0 {
					time.Sleep(time.Microsecond)
				}
			}
		}()
	}

	wg.Wait()

	deadline := time.Now().Add(5 * time.Second)
	expected := int64(producers * tasksPerProducer)
	for time.Now().Before(deadline) {
		if executed.Load() == expected {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("WAKEUP LOSS: Only %d/%d tasks executed", executed.Load(), expected)
}
