package eventloop

import (
	"math"
	"testing"
	"time"
)

func TestTPSCounterTimestampDiscontinuityResetsWindow(t *testing.T) {
	anchor := time.Unix(0, 0)
	tests := []struct {
		name string
		now  time.Time
	}{
		{name: "backward one bucket", now: anchor.Add(-100 * time.Millisecond)},
		{name: "exactly one window forward", now: anchor.Add(time.Second)},
		{name: "maximum duration forward", now: anchor.Add(time.Duration(math.MaxInt64))},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			counter := newTPSCounterAt(time.Second, 100*time.Millisecond, anchor)
			for index := range counter.buckets {
				counter.buckets[index].Store(int64(index + 1))
			}
			counter.current.Store(3)

			if got := counter.tpsAt(test.now); got != 0 {
				t.Fatalf("TPS after discontinuity = %v, want 0", got)
			}
			if got := counter.current.Load(); got != 0 {
				t.Fatalf("current bucket after discontinuity = %d, want 0", got)
			}
			if got := counter.lastRotation.Load().(time.Time); !got.Equal(test.now) {
				t.Fatalf("last rotation after discontinuity = %v, want %v", got, test.now)
			}
			for index := range counter.buckets {
				if got := counter.buckets[index].Load(); got != 0 {
					t.Fatalf("bucket %d after discontinuity = %d, want 0", index, got)
				}
			}
		})
	}
}

func TestTPSCounterBucketSaturates(t *testing.T) {
	anchor := time.Unix(1_700_000_000, 0)
	counter := newTPSCounterAt(time.Second, time.Second, anchor)
	counter.buckets[0].Store(math.MaxInt64 - 1)

	counter.IncrementAt(anchor)
	counter.IncrementAt(anchor)
	if got := counter.buckets[0].Load(); got != math.MaxInt64 {
		t.Fatalf("saturated bucket = %d, want %d", got, int64(math.MaxInt64))
	}
}
