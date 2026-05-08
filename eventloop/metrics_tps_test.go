package eventloop

import (
	"testing"
	"time"
)

func TestNewTPSCounterContracts(t *testing.T) {
	anchor := time.Unix(1_700_000_000, 123)
	tests := []struct {
		name       string
		window     time.Duration
		bucket     time.Duration
		wantPanic  string
		wantBucket int
	}{
		{name: "balanced", window: 10 * time.Second, bucket: 100 * time.Millisecond, wantBucket: 100},
		{name: "single bucket", window: time.Second, bucket: time.Second, wantBucket: 1},
		{name: "truncated monitored window", window: 1050 * time.Millisecond, bucket: 100 * time.Millisecond, wantBucket: 10},
		{name: "zero window", bucket: time.Millisecond, wantPanic: "eventloop: windowSize must be positive (use > 0 duration)"},
		{name: "negative window", window: -time.Second, bucket: time.Millisecond, wantPanic: "eventloop: windowSize must be positive (use > 0 duration)"},
		{name: "zero bucket", window: time.Second, wantPanic: "eventloop: bucketSize must be positive (use > 0 duration)"},
		{name: "negative bucket", window: time.Second, bucket: -time.Millisecond, wantPanic: "eventloop: bucketSize must be positive (use > 0 duration)"},
		{name: "bucket exceeds window", window: time.Millisecond, bucket: time.Second, wantPanic: "eventloop: bucketSize cannot exceed windowSize (use <= windowSize)"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.wantPanic != "" {
				defer func() {
					if got := recover(); got != test.wantPanic {
						t.Fatalf("panic = %#v, want %q", got, test.wantPanic)
					}
				}()
			}

			counter := newTPSCounterAt(test.window, test.bucket, anchor)
			if test.wantPanic != "" {
				t.Fatal("newTPSCounterAt did not panic")
			}
			if len(counter.buckets) != test.wantBucket {
				t.Fatalf("bucket count = %d, want %d", len(counter.buckets), test.wantBucket)
			}
			if counter.bucketSize != test.bucket || counter.current.Load() != 0 {
				t.Fatalf("counter configuration = (bucket %v, current %d)", counter.bucketSize, counter.current.Load())
			}
			if got := counter.lastRotation.Load().(time.Time); !got.Equal(anchor) {
				t.Fatalf("anchor = %v, want %v", got, anchor)
			}
			if got := counter.tpsAt(anchor); got != 0 {
				t.Fatalf("initial TPS = %v, want 0", got)
			}
		})
	}
}

func TestTPSCounterRollingWindow(t *testing.T) {
	anchor := time.Unix(1_700_000_000, 0)
	counter := newTPSCounterAt(time.Second, 100*time.Millisecond, anchor)

	for range 4 {
		counter.IncrementAt(anchor)
	}
	if got := counter.tpsAt(anchor); got != 4 {
		t.Fatalf("immediate TPS = %v, want 4", got)
	}

	secondBucket := anchor.Add(250 * time.Millisecond)
	for range 6 {
		counter.IncrementAt(secondBucket)
	}
	if got := counter.tpsAt(anchor.Add(999 * time.Millisecond)); got != 10 {
		t.Fatalf("TPS before first bucket expiration = %v, want 10", got)
	}
	if got := counter.tpsAt(anchor.Add(time.Second)); got != 6 {
		t.Fatalf("TPS at first bucket expiration = %v, want 6", got)
	}
	if got := counter.tpsAt(anchor.Add(1199 * time.Millisecond)); got != 6 {
		t.Fatalf("TPS before second bucket expiration = %v, want 6", got)
	}
	if got := counter.tpsAt(anchor.Add(1200 * time.Millisecond)); got != 0 {
		t.Fatalf("TPS at complete expiration = %v, want 0", got)
	}
}
