package eventloop

import (
	"testing"
	"time"
)

func TestTPSCounterConcurrentIncrementConservation(t *testing.T) {
	anchor := time.Unix(1_700_000_000, 0)
	counter := newTPSCounterAt(time.Second, 100*time.Millisecond, anchor)
	const producers = 128
	ready := make(chan struct{}, producers)
	start := make(chan struct{})
	done := make(chan struct{}, producers)
	for range producers {
		go func() {
			ready <- struct{}{}
			<-start
			counter.IncrementAt(anchor)
			done <- struct{}{}
		}()
	}
	for range producers {
		waitContractSignal(t, ready, "TPS producer readiness")
	}
	close(start)
	for range producers {
		waitContractSignal(t, done, "TPS producer completion")
	}

	if got := counter.tpsAt(anchor); got != producers {
		t.Fatalf("concurrent TPS = %v, want %d", got, producers)
	}
	if got := counter.buckets[0].Load(); got != producers {
		t.Fatalf("concurrent bucket count = %d, want %d", got, producers)
	}
}
