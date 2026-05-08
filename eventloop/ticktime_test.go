package eventloop

import (
	"testing"
	"time"
)

func TestCurrentTickTimeMonotonicAndZeroAllocations(t *testing.T) {
	loop := New()
	registerLoopCleanupT(t, loop)

	anchor := time.Now()
	loop.setTickAnchor(anchor)
	allocations := testing.AllocsPerRun(1000, func() {
		currentAnchor := loop.tickAnchorTime()
		loop.tickElapsedTime.Store(time.Since(currentAnchor).Nanoseconds())
		_ = loop.CurrentTickTime()
	})
	if allocations != 0 {
		t.Fatalf("tick-time update and read allocations = %f, want 0", allocations)
	}

	loop.tickElapsedTime.Store(int64(10 * time.Millisecond))
	first := loop.CurrentTickTime()
	loop.tickElapsedTime.Store(int64(20 * time.Millisecond))
	second := loop.CurrentTickTime()
	if want := anchor.Add(10 * time.Millisecond); !first.Equal(want) {
		t.Fatalf("first tick time = %v, want %v", first, want)
	}
	if want := anchor.Add(20 * time.Millisecond); !second.Equal(want) {
		t.Fatalf("second tick time = %v, want %v", second, want)
	}
	if !second.After(first) {
		t.Fatalf("tick time did not advance: first=%v second=%v", first, second)
	}
}
